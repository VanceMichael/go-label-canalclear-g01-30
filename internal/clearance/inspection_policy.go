package clearance

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type InspectionKind string

const (
	InspectionDocument InspectionKind = "document"
	InspectionSeal     InspectionKind = "seal"
	InspectionPhysical InspectionKind = "physical"
	InspectionHazard   InspectionKind = "hazardous_cargo"
)

type InspectionRequirement struct {
	Kind     InspectionKind
	Required bool
	Reason   string
}

func RequiredInspections(risk ManifestRisk, originPort string) ([]InspectionRequirement, error) {
	if risk.Band == "" || strings.TrimSpace(originPort) == "" {
		return nil, fmt.Errorf("%w: inspection policy input", domain.ErrInvalid)
	}
	requirements := []InspectionRequirement{{Kind: InspectionDocument, Required: true, Reason: "all declarations require document review"}}
	if risk.Band == RiskElevated || risk.Band == RiskCritical {
		requirements = append(requirements, InspectionRequirement{Kind: InspectionSeal, Required: true, Reason: "risk score requires seal verification"})
	}
	if risk.Band == RiskCritical {
		requirements = append(requirements, InspectionRequirement{Kind: InspectionPhysical, Required: true, Reason: "critical risk requires physical inspection"})
	}
	for _, reason := range risk.Reasons {
		if reason == "hazardous cargo" {
			requirements = append(requirements, InspectionRequirement{Kind: InspectionHazard, Required: true, Reason: "hazardous cargo requires specialist review"})
			break
		}
	}
	return requirements, nil
}

func InspectionSetComplete(requirements []InspectionRequirement, inspections []Inspection) error {
	passed := make(map[InspectionKind]bool)
	for _, inspection := range inspections {
		if inspection.Status == "passed" {
			passed[InspectionKind(inspection.Kind)] = true
		}
		if inspection.Status == "failed" {
			return fmt.Errorf("%w: inspection %s failed", domain.ErrState, inspection.ID)
		}
	}
	missing := make([]string, 0)
	for _, requirement := range requirements {
		if requirement.Required && !passed[requirement.Kind] {
			missing = append(missing, string(requirement.Kind))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%w: missing inspections %s", domain.ErrConflict, strings.Join(missing, ","))
	}
	return nil
}

func InspectionDeadline(voyage Voyage, kind InspectionKind, now time.Time) (time.Time, error) {
	if voyage.ETA.IsZero() || now.IsZero() {
		return time.Time{}, fmt.Errorf("%w: inspection deadline", domain.ErrInvalid)
	}
	lead := 2 * time.Hour
	switch kind {
	case InspectionDocument:
		lead = 12 * time.Hour
	case InspectionSeal:
		lead = 6 * time.Hour
	case InspectionPhysical, InspectionHazard:
		lead = 4 * time.Hour
	default:
		return time.Time{}, fmt.Errorf("%w: inspection kind", domain.ErrInvalid)
	}
	deadline := voyage.ETA.Add(-lead).UTC()
	if deadline.Before(now.UTC()) {
		return now.UTC(), nil
	}
	return deadline, nil
}

func LatestInspection(inspections []Inspection, kind InspectionKind) (Inspection, bool) {
	var latest Inspection
	found := false
	for _, inspection := range inspections {
		if InspectionKind(inspection.Kind) != kind {
			continue
		}
		if !found || inspection.OpenedAt.After(latest.OpenedAt) || (inspection.OpenedAt.Equal(latest.OpenedAt) && inspection.ID > latest.ID) {
			latest = inspection
			found = true
		}
	}
	return latest, found
}
