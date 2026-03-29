package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ledaian41/verix-stock/internal/core/ai"
	"github.com/ledaian41/verix-stock/internal/modules/article"
	"github.com/ledaian41/verix-stock/internal/worker"
	"github.com/redis/go-redis/v9"
)

type AISynthesisJob struct {
	articleRepo *article.Repository
	synthesizer *ai.Synthesizer
	rdb         *redis.Client
	logger      *slog.Logger
}

func NewAISynthesisJob(
	articleRepo *article.Repository,
	synthesizer *ai.Synthesizer,
	rdb *redis.Client,
	logger *slog.Logger,
) *AISynthesisJob {
	return &AISynthesisJob{
		articleRepo: articleRepo,
		synthesizer: synthesizer,
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

	for ticker, drafts := range groupedDrafts {
		log.Info("synthesizing articles for ticker", "ticker", ticker, "count", len(drafts))

		// 2. Synthesize using AI
		res, err := j.synthesizer.Synthesize(ctx, ticker, drafts)
		if err != nil {
			log.Error("synthesis failed", "ticker", ticker, "error", err)
			continue
		}

		// 3. Prepare sources list
		sources := make([]string, 0, len(drafts))
		draftIDs := make([]uint, 0, len(drafts))
		for _, d := range drafts {
			sources = append(sources, d.SourceURL)
			draftIDs = append(draftIDs, d.ID)
		}
		sourcesJSON, _ := json.Marshal(sources)

		// 4. Create Published Article
		sessionName := "Market Digest"
		if time.Now().Hour() < 12 {
			sessionName = "Morning Digest"
		} else if time.Now().Hour() >= 15 {
			sessionName = "Afternoon Wrapup"
		}

		pub := &article.PublishedArticle{
			Ticker:         ticker,
			Title:          fmt.Sprintf("[%s] %s - %s", ticker, sessionName, time.Now().Format("02/01/2006")),
			Summary:        res.Summary,
			SentimentScore: res.SentimentScore,
			ArticleCount:   len(drafts),
			Sources:        string(sourcesJSON),
			PublishedAt:    time.Now(),
		}

		if err := j.articleRepo.CreatePublished(pub); err != nil {
			log.Error("failed to save published article", "ticker", ticker, "error", err)
			continue
		}

		// 5. Mark drafts as processed
		if err := j.articleRepo.MarkDraftsAsProcessed(draftIDs); err != nil {
			log.Error("failed to mark drafts as processed", "ticker", ticker, "error", err)
		}
	}

	// 6. Final Cleanup: Clear processed drafts
	if err := j.articleRepo.ClearProcessedDrafts(); err != nil {
		log.Error("failed to clear processed drafts", "error", err)
	}

	log.Info("ai_synthesis: logic execution completed")
	return nil
}
