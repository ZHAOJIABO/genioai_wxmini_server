# Image2 图生图客户端接入文档

本文档面向调用 AI Brain 的服务端/客户端开发者，说明如何接入新增的 Image2 图生图能力。

Image2 图生图通过统一异步任务接口提交，AI Brain 内部会调用 VectorEngine 的 `POST /v1/images/edits` 接口。客户端不需要直接调用 VectorEngine，只需要调用 AI Brain 的 `SubmitAsyncTask` 和 `GetAsyncTask`。

## 适用场景

- 基于一张输入图进行风格化、修图、改图。
- 基于多张输入图做合成、融合、换场景。
- 需要复用现有 AI Brain 异步任务、任务查询、结果存储链路。

## 接入流程

1. 先将输入图片上传到可公网访问的 URL，或使用 AI Brain 的输入文件上传接口获取图片 URL。
2. 调用 `SubmitAsyncTask`，设置 `task_type=image_to_image`。
3. 设置 `provider_hint=gptimage`，模型推荐使用 `gpt-image-2-all`。
4. 将提示词、输入图片 URL、尺寸等参数放入 `parameters_json`。
5. 轮询 `GetAsyncTask`，直到任务状态为 `COMPLETED` 或 `FAILED`。
6. 从 `result.image_result.image_urls` 读取生成图片 URL。

## 提交任务

### RPC

```text
AIBrainService.SubmitAsyncTask
```

### 请求字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `task_type` | string | 是 | 固定传 `image_to_image` |
| `model` | string | 是 | 推荐 `gpt-image-2-all`；也兼容 `gpt-image-2` |
| `provider_hint` | string | 推荐 | 固定传 `gptimage`，避免走默认图生图 provider |
| `parameters_json` | string | 是 | JSON 字符串，详见下方参数说明 |

### parameters_json 参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `prompt` | string | 是 | 图像编辑提示词 |
| `image_urls` | string[] | 是 | 输入图片 URL 列表，推荐字段，支持一张或多张 |
| `image_url` | string | 否 | 单图输入别名 |
| `input_image_url` | string | 否 | 单图输入别名，兼容旧客户端 |
| `image` | string | 否 | 单图输入别名，可传 data URI |
| `size` | string | 否 | 输出尺寸，例如 `1024x1024`、`1024x1536` |
| `width` | number | 否 | 未传 `size` 时可与 `height` 一起生成尺寸 |
| `height` | number | 否 | 未传 `size` 时可与 `width` 一起生成尺寸 |
| `n` | number | 否 | 生成图片数量，默认 `1` |
| `quality` | string | 否 | 图片质量，例如 `low`、`medium`、`high`、`auto` |
| `format` | string | 否 | 输出格式，例如 `png`、`jpeg`、`webp` |
| `background` | string | 否 | 背景策略，例如 `transparent`、`opaque`、`auto` |
| `moderation` | string | 否 | 审核策略，例如 `low`、`auto` |
| `negative_prompt` | string | 否 | 负向提示词，会附加到 prompt 后 |

> 输入图必填。必须至少提供 `image_urls`、`image_url`、`input_image_url`、`image` 中的一个。

## 请求示例

### 多图合成

```json
{
  "task_type": "image_to_image",
  "model": "gpt-image-2-all",
  "provider_hint": "gptimage",
  "parameters_json": "{\"prompt\":\"将两张图片中的主体自然合并到同一个室内场景中，保持真实光照和比例\",\"image_urls\":[\"https://example.com/person.png\",\"https://example.com/room.png\"],\"size\":\"1024x1536\",\"n\":1,\"quality\":\"low\",\"format\":\"jpeg\"}"
}
```

展开后的 `parameters_json` 等价于：

```json
{
  "prompt": "将两张图片中的主体自然合并到同一个室内场景中，保持真实光照和比例",
  "image_urls": [
    "https://example.com/person.png",
    "https://example.com/room.png"
  ],
  "size": "1024x1536",
  "n": 1,
  "quality": "low",
  "format": "jpeg"
}
```

### 单图编辑

```json
{
  "task_type": "image_to_image",
  "model": "gpt-image-2-all",
  "provider_hint": "gptimage",
  "parameters_json": "{\"prompt\":\"把这张商品图改成白底电商主图，保留商品形状和材质\",\"input_image_url\":\"https://example.com/product.png\",\"size\":\"1024x1024\",\"format\":\"png\"}"
}
```

### Go 调用示例

