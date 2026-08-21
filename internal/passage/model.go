package passage

import (
	"fmt"
	"sort"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type SlotStatus string

const (
	SlotReserved  SlotStatus = "reserved"
	SlotConfirmed SlotStatus = "confirmed"
	SlotEntered   SlotStatus = "entered"
	SlotExited    SlotStatus = "exited"
	SlotCancelled SlotStatus = "cancelled"
)

type Chamber struct {
	ID              string
	Name            string
	MaxLengthM      float64
	MaxBeamM        float64
	MaxDraftM       float64
	MaintenanceFrom *time.Time
	MaintenanceTo   *time.Time
}

type Reservation struct {
	ID        string
	VoyageID  string
	TenantID  string
	ChamberID string
	StartsAt  time.Time
	EndsAt    time.Time
	LengthM   float64
	BeamM     float64
	DraftM    float64
	Status    SlotStatus
	Version   int64
}

func Reserve(id, voyageID, tenant string, chamber Chamber, starts, ends time.Time, length, beam, draft float64, existing []Reservation) (Reservation, error) {
	if id == "" || voyageID == "" || tenant == "" || chamber.ID == "" || !ends.After(starts) || ends.Sub(starts) > 3*time.Hour {
		return Reservation{}, fmt.Errorf("%w: passage reservation", domain.ErrInvalid)
	}
	if length <= 0 || beam <= 0 || draft <= 0 || length > chamber.MaxLengthM || beam > chamber.MaxBeamM || draft > chamber.MaxDraftM {
		return Reservation{}, fmt.Errorf("%w: vessel dimensions", domain.ErrConflict)
	}
	if chamber.MaintenanceFrom != nil && chamber.MaintenanceTo != nil && overlaps(starts, ends, *chamber.MaintenanceFrom, *chamber.MaintenanceTo) {
		return Reservation{}, fmt.Errorf("%w: chamber maintenance", domain.ErrConflict)
	}
	for _, reservation := range existing {
		if reservation.ChamberID == chamber.ID && reservation.Status != SlotCancelled && overlaps(starts, ends, reservation.StartsAt, reservation.EndsAt) {
			return Reservation{}, fmt.Errorf("%w: chamber slot occupied", domain.ErrConflict)
		}
	}
	return Reservation{ID: id, VoyageID: voyageID, TenantID: tenant, ChamberID: chamber.ID, StartsAt: starts.UTC(), EndsAt: ends.UTC(), LengthM: length, BeamM: beam, DraftM: draft, Status: SlotReserved, Version: 1}, nil
}

func Confirm(reservation Reservation, voyageCleared bool, expectedVersion int64) (Reservation, error) {
	if reservation.Status != SlotReserved || reservation.Version != expectedVersion || !voyageCleared {
		return reservation, fmt.Errorf("%w: passage confirmation", domain.ErrState)
	}
	out := reservation
	out.Status = SlotConfirmed
	out.Version++
	return out, nil
}

func Enter(reservation Reservation, at time.Time, expectedVersion int64) (Reservation, error) {
	if reservation.Status != SlotConfirmed || reservation.Version != expectedVersion || at.Before(reservation.StartsAt.Add(-15*time.Minute)) || !at.Before(reservation.EndsAt) {
		return reservation, fmt.Errorf("%w: lock entry", domain.ErrState)
	}
	out := reservation
	out.Status = SlotEntered
	out.Version++
	return out, nil
}

func Exit(reservation Reservation, at time.Time, expectedVersion int64) (Reservation, error) {
	if reservation.Status != SlotEntered || reservation.Version != expectedVersion || at.Before(reservation.StartsAt) {
		return reservation, fmt.Errorf("%w: lock exit", domain.ErrState)
	}
	out := reservation
	out.Status = SlotExited
	out.Version++
	return out, nil
}

func Cancel(reservation Reservation, at time.Time, expectedVersion int64) (Reservation, error) {
	if (reservation.Status != SlotReserved && reservation.Status != SlotConfirmed) || reservation.Version != expectedVersion || !at.Before(reservation.StartsAt) {
		return reservation, fmt.Errorf("%w: passage cancellation", domain.ErrState)
	}
	out := reservation
	out.Status = SlotCancelled
	out.Version++
	return out, nil
}

func NextAvailable(chamber Chamber, existing []Reservation, earliest time.Time, duration time.Duration) (time.Time, error) {
	if chamber.ID == "" || earliest.IsZero() || duration <= 0 || duration > 3*time.Hour {
		return time.Time{}, domain.ErrInvalid
	}
	active := make([]Reservation, 0, len(existing))
	for _, reservation := range existing {
		if reservation.ChamberID == chamber.ID && reservation.Status != SlotCancelled && reservation.EndsAt.After(earliest) {
			active = append(active, reservation)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].StartsAt.Before(active[j].StartsAt) })
	candidate := earliest.UTC()
	for _, reservation := range active {
		if !candidate.Add(duration).After(reservation.StartsAt) {
			break
		}
		if reservation.EndsAt.After(candidate) {
			candidate = reservation.EndsAt
		}
	}
	if chamber.MaintenanceFrom != nil && chamber.MaintenanceTo != nil && overlaps(candidate, candidate.Add(duration), *chamber.MaintenanceFrom, *chamber.MaintenanceTo) {
		candidate = chamber.MaintenanceTo.UTC()
	}
	return candidate, nil
}

func overlaps(leftStart, leftEnd, rightStart, rightEnd time.Time) bool {
	return leftStart.Before(rightEnd) && rightStart.Before(leftEnd)
}
