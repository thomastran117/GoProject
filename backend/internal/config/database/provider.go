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
	GoogleID     *string   `gorm:"column:google_id;uniqueIndex;size:255"`
	MicrosoftID  *string   `gorm:"column:microsoft_id;uniqueIndex;size:255"`
	CreatedAt    time.Time
}

type Config struct {
	Host         string
	Port         string
	User         string
	Password     string
	Name         string
	MaxOpenConns int
	MaxIdleConns int
	ConnMaxLife  time.Duration
	ConnMaxIdle  time.Duration
}


func Init(c Config) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4",
		c.User, c.Password, c.Host, c.Port, c.Name,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("database: failed to connect:", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("database: failed to get sql.DB:", err)
	}

	sqlDB.SetMaxOpenConns(c.MaxOpenConns)
	sqlDB.SetMaxIdleConns(c.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(c.ConnMaxLife)
	sqlDB.SetConnMaxIdleTime(c.ConnMaxIdle)

    err = db.AutoMigrate(&User{})
    if err != nil {
		log.Fatal("database: failed to migrate:", err)
    }

	DB = db
	log.Println("database: connected to MySQL")
}

