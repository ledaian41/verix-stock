package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/ledaian41/verix-stock/internal/core/db"
	"github.com/ledaian41/verix-stock/internal/core/events"
	"github.com/ledaian41/verix-stock/internal/core/ai"
	"github.com/ledaian41/verix-stock/internal/modules/article"
	"github.com/ledaian41/verix-stock/internal/modules/watchlist"
	"github.com/ledaian41/verix-stock/internal/worker"
	"github.com/ledaian41/verix-stock/internal/worker/jobs"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using OS environments")
	}

	// 1. Core initialization
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	database := db.InitDB()
	pubsub := events.NewRedisPubSub(os.Getenv("REDIS_URL"))

	// Manual Redis client for distributed locks
	redisOpts, _ := redis.ParseURL(os.Getenv("REDIS_URL")) // Defaulting on error handled by go-redis
	if os.Getenv("REDIS_URL") == "" {
		redisOpts = &redis.Options{Addr: "localhost:6379"}
	}
	rdb := redis.NewClient(redisOpts)

	// 2. AI & Jobs
	synthesizer, err := ai.NewSynthesizer(context.Background())
	if err != nil {
		logger.Error("failed to initialize AI synthesizer", "error", err)
	} else {
		defer synthesizer.Close()
	}

	sched := worker.NewScheduler(10, logger)

	articleRepo := article.NewRepository(database)
	watchlistRepo := watchlist.NewRepository(database)
	
	articleFetchJob := jobs.NewArticleFetchJob(articleRepo, watchlistRepo, pubsub, rdb, logger)
	aiSummaryJob := jobs.NewAISynthesisJob(articleRepo, synthesizer, pubsub, rdb, logger)

	// 3. Realtime Notification Subscriber
	notifier := article.NewTelegramNotifier()
	go func() {
		ctx := context.Background()
		ch := pubsub.Subscribe(ctx, "article.published")
		logger.Info("📡 Telegram notification subscriber started")
		for msg := range ch {
			var pub article.PublishedArticle
			if err := json.Unmarshal([]byte(msg.Payload), &pub); err != nil {
				logger.Error("failed to unmarshal published article event", "error", err)
				continue
			}

			if err := notifier.Notify(ctx, pub); err != nil {
				logger.Error("failed to send telegram notification", "ticker", pub.Ticker, "error", err)
			} else {
				logger.Info("✅ Telegram notification sent", "ticker", pub.Ticker)
			}
		}
	}()

	// Register article_fetch job: Morning at 08:00, Afternoon at 15:00
	_ = sched.Register(worker.JobEntry{
		Job:      articleFetchJob,
		Expr:     "0 0 8 * * 1-5",
		Timeout:  10 * time.Minute,
		MaxRetry: 2,
	})
	_ = sched.Register(worker.JobEntry{
		Job:      articleFetchJob,
		Expr:     "0 0 15 * * 1-5",
		Timeout:  10 * time.Minute,
		MaxRetry: 2,
	})

	// Register ai_synthesis job: 15 mins after fetch
	_ = sched.Register(worker.JobEntry{
		Job:      aiSummaryJob,
		Expr:     "0 15 8 * * 1-5",
		Timeout:  10 * time.Minute,
		MaxRetry: 2,
	})
	_ = sched.Register(worker.JobEntry{
		Job:      aiSummaryJob,
		Expr:     "0 15 15 * * 1-5",
		Timeout:  10 * time.Minute,
		MaxRetry: 2,
	})

	// 4. Daily Cleanup Job (retains 1 year of published articles)
	_ = sched.Register(worker.JobEntry{
		Job: &worker.GenericJob{
			JobName: "published_cleanup",
			Action: func(ctx context.Context) error {
				return articleRepo.CleanupOldPublished(365)
			},
		},
		Expr: "0 0 0 * * *", // Every midnight
	})

	// 5. Health & Metrics endpoints
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Readiness: fail if specific job hasn't run successfully in > 24 hours
		lastRun := sched.LastJobRun("article_fetch")
		if !lastRun.IsZero() && time.Since(lastRun) > 24*time.Hour {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "stale_job: last success was %v ago", time.Since(lastRun))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	})

	srv := &http.Server{Addr: ":9090", Handler: mux}
	go func() {
		logger.Info("metrics/health server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error("health server failed", "error", err)
		}
	}()

	// 4. Start & Graceful Shutdown
	sched.Start()
	logger.Info("⛏️  Verix Stock Worker started")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down...")
	sched.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)

	logger.Info("worker stopped gracefully")
}
