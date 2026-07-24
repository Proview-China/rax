package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const OperationDispatchEnforcementContractVersionV4 = "4.1.0"

type OperationDispatchEnforcementPhaseV4 string

const (
	OperationDispatchEnforcementPrepareV4 OperationDispatchEnforcementPhaseV4 = "prepare"
	OperationDispatchEnforcementExecuteV4 OperationDispatchEnforcementPhaseV4 = "execute"
)

type OperationDispatchSandboxFactRefV4 struct {
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r OperationDispatchSandboxFactRefV4) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "sandbox current fact ref is incomplete")
	}
	return r.Digest.Validate()
}

type OperationDispatchRuntimeLeaseBindingV4 struct {
	Ref              OperationDispatchSandboxFactRefV4 `json:"ref"`
	Lease            core.SandboxLeaseRef              `json:"sandbox_lease"`
	Instance         core.InstanceRef                  `json:"instance"`
	FenceEpoch       core.Epoch                        `json:"fence_epoch"`
	ScopeDigest      core.Digest                       `json:"scope_digest"`
	ObservedRevision core.Revision                     `json:"observed_revision"`
}

func (b OperationDispatchRuntimeLeaseBindingV4) Validate() error {
	if err := b.Ref.Validate(); err != nil {
		return err
	}
	if err := b.Lease.Validate(); err != nil {
		return err
	}
	if err := b.Instance.Validate(); err != nil {
		return err
	}
	if b.FenceEpoch == 0 || b.ObservedRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectFenceStale, "runtime lease binding watermarks are incomplete")
	}
	return b.ScopeDigest.Validate()
}

// OperationDispatchSandboxCurrentProjectionV4 is a neutral read-only
// projection supplied by the Sandbox Owner. It is not a Runtime Permit,
// Enforcement receipt, Sandbox Fact, or proof of Provider execution.
type OperationDispatchSandboxCurrentProjectionV4 struct {
	ContractVersion       string                                 `json:"contract_version"`
	Operation             OperationSubjectV3                     `json:"operation"`
	OperationDigest       core.Digest                            `json:"operation_digest"`
	EffectID              core.EffectIntentID                    `json:"effect_id"`
	IntentRevision        core.Revision                          `json:"intent_revision"`
	IntentDigest          core.Digest                            `json:"intent_digest"`
	AttemptID             string                                 `json:"attempt_id"`
	Attempt               OperationDispatchSandboxFactRefV4      `json:"attempt"`
	Reservation           OperationDispatchSandboxFactRefV4      `json:"reservation"`
	SandboxLease          core.SandboxLeaseRef                   `json:"sandbox_lease"`
	RuntimeLease          OperationDispatchRuntimeLeaseBindingV4 `json:"runtime_lease_binding"`
	Generation            GenerationBindingAssociationRefV1      `json:"generation_binding_association"`
	Placement             OperationDispatchSandboxFactRefV4      `json:"placement"`
	Backend               OperationDispatchSandboxFactRefV4      `json:"backend"`
	Slot                  OperationDispatchSandboxFactRefV4      `json:"slot"`
	ProviderBinding       ProviderBindingRefV2                   `json:"provider_binding"`
	ProviderBindingDigest core.Digest                            `json:"provider_binding_digest"`
	Current               bool                                   `json:"current"`
	ProjectionRevision    core.Revision                          `json:"projection_revision"`
	ExpiresUnixNano       int64                                  `json:"expires_unix_nano"`
	ProjectionDigest      core.Digest                            `json:"projection_digest"`
}

