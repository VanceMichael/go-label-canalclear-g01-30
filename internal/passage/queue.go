package passage

import (
	"fmt"
	"sort"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type Priority string

const (
	PriorityEmergency  Priority = "emergency"
	PriorityPerishable Priority = "perishable"
	PriorityScheduled  Priority = "scheduled"
	PriorityStandby    Priority = "standby"
)

type QueueEntry struct {
	Reservation Reservation
	Priority    Priority
	RequestedAt time.Time
	Deadline    *time.Time
	Reason      string
}

func ValidateQueueEntry(entry QueueEntry) error {
	if entry.Reservation.ID == "" || entry.RequestedAt.IsZero() || entry.Reservation.Status != SlotConfirmed {
		return fmt.Errorf("%w: passage queue entry", domain.ErrInvalid)
	}
	switch entry.Priority {
	case PriorityEmergency:
		if entry.Reason == "" {
			return fmt.Errorf("%w: emergency reason", domain.ErrInvalid)
		}
	case PriorityPerishable:
		if entry.Deadline == nil || !entry.Deadline.After(entry.RequestedAt) {
			return fmt.Errorf("%w: perishable deadline", domain.ErrInvalid)
		}
	case PriorityScheduled, PriorityStandby:
	default:
		return fmt.Errorf("%w: queue priority", domain.ErrInvalid)
	}
	return nil
}

func OrderQueue(entries []QueueEntry, now time.Time) ([]QueueEntry, error) {
	if now.IsZero() {
		return nil, fmt.Errorf("%w: queue time", domain.ErrInvalid)
	}
	result := append([]QueueEntry(nil), entries...)
	for _, entry := range result {
		if err := ValidateQueueEntry(entry); err != nil {
			return nil, err
		}
	}
	rank := map[Priority]int{
		PriorityEmergency:  0,
		PriorityPerishable: 1,
		PriorityScheduled:  2,
		PriorityStandby:    3,
	}
	sort.SliceStable(result, func(i, j int) bool {
		leftRank := rank[result[i].Priority]
		rightRank := rank[result[j].Priority]
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if result[i].Priority == PriorityPerishable && result[i].Deadline != nil && result[j].Deadline != nil && !result[i].Deadline.Equal(*result[j].Deadline) {
			return result[i].Deadline.Before(*result[j].Deadline)
		}
		if !result[i].Reservation.StartsAt.Equal(result[j].Reservation.StartsAt) {
			return result[i].Reservation.StartsAt.Before(result[j].Reservation.StartsAt)
		}
		if !result[i].RequestedAt.Equal(result[j].RequestedAt) {
			return result[i].RequestedAt.Before(result[j].RequestedAt)
		}
		return result[i].Reservation.ID < result[j].Reservation.ID
	})
	return result, nil
}

func PromoteStandby(entries []QueueEntry, cancelledReservationID string, now time.Time) ([]QueueEntry, QueueEntry, error) {
	if cancelledReservationID == "" || now.IsZero() {
		return nil, QueueEntry{}, fmt.Errorf("%w: standby promotion", domain.ErrInvalid)
	}
	remaining := make([]QueueEntry, 0, len(entries))
	foundCancelled := false
	for _, entry := range entries {
		if entry.Reservation.ID == cancelledReservationID {
			foundCancelled = true
			continue
		}
		remaining = append(remaining, entry)
	}
	if !foundCancelled {
		return nil, QueueEntry{}, fmt.Errorf("%w: cancelled reservation", domain.ErrNotFound)
	}
	ordered, err := OrderQueue(remaining, now)
	if err != nil {
		return nil, QueueEntry{}, err
	}
	for index, entry := range ordered {
		if entry.Priority != PriorityStandby || entry.Reservation.StartsAt.Before(now.UTC()) {
			continue
		}
		promoted := entry
		promoted.Priority = PriorityScheduled
		ordered[index] = promoted
		return ordered, promoted, nil
	}
	return ordered, QueueEntry{}, fmt.Errorf("%w: eligible standby", domain.ErrNotFound)
}
