package control

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ControlledOperationProviderEntryStateV2 string

const (
	ControlledOperationProviderEntryEnteredV2          ControlledOperationProviderEntryStateV2 = "entered"
	ControlledOperationProviderEntryUnknownV2          ControlledOperationProviderEntryStateV2 = "unknown"
	ControlledOperationProviderEntryObservedV2         ControlledOperationProviderEntryStateV2 = "observed"
	ControlledOperationProviderEntryRejectedNoEffectV2 ControlledOperationProviderEntryStateV2 = "rejected_no_effect"
)

type ControlledOperationProviderEntryFactV2 struct {
	ContractVersion          string                                                    `json:"contract_version"`
	EntryID                  string                                                    `json:"entry_id"`
	Revision                 core.Revision                                             `json:"revision"`
	Digest                   core.Digest                                               `json:"digest"`
	StableKeyDigest          core.Digest                                               `json:"stable_key_digest"`
	State                    ControlledOperationProviderEntryStateV2                   `json:"state"`
	Request                  ports.ControlledOperationProviderRequestV2                `json:"request"`
	UnifiedNotAfterUnixNano  int64                                                     `json:"unified_not_after_unix_nano"`
	FreshEffect              ports.ControlledOperationEffectCurrentProjectionV2        `json:"fresh_effect_current"`
	FreshRoute               ports.ControlledOperationProviderRouteCurrentProjectionV2 `json:"fresh_route_current"`
	FreshBindings            []ports.ProviderBindingCurrentProjectionV2                `json:"fresh_binding_currents"`
	FreshPrepared            ports.ControlledOperationPreparedCurrentProjectionV2      `json:"fresh_prepared_current"`
	FreshEvidencePolicy      ports.OperationScopeEvidencePolicyFactV3                  `json:"fresh_evidence_policy"`
	FreshApplicabilityPolicy ports.OperationScopeEvidenceApplicabilityPolicyFactV3     `json:"fresh_applicability_policy"`
	FreshBoundary            ports.OperationProviderBoundaryCurrentProjectionV1        `json:"fresh_boundary"`
	FreshExecuteEnforcement  ports.OperationDispatchEnforcementPhaseRefV4              `json:"fresh_execute_enforcement"`
	FreshExecuteHandoff      ports.OperationScopeEvidenceProviderHandoffFactV3         `json:"fresh_execute_handoff"`
	FreshQualification       ports.OperationScopeEvidenceQualificationFactV3           `json:"fresh_qualification"`
	AdmissionReceipt         *ports.ControlledOperationProviderAdmissionReceiptRefV2   `json:"admission_receipt,omitempty"`
	Observation              *ports.ProviderAttemptObservationRefV2                    `json:"observation,omitempty"`
	EnteredUnixNano          int64                                                     `json:"entered_unix_nano"`
	UpdatedUnixNano          int64                                                     `json:"updated_unix_nano"`
}

