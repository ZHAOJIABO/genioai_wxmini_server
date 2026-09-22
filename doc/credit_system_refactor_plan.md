# 创作次数（Credits）系统重构计划

本文档旨在跟踪和记录新的创作次数管理系统的重构过程。

## 计划步骤

- [x] **第一步：数据模型与数据库迁移**
  - [x] 创建 `CreditTransaction` model (`internal/model/credit.go`)
  - [x] 创建 `Credit` 相关常量 (`internal/constants/credit.go`)
  - [x] 修改 `UserAmount` model (`internal/model/amount.go`)
  - [x] 更新数据库自动迁移脚本 (`internal/db/mysql.go`)

- [x] **第二步：创建新的 `CreditService` 和 `CreditDao`**
  - [x] 创建 `CreditDao` (`internal/dao/credit.go`)
    - [x] `CreateTransaction` - 创建交易记录
    - [x] `FindTransactionBySourceID` - 根据源ID查找交易
    - [x] `GetUserAmounts` - 获取用户额度
    - [x] `CreateOrUpdateUserAmount` - 创建或更新用户额度
    - [x] `UpdateUserAmount` - 更新用户额度
    - [x] `GetUserTransactions` - 获取用户交易记录
  - [x] 创建 `CreditService` (`internal/service/credit/service.go`)
    - [x] `DeductCredits` - 扣减额度（优先扣减会员赠送次数）
    - [x] `AddCredits` - 增加额度
    - [x] `RefundCredits` - 退款
    - [x] `ClearExpiredCredits` - 清理过期额度
    - [x] `GetUserCreditBalance` - 查询用户额度余额
    - [x] `GetUserCreditTransactions` - 查询用户交易记录
    - [x] `GetExpiredCreditsStats` - 获取过期额度统计（基础实现）

- [x] **第三步：将图片/视频任务流对接新的 `CreditService`**
  - [x] 修改 `PictureTaskService` (`internal/service/picture_task.go`) 的任务提交流程。
  - [x] 移除对旧 `user_amount` 服务的调用。
  - [x] 在任务提交时，调用 `CreditService.DeductCredits` 进行扣款。
  - [x] 在任务失败时，调用 `CreditService.RefundCredits` 进行退款。
  - [x] 更新依赖注入系统，在 `bootstrap` 中注册 `CreditService`。
  - [x] 修改 `PictureForgeServer` 构造函数，使用新的 `CreditService`。
  - **注意**: 目前只在 `KlingImage2VideoExecutor` 中实现了退款逻辑。其他执行器（如 `GPT4oImage2ImageExecutor`）需要类似的修改。

- [x] **第四步：实现会员额度发放与过期处理**
  - [x] 创建会员额度发放服务 (`internal/service/credit/membership.go`)
  - [x] 实现会员升级时的额度发放
  - [x] 实现会员到期时的额度清零
  - [x] 创建定时任务处理会员额度过期 (`internal/task/credit_expiry_processor.go`)
  - [x] 集成到现有的会员服务中 (`internal/service/subscribe.go`)
  - [x] 在应用启动时启动定时任务 (`cmd/main.go`)

- [x] **第五步：清理旧的额度管理代码**
  - [x] 移除旧的 `user_amount` 服务 (`internal/service/user_amount`)。
  - [x] 验证并确保所有相关功能已迁移到新系统或确认可以安全移除。
  - [x] 运行 linter 并修复相关问题。

## 第六步：设计哲学 — 为什么 `UserAmount` 表没有 `TotalAmount` 字段？

在本次重构中，一个核心的设计决策是 `UserAmount` 模型（`va_user_amount` 表）仅包含 `RemainingAmount`（剩余额度），而没有 `TotalAmount`（累计获得额度）字段。这不是一个疏漏，而是一个经过深思熟虑的、旨在保证数据一致性和简化核心逻辑的架构选择。

### 1. 关注点分离：不可变流水 vs. 可变余额

系统的数据模型被清晰地划分为两个部分：

