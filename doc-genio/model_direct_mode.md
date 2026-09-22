# 模型直连模式使用文档

## 概述

本项目现在支持两种图片生成模式：

1. **Workflow模式（原有）**：使用预定义的workflow配置
2. **模型直连模式（新增）**：用户直接选择模型和参数进行生图

两种模式完全兼容，可以共存使用。

---

## 架构设计

### 1. 核心组件

```
┌─────────────────────────────────────────────────────┐
│                  API Layer                          │
│  SubmitPictureForgeTask                            │
│    ├── Workflow模式 (workflow_id)                   │
│    └── 模型直连模式 (model_config)                   │
└─────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────┐
│           ModelConfigManager                        │
│  - 模型元数据管理                                     │
│  - 模型能力验证                                       │
│  - 默认参数配置                                       │
└─────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────┐
│              Service Layer                          │
│  ├── PrepareTaskInputsAndWorkflow (workflow模式)    │
│  └── PrepareTaskInputsFromModelDirect (模型直连)     │
└─────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────┐
│              Task Executor                          │
│  执行器不关心来源，统一处理taskParams                  │
└─────────────────────────────────────────────────────┘
```

### 2. 虚拟Workflow适配

模型直连模式通过构建"虚拟workflow"来适配现有流程：

```go
// API层构建虚拟workflow
virtualWorkflow := &model.Workflow{
    WorkflowID:   "model_direct_flux-dev_xxxxx",
    Provider:     "comfyui",
    Prompt:       req.GetUserPrompt(),  // 用户输入
    CreditPoints: modelMeta.CreditPoints,
    ApiConfig: model.WorkflowApiConfig{
        ModelName:  "flux-dev",
        ApiIden:    "flux_dev",
        TaskType:   "text2image",
        // ...
    },
}
```

---

## 模型配置

### 1. 配置文件位置

```
conf/model_config.json
```

### 2. 模型配置示例

```json
{
  "model_name": "flux-dev",
  "display_name": "Flux Dev",
  "provider": "comfyui",
  "api_iden": "flux_dev",
  "effect_scene": "general",
  "task_type": "text2image",
  "credit_points": 10,
  "default_width": 1024,
  "default_height": 1024,
  "default_steps": 28,
  "default_cfg_scale": 3.5,
  "support_t2i": true,
  "support_i2i": true,
  "max_images": 4,
  "support_qualities": ["standard", "hd", "ultra"],
  "description": "Flux Dev 模型，适合高质量图片生成",
  "enabled": true
}
```

### 3. 配置字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `model_name` | string | 模型唯一标识 |
| `display_name` | string | 前端显示名称 |
| `provider` | string | 执行器名称（comfyui/kling/gpt4o等） |
| `api_iden` | string | API标识 |
| `credit_points` | int | 默认积分消耗 |
| `default_width` | int | 默认宽度 |
| `default_height` | int | 默认高度 |
| `support_t2i` | bool | 是否支持文生图 |
| `support_i2i` | bool | 是否支持图生图 |
| `enabled` | bool | 是否启用 |

---

## API使用

### 1. Proto定义

```protobuf
message ModelConfig {
  string model_name = 1;      // 模型名称
  int32 num_images = 2;       // 生成图片数量
  int32 steps = 3;            // 推理步数
  float cfg_scale = 4;        // CFG scale
  int64 seed = 5;             // 种子
  int32 width = 6;            // 宽度
  int32 height = 7;           // 高度
  string quality = 8;         // 图片质量
}

message SubmitPictureForgeTaskRequest {
  // Workflow模式字段
  string workflow_id = 2;
  repeated WorkflowInput workflow_input = 3;

  // 模型直连模式字段
  ModelConfig model_config = 7;
  string user_prompt = 8;
  string negative_prompt = 9;
  repeated string user_images = 10;
  float strength = 11;
}
```

### 2. 使用示例

#### 示例1：文生图（Text to Image）

