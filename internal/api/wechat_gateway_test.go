package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"va_visionai_server/internal/common"
	"va_visionai_server/internal/service/auth"
	vai "va_visionai_server/internal/va_interface"
)

type wechatGatewayStub struct {
	vai.UnimplementedAuthServiceServer
	received *vai.WeChatAuthRequest
	session  *vai.WeChatSessionRequest
	reset    bool
}

func (s *wechatGatewayStub) WeChatCheckSession(_ context.Context, req *vai.WeChatSessionRequest) (*vai.WeChatSessionResponse, error) {
	s.session, s.reset = req, false
	return &vai.WeChatSessionResponse{Valid: true}, nil
}

func (s *wechatGatewayStub) WeChatResetSession(_ context.Context, req *vai.WeChatSessionRequest) (*vai.WeChatSessionResponse, error) {
	s.session, s.reset = req, true
	return &vai.WeChatSessionResponse{Valid: true}, nil
}

func TestWeChatSessionGateway(t *testing.T) {
	for _, codec := range []runtime.Marshaler{&runtime.JSONPb{}, common.NewMultiMarshaler()} {
		for _, reset := range []bool{false, true} {
			mux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, codec))
			stub := &wechatGatewayStub{}
			if err := vai.RegisterAuthServiceHandlerServer(context.Background(), mux, stub); err != nil {
				t.Fatal(err)
			}
			body, err := codec.Marshal(&vai.WeChatSessionRequest{RequestHeader: &vai.RequestHeader{UserId: "user", AccessToken: "business-token", App: &vai.App{PackageName: "project"}}})
			if err != nil {
				t.Fatal(err)
			}
			path := "/v1/auth/wechat_check_session"
			if reset {
				path = "/v1/auth/wechat_reset_session"
			}
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, req)
			if response.Code != http.StatusOK {
				t.Fatalf("route failed: %d", response.Code)
			}
			if stub.reset != reset || stub.session.GetRequestHeader().GetUserId() != "user" || stub.session.GetRequestHeader().GetAccessToken() != "business-token" {
				t.Fatal("session request was not forwarded")
			}
			var result vai.WeChatSessionResponse
			if err := codec.Unmarshal(bytes.TrimSpace(response.Body.Bytes()), &result); err != nil {
				t.Fatal(err)
			}
			if !result.Valid {
				t.Fatal("missing session result")
			}
		}
	}
}

func TestWeChatSessionRequiresBusinessToken(t *testing.T) {
	server := &AuthServer{authService: auth.NewAuthService(nil, nil, nil, nil, nil)}
	for _, reset := range []bool{false, true} {
		response, err := server.weChatSession(context.Background(), &vai.WeChatSessionRequest{}, reset)
		if err != nil {
			t.Fatal(err)
		}
		if response.GetResponseHeader().GetCode() != vai.StatusCode_INVALID_ACCESS_TOKEN || response.Valid {
			t.Fatal("anonymous session operation accepted")
		}
	}
}

func (s *wechatGatewayStub) WeChatAuth(_ context.Context, req *vai.WeChatAuthRequest) (*vai.AuthResponse, error) {
	s.received = req
	return &vai.AuthResponse{AccessToken: "business-token", AuthType: vai.AuthType_AUTH_TYPE_WECHAT}, nil
}

func TestWeChatGateway(t *testing.T) {
	for _, production := range []bool{false, true} {
		name := "development"
		var codec runtime.Marshaler = &runtime.JSONPb{}
		if production {
			name = "production"
			codec = common.NewMultiMarshaler()
		}
		t.Run(name, func(t *testing.T) {
			mux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, codec))
			stub := &wechatGatewayStub{}
			if err := vai.RegisterAuthServiceHandlerServer(context.Background(), mux, stub); err != nil {
				t.Fatal(err)
			}
			body, err := codec.Marshal(&vai.WeChatAuthRequest{
				Code: "one-use-code", AuthType: "miniprogram",
				RequestHeader: &vai.RequestHeader{App: &vai.App{PackageName: "com.example.mini"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/auth/wechat_auth", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
			}
			if stub.received.GetCode() != "one-use-code" || stub.received.GetAuthType() != "miniprogram" || stub.received.GetRequestHeader().GetApp().GetPackageName() != "com.example.mini" {
				t.Fatalf("incorrect request: %v", stub.received)
			}
			var response vai.AuthResponse
			if err := codec.Unmarshal(bytes.TrimSpace(recorder.Body.Bytes()), &response); err != nil {
				t.Fatal(err)
			}
			if response.GetAccessToken() != "business-token" || response.GetAuthType() != vai.AuthType_AUTH_TYPE_WECHAT {
				t.Fatalf("incorrect response: %v", &response)
			}
		})
	}
}
