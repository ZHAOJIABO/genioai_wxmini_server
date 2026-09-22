package test

import (
	"context"
	"testing"

	vai "va_visionai_server/internal/va_interface"
)

func TestLogin(t *testing.T) {
	client, _, err := Getclient()
	if err != nil {
		t.Error(err)
	}
	t.Log("test Temp login")

	req := &vai.LoginRequest{
		RequestHeader: GetHeader(true),
	}
	rsp, err := client.Login(context.Background(), req)
	if err != nil {
		t.Error(err)
	}
	if err := checkRspCode(rsp.GetResponseHeader()); err != nil {
		t.Error(err)
	}
	t.Run("chat", TestChat)
}
