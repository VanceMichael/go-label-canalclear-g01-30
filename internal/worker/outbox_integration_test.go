package worker

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/store"
)

type controlledPublisher struct {
	err    error
	topics []string
}

func (publisher *controlledPublisher) Publish(ctx context.Context, topic string, _ []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	publisher.topics = append(publisher.topics, topic)
	return publisher.err
}

func workerDB(t *testing.T) *store.Database {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := store.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	lock, err := db.Pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lock.Exec(context.Background(), `SELECT pg_advisory_lock(74001001)`); err != nil {
		lock.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `TRUNCATE outbox_jobs CASCADE`)
		_, _ = lock.Exec(context.Background(), `SELECT pg_advisory_unlock(74001001)`)
		lock.Release()
	})
	if _, err := db.Pool.Exec(context.Background(), `TRUNCATE outbox_jobs CASCADE`); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertOutboxJob(t *testing.T, db *store.Database, id string, availableAt time.Time) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `INSERT INTO outbox_jobs(id,tenant_id,topic,payload,status,available_at,created_at,updated_at) VALUES($1,'demo-port','voyage.declared','{}','pending',$2,$2,$2)`, id, availableAt)
	if err != nil {
		t.Fatal(err)
	}
}

func TestOutboxDeliversClaimedJobExactlyOnce(t *testing.T) {
	db := workerDB(t)
	now := time.Now().UTC()
	insertOutboxJob(t, db, "job-deliver", now.Add(-time.Second))
	publisher := &controlledPublisher{}
	outbox := Outbox{DB: db, Publisher: publisher, Batch: 10, WorkerID: "worker-test", Now: func() time.Time { return now }, Policy: DefaultRetryPolicy()}
	if err := outbox.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(publisher.topics) != 1 || publisher.topics[0] != "voyage.declared" {
		t.Fatalf("topics=%v", publisher.topics)
	}
	var status string
	var attempts int
	if err := db.Pool.QueryRow(context.Background(), `SELECT status,attempts FROM outbox_jobs WHERE id='job-deliver'`).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "delivered" || attempts != 1 {
		t.Fatalf("status=%s attempts=%d", status, attempts)
	}
	if err := outbox.RunOnce(context.Background()); err != nil || len(publisher.topics) != 1 {
		t.Fatalf("second run topics=%v err=%v", publisher.topics, err)
	}
}

func TestOutboxMarksPermanentFailureAtAttemptLimit(t *testing.T) {
	db := workerDB(t)
	now := time.Now().UTC()
	insertOutboxJob(t, db, "job-fail", now.Add(-time.Second))
	publisher := &controlledPublisher{err: errors.New("broker unavailable")}
	policy := DefaultRetryPolicy()
	policy.MaximumAttempts = 1
	outbox := Outbox{DB: db, Publisher: publisher, WorkerID: "worker-fail", Now: func() time.Time { return now }, Policy: policy}
	if err := outbox.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var status, lastError string
	var attempts int
	if err := db.Pool.QueryRow(context.Background(), `SELECT status,attempts,last_error FROM outbox_jobs WHERE id='job-fail'`).Scan(&status, &attempts, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || attempts != 1 || lastError != "broker unavailable" {
		t.Fatalf("status=%s attempts=%d last_error=%q", status, attempts, lastError)
	}
}

func TestRecoveryReturnsExpiredWorkerLockToRetry(t *testing.T) {
	db := workerDB(t)
	now := time.Now().UTC()
	insertOutboxJob(t, db, "job-stale", now.Add(-time.Hour))
	if _, err := db.Pool.Exec(context.Background(), `UPDATE outbox_jobs SET status='running',locked_at=$2,locked_by='dead-worker' WHERE id=$1`, "job-stale", now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	policy := DefaultRetryPolicy()
	policy.LockTimeout = time.Minute
	count, err := (Recovery{DB: db, Policy: policy, Now: func() time.Time { return now }}).RecoverExpiredLocks(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	var status, lockedBy string
	if err := db.Pool.QueryRow(context.Background(), `SELECT status,locked_by FROM outbox_jobs WHERE id='job-stale'`).Scan(&status, &lockedBy); err != nil {
		t.Fatal(err)
	}
	if status != "retry" || lockedBy != "" {
		t.Fatalf("status=%s locked_by=%q", status, lockedBy)
	}
}
