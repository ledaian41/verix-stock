package article

import (
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

func (r *Repository) GetByID(id uint) (*Article, error) {
	var a Article
	if err := r.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}
