# GetUserInfo 接口文档

## 接口概述

获取用户完整信息，包括用户基本资料、订阅状态、用户配置等。

## 接口信息

- **服务**: UserService
- **方法**: GetUserInfo
- **接口类型**: gRPC (目前不支持HTTP/REST)
- **是否需要认证**: 是（需要valid access_token）

## 请求参数

### UserInfoRequest

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| request_header | RequestHeader | 是 | 请求头，包含用户认证信息 |

### RequestHeader (关键字段)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| req_id | string | 是 | 请求ID，用于追踪 |
| access_token | string | 是 | 用户访问令牌 |
| user_id | string | 是 | 用户ID |
| user_type | UserType | 是 | 用户类型：0-注册用户，1-游客 |
| request_time_ms | int64 | 是 | 请求时间戳（毫秒） |
| app | App | 否 | App信息（移动端） |
| device | Device | 否 | 设备信息（移动端） |
| web_client | WebClient | 否 | Web客户端信息（Web端） |
| browser_info | BrowserInfo | 否 | 浏览器信息（Web端） |

## 响应参数

### UserInfoResponse

| 字段 | 类型 | 说明 |
|------|------|------|
| response_header | ResponseHeader | 响应头 |
| user_info | UserInfo | 用户信息 |

### ResponseHeader

| 字段 | 类型 | 说明 |
|------|------|------|
| req_id | string | 请求ID |
| code | StatusCode | 错误码（已废弃，使用response_status_code） |
| msg | string | 响应消息 |
| server_time | string | 服务器时间 |
| server_version | string | 服务器版本 |
| response_time_ms | int64 | 响应时间戳（毫秒） |
| user_id | string | 用户ID |
| response_status_code | ResponseStatusCode | 响应状态码（推荐使用） |

### UserInfo

| 字段 | 类型 | 说明 |
|------|------|------|
| user_id | string | 用户ID |
| user_name | string | 用户名 |
| phone_number | string | 手机号（格式：+区号+手机号） |
| avatar | string | 头像URL |
| gender | Gender | 性别：0-未知，1-男，2-女，3-无性别，4-保密 |
| birthday | string | 生日（格式：YYYY-MM-DD HH:MM:SS） |
| nickname | string | 昵称 |
| register_time | string | 注册时间 |
| last_login_time | string | 最后登录时间 |
| user_config | UserConfig | 用户配置 |
| subscribe_info | SubscribeInfo | 订阅信息 |

### UserConfig

| 字段 | 类型 | 说明 |
|------|------|------|
| share_url | string | 分享链接 |
| pop_subscribe | bool | 是否弹出订阅页面 |
| pop_free_daily_score | bool | 是否弹出每日积分提示 |
| free_daily_credit | int32 | 免费用户每日赠送积分数 |
| should_show | bool | 是否展示首页弹窗 |
| home_popup | PopupInfo | 首页弹窗信息 |
| float_tool_bar_tool_id | string | 首页悬浮工具栏默认工具ID |

### SubscribeInfo

| 字段 | 类型 | 说明 |
|------|------|------|
| product_id | string | 订阅产品ID |
| status | SubscribeStatus | 订阅状态：0-未订阅，1-订阅生效中，2-订阅过期 |
| subscribe_expired_time | string | 订阅过期时间 |
| subscribe_level | int32 | 订阅级别 |
| credit | int32 | 积分额度 |

## 响应状态码

### 成功状态码

| 状态码 | 数值 | 说明 |
|--------|------|------|
| RESPONSE_STATUS_CODE_SUCCESS | 200 | 请求成功 |

### 常见错误码

| 状态码 | 数值 | 说明 | 处理建议 |
|--------|------|------|----------|
| RESPONSE_STATUS_CODE_INVALID_PARAM | 1001 | 非法参数 | 检查请求参数格式 |
| RESPONSE_STATUS_CODE_INVALID_REQUEST | 1002 | 非法请求 | 检查请求结构 |
| RESPONSE_STATUS_CODE_REQUEST_FAILED | 1003 | 请求失败 | 重试或联系技术支持 |
| RESPONSE_STATUS_CODE_INVALID_ACCESS_TOKEN | 2001 | AccessToken错误 | 提示用户重新登录 |
| RESPONSE_STATUS_CODE_EXPIRED_ACCESS_TOKEN | 2002 | AccessToken过期 | 调用RefreshToken接口刷新令牌 |
| RESPONSE_STATUS_CODE_INVALID_REFRESH_TOKEN | 2003 | RefreshToken错误 | 提示用户重新登录 |
| RESPONSE_STATUS_CODE_INVALID_USER | 2004 | 无效的用户 | 用户可能被封禁或不存在 |

## 业务逻辑说明

### 认证验证
1. **注册用户**（user_type=0）：必须验证access_token有效性
2. **游客用户**（user_type=1）：无需token验证

