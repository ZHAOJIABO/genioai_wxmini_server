# pay_usersubscribe_sync

这是一个轻量级数据库同步工具，用于将 A 数据库的 pay_user_subscription 表增量同步到 B 数据库。

## 功能特点

- 自动增量同步，每 5 分钟执行一次同步检查
- 使用事务确保同步过程的一致性
- 使用指数退避算法进行重试机制，最多重试 5 次
- 自动记录同步状态，避免重复同步
- 不同步 ID 字段，由目标库自动生成
- 在异常情况下会自动回滚，保证数据完整性

## 配置说明

配置项在 `main.go` 文件的常量部分：

```go
const (
    // 源、目标数据库 DSN（如需要改动配置，请修改这里）
    srcDSN     = "src_user:src_pass@tcp(a.com:3306)/src_db?parseTime=true&charset=utf8mb4"
    dstDSN     = "dst_user:dst_pass@tcp(b.com:3306)/dst_db?parseTime=true&charset=utf8mb4"
    tableName  = "pay_user_subscription"
    stateTable = "sync_state"

    batchSize    = 1000              // 每批同步的最大行数
    syncInterval = 5 * time.Minute   // 同步间隔
    maxRetries   = 5                 // 最大重试次数
)
```

## 使用方法

1. 在部署前，修改 `main.go` 中的数据库连接信息
2. 直接运行该二进制文件即可启动同步进程
3. 该工具将作为容器的一部分打包在镜像中，位于 `/application/bin/pay_usersubscribe_sync`

## 监控说明

该工具会将其运行状态通过日志输出，包括：
- 同步状态和进度 
- 错误信息和重试情况
- 每次同步的数据量

若需进一步监控，可以查询目标库的 `sync_state` 表，其中包含最近同步的信息。 