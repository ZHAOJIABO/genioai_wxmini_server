package picture_generate

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
	pb "va_visionai_server/internal/va_interface"
)

type EvaluateService struct {
	taskDao    *dao.PictureTaskDao
	llmFactory common.LLMFactory
	promptDao  *dao.PromptDao
}

func NewEvaluateService(taskDao *dao.PictureTaskDao, llmFactory common.LLMFactory, promptDao *dao.PromptDao) *EvaluateService {
	return &EvaluateService{
		taskDao:    taskDao,
		llmFactory: llmFactory,
		promptDao:  promptDao,
	}
}

func (s *EvaluateService) EvaluatePicture(ctx context.Context, taskID string, imageURL string) error {
	task, err := s.taskDao.GetTask(context.TODO(), taskID)
	if err != nil {
		return errors.Wrap(err, "get task")
	}
	scores, err := s.AnalyzeImageScore(ctx, imageURL)
	if err != nil {
		return errors.Wrap(err, "analyze image score")
	}

	overallScore := s.calculateOverallScore(scores)

	scoreJSON, err := json.Marshal(scores)
	if err != nil {
		return errors.Wrap(err, "marshal score")
	}
	task.ScoreJSON = string(scoreJSON)
	task.Score = overallScore

	if err := s.taskDao.UpdateTask(context.TODO(), task); err != nil {
		return errors.Wrap(err, "update task")
	}

	return nil
}

func (s *EvaluateService) calculateOverallScore(scores []model.ScoreData) float64 {
	var total float64
	for _, score := range scores {
		val, err := strconv.ParseFloat(score.Value, 64)
		if err != nil {
			continue
		}
		total += val
	}
	// 返回平均分
	if len(scores) > 0 {
		return total / float64(len(scores))
	}
	return 0
}

func (s *EvaluateService) AnalyzeImageScore(ctx context.Context, imageURL string) ([]model.ScoreData, error) {
	if imageURL == "" {
		return nil, errors.New("image URL is required")
	}

	prompt, err := s.promptDao.GetPrompt(constants.SystemPictureEvaluateScore, "zh")
	if err != nil {
		return nil, fmt.Errorf("get prompt failed: %w", err)
	}

	msgHistory := []model.MessageHistory{
		{
			URLs:     []string{imageURL},
			Sender:   "user",
			FileType: pb.MessageType_MT_IMAGE,
			Content:  prompt.Content,
		},
	}

	output := make(model.Output)
	cancelCh := make(model.CancelCh)
	defer close(cancelCh)

	req := &pb.ChatMessageSendRequest{
		Message: &pb.Message{
			ModelId:      pb.Model_MODEL_GEMINI_2_5_FLASH,
			SystemPrompt: prompt.Content,
		},
	}

	response := strings.Builder{}
	go func() {
		defer utils.SafeCloseChan(output)
		for msg := range output {
			response.WriteString(strings.Join(msg, ""))
		}
	}()

	var handler common.LlmHandler
	ctx, handler, err = s.llmFactory.CreateHandler(context.Background(), pb.Model_MODEL_GEMINI_2_5_FLASH)
	if err != nil {
		return nil, fmt.Errorf("create llm handler failed: %w", err)
	}

	err = handler.Process(ctx, common.LLMProcessParams{
		Request:      req,
		StreamServer: nil,
		History:      msgHistory,
		Output:       output,
		CancelCh:     cancelCh,
	})
	if err != nil {
		return nil, fmt.Errorf("process image failed: %w", err)
	}

	result := []model.ScoreData{}
	if err := json.Unmarshal([]byte(response.String()), &result); err != nil {
		return nil, fmt.Errorf("parse result failed: %w", err)
	}

	return result, nil
}
