package control

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func ValidateExecutionDelegationTransitionV2(current, next ports.ExecutionDelegationFactV2, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() || next.Revision != current.Revision+1 || next.ID != current.ID || next.BindingSetID != current.BindingSetID || next.BindingSetRevision != current.BindingSetRevision || !ports.SameOperationSubjectV3(next.Operation, current.Operation) || next.HostAdapter != current.HostAdapter || next.DataProvider != current.DataProvider || next.EndpointID != current.EndpointID || next.RuntimeSessionRef != current.RuntimeSessionRef || next.PayloadSchema != current.PayloadSchema || next.PayloadDigest != current.PayloadDigest || next.PayloadRevision != current.PayloadRevision || next.IntentID != current.IntentID || next.IntentRevision != current.IntentRevision || next.IntentDigest != current.IntentDigest || next.ProviderPermitID != current.ProviderPermitID || next.ProviderPermitRevision != current.ProviderPermitRevision || next.ProviderPermitDigest != current.ProviderPermitDigest || next.ProviderAttemptID != current.ProviderAttemptID || next.PreparedAttemptID != current.PreparedAttemptID || next.OperationExpiresUnixNano != current.OperationExpiresUnixNano || next.PermitExpiresUnixNano != current.PermitExpiresUnixNano || next.HostBindingExpiresUnixNano != current.HostBindingExpiresUnixNano || next.ProviderBindingExpiresUnixNano != current.ProviderBindingExpiresUnixNano || next.CreatedUnixNano != current.CreatedUnixNano || next.ExpiresUnixNano != current.ExpiresUnixNano || next.RelayHops == nil || len(next.RelayHops) != len(current.RelayHops) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "execution delegation transition changed immutable identity or skipped a revision")
	}
	for index := range current.RelayHops {
		if current.RelayHops[index] != next.RelayHops[index] {
			return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "execution delegation relay chain is immutable")
		}
	}
	switch current.State {
	case ports.ExecutionDelegationDeclaredV2:
		if next.State != ports.ExecutionDelegationPreparedV2 && next.State != ports.ExecutionDelegationRevokedV2 && next.State != ports.ExecutionDelegationExpiredV2 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "declared delegation may only prepare, revoke or expire")
		}
		if next.State == ports.ExecutionDelegationPreparedV2 {
			currentRef, err := current.RefV2()
			if err != nil || next.Preparation == nil || next.Preparation.Delegation != currentRef || next.Preparation.Prepared.DeclaredDelegation != currentRef || next.Preparation.Prepared.ID != current.PreparedAttemptID || next.Preparation.Prepared.IntentID != current.IntentID || next.Preparation.Prepared.IntentRevision != current.IntentRevision || next.Preparation.Prepared.IntentDigest != current.IntentDigest || next.Preparation.Prepared.PermitID != current.ProviderPermitID || next.Preparation.Prepared.PermitRevision != current.ProviderPermitRevision || next.Preparation.Prepared.PermitDigest != current.ProviderPermitDigest || next.Preparation.Prepared.AttemptID != current.ProviderAttemptID || next.Preparation.Prepared.Provider != current.DataProvider || next.Preparation.Prepared.OperationDigest != operationDigestV3(current.Operation) || next.Preparation.Prepared.PayloadSchema != current.PayloadSchema || next.Preparation.Prepared.PayloadDigest != current.PayloadDigest || next.Preparation.Prepared.PayloadRevision != current.PayloadRevision {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "prepared delegation does not persist its exact declared preparation attestation")
			}
		}
	case ports.ExecutionDelegationPreparedV2:
		if next.State != ports.ExecutionDelegationRevokedV2 && next.State != ports.ExecutionDelegationExpiredV2 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "prepared delegation may only revoke or expire")
		}
		currentPreparationDigest, currentErr := preparationDigestV2(current.Preparation)
		nextPreparationDigest, nextErr := preparationDigestV2(next.Preparation)
		if currentErr != nil || nextErr != nil || currentPreparationDigest != nextPreparationDigest {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "terminal delegation changed its preparation attestation")
		}
	default:
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "terminal delegation is immutable")
	}
	return nil
}

func operationDigestV3(subject ports.OperationSubjectV3) core.Digest {
	digest, _ := subject.DigestV3()
	return digest
}

func preparationDigestV2(attestation *ports.ProviderPreparationAttestationV2) (core.Digest, error) {
	if attestation == nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "preparation attestation is missing")
	}
	if err := attestation.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.execution-governance", ports.ExecutionGovernanceContractVersionV2, "ProviderPreparationAttestationV2", attestation)
}
