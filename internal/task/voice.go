package task

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"va_visionai_server/conf"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/zlog"
)

type VoiceTaskProcessor struct {
	Queue chan string
	dao   *dao.VoiceDao
}

var Voice VoiceTaskProcessor

func NewVoiceTaskProcessor(dao *dao.VoiceDao) *VoiceTaskProcessor {
	return &VoiceTaskProcessor{
		Queue: make(chan string, 50),
		dao:   dao,
	}
}

type ASRResp struct {
	AudioInfo struct {
		Duration int `json:"duration"`
	} `json:"audio_info"`
	Result struct {
		Text       string `json:"text"`
		Utterances []struct {
			Definite  bool   `json:"definite"`
			EndTime   int    `json:"end_time"`
			StartTime int    `json:"start_time"`
			Text      string `json:"text"`
			Words     []struct {
				BlankDuration int    `json:"blank_duration"`
				EndTime       int    `json:"end_time"`
				StartTime     int    `json:"start_time"`
				Text          string `json:"text"`
			} `json:"words"`
		} `json:"utterances"`
	} `json:"result"`
}

func (v *VoiceTaskProcessor) Listen() {
	if v.Queue == nil {
		v.Queue = make(chan string, 50)
	}
	if v.dao == nil {
		v.dao = dao.NewVoiceDAO(db.GetDB())
	}
	go v.process()
	zlog.Logger.Info("Voice Task Listen...")
}

func (v *VoiceTaskProcessor) Submit(fileHash string) error {
	select {
	case v.Queue <- fileHash:
		zlog.Logger.Info("Voice Task Submit...", zap.String("fileHash", fileHash))
		return nil
	case <-time.After(15 * time.Second):
		return errors.New("Submit Task Timeout")
	}
}

func (v *VoiceTaskProcessor) process() {
	appKey := conf.GlobalConfig.LlmConfig.VolcEngineASR.AppKey
	accessKey := conf.GlobalConfig.LlmConfig.VolcEngineASR.AccessKey
	for i := 0; i < 10; i++ {
		go func() {
			for fileHash := range v.Queue {
				var voiceInfo model.Voice
				var err error

				if voiceInfo, err = v.dao.GetByFileHash(fileHash); err != nil {
					zlog.Logger.Error("Query Voice Info Error", zap.Error(err))
					continue
				}
				zlog.Logger.Info("Start Voice Task")
				taskID := ""
				// isPord := conf.IsProd()
				// if isPord {
				if voiceInfo.Status == constants.VoiceWaiting {
					taskID, err = v.submitTask(appKey, accessKey, voiceInfo)
					if err != nil {
						zlog.Logger.Error("Submit Task Error", zap.Error(err))
						continue
					}
				} else {
					taskID = voiceInfo.TaskID
				}
				if err := v.fetchResult(appKey, accessKey, taskID, voiceInfo); err != nil {
					zlog.Logger.Error("Fetch Result Error", zap.Error(err))
					continue
				}
				// } else {
				// 	text, err := v.PredictAudioText(voiceInfo.FileUrl)
				// 	if err != nil {
				// 		zlog.Logger.Error("Whisper ASR Error", zap.Error(err))
				// 	}
				// 	voiceInfo.Status = constants.VoiceSuccess
				// 	voiceInfo.ParseStartTime = sql.NullTime{Valid: true, Time: time.Now()}
				// 	voiceInfo.Content = text
				// 	if err := vdao.UpdateStatus(voiceInfo); err != nil {
				// 		zlog.Logger.Error("Update Voice Status Error", zap.Error(err))
				// 	}
				// }
			}
		}()
	}
}

type Response struct {
	Headers http.Header `json:"headers"`
}

