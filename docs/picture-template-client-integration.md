# 模版生图客户端对接文档

面向客户端开发。描述「用户浏览模版分类 → 选模版 → 上传原图 → 生成效果图」的完整调用流程。

后端实现上，「模版」即 **Workflow**，「模版分类」即 **WorkflowKind**。文档中出现的 `workflow_id` 就是模版 ID。模版自带固定 prompt，由服务端注入，客户端不传、也无法覆盖。

## 1. 接口总览

| 步骤 | 功能 | 方法与路径 |
| --- | --- | --- |
| 1 | 分类 + 分类下模版（首屏一次拉全） | `POST /v1/pictureforge/list_kind_with_workflows` |
| 1a | 仅分类列表 | `POST /v1/pictureforge/list_workflow_kind` |
| 1b | 指定分类下模版分页 | `POST /v1/pictureforge/list_workflow_page` |
| 2 | 上传用户原图 | `POST /v1/media/upload_with_opts` |
| 3 | 提交生成任务 | `POST /v1/pictureforge/submit_picture_forge_task` |
| 4 | 查询任务结果（轮询） | `POST /v1/pictureforge/get_picture_forge_task_result` |
| 4a | 任务进度推送（流式，可选） | `POST /v1/report/event_watch` |
| 5 | 重试失败任务 | `POST /v1/pictureforge/retry_failed_task` |
| 6 | 用户生成历史 | `POST /v1/pictureforge/list_user_picture_forge_task` |

全部为 `POST`，`Content-Type: application/json`，参数放 body。

## 2. 通信格式

### 环境差异

| 环境 | 请求体 / 响应体 |
| --- | --- |
| DEV / LOCAL | 普通 JSON。响应为标准 grpc-gateway JSON（camelCase 字段、字符串枚举） |
| PROD | 整个 JSON body 经 XOR + Base64 编码，发送/接收原始 Base64 文本 |

PROD 的编解码在 `internal/common/grpc_gateway_marshal.go`（`CryptoMarshaler`），仅在 `conf.IsProd()` 为真时挂载（`cmd/main.go:283`）。客户端应复用项目现有网关编解码模块，密钥需与后端一致。

Base64 文本不要再次 `JSON.stringify`，也不要包成 `{data: ...}`。本文所有示例均为**编码前 / 解码后**的数据。

### 公共请求头

每个请求都要带 `request_header`：

```json
{
  "request_header": {
    "req_id": "客户端生成的请求ID",
    "user_id": "业务用户ID",
    "access_token": "业务Access Token",
    "app": { "package_name": "com.example.app", "app_version": "1.0.0" },
    "device": { "os": 2 }
  }
}
```

| 字段 | 说明 |
| --- | --- |
| `user_id` | **必填**。为空时接口直接返回 `INVALID_USER(2004)` |
| `access_token` | 按现有约定携带 |
| `app` / `device` | 移动端字段，可沿用现有封装 |

### 公共响应头

```json
{ "response_header": { "req_id": "...", "code": 0, "msg": "", "server_time": "..." } }
```

`code == 0`（`SUCCESS`）为成功。常用错误码：

| code | 含义 | 客户端处理 |
| --- | --- | --- |
| 1001 | `INVALID_PARAM` 参数非法 | 检查入参 |
| 1003 | `REQUEST_FAILED` 请求失败 | 展示 `msg` |
| 2002 | `EXPIRED_ACCESS_TOKEN` | 刷新 token 后重试 |
| 2003 | `INVALID_REFRESH_TOKEN` | 重新登录 |
| 2004 | `INVALID_USER` | 重新登录 |
| 4003 | `AMOUNT_EXHAUSTED` 积分耗尽 | 引导充值 |
| 4005 | `FILE_TOO_LARGE` 文件过大 | 压缩后重传 |
| 4010 | `UPLOAD_CONTENT_SENSITIVE` 内容敏感 | 提示更换图片 |
| 4012 | `TASK_PROCESSING_LIMIT_EXCEEDED` 并发任务超限 | 提示等待当前任务完成 |

---

## 3. 第一步：拉取模版分类与模版

### 方案 A：一次拉全（推荐）

首屏一个请求拿到所有分类及每个分类下的前 N 个模版，适合「分类横滑 + 模版网格」。

