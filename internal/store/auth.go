package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (db *Database) Bootstrap(ctx context.Context) error {
	passwordHash, err := auth.HashPassword("canalclear-demo-password")
	if err != nil {
		return err
	}
	users := []auth.User{
		{ID: "carrier-demo", TenantID: "demo-port", Email: "carrier@canalclear.test", PasswordHash: passwordHash, Role: auth.RoleCarrier, Version: 1},
		{ID: "dispatcher-demo", TenantID: "demo-port", Email: "dispatcher@canalclear.test", PasswordHash: passwordHash, Role: auth.RoleDispatcher, Version: 1},
		{ID: "customs-demo", TenantID: "demo-port", Email: "customs@canalclear.test", PasswordHash: passwordHash, Role: auth.RoleCustoms, Version: 1},
		{ID: "lock-demo", TenantID: "demo-port", Email: "lock@canalclear.test", PasswordHash: passwordHash, Role: auth.RoleLock, Version: 1},
		{ID: "auditor-demo", TenantID: "demo-port", Email: "auditor@canalclear.test", PasswordHash: passwordHash, Role: auth.RoleAuditor, Version: 1},
	}
	for _, user := range users {
		if _, err := db.Pool.Exec(ctx, `INSERT INTO users(id,tenant_id,email,password_hash,role,disabled,version) VALUES($1,$2,$3,$4,$5,false,1) ON CONFLICT(email) DO NOTHING`, user.ID, user.TenantID, user.Email, user.PasswordHash, user.Role); err != nil {
			return err
		}
	}
	return nil
}

func (db *Database) Login(ctx context.Context, email, password string, now time.Time, ttl time.Duration) (string, auth.Session, error) {
	var user auth.User
	err := db.Pool.QueryRow(ctx, `SELECT id,tenant_id,email,password_hash,role,disabled,version FROM users WHERE email=$1`, email).Scan(&user.ID, &user.TenantID, &user.Email, &user.PasswordHash, &user.Role, &user.Disabled, &user.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.Session{}, domain.ErrForbidden
	}
	if err != nil {
		return "", auth.Session{}, err
	}
	if err := auth.VerifyPassword(user.PasswordHash, password); err != nil || user.Disabled {
		return "", auth.Session{}, domain.ErrForbidden
	}
	session, token, err := auth.NewSession(user, now, ttl)
	if err != nil {
		return "", auth.Session{}, err
	}
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions(id,user_id,tenant_id,role,token_hash,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, session.ID, session.UserID, session.TenantID, session.Role, session.TokenHash, session.CreatedAt, session.ExpiresAt)
	return token, session, err
}

func (db *Database) Authenticate(ctx context.Context, token string, now time.Time) (auth.User, auth.Session, error) {
	var user auth.User
	var session auth.Session
	err := db.Pool.QueryRow(ctx, `SELECT u.id,u.tenant_id,u.email,u.password_hash,u.role,u.disabled,u.version,s.id,s.user_id,s.tenant_id,s.role,s.token_hash,s.created_at,s.expires_at,s.revoked_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1`, auth.TokenHash(token)).Scan(&user.ID, &user.TenantID, &user.Email, &user.PasswordHash, &user.Role, &user.Disabled, &user.Version, &session.ID, &session.UserID, &session.TenantID, &session.Role, &session.TokenHash, &session.CreatedAt, &session.ExpiresAt, &session.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, auth.Session{}, domain.ErrForbidden
	}
	if err != nil {
		return auth.User{}, auth.Session{}, err
	}
	if err := auth.Authenticate(session, user, token, now); err != nil {
		return auth.User{}, auth.Session{}, err
	}
	return user, session, nil
}

func (db *Database) Logout(ctx context.Context, token string, now time.Time) error {
	tag, err := db.Pool.Exec(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE token_hash=$1`, auth.TokenHash(token), now.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: session", domain.ErrNotFound)
	}
	return nil
}
