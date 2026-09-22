# Makefile for VA Interface Service

# Proto 配置
PROTO_REPO_URL :=
# 默认为 master 分支。如果需要使用当前 git 分支，请在 make 命令中传入 samename_branch=true
# 例如: make proto samename_branch=true
ifdef samename_branch
PROTO_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
else
PROTO_BRANCH := master
endif
PROTO_SUBDIR := protos/va_interface
PROTO_OUTPUT_DIR := internal/va_interface

# AIGC Core client 配置
AIGC_CORE_REPO_URL :=https://git.domob-inc.cn/visionai/va_interface
AIGC_CORE_BRANCH := master
AIGC_CORE_SUBDIR := protos/aigc_core
AIGC_CORE_OUTPUT_DIR := pkg/aigc_core
AIGC_CORE_BUF_CONFIG := buf.gen.aigc_core.yaml

# Events Sink 配置
EVENT_SINK_REPO_URL :=https://git.domob-inc.cn/visionai/va_interface
EVENT_SINK_BRANCH := master
EVENT_SINK_SUBDIR := protos/event_sink
EVENT_SINK_OUTPUT_DIR := pkg/event_sink
EVENT_SINK_BUF_CONFIG := buf.gen.event_sink.yaml

.PHONY: help proto proto-clean proto-verify proto-rebuild proto-local proto-local-clean proto-local-verify client client-clean client-rebuild event-proto event-proto-clean event-proto-rebuild

# 帮助信息
help:
	@echo "可用的命令:"
	@echo "  fmt               - 格式化代码"
	@echo "  run               - 启动服务，使用 conf/server-local.yaml 配置文件"
	@echo "  proto-local       - 从本地 pkg/va_interface 生成 protobuf 代码（推荐）"
	@echo "  proto-local-clean - 清理后重新生成本地 protobuf 代码"
	@echo "  proto-local-verify- 生成本地 protobuf 代码并验证编译"
	@echo "  proto             - 从远程仓库生成 VA Interface protobuf Go 代码，命令中传入 samename_branch=true,使用远程仓库同名分支"
	@echo "  proto-clean       - 清理生成的 protobuf 代码"
	@echo "  proto-verify      - 验证生成的代码是否正确"
	@echo "  proto-rebuild     - 重新生成代码 (先清理再生成)"
	@echo "  client            - 生成 AIGC Core 服务端代码"
	@echo "  client-clean - 清理生成的 AIGC Core 服务端代码"
	@echo "  client-rebuild - 重新生成 AIGC Core 服务端代码 (先清理再生成)"
	@echo "  event-proto  - 生成 Events Sink protobuf Go 代码"
	@echo "  event-proto-clean - 清理生成的 Events Sink protobuf 代码"
	@echo "  event-proto-rebuild - 重新生成 Events Sink 代码 (先清理再生成)"
	@echo "  help         - 显示此帮助信息"

fmt:
	@echo "格式化代码..."
	go fmt ./...
	golangci-lint run --fix
	golangci-lint run

# 启动服务，使用本地配置文件
run:
	@echo "正在启动服务..."
	go run ./cmd -c conf/server-local.yaml

# 从远程 Proto 仓库生成 VA Interface 代码
proto:
	@echo "正在从远程仓库生成 VA Interface protobuf 代码..."
	@echo "仓库地址: $(PROTO_REPO_URL)"
	@echo "分支: $(PROTO_BRANCH)"
	@echo "子目录: $(PROTO_SUBDIR)"
	@mkdir -p $(PROTO_OUTPUT_DIR)
	buf generate $(PROTO_REPO_URL).git#branch=$(PROTO_BRANCH),subdir=$(PROTO_SUBDIR)
	@echo "✅ VA Interface protobuf 代码生成完成!"

# AIGC Core client 代码生成
client:
	@echo "正在生成 AIGC Core 服务端代码..."
	@echo "仓库地址: $(AIGC_CORE_REPO_URL)"
	@echo "分支: $(AIGC_CORE_BRANCH)"
	@echo "子目录: $(AIGC_CORE_SUBDIR)"
	@mkdir -p $(AIGC_CORE_OUTPUT_DIR)
	buf generate $(AIGC_CORE_REPO_URL).git#branch=$(AIGC_CORE_BRANCH),subdir=$(AIGC_CORE_SUBDIR) --template $(AIGC_CORE_BUF_CONFIG)
	@echo "✅ AIGC Core 服务端代码生成完成!"

