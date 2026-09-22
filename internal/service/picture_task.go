package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dundunHa/comfy2go/client"
	"github.com/go-redis/redis"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/conf"
	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	aigcclient "va_visionai_server/internal/service/ai_clients/aigc_core"
	"va_visionai_server/internal/service/cache"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/service/event"
	"va_visionai_server/internal/service/event_reporter"
	"va_visionai_server/internal/service/picture_generate"
	"va_visionai_server/internal/service/push_gateway"
	qcfg "va_visionai_server/internal/service/quota"
	"va_visionai_server/internal/utils"
	pb "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
	v1 "va_visionai_server/internal/aibrain/v1"

	_ "golang.org/x/image/webp"
	_ "image/jpeg"
	_ "image/png"
)

const (
	taskProgressKeyPrefix       = "visionai:picture_task_progress:"
	userProcessingLockKeyPrefix = "visionai:user_processing_lock:"
)

type PromptOptimizationConfig struct {
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
}

type PictureTaskService struct {
	db                         *gorm.DB
	taskDao                    *dao.PictureTaskDao
	picForgeDao                *dao.PictureForgeDao
	uploadDao                  *dao.UploadDao
	promptDao                  *dao.PromptDao
	rdb                        *redis.Client
	httpClient                 *http.Client
	llmFactory                 common.LLMFactory
	evaluateService            *picture_generate.EvaluateService
	imageGenerator             *picture_generate.ImageGenerator
	faceDetectService          *FaceDetectService
	executorService            *picture_generate.ExecutorService
	creditService              credit.Service
	configService              *ConfigService
	taskChainService           *TaskChainService
	aigcClient                 *aigcclient.Client
	cacheService               *cache.CacheService
	workflowReplacementService *WorkflowReplacementService
	dailyFreeCreditsService    *DailyFreeCreditsService
	subscribeService           *SubscribeService
	eventReporter              event_reporter.EventReporter

	// 新增组件
	taskConfigManager *qcfg.TaskConfigManager
	taskQuotaChecker  *qcfg.TaskQuotaChecker
	taskStateManager  *qcfg.TaskStateManager

	// 推送网关客户端
	pushGatewayClient *push_gateway.Client
}

func (s *PictureTaskService) SetExecutorService(executorService *picture_generate.ExecutorService) {
	s.executorService = executorService
}

func (s *PictureTaskService) SetTaskChainService(taskChainService *TaskChainService) {
	s.taskChainService = taskChainService
}

func (s *PictureTaskService) SetDailyFreeCreditsService(dailySvc *DailyFreeCreditsService) {
	s.dailyFreeCreditsService = dailySvc
}

func (s *PictureTaskService) SetSubscribeService(subscribeSvc *SubscribeService) {
	s.subscribeService = subscribeSvc
}

func (s *PictureTaskService) SetPushGatewayClient(client *push_gateway.Client) {
	s.pushGatewayClient = client
}

func NewComfyClient(ctx context.Context) *client.ComfyClient {
	return client.NewComfyClientWithTimeout(conf.GlobalConfig.ComfyUI.Server, conf.GlobalConfig.ComfyUI.Port, nil, 30, 3)
}

func NewPictureTaskService(
	db *gorm.DB,
	llmFactory common.LLMFactory,
	taskDao *dao.PictureTaskDao,
	picForgeDao *dao.PictureForgeDao,
	uploadDao *dao.UploadDao,
	promptDao *dao.PromptDao,
	rdb *redis.Client,
	evaluateService *picture_generate.EvaluateService,
	imageGenerator *picture_generate.ImageGenerator,
	faceDetectService *FaceDetectService,
	executorService *picture_generate.ExecutorService,
	creditService credit.Service,
	configService *ConfigService,
	aigcClient *aigcclient.Client,
	cacheService *cache.CacheService,
	workflowReplacementService *WorkflowReplacementService,
	eventReporter event_reporter.EventReporter,
) *PictureTaskService {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   600 * time.Second,
			KeepAlive: 600 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       600 * time.Second,
		TLSHandshakeTimeout:   60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{Transport: transport}

	// 初始化新组件
	// 通过接口适配器传入 configService，避免循环依赖
	taskConfigManager := qcfg.NewTaskConfigManager(configService, cacheService)
	taskQuotaChecker := qcfg.NewTaskQuotaChecker(rdb, taskDao)
	taskStateManager := qcfg.NewTaskStateManager(rdb, taskDao, taskQuotaChecker)

	service := &PictureTaskService{
		db:                         db,
		taskDao:                    taskDao,
		picForgeDao:                picForgeDao,
		uploadDao:                  uploadDao,
		promptDao:                  promptDao,
		rdb:                        rdb,
		httpClient:                 httpClient,
		llmFactory:                 llmFactory,
		evaluateService:            evaluateService,
		imageGenerator:             imageGenerator,
		faceDetectService:          faceDetectService,
		executorService:            executorService,
		creditService:              creditService,
		configService:              configService,
		aigcClient:                 aigcClient,
		cacheService:               cacheService,
		workflowReplacementService: workflowReplacementService,
		eventReporter:              eventReporter,
		// 新组件
		taskConfigManager: taskConfigManager,
		taskQuotaChecker:  taskQuotaChecker,
		taskStateManager:  taskStateManager,
	}

	return service
}

func (s *PictureTaskService) GetDB() *gorm.DB { return s.db }

func (s *PictureTaskService) GetRepositories() *dao.Repositories { return dao.NewRepositories(s.db) }

func (s *PictureTaskService) GetRedis() *redis.Client { return s.rdb }

func (s *PictureTaskService) GetTaskQuotaChecker() *qcfg.TaskQuotaChecker { return s.taskQuotaChecker }

// 新增只读getter，供调度器装配使用
func (s *PictureTaskService) GetTaskConfigManager() *qcfg.TaskConfigManager {
	return s.taskConfigManager
}
func (s *PictureTaskService) GetTaskStateManager() *qcfg.TaskStateManager { return s.taskStateManager }

func (s *PictureTaskService) getWANXResolution(aspectRatioText, quality string) (width, height int) {
	if s.cacheService == nil || s.configService == nil {
		return 460, 460
	}

	cacheKey := constants.ConfigKeyWANX22ResolutionMap

	var config map[string]map[string]struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}

	err := s.cacheService.GetOrSet(context.Background(), cacheKey, &config, 10*time.Minute, func() (interface{}, error) {
		configStr, err := s.configService.GetStringConfig(constants.ConfigKeyWANX22ResolutionMap)
		if err != nil {
			return nil, err
		}

		var parsedConfig map[string]map[string]struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		}

		if err := json.Unmarshal([]byte(configStr), &parsedConfig); err != nil {
			return nil, err
		}

		return parsedConfig, nil
	})

	if err != nil {
		zlog.LogWithContext(context.Background()).Warn("failed to get WANX resolution config, using default",
			zap.String("aspect_ratio", aspectRatioText),
			zap.String("quality", quality),
			zap.Error(err))
		return 460, 460
	}

	if qualityMap, exists := config[aspectRatioText]; exists {
		if resolution, qualityExists := qualityMap[quality]; qualityExists {
			return resolution.Width, resolution.Height
		}
	}

	zlog.LogWithContext(context.Background()).Debug("WANX resolution not found in config, using default",
		zap.String("aspect_ratio", aspectRatioText),
		zap.String("quality", quality))
	return 460, 460
}

func GenerateTaskID() string {
	return "task_" + uuid.New().String()
}

func (s *PictureTaskService) validateSubmitTaskRequest(req *pb.SubmitPictureForgeTaskRequest) (userID string, err error) {
	if req.GetRequestHeader() == nil || req.GetRequestHeader().GetUserId() == "" {
		return "", errors.New("invalid user")
	}
	return req.GetRequestHeader().GetUserId(), nil
}

func (s *PictureTaskService) PrepareTaskInputsAndWorkflow(ctx context.Context,
	tx *gorm.DB,
	workflowInputs []*pb.WorkflowInput,
	workflowID string,
	duration string,
	resolution string,
) (
	taskParams map[string]string,
	userPictureInfoJSONString string,
	workflowInfo *model.Workflow,
	err error,
) {
	passThrough, userPictureInfos, aspectRatioText, resultAspect := s.collectInputsAndAspect(ctx, tx, workflowInputs)
	taskParams = make(map[string]string)
	for k, v := range passThrough {
		taskParams[k] = v
	}
	if resultAspect != "" {
		taskParams["result_aspect_ratio"] = resultAspect
	}
	if aspectRatioText != "" {
		taskParams["aspect_ratio_text"] = aspectRatioText
	}

	userPictureInfoBytes, jsonErr := json.Marshal(userPictureInfos)
	if jsonErr != nil {
		zlog.LogWithContext(ctx).Error("failed to marshal user picture info", zap.Error(jsonErr))
		return nil, "", nil, errors.Wrap(jsonErr, "marshal user_picture_info")
	}
	userPictureInfoJSONString = string(userPictureInfoBytes)

	workflowInfo, err = s.picForgeDao.GetWorkflow(ctx, tx, workflowID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("NotFoundWorkflow", zap.String("workflowID", workflowID), zap.Error(err))
		return nil, "", nil, errors.Wrap(err, "get workflow info")
	}
	taskParams["workflow_id"] = workflowID
	taskParams["prompt"] = workflowInfo.Prompt
	taskParams["api_iden"] = workflowInfo.ApiConfig.ApiIden
	taskParams["effect_scene"] = workflowInfo.ApiConfig.EffectScene
	taskParams["task_type"] = workflowInfo.ApiConfig.TaskType

	finalDuration, workflowDuration, finalResolution := s.applyDurationAndResolution(workflowInfo.ApiConfig, duration, resolution)
	if finalDuration != "" {
		taskParams["duration"] = finalDuration
	}
	if workflowDuration != "" {
		taskParams["workflow_duration"] = workflowDuration
	}
	if finalResolution != "" {
		taskParams["resolution"] = finalResolution
	}

	if workflowInfo.ApiConfig.ModelName == constants.WANXModelName && aspectRatioText != "" && finalResolution != "" {
		if w, h, ok := s.applyWANXResolutionMapping(workflowInfo.ApiConfig.ModelName, aspectRatioText, strings.ToUpper(finalResolution)); ok {
			taskParams["workflow_width"] = strconv.Itoa(w)
			taskParams["workflow_height"] = strconv.Itoa(h)
			zlog.LogWithContext(ctx).Info("应用WANX分辨率映射",
				zap.String("aspect_ratio_text", aspectRatioText),
				zap.String("quality", strings.ToUpper(finalResolution)),
				zap.Int("final_width", w),
				zap.Int("final_height", h))
		}
	}
	// 始终确保 WANX 下在有分辨率时具备工作流宽高（映射失败则使用默认值）
	s.ensureWanxDims(ctx, workflowInfo.ApiConfig.ModelName, aspectRatioText, strings.ToUpper(finalResolution), taskParams)

	return taskParams, userPictureInfoJSONString, workflowInfo, nil
}

