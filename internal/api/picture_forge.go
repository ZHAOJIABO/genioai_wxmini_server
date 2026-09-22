package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/common/submitcontext"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/service/event_reporter"
	"va_visionai_server/internal/service/picture_forge"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

const (
	defaultTimeout = 60 * time.Second
)

var (
	ErrInvalidUser     = errors.New("invalid user")
	ErrInvalidRequest  = errors.New("invalid request")
	ErrInvalidTaskID   = errors.New("invalid task id")
	ErrInvalidWorkflow = errors.New("invalid workflow id")
)

type PictureForgeServer struct {
	vai.UnimplementedPictureForgeServiceServer
	pictureForgeService     *picture_forge.PictureForgeService
	pictureTaskService      *service.PictureTaskService
	uploadService           *service.UploadService
	subscribeService        *service.SubscribeService
	dailyFreeCreditsService *service.DailyFreeCreditsService
	creditService           credit.Service
	configService           *service.ConfigService
	taskChainService        *service.TaskChainService
	eventReporter           event_reporter.EventReporter
	modelConfigManager      *service.ModelConfigManager // 模型配置管理器
}

func NewPictureForgeServer(svc *picture_forge.PictureForgeService, taskSvc *service.PictureTaskService, uploadSvc *service.UploadService, subscribeSvc *service.SubscribeService, dailySvc *service.DailyFreeCreditsService, creditSvc credit.Service, configSvc *service.ConfigService, chainSvc *service.TaskChainService, eventReporter event_reporter.EventReporter, modelConfigManager *service.ModelConfigManager) *PictureForgeServer {
	return &PictureForgeServer{
		pictureForgeService:     svc,
		pictureTaskService:      taskSvc,
		uploadService:           uploadSvc,
		subscribeService:        subscribeSvc,
		dailyFreeCreditsService: dailySvc,
		creditService:           creditSvc,
		configService:           configSvc,
		taskChainService:        chainSvc,
		eventReporter:           eventReporter,
		modelConfigManager:      modelConfigManager,
	}
}

func (s *PictureForgeServer) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, defaultTimeout)
}

// isImageWorkflow 判断工作流是否为图像类
func (s *PictureForgeServer) isImageWorkflow(ctx context.Context, workflow *model.Workflow) bool {
	if workflow == nil {
		return false
	}
	// 通过 Kind 信息判断类型，避免字段不一致
	kindInfoMap, err := s.pictureForgeService.GetKindInfoMap(ctx, []string{workflow.KindID})
	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to load kind info for workflow type check",
			zap.String("workflow_id", workflow.WorkflowID),
			zap.String("kind_id", workflow.KindID),
			zap.Error(err))
		return false
	}
	if kind, ok := kindInfoMap[workflow.KindID]; ok {
		return kind.KindType == vai.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String()
	}
	return false
}

// buildVirtualWorkflowFromModel 从模型配置构建虚拟workflow（模型直连模式）
// 这个虚拟workflow用于适配现有的workflow处理流程
func (s *PictureForgeServer) buildVirtualWorkflowFromModel(
	ctx context.Context,
	modelMeta *service.ModelMetadata,
	req *vai.SubmitPictureForgeTaskRequest) (*model.Workflow, error) {

	// 构建分辨率字符串
	resolution := fmt.Sprintf("%dx%d", modelMeta.DefaultWidth, modelMeta.DefaultHeight)
	if req.GetModelConfig().GetWidth() > 0 && req.GetModelConfig().GetHeight() > 0 {
		resolution = fmt.Sprintf("%dx%d", req.GetModelConfig().GetWidth(), req.GetModelConfig().GetHeight())
	}

	// 构建虚拟workflow
	virtualWorkflow := &model.Workflow{
		WorkflowID:   fmt.Sprintf("model_direct_%s_%s", modelMeta.ModelName, uuid.New().String()[:8]),
		Title:        modelMeta.DisplayName,
		Description:  modelMeta.Description,
		Provider:     modelMeta.Provider,
		Prompt:       req.GetUserPrompt(), // 直接使用用户输入的prompt
		CreditPoints: modelMeta.CreditPoints,
		UseApi:       true, // 标记为API模式
		ApiConfig: model.WorkflowApiConfig{
			ApiIden:           modelMeta.ApiIden,
			EffectScene:       modelMeta.EffectScene,
			TaskType:          modelMeta.TaskType,
			ModelName:         modelMeta.ModelName,
			DefaultResolution: resolution,
		},
		// 标记为图像类workflow
		KindID: "model_direct_image",
	}

	zlog.LogWithContext(ctx).Info("built virtual workflow from model",
		zap.String("model_name", modelMeta.ModelName),
		zap.String("virtual_workflow_id", virtualWorkflow.WorkflowID),
		zap.String("provider", virtualWorkflow.Provider))

	return virtualWorkflow, nil
}

