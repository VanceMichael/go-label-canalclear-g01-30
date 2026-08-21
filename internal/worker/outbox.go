package worker

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/store"
)

type Publisher interface {
	Publish(context.Context, string, []byte) error
}

type Outbox struct {
	DB        *store.Database
	Publisher Publisher
	Batch     int
	WorkerID  string
	Now       func() time.Time
	Policy    RetryPolicy
}

func (w Outbox) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = w.RunOnce(ctx)
		}
	}
}

func (w Outbox) RunOnce(ctx context.Context) error {
	if w.DB == nil || w.Publisher == nil {
		return fmt.Errorf("outbox database and publisher are required")
	}
	if w.Batch <= 0 {
		w.Batch = 20
	}
	if w.WorkerID == "" {
		w.WorkerID = fmt.Sprintf("worker-%d", os.Getpid())
	}
	now := time.Now().UTC()
	if w.Now != nil {
		now = w.Now().UTC()
	}
	policy := w.Policy
	if policy.Validate() != nil {
		policy = DefaultRetryPolicy()
	}
	type job struct {
		id      string
		topic   string
		payload []byte
	}
	rows, err := w.DB.Pool.Query(ctx, `UPDATE outbox_jobs SET status='running',locked_at=$1,locked_by=$2,updated_at=$1 WHERE id IN (SELECT id FROM outbox_jobs WHERE status IN ('pending','retry') AND available_at<=$1 ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT $3) RETURNING id,topic,payload`, now, w.WorkerID, w.Batch)
	if err != nil {
		return err
	}
	var jobs []job
	for rows.Next() {
		var current job
		if err := rows.Scan(&current.id, &current.topic, &current.payload); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, current)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, current := range jobs {
		publishErr := w.Publisher.Publish(ctx, current.topic, current.payload)
		status := "delivered"
		lastError := ""
		availableAt := now
		if publishErr != nil {
			var attempts int
			if err := w.DB.Pool.QueryRow(ctx, `SELECT attempts+1 FROM outbox_jobs WHERE id=$1 AND locked_by=$2`, current.id, w.WorkerID).Scan(&attempts); err != nil {
				return err
			}
			status = policy.StatusAfterFailure(attempts)
			lastError = publishErr.Error()
			availableAt = now.Add(policy.Backoff(attempts, 0.5))
		}
		if _, err := w.DB.Pool.Exec(ctx, `UPDATE outbox_jobs SET status=$2,attempts=attempts+1,last_error=$3,locked_at=NULL,locked_by='',available_at=$4,updated_at=$1 WHERE id=$5 AND locked_by=$6`, now, status, lastError, availableAt, current.id, w.WorkerID); err != nil {
			return err
		}
	}
	return nil
}

type LogPublisher struct{}

func (LogPublisher) Publish(ctx context.Context, topic string, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
