package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/store"
)

func integrationServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := store.Open(context.Background(), url)
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
	if _, err := db.Pool.Exec(context.Background(), `TRUNCATE passage_movements,outbox_jobs,idempotency_records,passage_reservations,inspections,manifests,voyages,sessions,users,audit_events RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if err := db.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := New(db, time.Hour)
	server.now = func() time.Time { return time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC) }
	return server, server.Handler()
}

func performJSON(t *testing.T, handler http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &payload)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "integration-request-123")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func loginToken(t *testing.T, handler http.Handler, email string) string {
	t.Helper()
	response := performJSON(t, handler, http.MethodPost, "/v1/auth/login", "", map[string]string{"email": email, "password": "canalclear-demo-password"})
	if response.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Token == "" {
		t.Fatalf("login=%+v err=%v", result, err)
	}
	return result.Token
}

func TestHTTPHealthReadyAndAuthorizationContract(t *testing.T) {
	_, handler := integrationServer(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		response := performJSON(t, handler, http.MethodGet, path, "", nil)
		if response.Code != http.StatusOK || response.Header().Get("X-Request-ID") != "integration-request-123" {
			t.Fatalf("path=%s status=%d headers=%v body=%s", path, response.Code, response.Header(), response.Body.String())
		}
	}
	response := performJSON(t, handler, http.MethodGet, "/v1/voyages/missing", "", nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated status=%d body=%s", response.Code, response.Body.String())
	}
	var failure errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Error.Code != "forbidden" || failure.Error.RequestID != "integration-request-123" {
		t.Fatalf("failure=%+v", failure)
	}
}

func TestHTTPRoleBoundaryAndLogoutRevocation(t *testing.T) {
	_, handler := integrationServer(t)
	customsToken := loginToken(t, handler, "customs@canalclear.test")
	declaration := map[string]any{
		"id": "voyage-role-test", "vessel_imo": "IMO7654321", "vessel_name": "Role Test",
		"origin_port": "CNQZH", "destination_port": "SGSIN", "eta": "2026-08-22T08:00:00Z",
		"items": []map[string]any{{"ContainerNo": "CNU0000001", "HSCode": "8501", "Description": "motors", "GrossKg": 1000, "Packages": 2}},
	}
	response := performJSON(t, handler, http.MethodPost, "/v1/voyages", customsToken, declaration)
	if response.Code != http.StatusForbidden {
		t.Fatalf("role status=%d body=%s", response.Code, response.Body.String())
	}
	response = performJSON(t, handler, http.MethodPost, "/v1/auth/logout", customsToken, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", response.Code, response.Body.String())
	}
	response = performJSON(t, handler, http.MethodGet, "/v1/voyages/missing", customsToken, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("revoked status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHTTPClearanceWorkflowAndTenantScopedQuery(t *testing.T) {
	_, handler := integrationServer(t)
	carrier := loginToken(t, handler, "carrier@canalclear.test")
	customs := loginToken(t, handler, "customs@canalclear.test")
	dispatcher := loginToken(t, handler, "dispatcher@canalclear.test")
	declaration := map[string]any{
		"id": "voyage-http", "vessel_imo": "IMO7654321", "vessel_name": "Canal HTTP",
		"origin_port": "CNQZH", "destination_port": "SGSIN", "eta": "2026-08-22T08:00:00Z",
		"items": []map[string]any{{"ContainerNo": "CNU0000001", "HSCode": "8501", "Description": "motors", "GrossKg": 1000, "Packages": 2}},
	}
	response := performJSON(t, handler, http.MethodPost, "/v1/voyages", carrier, declaration)
	if response.Code != http.StatusCreated {
		t.Fatalf("declare status=%d body=%s", response.Code, response.Body.String())
	}
	response = performJSON(t, handler, http.MethodPost, "/v1/voyages/voyage-http/inspections", customs, map[string]string{"ID": "inspection-http", "Kind": "document"})
	if response.Code != http.StatusCreated {
		t.Fatalf("inspection status=%d body=%s", response.Code, response.Body.String())
	}
	response = performJSON(t, handler, http.MethodPost, "/v1/inspections/inspection-http/pass", customs, map[string]string{"Finding": "documents verified"})
	if response.Code != http.StatusOK {
		t.Fatalf("release status=%d body=%s", response.Code, response.Body.String())
	}
	response = performJSON(t, handler, http.MethodPost, "/v1/voyages/voyage-http/passage-reservations", dispatcher, map[string]any{
		"ID": "slot-http", "chamber_id": "qishi-east", "starts_at": "2026-08-21T12:00:00Z", "ends_at": "2026-08-21T13:00:00Z", "length_m": 170, "beam_m": 27, "draft_m": 5.5,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("reserve status=%d body=%s", response.Code, response.Body.String())
	}
	response = performJSON(t, handler, http.MethodGet, "/v1/voyages?status=scheduled&port=CNQZH&limit=10", carrier, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}
	var page struct {
		Items []clearance.Voyage `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Items[0].Status != clearance.VoyageScheduled {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}