// PrepareTaskInputsFromModelDirect 从模型直连模式请求构建任务参数
// 用于处理用户直接指定模型的场景，不依赖预定义workflow
func (s *PictureTaskService) PrepareTaskInputsFromModelDirect(
	ctx context.Context,
	tx *gorm.DB,
	req *pb.SubmitPictureForgeTaskRequest,
	workflowInfo *model.Workflow, // 虚拟workflow，由API层构建
) (
	taskParams map[string]string,
	userPictureInfoJSONString string,
	err error,
) {
	taskParams = make(map[string]string)

	// 基础参数
	taskParams["workflow_id"] = workflowInfo.WorkflowID
	taskParams["prompt"] = req.GetUserPrompt()
	taskParams["api_iden"] = workflowInfo.ApiConfig.ApiIden
	taskParams["effect_scene"] = workflowInfo.ApiConfig.EffectScene
	taskParams["model_name"] = workflowInfo.ApiConfig.ModelName

	// 负面prompt
	if req.GetNegativePrompt() != "" {
		taskParams["negative_prompt"] = req.GetNegativePrompt()
	}

	// 判断任务类型：文生图 或 图生图
	hasInputImages := len(req.GetUserImages()) > 0
	if hasInputImages {
		taskParams["task_type"] = "image2image"

		// 处理用户上传的图片
		userPictureInfos := make([]model.UserPictureInfo, 0, len(req.GetUserImages()))
		for idx, imageURL := range req.GetUserImages() {
			userPictureInfos = append(userPictureInfos, model.UserPictureInfo{
				PhotoURL:    imageURL,
				AspectRatio: 0, // 可以后续计算
			})

			// 将图片URL添加到taskParams
			taskParams[fmt.Sprintf("input_image_%d", idx+1)] = imageURL
		}

		// 序列化用户图片信息
		userPictureInfoBytes, jsonErr := json.Marshal(userPictureInfos)
		if jsonErr != nil {
			zlog.LogWithContext(ctx).Error("failed to marshal user picture info", zap.Error(jsonErr))
			return nil, "", errors.Wrap(jsonErr, "marshal user_picture_info")
		}
		userPictureInfoJSONString = string(userPictureInfoBytes)

		// 图生图强度
		strength := req.GetStrength()
		if strength <= 0 {
			strength = 0.75 // 默认值
		}
		taskParams["strength"] = fmt.Sprintf("%.2f", strength)
	} else {
		taskParams["task_type"] = "text2image"
		userPictureInfoJSONString = "[]"
	}

	// 模型配置参数
	modelConfig := req.GetModelConfig()
	if modelConfig != nil {
		// 图片数量
		if modelConfig.GetNumImages() > 0 {
			taskParams["num_images"] = strconv.Itoa(int(modelConfig.GetNumImages()))
		} else {
			taskParams["num_images"] = "1" // 默认生成1张
		}

		// 推理步数
		if modelConfig.GetSteps() > 0 {
			taskParams["steps"] = strconv.Itoa(int(modelConfig.GetSteps()))
		}

		// CFG scale
		if modelConfig.GetCfgScale() > 0 {
			taskParams["cfg_scale"] = fmt.Sprintf("%.1f", modelConfig.GetCfgScale())
		}

		// 种子
		if modelConfig.GetSeed() >= 0 {
			taskParams["seed"] = strconv.FormatInt(modelConfig.GetSeed(), 10)
		} else {
			// 随机种子
			taskParams["seed"] = strconv.FormatInt(time.Now().UnixNano(), 10)
		}

		// 宽度和高度
		if modelConfig.GetWidth() > 0 && modelConfig.GetHeight() > 0 {
			taskParams["width"] = strconv.Itoa(int(modelConfig.GetWidth()))
			taskParams["height"] = strconv.Itoa(int(modelConfig.GetHeight()))
			taskParams["resolution"] = fmt.Sprintf("%dx%d", modelConfig.GetWidth(), modelConfig.GetHeight())
		} else {
			// 使用默认分辨率
			taskParams["resolution"] = workflowInfo.ApiConfig.DefaultResolution
		}

		// 图片质量
		if modelConfig.GetQuality() != "" {
			taskParams["quality"] = modelConfig.GetQuality()
		}

		// 图片宽高比
		if modelConfig.GetAspectRatio() != "" {
			taskParams["aspect_ratio"] = modelConfig.GetAspectRatio()
		}
	}

	zlog.LogWithContext(ctx).Info("prepared task params from model direct mode",
		zap.String("model_name", workflowInfo.ApiConfig.ModelName),
		zap.String("task_type", taskParams["task_type"]),
		zap.Int("num_input_images", len(req.GetUserImages())),
		zap.String("resolution", taskParams["resolution"]))

	return taskParams, userPictureInfoJSONString, nil
}


// collectInputsAndAspect 透传工作流输入并计算图片输入的纵横比（后者覆盖前者）
func (s *PictureTaskService) collectInputsAndAspect(
	ctx context.Context,
	tx *gorm.DB,
	inputs []*pb.WorkflowInput,
) (
	passThrough map[string]string,
	userPics []model.UserPictureInfo,
	aspectText string,
	resultAspect string,
) {
	passThrough = make(map[string]string)
	userPics = make([]model.UserPictureInfo, 0, len(inputs))

	for _, input := range inputs {
		passThrough[input.GetInputName()] = input.GetInputContent()

		if input.GetInputType() != pb.MessageType_MT_IMAGE {
			continue
		}

		info := model.UserPictureInfo{PhotoURL: input.GetInputContent(), AspectRatio: 1}
		at, av, w, h, ok := s.resolveImageAspect(ctx, tx, input.GetInputContent())
		if ok && w > 0 && h > 0 {
			ar := float64(h) / float64(w)
			info.AspectRatio = ar
			resultAspect = av
			aspectText = at
		}
		userPics = append(userPics, info)
	}

	return passThrough, userPics, aspectText, resultAspect
}

// applyDurationAndResolution 根据ApiConfig和入参计算最终时长与分辨率
func (s *PictureTaskService) applyDurationAndResolution(
	api model.WorkflowApiConfig,
	inDuration string,
	inResolution string,
) (
	finalDuration string,
	workflowDuration string,
	finalResolution string,
) {
	finalDuration = inDuration
	if finalDuration == "" && api.DefaultTimeDuration > 0 {
		finalDuration = strconv.Itoa(int(api.DefaultTimeDuration))
	}
	if api.TimeRatio > 0 && finalDuration != "" {
		if cd, err := strconv.Atoi(finalDuration); err == nil {
			workflowDuration = strconv.Itoa(int(float32(cd) * api.TimeRatio))
		}
	}

	finalResolution = inResolution
	if finalResolution == "" && api.DefaultResolution != "" {
		finalResolution = api.DefaultResolution
	}
	return
}

// applyWANXResolutionMapping 依据 WANX 模型与纵横比映射得到最终宽高
func (s *PictureTaskService) applyWANXResolutionMapping(
	modelName string,
	aspectText string,
	quality string,
) (width, height int, ok bool) {
	if modelName != constants.WANXModelName || aspectText == "" || quality == "" {
		return 0, 0, false
	}
	w, h := s.getWANXResolution(aspectText, quality)
	return w, h, true
}

// ensureWanxDims 确保在 WANX 模型 + 给定分辨率 下，一定得到工作流宽高
// - 若已有映射结果则不变
// - 若无法映射，则回填默认 345x460
func (s *PictureTaskService) ensureWanxDims(
	ctx context.Context,
	modelName string,
	aspectText string,
	quality string,
	taskParams map[string]string,
) {
	if modelName != constants.WANXModelName || strings.TrimSpace(quality) == "" {
		return
	}
	if _, ok := taskParams["workflow_width"]; ok {
		if _, okh := taskParams["workflow_height"]; okh {
			return
		}
	}
	if aspectText != "" {
		if w, h, ok := s.applyWANXResolutionMapping(modelName, aspectText, quality); ok {
			taskParams["workflow_width"] = strconv.Itoa(w)
			taskParams["workflow_height"] = strconv.Itoa(h)
			return
		}
	}
	// 默认兜底
	taskParams["workflow_width"] = "480"
	taskParams["workflow_height"] = "832"
	zlog.LogWithContext(ctx).Info("WANX未能映射分辨率，使用默认宽高",
		zap.String("quality", quality),
		zap.String("default_width", "480"),
		zap.String("default_height", "832"))
}

// resolveImageAspect 编排 URL→DB→hash→远程轻量读取，得到纵横比文本与数值
// 返回：aspectText（如"1:1"）、aspectValue（height/width的小数）、w、h、ok
func (s *PictureTaskService) resolveImageAspect(
	ctx context.Context,
	tx *gorm.DB,
	rawURL string,
) (string, string, int, int, bool) {
	cleanURL := utils.CleanURL(rawURL)
	// 1) 按地址查
	if info, err := s.uploadDao.GetImgUrlWithout(ctx, tx, cleanURL); err == nil && info != nil {
		if info.PhotoWidth > 0 && info.PhotoHeight > 0 {
			w := int(info.PhotoWidth)
			h := int(info.PhotoHeight)
			return utils.GetStandardAspectRatio(w, h), strconv.FormatFloat(float64(h)/float64(w), 'f', 6, 64), w, h, true
		}
	}

	// 2) 按文件名中的 MD5 (仅受信UGC域)
	if utils.IsUgcHost(cleanURL) {
		if md5, ok := utils.ExtractUgcMD5(cleanURL); ok {
			if infoByHash, err := s.uploadDao.GetExtendedByHash(ctx, tx, md5); err == nil && infoByHash != nil {
				if infoByHash.PhotoWidth > 0 && infoByHash.PhotoHeight > 0 {
					w := int(infoByHash.PhotoWidth)
					h := int(infoByHash.PhotoHeight)
					return utils.GetStandardAspectRatio(w, h), strconv.FormatFloat(float64(h)/float64(w), 'f', 6, 64), w, h, true
				}
			}
		}
	}

	// 3) 远程兜底：轻量读取图片头部，DecodeConfig 获取尺寸
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx2, "GET", cleanURL, nil)
	if err == nil {
		if resp, httpErr := s.httpClient.Do(req); httpErr == nil {
			defer resp.Body.Close()
			// 限制最多读取 512KB
			const maxPeek = 512 * 1024
			data, _ := io.ReadAll(io.LimitReader(resp.Body, maxPeek))
			if len(data) > 0 {
				if cfg, format, decErr := image.DecodeConfig(bytes.NewReader(data)); decErr == nil && cfg.Width > 0 && cfg.Height > 0 {
					w := cfg.Width
					h := cfg.Height
					zlog.LogWithContext(ctx).Debug("decode image config for aspect",
						zap.String("format", format),
						zap.Int("width", w),
						zap.Int("height", h))
					return utils.GetStandardAspectRatio(w, h), strconv.FormatFloat(float64(h)/float64(w), 'f', 6, 64), w, h, true
				}
			}
		}
	}

	return "", "", 0, 0, false
}

func (s *PictureTaskService) processWorkflowKind(ctx context.Context, workflowInfo *model.Workflow, taskParams map[string]string) (
	workflowKind *model.WorkflowKind,
	err error,
) {
	workflowKind, err = s.picForgeDao.GetWorkflowKind(ctx, workflowInfo.KindID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("NotFoundWorkflowKind", zap.String("kindID", workflowInfo.KindID), zap.Error(err))
		return nil, errors.Wrap(err, "get workflow kind")
	}
	if workflowKind.KindType == pb.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO.String() {
		taskParams["workflow_type"] = pb.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO.String()
	} else if workflowKind.KindType == pb.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String() {
		taskParams["workflow_type"] = pb.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String()
	}
	return workflowKind, nil
}

