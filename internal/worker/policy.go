package worker

import (
	"fmt"
	"math"
	"time"
)

type RetryPolicy struct {
	MaximumAttempts int
	InitialBackoff  time.Duration
	MaximumBackoff  time.Duration
	LockTimeout     time.Duration
	JitterFraction  float64
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaximumAttempts: 8,
		InitialBackoff:  2 * time.Second,
		MaximumBackoff:  5 * time.Minute,
		LockTimeout:     2 * time.Minute,
		JitterFraction:  0,
	}
}

func (policy RetryPolicy) Validate() error {
	if policy.MaximumAttempts < 1 || policy.MaximumAttempts > 100 {
		return fmt.Errorf("maximum attempts must be between 1 and 100")
	}
	if policy.InitialBackoff <= 0 || policy.MaximumBackoff < policy.InitialBackoff {
		return fmt.Errorf("invalid retry backoff")
	}
	if policy.LockTimeout <= 0 {
		return fmt.Errorf("lock timeout must be positive")
	}
	if policy.JitterFraction < 0 || policy.JitterFraction > 0.5 {
		return fmt.Errorf("jitter fraction must be between 0 and 0.5")
	}
	return nil
}

func (policy RetryPolicy) Backoff(attempt int, entropy float64) time.Duration {
	if policy.Validate() != nil {
		policy = DefaultRetryPolicy()
	}
	if attempt < 1 {
		attempt = 1
	}
	exponent := attempt - 1
	if exponent > 30 {
		exponent = 30
	}
	base := float64(policy.InitialBackoff) * math.Pow(2, float64(exponent))
	if base > float64(policy.MaximumBackoff) {
		base = float64(policy.MaximumBackoff)
	}
	if entropy < 0 {
		entropy = 0
	}
	if entropy > 1 {
		entropy = 1
	}
	spread := base * policy.JitterFraction
	adjustment := (entropy*2 - 1) * spread
	return time.Duration(base + adjustment)
}

func (policy RetryPolicy) StatusAfterFailure(attempt int) string {
	if attempt >= policy.MaximumAttempts {
		return "failed"
	}
	return "retry"
}