func (v *VoiceTaskProcessor) submitTask(appKey, accessKey string, voiceInfo model.Voice) (string, error) {
	submitURL := conf.GlobalConfig.LlmConfig.VolcEngineASR.Endpoint + "/submit"
	taskID := uuid.New().String()

	headers := map[string]string{
		"X-Api-App-Key":     appKey,
		"X-Api-Access-Key":  accessKey,
		"X-Api-Resource-Id": "volc.bigasr.auc",
		"X-Api-Request-Id":  taskID,
		"X-Api-Sequence":    "-1",
	}

	requestBody := map[string]interface{}{
		//"user": map[string]string{
		//	"uid": "",
		//},
		"audio": map[string]interface{}{
			"url":     voiceInfo.FileUrl,
			"format":  "mp3",
			"codec":   "raw",
			"rate":    16000,
			"bits":    16,
			"channel": 1,
		},
		"request": map[string]interface{}{
			"model_name":      "bigmodel",
			"enable_itn":      true,
			"enable_punc":     true,
			"enable_ddc":      false,
			"show_utterances": true,
			"corpus": map[string]string{
				"boosting_table_name": "",
				"correct_table_name":  "",
				"context":             "",
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(context.TODO(), "POST", submitURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		zlog.Logger.Error("IO Read VoiceTask Resp Body Error", zap.Error(err))
		return "", err
	}
	zlog.Logger.Info("Submit Voice Task Resp",
		zap.Any("body", string(respBody)),
		zap.Any("params", string(jsonBody)),
	)

	respCode := resp.Header.Get("X-Api-Status-Code")
	respMsg := resp.Header.Get("X-Api-Message")
	respLogID := resp.Header.Get("X-Tt-Logid")
	errMsg := fmt.Sprintf("%s|%s|%s", respCode, respMsg, respLogID)
	voiceInfo.TaskID = taskID
	if respCode == "20000000" {
		voiceInfo.Status = constants.VoicePending
		voiceInfo.ParseStartTime = sql.NullTime{Valid: true, Time: time.Now()}
		if err := v.dao.UpdateStatus(voiceInfo); err != nil {
			zlog.Logger.Error("Update Voice Status Error", zap.Error(err))
			return taskID, err
		}
	} else {
		zlog.Logger.Error("Submit ASR Task Error",
			zap.String("respCode", respCode),
			zap.String("respMsg", respMsg),
			zap.String("respLogID", respLogID),
		)
		voiceInfo.ErrMsg = errMsg
		voiceInfo.Status = constants.VoiceError
		if err := v.dao.UpdateStatus(voiceInfo); err != nil {
			zlog.Logger.Error("Update Voice Status Error", zap.Error(err))
			return taskID, err
		}
		return taskID, errors.New("Submit Voice Task Error")
	}

	return taskID, nil
}

func (v *VoiceTaskProcessor) fetchResult(appKey, accessKey, taskID string, voice model.Voice) error {
	currentRetry := 0
	for {
		resp, err := v.queryTask(appKey, accessKey, taskID)
		if err != nil {
			zlog.Logger.Error("Query Voice Task Result Error", zap.Error(err))
			return err
		}
		defer resp.Body.Close()

		respCode := resp.Header.Get("X-Api-Status-Code")
		respMsg := resp.Header.Get("X-Api-Message")
		respLogID := resp.Header.Get("X-Tt-Logid")

		code := resp.Header.Get("X-Api-Status-Code")
		if code == "20000000" {
			var result ASRResp
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				fmt.Println("Failed to decode response body:", err)
				return err
			}
			voice.Content = result.Result.Text
			voice.Status = constants.VoiceSuccess
			voice.ParseEndTime = sql.NullTime{Valid: true, Time: time.Now()}
			if err := v.dao.UpdateStatus(voice); err != nil {
				zlog.Logger.Error("Update Voice Content Error", zap.Error(err))
			}
			zlog.Logger.Info("Voice Task Success",
				zap.Any("voiceID", voice.ID),
				zap.String("taskID", taskID),
			)
			return err
		} else if code != "20000001" && code != "20000002" { // task failed
			zlog.Logger.Error("Query Voice Task Result Error",
				zap.String("respCode", respCode),
				zap.String("respMsg", respMsg),
				zap.String("respLogID", respLogID),
			)
			errMsg := fmt.Sprintf("%s|%s|%s", respCode, respMsg, respLogID)
			voice.Status = constants.VoiceError
			voice.ErrMsg = errMsg
			if err := v.dao.UpdateStatus(voice); err != nil {
				zlog.Logger.Error("Update Voice Status Error", zap.Error(err))
			}
			return errors.New("Query Voice Task Result Error")
		} else {
			currentRetry++
			if currentRetry > 100 {
				zlog.Logger.Error("Query Voice Task Result Error,More Than Max Retry Count",
					zap.String("fileHash", voice.FileHash),
					zap.String("taskID", taskID),
				)
				return errors.New("The number of retries exceeded the maximum. Procedure")
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func (v *VoiceTaskProcessor) queryTask(appKey, accessKey, taskID string) (*http.Response, error) {
	queryURL := conf.GlobalConfig.LlmConfig.VolcEngineASR.Endpoint + "/query"

	headers := map[string]string{
		"X-Api-App-Key":     appKey,
		"X-Api-Access-Key":  accessKey,
		"X-Api-Resource-Id": "volc.bigasr.auc",
		"X-Api-Request-Id":  taskID,
	}

	req, err := http.NewRequestWithContext(context.TODO(), "POST", queryURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (v *VoiceTaskProcessor) PredictAudioText(audioURL string) (string, error) {
	// Download the audio file
	resp, err := http.Get(audioURL)
	if err != nil {
		return "", fmt.Errorf("failed to download audio file: %w", err)
	}
	defer resp.Body.Close()

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read audio data: %w", err)
	}

	// Prepare multipart form data with the audio file.
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("audio", "audio.m4a")
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(audioBytes); err != nil {
		return "", fmt.Errorf("failed to write audio data to form: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close writer: %w", err)
	}

	// POST the multipart form data to the API.
	apiURL := "http://120.133.56.109:8081/predict"
	req, err := http.NewRequest("POST", apiURL, &buf)
	if err != nil {
		return "", fmt.Errorf("failed to create POST request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	apiResp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform POST request: %w", err)
	}
	defer apiResp.Body.Close()

	// Define a structure to parse the API response.
	var apiResponse struct {
		Result struct {
			Text string `json:"text"`
		} `json:"result"`
		Status string `json:"status"`
	}

	if err := json.NewDecoder(apiResp.Body).Decode(&apiResponse); err != nil {
		return "", fmt.Errorf("failed to decode API response: %w", err)
	}

	if apiResponse.Status != "success" {
		return "", fmt.Errorf("prediction failed, status: %s", apiResponse.Status)
	}

	return apiResponse.Result.Text, nil
}

// Stop 停止语音处理任务
func (v *VoiceTaskProcessor) Stop() {
	if v.Queue != nil {
		close(v.Queue)
		zlog.Logger.Info("语音任务处理器已停止")
	}
}
