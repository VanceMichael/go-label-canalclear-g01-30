package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type Event struct {
	ID           string
	TenantID     string
	ActorID      string
	Action       string
	ObjectType   string
	ObjectID     string
	Outcome      string
	Details      map[string]string
	OccurredAt   time.Time
	Sequence     int64
	PreviousHash string
	Hash         string
}

func Append(previous Event, event Event) (Event, error) {
	if event.ID == "" || event.TenantID == "" || event.ActorID == "" || event.Action == "" || event.ObjectType == "" || event.ObjectID == "" || event.Outcome == "" || event.OccurredAt.IsZero() {
		return Event{}, fmt.Errorf("%w: audit event", domain.ErrInvalid)
	}
	if previous.ID != "" && (previous.TenantID != event.TenantID || event.OccurredAt.Before(previous.OccurredAt)) {
		return Event{}, fmt.Errorf("%w: audit predecessor", domain.ErrConflict)
	}
	event.Details = clone(event.Details)
	event.Sequence = previous.Sequence + 1
	event.PreviousHash = previous.Hash
	event.Hash = hash(event)
	return event, nil
}

func Verify(events []Event) error {
	ordered := append([]Event(nil), events...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Sequence < ordered[j].Sequence })
	var previous Event
	for index, event := range ordered {
		if event.Sequence != int64(index+1) || event.PreviousHash != previous.Hash || event.Hash != hash(event) {
			return fmt.Errorf("%w: audit chain at sequence %d", domain.ErrConflict, event.Sequence)
		}
		previous = event
	}
	return nil
}

func hash(event Event) string {
	keys := make([]string, 0, len(event.Details))
	for key := range event.Details {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	details := make([][2]string, 0, len(keys))
	for _, key := range keys {
		details = append(details, [2]string{key, event.Details[key]})
	}
	payload, _ := json.Marshal([]any{event.ID, event.TenantID, event.ActorID, event.Action, event.ObjectType, event.ObjectID, event.Outcome, event.OccurredAt.UTC(), event.Sequence, event.PreviousHash, details})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func clone(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
