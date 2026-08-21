package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

type Role string

const (
	RoleCarrier    Role = "carrier_operator"
	RoleDispatcher Role = "port_dispatcher"
	RoleCustoms    Role = "customs_officer"
	RoleLock       Role = "lock_controller"
	RoleAuditor    Role = "compliance_auditor"
)

type User struct {
	ID           string
	TenantID     string
	Email        string
	PasswordHash string
	Role         Role
	Disabled     bool
	Version      int64
}

type Session struct {
	ID        string
	UserID    string
	TenantID  string
	Role      Role
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func HashPassword(password string) (string, error) {
	if len(password) < 10 || len(password) > 128 {
		return "", fmt.Errorf("%w: password length", domain.ErrInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func VerifyPassword(hash, password string) error {
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return domain.ErrForbidden
	}
	return nil
}

func NewSession(user User, now time.Time, ttl time.Duration) (Session, string, error) {
	if user.ID == "" || user.TenantID == "" || user.Disabled || !validRole(user.Role) || ttl <= 0 {
		return Session{}, "", domain.ErrForbidden
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return Session{}, "", err
	}
	token := hex.EncodeToString(bytes)
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return Session{}, "", err
	}
	session := Session{ID: hex.EncodeToString(idBytes), UserID: user.ID, TenantID: user.TenantID, Role: user.Role, TokenHash: TokenHash(token), CreatedAt: now.UTC(), ExpiresAt: now.UTC().Add(ttl)}
	return session, token, nil
}

func Authenticate(session Session, user User, token string, now time.Time) error {
	if session.UserID != user.ID || session.TenantID != user.TenantID || user.Disabled || session.RevokedAt != nil {
		return domain.ErrForbidden
	}
	if !session.ExpiresAt.After(now) {
		return domain.ErrExpired
	}
	if session.TokenHash != TokenHash(strings.TrimSpace(token)) {
		return domain.ErrForbidden
	}
	return nil
}

func Revoke(session Session, now time.Time) Session {
	if session.RevokedAt == nil {
		at := now.UTC()
		session.RevokedAt = &at
	}
	return session
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func validRole(role Role) bool {
	switch role {
	case RoleCarrier, RoleDispatcher, RoleCustoms, RoleLock, RoleAuditor:
		return true
	default:
		return false
	}
}
