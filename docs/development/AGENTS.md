<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always open `@/openspec/AGENTS.md` when the request:
- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

Use `@/openspec/AGENTS.md` to learn:
- How to create and apply change proposals
- Spec format and conventions
- Project structure and guidelines

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

# Repository Guidelines

This guide helps contributors work effectively with va_visionai_server. Keep changes focused and consistent with existing patterns.
Reply use chinese,this is important.
Run `make fmt` for check code.and fix question.
If technical planning and modifications involve certain third-party libraries, call context7 mcpserver to access detailed documentation for the corresponding libraries.

## Project Structure & Module Organization
- Code root: Go module `va_visionai_server`.
- Entrypoints: `cmd/main.go` (server), aux clients in `cmd/*.go`.
- Core packages: `internal/api` (gRPC services), `internal/bootstrap` (DI/init), `internal/dao` (DB access), `internal/service` (business logic), `internal/task` (background jobs), `internal/db` (DB/redis/migrations), `internal/zlog` (logging).
- Protocol: see [Proto 文档](doc/proto/PROTO_README.md) for generated stubs locations and commands.
- Config & assets: `conf/*` (YAML/JSON), migrations in `assets/migrations/*`.

## Build, Test, and Development Commands
- `make fmt`: Format and lint (go fmt + golangci-lint --fix).
- Proto commands: see [Proto 文档](doc/proto/PROTO_README.md).
- Run server locally: `go run ./cmd -c conf/server.yaml` or build with `go build -o build/server ./cmd && ./build/server -c conf/server.yaml`.
- Docker (CI example): see `.gitlab-ci.yml` for `docker build` using `Dockerfile-tpl`.

## Coding Style & Naming Conventions
- Go 1.23+; use standard Go formatting. Run `make fmt` before pushing.
- Packages: lower_snake or single word (e.g., `dao`, `service`).
- Files: keep `_test.go` for tests; protobuf outputs live under designated generated dirs.
- Errors: wrap with context using `fmt.Errorf("...: %w", err)`.

## Testing Guidelines
- Framework: Go test + `testify/assert` and `testify/require` for assertions only (no mocking framework).
- Run all tests: `go test ./...` (add `-v` if needed).
- Naming: place tests alongside code as `*_test.go`; table-driven where practical.
- Coverage: prefer >70% on new/changed business logic; include integration tests for API/service paths when feasible.

### Unit Testing Best Practices

Repository Pattern 实现规则:
- 一般在 Service（或 Domain）层定义 Repository 接口，接口应放在“使用方”而非实现方。
- DAO 层的具体类型直接实现 Repository 接口。
- 原则上不额外创建仅做方法转发的适配器层，除非为兼容旧接口或对接第三方库。
- Service 必须提供两个构造函数：
  - `NewService(repo)`：主构造函数，接收 Repository 接口，用于单元测试和生产。
  - `NewServiceWithDB(db)`：生产环境便捷构造，内部创建 DAO 并调用 `NewService(repo)`。

测试实现规则:
- 默认使用手写 Fake 实现，使用内存存储（map/slice）模拟数据。
- Fake 中应提供错误注入字段，用于控制不同测试场景。
- 建议将 Fake 实现放在测试文件开头，便于阅读。
- 按功能和场景对测试进行分组，推荐使用子测试 `t.Run`。

覆盖率要求:
- Model 层：要求 100% 覆盖。
- Service 层：要求 ≥85% 覆盖，需覆盖主要业务分支与错误处理逻辑。
- DAO 层：通过集成测试验证（连接真实数据库或测试容器），覆盖核心 CRUD 与关键查询。

测试命名与场景:
- 测试函数命名格式：`Test[Function]_[Scenario]`，例如 `TestCreateUser_Success`。
- 对每个核心函数，至少覆盖：
  - 成功路径（正常输入）。
  - 错误场景（含通过 Fake 注入的错误）。
  - 若存在时间依赖逻辑，需覆盖关键时间边界。
  - 若存在随机性逻辑，需通过可控随机源（可注入 rand）验证行为。

覆盖率验证命令示例:
go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out

## Commit & Pull Request Guidelines
- Messages: conventional prefixes `feat:`, `fix:`, `chore:`, `refactor:`, etc. Scope and imperative summary (<=72 chars).
- PRs: clear description, linked issues, what/why, migration notes, and screenshots/logs for behavior changes.
- Checks: pass `make fmt`, `go build ./...`, and `go test ./...` locally.

## Security & Configuration Tips
- Secrets live outside the repo; use `conf/server.yaml` and environment variables via `viper`.
- DB migrations directory is `assets/migrations`; ensure MySQL/Redis configs match your environment before running the server.

## Database Migration Guidelines
###  GORM AutoMigrate
- **位置**: `internal/db/mysql.go` 的 `mysqldb.AutoMigrate()`
- **用途**: 新功能开发时快速创建表结构
- **限制**: 不能删除字段/表、不能修改索引、无法回滚
- 不需要在结构体额外声明tableName mysql 初始化文件通过gorm设置表前缀 

## Architecture Overview
- gRPC server registers services from `internal/api` with dependencies wired via `internal/bootstrap.ServiceProvider`.
- Background processors (e.g., picture workflow, credit expiry) run from `cmd/main.go` using `internal/task/*` services.

## EventReporter 事件上报
- 异步事件上报客户端，用于发送业务事件到 event_sink 服务
- 位置：`internal/service/event_reporter/`
- 详见 [EventReporter 文档](doc/event_reporter/README.md)
  - [技术设计](doc/event_reporter/DESIGN.md) - 架构设计和实现细节
  - [使用示例](doc/event_reporter/EXAMPLES.md) - 代码使用示例

## MCP RULE
- 当用户输入提到tapd查询等关键字时，正确的使用mcp工具： mcp-server-tapd 查询，并在对应的任务修复完成后将状态正确的进行流转
- 每次进行代码修完成后 或 做规划时遇见有争议模糊不清时，积极调用mcp工具：mcp-feedback-enhanced来获取反馈
- 代码修改或规划时有使用第三方依赖库，积极调用mcp工具：context7 查询对应库的文档和使用方法。


## GRPC Proto
- Proto 定义在远程仓库，不要直接修改 `*.pb.go` 生成文件
- 如有 task 或 TODO 涉及 proto 协议修改请跳过
- 详见 [Proto 文档](doc/proto/PROTO_README.md)
