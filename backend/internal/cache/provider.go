package cache

import (
    "context"
    "log"
    "github.com/redis/go-redis/v9"
)

var ctx = context.Background()

func main() {
    rdb := redis.NewClient(&redis.Options{
        Addr:     "localhost:6379",
        Password: "",
        DB:       0,
    })

    if err := rdb.Ping(ctx).Err(); err != nil {
        log.Fatal("Redis connection failed:", err)
    }
    log.Println("Redis connected!")
}