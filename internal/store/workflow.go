package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/audit"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/passage"
	"github.com/jackc/pgx/v5"
)

type DeclareRequest struct {
	ID              string
	VesselIMO       string
	VesselName      string
	OriginPort      string
	DestinationPort string
	ETA             time.Time
	Items           []clearance.CargoItem
	At              time.Time
}

func (db *Database) DeclareVoyage(ctx context.Context, actor auth.User, request DeclareRequest) (clearance.Voyage, error) {
	if actor.Role != auth.RoleCarrier && actor.Role != auth.RoleDispatcher {
		return clearance.Voyage{}, domain.ErrForbidden
	}
	voyage, err := clearance.NewVoyage(request.ID, actor.TenantID, request.VesselIMO, request.VesselName, request.OriginPort, request.DestinationPort, request.ETA)
	if err != nil {
		return clearance.Voyage{}, err
	}
	manifest, err := clearance.BuildManifest(voyage.ID, request.Items)
	if err != nil {
		return clearance.Voyage{}, err
	}
	voyage, err = clearance.Declare(voyage, manifest, request.At, voyage.Version)
	if err != nil {
		return clearance.Voyage{}, err
	}
	itemsJSON, _ := json.Marshal(manifest.Items)
	err = db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO voyages(id,tenant_id,vessel_imo,vessel_name,origin_port,destination_port,eta,status,manifest_hash,declared_at,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, voyage.ID, voyage.TenantID, voyage.VesselIMO, voyage.VesselName, voyage.OriginPort, voyage.DestinationPort, voyage.ETA, voyage.Status, voyage.ManifestHash, voyage.DeclaredAt, voyage.Version); err != nil {
			return mapConstraint(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO manifests(voyage_id,hash,items) VALUES($1,$2,$3)`, manifest.VoyageID, manifest.Hash, itemsJSON); err != nil {
			return err
		}
		if err := appendAuditTx(ctx, tx, audit.Event{ID: "audit-declare-" + voyage.ID, TenantID: voyage.TenantID, ActorID: actor.ID, Action: "voyage.declared", ObjectType: "voyage", ObjectID: voyage.ID, Outcome: "ok", Details: map[string]string{"manifest_hash": manifest.Hash}, OccurredAt: request.At}); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"voyage_id": voyage.ID, "status": string(voyage.Status)})
		_, err := tx.Exec(ctx, `INSERT INTO outbox_jobs(id,tenant_id,topic,payload,status,available_at,created_at,updated_at) VALUES($1,$2,'voyage.declared',$3,'pending',$4,$4,$4)`, "outbox-declare-"+voyage.ID, voyage.TenantID, payload, request.At.UTC())
		return err
	})
	return voyage, err
}

func (db *Database) GetVoyage(ctx context.Context, tenant, id string) (clearance.Voyage, error) {
	var voyage clearance.Voyage
	err := db.Pool.QueryRow(ctx, `SELECT id,tenant_id,vessel_imo,vessel_name,origin_port,destination_port,eta,status,manifest_hash,declared_at,cleared_at,version FROM voyages WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&voyage.ID, &voyage.TenantID, &voyage.VesselIMO, &voyage.VesselName, &voyage.OriginPort, &voyage.DestinationPort, &voyage.ETA, &voyage.Status, &voyage.ManifestHash, &voyage.DeclaredAt, &voyage.ClearedAt, &voyage.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return clearance.Voyage{}, domain.ErrNotFound
	}
	return voyage, err
}

func (db *Database) OpenInspection(ctx context.Context, actor auth.User, id, voyageID, kind string, at time.Time) (clearance.Inspection, clearance.Voyage, error) {
	if actor.Role != auth.RoleCustoms {
		return clearance.Inspection{}, clearance.Voyage{}, domain.ErrForbidden
	}
	var inspection clearance.Inspection
	var updated clearance.Voyage
	err := db.WithTx(ctx, func(tx pgx.Tx) error {
		voyage, err := getVoyageTx(ctx, tx, actor.TenantID, voyageID)
		if err != nil {
			return err
		}
		inspection, updated, err = clearance.OpenInspection(id, voyage, actor.ID, kind, at)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO inspections(id,voyage_id,officer_id,kind,status,opened_at,version) VALUES($1,$2,$3,$4,$5,$6,$7)`, inspection.ID, inspection.VoyageID, inspection.OfficerID, inspection.Kind, inspection.Status, inspection.OpenedAt, inspection.Version); err != nil {
			return mapConstraint(err)
		}
		if err := saveVoyageTx(ctx, tx, updated, voyage.Version); err != nil {
			return err
		}
		return appendAuditTx(ctx, tx, audit.Event{ID: "audit-inspection-open-" + id, TenantID: actor.TenantID, ActorID: actor.ID, Action: "inspection.opened", ObjectType: "voyage", ObjectID: voyageID, Outcome: "ok", Details: map[string]string{"kind": kind}, OccurredAt: at})
	})
	return inspection, updated, err
}

func (db *Database) PassInspectionAndRelease(ctx context.Context, actor auth.User, inspectionID, finding string, at time.Time) (clearance.Voyage, error) {
	if actor.Role != auth.RoleCustoms {
		return clearance.Voyage{}, domain.ErrForbidden
	}
	var released clearance.Voyage
	err := db.WithTx(ctx, func(tx pgx.Tx) error {
		inspection, err := getInspectionTx(ctx, tx, inspectionID)
		if err != nil {
			return err
		}
		voyage, err := getVoyageTx(ctx, tx, actor.TenantID, inspection.VoyageID)
		if err != nil {
			return err
		}
		inspection, err = clearance.CloseInspection(inspection, finding, true, at, inspection.Version)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE inspections SET status=$2,finding=$3,closed_at=$4,version=$5 WHERE id=$1`, inspection.ID, inspection.Status, inspection.Finding, inspection.ClosedAt, inspection.Version); err != nil {
			return err
		}
		inspections, err := listInspectionsTx(ctx, tx, voyage.ID)
		if err != nil {
			return err
		}
		released, err = clearance.Release(voyage, inspections, at, voyage.Version)
		if err != nil {
			return err
		}
		if err := saveVoyageTx(ctx, tx, released, voyage.Version); err != nil {
			return err
		}
		if err := appendAuditTx(ctx, tx, audit.Event{ID: "audit-release-" + voyage.ID, TenantID: actor.TenantID, ActorID: actor.ID, Action: "voyage.released", ObjectType: "voyage", ObjectID: voyage.ID, Outcome: "ok", Details: map[string]string{"inspection": inspection.ID}, OccurredAt: at}); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"voyage_id": voyage.ID, "status": string(released.Status)})
		_, err = tx.Exec(ctx, `INSERT INTO outbox_jobs(id,tenant_id,topic,payload,status,available_at,created_at,updated_at) VALUES($1,$2,'voyage.released',$3,'pending',$4,$4,$4)`, "outbox-release-"+voyage.ID, voyage.TenantID, payload, at.UTC())
		return err
	})
	return released, err
}

