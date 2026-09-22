package submitcontext

import (
	"encoding/json"

	"va_visionai_server/internal/constants"
	vai "va_visionai_server/internal/va_interface"
)

type DeviceInfo struct {
	OS        string `json:"os,omitempty"`
	OSVersion string `json:"osVersion,omitempty"`
	Model     string `json:"model,omitempty"`
}

type AppInfo struct {
	Version string `json:"version,omitempty"`
}

type SubmitContext struct {
	Device DeviceInfo     `json:"device,omitempty"`
	App    AppInfo        `json:"app,omitempty"`
	Extra  map[string]any `json:"extra,omitempty"`
}

// BuildFromHeader 首期仅填 OS（来源于枚举映射）。若后续补齐版本/型号，可从 header.App.Version 与 header.Device.Model 读取。
func BuildFromHeader(h *vai.RequestHeader) SubmitContext {
	if h == nil || h.GetDevice() == nil {
		return SubmitContext{}
	}
	dev := h.GetDevice()
	ctx := SubmitContext{
		Device: DeviceInfo{
			OS:        constants.MappingOS(dev.GetOs()),
			OSVersion: "",
			Model:     dev.GetModel(),
		},
	}

	if h.GetApp() != nil {
		ctx.App = AppInfo{Version: h.GetApp().GetAppVersion()}
	}

	return ctx
}

func MustMarshal(ctx SubmitContext) string {
	b, err := json.Marshal(ctx)
	if err != nil {
		return "{}"
	}
	return string(b)
}
