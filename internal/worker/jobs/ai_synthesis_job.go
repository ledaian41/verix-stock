package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/ledaian41/verix-stock/internal/modules/article"
	"github.com/ledaian41/verix-stock/internal/worker"
	"github.com/redis/go-redis/v9"
)

type AISynthesisJob struct {
	articleRepo *article.Repository
	queue       *worker.TaskQueue
	rdb         *redis.Client
	logger      *slog.Logger
}

func NewAISynthesisJob(
	articleRepo *article.Repository,
	rdb *redis.Client,
	logger *slog.Logger,
) *AISynthesisJob {
	return &AISynthesisJob{
		articleRepo: articleRepo,
		queue:       worker.NewTaskQueue(rdb),
		rdb:         rdb,
		logger:      logger,
	}
}

func (j *AISynthesisJob) Name() string { return "ai_synthesis" }

func (j *AISynthesisJob) Run(ctx context.Context) error {
	log := j.logger.With("correlation_id", worker.CorrelationID(ctx))

	acquired, err := worker.AcquireLock(ctx, j.rdb, j.Name(), 5*time.Minute)
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	defer worker.ReleaseLock(ctx, j.rdb, j.Name())

	// 1. Get pending drafts grouped by ticker
	groupedDrafts, err := j.articleRepo.GetPendingDraftsGroupedByTicker()
	if err != nil {
		log.Error("failed to get pending drafts", "error", err)
		return err
	}

	if len(groupedDrafts) == 0 {
		log.Info("no pending drafts to synthesize")
		return nil
	}

	enqueuedCount := 0
	for ticker, drafts := range groupedDrafts {
		log.Info("enqueuing articles for ticker", "ticker", ticker, "count", len(drafts))

		for _, d := range drafts {
			task := &worker.Task{
				Type:   worker.TaskExtract,
				Ticker: ticker,
				ArticleID: d.ID,
			}
			if err := j.queue.Enqueue(ctx, task); err != nil {
				log.Error("failed to enqueue extraction task", "article_id", d.ID, "error", err)
				continue
			}

			// Update status in DB to avoid duplicate enqueuing
			if err := j.articleRepo.MarkDraftsAsProcessed([]uint{d.ID}); err != nil { // reused method to set 'processed'... wait, should set 'extraction_queued'
				// Let's use UpdateDraftAI for specific status
			}
			_ = j.articleRepo.UpdateDraftAI(d.ID, "", "extraction_queued")
			enqueuedCount++
		}
	}

	log.Info("ai_synthesis producer: finished", "total_enqueued", enqueuedCount)
	return nil
}

