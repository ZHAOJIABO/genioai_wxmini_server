package bootstrap

import (
	"net/http"

	"github.com/go-redis/redis"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/service"
	aigcclient "va_visionai_server/internal/service/ai_clients/aigc_core"
	"va_visionai_server/internal/service/cache"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/service/event_reporter"
	"va_visionai_server/internal/service/llm"
	"va_visionai_server/internal/service/picture_forge"
	"va_visionai_server/internal/service/picture_generate"
)

type PictureModuleDeps struct {
	PictureForgeService  *picture_forge.PictureForgeService
	PictureTaskService   *service.PictureTaskService
	EvaluateService      *picture_generate.EvaluateService
	ImageGenerator       *picture_generate.ImageGenerator
	FaceDetectService    *service.FaceDetectService
	ExecutorService      *picture_generate.ExecutorService
	CreditService        credit.Service
	ToolAggregateService *picture_forge.ToolAggregateService
}

func InitPictureModule(repos *dao.Repositories, llmFactory *llm.LLMFactoryImpl, rdb *redis.Client, aigcClient *aigcclient.Client, cacheService *cache.CacheService, eventReporter event_reporter.EventReporter) *PictureModuleDeps {
	creditDao := dao.NewCreditDao()
	creditService := credit.NewService(creditDao, db.GetDB())

	faceDetectService := service.NewFaceDetectService()
	evaluateService := picture_generate.NewEvaluateService(repos.Task, llmFactory, repos.Prompt)
	imageGenerator := picture_generate.NewImageGenerator(repos.Task, evaluateService, rdb, &http.Client{}, repos.Upload)

	executorService := picture_generate.NewExecutorService(
		repos.Task,
		repos.Upload,
		repos.Config,
		rdb,
		creditService,
		evaluateService,
		aigcClient,
		repos.Prompt,
	)

	configService := service.NewConfigService()
	pictureForgeService := picture_forge.NewPictureForgeService(repos.PicForge, repos.Task, configService, cacheService)
	toolAggregateService := picture_forge.NewToolAggregateService(repos.PicForge)
	workflowReplacementService := service.NewWorkflowReplacementService(configService, cacheService, repos.Coupon)
	pictureTaskService := service.NewPictureTaskService(
		db.GetDB(),
		llmFactory,
		repos.Task,
		repos.PicForge,
		repos.Upload,
		repos.Prompt,
		rdb,
		evaluateService,
		imageGenerator,
		faceDetectService,
		executorService,
		creditService,
		configService,
		aigcClient,
		cacheService,
		workflowReplacementService,
		eventReporter,
	)
	return &PictureModuleDeps{
		PictureForgeService:  pictureForgeService,
		PictureTaskService:   pictureTaskService,
		EvaluateService:      evaluateService,
		ImageGenerator:       imageGenerator,
		FaceDetectService:    faceDetectService,
		ExecutorService:      executorService,
		CreditService:        creditService,
		ToolAggregateService: toolAggregateService,
	}
}
