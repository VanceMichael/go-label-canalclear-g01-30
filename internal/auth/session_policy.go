package auth

import (
	"fmt"
	"sort"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type SessionPolicy struct {
	TTL                 time.Duration
	AbsoluteLifetime    time.Duration
	MaximumActive       int
	RefreshBeforeExpiry time.Duration
}

func (policy SessionPolicy) Validate() error {
	if policy.TTL < 5*time.Minute || policy.TTL > 24*time.Hour {
		return fmt.Errorf("%w: session TTL", domain.ErrInvalid)
	}
	if policy.AbsoluteLifetime < policy.TTL || policy.AbsoluteLifetime > 30*24*time.Hour {
		return fmt.Errorf("%w: absolute session lifetime", domain.ErrInvalid)
	}
	if policy.MaximumActive < 1 || policy.MaximumActive > 20 {
		return fmt.Errorf("%w: active session limit", domain.ErrInvalid)
	}
	if policy.RefreshBeforeExpiry <= 0 || policy.RefreshBeforeExpiry >= policy.TTL {
		return fmt.Errorf("%w: refresh window", domain.ErrInvalid)
	}
	return nil
}

func DefaultSessionPolicy() SessionPolicy {
	return SessionPolicy{
		TTL:                 8 * time.Hour,
		AbsoluteLifetime:    7 * 24 * time.Hour,
		MaximumActive:       5,
		RefreshBeforeExpiry: 30 * time.Minute,
	}
}

func SessionsToRevoke(sessions []Session, policy SessionPolicy, now time.Time) ([]Session, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	now = now.UTC()
	active := make([]Session, 0, len(sessions))
	for _, session := range sessions {
		if session.RevokedAt == nil && session.ExpiresAt.After(now) {
			active = append(active, session)
		}
	}
	if len(active) < policy.MaximumActive {
		return []Session{}, nil
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].CreatedAt.Equal(active[j].CreatedAt) {
			return active[i].ID < active[j].ID
		}
		return active[i].CreatedAt.Before(active[j].CreatedAt)
	})
	count := len(active) - policy.MaximumActive + 1
	return append([]Session(nil), active[:count]...), nil
}

func ShouldRefresh(session Session, policy SessionPolicy, now time.Time) bool {
	if policy.Validate() != nil || session.RevokedAt != nil {
		return false
	}
	now = now.UTC()
	if !session.ExpiresAt.After(now) {
		return false
	}
	if now.Sub(session.CreatedAt) >= policy.AbsoluteLifetime {
		return false
	}
	return session.ExpiresAt.Sub(now) <= policy.RefreshBeforeExpiry
}

func RefreshedExpiry(session Session, policy SessionPolicy, now time.Time) (time.Time, error) {
	if !ShouldRefresh(session, policy, now) {
		return time.Time{}, fmt.Errorf("%w: session is not refreshable", domain.ErrState)
	}
	candidate := now.UTC().Add(policy.TTL)
	absolute := session.CreatedAt.Add(policy.AbsoluteLifetime)
	if candidate.After(absolute) {
		candidate = absolute
	}
	return candidate, nil
}