```json
{
  "request_header": {
    "user_id": "user123",
    "project_id": "project456"
  },
  "model_config": {
    "model_name": "flux-dev",
    "num_images": 1,
    "steps": 28,
    "cfg_scale": 7.5,
    "width": 1024,
    "height": 1024,
    "quality": "hd"
  },
  "user_prompt": "a beautiful sunset over the ocean, 4k, highly detailed",
  "negative_prompt": "blurry, low quality"
}
```

#### 示例2：图生图（Image to Image）

```json
{
  "request_header": {
    "user_id": "user123",
    "project_id": "project456"
  },
  "model_config": {
    "model_name": "sd3",
    "num_images": 1,
    "steps": 30,
    "cfg_scale": 7.0,
    "width": 1024,
    "height": 1024
  },
  "user_prompt": "transform this photo into anime style",
  "user_images": [
    "https://example.com/input_image.jpg"
  ],
  "strength": 0.75
}
```

#### 示例3：Workflow模式（向后兼容）

```json
{
  "request_header": {
    "user_id": "user123",
    "project_id": "project456"
  },
  "workflow_id": "User_DIY_04",
  "workflow_input": [
    {
      "input_name": "prompt",
      "input_type": "TEXT",
      "input_content": "a beautiful landscape"
    }
  ]
}
```

---

## 初始化配置

### 1. 创建ModelConfigManager实例

```go
// 在服务初始化时创建
modelConfigManager, err := service.NewModelConfigManager("conf/model_config.json")
if err != nil {
    log.Fatal("failed to create model config manager:", err)
}
```

### 2. 注入到PictureForgeServer

```go
pictureForgeServer := api.NewPictureForgeServer(
    pictureForgeService,
    pictureTaskService,
    uploadService,
    subscribeService,
    dailyFreeCreditsService,
    creditService,
    configService,
    taskChainService,
    eventReporter,
    modelConfigManager, // 新增参数
)
```

### 3. 热更新配置

```go
// 支持运行时重新加载配置
err := modelConfigManager.ReloadConfig(ctx)
```

---

## 请求流程

### 模型直连模式流程

```
1. 客户端请求 (model_config + user_prompt)
   ↓
2. API层验证
   - 验证model_config.model_name是否有效
   - 验证模型是否支持所需操作（T2I/I2I）
   - 验证必填字段（user_prompt）
   ↓
3. 获取模型元数据
   - 从ModelConfigManager获取模型配置
   - 获取默认参数（width/height/steps等）
   ↓
4. 构建虚拟workflow
   - 将模型配置转换为workflow结构
   - 适配现有的workflow处理流程
   ↓
5. 计算积分消耗
   - 基于模型的credit_points计算
   - 支持会员免费策略
   ↓
6. 构建任务参数 (PrepareTaskInputsFromModelDirect)
   - 提取用户输入的prompt
   - 处理用户上传的图片（如果有）
   - 应用模型参数（steps/cfg_scale等）
   ↓
7. 创建任务 (SubmitTaskWithTx)
   - 扣除积分
   - 持久化任务记录
   - 返回task_id
   ↓
8. 任务执行
   - 执行器根据taskParams执行
   - 不关心是workflow模式还是模型直连模式
```

---

## 参数说明

### taskParams 参数映射

模型直连模式生成的 `taskParams` 包含：

| 参数名 | 来源 | 说明 |
|--------|------|------|
| `prompt` | req.user_prompt | 用户输入的prompt |
| `negative_prompt` | req.negative_prompt | 负面prompt |
| `model_name` | model_config.model_name | 模型名称 |
| `task_type` | 自动判断 | text2image/image2image |
| `num_images` | model_config.num_images | 生成图片数量 |
| `steps` | model_config.steps | 推理步数 |
| `cfg_scale` | model_config.cfg_scale | CFG scale |
| `seed` | model_config.seed | 随机种子 |
| `width` | model_config.width | 宽度 |
| `height` | model_config.height | 高度 |
| `quality` | model_config.quality | 图片质量 |
| `strength` | req.strength | 图生图强度 |
| `input_image_1` | req.user_images[0] | 输入图片URL |

---

## 积分计算

### 1. 默认积分

从模型配置的 `credit_points` 字段获取。

### 2. 会员策略

