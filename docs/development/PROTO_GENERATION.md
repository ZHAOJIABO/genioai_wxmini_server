# Protocol Buffer 代码生成指南

## 🚀 快速开始

修改 `.proto` 文件后，运行以下命令重新生成代码：

```bash
# 方法 1: 使用 make 命令（推荐）
make proto-local

# 方法 2: 直接运行脚本
./scripts/generate-proto.sh
```

## 📋 可用命令

### Make 命令（推荐）

```bash
# 基本生成
make proto-local              # 从本地生成 protobuf 代码

# 清理后生成
make proto-local-clean        # 清理旧文件后重新生成

# 生成并验证
make proto-local-verify       # 生成后验证 Go 代码编译
```

### 脚本命令

```bash
# 基本用法
./scripts/generate-proto.sh

# 带参数
./scripts/generate-proto.sh --clean          # 清理后生成
./scripts/generate-proto.sh --verify         # 生成并验证
./scripts/generate-proto.sh --clean --verify # 完整流程

# 查看帮助
./scripts/generate-proto.sh --help
```

## 📂 文件路径

```
输入目录:  pkg/va_interface/*.proto
输出目录:  internal/va_interface/*.pb.go
```

## 🛠️ 工作流程

### 1. 修改 Proto 文件

```bash
# 编辑 proto 文件
vim pkg/va_interface/service.proto
```

### 2. 生成代码

```bash
# 使用 make 命令
make proto-local

# 或使用脚本（带清理和验证）
./scripts/generate-proto.sh --clean --verify
```

### 3. 验证和提交

```bash
# 编译验证
go build ./...

# 查看变更
git status
git diff

# 提交代码（包括 .proto 和 .pb.go 文件）
git add pkg/va_interface/*.proto
git add internal/va_interface/*.pb.go
git commit -m "feat: 更新 proto 定义"
```

## 📊 生成的文件类型

每个 `.proto` 文件会生成以下文件：

| 文件类型 | 说明 | 示例 |
|---------|------|------|
| `.pb.go` | Protocol Buffer 消息定义 | `service.pb.go` |
| `_grpc.pb.go` | gRPC 服务接口和客户端 | `service_grpc.pb.go` |
| `.pb.gw.go` | HTTP/REST API 网关 | `service.pb.gw.go` |

## ⚠️ 注意事项

### 1. 生成的文件不要手动修改

```bash
# ❌ 错误：手动修改生成的文件
vim internal/va_interface/service.pb.go

# ✅ 正确：修改源 proto 文件后重新生成
vim pkg/va_interface/service.proto
make proto-local
```

### 2. 提交时包含生成的文件

```bash
# ✅ 正确：同时提交 .proto 和 .pb.go
git add pkg/va_interface/service.proto
git add internal/va_interface/service*.pb*

# ❌ 错误：只提交 .proto 文件
git add pkg/va_interface/service.proto
```

### 3. Proto 文件命名规范

- 使用小写字母和下划线：`service.proto` ✅
- 避免使用驼峰命名：`ServiceProto.proto` ❌

## 🔧 环境配置

### 必需工具

```bash
# 1. Protocol Buffers 编译器
brew install protobuf

# 2. Go 插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest

# 3. 验证安装
protoc --version
protoc-gen-go --version
```

### PATH 配置

确保 `$GOPATH/bin` 在 PATH 中：

```bash
# 临时添加（当前 shell 有效）
export PATH=$PATH:$HOME/go/bin

# 永久添加（添加到 ~/.bashrc 或 ~/.zshrc）
echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.zshrc
source ~/.zshrc
```

## 🐛 常见问题

### Q: 提示找不到 protoc-gen-* 插件

```bash
# 解决方案：确保插件已安装并在 PATH 中
export PATH=$PATH:$HOME/go/bin
which protoc-gen-go
which protoc-gen-go-grpc
which protoc-gen-grpc-gateway
```

### Q: 提示找不到 google/api/annotations.proto

```bash
# 解决方案：确保 third_party 目录存在
ls pkg/va_interface/third_party/googleapis/google/api/
```

### Q: 生成后编译失败

```bash
# 1. 清理后重新生成
make proto-local-clean

# 2. 清理 Go 缓存
go clean -cache -modcache

# 3. 重新编译
go build ./...
```

### Q: 如何只生成某个特定文件？

```bash
# 使用 protoc 直接生成单个文件
export PATH=$PATH:$HOME/go/bin

protoc \
  --proto_path=pkg/va_interface \
  --proto_path=pkg/va_interface/third_party/googleapis \
  --go_out=internal/va_interface \
  --go_opt=paths=source_relative \
  --go-grpc_out=internal/va_interface \
  --go-grpc_opt=paths=source_relative \
  --grpc-gateway_out=internal/va_interface \
  --grpc-gateway_opt=paths=source_relative \
  --grpc-gateway_opt=generate_unbound_methods=true \
  pkg/va_interface/service.proto
```

## 📚 相关文档

- [脚本详细文档](scripts/README.md)
- [Protocol Buffers 官方文档](https://protobuf.dev/)
- [gRPC Go 快速开始](https://grpc.io/docs/languages/go/quickstart/)
- [grpc-gateway 文档](https://grpc-ecosystem.github.io/grpc-gateway/)

## 💡 最佳实践

### 1. 开发工作流

```bash
# 1. 修改 proto
vim pkg/va_interface/service.proto

# 2. 生成并验证
make proto-local-verify

# 3. 运行测试
go test ./...

# 4. 提交代码
git add pkg/va_interface/*.proto internal/va_interface/*.pb*
git commit -m "feat: 添加新的 RPC 方法"
```

### 2. 团队协作

- ✅ **提交生成的代码**：确保团队成员无需手动生成
- ✅ **使用统一工具**：所有人使用相同版本的 protoc 和插件
- ✅ **文档更新**：修改 proto 后更新相关文档

### 3. CI/CD 集成

```yaml
# .github/workflows/proto-check.yml
name: Proto Check
on: [pull_request]
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - name: Install tools
        run: |
          # 安装必要工具
      - name: Generate proto
        run: make proto-local-clean
      - name: Check diff
        run: |
          git diff --exit-code || (echo "Proto files out of sync!" && exit 1)
```

---

**维护者**: VisionAI Backend Team
**最后更新**: 2025-01-29
