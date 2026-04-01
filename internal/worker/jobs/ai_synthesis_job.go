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
		queue:       worker.NewTaskQueue(rdb, logger),
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

	// 1. Enqueue extraction tasks for pending drafts
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

			_ = j.articleRepo.UpdateDraftAI(d.ID, "", "extraction_queued")
			enqueuedCount++
		}
	}

	// 2. NEW: Rescue abandoned tickers (like MBB with 36 extracted but no synthesis)
	abandoned, err := j.articleRepo.GetTickersReadyForSynthesis()
	if err != nil {
		log.Error("failed to get abandoned tickers", "error", err)
	} else {
		for _, ticker := range abandoned {
			log.Info("found abandoned ticker ready for synthesis, rescuing", "ticker", ticker)
			task := &worker.Task{
				Type:   worker.TaskSynthesize,
				Ticker: ticker,
			}
			if err := j.queue.Enqueue(ctx, task); err != nil {
				log.Error("failed to enqueue rescue synthesis task", "ticker", ticker, "error", err)
			}
		}
	}

	log.Info("ai_synthesis producer: finished", "total_enqueued", enqueuedCount, "rescued_tickers", len(abandoned))
	return nil
}

