package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/service/picture_generate"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// TaskChainService 处理两步固定链（图→视频）的编排
type TaskChainService struct {
	gdb         *gorm.DB
	taskDao     *dao.PictureTaskDao
	chainDao    *dao.TaskChainDao
	picForgeDao *dao.PictureForgeDao
	rdb         *redis.Client
	configSvc   *ConfigService
	creditSvc   credit.Service
	submitFunc  func(ctx context.Context, req *pb.SubmitPictureForgeTaskRequest) (*pb.SubmitPictureForgeTaskResponse, error)
}

func NewTaskChainService(gdb *gorm.DB, repos *dao.Repositories, rdb *redis.Client, cfg *ConfigService, creditSvc credit.Service) *TaskChainService {
	return &TaskChainService{
		gdb:         gdb,
		taskDao:     repos.Task,
		chainDao:    dao.NewTaskChainDao(gdb),
		picForgeDao: repos.PicForge,
		rdb:         rdb,
		configSvc:   cfg,
		creditSvc:   creditSvc,
	}
}

// SetSubmitFunc 注入提交任务的方法（避免与API或服务层循环依赖）
func (s *TaskChainService) SetSubmitFunc(f func(ctx context.Context, req *pb.SubmitPictureForgeTaskRequest) (*pb.SubmitPictureForgeTaskResponse, error)) {
	s.submitFunc = f
}

type chainConfig struct {
	Steps []struct {
		Index        int               `json:"index"`
		Type         string            `json:"type"`
		WorkflowID   string            `json:"workflow_id"`
		ParamMapping map[string]string `json:"param_mapping"`
	} `json:"steps"`
}

