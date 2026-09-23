# 微信小程序身份登录

前端开发请参阅 [微信小程序前端对接文档](./wechat-miniprogram-frontend-integration.md)。

接口：`POST /v1/auth/wechat_auth`，gRPC：`AuthService.WeChatAuth`。

小程序通过 `wx.login()` 获取一次性 code，后端请求微信 code2Session，使用经微信验证的 OpenID 查找或创建用户，并返回本系统的 Access Token / Refresh Token。

## 服务端配置

在实际启动使用的 YAML 配置中加入以下内容，替换示例项目标识和凭证后重启服务：

```yaml
WeChatMiniPrograms:
  - ProjectID: "com.example.mini"
    AppID: "wx你的真实小程序AppID"
    AppSecret: "你的真实小程序AppSecret"
```

项目标识必须为小写，与请求头 `app.package_name` 一致；也兼容现有 `web_client.package_name`。每个项目显式配置自己的凭证，不支持 `default` 项目回退。未配置此节时不启用微信登录，不影响其他登录方式；配置缺少凭证时启动报错。不要将真实 AppSecret 提交到代码仓库或下发客户端。

在微信小程序管理后台将业务后端 HTTPS 域名加入 request 合法域名。服务端需能访问 `https://api.weixin.qq.com`。code2Session 本身不需要微信平台 access_token；校验和重置接口会自动获取并缓存稳定版平台 access_token。若微信返回 40164，需在微信后台配置服务端出口 IP 白名单。

## 请求与响应

以下为逻辑 JSON 数据：

```json
{
  "request_header": {
    "app": {"package_name": "com.example.mini", "app_version": "1.0.0"}
  },
  "code": "wx.login 返回的 code",
  "auth_type": "miniprogram"
}
```

无需事先提供业务 `access_token` / `user_id`。`auth_type` 允许 `miniprogram` 或省略；App 和网页 OAuth 类型会被拒绝。设备信息沿用项目现有公共请求头。

**当前服务在 PROD 模式仍要求对请求做 XOR + Base64 编码；不能直接发送上述明文 JSON。** 响应已改为普通 JSON，无需 XOR + Base64 解码。DEV / LOCAL 响应字段通常为 camelCase；PROD 响应字段为 snake_case。下面 `apiRequest` 表示已经处理这些差异和公共请求头的客户端封装，不是本项目新增的 JS 函数：

```javascript
wx.login({
  success({ code }) {
    if (!code) return
    apiRequest('/v1/auth/wechat_auth', {
      code,
      auth_type: 'miniprogram'
    }).then(result => {
      // 检查 response_header 的业务状态后，保存业务 Token 和 user_info。
      // 后续请求在 request_header 中携带 user_id、access_token。
    }).catch(handleLoginError)
  },
  fail: handleLoginError
})
```

成功返回已有 `AuthResponse`：`access_token`、`refresh_token`、`expire_time`（Access Token 过期时间）、`user_info`、`is_first_login`、`auth_type`（WECHAT）。标准 JSON 的枚举通常为 `AUTH_TYPE_WECHAT`，PROD 编码内为数值 `4`。布尔值为 false 时可能省略。

登录凭证错误返回 `INVALID_REQUEST`；微信不可用、限流、配置缺失等返回 `REQUEST_FAILED`。HTTP 200 不等于业务成功，沿用公共响应头判断。失败后如需重新登录，应重新获取 code；后端不会自动重放 code2Session 请求。请求超时为 5 秒。

## 身份与范围

- 身份以 `wechat_miniprogram:{appid}:{openid}` 保存到现有 `provider_user_id`，业务用户仍按 `project_id` 隔离，复用现有用户、Token 刷新、登出及注册奖励流程。
- 不返回或保存微信原始 session_key。后端在 Redis 保存 `HMAC-SHA256(session_key, "")` 的十六进制签名，仅用于微信校验和重置。签名同样属于敏感凭证，不能暴露给前端或写日志。
- 不记录 code、AppSecret、微信原始响应或含密钥的请求 URL。
- 不依赖 unionid，不自动合并微信用户与手机号、邮箱或其他项目的账号。
- 本次仅实现身份登录；手机号授权、头像昵称填写和账号绑定需另行接入。

