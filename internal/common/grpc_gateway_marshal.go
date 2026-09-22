package common

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
)

type CryptoMarshaler struct {
	key []byte
}

func NewCryptoMarshaler() *CryptoMarshaler {
	return &CryptoMarshaler{
		key: []byte("visionai-2025-01-06-123456789aes"),
	}
}

// MultiMarshaler 用于根据不同条件选择合适的 marshaler
type MultiMarshaler struct {
	cryptoMarshaler *CryptoMarshaler
	streamMarshaler *StreamCryptoMarshaler
}

func NewMultiMarshaler() *MultiMarshaler {
	return &MultiMarshaler{
		cryptoMarshaler: NewCryptoMarshaler(),
		streamMarshaler: NewStreamCryptoMarshaler(),
	}
}

// StreamPaths 存储流式接口的路径配置
var StreamPaths = map[string]bool{
	"/v1/chat/message/stream": true, // 假设这是聊天流式接口
}

// isStreamPath 检查是否为流式请求路径
func isStreamPath(path string) bool {
	return StreamPaths[path]
}

// RequestInfo 存储请求相关信息
type RequestInfo struct {
	Path        string
	IsStreaming bool
}

// currentRequest 存储当前请求信息
var currentRequest RequestInfo

// SetRequestInfo 设置请求信息
func SetRequestInfo(path string) {
	currentRequest = RequestInfo{
		Path:        path,
		IsStreaming: isStreamPath(path),
	}
}

func (m *MultiMarshaler) ContentType(v interface{}) string {
	return "application/json"
}

func (m *MultiMarshaler) Marshal(v interface{}) ([]byte, error) {
	if currentRequest.IsStreaming {
		return m.streamMarshaler.Marshal(v)
	}
	return m.cryptoMarshaler.Marshal(v)
}

func (m *MultiMarshaler) Unmarshal(data []byte, v interface{}) error {
	if currentRequest.IsStreaming {
		return m.streamMarshaler.Unmarshal(data, v)
	}
	return m.cryptoMarshaler.Unmarshal(data, v)
}

func (m *MultiMarshaler) NewDecoder(r io.Reader) runtime.Decoder {
	return &multiDecoder{
		reader:    r,
		marshaler: m,
	}
}

func (m *MultiMarshaler) NewEncoder(w io.Writer) runtime.Encoder {
	return &multiEncoder{
		writer:    w,
		marshaler: m,
	}
}

type multiDecoder struct {
	reader    io.Reader
	marshaler *MultiMarshaler
}

type multiEncoder struct {
	writer    io.Writer
	marshaler *MultiMarshaler
}

func (d *multiDecoder) Decode(v interface{}) error {
	data, err := io.ReadAll(d.reader)
	if err != nil {
		return err
	}
	return d.marshaler.Unmarshal(data, v)
}

func (e *multiEncoder) Encode(v interface{}) error {
	data, err := e.marshaler.Marshal(v)
	if err != nil {
		return err
	}
	_, err = e.writer.Write(data)
	return err
}

// 基本 CryptoMarshaler 的方法实现
func (m *CryptoMarshaler) ContentType(v interface{}) string {
	return "application/json"
}

func (m *CryptoMarshaler) encrypt(data []byte) []byte {
	result := make([]byte, len(data))
	for i := range data {
		result[i] = data[i] ^ m.key[i%len(m.key)]
	}
	return result
}

func (m *CryptoMarshaler) decrypt(data []byte) []byte {
	return m.encrypt(data) // XOR 加密是对称的
}

func (m *CryptoMarshaler) Marshal(v interface{}) ([]byte, error) {
	// 使用 buffer 来处理 JSON 编码
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false) // 避免 HTML 转义，这对中文很重要

	if err := encoder.Encode(v); err != nil {
		return nil, err
	}

	// 去除 JSON 编码器添加的尾部换行符
	data := bytes.TrimSpace(buffer.Bytes())

	// 加密数据
	encrypted := m.encrypt(data)

	// Base64 编码
	return []byte(base64.StdEncoding.EncodeToString(encrypted)), nil
}