func (s *PictureTaskService) performFaceValidation(ctx context.Context, workflowInfo *model.Workflow, workflowInputs []*pb.WorkflowInput) error {
	if workflowInfo.FaceCount <= 0 {
		return nil
	}
	zlog.LogWithContext(ctx).Info("checking face count requirement", zap.Int32("required_faces", workflowInfo.FaceCount), zap.Int("input_count", len(workflowInputs)))
	totalFaceCount := 0
	for idx, input := range workflowInputs {
		inputContent := input.GetInputContent()
		if !strings.HasPrefix(inputContent, "http://") && !strings.HasPrefix(inputContent, "https://") {
			zlog.LogWithContext(ctx).Debug("skipping non-URL input for face detection", zap.Int("input_index", idx), zap.String("input_content", inputContent))
			continue
		}
		zlog.LogWithContext(ctx).Debug("processing input for face detection", zap.Int("input_index", idx), zap.String("input_url", inputContent))
		resp, httpErr := s.httpClient.Get(inputContent)
		if httpErr != nil {
			zlog.LogWithContext(ctx).Error("failed to download image for face detection", zap.Int("input_index", idx), zap.String("input_url", inputContent), zap.Error(httpErr))
			return fmt.Errorf("failed to download image for face detection (input %d): %w", idx, httpErr)
		}
		defer resp.Body.Close()
		imageBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			zlog.LogWithContext(ctx).Error("failed to read image data for face detection", zap.Int("input_index", idx), zap.String("input_url", inputContent), zap.Error(readErr))
			return fmt.Errorf("failed to read image data for face detection (input %d): %w", idx, readErr)
		}
		faceCount, detectErr := s.faceDetectService.FaceDetect(imageBytes)
		if detectErr != nil {
			zlog.LogWithContext(ctx).Error("face detection failed", zap.Int("input_index", idx), zap.String("input_url", inputContent), zap.Error(detectErr))
			return fmt.Errorf("face detection failed (input %d): %w", idx, detectErr)
		}
		zlog.LogWithContext(ctx).Info("face detection completed for input", zap.Int("input_index", idx), zap.String("input_url", inputContent), zap.Int("detected_faces", faceCount))
		totalFaceCount += faceCount
	}
	if totalFaceCount != int(workflowInfo.FaceCount) {
		zlog.LogWithContext(ctx).Warn("face count mismatch", zap.Int("total_detected_faces", totalFaceCount), zap.Int32("required_faces", workflowInfo.FaceCount))
		return fmt.Errorf("workflow requires %d face(s), but %d face(s) were detected in total", workflowInfo.FaceCount, totalFaceCount)
	}
	zlog.LogWithContext(ctx).Info("face count requirement check passed", zap.Int32("required_faces", workflowInfo.FaceCount), zap.Int("total_detected_faces", totalFaceCount))
	return nil
}

func (s *PictureTaskService) persistTaskDetailsWithStatus(
	ctx context.Context,
	tx *gorm.DB,
	taskID string, // 使用预生成的 taskID
	projectID string,
	userID string,
	workflowID string,
	userPictureInfoJSONString string,
	paramJSONString string,
	submitContextJSONString string,
	workflowKindTheme string,
	workflowKindType string,
	apiconfig model.WorkflowApiConfig,
	creditPoints int,
	creditDeductionInfo string,
	workflowProvider string,
	initialStatus pb.WorkflowTaskStatus,
	comment string,
) (*model.PictureTask, error) {
	canRetry := initialStatus != pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_CHAIN

	// 从 context 提取模型直连信息
	taskMode := int8(model.TaskModeWorkflow)
	modelName := ""
	userPrompt := ""
	negativePrompt := ""

	if isModelDirect, ok := ctx.Value("is_model_direct_mode").(bool); ok && isModelDirect {
		taskMode = int8(model.TaskModeModelDirect)
		if mn, ok := ctx.Value("model_name").(string); ok {
			modelName = mn
		}
		if up, ok := ctx.Value("user_prompt").(string); ok {
			userPrompt = up
		}
		if np, ok := ctx.Value("negative_prompt").(string); ok {
			negativePrompt = np
		}
	}

	task := &model.PictureTask{
		ProjectID:           projectID,
		TaskID:              taskID, // 使用传入的 taskID
		UserID:              userID,
		WorkflowID:          workflowID,
		Status:              int32(initialStatus),
		UserPictureInfoJson: userPictureInfoJSONString,
		ParamJSON:           paramJSONString,
		SubmitContextJson:   submitContextJSONString,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		CreditPoints:        creditPoints,
		ApiConfig:           apiconfig,
		Executer:            workflowProvider,
		Theme:               workflowKindTheme,
		WorkflowType:        workflowKindType,
		CreditDeductionInfo: creditDeductionInfo,
		CanRetry:            canRetry,
		Comment:             comment,
		// 模型直连模式字段
		TaskMode:       taskMode,
		ModelName:      modelName,
		UserPrompt:     userPrompt,
		NegativePrompt: negativePrompt,
	}

	return task, s.taskDao.CreateTask(ctx, tx, task)
}

func (s *PictureTaskService) persistTaskDetails(
	ctx context.Context,
	tx *gorm.DB,
	taskID string, // 添加 taskID 参数
	projectID string,
	userID string,
	workflowID string,
	userPictureInfoJSONString string,
	paramJSONString string,
	submitContextJSONString string,
	workflowKindTheme string,
	workflowKindType string,
	apiconfig model.WorkflowApiConfig,
	creditPoints int,
	creditDeductionInfo string,
	workflowProvider string,
	comment string,
) (*model.PictureTask, error) {
	return s.persistTaskDetailsWithStatus(
		ctx, tx, taskID, projectID, userID, workflowID, userPictureInfoJSONString,
		paramJSONString, submitContextJSONString, workflowKindTheme, workflowKindType, apiconfig,
		creditPoints, creditDeductionInfo, workflowProvider,
		pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING,
		comment,
	)
}

// checkActiveTasks 检查用户任务配额（两维度：队列长度 + 并发执行数）
// 1. 检查队列长度（pending + processing）是否超限 - 使用Redis原子计数器
// 2. 检查并发执行数（仅processing）是否超限 - 仅提示，不阻止提交
// 注意：
// - 队列满时，拒绝提交（ERR_TASK_QUEUE_LIMIT_EXCEEDED）
// - 并发满但队列未满时，允许提交到PENDING状态（任务会被调度器处理）
// reserveQueueSlot 通过Redis预留队列槽位，异常时记录并允许后续DB校验兜底
func (s *PictureTaskService) reserveQueueSlot(ctx context.Context, userID, taskID string, limit int) error {
	if err := s.taskQuotaChecker.CheckAndReserveQueueSlot(ctx, userID, taskID, limit); err != nil {
		if err == constants.ERR_TASK_QUEUE_LIMIT_EXCEEDED {
			return err
		}
		zlog.LogWithContext(ctx).Warn("failed to reserve queue slot via Redis, fallback to DB", zap.Error(err))
	}
	return nil
}

// verifyQueueSizeWithDB 使用DB进行二次验证，发现超限或查询失败时回滚Redis预留
func (s *PictureTaskService) verifyQueueSizeWithDB(ctx context.Context, tx *gorm.DB, userID, taskID string, limit int) error {
	activeTasks, err := s.taskDao.CountActiveTasksByUser(ctx, tx, userID)
	if err != nil {
		_ = s.taskQuotaChecker.ReleaseQueueSlot(ctx, userID, taskID)
		return errors.Wrap(err, "failed to count active tasks")
	}
	if activeTasks >= int64(limit) {
		_ = s.taskQuotaChecker.ReleaseQueueSlot(ctx, userID, taskID)
		zlog.LogWithContext(ctx).Warn("queue limit exceeded (DB check)", zap.Int64("current_active", activeTasks), zap.Int("max_queue_size", limit))
		return constants.ERR_TASK_QUEUE_LIMIT_EXCEEDED
	}
	return nil
}

// logConcurrentState 记录并发占用情况，仅提示不阻断提交流程
func (s *PictureTaskService) logConcurrentState(ctx context.Context, tx *gorm.DB, userID string, limit int) {
	processingTasks, err := s.taskDao.CountProcessingTasksByUser(ctx, tx, userID)
	if err != nil {
		// 并发检查失败不影响提交
		zlog.LogWithContext(ctx).Warn("failed to count processing tasks", zap.Error(err))
		return
	}
	if processingTasks >= int64(limit) {
		zlog.LogWithContext(ctx).Info("concurrent limit reached but queue not full, task will be queued",
			zap.Int64("current_processing", processingTasks),
			zap.Int("max_concurrent", limit))
	}
}

// checkActiveTasks 保持对外签名与行为不变，内部拆分职责更清晰
func (s *PictureTaskService) checkActiveTasks(ctx context.Context, tx *gorm.DB, userID string, taskID string, isMember bool) error {
	log := zlog.LogWithContext(ctx).With(
		zap.String("user_id", userID),
		zap.String("task_id", taskID),
		zap.Bool("is_member", isMember),
	)

	// 加载配置
	config := s.taskConfigManager.LoadConfig(ctx, isMember)
	log.Debug("loaded task limits config",
		zap.Int("max_queue_size", config.MaxQueueSize),
		zap.Int("max_concurrent", config.MaxConcurrent))

	// 预留队列槽位
	if err := s.reserveQueueSlot(ctx, userID, taskID, config.MaxQueueSize); err != nil {
		return err
	}

	// DB 二次验证（失败或超限时回滚）
	if err := s.verifyQueueSizeWithDB(ctx, tx, userID, taskID, config.MaxQueueSize); err != nil {
		return err
	}

	// 并发提示（不阻断）
	s.logConcurrentState(ctx, tx, userID, config.MaxConcurrent)

	return nil
}

