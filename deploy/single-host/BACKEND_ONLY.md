# 第一阶段：仅部署后端

当前只启动 backend、mysql、backend-redis；有证书后再启用 nginx。
AI Brain 和它的 Redis 在 `ai` profile 中，不会被默认启动。
MySQL 预建 AI Brain 的空数据库和账号以备后续，不会运行 AI Brain。

代码检查：AIGC 客户端使用非阻塞 gRPC 连接，没有等待 AI Brain 就绪的逻辑。
服务器完整启动仍待验证，AI Brain 不在线时可能有连接失败日志。
Nginx 拦截五个现有生图/重试提交接口（503），不让请求进入任务和额度处理。
本机 8200 和容器内调用不经过 Nginx，不要从这些入口提交生图任务。
这不是完整的后端功能开关，小程序暂不正式发布。

## 1. 在服务器拉取 GitHub 代码

已安装 Docker 和 Compose。下列命令在服务器 root 终端运行，使用新的
`/opt/genio-backend-code` 目录，避免覆盖之前可能存在的 `/opt/genio-backend`。
如果 GitHub 仓库是私有的，先给服务器配置只读 Deploy Key，然后把下面
HTTPS 地址改成 `git@github.com:ZHAOJIABO/genioai_wxmini_server.git`。

```bash
dnf install -y git python3
git clone https://github.com/ZHAOJIABO/genioai_wxmini_server.git /opt/genio-backend-code
cd /opt/genio-backend-code/deploy/single-host
python3 prepare.py
vi private/server.yaml
```

prepare.py 如提示配置已存在，保留现有文件、跳过生成，勿删除重建密码。
填写 WeChatMiniPrograms 的 ProjectID/AppID/AppSecret，以及 OssConfig 的新 AccessKey。
ProjectID 必须与小程序和业务项目一致。OSS 北京 endpoint、Bucket 和图片地址已经填写。
数据库连接已自动配置。暂时不用填写 private/ai-brain.env 或 providers.yaml。
不要把含密钥的文件发到对话。`.env` 的 RELEASE_TAG 保持 first-install。

## 2. 增加构建缓冲内存并启动

这台 ECS 只有 2 GiB RAM，当前无 Swap；Go 编译时可能内存不足。
40 GB 系统盘当前剩余约 33 GB，可先建 4 GB Swap：

```bash
fallocate -l 4G /swapfile
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile
free -h
```

若 `/swapfile` 已存在，先检查 `swapon --show`，不要覆盖已启用的文件。
需要重启后保留时，再把 `/swapfile none swap sw 0 0` 写入 `/etc/fstab`。
Swap 只是构建缓冲，不代表应用运行内存足够；建好后检查磁盘和内存。

```bash
docker compose config --quiet
docker compose --progress plain build backend
docker compose up -d mysql backend-redis backend
docker compose ps
docker stats --no-stream
curl -fsS http://127.0.0.1:8200/metrics -o /dev/null && echo 'HTTP OK'
```

镜像均从 Amazon ECR Public 上的 Docker 官方镜像仓库直接拉取；不依赖 Docker Hub 加速器。
Go 1.24 构建镜像基于 Debian 13（trixie），运行镜像也使用 trixie，避免 CGO/glibc 不兼容。
Debian 软件包安装使用阿里云镜像站。若某个 ECR 镜像拉取失败，保留完整错误信息，
先处理该镜像的网络或标签问题，不要反复重试编译。

第一次建库需等待健康检查。HTTP OK 只证明 HTTP 路由可访问。
失败时用 docker compose logs --tail=80 backend 查看原因，日志可能含 DSN，分享前遮盖密码。
自动建表不会自动创建项目、后台管理员或模板，需要后续初始化，不能只凭容器 running 判定完成。
不要执行 docker compose down -v。

## 3. HTTPS

先在 DNS 中添加 `geniomini.appbobo.cn` 的 A 记录指向 `60.205.114.31`，保留根域名原记录。
确认证书签发前该子域名已解析到新服务器，并在安全组开放 TCP 80、443。
准备覆盖 `geniomini.appbobo.cn` 的证书，放到服务器：

```text
/opt/genio-backend-code/deploy/single-host/private/tls/fullchain.pem
/opt/genio-backend-code/deploy/single-host/private/tls/privkey.pem
```

安全组开放 TCP 80、443，保留管理用 SSH，不开放数据库、8181、8200、9188。

```bash
chmod 600 private/tls/privkey.pem
docker compose --profile https run --rm --no-deps nginx nginx -t
docker compose --profile https up -d nginx
```

standalone 模式的续期也需要暂时占用 80 端口。首次启动 Nginx 后，将仓库中的
`certbot-pre.sh`、`certbot-deploy.sh`、`certbot-post.sh` 分别链接到
`/etc/letsencrypt/renewal-hooks/pre/`、`deploy/`、`post/`，使 Certbot 自动续期时
先暂停 Nginx，成功后复制新证书，最后恢复 Nginx。续期时 HTTP/HTTPS 会短暂中断。
用 `certbot renew --dry-run` 验证续期链路，结束后再次检查 `docker compose ps`。

用 curl -I https://geniomini.appbobo.cn/admin/ 验证证书和 Nginx（预期 403），
再用本机 http://127.0.0.1:8200/metrics 验证后端（预期 200）。
公网管理入口与 metrics 被 Nginx 禁止访问；需要管理后台时通过 SSH 端口转发访问本机 8200。
检查是否有指向旧服务器的 AAAA 记录。
无证书时先停在第 4 步，暂不配置小程序公网入口。

```bash
curl -i -X POST https://geniomini.appbobo.cn/v1/pictureforge/submit_picture_forge_task
```

预期 HTTP 503，不应创建任务。再验证微信登录、模板管理和 OSS 上传。

## 第二阶段

AI Brain 准备好后启动 docker compose --profile ai up -d ai-brain。
通过受控入口验证生图和额度，再清空 generation-disabled.conf，并执行
docker compose --profile https exec nginx nginx -t；成功后执行
docker compose --profile https exec nginx nginx -s reload。
