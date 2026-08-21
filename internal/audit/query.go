package audit

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type Filter struct {
	ActorID  string
	Action   string
	ObjectID string
	Outcome  string
	From     *time.Time
	To       *time.Time
	Sequence int64
}

type Summary struct {
	Total        int
	FirstAt      *time.Time
	LastAt       *time.Time
	ByAction     map[string]int
	ByOutcome    map[string]int
	UniqueActors int
}

func (filter Filter) Validate() error {
	if filter.Sequence < 0 {
		return fmt.Errorf("%w: audit sequence", domain.ErrInvalid)
	}
	if filter.From != nil && filter.To != nil && filter.To.Before(*filter.From) {
		return fmt.Errorf("%w: audit date range", domain.ErrInvalid)
	}
	return nil
}

func FilterEvents(events []Event, filter Filter) ([]Event, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	action := strings.TrimSpace(filter.Action)
	result := make([]Event, 0, len(events))
	for _, event := range events {
		if filter.ActorID != "" && event.ActorID != filter.ActorID {
			continue
		}
		if action != "" && event.Action != action {
			continue
		}
		if filter.ObjectID != "" && event.ObjectID != filter.ObjectID {
			continue
		}
		if filter.Outcome != "" && event.Outcome != filter.Outcome {
			continue
		}
		if filter.Sequence > 0 && event.Sequence <= filter.Sequence {
			continue
		}
		if filter.From != nil && event.OccurredAt.Before(filter.From.UTC()) {
			continue
		}
		if filter.To != nil && event.OccurredAt.After(filter.To.UTC()) {
			continue
		}
		result = append(result, cloneEvent(event))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Sequence == result[j].Sequence {
			return result[i].ID < result[j].ID
		}
		return result[i].Sequence < result[j].Sequence
	})
	return result, nil
}

func Summarize(events []Event) Summary {
	summary := Summary{Total: len(events), ByAction: make(map[string]int), ByOutcome: make(map[string]int)}
	actors := make(map[string]struct{})
	for _, event := range events {
		summary.ByAction[event.Action]++
		summary.ByOutcome[event.Outcome]++
		actors[event.ActorID] = struct{}{}
		occurredAt := event.OccurredAt.UTC()
		if summary.FirstAt == nil || occurredAt.Before(*summary.FirstAt) {
			copyTime := occurredAt
			summary.FirstAt = &copyTime
		}
		if summary.LastAt == nil || occurredAt.After(*summary.LastAt) {
			copyTime := occurredAt
			summary.LastAt = &copyTime
		}
	}
	summary.UniqueActors = len(actors)
	return summary
}

func cloneEvent(event Event) Event {
	out := event
	out.Details = make(map[string]string, len(event.Details))
	for key, value := range event.Details {
		out.Details[key] = value
	}
	return out
}
