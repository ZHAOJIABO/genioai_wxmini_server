# 微信小程序前端登录对接文档

本文对应当前后端已实现的三个接口，面向微信小程序前端开发。所有路径均相对于业务后端域名，不是微信官方 API 域名。

## 1. 接口总览

| 功能 | 方法与路径 | 是否需要业务登录 |
| --- | --- | --- |
| 微信身份登录 | `POST /v1/auth/wechat_auth` | 否 |
| 检验微信登录态 | `POST /v1/auth/wechat_check_session` | 是 |
| 重置微信登录态 | `POST /v1/auth/wechat_reset_session` | 是 |

需要先与后端约定：业务 HTTPS 域名、项目标识 `package_name`、环境编解码方式。项目标识必须与后端配置一致，使用小写；它是业务项目标识，不一定等于微信 AppID。后端负责配置真实小程序 AppID/AppSecret。

在小程序管理后台将业务后端域名加入 `request` 合法域名。

### 两种登录态

- **业务登录态**：后端返回的 `user_id`、`access_token`、`refresh_token`，用于调用业务 API。
- **微信登录态**：后端保存的微信会话校验签名，用于本次新增的检验、重置功能。前端不获取、不保存、不上传 `session_key`、`signature` 或微信平台 `access_token`。

微信登录态失效，不代表业务 Token 同时失效。重置微信登录态不能刷新业务 Token，也不能延长微信登录态有效期。不要将重置接口当成续期接口，或在每次页面打开时自动调用。

## 2. 通信格式和公共请求头

### 环境差异

| 环境 | 请求体 | 响应体 |
| --- | --- | --- |
| DEV / LOCAL | 普通 JSON；可使用本文 snake_case 请求字段 | 标准 grpc-gateway JSON，一般为 camelCase 字段、字符串枚举 |
| PROD | JSON 的 UTF-8 字节经过项目现有 XOR + Base64 编码，发送原始 Base64 文本 | 普通 JSON；一般为 snake_case 字段、数字枚举 |

所有请求设置 `Content-Type: application/json`。PROD 请求的 Base64 文本不要再次 `JSON.stringify` 加引号，也不要包成 `{data: ...}`。应复用项目现有请求编码模块，编码参数需与后端一致。示例 JSON 均指请求编码前、响应接收后的数据。

使用 `wx.request` 时可直接按 JSON 解析响应。已有响应解码封装需去掉 PROD 的 XOR + Base64 解码。本文末尾提供请求封装示例。

### 公共请求头

```json
{
  "request_header": {
    "app": {
      "package_name": "com.example.mini",
      "app_version": "1.0.0"
    },
    "device": { "os": 2 },
    "user_id": "业务用户ID",
    "access_token": "业务Access Token"
  }
}
```

| 字段 | 说明 |
| --- | --- |
| `request_header` | 三个接口均需传入 |
| `app.package_name` | 业务项目标识，登录与后续调用必须一致 |
| `app.app_version` | 可选，小程序业务版本号 |
| `device.os` | `1` 为 Android，`2` 为 iOS，`0` 为未知；可沿用现有设备封装。登录、检验、重置必须采用相同值，不要中途切换 |
| `user_id` | 登录接口可省略；其余两个接口必填，从登录响应 `user_info.user_id` 获取 |
| `access_token` | 登录接口可省略；其余两个接口必填，使用业务 Token |

鉴权字段放在请求体 `request_header` 中，仅设置 `Authorization: Bearer ...` 不能替代它。

### 业务成功判断

HTTP 200 只表示请求已处理。先解码响应，再检查 `response_header`（或 `responseHeader`）。

**当前三个接口使用旧 `code` 字段判断结果：数值 `0` 或字符串 `SUCCESS` 表示成功。** 标准 protobuf JSON 可能省略值为 0 的 `code`；在响应头存在时，可将省略的 `code` 按 0 处理。

当前实现将旧状态码直接赋给 `response_status_code`，因此成功时该字段可能为 `0`、`RESPONSE_STATUS_CODE_UNKNOWN_ERROR` 或省略，不能要求它必须为 `200`。不要只用这个字段判断成功。

## 3. 微信身份登录

