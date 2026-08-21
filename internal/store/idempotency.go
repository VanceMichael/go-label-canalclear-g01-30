package store

import (
	"context"
	"errors"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/idempotency"
	"github.com/jackc/pgx/v5"
)

func (db *Database) BeginIdempotent(ctx context.Context, candidate idempotency.Record, staleAfter time.Duration) (idempotency.Decision, idempotency.Record, error) {
	var decision idempotency.Decision
	var stored idempotency.Record
	err := db.WithTx(ctx, func(tx pgx.Tx) error {
		current, err := getIdempotencyTx(ctx, tx, candidate.TenantID, candidate.Key)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if errors.Is(err, domain.ErrNotFound) || !current.ExpiresAt.After(candidate.CreatedAt) {
			_, err := tx.Exec(ctx, `INSERT INTO idempotency_records(tenant_id,key,operation,request_hash,status,response_code,response_body,failure_message,created_at,updated_at,expires_at,version) VALUES($1,$2,$3,$4,$5,0,'','',$6,$6,$7,1) ON CONFLICT(tenant_id,key) DO UPDATE SET operation=EXCLUDED.operation,request_hash=EXCLUDED.request_hash,status=EXCLUDED.status,response_code=0,response_body='',failure_message='',created_at=EXCLUDED.created_at,updated_at=EXCLUDED.updated_at,expires_at=EXCLUDED.expires_at,version=idempotency_records.version+1 WHERE idempotency_records.expires_at<=EXCLUDED.created_at`, candidate.TenantID, candidate.Key, candidate.Operation, candidate.RequestHash, candidate.Status, candidate.CreatedAt, candidate.ExpiresAt)
			if err != nil {
				return err
			}
			stored, err = getIdempotencyTx(ctx, tx, candidate.TenantID, candidate.Key)
			decision = idempotency.DecisionStart
			return err
		}
		decision = idempotency.Decide(&current, candidate.RequestHash, candidate.CreatedAt, staleAfter)
		stored = current
		if decision != idempotency.DecisionStart {
			return nil
		}
		restarted, err := idempotency.Restart(current, candidate.CreatedAt, current.Version)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE idempotency_records SET status=$4,response_code=0,response_body='',failure_message='',updated_at=$5,version=$6 WHERE tenant_id=$1 AND key=$2 AND version=$3`, restarted.TenantID, restarted.Key, current.Version, restarted.Status, restarted.UpdatedAt, restarted.Version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return domain.ErrConflict
		}
		stored = restarted
		return nil
	})
	return decision, stored, err
}

func (db *Database) CompleteIdempotent(ctx context.Context, record idempotency.Record) error {
	if record.Status != idempotency.StatusCompleted || record.Version < 2 {
		return domain.ErrInvalid
	}
	tag, err := db.Pool.Exec(ctx, `UPDATE idempotency_records SET status=$4,response_code=$5,response_body=$6,failure_message='',updated_at=$7,version=$8 WHERE tenant_id=$1 AND key=$2 AND version=$3`, record.TenantID, record.Key, record.Version-1, record.Status, record.ResponseCode, record.ResponseBody, record.UpdatedAt, record.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return nil
}

func (db *Database) FailIdempotent(ctx context.Context, record idempotency.Record) error {
	if record.Status != idempotency.StatusFailed || record.Version < 2 {
		return domain.ErrInvalid
	}
	tag, err := db.Pool.Exec(ctx, `UPDATE idempotency_records SET status=$4,response_code=0,response_body='',failure_message=$5,updated_at=$6,version=$7 WHERE tenant_id=$1 AND key=$2 AND version=$3`, record.TenantID, record.Key, record.Version-1, record.Status, record.FailureMessage, record.UpdatedAt, record.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return nil
}

func getIdempotencyTx(ctx context.Context, tx pgx.Tx, tenantID, key string) (idempotency.Record, error) {
	var record idempotency.Record
	err := tx.QueryRow(ctx, `SELECT tenant_id,key,operation,request_hash,status,response_code,response_body,failure_message,created_at,updated_at,expires_at,version FROM idempotency_records WHERE tenant_id=$1 AND key=$2 FOR UPDATE`, tenantID, key).Scan(&record.TenantID, &record.Key, &record.Operation, &record.RequestHash, &record.Status, &record.ResponseCode, &record.ResponseBody, &record.FailureMessage, &record.CreatedAt, &record.UpdatedAt, &record.ExpiresAt, &record.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return idempotency.Record{}, domain.ErrNotFound
	}
	return record, err
}
