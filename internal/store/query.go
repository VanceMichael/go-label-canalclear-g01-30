package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/audit"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/passage"
	"github.com/jackc/pgx/v5"
)

func (db *Database) FindVoyage(ctx context.Context, tenantID, voyageID string) (clearance.Voyage, error) {
	return db.GetVoyage(ctx, tenantID, voyageID)
}

func (db *Database) ListVoyages(ctx context.Context, tenantID string, filter clearance.VoyageFilter, request domain.PageRequest) (domain.Page[clearance.Voyage], error) {
	if tenantID == "" {
		return domain.Page[clearance.Voyage]{}, fmt.Errorf("%w: tenant", domain.ErrInvalid)
	}
	if err := filter.Validate(); err != nil {
		return domain.Page[clearance.Voyage]{}, err
	}
	normalized, err := request.Normalize()
	if err != nil {
		return domain.Page[clearance.Voyage]{}, err
	}
	args := []any{tenantID}
	clauses := []string{"tenant_id=$1"}
	if len(filter.Statuses) > 0 {
		args = append(args, filter.Statuses)
		clauses = append(clauses, fmt.Sprintf("status = ANY($%d)", len(args)))
	}
	if filter.Port != "" {
		args = append(args, strings.ToUpper(strings.TrimSpace(filter.Port)))
		clauses = append(clauses, fmt.Sprintf("(origin_port=$%d OR destination_port=$%d)", len(args), len(args)))
	}
	if filter.ETAFrom != nil {
		args = append(args, filter.ETAFrom.UTC())
		clauses = append(clauses, fmt.Sprintf("eta >= $%d", len(args)))
	}
	if filter.ETATo != nil {
		args = append(args, filter.ETATo.UTC())
		clauses = append(clauses, fmt.Sprintf("eta <= $%d", len(args)))
	}
	if filter.VesselText != "" {
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(filter.VesselText))+"%")
		clauses = append(clauses, fmt.Sprintf("(lower(vessel_name) LIKE $%d OR lower(vessel_imo) LIKE $%d)", len(args), len(args)))
	}
	if normalized.Cursor != "" {
		cursor, err := domain.DecodeCursor(normalized.Cursor)
		if err != nil {
			return domain.Page[clearance.Voyage]{}, err
		}
		args = append(args, cursor.SortTime, cursor.ID)
		clauses = append(clauses, fmt.Sprintf("(eta,id) > ($%d,$%d)", len(args)-1, len(args)))
	}
	args = append(args, normalized.Limit+1)
	query := `SELECT id,tenant_id,vessel_imo,vessel_name,origin_port,destination_port,eta,status,manifest_hash,declared_at,cleared_at,version FROM voyages WHERE ` + strings.Join(clauses, " AND ") + fmt.Sprintf(" ORDER BY eta,id LIMIT $%d", len(args))
	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return domain.Page[clearance.Voyage]{}, err
	}
	defer rows.Close()
	items := make([]clearance.Voyage, 0, normalized.Limit+1)
	for rows.Next() {
		var voyage clearance.Voyage
		if err := rows.Scan(&voyage.ID, &voyage.TenantID, &voyage.VesselIMO, &voyage.VesselName, &voyage.OriginPort, &voyage.DestinationPort, &voyage.ETA, &voyage.Status, &voyage.ManifestHash, &voyage.DeclaredAt, &voyage.ClearedAt, &voyage.Version); err != nil {
			return domain.Page[clearance.Voyage]{}, err
		}
		items = append(items, voyage)
	}
	if err := rows.Err(); err != nil {
		return domain.Page[clearance.Voyage]{}, err
	}
	return domain.BuildPage(items, normalized.Limit, func(voyage clearance.Voyage) domain.Cursor {
		return domain.Cursor{SortTime: voyage.ETA, ID: voyage.ID}
	})
}

func (db *Database) ListInspections(ctx context.Context, tenantID, voyageID string) ([]clearance.Inspection, error) {
	rows, err := db.Pool.Query(ctx, `SELECT i.id,i.voyage_id,i.officer_id,i.kind,i.status,i.finding,i.opened_at,i.closed_at,i.version FROM inspections i JOIN voyages v ON v.id=i.voyage_id WHERE v.tenant_id=$1 AND i.voyage_id=$2 ORDER BY i.opened_at,i.id`, tenantID, voyageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInspections(rows)
}

func scanInspections(rows pgx.Rows) ([]clearance.Inspection, error) {
	result := make([]clearance.Inspection, 0)
	for rows.Next() {
		var inspection clearance.Inspection
		if err := rows.Scan(&inspection.ID, &inspection.VoyageID, &inspection.OfficerID, &inspection.Kind, &inspection.Status, &inspection.Finding, &inspection.OpenedAt, &inspection.ClosedAt, &inspection.Version); err != nil {
			return nil, err
		}
		result = append(result, inspection)
	}
	return result, rows.Err()
}

func (db *Database) ListReservations(ctx context.Context, tenantID, voyageID string) ([]passage.Reservation, error) {
	rows, err := db.Pool.Query(ctx, `SELECT id,voyage_id,tenant_id,chamber_id,starts_at,ends_at,length_m,beam_m,draft_m,status,version FROM passage_reservations WHERE tenant_id=$1 AND voyage_id=$2 ORDER BY starts_at,id`, tenantID, voyageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]passage.Reservation, 0)
	for rows.Next() {
		var reservation passage.Reservation
		if err := rows.Scan(&reservation.ID, &reservation.VoyageID, &reservation.TenantID, &reservation.ChamberID, &reservation.StartsAt, &reservation.EndsAt, &reservation.LengthM, &reservation.BeamM, &reservation.DraftM, &reservation.Status, &reservation.Version); err != nil {
			return nil, err
		}
		result = append(result, reservation)
	}
	return result, rows.Err()
}

func (db *Database) VerifyAuditChain(ctx context.Context, tenantID string) error {
	rows, err := db.Pool.Query(ctx, `SELECT id,tenant_id,actor_id,action,object_type,object_id,outcome,details,occurred_at,sequence,previous_hash,hash FROM audit_events WHERE tenant_id=$1 ORDER BY sequence,id`, tenantID)
	if err != nil {
		return err
	}
	defer rows.Close()
	events := make([]audit.Event, 0)
	for rows.Next() {
		var event audit.Event
		var details []byte
		if err := rows.Scan(&event.ID, &event.TenantID, &event.ActorID, &event.Action, &event.ObjectType, &event.ObjectID, &event.Outcome, &details, &event.OccurredAt, &event.Sequence, &event.PreviousHash, &event.Hash); err != nil {
			return err
		}
		if err := json.Unmarshal(details, &event.Details); err != nil {
			return err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return audit.Verify(events)
}
