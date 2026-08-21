package service

import (
	"context"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/audit"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/idempotency"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/passage"
)

type VoyageRepository interface {
	FindVoyage(context.Context, string, string) (clearance.Voyage, error)
	ListVoyages(context.Context, string, clearance.VoyageFilter, domain.PageRequest) (domain.Page[clearance.Voyage], error)
	ListInspections(context.Context, string, string) ([]clearance.Inspection, error)
	ListReservations(context.Context, string, string) ([]passage.Reservation, error)
}

type WorkflowRepository interface {
	DeclareVoyage(context.Context, auth.User, DeclareVoyageCommand) (clearance.Voyage, error)
	OpenInspection(context.Context, auth.User, OpenInspectionCommand) (clearance.Inspection, clearance.Voyage, error)
	PassInspectionAndRelease(context.Context, auth.User, PassInspectionCommand) (clearance.Voyage, error)
	ReservePassage(context.Context, auth.User, ReservePassageCommand) (passage.Reservation, error)
	MovePassage(context.Context, auth.User, MovePassageCommand) (passage.Reservation, error)
}

type AuditRepository interface {
	ListAuditEvents(context.Context, string, audit.Filter, domain.PageRequest) (domain.Page[audit.Event], error)
	VerifyAuditChain(context.Context, string) error
}

type IdempotencyRepository interface {
	BeginIdempotent(context.Context, idempotency.Record, time.Duration) (idempotency.Decision, idempotency.Record, error)
	CompleteIdempotent(context.Context, idempotency.Record) error
	FailIdempotent(context.Context, idempotency.Record) error
}

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type DeclareVoyageCommand struct {
	ID              string
	VesselIMO       string
	VesselName      string
	OriginPort      string
	DestinationPort string
	ETA             time.Time
	Items           []clearance.CargoItem
	IdempotencyKey  string
}

type OpenInspectionCommand struct {
	ID       string
	VoyageID string
	Kind     clearance.InspectionKind
	OpenedAt time.Time
}

type PassInspectionCommand struct {
	InspectionID string
	Finding      string
	PassedAt     time.Time
}

type ReservePassageCommand struct {
	ID          string
	VoyageID    string
	Chamber     passage.Chamber
	StartsAt    time.Time
	EndsAt      time.Time
	LengthM     float64
	BeamM       float64
	DraftM      float64
	RequestedAt time.Time
}

type MovePassageCommand struct {
	ReservationID string
	Next          passage.SlotStatus
	OccurredAt    time.Time
	Note          string
}
