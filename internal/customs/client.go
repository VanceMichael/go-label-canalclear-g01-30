package customs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type DecisionStatus string

const (
	DecisionAccepted DecisionStatus = "accepted"
	DecisionReview   DecisionStatus = "review"
	DecisionRejected DecisionStatus = "rejected"
)

type Declaration struct {
	TenantID     string    `json:"tenant_id"`
	VoyageID     string    `json:"voyage_id"`
	ManifestHash string    `json:"manifest_hash"`
	OriginPort   string    `json:"origin_port"`
	SubmittedAt  time.Time `json:"submitted_at"`
}

type Decision struct {
	Reference string         `json:"reference"`
	Status    DecisionStatus `json:"status"`
	Reason    string         `json:"reason"`
	DecidedAt time.Time      `json:"decided_at"`
}

type Gateway interface {
	Submit(context.Context, Declaration) (Decision, error)
}

type HTTPClient struct {
	Endpoint   string
	Credential string
	Client     *http.Client
	UserAgent  string
}

func (client HTTPClient) validate() error {
	parsed, err := url.Parse(client.Endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: customs gateway endpoint", domain.ErrInvalid)
	}
	if strings.TrimSpace(client.Credential) == "" {
		return fmt.Errorf("%w: customs gateway credential", domain.ErrInvalid)
	}
	return nil
}

func (client HTTPClient) Submit(ctx context.Context, declaration Declaration) (Decision, error) {
	if err := client.validate(); err != nil {
		return Decision{}, err
	}
	if err := validateDeclaration(declaration); err != nil {
		return Decision{}, err
	}
	payload, err := json.Marshal(declaration)
	if err != nil {
		return Decision{}, fmt.Errorf("encode customs declaration: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(client.Endpoint, "/")+"/v1/declarations", bytes.NewReader(payload))
	if err != nil {
		return Decision{}, fmt.Errorf("create customs request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.Credential)
	request.Header.Set("Content-Type", "application/json")
	if client.UserAgent != "" {
		request.Header.Set("User-Agent", client.UserAgent)
	}
	httpClient := client.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Decision{}, err
		}
		return Decision{}, fmt.Errorf("customs gateway request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return Decision{}, fmt.Errorf("read customs response: %w", err)
	}
	if response.StatusCode == http.StatusConflict {
		return Decision{}, domain.ErrConflict
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return Decision{}, domain.ErrForbidden
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Decision{}, fmt.Errorf("customs gateway status %d", response.StatusCode)
	}
	var decision Decision
	if err := json.Unmarshal(body, &decision); err != nil {
		return Decision{}, fmt.Errorf("decode customs response: %w", err)
	}
	if err := validateDecision(decision); err != nil {
		return Decision{}, err
	}
	return decision, nil
}

func validateDeclaration(declaration Declaration) error {
	if declaration.TenantID == "" || declaration.VoyageID == "" || len(declaration.ManifestHash) != 64 || declaration.OriginPort == "" || declaration.SubmittedAt.IsZero() {
		return fmt.Errorf("%w: customs declaration", domain.ErrInvalid)
	}
	return nil
}

func validateDecision(decision Decision) error {
	if decision.Reference == "" || decision.DecidedAt.IsZero() {
		return fmt.Errorf("%w: customs decision", domain.ErrInvalid)
	}
	switch decision.Status {
	case DecisionAccepted, DecisionReview:
		return nil
	case DecisionRejected:
		if strings.TrimSpace(decision.Reason) == "" {
			return fmt.Errorf("%w: rejection reason", domain.ErrInvalid)
		}
		return nil
	default:
		return fmt.Errorf("%w: customs decision status", domain.ErrInvalid)
	}
}
