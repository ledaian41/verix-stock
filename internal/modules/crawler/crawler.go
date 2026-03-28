package crawler

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Fetcher defines the interface for resource-specific news scrapers.
type Fetcher interface {
	// Fetch retrieves recent articles from the source for a specific ticker.
	// It stops if it encounters articles older than the 'since' time.
	Fetch(ctx context.Context, ticker string, since time.Time) ([]ScrapedArticle, error)
	// Name returns the identifier of the news source.
	Name() string
}

// Crawler manages multiple Fetchers and aggregates their results.
type Crawler interface {
	// Register adds a new Fetcher to the crawler.
	Register(fetcher Fetcher)
	// Crawl executes registered fetchers concurrently for a list of tickers.
	// Each ticker search can specify a 'since' time to limit results.
	Crawl(ctx context.Context, tickers map[string]time.Time) ([]ScrapedArticle, error)
}

type manager struct {
	fetchers []Fetcher
	maxConcurrent int
}

// NewManager creates a new Crawler instance with a specified concurrency limit.
func NewManager(maxConcurrent int) Crawler {
	return &manager{
		fetchers:      make([]Fetcher, 0),
		maxConcurrent: maxConcurrent,
	}
}

func (m *manager) Register(fetcher Fetcher) {
	m.fetchers = append(m.fetchers, fetcher)
}

func (m *manager) Crawl(ctx context.Context, tickers map[string]time.Time) ([]ScrapedArticle, error) {
	var (
		mu       sync.Mutex
		articles []ScrapedArticle
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(m.maxConcurrent)

	for ticker, since := range tickers {
		for _, f := range m.fetchers {
			ticker := ticker // capture loop variable
			since := since   // capture loop variable
			fetcher := f

			g.Go(func() error {
				results, err := fetcher.Fetch(ctx, ticker, since)
				if err != nil {
					// We might want to log this but continue with other fetchers
					return nil
				}

				mu.Lock()
				articles = append(articles, results...)
				mu.Unlock()
				return nil
			})
		}
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return articles, nil
}
