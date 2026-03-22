package worker

import (
    "context"
    "time"
)

// Job defines the contract for any cron job.
type Job interface {
    Name() string
    Run(ctx context.Context) error
}

// JobEntry bundles a Job with scheduling metadata.
type JobEntry struct {
    Job      Job
    Expr     string        // cron expression, e.g. "0 */30 * * * *"
    Timeout  time.Duration // per-run timeout (default 5m if zero)
    MaxRetry int           // retry on error (0 = no retry)
}
