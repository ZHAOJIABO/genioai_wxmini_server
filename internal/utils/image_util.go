package utils

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/nfnt/resize"
	"golang.org/x/image/draw"

	"va_visionai_server/internal/constants"

	_ "golang.org/x/image/webp"
)

// GetImageInfo 获取图片的基本信息
func GetImageInfo(img image.Image) (width, height int) {
	bounds := img.Bounds()
	width = bounds.Max.X - bounds.Min.X
	height = bounds.Max.Y - bounds.Min.Y
	return width, height
}

// ImageAspectRatio 计算图片高宽比
func ImageAspectRatio(img image.Image) (int, int, float64) {
	width, height := GetImageInfo(img)
	if height == 0 {
		return 0, 0, 0
	}
	return width, height, float64(height) / float64(width)
}

//func CompressImage(img image.Image, width, height int) ([]byte, error) {
//	lowQualityImg := imaging.Resize(img, 360, 0, imaging.Lanczos)
//
//	// Encode the low-quality image to buffer
//	buf := new(bytes.Buffer)
//	err := imaging.Encode(buf, lowQualityImg, imaging.JPEG, imaging.JPEGQuality(70))
//	if err != nil {
//		return nil, fmt.Errorf("failed to encode low-quality image: %v", err)
//	}
//
//	return buf.Bytes(), nil
//}

// func CompressImageWithMaxSize(img image.Image, maxSize int, format imaging.Format) ([]byte, error) {
// 	resizedImg := imaging.Fit(img, maxSize, maxSize, imaging.Lanczos)

// 	buf := new(bytes.Buffer)
// 	if err := imaging.Encode(buf, resizedImg, format); err != nil {
// 		return nil, fmt.Errorf("compress and encode image failed: %v", err)
// 	}

// 	return buf.Bytes(), nil
// }

func CompressImageWithMaxSize(img image.Image, maxSize int, format imaging.Format) ([]byte, error) {
	intermediateSize := int(float64(maxSize) * 1.5)
	intermediateImg := imaging.Fit(img, intermediateSize, intermediateSize, imaging.Box)
	resizedImg := imaging.Fit(intermediateImg, maxSize, maxSize, imaging.Linear)
	resizedImg = imaging.Blur(resizedImg, 0.5)

	if format == imaging.PNG {
		resizedImg = imaging.Blur(resizedImg, 0.3)
	}

	buf := new(bytes.Buffer)
	if err := imaging.Encode(buf, resizedImg, format); err != nil {
		return nil, fmt.Errorf("compress and encode image failed: %v", err)
	}

	return buf.Bytes(), nil
}

// ScaleImageDimensions 将图片尺寸等比缩放到最大边长1080
func ScaleImageDimensions(width, height int) (newWidth, newHeight int) {
	maxDim := 1080
	aspectRatio := float64(width) / float64(height)

	if width > height {
		if width > maxDim {
			newWidth = maxDim
			newHeight = int(float64(maxDim) / aspectRatio)
		} else {
			newWidth = width
			newHeight = height
		}
	} else {
		if height > maxDim {
			newHeight = maxDim
			newWidth = int(float64(maxDim) * aspectRatio)
		} else {
			newWidth = width
			newHeight = height
		}
	}

	return newWidth, newHeight
}

func ConvertToWebp(img image.Image) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := webp.Encode(buf, img, &webp.Options{Quality: 80})
	if err != nil {
		return nil, fmt.Errorf("failed to encode image to webp: %v", err)
	}

	return buf.Bytes(), nil
}

// 帮助函数：创建带 MIME 类型的 form 文件字段
func CreateFormFileWithMIME(w *multipart.Writer, fieldname, filename string, file io.Reader) error {
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldname, filename))

	// 检测 mime 类型
	ext := filepath.Ext(filename)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	partHeader.Set("Content-Type", mimeType)

	part, err := w.CreatePart(partHeader)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

