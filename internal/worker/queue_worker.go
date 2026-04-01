package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ledaian41/verix-stock/internal/core/ai"
	"github.com/ledaian41/verix-stock/internal/core/events"
	"github.com/ledaian41/verix-stock/internal/modules/article"
)

type QueueWorker struct {
	repo        *article.Repository
	synthesizer *ai.Synthesizer
	queue       *TaskQueue
	pubsub      events.PubSub
	logger      *slog.Logger
}

func NewQueueWorker(
	repo *article.Repository,
	synthesizer *ai.Synthesizer,
	queue *TaskQueue,
	pubsub events.PubSub,
	logger *slog.Logger,
) *QueueWorker {
	return &QueueWorker{
		repo:        repo,
		synthesizer: synthesizer,
		queue:       queue,
		pubsub:      pubsub,
		logger:      logger,
	}
}

func (w *QueueWorker) Start(ctx context.Context) {
	w.logger.Info("🚀 AI Synthesis Queue Worker started")
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("AI Synthesis Queue Worker shutting down")
			return
		default:
			task, err := w.queue.Dequeue(ctx)
			if err != nil {
				w.logger.Error("failed to dequeue task", "error", err)
				time.Sleep(2 * time.Second)
				continue
			}
			if task == nil {
				continue
			}

			w.logger.Info("processing task from redis", "type", task.Type, "ticker", task.Ticker, "id", task.ArticleID, "retry", task.Retry)

			if err := w.processTask(ctx, task); err != nil {
				w.logger.Error("failed to process task", "type", task.Type, "ticker", task.Ticker, "error", err)
				// Basic retry logic by re-enqueuing with a delay if needed
				if task.Retry < 3 {
					task.Retry++
					task.CreatedAt = time.Now().Add(time.Duration(task.Retry) * 10 * time.Second)
					_ = w.queue.Enqueue(ctx, task)
				} else {
					// Terminal failure: Update DB status to failed so it doesn't block synthesis
					if task.Type == TaskExtract && task.ArticleID > 0 {
						w.logger.Warn("task exhausted retries, marking as failed in DB", "article_id", task.ArticleID)
						_ = w.repo.UpdateDraftAI(task.ArticleID, "", "failed")

						// After marking as failed, we should still check if this ticker's batch is "done"
						// to trigger synthesis even with failures.
						w.checkAndTriggerSynthesis(ctx, task.Ticker)
					}
				}
			}
		}
	}
}

func (w *QueueWorker) checkAndTriggerSynthesis(ctx context.Context, ticker string) {
	incomplete, err := w.repo.CountIncompleteDraftsByTicker(ticker)
	if err == nil && incomplete == 0 {
		w.logger.Info("all articles (including failed) processed for ticker, enqueuing synthesis", "ticker", ticker)
		_ = w.queue.Enqueue(ctx, &Task{
			Type:   TaskSynthesize,
			Ticker: ticker,
		})
	}
}

func (w *QueueWorker) processTask(ctx context.Context, task *Task) error {
	log := w.logger.With("type", task.Type, "ticker", task.Ticker)

	switch task.Type {
	case TaskExtract:
		return w.handleExtract(ctx, task, log)
	case TaskSynthesize:
		return w.handleSynthesize(ctx, task, log)
	default:
		return fmt.Errorf("unknown task type: %s", task.Type)
	}
}

func (w *QueueWorker) handleExtract(ctx context.Context, task *Task, log *slog.Logger) error {
	// 1. Get draft
	d, err := w.repo.GetDraftByID(task.ArticleID)
	if err != nil {
		return err
	}

	// 2. Extract facts using Gemini
	res, err := w.synthesizer.ExtractFact(ctx, *d)
	if err != nil {
		return err
	}

	factJSON, _ := json.Marshal(res)

	// 3. Update DB
	if err := w.repo.UpdateDraftAI(d.ID, string(factJSON), "extracted"); err != nil {
		return err
	}

	log.Info("extracted facts for article", "id", d.ID)

	// 4. Check if all articles for this ticker are done
	incomplete, err := w.repo.CountIncompleteDraftsByTicker(task.Ticker)
	if err != nil {
		return err
	}

	if incomplete == 0 {
		log.Info("all articles extracted for ticker, enqueuing synthesis", "ticker", task.Ticker)
		return w.queue.Enqueue(ctx, &Task{
			Type:   TaskSynthesize,
			Ticker: task.Ticker,
		})
	} else {
		log.Info("article processed", "id", d.ID, "remaining_for_ticker", incomplete)
	}

	return nil
}

func (w *QueueWorker) handleSynthesize(ctx context.Context, task *Task, log *slog.Logger) error {
	// 1. Get all extracted facts for ticker
	drafts, err := w.repo.GetExtractedDraftsByTicker(task.Ticker)
	if err != nil {
		return err
	}

	if len(drafts) == 0 {
		log.Warn("no extracted drafts found for synthesis", "ticker", task.Ticker)
		return nil
	}

	log.Info("starting synthesis for ticker", "ticker", task.Ticker, "articles_count", len(drafts))

	facts := make([]ai.FactResult, 0, len(drafts))
	sources := make([]string, 0, len(drafts))
	for _, d := range drafts {
		var f ai.FactResult
		if err := json.Unmarshal([]byte(d.AIFacts), &f); err == nil {
			facts = append(facts, f)
			sources = append(sources, d.SourceURL)
		}
	}

	// 2. Synthesize using Gemini
	res, err := w.synthesizer.SynthesizeFromFacts(ctx, task.Ticker, facts)
	if err != nil {
		return err
	}

	sourcesJSON, _ := json.Marshal(sources)

	// 3. Create Published Article
	sessionName := "Market Digest"
	if time.Now().Hour() < 12 {
		sessionName = "Morning Digest"
	} else if time.Now().Hour() >= 15 {
		sessionName = "Afternoon Wrapup"
	}

	pub := &article.PublishedArticle{
		Ticker:         task.Ticker,
		Title:          fmt.Sprintf("[%s] %s - %s", task.Ticker, sessionName, time.Now().Format("02/01/2006")),
		Summary:        formatArticleSummary(res.Summary),
		Conclusion:     res.Conclusion,
		SentimentScore: res.SentimentScore,
		ArticleCount:   len(drafts),
		Sources:        string(sourcesJSON),
		PublishedAt:    time.Now(),
	}

	if err := w.repo.CreatePublished(pub); err != nil {
		return err
	}

	// 4. Emit event
	payload, _ := json.Marshal(pub)
	if err := w.pubsub.Publish(ctx, "article.published", payload); err != nil {
		log.Error("failed to publish event", "error", err)
	}

	// 5. CLEANUP
	if err := w.repo.DeleteDraftsByTicker(task.Ticker); err != nil {
		log.Error("failed to cleanup drafts", "error", err)
	}

	log.Info("✅ synthesis completed and drafts cleaned up")
	return nil
}

func formatArticleSummary(s string) string {
	parts := strings.Split(s, "-")
	var lines []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		lines = append(lines, "- "+p)
	}
	return strings.Join(lines, "\n")
}
