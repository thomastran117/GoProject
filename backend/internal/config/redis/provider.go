package redis

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client

type Config struct {
	Addr     string
	Password string
	DB       int
}

func Init(c Config) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     c.Addr,
		Password: c.Password,
		DB:       c.DB,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal("cache: failed to connect to Redis:", err)
	}

	Client = rdb
	log.Println("cache: connected to Redis")
}
