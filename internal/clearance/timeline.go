package clearance

import (
	"fmt"
	"sort"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type VoyageEvent struct {
	ID         string
	VoyageID   string
	Status     VoyageStatus
	OccurredAt time.Time
	ActorID    string
	Reason     string
	Version    int64
}

type Timeline struct {
	VoyageID string
	Events   []VoyageEvent
	Current  VoyageStatus
	Version  int64
}

func NewTimeline(voyage Voyage, actorID string, at time.Time) (Timeline, error) {
	if voyage.ID == "" || actorID == "" || at.IsZero() || voyage.Version < 1 {
		return Timeline{}, fmt.Errorf("%w: voyage timeline", domain.ErrInvalid)
	}
	event := VoyageEvent{
		ID: voyage.ID + "-1", VoyageID: voyage.ID, Status: voyage.Status,
		OccurredAt: at.UTC(), ActorID: actorID, Reason: "voyage created", Version: 1,
	}
	return Timeline{VoyageID: voyage.ID, Events: []VoyageEvent{event}, Current: voyage.Status, Version: 1}, nil
}

func AppendTimeline(timeline Timeline, event VoyageEvent, expectedVersion int64) (Timeline, error) {
	if timeline.VoyageID == "" || timeline.Version != expectedVersion || event.VoyageID != timeline.VoyageID || event.ID == "" || event.ActorID == "" || event.OccurredAt.IsZero() {
		return timeline, fmt.Errorf("%w: timeline event", domain.ErrConflict)
	}
	if len(timeline.Events) == 0 {
		return timeline, fmt.Errorf("%w: empty timeline", domain.ErrState)
	}
	last := timeline.Events[len(timeline.Events)-1]
	if event.OccurredAt.Before(last.OccurredAt) || event.Version != timeline.Version+1 {
		return timeline, fmt.Errorf("%w: timeline ordering", domain.ErrConflict)
	}
	probe := Voyage{Status: timeline.Current, Version: timeline.Version}
	if _, err := Transition(probe, event.Status, event.OccurredAt); err != nil {
		return timeline, err
	}
	out := cloneTimeline(timeline)
	out.Events = append(out.Events, event)
	out.Current = event.Status
	out.Version++
	return out, nil
}

func ReconstructTimeline(voyageID string, events []VoyageEvent) (Timeline, error) {
	if voyageID == "" || len(events) == 0 {
		return Timeline{}, fmt.Errorf("%w: timeline events", domain.ErrInvalid)
	}
	ordered := append([]VoyageEvent(nil), events...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Version == ordered[j].Version {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Version < ordered[j].Version
	})
	first := ordered[0]
	if first.VoyageID != voyageID || first.Version != 1 || first.Status != VoyageDraft {
		return Timeline{}, fmt.Errorf("%w: timeline origin", domain.ErrConflict)
	}
	timeline := Timeline{VoyageID: voyageID, Events: []VoyageEvent{first}, Current: first.Status, Version: 1}
	for _, event := range ordered[1:] {
		updated, err := AppendTimeline(timeline, event, timeline.Version)
		if err != nil {
			return Timeline{}, err
		}
		timeline = updated
	}
	return timeline, nil
}

func TimelineDuration(timeline Timeline, from, to VoyageStatus) (time.Duration, error) {
	var started time.Time
	for _, event := range timeline.Events {
		if event.Status == from && started.IsZero() {
			started = event.OccurredAt
			continue
		}
		if !started.IsZero() && event.Status == to {
			return event.OccurredAt.Sub(started), nil
		}
	}
	return 0, fmt.Errorf("%w: timeline milestones", domain.ErrNotFound)
}

func cloneTimeline(timeline Timeline) Timeline {
	out := timeline
	out.Events = append([]VoyageEvent(nil), timeline.Events...)
	return out
}
