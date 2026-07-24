package contract

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// AttestationNextCaseStateV1 is the Review Owner's sole Resolution-to-Case
// transition mapping. Adapters and callers cannot select a different state.
func AttestationNextCaseStateV1(resolution ResolutionV1) (CaseStateV1, error) {
	switch resolution {
	case ResolutionAcceptV1, ResolutionConditionalV1, ResolutionRejectV1:
		return CaseAttestedV1, nil
	case ResolutionRequestChangesV1:
		return CaseWaitingRevisionV1, nil
	case ResolutionEscalateHumanV1:
		return CaseWaitingHumanV1, nil
	case ResolutionInsufficientEvidenceV1:
		return CaseWaitingEvidenceV1, nil
	default:
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "attestation resolution is unsupported")
	}
}
