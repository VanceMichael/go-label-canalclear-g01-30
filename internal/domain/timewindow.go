package domain

import (
	"fmt"
	"sort"
	"time"
)

type TimeWindow struct {
	StartsAt time.Time
	EndsAt   time.Time
}

func NewTimeWindow(startsAt, endsAt time.Time, maximum time.Duration) (TimeWindow, error) {
	startsAt = startsAt.UTC()
	endsAt = endsAt.UTC()
	if startsAt.IsZero() || !endsAt.After(startsAt) || (maximum > 0 && endsAt.Sub(startsAt) > maximum) {
		return TimeWindow{}, fmt.Errorf("%w: time window", ErrInvalid)
	}
	return TimeWindow{StartsAt: startsAt, EndsAt: endsAt}, nil
}

func (window TimeWindow) Duration() time.Duration {
	return window.EndsAt.Sub(window.StartsAt)
}

func (window TimeWindow) Contains(at time.Time) bool {
	at = at.UTC()
	return !at.Before(window.StartsAt) && at.Before(window.EndsAt)
}

func (window TimeWindow) Overlaps(other TimeWindow) bool {
	return window.StartsAt.Before(other.EndsAt) && other.StartsAt.Before(window.EndsAt)
}

func MergeWindows(windows []TimeWindow) ([]TimeWindow, error) {
	if len(windows) == 0 {
		return []TimeWindow{}, nil
	}
	copyWindows := append([]TimeWindow(nil), windows...)
	for _, window := range copyWindows {
		if _, err := NewTimeWindow(window.StartsAt, window.EndsAt, 0); err != nil {
			return nil, err
		}
	}
	sort.Slice(copyWindows, func(i, j int) bool {
		if copyWindows[i].StartsAt.Equal(copyWindows[j].StartsAt) {
			return copyWindows[i].EndsAt.Before(copyWindows[j].EndsAt)
		}
		return copyWindows[i].StartsAt.Before(copyWindows[j].StartsAt)
	})
	merged := []TimeWindow{copyWindows[0]}
	for _, candidate := range copyWindows[1:] {
		last := &merged[len(merged)-1]
		if candidate.StartsAt.After(last.EndsAt) {
			merged = append(merged, candidate)
			continue
		}
		if candidate.EndsAt.After(last.EndsAt) {
			last.EndsAt = candidate.EndsAt
		}
	}
	return merged, nil
}

func SubtractWindows(base TimeWindow, blocked []TimeWindow, minimum time.Duration) ([]TimeWindow, error) {
	if _, err := NewTimeWindow(base.StartsAt, base.EndsAt, 0); err != nil || minimum <= 0 {
		return nil, fmt.Errorf("%w: availability window", ErrInvalid)
	}
	merged, err := MergeWindows(blocked)
	if err != nil {
		return nil, err
	}
	cursor := base.StartsAt
	available := make([]TimeWindow, 0, len(merged)+1)
	for _, current := range merged {
		if !current.Overlaps(base) {
			continue
		}
		start := current.StartsAt
		if start.Before(base.StartsAt) {
			start = base.StartsAt
		}
		if start.Sub(cursor) >= minimum {
			available = append(available, TimeWindow{StartsAt: cursor, EndsAt: start})
		}
		if current.EndsAt.After(cursor) {
			cursor = current.EndsAt
		}
		if cursor.After(base.EndsAt) {
			cursor = base.EndsAt
		}
	}
	if base.EndsAt.Sub(cursor) >= minimum {
		available = append(available, TimeWindow{StartsAt: cursor, EndsAt: base.EndsAt})
	}
	return available, nil
}