func (s *PictureTaskService) SubmitTaskWithTx(ctx context.Context, tx *gorm.DB, req *pb.SubmitPictureForgeTaskRequest, taskID string, creditDeductionInfo string, isMember bool, creditPoints int, submitContextJSON string) (*pb.SubmitPictureForgeTaskResponse, error) {
	userID, err := s.validateSubmitTaskRequest(req)
	if err != nil {
		return nil, err
	}
	projectID := common.CtxGetStrValue(ctx, constants.CtxProjectID)
	workflowTheme := req.GetWorkflowTheme()
	if err := s.checkActiveTasks(ctx, tx, userID, taskID, isMember); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = s.taskQuotaChecker.ReleaseQueueSlot(ctx, userID, taskID)
		}
	}()
	enableOptimization := false // 明确初始化默认值
	if enableOptVal := ctx.Value(common.EnableOptimizedParamsKey); enableOptVal != nil {
		if enableOptBool, ok := enableOptVal.(bool); ok {
			enableOptimization = enableOptBool
		}
	}

	// 检查是否为模型直连模式
	var taskParams map[string]string
	var userPictureInfoJSON string
	var workflowInfo *model.Workflow
	isModelDirectMode := false
	if modelDirectVal := ctx.Value("is_model_direct_mode"); modelDirectVal != nil {
		if isModelDirect, ok := modelDirectVal.(bool); ok && isModelDirect {
			isModelDirectMode = true
		}
	}

	if isModelDirectMode {
		// ===== 模型直连模式 =====
		// 从context获取虚拟workflow（由API层构建）
		if virtualWorkflowVal := ctx.Value("virtual_workflow_info"); virtualWorkflowVal != nil {
			if vw, ok := virtualWorkflowVal.(*model.Workflow); ok {
				workflowInfo = vw
			} else {
				zlog.LogWithContext(ctx).Error("failed to get virtual workflow from context")
				return nil, errors.New("failed to get virtual workflow from context")
			}
		} else {
			zlog.LogWithContext(ctx).Error("virtual workflow not found in context")
			return nil, errors.New("virtual workflow not found in context")
		}

		// 使用模型直连模式的参数构建方法
		taskParams, userPictureInfoJSON, err = s.PrepareTaskInputsFromModelDirect(ctx, tx, req, workflowInfo)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to prepare task inputs from model direct mode", zap.Error(err))
			return nil, err
		}

		zlog.LogWithContext(ctx).Info("using model direct mode for task submission",
			zap.String("model_name", workflowInfo.ApiConfig.ModelName),
			zap.String("virtual_workflow_id", workflowInfo.WorkflowID))

	} else {
		// ===== Workflow模式（原有逻辑） =====
		taskParams, userPictureInfoJSON, workflowInfo, err = s.PrepareTaskInputsAndWorkflow(ctx, tx, req.GetWorkflowInput(), req.GetWorkflowId(), req.GetDuration(), req.GetResolution())
		if err != nil {
			return nil, err
		}
	}

	var originalInputs, optimizedInputs []*pb.WorkflowInput

	// 以下逻辑仅在workflow模式下执行
	if !isModelDirectMode {
		originalInputs = make([]*pb.WorkflowInput, len(req.GetWorkflowInput()))
		for i, input := range req.GetWorkflowInput() {
			originalInputs[i] = &pb.WorkflowInput{
				InputName:    input.GetInputName(),
				InputType:    input.GetInputType(),
				InputContent: input.GetInputContent(),
			}
		}
	}

	apiIden := workflowInfo.ApiConfig.ApiIden

	// Check for workflow replacement for GOOGLENICE users (仅workflow模式)
	originalWorkflowID := req.GetWorkflowId()
	comment := ""
	if !isModelDirectMode && s.workflowReplacementService != nil {
		shouldReplace, err := s.workflowReplacementService.ShouldReplaceWorkflow(
			ctx,
			userID,
			projectID,
			workflowInfo.Provider,
			apiIden,
			workflowInfo.ApiConfig.RequireAuditBypass, // 传入工作流是否需要避审标记
		)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to check workflow replacement",
				zap.Error(err),
				zap.String("user_id", userID),
				zap.String("provider", workflowInfo.Provider),
				zap.String("api_iden", apiIden),
				zap.Bool("require_audit_bypass", workflowInfo.ApiConfig.RequireAuditBypass))
		} else if shouldReplace {
			replacementWorkflowID, err := s.workflowReplacementService.GetReplacementWorkflowID(ctx, workflowInfo)
			if err != nil {
				zlog.LogWithContext(ctx).Error("failed to get replacement workflow ID",
					zap.Error(err),
					zap.String("api_iden", apiIden))
			} else {
				// Update workflow ID in request and task params
				req.WorkflowId = replacementWorkflowID
				taskParams["workflow_id"] = replacementWorkflowID
				resolution := req.GetResolution()
				if workflowInfo.Provider == constants.CloudComfyExecutorName {
					resolution = picture_generate.MapComfyResolutionToMinimax(resolution)
				}
				// Re-fetch workflow info with replacement workflow
				taskParams, userPictureInfoJSON, workflowInfo, err = s.PrepareTaskInputsAndWorkflow(ctx, tx, req.GetWorkflowInput(), replacementWorkflowID, req.GetDuration(), resolution)
				if err != nil {
					zlog.LogWithContext(ctx).Error("failed to prepare task with replacement workflow",
						zap.Error(err),
						zap.String("replacement_workflow_id", replacementWorkflowID))
					return nil, err
				}

				// Generate comment for audit trail
				comment = s.workflowReplacementService.GenerateComment(originalWorkflowID, replacementWorkflowID, apiIden)
				zlog.LogWithContext(ctx).Info("workflow replacement applied",
					zap.String("original_workflow_id", originalWorkflowID),
					zap.String("replacement_workflow_id", replacementWorkflowID),
					zap.String("user_id", userID),
					zap.String("api_iden", apiIden))
			}
		}
	}

	// Prompt optimization (仅workflow模式)
	if !isModelDirectMode && enableOptimization {
		var optimizeErr error
		var originalPrompt, optimizedPrompt string
		optimizedInputs, originalPrompt, optimizedPrompt, optimizeErr = s.optimizeWorkflowInputs(ctx, req.GetWorkflowInput(), apiIden)
		if optimizeErr != nil {
			zlog.LogWithContext(ctx).Warn("prompt optimization failed, using original inputs",
				zap.Error(optimizeErr),
				zap.String("workflow_id", req.GetWorkflowId()))
		} else {
			taskParams, userPictureInfoJSON, _, err = s.PrepareTaskInputsAndWorkflow(ctx, tx, optimizedInputs, req.GetWorkflowId(), req.GetDuration(), req.GetResolution())
			taskParams["original_prompt"] = originalPrompt
			taskParams["optimized_prompt"] = optimizedPrompt
			if err != nil {
				zlog.LogWithContext(ctx).Warn("failed to prepare task with optimized inputs, using original inputs",
					zap.Error(err),
					zap.String("workflow_id", req.GetWorkflowId()))
				taskParams, userPictureInfoJSON, _, err = s.PrepareTaskInputsAndWorkflow(ctx, tx, req.GetWorkflowInput(), req.GetWorkflowId(), req.GetDuration(), req.GetResolution())
				if err != nil {
					return nil, err
				}
			} else {
				req.WorkflowInput = optimizedInputs
				zlog.LogWithContext(ctx).Info("prompt optimization completed successfully",
					zap.String("workflow_id", req.GetWorkflowId()))
			}
		}
	}

	// 人脸验证（workflow模式使用workflow_input，模型直连模式跳过或使用user_images）
	if !isModelDirectMode {
		if err := s.performFaceValidation(ctx, workflowInfo, req.GetWorkflowInput()); err != nil {
			return nil, err
		}
	}

	// 处理 workflow kind（模型直连模式跳过）
	var workflowKind *model.WorkflowKind
	if !isModelDirectMode {
		workflowKind, err = s.processWorkflowKind(ctx, workflowInfo, taskParams)
		if err != nil {
			return nil, err
		}
		if workflowTheme == "" {
			workflowTheme = workflowKind.GetPrimaryTheme()
		} else if !slices.Contains(workflowKind.Theme, workflowTheme) {
			zlog.LogWithContext(ctx).Error("Invalid Workflow Theme", zap.String("workflowTheme", workflowTheme), zap.Any("workflowKindTheme", workflowKind.Theme), zap.String("workflowID", req.GetWorkflowId()))
			return nil, errors.New("Invalid Workflow Theme")
		}
	} else {
		// 模型直连模式：使用默认的 kind type
		taskParams["workflow_type"] = pb.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String()
		if workflowTheme == "" {
			workflowTheme = "general" // 默认主题
		}
	}
	if toolIDVal := ctx.Value(common.PictureToolIDKey); toolIDVal != nil {
		if toolID, ok := toolIDVal.(string); ok && toolID != "" {
			taskParams["tool_id"] = toolID
		}
	}

	paramJSON, err := json.Marshal(taskParams)
	if err != nil {
		return nil, errors.Wrap(err, "marshal task_params")
	}

	// 获取 kind type（模型直连模式使用默认值）
	kindType := pb.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String()
	if !isModelDirectMode && workflowKind != nil {
		kindType = workflowKind.KindType
	}

	// 确定要存储的 workflow_id
	// - 模型直连模式：使用虚拟 workflow 的 ID
	// - Workflow 模式：使用用户请求中的 workflow_id
	workflowIDToStore := req.GetWorkflowId()
	if isModelDirectMode {
		workflowIDToStore = workflowInfo.WorkflowID // 使用虚拟 workflow ID
	}

	// 使用预生成的 taskID 持久化，确保与Redis中预留的队列槽位记录一致
	// 使用 API 透传的提交上下文（若为空不阻断）
	task, err := s.persistTaskDetails(ctx, tx, taskID, projectID, userID, workflowIDToStore, userPictureInfoJSON, string(paramJSON), submitContextJSON, workflowTheme, kindType, workflowInfo.ApiConfig, creditPoints, creditDeductionInfo, workflowInfo.Provider, comment)
	if err != nil {
		return nil, errors.Wrap(err, "failed to persist task")
	}

	// 增加 workflow 引用计数（模型直连模式跳过）
	if !isModelDirectMode {
		if err := s.picForgeDao.IncrementWorkflowRefCount(ctx, tx, req.GetWorkflowId()); err != nil {
			zlog.LogWithContext(ctx).Error("failed to increment workflow ref count",
				zap.String("workflowID", req.GetWorkflowId()),
				zap.String("taskID", task.TaskID),
				zap.Error(err))
			return nil, errors.Wrap(err, "failed to increment workflow ref count")
		}
	}

	// 上报 TASK_SUBMIT 事件
	s.trackTaskSubmitEvent(ctx, task, workflowInfo, workflowKind, req.GetWorkflowInput(), isMember)

	zlog.LogWithContext(ctx).Info("Task submitted successfully", zap.String("taskID", task.TaskID))

	return &pb.SubmitPictureForgeTaskResponse{
		TaskId: task.TaskID,
	}, nil
}

func (s *PictureTaskService) CreateTaskRecordWithStatus(
	ctx context.Context,
	tx *gorm.DB,
	projectID string,
	userID string,
	workflowID string,
	userPictureInfoJSON string,
	paramJSON string,
	submitContextJSON string,
	workflowTheme string,
	workflowKindType string,
	apiConfig model.WorkflowApiConfig,
	creditPoints int,
	creditDeductionInfo string,
	workflowProvider string,
	initialStatus pb.WorkflowTaskStatus,
	comment string,
) (string, error) {
	// 生成 taskID 以确保与可能的配额记录保持一致
	genTaskID := GenerateTaskID()
	task, err := s.persistTaskDetailsWithStatus(
		ctx, tx, genTaskID, projectID, userID, workflowID,
		userPictureInfoJSON, paramJSON, submitContextJSON, workflowTheme, workflowKindType,
		apiConfig, creditPoints, creditDeductionInfo, workflowProvider,
		initialStatus,
		comment,
	)
	if err != nil {
		return "", err
	}
	return task.TaskID, nil
}

func (s *PictureTaskService) GetTaskResult(ctx context.Context, req *pb.GetPictureForgeTaskResultRequest) (*pb.GetPictureForgeTaskResultResponse, error) {
	task, err := s.taskDao.GetTask(ctx, req.GetTaskId())
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get task",
			zap.String("TaskID", req.GetTaskId()),
			zap.Error(err))
		return nil, errors.Wrap(err, "get task")
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING) {
		progress, err := s.rdb.Get(taskProgressKeyPrefix + task.TaskID).Float64()
		if err != nil && err != redis.Nil {
			zlog.LogWithContext(ctx).Error("failed to get progress",
				zap.String("TaskID", task.TaskID),
				zap.Error(err))
		}
		task.Progress = int32(progress)
	} else if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) {
		task.Progress = 100
	}

	pictureInfo, err := s.buildPictureForgeInfo(ctx, task)
	if err != nil {
		return nil, errors.Wrap(err, "build picture info")
	}

	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) && task.UnreadTaskResult {
		task.UnreadTaskResult = false
		if err := s.taskDao.UpdateTask(ctx, task); err != nil {
			zlog.LogWithContext(ctx).Error("failed to update task",
				zap.String("TaskID", task.TaskID),
				zap.Error(err))
		}
	}

	return &pb.GetPictureForgeTaskResultResponse{
		PictureForgeInfo: pictureInfo,
	}, nil
}

