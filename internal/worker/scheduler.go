package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/robfig/cron/v3"
)

// ── Prometheus metrics ────────────────────────────────────────────────────────

var (
	jobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "cron_job_duration_seconds",
		Help:    "Duration of each cron job run.",
		Buckets: prometheus.DefBuckets,
	}, []string{"job"})

	jobErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cron_job_errors_total",
		Help: "Total errors per cron job.",
	}, []string{"job", "reason"})

	lockContention = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cron_job_lock_contention_total",
		Help: "Times a job was skipped due to distributed lock.",
	}, []string{"job"})
)

// ── Health tracking ───────────────────────────────────────────────────────────

type Scheduler struct {
	c              *cron.Cron
	pool           *Pool
	logger         *slog.Logger
	lastJobSuccess map[string]time.Time // when job last completed without error
	jobEntries     map[string]JobEntry  // Store registered jobs for manual triggering
	mu             sync.RWMutex
}

func NewScheduler(concurrency int, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		c:              cron.New(cron.WithSeconds()),
		pool:           NewPool(concurrency),
		logger:         logger,
		lastJobSuccess: make(map[string]time.Time),
		jobEntries:     make(map[string]JobEntry),
	}
}

// LastJobRun returns when the job last succeeded — used by /readyz.
func (s *Scheduler) LastJobRun(jobName string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastJobSuccess[jobName]
}

func (s *Scheduler) recordSuccess(jobName string) {
	s.mu.Lock()
	s.lastJobSuccess[jobName] = time.Now()
	s.mu.Unlock()
}

// ── Registration ──────────────────────────────────────────────────────────────

// Register adds a JobEntry into the cron scheduler.
func (s *Scheduler) Register(entry JobEntry) error {
	id, err := s.c.AddFunc(entry.Expr, func() {
		s.pool.Submit(func() {
			timeout := entry.Timeout
			if timeout == 0 {
				timeout = 5 * time.Minute
			}
			s.runWithRetry(entry, timeout)
		})
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.jobEntries[entry.Job.Name()] = entry
	s.mu.Unlock()

	s.logger.Info("job registered", "job", entry.Job.Name(), "expr", entry.Expr, "id", id)
	return nil
}

// RunJob executes a registered job manually by its name.
func (s *Scheduler) RunJob(name string) error {
	s.mu.RLock()
	entry, ok := s.jobEntries[name]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("job %s not found in scheduler", name)
	}

	timeout := entry.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	return s.runOnce(entry.Job, timeout)
}

func (s *Scheduler) runWithRetry(entry JobEntry, timeout time.Duration) {
	attempts := entry.MaxRetry + 1
	if attempts < 1 {
		attempts = 1
	}

	for i := 0; i < attempts; i++ {
		err := s.runOnce(entry.Job, timeout)
		if err == nil {
			return
		}
		if i < attempts-1 {
			backoff := time.Duration(1<<uint(i)) * time.Second
			s.logger.Warn("job retry", "job", entry.Job.Name(), "attempt", i+1, "backoff", backoff)
			time.Sleep(backoff)
		}
	}
}

func (s *Scheduler) runOnce(job Job, timeout time.Duration) error {
	// Inject correlation ID into context
	correlationID := uuid.New().String()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ctx = context.WithValue(ctx, correlationIDKey{}, correlationID)

	log := s.logger.With("job", job.Name(), "correlation_id", correlationID)
	start := time.Now()
	log.Info("job starting")

	err := job.Run(ctx)
	elapsed := time.Since(start)
	jobDuration.WithLabelValues(job.Name()).Observe(elapsed.Seconds())

	if err != nil {
		// Custom logic for lock contention can be used if we return a specific error
		reason := "other"
		if err == context.DeadlineExceeded {
			reason = "timeout"
		}
		jobErrors.WithLabelValues(job.Name(), reason).Inc()
		log.Error("job failed", "error", err, "elapsed", elapsed)
		return err
	}

	s.recordSuccess(job.Name())
	log.Info("job done", "elapsed", elapsed)
	return nil
}

// correlationIDKey is an unexported context key (avoids collisions).
type correlationIDKey struct{}

// CorrelationID extracts the ID from a job's context.
func CorrelationID(ctx context.Context) string {
	if v, ok := ctx.Value(correlationIDKey{}).(string); ok {
		return v
	}
	return ""
}

func (s *Scheduler) Start() { s.c.Start() }

func (s *Scheduler) Stop() {
	ctx := s.c.Stop()
	<-ctx.Done()
	s.pool.Wait()
}
