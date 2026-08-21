package store

import (
	"context"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/passage"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/service"
)

type WorkflowAdapter struct {
	DB *Database
}

func (adapter WorkflowAdapter) DeclareVoyage(ctx context.Context, actor auth.User, command service.DeclareVoyageCommand) (clearance.Voyage, error) {
	return adapter.DB.DeclareVoyage(ctx, actor, DeclareRequest{
		ID: command.ID, VesselIMO: command.VesselIMO, VesselName: command.VesselName,
		OriginPort: command.OriginPort, DestinationPort: command.DestinationPort,
		ETA: command.ETA, Items: command.Items, At: command.ETA.Add(-12 * time.Hour),
	})
}

func (adapter WorkflowAdapter) OpenInspection(ctx context.Context, actor auth.User, command service.OpenInspectionCommand) (clearance.Inspection, clearance.Voyage, error) {
	return adapter.DB.OpenInspection(ctx, actor, command.ID, command.VoyageID, string(command.Kind), command.OpenedAt)
}

func (adapter WorkflowAdapter) PassInspectionAndRelease(ctx context.Context, actor auth.User, command service.PassInspectionCommand) (clearance.Voyage, error) {
	return adapter.DB.PassInspectionAndRelease(ctx, actor, command.InspectionID, command.Finding, command.PassedAt)
}

func (adapter WorkflowAdapter) ReservePassage(ctx context.Context, actor auth.User, command service.ReservePassageCommand) (passage.Reservation, error) {
	return adapter.DB.ReservePassage(ctx, actor, command.ID, command.VoyageID, command.Chamber, command.StartsAt, command.EndsAt, command.LengthM, command.BeamM, command.DraftM, command.RequestedAt)
}

func (adapter WorkflowAdapter) MovePassage(ctx context.Context, actor auth.User, command service.MovePassageCommand) (passage.Reservation, error) {
	return adapter.DB.MovePassage(ctx, actor, command)
}

var _ service.WorkflowRepository = WorkflowAdapter{}
var _ service.VoyageRepository = (*Database)(nil)
var _ service.AuditRepository = (*Database)(nil)
var _ service.IdempotencyRepository = (*Database)(nil)
