package database

import (
	"fmt"
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

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

	if err := migrate(db); err != nil {
		log.Fatal("database: migration failed:", err)
	}

	DB = db
	log.Println("database: connected to MySQL")
}

func migrate(db *gorm.DB) error {
	return db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			email         VARCHAR(255)    NOT NULL UNIQUE,
			password_hash VARCHAR(255)    NOT NULL,
			created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`).Error
}