func (s *PictureForgeServer) SubmitPictureForgeTask(ctx context.Context, req *vai.SubmitPictureForgeTaskRequest) (*vai.SubmitPictureForgeTaskResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	userID := common.GetUserID(ctx)
	projectID := common.GetProjectID(ctx)

	if err := validateSubmitTaskRequest(req); err != nil {
		return BuildErrorResponse[vai.SubmitPictureForgeTaskResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid submit task request")
	}

	var workflowInfo *model.Workflow
	var err error
	isModelDirectMode := false

	// 判断使用哪种模式：workflow模式 或 模型直连模式
	if req.GetModelConfig() != nil && req.GetModelConfig().GetModelName() != "" {
		// ===== 模型直连模式 =====
		isModelDirectMode = true

		// 获取项目ID
		projectID := common.GetProjectID(ctx)
		if projectID == "" || projectID == constants.ProjectIdVisualAI {
			projectID = constants.ProjectIdVisionAI
		}

		// 从数据库获取模型元数据
		modelDao := dao.NewImageGenerationModelDao(db.GetDB())
		dbModel, err := modelDao.GetModelByName(ctx, req.GetModelConfig().GetModelName(), projectID)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to get model from database",
				zap.String("model_name", req.GetModelConfig().GetModelName()),
				zap.String("project_id", projectID),
				zap.Error(err))
			return BuildErrorResponse[vai.SubmitPictureForgeTaskResponse](ctx,
				vai.StatusCode_INVALID_PARAM, err, "model not found or disabled")
		}

		// 验证模型是否支持所需操作
		hasInputImages := len(req.GetUserImages()) > 0
		if hasInputImages && !dbModel.SupportI2I {
			zlog.LogWithContext(ctx).Error("model does not support image-to-image",
				zap.String("model_name", req.GetModelConfig().GetModelName()))
			return BuildErrorResponse[vai.SubmitPictureForgeTaskResponse](ctx,
				vai.StatusCode_INVALID_PARAM,
				errors.New("model does not support image-to-image generation"),
				"unsupported operation")
		}
		if !hasInputImages && !dbModel.SupportT2I {
			zlog.LogWithContext(ctx).Error("model does not support text-to-image",
				zap.String("model_name", req.GetModelConfig().GetModelName()))
			return BuildErrorResponse[vai.SubmitPictureForgeTaskResponse](ctx,
				vai.StatusCode_INVALID_PARAM,
				errors.New("model does not support text-to-image generation"),
				"unsupported operation")
		}

		// 校验输入图片数量是否超限
		if hasInputImages && dbModel.MaxInputImages > 0 {
			if len(req.GetUserImages()) > dbModel.MaxInputImages {
				zlog.LogWithContext(ctx).Error("input image count exceeds model limit",
					zap.Int("count", len(req.GetUserImages())),
					zap.Int("max", dbModel.MaxInputImages),
					zap.String("model_name", req.GetModelConfig().GetModelName()))
				return BuildErrorResponse[vai.SubmitPictureForgeTaskResponse](ctx,
					vai.StatusCode_INVALID_PARAM,
					fmt.Errorf("input image count %d exceeds model limit %d",
						len(req.GetUserImages()), dbModel.MaxInputImages),
					"too many input images")
			}
		}

		// 转换为 ModelMetadata 格式（兼容现有代码）
		modelMeta := &service.ModelMetadata{
			ModelName:      dbModel.ModelName,
			DisplayName:    dbModel.DisplayName,
			Provider:       dbModel.Provider,
			Description:    dbModel.Description,
			CreditPoints:   dbModel.CreditPoints,
			DefaultWidth:   dbModel.DefaultWidth,
			DefaultHeight:  dbModel.DefaultHeight,
			SupportT2I:     dbModel.SupportT2I,
			SupportI2I:     dbModel.SupportI2I,
			MaxImages:      dbModel.MaxImages,
			MaxInputImages: dbModel.MaxInputImages,
			Enabled:        dbModel.Enabled,
		}

		// 解析 support_qualities
		if dbModel.SupportQualities != "" {
			var qualities []string
			if err := json.Unmarshal([]byte(dbModel.SupportQualities), &qualities); err == nil {
				modelMeta.SupportQualities = qualities
			}
		}

		// 验证用户prompt是否提供
		if req.GetUserPrompt() == "" {
			return BuildErrorResponse[vai.SubmitPictureForgeTaskResponse](ctx,
				vai.StatusCode_INVALID_PARAM,
				errors.New("user_prompt is required in model direct mode"),
				"user_prompt is required")
		}

		// 构建虚拟workflow（适配现有流程）
		workflowInfo, err = s.buildVirtualWorkflowFromModel(ctx, modelMeta, req)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to build virtual workflow", zap.Error(err))
			return BuildErrorResponse[vai.SubmitPictureForgeTaskResponse](ctx,
				vai.StatusCode_REQUEST_FAILED, err, "failed to build workflow")
		}

		// 解析 quality_credit_rules 并赋值到虚拟 workflow
		if dbModel.QualityCreditRules != "" {
			var qcr map[string]int
			if parseErr := json.Unmarshal([]byte(dbModel.QualityCreditRules), &qcr); parseErr == nil {
				workflowInfo.QualityCreditRules = qcr
			}
		}

		zlog.LogWithContext(ctx).Info("using model direct mode",
			zap.String("model_name", req.GetModelConfig().GetModelName()),
			zap.Bool("is_image_to_image", hasInputImages),
			zap.String("virtual_workflow_id", workflowInfo.WorkflowID))

	} else {
		// ===== Workflow模式（保持向后兼容） =====
		workflowInfo, err = s.pictureForgeService.GetWorkflowByID(ctx, req.GetWorkflowId())
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to get workflow info", zap.Error(err))
			return BuildErrorResponse[vai.SubmitPictureForgeTaskResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get workflow info")
		}

		zlog.LogWithContext(ctx).Info("using workflow mode",
			zap.String("workflow_id", req.GetWorkflowId()))
	}

	// 获取toolID
	toolID := ""
	if toolIDValue := ctx.Value(common.PictureToolIDKey); toolIDValue != nil {
		toolID = toolIDValue.(string)
	}

	// 统一计算最终积分
	// 模型直连模式下传入 quality 参数用于差异化积分计算
	quality := ""
	if isModelDirectMode {
		quality = req.GetModelConfig().GetQuality()
	}

	// 计算图片数量（用于多图积分计算）
	// 优先使用 num_images（输出图片数量），其次使用输入图片数量
	imageCount := len(req.GetUserImages())
	if isModelDirectMode && req.GetModelConfig().GetNumImages() > 1 {
		imageCount = int(req.GetModelConfig().GetNumImages())
	}

	zlog.LogWithContext(ctx).Info("CalculateFinalCredits input",
		zap.Bool("is_model_direct", isModelDirectMode),
		zap.String("quality", quality),
		zap.Any("quality_credit_rules", workflowInfo.QualityCreditRules),
		zap.Int("base_credits", workflowInfo.CreditPoints),
		zap.Int("image_count", imageCount))

	creditResult, err := s.pictureForgeService.CalculateFinalCredits(
		ctx, workflowInfo, toolID, req.GetDuration(), req.GetResolution(), quality, imageCount)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to calculate final credits", zap.Error(err))
		return BuildErrorResponse[vai.SubmitPictureForgeTaskResponse](ctx,
			vai.StatusCode_REQUEST_FAILED, err, "failed to calculate credits")
	}

	// 将模式信息和workflow注入到context中，供后续服务使用
	if isModelDirectMode {
		ctx = context.WithValue(ctx, "is_model_direct_mode", true)
		ctx = context.WithValue(ctx, "virtual_workflow_info", workflowInfo)
		ctx = context.WithValue(ctx, "model_name", req.GetModelConfig().GetModelName())
		ctx = context.WithValue(ctx, "user_prompt", req.GetUserPrompt())
		ctx = context.WithValue(ctx, "negative_prompt", req.GetNegativePrompt())
	}

	creditPoints := creditResult.FinalCredits

	// 统一判定会员
	isMember := false
	if s.subscribeService != nil {
		if info, _ := s.subscribeService.GetSubscribeInfo(ctx, userID, "", "", ""); info != nil && info.GetSubscribeLevel() > 0 {
			isMember = true
		}
	}

	// 在所有加价规则之后应用免费策略。模型直连使用服务端已确认的模式，
	// 不依赖虚拟 KindID 在数据库中存在。
	creditPoints = credit.ImageGenerationCost(creditPoints,
		isModelDirectMode || s.isImageWorkflow(ctx, workflowInfo), isMember)

	var resp *vai.SubmitPictureForgeTaskResponse
	// 构建并注入提交上下文 JSON（首期仅 OS 等）
	submitCtxJSON := submitcontext.MustMarshal(submitcontext.BuildFromHeader(req.GetRequestHeader()))
	ctx = common.CtxSetStrValue(ctx, constants.CtxSubmitContextJSON, submitCtxJSON)
	// 提前生成 taskID，用于关联扣费流水和任务记录
	taskID := service.GenerateTaskID()
	err = s.pictureTaskService.Transaction(func(tx *gorm.DB) error {
		// 1) 每日积分（已禁用）
		dailyGrantFailed := false
		// if s.dailyFreeCreditsService != nil {
		// 	if _, _, gerr := s.dailyFreeCreditsService.CheckAndGrantDailyCreditsWithTx(ctx, tx, projectID, userID, isMember); gerr != nil {
		// 		zlog.LogWithContext(ctx).Warn("grant daily credits (tx) failed",
		// 			zap.Error(gerr),
		// 			zap.String("user_id", userID))
		// 		dailyGrantFailed = true
		// 	}
		// }

		// 2) 扣款（需要扣款时）
		var deductionInfoJSON string
		if creditPoints > 0 {
			deductionInfo, deductErr := s.creditService.DeductCreditsWithTx(ctx, tx, projectID, userID, taskID, constants.TransactionTypeUsageDeduction, int64(creditPoints), "Submit Task Deduct Credits", isMember)
			if deductErr != nil {
				if errors.Is(deductErr, credit.ErrInsufficientCredits) {
					// 非会员且当次每日发放失败，给出更清晰提示
					if !isMember && dailyGrantFailed {
						return errors.New("insufficient credits: daily free grant failed today")
					}
					return deductErr
				}
				return deductErr
			}
			deductionBytes, jsonErr := json.Marshal(deductionInfo)
			if jsonErr != nil {
				return jsonErr
			}
			deductionInfoJSON = string(deductionBytes)
		}

		// 3) 创建任务（同事务）
		var submitErr error
		resp, submitErr = s.pictureTaskService.SubmitTaskWithTx(ctx, tx, req, taskID, deductionInfoJSON, isMember, creditPoints, submitCtxJSON)
		return submitErr
	})

	if err != nil {
		if errors.Is(err, credit.ErrInsufficientCredits) {
			return BuildErrorResponse[vai.SubmitPictureForgeTaskResponse](ctx, vai.StatusCode_CREDIT_POINT_NOT_ENOUGH, err, "user amount is not enough")
		}
		return BuildErrorResponse[vai.SubmitPictureForgeTaskResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to submit task")
	}

	return BuildSuccessResponse(resp)
}

