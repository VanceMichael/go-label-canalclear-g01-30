package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/passage"
)

type voyageRepositoryStub struct {
	voyage       clearance.Voyage
	inspections  []clearance.Inspection
	reservations []passage.Reservation
	err          error
}

func (stub voyageRepositoryStub) FindVoyage(context.Context, string, string) (clearance.Voyage, error) {
	return stub.voyage, stub.err
}

func (stub voyageRepositoryStub) ListVoyages(context.Context, string, clearance.VoyageFilter, domain.PageRequest) (domain.Page[clearance.Voyage], error) {
	return domain.Page[clearance.Voyage]{Items: []clearance.Voyage{stub.voyage}}, stub.err
}

func (stub voyageRepositoryStub) ListInspections(context.Context, string, string) ([]clearance.Inspection, error) {
	return append([]clearance.Inspection(nil), stub.inspections...), stub.err
}

func (stub voyageRepositoryStub) ListReservations(context.Context, string, string) ([]passage.Reservation, error) {
	return append([]passage.Reservation(nil), stub.reservations...), stub.err
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

func TestPassageEligibilityRequiresClearanceAndPassedInspection(t *testing.T) {
	now := time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)
	actor := auth.User{ID: "dispatcher", TenantID: "tenant-a", Role: auth.RoleDispatcher}
	repository := voyageRepositoryStub{
		voyage:      clearance.Voyage{ID: "voyage", TenantID: "tenant-a", Status: clearance.VoyageCleared},
		inspections: []clearance.Inspection{{ID: "inspection", Status: "passed"}},
	}
	reporting := Reporting{Voyages: repository, Clock: fixedClock{at: now}}
	eligibility, err := reporting.PassageEligibility(context.Background(), actor, "voyage")
	if err != nil || !eligibility.Eligible || len(eligibility.OutstandingChecks) != 0 {
		t.Fatalf("eligibility=%+v err=%v", eligibility, err)
	}
	repository.voyage.Status = clearance.VoyageHeld
	repository.inspections[0].Status = "open"
	reporting.Voyages = repository
	eligibility, err = reporting.PassageEligibility(context.Background(), actor, "voyage")
	if err != nil || eligibility.Eligible || len(eligibility.OutstandingChecks) != 2 {
		t.Fatalf("eligibility=%+v err=%v", eligibility, err)
	}
}

func TestPassageEligibilityRejectsExistingActiveReservation(t *testing.T) {
	actor := auth.User{ID: "dispatcher", TenantID: "tenant-a", Role: auth.RoleDispatcher}
	repository := voyageRepositoryStub{
		voyage:      clearance.Voyage{ID: "voyage", TenantID: "tenant-a", Status: clearance.VoyageCleared},
		inspections: []clearance.Inspection{{ID: "inspection", Status: "passed"}},
		reservations: []passage.Reservation{
			{ID: "cancelled", Status: passage.SlotCancelled},
			{ID: "active", Status: passage.SlotConfirmed},
		},
	}
	eligibility, err := (Reporting{Voyages: repository, Clock: fixedClock{at: time.Now()}}).PassageEligibility(context.Background(), actor, "voyage")
	if err != nil || eligibility.Eligible || eligibility.ActiveReservations != 1 || eligibility.BlockingReservation != "active" {
		t.Fatalf("eligibility=%+v err=%v", eligibility, err)
	}
}

func TestOverviewIsolatesRepositorySlices(t *testing.T) {
	actor := auth.User{ID: "carrier", TenantID: "tenant-a", Role: auth.RoleCarrier}
	repository := voyageRepositoryStub{
		voyage:       clearance.Voyage{ID: "voyage", TenantID: "tenant-a", Status: clearance.VoyageCleared},
		inspections:  []clearance.Inspection{{ID: "inspection", Kind: string(clearance.InspectionDocument), Status: "passed"}},
		reservations: []passage.Reservation{{ID: "reservation", Status: passage.SlotConfirmed}},
	}
	overview, err := (Reporting{Voyages: repository, Clock: fixedClock{at: time.Now()}}).Overview(context.Background(), actor, "voyage")
	if err != nil || !overview.ReadyForPassage || len(overview.OutstandingChecks) != 0 {
		t.Fatalf("overview=%+v err=%v", overview, err)
	}
	overview.Inspections[0].ID = "changed"
	overview.Reservations[0].ID = "changed"
	if repository.inspections[0].ID != "inspection" || repository.reservations[0].ID != "reservation" {
		t.Fatal("overview mutated repository state")
	}
}

func TestReportingPreservesRepositoryErrorsAndAuthorization(t *testing.T) {
	repositoryErr := errors.New("repository unavailable")
	actor := auth.User{ID: "carrier", TenantID: "tenant-a", Role: auth.RoleCarrier}
	reporting := Reporting{Voyages: voyageRepositoryStub{err: repositoryErr}, Clock: fixedClock{at: time.Now()}}
	if _, err := reporting.Overview(context.Background(), actor, "voyage"); !errors.Is(err, repositoryErr) {
		t.Fatalf("repository error=%v", err)
	}
	actor.Disabled = true
	if _, err := reporting.PassageEligibility(context.Background(), actor, "voyage"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("authorization error=%v", err)
	}
}
