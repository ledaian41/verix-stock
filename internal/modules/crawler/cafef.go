package crawler

import (
	"context"
	"fmt"
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

func (f *cafeFFetcher) Fetch(ctx context.Context, ticker string, since time.Time) ([]ScrapedArticle, error) {
	var articles []ScrapedArticle
	var mu sync.Mutex

	c := colly.NewCollector(
		colly.AllowedDomains("cafef.vn"),
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
	)

	c.SetRequestTimeout(30 * time.Second)

	// Regex for CafeF article ID which encodes the date: 188 + YY + MM + DD
	dateRegex := regexp.MustCompile(`188(\d{2})(\d{2})(\d{2})`)

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
		
		publishedAt := time.Now()
		matches := dateRegex.FindStringSubmatch(link)
		if len(matches) >= 4 {
			yearStr := "20" + matches[1]
			monthStr := matches[2]
			dayStr := matches[3]
			
			t, err := time.Parse("2006-01-02", yearStr+"-"+monthStr+"-"+dayStr)
			if err == nil {
				publishedAt = t
			}
		}

		// Stop adding if the article is older than 'since'
		if publishedAt.Before(since) {
			return
		}

		mu.Lock()
		articles = append(articles, ScrapedArticle{
			TargetTicker: ticker,
			Title:        title,
			Link:         link,
			Description:  description,
			Source:       f.name,
			PublishedAt:  publishedAt,
		})
		mu.Unlock()
	})

	// Fetch up to 3 pages
	for page := 1; page <= 3; page++ {
		var searchURL string
		if page == 1 {
			searchURL = fmt.Sprintf("https://cafef.vn/tim-kiem.chn?keywords=%s", strings.ToUpper(ticker))
		} else {
			searchURL = fmt.Sprintf("https://cafef.vn/tim-kiem/trang-%d.chn?keywords=%s", page, strings.ToUpper(ticker))
		}

		err := c.Visit(searchURL)
		if err != nil {
			// If a page fails, we stop
			break
		}

		// If no articles found on this page OR the oldest article on this page is already past 'since',
		// we could optimize by checking the length of 'articles' before/after.
		// For simplicity, we just visit 3 pages as requested.
	}

	return articles, nil
}
