package domain

import (
	"errors"
	"testing"
	"time"
)

func TestTimeWindowBoundariesAreHalfOpen(t *testing.T) {
	start := time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)
	window, err := NewTimeWindow(start, start.Add(time.Hour), 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !window.Contains(start) || !window.Contains(start.Add(59*time.Minute)) || window.Contains(start.Add(time.Hour)) {
		t.Fatalf("half-open boundary violated: %+v", window)
	}
	adjacent, _ := NewTimeWindow(start.Add(time.Hour), start.Add(2*time.Hour), 0)
	if window.Overlaps(adjacent) {
		t.Fatal("adjacent windows must not overlap")
	}
}

func TestNewTimeWindowRejectsInvalidRanges(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		start time.Time
		end   time.Time
		max   time.Duration
	}{
		{start: time.Time{}, end: now, max: 0},
		{start: now, end: now, max: 0},
		{start: now, end: now.Add(-time.Second), max: 0},
		{start: now, end: now.Add(2 * time.Hour), max: time.Hour},
	} {
		if _, err := NewTimeWindow(test.start, test.end, test.max); !errors.Is(err, ErrInvalid) {
			t.Errorf("range=%+v error=%v", test, err)
		}
	}
}

func TestMergeWindowsJoinsOverlapAndAdjacency(t *testing.T) {
	base := time.Date(2026, 8, 21, 6, 0, 0, 0, time.UTC)
	windows := []TimeWindow{
		{StartsAt: base.Add(3 * time.Hour), EndsAt: base.Add(4 * time.Hour)},
		{StartsAt: base, EndsAt: base.Add(time.Hour)},
		{StartsAt: base.Add(30 * time.Minute), EndsAt: base.Add(2 * time.Hour)},
		{StartsAt: base.Add(2 * time.Hour), EndsAt: base.Add(3 * time.Hour)},
	}
	merged, err := MergeWindows(windows)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 || !merged[0].StartsAt.Equal(base) || !merged[0].EndsAt.Equal(base.Add(4*time.Hour)) {
		t.Fatalf("merged=%+v", merged)
	}
	if !windows[0].StartsAt.Equal(base.Add(3 * time.Hour)) {
		t.Fatal("input order was mutated")
	}
}

func TestSubtractWindowsReturnsMinimumSizedAvailability(t *testing.T) {
	baseTime := time.Date(2026, 8, 21, 6, 0, 0, 0, time.UTC)
	base := TimeWindow{StartsAt: baseTime, EndsAt: baseTime.Add(8 * time.Hour)}
	blocked := []TimeWindow{
		{StartsAt: baseTime.Add(-time.Hour), EndsAt: baseTime.Add(time.Hour)},
		{StartsAt: baseTime.Add(2 * time.Hour), EndsAt: baseTime.Add(3 * time.Hour)},
		{StartsAt: baseTime.Add(5 * time.Hour), EndsAt: baseTime.Add(7*time.Hour + 45*time.Minute)},
	}
	available, err := SubtractWindows(base, blocked, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 2 {
		t.Fatalf("available=%+v", available)
	}
	if available[0].Duration() != time.Hour || available[1].Duration() != 2*time.Hour {
		t.Fatalf("durations=%v,%v", available[0].Duration(), available[1].Duration())
	}
}