*   **`CreditTransaction` (交易流水)**: 这是系统的**事实来源 (Source of Truth)**。每一笔积分的增减都被记录为一条不可变的流水。一旦生成，就不应修改。它保证了所有操作都有据可查。
*   **`UserAmount` (额度余额)**: 这是一个**可变的状态快照 (Mutable State Snapshot)**。它仅仅是基于所有历史交易计算出的一个当前结果。它的存在是为了**性能优化**，使得查询用户当前余额时，无需实时计算海量的交易流水，从而获得极高的查询性能。

### 2. 避免数据冗余与不一致

如果在 `UserAmount` 表中同时存储 `RemainingAmount` 和 `TotalAmount`，会引入数据冗余，并带来风险：

*   **更新复杂性**: 每次增加额度时，需要同时更新两个字段，增加了逻辑的复杂性。
*   **不一致风险**: 存在代码只更新了其一而忘记更新另一个的风险，导致数据不一致。当余额表的累计值与流水表的计算结果不一致时，我们必须以流水表为准，这使得余额表中的 `TotalAmount` 字段变得不可靠。

### 3. 如何统计"累计获得总额"？

当业务需要统计用户累计获得了多少积分时，正确且唯一可靠的方式是查询交易流水表：

```sql
-- 示例：查询用户累计获得了多少"订阅赠送"积分
SELECT SUM(amount_change)
FROM va_credit_transaction
WHERE user_id = 'some_user_id'
  AND credit_type = 'membership_grant'
  AND amount_change > 0;
```

这种按需计算的方式确保了数据的绝对准确性。对于需要频繁展示的场景，可以通过缓存或定时任务来进行优化，但这属于查询性能优化的范畴，不应破坏核心数据模型的一致性。

**总结**：当前的设计用"查询时的计算"换取了"写入时的数据一致性和简单性"，是一种在金融和账务系统中广泛采用的、健壮的设计模式。 

## 第七步：非会员每日免费积分不混用策略 (2025-10-11)

### 背景

非会员用户每日会获得免费积分（`daily_free`），但也可能拥有之前在VIP有效期内购买的积分（`credits_purchase`）或已过期会员的赠送积分（`membership_grant`）。

原有的扣减逻辑会按 `expired_at` 时间排序混合扣减所有类型的积分，这导致每日免费积分和内购积分可能在同一次任务提交中混合使用。

### 新策略

**非会员用户：**
1. 如果 `amount <= daily_free_total`：**仅扣减每日免费积分**
2. 否则，如果 `amount <= (purchased_total + membership_grant_total)`：**仅扣减内购和会员赠送积分**（这两类可混用）
3. 否则：返回 `ErrInsufficientCredits`（**不允许混用**）

**会员用户：**
- 跳过 `daily_free`，其余类型（`credits_purchase`, `membership_grant`）可按过期时间混用（维持原逻辑）

### 退款行为

- 退款严格按照 `CreditDeductionInfo` 中记录的 `amountID -> amount` 原路返还
- 如果扣减时使用的 `UserAmount` 记录在退款时已过期：
  - 积分仍会加回到该记录的 `RemainingAmount`
  - 但由于 `GetUserAmounts` 查询时会过滤 `expired_at < now` 的记录，该积分实际无法再使用
  - **这是设计预期**，确保每日积分的时效性
  - 如需改进，可在退款时检测过期并转发到新的系统补偿记录，或延长有效期（需另行实现）

### 实现细节

**修改文件：**
- `internal/service/credit/service.go`
  - 重构 `DeductCreditsWithTx`：新增 `deductCreditsForMember` 和 `deductCreditsForNonMember` 两个分支函数
  - 新增 `performDeduction`：统一的扣减执行逻辑
  
**测试覆盖：**
- `internal/service/credit/service_test.go`
  - `TestDeductCreditsForNonMember_DailyFreeOnly`：每日免费足够时仅扣每日
  - `TestDeductCreditsForNonMember_PurchasedOnly`：每日免费不足时仅扣内购
  - `TestDeductCreditsForNonMember_NoMixing`：总额足够但不混用时报错
  - `TestDeductCreditsForNonMember_WithMembershipGrant`：内购和会员赠送可混用
  - `TestDeductCreditsForMember_SkipDailyFree`：会员跳过每日免费
  - `TestRefundCredits_ExpiredCredit`：退款到过期记录的行为说明

### 影响范围

