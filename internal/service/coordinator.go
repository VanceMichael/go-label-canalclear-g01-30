package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/idempotency"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/passage"
)

type Coordinator struct {
	Voyages    VoyageRepository
	Workflow   WorkflowRepository
	Idempotent IdempotencyRepository
	Clock      Clock
	RecordTTL  time.Duration
	StaleAfter time.Duration
}

func (coordinator Coordinator) validate() error {
	if coordinator.Voyages == nil || coordinator.Workflow == nil || coordinator.Clock == nil {
		return fmt.Errorf("%w: coordinator dependencies", domain.ErrInvalid)
	}
	if coordinator.RecordTTL <= 0 {
		coordinator.RecordTTL = 24 * time.Hour
	}
	return nil
}

func (coordinator Coordinator) Declare(ctx context.Context, actor auth.User, command DeclareVoyageCommand) (clearance.Voyage, error) {
	if err := coordinator.validate(); err != nil {
		return clearance.Voyage{}, err
	}
	if err := auth.Authorize(actor, actor.TenantID, auth.PermissionVoyageDeclare); err != nil {
		return clearance.Voyage{}, err
	}
	if err := ctx.Err(); err != nil {
		return clearance.Voyage{}, err
	}
	if command.ETA.IsZero() || len(command.Items) == 0 {
		return clearance.Voyage{}, fmt.Errorf("%w: declaration command", domain.ErrInvalid)
	}
	if command.IdempotencyKey == "" || coordinator.Idempotent == nil {
		return coordinator.Workflow.DeclareVoyage(ctx, actor, command)
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return clearance.Voyage{}, fmt.Errorf("encode declaration: %w", err)
	}
	now := coordinator.Clock.Now()
	recordTTL := coordinator.RecordTTL
	if recordTTL <= 0 {
		recordTTL = 24 * time.Hour
	}
	record, err := idempotency.NewRecord(actor.TenantID, command.IdempotencyKey, "voyage.declare", idempotency.Fingerprint("POST", "/v1/voyages", payload), now, recordTTL)
	if err != nil {
		return clearance.Voyage{}, err
	}
	decision, stored, err := coordinator.Idempotent.BeginIdempotent(ctx, record, coordinator.StaleAfter)
	if err != nil {
		return clearance.Voyage{}, err
	}
	switch decision {
	case idempotency.DecisionReplay:
		var voyage clearance.Voyage
		if err := json.Unmarshal(stored.ResponseBody, &voyage); err != nil {
			return clearance.Voyage{}, fmt.Errorf("decode idempotent response: %w", err)
		}
		return voyage, nil
	case idempotency.DecisionBusy:
		return clearance.Voyage{}, fmt.Errorf("%w: declaration is processing", domain.ErrConflict)
	case idempotency.DecisionConflict:
		return clearance.Voyage{}, fmt.Errorf("%w: idempotency key reused", domain.ErrConflict)
	case idempotency.DecisionStart:
	default:
		return clearance.Voyage{}, fmt.Errorf("%w: idempotency decision", domain.ErrState)
	}
	voyage, workflowErr := coordinator.Workflow.DeclareVoyage(ctx, actor, command)
	if workflowErr != nil {
		failed, transitionErr := idempotency.Fail(stored, workflowErr.Error(), coordinator.Clock.Now(), stored.Version)
		if transitionErr == nil {
			_ = coordinator.Idempotent.FailIdempotent(ctx, failed)
		}
		return clearance.Voyage{}, workflowErr
	}
	response, err := json.Marshal(voyage)
	if err != nil {
		return clearance.Voyage{}, fmt.Errorf("encode declaration response: %w", err)
	}
	completed, err := idempotency.Complete(stored, 201, response, coordinator.Clock.Now(), stored.Version)
	if err != nil {
		return clearance.Voyage{}, err
	}
	if err := coordinator.Idempotent.CompleteIdempotent(ctx, completed); err != nil {
		return clearance.Voyage{}, err
	}
	return voyage, nil
}

func (coordinator Coordinator) OpenInspection(ctx context.Context, actor auth.User, command OpenInspectionCommand) (clearance.Inspection, clearance.Voyage, error) {
	if err := coordinator.validate(); err != nil {
		return clearance.Inspection{}, clearance.Voyage{}, err
	}
	if err := auth.Authorize(actor, actor.TenantID, auth.PermissionInspectionManage); err != nil {
		return clearance.Inspection{}, clearance.Voyage{}, err
	}
	if command.ID == "" || command.VoyageID == "" || command.Kind == "" {
		return clearance.Inspection{}, clearance.Voyage{}, fmt.Errorf("%w: inspection command", domain.ErrInvalid)
	}
	if command.OpenedAt.IsZero() {
		command.OpenedAt = coordinator.Clock.Now()
	}
	return coordinator.Workflow.OpenInspection(ctx, actor, command)
}

func (coordinator Coordinator) PassInspection(ctx context.Context, actor auth.User, command PassInspectionCommand) (clearance.Voyage, error) {
	if err := coordinator.validate(); err != nil {
		return clearance.Voyage{}, err
	}
	if err := auth.Authorize(actor, actor.TenantID, auth.PermissionInspectionManage); err != nil {
		return clearance.Voyage{}, err
	}
	if command.InspectionID == "" || command.Finding == "" {
		return clearance.Voyage{}, fmt.Errorf("%w: inspection result", domain.ErrInvalid)
	}
	if command.PassedAt.IsZero() {
		command.PassedAt = coordinator.Clock.Now()
	}
	return coordinator.Workflow.PassInspectionAndRelease(ctx, actor, command)
}

func (coordinator Coordinator) Reserve(ctx context.Context, actor auth.User, command ReservePassageCommand) (passage.Reservation, error) {
	if err := coordinator.validate(); err != nil {
		return passage.Reservation{}, err
	}
	if err := auth.Authorize(actor, actor.TenantID, auth.PermissionPassageReserve); err != nil {
		return passage.Reservation{}, err
	}
	if command.ID == "" || command.VoyageID == "" || command.Chamber.ID == "" {
		return passage.Reservation{}, fmt.Errorf("%w: reservation command", domain.ErrInvalid)
	}
	if command.RequestedAt.IsZero() {
		command.RequestedAt = coordinator.Clock.Now()
	}
	return coordinator.Workflow.ReservePassage(ctx, actor, command)
}

func (coordinator Coordinator) Move(ctx context.Context, actor auth.User, command MovePassageCommand) (passage.Reservation, error) {
	if err := coordinator.validate(); err != nil {
		return passage.Reservation{}, err
	}
	if err := auth.Authorize(actor, actor.TenantID, auth.PermissionPassageOperate); err != nil {
		return passage.Reservation{}, err
	}
	if command.ReservationID == "" || command.Next == "" {
		return passage.Reservation{}, fmt.Errorf("%w: passage movement command", domain.ErrInvalid)
	}
	if command.OccurredAt.IsZero() {
		command.OccurredAt = coordinator.Clock.Now()
	}
	return coordinator.Workflow.MovePassage(ctx, actor, command)
}
