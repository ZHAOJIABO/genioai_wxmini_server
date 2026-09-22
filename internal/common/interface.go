package common

import (
	"context"

	vai "va_visionai_server/internal/va_interface"
)

type LlmHandler interface {
	Process(ctx context.Context, params LLMProcessParams) error
}

type LLMFactory interface {
	CreateHandler(ctx context.Context, modelID vai.Model) (context.Context, LlmHandler, error)
}