func (s *PictureTaskService) buildWorkflowInfo(ctx context.Context, workflowID string) (*pb.Workflow, error) {
	// 检查是否为模型直连模式的虚拟 workflow
	if strings.HasPrefix(workflowID, "model_direct_") {
		// 从虚拟 workflow_id 解析模型名称
		// 格式：model_direct_{model_name}_{random_id}
		parts := strings.Split(workflowID, "_")
		if len(parts) < 3 {
			zlog.LogWithContext(ctx).Error("invalid virtual workflow id format",
				zap.String("workflow_id", workflowID))
			return nil, errors.New("invalid virtual workflow id")
		}

		// 提取模型名称（可能包含多个下划线，如 flux-dev）
		modelName := strings.Join(parts[2:len(parts)-1], "_")

		// 从数据库获取模型信息
		projectID := common.GetProjectID(ctx)
		if projectID == "" || projectID == constants.ProjectIdVisualAI {
			projectID = constants.ProjectIdVisionAI
		}

		modelDao := dao.NewImageGenerationModelDao(db.GetDB())
		dbModel, err := modelDao.GetModelByName(ctx, modelName, projectID)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to get model for virtual workflow",
				zap.String("model_name", modelName),
				zap.String("workflow_id", workflowID),
				zap.Error(err))
			// 如果找不到模型，返回一个基本的 workflow 信息
			return &pb.Workflow{
				WorkflowId:  workflowID,
				Title:       modelName,
				Description: "Model Direct Mode",
				IsFree:      false,
			}, nil
		}

		// 构建虚拟 workflow 信息
		workflow := &pb.Workflow{
			WorkflowId:     workflowID,
			Title:          dbModel.DisplayName,
			Description:    dbModel.Description,
			Icon:           dbModel.Icon,
			IsFree:         dbModel.CreditPoints == 0,
			RecreatePrompt: "", // 模型直连模式暂不支持二创 prompt
		}

		return workflow, nil
	}

	// 普通 workflow，从数据库查询
	workflowInfo, err := s.picForgeDao.GetWorkflow(ctx, db.GetDB(), workflowID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get workflow", zap.Error(err))
		return nil, err
	}

	workflow := &pb.Workflow{
		WorkflowId:     workflowInfo.WorkflowID,
		Title:          workflowInfo.Title,
		Description:    workflowInfo.Description,
		KindId:         workflowInfo.KindID,
		Icon:           workflowInfo.Icon,
		FaceCount:      workflowInfo.FaceCount,
		IsFree:         workflowInfo.CreditPoints == 0,
		RecreatePrompt: workflowInfo.RecreatePrompt,
	}

	if workflowInfo.InputsJSON != "" {
		var inputs []*model.WorkflowInput
		if err := json.Unmarshal([]byte(workflowInfo.InputsJSON), &inputs); err != nil {
			zlog.LogWithContext(ctx).Error("failed to unmarshal inputs json", zap.Error(err))
			return workflow, nil
		}
		workflow.Inputs = make([]*pb.WorkflowInput, 0, len(inputs))
		for _, input := range inputs {
			workflow.Inputs = append(workflow.Inputs, &pb.WorkflowInput{
				InputName:    input.InputName,
				InputType:    pb.MessageType(input.InputType),
				InputContent: input.InputContent,
			})
		}
	}

	if workflowInfo.ExampleJson != "" {
		var exampleImage []*model.WorkflowExampleImage
		if err := json.Unmarshal([]byte(workflowInfo.ExampleJson), &exampleImage); err != nil {
			zlog.LogWithContext(ctx).Error("failed to unmarshal example json", zap.Error(err))
			return workflow, nil
		}
		workflow.ExampleImage = &pb.WorkflowExampleImage{
			OriContent:         exampleImage[0].OriContent,
			ResultContent:      exampleImage[0].ResultContent,
			AspectRatio:        exampleImage[0].AspectRatio,
			ResultThumbnailUrl: exampleImage[0].ResultThumbnailUrl,
		}
	}

	workflowKind, err := s.picForgeDao.GetWorkflowKind(ctx, workflowInfo.KindID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get workflow kind", zap.Error(err))
		return workflow, nil
	}
	workflow.KindType = pb.WorkflowKindType(pb.WorkflowKindType_value[workflowKind.KindType])
	workflow.Theme = pb.WorkflowTheme(pb.WorkflowTheme_value[workflowKind.GetPrimaryTheme()])

	return workflow, nil
}

func (s *PictureTaskService) newBasePictureForgeInfo(task *model.PictureTask) *pb.PictureForgeInfo {
	return &pb.PictureForgeInfo{
		UserId:              task.UserID,
		TaskId:              task.TaskID,
		TaskStatus:          pb.WorkflowTaskStatus(task.Status),
		TaskProgress:        task.Progress,
		PublishStatus:       getPublishStatus(task.IsPublished),
		ViewCount:           formatViewCount(task.FakeViewCount),
		AllowShowOriPicture: task.AllowShowOriPicture,
		IsFree:              task.CreditPoints == 0,
		CreditCost:          int32(task.CreditPoints),
	}
}

func (s *PictureTaskService) parseUserPictures(jsonStr string, taskID string) []*pb.PictureInfo {
	if jsonStr == "" {
		return nil
	}
	var ups []model.UserPictureInfo
	if err := json.Unmarshal([]byte(jsonStr), &ups); err != nil {
		zlog.LogWithContext(context.Background()).Error("failed to unmarshal user picture info",
			zap.String("TaskID", taskID),
			zap.Error(err))
		return nil
	}
	result := make([]*pb.PictureInfo, 0, len(ups))
	for _, u := range ups {
		thumb := strings.Replace(u.PhotoURL, ".jpg", "-low.webp", -1)
		result = append(result, &pb.PictureInfo{
			Url:          u.PhotoURL,
			AspectRatio:  float32(u.AspectRatio),
			ThumbnailUrl: thumb,
		})
	}
	return result
}

func (s *PictureTaskService) parseResultPictures(ctx context.Context, resultJSON string, taskID string, workflowType string) (taskRes, userShow *pb.PictureInfo, multiResults []*pb.PictureInfo) {
	if resultJSON == "" {
		return
	}
	var rd model.PictureTaskResult
	if err := json.Unmarshal([]byte(resultJSON), &rd); err != nil {
		zlog.LogWithContext(ctx).Error("failed to unmarshal result JSON",
			zap.String("TaskID", taskID),
			zap.Error(err))
		return
	}

	switch workflowType {
	case pb.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String():
		if rd.ResultURL != "" {
			low := strings.Replace(rd.ResultURL, ".jpg", "-low.webp", -1)
			taskRes = &pb.PictureInfo{
				Url:          rd.ResultURL,
				AspectRatio:  float32(rd.AspectRatio),
				ThumbnailUrl: low,
			}
		}
		// 解析多图结果
		if len(rd.ImageResults) > 0 {
			multiResults = make([]*pb.PictureInfo, 0, len(rd.ImageResults))
			for _, item := range rd.ImageResults {
				low := strings.Replace(item.ResultURL, ".jpg", "-low.webp", -1)
				if item.UserShowImageURL != "" {
					low = item.UserShowImageURL
				}
				multiResults = append(multiResults, &pb.PictureInfo{
					Url:          item.ResultURL,
					AspectRatio:  float32(item.AspectRatio),
					ThumbnailUrl: low,
				})
			}
		}
	case pb.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO.String():
		taskRes = &pb.PictureInfo{
			Url:          rd.ResultURL,
			AspectRatio:  float32(rd.VideoFrameAspectRatio),
			ThumbnailUrl: rd.ResultURL,
		}
		if rd.VideoFrameURL != "" {
			taskRes.ThumbnailUrl = rd.VideoFrameURL
		}
	}

	return
}

func (s *PictureTaskService) parseScores(jsonStr string, taskID string) []*pb.PicScoreInfo {
	if jsonStr == "" {
		return nil
	}
	var ss []model.ScoreData
	if err := json.Unmarshal([]byte(jsonStr), &ss); err != nil {
		zlog.LogWithContext(context.Background()).Error("failed to unmarshal score json",
			zap.String("TaskID", taskID),
			zap.Error(err))
		return nil
	}
	out := make([]*pb.PicScoreInfo, 0, len(ss))
	for _, sc := range ss {
		if sc.Name == "ethical_review" {
			continue
		}
		out = append(out, &pb.PicScoreInfo{
			Name:  sc.Name,
			Value: sc.Value,
		})
	}
	return out
}

func (s *PictureTaskService) buildPictureForgeInfo(ctx context.Context, task *model.PictureTask) (*pb.PictureForgeInfo, error) {
	pic := s.newBasePictureForgeInfo(task)

	workflow, _ := s.buildWorkflowInfo(ctx, task.WorkflowID)
	pic.Workflow = workflow

	var taskParams map[string]string
	if task.ParamJSON != "" {
		if err := json.Unmarshal([]byte(task.ParamJSON), &taskParams); err != nil {
			zlog.LogWithContext(ctx).Warn("failed to unmarshal task params when building picture info",
				zap.String("task_id", task.TaskID),
				zap.Error(err))
		}
	}
	if taskParams != nil {
		if toolID, ok := taskParams["tool_id"]; ok && toolID != "" {
			pic.ToolId = toolID
			pic.Prompt = s.extractFinalPrompt(ctx, taskParams)
		}
	}

	pic.UserPictureInfo = s.parseUserPictures(task.UserPictureInfoJson, task.TaskID)
	pic.TaskResultPictureInfo, pic.UserShowPictureInfo, pic.TaskResultPictures = s.parseResultPictures(ctx, task.ResultJSON, task.TaskID, task.WorkflowType)
	pic.Score = s.parseScores(task.ScoreJSON, task.TaskID)
	pic.UnreadTaskResult = task.UnreadTaskResult
	pic.KindType = pb.WorkflowKindType(pb.WorkflowKindType_value[task.WorkflowType])
	if task.CompletedAt.Valid {
		pic.TaskCompleteTime = task.CompletedAt.Time.Unix()
	}

	if task.Theme != "" {
		if themeValue, exists := pb.WorkflowTheme_value[task.Theme]; exists {
			pic.Theme = pb.WorkflowTheme(themeValue)
			pic.WorkflowTheme = task.Theme
		} else {
			pic.Theme = pb.WorkflowTheme_WORKFLOW_THEME_UNKNOWN
		}
	} else {
		pic.Theme = pb.WorkflowTheme_WORKFLOW_THEME_UNKNOWN
	}

	if pic.GetTaskStatus() == pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_SUBMISSION_RETRY ||
		pic.GetTaskStatus() == pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION {
		pic.TaskStatus = pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING
	}
	pic.CanRetry = task.CanRetry
	tags, err := s.buildPictureForgeResultTags(ctx, task)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to build picture forge result tags", zap.Error(err))
	}
	pic.Tags = tags
	pic.TaskFailReason = task.FailedReason
	pic.TaskFailReasonFull = task.FailedReasonFull

	// 从配置表读取默认图片转视频工具ID
	if s.configService != nil {
		if defaultI2VToolID, err := s.configService.GetStringConfig(constants.ConfigKeyDefaultI2VToolID); err == nil && defaultI2VToolID != "" {
			pic.DefaultPictureToVideoToolId = defaultI2VToolID
		}
	}

	// 从 workflow 中获取 recreate_prompt（用于图生视频的二创prompt）
	if workflow != nil {
		pic.RecreatePrompt = workflow.GetRecreatePrompt()
	}

	// 填充模型直连模式信息
	pic.IsModelDirect = task.TaskMode == model.TaskModeModelDirect
	if pic.IsModelDirect {
		pic.ModelName = task.ModelName
		pic.Prompt = task.UserPrompt
		pic.NegativePrompt = task.NegativePrompt
	}

	// 判断并设置生成类型（文生图/图生图）
	pic.GenerationType = s.determineGenerationType(task)

	return pic, nil
}

// determineGenerationType 判断任务是文生图还是图生图
func (s *PictureTaskService) determineGenerationType(task *model.PictureTask) pb.ImageGenerationType {
	// 方式1: 通过 user_picture_info_json 判断（主要方式）
	if task.UserPictureInfoJson != "" && task.UserPictureInfoJson != "[]" {
		var userPics []model.UserPictureInfo
		if err := json.Unmarshal([]byte(task.UserPictureInfoJson), &userPics); err == nil {
			if len(userPics) > 0 {
				return pb.ImageGenerationType_IMAGE_GENERATION_TYPE_IMAGE_TO_IMAGE
			}
		}
	}

	// 方式2: 通过 param_json 中的 task_type 判断（作为备用）
	if task.ParamJSON != "" {
		var taskParams map[string]string
		if err := json.Unmarshal([]byte(task.ParamJSON), &taskParams); err == nil {
			if taskType, ok := taskParams["task_type"]; ok {
				if taskType == "image2image" {
					return pb.ImageGenerationType_IMAGE_GENERATION_TYPE_IMAGE_TO_IMAGE
				}
			}
		}
	}

	// 默认为文生图
	return pb.ImageGenerationType_IMAGE_GENERATION_TYPE_TEXT_TO_IMAGE
}

func calculateDurationTag(duration int64) string {
	rounded := (duration / 5) * 5
	if rounded < 5 {
		rounded = 5
	}
	return fmt.Sprintf("%ds", rounded)
}

func (s *PictureTaskService) buildPictureForgeResultTags(ctx context.Context, task *model.PictureTask) ([]string, error) {
	var tags []string
	var taskParams map[string]string
	if err := json.Unmarshal([]byte(task.ParamJSON), &taskParams); err != nil {
		zlog.LogWithContext(ctx).Error("failed to unmarshal task params", zap.Error(err))
		return nil, err
	}

	if durationStr, ok := taskParams["duration"]; ok {
		duration, err := strconv.ParseInt(durationStr, 10, 64)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to parse duration", zap.Error(err))
			return nil, err
		}
		if duration >= 10 {
			durationTag := calculateDurationTag(duration)
			tags = append(tags, durationTag)
		}
	}

	return tags, nil
}

