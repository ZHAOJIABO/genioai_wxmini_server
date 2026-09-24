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

func TestProtoRequestDecodingVariants(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		marshaler interface {
			Unmarshal([]byte, interface{}) error
		}
	}{
		{
			name:      "plain streaming request with camel case",
			body:      `{"requestHeader":{"app":{"packageName":"com.mini.genioai"}},"workflowInput":[{"inputType":"MT_IMAGE"}]}`,
			marshaler: NewStreamCryptoMarshaler(),
		},
		{
			name:      "plain streaming request with numeric enum and unknown field",
			body:      `{"request_header":{"web_client":{"package_name":"com.mini.genioai"}},"workflow_input":[{"input_type":1}],"future_field":true}`,
			marshaler: NewStreamCryptoMarshaler(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got vai.SubmitPictureForgeTaskRequest
			if err := tc.marshaler.Unmarshal([]byte(tc.body), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.GetWorkflowInput()) != 1 || got.GetWorkflowInput()[0].GetInputType() != vai.MessageType_MT_IMAGE {
				t.Fatal("workflow input enum was not decoded")
			}
			header := got.GetRequestHeader()
			if header.GetApp().GetPackageName() != "com.mini.genioai" && header.GetWebClient().GetPackageName() != "com.mini.genioai" {
				t.Fatal("project ID was not decoded")
			}
		})
	}
}
