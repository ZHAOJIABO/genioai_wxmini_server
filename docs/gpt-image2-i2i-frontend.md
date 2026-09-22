# GPT-Image-2 图生图接入文档（前端）

## 概述

基于 GPT-Image-2 模型的图生图功能，支持单图/多图输入，生成风格化、修图、合成等效果。

接口与文生图共用同一个端点，区别在于请求中携带 `user_images` 字段。

## 接口信息

- **接口**：`POST /v1/pictureforge/submit_picture_forge_task`
- **模型名称**：`gpt-image-2`（图生图与文生图共用同一模型，数据库中 `support_i2i = true`）

## 请求参数

```json
{
  "request_header": {
    "req_id": "web-req-xxx",
    "user_id": "用户ID",
    "access_token": "用户token",
    "web_client": {
      "package_name": "com.web.genioai",
      "client_version": "1.0.0"
    }
  },
  "model_config": {
    "model_name": "gpt-image-2",
    "num_images": 1,
    "seed": -1,
    "aspect_ratio": "1:1",
    "quality": "auto",
    "width": 1024,
    "height": 1024
  },
  "user_prompt": "把这张照片改成油画风格，保留人物五官",
  "user_images": [
    "https://example.com/input1.png"
  ]
}
```

## 字段说明

### model_config

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model_name` | string | 是 | 固定传 `gpt-image-2` |
| `num_images` | int | 否 | 生成图片数量，默认 1，最大 4 |
| `seed` | int | 否 | 随机种子，-1 表示随机 |
| `aspect_ratio` | string | 否 | 宽高比，如 `1:1`、`2:3`、`3:2`、`16:9` 等 |
| `quality` | string | 否 | 图片质量：`low`、`medium`、`high`、`auto` |
| `width` | int | 否 | 输出宽度，需与 aspect_ratio 对应 |
| `height` | int | 否 | 输出高度，需与 aspect_ratio 对应 |

### 顶层字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `user_prompt` | string | 是 | 图像编辑提示词，描述期望的修改效果 |
| `user_images` | string[] | 是（图生图） | 输入图片 URL 列表，支持 1~5 张 |
| `negative_prompt` | string | 否 | 负向提示词，描述不想要的内容 |
| `strength` | float | 否 | 图生图强度 0.0~1.0，默认 0.75（值越大变化越大） |

## 常用尺寸对照

| aspect_ratio | width | height |
| --- | --- | --- |
| `1:1` | 1024 | 1024 |
| `2:3` | 1024 | 1536 |
| `3:2` | 1536 | 1024 |
| `16:9` | 1536 | 1024 |
| `9:16` | 1024 | 1536 |

## 请求示例

### 单图编辑

```json
{
  "request_header": {
    "req_id": "web-req-1781772858958-abc123",
    "user_id": "6632b23de9e927fc10f4cca925ed0497",
    "access_token": "your_token_here",
    "web_client": {"package_name": "com.web.genioai", "client_version": "1.0.0"}
  },
  "model_config": {
    "model_name": "gpt-image-2",
    "num_images": 1,
    "seed": -1,
    "aspect_ratio": "1:1",
    "quality": "auto",
    "width": 1024,
    "height": 1024
  },
  "user_prompt": "把这张商品图改成白底电商主图，保留商品形状和材质",
  "user_images": [
    "https://cdn.example.com/product.png"
  ]
}
```

### 多图合成

```json
{
  "request_header": {
    "req_id": "web-req-1781772858958-def456",
    "user_id": "6632b23de9e927fc10f4cca925ed0497",
    "access_token": "your_token_here",
    "web_client": {"package_name": "com.web.genioai", "client_version": "1.0.0"}
  },
  "model_config": {
    "model_name": "gpt-image-2",
    "num_images": 1,
    "seed": -1,
    "aspect_ratio": "2:3",
    "quality": "auto",
    "width": 1024,
    "height": 1536
  },
  "user_prompt": "将两张图片中的人物自然合并到同一个室内场景中，保持真实光照和比例",
  "user_images": [
    "https://cdn.example.com/person.png",
    "https://cdn.example.com/room.png"
  ]
}
```

## 响应

### 提交成功

```json
{
  "responseHeader": {
    "code": "SUCCESS",
    "msg": "Success"
  },
  "taskId": "task_83f8aee1-25b8-4355-a760-6016451a2325"
}
```

### 参数错误

```json
{
  "responseHeader": {
    "code": "INVALID_PARAM",
    "msg": "Invalid Param"
  },
  "taskId": ""
}
```

常见原因：
- `user_prompt` 为空
- `user_images` 超过模型限制的最大输入图片数
- 模型不支持图生图（`support_i2i = false`）
- 用户未登录或 token 无效

## 任务结果查询

提交成功后，使用返回的 `taskId` 轮询任务状态：

- **接口**：`POST /v1/pictureforge/get_picture_forge_task_result`
- **轮询间隔**：建议 2~5 秒
- **终止条件**：任务状态为完成或失败时停止轮询

## 与文生图的区别

| | 文生图 | 图生图 |
| --- | --- | --- |
| `user_images` | 不传或空数组 | 必须传至少一张图片 URL |
| `user_prompt` | 描述要生成的图 | 描述要对输入图做的修改 |
| `strength` | 不生效 | 控制修改强度 |
| 模型 provider | `gpt_image2` | `gpt_image2_i2i` |

## 注意事项

1. 图片 URL 必须可被服务端公网访问，不要使用需要登录态的内网地址
2. `user_prompt` 要清楚描述期望的修改效果，而不是描述输入图本身
3. 多图合成时，prompt 应明确说明各图片的关系和融合方式
4. `quality` 目前服务端强制使用 `low` 以节省成本，前端传 `auto` 即可
