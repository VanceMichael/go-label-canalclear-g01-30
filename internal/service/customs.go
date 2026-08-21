package service

import (
	"context"
	"fmt"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/customs"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type CustomsSubmission struct {
	Voyages VoyageRepository
	Gateway customs.Gateway
	Clock   Clock
}

func (submission CustomsSubmission) Submit(ctx context.Context, actor auth.User, voyageID string) (customs.Decision, error) {
	if submission.Voyages == nil || submission.Gateway == nil || submission.Clock == nil || voyageID == "" {
		return customs.Decision{}, fmt.Errorf("%w: customs submission", domain.ErrInvalid)
	}
	if err := auth.Authorize(actor, actor.TenantID, auth.PermissionInspectionManage); err != nil {
		return customs.Decision{}, err
	}
	voyage, err := submission.Voyages.FindVoyage(ctx, actor.TenantID, voyageID)
	if err != nil {
		return customs.Decision{}, err
	}
	if voyage.ManifestHash == "" {
		return customs.Decision{}, fmt.Errorf("%w: undeclared voyage", domain.ErrState)
	}
	decision, err := submission.Gateway.Submit(ctx, customs.Declaration{
		TenantID: actor.TenantID, VoyageID: voyage.ID, ManifestHash: voyage.ManifestHash,
		OriginPort: voyage.OriginPort, SubmittedAt: submission.Clock.Now(),
	})
	if err != nil {
		return customs.Decision{}, fmt.Errorf("submit voyage %s: %w", voyage.ID, err)
	}
	return decision, nil
}
