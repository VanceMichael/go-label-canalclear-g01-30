package clearance

import (
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func voyageFixture(t *testing.T, now time.Time) (Voyage, Manifest) {
	t.Helper()
	voyage, err := NewVoyage("voyage-1", "tenant-1", "IMO1234567", "West Passage", "CNQZH", "SGSIN", now.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildManifest(voyage.ID, []CargoItem{{ContainerNo: "CNU1234567", HSCode: "8409", Description: "marine engine parts", GrossKg: 8000, Packages: 12}})
	if err != nil {
		t.Fatal(err)
	}
	return voyage, manifest
}

func TestClearanceRequiresPassedInspection(t *testing.T) {
	now := time.Now().UTC()
	voyage, manifest := voyageFixture(t, now)
	declared, err := Declare(voyage, manifest, now, 1)
	if err != nil {
		t.Fatal(err)
	}
	inspection, held, err := OpenInspection("inspection-1", declared, "officer-1", "document", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Release(held, []Inspection{inspection}, now.Add(2*time.Minute), held.Version); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("open inspection release err=%v", err)
	}
	inspection, err = CloseInspection(inspection, "documents consistent", true, now.Add(3*time.Minute), 1)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := Release(held, []Inspection{inspection}, now.Add(4*time.Minute), held.Version)
	if err != nil || cleared.Status != VoyageCleared || cleared.ClearedAt == nil {
		t.Fatalf("cleared=%+v err=%v", cleared, err)
	}
}

func TestManifestIsCanonicalAndIndependent(t *testing.T) {
	items := []CargoItem{
		{ContainerNo: "Z", HSCode: "1", Description: "z", GrossKg: 20, Packages: 1},
		{ContainerNo: "A", HSCode: "2", Description: "a", GrossKg: 10, Packages: 2},
	}
	manifest, err := BuildManifest("voyage", items)
	if err != nil {
		t.Fatal(err)
	}
	items[0].Description = "mutated"
	if manifest.Items[0].ContainerNo != "A" || manifest.Items[1].Description != "z" || len(manifest.Hash) != 64 {
		t.Fatalf("manifest=%+v", manifest)
	}
}
