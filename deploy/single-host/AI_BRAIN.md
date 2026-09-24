# 在现有单机 Compose 中部署 AI Brain

AI Brain 与后端在同一个 Compose 网络里。后端使用 `ai-brain:8080` 连接 gRPC；
不要在 ECS 安全组或 Compose 中公开 8080。共用现有 MySQL 容器中的 `ai_brain`
数据库，并使用独立的 `brain-redis`。部署前用 `free -h`、`df -h /` 和
`docker stats --no-stream` 确认 2 GiB ECS 有运行余量。

1. 将 AI Brain **源代码**安全推送到 GitHub，再在服务器克隆到
   `/opt/ai-brain-code`。AI Brain 仓库目前跟踪 `.env` 和 `configs/providers.yaml`；
   提交时明确选择源代码文件，不要用 `git add -A`，不要把新凭据推送到仓库。
2. 在 `/opt/genio-backend-code/deploy/single-host` 中执行：

   ```bash
   bash build-brain.sh /opt/ai-brain-code first-install
   ```

   构建脚本只复制 Go 源码与依赖清单，不把 AI Brain 的 `.env`、
   `configs/providers.yaml` 或其他凭据打进镜像。
3. 编辑已有的 `private/ai-brain.env`：填写 GPT Image 的访问令牌和 OSS
   环境变量。编辑 `private/providers.yaml`：将 `gptimage.enabled` 设为 true，
   填写真实 API endpoint 与模型名，并使两个 routing 的模型名一致。
   保留 `${GPTIMAGE_BEARER_TOKEN}` 环境变量引用。不要发送或提交这两个文件。
4. 确认 `.env` 中 `BRAIN_DB_PASSWORD` 保持首次部署时的值，然后运行：

   ```bash
   docker compose --profile ai up -d brain-redis ai-brain
   docker compose --profile ai ps
   docker compose --profile ai logs --tail=80 ai-brain
   docker stats --no-stream
   ```

5. AI Brain 容器显示 `Up` 只是启动信号。用受控模板任务验证后端到
   AI Brain 的 gRPC、GPT Image 请求、OSS 结果、失败处理和额度，再移除
   `generation-disabled.conf` 中的生图拦截规则并重载 Nginx。验证前保持 503。

不要在 AI Brain 仓库单独执行其 `docker-compose.prod.yml`，否则会另外启动
MySQL、Redis，无法直接接入此处的网络和账号。不要执行 `docker compose down -v`。