```go
params := map[string]any{
	"prompt": "把这张照片改成电影海报风格，保留人物五官",
	"image_urls": []string{
		"https://example.com/input.png",
	},
	"size":    "1024x1024",
	"n":       1,
	"quality": "low",
	"format":  "jpeg",
}

paramsJSON, _ := json.Marshal(params)

resp, err := client.SubmitAsyncTask(ctx, &pb.SubmitAsyncTaskRequest{
	TaskType:       "image_to_image",
	Model:          "gpt-image-2-all",
	ProviderHint:   "gptimage",
	ParametersJson: string(paramsJSON),
})
if err != nil {
	return err
}

taskID := resp.GetTaskId()
```

## 提交响应

`SubmitAsyncTask` 成功后只表示任务已进入队列，不表示图片已经生成完成。

```json
{
  "task_id": "0d3d6c54-5e19-4f1a-a7c7-2bba2e4c9c9d",
  "status": "ASYNC_TASK_STATUS_PENDING",
  "created_at": 1781760000,
  "estimated_completion_at": 1781760300,
  "selected_provider": "gptimage",
  "selected_model": "gpt-image-2-all",
  "selection_source": "hint"
}
```

客户端需要保存 `task_id`，用于后续查询。

## 查询任务

### RPC

```text
AIBrainService.GetAsyncTask
```

### 请求示例

```json
{
  "task_id": "0d3d6c54-5e19-4f1a-a7c7-2bba2e4c9c9d"
}
```

### 处理中响应

```json
{
  "task_id": "0d3d6c54-5e19-4f1a-a7c7-2bba2e4c9c9d",
  "task_type": "ASYNC_TASK_TYPE_IMAGE_TO_IMAGE",
  "status": "ASYNC_TASK_STATUS_PROCESSING",
  "progress": 60,
  "created_at": 1781760000,
  "updated_at": 1781760060
}
```

### 完成响应

```json
{
  "task_id": "0d3d6c54-5e19-4f1a-a7c7-2bba2e4c9c9d",
  "task_type": "ASYNC_TASK_TYPE_IMAGE_TO_IMAGE",
  "status": "ASYNC_TASK_STATUS_COMPLETED",
  "progress": 100,
  "result": {
    "image_result": {
      "image_urls": [
        "https://cdn.example.com/gptimage-dev/output/task_0d3d6c54-5e19-4f1a-a7c7-2bba2e4c9c9d.png"
      ],
      "width": 1024,
      "height": 1536,
      "model_used": "gpt-image-2-all"
    }
  },
  "created_at": 1781760000,
  "updated_at": 1781760120,
  "completed_at": 1781760120
}
```

客户端应优先读取：

```text
result.image_result.image_urls
```

如果一次生成多张图，数组中会有多个 URL。

## 失败响应

### 缺少输入图

提交 `image_to_image` 时如果没有传输入图，会直接失败：

```text
image_urls, image_url, input_image_url, or image is required for image_to_image
```

处理方式：补充 `image_urls` 或任一单图字段后重试。

### Provider 或模型不可用

如果没有传 `provider_hint=gptimage`，请求可能按默认图生图路由走其他 provider。建议客户端显式传：

```json
{
  "provider_hint": "gptimage",
  "model": "gpt-image-2-all"
}
```

如果返回 provider selection 相关错误，检查：

- `provider_hint` 是否为 `gptimage`。
- `model` 是否为 `gpt-image-2-all` 或 `gpt-image-2`。
- 服务端是否已配置 `GPTIMAGE_BEARER_TOKEN`。

### 任务执行失败

查询结果中如果状态为 `ASYNC_TASK_STATUS_FAILED`，读取 `error_message`：

```json
{
  "task_id": "0d3d6c54-5e19-4f1a-a7c7-2bba2e4c9c9d",
  "task_type": "ASYNC_TASK_TYPE_IMAGE_TO_IMAGE",
  "status": "ASYNC_TASK_STATUS_FAILED",
  "progress": 60,
  "error_message": "API error (status 400): invalid_request - invalid image"
}
```

常见原因：

- 输入图片 URL 无法访问。
- 输入图片格式不支持。
- 图片过大或内容不符合上游审核策略。
- prompt 为空或描述不清晰。
- 上游接口返回错误。

## 客户端建议

- 推荐使用 `image_urls` 数组字段，即使只有一张图也传数组，便于后续扩展多图。
- 图片 URL 应可被 AI Brain 服务端访问，不要使用需要登录态的内网地址。
- 建议轮询间隔为 2 到 5 秒，任务完成或失败后停止轮询。
- 不要依赖提交响应中的 `estimated_completion_at` 判断完成，只以 `GetAsyncTask.status` 为准。
- 结果 URL 可能来自对象存储或上游返回地址，客户端只需要按普通图片 URL 使用。

## 最小可用请求

```json
{
  "task_type": "image_to_image",
  "model": "gpt-image-2-all",
  "provider_hint": "gptimage",
  "parameters_json": "{\"prompt\":\"把图片改成写实摄影风格\",\"image_urls\":[\"https://example.com/input.png\"]}"
}
```
