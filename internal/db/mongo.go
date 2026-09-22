package db

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"va_visionai_server/conf"
)

var mongoClient *mongo.Client

func GetMongo() *mongo.Client {
	return mongoClient
}

func InitMongoClient() error {
	// 如果没有配置 MongoDB 地址，跳过初始化
	if conf.GlobalConfig.MongoDB.Addr == "" {
		return nil
	}

	var err error
	if mongoClient == nil {
		mongoAddr := conf.GlobalConfig.MongoDB.Addr
		//if conf.IsLocal() {
		//	mongoAddr += "?authSource=admin"
		//}
		clientOptions := options.Client().ApplyURI(mongoAddr)
		// 连接

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		mongoClient, err = mongo.Connect(ctx, clientOptions)
		if err != nil {
			return err
		}

		// Ping 检查连接
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer pingCancel()
		err = mongoClient.Ping(pingCtx, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func GetCollection(chatCreateDate string) *mongo.Collection {
	config := conf.GlobalConfig.MongoDB

	collectionName := fmt.Sprintf("%s%s", config.Collection, chatCreateDate)
	return mongoClient.Database(config.Db).Collection(collectionName)
}
