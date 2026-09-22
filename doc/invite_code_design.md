# 邀请码业务流程分析

## 一、代码架构

```
├── internal/
│   ├── model/invite.go          # 数据模型
│   ├── constants/invite.go      # 常量定义
│   ├── dao/invite.go            # 数据访问层
│   ├── service/invite/service.go # 业务逻辑层
│   ├── api/invite.go            # API 层 (gRPC)
│   └── bootstrap/invite_module.go # 模块初始化
```

## 二、数据模型

### 1. InviteCode（邀请码表）
| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| project_id | varchar(100) | 项目ID（邀请码归属项目） |
| user_id | varchar(100) | 用户ID（邀请码拥有者）|
| code | varchar(20) | 邀请码（10位，排除易混淆字符）|
| created_at | datetime | 创建时间 |

**唯一索引**：
- `(project_id, user_id)` - 每用户每项目一个邀请码
- `(code)` - 邀请码全局唯一

### 2. InviteRecord（邀请记录表）
| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| project_id | varchar(100) | **邀请人的项目ID**（记录归属，用于展示） |
| inviter_user_id | varchar(100) | 邀请人用户ID |
| invite_user_id | varchar(100) | 被邀请人用户ID |
| invite_code | varchar(20) | 使用的邀请码 |
| inviter_credit_granted | tinyint(1) | 邀请人是否已获得积分 |
| invite_credit_granted | tinyint(1) | 被邀请人是否已获得积分 |
| created_at | datetime | 创建时间 |

**唯一索引**：
- `(project_id, invite_user_id)` - DB层唯一约束（历史索引）

> **注意**：业务逻辑上按 `invite_user_id` 全局检查，确保每用户只能使用一次邀请码（跨项目）

## 三、配置项

| 配置键 | 说明 | 默认值 |
|--------|------|--------|
| `invite_base_url` | 邀请链接基础URL | - |
| `invite_credit_amount` | 每次邀请奖励积分 | 300 |
| `invite_max_credits` | 邀请积分封顶（0=无上限）| 0 |

## 四、API 接口

### 1. GetInviteInfo - 获取邀请信息
**请求**：用户ID（从 header 获取）

**响应**：
```json
{
  "invite_code": "ABC123XYZ",
  "invited_count": 5,
  "total_credits": 1500,
  "used_invite_code": "DEF456",
  "invite_url": "https://xxx?code=ABC123XYZ"
}
```

> `invited_count` 和 `total_credits` 为全局统计（跨项目）

### 2. UseInviteCode - 使用邀请码
**请求**：邀请码

**响应**：
```json
{
  "credit_amount": 300
}
```

**错误码**：
- `INVALID_PARAM` - 邀请码不存在/不能使用自己的邀请码
- `INVALID_REQUEST` - 已使用过邀请码

### 3. GetInviteRecords - 获取邀请记录
**请求**：用户ID（从 header 获取）

**响应**：
```json
{
  "records": [
    {
      "invite_user_id": "user123",
      "invite_time": "Dec 29,2025",
      "credits": "+300"
    }
  ]
}
```

> 返回该用户全局的邀请记录（跨项目）

## 五、业务流程

### 流程1：获取/生成邀请码

```
用户请求获取邀请信息
        ↓
查询用户是否已有邀请码
        ↓
    ┌───┴───┐
    有      无
    ↓       ↓
  返回    生成10位随机码
          （排除0OoIl1）
            ↓
        检查是否唯一
            ↓
        ┌───┴───┐
      唯一    重复
        ↓       ↓
      保存   重试(最多10次)
        ↓
      返回邀请码
```

### 流程2：使用邀请码

```
被邀请人(B)输入邀请码
            ↓
    检查B是否已使用过邀请码【全局检查】
            ↓
        ┌───┴───┐
       是       否
        ↓       ↓
    返回错误   查找邀请码对应的邀请人(A)
                ↓
            ┌───┴───┐
          不存在   存在
            ↓       ↓
        返回错误   检查是否自己的码
                    ↓
                ┌───┴───┐
               是       否
                ↓       ↓
            返回错误   检查A是否达到积分封顶【全局统计】
                        ↓
                    ┌───┴───┐
                  封顶    未封顶
                    ↓       ↓
              [事务开始]  [事务开始]
                    ↓       ↓
              创建邀请记录  创建邀请记录
              (projectID   (projectID
               =A的项目)    =A的项目)
                    ↓       ↓
              发放B积分    发放B积分
              (B的项目)    (B的项目)
                    ↓       ↓
                  跳过     发放A积分
                          (A的项目)
                    ↓       ↓
              更新状态    更新状态
              (A=false)   (A=true)
              (B=true)    (B=true)
                    ↓       ↓
              [事务提交]  [事务提交]
```

### 流程3：积分封顶检查

