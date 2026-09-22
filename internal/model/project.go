package model

import "gorm.io/gorm"

type Project struct {
	gorm.Model

	ProjectID   string `gorm:"column:project_id"`
	ProjectName string `gorm:"column:project_name"`
}