func (s *PictureTaskService) GetRecentTasks(ctx context.Context, userID string, theme string, kindType string, taskMode *int8, page, pageSize int32) ([]*pb.PictureForgeInfo, int64, error) {
	tasks, total, err := s.taskDao.GetRecentTasks(ctx, userID, theme, kindType, taskMode, page, pageSize)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get recent tasks from dao", zap.Error(err))
		return nil, 0, err
	}

	infos := make([]*pb.PictureForgeInfo, 0, len(tasks))
	for _, task := range tasks {
		picInfo, err := s.buildPictureForgeInfo(ctx, task)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to build picture forge info",
				zap.String("TaskID", task.TaskID),
				zap.Error(err))
			continue
		}
		infos = append(infos, picInfo)
	}

	return infos, total, nil
}

func (s *PictureTaskService) PublishPicture(ctx context.Context, req *pb.PublishPictureRequest) (*pb.PublishPictureResponse, error) {
	userShowPictureUrl := req.GetUserShowPictureUrl()
	task, err := s.taskDao.GetTask(ctx, req.GetPictureForgeTaskId())
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to get task",
			zap.String("TaskID", req.GetPictureForgeTaskId()),
			zap.Error(err))
		return nil, errors.Wrap(err, "get task")
	}

	if task.UserID != req.GetRequestHeader().GetUserId() {
		return nil, errors.New("permission denied")
	}

	task.IsPublished = true
	task.AllowShowOriPicture = req.GetAllowShowOriPicture()

	if userShowPictureUrl != "" {
		var resultData model.PictureTaskResult
		if task.ResultJSON != "" {
			if err := json.Unmarshal([]byte(task.ResultJSON), &resultData); err != nil {
				zlog.LogWithContext(ctx).Error("failed to unmarshal result json", zap.Error(err))
				return nil, errors.Wrap(err, "unmarshal result json")
			}
		}
		resultData.UserShowImageURL = userShowPictureUrl
		resultJSON, _ := json.Marshal(resultData)
		task.ResultJSON = string(resultJSON)
	}

	if err := s.taskDao.UpdateTask(ctx, task); err != nil {
		zlog.LogWithContext(ctx).Error("failed to update task",
			zap.String("TaskID", req.GetPictureForgeTaskId()),
			zap.Error(err))
		return nil, errors.Wrap(err, "update task")
	}

	return &pb.PublishPictureResponse{}, nil
}

func (s *PictureTaskService) DeletePicture(ctx context.Context, req *pb.DeletePictureRequest) (*pb.DeletePictureResponse, error) {
	userID := req.GetRequestHeader().GetUserId()

	// 收集要删除的任务ID列表
	var taskIDs []string
	if len(req.GetPictureForgeTaskIds()) > 0 {
		// 优先使用批量删除字段
		taskIDs = req.GetPictureForgeTaskIds()
	} else if req.GetPictureForgeTaskId() != "" {
		// 向后兼容单个任务ID
		taskIDs = []string{req.GetPictureForgeTaskId()}
	}

	if len(taskIDs) == 0 {
		zlog.LogWithContext(ctx).Error("no task IDs provided for deletion")
		return nil, errors.New("no task IDs provided")
	}

	// 调用 DAO 层批量删除
	if err := s.taskDao.DeleteTasks(ctx, taskIDs, userID); err != nil {
		zlog.LogWithContext(ctx).Error("failed to delete pictures",
			zap.Strings("TaskIDs", taskIDs),
			zap.String("UserID", userID),
			zap.Error(err))
		return nil, errors.Wrap(err, "delete tasks")
	}

	zlog.LogWithContext(ctx).Info("successfully deleted pictures",
		zap.Int("count", len(taskIDs)),
		zap.Strings("TaskIDs", taskIDs),
		zap.String("UserID", userID))

	return &pb.DeletePictureResponse{}, nil
}

func getPublishStatus(isPublished bool) pb.PublishStatus {
	if isPublished {
		return pb.PublishStatus_PUBLISH_STATUS_PUBLISHED
	}
	return pb.PublishStatus_PUBLISH_STATUS_UNPUBLISHED
}

func formatViewCount(count int32) string {
	if count < 1000 {
		return strconv.Itoa(int(count))
	}

	k := float64(count) / 1000.0
	return fmt.Sprintf("%.1fK", k)
}

func (s *PictureTaskService) RetryFailedTask(ctx context.Context, taskID string) (*pb.RetryFailedTaskResponse, error) {
	zlog.LogWithContext(ctx).Info("start to retry failed task", zap.String("taskID", taskID))

	tx := db.GetDB().Begin()
	if tx.Error != nil {
		return nil, errors.Wrap(tx.Error, "begin transaction for retry")
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	task, err := s.taskDao.GetTaskForUpdate(ctx, tx, taskID)
	if err != nil {
		tx.Rollback()
		return nil, errors.Wrap(err, "get task for update")
	}

	if !task.CanRetry {
		tx.Rollback()
		return nil, constants.ERR_TASK_CANNOT_BE_REtried
	}

	if task.Status != int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		tx.Rollback()
		return nil, constants.ERR_TASK_CANNOT_BE_REtried
	}

	creditCost := credit.ImageGenerationCost(task.CreditPoints,
		task.WorkflowType == pb.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String(), false)
	zlog.LogWithContext(ctx).Info("retry task credit cost", zap.String("taskID", taskID), zap.Int("creditCost", creditCost))

	var deductionInfoJSON string
	isMember := false
	if s.subscribeService != nil {
		if info, _ := s.subscribeService.GetSubscribeInfo(ctx, task.UserID, "", "", ""); info != nil && info.GetSubscribeLevel() > 0 {
			isMember = true
		}
	}
	// 当日免费积分（已禁用）
	dailyGrantFailed := false
	// if s.dailyFreeCreditsService != nil {
	// 	if _, _, gerr := s.dailyFreeCreditsService.CheckAndGrantDailyCreditsWithTx(ctx, tx, task.ProjectID, task.UserID, isMember); gerr != nil {
	// 		zlog.LogWithContext(ctx).Warn("grant daily credits (retry, tx) failed", zap.Error(gerr))
	// 		dailyGrantFailed = true
	// 	}
	// }
	if creditCost > 0 {
		deductionInfo, err := s.creditService.DeductCreditsWithTx(ctx, tx, task.ProjectID, task.UserID, task.TaskID, constants.TransactionTypeTaskRetryDeduction, int64(creditCost), "任务重试扣款", isMember)
		if err != nil {
			tx.Rollback()
			if errors.Is(err, credit.ErrInsufficientCredits) {
				if !isMember && dailyGrantFailed {
					return nil, constants.ERR_USER_AMOUNT_NOT_ENOUGH
				}
				return nil, constants.ERR_USER_AMOUNT_NOT_ENOUGH
			}
			return nil, constants.ERR_USER_AMOUNT_NOT_ENOUGH
		}
		deductionBytes, jsonErr := json.Marshal(deductionInfo)
		if jsonErr != nil {
			tx.Rollback()
			return nil, errors.Wrap(jsonErr, "failed to marshal deduction info")
		}
		deductionInfoJSON = string(deductionBytes)
	}

	task.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING)
	task.Error = ""
	task.ResultJSON = ""
	task.Progress = 0
	task.RetryCount++
	task.ProviderSyncErrorCount = 0
	task.UpdatedAt = time.Now()
	task.CreditDeductionInfo = deductionInfoJSON
	// 保存本次实际费用，防止免费重试后仍保留历史扣费金额。
	task.CreditPoints = creditCost
	task.IsRefunded = false
	if err := tx.WithContext(ctx).Save(task).Error; err != nil {
		tx.Rollback()
		return nil, errors.Wrap(err, "update task status for retry")
	}

	if err := tx.Commit().Error; err != nil {
		return nil, errors.Wrap(err, "commit transaction for retry")
	}
	s.sendProgressEvent(task)

	zlog.LogWithContext(ctx).Info("retry task submitted to queue successfully", zap.String("taskID", taskID))
	return &pb.RetryFailedTaskResponse{}, nil
}

type ImageScoreResult struct {
	Composition float64 `json:"composition"`
	Color       float64 `json:"color"`
	Creativity  float64 `json:"creativity"`
}

// SendTaskCompletionPush 发送任务完成推送通知
func (s *PictureTaskService) SendTaskCompletionPush(ctx context.Context, task *model.PictureTask) {
	if s.pushGatewayClient == nil || !s.pushGatewayClient.IsEnabled() {
		return
	}

	// 检查推送开关
	enabled, _ := s.configService.GetBoolConfig(constants.ConfigKeyTaskPushEnabled)
	if !enabled {
		return
	}

	title, _ := s.configService.GetStringConfig(constants.ConfigKeyTaskPushTitle)
	body, _ := s.configService.GetStringConfig(constants.ConfigKeyTaskPushBody)

	if title == "" && body == "" {
		return // 未配置则不发送
	}

	_, err := s.pushGatewayClient.SendPush(ctx, &push_gateway.SendPushRequest{
		UserIDs: []string{task.UserID},
		Title:   title,
		Body:    body,
		BizID:   "task_completed_" + task.TaskID,
		Data: map[string]interface{}{
			"target": "album_page",
		},
	})
	if err != nil {
		zlog.LogWithContext(ctx).Error("发送任务完成推送失败",
			zap.String("task_id", task.TaskID),
			zap.String("user_id", task.UserID),
			zap.Error(err))
	}
}

func (s *PictureTaskService) FailTask(ctx context.Context, task *model.PictureTask, errMsg string) (err error) {
	log := zlog.LogWithContext(ctx).With(zap.String("taskID", task.TaskID), zap.String("userID", task.UserID))
	// 仅释放旧处理锁，避免在事务未提交或早退时遗留锁
	defer func() {
		s.releaseUserProcessingLock(ctx, task.UserID, task.TaskID)
	}()

	tx := s.db.Begin()
	if tx.Error != nil {
		log.Error("FailTask: failed to begin transaction", zap.Error(tx.Error))
		task.Error = tx.Error.Error()
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Error("FailTask: recovered from panic", zap.Any("panic", r), zap.Stack("stack"))
			task.Error = fmt.Sprintf("panic: %v", r)
		}
	}()

	latestTask, err := s.taskDao.GetTaskForUpdate(context.Background(), tx, task.TaskID)
	if err != nil {
		tx.Rollback()
		log.Error("FailTask: failed to get and lock task", zap.Error(err))
		task.Error = err.Error()
		return err
	}
	latestTask.FailedReason = task.FailedReason
	latestTask.FailedReasonFull = task.FailedReasonFull
	*task = *latestTask

	if latestTask.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) ||
		latestTask.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED) {
		tx.Rollback()
		log.Warn("FailTask: Task is already in a terminal state, skipping.", zap.Int32("status", latestTask.Status))
		return nil
	}

	if latestTask.CreditPoints > 0 && !latestTask.IsRefunded {
		// 为退款操作创建独立的 context,避免受父 context 取消影响,但设置 30 秒超时防止无限挂起
		refundCtx, refundCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer refundCancel()
		if refundErr := s.creditService.RefundCredits(refundCtx, latestTask, "任务执行失败退款"); refundErr != nil {
			log.Error("FailTask: failed to refund credits, will not block task failure", zap.Error(refundErr))
		} else {
			log.Info("FailTask: successfully refunded credits", zap.Int("amount", latestTask.CreditPoints))
			latestTask.IsRefunded = true
		}
	} else if latestTask.IsRefunded {
		log.Info("FailTask: task already refunded, skipping refund process", zap.Int("amount", latestTask.CreditPoints))
	}

	latestTask.Status = int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED)
	latestTask.Error = errMsg
	latestTask.Progress = -1
	latestTask.UpdatedAt = time.Now()
	if updateErr := tx.Save(latestTask).Error; updateErr != nil {
		tx.Rollback()
		log.Error("FailTask: failed to update task status to FAILED", zap.Error(updateErr))
		task.Error = updateErr.Error()
		return updateErr
	}

	if commitErr := tx.Commit().Error; commitErr != nil {
		log.Error("FailTask: failed to commit transaction", zap.Error(commitErr))
		task.Error = commitErr.Error()
		return commitErr
	}

	// 已持久化为失败状态后，发送事件并触发终态Hook以统一释放资源
	s.sendProgressEvent(latestTask)
	picture_generate.TriggerOnTaskTerminated(ctx, latestTask)

	// 发送任务完成推送通知（异步，不阻塞主流程）
	go s.SendTaskCompletionPush(context.Background(), latestTask)

	if err := s.handleChainTaskFailure(ctx, latestTask, errMsg); err != nil {
		log.Error("FailTask: failed to handle chain task failure", zap.Error(err))
	}

	*task = *latestTask
	return nil
}

