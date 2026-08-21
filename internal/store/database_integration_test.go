package store

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/idempotency"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/passage"
	"github.com/jackc/pgx/v5"
)

func integrationDB(t *testing.T) *Database {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	lock, err := db.Pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lock.Exec(context.Background(), `SELECT pg_advisory_lock(74001001)`); err != nil {
		lock.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `TRUNCATE passage_movements,outbox_jobs,idempotency_records,passage_reservations,inspections,manifests,voyages,sessions,users,audit_events RESTART IDENTITY CASCADE`)
		_, _ = lock.Exec(context.Background(), `SELECT pg_advisory_unlock(74001001)`)
		lock.Release()
	})
	for _, table := range []string{"passage_movements", "outbox_jobs", "idempotency_records", "passage_reservations", "inspections", "manifests", "voyages", "sessions", "users", "audit_events"} {
		if _, err := db.Pool.Exec(context.Background(), "DELETE FROM "+table); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func loginUser(t *testing.T, db *Database, email string, now time.Time) (auth.User, string) {
	t.Helper()
	token, _, err := db.Login(context.Background(), email, "canalclear-demo-password", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := db.Authenticate(context.Background(), token, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return user, token
}

func TestLoginLogoutAndDisabledAccount(t *testing.T) {
	db := integrationDB(t)
	now := time.Now().UTC()
	user, token := loginUser(t, db, "dispatcher@canalclear.test", now)
	if _, err := db.Pool.Exec(context.Background(), `UPDATE users SET disabled=true WHERE id=$1`, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Authenticate(context.Background(), token, now.Add(2*time.Minute)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("disabled session err=%v", err)
	}
	if _, err := db.Pool.Exec(context.Background(), `UPDATE users SET disabled=false WHERE id=$1`, user.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Logout(context.Background(), token, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Authenticate(context.Background(), token, now.Add(4*time.Minute)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("revoked session err=%v", err)
	}
}

func TestClearanceAndPassageWorkflowIsConnected(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	carrier, _ := loginUser(t, db, "carrier@canalclear.test", now)
	customs, _ := loginUser(t, db, "customs@canalclear.test", now)
	dispatcher, _ := loginUser(t, db, "dispatcher@canalclear.test", now)
	voyage, err := db.DeclareVoyage(ctx, carrier, DeclareRequest{ID: "voyage-integration", VesselIMO: "IMO7654321", VesselName: "Canal Pioneer", OriginPort: "CNQZH", DestinationPort: "SGSIN", ETA: now.Add(12 * time.Hour), At: now, Items: []clearance.CargoItem{{ContainerNo: "CNU7654321", HSCode: "8501", Description: "electric motors", GrossKg: 4200, Packages: 8}}})
	if err != nil || voyage.Status != clearance.VoyageDeclared {
		t.Fatalf("voyage=%+v err=%v", voyage, err)
	}
	inspection, held, err := db.OpenInspection(ctx, customs, "inspection-integration", voyage.ID, "document", now.Add(time.Minute))
	if err != nil || held.Status != clearance.VoyageHeld {
		t.Fatalf("inspection=%+v held=%+v err=%v", inspection, held, err)
	}
	released, err := db.PassInspectionAndRelease(ctx, customs, inspection.ID, "manifest and seal verified", now.Add(2*time.Minute))
	if err != nil || released.Status != clearance.VoyageCleared {
		t.Fatalf("released=%+v err=%v", released, err)
	}
	reservation, err := db.ReservePassage(ctx, dispatcher, "slot-integration", voyage.ID, passage.Chamber{ID: "qishi-east", Name: "Qishi East", MaxLengthM: 220, MaxBeamM: 34, MaxDraftM: 8}, now.Add(3*time.Hour), now.Add(4*time.Hour), 170, 27, 5.5, now.Add(3*time.Minute))
	if err != nil || reservation.Status != passage.SlotConfirmed {
		t.Fatalf("reservation=%+v err=%v", reservation, err)
	}
	stored, err := db.GetVoyage(ctx, carrier.TenantID, voyage.ID)
	if err != nil || stored.Status != clearance.VoyageScheduled {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
	var auditCount, outboxCount int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1`, carrier.TenantID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM outbox_jobs WHERE tenant_id=$1`, carrier.TenantID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 4 || outboxCount != 2 {
		t.Fatalf("audit=%d outbox=%d", auditCount, outboxCount)
	}
	var reservationAuditAt time.Time
	if err := db.Pool.QueryRow(ctx, `SELECT occurred_at FROM audit_events WHERE id=$1`, "audit-reserve-slot-integration").Scan(&reservationAuditAt); err != nil {
		t.Fatal(err)
	}
	if !reservationAuditAt.Equal(now.Add(3 * time.Minute).Truncate(time.Microsecond)) {
		t.Fatalf("reservation audit time=%v", reservationAuditAt)
	}
}

func TestWithTxRollsBackAllWrites(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	errSentinel := errors.New("stop transaction")
	err := db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,tenant_id,email,password_hash,role,version) VALUES('rollback-user','demo-port','rollback@test','hash','auditor',1)`); err != nil {
			return err
		}
		return errSentinel
	})
	if !errors.Is(err, errSentinel) {
		t.Fatalf("transaction error=%v", err)
	}
	var count int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id='rollback-user'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestDataSurvivesASecondDatabaseConnection(t *testing.T) {
	db := integrationDB(t)
	now := time.Now().UTC()
	carrier, _ := loginUser(t, db, "carrier@canalclear.test", now)
	created, err := db.DeclareVoyage(context.Background(), carrier, DeclareRequest{
		ID: "voyage-restart", VesselIMO: "IMO7654321", VesselName: "Persistent Passage",
		OriginPort: "CNQZH", DestinationPort: "SGSIN", ETA: now.Add(12 * time.Hour), At: now,
		Items: []clearance.CargoItem{{ContainerNo: "CNU7654321", HSCode: "8501", Description: "electric motors", GrossKg: 4200, Packages: 8}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	stored, err := reopened.GetVoyage(context.Background(), carrier.TenantID, created.ID)
	if err != nil || stored.ID != created.ID || stored.ManifestHash != created.ManifestHash || stored.Status != clearance.VoyageDeclared {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
}

func TestIdempotencyLifecyclePersistsReplayPayload(t *testing.T) {
	db := integrationDB(t)
	now := time.Now().UTC()
	record, err := idempotency.NewRecord("demo-port", "request-key-persist", "voyage.declare", idempotency.Fingerprint("POST", "/v1/voyages", []byte(`{"id":"one"}`)), now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	decision, started, err := db.BeginIdempotent(context.Background(), record, time.Minute)
	if err != nil || decision != idempotency.DecisionStart || started.Version != 1 {
		t.Fatalf("decision=%s started=%+v err=%v", decision, started, err)
	}
	completed, err := idempotency.Complete(started, 201, []byte(`{"id":"one"}`), now.Add(time.Second), started.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteIdempotent(context.Background(), completed); err != nil {
		t.Fatal(err)
	}
	decision, replay, err := db.BeginIdempotent(context.Background(), record, time.Minute)
	if err != nil || decision != idempotency.DecisionReplay || replay.ResponseCode != 201 || string(replay.ResponseBody) != `{"id":"one"}` {
		t.Fatalf("decision=%s replay=%+v err=%v", decision, replay, err)
	}
	conflict := record
	conflict.RequestHash = idempotency.Fingerprint("POST", "/v1/voyages", []byte(`{"id":"other"}`))
	decision, _, err = db.BeginIdempotent(context.Background(), conflict, time.Minute)
	if err != nil || decision != idempotency.DecisionConflict {
		t.Fatalf("conflict decision=%s err=%v", decision, err)
	}
}

func TestConcurrentPassageReservationsKeepSingleChamberInvariant(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	carrier, _ := loginUser(t, db, "carrier@canalclear.test", now)
	customs, _ := loginUser(t, db, "customs@canalclear.test", now)
	dispatcher, _ := loginUser(t, db, "dispatcher@canalclear.test", now)
	for index, voyageID := range []string{"voyage-concurrent-a", "voyage-concurrent-b"} {
		workflowAt := now.Add(-30*time.Minute + time.Duration(index)*10*time.Minute)
		voyage, err := db.DeclareVoyage(ctx, carrier, DeclareRequest{ID: voyageID, VesselIMO: "IMO7654321", VesselName: voyageID, OriginPort: "CNQZH", DestinationPort: "SGSIN", ETA: now.Add(12 * time.Hour), At: workflowAt, Items: []clearance.CargoItem{{ContainerNo: "CNU765432" + string(rune('1'+index)), HSCode: "8501", Description: "motors", GrossKg: 1000, Packages: 1}}})
		if err != nil {
			t.Fatal(err)
		}
		inspection, _, err := db.OpenInspection(ctx, customs, "inspection-"+voyageID, voyage.ID, "document", workflowAt.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.PassInspectionAndRelease(ctx, customs, inspection.ID, "verified", workflowAt.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	starts := now.Add(4 * time.Hour)
	chamber := passage.Chamber{ID: "qishi-concurrent", Name: "Qishi Concurrent", MaxLengthM: 220, MaxBeamM: 34, MaxDraftM: 8}
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	for index, voyageID := range []string{"voyage-concurrent-a", "voyage-concurrent-b"} {
		go func(index int, voyageID string) {
			ready.Done()
			<-start
			_, err := db.ReservePassage(ctx, dispatcher, "slot-concurrent-"+string(rune('a'+index)), voyageID, chamber, starts, starts.Add(time.Hour), 170, 27, 5, now.Add(-5*time.Minute))
			results <- err
		}(index, voyageID)
	}
	ready.Wait()
	close(start)
	successes := 0
	conflicts := 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, domain.ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected reservation error=%v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}
