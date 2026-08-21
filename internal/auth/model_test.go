package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func TestSessionLifecycleAndDisabledUser(t *testing.T) {
	now := time.Now().UTC()
	user := User{ID: "user-1", TenantID: "tenant-1", Email: "ops@example.test", Role: RoleDispatcher}
	session, token, err := NewSession(user, now, time.Hour)
	if err != nil || token == "" {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	if err := Authenticate(session, user, token, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Authenticate(session, user, token, now.Add(2*time.Hour)); !errors.Is(err, domain.ErrExpired) {
		t.Fatalf("expired error=%v", err)
	}
	user.Disabled = true
	if err := Authenticate(session, user, token, now.Add(time.Minute)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("disabled error=%v", err)
	}
	user.Disabled = false
	session = Revoke(session, now.Add(2*time.Minute))
	if err := Authenticate(session, user, token, now.Add(3*time.Minute)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("revoked error=%v", err)
	}
}

func TestPasswordsAreSaltedAndVerified(t *testing.T) {
	left, err := HashPassword("very-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	right, err := HashPassword("very-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	if left == right {
		t.Fatal("password hashes reused salt")
	}
	if err := VerifyPassword(left, "very-secure-password"); err != nil {
		t.Fatal(err)
	}
}
