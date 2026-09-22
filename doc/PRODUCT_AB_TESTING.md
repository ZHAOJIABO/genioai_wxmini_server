# 商品 AB 测试系统

## 概述

商品 AB 测试系统允许根据用户特征动态调整商品展示策略，支持商品过滤、属性修改等功能。系统采用基于场景（groupID）的商品组织方式，结合灵活的实验配置，实现精细化的商品展示控制。

## 系统架构

### 核心组件

1. **场景商品组（groupID）**：按业务场景组织商品
   - `main_page`：主页展示
   - `upgrade_popup`：升级弹窗
   - `payment_page`：支付页面
   - 自定义场景...

2. **实验引擎**：基于 JSON 配置的 A/B 测试规则
   - 用户匹配规则
   - 实验优先级
   - 商品策略（包含、排除、修改）

### API 接口

```
GET /api/products?groupID=<scene_name>
```

## 配置管理

### 配置键格式

```
<projectID>:product_experiments
```

例如：
- `project_a:product_experiments`
- `project_b:product_experiments`

### 配置存储

实验配置存储在 `va_config` 表中，支持项目级别的独立配置。

## 实验配置

### 基本结构

```json
[
  {
    "name": "实验名称",
    "priority": 优先级数字,
    "rule": {
      "user_id_suffix": [用户ID后缀数组]
    },
    "payload": {
      "inclusions": ["包含的商品ID"],
      "exclusions": ["排除的商品ID"],
      "modifications": {
        "商品ID": {
          "字段名": "新值"
        }
      }
    }
  }
]
```

### 字段说明

#### Experiment（实验）
- `name`：实验名称，用于标识和日志记录
- `priority`：优先级，数字越大优先级越高
- `rule`：用户匹配规则
- `payload`：实验策略配置

#### Rule（规则）
- `user_id_suffix`：用户ID后缀匹配数组
  - 例如：`[0, 1, 2]` 表示用户ID以0、1、2结尾的用户

#### Payload（策略）
执行顺序：Inclusions → Exclusions → Modifications

1. **Inclusions（包含）**
   - 如果指定，则只保留这些商品ID
   - 为空则不限制

2. **Exclusions（排除）**
   - 从商品列表中移除指定的商品ID

3. **Modifications（修改）**
   - 对指定商品的属性进行覆盖修改
   - 键名必须匹配 Go 结构体字段名（如 `Price`、`PriceLabel`）

## 使用示例

### 示例1：隐藏周卡实验

```json
{
  "name": "hide_weekly_for_new_users",
  "priority": 10,
  "rule": {
    "user_id_suffix": [0, 1, 2, 3, 4]
  },
  "payload": {
    "exclusions": ["weekly"]
  }
}
```

**效果**：用户ID以0-4结尾的用户不会看到周卡商品。

### 示例2：新用户专享实验

```json
{
  "name": "premium_pricing_test",
  "priority": 20,
  "rule": {
    "user_id_suffix": [5, 6, 7]
  },
  "payload": {
    "inclusions": ["monthly", "yearly"],
    "modifications": {
      "monthly": {
        "Price": 19.99,
        "PriceLabel": "$19.99/mo",
        "Name": "Premium Monthly"
      }
    }
  }
}
```

**效果**：用户ID以5-7结尾的用户只看到月卡和年卡，且月卡价格调整为$19.99。

### 示例3：价格测试实验

```json
{
  "name": "discount_experiment",
  "priority": 15,
  "rule": {
    "user_id_suffix": [8, 9]
  },
  "payload": {
    "modifications": {
      "yearly": {
        "Price": 79.99,
        "PriceLabel": "$79.99/yr",
        "Discount": 0.33,
        "DiscountLabel": "Limited Time: 33% OFF!"
      }
    }
  }
}
```

**效果**：用户ID以8-9结尾的用户看到年卡特价$79.99，显示33%折扣标签。

## 实验匹配逻辑

### 优先级规则

1. 按优先级降序排序实验
2. 遍历实验，找到第一个匹配用户的实验
3. 应用该实验的策略
4. 如果没有匹配的实验，返回基础商品列表

### 用户匹配

目前支持基于用户ID后缀的匹配：
- 提取用户ID的最后一位数字
- 检查是否在实验规则的后缀列表中

例如：
- 用户ID `user12345` → 后缀 `5`
- 规则 `[5, 6, 7]` → 匹配成功

## 性能优化

### 缓存机制

- **Redis 缓存**：实验配置缓存5分钟
- **缓存键格式**：`cache:product_experiments:<projectID>`
- **缓存更新**：配置变更后自动更新

### 容错机制

- 配置获取失败时返回基础商品列表
- 实验应用失败时降级到基础商品列表
- 详细的错误日志记录

## 监控与日志

### 关键日志

1. **实验匹配**：记录用户匹配到的实验
2. **策略应用**：记录包含、排除、修改的详细信息
3. **错误处理**：记录配置获取和应用失败

### 日志示例

```
INFO: user matched experiment {"userID": "user1235", "experiment": "premium_pricing_test"}
DEBUG: applied inclusions filter {"inclusions": ["monthly", "yearly"], "before": 3, "after": 2}
DEBUG: applied modifications {"modifiedCount": 1}
```

## 部署配置

### 数据库配置

在 `va_config` 表中添加实验配置：

```sql
INSERT INTO va_config (config_key, config_value, project_id) 
VALUES ('project_a:product_experiments', '[实验配置JSON]', 'project_a');
```

### 环境变量

无需额外环境变量，使用现有的数据库和Redis配置。

## 最佳实践

### 实验设计

1. **明确目标**：每个实验应有明确的业务目标
2. **用户分组**：合理分配用户ID后缀，确保样本均匀
3. **优先级管理**：重要实验设置更高优先级
4. **渐进式发布**：从小范围用户开始测试

### 配置管理

1. **版本控制**：保留实验配置的历史版本
2. **测试验证**：在测试环境充分验证配置
3. **监控观察**：密切关注实验效果和系统性能
4. **及时清理**：删除过期的实验配置

### 故障排查

1. **检查配置**：确认实验配置格式正确
2. **验证用户匹配**：确认用户ID后缀匹配逻辑
3. **查看日志**：分析详细的实验执行日志
4. **缓存清理**：必要时清理Redis缓存

## 扩展功能

### 未来规划

1. **更多匹配规则**：支持地理位置、设备类型等
2. **实验分析**：内置实验效果分析工具
3. **可视化配置**：提供Web界面管理实验
4. **自动化测试**：集成A/B测试效果评估

### 自定义扩展

系统设计支持灵活扩展：
- 新增匹配规则类型
- 扩展商品属性修改范围
- 集成外部分析工具 