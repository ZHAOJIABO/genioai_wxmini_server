package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// // ExtractFrameFromVideoBytes 从视频字节流中提取首帧图像。
// // 它将视频数据写入临时文件，然后使用 gocv 打开并处理。
// // 返回提取帧的 JPEG 编码字节流和错误（如果发生）。
// func ExtractFrameFromVideoBytes(videoData []byte) ([]byte, error) {
// 	if len(videoData) == 0 {
// 		return nil, errors.New("video data is empty")
// 	}

// 	tempFile, err := os.CreateTemp("", "video_*.tmp")
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create temp file: %w", err)
// 	}
// 	defer os.Remove(tempFile.Name())
// 	defer tempFile.Close()

// 	if _, err := tempFile.Write(videoData); err != nil {
// 		return nil, fmt.Errorf("failed to write video data to temp file: %w", err)
// 	}
// 	if err := tempFile.Sync(); err != nil {
// 		return nil, fmt.Errorf("failed to sync temp file: %w", err)
// 	}
// 	tempFile.Close()

// 	vc, err := gocv.VideoCaptureFile(tempFile.Name())
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to open video file with gocv: %w", err)
// 	}
// 	defer vc.Close()

// 	vc.Set(gocv.VideoCapturePosFrames, 0)

// 	img := gocv.NewMat()
// 	defer img.Close()
// 	if ok := vc.Read(&img); !ok || img.Empty() {
// 		return nil, errors.New("failed to read first frame or frame is empty")
// 	}

// 	buf, err := gocv.IMEncode(gocv.JPEGFileExt, img)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to encode frame to JPEG: %w", err)
// 	}
// 	imgBytes := buf.GetBytes()
// 	buf.Close()

// 	if len(imgBytes) == 0 {
// 		return nil, errors.New("encoded image bytes are empty")
// 	}

// 	return imgBytes, nil
// }

// ExtractFrameFromVideoBytesWithFFmpeg 使用 FFmpeg 从视频字节流中提取首帧图像。
// 它将视频数据写入临时文件，然后使用 FFmpeg 提取首帧并保存为 JPEG 图像。
// 返回提取帧的 JPEG 编码字节流和错误（如果发生）。
func ExtractFrameFromVideoBytesWithFFmpeg(videoData []byte) ([]byte, error) {
	if len(videoData) == 0 {
		return nil, errors.New("video data is empty")
	}

	// 创建临时视频文件
	videoTempFile, err := os.CreateTemp("", "video_*.mp4")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp video file: %w", err)
	}
	defer os.Remove(videoTempFile.Name())
	defer videoTempFile.Close()

	// 写入视频数据
	if _, err := videoTempFile.Write(videoData); err != nil {
		return nil, fmt.Errorf("failed to write video data to temp file: %w", err)
	}
	if err := videoTempFile.Sync(); err != nil {
		return nil, fmt.Errorf("failed to sync temp video file: %w", err)
	}
	videoTempFile.Close()

	// 创建临时图像文件
	imageTempFile, err := os.CreateTemp("", "frame_*.jpg")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp image file: %w", err)
	}
	defer os.Remove(imageTempFile.Name())
	defer imageTempFile.Close()
	imageTempFile.Close()

	// 使用 FFmpeg 提取首帧
	// -i: 输入文件
	// -vframes 1: 仅提取 1 帧
	// -an: 禁用音频
	// -ss 0: 从开始位置提取
	// -y: 覆盖输出文件（如果存在）
	cmd := exec.Command(
		"ffmpeg",
		"-i", videoTempFile.Name(),
		"-vframes", "1",
		"-an",
		"-ss", "0",
		"-y",
		imageTempFile.Name(),
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w, output: %s", err, string(output))
	}

	// 读取生成的图像文件
	imgBytes, err := os.ReadFile(imageTempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read image file: %w", err)
	}

	if len(imgBytes) == 0 {
		return nil, errors.New("extracted image is empty")
	}

	return imgBytes, nil
}
