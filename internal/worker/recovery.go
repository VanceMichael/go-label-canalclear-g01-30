package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/store"
)

type Recovery struct {
	DB     *store.Database
	Policy RetryPolicy
	Now    func() time.Time
}

func (recovery Recovery) RecoverExpiredLocks(ctx context.Context) (int64, error) {
	if recovery.DB == nil {
		return 0, fmt.Errorf("recovery database is required")
	}
	policy := recovery.Policy
	if policy.Validate() != nil {
		policy = DefaultRetryPolicy()
	}
	now := time.Now().UTC()
	if recovery.Now != nil {
		now = recovery.Now().UTC()
	}
	cutoff := now.Add(-policy.LockTimeout)
	tag, err := recovery.DB.Pool.Exec(ctx, `UPDATE outbox_jobs SET status=CASE WHEN attempts >= $3 THEN 'failed' ELSE 'retry' END,locked_at=NULL,locked_by='',last_error=CASE WHEN last_error='' THEN 'worker lock expired' ELSE last_error END,available_at=$1,updated_at=$1 WHERE status='running' AND locked_at<$2`, now, cutoff, policy.MaximumAttempts)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (recovery Recovery) DeleteDeliveredBefore(ctx context.Context, before time.Time, batch int) (int64, error) {
	if recovery.DB == nil || before.IsZero() {
		return 0, fmt.Errorf("cleanup database and cutoff are required")
	}
	if batch <= 0 {
		batch = 500
	}
	tag, err := recovery.DB.Pool.Exec(ctx, `DELETE FROM outbox_jobs WHERE id IN (SELECT id FROM outbox_jobs WHERE status='delivered' AND updated_at<$1 ORDER BY updated_at,id LIMIT $2)`, before.UTC(), batch)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (recovery Recovery) CountByStatus(ctx context.Context) (map[string]int64, error) {
	if recovery.DB == nil {
		return nil, fmt.Errorf("recovery database is required")
	}
	rows, err := recovery.DB.Pool.Query(ctx, `SELECT status,count(*) FROM outbox_jobs GROUP BY status ORDER BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}