func (p OperationDispatchSandboxCurrentProjectionV4) Validate() error {
	if p.ContractVersion != OperationDispatchEnforcementContractVersionV4 || p.ProjectionRevision == 0 || p.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonProviderBindingStale, "sandbox current projection identity and TTL are incomplete")
	}
	if err := p.Operation.Validate(); err != nil {
		return err
	}
	operationDigest, err := p.Operation.DigestV3()
	if err != nil || p.OperationDigest != operationDigest || validateEvidenceIDV2(string(p.EffectID)) != nil || p.IntentRevision == 0 || validateEvidenceIDV2(p.AttemptID) != nil {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "sandbox projection operation, Effect or attempt drifted")
	}
	if err := p.IntentDigest.Validate(); err != nil {
		return err
	}
	for _, ref := range []OperationDispatchSandboxFactRefV4{p.Attempt, p.Reservation, p.Placement, p.Backend, p.Slot} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if p.Attempt.ID != p.AttemptID {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "sandbox Attempt Fact does not bind the attempt identity")
	}
	if err := p.RuntimeLease.Validate(); err != nil {
		return err
	}
	if err := p.Generation.Validate(); err != nil {
		return err
	}
	if err := p.ProviderBinding.Validate(); err != nil {
		return err
	}
	providerDigest, err := OperationDispatchProviderBindingDigestV4(p.ProviderBinding)
	if err != nil || providerDigest != p.ProviderBindingDigest {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "sandbox provider binding digest drifted")
	}
	if p.Operation.ExecutionScope.SandboxLease == nil || *p.Operation.ExecutionScope.SandboxLease != p.SandboxLease || p.RuntimeLease.Lease != p.SandboxLease || p.RuntimeLease.Instance != p.Operation.ExecutionScope.Instance || p.RuntimeLease.ScopeDigest != p.Operation.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "sandbox lease binding does not match the exact operation scope")
	}
	for _, expires := range []int64{p.Attempt.ExpiresUnixNano, p.Reservation.ExpiresUnixNano, p.RuntimeLease.Ref.ExpiresUnixNano, p.Placement.ExpiresUnixNano, p.Backend.ExpiresUnixNano, p.Slot.ExpiresUnixNano} {
		if p.ExpiresUnixNano > expires {
			return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "sandbox projection TTL exceeds a bound fact")
		}
	}
	digest, err := p.DigestV4()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "sandbox current projection digest drifted")
	}
	return nil
}

func (p OperationDispatchSandboxCurrentProjectionV4) DigestV4() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-enforcement", OperationDispatchEnforcementContractVersionV4, "OperationDispatchSandboxCurrentProjectionV4", copy)
}

