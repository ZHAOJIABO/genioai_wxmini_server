# ListUserPictureForgeTask 接口模型直连模式兼容改造

## 修改日期
2026-02-03

## 概述
为 `ListUserPictureForgeTask` 接口添加模型直连模式支持，使其能够区分和筛选 Workflow 模式和模型直连模式的任务。

## 修改内容

### 1. Proto 定义修改

#### 新增枚举类型 (constants.proto)
```protobuf
// 任务模式
enum TaskMode {
  // 全部模式
  TASK_MODE_ALL = 0;
  // 仅Workflow模式
  TASK_MODE_WORKFLOW = 1;
  // 仅模型直连模式
  TASK_MODE_MODEL_DIRECT = 2;
}
```

#### 请求消息扩展 (picture.proto)
```protobuf
message ListUserPictureForgeTaskRequest {
  // ... 现有字段 ...
  // 任务模式筛选（可选，不传或传TASK_MODE_ALL则返回所有）
  TaskMode task_mode = 6;
}
```

#### 响应消息扩展 (picture.proto)
```protobuf
message PictureForgeInfo {
  // ... 现有字段 ...
  // 是否为模型直连模式
  bool is_model_direct = 27;
  // 模型名称（仅模型直连模式有值）
  string model_name = 28;
  // 负面prompt（仅模型直连模式有值）
  string negative_prompt = 29;
}
```

### 2. 数据库表修改

新增迁移文件: `migrations/add_model_direct_fields.sql`

为 `picture_task` 表添加字段：
- `task_mode`: TINYINT(1) - 任务模式标识 (0=workflow, 1=模型直连)
- `model_name`: VARCHAR(64) - 模型名称
- `user_prompt`: TEXT - 用户输入的prompt
- `negative_prompt`: TEXT - 负面prompt
- `idx_task_mode`: INDEX - 索引优化查询性能

### 3. Model 层修改

**文件**: `internal/model/picture_forge.go`

```go
type PictureTask struct {
    // ... 现有字段 ...
    TaskMode       int8   `json:"task_mode" ...`
    ModelName      string `json:"model_name" ...`
    UserPrompt     string `json:"user_prompt" ...`
    NegativePrompt string `json:"negative_prompt" ...`
}

const (
    TaskModeWorkflow    = 0
    TaskModeModelDirect = 1
)
```

### 4. DAO 层修改

**文件**: `internal/dao/picture_task.go`

**方法**: `GetRecentTasks`
- 新增参数: `taskMode *int8`
- 功能: 支持按任务模式筛选

```go
func (d *PictureTaskDao) GetRecentTasks(ctx context.Context, userID string, theme string, kindType string, taskMode *int8, page, pageSize int32) ([]*model.PictureTask, int64, error)
```

### 5. Service 层修改

**文件**: `internal/service/picture_task.go`

#### 方法 1: `GetRecentTasks`
- 新增参数: `taskMode *int8`
- 传递给 DAO 层

#### 方法 2: `persistTaskDetailsWithStatus`
- 从 context 提取模型直连信息
- 保存到数据库

#### 方法 3: `buildPictureForgeInfo`
- 填充模型直连字段到响应

### 6. API 层修改

**文件**: `internal/api/picture_forge.go`

#### 方法 1: `SubmitPictureForgeTask`
将模型直连信息注入到 context:
```go
if isModelDirectMode {
    ctx = context.WithValue(ctx, "is_model_direct_mode", true)
    ctx = context.WithValue(ctx, "model_name", req.GetModelConfig().GetModelName())
    ctx = context.WithValue(ctx, "user_prompt", req.GetUserPrompt())
    ctx = context.WithValue(ctx, "negative_prompt", req.GetNegativePrompt())
}
```

#### 方法 2: `ListUserPictureForgeTask`
解析并使用 `task_mode` 筛选参数:
```go
var taskMode *int8
if req.GetTaskMode() != vai.TaskMode_TASK_MODE_ALL {
    mode := int8(0)
    if req.GetTaskMode() == vai.TaskMode_TASK_MODE_MODEL_DIRECT {
        mode = 1
    }
    taskMode = &mode
}
```

#### 方法 3: `ListPictureToolsTaskResult`
传递 `nil` 作为 taskMode（不筛选）

## 部署步骤

### 1. 重新生成 Proto 代码
```bash
cd /Users/zhaojiabo/Documents/trae_projects/genioAI_server
./scripts/generate-proto.sh --clean --verify
```

### 2. 执行数据库迁移
```bash
# 连接到数据库
mysql -u<username> -p<password> <database_name>

# 执行迁移文件
source migrations/add_model_direct_fields.sql;
```

### 3. 验证编译
```bash
go build -o /tmp/va_server_test ./cmd
```

### 4. 运行测试
```bash
go test ./internal/service/...
go test ./internal/api/...
```

## 向后兼容性

✅ **完全向后兼容**

1. **数据库字段**: `task_mode` 默认值为 0 (Workflow模式)，历史数据自动兼容
2. **API 参数**: `task_mode` 为可选参数，不传默认返回所有任务
3. **响应字段**: 新字段追加在消息末尾，不影响现有客户端

## 使用示例

### 客户端调用

```protobuf
// 查询所有任务
ListUserPictureForgeTaskRequest {
  task_mode: TASK_MODE_ALL  // 或不传
}

// 仅查询 Workflow 模式任务
ListUserPictureForgeTaskRequest {
  task_mode: TASK_MODE_WORKFLOW
}

// 仅查询模型直连模式任务
ListUserPictureForgeTaskRequest {
  task_mode: TASK_MODE_MODEL_DIRECT
}
```

### 响应示例

```json
{
  "picture_forge_infos": [
    {
      "task_id": "task_xxx",
      "is_model_direct": true,
      "model_name": "flux-dev",
      "prompt": "a beautiful sunset",
      "negative_prompt": "blurry, low quality",
      // ... 其他字段
    }
  ]
}
```

## 注意事项

1. **Proto 代码生成**: 修改 proto 后必须重新生成代码
2. **数据库迁移**: 生产环境执行前建议先在测试环境验证
3. **索引优化**: `idx_task_mode` 索引用于优化按模式筛选的查询性能
4. **空值处理**: `taskMode` 为 nil 时表示不筛选，返回所有模式的任务

## 测试建议

1. **单元测试**: 测试 DAO 层的 `taskMode` 筛选逻辑
2. **集成测试**: 测试完整的请求-响应流程
3. **兼容性测试**: 验证历史数据查询正常
4. **性能测试**: 验证新增索引的查询性能

## 相关文件清单

### 修改的文件
- `pkg/va_interface/constants.proto`
- `pkg/va_interface/picture.proto`
- `internal/model/picture_forge.go`
- `internal/dao/picture_task.go`
- `internal/service/picture_task.go`
- `internal/api/picture_forge.go`

### 新增的文件
- `migrations/add_model_direct_fields.sql`
- `CHANGELOG_MODEL_DIRECT.md`

## 下一步工作

1. 执行 proto 代码生成
2. 运行数据库迁移
3. 编译测试
4. 代码审查
5. 部署到测试环境
6. 客户端适配（如需要）

---
**修改人**: Claude
**审核人**: (待填写)
**测试人**: (待填写)
