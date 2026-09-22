# GenioAI Backend Server

GenioAI 后端服务，提供 AI 图片生成和管理功能。

## 📚 文档导航

### 🚀 快速开始
- [快速开始指南](docs/configuration/QUICKSTART.md) - 5分钟快速部署指南
- [环境配置指南](docs/configuration/ENV_CONFIG_GUIDE.md) - 环境变量配置详解

### 🏗️ 部署文档
- [后端部署指南](docs/deployment/BACKEND_DEPLOYMENT_GUIDE.md) - 传统部署方式（有停机）
- [零停机部署指南](docs/deployment/ZERO_DOWNTIME_DEPLOYMENT_GUIDE.md) - 最小停机部署方案
- [部署方式对比](docs/deployment/DEPLOYMENT_COMPARISON.md) - 帮助你选择合适的部署方式
- [部署脚本使用指南](docs/deployment/DEPLOY_SCRIPT_GUIDE.md) - 部署脚本详细说明
- [服务器监控命令](docs/deployment/SERVER_COMMANDS.md) - 常用运维命令速查手册
- [分离部署指南](docs/deployment/SEPARATE_DEPLOYMENT_GUIDE.md) - Backend 和 AI Brain 分离部署

### 🧪 测试文档
- [Apifox 测试指南](docs/testing/APIFOX_TEST_GUIDE.md) - API 测试工具使用指南

### 📝 更新日志
- [更新指南](docs/changelog/UPDATE_GUIDE.md) - 系统更新和升级指南
- [模型直连模式更新日志](docs/changelog/CHANGELOG_MODEL_DIRECT.md) - 模型直连功能变更记录
- [Gemini Provider 修复说明](docs/changelog/GEMINI_PROVIDER_FIX.md) - Gemini 提供商问题修复

### 👨‍💻 开发文档
- [代理服务说明](docs/development/AGENTS.md) - 系统代理服务架构
- [代码库分析](docs/development/CODEBASE_ANALYSIS.md) - 代码结构分析
- [Proto 生成指南](docs/development/PROTO_GENERATION.md) - gRPC Proto 文件生成

## 🎯 核心功能

- AI 图片生成（支持多种模型）
- 用户管理和认证
- 任务队列管理
- 图片存储和管理
- gRPC 和 HTTP 双协议支持
- Metrics 监控

## 🛠️ 技术栈

- **语言**: Go
- **框架**: Kratos (微服务框架)
- **数据库**: MySQL, Redis
- **协议**: gRPC, HTTP
- **容器化**: Docker, Docker Compose
- **监控**: Prometheus Metrics

## 📦 快速部署

### 传统部署（适合开发/测试）
```bash
# 一键部署（约1-2分钟停机）
./deploy.sh
```

### 最小停机部署（适合生产环境）
```bash
# 最小停机部署（约5-10秒停机）
./deploy-zero-downtime.sh
```

详细对比请查看 [部署方式对比](docs/deployment/DEPLOYMENT_COMPARISON.md)

## 🔧 配置

主要配置文件：
- `.env` - 环境变量配置（数据库、Redis 等）
- `conf/server.yaml` - 服务器配置
- `docker-compose.yml` - Docker 编排配置

配置模板：
- [.env.example](.env.example) - 环境变量示例
- `conf/server.yaml.example` - 配置文件示例

## 📡 API 端口

- **HTTP API**: 8200
- **gRPC**: 8181
- **Metrics**: 9188

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

[添加你的许可证信息]