func (s *TaskChainService) CreateChain(ctx context.Context, firstTaskID, prompt string) (*model.TaskChain, error) {
	cfg := chainConfig{Steps: []struct {
		Index        int               `json:"index"`
		Type         string            `json:"type"`
		WorkflowID   string            `json:"workflow_id"`
		ParamMapping map[string]string `json:"param_mapping"`
	}{
		{Index: 1, Type: "image"},
		{Index: 2, Type: "video", WorkflowID: s.getPictureToVideoWorkflowID(ctx), ParamMapping: map[string]string{
			"LoadImage1": "$.result_url",
			"prompt":     "$.meta.prompt?",
		}},
	}}
	cfgBytes, _ := json.Marshal(cfg)
	chain := &model.TaskChain{
		ChainID:     uuid.NewString(),
		RootTaskID:  firstTaskID,
		UserID:      common.GetUserID(ctx),
		ProjectID:   common.GetProjectID(ctx),
		Status:      0,
		CurrentStep: 1,
		ConfigJSON:  string(cfgBytes),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := s.chainDao.Create(ctx, s.gdb, chain); err != nil {
		return nil, err
	}
	return chain, nil
}

func (s *TaskChainService) OnTaskCompleted(ctx context.Context, task *model.PictureTask) {
	chain, err := s.chainDao.GetByRootTask(ctx, task.TaskID)
	if err != nil || chain == nil {
		return // 不是链任务，正常返回
	}

	// 快速返回：已推进或已完成则无需重复触发
	if chain.Status == 2 || chain.Status == 3 || chain.CurrentStep != 1 {
		return
	}

	// 防重复激活锁
	lockKey := "visionai:task_chain:activate:" + chain.ChainID
	if !s.rdb.SetNX(lockKey, "1", 2*time.Minute).Val() {
		return
	}
	defer s.rdb.Del(lockKey)

	// 检查任务结果
	if task.ResultJSON == "" {
		zlog.LogWithContext(ctx).Error("empty result json for root task", zap.String("task_id", task.TaskID))
		chain.Status = 3 // FAILED
		_ = s.chainDao.Update(ctx, chain)
		return
	}

	// 解析任务ID列表
	var allTaskIDs []string
	if err := json.Unmarshal([]byte(chain.AllTaskIDs), &allTaskIDs); err != nil {
		zlog.LogWithContext(ctx).Error("failed to unmarshal all task ids", zap.Error(err))
		chain.Status = 3 // FAILED
		_ = s.chainDao.Update(ctx, chain)
		return
	}

	// 激活第二步任务
	if len(allTaskIDs) > 1 {
		secondTaskID := allTaskIDs[1]
		if err := s.ActivateNextTask(ctx, secondTaskID, task.ResultJSON); err != nil {
			zlog.LogWithContext(ctx).Error("failed to activate next task",
				zap.String("chain_id", chain.ChainID),
				zap.String("second_task_id", secondTaskID),
				zap.Error(err))
			chain.Status = 3 // FAILED
			_ = s.chainDao.Update(ctx, chain)
			return
		}
	}

	// 更新链状态
	chain.Status = 1 // RUNNING
	chain.CurrentStep = 2
	_ = s.chainDao.Update(ctx, chain)

	zlog.LogWithContext(ctx).Info("task chain step completed and next task activated",
		zap.String("chain_id", chain.ChainID),
		zap.String("first_task_id", task.TaskID))
}

func (s *TaskChainService) getPictureToVideoWorkflowID(ctx context.Context) string {
	id, err := s.configSvc.GetStringConfig(constants.ConfigKeyPictureToVideoGuideWorkflowID)
	if err != nil {
		return ""
	}
	return id
}

// CalculateChainCreditCost 计算任务链总积分消耗
func (s *TaskChainService) CalculateChainCreditCost(ctx context.Context, firstWorkflowID string) (int, error) {
	// TODO: 需要注入PictureForgeService来获取工作流信息
	// 暂时返回0，在API层调用时会重新计算
	return 0, nil
}

// CreateChainWithAllTasks 创建包含所有任务的链记录
func (s *TaskChainService) CreateChainWithAllTasks(ctx context.Context, tx *gorm.DB, firstTaskID, secondTaskID string, totalCost int, deductionInfo string) (*model.TaskChain, error) {
	// 构建任务ID列表
	allTaskIDs := []string{firstTaskID, secondTaskID}
	allTaskIDsJSON, err := json.Marshal(allTaskIDs)
	if err != nil {
		return nil, err
	}

	// 构建链配置
	cfg := chainConfig{Steps: []struct {
		Index        int               `json:"index"`
		Type         string            `json:"type"`
		WorkflowID   string            `json:"workflow_id"`
		ParamMapping map[string]string `json:"param_mapping"`
	}{
		{Index: 1, Type: "image"},
		{Index: 2, Type: "video", WorkflowID: s.getPictureToVideoWorkflowID(ctx), ParamMapping: map[string]string{
			"LoadImage1": "$.result_url",
			"prompt":     "$.meta.prompt?",
		}},
	}}
	cfgBytes, _ := json.Marshal(cfg)

	chain := &model.TaskChain{
		ChainID:             uuid.NewString(),
		RootTaskID:          firstTaskID,
		NextTaskID:          secondTaskID,
		UserID:              common.GetUserID(ctx),
		ProjectID:           common.GetProjectID(ctx),
		Status:              0, // PENDING
		CurrentStep:         1,
		ConfigJSON:          string(cfgBytes),
		AllTaskIDs:          string(allTaskIDsJSON),
		TotalCreditCost:     totalCost,
		CreditDeductionInfo: deductionInfo,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	if err := s.chainDao.Create(ctx, tx, chain); err != nil {
		return nil, err
	}
	return chain, nil
}

// GetChainByAnyTaskID 根据任何任务ID获取对应的链
func (s *TaskChainService) GetChainByAnyTaskID(ctx context.Context, taskID string) (*model.TaskChain, error) {
	// 先尝试作为根任务查找
	chain, err := s.chainDao.GetByRootTask(ctx, taskID)
	if err == nil && chain != nil {
		return chain, nil
	}

	// 再尝试作为下一步任务查找
	chain, err = s.chainDao.GetByNextTask(ctx, taskID)
	if err == nil && chain != nil {
		return chain, nil
	}

	// 都没找到，返回nil
	return nil, nil
}

// ActivateNextTask 激活下一步任务
func (s *TaskChainService) ActivateNextTask(ctx context.Context, taskID, previousResult string) error {
	// 1. 获取任务并锁定
	task, err := s.taskDao.GetTaskForUpdate(ctx, s.gdb, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task for update: %v", err)
	}

	// 2. 检查任务状态
	if task.Status != int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_CHAIN) {
		return fmt.Errorf("task %s is not in PENDING_CHAIN status, current: %d", taskID, task.Status)
	}

	// 3. 获取完整的工作流信息
	workflow, err := s.picForgeDao.GetWorkflow(ctx, s.gdb, task.WorkflowID)
	if err != nil {
		return fmt.Errorf("failed to get workflow %s: %v", task.WorkflowID, err)
	}

	// 4. 从第一步结果中提取参数
	var result model.PictureTaskResult
	if err := json.Unmarshal([]byte(previousResult), &result); err != nil {
		return fmt.Errorf("failed to unmarshal previous result: %v", err)
	}

	// 5. 提取前一个任务结果中的prompt信息
	extractedPrompt := s.extractPromptFromResult(ctx, previousResult)

	// 6. 构建完整的任务参数
	taskParams, err := s.buildChainTaskParams(ctx, workflow, &result, task, extractedPrompt)
	if err != nil {
		return fmt.Errorf("failed to build task parameters: %v", err)
	}

	// 7. 序列化参数
	paramJSON, err := json.Marshal(taskParams)
	if err != nil {
		return fmt.Errorf("failed to marshal task parameters: %v", err)
	}

	// 8. 更新任务参数和状态
	task.ParamJSON = string(paramJSON)
	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING) // 进入执行队列
	task.CanRetry = true                                                    // 激活后任务可重试

	// 构建并更新 UserPictureInfoJson 字段
	userPictureInfo := model.UserPictureInfo{
		PhotoURL:    result.ResultURL,
		AspectRatio: result.AspectRatio,
	}
	userPictureInfoJSON, err := json.Marshal(userPictureInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal user picture info: %v", err)
	}
	task.UserPictureInfoJson = string(userPictureInfoJSON)

	task.UpdatedAt = time.Now()

	// 9. 保存更新
	if err := s.taskDao.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("failed to update task: %v", err)
	}

	zlog.LogWithContext(ctx).Info("successfully activated next task in chain",
		zap.String("task_id", taskID),
		zap.String("workflow_id", task.WorkflowID),
		zap.String("provider", workflow.Provider),
		zap.Int("param_count", len(taskParams)),
		zap.Bool("can_retry", true),
		zap.String("user_picture_url", result.ResultURL))

	return nil
}