## 验证

```sh
go test ./conf ./internal/service/auth/providers/wechat ./internal/service/auth/types ./internal/service/auth ./internal/api ./internal/rpc ./internal/va_interface
```

测试覆盖微信响应、失效凭证、限流、缺少 OpenID、异常 HTTP / JSON、敏感错误脱敏、跨项目拒绝、重定向拒绝，以及 DEV/PROD 两种网关格式的路由和序列化。真实端到端登录需配置有效凭证，并在小程序中使用新生成的 code 联调。

官方接口：https://developers.weixin.qq.com/miniprogram/dev/server/API/user-login/api_code2session.html

## 检验与重置微信登录态

| 业务接口（POST） | gRPC | 作用 |
| --- | --- | --- |
| `/v1/auth/wechat_check_session` | `WeChatCheckSession` | 调用微信 checkSessionKey，验证后端签名是否仍有效 |
| `/v1/auth/wechat_reset_session` | `WeChatResetSession` | 调用微信 ResetUserSessionKey，更新后端签名 |

请求只携带现有公共请求头，不传 OpenID、session_key 或 signature：

```json
{
  "request_header": {
    "app": {"package_name": "com.example.mini"},
    "user_id": "业务用户ID",
    "access_token": "业务Access Token"
  }
}
```

设备 OS 应与登录时保持一致；这两个接口与登录接口一样，在 PROD 下需要现有网关编解码。

调用前严格校验项目、用户、业务 Token、Token 过期时间、Redis 登录态和用户状态，仅允许微信登录用户操作自己的微信身份。业务 Token 无效时返回 `INVALID_ACCESS_TOKEN`，不会向微信发送重置请求。

返回 `WeChatSessionResponse`：公共 `response_header` 和 `valid`。

- 先判断公共业务状态。成功且 `valid=true`：微信登录态有效，或重置成功。
- 检验成功且 `valid=false`（也可能省略）：签名不存在或微信确认已失效。重新 `wx.login()` 并调用 `/v1/auth/wechat_auth`。
- 微信限流、网络故障、平台凭证问题返回 `REQUEST_FAILED`，不能把这些错误当成“用户登录态失效”。重置所需签名缺失/失效返回 `INVALID_REQUEST`。

后端对同一 AppID/OpenID 的重置限制为每 30 秒一次；微信还可能施加自己的限制。重置成功会使旧 session_key 失效，但**不会延长微信 session_key 有效期，也不会刷新或撤销业务 Access Token / Refresh Token**。业务 Token 仍使用现有刷新和登出接口。

签名缓存按 AppID/OpenID 隔离，相同 AppID 的多个业务项目共享同一份签名，以符合微信单一有效 session_key 的语义。登录时本地最多保留 30 天，此期限是缓存保留策略，不能用来推断微信是否过期；重置保留原有剩余缓存 TTL。并发更新使用 Redis 比较更新，避免旧重置响应覆盖新登录签名。重置超时或响应不完整时不自动重放，清理旧签名后要求重新登录恢复。

平台凭证使用稳定版 `stable_token` 的普通模式（`force_refresh=false`），按微信返回有效期提前 60 秒过期缓存。平台凭证与业务 Token 完全独立，不返回前端。

从仅身份登录版本升级后，已有用户缓存中没有微信签名，需要重新微信登录一次；原有业务 Token 不会因此被撤销。

校验签名、TTL 保留、凭证缓存、失效分类、重置限频、并发更新及网关鉴权测试使用临时 Redis 模拟服务，不连接业务 Redis。

官方文档：[检验登录态](https://developers.weixin.qq.com/miniprogram/dev/server/API/user-login/api_checksessionkey.html)、[重置登录态](https://developers.weixin.qq.com/miniprogram/dev/server/API/user-login/api_resetusersessionkey.html)、[稳定版接口凭证](https://developers.weixin.qq.com/miniprogram/dev/server/API/mp-access-token/api_getstableaccesstoken.html)。
