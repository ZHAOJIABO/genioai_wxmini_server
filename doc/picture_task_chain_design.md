## 图片任务链（图→视频）方案（路径A：进程内Hook编排）

### 背景与目标
- 支持在模板详情页「Create Video」：当图片生成完成后，自动衔接一次「图→视频」的二阶段生成。
- 保持对现有`SubmitPictureForgeTask`零侵入；沿用积分扣减、事务、退款、进度事件等已有能力。
- 以最小改动交付，预留向事件驱动编排（路径B）的平滑演进空间。

### 范围
- 仅实现“两步固定链”：图片 → 视频。
- 第二步工作流ID固定从配置项读取：`picture_to_video_guide_workflow_id`（`internal/constants/config_keys.go`）。
- 新增`chain_id`并在创建接口返回。

### 名词
- 任务链（Task Chain）：描述多阶段任务依赖关系与编排的最小单元。
- 步骤（Step）：链中的一个任务阶段。本期仅第二步为视频生成。

### 总体设计
1) 前端点击「Create Video」→ 调用新API：`SubmitPictureTaskChain`。
2) 若第一步图片任务尚未创建，则先提交图片任务（复用`SubmitPictureForgeTask`），并写入一条任务链记录（记录第二步工作流与参数映射）。
3) 执行器将第一步任务置为 COMPLETED 后，在“统一完成路径Hook”调用`TaskChainService.OnTaskCompleted`，读取链并提交第二步视频任务（同样走`SubmitPictureForgeTask`）。
4) 第二步完成后，链状态置为 DONE。若第二步提交或执行失败，仅退款第二步，并将链置为 FAILED，不影响第一步成果。

### 数据模型
为保持主表稳定，单独建立轻量链表（仅一张表足够两步固定链）。

表：`va_task_chain`
- `chain_id` varchar(64) PK
- `root_task_id` varchar(64) 第一阶段任务ID
- `user_id` varchar(64)
- `project_id` varchar(64)
- `status` tinyint 枚举：0-PENDING, 1-RUNNING, 2-DONE, 3-FAILED
- `current_step` tinyint 当前推进到第几步（0/1/2）
- `config_json` text 链配置（含第二步workflow_id与参数映射）
- `next_task_id` varchar(64) 第二步任务ID（创建后回填）
- `created_at`/`updated_at`

建议索引：(`user_id`, `created_at`), (`root_task_id`), (`next_task_id`)

config_json（两步固定链示例）
```json
{
  "steps": [
    {
      "index": 1,
      "type": "image",
      "workflow_id": "<由前端或服务确定>"
    },
    {
      "index": 2,
      "type": "video",
      "workflow_id": "${config.picture_to_video_guide_workflow_id}",
      "param_mapping": {
        "LoadImage1": "$.ResultURL",
        "prompt": "$.meta.prompt?"  
      }
    }
  ]
}
```

说明：本期只使用`steps[1]`提交第二步；`param_mapping`使用JSONPath从第一步`task.ResultJSON`提取值。

### API 设计（proto 草案）
仅列关键字段，具体定义放在`internal/va_interface`后续补充。

1) 提交链：`SubmitPictureTaskChain`
- Request
  - `request_header`
  - `first_workflow_id` 可选；如前端直接选定图片工作流
  - `first_workflow_input` 可选；与现有`WorkflowInput`一致
  - `first_task_id` 可选；若已存在图片任务，可直接传入用作第一步
  - `prompt` 可选；可透传给第二步
- Response
  - `chain_id`
  - `first_task_id`

2) 可选：`GetTaskChain(chain_id)`（便于Loading页查询链的各步状态）。首期可不做，仅依赖现有任务事件和任务结果查询。

### 编排与触发点
- 统一完成路径Hook：在任务被标记为`COMPLETED`并持久化后，调用：
  - `TaskChainService.OnTaskCompleted(ctx, task)`
  - 位置建议：在现有执行器完成逻辑末尾或`updateTaskProgressAndSendEvent`之后（避免影响主路径）。
- 行为：
  - 根据`task.TaskID`查询是否存在`va_task_chain.root_task_id=task_id`且`status`为`PENDING/RUNNING, current_step=1`。
  - 若命中：解析`config_json`，通过JSONPath从`task.ResultJSON`取图URL，构造`SubmitPictureForgeTaskRequest`提交第二步视频任务（`workflow_id`来自配置键`picture_to_video_guide_workflow_id`）。
  - 将`next_task_id`写回链表，`status`→RUNNING，`current_step`→2。
  - 幂等：基于`SETNX visionai:task_chain:submit:<chain_id>:2`防重复提交。

### 参数映射
- 使用简单模板 + JSONPath：
  - `LoadImage1 = $.ResultURL`（必需）
  - `prompt`（可选，若前端传入则覆盖）