// 生成蒙版图片
func GenerateMaskImage(imagePath, maskPath string) error {
	// 打开原图以获取尺寸
	srcImgFile, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer srcImgFile.Close()

	// 解码图像
	srcImg, _, err := image.Decode(srcImgFile)
	if err != nil {
		return err
	}

	// 获取原图的尺寸
	bounds := srcImg.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// 创建一个同尺寸的 RGBA 图片（透明背景）
	maskImg := image.NewRGBA(image.Rect(0, 0, width, height))

	// 可选：先填满透明（虽然 image.NewRGBA 默认就是透明）
	draw.Draw(maskImg, maskImg.Bounds(), &image.Uniform{color.RGBA{0, 0, 0, 0}}, image.Point{}, draw.Src)

	// 画一个白色不透明的矩形（你可以改成其他区域）
	rect := image.Rect(width/4, height/4, 3*width/4, 3*height/4)
	draw.Draw(maskImg, rect, &image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)

	// 保存为 mask.png
	outFile, err := os.Create(maskPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// 编码为 PNG 格式
	if err := png.Encode(outFile, maskImg); err != nil {
		return err
	}

	// 返回成功
	return nil
}

// 动态压缩图片
func CompressImage(inputBytes []byte) ([]byte, error) {
	if len(inputBytes) <= constants.MaxImageSize {
		return inputBytes, errors.New("小于最大尺寸，无需压缩")
	}

	img, format, err := image.Decode(bytes.NewReader(inputBytes))
	println("format:", format)
	println(err)
	if err != nil {
		return inputBytes, err
	}

	// 降低分辨率逐步压缩
	scaleFactor := 1.0
	currImg := img
	buf := new(bytes.Buffer)
	err = jpeg.Encode(buf, currImg, &jpeg.Options{Quality: 100})
	if err != nil {
		return nil, err
	}

	for buf.Len() > constants.MaxImageSize && scaleFactor > 0.1 {
		scaleFactor -= 0.1
		newWidth := uint(float64(currImg.Bounds().Dx()) * scaleFactor)
		newHeight := uint(float64(currImg.Bounds().Dy()) * scaleFactor)
		println(buf.Len())
		resized := resize.Resize(newWidth, newHeight, currImg, resize.Lanczos3)

		currImg = resized
		buf.Reset()
		err = jpeg.Encode(buf, currImg, &jpeg.Options{Quality: 100})
		if err != nil {
			return nil, err
		}
	}

	// 调整 JPEG 质量
	quality := 100
	for buf.Len() > constants.MaxImageSize && quality > 0 {
		buf.Reset()
		err = jpeg.Encode(buf, currImg, &jpeg.Options{Quality: quality})
		if err != nil {
			return nil, err
		}
		quality--
	}

	return buf.Bytes(), nil
}

// ExtractFirstFrameFromGIF 从GIF数据中提取第一帧
func ExtractFirstFrameFromGIF(gifData []byte) (image.Image, error) {
	reader := bytes.NewReader(gifData)

	// 解码GIF
	gifImg, err := gif.DecodeAll(reader)
	if err != nil {
		return nil, fmt.Errorf("decode GIF failed: %w", err)
	}

	if len(gifImg.Image) == 0 {
		return nil, errors.New("GIF contains no frames")
	}

	// 返回第一帧
	return gifImg.Image[0], nil
}

// IsImageURL 判断字符串是否为图片URL
func IsImageURL(url string) bool {
	if !strings.HasPrefix(url, "http") {
		return false
	}

	lowerURL := strings.ToLower(url)
	return strings.Contains(lowerURL, ".jpg") ||
		strings.Contains(lowerURL, ".jpeg") ||
		strings.Contains(lowerURL, ".png") ||
		strings.Contains(lowerURL, ".gif") ||
		strings.Contains(lowerURL, ".bmp") ||
		strings.Contains(lowerURL, ".webp") ||
		strings.Contains(lowerURL, "image") ||
		strings.Contains(lowerURL, "picture") ||
		strings.Contains(lowerURL, "photo")
}
