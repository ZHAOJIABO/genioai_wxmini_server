# Protocol Buffer 代码生成脚本

## 概述

`generate-proto.sh` 是一个用于自动生成 Protocol Buffer 代码的脚本，它会从 `pkg/va_interface` 目录读取 `.proto` 文件，并生成对应的 Go 代码到 `internal/va_interface` 目录。

## 前置要求

在使用脚本之前，请确保已安装以下工具：

### 1. Protocol Buffers 编译器 (protoc)

```bash
# macOS
brew install protobuf

# Linux
sudo apt-get install protobuf-compiler

# 验证安装
protoc --version
```

### 2. Go 插件

```bash
# protoc-gen-go (protobuf 消息定义生成器)
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# protoc-gen-go-grpc (gRPC 服务生成器)
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# protoc-gen-grpc-gateway (HTTP/REST API 网关生成器)
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
```

## 使用方法

### 基本用法

```bash
# 直接生成代码（最常用）
./scripts/generate-proto.sh
```

### 带清理的生成

```bash
# 清理旧文件后重新生成
./scripts/generate-proto.sh --clean
```

### 带验证的生成

```bash
# 生成后验证 Go 代码是否能正常编译
./scripts/generate-proto.sh --verify

# 或使用简写
./scripts/generate-proto.sh -v
```

### 完整流程

```bash
# 清理 + 生成 + 验证（推荐用于重要更新）
./scripts/generate-proto.sh --clean --verify
```

### 查看帮助

```bash
./scripts/generate-proto.sh --help
```

## 脚本功能

### ✅ 自动检查

- 检查必要的工具是否已安装
- 检查目录结构是否正确
- 自动设置 `$GOPATH/bin` 到 PATH

### 📦 生成内容

脚本会为每个 `.proto` 文件生成三类代码：

1. **`.pb.go`** - Protocol Buffer 消息定义
2. **`_grpc.pb.go`** - gRPC 服务接口和实现
3. **`.pb.gw.go`** - HTTP/REST API 网关（grpc-gateway）

### 📊 统计信息

生成完成后会显示：
- 成功/失败的文件数量
- 各类型文件的统计
- 总计生成的文件数

### 🎨 彩色输出

- ✓ 绿色：成功
- ✗ 红色：失败
- ⚠ 黄色：警告
- ℹ 蓝色：信息

## 目录结构

```
va_visionai_server/
├── pkg/va_interface/              # 输入：.proto 文件
│   ├── service.proto
│   ├── auth.proto
│   ├── common.proto
│   └── ...
│   └── third_party/               # googleapis 依赖
│       └── googleapis/
│           └── google/api/
│               ├── annotations.proto
│               └── http.proto
├── internal/va_interface/         # 输出：生成的 Go 代码
│   ├── service.pb.go
│   ├── service_grpc.pb.go
│   ├── service.pb.gw.go
│   └── ...
└── scripts/
    ├── generate-proto.sh          # 生成脚本
    └── README.md                  # 本文档
```

## 常见问题

### Q: 生成失败，提示找不到 protoc-gen-* 插件

**A:** 确保已安装所有必要的插件，并且 `$GOPATH/bin` 在 PATH 中：

```bash
export PATH=$PATH:$HOME/go/bin
# 或将上述命令添加到 ~/.bashrc 或 ~/.zshrc
```

### Q: 生成失败，提示找不到 google/api/annotations.proto

**A:** 确保 `pkg/va_interface/third_party/googleapis` 目录存在且包含 googleapis 文件。

### Q: 如何只重新生成某个特定的 proto 文件？

**A:** 可以直接使用 protoc 命令：

```bash
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

### Q: 修改了 proto 文件后需要做什么？

**A:** 按照以下步骤操作：

1. 修改 `.proto` 文件
2. 运行生成脚本：`./scripts/generate-proto.sh`
3. 验证编译：`go build ./...`
4. 提交代码（包括 `.proto` 和生成的 `.pb.go` 文件）

## 更新日志

### 2025-01-29
- ✨ 初始版本
- ✅ 支持批量生成所有 proto 文件
- ✅ 支持清理旧文件
- ✅ 支持编译验证
- ✅ 彩色输出和详细统计

## 相关文档

- [Protocol Buffers 官方文档](https://protobuf.dev/)
- [gRPC Go 快速开始](https://grpc.io/docs/languages/go/quickstart/)
- [grpc-gateway 文档](https://grpc-ecosystem.github.io/grpc-gateway/)

## 技术支持

如有问题，请联系后端团队或提交 Issue。