func SealOperationDispatchSandboxCurrentProjectionV4(p OperationDispatchSandboxCurrentProjectionV4) (OperationDispatchSandboxCurrentProjectionV4, error) {
	p.ContractVersion = OperationDispatchEnforcementContractVersionV4
	providerDigest, err := OperationDispatchProviderBindingDigestV4(p.ProviderBinding)
	if err != nil {
		return OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	p.ProviderBindingDigest = providerDigest
	p.ProjectionDigest = ""
	digest, err := p.DigestV4()
	if err != nil {
		return OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

func OperationDispatchProviderBindingDigestV4(binding ProviderBindingRefV2) (core.Digest, error) {
	if err := binding.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-enforcement", OperationDispatchEnforcementContractVersionV4, "ProviderBindingRefV2", binding)
}

func (p OperationDispatchSandboxCurrentProjectionV4) ValidateCurrent(operation OperationSubjectV3, effectID core.EffectIntentID, intentRevision core.Revision, intentDigest core.Digest, attemptID string, provider ProviderBindingRefV2, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || !p.Current || !SameOperationSubjectV3(p.Operation, operation) || p.EffectID != effectID || p.IntentRevision != intentRevision || p.IntentDigest != intentDigest || p.AttemptID != attemptID || p.ProviderBinding != provider || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "sandbox attempt is not current for the exact dispatch")
	}
	return nil
}

type OperationDispatchSandboxCurrentReaderV4 interface {
	InspectOperationDispatchSandboxCurrentV4(context.Context, OperationSubjectV3, core.EffectIntentID, OperationDispatchSandboxFactRefV4) (OperationDispatchSandboxCurrentProjectionV4, error)
}

type OperationDispatchEnforcementPhaseRefV4 struct {
	OperationDigest       core.Digest                         `json:"operation_digest"`
	EffectID              core.EffectIntentID                 `json:"effect_id"`
	PermitID              string                              `json:"permit_id"`
	PermitFactRevision    core.Revision                       `json:"permit_fact_revision"`
	PermitDigest          core.Digest                         `json:"permit_digest"`
	AdmissionDigest       core.Digest                         `json:"admission_digest"`
	ReviewAuthorization   OperationReviewAuthorizationRefV4   `json:"review_authorization"`
	AttemptID             string                              `json:"attempt_id"`
	SandboxAttempt        OperationDispatchSandboxFactRefV4   `json:"sandbox_attempt"`
	Phase                 OperationDispatchEnforcementPhaseV4 `json:"phase"`
	ReceiptDigest         core.Digest                         `json:"receipt_digest"`
	JournalRevision       core.Revision                       `json:"journal_revision"`
	ValidatedUnixNano     int64                               `json:"validated_unix_nano"`
	ExpiresUnixNano       int64                               `json:"expires_unix_nano"`
	PrepareReceiptDigest  core.Digest                         `json:"prepare_receipt_digest,omitempty"`
	PreparedAttemptDigest core.Digest                         `json:"prepared_attempt_digest,omitempty"`
}

func (r OperationDispatchEnforcementPhaseRefV4) Validate() error {
	if validateEvidenceIDV2(string(r.EffectID)) != nil || validateEvidenceIDV2(r.PermitID) != nil || r.PermitFactRevision == 0 || validateEvidenceIDV2(r.AttemptID) != nil || r.JournalRevision == 0 || r.ValidatedUnixNano <= 0 || r.ExpiresUnixNano <= r.ValidatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "enforcement phase ref identity or TTL is incomplete")
	}
	for _, digest := range []core.Digest{r.OperationDigest, r.PermitDigest, r.AdmissionDigest, r.ReceiptDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	if err := r.SandboxAttempt.Validate(); err != nil {
		return err
	}
	if r.SandboxAttempt.ID != r.AttemptID || r.ExpiresUnixNano > r.SandboxAttempt.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "enforcement ref sandbox Attempt drifted")
	}
	switch r.Phase {
	case OperationDispatchEnforcementPrepareV4:
		if r.JournalRevision != 1 || r.PrepareReceiptDigest != "" || r.PreparedAttemptDigest != "" {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "prepare enforcement ref carries execute provenance")
		}
	case OperationDispatchEnforcementExecuteV4:
		if r.JournalRevision < 2 || r.PrepareReceiptDigest.Validate() != nil || r.PreparedAttemptDigest.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "execute enforcement ref lacks exact prepare provenance")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "enforcement phase is invalid")
	}
	return nil
}

type OperationDispatchEnforcementPhaseReceiptV4 struct {
	ContractVersion     string                                               `json:"contract_version"`
	Phase               OperationDispatchEnforcementPhaseV4                  `json:"phase"`
	Operation           OperationSubjectV3                                   `json:"operation"`
	OperationDigest     core.Digest                                          `json:"operation_digest"`
	EffectID            core.EffectIntentID                                  `json:"effect_id"`
	IntentRevision      core.Revision                                        `json:"intent_revision"`
	IntentDigest        core.Digest                                          `json:"intent_digest"`
	PermitID            string                                               `json:"permit_id"`
	PermitFactRevision  core.Revision                                        `json:"permit_fact_revision"`
	PermitDigest        core.Digest                                          `json:"permit_digest"`
	AdmissionDigest     core.Digest                                          `json:"admission_digest"`
	ReviewAuthorization OperationReviewAuthorizationRefV4                    `json:"review_authorization"`
	AttemptID           string                                               `json:"attempt_id"`
	SandboxAttempt      OperationDispatchSandboxFactRefV4                    `json:"sandbox_attempt"`
	Verifier            ProviderBindingRefV2                                 `json:"verifier"`
	Sandbox             OperationDispatchSandboxCurrentProjectionV4          `json:"sandbox_current"`
	CheckpointSandbox   *CheckpointRestoreDispatchSandboxCurrentProjectionV1 `json:"checkpoint_sandbox_current,omitempty"`
	Prepare             *OperationDispatchEnforcementPhaseRefV4              `json:"prepare,omitempty"`
	PreparedAttempt     *PreparedProviderAttemptRefV2                        `json:"prepared_attempt,omitempty"`
	ValidatedUnixNano   int64                                                `json:"validated_unix_nano"`
	ExpiresUnixNano     int64                                                `json:"expires_unix_nano"`
	Digest              core.Digest                                          `json:"digest"`
}