```json
POST /v1/pictureforge/list_kind_with_workflows
{
  "request_header": { "...": "..." },
  "limit": 10
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `limit` | int32 | 每个分类下返回的模版数量上限 |
| `workflow_theme` | string | 可选，按主题过滤 |

响应：

```json
{
  "response_header": { "code": 0 },
  "workflow_kinds": [
    {
      "kind_id": "portrait_style",
      "label": "人像风格",
      "kind_type": "WORKFLOW_KIND_TYPE_IMAGE",
      "description": "分类描述",
      "display_style": "WORKFLOW_KIND_DISPLAY_STYLE_ONE",
      "cover_pictures": [{ "url": "...", "aspect_ratio": 1.0 }],
      "workflows": [ "见下方 Workflow 结构" ]
    }
  ]
}
```

### 方案 B：分类与模版分开拉

分类多、单分类模版多时使用。

```json
POST /v1/pictureforge/list_workflow_kind
{
  "request_header": { "...": "..." },
  "workflow_kind_type": "WORKFLOW_KIND_TYPE_IMAGE"
}
```

```json
POST /v1/pictureforge/list_workflow_page
{
  "request_header": { "...": "..." },
  "kind_id": "portrait_style",
  "workflow_kind_type": "WORKFLOW_KIND_TYPE_IMAGE",
  "page_common": { "page": 0, "page_size": 20 }
}
```

响应中 `page_common.total` 为总数，`page` 从 `0` 开始。

### Workflow（模版）结构

| 字段 | 类型 | 客户端用途 |
| --- | --- | --- |
| `workflow_id` | string | **模版 ID，提交任务必传** |
| `title` | string | 模版名称 |
| `description` | string | 模版说明 |
| `icon` | string | 模版图标 |
| `example_image` | object | **示例原图 / 结果图，用于前后对比展示** |
| `inputs` | array | **入参定义，提交任务时按此构造，见第 5 节** |
| `face_count` | int32 | 要求的人脸数，`0` 表示不限制 |
| `credit_cost` | int32 | 消耗积分 |
| `is_free` | bool | 是否免费 |
| `is_new` / `is_hot` | bool | 新上 / 热门角标 |
| `kind_type` | enum | `WORKFLOW_KIND_TYPE_IMAGE` / `_VIDEO` |

`example_image` 结构：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ori_content` | string[] | 示例**原图** URL 列表 |
| `result_content` | string | 示例**结果图** URL |
| `aspect_ratio` | float | 结果图高宽比 |
| `result_thumbnail_url` | string | 结果图缩略图，列表页优先用这个 |

`WorkflowKindDisplayStyle` 控制分类的展示样式，客户端按需映射布局：

| 值 | 含义 |
| --- | --- |
| `WORKFLOW_KIND_DISPLAY_STYLE_ONE` | 普通 |
| `WORKFLOW_KIND_DISPLAY_STYLE_TWO` | 重点-多图 |
| `WORKFLOW_KIND_DISPLAY_STYLE_THREE` | 重点-16:9 |
| `WORKFLOW_KIND_DISPLAY_STYLE_FOUR` | 重点-大图 |

---

## 4. 第二步：上传用户原图

```json
POST /v1/media/upload_with_opts
{
  "request_header": { "...": "..." },
  "file_name": "photo.jpg",
  "file_type": "FT_IMAGE",
  "file_size": 123456,
  "data": "<图片二进制的 Base64>",
  "image_opts": { "face_detection": true }
}
```

响应：

```json
{
  "response_header": { "code": 0 },
  "file_url": "https://cdn.example.com/xxx.jpg",
  "image_info": { "...": "..." }
}
```

拿到的 `file_url` 用于第三步提交。

**建议开启 `face_detection`。** 若所选模版的 `face_count > 0`，提交任务时服务端会校验图中人脸数是否匹配，不匹配直接失败。上传阶段就检测可以提前给用户提示，而不是等提交后报错。

---

## 5. 第三步：提交生成任务

```json
POST /v1/pictureforge/submit_picture_forge_task
{
  "request_header": { "...": "..." },
  "workflow_id": "选中的模版ID",
  "workflow_input": [
    {
      "input_name": "LoadImage_1",
      "input_type": "MT_IMAGE",
      "input_content": "上一步返回的 file_url"
    }
  ]
}
```

响应：

