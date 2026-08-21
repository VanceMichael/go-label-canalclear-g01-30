package customs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type RetryPolicy struct {
	Attempts int
	Backoff  time.Duration
	Maximum  time.Duration
}

type RetryingGateway struct {
	Next   Gateway
	Policy RetryPolicy
	Sleep  func(context.Context, time.Duration) error
}

func (gateway RetryingGateway) Submit(ctx context.Context, declaration Declaration) (Decision, error) {
	if gateway.Next == nil {
		return Decision{}, fmt.Errorf("%w: customs gateway", domain.ErrInvalid)
	}
	policy := gateway.Policy
	if policy.Attempts <= 0 {
		policy.Attempts = 3
	}
	if policy.Backoff <= 0 {
		policy.Backoff = 100 * time.Millisecond
	}
	if policy.Maximum < policy.Backoff {
		policy.Maximum = 2 * time.Second
	}
	sleep := gateway.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	var lastErr error
	for attempt := 1; attempt <= policy.Attempts; attempt++ {
		decision, err := gateway.Next.Submit(ctx, declaration)
		if err == nil {
			return decision, nil
		}
		lastErr = err
		if !retryable(err) || attempt == policy.Attempts {
			break
		}
		backoff := policy.Backoff << (attempt - 1)
		if backoff > policy.Maximum {
			backoff = policy.Maximum
		}
		if err := sleep(ctx, backoff); err != nil {
			return Decision{}, err
		}
	}
	return Decision{}, fmt.Errorf("customs submission failed: %w", lastErr)
}

func retryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrInvalid) || errors.Is(err, domain.ErrConflict) {
		return false
	}
	return true
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
