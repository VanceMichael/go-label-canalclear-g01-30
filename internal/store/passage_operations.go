package store

import (
	"context"
	"errors"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/audit"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/passage"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/service"
	"github.com/jackc/pgx/v5"
)

func (db *Database) MovePassage(ctx context.Context, actor auth.User, command service.MovePassageCommand) (passage.Reservation, error) {
	if actor.Role != auth.RoleLock {
		return passage.Reservation{}, domain.ErrForbidden
	}
	var updated passage.Reservation
	err := db.WithTx(ctx, func(tx pgx.Tx) error {
		reservation, err := getReservationTx(ctx, tx, actor.TenantID, command.ReservationID)
		if err != nil {
			return err
		}
		movement, next, err := passage.BuildMovement(reservation, command.Next, command.OccurredAt, actor.ID, command.Note)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE passage_reservations SET status=$2,version=$3 WHERE id=$1 AND version=$4`, next.ID, next.Status, next.Version, reservation.Version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return domain.ErrConflict
		}
		if _, err := tx.Exec(ctx, `INSERT INTO passage_movements(reservation_id,status,occurred_at,operator_id,note) VALUES($1,$2,$3,$4,$5)`, movement.ReservationID, movement.Status, movement.OccurredAt, movement.OperatorID, movement.Note); err != nil {
			return err
		}
		updated = next
		return appendAuditTx(ctx, tx, audit.Event{ID: "audit-passage-" + string(command.Next) + "-" + reservation.ID, TenantID: actor.TenantID, ActorID: actor.ID, Action: "passage." + string(command.Next), ObjectType: "passage_reservation", ObjectID: reservation.ID, Outcome: "ok", Details: map[string]string{"note": command.Note}, OccurredAt: command.OccurredAt})
	})
	return updated, err
}

func getReservationTx(ctx context.Context, tx pgx.Tx, tenantID, reservationID string) (passage.Reservation, error) {
	var reservation passage.Reservation
	err := tx.QueryRow(ctx, `SELECT id,voyage_id,tenant_id,chamber_id,starts_at,ends_at,length_m,beam_m,draft_m,status,version FROM passage_reservations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, reservationID).Scan(&reservation.ID, &reservation.VoyageID, &reservation.TenantID, &reservation.ChamberID, &reservation.StartsAt, &reservation.EndsAt, &reservation.LengthM, &reservation.BeamM, &reservation.DraftM, &reservation.Status, &reservation.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return passage.Reservation{}, domain.ErrNotFound
	}
	return reservation, err
}
