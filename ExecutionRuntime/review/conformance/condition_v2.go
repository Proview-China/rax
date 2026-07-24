package conformance

import (
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ConditionChainFixtureV2 is a reusable exact-set oracle for Review-owned
// producers and stores. External admissibility/current readers are deliberately
// outside this owner-local suite.
type ConditionChainFixtureV2 struct {
	Attestation  contract.AttestationV1
	Verdict      contract.VerdictV1
	HumanVotes   []contract.HumanAttestationV2
	HumanQuorum  *contract.HumanQuorumDecisionV2
	HumanVerdict *contract.HumanVerdictV2
}

func CheckConditionChainV2(f ConditionChainFixtureV2) error {
	if f.Attestation.ID != "" || f.Verdict.ID != "" {
		if err := f.Attestation.ValidateProductionAutoProvenanceV4(); err != nil {
			return err
		}
		if err := f.Verdict.ValidateProductionConditionsV2(); err != nil {
			return err
		}
		if f.Attestation.ConditionsDigest != f.Verdict.ConditionsDigest || !reflect.DeepEqual(f.Attestation.Conditions, f.Verdict.Conditions) {
			return core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "V1 Attestation/Verdict condition set drifted")
		}
	}
	if f.HumanQuorum == nil && f.HumanVerdict == nil && len(f.HumanVotes) == 0 {
		return nil
	}
	if f.HumanQuorum == nil || f.HumanVerdict == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human condition conformance requires Quorum and Verdict")
	}
	if err := f.HumanQuorum.Validate(); err != nil {
		return err
	}
	if err := f.HumanVerdict.Validate(); err != nil {
		return err
	}
	conditions, digest, err := contract.CanonicalAcceptedConditionsV2(f.HumanVotes, f.HumanQuorum.AcceptedAttestationRefs)
	if err != nil {
		return err
	}
	if digest != f.HumanQuorum.ConditionsDigest || digest != f.HumanVerdict.ConditionsDigest || !reflect.DeepEqual(conditions, f.HumanQuorum.Conditions) || !reflect.DeepEqual(conditions, f.HumanVerdict.Conditions) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "human Attestation/Quorum/Verdict condition set drifted")
	}
	return nil
}