func (s *PictureTaskService) FailTaskWithReason(ctx context.Context, task *model.PictureTask, errMsg string, reason string) error {
	task.FailedReason = reason
	if task.FailedReasonFull == "" {
		task.FailedReasonFull = reason
	}
	return s.FailTask(ctx, task, errMsg)
}

func (s *PictureTaskService) releaseUserProcessingLock(ctx context.Context, userID string, taskID string) {
	lockKey := fmt.Sprintf("%s%s", userProcessingLockKeyPrefix, userID)
	log := zlog.LogWithContext(ctx).With(
		zap.String("userID", userID),
		zap.String("taskID", taskID),
		zap.String("lockKey", lockKey),
	)

	currentLockVal, err := s.rdb.Get(lockKey).Result()
	if err != nil {
		if err != redis.Nil {
			log.Error("Failed to get user processing lock before releasing", zap.Error(err))
		}
		return
	}

	if currentLockVal == taskID {
		if err := s.rdb.Del(lockKey).Err(); err != nil {
			log.Error("Failed to release user processing lock", zap.Error(err))
		} else {
			log.Info("Successfully released user processing lock")
		}
	} else {
		log.Warn("Did not release user processing lock: lock is held by another task", zap.String("lock_holder_task_id", currentLockVal))
	}
}

func (s *PictureTaskService) sendProgressEvent(task *model.PictureTask) {
	eventData := &pb.TaskProgressEventData{
		PictureTaskId:      task.TaskID,
		Progress:           task.Progress,
		CanRetry:           task.CanRetry,
		TaskFailReason:     task.FailedReason,
		TaskFailReasonFull: task.FailedReasonFull,
	}
	if task.Status == int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED) && task.ResultJSON != "" {
		var result model.PictureTaskResult
		if err := json.Unmarshal([]byte(task.ResultJSON), &result); err == nil {
			switch task.WorkflowType {
			case pb.WorkflowKindType_WORKFLOW_KIND_TYPE_VIDEO.String():
				eventData.PictureInfo = &pb.PictureInfo{
					Url:          result.ResultURL,
					AspectRatio:  float32(result.VideoFrameAspectRatio),
					ThumbnailUrl: result.VideoFrameURL,
				}
			default:
				eventData.PictureInfo = &pb.PictureInfo{
					Url:          result.ResultURL,
					AspectRatio:  float32(result.AspectRatio),
					ThumbnailUrl: "",
				}
			}
		}
	}
	event.GetEventRegistry().DispatchToUser(
		pb.WatchEventType_WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS,
		eventData,
		task.ProjectID,
		task.UserID,
	)
}

func (s *PictureTaskService) Transaction(fc func(tx *gorm.DB) error) error {
	return s.db.Transaction(fc)
}

func (s *PictureTaskService) handleChainTaskFailure(ctx context.Context, task *model.PictureTask, reason string) error {
	if s.taskChainService != nil {
		s.taskChainService.OnTaskFailed(ctx, task)
		return nil
	}

	zlog.LogWithContext(ctx).Warn("task chain service not set, using fallback chain failure handling",
		zap.String("task_id", task.TaskID))

	chainDao := dao.NewTaskChainDao(s.db)

	chain, err := chainDao.GetByRootTask(ctx, task.TaskID)
	if err != nil {
		chain, err = chainDao.GetByNextTask(ctx, task.TaskID)
		if err != nil {
			return nil
		}
	}

	if chain == nil {
		return nil
	}

	failedStep := s.determineTaskStep(chain, task.TaskID)

	if err := s.refundFailedChainSteps(ctx, chain, failedStep, reason); err != nil {
		zlog.LogWithContext(ctx).Error("failed to refund chain credits",
			zap.String("chain_id", chain.ChainID),
			zap.String("task_id", task.TaskID),
			zap.Int("failed_step", failedStep),
			zap.Error(err))
		return err
	}

	chain.Status = 3
	if err := chainDao.Update(ctx, chain); err != nil {
		zlog.LogWithContext(ctx).Error("failed to update chain status",
			zap.String("chain_id", chain.ChainID),
			zap.Error(err))
		return err
	}

	zlog.LogWithContext(ctx).Info("chain task failure handled (fallback)",
		zap.String("chain_id", chain.ChainID),
		zap.String("failed_task_id", task.TaskID),
		zap.Int("failed_step", failedStep))

	return nil
}

func (s *PictureTaskService) determineTaskStep(chain *model.TaskChain, taskID string) int {
	var allTaskIDs []string
	if err := json.Unmarshal([]byte(chain.AllTaskIDs), &allTaskIDs); err != nil {
		if chain.RootTaskID == taskID {
			return 1
		}
		if chain.NextTaskID == taskID {
			return 2
		}
		return 1
	}

	for i, id := range allTaskIDs {
		if id == taskID {
			return i + 1
		}
	}
	return 1
}

func (s *PictureTaskService) refundFailedChainSteps(ctx context.Context, chain *model.TaskChain, failedStep int, reason string) error {
	switch failedStep {
	case 1:
		zlog.LogWithContext(ctx).Info("first step failed, credits already refunded by regular flow",
			zap.String("chain_id", chain.ChainID),
			zap.String("reason", reason))
		return nil

	case 2:
		return s.refundSecondStepCredits(ctx, chain, reason)

	default:
		zlog.LogWithContext(ctx).Warn("unknown failed step",
			zap.String("chain_id", chain.ChainID),
			zap.Int("failed_step", failedStep))
		return nil
	}
}

func (s *PictureTaskService) refundSecondStepCredits(ctx context.Context, chain *model.TaskChain, reason string) error {
	secondStepCredits, err := s.calculateSecondStepCredits(ctx, chain)
	if err != nil {
		zlog.LogWithContext(ctx).Error("failed to calculate second step credits",
			zap.String("chain_id", chain.ChainID),
			zap.Error(err))
		return err
	}

	if secondStepCredits <= 0 {
		return nil
	}

	refundRatio := float64(secondStepCredits) / float64(chain.TotalCreditCost)
	if refundRatio > 1.0 {
		refundRatio = 1.0
	}

	if chain.CreditDeductionInfo == "" {
		return nil
	}

	tempTask := &model.PictureTask{
		ProjectID:           chain.ProjectID,
		UserID:              chain.UserID,
		TaskID:              "chain_second_step_refund_" + chain.ChainID,
		CreditDeductionInfo: s.calculatePartialRefundInfo(chain.CreditDeductionInfo, refundRatio),
		CreditPoints:        secondStepCredits,
	}

	if err := s.creditService.RefundCredits(ctx, tempTask, "Second step failed: "+reason); err != nil {
		zlog.LogWithContext(ctx).Error("failed to refund second step credits",
			zap.String("chain_id", chain.ChainID),
			zap.Int("second_step_credits", secondStepCredits),
			zap.Float64("refund_ratio", refundRatio),
			zap.Error(err))
		return err
	}

	zlog.LogWithContext(ctx).Info("second step credits refunded accurately",
		zap.String("chain_id", chain.ChainID),
		zap.Int("refunded_credits", secondStepCredits),
		zap.Float64("refund_ratio", refundRatio),
		zap.String("reason", reason))

	return nil
}

func (s *PictureTaskService) calculateSecondStepCredits(ctx context.Context, chain *model.TaskChain) (int, error) {
	secondWorkflowID, err := s.configService.GetStringConfig(constants.ConfigKeyPictureToVideoGuideWorkflowID)
	if err != nil {
		return 0, fmt.Errorf("failed to get second workflow ID: %v", err)
	}

	secondWorkflow, err := s.picForgeDao.GetWorkflow(ctx, s.db, secondWorkflowID)
	if err != nil {
		return 0, fmt.Errorf("failed to get second workflow: %v", err)
	}

	if secondWorkflow.CreditPoints <= 0 {
		if secondWorkflow.ApiConfig.ApiIden != "" {
			configKey := "credit_cost_" + secondWorkflow.ApiConfig.ApiIden
			if defaultCost, err := s.configService.GetIntConfig(configKey); err == nil {
				return defaultCost, nil
			}
		}

		estimatedCredits := chain.TotalCreditCost / 2
		zlog.LogWithContext(ctx).Warn("using estimated second step credits",
			zap.String("chain_id", chain.ChainID),
			zap.String("workflow_id", secondWorkflowID),
			zap.Int("estimated_credits", estimatedCredits))
		return estimatedCredits, nil
	}

	return secondWorkflow.CreditPoints, nil
}

func (s *PictureTaskService) calculatePartialRefundInfo(originalDeductionInfo string, refundRatio float64) string {
	var deductionRecords map[uint]int64
	if err := json.Unmarshal([]byte(originalDeductionInfo), &deductionRecords); err != nil {
		return originalDeductionInfo
	}

	partialRecords := make(map[uint]int64)
	for amountID, amount := range deductionRecords {
		partialAmount := int64(float64(amount) * refundRatio)
		if partialAmount > 0 {
			partialRecords[amountID] = partialAmount
		}
	}

	if partialData, err := json.Marshal(partialRecords); err == nil {
		return string(partialData)
	}

	return originalDeductionInfo
}

func (s *PictureTaskService) optimizeWorkflowInputs(ctx context.Context, inputs []*pb.WorkflowInput, apiIden string) ([]*pb.WorkflowInput, string, string, error) {
	if s.aigcClient == nil {
		return nil, "", "", errors.New("aigc client not available")
	}

	optimizedInputs := make([]*pb.WorkflowInput, len(inputs))
	for i, input := range inputs {
		optimizedInputs[i] = &pb.WorkflowInput{
			InputName:    input.GetInputName(),
			InputType:    input.GetInputType(),
			InputContent: input.GetInputContent(),
		}
	}

	optimizedCount := 0
	originalPrompt := ""
	optimizedPrompt := ""
	var err error
	for i, input := range optimizedInputs {
		if s.shouldOptimizeInput(input) {
			originalPrompt = input.GetInputContent()
			optimizedPrompt, err = s.optimizePrompt(ctx, originalPrompt, apiIden)
			if err != nil {
				zlog.LogWithContext(ctx).Warn("failed to optimize single input, skipping",
					zap.String("input_name", input.GetInputName()),
					zap.String("original_content", originalPrompt),
					zap.Error(err))
				continue
			}

			optimizedInputs[i].InputContent = optimizedPrompt
			optimizedCount++

			zlog.LogWithContext(ctx).Debug("input optimized successfully",
				zap.String("input_name", input.GetInputName()),
				zap.String("original", originalPrompt),
				zap.String("optimized", optimizedPrompt))
		}
	}

	if optimizedCount == 0 {
		return nil, "", "", errors.New("no inputs were optimized")
	}

	zlog.LogWithContext(ctx).Info("workflow inputs optimization completed",
		zap.Int("total_inputs", len(inputs)),
		zap.Int("optimized_count", optimizedCount))

	return optimizedInputs, originalPrompt, optimizedPrompt, nil
}

