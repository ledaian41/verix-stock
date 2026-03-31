package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	TaskQueueKey = "verix:task_queue"
)

type TaskType string

const (
	TaskExtract   TaskType = "extract"
	TaskSynthesize TaskType = "synthesize"
)

type Task struct {
	Type      TaskType `json:"type"`
	Ticker    string   `json:"ticker"`
	ArticleID uint     `json:"article_id,omitempty"` // For extraction
	Retry     int      `json:"retry"`
	CreatedAt time.Time `json:"created_at"`
}

type TaskQueue struct {
	rdb *redis.Client
}

func NewTaskQueue(rdb *redis.Client) *TaskQueue {
	return &TaskQueue{rdb: rdb}
}

func (q *TaskQueue) Enqueue(ctx context.Context, task *Task) error {
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return q.rdb.LPush(ctx, TaskQueueKey, data).Err()
}

func (q *TaskQueue) Dequeue(ctx context.Context) (*Task, error) {
	// Blocking pop with a 5s timeout to allow context cancellation
	res, err := q.rdb.BRPop(ctx, 5*time.Second, TaskQueueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Timeout, no task
		}
		return nil, err
	}

	if len(res) < 2 {
		return nil, fmt.Errorf("unexpected brpop result length")
	}

	var task Task
	if err := json.Unmarshal([]byte(res[1]), &task); err != nil {
		return nil, err
	}

	return &task, nil
}