```
获取配置的 invite_max_credits
            ↓
        ┌───┴───┐
       =0      >0
        ↓       ↓
    无上限   查询A已获得积分的记录数【全局统计】
              (inviter_credit_granted=true)
                ↓
            已获得积分 = 记录数 × 单次积分
                ↓
            ┌───┴───┐
        >=封顶    <封顶
            ↓       ↓
      不发放A积分  发放A积分
```

## 六、跨项目规则

### 设计原则
- **ProjectID 仅作为"来源标记"**，不参与业务逻辑过滤
- 所有统计和查询都是**全局的**（不按项目隔离）
- 支持跨项目邀请（如 `com.bluex.picflow` 和 `com.domob.piclib` 互通）

### 数据归属
| 数据 | 归属项目 |
|------|----------|
| 邀请码 | 创建时用户所在项目 |
| 邀请记录 | 邀请人的项目（用于记录展示） |
| 被邀请人积分 | 被邀请人所在项目 |
| 邀请人积分 | 邀请人所在项目 |

### 跨项目场景示例
```
邀请人(项目A) ──分享邀请码──→ 被邀请人(项目B)
     │                              │
     │                              ↓
     │                     使用邀请码
     │                              │
     ├──────────────────────────────┤
     ↓                              ↓
邀请记录存储在项目A          积分发到项目B
积分发到项目A
```

## 七、关键代码路径

### 使用邀请码完整链路
```
api/invite.go:UseInviteCode()
    ↓
service/invite/service.go:UseInviteCode()
    ├── dao/invite.go:GetInviteRecordByInvite(inviteUserID)  // 全局检查是否已使用
    ├── dao/invite.go:GetInviteCodeByCode()                  // 查找邀请人
    └── db.Transaction()
        ├── dao/invite.go:GetGrantedCreditsCountByInviterWithTx(inviterUserID)  // 全局封顶检查
        ├── dao/invite.go:CreateInviteRecord()   // 创建记录（projectID=邀请人项目）
        ├── credit.Service:AddCreditsWithTx()    // 发放被邀请人积分（被邀请人项目）
        ├── credit.Service:AddCreditsWithTx()    // 发放邀请人积分（邀请人项目，可选）
        └── dao/invite.go:UpdateInviteRecordCreditStatus()  // 更新状态
```

## 八、积分发放

### sourceID 格式
- 被邀请人：`invite_invite_{被邀请人projectID}_{inviteUserID}_{code}`
- 邀请人：`invite_inviter_{邀请人projectID}_{inviterUserID}_{code}`

### 积分属性
- 类型：`CreditTypeEventGrant`（事件赠送）
- 交易类型：`TransactionTypeInviteReward`（邀请奖励）
- 过期时间：永不过期（零值）
- 备注：被邀请人="邀请码奖励"，邀请人="邀请好友奖励"

### 积分归属规则
| 角色 | 积分数 | 发放条件 | 积分归属项目 |
|------|--------|----------|--------------|
| 被邀请人 | 300（可配置） | 始终发放 | 被邀请人的项目 |
| 邀请人 | 300（可配置） | 未达封顶时发放 | 邀请人的项目 |

## 九、并发安全

1. **邀请码生成**：通过唯一索引 + 重试机制处理并发
2. **使用邀请码**：业务层按 `invite_user_id` 全局检查，防止重复使用
3. **积分发放**：在数据库事务中执行，保证原子性
4. **封顶检查**：在事务内执行 `GetGrantedCreditsCountByInviterWithTx`，全局统计避免竞态条件

## 十、字段用途总结

| 字段 | 读取场景 | 写入场景 |
|------|---------|---------|
| `project_id` (InviteRecord) | 仅作为记录归属标记 | 使用邀请码时写入（邀请人的项目）|
| `inviter_credit_granted` | 全局统计已获得积分的邀请数（封顶检查、TotalCredits计算）| 使用邀请码时更新 |
| `invite_credit_granted` | 预留字段（用于未来延迟发放被邀请人积分场景）| 使用邀请码时更新（当前始终为true）|

## 十一、错误处理策略

### 可容忍错误（记录日志，返回默认值）

| 方法 | 调用 | 默认值 |
|------|------|--------|
| `GetInviteInfo` | `GetInviteStatsByInviter` | 0 |
| `GetInviteInfo` | `GetGrantedCreditsCountByInviter` | 0 |
| `GetInviteInfo` | `GetInviteRecordByInvite` | 未使用过邀请码 |
| `GetInviteRecords` | `GetInviteStatsByInviter` | 0 |
| `GetInviteRecords` | `GetInviteRecordsByInviter` | 空列表 |

### 不可容忍错误（必须中断流程）

| 方法 | 说明 |
|------|------|
| `UseInviteCode` | 涉及积分发放，必须保证数据一致性，所有错误都需中断 |
| `GetOrCreateInviteCode` | 邀请码是核心功能，生成失败需要返回错误 |