// RefundChainCredits 退还链积分
func (s *TaskChainService) RefundChainCredits(ctx context.Context, chain *model.TaskChain, reason string) error {
	if chain.CreditDeductionInfo == "" {
		return nil // 没有扣费记录，无需退款
	}

	// 构造临时任务用于退款
	tempTask := &model.PictureTask{
		ProjectID:           chain.ProjectID,
		UserID:              chain.UserID,
		TaskID:              "chain_refund_" + chain.ChainID + "_" + uuid.NewString(),
		CreditDeductionInfo: chain.CreditDeductionInfo,
	}

	// 调用信用服务执行退款
	if err := s.creditSvc.RefundCredits(ctx, tempTask, reason); err != nil {
		zlog.LogWithContext(ctx).Error("failed to refund chain credits",
			zap.String("chain_id", chain.ChainID),
			zap.String("reason", reason),
			zap.Error(err))
		return fmt.Errorf("failed to refund chain credits: %v", err)
	}

	zlog.LogWithContext(ctx).Info("chain credits refunded successfully",
		zap.String("chain_id", chain.ChainID),
		zap.String("reason", reason),
		zap.Int("total_cost", chain.TotalCreditCost))

	return nil
}

// extractPromptFromResult 从前一个任务的结果中提取prompt信息
func (s *TaskChainService) extractPromptFromResult(ctx context.Context, previousResult string) string {
	if previousResult == "" {
		return ""
	}

	// 尝试解析结果JSON
	var resultData map[string]interface{}
	if err := json.Unmarshal([]byte(previousResult), &resultData); err != nil {
		zlog.LogWithContext(ctx).Debug("failed to unmarshal previous result for prompt extraction",
			zap.Error(err),
			zap.String("result", previousResult))
		return ""
	}

	// 查找 meta.prompt 字段
	if meta, ok := resultData["meta"].(map[string]interface{}); ok {
		if prompt, ok := meta["prompt"].(string); ok {
			zlog.LogWithContext(ctx).Debug("extracted prompt from previous result",
				zap.String("prompt", prompt))
			return prompt
		}
	}

	zlog.LogWithContext(ctx).Debug("no prompt found in previous result meta")
	return ""
}

