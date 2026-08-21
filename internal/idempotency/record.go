package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type Status string

const (
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

type Record struct {
	TenantID       string
	Key            string
	Operation      string
	RequestHash    string
	Status         Status
	ResponseCode   int
	ResponseBody   []byte
	FailureMessage string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ExpiresAt      time.Time
	Version        int64
}

type Decision string

const (
	DecisionStart    Decision = "start"
	DecisionReplay   Decision = "replay"
	DecisionConflict Decision = "conflict"
	DecisionBusy     Decision = "busy"
)

func Fingerprint(method, path string, body []byte) string {
	canonical := strings.ToUpper(strings.TrimSpace(method)) + "\n" + strings.TrimSpace(path) + "\n" + string(body)
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func NewRecord(tenantID, key, operation, requestHash string, now time.Time, ttl time.Duration) (Record, error) {
	tenantID = strings.TrimSpace(tenantID)
	key = strings.TrimSpace(key)
	operation = strings.TrimSpace(operation)
	requestHash = strings.TrimSpace(requestHash)
	if tenantID == "" || len(key) < 8 || len(key) > 128 || operation == "" || len(requestHash) != 64 || now.IsZero() || ttl <= 0 {
		return Record{}, fmt.Errorf("%w: idempotency record", domain.ErrInvalid)
	}
	now = now.UTC()
	return Record{
		TenantID: tenantID, Key: key, Operation: operation, RequestHash: requestHash,
		Status: StatusProcessing, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(ttl), Version: 1,
	}, nil
}

func Decide(existing *Record, requestHash string, now time.Time, staleAfter time.Duration) Decision {
	if existing == nil || !existing.ExpiresAt.After(now.UTC()) {
		return DecisionStart
	}
	if existing.RequestHash != requestHash {
		return DecisionConflict
	}
	switch existing.Status {
	case StatusCompleted:
		return DecisionReplay
	case StatusProcessing:
		if staleAfter > 0 && !existing.UpdatedAt.Add(staleAfter).After(now.UTC()) {
			return DecisionStart
		}
		return DecisionBusy
	case StatusFailed:
		return DecisionStart
	default:
		return DecisionConflict
	}
}

func Complete(record Record, statusCode int, body []byte, now time.Time, expectedVersion int64) (Record, error) {
	if record.Status != StatusProcessing || record.Version != expectedVersion || statusCode < 200 || statusCode >= 300 || now.Before(record.UpdatedAt) {
		return record, fmt.Errorf("%w: idempotency completion", domain.ErrConflict)
	}
	out := clone(record)
	out.Status = StatusCompleted
	out.ResponseCode = statusCode
	out.ResponseBody = append([]byte(nil), body...)
	out.FailureMessage = ""
	out.UpdatedAt = now.UTC()
	out.Version++
	return out, nil
}

func Fail(record Record, message string, now time.Time, expectedVersion int64) (Record, error) {
	message = strings.TrimSpace(message)
	if record.Status != StatusProcessing || record.Version != expectedVersion || message == "" || now.Before(record.UpdatedAt) {
		return record, fmt.Errorf("%w: idempotency failure", domain.ErrConflict)
	}
	out := clone(record)
	out.Status = StatusFailed
	out.ResponseCode = 0
	out.ResponseBody = nil
	out.FailureMessage = message
	out.UpdatedAt = now.UTC()
	out.Version++
	return out, nil
}

func Restart(record Record, now time.Time, expectedVersion int64) (Record, error) {
	if record.Version != expectedVersion || record.Status == StatusCompleted || !record.ExpiresAt.After(now.UTC()) {
		return record, fmt.Errorf("%w: idempotency restart", domain.ErrConflict)
	}
	out := clone(record)
	out.Status = StatusProcessing
	out.ResponseCode = 0
	out.ResponseBody = nil
	out.FailureMessage = ""
	out.UpdatedAt = now.UTC()
	out.Version++
	return out, nil
}

func clone(record Record) Record {
	out := record
	out.ResponseBody = append([]byte(nil), record.ResponseBody...)
	return out
}