func (s *PictureTaskService) shouldOptimizeInput(input *pb.WorkflowInput) bool {
	if input.GetInputType() != pb.MessageType_MT_TEXT {
		return false
	}

	content := strings.TrimSpace(input.GetInputContent())
	if len(content) == 0 {
		return false
	}

	inputName := strings.ToLower(input.GetInputName())
	optimizableFields := []string{"prompt", "custom_prompt"}

	for _, field := range optimizableFields {
		if strings.Contains(inputName, field) {
			return true
		}
	}

	return false
}

func (s *PictureTaskService) optimizePrompt(ctx context.Context, originalPrompt string, apiIden string) (string, error) {
	if s.aigcClient == nil {
		return "", errors.New("aigc client not available")
	}

	optimizationConfig, err := s.getPromptOptimizationConfig(apiIden)
	if err != nil {
		return "", errors.Wrap(err, "failed to get optimization config")
	}

	messages := []*v1.ChatMessage{
		{
			Role: v1.ChatRole_CHAT_ROLE_SYSTEM,
			Parts: []*v1.ContentPart{
				{
					Data: &v1.ContentPart_Text{
						Text: optimizationConfig.SystemPrompt,
					},
				},
			},
		},
		{
			Role: v1.ChatRole_CHAT_ROLE_USER,
			Parts: []*v1.ContentPart{
				{
					Data: &v1.ContentPart_Text{
						Text: originalPrompt,
					},
				},
			},
		},
	}

	req := &v1.ChatStreamRequest{
		ModelName: optimizationConfig.Model,
		Messages:  messages,
		Parameters: &v1.ChatParameters{
			Temperature: 0.7,
			MaxTokens:   4096,
		},
		Metadata: map[string]string{
			"task_type": "prompt_optimization",
			"user_id":   common.GetUserID(ctx),
		},
	}

	stream, err := s.aigcClient.ChatStream(ctx, req)
	if err != nil {
		return "", errors.Wrap(err, "failed to create chat stream")
	}

	var optimizedPrompt strings.Builder
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errors.Wrap(err, "failed to receive chat response")
		}

		optimizedPrompt.WriteString(resp.GetContentChunk())

		if resp.GetIsFinish() {
			break
		}
	}

	result := strings.TrimSpace(optimizedPrompt.String())
	if result == "" {
		return "", errors.New("received empty optimized prompt")
	}

	if len(result) > len(originalPrompt)*5 {
		zlog.LogWithContext(ctx).Warn("optimized prompt seems too long, may have quality issues",
			zap.String("original", originalPrompt),
			zap.String("optimized", result),
			zap.Int("original_length", len(originalPrompt)),
			zap.Int("optimized_length", len(result)))
	}

	return result, nil
}

func (s *PictureTaskService) getPromptOptimizationConfig(apiIden string) (*PromptOptimizationConfig, error) {
	defaultConfig := &PromptOptimizationConfig{}

	if s.configService != nil {
		var configFromDB PromptOptimizationConfig
		if err := s.configService.GetJSONConfig(constants.ConfigKeyPromptOptimizationInfo+"_"+apiIden, &configFromDB); err == nil {
			if configFromDB.Model != "" {
				defaultConfig.Model = configFromDB.Model
			}
			if configFromDB.SystemPrompt != "" {
				defaultConfig.SystemPrompt = configFromDB.SystemPrompt
			}
		}
	}

	return defaultConfig, nil
}

// WorkflowReplacementService 工作流替换服务
type WorkflowReplacementService struct {
	configService *ConfigService
	cacheService  *cache.CacheService
	couponDao     *dao.CouponDao
}

func NewWorkflowReplacementService(configService *ConfigService, cacheService *cache.CacheService, couponDao *dao.CouponDao) *WorkflowReplacementService {
	return &WorkflowReplacementService{
		configService: configService,
		cacheService:  cacheService,
		couponDao:     couponDao,
	}
}

// ShouldReplaceWorkflow 检查是否需要替换工作流
// 检查工作流是否需要避审替换，以及用户是否满足替换条件
func (s *WorkflowReplacementService) ShouldReplaceWorkflow(ctx context.Context, userID, projectID, provider, apiIden string, workflowRequireAuditBypass bool) (bool, error) {
	log := zlog.LogWithContext(ctx).With(
		zap.String("user_id", userID),
		zap.String("project_id", projectID),
		zap.String("provider", provider),
		zap.String("api_iden", apiIden),
		zap.Bool("require_audit_bypass", workflowRequireAuditBypass),
	)

	if !workflowRequireAuditBypass {
		log.Debug("workflow replacement skipped: workflow not marked for audit bypass")
		return false, nil
	}

	if provider != "cloud_comfy" {
		log.Debug("workflow replacement skipped: provider not cloud comfy")
		return false, nil
	}

	if !s.isReplacementApiIden(apiIden) {
		log.Debug("workflow replacement skipped: api_iden not eligible for replacement")
		return false, nil
	}

	redeemed := s.couponDao.RedeemedCoupon(ctx, projectID, userID, []string{"GOOGLENICE"})
	if !redeemed {
		log.Debug("workflow replacement skipped: user has not redeemed GOOGLENICE coupon")
		return false, nil
	}

	log.Info("workflow replacement eligible - all conditions met")
	return true, nil
}

// isReplacementApiIden 检查apiIden是否是需要替换的类型
func (s *WorkflowReplacementService) isReplacementApiIden(apiIden string) bool {
	return apiIden == "image_to_video" || apiIden == "text_to_video" || apiIden == "multi_image_to_video" || apiIden == "image_to_image"
}

// GetReplacementWorkflowID 获取替换的工作流ID
// 优先从workflow的AuditReplaceBy字段获取，如果为空则从全局配置获取
func (s *WorkflowReplacementService) GetReplacementWorkflowID(ctx context.Context, workflowInfo *model.Workflow) (string, error) {
	apiIden := workflowInfo.ApiConfig.ApiIden

	if workflowInfo.ApiConfig.AuditReplaceBy != "" {
		zlog.LogWithContext(ctx).Info("using workflow-level audit replacement",
			zap.String("workflow_id", workflowInfo.WorkflowID),
			zap.String("api_iden", apiIden),
			zap.String("audit_replace_by", workflowInfo.ApiConfig.AuditReplaceBy))
		return workflowInfo.ApiConfig.AuditReplaceBy, nil
	}

	configKey := s.getConfigKeyForApiIden(apiIden)
	if configKey == "" {
		return "", fmt.Errorf("unsupported api_iden for replacement: %s", apiIden)
	}

	var replacementWorkflowID string
	cacheKey := "workflow_replacement:" + configKey

	err := s.cacheService.GetOrSet(ctx, cacheKey, &replacementWorkflowID, 5*time.Minute, func() (interface{}, error) {
		workflowID, err := s.configService.GetConfValue(configKey)
		if err != nil {
			zlog.LogWithContext(ctx).Error("failed to get replacement workflow ID from config",
				zap.String("config_key", configKey),
				zap.Error(err))
			return "", fmt.Errorf("failed to get replacement workflow ID: %w", err)
		}

		if workflowID == "" {
			zlog.LogWithContext(ctx).Warn("replacement workflow ID is empty",
				zap.String("config_key", configKey),
				zap.String("api_iden", apiIden))
			return "", fmt.Errorf("replacement workflow ID is not configured for %s", apiIden)
		}

		zlog.LogWithContext(ctx).Debug("loaded replacement workflow ID from config",
			zap.String("config_key", configKey),
			zap.String("workflow_id", workflowID))
		return workflowID, nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to get replacement workflow ID: %w", err)
	}

	zlog.LogWithContext(ctx).Info("using global config audit replacement",
		zap.String("workflow_id", workflowInfo.WorkflowID),
		zap.String("api_iden", apiIden),
		zap.String("config_key", configKey),
		zap.String("replacement_workflow_id", replacementWorkflowID))

	return replacementWorkflowID, nil
}

// getConfigKeyForApiIden 根据apiIden获取对应的配置键
func (s *WorkflowReplacementService) getConfigKeyForApiIden(apiIden string) string {
	switch apiIden {
	case "image_to_video", "text_to_video":
		return constants.ConfigKeyCloudComfyVideoReplaceBy
	case "multi_image_to_video":
		return constants.ConfigKeyCloudComfyMultiImageToVideoReplaceBy
	case "image_to_image":
		return constants.ConfigKeyCloudComfyImageToImageReplaceBy
	default:
		return ""
	}
}

func (s *WorkflowReplacementService) GenerateComment(originalWorkflowID, replacementWorkflowID, apiIden string) string {
	return fmt.Sprintf("GOOGLENICE用户工作流替换: cloud comfy %s 工作流从 %s 替换为 %s",
		apiIden, originalWorkflowID, replacementWorkflowID)
}

func (s *PictureTaskService) extractFinalPrompt(ctx context.Context, task map[string]string) string {
	keys := []string{"inputPrompt", "custom_prompt", "prompt"}
	for _, key := range keys {
		if value, ok := task[key]; ok {
			return value
		}
	}
	return ""
}

// extractUserInputFromWorkflowInputs 从 WorkflowInput 列表中提取用户输入
// 返回 user_prompt（截断至 500 字符）和 input_images（图片 URL 列表）
func extractUserInputFromWorkflowInputs(inputs []*pb.WorkflowInput) (userPrompt string, inputImages []string) {
	inputImages = make([]string, 0)
	for _, input := range inputs {
		switch input.GetInputType() {
		case pb.MessageType_MT_TEXT:
			// 提取第一个非空文本作为 user_prompt
			if userPrompt == "" && input.GetInputContent() != "" {
				userPrompt = input.GetInputContent()
				// 截断至 500 字符
				if len(userPrompt) > 500 {
					userPrompt = userPrompt[:500]
				}
			}
		case pb.MessageType_MT_IMAGE:
			// 收集所有图片 URL
			if input.GetInputContent() != "" {
				inputImages = append(inputImages, input.GetInputContent())
			}
		}
	}
	return userPrompt, inputImages
}

// trackTaskSubmitEvent 上报 TASK_SUBMIT 事件
func (s *PictureTaskService) trackTaskSubmitEvent(
	ctx context.Context,
	task *model.PictureTask,
	workflowInfo *model.Workflow,
	workflowKind *model.WorkflowKind,
	workflowInputs []*pb.WorkflowInput,
	isMember bool,
) {
	// 检查 eventReporter 是否为 nil
	if s.eventReporter == nil {
		return
	}

	// 从 Context 读取 submit_source（默认 "normal"）
	submitSource := constants.SubmitSourceNormal
	if src := ctx.Value(constants.CtxTaskSubmitSource); src != nil {
		if srcStr, ok := src.(string); ok && srcStr != "" {
			submitSource = srcStr
		}
	}

	// 从 Context 读取 tool_id
	var toolID string
	if tid := ctx.Value(constants.CtxToolID); tid != nil {
		if tidStr, ok := tid.(string); ok {
			toolID = tidStr
		}
	}
	// 也检查 common.PictureToolIDKey
	if toolID == "" {
		if tid := ctx.Value(common.PictureToolIDKey); tid != nil {
			if tidStr, ok := tid.(string); ok {
				toolID = tidStr
			}
		}
	}

	// 构建 payload
	workflowType := pb.WorkflowKindType_WORKFLOW_KIND_TYPE_IMAGE.String()
	if workflowKind != nil {
		workflowType = workflowKind.KindType
	}
	payload := map[string]any{
		"task_id":        task.TaskID,
		"workflow_id":    task.WorkflowID,
		"workflow_type":  workflowType,
		"workflow_theme": task.Theme,
		"credit_points":  task.CreditPoints,
		"is_member":      isMember,
		"submit_source":  submitSource,
	}
	if toolID != "" {
		payload["tool_id"] = toolID
	}

	// 使用 EventBuilder 构建并异步上报
	s.eventReporter.NewEvent(event_reporter.EventTypeTaskSubmit).
		FromContext(ctx).
		Payload(payload).
		Track()

	zlog.LogWithContext(ctx).Debug("task submit event tracked",
		zap.String("task_id", task.TaskID),
		zap.String("submit_source", submitSource),
		zap.String("tool_id", toolID))
}
