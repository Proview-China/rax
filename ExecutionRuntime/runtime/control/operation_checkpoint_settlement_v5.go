package control

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func BuildOperationCheckpointRestoreSettlementBundleV5(submission ports.OperationCheckpointRestoreSettlementSubmissionV5) (ports.OperationCheckpointRestoreSettlementCommitBundleV5, error) {
	if err := submission.Validate(); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	settlementDigest, err := core.CanonicalJSONDigest("praxis.runtime.operation-settlement-checkpoint-restore", ports.OperationCheckpointRestoreSettlementContractVersionV5, "OperationCheckpointRestoreSettlementV5", submission)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	settlement := ports.OperationCheckpointRestoreSettlementRefV5{ID: submission.ID, Revision: 1, TenantID: submission.Operation.ExecutionScope.Identity.TenantID, EffectID: submission.EffectID, Attempt: submission.CheckpointAttempt, Phase: submission.Phase, OperationDigest: submission.OperationDigest, Digest: settlementDigest}
	associationRef := ports.OperationCheckpointRestoreSettlementAssociationRefV5{ID: submission.ID + "-association", Revision: 1, Settlement: settlement}
	associationRef.Digest, err = core.CanonicalJSONDigest("praxis.runtime.operation-settlement-checkpoint-restore", ports.OperationCheckpointRestoreSettlementContractVersionV5, "OperationCheckpointRestoreSettlementAssociationRefV5", associationRef)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	guardRef := ports.OperationCheckpointRestoreTerminalGuardRefV5{TenantID: settlement.TenantID, EffectID: settlement.EffectID, Revision: 1, Settlement: settlement}
	guardRef.Digest, err = core.CanonicalJSONDigest("praxis.runtime.operation-settlement-checkpoint-restore", ports.OperationCheckpointRestoreSettlementContractVersionV5, "OperationCheckpointRestoreTerminalGuardRefV5", guardRef)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	projectionRef := ports.OperationCheckpointRestoreTerminalProjectionRefV5{ID: submission.ID + "-projection", Revision: 1, Settlement: settlement}
	projectionRef.Digest, err = core.CanonicalJSONDigest("praxis.runtime.operation-settlement-checkpoint-restore", ports.OperationCheckpointRestoreSettlementContractVersionV5, "OperationCheckpointRestoreTerminalProjectionRefV5", projectionRef)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	effectTerminalRef := ports.OperationCheckpointRestoreEffectTerminalRefV5{TenantID: settlement.TenantID, EffectID: settlement.EffectID, PreviousRevision: submission.ExpectedEffectRevision, Revision: submission.ExpectedEffectRevision + 1, OperationDigest: submission.OperationDigest, Settlement: settlement}
	effectTerminalRef.Digest, err = core.CanonicalJSONDigest("praxis.runtime.operation-settlement-checkpoint-restore", ports.OperationCheckpointRestoreSettlementContractVersionV5, "OperationCheckpointRestoreEffectTerminalRefV5", effectTerminalRef)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	bundle := ports.OperationCheckpointRestoreSettlementCommitBundleV5{
		Submission:     submission,
		Settlement:     settlement,
		Association:    ports.OperationCheckpointRestoreSettlementAssociationV5{Ref: associationRef, SubmissionDigest: settlementDigest},
		Guard:          ports.OperationCheckpointRestoreTerminalGuardV5{Ref: guardRef, OperationDigest: submission.OperationDigest},
		Projection:     ports.OperationCheckpointRestoreTerminalProjectionV5{Ref: projectionRef, Association: associationRef, Guard: guardRef, DomainResult: submission.DomainResult},
		EffectTerminal: ports.OperationCheckpointRestoreEffectTerminalV5{Ref: effectTerminalRef, State: "settled", PublishedUnixNano: submission.SettledUnixNano},
	}
	return bundle, bundle.Validate()
}
