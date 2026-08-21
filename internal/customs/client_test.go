package customs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func declaration() Declaration {
	return Declaration{
		TenantID: "tenant-a", VoyageID: "voyage-one", ManifestHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		OriginPort: "CNQZH", SubmittedAt: time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC),
	}
}

func TestHTTPClientSubmitsAuthenticatedDeclaration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/declarations" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer gateway-secret" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("headers=%v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"reference":"customs-1","status":"accepted","reason":"","decided_at":"2026-08-21T08:01:00Z"}`))
	}))
	defer server.Close()
	client := HTTPClient{Endpoint: server.URL, Credential: "gateway-secret", Client: server.Client(), UserAgent: "CanalClear/test"}
	decision, err := client.Submit(context.Background(), declaration())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Reference != "customs-1" || decision.Status != DecisionAccepted {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestHTTPClientMapsStableGatewayFailures(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{status: http.StatusUnauthorized, want: domain.ErrForbidden},
		{status: http.StatusForbidden, want: domain.ErrForbidden},
		{status: http.StatusConflict, want: domain.ErrConflict},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(test.status) }))
			defer server.Close()
			client := HTTPClient{Endpoint: server.URL, Credential: "secret", Client: server.Client()}
			if _, err := client.Submit(context.Background(), declaration()); !errors.Is(err, test.want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestHTTPClientRejectsMalformedConfigurationAndResponse(t *testing.T) {
	if _, err := (HTTPClient{Endpoint: "://bad", Credential: "secret"}).Submit(context.Background(), declaration()); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("endpoint error=%v", err)
	}
	if _, err := (HTTPClient{Endpoint: "https://example.test"}).Submit(context.Background(), declaration()); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("credential error=%v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"reference":"","status":"unknown"}`))
	}))
	defer server.Close()
	client := HTTPClient{Endpoint: server.URL, Credential: "secret", Client: server.Client()}
	if _, err := client.Submit(context.Background(), declaration()); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("decision error=%v", err)
	}
}

type gatewaySequence struct {
	mu       sync.Mutex
	errors   []error
	decision Decision
	attempts int
}

func (gateway *gatewaySequence) Submit(_ context.Context, _ Declaration) (Decision, error) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	index := gateway.attempts
	gateway.attempts++
	if index < len(gateway.errors) && gateway.errors[index] != nil {
		return Decision{}, gateway.errors[index]
	}
	return gateway.decision, nil
}

func TestRetryingGatewayRetriesTransientFailureOnly(t *testing.T) {
	transient := errors.New("temporary network failure")
	next := &gatewaySequence{errors: []error{transient, transient}, decision: Decision{Reference: "ok", Status: DecisionAccepted, DecidedAt: time.Now()}}
	var sleeps []time.Duration
	gateway := RetryingGateway{
		Next: next, Policy: RetryPolicy{Attempts: 3, Backoff: time.Second, Maximum: 2 * time.Second},
		Sleep: func(_ context.Context, duration time.Duration) error { sleeps = append(sleeps, duration); return nil },
	}
	decision, err := gateway.Submit(context.Background(), declaration())
	if err != nil || decision.Reference != "ok" || next.attempts != 3 {
		t.Fatalf("decision=%+v attempts=%d err=%v", decision, next.attempts, err)
	}
	if len(sleeps) != 2 || sleeps[0] != time.Second || sleeps[1] != 2*time.Second {
		t.Fatalf("sleeps=%v", sleeps)
	}
	next = &gatewaySequence{errors: []error{domain.ErrForbidden}}
	gateway.Next = next
	if _, err := gateway.Submit(context.Background(), declaration()); !errors.Is(err, domain.ErrForbidden) || next.attempts != 1 {
		t.Fatalf("permanent attempts=%d err=%v", next.attempts, err)
	}
}

func TestRetrySleepHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("sleep error=%v", err)
	}
}