func (f ControlledOperationProviderEntryFactV2) Validate() error {
	if f.ContractVersion != ports.ControlledOperationProviderContractVersionV2 || f.EntryID == "" || f.Revision == 0 || f.Digest.Validate() != nil || f.StableKeyDigest.Validate() != nil || f.Request.Validate() != nil || f.UnifiedNotAfterUnixNano <= 0 || f.EnteredUnixNano <= 0 || f.UpdatedUnixNano < f.EnteredUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "controlled Provider Entry is incomplete")
	}
	stable, entryID, err := DeriveControlledOperationProviderEntryIdentityV2(f.Request)
	if err != nil || stable != f.StableKeyDigest || entryID != f.EntryID {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled Provider Entry identity drifted")
	}
	if f.FreshEffect.Validate(time.Unix(0, f.FreshEffect.CheckedUnixNano)) != nil || f.FreshRoute.Validate() != nil || f.FreshPrepared.Validate() != nil || f.FreshEvidencePolicy.Validate() != nil || f.FreshApplicabilityPolicy.Validate() != nil || f.FreshBoundary.Validate() != nil || f.FreshExecuteEnforcement.Validate() != nil || f.FreshExecuteHandoff.Validate() != nil || f.FreshQualification.Validate() != nil || len(f.FreshBindings) != 7 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "controlled Provider Entry current closure is incomplete")
	}
	entered := time.Unix(0, f.EnteredUnixNano)
	if !entered.Before(time.Unix(0, f.UnifiedNotAfterUnixNano)) || entered.Before(time.Unix(0, f.FreshEffect.CheckedUnixNano)) || !entered.Before(time.Unix(0, f.FreshEffect.ExpiresUnixNano)) || entered.Before(time.Unix(0, f.FreshRoute.CheckedUnixNano)) || !entered.Before(time.Unix(0, f.FreshRoute.ExpiresUnixNano)) || entered.Before(time.Unix(0, f.FreshPrepared.CheckedUnixNano)) || !entered.Before(time.Unix(0, f.FreshPrepared.ExpiresUnixNano)) || !entered.Before(time.Unix(0, f.FreshEvidencePolicy.ExpiresUnixNano)) || !entered.Before(time.Unix(0, f.FreshApplicabilityPolicy.ExpiresUnixNano)) || entered.Before(time.Unix(0, f.FreshBoundary.CheckedUnixNano)) || !entered.Before(time.Unix(0, f.FreshBoundary.ExpiresUnixNano)) || entered.Before(time.Unix(0, f.FreshExecuteEnforcement.ValidatedUnixNano)) || !entered.Before(time.Unix(0, f.FreshExecuteEnforcement.ExpiresUnixNano)) || entered.Before(time.Unix(0, f.FreshExecuteHandoff.CheckedUnixNano)) || !entered.Before(time.Unix(0, f.FreshExecuteHandoff.NotAfterUnixNano)) || entered.Before(time.Unix(0, f.FreshQualification.UpdatedUnixNano)) || !entered.Before(time.Unix(0, f.FreshQualification.ExpiresUnixNano)) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingExpired, "controlled Provider Entry time is outside its historical current closure")
	}
	if f.FreshEffect.IntentDigest != f.Request.IntentDigest || f.FreshEffect.FactRevision != f.Request.EffectRevision || f.FreshRoute.Ref != f.Request.RouteCurrentRef || f.FreshPrepared.Snapshot.SemanticDigest != f.Request.PreparedSemantics.SemanticDigest || f.FreshEvidencePolicy.RefV3() != f.Request.EvidencePolicy || f.FreshApplicabilityPolicy.RefV3() != f.Request.ApplicabilityPolicy || f.FreshBoundary.Ref != f.Request.Boundary || f.FreshExecuteEnforcement != f.Request.ExecuteEnforcement || f.FreshExecuteHandoff.RefV3() != f.Request.ExecuteEvidenceHandoff || f.FreshExecuteHandoff.Qualification != f.FreshQualification.RefV3() {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "controlled Provider Entry current closure drifted")
	}
	expectedBindings := []ports.ProviderBindingRefV2{
		f.FreshRoute.ToolAdapterBinding,
		f.FreshRoute.GatewayBinding,
		f.FreshRoute.ProviderTransportBinding,
		f.FreshRoute.PreparedReaderBinding,
		f.FreshRoute.BoundaryReaderBinding,
		f.FreshRoute.ProviderInspectBinding,
		f.FreshRoute.ProviderBinding,
	}
	for index, binding := range f.FreshBindings {
		if err := binding.ValidateCurrent(expectedBindings[index], time.Unix(0, binding.IssuedUnixNano)); err != nil {
			return err
		}
		if binding.BindingSetDigest != f.FreshRoute.BindingSetDigest || binding.BindingSetSemanticDigest != f.FreshRoute.BindingSetSemanticDigest {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider Entry Binding closure drifted from its route")
		}
		if entered.Before(time.Unix(0, binding.IssuedUnixNano)) || !entered.Before(time.Unix(0, binding.ExpiresUnixNano)) {
			return core.NewError(core.ErrorConflict, core.ReasonBindingExpired, "controlled Provider Entry time is outside a Binding current projection")
		}
		if f.UnifiedNotAfterUnixNano > binding.ExpiresUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonBindingExpired, "controlled Provider Entry exceeds a Binding current lifetime")
		}
	}
	for _, expires := range []int64{f.Request.CallerDeadlineUnixNano, f.FreshEffect.ExpiresUnixNano, f.FreshRoute.ExpiresUnixNano, f.FreshPrepared.ExpiresUnixNano, f.FreshEvidencePolicy.ExpiresUnixNano, f.FreshApplicabilityPolicy.ExpiresUnixNano, f.FreshBoundary.ExpiresUnixNano, f.FreshExecuteEnforcement.ExpiresUnixNano, f.FreshExecuteHandoff.NotAfterUnixNano, f.FreshQualification.IngestNotAfterUnixNano} {
		if f.UnifiedNotAfterUnixNano > expires {
			return core.NewError(core.ErrorConflict, core.ReasonBindingExpired, "controlled Provider Entry exceeds a current fact lifetime")
		}
	}
	switch f.State {
	case ControlledOperationProviderEntryEnteredV2:
		if f.AdmissionReceipt != nil || f.Observation != nil {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "entered Entry carries terminal sidecars")
		}
	case ControlledOperationProviderEntryUnknownV2:
		if f.Observation != nil {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "unknown Entry carries an observation")
		}
	case ControlledOperationProviderEntryObservedV2:
		if f.Observation == nil || f.Observation.Validate() != nil || f.Observation.PreparedAttemptID != f.Request.Prepared.ID {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "observed Entry lacks exact observation")
		}
	case ControlledOperationProviderEntryRejectedNoEffectV2:
		if f.AdmissionReceipt == nil || f.AdmissionReceipt.Validate() != nil || !f.AdmissionReceipt.NoEffect || f.Observation != nil {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "no-effect Entry lacks exact receipt")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "controlled Provider Entry state is invalid")
	}
	digest, err := f.DigestV2()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider Entry digest drifted")
	}
	return nil
}

