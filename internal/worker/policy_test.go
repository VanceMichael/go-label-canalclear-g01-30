package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultRetryPolicyIsValid(t *testing.T) {
	policy := DefaultRetryPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	if policy.MaximumAttempts != 8 || policy.InitialBackoff != 2*time.Second || policy.LockTimeout != 2*time.Minute {
		t.Fatalf("policy=%+v", policy)
	}
}

func TestRetryPolicyRejectsInvalidBounds(t *testing.T) {
	valid := DefaultRetryPolicy()
	tests := []RetryPolicy{
		{MaximumAttempts: 0, InitialBackoff: time.Second, MaximumBackoff: time.Second, LockTimeout: time.Second},
		{MaximumAttempts: 101, InitialBackoff: time.Second, MaximumBackoff: time.Second, LockTimeout: time.Second},
		{MaximumAttempts: 3, InitialBackoff: 0, MaximumBackoff: time.Second, LockTimeout: time.Second},
		{MaximumAttempts: 3, InitialBackoff: 2 * time.Second, MaximumBackoff: time.Second, LockTimeout: time.Second},
		{MaximumAttempts: 3, InitialBackoff: time.Second, MaximumBackoff: time.Second, LockTimeout: 0},
		{MaximumAttempts: valid.MaximumAttempts, InitialBackoff: valid.InitialBackoff, MaximumBackoff: valid.MaximumBackoff, LockTimeout: valid.LockTimeout, JitterFraction: 0.75},
	}
	for index, policy := range tests {
		if err := policy.Validate(); err == nil {
			t.Errorf("case %d expected validation error", index)
		}
	}
}

func TestBackoffIsExponentialCappedAndDeterministic(t *testing.T) {
	policy := RetryPolicy{MaximumAttempts: 8, InitialBackoff: time.Second, MaximumBackoff: 5 * time.Second, LockTimeout: time.Minute, JitterFraction: 0}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 5 * time.Second, 5 * time.Second}
	for index, expected := range want {
		if got := policy.Backoff(index+1, 0.5); got != expected {
			t.Errorf("attempt=%d got=%v want=%v", index+1, got, expected)
		}
	}
	if got := policy.Backoff(0, 0.5); got != time.Second {
		t.Fatalf("attempt zero=%v", got)
	}
}

func TestBackoffJitterStaysInsideConfiguredBand(t *testing.T) {
	policy := RetryPolicy{MaximumAttempts: 3, InitialBackoff: 10 * time.Second, MaximumBackoff: time.Minute, LockTimeout: time.Minute, JitterFraction: 0.2}
	low := policy.Backoff(1, 0)
	middle := policy.Backoff(1, 0.5)
	high := policy.Backoff(1, 1)
	if low != 8*time.Second || middle != 10*time.Second || high != 12*time.Second {
		t.Fatalf("jitter low=%v middle=%v high=%v", low, middle, high)
	}
	if got := policy.Backoff(1, -1); got != low {
		t.Fatalf("low clamp=%v", got)
	}
	if got := policy.Backoff(1, 2); got != high {
		t.Fatalf("high clamp=%v", got)
	}
}

func TestFailureStatusChangesAtMaximumAttempts(t *testing.T) {
	policy := DefaultRetryPolicy()
	if got := policy.StatusAfterFailure(policy.MaximumAttempts - 1); got != "retry" {
		t.Fatalf("status=%q", got)
	}
	if got := policy.StatusAfterFailure(policy.MaximumAttempts); got != "failed" {
		t.Fatalf("status=%q", got)
	}
	if got := policy.StatusAfterFailure(policy.MaximumAttempts + 10); got != "failed" {
		t.Fatalf("status=%q", got)
	}
}

func TestLogPublisherPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (LogPublisher{}).Publish(ctx, "voyage.declared", []byte(`{}`)); !errors.Is(err, context.Canceled) {
		t.Fatalf("publish error=%v", err)
	}
	if err := (LogPublisher{}).Publish(context.Background(), "voyage.declared", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxRunStopsPromptlyWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		(Outbox{}).Run(ctx, time.Hour)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("outbox did not stop after cancellation")
	}
}
