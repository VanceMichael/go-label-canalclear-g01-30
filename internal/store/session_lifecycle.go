package store

import (
	"context"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func (db *Database) ActiveSessions(ctx context.Context, userID string, now time.Time) ([]auth.Session, error) {
	if userID == "" || now.IsZero() {
		return nil, domain.ErrInvalid
	}
	rows, err := db.Pool.Query(ctx, `SELECT id,user_id,tenant_id,role,token_hash,created_at,expires_at,revoked_at FROM sessions WHERE user_id=$1 AND revoked_at IS NULL AND expires_at>$2 ORDER BY created_at,id`, userID, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]auth.Session, 0)
	for rows.Next() {
		var session auth.Session
		if err := rows.Scan(&session.ID, &session.UserID, &session.TenantID, &session.Role, &session.TokenHash, &session.CreatedAt, &session.ExpiresAt, &session.RevokedAt); err != nil {
			return nil, err
		}
		result = append(result, session)
	}
	return result, rows.Err()
}

func (db *Database) RevokeSessions(ctx context.Context, userID string, sessionIDs []string, now time.Time) (int64, error) {
	if userID == "" || len(sessionIDs) == 0 || now.IsZero() {
		return 0, domain.ErrInvalid
	}
	tag, err := db.Pool.Exec(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,$3) WHERE user_id=$1 AND id=ANY($2)`, userID, sessionIDs, now.UTC())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (db *Database) DeleteExpiredSessions(ctx context.Context, before time.Time, limit int) (int64, error) {
	if before.IsZero() {
		return 0, domain.ErrInvalid
	}
	if limit <= 0 {
		limit = 500
	}
	tag, err := db.Pool.Exec(ctx, `DELETE FROM sessions WHERE id IN (SELECT id FROM sessions WHERE expires_at<$1 OR (revoked_at IS NOT NULL AND revoked_at<$1) ORDER BY expires_at,id LIMIT $2)`, before.UTC(), limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
