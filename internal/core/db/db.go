package db

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"github.com/ledaian41/verix-stock/internal/modules/article"
	"github.com/ledaian41/verix-stock/internal/modules/watchlist"
)

var DB *gorm.DB

func InitDB() *gorm.DB {
	dsn := os.Getenv("SUPABASE_DB_URL")
	if dsn == "" {
		log.Fatal("SUPABASE_DB_URL is not set in environment")
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Enable AutoMigrate for all models
	err = DB.AutoMigrate(
		&article.DraftArticle{},
		&article.PublishedArticle{},
		&article.CrawlerMetadata{},
		&watchlist.StockConfig{},
	)
	if err != nil {
		log.Fatalf("Failed to auto-migrate database: %v", err)
	}

	fmt.Println("🚀 Database connected and migrated successfully")
	return DB
}