func (r OperationDispatchEnforcementPhaseReceiptV4) Validate() error {
	if r.ContractVersion != OperationDispatchEnforcementContractVersionV4 || r.IntentRevision == 0 || r.PermitFactRevision == 0 || r.ValidatedUnixNano <= 0 || r.ExpiresUnixNano <= r.ValidatedUnixNano || validateEvidenceIDV2(string(r.EffectID)) != nil || validateEvidenceIDV2(r.PermitID) != nil || validateEvidenceIDV2(r.AttemptID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "enforcement receipt identity or TTL is incomplete")
	}
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	operationDigest, err := r.Operation.DigestV3()
	if err != nil || operationDigest != r.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "enforcement receipt operation digest drifted")
	}
	for _, digest := range []core.Digest{r.IntentDigest, r.PermitDigest, r.AdmissionDigest, r.Digest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	if err := r.SandboxAttempt.Validate(); err != nil {
		return err
	}
	if err := r.Verifier.Validate(); err != nil {
		return err
	}
	if r.CheckpointSandbox == nil {
		if err := r.Sandbox.Validate(); err != nil {
			return err
		}
		if !SameOperationSubjectV3(r.Sandbox.Operation, r.Operation) || r.Sandbox.EffectID != r.EffectID || r.Sandbox.IntentRevision != r.IntentRevision || r.Sandbox.IntentDigest != r.IntentDigest || r.Sandbox.AttemptID != r.AttemptID || r.Sandbox.Attempt != r.SandboxAttempt || r.Sandbox.ProviderBinding != r.Verifier || r.ExpiresUnixNano > r.Sandbox.ExpiresUnixNano || r.ExpiresUnixNano > r.SandboxAttempt.ExpiresUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "enforcement receipt changed sandbox dispatch coordinates")
		}
	} else {
		if r.Sandbox != (OperationDispatchSandboxCurrentProjectionV4{}) {
			return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "checkpoint enforcement receipt also carries an ordinary Sandbox projection")
		}
		checkpoint := r.CheckpointSandbox
		if err := checkpoint.Validate(time.Unix(0, r.ValidatedUnixNano)); err != nil {
			return err
		}
		if !SameOperationSubjectV3(checkpoint.Operation, r.Operation) || checkpoint.EffectID != r.EffectID || checkpoint.IntentRevision != r.IntentRevision || checkpoint.IntentDigest != r.IntentDigest || checkpoint.DispatchAttempt.ID != r.AttemptID || checkpoint.DispatchAttempt != r.SandboxAttempt || checkpoint.Verifier != r.Verifier || r.ExpiresUnixNano > checkpoint.ExpiresUnixNano || r.ExpiresUnixNano > r.SandboxAttempt.ExpiresUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "checkpoint enforcement receipt changed Sandbox dispatch coordinates")
		}
		if r.Phase == OperationDispatchEnforcementPrepareV4 && checkpoint.Stage != CheckpointRestoreDispatchSandboxPrePrepareV1 || r.Phase == OperationDispatchEnforcementExecuteV4 && (checkpoint.Stage != CheckpointRestoreDispatchSandboxPreExecuteV1 || checkpoint.PrepareEnforcement == nil || checkpoint.PreparedAttempt == nil || r.Prepare == nil || r.PreparedAttempt == nil || *checkpoint.PrepareEnforcement != *r.Prepare || *checkpoint.PreparedAttempt != *r.PreparedAttempt) {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "checkpoint enforcement receipt does not bind the actual-point stage")
		}
	}
	switch r.Phase {
	case OperationDispatchEnforcementPrepareV4:
		if r.Prepare != nil || r.PreparedAttempt != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "prepare receipt carries execute provenance")
		}
	case OperationDispatchEnforcementExecuteV4:
		if r.Prepare == nil || r.PreparedAttempt == nil || r.Prepare.Validate() != nil || r.PreparedAttempt.Validate() != nil || r.Prepare.Phase != OperationDispatchEnforcementPrepareV4 || r.Prepare.OperationDigest != r.OperationDigest || r.Prepare.EffectID != r.EffectID || r.Prepare.PermitID != r.PermitID || r.Prepare.PermitDigest != r.PermitDigest || r.Prepare.AttemptID != r.AttemptID || r.PreparedAttempt.OperationDigest != r.OperationDigest || r.PreparedAttempt.IntentID != r.EffectID || r.PreparedAttempt.IntentRevision != r.IntentRevision || r.PreparedAttempt.IntentDigest != r.IntentDigest || r.PreparedAttempt.PermitID != r.PermitID || r.PreparedAttempt.AttemptID != r.AttemptID || r.PreparedAttempt.Provider != r.Verifier {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "execute receipt does not bind exact prepare and prepared attempt")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "enforcement receipt phase is invalid")
	}
	digest, err := r.DigestV4()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "enforcement receipt digest drifted")
	}
	return nil
}

