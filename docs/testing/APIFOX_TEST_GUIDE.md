# Apifox 测试指南 - ListUserPictureForgeTask 接口

## 前置准备

### 1. 确保服务已启动并生成 Proto 代码

```bash
cd /Users/zhaojiabo/Documents/trae_projects/genioAI_server

# 1. 重新生成 Proto 代码
./scripts/generate-proto.sh --clean --verify

# 2. 启动服务
go run cmd/main.go
# 或
make run
```

### 2. 准备测试数据

在数据库中插入不同模式的测试任务：

```sql
-- 1. 插入一个 Workflow 模式的任务
INSERT INTO picture_task (
    task_id, project_id, user_id, workflow_id,
    status, task_mode, model_name, user_prompt,
    created_at, updated_at
) VALUES (
    'task_workflow_test_001',
    'project_test',
    'user_test_123',
    'workflow_flux_01',
    20, -- PROCESSING
    0,  -- Workflow 模式
    '',
    '',
    NOW(),
    NOW()
);

-- 2. 插入一个模型直连模式的任务
INSERT INTO picture_task (
    task_id, project_id, user_id, workflow_id,
    status, task_mode, model_name, user_prompt, negative_prompt,
    created_at, updated_at
) VALUES (
    'task_model_direct_test_001',
    'project_test',
    'user_test_123',
    'model_direct_flux-dev_xxx',
    100, -- COMPLETED
    1,   -- 模型直连模式
    'flux-dev',
    'a beautiful sunset over mountains',
    'blurry, low quality, distorted',
    NOW(),
    NOW()
);

-- 3. 再插入一个模型直连任务（不同模型）
INSERT INTO picture_task (
    task_id, project_id, user_id, workflow_id,
    status, task_mode, model_name, user_prompt,
    created_at, updated_at
) VALUES (
    'task_model_direct_test_002',
    'project_test',
    'user_test_123',
    'model_direct_sd3_yyy',
    20, -- PROCESSING
    1,  -- 模型直连模式
    'sd3',
    'cyberpunk city at night',
    NOW(),
    NOW()
);
```

## Apifox 配置步骤

### 步骤 1: 创建接口

1. **打开 Apifox** → 选择你的项目
2. **点击「新建接口」**
3. **填写基本信息**：
   - **接口名称**: `ListUserPictureForgeTask - 获取用户图片任务列表`
   - **请求方法**: `POST`
   - **接口路径**: `/v1/pictureforge/list_user_picture_forge_task`
   - **分组**: PictureForge Service

### 步骤 2: 配置请求头

在「请求参数」→「Header」中添加：

| 参数名 | 示例值 | 必填 | 说明 |
|--------|--------|------|------|
| Content-Type | application/json | 是 | 固定值 |
| Authorization | Bearer YOUR_TOKEN | 否 | 如需要鉴权 |

### 步骤 3: 配置请求体（Body）

选择「Body」→「JSON」，输入以下内容：

## 测试用例

### 用例 1: 查询所有任务（不筛选）

```json
{
  "request_header": {
    "user_id": "user_test_123",
    "device": {
      "os": "iOS",
      "os_version": "17.0",
      "device_id": "test_device_001"
    },
    "app": {
      "app_version": "1.0.0",
      "app_package_name": "com.example.app"
    }
  },
  "page_common": {
    "page": 1,
    "page_size": 20
  },
  "workflow_theme": "",
  "workflow_kind_type": 0,
  "task_mode": 0
}
```

**预期结果**: 返回所有任务（包括 Workflow 和模型直连模式）

---

### 用例 2: 仅查询 Workflow 模式任务

```json
{
  "request_header": {
    "user_id": "user_test_123",
    "device": {
      "os": "iOS",
      "os_version": "17.0",
      "device_id": "test_device_001"
    },
    "app": {
      "app_version": "1.0.0",
      "app_package_name": "com.example.app"
    }
  },
  "page_common": {
    "page": 1,
    "page_size": 20
  },
  "task_mode": 1
}
```

