package passage

import (
	"fmt"
	"sort"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type Movement struct {
	ReservationID string
	Status        SlotStatus
	OccurredAt    time.Time
	OperatorID    string
	Note          string
}

func BuildMovement(reservation Reservation, next SlotStatus, at time.Time, operatorID, note string) (Movement, Reservation, error) {
	if operatorID == "" || at.IsZero() {
		return Movement{}, reservation, fmt.Errorf("%w: passage movement", domain.ErrInvalid)
	}
	updated := reservation
	var err error
	switch next {
	case SlotEntered:
		updated, err = Enter(reservation, at, reservation.Version)
	case SlotExited:
		updated, err = Exit(reservation, at, reservation.Version)
	case SlotCancelled:
		updated, err = Cancel(reservation, at, reservation.Version)
	default:
		err = fmt.Errorf("%w: movement status", domain.ErrInvalid)
	}
	if err != nil {
		return Movement{}, reservation, err
	}
	movement := Movement{ReservationID: reservation.ID, Status: next, OccurredAt: at.UTC(), OperatorID: operatorID, Note: note}
	return movement, updated, nil
}

func ValidateMovementHistory(reservation Reservation, movements []Movement) error {
	if reservation.ID == "" {
		return fmt.Errorf("%w: reservation", domain.ErrInvalid)
	}
	ordered := append([]Movement(nil), movements...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].OccurredAt.Equal(ordered[j].OccurredAt) {
			return ordered[i].Status < ordered[j].Status
		}
		return ordered[i].OccurredAt.Before(ordered[j].OccurredAt)
	})
	status := SlotConfirmed
	lastTime := reservation.StartsAt.Add(-15 * time.Minute)
	for _, movement := range ordered {
		if movement.ReservationID != reservation.ID || movement.OperatorID == "" || movement.OccurredAt.Before(lastTime) {
			return fmt.Errorf("%w: movement history", domain.ErrConflict)
		}
		switch status {
		case SlotConfirmed:
			if movement.Status != SlotEntered && movement.Status != SlotCancelled {
				return fmt.Errorf("%w: expected entry or cancellation", domain.ErrState)
			}
		case SlotEntered:
			if movement.Status != SlotExited {
				return fmt.Errorf("%w: expected exit", domain.ErrState)
			}
		default:
			return fmt.Errorf("%w: terminal reservation has movement", domain.ErrState)
		}
		status = movement.Status
		lastTime = movement.OccurredAt
	}
	if status != reservation.Status {
		return fmt.Errorf("%w: history status differs from reservation", domain.ErrConflict)
	}
	return nil
}

func ChamberUtilization(chamberID string, reservations []Reservation, window domain.TimeWindow) (float64, error) {
	if chamberID == "" || window.Duration() <= 0 {
		return 0, fmt.Errorf("%w: utilization input", domain.ErrInvalid)
	}
	active := make([]domain.TimeWindow, 0)
	for _, reservation := range reservations {
		if reservation.ChamberID != chamberID || reservation.Status == SlotCancelled {
			continue
		}
		current, err := domain.NewTimeWindow(reservation.StartsAt, reservation.EndsAt, 0)
		if err != nil {
			return 0, err
		}
		if current.Overlaps(window) {
			if current.StartsAt.Before(window.StartsAt) {
				current.StartsAt = window.StartsAt
			}
			if current.EndsAt.After(window.EndsAt) {
				current.EndsAt = window.EndsAt
			}
			active = append(active, current)
		}
	}
	merged, err := domain.MergeWindows(active)
	if err != nil {
		return 0, err
	}
	occupied := time.Duration(0)
	for _, current := range merged {
		occupied += current.Duration()
	}
	return float64(occupied) / float64(window.Duration()), nil
}
