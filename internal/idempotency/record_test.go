package idempotency

import (
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func validRecord(t *testing.T) Record {
	t.Helper()
	now := time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)
	record, err := NewRecord("tenant-a", "request-key-0001", "voyage.declare", Fingerprint("POST", "/v1/voyages", []byte(`{"id":"one"}`)), now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestFingerprintSeparatesMethodPathAndBody(t *testing.T) {
	first := Fingerprint("post", "/v1/voyages", []byte("one"))
	if len(first) != 64 {
		t.Fatalf("fingerprint=%q", first)
	}
	if first != Fingerprint(" POST ", " /v1/voyages ", []byte("one")) {
		t.Fatal("canonical method and path should match")
	}
	if first == Fingerprint("PUT", "/v1/voyages", []byte("one")) {
		t.Fatal("method must affect fingerprint")
	}
	if first == Fingerprint("POST", "/v1/voyages/other", []byte("one")) {
		t.Fatal("path must affect fingerprint")
	}
	if first == Fingerprint("POST", "/v1/voyages", []byte("two")) {
		t.Fatal("body must affect fingerprint")
	}
}

func TestNewRecordNormalizesTimeAndInitializesLifecycle(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, location)
	record, err := NewRecord("tenant-a", "request-key-0001", "voyage.declare", Fingerprint("POST", "/", nil), now, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusProcessing || record.Version != 1 || record.CreatedAt.Location() != time.UTC {
		t.Fatalf("record=%+v", record)
	}
	if !record.ExpiresAt.Equal(now.Add(2*time.Hour)) || !record.UpdatedAt.Equal(now) {
		t.Fatalf("record times=%+v", record)
	}
}

func TestNewRecordRejectsMalformedInputs(t *testing.T) {
	now := time.Now()
	hash := Fingerprint("POST", "/", nil)
	tests := []struct {
		tenant    string
		key       string
		operation string
		hash      string
		ttl       time.Duration
	}{
		{tenant: "", key: "request-key", operation: "op", hash: hash, ttl: time.Hour},
		{tenant: "tenant", key: "short", operation: "op", hash: hash, ttl: time.Hour},
		{tenant: "tenant", key: "request-key", operation: "", hash: hash, ttl: time.Hour},
		{tenant: "tenant", key: "request-key", operation: "op", hash: "bad", ttl: time.Hour},
		{tenant: "tenant", key: "request-key", operation: "op", hash: hash, ttl: 0},
	}
	for index, test := range tests {
		if _, err := NewRecord(test.tenant, test.key, test.operation, test.hash, now, test.ttl); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("case %d error=%v", index, err)
		}
	}
}

func TestDecisionCoversReplayConflictBusyAndRecovery(t *testing.T) {
	record := validRecord(t)
	now := record.CreatedAt.Add(time.Minute)
	if got := Decide(nil, record.RequestHash, now, time.Minute); got != DecisionStart {
		t.Fatalf("nil decision=%s", got)
	}
	if got := Decide(&record, "different", now, time.Minute); got != DecisionConflict {
		t.Fatalf("hash decision=%s", got)
	}
	if got := Decide(&record, record.RequestHash, now, 2*time.Minute); got != DecisionBusy {
		t.Fatalf("busy decision=%s", got)
	}
	if got := Decide(&record, record.RequestHash, now, 30*time.Second); got != DecisionStart {
		t.Fatalf("stale decision=%s", got)
	}
	record.Status = StatusCompleted
	if got := Decide(&record, record.RequestHash, now, time.Minute); got != DecisionReplay {
		t.Fatalf("replay decision=%s", got)
	}
	record.ExpiresAt = now
	if got := Decide(&record, record.RequestHash, now, time.Minute); got != DecisionStart {
		t.Fatalf("expired decision=%s", got)
	}
}

func TestCompletionAndFailureAreVersionedAndIsolated(t *testing.T) {
	record := validRecord(t)
	body := []byte(`{"id":"voyage"}`)
	completed, err := Complete(record, 201, body, record.CreatedAt.Add(time.Minute), 1)
	if err != nil {
		t.Fatal(err)
	}
	body[0] = 'x'
	if completed.Status != StatusCompleted || completed.Version != 2 || completed.ResponseBody[0] != '{' {
		t.Fatalf("completed=%+v", completed)
	}
	if _, err := Complete(record, 500, nil, record.CreatedAt.Add(time.Minute), 1); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("bad status error=%v", err)
	}
	failed, err := Fail(record, "gateway unavailable", record.CreatedAt.Add(time.Minute), 1)
	if err != nil || failed.Status != StatusFailed || failed.Version != 2 {
		t.Fatalf("failed=%+v err=%v", failed, err)
	}
	restarted, err := Restart(failed, record.CreatedAt.Add(2*time.Minute), failed.Version)
	if err != nil || restarted.Status != StatusProcessing || restarted.FailureMessage != "" || restarted.Version != 3 {
		t.Fatalf("restarted=%+v err=%v", restarted, err)
	}
}