- **任务提交**：所有通过 `DeductCreditsWithTx` 的扣减路径（单任务、工具任务、任务链）统一执行新策略
- **向后兼容**：会员用户行为保持不变；非会员用户行为优化，不会破坏现有功能

## 第八步：订阅信用点发放逻辑重构

随着业务发展，我们需要更精细化地管理用户通过订阅获得的信用点。原有的模型将同一类型（如"会员赠送"）的所有信用点聚合到一条 `UserAmount` 记录中，这在追踪特定批次信用点的来源和生命周期（特别是过期）方面存在局限。

本次重构旨在将 `UserAmount` 表从一个简单的"余额聚合快照"升级为一个"额度分账记录"表。每一笔来自订阅的信用点赠送，都将是一条独立的、可追溯的记录。

### 1. 设计变更：从"聚合"到"分账"

核心变更：`UserAmount` 表的每一条记录现在代表一笔**具体的额度入账**，而不是某个用户某种额度的总和。

*   **引入 `SourceID`**: `UserAmount` 表中增加 `SourceID` 字段，并建立唯一索引。`SourceID` 通常关联到触发这笔额度变更的外部事件ID，例如支付成功事件中的支付ID (`payment_id`)。这确保了每一笔额度发放都是幂等的、可追溯的。
*   **独立的生命周期**: 每一条 `UserAmount` 记录都有自己的 `RemainingAmount` 和 `ExpiredAt`。例如，用户连续两个月订阅，会产生两条独立的 `UserAmount` 记录，它们可以有不同的过期时间。
*   **"总余额"是计算字段**: 用户的某种类型的总余额（例如，会员赠送的总信用点），不再是表中的一个字段，而是通过查询该用户所有未过期的、该类型的 `UserAmount` 记录的 `RemainingAmount` 之和来动态计算得出。这与"第六步"中 `TotalAmount` 的设计哲学一脉相承，即用"查询时计算"来保证数据的准确性。

### 2. 关键业务逻辑调整

#### a. 续订与过期处理

*   **实现方案**: 每条 `UserAmount` 记录在创建时就已根据订阅周期设置了明确的 `ExpiredAt` 过期时间。额度的生命周期完全由其自身的 `ExpiredAt` 字段决定。系统将依赖一个独立的定时任务（如 `credit_expiry_processor`）来定期清理或处理已过期的额度记录，而无需在续订时进行额外操作。这简化了单次续订事件的业务逻辑。

#### b. 信用点扣减 (DeductCredits)

扣减逻辑需要变得更智能，以支持从多条 `UserAmount` 记录中消费。

*   **扣减顺序**: 为了最大化用户利益，扣减时应遵循"**最早过期优先**" (First-Expire, First-Out) 的原则。服务会查询用户所有有效的 `UserAmount` 记录，并按 `ExpiredAt` 升序排序。
*   **事务性操作**: 扣减将从最先过期的记录开始，逐条扣减 `RemainingAmount`，直到满足本次消费总额。整个过程将在一个数据库事务中完成，以保证数据一致性。

### 3. 技术方案实施计划

**重要注意事项**: 在修改 `CreditService` 中现有函数（如 `AddCredits`, `DeductCredits`）时，必须格外谨慎。在实施修改前，需要仔细检查这些函数的上层调用关系，确保变更不会对本次规划范围之外的业务逻辑（例如：非订阅类的额度购买、手动后台充值等）产生非预期的影响。如有必要，应考虑创建新的函数或增加逻辑分支来隔离新旧逻辑。

以下是具体的代码修改计划，我们将分阶段完成。

- [ ] **阶段一：数据层与核心服务调整**
    - [ ] **DAO层 (`internal/dao/user_amount.go`)**:
        - [ ] 移除或重构 `CreateOrUpdateUserAmount` 方法，因为聚合逻辑已不再适用。
        - [ ] 创建 `CreateUserAmount(tx *gorm.DB, amount *model.UserAmount)` 用于插入新的独立额度记录。
        - [ ] 创建 `FindActiveUserAmounts(userID, creditType)`，返回一个 `[]*model.UserAmount` 列表，按 `ExpiredAt` 升序排序，用于扣减和余额查询。
    - [ ] **Credit服务 (`internal/service/credit/service.go`)**:
        - [ ] **重构 `AddCredits`**:
            - [ ] 调用 `dao.CreateUserAmount` 创建新的额度记录，并传入 `SourceID` 和 `ExpiredAt`。
        - [ ] **重构 `DeductCredits`**:
            - [ ] 必须在数据库事务中执行。
            - [ ] 调用 `dao.FindActiveUserAmounts` 获取所有可用额度记录。
            - [ ] 按照"最早过期优先"的顺序，循环扣减每条记录的 `RemainingAmount`，并更新回数据库。
        - [ ] **重构 `GetUserCreditBalance`**:
            - [ ] 调用 `dao.FindActiveUserAmounts` 并加总所有记录的 `RemainingAmount`，得出当前总余额。