```json
{ "response_header": { "code": 0 }, "task_id": "xxx" }
```

### 构造 workflow_input 的规则

**`input_name` 必须直接照抄第一步返回的 `workflow.inputs[].input_name`，不要自行命名。**

服务端执行器依赖 `input_name` 的前缀识别图片参数——只有以 `input_image_` 或 `LoadImage` 开头的入参才会被当作图片传给模型（`internal/service/picture_generate/gpt_image2_i2i_executor.go:124`）。名字写错不会报错，但用户的原图会被静默丢弃，最终生成一张与原图无关的图。

遍历 `workflow.inputs`，逐项构造：

| `input_type` | 处理方式 |
| --- | --- |
| `MT_IMAGE` | `input_content` 填用户上传的 `file_url`。多个图片入参按顺序对应多张图 |
| `MT_TEXT` | 保持模版返回的 `input_content` 原值不动 |

**不要传 prompt。** 模版的固定 prompt 存在服务端 `Workflow.Prompt`，提交时由服务端注入（`internal/service/picture_task.go:292`）。客户端若传入名为 `InputPrompt` 的入参，会**覆盖**模版的固定 prompt（`gpt_image2_i2i_executor.go:115-118`）——除非明确要做「用户自定义提示词」功能，否则不要传。

### 提交前的本地校验

- `workflow_id` 非空
- 至少一张图片入参已填充
- 若 `face_count > 0`，用上传时的人脸检测结果先行拦截
- 若 `is_free == false`，先确认用户积分足够，避免提交后拿 `AMOUNT_EXHAUSTED(4003)`

---

## 6. 第四步：等待生成结果

### 方案 A：轮询（推荐，简单可靠）

```json
POST /v1/pictureforge/get_picture_forge_task_result
{
  "request_header": { "...": "..." },
  "task_id": "xxx"
}
```

响应：

```json
{
  "response_header": { "code": 0 },
  "picture_forge_info": {
    "task_id": "xxx",
    "task_status": "WORKFLOW_TASK_STATUS_COMPLETED",
    "task_progress": 100,
    "task_result_picture_info": {
      "url": "https://cdn.example.com/result.jpg",
      "thumbnail_url": "...",
      "aspect_ratio": 1.5,
      "file_id": "..."
    },
    "user_picture_info": [{ "url": "用户输入的原图", "aspect_ratio": 1.0 }],
    "can_retry": false,
    "task_fail_reason": "",
    "task_fail_reason_full": "",
    "workflow": { "...": "模版信息" }
  }
}
```

建议轮询间隔 **2~3 秒**，并设置整体超时（图生图通常几十秒，视频更久）。

### 任务状态机

| 值 | 枚举 | 是否终态 |
| --- | --- | --- |
| 0 | `WORKFLOW_TASK_STATUS_PENDING` 未开始 | 否 |
| 20 | `WORKFLOW_TASK_STATUS_PROCESSING` 处理中 | 否 |
| 30 | `WORKFLOW_TASK_STATUS_PENDING_SUBMISSION_RETRY` 等待重试提交 | 否 |
| 40 | `WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION` 等待外部完成 | 否 |
| 50 | `WORKFLOW_TASK_STATUS_PENDING_CHAIN` 等待任务链触发 | 否 |
| **100** | `WORKFLOW_TASK_STATUS_COMPLETED` 已完成 | **是** |
| **10** | `WORKFLOW_TASK_STATUS_FAILED` 失败 | **是** |

**只把 `100` 和 `10` 当终态，其余（含 30 / 40 / 50）继续轮询。** 状态值不连续，不要用大小比较判断进度，只做等值判断。

- `100`：读 `task_result_picture_info.url` 展示结果图
- `10`：用 `task_fail_reason` 做标题、`task_fail_reason_full` 做详情展示；`can_retry == true` 时展示重试入口

### 方案 B：进度推送（流式，可选）

`POST /v1/report/event_watch` 是 server-streaming 接口，省去轮询流量。

```json
{
  "request_header": { "...": "..." },
  "event_type": "WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS"
}
```

服务端持续下推：

```json
{
  "event_type": "WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS",
  "task_progress_data": {
    "picture_task_id": "xxx",
    "progress": 60,
    "picture_info": { "url": "..." },
    "can_retry": false,
    "task_fail_reason": "",
    "task_fail_reason_full": ""
  }
}
```

