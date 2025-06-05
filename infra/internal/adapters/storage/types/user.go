package types

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	FirstName   string        `gorm:"type:varchar(255);not null"`
	LastName    string        `gorm:"type:varchar(255);not null"`
	Phone       string        `gorm:"type:varchar(255);not null;unique"`
	Permissions uint64        `gorm:"not null;default:1;not null;"`
	Roles       uint64        `gorm:"not null;default:4096;not null;"`
	Restaurants []*Restaurant `gorm:"foreignKey:OwnerID"`
	Tokens      []*TokenWhitelist `gorm:"foreignKey:UserID"`
}

type TokenWhitelist struct {
	ExpiresAt time.Time
	Value     string `gorm:"type:text;not null;unique;"`
	UserID    uint   `gorm:"not null;"`
}
