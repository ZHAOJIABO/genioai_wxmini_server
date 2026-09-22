# Change: 重构 Prompt 服务目录结构

## Why
当前 `internal/service/` 目录下有多个 prompt 相关文件（prompt.go、prompt_kind.go、prompt_optimizer.go、inspiration_prompt.go 等），随着功能增加，这些文件分散在根目录下不利于代码组织和维护。将其整合到独立的 `prompt/` 子目录中，提升代码可读性和模块内聚性。

## What Changes
- 创建 `internal/service/prompt/` 目录
- 移动 6 个 prompt 相关文件到新目录:
  - `prompt.go` → `prompt/prompt.go`
  - `prompt_kind.go` → `prompt/prompt_kind.go`
  - `prompt_optimizer.go` → `prompt/optimizer.go`
  - `inspiration_prompt.go` → `prompt/inspiration.go`
  - `inspiration_prompt_test.go` → `prompt/inspiration_test.go`
  - `inspiration_repository.go` → `prompt/repository.go`
- 更新 package 声明为 `package prompt`
- 更新所有上游引用的 import 路径
- 删除原有文件

## Impact
- Affected specs: 无（纯重构，不影响功能）
- Affected code:
  - `internal/service/prompt.go` (移动)
  - `internal/service/prompt_kind.go` (移动)
  - `internal/service/prompt_optimizer.go` (移动)
  - `internal/service/inspiration_prompt.go` (移动)
  - `internal/service/inspiration_prompt_test.go` (移动)
  - `internal/service/inspiration_repository.go` (移动)
  - `internal/bootstrap/service_provider.go` (import 更新)
  - `internal/bootstrap/test_service_provider.go` (import 更新)
  - `internal/api/prompt.go` (import 更新)
  - `internal/api/chat.go` (import 更新)
  - `internal/api/report.go` (import 更新)
  - `cmd/main.go` (import 更新)

## Risk Assessment
- **风险等级**: 低
- **回滚方案**: git revert
- **验证方式**: `go build ./...` && `go test ./...` && `make fmt`