func (r OperationDispatchEnforcementPhaseReceiptV4) DigestV4() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-enforcement", OperationDispatchEnforcementContractVersionV4, "OperationDispatchEnforcementPhaseReceiptV4", copy)
}

func SealOperationDispatchEnforcementPhaseReceiptV4(r OperationDispatchEnforcementPhaseReceiptV4) (OperationDispatchEnforcementPhaseReceiptV4, error) {
	r.ContractVersion = OperationDispatchEnforcementContractVersionV4
	r.Digest = ""
	digest, err := r.DigestV4()
	if err != nil {
		return OperationDispatchEnforcementPhaseReceiptV4{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

func (r OperationDispatchEnforcementPhaseReceiptV4) RefV4(journalRevision core.Revision) (OperationDispatchEnforcementPhaseRefV4, error) {
	if err := r.Validate(); err != nil {
		return OperationDispatchEnforcementPhaseRefV4{}, err
	}
	ref := OperationDispatchEnforcementPhaseRefV4{
		OperationDigest: r.OperationDigest, EffectID: r.EffectID, PermitID: r.PermitID,
		PermitFactRevision: r.PermitFactRevision, PermitDigest: r.PermitDigest, AdmissionDigest: r.AdmissionDigest,
		ReviewAuthorization: r.ReviewAuthorization, AttemptID: r.AttemptID, SandboxAttempt: r.SandboxAttempt, Phase: r.Phase,
		ReceiptDigest: r.Digest, JournalRevision: journalRevision, ValidatedUnixNano: r.ValidatedUnixNano, ExpiresUnixNano: r.ExpiresUnixNano,
	}
	if r.Prepare != nil {
		ref.PrepareReceiptDigest = r.Prepare.ReceiptDigest
	}
	if r.PreparedAttempt != nil {
		ref.PreparedAttemptDigest = r.PreparedAttempt.Digest
	}
	return ref, ref.Validate()
}

type OperationDispatchEnforcementJournalV4 struct {
	ContractVersion string                                      `json:"contract_version"`
	OperationDigest core.Digest                                 `json:"operation_digest"`
	EffectID        core.EffectIntentID                         `json:"effect_id"`
	PermitID        string                                      `json:"permit_id"`
	AttemptID       string                                      `json:"attempt_id"`
	SandboxAttempt  OperationDispatchSandboxFactRefV4           `json:"sandbox_attempt"`
	Revision        core.Revision                               `json:"revision"`
	Prepare         *OperationDispatchEnforcementPhaseReceiptV4 `json:"prepare,omitempty"`
	Execute         *OperationDispatchEnforcementPhaseReceiptV4 `json:"execute,omitempty"`
	UpdatedUnixNano int64                                       `json:"updated_unix_nano"`
	Digest          core.Digest                                 `json:"digest"`
}

func (j OperationDispatchEnforcementJournalV4) Validate() error {
	if j.ContractVersion != OperationDispatchEnforcementContractVersionV4 || validateEvidenceIDV2(string(j.EffectID)) != nil || validateEvidenceIDV2(j.PermitID) != nil || validateEvidenceIDV2(j.AttemptID) != nil || j.Revision == 0 || j.UpdatedUnixNano <= 0 || j.Prepare == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "enforcement journal identity and prepare slot are required")
	}
	if err := j.OperationDigest.Validate(); err != nil {
		return err
	}
	if err := j.SandboxAttempt.Validate(); err != nil {
		return err
	}
	if err := j.Prepare.Validate(); err != nil {
		return err
	}
	if j.Prepare.Phase != OperationDispatchEnforcementPrepareV4 || j.Prepare.OperationDigest != j.OperationDigest || j.Prepare.EffectID != j.EffectID || j.Prepare.PermitID != j.PermitID || j.Prepare.AttemptID != j.AttemptID || j.Prepare.SandboxAttempt != j.SandboxAttempt {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "journal prepare slot belongs to another dispatch")
	}
	if j.Execute == nil {
		if j.Revision != 1 || j.UpdatedUnixNano != j.Prepare.ValidatedUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "prepare-only journal revision drifted")
		}
	} else {
		if err := j.Execute.Validate(); err != nil {
			return err
		}
		prepareRef, err := j.Prepare.RefV4(1)
		if err != nil || j.Revision != 2 || j.Execute.Phase != OperationDispatchEnforcementExecuteV4 || j.Execute.Prepare == nil || *j.Execute.Prepare != prepareRef || j.Execute.OperationDigest != j.OperationDigest || j.Execute.EffectID != j.EffectID || j.Execute.PermitID != j.PermitID || j.Execute.AttemptID != j.AttemptID || j.Execute.SandboxAttempt != j.SandboxAttempt || j.UpdatedUnixNano != j.Execute.ValidatedUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "journal execute slot or prepare provenance drifted")
		}
	}
	digest, err := j.DigestV4()
	if err != nil || digest != j.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "enforcement journal digest drifted")
	}
	return nil
}

