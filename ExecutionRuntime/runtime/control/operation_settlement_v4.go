package control

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// BuildOperationSettlementCommitBundleV4 deterministically derives the four
// create-once terminal objects committed by the Runtime Effect Owner. It does
// not authorize settlement; the governance Gateway and Fact Owner both re-read
// the authoritative inputs before this bundle may be persisted.
func BuildOperationSettlementCommitBundleV4(submission ports.OperationSettlementSubmissionV4) (ports.OperationSettlementCommitBundleV4, error) {
	if err := submission.Validate(); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	settlement, err := ports.SealOperationSettlementFactV4(ports.OperationSettlementFactV4{Submission: submission})
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	settlementRef := settlement.RefV4()
	associationID, err := operationSettlementObjectIDV4("association", settlementRef)
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	association, err := ports.SealOperationSettlementEvidenceAssociationV4(ports.OperationSettlementEvidenceAssociationV4{
		ID:         associationID,
		Settlement: settlementRef,
		Prepare:    submission.Evidence[0],
		Execute:    submission.Evidence[1],
	})
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	guardID, err := operationSettlementObjectIDV4("guard", struct {
		TenantID core.TenantID       `json:"tenant_id"`
		EffectID core.EffectIntentID `json:"effect_id"`
	}{TenantID: submission.TenantID, EffectID: submission.EffectID})
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	guard, err := ports.SealOperationSettlementTerminalGuardV4(ports.OperationSettlementTerminalGuardV4{
		ID:              guardID,
		TenantID:        submission.TenantID,
		OperationDigest: submission.OperationDigest,
		EffectID:        submission.EffectID,
		Settlement:      settlementRef,
	})
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	projectionID, err := operationSettlementObjectIDV4("projection", settlementRef)
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	projection, err := ports.SealOperationSettlementTerminalProjectionV4(ports.OperationSettlementTerminalProjectionV4{
		ID:              projectionID,
		TenantID:        submission.TenantID,
		OperationDigest: submission.OperationDigest,
		EffectID:        submission.EffectID,
		Settlement:      settlementRef,
		Association:     association.RefV4(),
		Guard:           guard.RefV4(),
		DomainResult:    submission.DomainResult,
	})
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	bundle := ports.OperationSettlementCommitBundleV4{Settlement: settlement, Association: association, Guard: guard, Projection: projection}
	return bundle, bundle.Validate()
}

func operationSettlementObjectIDV4(kind string, value any) (string, error) {
	digest, err := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", ports.OperationSettlementContractVersionV4, "OperationSettlementObjectIDV4/"+kind, value)
	if err != nil {
		return "", err
	}
	return "settlement-" + kind + "-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func OperationSettlementCommitBundleDigestV4(bundle ports.OperationSettlementCommitBundleV4) (core.Digest, error) {
	if err := bundle.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-settlement", ports.OperationSettlementContractVersionV4, "OperationSettlementCommitBundleV4", bundle)
}
