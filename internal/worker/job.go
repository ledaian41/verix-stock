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
// GenericJob allows defining a Job with a simple anonymous function.
type GenericJob struct {
	JobName string
	Action  func(context.Context) error
}

func (g *GenericJob) Name() string                     { return g.JobName }
func (g *GenericJob) Run(ctx context.Context) error { return g.Action(ctx) }
