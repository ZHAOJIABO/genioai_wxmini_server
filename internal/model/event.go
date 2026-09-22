package model

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// UserEvent 用户事件记录
type UserEvent struct {
	ID        int64           `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	UserID    string          `json:"user_id" gorm:"column:user_id;type:varchar(64);index"`
	EventType string          `json:"event_type" gorm:"column:event_type;type:varchar(255)"`
	EventData json.RawMessage `json:"event_data" gorm:"column:event_data;type:text"`
	CreatedAt time.Time       `json:"created_at" gorm:"column:created_at"`
	UpdatedAt time.Time       `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt  `json:"deleted_at" gorm:"column:deleted_at;index"`
}

// TrackingEvent 埋点事件记录
type TrackingEvent struct {
	ID        int64     `json:"-" gorm:"column:id;primaryKey;autoIncrement"` // 事件的唯一标识符
	CreatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP"`
	AppStore  string    `json:"-"    gorm:"type:varchar(255);not null"`
	OS        string    `json:"-"    gorm:"type:varchar(100);not null"`
	UserID    string    `json:"user_id" gorm:"column:user_id;type:varchar(64);index"`
	// 事件类型
	TrackingEventType string `json:"-" gorm:"column:tracking_event_type;type:varchar(255)"`
	// 事件来源
	TrackingEventSource string `json:"-" gorm:"column:tracking_event_source;type:varchar(255)"`
	ExtraData           string `json:"-" gorm:"type:text"`
	ProjectID           string `json:"project_id" gorm:"column:project_id;type:varchar(64);index"`
	Version             string `json:"version" gorm:"column:version;type:varchar(64);index"`
	IP                  string `json:"ip" gorm:"column:ip;type:varchar(64);index"`
	Brand               string `json:"brand" gorm:"column:brand;type:varchar(64);index"`
	Language            string `json:"language" gorm:"column:language;type:varchar(64);index"`
}
