# Verix Stock

AI-powered stock news aggregator and alert system. Designed for simplified investment tracking, Verix Stock provides real-time summaries and sentiment analysis of market news via a "Telegram-first" approach.

## 🚀 Overview

Verix Stock addresses the pain points of modern investors (User Persona: F0) such as fragmented news sources, technical terminology, and the need for constant monitoring. It automates news collection, uses AI to distill key information, and delivers instant alerts through a Telegram bot.

### Key Features

- **Multi-threaded Scraping**: High-performance Go-based worker for real-time news collection from various sources.
- **AI-Powered Insights**: Integrates with Gemini API to provide concise summaries and sentiment scoring (Positive/Negative).
- **Intelligent Deduplication**: Ensures each news piece is processed and summarized only once, optimizing costs and resources.
- **Telegrambot Integration**:
  - **Auth**: Simplified login via Telegram Login Widget.
  - **Alerts**: Instant "Push" notifications when sentiment scores for watched stocks fluctuate significantly.
  - **Watchlist**: Manage your followed stocks directly from a minimalist UI.
- **Public News Timeline**: A Next.js-based frontend showcasing an aggregated timeline of market mood and news.

## 🏗 System Architecture

The project is structured into three main layers:

1.  **The Engine (Go Worker)**:
    - Scans news outlets using goroutines.
    - Summarizes content into 3 bullet points using Gemini.
    - Performs sentiment analysis.
2.  **The API Host (Go API)**:
    - Manages user authentication via Telegram.
    - Handles configuration for user watchlists.
    - Serves processed data via JSON endpoints for the frontend.
3.  **The Frontend & Alerts**:
    - **Next.js**: A high-performance, SEO-optimized dashboard for the public timeline.
    - **Telegram Bot**: The primary delivery channel for real-time alerts.

## 🛠 Tech Stack

- **Backend**: Go (Gin, GORM)
- **Database**: PostgreSQL (Supabase)
- **Cache/Queue**: Redis
- **Frontend**: Next.js
- **Intelligence**: Google Gemini API
- **Deployment**: Docker & Docker Compose

## ⚙️ Setup & Installation

### Prerequisites

- Go 1.25+
- Docker & Docker Compose
- PostgreSQL & Redis (or use provided Docker Compose)
- Telegram Bot Token
- Google Gemini API Key

### Configuration

Copy the `.env.example` to `.env` and fill in your details:

```bash
cp .env.example .env
```

Key variables:
- `SUPABASE_DB_URL`: Connection string for your PostgreSQL instance.
- `TELEGRAM_BOT_TOKEN`: Your bot token from @BotFather.
- `REDIS_URL`: URL for your Redis instance.
- `PORT`: API server port (default 8080).
- `GEMINI_API_KEY`: Your Google Gemini API key.

### Running with Docker

```bash
docker-compose up --build
```

### Running Locally

**Start the API Server:**
```bash
go run cmd/api/main.go
```

**Start the Worker:**
```bash
go run cmd/worker/main.go
```

## 🗺 Data Flow

1.  **Worker** → Scrapes → Summarizes (AI) → Stores in DB.
2.  **User (Web)** → Login (Telegram) → Manage Watchlist.
3.  **System (Trigger)** → News detected on watchlist stock → Sentiment check → Send Telegram Alert.
4.  **Frontend** → Fetch & Display Timeline.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📄 License

[Insert License Type, e.g., MIT]
