# 获取生图模型列表 API

## 接口信息

### gRPC 接口
- **服务**: `PictureForgeService`
- **方法**: `ListImageGenerationModels`

### HTTP/REST 接口
- **URL**: `/v1/pictureforge/list_image_generation_models`
- **Method**: `POST`
- **认证**: ❌ **无需用户认证** （公开接口）

## 请求参数

```protobuf
message ListImageGenerationModelsRequest {
  RequestHeader request_header = 1;  // 可选，无需包含 user_id 和 access_token
}
```

### 请求示例 (HTTP)

```json
{
  "request_header": {
    "device": {
      "os": "web",
      "language": "zh-CN"
    }
  }
}
```

**注意**：
- ✅ 该接口**不需要用户登录**，可以在用户未登录状态下调用
- ✅ `request_header` 中的 `user_id` 和 `access_token` 字段**非必填**
- ✅ 建议传入 `device` 信息（OS、语言等）以便服务端统计

## 响应参数

```protobuf
message ListImageGenerationModelsResponse {
  ResponseHeader response_header = 1;
  repeated ImageGenerationModelInfo models = 2;
}

message ImageGenerationModelInfo {
  string model_name = 1;           // 模型名称（如 flux-dev, sd3, wanx）
  string display_name = 2;         // 显示名称（如 "Flux Dev", "Stable Diffusion 3"）
  string description = 3;          // 模型描述
  string icon = 4;                 // 模型图标URL（可选）
  bool support_t2i = 5;            // 是否支持文生图
  bool support_i2i = 6;            // 是否支持图生图
  int32 credit_points = 7;         // 积分消耗
  int32 default_width = 8;         // 默认宽度
  int32 default_height = 9;        // 默认高度
  repeated string support_qualities = 10;  // 支持的质量选项
  int32 max_images = 11;           // 最大生成图片数
}
```

### 响应示例 (HTTP)

```json
{
  "response_header": {
    "code": "SUCCESS",
    "msg": "success",
    "response_time_ms": 1234567890
  },
  "models": [
    {
      "model_name": "flux-dev",
      "display_name": "Flux Dev",
      "description": "Flux Dev 模型，适合高质量图片生成",
      "icon": "",
      "support_t2i": true,
      "support_i2i": true,
      "credit_points": 10,
      "default_width": 1024,
      "default_height": 1024,
      "support_qualities": ["standard", "hd"],
      "max_images": 4
    },
    {
      "model_name": "flux-schnell",
      "display_name": "Flux Schnell",
      "description": "Flux Schnell 模型，快速生成",
      "icon": "",
      "support_t2i": true,
      "support_i2i": true,
      "credit_points": 5,
      "default_width": 1024,
      "default_height": 1024,
      "support_qualities": ["standard"],
      "max_images": 4
    },
    {
      "model_name": "sd3",
      "display_name": "Stable Diffusion 3",
      "description": "Stable Diffusion 3 模型",
      "icon": "",
      "support_t2i": true,
      "support_i2i": true,
      "credit_points": 8,
      "default_width": 1024,
      "default_height": 1024,
      "support_qualities": ["standard", "hd"],
      "max_images": 4
    },
    {
      "model_name": "wanx",
      "display_name": "通义万相",
      "description": "阿里通义万相模型",
      "icon": "",
      "support_t2i": true,
      "support_i2i": false,
      "credit_points": 6,
      "default_width": 1024,
      "default_height": 1024,
      "support_qualities": ["standard"],
      "max_images": 1
    }
  ]
}
```

## 使用说明

### 1. 模型配置来源

模型列表从**数据库表 `va_image_generation_model`** 查询：
- **表名**：`va_image_generation_model`
- **数据库**：`test_zhao`
- **查询条件**：`enabled = 1 AND project_id = 'visionai'`
- **排序规则**：按 `sort` 字段降序，`id` 升序

#### 数据库表结构