- [ ] **阶段二：业务逻辑集成**
    - [ ] **支付事件处理器 (`internal/service/event/handlers/payment_handler.go`)**:
        - [ ] 修改处理器逻辑，从支付事件中提取 `payment_id` 作为 `SourceID`，以及计算本次额度的 `ExpiredAt`。
        - [ ] 调用重构后的 `CreditService.AddCredits` 方法，传入新的参数。
    - [ ] **数据库迁移**:
        - [ ] (已完成) 确认 `user_amount` 表结构已按 `internal/model/amount.go` 中的定义更新，包括 `SourceID` 字段和唯一索引。 

- [ ] **阶段三：审阅与验证**
    - [ ] 审阅所有调用 `RefundCredits` 的地方（主要是 `TaskFailureHandler` 的实现），确保调用方式正确无误。

### 第九步：实现基于"信息传递"的精确退款（最终方案）

**设计哲学**: 积分退款问题的根源在于信息在业务流程中的丢失。与其进行大规模系统替换，不如在关键节点精确传递和保存必要的信息。本方案通过在任务创建时记录积分来源，实现了轻量级、低风险且逻辑健壮的精确退款。

**技术方案实施计划**:

- [ ] **阶段一：增强扣款服务，使其能够"述源"**
    - [ ] **任务**: 修改 `internal/service/credit/service.go` 中的 `DeductCredits` 函数。
    - [ ] **变更**: 调整其返回值，使其在成功扣款后，能够返回一个 `[]uint` 列表。此列表包含了所有在此次操作中被扣减的 `UserAmount` 记录的主键 `ID`。

- [ ] **阶段二：在业务流程中"传递并记录"溯源信息**
    - [ ] **任务**: 修改创建 `PictureTask` 的核心业务逻辑（例如在 `picture_forge.go` 服务中）。
    - [ ] **变更**:
        1.  在调用 `DeductCredits` 后，接收返回的 `userAmountIDs` 列表。
        2.  将此ID列表序列化为JSON字符串。
        3.  创建一个新的结构体来承载这些元数据，例如 `{"refund_source_amount_ids": [101, 102]}`，然后将此结构体的JSON表示存入 `PictureTask` 的 `ExecuterTaskInfo` 字段中。

- [ ] **阶段三：实现具备"降级策略"的精确退款服务**
    - [ ] **任务**: 在 `internal/service/credit/service.go` 中创建一个**新的**、独立的退款函数 `RefundToSourceAmounts(ctx, sourceAmountIDs []uint, totalAmountToRefund int64, description string)`。
    - [ ] **核心逻辑**:
        1.  在数据库事务中执行。
        2.  **主路径**: 遍历 `sourceAmountIDs`，`SELECT ... FOR UPDATE` 锁定并查找对应的 `UserAmount` 记录。
        3.  **有效性检查**: 检查查找到的记录是否已过期 (`ExpiredAt`)。
        4.  **精确返还**: 如记录未过期，则将相应部分的退款金额返还至 `RemainingAmount`。
        5.  **降级处理**: 如记录已过期，则触发降级逻辑：尝试查找与该过期记录同 `CreditType` 的、仍在有效期内的其他积分池，并将退款返还至其中（优先返还至最先要过期的池子）。
        6.  **最终丢弃**: 如降级逻辑也无法找到合适的积分池，则记录一条警告日志，该部分退款将被放弃。

