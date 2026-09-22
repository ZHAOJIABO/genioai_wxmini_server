# GenioAI Backend 文档中心

完整的部署、配置和开发文档。

## 📂 文档分类

### 🚀 配置文档 (Configuration)
新手入门和环境配置相关文档。

- **[快速开始指南](configuration/QUICKSTART.md)**
  - 5 分钟快速部署
  - 适合首次使用

- **[环境配置指南](configuration/ENV_CONFIG_GUIDE.md)**
  - 详细的环境变量配置
  - 数据库、Redis、AI Brain 连接配置

---

### 🏗️ 部署文档 (Deployment)
生产环境和开发环境的部署方案。

#### 主要部署指南

- **[部署方式对比](deployment/DEPLOYMENT_COMPARISON.md)** ⭐
  - 帮助你选择合适的部署方式
  - 传统部署 vs 零停机部署

- **[后端部署指南](deployment/BACKEND_DEPLOYMENT_GUIDE.md)**
  - 传统部署方式（有 1-2 分钟停机）
  - 适合开发/测试环境
  - 使用 `deploy.sh` 脚本

- **[零停机部署指南](deployment/ZERO_DOWNTIME_DEPLOYMENT_GUIDE.md)**
  - 最小停机部署（5-10 秒停机）
  - 适合生产环境
  - 使用 `deploy-zero-downtime.sh` 脚本

- **[部署脚本使用指南](deployment/DEPLOY_SCRIPT_GUIDE.md)**
  - 部署脚本详细说明
  - 参数和选项说明

- **[服务器监控命令](deployment/SERVER_COMMANDS.md)** ⭐
  - 常用运维命令速查
  - Docker、日志、资源监控
  - 故障排查流程

#### 高级部署方案

- **[分离部署指南](deployment/SEPARATE_DEPLOYMENT_GUIDE.md)**
  - Backend 和 AI Brain 分离部署
  - 微服务架构部署

- **[AI Brain 部署指南](deployment/AI_BRAIN_DEPLOYMENT_GUIDE.md)**
  - AI Brain 服务独立部署
  - 与 Backend 集成配置

- **[日本服务器部署计划](deployment/DEPLOYMENT_PLAN_JAPAN.md)**
  - 日本区域部署方案
  - 跨区域部署架构

---

### 🧪 测试文档 (Testing)
API 测试和质量保证相关文档。

- **[Apifox 测试指南](testing/APIFOX_TEST_GUIDE.md)**
  - API 测试工具使用
  - 测试用例编写

---

### 📝 更新日志 (Changelog)
系统更新、变更记录和问题修复。

- **[更新指南](changelog/UPDATE_GUIDE.md)**
  - 系统更新流程
  - 版本升级步骤

- **[模型直连模式更新日志](changelog/CHANGELOG_MODEL_DIRECT.md)**
  - 模型直连功能变更
  - 新功能说明

- **[AIGC 重命名说明](changelog/RENAME_AIGC_TO_AIBRAIN.md)**
  - AIGCServer → AIBrainServer 变更
  - 配置迁移指南

- **[Gemini Provider 修复说明](changelog/GEMINI_PROVIDER_FIX.md)**
  - Gemini 提供商问题修复
  - 已知问题和解决方案

---

### 👨‍💻 开发文档 (Development)
代码结构、架构设计和开发指南。

- **[代理服务说明](development/AGENTS.md)**
  - 系统代理服务架构
  - 服务间通信

- **[代码库分析](development/CODEBASE_ANALYSIS.md)**
  - 代码结构分析
  - 模块职责划分

- **[Proto 生成指南](development/PROTO_GENERATION.md)**
  - gRPC Proto 文件生成
  - Protocol Buffers 使用

---

## 🎯 推荐阅读顺序

### 首次部署
1. [快速开始指南](configuration/QUICKSTART.md)
2. [环境配置指南](configuration/ENV_CONFIG_GUIDE.md)
3. [后端部署指南](deployment/BACKEND_DEPLOYMENT_GUIDE.md)

### 生产环境
1. [部署方式对比](deployment/DEPLOYMENT_COMPARISON.md)
2. [零停机部署指南](deployment/ZERO_DOWNTIME_DEPLOYMENT_GUIDE.md)
3. [更新指南](changelog/UPDATE_GUIDE.md)

### 开发人员
1. [代码库分析](development/CODEBASE_ANALYSIS.md)
2. [Proto 生成指南](development/PROTO_GENERATION.md)
3. [Apifox 测试指南](testing/APIFOX_TEST_GUIDE.md)

---

## 🔍 快速查找

### 我想...

**部署系统**
- 首次部署 → [快速开始指南](configuration/QUICKSTART.md)
- 生产部署 → [零停机部署指南](deployment/ZERO_DOWNTIME_DEPLOYMENT_GUIDE.md)
- 选择部署方式 → [部署方式对比](deployment/DEPLOYMENT_COMPARISON.md)

**配置系统**
- 配置数据库 → [环境配置指南](configuration/ENV_CONFIG_GUIDE.md)
- 配置 AI Brain → [AI Brain 部署指南](deployment/AI_BRAIN_DEPLOYMENT_GUIDE.md)

**更新系统**
- 升级版本 → [更新指南](changelog/UPDATE_GUIDE.md)
- 查看变更 → [更新日志目录](changelog/)

**开发调试**
- 理解代码 → [代码库分析](development/CODEBASE_ANALYSIS.md)
- 测试 API → [Apifox 测试指南](testing/APIFOX_TEST_GUIDE.md)

---

## 📞 获取帮助

- 查看对应文档的故障排查部分
- 提交 GitHub Issue
- 联系技术支持

---

## 📄 文档贡献

欢迎改进文档！贡献指南：

1. Fork 项目
2. 创建文档分支
3. 编写/修改文档
4. 提交 Pull Request

文档编写规范：
- 使用清晰的标题层级
- 提供代码示例
- 包含故障排查部分
- 添加相关文档链接