// buildChainTaskParams 构建任务链的完整参数
func (s *TaskChainService) buildChainTaskParams(ctx context.Context, workflow *model.Workflow, previousResult *model.PictureTaskResult, task *model.PictureTask, extractedPrompt string) (map[string]string, error) {
	// 基础参数
	taskParams := map[string]string{
		"LoadImage1":    previousResult.ResultURL,
		"workflow_id":   task.WorkflowID,
		"workflow_type": pb.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO.String(),
	}

	// 添加工作流API配置参数
	if workflow.ApiConfig.ApiIden != "" {
		taskParams["api_iden"] = workflow.ApiConfig.ApiIden
	}
	if workflow.ApiConfig.EffectScene != "" {
		taskParams["effect_scene"] = workflow.ApiConfig.EffectScene
	}

	// 添加provider信息用于执行器匹配
	if workflow.Provider != "" {
		taskParams["provider"] = workflow.Provider
	}

	// 处理prompt - 优先使用提取的prompt，否则使用工作流默认prompt
	finalPrompt := extractedPrompt
	if finalPrompt == "" && workflow.Prompt != "" {
		finalPrompt = workflow.Prompt
	}
	if finalPrompt != "" {
		taskParams["prompt"] = finalPrompt
	}

	zlog.LogWithContext(ctx).Info("built comprehensive task chain parameters",
		zap.String("task_id", task.TaskID),
		zap.String("workflow_id", task.WorkflowID),
		zap.String("provider", workflow.Provider),
		zap.String("api_iden", workflow.ApiConfig.ApiIden),
		zap.String("effect_scene", workflow.ApiConfig.EffectScene),
		zap.String("prompt", finalPrompt),
		zap.String("load_image", previousResult.ResultURL))

	return taskParams, nil
}

// OnTaskFailed 处理任务链中任务失败的情况
func (s *TaskChainService) OnTaskFailed(ctx context.Context, failedTask *model.PictureTask) {
	// 检查是否为链任务
	chain, err := s.GetChainByAnyTaskID(ctx, failedTask.TaskID)
	if err != nil || chain == nil {
		return // 不是链任务，正常返回
	}

	// 防重复处理锁
	lockKey := "visionai:task_chain:failure:" + chain.ChainID
	if !s.rdb.SetNX(lockKey, "1", 5*time.Minute).Val() {
		return
	}
	defer s.rdb.Del(lockKey)

	zlog.LogWithContext(ctx).Info("processing task chain failure",
		zap.String("chain_id", chain.ChainID),
		zap.String("failed_task_id", failedTask.TaskID),
		zap.String("failed_task_status", strconv.Itoa(int(failedTask.Status))))

	// 计算需要退还的积分
	refundAmount, err := s.calculatePartialRefund(ctx, chain, failedTask.TaskID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to calculate partial refund",
			zap.String("chain_id", chain.ChainID),
			zap.String("failed_task_id", failedTask.TaskID),
			zap.Error(err))
		return
	}

	// 如果需要退款，执行部分退款
	if refundAmount > 0 {
		if err := s.executePartialRefund(ctx, chain, failedTask.TaskID, refundAmount); err != nil {
			zlog.LogWithContext(ctx).Error("failed to execute partial refund",
				zap.String("chain_id", chain.ChainID),
				zap.String("failed_task_id", failedTask.TaskID),
				zap.Int("refund_amount", refundAmount),
				zap.Error(err))
		}
	}

	// 更新链状态为失败
	chain.Status = 3 // FAILED
	if err := s.chainDao.Update(ctx, chain); err != nil {
		zlog.LogWithContext(ctx).Error("failed to update chain status to failed",
			zap.String("chain_id", chain.ChainID),
			zap.Error(err))
	}

	// 发送失败通知给所有相关任务
	s.notifyChainFailure(ctx, chain, failedTask)

	zlog.LogWithContext(ctx).Info("task chain failure processing completed",
		zap.String("chain_id", chain.ChainID),
		zap.String("failed_task_id", failedTask.TaskID),
		zap.Int("refund_amount", refundAmount))
}

