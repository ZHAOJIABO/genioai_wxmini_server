package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

var key = []byte("visionai@2024822") // 固定 16 字节密钥

// PKCS7Padding 填充数据以适应块大小
func PKCS7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// PKCS7UnPadding 去除填充数据
func PKCS7UnPadding(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("decrypted data is empty")
	}
	unPadding := int(data[length-1])
	if unPadding > length {
		return nil, errors.New("unpadding size is invalid")
	}
	return data[:(length - unPadding)], nil
}

// AesEncrypt AES加密
func AesEncrypt(plainText []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	plainText = PKCS7Padding(plainText, block.BlockSize())

	cipherText := make([]byte, aes.BlockSize+len(plainText))
	iv := cipherText[:aes.BlockSize]

	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(cipherText[aes.BlockSize:], plainText)

	return base64.StdEncoding.EncodeToString(cipherText), nil
}

// AesDecrypt AES解密
func AesDecrypt(cipherText string) (string, error) {
	cipherData, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	if len(cipherData) < aes.BlockSize {
		return "", errors.New("cipherText too short")
	}

	iv := cipherData[:aes.BlockSize]
	cipherData = cipherData[aes.BlockSize:]

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(cipherData, cipherData)

	plainText, err := PKCS7UnPadding(cipherData)
	if err != nil {
		return "", err
	}

	return string(plainText), nil
}
