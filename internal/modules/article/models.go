package article

import (
	"time"
)

type DraftArticle struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Ticker      string    `gorm:"index;size:20" json:"ticker"`
	Source      string    `gorm:"index;size:100" json:"source"`
	SourceURL   string    `gorm:"uniqueIndex" json:"source_url"`
	Title       string    `gorm:"size:255" json:"title"`
	FullContent string    `gorm:"type:text" json:"full_content"`
	ContentHash string    `gorm:"uniqueIndex;size:64" json:"-"`
	AIStatus    string    `gorm:"index;default:'pending';size:20" json:"ai_status"` // pending, extraction_queued, extracted, failed
	AIFacts     string    `gorm:"type:text" json:"ai_facts"`                        // JSON storage for FactResult
	PublishedAt time.Time `gorm:"index" json:"published_at"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}


func (DraftArticle) TableName() string {
	return "stock_draft_article"
}

type PublishedArticle struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Ticker         string    `gorm:"index;size:20" json:"ticker"`
	Title          string    `gorm:"size:255" json:"title"`
	Summary        string    `gorm:"type:text" json:"summary"`
	Conclusion     string    `gorm:"type:text" json:"conclusion"`
	SentimentScore float64   `gorm:"type:decimal(5,2)" json:"sentiment_score"`
	ArticleCount   int       `json:"article_count"`
	Sources        string    `gorm:"type:jsonb" json:"sources"` // Store as JSON array of URLs
	PublishedAt    time.Time `gorm:"index" json:"published_at"`
	CreatedAt      time.Time `gorm:"index" json:"created_at"` // Indexed for Timeline
}

func (PublishedArticle) TableName() string {
	return "stock_published_article"
}

type CrawlerMetadata struct {
	Ticker         string    `gorm:"primaryKey;size:20" json:"ticker"`
	Source         string    `gorm:"primaryKey;size:100" json:"source"`
	LastCrawledAt  time.Time `json:"last_crawled_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CrawlerMetadata) TableName() string {
	return "stock_crawler_metadata"
}
