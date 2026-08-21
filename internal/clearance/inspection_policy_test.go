package clearance

import (
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func TestRequiredInspectionsFollowRiskBand(t *testing.T) {
	tests := []struct {
		name string
		risk ManifestRisk
		want []InspectionKind
	}{
		{name: "routine", risk: ManifestRisk{Band: RiskRoutine}, want: []InspectionKind{InspectionDocument}},
		{name: "elevated", risk: ManifestRisk{Band: RiskElevated}, want: []InspectionKind{InspectionDocument, InspectionSeal}},
		{name: "critical hazardous", risk: ManifestRisk{Band: RiskCritical, Reasons: []string{"hazardous cargo"}}, want: []InspectionKind{InspectionDocument, InspectionSeal, InspectionPhysical, InspectionHazard}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirements, err := RequiredInspections(test.risk, "CNQZH")
			if err != nil {
				t.Fatal(err)
			}
			if len(requirements) != len(test.want) {
				t.Fatalf("requirements=%+v", requirements)
			}
			for index := range test.want {
				if requirements[index].Kind != test.want[index] || !requirements[index].Required {
					t.Fatalf("requirement[%d]=%+v", index, requirements[index])
				}
			}
		})
	}
}

func TestInspectionSetCompleteRejectsMissingOrFailedChecks(t *testing.T) {
	requirements := []InspectionRequirement{
		{Kind: InspectionDocument, Required: true},
		{Kind: InspectionSeal, Required: true},
	}
	passed := []Inspection{
		{ID: "document", Kind: string(InspectionDocument), Status: "passed"},
		{ID: "seal", Kind: string(InspectionSeal), Status: "passed"},
	}
	if err := InspectionSetComplete(requirements, passed); err != nil {
		t.Fatal(err)
	}
	if err := InspectionSetComplete(requirements, passed[:1]); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("missing error=%v", err)
	}
	failed := append([]Inspection(nil), passed...)
	failed[1].Status = "failed"
	if err := InspectionSetComplete(requirements, failed); !errors.Is(err, domain.ErrState) {
		t.Fatalf("failed error=%v", err)
	}
}

func TestInspectionDeadlineUsesKindSpecificLeadTime(t *testing.T) {
	eta := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	voyage := Voyage{ETA: eta}
	now := eta.Add(-24 * time.Hour)
	tests := []struct {
		kind InspectionKind
		lead time.Duration
	}{
		{kind: InspectionDocument, lead: 12 * time.Hour},
		{kind: InspectionSeal, lead: 6 * time.Hour},
		{kind: InspectionPhysical, lead: 4 * time.Hour},
		{kind: InspectionHazard, lead: 4 * time.Hour},
	}
	for _, test := range tests {
		deadline, err := InspectionDeadline(voyage, test.kind, now)
		if err != nil || !deadline.Equal(eta.Add(-test.lead)) {
			t.Errorf("kind=%s deadline=%v err=%v", test.kind, deadline, err)
		}
	}
	late := eta.Add(time.Hour)
	deadline, err := InspectionDeadline(voyage, InspectionDocument, late)
	if err != nil || !deadline.Equal(late) {
		t.Fatalf("late deadline=%v err=%v", deadline, err)
	}
}

func TestLatestInspectionBreaksTimestampTiesByID(t *testing.T) {
	at := time.Now().UTC()
	inspections := []Inspection{
		{ID: "a", Kind: string(InspectionSeal), OpenedAt: at},
		{ID: "z", Kind: string(InspectionSeal), OpenedAt: at},
		{ID: "physical", Kind: string(InspectionPhysical), OpenedAt: at.Add(time.Hour)},
	}
	latest, found := LatestInspection(inspections, InspectionSeal)
	if !found || latest.ID != "z" {
		t.Fatalf("latest=%+v found=%v", latest, found)
	}
	if _, found := LatestInspection(inspections, InspectionHazard); found {
		t.Fatal("unexpected hazardous inspection")
	}
}