注意 `task_progress_data` **不含 `task_status` 字段**。判断终态需靠：`picture_info.url` 非空视为成功，`task_fail_reason` 非空视为失败。

推送依赖长连接，移动端易受切后台、弱网影响。建议**推送为主 + 轮询兜底**：收到推送即刷新 UI，同时保留一个低频（如 10 秒）轮询兜底，避免连接静默断开后卡在「生成中」。

---

## 7. 重试与历史

### 重试失败任务

```json
POST /v1/pictureforge/retry_failed_task
{ "request_header": { "...": "..." }, "task_id": "xxx" }
```

仅在结果里 `can_retry == true` 时展示入口。重试后 `task_id` 不变，继续按第六节轮询。

### 用户生成历史

```json
POST /v1/pictureforge/list_user_picture_forge_task
{ "request_header": { "...": "..." }, "page_common": { "page": 0, "page_size": 20 } }
```

相关可选接口：

| 功能 | 路径 |
| --- | --- |
| 发布到社区 | `POST /v1/pictureforge/publish_picture` |
| 删除生成图 | `POST /v1/pictureforge/delete_picture` |
| 最近上传的图片 | `POST /v1/pictureforge/list_user_upload_picture` |
| 社区图片流 | `POST /v1/pictureforge/list_community_picture` |

---

## 8. 完整流程时序

```
客户端                                          服务端
  │
  │─ 1. list_kind_with_workflows ──────────────▶
  │◀───────── 分类 + 模版（含示例原图/结果图）──┤
  │
  │  用户浏览分类，点击某个模版
  │  展示 example_image.ori_content / result_content 前后对比
  │
  │  用户选择本地图片
  │─ 2. upload_with_opts（含人脸检测）─────────▶
  │◀───────────────────────── file_url ────────┤
  │
  │  按 workflow.inputs 构造 workflow_input
  │─ 3. submit_picture_forge_task ─────────────▶
  │◀───────────────────────── task_id ─────────┤
  │
  │─ 4. get_picture_forge_task_result ─────────▶  ┐
  │◀──── task_status=20 / 40, progress ────────┤  │ 每 2~3 秒
  │  ...                                          ┘
  │─ 4. get_picture_forge_task_result ─────────▶
  │◀──── task_status=100 + 结果图 URL ─────────┤
  │
  │  展示结果图（可发布 / 保存 / 再次生成）
```

## 9. 易错点汇总

1. **`input_name` 自行命名** —— 必须照抄 `workflow.inputs[].input_name`。图片入参名不以 `input_image_` / `LoadImage` 开头时，原图会被静默丢弃，生成结果与原图无关且不报错。这是最容易踩且最难排查的问题。
2. **误传 `InputPrompt`** —— 会覆盖模版的固定 prompt，导致模版效果失效。
3. **用大小比较判断任务状态** —— 状态值是 0/10/20/30/40/50/100，不连续。`40` 不是失败，`10` 才是。只做等值判断。
4. **漏处理 30 / 40 / 50** —— 把它们当未知状态而停止轮询，会导致任务卡在「生成中」。
5. **PROD 环境漏做 XOR + Base64** —— 请求会直接解析失败。同时响应也需解码，勿让 HTTP 库自动按 JSON 解析。
6. **忽略 `face_count`** —— 模版要求人脸而用户图无人脸时提交必失败，应在上传阶段用 `face_detection` 提前拦截。
7. **只依赖流式推送** —— `task_progress_data` 无状态字段，且长连接易断，需轮询兜底。

## 10. 相关代码位置

| 关注点 | 位置 |
| --- | --- |
| 接口定义与 HTTP 路径 | `pkg/va_interface/service.proto` |
| 模版 / 任务消息体 | `pkg/va_interface/picture.proto` |
| 枚举（状态、类型、错误码） | `pkg/va_interface/constants.proto`、`common.proto` |
| 模版数据模型 | `internal/model/picture_forge.go` |
| 模版查询实现 | `internal/service/picture_forge/workflow_query_service.go` |
| 任务提交与参数组装 | `internal/service/picture_task.go` |
| 图生图执行器 | `internal/service/picture_generate/gpt_image2_i2i_executor.go` |
| PROD body 编解码 | `internal/common/grpc_gateway_marshal.go` |