会员对图像类任务免费：

```go
if isMember && isImageWorkflow(workflowInfo) {
    creditPoints = 0
}
```

### 3. 自定义计算

可以根据不同参数调整积分：

```go
// 示例：高分辨率图片额外加分
if width > 1024 || height > 1024 {
    creditPoints *= 1.5
}
```

---

## 错误处理

### 常见错误码

| 错误码 | 说明 | 解决方案 |
|--------|------|----------|
| `INVALID_PARAM` | 参数无效 | 检查model_name是否正确 |
| `INVALID_PARAM` | 模型不支持操作 | 检查模型是否支持T2I/I2I |
| `INVALID_PARAM` | user_prompt为空 | 提供user_prompt |
| `CREDIT_POINT_NOT_ENOUGH` | 积分不足 | 充值或购买会员 |

---

## 监控和日志

### 关键日志

```go
// API层
zlog.Info("using model direct mode",
    zap.String("model_name", modelName),
    zap.Bool("is_image_to_image", hasInputImages))

// 服务层
zlog.Info("prepared task params from model direct mode",
    zap.String("model_name", modelName),
    zap.String("task_type", taskType),
    zap.String("resolution", resolution))
```

### 指标监控

建议监控以下指标：

- 模型直连模式使用率
- 各模型的使用频率
- 任务成功率（按模型分组）
- 平均执行时间（按模型分组）

---

## 扩展

### 1. 添加新模型

在 `conf/model_config.json` 中添加新配置：

```json
{
  "model_name": "new_model",
  "display_name": "New Model",
  "provider": "comfyui",
  "api_iden": "new_model_api",
  "credit_points": 15,
  "support_t2i": true,
  "support_i2i": false,
  "enabled": true
}
```

### 2. 自定义参数验证

在 `ModelConfigManager` 中添加验证逻辑：

```go
func (m *ModelConfigManager) ValidateParameters(
    modelName string,
    params map[string]interface{}) error {
    // 自定义验证逻辑
}
```

### 3. 动态调整积分

根据负载或时间动态调整积分消耗。

---

## 最佳实践

### 1. 客户端实现

- 优先使用模型直连模式（更灵活）
- 为常用场景预设参数模板
- 提供参数预览功能

### 2. 性能优化

- 对模型配置进行缓存
- 批量请求时合并参数验证
- 异步加载模型元数据

### 3. 用户体验

- 提供参数说明和推荐值
- 显示预估的积分消耗
- 支持参数保存和复用

---

## 常见问题

### Q1: 模型直连模式和Workflow模式有什么区别？

**A:**
- Workflow模式：使用预定义配置，参数固定，适合标准化场景
- 模型直连模式：用户自定义参数，更灵活，适合高级用户

### Q2: 如何选择使用哪种模式？

**A:**
- 如果需要标准化的生成效果，使用Workflow模式
- 如果需要精细控制参数，使用模型直连模式
- 两种模式可以混用

### Q3: 模型直连模式是否支持任务链？

**A:**
支持。虚拟workflow完全兼容现有的任务链机制。

### Q4: 如何热更新模型配置？

**A:**
```bash
# 方式1：修改配置文件后重启服务
# 方式2：调用热更新接口（需要实现）
curl -X POST http://api/admin/reload-model-config
```

---

## 迁移指南

### 从Workflow模式迁移到模型直连模式

```go
// 原有代码
req := &vai.SubmitPictureForgeTaskRequest{
    WorkflowId: "User_DIY_04",
    WorkflowInput: []*vai.WorkflowInput{
        {InputName: "prompt", InputContent: "beautiful landscape"},
    },
}

// 迁移后代码
req := &vai.SubmitPictureForgeTaskRequest{
    ModelConfig: &vai.ModelConfig{
        ModelName: "flux-dev",
        Steps: 28,
        Width: 1024,
        Height: 1024,
    },
    UserPrompt: "beautiful landscape",
}
```

---

## 总结

模型直连模式提供了更灵活的图片生成方式，同时完全兼容现有的Workflow模式。通过虚拟workflow的设计，确保了系统的稳定性和可维护性。
