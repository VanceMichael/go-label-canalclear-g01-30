package clearance

import (
	"fmt"
	"sort"
	"strings"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type ManifestTotals struct {
	Containers      int
	Packages        int
	GrossKg         int64
	HazardousUnits  int
	DistinctHSCodes int
}

type ManifestChange struct {
	ContainerNo string
	Remove      bool
	Replacement *CargoItem
}

type RiskBand string

const (
	RiskRoutine  RiskBand = "routine"
	RiskElevated RiskBand = "elevated"
	RiskCritical RiskBand = "critical"
)

type ManifestRisk struct {
	Band    RiskBand
	Score   int
	Reasons []string
}

func SummarizeManifest(manifest Manifest) (ManifestTotals, error) {
	if manifest.VoyageID == "" || len(manifest.Items) == 0 {
		return ManifestTotals{}, fmt.Errorf("%w: empty manifest", domain.ErrInvalid)
	}
	totals := ManifestTotals{Containers: len(manifest.Items)}
	hsCodes := make(map[string]struct{})
	for _, item := range manifest.Items {
		if item.ContainerNo == "" || item.HSCode == "" || item.GrossKg <= 0 || item.Packages <= 0 {
			return ManifestTotals{}, fmt.Errorf("%w: cargo item", domain.ErrInvalid)
		}
		totals.Packages += item.Packages
		totals.GrossKg += item.GrossKg
		if item.Hazardous {
			totals.HazardousUnits++
		}
		hsCodes[item.HSCode] = struct{}{}
	}
	totals.DistinctHSCodes = len(hsCodes)
	return totals, nil
}

func AmendManifest(current Manifest, changes []ManifestChange) (Manifest, error) {
	if len(changes) == 0 {
		return Manifest{}, fmt.Errorf("%w: manifest changes", domain.ErrInvalid)
	}
	items := make(map[string]CargoItem, len(current.Items))
	for _, item := range current.Items {
		items[item.ContainerNo] = item
	}
	seen := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		container := strings.ToUpper(strings.TrimSpace(change.ContainerNo))
		if container == "" {
			return Manifest{}, fmt.Errorf("%w: change container", domain.ErrInvalid)
		}
		if _, duplicate := seen[container]; duplicate {
			return Manifest{}, fmt.Errorf("%w: repeated manifest change", domain.ErrConflict)
		}
		seen[container] = struct{}{}
		_, exists := items[container]
		switch {
		case change.Remove:
			if !exists || change.Replacement != nil {
				return Manifest{}, fmt.Errorf("%w: removal target", domain.ErrConflict)
			}
			delete(items, container)
		case change.Replacement != nil:
			replacement := *change.Replacement
			replacement.ContainerNo = container
			items[container] = replacement
		default:
			return Manifest{}, fmt.Errorf("%w: empty manifest change", domain.ErrInvalid)
		}
	}
	if len(items) == 0 {
		return Manifest{}, fmt.Errorf("%w: manifest cannot be empty", domain.ErrState)
	}
	updated := make([]CargoItem, 0, len(items))
	for _, item := range items {
		updated = append(updated, item)
	}
	return BuildManifest(current.VoyageID, updated)
}

func CompareManifests(before, after Manifest) ([]ManifestChange, error) {
	if before.VoyageID == "" || before.VoyageID != after.VoyageID {
		return nil, fmt.Errorf("%w: manifest voyage mismatch", domain.ErrInvalid)
	}
	left := make(map[string]CargoItem, len(before.Items))
	right := make(map[string]CargoItem, len(after.Items))
	for _, item := range before.Items {
		left[item.ContainerNo] = item
	}
	for _, item := range after.Items {
		right[item.ContainerNo] = item
	}
	containers := make(map[string]struct{}, len(left)+len(right))
	for container := range left {
		containers[container] = struct{}{}
	}
	for container := range right {
		containers[container] = struct{}{}
	}
	ordered := make([]string, 0, len(containers))
	for container := range containers {
		ordered = append(ordered, container)
	}
	sort.Strings(ordered)
	changes := make([]ManifestChange, 0)
	for _, container := range ordered {
		beforeItem, inBefore := left[container]
		afterItem, inAfter := right[container]
		switch {
		case inBefore && !inAfter:
			changes = append(changes, ManifestChange{ContainerNo: container, Remove: true})
		case !inBefore && inAfter:
			copyItem := afterItem
			changes = append(changes, ManifestChange{ContainerNo: container, Replacement: &copyItem})
		case beforeItem != afterItem:
			copyItem := afterItem
			changes = append(changes, ManifestChange{ContainerNo: container, Replacement: &copyItem})
		}
	}
	return changes, nil
}

func AssessManifestRisk(manifest Manifest, highRiskHSCodes map[string]struct{}) (ManifestRisk, error) {
	totals, err := SummarizeManifest(manifest)
	if err != nil {
		return ManifestRisk{}, err
	}
	risk := ManifestRisk{Band: RiskRoutine}
	if totals.GrossKg > 500_000 {
		risk.Score += 25
		risk.Reasons = append(risk.Reasons, "high gross weight")
	}
	if totals.Containers > 100 {
		risk.Score += 20
		risk.Reasons = append(risk.Reasons, "large container count")
	}
	if totals.HazardousUnits > 0 {
		risk.Score += 35
		risk.Reasons = append(risk.Reasons, "hazardous cargo")
	}
	for _, item := range manifest.Items {
		if _, highRisk := highRiskHSCodes[item.HSCode]; highRisk {
			risk.Score += 15
			risk.Reasons = append(risk.Reasons, "controlled HS code")
			break
		}
	}
	if risk.Score >= 60 {
		risk.Band = RiskCritical
	} else if risk.Score >= 25 {
		risk.Band = RiskElevated
	}
	return risk, nil
}
