package events

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

// PubSub defines the interface for our event bus
type PubSub interface {
	Publish(ctx context.Context, channel string, message interface{}) error
	Subscribe(ctx context.Context, channel string) <-chan *redis.Message
}

type RedisPubSub struct {
	client *redis.Client
}

func NewRedisPubSub(redisURL string) *RedisPubSub {
	if redisURL == "" {
		redisURL = "localhost:6379" // Default local
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("Invalid REDIS_URL: %v", err)
	}

	client := redis.NewClient(opts)
	// Test connection
	if err := client.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Println("⚡️ Redis Pub/Sub connected successfully")

	return &RedisPubSub{
		client: client,
	}
}

func (r *RedisPubSub) Publish(ctx context.Context, channel string, message interface{}) error {
	return r.client.Publish(ctx, channel, message).Err()
}

func (r *RedisPubSub) Subscribe(ctx context.Context, channel string) <-chan *redis.Message {
	pubsub := r.client.Subscribe(ctx, channel)
	return pubsub.Channel()
}
