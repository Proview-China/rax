package conformance

import (
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

type Level string

const (
	FullyControlled      Level = "fully_controlled"
	RestrictedControlled Level = "restricted_controlled"
	ContainedObserveOnly Level = "contained_observe_only"
	Rejected             Level = "rejected"
)

const (
	CapabilityTimelineProjection    = "continuity/timeline-projection-v1"
	CapabilityObservationProjection = "continuity/evidence-observation-projection-v1"
	CapabilityQueryCursorWatch      = "continuity/query-cursor-watch-v1"
	CapabilityProjectionRebuild     = "continuity/projection-rebuild-v1"
	CapabilityManifestValidation    = "continuity/checkpoint-manifest-validation-v1"
	CapabilityPlanValidation        = "continuity/plan-validation-v1"
	CapabilityContentSPI            = "continuity/content-spi-v1"
	CapabilityJournalRecovery       = "continuity/journal-recovery-v1"
	CapabilityRetentionMetadata     = "continuity/retention-metadata-v1"
	CapabilityMemoryReference       = "continuity/memory-reference-backend-v1"
	CapabilitySettlementApply       = "continuity/settlement-apply-ref-only-v1"
	CapabilityCheckpointManifestV2  = "continuity/checkpoint-manifest-governance-v2"
	CapabilityRestorePlanV2         = "continuity/restore-plan-governance-v2-shape-only"
	CapabilityRewindPlanV2          = "continuity/rewind-plan-governance-v2-shape-only"
	CapabilityArtifactRelationV1    = "continuity/artifact-relation-governance-v1-reference"
	CapabilityContentIntegrityV1    = "continuity/content-integrity-audit-v1-diagnostic"
	CapabilityContentDeltaV1        = "continuity/content-delta-v1-reference"
	CapabilityHistoryDerivationV1   = "continuity/history-derivation-candidate-v1-reference"
)

var supportedCapabilities = []string{
	CapabilityTimelineProjection,
	CapabilityObservationProjection,
	CapabilityQueryCursorWatch,
	CapabilityProjectionRebuild,
	CapabilityManifestValidation,
	CapabilityPlanValidation,
	CapabilityContentSPI,
	CapabilityJournalRecovery,
	CapabilityRetentionMetadata,
	CapabilityMemoryReference,
	CapabilitySettlementApply,
	CapabilityCheckpointManifestV2,
	CapabilityRestorePlanV2,
	CapabilityRewindPlanV2,
	CapabilityArtifactRelationV1,
	CapabilityContentIntegrityV1,
	CapabilityContentDeltaV1,
	CapabilityHistoryDerivationV1,
}

var requiredUnsupported = []string{
	"continuity/checkpoint-capture",
	"continuity/restore-execute",
	"continuity/remote-blob",
	"continuity/physical-purge",
	"continuity/remote-archive",
	"continuity/history-derivation-execute",
	"continuity/production-runtime-root",
	"continuity/harness-adapter",
	"continuity/production-application-root",
	"continuity/model-internal",
}

type Manifest struct {
	ContractVersion string   `json:"contract_version"`
	ComponentID     string   `json:"component_id"`
	Level           Level    `json:"level"`
	ReferenceOnly   bool     `json:"reference_only"`
	ProductionSLA   bool     `json:"production_sla"`
	Supported       []string `json:"supported"`
	Unsupported     []string `json:"unsupported"`
}

func Wave1Manifest() Manifest {
	return Manifest{
		ContractVersion: contract.ContractVersion,
		ComponentID:     "praxis/continuity",
		Level:           RestrictedControlled,
		ReferenceOnly:   true,
		ProductionSLA:   false,
		Supported:       append([]string{}, supportedCapabilities...),
		Unsupported:     append([]string{}, requiredUnsupported...),
	}
}

func Validate(manifest Manifest) error {
	if manifest.ContractVersion != contract.ContractVersion || manifest.ComponentID != "praxis/continuity" {
		return contract.NewError(contract.ErrInvalidArgument, "manifest", "wrong component or contract version")
	}
	if manifest.Level != RestrictedControlled || !manifest.ReferenceOnly || manifest.ProductionSLA {
		return contract.NewError(contract.ErrInvalidArgument, "manifest", "Wave 1 must remain restricted, reference-only, and without production SLA")
	}
	if err := validateExactSet("supported", manifest.Supported, supportedCapabilities); err != nil {
		return err
	}
	return validateExactSet("unsupported", manifest.Unsupported, requiredUnsupported)
}

func validateExactSet(field string, actual, expected []string) error {
	if len(actual) != len(expected) {
		return contract.NewError(contract.ErrInvalidArgument, field, fmt.Sprintf("capability count is %d, expected %d", len(actual), len(expected)))
	}
	want := make(map[string]struct{}, len(expected))
	for _, value := range expected {
		want[value] = struct{}{}
	}
	seen := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		if _, ok := want[value]; !ok {
			return contract.NewError(contract.ErrInvalidArgument, field, fmt.Sprintf("unknown capability %s", value))
		}
		if _, duplicate := seen[value]; duplicate {
			return contract.NewError(contract.ErrInvalidArgument, field, fmt.Sprintf("duplicate capability %s", value))
		}
		seen[value] = struct{}{}
	}
	return nil
}