func (f ControlledOperationProviderEntryFactV2) DigestV2() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	if copy.FreshBindings == nil {
		copy.FreshBindings = []ports.ProviderBindingCurrentProjectionV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider", ports.ControlledOperationProviderContractVersionV2, "ControlledOperationProviderEntryFactV2", copy)
}

func SealControlledOperationProviderEntryFactV2(f ControlledOperationProviderEntryFactV2) (ControlledOperationProviderEntryFactV2, error) {
	f.ContractVersion = ports.ControlledOperationProviderContractVersionV2
	f.Digest = ""
	digest, err := f.DigestV2()
	if err != nil {
		return ControlledOperationProviderEntryFactV2{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func (f ControlledOperationProviderEntryFactV2) RefV2() ports.ControlledOperationProviderEntryRefV2 {
	return ports.ControlledOperationProviderEntryRefV2{EntryID: f.EntryID, Revision: f.Revision, StableKeyDigest: f.StableKeyDigest, Digest: f.Digest}
}

func DeriveControlledOperationProviderEntryIdentityV2(request ports.ControlledOperationProviderRequestV2) (core.Digest, string, error) {
	key, err := ports.DeriveControlledOperationProviderEntryKeyV2(request)
	if err != nil {
		return "", "", err
	}
	return key.StableKeyDigest, key.EntryID, nil
}

// SameControlledOperationProviderEntryImmutableV2 compares only the immutable
// create-once identity. Current closure snapshots, timestamps, state and
// revisions are deliberately excluded because another reconciler may have
// refreshed or advanced the same logical Entry.
func SameControlledOperationProviderEntryImmutableV2(left, right ControlledOperationProviderEntryFactV2) bool {
	if left.Validate() != nil || right.Validate() != nil {
		return false
	}
	leftKey, leftErr := ports.DeriveControlledOperationProviderEntryKeyV2(left.Request)
	rightKey, rightErr := ports.DeriveControlledOperationProviderEntryKeyV2(right.Request)
	return leftErr == nil && rightErr == nil && left.EntryID == leftKey.EntryID && right.EntryID == rightKey.EntryID && leftKey == rightKey && left.StableKeyDigest == leftKey.StableKeyDigest && right.StableKeyDigest == rightKey.StableKeyDigest
}

// IsControlledOperationProviderEntryRecoverySuccessV2 accepts an exact
// persisted target state or one of its monotonic terminal descendants. It
// rejects a predecessor, a sibling terminal state and any immutable drift.
func IsControlledOperationProviderEntryRecoverySuccessV2(expected, stored ControlledOperationProviderEntryFactV2) bool {
	if !SameControlledOperationProviderEntryImmutableV2(expected, stored) || stored.Revision < expected.Revision {
		return false
	}
	switch expected.State {
	case ControlledOperationProviderEntryEnteredV2:
		return stored.State == ControlledOperationProviderEntryEnteredV2 || stored.State == ControlledOperationProviderEntryUnknownV2 || stored.State == ControlledOperationProviderEntryObservedV2 || stored.State == ControlledOperationProviderEntryRejectedNoEffectV2
	case ControlledOperationProviderEntryUnknownV2:
		return stored.State == ControlledOperationProviderEntryUnknownV2 || stored.State == ControlledOperationProviderEntryObservedV2 || stored.State == ControlledOperationProviderEntryRejectedNoEffectV2
	case ControlledOperationProviderEntryObservedV2:
		return stored.State == ControlledOperationProviderEntryObservedV2
	case ControlledOperationProviderEntryRejectedNoEffectV2:
		return stored.State == ControlledOperationProviderEntryRejectedNoEffectV2
	default:
		return false
	}
}

type ControlledOperationProviderEntryCreateDispositionV2 string

const (
	ControlledOperationProviderEntryCreatedV2  ControlledOperationProviderEntryCreateDispositionV2 = "created"
	ControlledOperationProviderEntryExistingV2 ControlledOperationProviderEntryCreateDispositionV2 = "existing"
)

type controlledOperationProviderOpaqueClaimV2 struct{ nonce core.Digest }

type CreateControlledOperationProviderEntryResultV2 struct {
	Fact        ControlledOperationProviderEntryFactV2
	Disposition ControlledOperationProviderEntryCreateDispositionV2
	claim       *controlledOperationProviderOpaqueClaimV2
}

func (r CreateControlledOperationProviderEntryResultV2) HasOpaqueClaimV2() bool {
	return r.Disposition == ControlledOperationProviderEntryCreatedV2 && r.claim != nil
}

func NewCreateControlledOperationProviderEntryResultV2(fact ControlledOperationProviderEntryFactV2, created bool, nonce core.Digest) (CreateControlledOperationProviderEntryResultV2, error) {
	if err := fact.Validate(); err != nil {
		return CreateControlledOperationProviderEntryResultV2{}, err
	}
	if created {
		if nonce.Validate() != nil {
			return CreateControlledOperationProviderEntryResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "opaque Entry claim nonce is invalid")
		}
		return CreateControlledOperationProviderEntryResultV2{Fact: fact, Disposition: ControlledOperationProviderEntryCreatedV2, claim: &controlledOperationProviderOpaqueClaimV2{nonce: nonce}}, nil
	}
	return CreateControlledOperationProviderEntryResultV2{Fact: fact, Disposition: ControlledOperationProviderEntryExistingV2}, nil
}

type ControlledOperationProviderEntryCASRequestV2 struct {
	ExpectedRevision core.Revision                          `json:"expected_revision"`
	Next             ControlledOperationProviderEntryFactV2 `json:"next"`
}

type ControlledOperationProviderEntryFactPortV2 interface {
	CreateControlledOperationProviderEntryV2(context.Context, ControlledOperationProviderEntryFactV2) (CreateControlledOperationProviderEntryResultV2, error)
	InspectControlledOperationProviderEntryV2(context.Context, ports.OperationSubjectV3, string) (ControlledOperationProviderEntryFactV2, error)
	CompareAndSwapControlledOperationProviderEntryV2(context.Context, ports.OperationSubjectV3, ControlledOperationProviderEntryCASRequestV2) (ControlledOperationProviderEntryFactV2, error)
}

func ValidateControlledOperationProviderEntryTransitionV2(current, next ControlledOperationProviderEntryFactV2, now time.Time) error {
	if current.Validate() != nil || next.Validate() != nil || now.IsZero() || next.Revision != current.Revision+1 || next.EntryID != current.EntryID || next.StableKeyDigest != current.StableKeyDigest || next.Request.RequestDigest != current.Request.RequestDigest || next.EnteredUnixNano != current.EnteredUnixNano || next.UpdatedUnixNano != now.UnixNano() || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "controlled Provider Entry transition changed immutable identity or time")
	}
	allowed := current.State == ControlledOperationProviderEntryEnteredV2 && (next.State == ControlledOperationProviderEntryUnknownV2 || next.State == ControlledOperationProviderEntryObservedV2 || next.State == ControlledOperationProviderEntryRejectedNoEffectV2) || current.State == ControlledOperationProviderEntryUnknownV2 && (next.State == ControlledOperationProviderEntryObservedV2 || next.State == ControlledOperationProviderEntryRejectedNoEffectV2)
	if !allowed {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "controlled Provider Entry transition is not monotonic")
	}
	return nil
}
