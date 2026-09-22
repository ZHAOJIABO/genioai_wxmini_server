# 用户积分增加接口文档

## 接口概述

该接口用于给指定用户增加积分（系统赠送），每次调用固定增加 **10000 积分**。

**参数说明**：接口接收用户的 UUID，后端会通过 UUID 在 `user_record` 表中查询出真正的 `user_id`，然后使用该 `user_id` 进行积分操作。

## 接口信息

- **接口路径**: `/v1/subscribe/add_user_credits`
- **请求方法**: `POST`
- **请求格式**: `application/json`
- **响应格式**: `application/json`

## 请求参数

### 请求头 (RequestHeader)

标准的请求头信息，包含用户认证等信息。

### 请求体 (AddUserCreditsRequest)

```json
{
  "request_header": {
    // 标准请求头信息
  },
  "user_id": "用户的UUID"
}
```

| 字段名 | 类型 | 必填 | 说明 |
|-------|------|------|------|
| user_id | string | 是 | 用户的UUID（注意：这里虽然字段名是user_id，但实际传入的是UUID，后端会通过UUID在user_record表查询出真正的user_id）|

## 响应参数

### 成功响应 (AddUserCreditsResponse)

```json
{
  "response_header": {
    "code": "SUCCESS",
    "msg": "success"
  },
  "success": true,
  "total_credits": 15000
}
```

| 字段名 | 类型 | 说明 |
|-------|------|------|
| success | bool | 操作是否成功 |
| total_credits | int64 | 增加后用户的总积分 |

### 错误响应

```json
{
  "response_header": {
    "code": "INVALID_PARAM",
    "msg": "UUID不能为空"
  },
  "success": false,
  "total_credits": 0
}
```

或

```json
{
  "response_header": {
    "code": "REQUEST_FAILED",
    "msg": "用户不存在"
  },
  "success": false,
  "total_credits": 0
}
```

## 业务逻辑说明

1. **参数接收**: 接口接收用户的 UUID
2. **用户查询**: 通过 UUID 在 `user_record` 表中查询用户信息，获取真正的 `user_id`（UserId 字段）
3. **积分类型**: 使用活动赠送类型 (`event_grant`)
4. **交易类型**: 系统赠送 (`system_grant`)
5. **固定金额**: 每次调用固定增加 10000 积分
6. **过期时间**: 100 年后过期（实际上永不过期）
7. **描述信息**: "系统赠送积分"
8. **返回结果**: 返回用户增加后的总积分

## 调用示例

### cURL 示例

```bash
curl -X POST 'http://localhost:8080/v1/subscribe/add_user_credits' \
  -H 'Content-Type: application/json' \
  -d '{
    "request_header": {
      "user_id": "user_123"
    },
    "user_id": "550e8400-e29b-41d4-a716-446655440000"
  }'
```

### Go 客户端示例

```go
client := vai.NewSubscribeServiceClient(conn)

req := &vai.AddUserCreditsRequest{
    RequestHeader: &vai.RequestHeader{
        UserId: "user_123",
    },
    UserId: "550e8400-e29b-41d4-a716-446655440000",
}

resp, err := client.AddUserCredits(context.Background(), req)
if err != nil {
    log.Fatalf("AddUserCredits failed: %v", err)
}

fmt.Printf("Success: %v, Total Credits: %d\n", resp.Success, resp.TotalCredits)
```

## 数据库影响

调用此接口后，会产生以下数据库变更：

### 查询操作
1. **user_record 表**: 通过 UUID 查询用户信息
   - 查询条件：`project_id = ? AND uuid = ?`
   - 获取字段：`user_id`（UserId）

### 写入操作
1. **va_user_amount 表**: 新增积分记录
   - `source_id`: 系统生成的唯一ID（格式：`system_grant_<uuid>`）
   - `amount_iden`: `event_grant`
   - `user_id`: 从 user_record 表查询到的 UserId
   - `remaining_amount`: `10000`
   - `expired_at`: 100年后的时间

2. **credit_transaction 表**: 新增交易记录
   - `transaction_type`: `system_grant`
   - `user_id`: 从 user_record 表查询到的 UserId
   - `amount_change`: `10000`（正数表示增加）
   - `credit_type`: `event_grant`
   - `description`: "系统赠送积分"

## 数据流程图

```
客户端请求（UUID）
    ↓
API Handler (api/subscribe.go::AddUserCredits)
    ↓
验证 UUID 参数
    ↓
SubscribeService::AddUserCreditsByUUID
    ↓
UserDao::GetUserByUUID（查询 user_record 表）
    ↓
获取 user_id（UserId 字段）
    ↓
SubscribeService::AddUserCredits（使用 user_id）
    ↓
CreditService::AddCredits
    ↓
写入 va_user_amount 表 + credit_transaction 表
    ↓
查询用户总积分
    ↓
返回总积分给客户端
```

## 错误码说明

| 错误码 | 说明 |
|-------|------|
| SUCCESS | 操作成功 |
| INVALID_PARAM | 参数无效（如 UUID 为空）|
| REQUEST_FAILED | 请求失败（如用户不存在、数据库操作失败）|

## 注意事项

1. 接口参数虽然字段名是 `user_id`，但实际传入的是用户的 **UUID**
2. 后端会自动通过 UUID 在 `user_record` 表中查询出真正的 `user_id`（UserId 字段）
3. 如果 UUID 在数据库中不存在，会返回"用户不存在"错误
4. 该接口每次调用固定增加 10000 积分，不支持自定义金额
5. 积分类型为活动赠送，100年后过期（实际上永不过期）
6. 如需修改赠送金额，需要修改源代码中的硬编码值
7. 该接口会记录完整的交易流水，可在积分明细中查询

## 相关接口

- **获取用户积分**: `/v1/subscribe/get_genio_user_amount`
- **获取积分明细**: `/v1/subscribe/get_credit_detail`
- **用户信息查询**: `/v1/user/get_user_info`（包含积分信息）

## 实现文件位置

- **Proto 定义**: `pkg/va_interface/subscribe.proto`
- **Service 层**:
  - `internal/service/subscribe.go::AddUserCreditsByUUID()` - 通过UUID查询用户并添加积分
  - `internal/service/subscribe.go::AddUserCredits()` - 核心积分添加逻辑
- **API Handler**: `internal/api/subscribe.go::AddUserCredits()`
- **DAO 层**: `internal/dao/user.go::GetUserByUUID()` - 通过UUID查询用户
- **积分服务**: `internal/service/credit/service.go`

## 开发日志

- **创建时间**: 2026-02-12
- **创建者**: Claude Code
- **版本**: v1.0.0
