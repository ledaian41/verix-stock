package article

import (
	"time"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListPublished(page, limit int) ([]PublishedArticle, int64, error) {
	var articles []PublishedArticle
	var total int64

	offset := (page - 1) * limit

	if err := r.db.Model(&PublishedArticle{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := r.db.Order("published_at DESC").Offset(offset).Limit(limit).Find(&articles).Error; err != nil {
		return nil, 0, err
	}
	return articles, total, nil
}

func (r *Repository) GetPublishedByID(id uint) (*PublishedArticle, error) {
	var a PublishedArticle
	if err := r.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// Crawler Metadata Methods
func (r *Repository) GetLastCrawledAt(ticker, source string) (time.Time, error) {
	var meta CrawlerMetadata
	err := r.db.Where("ticker = ? AND source = ?", ticker, source).First(&meta).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), nil
		}
		return time.Time{}, err
	}
	return meta.LastCrawledAt, nil
}

func (r *Repository) UpdateLastCrawledAt(ticker, source string, t time.Time) error {
	return r.db.Save(&CrawlerMetadata{
		Ticker:        ticker,
		Source:        source,
		LastCrawledAt: t,
	}).Error
}

// Draft Article Methods
func (r *Repository) CreateDraft(a *DraftArticle) error {
	return r.db.Where(DraftArticle{SourceURL: a.SourceURL}).FirstOrCreate(a).Error
}

func (r *Repository) GetPendingDraftsGroupedByTicker() (map[string][]DraftArticle, error) {
	var drafts []DraftArticle
	if err := r.db.Where("ai_status = ?", "pending").Find(&drafts).Error; err != nil {
		return nil, err
	}

	result := make(map[string][]DraftArticle)
	for _, d := range drafts {
		result[d.Ticker] = append(result[d.Ticker], d)
	}
	return result, nil
}

func (r *Repository) MarkDraftsAsProcessed(ids []uint) error {
	return r.db.Model(&DraftArticle{}).Where("id IN ?", ids).Update("ai_status", "processed").Error
}

func (r *Repository) ClearProcessedDrafts() error {
	return r.db.Where("ai_status = ?", "processed").Delete(&DraftArticle{}).Error
}

func (r *Repository) UpdateDraftAI(id uint, facts string, status string) error {
	return r.db.Model(&DraftArticle{}).Where("id = ?", id).Updates(map[string]interface{}{
		"ai_facts":  facts,
		"ai_status": status,
	}).Error
}

func (r *Repository) GetExtractedDraftsByTicker(ticker string) ([]DraftArticle, error) {
	var drafts []DraftArticle
	err := r.db.Where("ticker = ? AND ai_status = ?", ticker, "extracted").Find(&drafts).Error
	return drafts, err
}

func (r *Repository) CountIncompleteDraftsByTicker(ticker string) (int64, error) {
	var count int64
	err := r.db.Model(&DraftArticle{}).
		Where("ticker = ? AND ai_status IN ?", ticker, []string{"pending", "extraction_queued"}).
		Count(&count).Error
	return count, err
}

func (r *Repository) GetDraftByID(id uint) (*DraftArticle, error) {
	var a DraftArticle
	if err := r.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) DeleteDraftsByTicker(ticker string) error {
	return r.db.Where("ticker = ?", ticker).Delete(&DraftArticle{}).Error
}

func (r *Repository) GetTickersReadyForSynthesis() ([]string, error) {
	var tickers []string
	err := r.db.Model(&DraftArticle{}).
		Select("ticker").
		Group("ticker").
		Having("SUM(CASE WHEN ai_status IN ? THEN 1 ELSE 0 END) = 0", []string{"pending", "extraction_queued"}).
		Having("SUM(CASE WHEN ai_status = ? THEN 1 ELSE 0 END) > 0", "extracted").
		Pluck("ticker", &tickers).Error
	return tickers, err
}



// Published Article Methods
func (r *Repository) CreatePublished(p *PublishedArticle) error {
	return r.db.Create(p).Error
}

func (r *Repository) CleanupOldPublished(days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)
	return r.db.Where("created_at < ?", cutoff).Delete(&PublishedArticle{}).Error
}