func (m *CryptoMarshaler) Unmarshal(data []byte, v interface{}) error {
	// Base64 解码
	encryptedData, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return err
	}

	// 解密数据
	decrypted := m.decrypt(encryptedData)

	// 使用 decoder 来处理 JSON 解码
	decoder := json.NewDecoder(bytes.NewReader(decrypted))
	decoder.UseNumber() // 使用 Number 类型来保持数字精度
	return decoder.Decode(v)
}

func (m *CryptoMarshaler) NewDecoder(r io.Reader) runtime.Decoder {
	return &cryptoDecoder{
		reader:    r,
		marshaler: m,
	}
}

func (m *CryptoMarshaler) NewEncoder(w io.Writer) runtime.Encoder {
	return &cryptoEncoder{
		writer:    w,
		marshaler: m,
	}
}

type cryptoDecoder struct {
	reader    io.Reader
	marshaler *CryptoMarshaler
}

type cryptoEncoder struct {
	writer    io.Writer
	marshaler *CryptoMarshaler
}

func (d *cryptoDecoder) Decode(v interface{}) error {
	data, err := io.ReadAll(d.reader)
	if err != nil {
		return err
	}
	return d.marshaler.Unmarshal(data, v)
}

func (e *cryptoEncoder) Encode(v interface{}) error {
	data, err := e.marshaler.Marshal(v)
	if err != nil {
		return err
	}
	_, err = e.writer.Write(data)
	return err
}

// StreamCryptoMarshaler 实现流式接口的加解密
type StreamCryptoMarshaler struct {
	key []byte
}

func NewStreamCryptoMarshaler() *StreamCryptoMarshaler {
	return &StreamCryptoMarshaler{
		key: []byte("visionai-2025-01-06-123456789aes"),
	}
}

func (m *StreamCryptoMarshaler) ContentType(v interface{}) string {
	return "application/json"
}

func (m *StreamCryptoMarshaler) encrypt(data []byte) []byte {
	result := make([]byte, len(data))
	for i := range data {
		result[i] = data[i] ^ m.key[i%len(m.key)]
	}
	return result
}

func (m *StreamCryptoMarshaler) decrypt(data []byte) []byte {
	return m.encrypt(data)
}

func (m *StreamCryptoMarshaler) Marshal(v interface{}) ([]byte, error) {
	// 流式返回使用明文 JSON
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(v); err != nil {
		return nil, err
	}

	return bytes.TrimSpace(buffer.Bytes()), nil
}

func (m *StreamCryptoMarshaler) Unmarshal(data []byte, v interface{}) error {
	// 解密请求数据
	if needsDecryption(string(data)) {
		decryptedData, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return err
		}
		data = m.decrypt(decryptedData)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(v)
}

func (m *StreamCryptoMarshaler) NewDecoder(r io.Reader) runtime.Decoder {
	return &streamDecoder{
		reader:    r,
		marshaler: m,
	}
}

func (m *StreamCryptoMarshaler) NewEncoder(w io.Writer) runtime.Encoder {
	return &streamEncoder{
		writer:    w,
		marshaler: m,
	}
}

type streamDecoder struct {
	reader    io.Reader
	marshaler *StreamCryptoMarshaler
}

type streamEncoder struct {
	writer    io.Writer
	marshaler *StreamCryptoMarshaler
}

func (d *streamDecoder) Decode(v interface{}) error {
	data, err := io.ReadAll(d.reader)
	if err != nil {
		return err
	}
	return d.marshaler.Unmarshal(data, v)
}

func (e *streamEncoder) Encode(v interface{}) error {
	data, err := e.marshaler.Marshal(v)
	if err != nil {
		return err
	}
	_, err = e.writer.Write(data)
	return err
}

// needsDecryption 检查是否需要解密
func needsDecryption(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// 如果是明文 JSON，不需要解密
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return false
	}
	// 尝试 base64 解码
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}