`POST /v1/auth/wechat_auth`

### 请求

先调用 `wx.login()`，取得 `res.code` 后立即提交：

```json
{
  "request_header": {
    "app": { "package_name": "com.example.mini", "app_version": "1.0.0" },
    "device": { "os": 2 }
  },
  "code": "wx.login返回的code",
  "auth_type": "miniprogram"
}
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `code` | 是 | `wx.login()` 返回的临时凭证，不能重复使用；请求失败后重新尝试登录也要重新获取 |
| `auth_type` | 否 | 固定 `miniprogram`，省略也按小程序处理；不支持 `app` 或网页授权类型 |

### 成功响应示例

以下是解码后、统一成 snake_case 的精简示例；时间和用户资料仅为示意：

```json
{
  "response_header": { "code": 0, "msg": "success" },
  "access_token": "business-access-token",
  "refresh_token": "business-refresh-token",
  "expire_time": "2026-10-14 12:00:00",
  "user_info": {
    "user_id": "business-user-id",
    "user_name": "12345678",
    "nickname": "12345678",
    "avatar": ""
  },
  "is_first_login": true,
  "auth_type": 4
}
```

| 响应字段 | 前端处理 |
| --- | --- |
| `access_token` / `accessToken` | 保存为业务 Access Token |
| `refresh_token` / `refreshToken` | 保存为业务 Refresh Token，供现有业务刷新流程使用 |
| `user_info.user_id` / `userInfo.userId` | 保存为业务用户 ID，不要从响应头中寻找新登录用户 ID |
| `expire_time` / `expireTime` | Access Token 过期时间，格式为 `YYYY-MM-DD HH:mm:ss`，未携带时区；不要直接依赖 `new Date(value)` 进行跨平台鉴权判断，以服务器结果为准 |
| `user_info` / `userInfo` | 业务用户资料，不保证有微信头像、昵称、手机号 |
| `is_first_login` / `isFirstLogin` | 是否首次创建业务账号，缺失按 `false` 处理 |
| `auth_type` / `authType` | PROD 通常为数字 `4`，标准 JSON 通常为 `AUTH_TYPE_WECHAT` |

登录成功后一起替换本地用户 ID 和两个 Token。同一客户端避免并行登录；重新登录会签发新的业务 Token，本地旧 Token 不应继续使用。

本接口只完成微信身份登录，不包含手机号授权、微信头像昵称获取或已有手机号账号合并。

## 4. 检验微信登录态

`POST /v1/auth/wechat_check_session`

### 请求

只发送第 2 节的公共请求头，必须包含业务用户 ID 和 Access Token。无需再调用 `wx.login()` 获取 code，也不传 OpenID。

### 成功响应

```json
{
  "response_header": { "code": 0, "msg": "success" },
  "valid": true
}
```

- 业务成功且 `valid === true`：后端保存的微信登录态有效。
- 业务成功且 `valid` 为 `false` 或省略：后端没有签名，或微信确认已失效；需要使用微信登录态的流程应重新 `wx.login()` → `/wechat_auth`，并更新业务 Token。
- 业务失败：先按错误码处理，不能把网络故障、微信限流等一概当作 `valid=false`，也不要因此立即清空本地业务 Token。

这一步不是每个普通业务请求的必经步骤。仅需判断业务登录时，应使用业务接口的鉴权结果；需要确认后端微信会话是否可用时，再调用本接口。小程序侧 `wx.checkSession()` 的检查结果也不能代替本接口对后端已存签名的校验。

## 5. 重置微信登录态

`POST /v1/auth/wechat_reset_session`

请求与检验接口相同，成功响应也为 `response_header` 加 `valid: true`。

仅在产品明确需要轮换微信会话时调用。重置成功后：

1. 旧微信 session_key 失效，后端保存新的校验签名。
2. 原微信登录态有效期不延长，业务 Token 不变。
3. 前端不需要保存任何新的微信密钥，也不需要改写业务 Token。

调用期间禁用重复触发。同一 AppID/OpenID 后端至少间隔 30 秒允许再次重置，微信还可能有额外限制。

**重置请求超时后不要自动重发。** 服务端或微信可能已经完成重置。可以稍后检查状态；若确需恢复微信会话，重新完成微信登录。不要无限循环“重置 → 失败 → 重置”。

## 6. 错误处理

下表为这三个处理器的主要业务错误。DEV/LOCAL 可能返回枚举名称，PROD 通常返回数字。HTTP 非 2xx、响应解码失败也需单独处理。

| `code` 数值 / 名称 | 场景 | 前端动作 |
| --- | --- | --- |
| `0` / `SUCCESS` | 业务成功 | 登录保存凭证；检验/重置再读取 `valid` |
| `1002` / `INVALID_REQUEST` | 登录 code 无效、授权类型错误；重置所需签名缺失/失效 | 先检查参数；需要恢复会话时重新微信登录，勿复用旧 code |
| `1003` / `REQUEST_FAILED` | 微信限流、网络异常、后端配置或缓存异常、重置冷却期、并发状态变化等 | 提示稍后尝试，保留业务 Token；该码不能区分所有具体原因，不自动重置或连续重登 |
| `2001` / `INVALID_ACCESS_TOKEN` | 检验/重置的业务 Token 缺失、错误、过期、已失效，或调用者不满足微信用户要求 | 转现有业务鉴权恢复流程；也可重新微信登录。不要继续用该 Token 调用重置 |

`msg` 用于提示，不能靠匹配中文文案决定分支。微信原始错误码不会直接透传给前端。

这两个会话接口把业务 Token 过期也统一归为 `2001`，不要只监听 `2002` 才触发恢复。

## 7. 前端参考封装

以下 JavaScript 是文档示例，可放入小程序工具模块。`encodeBody` 和 `decodeBody` 是需要对接的通信适配器：

- DEV/LOCAL：分别传 `JSON.stringify`、`JSON.parse`。
- PROD：`encodeBody` 使用现有项目的“对象 → UTF-8 JSON → XOR → Base64 文本”；`decodeBody` 使用 `JSON.parse`（若 `wx.request` 已返回对象，则直接返回该对象）。

该封装保留原始响应，内部兼容字段名和主要错误枚举，串行执行三个操作，并合并同一时刻的重复登录/重置。它不会自动重试网络请求或实现业务 Refresh Token 刷新。

```javascript
export function createWechatAuthClient({
  baseURL, projectID, deviceOS = 0, appVersion = '1.0.0',
  encodeBody, decodeBody
}) {
  if (!baseURL || !projectID ||
      typeof encodeBody !== 'function' || typeof decodeBody !== 'function') {
    throw new Error('请配置后端域名、项目标识和通信编解码器')
  }
  const storageKey = `wechat-auth:${baseURL}:${projectID}:${deviceOS}`
  const codes = {
    SUCCESS: 0, INVALID_REQUEST: 1002, REQUEST_FAILED: 1003,
    INVALID_ACCESS_TOKEN: 2001
  }
  const pick = (obj, snake, camel) => obj[snake] ?? obj[camel]
  const readAuth = () => wx.getStorageSync(storageKey) || null
  let queue = Promise.resolve()
  let loginPending = null
  let resetPending = null

  function serialize(action) {
    const task = queue.then(action)
    queue = task.catch(() => {})
    return task
  }

  async function post(path, fields, authenticated) {
    const saved = readAuth()
    if (authenticated && (!saved?.userId || !saved?.accessToken)) {
      throw Object.assign(new Error('请先登录'), { code: 2001 })
    }
    const request_header = {
      app: { package_name: projectID, app_version: appVersion },
      device: { os: deviceOS }
    }
    if (authenticated) {
      request_header.user_id = saved.userId
      request_header.access_token = saved.accessToken
    }
    const body = encodeBody({ ...fields, request_header })
    const response = await new Promise((resolve, reject) => wx.request({
      url: baseURL.replace(/\/$/, '') + path,
      method: 'POST',
      header: { 'content-type': 'application/json' },
      data: body,
      dataType: 'text',
      responseType: 'text',
      timeout: 15000,
      success: resolve,
      fail: reject
    }))
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw new Error(`HTTP ${response.statusCode}`)
    }
    const data = decodeBody(response.data)
    if (!data || typeof data !== 'object') throw new Error('响应格式错误')
    const header = pick(data, 'response_header', 'responseHeader')
    if (!header || typeof header !== 'object') throw new Error('响应缺少业务状态')
    // protobuf JSON 可能省略默认值 0；不要改用 response_status_code。
    const rawCode = header.code ?? 0
    const code = codes[rawCode] ?? Number(rawCode)
    if (!Number.isFinite(code) || code !== 0) {
      throw Object.assign(new Error(header.msg || '请求失败'), { code: rawCode, numericCode: code })
    }
    return data
  }

  function login() {
    if (loginPending) return loginPending
    loginPending = serialize(async () => {
      const result = await new Promise((resolve, reject) => wx.login({
        success: resolve, fail: reject
      }))
      if (!result.code) throw new Error('微信未返回登录 code')
      const data = await post('/v1/auth/wechat_auth', {
        code: result.code, auth_type: 'miniprogram'
      }, false)
      const info = pick(data, 'user_info', 'userInfo') || {}
      const saved = {
        userId: pick(info, 'user_id', 'userId'),
        accessToken: pick(data, 'access_token', 'accessToken'),
        refreshToken: pick(data, 'refresh_token', 'refreshToken'),
        expireTime: pick(data, 'expire_time', 'expireTime'),
        userInfo: info
      }
      if (!saved.userId || !saved.accessToken || !saved.refreshToken) {
        throw new Error('登录响应缺少业务凭证')
      }
      wx.setStorageSync(storageKey, saved)
      return data
    }).finally(() => { loginPending = null })
    return loginPending
  }

  function checkSession() {
    return serialize(async () => {
      const data = await post('/v1/auth/wechat_check_session', {}, true)
      return data.valid === true
    })
  }

  function resetSession() {
    if (resetPending) return resetPending
    resetPending = serialize(async () => {
      const data = await post('/v1/auth/wechat_reset_session', {}, true)
      if (data.valid !== true) throw new Error('重置未返回有效状态，请检查登录态')
      return true
    }).finally(() => { resetPending = null })
    return resetPending
  }

  return { login, checkSession, resetSession, readAuth }
}
```

开发环境初始化示例：

```javascript
const auth = createWechatAuthClient({
  baseURL: 'https://你的开发后端域名',
  projectID: 'com.example.mini',
  deviceOS: 2, // 示例：iOS；实际值取自项目统一设备封装
  encodeBody: JSON.stringify,
  decodeBody: JSON.parse
})