// PhaseRefV4 derives the only valid public phase reference from the immutable
// receipt stored in this append-only journal. Callers must never construct a
// phase ref independently from the journal receipt.
func (j OperationDispatchEnforcementJournalV4) PhaseRefV4(phase OperationDispatchEnforcementPhaseV4) (OperationDispatchEnforcementPhaseRefV4, error) {
	if err := j.Validate(); err != nil {
		return OperationDispatchEnforcementPhaseRefV4{}, err
	}
	switch phase {
	case OperationDispatchEnforcementPrepareV4:
		return j.Prepare.RefV4(1)
	case OperationDispatchEnforcementExecuteV4:
		if j.Execute == nil {
			return OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "execute enforcement receipt is not present")
		}
		return j.Execute.RefV4(2)
	default:
		return OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "enforcement phase is invalid")
	}
}

func (j OperationDispatchEnforcementJournalV4) DigestV4() (core.Digest, error) {
	copy := j
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-enforcement", OperationDispatchEnforcementContractVersionV4, "OperationDispatchEnforcementJournalV4", copy)
}

func SealOperationDispatchEnforcementJournalV4(j OperationDispatchEnforcementJournalV4) (OperationDispatchEnforcementJournalV4, error) {
	j.ContractVersion = OperationDispatchEnforcementContractVersionV4
	j.Digest = ""
	digest, err := j.DigestV4()
	if err != nil {
		return OperationDispatchEnforcementJournalV4{}, err
	}
	j.Digest = digest
	return j, j.Validate()
}

