package common

type contextKey string

// EnableOptimizedParamsKey 用于在 Context 中传递是否启用 Prompt 优化的布尔值。
var EnableOptimizedParamsKey contextKey = "enableOptimizedParams"

// PictureToolIDKey 用于在 Context 中传递图片工具的 ID。
var PictureToolIDKey contextKey = "pictureToolID"
