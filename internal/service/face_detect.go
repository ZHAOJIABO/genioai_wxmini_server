package service

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"go.uber.org/zap"

	"va_visionai_server/internal/utils"
	"va_visionai_server/internal/zlog"

	_ "image/jpeg"
	_ "image/png"
)

type FaceDetectService struct {
	socketPath string
	conn       net.Conn
	mu         sync.Mutex
	closed     bool
}

type faceDetectRequest struct {
	Image string `json:"image"`
}

type faceDetectResponse struct {
	FaceCount    int   `json:"face_count"`
	DetectTimeMs int64 `json:"detect_time_ms"`
}

func NewFaceDetectService() *FaceDetectService {
	s := &FaceDetectService{
		socketPath: "/tmp/face_detect.sock",
	}
	if err := s.connect(); err != nil {
		zlog.Logger.Error("initial connection failed", zap.Error(err))
	}
	return s
}

func (s *FaceDetectService) connect() error {
	conn, err := net.Dial("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("connect to face detect service failed: %v", err)
	}

	if s.conn != nil {
		s.conn.Close()
	}
	s.conn = conn
	return nil
}

//nolint:perfsprint
func (s *FaceDetectService) ensureConnected() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("service is closed")
	}

	if s.conn == nil {
		return s.connect()
	}

	s.conn.SetDeadline(time.Now().Add(time.Second))
	_, err := s.conn.Write([]byte{})
	s.conn.SetDeadline(time.Time{})
	if err != nil {
		return s.connect()
	}
	return nil
}

func (s *FaceDetectService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func (s *FaceDetectService) FaceDetect(imageBytes []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		//nolint:perfsprint
		return 0, fmt.Errorf("service is closed")
	}

	if s.conn == nil {
		if err := s.connect(); err != nil {
			return 0, fmt.Errorf("connect failed: %v", err)
		}
	}

	// 转换为image.Image
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return 0, fmt.Errorf("decode image failed: %v", err)
	}

	// 压缩图片
	compressedBytes, err := utils.CompressImageWithMaxSize(img, 1080, imaging.PNG)
	if err != nil {
		return 0, fmt.Errorf("compress image failed: %v", err)
	}

	imageBase64 := base64.StdEncoding.EncodeToString(compressedBytes)
	request := faceDetectRequest{
		Image: imageBase64,
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("marshal request failed: %v", err)
	}

	for retry := 0; retry < 2; retry++ {
		s.conn.SetDeadline(time.Now().Add(120 * time.Second))

		if err := binary.Write(s.conn, binary.BigEndian, uint32(len(requestData))); err != nil {
			if retry < 1 {
				s.connect()
				continue
			}
			return 0, fmt.Errorf("write request length failed after retry: %v", err)
		}

		if _, err := s.conn.Write(requestData); err != nil {
			if retry < 1 {
				s.connect()
				continue
			}
			return 0, fmt.Errorf("write request data failed after retry: %v", err)
		}

		var responseLen uint32
		if err := binary.Read(s.conn, binary.BigEndian, &responseLen); err != nil {
			if retry < 1 {
				s.connect()
				continue
			}
			return 0, fmt.Errorf("read response length failed after retry: %v", err)
		}

		responseData := make([]byte, responseLen)
		if _, err := io.ReadFull(s.conn, responseData); err != nil {
			if retry < 1 {
				s.connect()
				continue
			}
			return 0, fmt.Errorf("read response data failed after retry: %v", err)
		}

		s.conn.SetDeadline(time.Time{})

		var response faceDetectResponse
		if err := json.Unmarshal(responseData, &response); err != nil {
			return 0, fmt.Errorf("unmarshal response failed: %v", err)
		}

		return response.FaceCount, nil
	}

	//nolint:perfsprint
	return 0, fmt.Errorf("max retries exceeded")
}
