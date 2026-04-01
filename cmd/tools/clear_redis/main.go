package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	_ = godotenv.Load()

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Error("Failed to parse Redis URL", "error", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(opts)
	ctx := context.Background()

	queueKey := "verix:task_queue"

	// Check length
	len, err := rdb.LLen(ctx, queueKey).Result()
	if err != nil {
		slog.Error("Failed to check queue length", "error", err)
		os.Exit(1)
	}

	slog.Info("Current queue length", "len", len)

	if len > 0 {
		err = rdb.Del(ctx, queueKey).Err()
		if err != nil {
			slog.Error("Failed to clear queue", "error", err)
			os.Exit(1)
		}
		slog.Info("Successfully cleared queue", "key", queueKey)
	} else {
		slog.Info("Queue is already empty")
	}
}
