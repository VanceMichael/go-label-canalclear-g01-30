package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/passage"
)

type PassageEligibility struct {
	Eligible            bool
	VoyageStatus        clearance.VoyageStatus
	OutstandingChecks   []string
	ActiveReservations  int
	BlockingReservation string
}

func (reporting Reporting) PassageEligibility(ctx context.Context, actor auth.User, voyageID string) (PassageEligibility, error) {
	if reporting.Voyages == nil || voyageID == "" {
		return PassageEligibility{}, fmt.Errorf("%w: passage eligibility", domain.ErrInvalid)
	}
	if err := auth.Authorize(actor, actor.TenantID, auth.PermissionVoyageRead); err != nil {
		return PassageEligibility{}, err
	}
	voyage, err := reporting.Voyages.FindVoyage(ctx, actor.TenantID, voyageID)
	if err != nil {
		return PassageEligibility{}, err
	}
	inspections, err := reporting.Voyages.ListInspections(ctx, actor.TenantID, voyageID)
	if err != nil {
		return PassageEligibility{}, err
	}
	reservations, err := reporting.Voyages.ListReservations(ctx, actor.TenantID, voyageID)
	if err != nil {
		return PassageEligibility{}, err
	}
	result := PassageEligibility{VoyageStatus: voyage.Status}
	if voyage.Status != clearance.VoyageCleared {
		result.OutstandingChecks = append(result.OutstandingChecks, "voyage must be cleared")
	}
	if len(inspections) == 0 {
		result.OutstandingChecks = append(result.OutstandingChecks, "customs inspection is required")
	}
	for _, inspection := range inspections {
		if inspection.Status != "passed" {
			result.OutstandingChecks = append(result.OutstandingChecks, "inspection "+inspection.ID+" is "+inspection.Status)
		}
	}
	for _, reservation := range reservations {
		if reservation.Status == passage.SlotReserved || reservation.Status == passage.SlotConfirmed || reservation.Status == passage.SlotEntered {
			result.ActiveReservations++
			if result.BlockingReservation == "" {
				result.BlockingReservation = reservation.ID
			}
		}
	}
	if result.ActiveReservations > 0 {
		result.OutstandingChecks = append(result.OutstandingChecks, "voyage already has an active passage reservation")
	}
	sort.Strings(result.OutstandingChecks)
	result.Eligible = len(result.OutstandingChecks) == 0
	return result, nil
}
