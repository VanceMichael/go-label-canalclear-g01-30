package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func TestDefaultSessionPolicyIsValid(t *testing.T) {
	policy := DefaultSessionPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	if policy.TTL != 8*time.Hour || policy.MaximumActive != 5 {
		t.Fatalf("policy=%+v", policy)
	}
}

func TestSessionPolicyRejectsUnsafeValues(t *testing.T) {
	valid := DefaultSessionPolicy()
	tests := []SessionPolicy{
		{TTL: time.Minute, AbsoluteLifetime: time.Hour, MaximumActive: 1, RefreshBeforeExpiry: 30 * time.Second},
		{TTL: valid.TTL, AbsoluteLifetime: time.Hour, MaximumActive: valid.MaximumActive, RefreshBeforeExpiry: time.Minute},
		{TTL: valid.TTL, AbsoluteLifetime: valid.AbsoluteLifetime, MaximumActive: 0, RefreshBeforeExpiry: time.Minute},
		{TTL: valid.TTL, AbsoluteLifetime: valid.AbsoluteLifetime, MaximumActive: valid.MaximumActive, RefreshBeforeExpiry: valid.TTL},
	}
	for index, policy := range tests {
		if err := policy.Validate(); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("case %d error=%v", index, err)
		}
	}
}

func TestSessionsToRevokeSelectsOldestActiveSessions(t *testing.T) {
	now := time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)
	policy := DefaultSessionPolicy()
	policy.MaximumActive = 2
	revokedAt := now.Add(-time.Hour)
	sessions := []Session{
		{ID: "new", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)},
		{ID: "expired", CreatedAt: now.Add(-5 * time.Hour), ExpiresAt: now.Add(-time.Minute)},
		{ID: "old", CreatedAt: now.Add(-3 * time.Hour), ExpiresAt: now.Add(time.Hour)},
		{ID: "revoked", CreatedAt: now.Add(-4 * time.Hour), ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt},
		{ID: "middle", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(time.Hour)},
	}
	selected, err := SessionsToRevoke(sessions, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].ID != "old" || selected[1].ID != "middle" {
		t.Fatalf("selected=%+v", selected)
	}
}

func TestSessionRefreshRespectsRollingAndAbsoluteExpiry(t *testing.T) {
	policy := DefaultSessionPolicy()
	created := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	session := Session{ID: "session", CreatedAt: created, ExpiresAt: created.Add(policy.TTL)}
	if ShouldRefresh(session, policy, created.Add(time.Hour)) {
		t.Fatal("session should not refresh early")
	}
	now := session.ExpiresAt.Add(-15 * time.Minute)
	if !ShouldRefresh(session, policy, now) {
		t.Fatal("session should refresh in refresh window")
	}
	expiry, err := RefreshedExpiry(session, policy, now)
	if err != nil || !expiry.Equal(now.Add(policy.TTL)) {
		t.Fatalf("expiry=%v err=%v", expiry, err)
	}
	session.CreatedAt = now.Add(-policy.AbsoluteLifetime)
	if ShouldRefresh(session, policy, now) {
		t.Fatal("session at absolute lifetime must not refresh")
	}
	if _, err := RefreshedExpiry(session, policy, now); !errors.Is(err, domain.ErrState) {
		t.Fatalf("refresh error=%v", err)
	}
}
