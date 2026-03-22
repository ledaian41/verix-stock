package worker

import (
    "context"
    "time"
    "github.com/redis/go-redis/v9"
)

// AcquireLock attempts to SETNX a key in Redis.
func AcquireLock(ctx context.Context, rdb *redis.Client, key string, ttl time.Duration) (bool, error) {
    return rdb.SetNX(ctx, "lock:"+key, "1", ttl).Result()
}

// ReleaseLock deletes the lock key from Redis.
func ReleaseLock(ctx context.Context, rdb *redis.Client, key string) {
    rdb.Del(ctx, "lock:"+key)
}
