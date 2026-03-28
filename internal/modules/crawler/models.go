package crawler

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// ScrapedArticle represents raw data fetched from a news source.
type ScrapedArticle struct {
	TargetTicker string    // Mã cổ phiếu mà Fetcher đang nhắm tới (VD: "HPG")
	Title        string
	Link         string
	Description  string    // Optional short description/snippet
	Content      string    // Full text or main content of the article
	Source       string    // e.g., "CafeF", "VnEconomy"
	PublishedAt  time.Time
}

// Fingerprint returns a unique identifier for deduplication (e.g., hash of the Link).
func (a *ScrapedArticle) Fingerprint() string {
	hash := sha256.Sum256([]byte(a.Link))
	return hex.EncodeToString(hash[:])
}
