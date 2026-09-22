package utils

import "testing"

func TestEncode(t *testing.T) {
	input := "Hello, World!"
	expectedOutput := "SGVsbG8sIFdvcmxkIQ=="
	output := Base64Encode(input)

	if output != expectedOutput {
		t.Errorf("Encode(%s) = %s; want %s", input, output, expectedOutput)
	}
}

// 测试 Decode 函数
func TestDecode(t *testing.T) {
	input := "SGVsbG8sIFdvcmxkIQ=="
	expectedOutput := "Hello, World!"
	output, err := Base64Decode(input)
	if err != nil {
		t.Errorf("Decode(%s) returned error: %v", input, err)
	}

	if output != expectedOutput {
		t.Errorf("Decode(%s) = %s; want %s", input, output, expectedOutput)
	}
}

// 测试 Decode 函数处理无效 Base64 字符串的情况
func TestDecodeInvalid(t *testing.T) {
	invalidInput := "InvalidBase64Data"

	_, err := Base64Decode(invalidInput)

	if err == nil {
		t.Errorf("Decode(%s) should have returned an error", invalidInput)
	}
}