type EnforceCurrentOperationDispatchRequestV4 struct {
	Operation                  OperationSubjectV3                      `json:"operation"`
	EffectID                   core.EffectIntentID                     `json:"effect_id"`
	PermitID                   string                                  `json:"permit_id"`
	ExpectedPermitFactRevision core.Revision                           `json:"expected_permit_fact_revision"`
	PermitDigest               core.Digest                             `json:"permit_digest"`
	AdmissionDigest            core.Digest                             `json:"admission_digest"`
	ReviewAuthorization        OperationReviewAuthorizationRefV4       `json:"review_authorization"`
	AttemptID                  string                                  `json:"attempt_id"`
	Phase                      OperationDispatchEnforcementPhaseV4     `json:"phase"`
	SandboxAttempt             OperationDispatchSandboxFactRefV4       `json:"sandbox_attempt"`
	SandboxReservation         OperationDispatchSandboxFactRefV4       `json:"sandbox_reservation"`
	SandboxProjectionDigest    core.Digest                             `json:"sandbox_projection_digest"`
	Verifier                   ProviderBindingRefV2                    `json:"verifier"`
	ExpectedJournalRevision    core.Revision                           `json:"expected_journal_revision"`
	Prepare                    *OperationDispatchEnforcementPhaseRefV4 `json:"prepare,omitempty"`
	PreparedAttempt            *PreparedProviderAttemptRefV2           `json:"prepared_attempt,omitempty"`
}

func (r EnforceCurrentOperationDispatchRequestV4) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || validateEvidenceIDV2(r.PermitID) != nil || r.ExpectedPermitFactRevision == 0 || validateEvidenceIDV2(r.AttemptID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "enforcement request dispatch identity is incomplete")
	}
	for _, digest := range []core.Digest{r.PermitDigest, r.AdmissionDigest, r.SandboxProjectionDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	if err := r.SandboxAttempt.Validate(); err != nil {
		return err
	}
	if r.SandboxAttempt.ID != r.AttemptID {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "enforcement request Attempt Fact identity drifted")
	}
	if err := r.SandboxReservation.Validate(); err != nil {
		return err
	}
	if err := r.Verifier.Validate(); err != nil {
		return err
	}
	switch r.Phase {
	case OperationDispatchEnforcementPrepareV4:
		if r.ExpectedJournalRevision != 0 || r.Prepare != nil || r.PreparedAttempt != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "prepare enforcement request carries later phase watermarks")
		}
	case OperationDispatchEnforcementExecuteV4:
		if r.ExpectedJournalRevision != 1 || r.Prepare == nil || r.PreparedAttempt == nil || r.Prepare.Validate() != nil || r.PreparedAttempt.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "execute enforcement request lacks exact prepare watermarks")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "enforcement request phase is invalid")
	}
	return nil
}

type InspectOperationDispatchEnforcementRequestV4 struct {
	Operation OperationSubjectV3                  `json:"operation"`
	EffectID  core.EffectIntentID                 `json:"effect_id"`
	PermitID  string                              `json:"permit_id"`
	Phase     OperationDispatchEnforcementPhaseV4 `json:"phase"`
}

func (r InspectOperationDispatchEnforcementRequestV4) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || validateEvidenceIDV2(r.PermitID) != nil || (r.Phase != OperationDispatchEnforcementPrepareV4 && r.Phase != OperationDispatchEnforcementExecuteV4) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "enforcement Inspect key is incomplete")
	}
	return nil
}

type InspectCurrentOperationDispatchEnforcementRequestV4 struct {
	Inspect                 InspectOperationDispatchEnforcementRequestV4 `json:"inspect"`
	PermitDigest            core.Digest                                  `json:"permit_digest"`
	AdmissionDigest         core.Digest                                  `json:"admission_digest"`
	ReviewAuthorization     OperationReviewAuthorizationRefV4            `json:"review_authorization"`
	SandboxAttempt          OperationDispatchSandboxFactRefV4            `json:"sandbox_attempt"`
	SandboxProjectionDigest core.Digest                                  `json:"sandbox_projection_digest"`
}

