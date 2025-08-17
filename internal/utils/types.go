package utils

import "time"

type User struct {
	ID       string `gorm:"primary_key;not null;unique" json:"id"`
	Email    string `gorm:"not null;unique" json:"email"`
	Password string `gorm:"not null" json:"password"`
	DeletedAt time.Time `gorm:"default:null" json:"deleted_at"`
}

type Document struct {
	ID      string    `gorm:"primary_key;not null;unique" json:"id"`
	UserID  string    `gorm:"not null" json:"user_id"`
	Content string    `gorm:"type:text;not null" json:"content"`
	Updated time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated"`
	Created time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created"`
	Access  string    `gorm:"type:jsonb;not null" json:"access"`
}
