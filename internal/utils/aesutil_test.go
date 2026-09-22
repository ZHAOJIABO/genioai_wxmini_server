package utils

import "testing"

func TestAesEncryptDecrypt(t *testing.T) {
	plainText := "Hello, World!"

	// 测试 AES 加密
	encrypted, err := AesEncrypt([]byte(plainText))
	if err != nil {
		t.Fatalf("AesEncrypt error: %v", err)
	}
	if encrypted == "" {
		t.Fatal("AesEncrypt returned an empty string")
	}

	// 测试 AES 解密
	decrypted, err := AesDecrypt(encrypted)
	if err != nil {
		t.Fatalf("AesDecrypt error: %v", err)
	}

	if decrypted != plainText {
		t.Fatalf("Decrypted text does not match original. Got %s, want %s", decrypted, plainText)
	}
}

func TestAesDecryptInvalidData(t *testing.T) {
	invalidCipherText := "invalid ciphertext"

	_, err := AesDecrypt(invalidCipherText)
	if err == nil {
		t.Fatal("AesDecrypt should return an error for invalid input")
	}
}
