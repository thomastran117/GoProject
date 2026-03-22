package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

var DB *sql.DB

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

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("database: failed to open connection:", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal("database: failed to ping:", err)
	}

	if err := migrate(db); err != nil {
		log.Fatal("database: migration failed:", err)
	}

	DB = db
	log.Println("database: connected to MySQL")
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			email         VARCHAR(255)    NOT NULL UNIQUE,
			password_hash VARCHAR(255)    NOT NULL,
			created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}