// calculatePartialRefund 计算部分退款金额
func (s *TaskChainService) calculatePartialRefund(ctx context.Context, chain *model.TaskChain, failedTaskID string) (int, error) {
	// 解析链中的所有任务ID
	var allTaskIDs []string
	if err := json.Unmarshal([]byte(chain.AllTaskIDs), &allTaskIDs); err != nil {
		return 0, fmt.Errorf("failed to unmarshal task IDs: %v", err)
	}

	if len(allTaskIDs) == 0 {
		return 0, errors.New("no tasks found in chain")
	}

	// 确定失败的是第几步任务
	var failedStepIndex int = -1
	for i, taskID := range allTaskIDs {
		if taskID == failedTaskID {
			failedStepIndex = i
			break
		}
	}

	if failedStepIndex == -1 {
		zlog.LogWithContext(ctx).Warn("failed task not found in chain task list",
			zap.String("chain_id", chain.ChainID),
			zap.String("failed_task_id", failedTaskID),
			zap.Strings("all_task_ids", allTaskIDs))
		return 0, nil // 不在链中，无需退款
	}

	// 获取第一步任务的工作流ID
	firstTask, err := s.taskDao.GetTask(ctx, allTaskIDs[0])
	if err != nil {
		return 0, fmt.Errorf("failed to get first task: %v", err)
	}

	firstWorkflow, err := s.picForgeDao.GetWorkflow(ctx, s.gdb, firstTask.WorkflowID)
	if err != nil {
		return 0, fmt.Errorf("failed to get first workflow: %v", err)
	}

	// 获取第二步工作流ID
	secondWorkflowID, err := s.configSvc.GetStringConfig(constants.ConfigKeyPictureToVideoGuideWorkflowID)
	if err != nil {
		return 0, fmt.Errorf("failed to get second workflow ID: %v", err)
	}

	secondWorkflow, err := s.picForgeDao.GetWorkflow(ctx, s.gdb, secondWorkflowID)
	if err != nil {
		return 0, fmt.Errorf("failed to get second workflow: %v", err)
	}

	// 计算退款金额
	switch failedStepIndex {
	case 0:
		// 第一步失败：退还整个链的积分
		refundAmount := firstWorkflow.CreditPoints + secondWorkflow.CreditPoints
		zlog.LogWithContext(ctx).Info("first step failed, refunding full chain cost",
			zap.String("chain_id", chain.ChainID),
			zap.Int("first_workflow_cost", firstWorkflow.CreditPoints),
			zap.Int("second_workflow_cost", secondWorkflow.CreditPoints),
			zap.Int("total_refund", refundAmount))
		return refundAmount, nil
	case 1:
		// 第二步失败：只退还第二步的积分
		refundAmount := secondWorkflow.CreditPoints
		zlog.LogWithContext(ctx).Info("second step failed, refunding second step cost only",
			zap.String("chain_id", chain.ChainID),
			zap.Int("second_workflow_cost", secondWorkflow.CreditPoints),
			zap.Int("refund_amount", refundAmount))
		return refundAmount, nil
	default:
		// 不应该到达这里，因为我们只有两步链
		zlog.LogWithContext(ctx).Warn("unexpected step index for failure",
			zap.String("chain_id", chain.ChainID),
			zap.Int("step_index", failedStepIndex))
		return 0, nil
	}
}