// 应用中共享同一个 auth 实例，避免多个页面各建实例并发登录。
await auth.login()

// 只在需要使用微信会话的场景中检查。
const valid = await auth.checkSession()
if (!valid) await auth.login()

// resetSession() 应由明确的业务操作触发，不紧接登录自动执行。
```

示例错误可读取 `error.numericCode ?? error.code`。错误分支按第 6 节处理，不建议无条件清空本地凭证。若已有统一 Token 存储/刷新模块，请将示例的 `readAuth` 和保存逻辑接到同一份状态中，避免产生两份不一致的 Token。

## 8. 联调验收清单

- [ ] 后端项目标识与小程序请求一致，真实 AppID/AppSecret 已由后端配置。
- [ ] DEV/LOCAL 明文 JSON 与 PROD 编解码分别验证，未把 Base64 作为 JSON 二次包装。
- [ ] 登录成功后，从 `user_info` 获取用户 ID，并保存两个业务 Token。
- [ ] 正确处理 camelCase / snake_case、字符串 / 数字状态码以及省略的默认值。
- [ ] 使用同一个设备 OS 调用登录、检验、重置。
- [ ] 检验成功且 `valid=false` 时可重新微信登录；网络失败不会被当成 `false`。
- [ ] 未登录、错误 Token、过期 Token不能操作微信会话。
- [ ] 重置不会修改本地业务 Token；短时间重复操作受限，超时不自动重发。
- [ ] 已有微信账号在升级后重新登录一次建立签名缓存；原业务 Token 不会仅因升级被撤销。
- [ ] 日志、埋点和错误上报不包含 code、业务 Token 或完整登录响应。

后端配置及实现说明见 [微信小程序登录后端文档](./wechat-miniprogram-login.md)。
