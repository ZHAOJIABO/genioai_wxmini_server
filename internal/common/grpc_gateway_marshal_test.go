package common

import (
	"encoding/json"
	"testing"
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