**预期结果**: 只返回 `task_mode = 0` 的任务

**验证点**:
- ✅ `picture_forge_infos` 中所有项的 `is_model_direct` 为 `false`
- ✅ `model_name` 字段为空
- ✅ `negative_prompt` 字段为空

---

### 用例 3: 仅查询模型直连模式任务

```json
{
  "request_header": {
    "user_id": "user_test_123",
    "device": {
      "os": "iOS",
      "os_version": "17.0",
      "device_id": "test_device_001"
    },
    "app": {
      "app_version": "1.0.0",
      "app_package_name": "com.example.app"
    }
  },
  "page_common": {
    "page": 1,
    "page_size": 20
  },
  "task_mode": 2
}
```

**预期结果**: 只返回 `task_mode = 1` 的任务

**验证点**:
- ✅ `picture_forge_infos` 中所有项的 `is_model_direct` 为 `true`
- ✅ `model_name` 有值（如 `flux-dev`, `sd3`）
- ✅ `prompt` 包含用户输入的 prompt
- ✅ `negative_prompt` 可能有值

---

### 用例 4: 结合其他筛选条件

```json
{
  "request_header": {
    "user_id": "user_test_123",
    "device": {
      "os": "iOS",
      "os_version": "17.0",
      "device_id": "test_device_001"
    },
    "app": {
      "app_version": "1.0.0",
      "app_package_name": "com.example.app"
    }
  },
  "page_common": {
    "page": 1,
    "page_size": 10
  },
  "workflow_kind_type": 1,  // 仅图片类型
  "task_mode": 2             // 仅模型直连模式
}
```

**预期结果**: 返回模型直连模式 + 图片类型的任务

---

### 用例 5: 分页测试

```json
{
  "request_header": {
    "user_id": "user_test_123",
    "device": {
      "os": "iOS",
      "os_version": "17.0",
      "device_id": "test_device_001"
    },
    "app": {
      "app_version": "1.0.0",
      "app_package_name": "com.example.app"
    }
  },
  "page_common": {
    "page": 1,
    "page_size": 1
  },
  "task_mode": 0
}
```

**预期结果**:
- 返回 1 条记录
- `page_common.total` 显示总数

## 预期响应格式

### 成功响应示例

```json
{
  "response_header": {
    "code": 0,
    "msg": "success",
    "response_time_ms": 1234567890
  },
  "page_common": {
    "page": 1,
    "page_size": 20,
    "total": 3
  },
  "picture_forge_infos": [
    {
      "task_id": "task_model_direct_test_001",
      "user_id": "user_test_123",
      "task_status": 100,
      "task_progress": 100,
      "is_model_direct": true,
      "model_name": "flux-dev",
      "prompt": "a beautiful sunset over mountains",
      "negative_prompt": "blurry, low quality, distorted",
      "workflow": null,
      "kind_type": 1,
      "created_at": "2026-02-03T10:00:00Z"
    },
    {
      "task_id": "task_model_direct_test_002",
      "user_id": "user_test_123",
      "task_status": 20,
      "task_progress": 45,
      "is_model_direct": true,
      "model_name": "sd3",
      "prompt": "cyberpunk city at night",
      "negative_prompt": "",
      "workflow": null,
      "kind_type": 1,
      "created_at": "2026-02-03T10:05:00Z"
    },
    {
      "task_id": "task_workflow_test_001",
      "user_id": "user_test_123",
      "task_status": 20,
      "task_progress": 30,
      "is_model_direct": false,
      "model_name": "",
      "prompt": "",
      "negative_prompt": "",
      "workflow": {
        "workflow_id": "workflow_flux_01",
        "title": "Flux Workflow"
      },
      "kind_type": 1,
      "created_at": "2026-02-03T09:55:00Z"
    }
  ]
}
```

## 关键验证点

### 1. task_mode 参数

