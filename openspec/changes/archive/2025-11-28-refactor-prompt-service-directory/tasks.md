# Tasks: 重构 Prompt 服务目录结构

## 1. 准备工作
- [x] 1.1 确认当前代码可编译通过: `go build ./...`
- [x] 1.2 确认测试通过: `go test ./...`

## 2. 创建目录并移动文件
- [x] 2.1 创建 `internal/service/prompt/` 目录
- [x] 2.2 移动 `prompt.go` → `prompt/prompt.go`，更新 package 为 `prompt`
- [x] 2.3 移动 `prompt_kind.go` → `prompt/prompt_kind.go`，更新 package 为 `prompt`
- [x] 2.4 移动 `prompt_optimizer.go` → `prompt/optimizer.go`，更新 package 为 `prompt`
- [x] 2.5 移动 `inspiration_prompt.go` → `prompt/inspiration.go`，更新 package 为 `prompt`
- [x] 2.6 移动 `inspiration_prompt_test.go` → `prompt/inspiration_test.go`，更新 package 为 `prompt`
- [x] 2.7 移动 `inspiration_repository.go` → `prompt/repository.go`，更新 package 为 `prompt`

## 3. 更新上游引用
- [x] 3.1 更新 `internal/bootstrap/service_provider.go`:
  - 添加 import `"va_visionai_server/internal/service/prompt"`
  - 替换 `service.PromptService` → `prompt.PromptService`
  - 替换 `service.PromptKindService` → `prompt.PromptKindService`
  - 替换 `service.PromptOptimizerService` → `prompt.OptimizerService`
  - 替换 `service.InspirationPromptService` → `prompt.InspirationService`
  - 替换 `service.NewPromptService` → `prompt.NewPromptService`
  - 替换 `service.NewPromptKindService` → `prompt.NewPromptKindService`
  - 替换 `service.NewPromptOptimizerService` → `prompt.NewOptimizerService`
  - 替换 `service.NewInspirationPromptServiceWithDB` → `prompt.NewInspirationServiceWithDB`
  - 替换 `service.HeatTracker` → `prompt.HeatTracker`
  - 替换 `service.NewHeatTracker` → `prompt.NewHeatTracker`
- [x] 3.2 更新 `internal/bootstrap/test_service_provider.go`:
  - 添加 import `"va_visionai_server/internal/service/prompt"`
  - 替换相关类型和函数调用
- [x] 3.3 更新 `internal/api/prompt.go`:
  - 添加 import alias `prompt "va_visionai_server/internal/service/prompt"`
  - 替换 `service.PromptService` → `prompt.PromptService`
  - 替换 `service.PromptKindService` → `prompt.PromptKindService`
  - 替换 `service.PromptOptimizerService` → `prompt.OptimizerService`
  - 替换 `service.InspirationPromptService` → `prompt.InspirationService`
- [x] 3.4 更新 `internal/api/chat.go`:
  - 添加 import `"va_visionai_server/internal/service/prompt"`
  - 替换 `service.PromptService` → `prompt.PromptService`
- [x] 3.5 更新 `internal/api/report.go`:
  - 添加 import `"va_visionai_server/internal/service/prompt"`
  - 替换 `service.InspirationPromptService` → `prompt.InspirationService`
- [x] 3.6 更新 `internal/service/chat/service.go`:
  - 添加 import `"va_visionai_server/internal/service/prompt"`
  - 替换 `service.HeatTracker` → `prompt.HeatTracker`
- [x] 3.7 更新 `cmd/main.go`:
  - 替换 `deps.InspirationPromptService` → `deps.InspirationService`

## 4. 删除旧文件
- [x] 4.1 删除 `internal/service/prompt.go`
- [x] 4.2 删除 `internal/service/prompt_kind.go`
- [x] 4.3 删除 `internal/service/prompt_optimizer.go`
- [x] 4.4 删除 `internal/service/inspiration_prompt.go`
- [x] 4.5 删除 `internal/service/inspiration_prompt_test.go`
- [x] 4.6 删除 `internal/service/inspiration_repository.go`

## 5. 验证
- [x] 5.1 运行 `make fmt` 检查代码格式
- [x] 5.2 运行 `go build ./...` 确认编译通过
- [x] 5.3 运行 `go test ./internal/service/prompt/...` 确认测试通过 (19 tests passed)
- [x] 5.4 运行 `go vet ./...` 确认无新警告（存在预先存在的警告，与本次重构无关）

## 6. 提交
- [ ] 6.1 提交更改并推送

## Notes
- 类型重命名规则:
  - `PromptOptimizerService` → `OptimizerService`（避免冗余前缀）
  - `InspirationPromptService` → `InspirationService`（避免冗余前缀）
  - `InspirationRepository` → `InspirationRepository`（保持不变，接口名）
  - `NewInspirationPromptServiceWithDB` → `NewInspirationServiceWithDB`
- 所有公开类型保持导出（首字母大写）
- 额外更新了 `internal/service/chat/service.go` 和 `cmd/main.go`（原 tasks.md 未列出但需要更新）