```sql
CREATE TABLE `va_image_generation_model` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `model_name` varchar(64) NOT NULL COMMENT '模型名称',
  `display_name` varchar(128) NOT NULL COMMENT '显示名称',
  `description` varchar(500) DEFAULT NULL COMMENT '模型描述',
  `icon` varchar(255) DEFAULT NULL COMMENT '模型图标URL',
  `provider` varchar(64) DEFAULT NULL COMMENT '提供商',
  `support_t2i` tinyint(1) NOT NULL DEFAULT '1' COMMENT '支持文生图',
  `support_i2i` tinyint(1) NOT NULL DEFAULT '0' COMMENT '支持图生图',
  `credit_points` int(11) NOT NULL DEFAULT '0' COMMENT '积分消耗',
  `default_width` int(11) NOT NULL DEFAULT '1024' COMMENT '默认宽度',
  `default_height` int(11) NOT NULL DEFAULT '1024' COMMENT '默认高度',
  `support_qualities` varchar(255) DEFAULT NULL COMMENT '质量选项(JSON)',
  `max_images` int(11) NOT NULL DEFAULT '1' COMMENT '最大图片数',
  `enabled` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否启用',
  `sort` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `project_id` varchar(64) NOT NULL DEFAULT 'visionai' COMMENT '项目ID',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_model_name` (`model_name`,`project_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

#### 添加新模型

在数据库中插入新记录即可：

```sql
INSERT INTO `va_image_generation_model`
(`model_name`, `display_name`, `description`, `icon`, `provider`,
 `support_t2i`, `support_i2i`, `credit_points`, `default_width`, `default_height`,
 `support_qualities`, `max_images`, `enabled`, `sort`, `project_id`)
VALUES
('new-model', '新模型', '这是一个新的生图模型', 'https://example.com/icon.png', 'comfyui',
 1, 1, 15, 1024, 1024, '["standard","hd","ultra"]', 4, 1, 110, 'visionai');
```

#### 禁用/启用模型

```sql
-- 禁用模型
UPDATE `va_image_generation_model` SET `enabled` = 0 WHERE `model_name` = 'flux-dev';

-- 启用模型
UPDATE `va_image_generation_model` SET `enabled` = 1 WHERE `model_name` = 'flux-dev';
```

### 2. Web 端调用示例

```javascript
const response = await fetch('https://your-api.com/v1/pictureforge/list_image_generation_models', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer YOUR_TOKEN'
  },
  body: JSON.stringify({
    request_header: {
      user_id: 'user_123',
      access_token: 'your_access_token',
      device: {
        os: 'web',
        language: 'zh-CN'
      }
    }
  })
});

const data = await response.json();

// 展示模型列表
data.models.forEach(model => {
  console.log(`${model.display_name}: ${model.credit_points} 积分`);
  console.log(`  支持: ${model.support_t2i ? '文生图' : ''} ${model.support_i2i ? '图生图' : ''}`);
  console.log(`  质量选项: ${model.support_qualities.join(', ')}`);
});
```

### 3. 前端 UI 展示建议

根据返回的模型信息，可以这样展示：

```html
<div class="model-selector">
  <h3>选择生图模型</h3>
  <div class="model-list">
    <!-- 遍历 models 数组 -->
    <div class="model-card">
      <img src="${model.icon}" alt="${model.display_name}">
      <h4>${model.display_name}</h4>
      <p>${model.description}</p>
      <div class="model-info">
        <span class="credits">${model.credit_points} 积分</span>
        <span class="capabilities">
          ${model.support_t2i ? '✓ 文生图' : ''}
          ${model.support_i2i ? '✓ 图生图' : ''}
        </span>
      </div>
      <div class="model-specs">
        <span>默认尺寸: ${model.default_width}×${model.default_height}</span>
        <span>质量: ${model.support_qualities.join(', ')}</span>
        <span>最多生成: ${model.max_images} 张</span>
      </div>
    </div>
  </div>
</div>
```

## 字段说明

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `model_name` | string | 模型标识符，用于提交任务时指定模型 |
| `display_name` | string | 显示给用户的模型名称 |
| `description` | string | 模型描述，介绍模型特点 |
| `icon` | string | 模型图标 URL（当前为空，后续可配置） |
| `support_t2i` | bool | 是否支持文生图（Text-to-Image） |
| `support_i2i` | bool | 是否支持图生图（Image-to-Image） |
| `credit_points` | int32 | 使用该模型消耗的默认积分 |
| `default_width` | int32 | 默认生成图片宽度（像素） |
| `default_height` | int32 | 默认生成图片高度（像素） |
| `support_qualities` | []string | 支持的质量选项（如 standard, hd, ultra） |
| `max_images` | int32 | 该模型一次最多可生成的图片数量 |

## 注意事项

1. **模型可用性**：接口只返回 `enabled = 1` 的模型
2. **动态配置**：模型列表存储在数据库中，通过 SQL 可以实时调整，无需重启服务
3. **项目隔离**：不同项目（project_id）可以配置不同的模型列表
4. **排序控制**：通过 `sort` 字段控制模型展示顺序，值越大排序越靠前
5. **质量选项**：`support_qualities` 字段为 JSON 数组字符串格式，如 `["standard","hd"]`
6. **版本兼容**：建议前端做好容错处理，某些字段（如 icon）可能为空
7. **数据库迁移**：运行 `migrate up` 会自动创建表并插入默认数据

## 数据迁移

系统已包含数据库迁移文件：
- **迁移文件**：`assets/migrations/20260202190205_create_image_generation_model.up.sql`
- **回滚文件**：`assets/migrations/20260202190205_create_image_generation_model.down.sql`

运行迁移：
```bash
# 执行迁移（创建表和初始数据）
migrate -path ./assets/migrations -database "mysql://user:pass@tcp(host:port)/test_zhao" up

# 回滚迁移（删除表）
migrate -path ./assets/migrations -database "mysql://user:pass@tcp(host:port)/test_zhao" down 1
```

## 相关接口

- `SubmitPictureForgeTask`: 提交生图任务（需要指定 model_name）
- `GetPictureForgeTaskResult`: 获取生图结果
- `EventWatch`: 监听生图进度推送
