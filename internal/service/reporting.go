package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/audit"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/passage"
)

type Reporting struct {
	Voyages VoyageRepository
	Audit   AuditRepository
	Clock   Clock
}

type VoyageOverview struct {
	Voyage            clearance.Voyage
	Inspections       []clearance.Inspection
	Reservations      []passage.Reservation
	OutstandingChecks []clearance.InspectionKind
	ReadyForPassage   bool
}

type OperationsSummary struct {
	GeneratedAt        time.Time
	VoyageCounts       map[clearance.VoyageStatus]int
	DelayedVoyages     []clearance.Voyage
	UpcomingPassages   []passage.Reservation
	AuditChainVerified bool
}

func (reporting Reporting) Overview(ctx context.Context, actor auth.User, voyageID string) (VoyageOverview, error) {
	if reporting.Voyages == nil || reporting.Clock == nil || voyageID == "" {
		return VoyageOverview{}, fmt.Errorf("%w: overview input", domain.ErrInvalid)
	}
	if err := auth.Authorize(actor, actor.TenantID, auth.PermissionVoyageRead); err != nil {
		return VoyageOverview{}, err
	}
	voyage, err := reporting.Voyages.FindVoyage(ctx, actor.TenantID, voyageID)
	if err != nil {
		return VoyageOverview{}, err
	}
	inspections, err := reporting.Voyages.ListInspections(ctx, actor.TenantID, voyageID)
	if err != nil {
		return VoyageOverview{}, err
	}
	reservations, err := reporting.Voyages.ListReservations(ctx, actor.TenantID, voyageID)
	if err != nil {
		return VoyageOverview{}, err
	}
	result := VoyageOverview{
		Voyage:       voyage,
		Inspections:  append([]clearance.Inspection(nil), inspections...),
		Reservations: append([]passage.Reservation(nil), reservations...),
	}
	required := []clearance.InspectionKind{clearance.InspectionDocument}
	passed := make(map[clearance.InspectionKind]bool)
	for _, inspection := range inspections {
		if inspection.Status == "passed" {
			passed[clearance.InspectionKind(inspection.Kind)] = true
		}
	}
	for _, requirement := range required {
		if !passed[requirement] {
			result.OutstandingChecks = append(result.OutstandingChecks, requirement)
		}
	}
	result.ReadyForPassage = voyage.Status == clearance.VoyageCleared && len(result.OutstandingChecks) == 0
	return result, nil
}

func (reporting Reporting) Summary(ctx context.Context, actor auth.User, from, to time.Time) (OperationsSummary, error) {
	if reporting.Voyages == nil || reporting.Audit == nil || reporting.Clock == nil || to.Before(from) {
		return OperationsSummary{}, fmt.Errorf("%w: summary input", domain.ErrInvalid)
	}
	if err := auth.Authorize(actor, actor.TenantID, auth.PermissionAuditRead); err != nil {
		return OperationsSummary{}, err
	}
	page, err := reporting.Voyages.ListVoyages(ctx, actor.TenantID, clearance.VoyageFilter{ETAFrom: &from, ETATo: &to}, domain.PageRequest{Limit: domain.MaximumPageSize})
	if err != nil {
		return OperationsSummary{}, err
	}
	result := OperationsSummary{
		GeneratedAt:    reporting.Clock.Now(),
		VoyageCounts:   clearance.GroupVoyagesByStatus(page.Items),
		DelayedVoyages: clearance.DelayedVoyages(page.Items, reporting.Clock.Now(), 30*time.Minute),
	}
	for _, voyage := range page.Items {
		reservations, err := reporting.Voyages.ListReservations(ctx, actor.TenantID, voyage.ID)
		if err != nil {
			return OperationsSummary{}, err
		}
		for _, reservation := range reservations {
			if reservation.Status == passage.SlotConfirmed && reservation.StartsAt.After(reporting.Clock.Now()) {
				result.UpcomingPassages = append(result.UpcomingPassages, reservation)
			}
		}
	}
	sort.Slice(result.UpcomingPassages, func(i, j int) bool {
		if result.UpcomingPassages[i].StartsAt.Equal(result.UpcomingPassages[j].StartsAt) {
			return result.UpcomingPassages[i].ID < result.UpcomingPassages[j].ID
		}
		return result.UpcomingPassages[i].StartsAt.Before(result.UpcomingPassages[j].StartsAt)
	})
	result.AuditChainVerified = reporting.Audit.VerifyAuditChain(ctx, actor.TenantID) == nil
	return result, nil
}

func (reporting Reporting) AuditTrail(ctx context.Context, actor auth.User, filter audit.Filter, page domain.PageRequest) (domain.Page[audit.Event], error) {
	if reporting.Audit == nil {
		return domain.Page[audit.Event]{}, fmt.Errorf("%w: audit repository", domain.ErrInvalid)
	}
	if err := auth.Authorize(actor, actor.TenantID, auth.PermissionAuditRead); err != nil {
		return domain.Page[audit.Event]{}, err
	}
	if err := filter.Validate(); err != nil {
		return domain.Page[audit.Event]{}, err
	}
	return reporting.Audit.ListAuditEvents(ctx, actor.TenantID, filter, page)
}
