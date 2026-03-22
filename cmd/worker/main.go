package main

import (
	"context"
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
	"github.com/ledaian41/verix-stock/internal/modules/article"
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

	// 2. Scheduler & Jobs
	sched := worker.NewScheduler(10, logger)

	articleRepo := article.NewRepository(database)
	articleFetchJob := jobs.NewArticleFetchJob(articleRepo, pubsub, rdb, logger)

	// Register job to run every 30 minutes
	// Note: Expression "0 */30 * * * *" is forrobfig/cron with seconds enabled
	_ = sched.Register(worker.JobEntry{
		Job:      articleFetchJob,
		Expr:     "0 */30 * * * *",
		Timeout:  10 * time.Minute,
		MaxRetry: 2,
	})

	// Test Cron Tasks (Telegram)
	// Task 1: Runs at 22:47:00
	sched.Register(worker.JobEntry{
		Job:  &jobs.TelegramTestJob{NameSuffix: "task_1"},
		Expr: "0 57 22 * * *",
	})

	// Task 2: Runs at 22:50:00
	sched.Register(worker.JobEntry{
		Job:  &jobs.TelegramTestJob{NameSuffix: "task_2"},
		Expr: "0 58 22 * * *",
	})

	// 3. Health & Metrics endpoints
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Readiness: fail if specific job hasn't run successfully in > 90 min
		lastRun := sched.LastJobRun("article_fetch")
		if !lastRun.IsZero() && time.Since(lastRun) > 90*time.Minute {
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
