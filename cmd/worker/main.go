package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/ledaian41/verix-stock/internal/core/db"
	"github.com/ledaian41/verix-stock/internal/core/events"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using OS environments")
	}

	db.InitDB()

	// Khởi tạo PubSub event bus
	pubsub := events.NewRedisPubSub(os.Getenv("REDIS_URL"))

	// Khởi tạo Worker
	log.Println("⛏️ Starting Verix Stock Worker...")

	// Worker cũng sẽ chịu trách nhiệm chính giao tiếp với Bot Telegram:
	// - Fetch data
	// - AI generate summary & sentiment
	// - Publish 'ArticleAnalyzedEvent'
	// - Send Telegram messages to users watching the stock

	_ = pubsub // use pubsub
}
