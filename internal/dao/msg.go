package dao

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"

	"va_visionai_server/internal/model"
	"va_visionai_server/internal/zlog"
)

type MsgDao struct{}

func NewMsgDao() *MsgDao {
	return &MsgDao{}
}

func (d *MsgDao) AddMessage(collection *mongo.Collection, message *model.Message) error {
	_, err := collection.InsertOne(context.Background(), message)
	return err
}

func (d *MsgDao) GetMessages(collection *mongo.Collection, chatID string, offset, count int32, sortByTimeDesc bool) ([]*model.Message, error) {
	// messageType 4 表示视频
	filter := bson.M{"chatId": chatID, "messageType": bson.M{"$ne": 4}, "isDeleted": bson.M{"$ne": true}}

	findOptions := options.Find()
	if sortByTimeDesc {
		findOptions.SetSort(bson.D{
			{Key: "createTime", Value: -1},
			{Key: "_id", Value: -1},
		})
	}

	findOptions.SetSkip(int64((offset - 1) * count))
	findOptions.SetLimit(int64(count))
	cur, err := collection.Find(context.Background(), filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cur.Close(context.Background())

	var messages []*model.Message
	for cur.Next(context.Background()) {
		var message model.Message
		err := cur.Decode(&message)
		if err != nil {
			return nil, err
		}
		messages = append(messages, &message)
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

func (d *MsgDao) GetMessageByID(collection *mongo.Collection, msgID string) (*model.Message, error) {
	filter := bson.M{"messageId": msgID}
	message := &model.Message{}
	result := collection.FindOne(context.Background(), filter, nil)
	if result.Err() != nil {
		return nil, result.Err()
	}
	if err := result.Decode(&message); err != nil {
		return nil, err
	}

	return message, nil
}

func (d *MsgDao) GetMessagesInCollectionBeforeCursor(
	ctx context.Context,
	collection *mongo.Collection, // 传入要查询的具体 Collection
	chatID string, // 用于精确过滤 Collection 内的消息
	limit int, // 需要获取的消息数量
	beforeTime *time.Time, // 游标时间戳，nil 表示从最新的开始
	beforeMsgID string, // 游标消息 ID，空表示从最新的开始
) ([]*model.Message, error) {
	// 基本过滤条件
	filter := bson.M{
		"chatId":      chatID,                  // 筛选特定聊天的消息
		"messageType": bson.M{"$ne": int32(4)}, // 排除特定类型的消息 (根据你的业务逻辑)
		"isDeleted":   bson.M{"$ne": true},
	}
	if beforeMsgID != "" {
		objectID, err := primitive.ObjectIDFromHex(beforeMsgID)
		if err == nil {
			filter["_id"] = bson.M{"$lt": objectID}
		} else {
			zlog.LogWithContext(ctx).Error("Failed to convert beforeMsgID to ObjectID", zap.String("beforeMsgID", beforeMsgID), zap.Error(err))
		}
	}

	// if beforeTime != nil && beforeMsgID != "" {
	// 	filter["$or"] = []bson.M{
	// 		{"createTime": bson.M{"$lt": *beforeTime}},
	// 		{
	// 			"createTime": *beforeTime,
	// 			"_id":  bson.M{"$lt": objectID},
	// 		},
	// 	}
	// }

	// 设置查询选项
	findOptions := options.Find()

	// 排序：必须按消息创建时间降序，然后按 MessageID (或其他唯一键) 降序或升序作为次要排序，以确保分页稳定
	findOptions.SetSort(bson.D{
		{Key: "_id", Value: -1},
		{Key: "createTime", Value: -1}, // 时间降序 (最新的在前)
		// {Key: "messageId", Value: -1},  // MessageID 降序 (或 -1，取决于 ID 生成和比较规则，确保与 $lt/$gt 逻辑一致)
		// 如果使用 MongoDB 自动生成的 _id 作为次要排序: {Key: "_id", Value: -1}
	})

	// 限制返回数量
	findOptions.SetLimit(int64(limit))

	// 执行查询
	cur, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		// 对于 Find 操作，ErrNoDocuments 不常见，但检查总没错
		if err == mongo.ErrNoDocuments {
			return []*model.Message{}, nil // 没有找到匹配的消息，返回空列表，不是错误
		}
		// 其他数据库错误
		return nil, fmt.Errorf("failed to execute find query: %w", err)
	}
	// 函数结束时确保关闭游标
	defer cur.Close(ctx)

	// 解码结果
	var messages []*model.Message
	if err = cur.All(ctx, &messages); err != nil {
		return nil, fmt.Errorf("failed to decode messages: %w", err)
	}

	// 检查游标处理过程中是否发生错误
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("cursor error during message retrieval: %w", err)
	}

	return messages, nil
}

// 逻辑删除
func (d *MsgDao) DeleteMessage(collection *mongo.Collection, messageID string) error {
	_, err := collection.UpdateOne(context.Background(), bson.M{"messageId": messageID}, bson.M{"$set": bson.M{"isDeleted": true, "deleteTime": time.Now()}})
	return err
}
