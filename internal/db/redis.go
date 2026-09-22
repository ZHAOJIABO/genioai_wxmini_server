package db

import (
	"github.com/go-redis/redis"

	"va_visionai_server/conf"
)

var redisClient *redis.Client

func RedisInit() error {
	if redisClient == nil {
		redisClient = redis.NewClient(&redis.Options{
			PoolSize: 1000,
			Addr:     conf.GlobalConfig.Redis.Addr,
			Password: conf.GlobalConfig.Redis.AuthInfo,
			DB:       int(conf.GlobalConfig.Redis.Db),
		})
	}
	return redisClient.Ping().Err()
}

func GetRedis() *redis.Client {
	return redisClient
}