### 数据获取流程
1. 从数据库获取用户基本信息
2. 从个人信息表获取昵称、头像、性别、生日
3. 获取订阅信息（SubscribeInfo）
4. 获取用户配置（UserConfig），包含首页弹窗等配置
5. 刷新refresh_token过期时间

### 特殊处理
- **订阅弹窗**：如果用户订阅状态为"生效中"（status=1），则不弹出订阅页面（pop_subscribe=false）
- **积分弹窗**：GetUserInfo接口中的积分弹窗字段始终为false，积分弹窗仅在登录时显示
- **头像和昵称**：优先从个人信息表获取，如果不存在则使用默认值

## 调用示例

### gRPC 调用示例（Go）

```go
package main

import (
    "context"
    "log"

    vai "your_project/internal/va_interface"
    "google.golang.org/grpc"
)

func main() {
    // 创建gRPC连接
    conn, err := grpc.Dial("your-server:port", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("连接失败: %v", err)
    }
    defer conn.Close()

    // 创建客户端
    client := vai.NewUserServiceClient(conn)

    // 构造请求
    req := &vai.UserInfoRequest{
        RequestHeader: &vai.RequestHeader{
            ReqId:         "unique-request-id",
            AccessToken:   "user-access-token",
            UserId:        "user-id",
            UserType:      vai.UserType_USER_TYPE_REGISTER,
            RequestTimeMs: time.Now().UnixMilli(),
            Device: &vai.Device{
                Os:       1, // Android
                Language: "zh-CN",
            },
            App: &vai.App{
                PackageName: "com.example.app",
                Version:     "1.0.0",
            },
        },
    }

    // 发起请求
    resp, err := client.GetUserInfo(context.Background(), req)
    if err != nil {
        log.Fatalf("请求失败: %v", err)
    }

    // 处理响应
    if resp.ResponseHeader.ResponseStatusCode == vai.ResponseStatusCode_RESPONSE_STATUS_CODE_SUCCESS {
        userInfo := resp.UserInfo
        log.Printf("用户ID: %s", userInfo.UserId)
        log.Printf("用户名: %s", userInfo.UserName)
        log.Printf("昵称: %s", userInfo.Nickname)
        log.Printf("订阅状态: %v", userInfo.SubscribeInfo.Status)
    } else {
        log.Printf("请求失败: %s", resp.ResponseHeader.Msg)
    }
}
```

### gRPC 调用示例（JavaScript/TypeScript）

```typescript
import { UserServiceClient } from './generated/service_grpc_pb';
import { UserInfoRequest, RequestHeader, UserType } from './generated/userinfo_pb';
import * as grpc from '@grpc/grpc-js';

// 创建客户端
const client = new UserServiceClient(
  'your-server:port',
  grpc.credentials.createInsecure()
);

// 构造请求
const request = new UserInfoRequest();
const header = new RequestHeader();
header.setReqId('unique-request-id');
header.setAccessToken('user-access-token');
header.setUserId('user-id');
header.setUserType(UserType.USER_TYPE_REGISTER);
header.setRequestTimeMs(Date.now());

request.setRequestHeader(header);

// 发起请求
client.getUserInfo(request, (error, response) => {
  if (error) {
    console.error('请求失败:', error);
    return;
  }

  const statusCode = response.getResponseHeader()?.getResponseStatusCode();
  if (statusCode === 200) {
    const userInfo = response.getUserInfo();
    console.log('用户ID:', userInfo?.getUserId());
    console.log('用户名:', userInfo?.getUserName());
    console.log('昵称:', userInfo?.getNickname());
    console.log('订阅状态:', userInfo?.getSubscribeInfo()?.getStatus());
  } else {
    console.error('请求失败:', response.getResponseHeader()?.getMsg());
  }
});
```

## 注意事项

### 安全性
1. **必须传递有效的access_token**：注册用户必须通过token认证
2. **Token过期处理**：收到2002错误码时，应调用RefreshToken接口刷新token
3. **敏感信息保护**：手机号等敏感信息仅返回给本人

### 性能优化
1. **缓存建议**：可以在客户端缓存用户信息，减少频繁请求
2. **按需更新**：仅在必要时（如登录后、修改资料后）调用此接口

### 数据一致性
1. **昵称和头像优先级**：个人信息表 > 用户表默认值
2. **订阅状态实时性**：订阅信息为实时查询，确保状态准确

### 平台差异
1. **移动端**：必须提供app和device信息
2. **Web端**：必须提供web_client和browser_info信息
3. **字段兼容**：RequestHeader支持移动端和Web端字段，使用optional标记

## 相关接口

- **RefreshToken**: 刷新访问令牌
- **EditUserName**: 修改用户名
- **DeleteUser**: 注销账号
- **UserNotifyToken**: 上报推送token

## 更新日志

| 日期 | 版本 | 说明 |
|------|------|------|
| 2026-01-29 | v1.0 | 初始版本 |

## 联系方式

如有疑问，请联系后端开发团队。
