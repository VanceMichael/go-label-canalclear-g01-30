package clearance

import (
	"errors"
	"reflect"
	"testing"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func cargo(container, hs string, gross int64, packages int, hazardous bool) CargoItem {
	return CargoItem{ContainerNo: container, HSCode: hs, Description: "declared cargo", GrossKg: gross, Packages: packages, Hazardous: hazardous}
}

func TestSummarizeManifestAggregatesOperationalTotals(t *testing.T) {
	manifest, err := BuildManifest("voyage-one", []CargoItem{
		cargo("CNU0000001", "8501", 1200, 3, false),
		cargo("CNU0000002", "8501", 800, 2, true),
		cargo("CNU0000003", "2901", 500, 1, true),
	})
	if err != nil {
		t.Fatal(err)
	}
	totals, err := SummarizeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	want := ManifestTotals{Containers: 3, Packages: 6, GrossKg: 2500, HazardousUnits: 2, DistinctHSCodes: 2}
	if totals != want {
		t.Fatalf("totals=%+v want=%+v", totals, want)
	}
}

func TestAmendManifestSupportsAddReplaceAndRemoveWithoutMutation(t *testing.T) {
	current, err := BuildManifest("voyage-one", []CargoItem{
		cargo("CNU0000001", "8501", 1200, 3, false),
		cargo("CNU0000002", "8502", 800, 2, false),
	})
	if err != nil {
		t.Fatal(err)
	}
	replacement := cargo("ignored", "8501", 1300, 4, true)
	addition := cargo("ignored", "2901", 600, 1, false)
	updated, err := AmendManifest(current, []ManifestChange{
		{ContainerNo: "CNU0000001", Replacement: &replacement},
		{ContainerNo: "CNU0000002", Remove: true},
		{ContainerNo: "CNU0000003", Replacement: &addition},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Items) != 2 || updated.Items[0].ContainerNo != "CNU0000001" || updated.Items[1].ContainerNo != "CNU0000003" {
		t.Fatalf("items=%+v", updated.Items)
	}
	if !updated.Items[0].Hazardous || updated.Items[0].GrossKg != 1300 || current.Items[0].GrossKg != 1200 {
		t.Fatalf("updated=%+v current=%+v", updated.Items[0], current.Items[0])
	}
	if updated.Hash == current.Hash {
		t.Fatal("manifest hash must change")
	}
}

func TestAmendManifestRejectsAmbiguousChanges(t *testing.T) {
	current, _ := BuildManifest("voyage-one", []CargoItem{cargo("CNU0000001", "8501", 1200, 3, false)})
	replacement := cargo("CNU0000001", "8501", 1000, 2, false)
	tests := [][]ManifestChange{
		{},
		{{ContainerNo: "missing", Remove: true}},
		{{ContainerNo: "CNU0000001", Remove: true, Replacement: &replacement}},
		{{ContainerNo: "CNU0000001"}},
		{{ContainerNo: "CNU0000001", Remove: true}, {ContainerNo: "CNU0000001", Replacement: &replacement}},
	}
	for index, changes := range tests {
		if _, err := AmendManifest(current, changes); err == nil {
			t.Errorf("case %d expected error", index)
		}
	}
}

func TestCompareManifestsProducesCanonicalDiff(t *testing.T) {
	before, _ := BuildManifest("voyage-one", []CargoItem{
		cargo("CNU0000001", "8501", 100, 1, false),
		cargo("CNU0000002", "8502", 200, 2, false),
	})
	after, _ := BuildManifest("voyage-one", []CargoItem{
		cargo("CNU0000001", "8501", 150, 1, false),
		cargo("CNU0000003", "8503", 300, 3, true),
	})
	changes, err := CompareManifests(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0].ContainerNo != "CNU0000001" || changes[1].ContainerNo != "CNU0000002" || changes[2].ContainerNo != "CNU0000003" {
		t.Fatalf("changes=%+v", changes)
	}
	if changes[1].Remove != true || changes[2].Replacement == nil {
		t.Fatalf("changes=%+v", changes)
	}
}

func TestAssessManifestRiskUsesIndependentSignals(t *testing.T) {
	manifest, _ := BuildManifest("voyage-risk", []CargoItem{cargo("CNU0000001", "2901", 600000, 10, true)})
	risk, err := AssessManifestRisk(manifest, map[string]struct{}{"2901": {}})
	if err != nil {
		t.Fatal(err)
	}
	if risk.Band != RiskCritical || risk.Score != 75 {
		t.Fatalf("risk=%+v", risk)
	}
	wantReasons := []string{"high gross weight", "hazardous cargo", "controlled HS code"}
	if !reflect.DeepEqual(risk.Reasons, wantReasons) {
		t.Fatalf("reasons=%v", risk.Reasons)
	}
	if _, err := SummarizeManifest(Manifest{}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty manifest error=%v", err)
	}
}
