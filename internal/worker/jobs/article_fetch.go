package jobs

import (
    "context"
    "log/slog"
    "time"

    "github.com/ledaian41/verix-stock/internal/core/events"
    "github.com/ledaian41/verix-stock/internal/modules/article"
    "github.com/ledaian41/verix-stock/internal/worker"
    "github.com/redis/go-redis/v9"
)

const articleFetchLockTTL = 2 * time.Minute

// ArticleFetchJob implements the worker.Job interface.
type ArticleFetchJob struct {
    repo   *article.Repository
    pubsub events.PubSub
    rdb    *redis.Client
    logger *slog.Logger
}

func NewArticleFetchJob(repo *article.Repository, pubsub events.PubSub, rdb *redis.Client, logger *slog.Logger) *ArticleFetchJob {
    return &ArticleFetchJob{repo: repo, pubsub: pubsub, rdb: rdb, logger: logger}
}

func (j *ArticleFetchJob) Name() string { return "article_fetch" }

func (j *ArticleFetchJob) Run(ctx context.Context) error {
    log := j.logger.With("correlation_id", worker.CorrelationID(ctx))

    acquired, err := worker.AcquireLock(ctx, j.rdb, j.Name(), articleFetchLockTTL)
    if err != nil {
        return err
    }
    if !acquired {
        log.Info("lock not acquired, another replica is running")
        return nil
    }
    defer worker.ReleaseLock(ctx, j.rdb, j.Name())

    // Placeholder for actual scraping and analysis logic
    log.Info("article_fetch: logic execution start")
    
    // Simulate work
    select {
    case <-time.After(2 * time.Second):
        log.Info("article_fetch: logic execution completed")
    case <-ctx.Done():
        return ctx.Err()
    }

    return nil
}
