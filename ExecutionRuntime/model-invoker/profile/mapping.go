package profile

import (
	"fmt"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type MappingAction string

const (
	MappingExact             MappingAction = "exact"
	MappingTransformed       MappingAction = "transformed"
	MappingConfigured        MappingAction = "configured"
	MappingSynthesized       MappingAction = "synthesized"
	MappingDegraded          MappingAction = "degraded"
	MappingRetainedExtension MappingAction = "retained_extension"
	MappingRejected          MappingAction = "rejected"
	MappingUnobservable      MappingAction = "unobservable"
)

type MappingDecisionV2 struct {
	SourcePath string                 `json:"source_path"`
	TargetPath string                 `json:"target_path"`
	Action     MappingAction          `json:"action"`
	Origin     union.CapabilityOrigin `json:"origin"`
	Detail     string                 `json:"detail"`
	Evidence   string                 `json:"evidence"`
}

type MappingReportV2 struct {
	Decisions []MappingDecisionV2 `json:"decisions"`
	Digest    string              `json:"digest"`
}

type CapabilityResidual struct {
	Path       string `json:"path"`
	Capability string `json:"capability,omitempty"`
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Impact     string `json:"impact"`
	Mitigation string `json:"mitigation"`
}

func (report *MappingReportV2) finalize() error {
	if report == nil {
		return fmt.Errorf("mapping report is nil")
	}
	sort.Slice(report.Decisions, func(i, j int) bool {
		left, right := report.Decisions[i], report.Decisions[j]
		if left.SourcePath != right.SourcePath {
			return left.SourcePath < right.SourcePath
		}
		if left.TargetPath != right.TargetPath {
			return left.TargetPath < right.TargetPath
		}
		return left.Action < right.Action
	})
	seen := make(map[string]struct{}, len(report.Decisions))
	for _, decision := range report.Decisions {
		if decision.SourcePath == "" || decision.TargetPath == "" || decision.Detail == "" || decision.Evidence == "" ||
			decision.Origin == "" {
			return fmt.Errorf("mapping decision is incomplete")
		}
		switch decision.Origin {
		case union.CapabilityOriginNative, union.CapabilityOriginProviderHosted, union.CapabilityOriginHarnessHosted,
			union.CapabilityOriginCallerHosted, union.CapabilityOriginEmulated, union.CapabilityOriginUnavailable:
		default:
			return fmt.Errorf("mapping origin %q is invalid", decision.Origin)
		}
		switch decision.Action {
		case MappingExact, MappingTransformed, MappingConfigured, MappingSynthesized, MappingDegraded,
			MappingRetainedExtension, MappingRejected, MappingUnobservable:
		default:
			return fmt.Errorf("mapping action %q is invalid", decision.Action)
		}
		key := decision.SourcePath + "\x00" + decision.TargetPath
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("mapping decision %q is duplicated", decision.SourcePath)
		}
		seen[key] = struct{}{}
	}
	digest, err := digestJSON(report.Decisions)
	if err != nil {
		return err
	}
	report.Digest = digest
	return nil
}

func mappingActionForFidelity(fidelity union.SemanticFidelity, surface ExecutionSurface) MappingAction {
	switch fidelity {
	case union.SemanticFidelityExact:
		if surface == ExecutionSurfaceDirectAPI {
			return MappingExact
		}
		return MappingConfigured
	case union.SemanticFidelityTransformed:
		return MappingTransformed
	case union.SemanticFidelityDegraded:
		return MappingDegraded
	default:
		return MappingRejected
	}
}

func unionFidelityForMapping(action MappingAction) union.SemanticFidelity {
	switch action {
	case MappingExact:
		return union.SemanticFidelityExact
	case MappingConfigured, MappingTransformed, MappingSynthesized, MappingRetainedExtension:
		return union.SemanticFidelityTransformed
	case MappingDegraded, MappingUnobservable:
		return union.SemanticFidelityDegraded
	default:
		return union.SemanticFidelityUnavailable
	}
}
