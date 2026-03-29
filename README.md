# Verix Stock

AI-powered stock news aggregator and alert system. Designed for simplified investment tracking, Verix Stock provides real-time summaries and sentiment analysis of market news via a "Telegram-first" approach.

## 🚀 Overview

Verix Stock addresses the pain points of modern investors (User Persona: F0) such as fragmented news sources, technical terminology, and the need for constant monitoring. It automates news collection, uses AI to distill key information, and delivers instant alerts through a Telegram bot.

### Key Features

- **Deep Multi-threaded Scraping**: High-performance Go-based worker that visits each article page to extract **Full Body Content** for high-quality analysis.
- **AI-Powered Synthesis**: Uses Gemini API to group multiple news items for a single ticker and synthesize them into a concise, high-value "Market Digest" per session.
- **Source Transparency**: Each summary includes the original source URLs to credit publishers and ensure copyright compliance.
- **Optimized Storage**: Maintains a **Transient Draft** table for raw news and an indexed **Published** table for summaries, achieving O(1) query performance.
- **Task Scheduling**: Managed via `robfig/cron` (v3) for reliable automation of fetching (08:00, 15:00), synthesis (08:15, 15:15), and daily cleanup.
- **Telegrambot Integration**:
  - **Auth**: Simplified login via Telegram Login Widget.
  - **Alerts**: Instant "Push" notifications when new synthesized news is available for watched stocks.
  - **Watchlist**: Manage your followed stocks directly from a minimalist UI.
- **Public News Timeline**: A Next.js-based frontend showcasing an aggregated timeline of market mood and news.

## 🏗 System Architecture

The project is structured into three main layers:

2.  **The Engine (Go Worker)**:
    - **Step 1: Fetcher**: Scans news outlets using goroutines and extracts full article content.
    - **Step 2: Synthesizer**: Groups news by ticker, uses Gemini to synthesize multiple articles into one concise "Market Digest" per session (Morning/Afternoon).
    - **Step 3: Cleanup**: Maintains a transient draft table and a 1-year retention policy for published summaries.
3.  **The API Host (Go API)**:
    - Manages user authentication via Telegram.
    - Handles configuration for user watchlists.
    - Serves indexed, high-performance JSON endpoints for the frontend.
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

1. **Worker (Fetch)**: Quét tin sâu $\rightarrow$ Lưu bản tin thô vào `DraftArticle` $\rightarrow$ Cập nhật `CrawlerMetadata`.
2. **Worker (Synthesis)**: AI đọc toàn văn các bài Draft của cùng 1 mã $\rightarrow$ Tổng hợp thành 1 bài Published $\rightarrow$ Link các nguồn gốc (Reference URLs) $\rightarrow$ Xóa Draft.
3. **User (Web)**: Login Telegram $\rightarrow$ Lưu mã HPG vào Watchlist trên API Host.
4. **Hệ thống (Trigger)**: AI thấy tin HPG mới $\rightarrow$ Kiểm tra ai đang Watch HPG $\rightarrow$ Gọi API Telegram gửi tin nhắn tổng hợp.
5. **Frontend (Next.js)**: Fetch data từ bảng Published (đã được đánh Index) để hiển thị Timeline tin tức tổng hợp siêu nhanh.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📄 License

[Insert License Type, e.g., MIT]
