package audit

import (
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func TestAuditChainDetectsTampering(t *testing.T) {
	now := time.Now().UTC()
	first, err := Append(Event{}, Event{ID: "1", TenantID: "tenant", ActorID: "actor", Action: "voyage.declare", ObjectType: "voyage", ObjectID: "v1", Outcome: "ok", Details: map[string]string{"port": "CNQZH"}, OccurredAt: now})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Append(first, Event{ID: "2", TenantID: "tenant", ActorID: "officer", Action: "voyage.release", ObjectType: "voyage", ObjectID: "v1", Outcome: "ok", OccurredAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify([]Event{second, first}); err != nil {
		t.Fatal(err)
	}
	second.Outcome = "failed"
	if err := Verify([]Event{first, second}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("tamper err=%v", err)
	}
}
