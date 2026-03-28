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

func (r *Repository) List(page, limit int) ([]Article, int64, error) {
	var articles []Article
	var total int64

	offset := (page - 1) * limit

	if err := r.db.Model(&Article{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := r.db.Order("published_at DESC").Offset(offset).Limit(limit).Find(&articles).Error; err != nil {
		return nil, 0, err
	}
	return articles, total, nil
}

func (r *Repository) GetLatestDateForTicker(ticker string) (time.Time, error) {
	var a Article
	err := r.db.Where("ticker = ?", ticker).Order("published_at DESC").First(&a).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Return a very old date if no records exist
			return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), nil
		}
		return time.Time{}, err
	}
	return a.PublishedAt, nil
}

func (r *Repository) GetByID(id uint) (*Article, error) {
	var a Article
	if err := r.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// Create stores a new article. If SourceURL already exists, it does nothing (ignored).
func (r *Repository) Create(a *Article) error {
	return r.db.Where(Article{SourceURL: a.SourceURL}).FirstOrCreate(a).Error
}