- 失败策略：若JSONPath解析失败，记日志并置链为FAILED（不影响第一步成果）。

### 计费与退款
- 各步骤独立计费：
  - 第一步图片：沿用现有`DeductCredits`与失败退款逻辑。
  - 第二步视频：提交前按工作流积分扣减；若提交失败或执行失败，通过`RefundCredits`仅退第二步消耗。
- 链级不做事务性回滚（避免破坏已完成产物）。

### 幂等与重试
- 提交锁：`SETNX visionai:task_chain:submit:<chain_id>:<step>`。
- 提交重试：若二步提交失败，可在`OnTaskCompleted`内部按可重试错误做指数退避；或提供`RetryChainStep(chain_id, step)`后台接口。
- 第二步执行重试：沿用任务级重试已有能力（如执行器的Provider重试）。

### 事件与观测
- 复用现有`TaskProgressEvent`（`WATCH_EVENT_TYPE_PICTURE_TASK_PROGRESS`）。
- 指标建议：
  - `task_chain_created_total`、`task_chain_completed_total`、`task_chain_failed_total`、`task_chain_step_retry_total`。
- 日志：所有链路日志带`chain_id`、`step`、`first_task_id/next_task_id`。

### 迁移与上线步骤
1) 新建表`va_task_chain`（新增SQL迁移脚本到`assets/migrations`）。
2) 新增`TaskChainService`与DAO：
   - `CreateChain(ctx, firstTaskId, cfg)`
   - `OnTaskCompleted(ctx, task)`
   - `MarkNextTask(ctx, chainID, nextTaskID)`
   - 基于`redis`的提交锁。
3) 新增API：`SubmitPictureTaskChain`：
   - 若`first_task_id`为空：提交第一步图片任务并创建链；否则仅创建链并等待触发。
4) 在任务完成公共路径调用`TaskChainService.OnTaskCompleted`。
5) 验证：
   - 用内置图/用户上传图两条路径测试；
   - 人脸校验/积分不足/第二步提交失败/第二步执行失败的边界；
   - Loading页体验（图片完成后自动生成视频）。

### 与现有代码的对接点（精确落点）
1) 图片任务提交与扣费逻辑保持不变，位于 API 层：
```startLine:endLine:internal/api/picture_forge.go
61:126
```

2) 统一完成路径（优先落点）：当执行器通过通用函数更新状态并派发事件时，在“完成态”追加调用：`TaskChainService.OnTaskCompleted(ctx, task)`。
```startLine:endLine:internal/service/picture_generate/service.go
205:244
```
建议在上述函数内部，当`status == WORKFLOW_TASK_STATUS_COMPLETED`且数据库已持久化后，调用链服务。

3) 对于未走通用函数、在执行器内手动设置完成并直接发送事件的路径，需要追加一个相同的调用（最小插桩）：
— 可灵图生视频执行器完成：
```startLine:endLine:internal/service/picture_generate/kling_image2video_executor.go
429:457
```

— gpt4o 图生图执行器完成（类似，设置 COMPLETED 后发送事件）：
```startLine:endLine:internal/service/picture_generate/gpt4o_image2image_executor.go
279:308
```

说明：为保证“一次且仅一次”触发链下一步，`TaskChainService.OnTaskCompleted`内部使用`SETNX visionai:task_chain:submit:<chain_id>:2`做幂等控制。

### 安全与权限
- `chain`与任务必须校验`user_id/project_id`一致。
- API 仅允许当前用户对其自己的链发起与查询。

### 风险与回退
- 风险：提交二步映射失败/Provider异常、双写不一致。
- 回退：关闭「Create Video」入口或下线`OnTaskCompleted`调用，链表保留不影响已有任务。

### 演进到路径B（事件驱动编排）
- 将`OnTaskCompleted`从进程内Hook迁移为订阅`TaskProgressEvent`（完成态）的消费者`TaskChainOrchestrator`。
- 表与API保持不变，仅改变触发方式。

### 开发清单
- [ ] SQL：`va_task_chain`表迁移脚本
- [ ] Service/DAO：`TaskChainService`（`CreateChain`/`OnTaskCompleted`/`MarkNextTask`）
- [ ] API：`SubmitPictureTaskChain`
- [ ] Hook：在任务完成后调用`OnTaskCompleted`
- [ ] 配置读取：`picture_to_video_guide_workflow_id`
- [ ] 指标与日志埋点
- [ ] 用例：正常链路、失败与退款、幂等

### 自我评审（可维护性/可扩展性）
- 职责清晰：提交/扣费与编排分离；主任务逻辑零侵入。
- 幂等完备：基于链+步的提交锁；失败只影响当前步。
- 可观测：事件复用、指标补充；日志上下文化。
- 易演进：切换到事件驱动无需改表与API。