func (db *Database) ReservePassage(ctx context.Context, actor auth.User, reservationID, voyageID string, chamber passage.Chamber, starts, ends time.Time, length, beam, draft float64, requestedAt time.Time) (passage.Reservation, error) {
	if actor.Role != auth.RoleDispatcher {
		return passage.Reservation{}, domain.ErrForbidden
	}
	if requestedAt.IsZero() {
		return passage.Reservation{}, domain.ErrInvalid
	}
	var reserved passage.Reservation
	err := db.WithTx(ctx, func(tx pgx.Tx) error {
		voyage, err := getVoyageTx(ctx, tx, actor.TenantID, voyageID)
		if err != nil {
			return err
		}
		if voyage.Status != clearance.VoyageCleared {
			return domain.ErrState
		}
		existing, err := listReservationsTx(ctx, tx, chamber.ID, starts, ends)
		if err != nil {
			return err
		}
		reserved, err = passage.Reserve(reservationID, voyageID, actor.TenantID, chamber, starts, ends, length, beam, draft, existing)
		if err != nil {
			return err
		}
		reserved, err = passage.Confirm(reserved, true, reserved.Version)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO passage_reservations(id,voyage_id,tenant_id,chamber_id,starts_at,ends_at,length_m,beam_m,draft_m,status,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, reserved.ID, reserved.VoyageID, reserved.TenantID, reserved.ChamberID, reserved.StartsAt, reserved.EndsAt, reserved.LengthM, reserved.BeamM, reserved.DraftM, reserved.Status, reserved.Version); err != nil {
			return mapConstraint(err)
		}
		scheduled, err := clearance.Transition(voyage, clearance.VoyageScheduled, starts)
		if err != nil {
			return err
		}
		if err := saveVoyageTx(ctx, tx, scheduled, voyage.Version); err != nil {
			return err
		}
		return appendAuditTx(ctx, tx, audit.Event{ID: "audit-reserve-" + reservationID, TenantID: actor.TenantID, ActorID: actor.ID, Action: "passage.reserved", ObjectType: "voyage", ObjectID: voyageID, Outcome: "ok", Details: map[string]string{"chamber": chamber.ID}, OccurredAt: requestedAt.UTC()})
	})
	return reserved, err
}