// executePartialRefund 执行部分退款
func (s *TaskChainService) executePartialRefund(ctx context.Context, chain *model.TaskChain, failedTaskID string, refundAmount int) error {
	// 构造临时任务用于部分退款
	tempTask := &model.PictureTask{
		ProjectID:           chain.ProjectID,
		UserID:              chain.UserID,
		TaskID:              fmt.Sprintf("partial_refund_%s_%s_%s", chain.ChainID, failedTaskID, uuid.NewString()),
		CreditDeductionInfo: chain.CreditDeductionInfo,
	}

	// 计算部分退款的扣除信息
	partialDeductionInfo, err := s.calculatePartialDeductionInfo(ctx, chain.CreditDeductionInfo, refundAmount, chain.TotalCreditCost)
	if err != nil {
		return fmt.Errorf("failed to calculate partial deduction info: %v", err)
	}

	tempTask.CreditDeductionInfo = partialDeductionInfo

	// 执行退款
	reason := fmt.Sprintf("Chain task failure - step failed (task: %s)", failedTaskID)
	if err := s.creditSvc.RefundCredits(ctx, tempTask, reason); err != nil {
		return fmt.Errorf("failed to refund partial credits: %v", err)
	}

	zlog.LogWithContext(ctx).Info("partial refund executed successfully",
		zap.String("chain_id", chain.ChainID),
		zap.String("failed_task_id", failedTaskID),
		zap.Int("refund_amount", refundAmount),
		zap.String("reason", reason))

	return nil
}

// calculatePartialDeductionInfo 计算部分退款的扣除信息
func (s *TaskChainService) calculatePartialDeductionInfo(ctx context.Context, originalDeductionInfo string, refundAmount, totalCost int) (string, error) {
	if originalDeductionInfo == "" {
		return "", nil
	}

	// 解析原始扣除信息
	var originalDeduction map[uint]int64
	if err := json.Unmarshal([]byte(originalDeductionInfo), &originalDeduction); err != nil {
		return "", fmt.Errorf("failed to unmarshal original deduction info: %v", err)
	}

	// 计算退款比例
	refundRatio := float64(refundAmount) / float64(totalCost)

	// 按比例计算每个额度记录的退款金额
	partialDeduction := make(map[uint]int64)
	for amountID, deductedAmount := range originalDeduction {
		partialRefundAmount := int64(float64(deductedAmount) * refundRatio)
		if partialRefundAmount > 0 {
			partialDeduction[amountID] = partialRefundAmount
		}
	}

	// 序列化部分扣除信息
	partialDeductionBytes, err := json.Marshal(partialDeduction)
	if err != nil {
		return "", fmt.Errorf("failed to marshal partial deduction info: %v", err)
	}

	return string(partialDeductionBytes), nil
}

// notifyChainFailure 通知链失败状态
func (s *TaskChainService) notifyChainFailure(ctx context.Context, chain *model.TaskChain, failedTask *model.PictureTask) {
	// 解析链中的所有任务ID
	var allTaskIDs []string
	if err := json.Unmarshal([]byte(chain.AllTaskIDs), &allTaskIDs); err != nil {
		zlog.LogWithContext(ctx).Error("failed to unmarshal task IDs for notification",
			zap.String("chain_id", chain.ChainID),
			zap.Error(err))
		return
	}

	// 为链中的每个任务发送失败通知
	for _, taskID := range allTaskIDs {
		// 获取任务信息
		task, err := s.taskDao.GetTask(ctx, taskID)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("failed to get task for failure notification",
				zap.String("task_id", taskID),
				zap.Error(err))
			continue
		}

		// 如果任务还未失败，标记为失败
		if task.Status != int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
			task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
			task.Progress = -1
			if task.TaskID != failedTask.TaskID {
				task.Error = fmt.Sprintf("Chain failure: previous step failed (failed task: %s)", failedTask.TaskID)
			}

			if err := s.taskDao.UpdateTask(ctx, task); err != nil {
				zlog.LogWithContext(ctx).Error("failed to update task status for chain failure",
					zap.String("task_id", taskID),
					zap.Error(err))
				continue
			}
		}

		// 发送失败进度通知
		picture_generate.SendProgressEvent(task)
	}

	zlog.LogWithContext(ctx).Info("chain failure notifications sent",
		zap.String("chain_id", chain.ChainID),
		zap.Int("notified_tasks", len(allTaskIDs)))
}
