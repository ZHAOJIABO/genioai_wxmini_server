package model

import vai "va_visionai_server/internal/va_interface"

type Message struct {
	ID          string            `bson:"_id,omitempty"`
	MessageID   string            `bson:"messageId"`
	ChatID      string            `bson:"chatId"`
	Content     string            `bson:"content"`
	URL         string            `bson:"url"`
	MessageType int               `bson:"messageType"`
	UserID      string            `bson:"userId"`
	Sender      int               `bson:"sender"`
	Date        string            `bson:"date"`
	CreateTime  string            `bson:"createTime"`
	VoiceHash   string            `bson:"voiceId"`
	Quality     vai.LLMMsgQuality `bson:"quality"`
	ModelId     string            `bson:"modelId"`
}

func (m *Message) ToProto() *vai.Message {
	return &vai.Message{
		MessageId:   m.MessageID,
		ChatId:      m.ChatID,
		Content:     m.Content,
		Url:         m.URL,
		MessageType: vai.MessageType(m.MessageType),
		Sender:      vai.MessageSender(m.Sender),
		CreateTime:  m.CreateTime,
		ModelId:     vai.Model(vai.Model_value[m.ModelId]),
		Quality:     m.Quality,
		VoiceInfo:   &vai.VoiceInfo{Md5: m.VoiceHash},
	}
}
