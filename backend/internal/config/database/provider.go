package database

import (
	"fmt"
	"log"
	"time"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

type User struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	Email        string    `gorm:"uniqueIndex;size:255;not null"`
	PasswordHash string    `gorm:"column:password_hash;size:255;not null"`
	Role         string    `gorm:";size:255;not null"`
	CreatedAt    time.Time
}

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

func Init(c Config) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4",
		c.User, c.Password, c.Host, c.Port, c.Name,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("database: failed to connect:", err)
	}

    err = db.AutoMigrate(&User{})
    if err != nil {
		log.Fatal("database: failed to migrate:", err)
    }

	DB = db
	log.Println("database: connected to MySQL")
}

