package watchlist

import (
	"errors"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// GetByChatID returns the StockConfig for a chat
func (r *Repository) GetByChatID(chatID int64) (*StockConfig, error) {
	var cfg StockConfig
	err := r.db.Where("chat_id = ?", chatID).First(&cfg).Error
	return &cfg, err
}

// Upsert creates or updates the configuration
func (r *Repository) Upsert(cfg *StockConfig) (bool, error) {
	var existing StockConfig
	err := r.db.Where("chat_id = ?", cfg.ChatID).First(&existing).Error

	isNew := false
	if errors.Is(err, gorm.ErrRecordNotFound) {
		isNew = true
		err = r.db.Create(cfg).Error
	} else if err == nil {
		err = r.db.Model(&existing).Updates(cfg).Error
	}

	return isNew, err
}

// Delete removes the configuration
func (r *Repository) Delete(chatID int64) error {
	return r.db.Where("chat_id = ?", chatID).Delete(&StockConfig{}).Error
}

// GetAllUniqueSymbols retrieves all unique symbols from all StockConfigs.
func (r *Repository) GetAllUniqueSymbols() ([]string, error) {
	var symbols []string
	// Use unnest in PostgreSQL to get all symbols as a flat list
	err := r.db.Model(&StockConfig{}).
		Select("DISTINCT unnest(symbols) as symbol").
		Pluck("symbol", &symbols).Error
	return symbols, err
}