func (r InspectCurrentOperationDispatchEnforcementRequestV4) Validate() error {
	if err := r.Inspect.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{r.PermitDigest, r.AdmissionDigest, r.SandboxProjectionDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := r.SandboxAttempt.Validate(); err != nil {
		return err
	}
	return r.ReviewAuthorization.Validate()
}

type CurrentOperationDispatchEnforcementV4 struct {
	Dispatch        CurrentOperationDispatchAuthorizationV4     `json:"dispatch_current"`
	Sandbox         OperationDispatchSandboxCurrentProjectionV4 `json:"sandbox_current"`
	Journal         OperationDispatchEnforcementJournalV4       `json:"journal"`
	Phase           OperationDispatchEnforcementPhaseRefV4      `json:"phase"`
	CheckedUnixNano int64                                       `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                       `json:"expires_unix_nano"`
	Digest          core.Digest                                 `json:"digest"`
}

func (e CurrentOperationDispatchEnforcementV4) Validate() error {
	if err := e.Dispatch.Validate(); err != nil {
		return err
	}
	if err := e.Sandbox.Validate(); err != nil {
		return err
	}
	if err := e.Journal.Validate(); err != nil {
		return err
	}
	if err := e.Phase.Validate(); err != nil {
		return err
	}
	expectedPhase, err := e.Journal.PhaseRefV4(e.Phase.Phase)
	if err != nil || expectedPhase != e.Phase {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "current enforcement phase ref does not equal the journal receipt ref")
	}
	if e.CheckedUnixNano <= 0 || e.ExpiresUnixNano <= e.CheckedUnixNano || e.ExpiresUnixNano > e.Dispatch.Record.Permit.LegacyPermit.ExpiresUnixNano || e.ExpiresUnixNano > e.Sandbox.ExpiresUnixNano || e.ExpiresUnixNano > e.Sandbox.Attempt.ExpiresUnixNano || e.Phase.PermitDigest != e.Dispatch.Record.PermitDigest || e.Phase.AdmissionDigest != e.Dispatch.Record.Permit.Admission.Digest || e.Phase.ReviewAuthorization != e.Dispatch.ReviewAuthorization || e.Phase.SandboxAttempt != e.Sandbox.Attempt {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "current enforcement envelope drifted")
	}
	digest, err := e.DigestV4()
	if err != nil || digest != e.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "current enforcement envelope digest drifted")
	}
	return nil
}

func (e CurrentOperationDispatchEnforcementV4) DigestV4() (core.Digest, error) {
	copy := e
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-enforcement", OperationDispatchEnforcementContractVersionV4, "CurrentOperationDispatchEnforcementV4", copy)
}

func SealCurrentOperationDispatchEnforcementV4(e CurrentOperationDispatchEnforcementV4) (CurrentOperationDispatchEnforcementV4, error) {
	e.Digest = ""
	digest, err := e.DigestV4()
	if err != nil {
		return CurrentOperationDispatchEnforcementV4{}, err
	}
	e.Digest = digest
	return e, e.Validate()
}

// OperationDispatchEnforcementGovernancePortV4 is the Application-facing
// third gate. It never calls a Provider. A future controlled Runner must use
// InspectCurrent immediately before its phase side effect.
type OperationDispatchEnforcementGovernancePortV4 interface {
	EnforceCurrentOperationDispatchV4(context.Context, EnforceCurrentOperationDispatchRequestV4) (CurrentOperationDispatchEnforcementV4, error)
	InspectOperationDispatchEnforcementV4(context.Context, InspectOperationDispatchEnforcementRequestV4) (OperationDispatchEnforcementJournalV4, error)
	InspectCurrentOperationDispatchEnforcementV4(context.Context, InspectCurrentOperationDispatchEnforcementRequestV4) (CurrentOperationDispatchEnforcementV4, error)
}
