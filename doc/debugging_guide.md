# 调试指南 (Debugging Guide)

## 📋 目录

1. [VS Code 调试配置](#vs-code-调试配置)
2. [准备工作](#准备工作)
3. [启动调试](#启动调试)
4. [设置断点](#设置断点)
5. [调试新增的生图模型接口](#调试新增的生图模型接口)
6. [常见调试场景](#常见调试场景)

---

## VS Code 调试配置

已经为项目配置好了两个调试配置，位于 [.vscode/launch.json](../.vscode/launch.json)：

### 1. Debug Server
标准调试模式，适合日常开发调试。

```json
{
  "name": "Debug Server",
  "type": "go",
  "request": "launch",
  "mode": "debug",
  "program": "${workspaceFolder}/cmd",
  "cwd": "${workspaceFolder}",
  "args": ["-c", "conf/server.yaml"],
  "env": {
    "GO_ENV": "development"
  },
  "showLog": true,
  "trace": "verbose",
  "asRoot": false,
  "console": "integratedTerminal"
}
```

### 2. Debug Server (with breakpoint)
带断点启动的调试模式，适合需要从启动时就开始调试的场景。

```json
{
  "name": "Debug Server (with breakpoint)",
  "type": "go",
  "request": "launch",
  "mode": "debug",
  "program": "${workspaceFolder}/cmd",
  "cwd": "${workspaceFolder}",
  "args": ["-c", "conf/server.yaml"],
  "env": {
    "GO_ENV": "development"
  },
  "stopOnEntry": false,
  "showLog": true,
  "asRoot": false,
  "console": "integratedTerminal"
}
```

**配置说明**：
- `program`: 指向 `cmd` 目录（服务入口）
- `cwd`: 设置工作目录为项目根目录（确保能正确找到配置文件）
- `args`: 使用 `conf/server.yaml` 配置文件
- `env`: 设置为开发环境
- `showLog`: 显示调试日志
- `trace`: verbose 模式会输出详细的调试器交互信息
- `console`: 使用集成终端（便于查看输出和优雅停止）
- `asRoot`: 设为 false，不需要 root 权限

---

## 准备工作

### 1. 安装 Go 扩展

确保 VS Code 已安装 Go 扩展：
```
名称: Go
ID: golang.go
```

### 2. 配置文件检查

确认配置文件 `conf/server.yaml` 存在并正确配置：
- 数据库连接信息
- Redis 连接信息
- gRPC/HTTP 端口配置

**注意**：配置文件已存在于项目中 [conf/server.yaml](../conf/server.yaml)，无需额外创建。

### 3. 数据库迁移

在首次调试前，需要运行数据库迁移以创建 `va_image_generation_model` 表：

```bash
# 方式1: 使用 migrate 工具
migrate -path ./assets/migrations \
  -database "mysql://user:password@tcp(localhost:3306)/test_zhao" up

# 方式2: 手动执行 SQL
mysql -u user -p test_zhao < assets/migrations/20260202190205_create_image_generation_model.up.sql
```

验证表已创建：
```sql
USE test_zhao;
SHOW TABLES LIKE 'va_image_generation_model';
SELECT * FROM va_image_generation_model LIMIT 5;
```

### 4. 确认依赖安装

```bash
# 安装项目依赖
go mod download

# 安装 delve 调试器（如果还没有）
go install github.com/go-delve/delve/cmd/dlv@latest
```

---

## 启动调试

### 方法1: 使用 VS Code 调试面板

1. 打开 VS Code 侧边栏的"运行和调试"面板（快捷键：`Cmd+Shift+D` / `Ctrl+Shift+D`）
2. 在顶部下拉菜单中选择 **"Debug Server"**
3. 点击绿色的开始调试按钮（或按 `F5`）

### 方法2: 使用快捷键

1. 按 `F5` 直接启动调试（使用当前选中的配置）
2. 或者按 `Cmd+Shift+D` / `Ctrl+Shift+D` 打开调试面板后按 `F5`

### 调试器启动成功标志

当看到以下日志时，说明服务已启动：
```
[GIN-debug] Listening and serving HTTP on :8080
gRPC server listening on :50051
```

---

## 设置断点

### 基础断点设置

1. **在代码行左侧点击**：在行号左侧点击，出现红点即为断点
2. **快捷键**：将光标放在代码行，按 `F9` 切换断点
3. **条件断点**：右键点击断点 → 选择"编辑断点" → 添加条件表达式

### 断点类型示例

#### 普通断点
在 [internal/api/picture_forge.go:1533](../internal/api/picture_forge.go#L1533) 设置断点：
```go
func (s *PictureForgeServer) ListImageGenerationModels(ctx context.Context, req *vai.ListImageGenerationModelsRequest) (*vai.ListImageGenerationModelsResponse, error) {
    // ← 在这里设置断点
    projectID := common.GetProjectID(ctx)
```

#### 条件断点
只在特定项目 ID 时中断：
```go
// 断点条件: projectID == "visionai"
modelDao := dao.NewImageGenerationModelDao(db.GetDB())
```

#### 日志断点
不中断执行，只输出日志：
- 右键断点 → "编辑断点" → 勾选"日志消息"
- 输入: `查询项目 {projectID} 的模型列表`

---

## 调试新增的生图模型接口

### 场景1: 调试 ListImageGenerationModels 接口

#### 步骤1: 设置断点

在以下关键位置设置断点：

1. **API 入口** - [internal/api/picture_forge.go:1533](../internal/api/picture_forge.go#L1533)
   ```go
   func (s *PictureForgeServer) ListImageGenerationModels(...)
   ```

2. **DAO 查询** - [internal/dao/image_generation_model.go:20](../internal/dao/image_generation_model.go#L20)
   ```go
   func (d *ImageGenerationModelDao) ListEnabledModels(...)
   ```

3. **模型转换** - [internal/model/image_generation_model.go:36](../internal/model/image_generation_model.go#L36)
   ```go
   func (m *ImageGenerationModel) ToProto() *vai.ImageGenerationModelInfo
   ```

#### 步骤2: 启动调试并发送请求

启动调试后，使用 curl 发送测试请求：

```bash
curl -X POST http://localhost:8080/v1/pictureforge/list_image_generation_models \
  -H "Content-Type: application/json" \
  -d '{
    "request_header": {
      "user_id": "test_user",
      "session_id": "test_session"
    }
  }'
```

#### 步骤3: 观察调试信息

当断点触发时，可以在调试面板查看：

**变量面板**：
- `projectID`: 当前项目ID
- `models`: 从数据库查询到的模型列表
- `req`: 请求参数

**调用栈**：
```
ListImageGenerationModels (picture_forge.go:1533)
  → ListEnabledModels (image_generation_model.go:20)
    → ToProto (image_generation_model.go:36)
```

**调试控制台输入**：
```go
// 查看模型数量
len(models)

// 查看第一个模型的详细信息
models[0]

// 查看 SQL 查询（需要启用 GORM 日志）
d.db.ToSQL(...)
```

---

### 场景2: 调试 SubmitPictureForgeTask 模型查询

#### 步骤1: 设置断点

在 [internal/api/picture_forge.go:151](../internal/api/picture_forge.go#L151) 的模型查询部分设置断点：

```go
// 在这里设置断点
modelDao := dao.NewImageGenerationModelDao(db.GetDB())
dbModel, err := modelDao.GetModelByName(ctx, req.GetModelConfig().GetModelName(), projectID)
```

#### 步骤2: 发送生图任务请求

```bash
curl -X POST http://localhost:8080/v1/pictureforge/submit_task \
  -H "Content-Type: application/json" \
  -d '{
    "request_header": {
      "user_id": "test_user"
    },
    "model_config": {
      "model_name": "flux-dev"
    },
    "prompt": "A beautiful sunset"
  }'
```

#### 步骤3: 检查调试变量

- `req.GetModelConfig().GetModelName()`: 应该是 "flux-dev"
- `dbModel`: 从数据库查询到的模型元数据
- `dbModel.CreditPoints`: 该模型消耗的积分

---

### 场景3: 调试虚拟工作流列表错误

当遇到 "workflow not found" 错误时，调试 [internal/service/picture_task.go:1136](../internal/service/picture_task.go#L1136)：

#### 步骤1: 设置断点

```go
func (s *PictureTaskService) buildWorkflowInfo(ctx context.Context, workflowID string) (*pb.Workflow, error) {
    // 在这里设置断点
    if strings.HasPrefix(workflowID, "model_direct_") {
        // 虚拟工作流处理逻辑
    }
```

#### 步骤2: 观察 workflowID 格式

检查变量：
- `workflowID`: 应该类似 "model_direct_flux-dev_abc123"
- `modelName`: 解析出的模型名称（如 "flux-dev"）

#### 步骤3: 验证模型查询

单步执行到模型查询：
```go
dbModel, err := modelDao.GetModelByName(ctx, modelName, projectID)
```

检查：
- 是否成功从数据库查询到模型
- 如果失败，检查数据库中是否存在该模型

---

## 常见调试场景

### 1. 数据库查询问题

**问题**: DAO 查询返回空或错误

**调试步骤**:
1. 在 DAO 方法设置断点
2. 检查 SQL 查询条件（`projectID`, `enabled` 等）
3. 启用 GORM 日志查看实际 SQL：
   ```go
   // 在 server-local.yml 中配置
   database:
     log_level: 4  # Info 级别，会输出 SQL
   ```

4. 在调试控制台执行 SQL 验证：
   ```bash
   mysql -u user -p test_zhao -e "SELECT * FROM va_image_generation_model WHERE enabled=1 AND project_id='visionai'"
   ```

---

### 2. Proto 转换错误

**问题**: ToProto() 转换时数据丢失或类型错误

**调试步骤**:
1. 在 [internal/model/image_generation_model.go:36](../internal/model/image_generation_model.go#L36) 设置断点
2. 检查原始数据：
   ```go
   // 调试控制台
   m.SupportQualities  // JSON 字符串格式
   qualities           // 解析后的数组
   ```

3. 验证 JSON 解析：
   ```go
   // 如果解析失败，检查数据库中的值
   // 应该是: ["standard", "hd"] 格式
   ```

---

### 3. 项目隔离问题

**问题**: 不同项目看到相同的模型列表

**调试步骤**:
1. 在 API 入口检查 `projectID`:
   ```go
   projectID := common.GetProjectID(ctx)
   // 断点检查这个值是否正确
   ```

2. 检查数据库数据：
   ```sql
   SELECT DISTINCT project_id FROM va_image_generation_model;
   ```

3. 验证查询条件：
   ```go
   // 在 ListEnabledModels 断点检查
   Where("enabled = ? AND project_id = ?", true, projectID)
   ```

---

### 4. 性能问题调试

**问题**: 接口响应慢

**调试步骤**:
1. 使用 VS Code 的性能分析器：
   - 调试面板 → "CALL STACK" → 右键 → "Profile"

2. 添加时间日志：
   ```go
   import "time"

   start := time.Now()
   models, err := modelDao.ListEnabledModels(ctx, projectID)
   log.Printf("查询耗时: %v", time.Since(start))
   ```

3. 检查数据库索引：
   ```sql
   EXPLAIN SELECT * FROM va_image_generation_model
   WHERE enabled = 1 AND project_id = 'visionai'
   ORDER BY sort DESC;
   ```

---

## 调试技巧

### 1. 条件断点高级用法

只在特定模型名称时中断：
```go
// 断点条件
modelName == "flux-dev"
```

只在错误发生时中断：
```go
// 断点条件
err != nil
```

### 2. 日志点（Logpoints）

不中断执行，仅输出变量值：
```
右键断点 → 勾选"日志消息" → 输入:
查询模型 {modelName}, 项目 {projectID}
```

### 3. 使用调试控制台

在断点暂停时，可以在调试控制台执行 Go 表达式：
```go
// 查看结构体详细信息
fmt.Printf("%+v\n", models[0])

// 执行函数
len(models)

// 修改变量测试
projectID = "test-project"
```

### 4. 监视表达式

在调试面板的"监视"区域添加表达式：
```go
len(models)
models[0].ModelName
err != nil
```

---

## 测试命令集合

### 测试生图模型列表接口
```bash
# 基本请求
curl -X POST http://localhost:8080/v1/pictureforge/list_image_generation_models \
  -H "Content-Type: application/json" \
  -d '{"request_header": {"user_id": "test_user"}}'

# 格式化输出
curl -X POST http://localhost:8080/v1/pictureforge/list_image_generation_models \
  -H "Content-Type: application/json" \
  -d '{"request_header": {"user_id": "test_user"}}' | jq .
```

### 测试提交生图任务（使用数据库模型）
```bash
curl -X POST http://localhost:8080/v1/pictureforge/submit_task \
  -H "Content-Type: application/json" \
  -d '{
    "request_header": {"user_id": "test_user"},
    "model_config": {"model_name": "flux-dev"},
    "prompt": "A beautiful sunset over mountains",
    "image_config": {
      "width": 1024,
      "height": 1024,
      "num_images": 1
    }
  }' | jq .
```

### 测试任务列表（验证虚拟工作流）
```bash
curl -X POST http://localhost:8080/v1/pictureforge/list_tasks \
  -H "Content-Type: application/json" \
  -d '{
    "request_header": {"user_id": "test_user"},
    "page": 1,
    "page_size": 10
  }' | jq .
```

---

## 故障排除

### 调试器无法启动

**错误**: `could not launch process: fork/exec...`

**解决方案**:
```bash
# 重新安装 delve
go install github.com/go-delve/delve/cmd/dlv@latest

# 检查 dlv 版本
dlv version
```

### 断点不生效

**可能原因**:
1. 代码未重新编译 → 停止调试，清理缓存后重新启动
2. 编译优化导致 → 确认使用 debug 模式（launch.json 中 `mode: "debug"`）

### 调试器无法停止

**问题描述**: 点击停止按钮后，调试器没有响应或需要很长时间才能停止

**原因**:
- 服务启动了多个后台 goroutine（任务处理器、监听器等）
- 这些 goroutine 可能没有正确响应关闭信号

**解决方案**:

#### 方法1: 使用 VS Code 停止调试
1. 在调试工具栏点击"停止"按钮（红色方块）
2. 如果没有响应，再点击一次（会强制终止）

#### 方法2: 手动终止调试进程
```bash
# 查找所有调试相关进程
ps aux | grep -E "(dlv|__debug_bin)" | grep -v grep

# 强制终止所有调试进程（替换 PID 为实际进程ID）
kill -9 <PID1> <PID2> <PID3>

# 或者一键清理所有调试进程
pkill -9 dlv && pkill -9 __debug_bin
```

#### 方法3: 创建清理脚本（推荐）
在项目根目录创建 `scripts/kill_debug.sh`：
```bash
#!/bin/bash
# 清理所有调试进程
echo "正在清理调试进程..."
pkill -9 -f "dlv dap"
pkill -9 -f "__debug_bin"
pkill -9 -f "debugserver.*genioAI_server"
echo "调试进程已清理完成"
```

使用方法：
```bash
chmod +x scripts/kill_debug.sh
./scripts/kill_debug.sh
```

**预防措施**:
- 使用 `Ctrl+C` 在集成终端中优雅地停止服务
- 确保代码中正确处理 `context.Done()` 和信号中断
- 调试配置已添加 `"console": "integratedTerminal"` 使停止更可控

### 数据库连接失败

**检查清单**:
1. 确认 MySQL 服务运行：`mysql -u user -p`
2. 检查 `conf/server.yaml` 中的数据库配置
3. 验证数据库权限：`SHOW GRANTS FOR 'user'@'localhost';`

### 配置文件找不到

**错误**: `open conf/server.yaml: no such file or directory`

**原因**: 工作目录设置不正确

**解决方案**:
1. 确认 launch.json 中包含 `"cwd": "${workspaceFolder}"` 配置
2. 验证配置文件确实存在：`ls -la conf/server.yaml`
3. 如果文件不存在，从示例复制或检查配置文件路径

---

## 相关资源

- **调试配置**: [.vscode/launch.json](../.vscode/launch.json)
- **API 文档**: [doc/api_list_image_generation_models.md](./api_list_image_generation_models.md)
- **实现文档**: [doc/implementation_image_generation_models_db.md](./implementation_image_generation_models_db.md)
- **Go 调试指南**: https://github.com/golang/vscode-go/wiki/debugging
- **Delve 文档**: https://github.com/go-delve/delve

---

**最后更新**: 2026-02-02
