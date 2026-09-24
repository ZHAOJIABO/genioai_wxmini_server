package common

import (
	"encoding/json"
	"testing"

	vai "va_visionai_server/internal/va_interface"
)

func TestMultiMarshalerReturnsJSONAndAcceptsEncodedRequest(t *testing.T) {
	marshaler := NewMultiMarshaler()
	want := map[string]string{"message": "成功"}

	response, err := marshaler.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(response) {
		t.Fatalf("response is not JSON: %q", response)
	}
	var got map[string]string
	if err := json.Unmarshal(response, &got); err != nil {
		t.Fatal(err)
	}
	if got["message"] != want["message"] {
		t.Fatalf("response message = %q, want %q", got["message"], want["message"])
	}

	encodedRequest, err := marshaler.cryptoMarshaler.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := marshaler.Unmarshal(encodedRequest, &got); err != nil {
		t.Fatal(err)
	}
	if got["message"] != want["message"] {
		t.Fatalf("decoded request message = %q, want %q", got["message"], want["message"])
	}
}

func TestEncryptedProtoRequestAcceptsEnumName(t *testing.T) {
	request := map[string]interface{}{
		"workflow_id": "template-1",
		"workflow_input": []map[string]interface{}{{
			"input_name": "photo", "input_type": "MT_IMAGE", "input_content": "https://example.com/photo.png",
		}},
		"request_header": map[string]interface{}{
			"app": map[string]string{"package_name": "com.mini.genioai"},
		},
	}
	encoded, err := NewCryptoMarshaler().Marshal(request)
	if err != nil {
		t.Fatal(err)
	}

	var got vai.SubmitPictureForgeTaskRequest
	if err := NewCryptoMarshaler().Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetRequestHeader().GetApp().GetPackageName() != "com.mini.genioai" ||
		got.GetWorkflowInput()[0].GetInputType() != vai.MessageType_MT_IMAGE {
		t.Fatalf("decoded project or input type incorrectly: %#v", &got)
	}
}
