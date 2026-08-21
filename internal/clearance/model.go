package clearance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type VoyageStatus string

const (
	VoyageDraft     VoyageStatus = "draft"
	VoyageDeclared  VoyageStatus = "declared"
	VoyageHeld      VoyageStatus = "held"
	VoyageCleared   VoyageStatus = "cleared"
	VoyageScheduled VoyageStatus = "scheduled"
	VoyageInTransit VoyageStatus = "in_transit"
	VoyageCompleted VoyageStatus = "completed"
	VoyageCancelled VoyageStatus = "cancelled"
)

type Voyage struct {
	ID              string
	TenantID        string
	VesselIMO       string
	VesselName      string
	OriginPort      string
	DestinationPort string
	ETA             time.Time
	Status          VoyageStatus
	ManifestHash    string
	DeclaredAt      *time.Time
	ClearedAt       *time.Time
	Version         int64
}

type CargoItem struct {
	ContainerNo string
	HSCode      string
	Description string
	GrossKg     int64
	Packages    int
	Hazardous   bool
}

type Manifest struct {
	VoyageID string
	Items    []CargoItem
	Hash     string
}

type Inspection struct {
	ID        string
	VoyageID  string
	OfficerID string
	Kind      string
	Status    string
	Finding   string
	OpenedAt  time.Time
	ClosedAt  *time.Time
	Version   int64
}

func NewVoyage(id, tenant, imo, name, origin, destination string, eta time.Time) (Voyage, error) {
	if id == "" || tenant == "" || len(strings.TrimSpace(imo)) < 7 || name == "" || origin == "" || destination == "" || origin == destination || eta.IsZero() {
		return Voyage{}, fmt.Errorf("%w: voyage declaration", domain.ErrInvalid)
	}
	return Voyage{ID: id, TenantID: tenant, VesselIMO: strings.TrimSpace(imo), VesselName: strings.TrimSpace(name), OriginPort: origin, DestinationPort: destination, ETA: eta.UTC(), Status: VoyageDraft, Version: 1}, nil
}

func BuildManifest(voyageID string, items []CargoItem) (Manifest, error) {
	if voyageID == "" || len(items) == 0 {
		return Manifest{}, fmt.Errorf("%w: manifest", domain.ErrInvalid)
	}
	copyItems := append([]CargoItem(nil), items...)
	seen := make(map[string]struct{}, len(items))
	for _, item := range copyItems {
		if item.ContainerNo == "" || item.HSCode == "" || item.Description == "" || item.GrossKg <= 0 || item.Packages <= 0 {
			return Manifest{}, fmt.Errorf("%w: cargo item", domain.ErrInvalid)
		}
		if _, exists := seen[item.ContainerNo]; exists {
			return Manifest{}, fmt.Errorf("%w: duplicate container", domain.ErrConflict)
		}
		seen[item.ContainerNo] = struct{}{}
	}
	sort.Slice(copyItems, func(i, j int) bool { return copyItems[i].ContainerNo < copyItems[j].ContainerNo })
	payload, _ := json.Marshal(copyItems)
	sum := sha256.Sum256(payload)
	return Manifest{VoyageID: voyageID, Items: copyItems, Hash: hex.EncodeToString(sum[:])}, nil
}

func Declare(voyage Voyage, manifest Manifest, at time.Time, expectedVersion int64) (Voyage, error) {
	if voyage.Status != VoyageDraft || voyage.Version != expectedVersion || manifest.VoyageID != voyage.ID || len(manifest.Hash) != 64 || at.IsZero() {
		return voyage, fmt.Errorf("%w: voyage cannot be declared", domain.ErrConflict)
	}
	out := voyage
	out.Status = VoyageDeclared
	out.ManifestHash = manifest.Hash
	declared := at.UTC()
	out.DeclaredAt = &declared
	out.Version++
	return out, nil
}

func OpenInspection(id string, voyage Voyage, officer, kind string, at time.Time) (Inspection, Voyage, error) {
	if id == "" || officer == "" || kind == "" || voyage.Status != VoyageDeclared || at.IsZero() {
		return Inspection{}, voyage, fmt.Errorf("%w: inspection opening", domain.ErrState)
	}
	inspection := Inspection{ID: id, VoyageID: voyage.ID, OfficerID: officer, Kind: kind, Status: "open", OpenedAt: at.UTC(), Version: 1}
	out := voyage
	out.Status = VoyageHeld
	out.Version++
	return inspection, out, nil
}

func CloseInspection(inspection Inspection, finding string, passed bool, at time.Time, expectedVersion int64) (Inspection, error) {
	if inspection.Status != "open" || inspection.Version != expectedVersion || strings.TrimSpace(finding) == "" || at.Before(inspection.OpenedAt) {
		return inspection, fmt.Errorf("%w: inspection closure", domain.ErrConflict)
	}
	out := inspection
	if passed {
		out.Status = "passed"
	} else {
		out.Status = "failed"
	}
	out.Finding = strings.TrimSpace(finding)
	closed := at.UTC()
	out.ClosedAt = &closed
	out.Version++
	return out, nil
}

func Release(voyage Voyage, inspections []Inspection, at time.Time, expectedVersion int64) (Voyage, error) {
	if (voyage.Status != VoyageDeclared && voyage.Status != VoyageHeld) || voyage.Version != expectedVersion || voyage.ManifestHash == "" {
		return voyage, fmt.Errorf("%w: voyage release state", domain.ErrState)
	}
	if len(inspections) == 0 {
		return voyage, fmt.Errorf("%w: release inspection", domain.ErrConflict)
	}
	for _, inspection := range inspections {
		if inspection.VoyageID != voyage.ID || inspection.Status != "passed" {
			return voyage, fmt.Errorf("%w: unresolved inspection", domain.ErrConflict)
		}
	}
	out := voyage
	out.Status = VoyageCleared
	cleared := at.UTC()
	out.ClearedAt = &cleared
	out.Version++
	return out, nil
}

func Transition(voyage Voyage, next VoyageStatus, at time.Time) (Voyage, error) {
	allowed := map[VoyageStatus]map[VoyageStatus]bool{
		VoyageDraft:     {VoyageDeclared: true, VoyageCancelled: true},
		VoyageDeclared:  {VoyageHeld: true, VoyageCleared: true, VoyageCancelled: true},
		VoyageHeld:      {VoyageCleared: true, VoyageCancelled: true},
		VoyageCleared:   {VoyageScheduled: true, VoyageCancelled: true},
		VoyageScheduled: {VoyageInTransit: true, VoyageCancelled: true},
		VoyageInTransit: {VoyageCompleted: true},
	}
	if !allowed[voyage.Status][next] || at.IsZero() {
		return voyage, fmt.Errorf("%w: %s to %s", domain.ErrState, voyage.Status, next)
	}
	out := voyage
	out.Status = next
	out.Version++
	return out, nil
}
