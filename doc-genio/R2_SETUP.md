# Cloudflare R2 配置指南

本项目已集成 Cloudflare R2 对象存储，享受零流量费用和全球 CDN 加速。

## 配置步骤

### 1. 创建配置文件

将示例配置复制为实际配置：

```bash
cp conf/server.yaml.example conf/server.yaml
```

### 2. 配置 Cloudflare R2

编辑 `conf/server.yaml`，填入您的 R2 凭证：

```yaml
VisionAiServerConfig:
  Region: r2  # 使用 Cloudflare R2

R2Config:
  Endpoint: https://<your-account-id>.r2.cloudflarestorage.com
  AccessKeyId: <your-r2-access-key-id>
  AccessKeySecret: <your-r2-secret-access-key>
  Bucket: <your-bucket-name>
  UgcAddr: https://<your-custom-domain>  # 或 R2 公开 URL
```

### 3. 获取 R2 凭证

1. 登录 [Cloudflare Dashboard](https://dash.cloudflare.com)
2. 导航到 **R2** → **管理 R2 API 令牌**
3. 创建 **帐户 API 令牌**（推荐生产环境使用）
4. 权限选择：**对象读和写**
5. 记录以下信息：
   - Account ID（在 R2 主页右侧）
   - Access Key ID
   - Secret Access Key

### 4. 配置自定义域名（推荐）

使用自定义域名可享受：
- ✅ 零流量费用
- ✅ 全球 CDN 加速
- ✅ 自动 HTTPS
- ✅ DDoS 防护

步骤：
1. 在 R2 存储桶页面点击 **"连接域"**
2. 输入域名（如：`cdn.yourdomain.com`）
3. Cloudflare 会自动配置 DNS（如果域名在 Cloudflare）
4. 在配置文件中使用：`UgcAddr: https://cdn.yourdomain.com`

### 5. 启用存储桶公开访问

在存储桶设置中启用公开访问，以便用户可以直接访问图片。

## 存储区域选择

项目支持三种云存储：

| Region 值 | 服务商 | 适用场景 |
|-----------|--------|---------|
| `cn` | 阿里云 OSS | 中国大陆用户 |
| `r2` | Cloudflare R2 | 国际用户（推荐） |
| 其他 | 腾讯云 COS | 亚太地区 |

## 文件存储路径

生成的图片存储格式：

```
{bucket}/{project_id}/generated/{user_id}/{task_id}.jpg      # 原图
{bucket}/{project_id}/generated/{user_id}/{task_id}-low.jpg  # JPEG缩略图
{bucket}/{project_id}/generated/{user_id}/{task_id}.webp     # WebP缩略图
```

## 支持的图片生成功能

- ✅ ComfyUI 工作流
- ✅ Gemini 文生图
- ✅ Gemini 图生图
- ✅ 用户上传图片

## 安全提示

⚠️ **重要**：`conf/server.yaml` 包含敏感信息，已添加到 `.gitignore`。

- ✅ 不要将真实配置提交到 Git
- ✅ 在生产环境使用环境变量或密钥管理服务
- ✅ 定期轮换 API 密钥

## 测试上传

运行测试脚本验证配置：

```bash
# 编辑 test_r2_upload.go 填入凭证
go run test_r2_upload.go
```

## 成本估算

Cloudflare R2 定价：
- 存储：$0.015/GB/月
- Class A 操作（写入）：$4.50/百万次
- Class B 操作（读取）：$0.36/百万次
- **出站流量：完全免费** 🎉

## 技术支持

- [Cloudflare R2 文档](https://developers.cloudflare.com/r2/)
- [项目 Issue](../../issues)

## 更新日志

### 2026-01-26
- ✅ 添加 Cloudflare R2 支持
- ✅ 实现 Gemini 图片上传到 R2
- ✅ 配置自定义域名 CDN 加速
- ✅ 零流量费用优化
