package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly/v2"
)

type cafeFFetcher struct {
	name string
}

// NewCafeFFetcher creates a new fetcher for CafeF.vn.
func NewCafeFFetcher() Fetcher {
	return &cafeFFetcher{
		name: "CafeF",
	}
}

func (f *cafeFFetcher) Name() string {
	return f.name
}

func (f *cafeFFetcher) Fetch(ctx context.Context, logger *slog.Logger, ticker string, since time.Time) ([]ScrapedArticle, error) {
	logger.Info("fetching articles from CafeF", "since", since.Format("2006-01-02 15:04"))
	var articles []ScrapedArticle
	var mu sync.Mutex

	c := colly.NewCollector(
		colly.AllowedDomains("cafef.vn"),
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
	)

	c.SetRequestTimeout(30 * time.Second)

	// Regex for CafeF article ID which encodes the date: 188 + YY + MM + DD + HH + mm + ss
	// Example: 188260331102030 -> 188 + 26 + 03 + 31 + 10 + 20 + 30
	dateRegex := regexp.MustCompile(`188(\d{2})(\d{2})(\d{2})(\d{2})?(\d{2})?(\d{2})?`)
	ict := time.FixedZone("ICT", 7*3600)

	c.OnHTML(".timeline .item", func(e *colly.HTMLElement) {
		title := e.ChildText(".titlehidden a")
		link := e.ChildAttr(".titlehidden a", "href")
		if link == "" {
			return
		}
		if !strings.HasPrefix(link, "http") {
			link = "https://cafef.vn" + link
		}
		description := e.ChildText(".sapo")
		
		// Initial extraction from link (may be unreliable but fast for skipping)
		publishedAt := time.Time{} // Default to zero time
		dateExtracted := false

		matches := dateRegex.FindStringSubmatch(link)
		if len(matches) >= 4 {
			yearStr := "20" + matches[1]
			monthStr := matches[2]
			dayStr := matches[3]
			
			hourStr := "00"
			minStr := "00"
			secStr := "00"
			if matches[4] != "" { hourStr = matches[4] }
			if matches[5] != "" { minStr = matches[5] }
			if matches[6] != "" { secStr = matches[6] }

			t, err := time.ParseInLocation("2006-01-02 15:04:05", yearStr+"-"+monthStr+"-"+dayStr+" "+hourStr+":"+minStr+":"+secStr, ict)
			if err == nil {
				publishedAt = t
				dateExtracted = true
			}
		}

		// Optimization: if we extracted a date from URL and it's definitely older than 'since', skip NOW
		if dateExtracted && publishedAt.Before(since) {
			return
		}

		// FINAL FILTER: Check if we have a valid date and if it's within the requested range
		if !dateExtracted {
			logger.Debug("skipping article: could not extract date", "title", title, "link", link)
			return
		}

		if publishedAt.Before(since) {
			logger.Debug("skipping article: older than threshold", 
				"title", title, 
				"published_at", publishedAt.Format("2006-01-02 15:04"),
				"threshold", since.Format("2006-01-02 15:04"))
			return
		}

		// Visit the article page to get FULL content and precise date
		fullContent := ""
		detailCollector := c.Clone()
		
		// Priority extraction from .pdate
		detailCollector.OnHTML(".pdate, [data-role='publishdate']", func(de *colly.HTMLElement) {
			dateStr := strings.TrimSpace(de.Text) // Example: "09-03-2026 - 01:04 AM"
			// Try to parse using the format provided by user
			t, err := time.ParseInLocation("02-01-2006 - 03:04 PM", dateStr, ict)
			if err == nil {
				publishedAt = t
				dateExtracted = true
			}
		})

		detailCollector.OnHTML(".left_detail, .content_detail, #content", func(de *colly.HTMLElement) {
			// Remove script and style tags
			de.DOM.Find("script, style, .social_media, .related_news").Remove()
			if fullContent == "" {
				fullContent = strings.TrimSpace(de.Text)
			}
		})
		
		err := detailCollector.Visit(link)
		if err != nil {
			logger.Warn("failed to visit article detail", "link", link, "error", err)
		}

		// Re-check date after detail visit just in case the URL date was wrong but pdate corrected it
		if publishedAt.Before(since) {
			logger.Debug("skipping article: older than threshold (confirmed path)", 
				"title", title, 
				"published_at", publishedAt.Format("2006-01-02 15:04"))
			return
		}

		mu.Lock()
		articles = append(articles, ScrapedArticle{
			TargetTicker: ticker,
			Title:        title,
			Link:         link,
			Description:  description,
			FullContent:  fullContent,
			Source:       f.name,
			PublishedAt:  publishedAt,
		})
		mu.Unlock()

		logger.Info("found new article", "title", title, "published_at", publishedAt.Format("2006-01-02 15:04"))
	})

	// Fetch up to 3 pages
	for page := 1; page <= 3; page++ {
		var searchURL string
		if page == 1 {
			searchURL = fmt.Sprintf("https://cafef.vn/tim-kiem.chn?keywords=%s", strings.ToUpper(ticker))
		} else {
			searchURL = fmt.Sprintf("https://cafef.vn/tim-kiem/trang-%d.chn?keywords=%s", page, strings.ToUpper(ticker))
		}

		logger.Info("visiting search page", "page", page, "url", searchURL)
		preFetchCount := len(articles)
		err := c.Visit(searchURL)
		if err != nil {
			logger.Error("failed to visit search page", "page", page, "error", err)
			break
		}

		// Optimization: if no new articles were added on this page AND we visited some items,
		// it likely means they were all older than 'since'. We can stop.
		if len(articles) == preFetchCount {
			logger.Info("no new articles on this page, stopping search", "page", page)
			break
		}
	}

	logger.Info("fetch completed", "total_new_articles", len(articles))
	return articles, nil
}
