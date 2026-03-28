package article

import (
	"time"
)

type Article struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Ticker         string    `gorm:"index;size:20" json:"ticker"`
	Source         string    `gorm:"index;size:100" json:"source"`
	SourceURL      string    `gorm:"uniqueIndex" json:"source_url"`
	Title          string    `gorm:"size:255" json:"title"`
	ContentHash    string    `gorm:"uniqueIndex;size:64" json:"-"`
	Summary        string    `gorm:"type:text" json:"summary"`
	SentimentScore float64   `gorm:"type:decimal(5,2)" json:"sentiment_score"`
	PublishedAt    time.Time `gorm:"index" json:"published_at"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName overrides the table name
func (Article) TableName() string {
	return "stock_article"
}
