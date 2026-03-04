package watchlist

import (
	"time"

	"github.com/lib/pq"
)

// StockConfig manages what symbols a particular Telegram Chat is watching.
// Primary key is the Telegram chat_id — no separate User model needed.
type StockConfig struct {
	ChatID    int64          `gorm:"primaryKey;autoIncrement:false" json:"chat_id"`
	Symbols   pq.StringArray `gorm:"type:text[]" json:"symbols"` // ["HPG", "SSI", ...]
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName overrides the table name
func (StockConfig) TableName() string {
	return "stock_config"
}
