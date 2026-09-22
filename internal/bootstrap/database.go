package bootstrap

import (
	"github.com/go-redis/redis"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
)

// InitDatabase 初始化数据库连接
func InitDatabase() (*gorm.DB, error) {
	return db.GetDB(), nil
}

// InitRedis 初始化Redis连接
func InitRedis() *redis.Client {
	return db.GetRedis()
}

// InitMongo 初始化MongoDB连接
func InitMongo() *mongo.Client {
	return db.GetMongo()
}

// InitRepositories 初始化所有 DAO 仓库
func InitRepositories(db *gorm.DB) *dao.Repositories {
	return dao.NewRepositories(db)
}
