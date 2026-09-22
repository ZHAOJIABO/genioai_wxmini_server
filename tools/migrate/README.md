### 生成数据库迁移文件
 注意修改版本号和描述
``` shell
go run tools/migrate/main.go -v 1.9.5 -desc create_prompt_index -dir ./assets/migrations
```