# Events Sink protobuf 代码生成
event-proto:
	@echo "正在生成 Events Sink protobuf 代码..."
	@echo "仓库地址: $(EVENT_SINK_REPO_URL)"
	@echo "分支: $(EVENT_SINK_BRANCH)"
	@echo "子目录: $(EVENT_SINK_SUBDIR)"
	@mkdir -p $(EVENT_SINK_OUTPUT_DIR)
	buf generate $(EVENT_SINK_REPO_URL).git#branch=$(EVENT_SINK_BRANCH),subdir=$(EVENT_SINK_SUBDIR) --template $(EVENT_SINK_BUF_CONFIG)
	@echo "✅ Events Sink protobuf 代码生成完成!"

# 清理生成的代码
proto-clean:
	@echo "正在清理生成的 protobuf 代码..."
	rm -rf $(PROTO_OUTPUT_DIR)
	@echo "✅ 清理完成!"

# 清理生成的 AIGC Core client 代码
client-clean:
	@echo "正在清理生成的 AIGC Core 服务端代码..."
	rm -rf $(AIGC_CORE_OUTPUT_DIR)
	@echo "✅ 清理完成!"

# 清理生成的 Events Sink 代码
event-proto-clean:
	@echo "正在清理生成的 Events Sink protobuf 代码..."
	rm -rf $(EVENT_SINK_OUTPUT_DIR)
	@echo "✅ 清理完成!"

# 验证生成的代码
proto-verify:
	@echo "正在验证生成的 protobuf 代码..."
	@echo "✅ protobuf 代码验证成功!"
	@echo "生成的文件列表:"
	@find $(PROTO_OUTPUT_DIR) -name "*.pb.go" | wc -l | xargs printf "总共生成了 %s 个 .pb.go 文件\n"
	@find $(PROTO_OUTPUT_DIR) -name "*.pb.go"

# 重新生成代码 (先清理再生成)
proto-rebuild: proto-clean proto proto-verify

# 重新生成 AIGC Core client 代码 (先清理再生成)
client-rebuild: client-clean client

# 重新生成 Events Sink 代码 (先清理再生成)
event-proto-rebuild: event-proto-clean event-proto

# 检查 buf 工具是否已安装
check-buf:
	@which buf > /dev/null || (echo "❌ 错误: buf 工具未安装，请访问 https://docs.buf.build/installation 进行安装" && exit 1)
	@echo "✅ buf 工具已安装"

# 显示当前配置
config:
	@echo "当前配置:"
	@echo "  仓库地址: $(PROTO_REPO_URL)"
	@echo "  分支: $(PROTO_BRANCH)"
	@echo "  子目录: $(PROTO_SUBDIR)"
	@echo "  输出目录: $(PROTO_OUTPUT_DIR)"
	@echo "---"
	@echo "AIGC Core client 配置:"
	@echo "  仓库地址: $(AIGC_CORE_REPO_URL)"
	@echo "  分支: $(AIGC_CORE_BRANCH)"
	@echo "  子目录: $(AIGC_CORE_SUBDIR)"
	@echo "  输出目录: $(AIGC_CORE_OUTPUT_DIR)"
	@echo "---"
	@echo "Events Sink 配置:"
	@echo "  仓库地址: $(EVENT_SINK_REPO_URL)"
	@echo "  分支: $(EVENT_SINK_BRANCH)"
	@echo "  子目录: $(EVENT_SINK_SUBDIR)"
	@echo "  输出目录: $(EVENT_SINK_OUTPUT_DIR)"

# 全新安装 (检查环境 + 生成代码)
install: check-buf proto-rebuild
	@echo "🎉 VA Interface 服务代码生成完成!"

# 本地 protobuf 代码生成（使用 scripts/generate-proto.sh）
proto-local:
	@echo "正在从本地 pkg/va_interface 生成 protobuf 代码..."
	@bash scripts/generate-proto.sh

# 清理后重新生成本地 protobuf 代码
proto-local-clean:
	@echo "清理并重新生成本地 protobuf 代码..."
	@bash scripts/generate-proto.sh --clean

# 生成本地 protobuf 代码并验证编译
proto-local-verify:
	@echo "生成本地 protobuf 代码并验证编译..."
	@bash scripts/generate-proto.sh --verify
