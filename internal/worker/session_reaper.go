package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/store"
)

type SessionReaper struct {
	DB        *store.Database
	Batch     int
	Retention time.Duration
	Now       func() time.Time
}

func (reaper SessionReaper) Run(ctx context.Context, interval time.Duration) error {
	if reaper.DB == nil {
		return fmt.Errorf("session reaper database is required")
	}
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := reaper.RunOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (reaper SessionReaper) RunOnce(ctx context.Context) (int64, error) {
	if reaper.DB == nil {
		return 0, fmt.Errorf("session reaper database is required")
	}
	now := time.Now().UTC()
	if reaper.Now != nil {
		now = reaper.Now().UTC()
	}
	retention := reaper.Retention
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	batch := reaper.Batch
	if batch <= 0 {
		batch = 500
	}
	return reaper.DB.DeleteExpiredSessions(ctx, now.Add(-retention), batch)
}
