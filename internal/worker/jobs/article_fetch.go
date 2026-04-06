package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/ledaian41/verix-stock/internal/core/events"
	"github.com/ledaian41/verix-stock/internal/modules/article"
	"github.com/ledaian41/verix-stock/internal/modules/crawler"
	"github.com/ledaian41/verix-stock/internal/modules/watchlist"
	"github.com/ledaian41/verix-stock/internal/worker"
	"github.com/redis/go-redis/v9"
)

const articleFetchLockTTL = 2 * time.Minute

// ArticleFetchJob implements the worker.Job interface.
type ArticleFetchJob struct {
	articleRepo   *article.Repository
	watchlistRepo *watchlist.Repository
	crawler       crawler.Crawler
	pubsub        events.PubSub
	rdb           *redis.Client
	logger        *slog.Logger
}

func NewArticleFetchJob(
	articleRepo *article.Repository,
	watchlistRepo *watchlist.Repository,
	pubsub events.PubSub,
	rdb *redis.Client,
	logger *slog.Logger,
) *ArticleFetchJob {
	// Initialize crawler with CafeF fetcher
	mgr := crawler.NewManager(5) // Max 5 concurrent fetches
	mgr.Register(crawler.NewCafeFFetcher())

	return &ArticleFetchJob{
		articleRepo:   articleRepo,
		watchlistRepo: watchlistRepo,
		crawler:       mgr,
		pubsub:        pubsub,
		rdb:           rdb,
		logger:        logger,
	}
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

	log.Info("article_fetch: logic execution start")

	// 1. Get all symbols from watchlist
	symbols, err := j.watchlistRepo.GetAllUniqueSymbols()
	if err != nil {
		log.Error("failed to get unique symbols", "error", err)
		return err
	}

	if len(symbols) == 0 {
		log.Info("no symbols to track, skipping")
		return nil
	}

	// 2. Prepare ticker map with latest dates from Metadata table
	tickerMap := make(map[string]time.Time)
	ict := time.FixedZone("ICT", 7*3600)
	now := time.Now().In(ict)
	
	// Default window is 72 hours as requested by user to capture recent content
	window := 72 * time.Hour
	sinceThreshold := now.Add(-window)

	for _, s := range symbols {
		lastDate, err := j.articleRepo.GetLastCrawledAt(s, "CafeF")
		if err != nil {
			log.Warn("failed to get latest date from metadata", "ticker", s, "error", err)
			lastDate = sinceThreshold
		}

		// Use the metadata date if it's more recent than our 72h window,
		// otherwise use the 72h window to satisfy the "within 72 hours" requirement.
		if lastDate.Before(sinceThreshold) {
			lastDate = sinceThreshold
		}

		tickerMap[s] = lastDate
	}

	// 3. Crawl articles (now with Deep Extraction from previous change)
	scraped, err := j.crawler.Crawl(ctx, log, tickerMap)
	if err != nil {
		log.Error("crawling failed", "error", err)
		return err
	}

	log.Info("scraped articles", "total_count", len(scraped))

	// 4. Save to Draft database
	newCount := 0
	tickerUpdateMap := make(map[string]time.Time)

	for _, s := range scraped {
		art := &article.DraftArticle{
			Ticker:      s.TargetTicker,
			Source:      s.Source,
			SourceURL:   s.Link,
			Title:       s.Title,
			FullContent: s.FullContent,
			ContentHash: s.Fingerprint(),
			PublishedAt: s.PublishedAt,
			AIStatus:    "pending",
		}

		if err := j.articleRepo.CreateDraft(art); err != nil {
			log.Warn("failed to save draft article", "url", s.Link, "error", err)
			continue
		}
		newCount++

		// Keep track of the most recent article for metadata update
		if latest, ok := tickerUpdateMap[s.TargetTicker]; !ok || s.PublishedAt.After(latest) {
			tickerUpdateMap[s.TargetTicker] = s.PublishedAt
		}
	}

	// 5. Update Metadata table
	for ticker, lastDate := range tickerUpdateMap {
		if err := j.articleRepo.UpdateLastCrawledAt(ticker, "CafeF", lastDate); err != nil {
			log.Error("failed to update metadata", "ticker", ticker, "error", err)
		}
	}

	log.Info("article_fetch: logic execution completed", "new_drafts_saved", newCount)
	return nil
}
