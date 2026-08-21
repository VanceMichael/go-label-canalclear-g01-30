package passage

import (
	"fmt"
	"sort"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type SchedulePolicy struct {
	OperatingFrom time.Duration
	OperatingTo   time.Duration
	Buffer        time.Duration
	MinimumNotice time.Duration
	MaximumAhead  time.Duration
	Location      *time.Location
}

type SlotCandidate struct {
	ChamberID string
	StartsAt  time.Time
	EndsAt    time.Time
	Wait      time.Duration
}

func DefaultSchedulePolicy(location *time.Location) SchedulePolicy {
	if location == nil {
		location = time.UTC
	}
	return SchedulePolicy{
		OperatingFrom: 6 * time.Hour,
		OperatingTo:   22 * time.Hour,
		Buffer:        15 * time.Minute,
		MinimumNotice: 30 * time.Minute,
		MaximumAhead:  30 * 24 * time.Hour,
		Location:      location,
	}
}

func (policy SchedulePolicy) Validate() error {
	if policy.Location == nil || policy.OperatingFrom < 0 || policy.OperatingTo > 24*time.Hour || policy.OperatingTo <= policy.OperatingFrom {
		return fmt.Errorf("%w: operating hours", domain.ErrInvalid)
	}
	if policy.Buffer < 0 || policy.MinimumNotice < 0 || policy.MaximumAhead <= policy.MinimumNotice {
		return fmt.Errorf("%w: scheduling policy", domain.ErrInvalid)
	}
	return nil
}

func DailyOperatingWindow(day time.Time, policy SchedulePolicy) (domain.TimeWindow, error) {
	if err := policy.Validate(); err != nil {
		return domain.TimeWindow{}, err
	}
	local := day.In(policy.Location)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, policy.Location)
	return domain.NewTimeWindow(midnight.Add(policy.OperatingFrom), midnight.Add(policy.OperatingTo), 24*time.Hour)
}

func AvailableWindows(chamber Chamber, reservations []Reservation, day time.Time, duration time.Duration, policy SchedulePolicy) ([]domain.TimeWindow, error) {
	base, err := DailyOperatingWindow(day, policy)
	if err != nil {
		return nil, err
	}
	if duration <= 0 || duration > 3*time.Hour {
		return nil, fmt.Errorf("%w: passage duration", domain.ErrInvalid)
	}
	blocked := make([]domain.TimeWindow, 0, len(reservations)+1)
	for _, reservation := range reservations {
		if reservation.ChamberID != chamber.ID || reservation.Status == SlotCancelled {
			continue
		}
		window, err := domain.NewTimeWindow(reservation.StartsAt.Add(-policy.Buffer), reservation.EndsAt.Add(policy.Buffer), 0)
		if err != nil {
			return nil, err
		}
		blocked = append(blocked, window)
	}
	if chamber.MaintenanceFrom != nil && chamber.MaintenanceTo != nil {
		window, err := domain.NewTimeWindow(*chamber.MaintenanceFrom, *chamber.MaintenanceTo, 0)
		if err != nil {
			return nil, fmt.Errorf("%w: chamber maintenance", domain.ErrInvalid)
		}
		blocked = append(blocked, window)
	}
	return domain.SubtractWindows(base, blocked, duration)
}

func RankSlotCandidates(chambers []Chamber, reservations []Reservation, earliest time.Time, duration time.Duration, vesselLength, beam, draft float64, policy SchedulePolicy) ([]SlotCandidate, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	now := time.Now().In(policy.Location)
	if earliest.Before(now.Add(policy.MinimumNotice)) || earliest.After(now.Add(policy.MaximumAhead)) {
		return nil, fmt.Errorf("%w: requested planning horizon", domain.ErrInvalid)
	}
	result := make([]SlotCandidate, 0)
	for _, chamber := range chambers {
		if vesselLength > chamber.MaxLengthM || beam > chamber.MaxBeamM || draft > chamber.MaxDraftM {
			continue
		}
		for dayOffset := 0; dayOffset < 31; dayOffset++ {
			day := earliest.In(policy.Location).AddDate(0, 0, dayOffset)
			windows, err := AvailableWindows(chamber, reservations, day, duration, policy)
			if err != nil {
				return nil, err
			}
			for _, window := range windows {
				starts := window.StartsAt
				if starts.Before(earliest) {
					starts = earliest
				}
				if !starts.Add(duration).After(window.EndsAt) {
					result = append(result, SlotCandidate{ChamberID: chamber.ID, StartsAt: starts.UTC(), EndsAt: starts.Add(duration).UTC(), Wait: starts.Sub(earliest)})
					break
				}
			}
			if len(result) > 0 && result[len(result)-1].ChamberID == chamber.ID {
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartsAt.Equal(result[j].StartsAt) {
			return result[i].ChamberID < result[j].ChamberID
		}
		return result[i].StartsAt.Before(result[j].StartsAt)
	})
	return result, nil
}