- [ ] **阶段四：应用新的退款服务**
    - [ ] **任务**: 修改 `internal/task/picture_workflow/failure_handler.go` 中的 `FailTask` 方法。
    - [ ] **变更**:
        1.  在处理任务失败时，从 `PictureTask` 的 `ExecuterTaskInfo` 字段中解析出 `refund_source_amount_ids` 列表。
        2.  调用新建的 `RefundToSourceAmounts` 函数来执行精确退款，取代任何旧的、模糊的退款调用。 

## 第十步：精确退款机制落地与旧逻辑修正

**背景**: 当前的 `RefundCredits` 函数依赖 `TaskID` 作为 `SourceID` 来查找原始扣款记录，但这与 `DeductCredits` 函数记录 `UserAmount.SourceID` 的行为不匹配，导致退款逻辑无法正确执行。第九步中规划了基于"信息传递"的精确退款方案，本步骤将此方案具体化并落地实施。

**核心思想**: 将积分扣款视为一个产生"收据"（DeductionInfo）的过程。此"收据"详细记录了从哪些积分账户（`UserAmount` 记录）中扣除了多少金额。业务流程（如创建任务）负责保存这张"收据"，当需要退款时，将"收据"交还给积分服务，即可实现精确、无歧义的退款。

### 实施计划

- [ ] **第一步：改造 `CreditService` 的扣款接口，使其返回"扣款收据"**
    - **任务**: 修改 `internal/service/credit/service.go` 中的 `DeductCreditsWithTx` 函数。
    - **变更**:
        - 将函数签名从 `func(...) error` 修改为 `func(...) (map[uint]int64, error)`。
        - 成功执行扣款后，函数不再仅返回 `nil`，而是返回一个 `map`。此 `map` 的键（`uint`）是被扣款的 `UserAmount` 记录的主键 `ID`，值（`int64`）是从该记录中具体扣除的金额。这就是"扣款收据"。
        - `DeductCredits` 函数也应相应修改。

- [ ] **第二步：在 `PictureTask` 模型中增加字段以存储"扣款收据"**
    - **任务**: 修改 `internal/model/picture_forge.go` 中的 `PictureTask` 结构体。
    - **变更**:
        - 增加一个新字段 `CreditDeductionInfo string `json:"credit_deduction_info"`。
        - 这个字段将用于存储"扣款收据"的JSON序列化字符串。
        - 别忘了在 `internal/db/mysql.go` 的 `AutoMigrate` 中加入 `PictureTask` 模型以触发数据库表结构更新。

- [ ] **第三步：在任务提交流程中，保存"扣款收据"**
    - **任务**: 修改 `PictureTaskService` (`internal/service/picture_task.go`) 中提交任务的核心逻辑，如 `SubmitTaskWithTx`。
    - **变更**:
        - 在调用 `creditService.DeductCredits` 的地方，接收其返回的"扣款收据" map。
        - 将此 map 序列化为 JSON 字符串。
        - 在创建 `PictureTask` 实例并存入数据库之前，将此 JSON 字符串赋值给新的 `CreditDeductionInfo` 字段。

- [ ] **第四步：实现全新的、精确的退款服务并废弃旧版**
    - **任务**: 在 `internal/service/credit/service.go` 中进行修改。
    - **变更**:
        1.  **创建新方法**: 实现一个新的私有退款方法 `refundCreditsByDeductionInfo(tx *gorm.DB, deductionInfo map[uint]int64, description string) error`。此方法将遍历传入的"收据"，在事务中将金额精确返还到对应的 `UserAmount` 记录中。
        2.  **废弃旧方法**: 将现有的 `RefundCredits` 函数标记为 `// Deprecated: a new precise refund mechanism is in place.`，并使其内部直接返回一个错误，防止被意外调用。
        3.  **提供新接口**: 创建一个新的公共方法 `RefundCredits(ctx context.Context, task *model.PictureTask, description string)`，该方法负责从 `task.CreditDeductionInfo` 中解析出"收据"信息，并调用新的私有退款方法。

- [ ] **第五步：在任务失败流程中，启用精确退款**
    - **任务**: 修改 `PictureTaskService.FailTask` (`internal/service/picture_task.go`)。
    - **变更**:
        - 移除对旧的 `creditService.RefundCredits(..., taskID, ...)` 的调用。
        - 改为调用新的 `creditService.RefundCredits(ctx, latestTask, "任务执行失败退款")`。 