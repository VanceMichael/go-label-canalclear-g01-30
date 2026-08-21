package clearance

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type ComplianceRule struct {
	ID             string
	HSCodes        []string
	OriginPorts    []string
	HazardousOnly  bool
	MaximumGrossKg int64
	EffectiveFrom  time.Time
	EffectiveTo    *time.Time
	RequiredKind   InspectionKind
}

type ComplianceFinding struct {
	RuleID      string
	ContainerNo string
	Severity    RiskBand
	Message     string
	Inspection  InspectionKind
}

func EvaluateCompliance(manifest Manifest, originPort string, rules []ComplianceRule, at time.Time) ([]ComplianceFinding, error) {
	if manifest.VoyageID == "" || originPort == "" || at.IsZero() {
		return nil, fmt.Errorf("%w: compliance input", domain.ErrInvalid)
	}
	findings := make([]ComplianceFinding, 0)
	for _, rule := range rules {
		if err := validateComplianceRule(rule); err != nil {
			return nil, err
		}
		if at.Before(rule.EffectiveFrom) || rule.EffectiveTo != nil && at.After(*rule.EffectiveTo) || !containsFold(rule.OriginPorts, originPort) && len(rule.OriginPorts) > 0 {
			continue
		}
		for _, item := range manifest.Items {
			if len(rule.HSCodes) > 0 && !containsFold(rule.HSCodes, item.HSCode) {
				continue
			}
			if rule.HazardousOnly && !item.Hazardous {
				continue
			}
			if rule.MaximumGrossKg > 0 && item.GrossKg <= rule.MaximumGrossKg {
				continue
			}
			severity := RiskElevated
			if item.Hazardous || rule.MaximumGrossKg > 0 && item.GrossKg > rule.MaximumGrossKg*2 {
				severity = RiskCritical
			}
			findings = append(findings, ComplianceFinding{
				RuleID: rule.ID, ContainerNo: item.ContainerNo, Severity: severity,
				Message: "cargo requires additional compliance review", Inspection: rule.RequiredKind,
			})
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity > findings[j].Severity
		}
		if findings[i].RuleID != findings[j].RuleID {
			return findings[i].RuleID < findings[j].RuleID
		}
		return findings[i].ContainerNo < findings[j].ContainerNo
	})
	return findings, nil
}

func RequiredKindsFromFindings(findings []ComplianceFinding) []InspectionKind {
	unique := make(map[InspectionKind]struct{})
	for _, finding := range findings {
		if finding.Inspection != "" {
			unique[finding.Inspection] = struct{}{}
		}
	}
	result := make([]InspectionKind, 0, len(unique))
	for kind := range unique {
		result = append(result, kind)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func validateComplianceRule(rule ComplianceRule) error {
	if rule.ID == "" || rule.EffectiveFrom.IsZero() || rule.RequiredKind == "" {
		return fmt.Errorf("%w: compliance rule", domain.ErrInvalid)
	}
	if rule.EffectiveTo != nil && rule.EffectiveTo.Before(rule.EffectiveFrom) {
		return fmt.Errorf("%w: compliance effective range", domain.ErrInvalid)
	}
	if len(rule.HSCodes) == 0 && len(rule.OriginPorts) == 0 && !rule.HazardousOnly && rule.MaximumGrossKg <= 0 {
		return fmt.Errorf("%w: compliance rule has no condition", domain.ErrInvalid)
	}
	return nil
}

func containsFold(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}
