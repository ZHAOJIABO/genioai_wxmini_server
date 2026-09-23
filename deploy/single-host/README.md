# 单机部署

当前部署阶段：从 GitHub 拉取代码，在 Alibaba Cloud Linux 3 服务器上构建后端镜像，
启动后端、MySQL 和 Redis。请按 [BACKEND_ONLY.md](BACKEND_ONLY.md) 操作。

服务器为 x86_64、2 vCPU、2 GiB RAM、40 GB 系统盘。后端默认只绑定宿主机
`127.0.0.1:8200`；数据库和 Redis 无宿主机端口。Nginx 需要 HTTPS 证书才启用。
生图提交和重试路径暂由 Nginx 返回 503，AI Brain 完成部署和业务联调后再开放。

Compose 的 `ai` profile 保留 AI Brain 与它的 Redis。将来部署 AI Brain 时，
需另行构建 `genio-ai-brain:<RELEASE_TAG>` 镜像、配置 provider 和 OSS，然后执行：

```bash
docker compose --profile ai up -d ai-brain
```

此处的 `service_started` 不代表 AI Brain gRPC 可用。联调必须覆盖真实模板任务、
进度、上传的 OSS 结果、失败和额度处理。确认后清空 generation-disabled.conf，
先执行 `docker compose --profile https exec nginx nginx -t`，通过后再 reload Nginx。

配置目录 `private/` 和 `.env` 在 Git 忽略列表中。`prepare.py` 只在首次创建
私有文件和随机密码；已有数据库卷后不要更换这些密码。数据库启动时自动建部分表，
但不会自动创建管理员、业务项目、模型与模板。备份 MySQL、Redis 和 private 目录。
不要执行 `docker compose down -v`；删除卷会删除这些数据。

`docker compose ps`、`docker stats --no-stream`、
`docker compose logs --tail=80 backend` 可用于日常检查。应用启动日志可能包含
数据库连接串，分享日志前需遮盖其中的密码。
