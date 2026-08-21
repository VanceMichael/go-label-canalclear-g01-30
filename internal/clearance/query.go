package clearance

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type VoyageFilter struct {
	Statuses   []VoyageStatus
	Port       string
	ETAFrom    *time.Time
	ETATo      *time.Time
	VesselText string
}

func (filter VoyageFilter) Validate() error {
	validStatuses := map[VoyageStatus]struct{}{
		VoyageDraft: {}, VoyageDeclared: {}, VoyageHeld: {}, VoyageCleared: {},
		VoyageScheduled: {}, VoyageInTransit: {}, VoyageCompleted: {}, VoyageCancelled: {},
	}
	for _, status := range filter.Statuses {
		if _, ok := validStatuses[status]; !ok {
			return fmt.Errorf("%w: voyage status", domain.ErrInvalid)
		}
	}
	if filter.ETAFrom != nil && filter.ETATo != nil && filter.ETATo.Before(*filter.ETAFrom) {
		return fmt.Errorf("%w: ETA range", domain.ErrInvalid)
	}
	if len(filter.VesselText) > 100 {
		return fmt.Errorf("%w: vessel query", domain.ErrInvalid)
	}
	return nil
}

func FilterVoyages(voyages []Voyage, filter VoyageFilter) ([]Voyage, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	statuses := make(map[VoyageStatus]struct{}, len(filter.Statuses))
	for _, status := range filter.Statuses {
		statuses[status] = struct{}{}
	}
	port := strings.ToUpper(strings.TrimSpace(filter.Port))
	vesselText := strings.ToLower(strings.TrimSpace(filter.VesselText))
	result := make([]Voyage, 0, len(voyages))
	for _, voyage := range voyages {
		if len(statuses) > 0 {
			if _, ok := statuses[voyage.Status]; !ok {
				continue
			}
		}
		if port != "" && voyage.OriginPort != port && voyage.DestinationPort != port {
			continue
		}
		if filter.ETAFrom != nil && voyage.ETA.Before(filter.ETAFrom.UTC()) {
			continue
		}
		if filter.ETATo != nil && voyage.ETA.After(filter.ETATo.UTC()) {
			continue
		}
		if vesselText != "" && !strings.Contains(strings.ToLower(voyage.VesselName), vesselText) && !strings.Contains(strings.ToLower(voyage.VesselIMO), vesselText) {
			continue
		}
		result = append(result, voyage)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ETA.Equal(result[j].ETA) {
			return result[i].ID < result[j].ID
		}
		return result[i].ETA.Before(result[j].ETA)
	})
	return result, nil
}

func GroupVoyagesByStatus(voyages []Voyage) map[VoyageStatus]int {
	counts := make(map[VoyageStatus]int)
	for _, voyage := range voyages {
		counts[voyage.Status]++
	}
	return counts
}

func DelayedVoyages(voyages []Voyage, now time.Time, grace time.Duration) []Voyage {
	if grace < 0 {
		grace = 0
	}
	threshold := now.UTC().Add(-grace)
	result := make([]Voyage, 0)
	for _, voyage := range voyages {
		if voyage.ETA.Before(threshold) && voyage.Status != VoyageCompleted && voyage.Status != VoyageCancelled {
			result = append(result, voyage)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ETA.Equal(result[j].ETA) {
			return result[i].ID < result[j].ID
		}
		return result[i].ETA.Before(result[j].ETA)
	})
	return result
}