| task_mode 值 | 说明 | 预期行为 |
|--------------|------|----------|
| 0 (或不传) | 全部模式 | 返回所有任务 |
| 1 | Workflow 模式 | 只返回 `task_mode=0` 的任务 |
| 2 | 模型直连模式 | 只返回 `task_mode=1` 的任务 |

### 2. 响应字段验证

**模型直连模式任务** (`is_model_direct: true`):
- ✅ `model_name` 有值
- ✅ `prompt` 包含用户输入
- ✅ `negative_prompt` 可能有值
- ✅ `workflow` 可能为 null

**Workflow 模式任务** (`is_model_direct: false`):
- ✅ `model_name` 为空
- ✅ `prompt` 可能为空（或工具生成的）
- ✅ `negative_prompt` 为空
- ✅ `workflow` 有值

## 常见问题排查

### Q1: 返回的任务为空
**检查**:
- 数据库中是否有对应 `user_id` 的任务
- `task_mode` 筛选是否正确
- 任务是否被软删除 (`deleted_at IS NOT NULL`)

### Q2: 模型直连字段为空
**检查**:
- 数据库表是否已执行迁移（字段是否存在）
- Proto 代码是否重新生成
- 服务是否重新编译和启动

### Q3: task_mode 参数不生效
**检查**:
- Proto 枚举值是否正确映射
- 代码中 `task_mode` 参数是否正确传递到 DAO 层
- 数据库查询语句中是否包含 `task_mode` 条件

## Apifox 测试脚本

你可以在 Apifox 的「后置操作」中添加自动化断言：

```javascript
// 测试脚本示例
pm.test("Status code is 200", function () {
    pm.response.to.have.status(200);
});

pm.test("Response has success code", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData.response_header.code).to.eql(0);
});

pm.test("Task mode filter works", function () {
    var jsonData = pm.response.json();
    var taskMode = pm.request.body.task_mode;

    if (taskMode === 2) {
        // 验证只返回模型直连模式
        jsonData.picture_forge_infos.forEach(function(task) {
            pm.expect(task.is_model_direct).to.eql(true);
            pm.expect(task.model_name).to.not.be.empty;
        });
    } else if (taskMode === 1) {
        // 验证只返回 Workflow 模式
        jsonData.picture_forge_infos.forEach(function(task) {
            pm.expect(task.is_model_direct).to.eql(false);
        });
    }
});
```

## 快速测试命令（使用 curl）

如果你想快速测试，也可以使用 curl：

```bash
# 测试用例 1: 查询所有任务
curl -X POST http://localhost:8080/v1/pictureforge/list_user_picture_forge_task \
  -H "Content-Type: application/json" \
  -d '{
    "request_header": {
      "user_id": "user_test_123",
      "device": {"os": "iOS"},
      "app": {"app_version": "1.0.0"}
    },
    "page_common": {"page": 1, "page_size": 20},
    "task_mode": 0
  }'

# 测试用例 2: 只查询模型直连模式
curl -X POST http://localhost:8080/v1/pictureforge/list_user_picture_forge_task \
  -H "Content-Type: application/json" \
  -d '{
    "request_header": {
      "user_id": "user_test_123",
      "device": {"os": "iOS"},
      "app": {"app_version": "1.0.0"}
    },
    "page_common": {"page": 1, "page_size": 20},
    "task_mode": 2
  }'
```

## 性能测试建议

1. **并发测试**: 使用 Apifox 的「场景测试」功能，模拟多用户并发请求
2. **大数据量测试**: 插入 1000+ 条任务，测试分页性能
3. **索引效果**: 对比添加 `idx_task_mode` 索引前后的查询性能

---

## 测试检查清单

- [ ] Proto 代码已重新生成
- [ ] 数据库迁移已执行
- [ ] 服务已重启
- [ ] 测试数据已准备
- [ ] Apifox 接口已配置
- [ ] 所有测试用例已执行
- [ ] 响应格式符合预期
- [ ] 筛选功能正常工作
- [ ] 向后兼容性验证通过

祝测试顺利！🎉
