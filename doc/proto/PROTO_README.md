# Proto 代码自动化生成

## 概述

本项目通过 `buf` 工具从远程 Git 仓库直接生成 protobuf Go 代码，无需 git submodule 依赖。

## 远程仓库

所有 proto 定义存放在：`https://git.domob-inc.cn/visionai/va_interface`

## Proto 模块

项目包含三个独立的 proto 模块：

| 模块 | 远程子目录 | 本地输出目录 | Go 包路径 | buf 配置 |
|------|-----------|-------------|----------|---------|
| VA Interface | `protos/va_interface` | `internal/va_interface` | `va_visionai_server/internal/va_interface` | `buf.gen.yaml` |
| AIGC Core | `protos/aigc_core` | `pkg/aigc_core` | `va_visionai_server/pkg/aigc_core` | `buf.gen.aigc_core.yaml` |
| Event Sink | `protos/event_sink` | `pkg/event_sink` | `va_visionai_server/pkg/event_sink` | `buf.gen.event_sink.yaml` |

## Makefile 命令

### VA Interface

```bash
make proto              # 生成代码（默认 master 分支）
make proto samename_branch=true  # 使用当前 git 分支同名的远程分支
make proto-clean        # 清理生成的代码
make proto-rebuild      # 先清理再生成
make proto-verify       # 验证生成结果
```

### AIGC Core

```bash
make client             # 生成代码
make client-clean       # 清理
make client-rebuild     # 重新生成
```

### Event Sink

```bash
make event-proto        # 生成代码
make event-proto-clean  # 清理
make event-proto-rebuild # 重新生成
```

### 辅助命令

```bash
make config             # 显示所有模块配置
make check-buf          # 检查 buf 工具是否安装
make help               # 显示帮助
```

## buf 配置结构

每个模块的 `buf.gen.*.yaml` 配置结构相同：

```yaml
version: v1
managed:
  enabled: true
  go_package_prefix:
    default: <go_package_path>
plugins:
  - plugin: buf.build/protocolbuffers/go:v1.31.0
    out: <output_dir>
    opt: paths=source_relative
  - plugin: buf.build/grpc/go:v1.3.0
    out: <output_dir>
    opt: paths=source_relative
```

## 工作原理

`buf generate` 命令格式：
```bash
buf generate <repo_url>.git#branch=<branch>,subdir=<subdir> --template <config_file>
```

- 直接从远程 Git 仓库拉取指定分支和子目录的 proto 文件
- 使用对应的 buf 配置文件生成 Go 代码
- 输出到配置指定的本地目录

## 代码导入

```go
import pb "va_visionai_server/internal/va_interface"
import aigc "va_visionai_server/pkg/aigc_core"
import eventsink "va_visionai_server/pkg/event_sink"
```

## 故障排除

| 问题 | 解决方案 |
|-----|---------|
| buf 命令不存在 | `brew install bufbuild/buf/buf` |
| 无法访问远程仓库 | 检查网络和仓库权限 |
| 生成代码编译错误 | 运行 `make <module>-rebuild` 重新生成 |