func (s *PictureForgeServer) GetPictureForgeTaskResult(ctx context.Context, req *vai.GetPictureForgeTaskResultRequest) (*vai.GetPictureForgeTaskResultResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if req.GetTaskId() == "" {
		return BuildErrorResponse[vai.GetPictureForgeTaskResultResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidTaskID, "invalid task id")
	}
	resp, err := s.pictureTaskService.GetTaskResult(ctx, req)
	if err != nil {
		return BuildErrorResponse[vai.GetPictureForgeTaskResultResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get task result")
	}

	return BuildSuccessResponse(resp)
}

func (s *PictureForgeServer) ListUserUploadPicture(ctx context.Context, req *vai.ListUserUploadPictureRequest) (*vai.ListUserUploadPictureResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if req.GetRequestHeader().GetUserId() == "" {
		return BuildErrorResponse[vai.ListUserUploadPictureResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidUser, "invalid user id")
	}

	files, err := s.uploadService.RecentUpload(ctx, req.GetRequestHeader().GetUserId(), "jpg")
	if err != nil {
		return BuildErrorResponse[vai.ListUserUploadPictureResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list user pictures")
	}

	pictureInfos := make([]*vai.PictureInfo, 0, len(files))
	for _, file := range files {
		if file.PhotoWidth <= 0 || file.PhotoHeight <= 0 {
			continue
		}
		pictureInfos = append(pictureInfos, &vai.PictureInfo{
			Url:          file.OSSAddr,
			AspectRatio:  float32(file.PhotoHeight) / float32(file.PhotoWidth),
			ThumbnailUrl: GetLowQualityImages(file.OSSAddr),
			FileId:       strconv.FormatUint(uint64(file.ID), 10),
		})
	}
	if len(files) < 8 {
		builtInPics, err := s.pictureForgeService.GetBuiltInPictures(ctx)
		if err != nil {
			return BuildErrorResponse[vai.ListUserUploadPictureResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get built-in pictures")
		}
		for _, item := range builtInPics {
			item.ThumbnailUrl = GetLowQualityImages(item.GetUrl())
		}
		pictureInfos = append(pictureInfos, builtInPics...)
	}

	return BuildSuccessResponse(&vai.ListUserUploadPictureResponse{
		PictureInfo: pictureInfos,
	})
}

func (s *PictureForgeServer) PublishPicture(ctx context.Context, req *vai.PublishPictureRequest) (*vai.PublishPictureResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if err := validatePublishRequest(req); err != nil {
		return BuildErrorResponse[vai.PublishPictureResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid publish request")
	}

	resp, err := s.pictureTaskService.PublishPicture(ctx, req)
	if err != nil {
		return BuildErrorResponse[vai.PublishPictureResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to publish picture")
	}

	return BuildSuccessResponse(resp)
}

func (s *PictureForgeServer) DeletePicture(ctx context.Context, req *vai.DeletePictureRequest) (*vai.DeletePictureResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if err := validateDeleteRequest(req); err != nil {
		return BuildErrorResponse[vai.DeletePictureResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid delete request")
	}

	// 记录删除操作信息
	taskCount := len(req.GetPictureForgeTaskIds())
	if taskCount == 0 && req.GetPictureForgeTaskId() != "" {
		taskCount = 1
	}
	zlog.LogWithContext(ctx).Info("deleting pictures",
		zap.Int("task_count", taskCount),
		zap.String("user_id", req.GetRequestHeader().GetUserId()))

	resp, err := s.pictureTaskService.DeletePicture(ctx, req)
	if err != nil {
		return BuildErrorResponse[vai.DeletePictureResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to delete picture")
	}

	return BuildSuccessResponse(resp)
}

func (s *PictureForgeServer) ListUserPictureForgeTask(ctx context.Context, req *vai.ListUserPictureForgeTaskRequest) (*vai.ListUserPictureForgeTaskResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	var theme string
	if req.GetWorkflowTheme() != "" {
		theme = req.GetWorkflowTheme()
	} else if req.GetTheme() != vai.WorkflowTheme_WORKFLOW_THEME_UNKNOWN {
		theme = req.GetTheme().String()
	}

	var kindType string
	if req.GetWorkflowKindType() != vai.WorkflowKindType_WORKFLOW_KIND_TYPE_UNKNOWN {
		kindType = req.GetWorkflowKindType().String()
	}

	// 解析任务模式筛选参数
	var taskMode *int8
	if req.GetTaskMode() != vai.TaskMode_TASK_MODE_ALL {
		mode := int8(0)
		if req.GetTaskMode() == vai.TaskMode_TASK_MODE_MODEL_DIRECT {
			mode = 1
		}
		taskMode = &mode
	}

	if req.GetRequestHeader() == nil || req.GetRequestHeader().GetUserId() == "" {
		return BuildErrorResponse[vai.ListUserPictureForgeTaskResponse](ctx, vai.StatusCode_INVALID_USER, ErrInvalidUser, "invalid user")
	}

	if err := validateListRequest(req.GetPageCommon().GetPage(), req.GetPageCommon().GetPageSize()); err != nil {
		return BuildErrorResponse[vai.ListUserPictureForgeTaskResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid list request")
	}

	tasks, total, err := s.pictureTaskService.GetRecentTasks(ctx,
		req.GetRequestHeader().GetUserId(),
		theme,
		kindType,
		taskMode,
		req.GetPageCommon().GetPage(),
		req.GetPageCommon().GetPageSize(),
	)
	if err != nil {
		return BuildErrorResponse[vai.ListUserPictureForgeTaskResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get recent tasks")
	}

	return BuildSuccessResponse(&vai.ListUserPictureForgeTaskResponse{
		PageCommon: &vai.PageCommon{
			Page:     req.GetPageCommon().GetPage(),
			PageSize: req.GetPageCommon().GetPageSize(),
			Total:    int32(total),
		},
		PictureForgeInfos: tasks,
	})
}

func (s *PictureForgeServer) ListKindWithWorkflows(ctx context.Context, req *vai.ListKindWithWorkflowsRequest) (*vai.ListKindWithWorkflowsResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.ListKindWithWorkflowsResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}

	limit := req.GetLimit()
	theme := req.GetTheme()

	kinds, err := s.pictureForgeService.ListKindWithWorkflows(ctx, int(limit), theme)
	if err != nil {
		return BuildErrorResponse[vai.ListKindWithWorkflowsResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list kind with workflows")
	}

	for _, kind := range kinds {
		for _, workflow := range kind.GetWorkflows() {
			workflow.IsHot = false
			workflow.IsNew = false
			workflow.WorkflowTheme = ""
		}
	}

	return BuildSuccessResponse(&vai.ListKindWithWorkflowsResponse{
		WorkflowKinds: kinds,
	})
}

func (s *PictureForgeServer) ListWorkflowPage(ctx context.Context, req *vai.ListWorkflowPageRequest) (*vai.ListWorkflowPageResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if req.GetRequestHeader() == nil {
		return BuildErrorResponse[vai.ListWorkflowPageResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid request")
	}

	page := int32(0)
	pageSize := int32(0)
	if req.GetPageCommon() != nil {
		page = req.GetPageCommon().GetPage()
		pageSize = req.GetPageCommon().GetPageSize()
	}

	if page == 0 || pageSize == 0 {
		page = 1
		pageSize = -1
	}

	workflows, total, err := s.pictureForgeService.ListWorkflowsPage(ctx, req.GetKindId(), req.GetTheme(), req.GetWorkflowKindType(), page, pageSize)
	if err != nil {
		return BuildErrorResponse[vai.ListWorkflowPageResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list workflows")
	}

	respPageCommon := &vai.PageCommon{
		Total: int32(total),
	}
	if req.GetPageCommon() != nil {
		respPageCommon.Page = req.GetPageCommon().GetPage()
		respPageCommon.PageSize = req.GetPageCommon().GetPageSize()
	}

	return BuildSuccessResponse(&vai.ListWorkflowPageResponse{
		PageCommon: respPageCommon,
		Workflows:  workflows,
	})
}

func (s *PictureForgeServer) ClearUserUploadPicture(ctx context.Context, req *vai.ClearUserUploadPictureRequest) (*vai.ClearUserUploadPictureResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if req.GetRequestHeader().GetUserId() == "" {
		return BuildErrorResponse[vai.ClearUserUploadPictureResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidUser, "invalid user id")
	}

	if err := s.uploadService.ClearUserUploadPicture(ctx, req.GetRequestHeader().GetUserId()); err != nil {
		return BuildErrorResponse[vai.ClearUserUploadPictureResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to clear user pictures")
	}

	return BuildSuccessResponse(&vai.ClearUserUploadPictureResponse{})
}

func (s *PictureForgeServer) DeleteUserUploadPicture(ctx context.Context, req *vai.DeleteUserUploadPictureRequest) (*vai.DeleteUserUploadPictureResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if req.GetRequestHeader().GetUserId() == "" {
		return BuildErrorResponse[vai.DeleteUserUploadPictureResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidUser, "invalid user id")
	}
	if req.GetFileId() == "" {
		return BuildErrorResponse[vai.DeleteUserUploadPictureResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid file id")
	}

	if err := s.uploadService.DeleteUserUploadPicture(ctx, req.GetRequestHeader().GetUserId(), req.GetFileId()); err != nil {
		return BuildErrorResponse[vai.DeleteUserUploadPictureResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to delete user pictures")
	}

	return BuildSuccessResponse(&vai.DeleteUserUploadPictureResponse{})
}

func validateSubmitTaskRequest(req *vai.SubmitPictureForgeTaskRequest) error {
	if req.GetRequestHeader() == nil || req.GetRequestHeader().GetUserId() == "" {
		return ErrInvalidUser
	}

	// 检查是否为模型直连模式
	hasModelConfig := req.GetModelConfig() != nil && req.GetModelConfig().GetModelName() != ""
	hasWorkflowId := req.GetWorkflowId() != ""

	// 必须有一种模式（workflow 或 model_config）
	if !hasModelConfig && !hasWorkflowId {
		return errors.New("either workflow_id or model_config must be provided")
	}

	// 模型直连模式的额外验证
	if hasModelConfig {
		if req.GetUserPrompt() == "" {
			return errors.New("user_prompt is required when using model_config")
		}
		// 如果有 user_images，验证 strength 范围
		if len(req.GetUserImages()) > 0 && req.GetStrength() > 0 {
			if req.GetStrength() < 0 || req.GetStrength() > 1 {
				return errors.New("strength must be between 0.0 and 1.0")
			}
		}
	}

	// Workflow模式的验证
	if !hasModelConfig && req.GetWorkflowId() == "" {
		return ErrInvalidWorkflow
	}

	return nil
}

func validateListRequest(page, pageSize int32) error {
	if page < 0 {
		page = 0
	}
	if pageSize <= 0 {
		pageSize = 1000
	}

	return nil
}

func validatePublishRequest(req *vai.PublishPictureRequest) error {
	if req.GetRequestHeader() == nil || req.GetRequestHeader().GetUserId() == "" {
		return ErrInvalidUser
	}
	if req.GetPictureForgeTaskId() == "" {
		return ErrInvalidTaskID
	}
	return nil
}

func validateDeleteRequest(req *vai.DeletePictureRequest) error {
	if req.GetRequestHeader() == nil || req.GetRequestHeader().GetUserId() == "" {
		return ErrInvalidUser
	}
	// 至少需要提供单个任务ID或批量任务ID之一
	if req.GetPictureForgeTaskId() == "" && len(req.GetPictureForgeTaskIds()) == 0 {
		return ErrInvalidTaskID
	}
	return nil
}

func validateRequest(header *vai.RequestHeader) error {
	if header == nil || header.GetUserId() == "" {
		return ErrInvalidUser
	}
	return nil
}

func validatePageCommon(page *vai.PageCommon) error {
	if page == nil {
		return errors.New("page_common is required")
	}
	return validateListRequest(page.GetPage(), page.GetPageSize())
}

func (s *PictureForgeServer) RetryFailedTask(ctx context.Context, req *vai.RetryFailedTaskRequest) (*vai.RetryFailedTaskResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if req.GetTaskId() == "" {
		return BuildErrorResponse[vai.RetryFailedTaskResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidTaskID, "invalid task id")
	}

	resp, err := s.pictureTaskService.RetryFailedTask(ctx, req.GetTaskId())
	if err != nil {
		return BuildErrorResponse[vai.RetryFailedTaskResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to retry failed task")
	}

	return BuildSuccessResponse(resp)
}

func (s *PictureForgeServer) ListBannerKindWithWorkflows(ctx context.Context, req *vai.ListBannerKindWithWorkflowsRequest) (*vai.ListBannerKindWithWorkflowsResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	person, pet, err := s.pictureForgeService.ListBannerKindWithWorkflows(ctx)
	if err != nil {
		return BuildErrorResponse[vai.ListBannerKindWithWorkflowsResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list banner kind with workflows")
	}

	return BuildSuccessResponse(&vai.ListBannerKindWithWorkflowsResponse{
		PersonBanner: person,
		PetBanner:    pet,
	})
}

func (s *PictureForgeServer) ListThemeWithWorkflows(ctx context.Context, req *vai.ListThemeWithWorkflowsRequest) (*vai.ListThemeWithWorkflowsResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	lang := common.GetLang(ctx)
	limit := req.GetWorkflowLimit()
	withoutKind := req.GetWithoutWorkflowKind()
	kindType := req.GetKindType()
	defer zlog.LogWithContext(ctx).Info("ListThemeWithWorkflows Completed")

	themes, err := s.pictureForgeService.ListThemeWithWorkflows(ctx, lang, int(limit), kindType, withoutKind)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to list theme with workflows", zap.Error(err))
		return BuildErrorResponse[vai.ListThemeWithWorkflowsResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list theme with workflows")
	}

	return BuildSuccessResponse(&vai.ListThemeWithWorkflowsResponse{
		Datas: themes,
	})
}

func (s *PictureForgeServer) GetWorkflowCreditPoints(ctx context.Context, workflowID string) int {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	workflow, err := s.pictureForgeService.GetWorkflowByID(ctx, workflowID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("[(s *PictureForgeServer) GetWorkflowCreditPoints] failed to get workflow", zap.Error(err))
		return 0
	}
	return workflow.CreditPoints
}

func (s *PictureForgeServer) ListAllThemeInfo(ctx context.Context, req *vai.ListAllThemeInfoRequest) (*vai.ListAllThemeInfoResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	themes, err := s.pictureForgeService.ListAllThemeInfo(ctx)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to list all theme info", zap.Error(err))
		return BuildErrorResponse[vai.ListAllThemeInfoResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list all theme info")
	}

	return BuildSuccessResponse(&vai.ListAllThemeInfoResponse{
		ThemeInfos: themes,
	})
}

func (s *PictureForgeServer) buildWorkflowInputsFromImage(ctx context.Context, workflowInfo *model.Workflow, imageURLs []string, prompt string) ([]*vai.WorkflowInput, error) {
	var inputDefinitions []*model.WorkflowInput
	if workflowInfo.InputsJSON != "" {
		if err := json.Unmarshal([]byte(workflowInfo.InputsJSON), &inputDefinitions); err != nil {
			zlog.LogWithContext(ctx).Error("failed to unmarshal workflow inputs", zap.Error(err))
			return nil, err
		}
	}

	workflowInputs := make([]*vai.WorkflowInput, 0, len(inputDefinitions))
	imageURLIndex := 0

	for _, inputDef := range inputDefinitions {
		workflowInput := &vai.WorkflowInput{
			InputName: inputDef.InputName,
			InputType: vai.MessageType(inputDef.InputType),
		}

		switch vai.MessageType(inputDef.InputType) {
		case vai.MessageType_MT_IMAGE:
			if imageURLIndex < len(imageURLs) {
				workflowInput.InputContent = imageURLs[imageURLIndex]
				imageURLIndex++
			} else {
				workflowInput.InputContent = inputDef.InputContent
			}
		case vai.MessageType_MT_TEXT:
			if inputDef.InputName == "InputPrompt" && prompt != "" {
				workflowInput.InputContent = prompt
			} else {
				workflowInput.InputContent = inputDef.InputContent
			}
		default:
			workflowInput.InputContent = inputDef.InputContent
		}

		workflowInputs = append(workflowInputs, workflowInput)
	}

	if prompt != "" {
		workflowInputs = append(workflowInputs, &vai.WorkflowInput{
			InputName:    "custom_prompt",
			InputType:    vai.MessageType_MT_TEXT,
			InputContent: prompt,
		})
	}

	return workflowInputs, nil
}

func (s *PictureForgeServer) SubmitRandomWorkflowTask(ctx context.Context, req *vai.SubmitRandomWorkflowTaskRequest) (*vai.SubmitRandomWorkflowTaskResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if req.GetRequestHeader() == nil || req.GetRequestHeader().GetUserId() == "" {
		return BuildErrorResponse[vai.SubmitRandomWorkflowTaskResponse](ctx, vai.StatusCode_INVALID_USER, ErrInvalidUser, "invalid user")
	}

	workflows, kindInfoMap, _, err := s.pictureForgeService.FetchWorkflowsWithKinds(ctx, "", "", vai.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String(), 0, -1)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to list image workflows", zap.Error(err))
		return BuildErrorResponse[vai.SubmitRandomWorkflowTaskResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list image workflows")
	}

	if len(workflows) == 0 {
		return BuildErrorResponse[vai.SubmitRandomWorkflowTaskResponse](ctx, vai.StatusCode_REQUEST_FAILED, errors.New("no image workflows available"), "no image workflows available")
	}

	restrictionRules, err := s.pictureForgeService.GetCreationRestrictionRules(ctx)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get creation restriction rules, proceeding without restrictions", zap.Error(err))
	}

	if restrictionRules != nil {
		filteredWorkflows := picture_forge.FilterWorkflowsByRestrictionRules(ctx, workflows, kindInfoMap, restrictionRules)
		// 如果配置了 AllowedKindIDs，则严格要求过滤后仍需有结果；否则允许回退
		if len(restrictionRules.AllowedKindIDs) > 0 {
			workflows = filteredWorkflows
		} else {
			if len(filteredWorkflows) > 0 {
				workflows = filteredWorkflows
			} else {
				zlog.LogWithContext(ctx).Warn("no workflows available after applying non-strict restriction rules, falling back to original list")
			}
		}
		if len(workflows) > 0 {
			zlog.LogWithContext(ctx).Debug("applied creation restriction rules",
				zap.Int("filtered_workflow_count", len(workflows)))
		}
	}

	if len(workflows) == 0 {
		return BuildErrorResponse[vai.SubmitRandomWorkflowTaskResponse](ctx, vai.StatusCode_REQUEST_FAILED, errors.New("no workflows available after applying restrictions"), "no workflows available after applying restrictions")
	}

	randomWorkflow := workflows[rand.Intn(len(workflows))]

	workflowInputs, err := s.buildWorkflowInputsFromImage(ctx, randomWorkflow, req.GetPictureUrl(), "")
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to build workflow inputs from image URLs", zap.Error(err))
		return BuildErrorResponse[vai.SubmitRandomWorkflowTaskResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to build workflow inputs")
	}

	submitReq := &vai.SubmitPictureForgeTaskRequest{
		RequestHeader: req.GetRequestHeader(),
		WorkflowId:    randomWorkflow.WorkflowID,
		WorkflowInput: workflowInputs,
	}

	// 注入提交来源标识
	ctx = context.WithValue(ctx, constants.CtxTaskSubmitSource, constants.SubmitSourceRandom)
	submitResp, _ := s.SubmitPictureForgeTask(ctx, submitReq)
	if submitResp.GetResponseHeader().GetCode() != vai.StatusCode_SUCCESS {
		return BuildErrorResponse[vai.SubmitRandomWorkflowTaskResponse](ctx, submitResp.GetResponseHeader().GetCode(), errors.New(submitResp.GetResponseHeader().GetMsg()), "failed to submit random task")
	}
	workflowTheme := s.getWorkflowTheme(ctx, randomWorkflow, kindInfoMap)
	randomResp := &vai.SubmitRandomWorkflowTaskResponse{
		ResponseHeader: submitResp.GetResponseHeader(),
		TaskId:         submitResp.GetTaskId(),
		WorkflowTheme:  workflowTheme,
	}

	return BuildSuccessResponse(randomResp)
}

func (s *PictureForgeServer) PictureToVideoGuide(ctx context.Context, req *vai.PictureToVideoGuideRequest) (*vai.PictureToVideoGuideResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if req.GetRequestHeader() == nil || req.GetRequestHeader().GetUserId() == "" {
		return BuildErrorResponse[vai.PictureToVideoGuideResponse](ctx, vai.StatusCode_INVALID_USER, ErrInvalidUser, "invalid user")
	}

	if len(req.GetPictureUrl()) == 0 {
		return BuildErrorResponse[vai.PictureToVideoGuideResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "invalid picture url")
	}

	workflowID, err := s.configService.GetStringConfig(constants.ConfigKeyPictureToVideoGuideWorkflowID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get picture to video workflow id from config", zap.Error(err))
		return BuildErrorResponse[vai.PictureToVideoGuideResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get workflow configuration")
	}

	if workflowID == "" {
		return BuildErrorResponse[vai.PictureToVideoGuideResponse](ctx, vai.StatusCode_REQUEST_FAILED, errors.New("picture to video workflow id not configured"), "workflow not configured")
	}

	workflowInfo, err := s.pictureForgeService.GetWorkflowByID(ctx, workflowID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get workflow info", zap.Error(err))
		return BuildErrorResponse[vai.PictureToVideoGuideResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get workflow info")
	}
	workflowTheme := s.getWorkflowTheme(ctx, workflowInfo, nil)
	workflowInputs, err := s.buildWorkflowInputsFromImage(ctx, workflowInfo, req.GetPictureUrl(), "")
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to build workflow inputs from image URLs", zap.Error(err))
		return BuildErrorResponse[vai.PictureToVideoGuideResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to build workflow inputs")
	}

	submitReq := &vai.SubmitPictureForgeTaskRequest{
		RequestHeader: req.GetRequestHeader(),
		WorkflowId:    workflowID,
		WorkflowInput: workflowInputs,
		WorkflowTheme: workflowTheme,
	}

	// 注入提交来源标识
	ctx = context.WithValue(ctx, constants.CtxTaskSubmitSource, constants.SubmitSourceGuide)
	submitResp, _ := s.SubmitPictureForgeTask(ctx, submitReq)
	if submitResp.GetResponseHeader().GetCode() != vai.StatusCode_SUCCESS {
		return BuildErrorResponse[vai.PictureToVideoGuideResponse](ctx, submitResp.GetResponseHeader().GetCode(), errors.New(submitResp.GetResponseHeader().GetMsg()), "failed to submit picture to video task")
	}

	return BuildSuccessResponse(&vai.PictureToVideoGuideResponse{
		TaskId:        submitResp.GetTaskId(),
		WorkflowTheme: workflowTheme,
	})
}

func (s *PictureForgeServer) SubmitPictureTaskChain(ctx context.Context, req *vai.SubmitPictureTaskChainRequest) (*vai.SubmitPictureTaskChainResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if req.GetRequestHeader() == nil || req.GetRequestHeader().GetUserId() == "" {
		return BuildErrorResponse[vai.SubmitPictureTaskChainResponse](ctx, vai.StatusCode_INVALID_USER, ErrInvalidUser, "invalid user")
	}

	userID := common.GetUserID(ctx)
	projectID := common.GetProjectID(ctx)

	// 构建并注入提交上下文 JSON（与普通提交保持一致）
	submitCtxJSON := submitcontext.MustMarshal(submitcontext.BuildFromHeader(req.GetRequestHeader()))
	ctx = common.CtxSetStrValue(ctx, constants.CtxSubmitContextJSON, submitCtxJSON)
	// 统一判定会员（在计算总扣费前）
	isMember := false
	if s.subscribeService != nil {
		if info, _ := s.subscribeService.GetSubscribeInfo(ctx, userID, "", "", ""); info != nil && info.GetSubscribeLevel() > 0 {
			isMember = true
		}
	}

	totalCost, firstWorkflowID, err := s.calculateTotalChainCost(ctx, req, isMember)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to calculate chain cost", zap.Error(err))
		return BuildErrorResponse[vai.SubmitPictureTaskChainResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to calculate chain cost")
	}

	var deductionInfoJSON string
	// 第一步记录与组合任务总扣款采用同一免费策略。
	firstWorkflowInfo, wfErr := s.pictureForgeService.GetWorkflowByID(ctx, firstWorkflowID)
	if wfErr != nil {
		zlog.LogWithContext(ctx).Error("failed to get first workflow info for chain", zap.Error(wfErr))
		return BuildErrorResponse[vai.SubmitPictureTaskChainResponse](ctx, vai.StatusCode_REQUEST_FAILED, wfErr, "failed to get first workflow info")
	}
	firstTaskCreditPoints := credit.ImageGenerationCost(firstWorkflowInfo.CreditPoints,
		s.isImageWorkflow(ctx, firstWorkflowInfo), isMember)

	var chainResp *vai.SubmitPictureTaskChainResponse
	// 提前生成第一个任务的 taskID，用于关联扣费流水
	chainFirstTaskID := service.GenerateTaskID()
	err = s.pictureTaskService.Transaction(func(tx *gorm.DB) error {
		// 1) 每日积分（已禁用）
		dailyGrantFailed := false
		// if s.dailyFreeCreditsService != nil {
		// 	if _, _, gerr := s.dailyFreeCreditsService.CheckAndGrantDailyCreditsWithTx(ctx, tx, projectID, userID, isMember); gerr != nil {
		// 		zlog.LogWithContext(ctx).Warn("grant daily credits (chain, tx) failed", zap.Error(gerr))
		// 		dailyGrantFailed = true
		// 	}
		// }
		// 2) 扣款
		if totalCost > 0 {
			deductionInfo, deductErr := s.creditService.DeductCreditsWithTx(ctx, tx, projectID, userID, chainFirstTaskID, constants.TransactionTypeUsageDeduction, int64(totalCost), "Task Chain Credits", isMember)
			if deductErr != nil {
				if errors.Is(deductErr, credit.ErrInsufficientCredits) {
					if !isMember && dailyGrantFailed {
						return errors.New("insufficient credits: daily free grant failed today")
					}
					return deductErr
				}
				return deductErr
			}
			deductionBytes, jsonErr := json.Marshal(deductionInfo)
			if jsonErr != nil {
				return jsonErr
			}
			deductionInfoJSON = string(deductionBytes)
		}
		firstTask, err := s.createOrGetFirstTask(ctx, tx, req, firstWorkflowID, chainFirstTaskID, deductionInfoJSON, firstTaskCreditPoints)
		if err != nil {
			return err
		}
		firstTaskID := firstTask.TaskID
		secondTaskID, err := s.preCreateSecondTask(ctx, tx, deductionInfoJSON, firstTask)
		if err != nil {
			return err
		}

		if s.taskChainService == nil {
			return errors.New("task chain service not initialized")
		}
		_, err = s.taskChainService.CreateChainWithAllTasks(ctx, tx, firstTaskID, secondTaskID, totalCost, deductionInfoJSON)
		if err != nil {
			return err
		}

		chainResp = &vai.SubmitPictureTaskChainResponse{
			ImageTaskId: firstTaskID,
			VideoTaskId: secondTaskID,
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, credit.ErrInsufficientCredits) {
			return BuildErrorResponse[vai.SubmitPictureTaskChainResponse](ctx, vai.StatusCode_CREDIT_POINT_NOT_ENOUGH, err, "insufficient credits for chain")
		}
		return BuildErrorResponse[vai.SubmitPictureTaskChainResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to create task chain")
	}

	// 上报 TASK_CHAIN_SUBMIT 事件
	s.trackTaskChainSubmitEvent(ctx, chainResp.GetImageTaskId(), chainResp.GetVideoTaskId(), firstWorkflowID, totalCost, isMember, req.GetWorkflowInput())

	zlog.LogWithContext(ctx).Info("task chain created successfully",
		zap.String("image_task_id", chainResp.GetImageTaskId()),
		zap.String("video_task_id", chainResp.GetVideoTaskId()),
		zap.Int("total_cost", totalCost))

	return BuildSuccessResponse(chainResp)
}

func (s *PictureForgeServer) getWorkflowTheme(ctx context.Context, workflow *model.Workflow, preloadedKindInfoMap map[string]*model.WorkflowKind) string {
	if workflow == nil || workflow.KindID == "" {
		return ""
	}

	if preloadedKindInfoMap != nil {
		if kindInfo, exists := preloadedKindInfoMap[workflow.KindID]; exists {
			return kindInfo.GetPrimaryTheme()
		}
		zlog.LogWithContext(ctx).Warn("workflow kind not found in preloaded map, attempting dynamic retrieval",
			zap.String("workflow_id", workflow.WorkflowID),
			zap.String("kind_id", workflow.KindID))
	}

	kindInfoMap, err := s.pictureForgeService.GetKindInfoMap(ctx, []string{workflow.KindID})
	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to get workflow kind info for theme",
			zap.String("workflow_id", workflow.WorkflowID),
			zap.String("kind_id", workflow.KindID),
			zap.Error(err))
		return ""
	}

	if kindInfo, exists := kindInfoMap[workflow.KindID]; exists {
		return kindInfo.GetPrimaryTheme()
	}

	zlog.LogWithContext(ctx).Warn("workflow kind info not found",
		zap.String("workflow_id", workflow.WorkflowID),
		zap.String("kind_id", workflow.KindID))
	return ""
}

func (s *PictureForgeServer) ListPictureTools(ctx context.Context, req *vai.ListPictureToolsRequest) (*vai.ListPictureToolsResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if err := validateRequest(req.GetRequestHeader()); err != nil {
		return BuildErrorResponse[vai.ListPictureToolsResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid request header")
	}
	appVersion := req.GetRequestHeader().GetApp().GetAppVersion()
	platform := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
	toolsInfo, err := s.pictureForgeService.ListPictureTools(ctx, appVersion, platform)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to list picture tools", zap.Error(err))
		return BuildErrorResponse[vai.ListPictureToolsResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list picture tools")
	}

	resp := &vai.ListPictureToolsResponse{
		PictureToolsInfos: toolsInfo,
	}

	return BuildSuccessResponse(resp)
}

func (s *PictureForgeServer) GetToolEnhance(ctx context.Context, req *vai.GetToolEnhanceRequest) (*vai.GetToolEnhanceResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if err := validateRequest(req.GetRequestHeader()); err != nil {
		return BuildErrorResponse[vai.GetToolEnhanceResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid request header")
	}

	if req.GetToolId() == "" {
		return BuildErrorResponse[vai.GetToolEnhanceResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "tool_id is required")
	}

	enhances, err := s.pictureForgeService.GetToolEnhance(ctx, req.GetToolId())
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get tool enhance", zap.Error(err), zap.String("tool_id", req.GetToolId()))
		return BuildErrorResponse[vai.GetToolEnhanceResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get tool enhance")
	}

	resp := &vai.GetToolEnhanceResponse{
		ToolEnhances: enhances,
	}

	return BuildSuccessResponse(resp)
}

func (s *PictureForgeServer) GetToolRecommend(ctx context.Context, req *vai.GetToolRecommendRequest) (*vai.GetToolRecommendResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if err := validateRequest(req.GetRequestHeader()); err != nil {
		return BuildErrorResponse[vai.GetToolRecommendResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid request header")
	}

	appVersion := req.GetRequestHeader().GetApp().GetAppVersion()
	platform := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())

	tools, err := s.pictureForgeService.GetToolRecommend(ctx, appVersion, platform)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get tool recommend", zap.Error(err))
		return BuildErrorResponse[vai.GetToolRecommendResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get tool recommend")
	}

	resp := &vai.GetToolRecommendResponse{ToolInfos: tools}
	return BuildSuccessResponse(resp)
}

func (s *PictureForgeServer) SubmitPictureToolsTask(ctx context.Context, req *vai.SubmitPictureToolsTaskRequest) (*vai.SubmitPictureToolsTaskResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	if err := validateRequest(req.GetRequestHeader()); err != nil {
		return BuildErrorResponse[vai.SubmitPictureToolsTaskResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid request header")
	}

	if req.GetPictureToolId() == "" {
		return BuildErrorResponse[vai.SubmitPictureToolsTaskResponse](ctx, vai.StatusCode_INVALID_PARAM, ErrInvalidRequest, "picture_tool_id is required")
	}

	tool, err := s.pictureForgeService.ValidateAndResolvePictureTool(ctx, req)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to validate picture tools task", zap.Error(err))
		return BuildErrorResponse[vai.SubmitPictureToolsTaskResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to validate picture tools task")
	}

	if tool.IsVipTool {
		userID := common.GetUserID(ctx)
		projectID := common.GetProjectID(ctx)
		isSubscriber := false
		if s.subscribeService != nil {
			isSubscriber, err = s.subscribeService.IsUserSubscriber(ctx, projectID, userID, "", "", "")
			if err != nil {
				zlog.LogWithContext(ctx).Error("failed to check user subscription status for VIP tool", zap.Error(err), zap.String("user_id", userID), zap.String("tool_id", tool.ToolID))
			}
		} else {
			zlog.LogWithContext(ctx).Warn("subscribeService is not initialized, denying access to VIP tool", zap.String("user_id", userID), zap.String("tool_id", tool.ToolID))
		}

		if !isSubscriber {
			zlog.LogWithContext(ctx).Info("non-subscriber user attempted to use VIP tool", zap.String("user_id", userID), zap.String("tool_id", tool.ToolID))
			return BuildErrorResponse[vai.SubmitPictureToolsTaskResponse](ctx, vai.StatusCode_SUBSCRIBE_NOT_ACTIVATE, errors.New("membership required"), "This tool is for members only. Please subscribe to use it.")
		}
	}

	workflowInfo, err := s.pictureForgeService.GetWorkflowByID(ctx, tool.RefWorkflowID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get workflow info", zap.Error(err))
		return BuildErrorResponse[vai.SubmitPictureToolsTaskResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get workflow info")
	}

	workflowInputs, err := s.buildWorkflowInputsFromImage(ctx, workflowInfo, req.GetInputImages(), req.GetPrompt())
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to build workflow inputs", zap.Error(err))
		return BuildErrorResponse[vai.SubmitPictureToolsTaskResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to build workflow inputs")
	}

	submitReq := &vai.SubmitPictureForgeTaskRequest{
		RequestHeader: req.GetRequestHeader(),
		WorkflowId:    tool.RefWorkflowID,
		WorkflowInput: workflowInputs,
		WorkflowTheme: "",
		Duration:      req.GetVideoDuration(),
		Resolution:    req.GetResolution(),
	}
	// ctx = context.WithValue(ctx, "optimizedParams", optimizedParams)
	ctx = context.WithValue(ctx, common.EnableOptimizedParamsKey, req.GetEnablePromptOptimization())
	ctx = context.WithValue(ctx, common.PictureToolIDKey, req.GetPictureToolId())
	// 注入提交来源标识
	ctx = context.WithValue(ctx, constants.CtxTaskSubmitSource, constants.SubmitSourceTool)
	ctx = context.WithValue(ctx, constants.CtxToolID, req.GetPictureToolId())
	submitResp, _ := s.SubmitPictureForgeTask(ctx, submitReq)
	if submitResp.GetResponseHeader().GetCode() != vai.StatusCode_SUCCESS {
		return BuildErrorResponse[vai.SubmitPictureToolsTaskResponse](ctx, submitResp.GetResponseHeader().GetCode(), errors.New(submitResp.GetResponseHeader().GetMsg()), "failed to submit picture tools task")
	}

	// 写入自定义 Prompt 历史（成功后，不影响主流程）
	if tool.SupportCustomPrompt && req.GetPrompt() != "" {
		userID := common.GetUserID(ctx)
		if err := s.pictureForgeService.SaveCustomPromptHistory(ctx, userID, tool.ToolID, tool.ToolType, req.GetPrompt()); err != nil {
			zlog.LogWithContext(ctx).Warn("failed to save custom prompt history", zap.Error(err), zap.String("user_id", userID), zap.String("tool_id", tool.ToolID))
		}
	}

	return BuildSuccessResponse(&vai.SubmitPictureToolsTaskResponse{TaskId: submitResp.GetTaskId()})
}

func (s *PictureForgeServer) ListPictureToolsTaskResult(ctx context.Context, req *vai.ListPictureToolsTaskResultRequest) (*vai.ListPictureToolsTaskResultResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	userID := common.GetUserID(ctx)

	if err := validateRequest(req.GetRequestHeader()); err != nil {
		return BuildErrorResponse[vai.ListPictureToolsTaskResultResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid request header")
	}

	if err := validatePageCommon(req.GetPageCommon()); err != nil {
		return BuildErrorResponse[vai.ListPictureToolsTaskResultResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid page common")
	}

	page := req.GetPageCommon().GetPage()
	pageSize := req.GetPageCommon().GetPageSize()

	tasks, _, err := s.pictureTaskService.GetRecentTasks(ctx, userID, "", "", nil, page, pageSize)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get recent tasks", zap.Error(err))
		return BuildErrorResponse[vai.ListPictureToolsTaskResultResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get recent tasks")
	}

	appVersion := req.GetRequestHeader().GetApp().GetAppVersion()
	platform := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
	tools, err := s.pictureForgeService.ListPictureTools(ctx, appVersion, platform)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to list picture tools", zap.Error(err))
		return BuildErrorResponse[vai.ListPictureToolsTaskResultResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list picture tools")
	}

	toolWorkflowMap := make(map[string]bool)
	for _, tool := range tools {
		if tool.GetId() != "" {
			if toolDetail, err := s.pictureForgeService.ValidateAndResolvePictureTool(ctx, &vai.SubmitPictureToolsTaskRequest{
				PictureToolId: tool.GetId(),
			}); err == nil {
				toolWorkflowMap[toolDetail.RefWorkflowID] = true
			}
		}
	}

	taskDetails := make([]*vai.PictureToolsTaskDetail, 0)
	for _, task := range tasks {
		if task.GetWorkflow() != nil && toolWorkflowMap[task.GetWorkflow().GetWorkflowId()] {
			detail := &vai.PictureToolsTaskDetail{
				TaskId:                task.GetTaskId(),
				UserUploadImages:      task.GetUserPictureInfo(),
				TaskStatus:            task.GetTaskStatus(),
				TaskProgress:          task.GetTaskProgress(),
				TaskResultPictureInfo: task.GetTaskResultPictureInfo(),
				TaskResultType:        task.GetKindType(),
				CreditCost:            task.GetCreditCost(),
			}
			taskDetails = append(taskDetails, detail)
		}
	}

	resp := &vai.ListPictureToolsTaskResultResponse{
		PageCommon:  req.GetPageCommon(),
		TaskDetails: taskDetails,
	}
	resp.PageCommon.Total = int32(len(taskDetails))

	return BuildSuccessResponse(resp)
}

func (s *PictureForgeServer) calculateTotalChainCost(ctx context.Context, req *vai.SubmitPictureTaskChainRequest, isMember bool) (int, string, error) {
	firstWorkflowID := req.GetWorkflowId()
	if firstWorkflowID == "" {
		return 0, "", errors.New("workflow_id is required")
	}

	firstWorkflow, err := s.pictureForgeService.GetWorkflowByID(ctx, firstWorkflowID)
	if err != nil {
		return 0, "", fmt.Errorf("failed to get first workflow: %v", err)
	}

	secondWorkflowID, err := s.configService.GetStringConfig(constants.ConfigKeyPictureToVideoGuideWorkflowID)
	if err != nil {
		return 0, "", fmt.Errorf("failed to get second workflow ID: %v", err)
	}

	secondWorkflow, err := s.pictureForgeService.GetWorkflowByID(ctx, secondWorkflowID)
	if err != nil {
		return 0, "", fmt.Errorf("failed to get second workflow: %v", err)
	}

	// 只免除图片步骤，视频步骤保持原计价。
	firstCost := credit.ImageGenerationCost(firstWorkflow.CreditPoints,
		s.isImageWorkflow(ctx, firstWorkflow), isMember)
	totalCost := firstCost + secondWorkflow.CreditPoints
	return totalCost, firstWorkflowID, nil
}

func (s *PictureForgeServer) createOrGetFirstTask(ctx context.Context, tx *gorm.DB, req *vai.SubmitPictureTaskChainRequest, firstWorkflowID, taskID, deductionInfoJSON string, creditPoints int) (*model.PictureTask, error) {
	return s.createFirstTaskDirectly(ctx, tx, req, firstWorkflowID, taskID, deductionInfoJSON, creditPoints)
}

func (s *PictureForgeServer) createFirstTaskDirectly(ctx context.Context, tx *gorm.DB, req *vai.SubmitPictureTaskChainRequest, workflowID, taskID, deductionInfoJSON string, creditPoints int) (*model.PictureTask, error) {
	taskParams, userPictureInfoJSON, workflowInfo, err := s.pictureTaskService.PrepareTaskInputsAndWorkflow(ctx, tx, req.GetWorkflowInput(), workflowID, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to prepare task inputs and workflow: %v", err)
	}
	// 持久化时使用传入的积分（允许为会员图像任务置0），否则回退工作流默认值
	creditToPersist := workflowInfo.CreditPoints
	if creditPoints >= 0 {
		creditToPersist = creditPoints
	}
	task := &model.PictureTask{
		TaskID:              taskID,
		ProjectID:           common.GetProjectID(ctx),
		UserID:              common.GetUserID(ctx),
		WorkflowID:          workflowID,
		Status:              int32(vai.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING),
		ParamJSON:           mustMarshal(taskParams),
		UserPictureInfoJson: string(userPictureInfoJSON),
		SubmitContextJson:   common.CtxGetStrValue(ctx, constants.CtxSubmitContextJSON),
		CreditPoints:        creditToPersist,
		CreditDeductionInfo: deductionInfoJSON,
		Theme:               "",
		WorkflowType:        vai.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		CanRetry:            true,
	}

	if err := tx.WithContext(ctx).Create(task).Error; err != nil {
		return nil, err
	}

	return task, nil
}

func (s *PictureForgeServer) preCreateSecondTask(ctx context.Context, tx *gorm.DB, deductionInfoJSON string, firstTask *model.PictureTask) (string, error) {
	secondWorkflowID, err := s.configService.GetStringConfig(constants.ConfigKeyPictureToVideoGuideWorkflowID)
	if err != nil {
		return "", err
	}

	secondWorkflow, err := s.pictureForgeService.GetWorkflowByID(ctx, secondWorkflowID)
	if err != nil {
		return "", err
	}

	repositories := s.pictureTaskService.GetRepositories()
	workflowKind, err := repositories.PicForge.GetWorkflowKind(ctx, secondWorkflow.KindID)
	if err != nil {
		return "", err
	}

	taskID, err := s.pictureTaskService.CreateTaskRecordWithStatus(
		ctx,
		tx,
		common.GetProjectID(ctx),
		common.GetUserID(ctx),
		secondWorkflowID,
		firstTask.UserPictureInfoJson,
		`{}`,
		firstTask.SubmitContextJson,
		workflowKind.GetPrimaryTheme(),
		workflowKind.KindType,
		secondWorkflow.ApiConfig,
		secondWorkflow.CreditPoints,
		deductionInfoJSON,
		secondWorkflow.Provider,
		vai.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_CHAIN,
		"", // no comment for chain tasks
	)
	if err != nil {
		return "", err
	}

	return taskID, nil
}

// GetWorkflowRecommend 获取工作流推荐列表
// 基于近7天使用率Top10的热门workflow进行随机推荐
func (s *PictureForgeServer) GetWorkflowRecommend(ctx context.Context, req *vai.GetWorkflowRecommendRequest) (*vai.GetWorkflowRecommendResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 验证请求参数
	if err := validateRequest(req.GetRequestHeader()); err != nil {
		return BuildErrorResponse[vai.GetWorkflowRecommendResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid request header")
	}

	// 获取limit参数,默认为10
	limit := req.GetLimit()
	if limit <= 0 {
		limit = 10
	}

	// 提取客户端版本和平台信息用于版本过滤
	appVersion := req.GetRequestHeader().GetApp().GetAppVersion()
	platform := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())

	// 调用Service层获取推荐workflow列表
	workflows, err := s.pictureForgeService.GetWorkflowRecommend(ctx, limit, appVersion, platform)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get workflow recommend", zap.Error(err))
		return BuildErrorResponse[vai.GetWorkflowRecommendResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get workflow recommend")
	}

	// 记录成功日志
	zlog.LogWithContext(ctx).Info("workflow recommend completed",
		zap.Int32("limit", limit),
		zap.Int("workflows_count", len(workflows)))

	resp := &vai.GetWorkflowRecommendResponse{Workflows: workflows}
	return BuildSuccessResponse(resp)
}

func mustMarshal(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// GetBannerList 获取Banner列表
func (s *PictureForgeServer) GetBannerList(ctx context.Context, req *vai.GetBannerListRequest) (*vai.GetBannerListResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 验证请求头
	if err := validateRequest(req.GetRequestHeader()); err != nil {
		return BuildErrorResponse[vai.GetBannerListResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid request header")
	}

	// 调用Service层获取Banner列表
	banners, err := s.pictureForgeService.GetBannerList(ctx)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get banner list", zap.Error(err))
		return BuildErrorResponse[vai.GetBannerListResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to get banner list")
	}

	zlog.LogWithContext(ctx).Info("get banner list success", zap.Int("count", len(banners)))

	resp := &vai.GetBannerListResponse{
		Banners: banners,
	}

	return BuildSuccessResponse(resp)
}

// ClickBanner 处理Banner点击
func (s *PictureForgeServer) ClickBanner(ctx context.Context, req *vai.ClickBannerRequest) (*vai.ClickBannerResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 验证请求头
	if err := validateRequest(req.GetRequestHeader()); err != nil {
		return BuildErrorResponse[vai.ClickBannerResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid request header")
	}

	// 验证请求参数
	if req.GetBannerKey() == "" {
		return BuildErrorResponse[vai.ClickBannerResponse](ctx, vai.StatusCode_INVALID_PARAM,
			errors.New("banner_key is required"), "banner_key is required")
	}

	// 获取用户ID和项目ID
	userID := common.GetUserID(ctx)
	projectID := common.GetProjectID(ctx)

	if userID == "" {
		return BuildErrorResponse[vai.ClickBannerResponse](ctx, vai.StatusCode_INVALID_PARAM,
			ErrInvalidUser, "user_id is required")
	}

	bannerKey := req.GetBannerKey()

	// 调用Service层处理点击事件
	shouldShow, popupInfo, err := s.pictureForgeService.ClickBanner(ctx, userID, bannerKey)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to handle banner click",
			zap.String("user_id", userID),
			zap.String("banner_key", bannerKey),
			zap.Error(err))
		return BuildErrorResponse[vai.ClickBannerResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to handle banner click")
	}

	// 如果需要奖励积分，则在事务中执行
	if popupInfo != nil && popupInfo.GetPointsAwarded() > 0 {
		// 生成唯一的sourceID，用于积分幂等性控制
		sourceID := fmt.Sprintf("banner_click_%s_%s", userID, bannerKey)
		pointsAwarded := int(popupInfo.GetPointsAwarded())

		err := s.pictureForgeService.Transaction(func(tx *gorm.DB) error {
			// 1. 插入banner点击记录
			if err := s.pictureForgeService.ClickBannerWithReward(ctx, tx, userID, bannerKey, pointsAwarded, shouldShow); err != nil {
				return err
			}

			// 2. 增加用户积分
			// 积分有效期设置为1年
			expiredAt := time.Now().AddDate(1, 0, 0)
			if err := s.creditService.AddCreditsWithTx(
				ctx,
				tx,
				projectID,
				userID,
				sourceID,
				constants.CreditTypeEventGrant,      // Banner奖励积分类型
				constants.TransactionTypeEventGrant, // Banner奖励事务类型
				int64(pointsAwarded),
				expiredAt,
				"Banner点击奖励: "+bannerKey,
				0, // subscribeLevel
			); err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to grant banner reward in transaction",
				zap.String("user_id", userID),
				zap.String("banner_key", bannerKey),
				zap.Int("points", pointsAwarded),
				zap.Error(err))

			// 如果是重复发放的错误（唯一索引冲突），返回成功但不发放积分
			if errors.Is(err, gorm.ErrDuplicatedKey) ||
				(err.Error() != "" && (errors.Is(err, gorm.ErrDuplicatedKey) ||
					strings.Contains(err.Error(), "Duplicate entry") ||
					strings.Contains(err.Error(), "UNIQUE constraint failed"))) {
				zlog.LogWithContext(ctx).Info("banner reward already granted",
					zap.String("user_id", userID),
					zap.String("banner_key", bannerKey))

				// 已经发放过，返回不弹窗
				resp := &vai.ClickBannerResponse{
					ShouldShow: false,
					Popup:      nil,
				}
				return BuildSuccessResponse(resp)
			}

			return BuildErrorResponse[vai.ClickBannerResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to grant banner reward")
		}

		zlog.LogWithContext(ctx).Info("banner click reward granted successfully",
			zap.String("user_id", userID),
			zap.String("banner_key", bannerKey),
			zap.Int("points", pointsAwarded))
	}

	resp := &vai.ClickBannerResponse{
		ShouldShow: shouldShow,
		Popup:      popupInfo,
	}

	return BuildSuccessResponse(resp)
}

// ListImageGenerationModels 获取生图模型列表
func (s *PictureForgeServer) ListImageGenerationModels(ctx context.Context, req *vai.ListImageGenerationModelsRequest) (*vai.ListImageGenerationModelsResponse, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 验证请求头
	// if err := validateRequest(req.GetRequestHeader()); err != nil {
	// 	return BuildErrorResponse[vai.ListImageGenerationModelsResponse](ctx, vai.StatusCode_INVALID_PARAM, err, "invalid request header")
	// }

	// 获取项目ID
	projectID := common.GetProjectID(ctx)
	if projectID == "" || projectID == constants.ProjectIdVisualAI {
		projectID = constants.ProjectIdVisionAI
	}

	// 从数据库获取启用的生图模型列表
	modelDao := dao.NewImageGenerationModelDao(db.GetDB())
	models, err := modelDao.ListEnabledModels(ctx, projectID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to list image generation models from database",
			zap.Error(err),
			zap.String("project_id", projectID))
		return BuildErrorResponse[vai.ListImageGenerationModelsResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list image generation models")
	}

	// 转换为 Proto 格式
	modelInfos := make([]*vai.ImageGenerationModelInfo, 0, len(models))
	for _, model := range models {
		modelInfos = append(modelInfos, model.ToProto())
	}

	zlog.LogWithContext(ctx).Info("list image generation models success",
		zap.Int("count", len(modelInfos)),
		zap.String("project_id", projectID))

	resp := &vai.ListImageGenerationModelsResponse{
		Models: modelInfos,
	}

	return BuildSuccessResponse(resp)
}

// extractUserInputFromWorkflowInputs 从 WorkflowInput 列表中提取用户输入
// 返回 user_prompt（截断至 500 字符）和 input_images（图片 URL 列表）
func extractUserInputFromWorkflowInputs(inputs []*vai.WorkflowInput) (userPrompt string, inputImages []string) {
	inputImages = make([]string, 0)
	for _, input := range inputs {
		switch input.GetInputType() {
		case vai.MessageType_MT_TEXT:
			// 提取第一个非空文本作为 user_prompt
			if userPrompt == "" && input.GetInputContent() != "" {
				userPrompt = input.GetInputContent()
				// 截断至 500 字符
				if len(userPrompt) > 500 {
					userPrompt = userPrompt[:500]
				}
			}
		case vai.MessageType_MT_IMAGE:
			// 收集所有图片 URL
			if input.GetInputContent() != "" {
				inputImages = append(inputImages, input.GetInputContent())
			}
		}
	}
	return userPrompt, inputImages
}

// trackTaskChainSubmitEvent 上报 TASK_CHAIN_SUBMIT 事件
func (s *PictureForgeServer) trackTaskChainSubmitEvent(
	ctx context.Context,
	imageTaskID string,
	videoTaskID string,
	firstWorkflowID string,
	totalCost int,
	isMember bool,
	workflowInputs []*vai.WorkflowInput,
) {
	// 检查 eventReporter 是否为 nil
	if s.eventReporter == nil {
		return
	}

	// 提取用户输入
	userPrompt, inputImages := extractUserInputFromWorkflowInputs(workflowInputs)

	// 构建 payload
	payload := map[string]any{
		"image_task_id":     imageTaskID,
		"video_task_id":     videoTaskID,
		"first_workflow_id": firstWorkflowID,
		"total_cost":        totalCost,
		"is_member":         isMember,
		"user_prompt":       userPrompt,
		"input_images":      inputImages,
	}

	// 使用 EventBuilder 构建并异步上报
	s.eventReporter.NewEvent(event_reporter.EventTypeTaskChainSubmit).
		FromContext(ctx).
		Payload(payload).
		Track()

	zlog.LogWithContext(ctx).Debug("task chain submit event tracked",
		zap.String("image_task_id", imageTaskID),
		zap.String("video_task_id", videoTaskID))
}