func getVoyageTx(ctx context.Context, tx pgx.Tx, tenant, id string) (clearance.Voyage, error) {
	var voyage clearance.Voyage
	err := tx.QueryRow(ctx, `SELECT id,tenant_id,vessel_imo,vessel_name,origin_port,destination_port,eta,status,manifest_hash,declared_at,cleared_at,version FROM voyages WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenant, id).Scan(&voyage.ID, &voyage.TenantID, &voyage.VesselIMO, &voyage.VesselName, &voyage.OriginPort, &voyage.DestinationPort, &voyage.ETA, &voyage.Status, &voyage.ManifestHash, &voyage.DeclaredAt, &voyage.ClearedAt, &voyage.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return clearance.Voyage{}, domain.ErrNotFound
	}
	return voyage, err
}

func saveVoyageTx(ctx context.Context, tx pgx.Tx, voyage clearance.Voyage, expected int64) error {
	tag, err := tx.Exec(ctx, `UPDATE voyages SET status=$2,manifest_hash=$3,declared_at=$4,cleared_at=$5,version=$6 WHERE id=$1 AND version=$7`, voyage.ID, voyage.Status, voyage.ManifestHash, voyage.DeclaredAt, voyage.ClearedAt, voyage.Version, expected)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return nil
}

func getInspectionTx(ctx context.Context, tx pgx.Tx, id string) (clearance.Inspection, error) {
	var inspection clearance.Inspection
	err := tx.QueryRow(ctx, `SELECT id,voyage_id,officer_id,kind,status,finding,opened_at,closed_at,version FROM inspections WHERE id=$1 FOR UPDATE`, id).Scan(&inspection.ID, &inspection.VoyageID, &inspection.OfficerID, &inspection.Kind, &inspection.Status, &inspection.Finding, &inspection.OpenedAt, &inspection.ClosedAt, &inspection.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return clearance.Inspection{}, domain.ErrNotFound
	}
	return inspection, err
}

func listInspectionsTx(ctx context.Context, tx pgx.Tx, voyageID string) ([]clearance.Inspection, error) {
	rows, err := tx.Query(ctx, `SELECT id,voyage_id,officer_id,kind,status,finding,opened_at,closed_at,version FROM inspections WHERE voyage_id=$1 ORDER BY opened_at,id`, voyageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []clearance.Inspection
	for rows.Next() {
		var inspection clearance.Inspection
		if err := rows.Scan(&inspection.ID, &inspection.VoyageID, &inspection.OfficerID, &inspection.Kind, &inspection.Status, &inspection.Finding, &inspection.OpenedAt, &inspection.ClosedAt, &inspection.Version); err != nil {
			return nil, err
		}
		result = append(result, inspection)
	}
	return result, rows.Err()
}

func listReservationsTx(ctx context.Context, tx pgx.Tx, chamber string, starts, ends time.Time) ([]passage.Reservation, error) {
	rows, err := tx.Query(ctx, `SELECT id,voyage_id,tenant_id,chamber_id,starts_at,ends_at,length_m,beam_m,draft_m,status,version FROM passage_reservations WHERE chamber_id=$1 AND status<>'cancelled' AND starts_at<$3 AND ends_at>$2 FOR UPDATE`, chamber, starts, ends)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []passage.Reservation
	for rows.Next() {
		var reservation passage.Reservation
		if err := rows.Scan(&reservation.ID, &reservation.VoyageID, &reservation.TenantID, &reservation.ChamberID, &reservation.StartsAt, &reservation.EndsAt, &reservation.LengthM, &reservation.BeamM, &reservation.DraftM, &reservation.Status, &reservation.Version); err != nil {
			return nil, err
		}
		result = append(result, reservation)
	}
	return result, rows.Err()
}

func appendAuditTx(ctx context.Context, tx pgx.Tx, candidate audit.Event) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, candidate.TenantID); err != nil {
		return err
	}
	var previous audit.Event
	err := tx.QueryRow(ctx, `SELECT id,tenant_id,actor_id,action,object_type,object_id,outcome,details,occurred_at,sequence,previous_hash,hash FROM audit_events WHERE tenant_id=$1 ORDER BY sequence DESC LIMIT 1 FOR UPDATE`, candidate.TenantID).Scan(&previous.ID, &previous.TenantID, &previous.ActorID, &previous.Action, &previous.ObjectType, &previous.ObjectID, &previous.Outcome, &previous.Details, &previous.OccurredAt, &previous.Sequence, &previous.PreviousHash, &previous.Hash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	event, err := audit.Append(previous, candidate)
	if err != nil {
		return err
	}
	details, _ := json.Marshal(event.Details)
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,tenant_id,actor_id,action,object_type,object_id,outcome,details,occurred_at,sequence,previous_hash,hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, event.ID, event.TenantID, event.ActorID, event.Action, event.ObjectType, event.ObjectID, event.Outcome, details, event.OccurredAt, event.Sequence, event.PreviousHash, event.Hash)
	return err
}

func mapConstraint(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", domain.ErrConflict, err)
}
