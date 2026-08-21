package passage

import (
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func TestReservationHonorsCapacityConflictsAndClearance(t *testing.T) {
	now := time.Now().UTC()
	chamber := Chamber{ID: "lock-1", Name: "Qishi East", MaxLengthM: 220, MaxBeamM: 34, MaxDraftM: 8}
	reservation, err := Reserve("slot-1", "voyage-1", "tenant-1", chamber, now.Add(time.Hour), now.Add(2*time.Hour), 180, 28, 6, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Reserve("slot-2", "voyage-2", "tenant-1", chamber, now.Add(90*time.Minute), now.Add(150*time.Minute), 120, 20, 4, []Reservation{reservation}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("overlap err=%v", err)
	}
	if _, err := Confirm(reservation, false, 1); !errors.Is(err, domain.ErrState) {
		t.Fatalf("uncleared confirm err=%v", err)
	}
	confirmed, err := Confirm(reservation, true, 1)
	if err != nil || confirmed.Status != SlotConfirmed {
		t.Fatalf("confirmed=%+v err=%v", confirmed, err)
	}
}

func TestNextAvailableSkipsBusyAndMaintenance(t *testing.T) {
	now := time.Now().UTC()
	maintenanceFrom := now.Add(2 * time.Hour)
	maintenanceTo := now.Add(3 * time.Hour)
	chamber := Chamber{ID: "lock-1", MaxLengthM: 200, MaxBeamM: 30, MaxDraftM: 7, MaintenanceFrom: &maintenanceFrom, MaintenanceTo: &maintenanceTo}
	busy := []Reservation{{ChamberID: chamber.ID, StartsAt: now, EndsAt: now.Add(time.Hour), Status: SlotConfirmed}}
	next, err := NextAvailable(chamber, busy, now.Add(30*time.Minute), 90*time.Minute)
	if err != nil || !next.Equal(maintenanceTo) {
		t.Fatalf("next=%v err=%v", next, err)
	}
}
