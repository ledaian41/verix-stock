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

	// 2. Prepare ticker map with latest dates
	tickerMap := make(map[string]time.Time)
	for _, s := range symbols {
		lastDate, err := j.articleRepo.GetLatestDateForTicker(s)
		if err != nil {
			log.Warn("failed to get latest date for ticker", "ticker", s, "error", err)
			lastDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		tickerMap[s] = lastDate
	}

	// 3. Crawl articles (now with state and pagination)
	scraped, err := j.crawler.Crawl(ctx, tickerMap)
	if err != nil {
		log.Error("crawling failed", "error", err)
		return err
	}

	log.Info("scraped articles", "total_count", len(scraped))

	// 4. Save to database
	newCount := 0
	for _, s := range scraped {
		art := &article.Article{
			Ticker:      s.TargetTicker,
			Source:      s.Source,
			SourceURL:   s.Link,
			Title:       s.Title,
			Summary:     s.Description,
			ContentHash: s.Fingerprint(),
			PublishedAt: s.PublishedAt,
		}

		if err := j.articleRepo.Create(art); err != nil {
			log.Warn("failed to save article", "url", s.Link, "error", err)
			continue
		}
		newCount++
	}

	log.Info("article_fetch: logic execution completed", "new_articles_saved", newCount)

	log.Info("article_fetch: logic execution completed")
	return nil
}
