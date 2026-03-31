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
		
		publishedAt := time.Now()
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
			}
		}

		// Stop adding if the article is older than 'since'
		if publishedAt.Before(since) {
			return
		}

		// Visit the article page to get FULL content
		fullContent := ""
		detailCollector := c.Clone()
		detailCollector.OnHTML(".left_detail, .content_detail, #content", func(de *colly.HTMLElement) {
			// Remove script and style tags
			de.DOM.Find("script, style, .social_media, .related_news").Remove()
			fullContent = strings.TrimSpace(de.Text)
		})
		
		err := detailCollector.Visit(link)
		if err != nil {
			// If failed to visit detail, we still keep the article but with empty content
			// or we can skip it. Let's keep it for now.
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
	})

	// Fetch up to 3 pages
	for page := 1; page <= 3; page++ {
		var searchURL string
		if page == 1 {
			searchURL = fmt.Sprintf("https://cafef.vn/tim-kiem.chn?keywords=%s", strings.ToUpper(ticker))
		} else {
			searchURL = fmt.Sprintf("https://cafef.vn/tim-kiem/trang-%d.chn?keywords=%s", page, strings.ToUpper(ticker))
		}

		preFetchCount := len(articles)
		err := c.Visit(searchURL)
		if err != nil {
			break
		}

		// Optimization: if no new articles were added on this page AND we visited some items,
		// it likely means they were all older than 'since'. We can stop.
		// Note: 'articles' is appended inside OnHTML.
		if len(articles) == preFetchCount {
			// Check if we hit the 'since' limit or just no results
			break
		}
	}

	return articles, nil
}
