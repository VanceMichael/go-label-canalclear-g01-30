package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/audit"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func (db *Database) ListAuditEvents(ctx context.Context, tenantID string, filter audit.Filter, request domain.PageRequest) (domain.Page[audit.Event], error) {
	if tenantID == "" {
		return domain.Page[audit.Event]{}, domain.ErrInvalid
	}
	if err := filter.Validate(); err != nil {
		return domain.Page[audit.Event]{}, err
	}
	normalized, err := request.Normalize()
	if err != nil {
		return domain.Page[audit.Event]{}, err
	}
	args := []any{tenantID}
	clauses := []string{"tenant_id=$1"}
	addString := func(column, value string) {
		if value == "" {
			return
		}
		args = append(args, strings.TrimSpace(value))
		clauses = append(clauses, fmt.Sprintf("%s=$%d", column, len(args)))
	}
	addString("actor_id", filter.ActorID)
	addString("action", filter.Action)
	addString("object_id", filter.ObjectID)
	addString("outcome", filter.Outcome)
	if filter.From != nil {
		args = append(args, filter.From.UTC())
		clauses = append(clauses, fmt.Sprintf("occurred_at >= $%d", len(args)))
	}
	if filter.To != nil {
		args = append(args, filter.To.UTC())
		clauses = append(clauses, fmt.Sprintf("occurred_at <= $%d", len(args)))
	}
	if filter.Sequence > 0 {
		args = append(args, filter.Sequence)
		clauses = append(clauses, fmt.Sprintf("sequence > $%d", len(args)))
	}
	if normalized.Cursor != "" {
		cursor, err := domain.DecodeCursor(normalized.Cursor)
		if err != nil {
			return domain.Page[audit.Event]{}, err
		}
		args = append(args, cursor.SortTime, cursor.ID)
		clauses = append(clauses, fmt.Sprintf("(occurred_at,id) > ($%d,$%d)", len(args)-1, len(args)))
	}
	args = append(args, normalized.Limit+1)
	query := `SELECT id,tenant_id,actor_id,action,object_type,object_id,outcome,details,occurred_at,sequence,previous_hash,hash FROM audit_events WHERE ` + strings.Join(clauses, " AND ") + fmt.Sprintf(" ORDER BY occurred_at,id LIMIT $%d", len(args))
	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return domain.Page[audit.Event]{}, err
	}
	defer rows.Close()
	items := make([]audit.Event, 0, normalized.Limit+1)
	for rows.Next() {
		var event audit.Event
		var details []byte
		if err := rows.Scan(&event.ID, &event.TenantID, &event.ActorID, &event.Action, &event.ObjectType, &event.ObjectID, &event.Outcome, &details, &event.OccurredAt, &event.Sequence, &event.PreviousHash, &event.Hash); err != nil {
			return domain.Page[audit.Event]{}, err
		}
		if err := json.Unmarshal(details, &event.Details); err != nil {
			return domain.Page[audit.Event]{}, err
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return domain.Page[audit.Event]{}, err
	}
	return domain.BuildPage(items, normalized.Limit, func(event audit.Event) domain.Cursor {
		return domain.Cursor{SortTime: event.OccurredAt, ID: event.ID}
	})
}
