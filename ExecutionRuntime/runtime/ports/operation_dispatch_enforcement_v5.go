package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const OperationDispatchEnforcementContractVersionV5 = "5.1.0"

type OperationDispatchEnforcementPhaseRefV5 struct {
	OperationDigest       core.Digest                         `json:"operation_digest"`
	EffectID              core.EffectIntentID                 `json:"effect_id"`
	PermitID              string                              `json:"permit_id"`
	PermitFactRevision    core.Revision                       `json:"permit_fact_revision"`
	PermitDigest          core.Digest                         `json:"permit_digest"`
	AdmissionDigest       core.Digest                         `json:"admission_digest"`
	ReviewAuthorization   OperationReviewAuthorizationRefV5   `json:"review_authorization"`
	AuthorizationBasis    OperationReviewAuthorizationBasisV5 `json:"review_authorization_basis"`
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

func (r OperationDispatchEnforcementPhaseRefV5) Validate() error {
	if validateEvidenceIDV2(string(r.EffectID)) != nil || validateEvidenceIDV2(r.PermitID) != nil || r.PermitFactRevision == 0 || validateEvidenceIDV2(r.AttemptID) != nil || r.JournalRevision == 0 || r.ValidatedUnixNano <= 0 || r.ExpiresUnixNano <= r.ValidatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 enforcement phase ref identity is incomplete")
	}
	for _, d := range []core.Digest{r.OperationDigest, r.PermitDigest, r.AdmissionDigest, r.ReceiptDigest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	if err := validateOperationReviewAuthorizationBasisV5(r.AuthorizationBasis); err != nil {
		return err
	}
	if err := r.SandboxAttempt.Validate(); err != nil {
		return err
	}
	switch r.Phase {
	case OperationDispatchEnforcementPrepareV4:
		if r.JournalRevision != 1 || r.PrepareReceiptDigest != "" || r.PreparedAttemptDigest != "" {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 prepare ref carries execute provenance")
		}
	case OperationDispatchEnforcementExecuteV4:
		if r.JournalRevision != 2 || r.PrepareReceiptDigest.Validate() != nil || r.PreparedAttemptDigest.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 execute ref lacks prepare provenance")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 enforcement phase is invalid")
	}
	return nil
}

type OperationDispatchEnforcementPhaseReceiptV5 struct {
	ContractVersion     string                                      `json:"contract_version"`
	Phase               OperationDispatchEnforcementPhaseV4         `json:"phase"`
	Operation           OperationSubjectV3                          `json:"operation"`
	OperationDigest     core.Digest                                 `json:"operation_digest"`
	EffectID            core.EffectIntentID                         `json:"effect_id"`
	IntentRevision      core.Revision                               `json:"intent_revision"`
	IntentDigest        core.Digest                                 `json:"intent_digest"`
	PermitID            string                                      `json:"permit_id"`
	PermitFactRevision  core.Revision                               `json:"permit_fact_revision"`
	PermitDigest        core.Digest                                 `json:"permit_digest"`
	AdmissionDigest     core.Digest                                 `json:"admission_digest"`
	ReviewAuthorization OperationReviewAuthorizationRefV5           `json:"review_authorization"`
	AuthorizationBasis  OperationReviewAuthorizationBasisV5         `json:"review_authorization_basis"`
	AttemptID           string                                      `json:"attempt_id"`
	SandboxAttempt      OperationDispatchSandboxFactRefV4           `json:"sandbox_attempt"`
	Verifier            ProviderBindingRefV2                        `json:"verifier"`
	Sandbox             OperationDispatchSandboxCurrentProjectionV4 `json:"sandbox_current"`
	Prepare             *OperationDispatchEnforcementPhaseRefV5     `json:"prepare,omitempty"`
	PreparedAttempt     *PreparedProviderAttemptRefV2               `json:"prepared_attempt,omitempty"`
	ValidatedUnixNano   int64                                       `json:"validated_unix_nano"`
	ExpiresUnixNano     int64                                       `json:"expires_unix_nano"`
	Digest              core.Digest                                 `json:"digest"`
}

func (r OperationDispatchEnforcementPhaseReceiptV5) Validate() error {
	if r.ContractVersion != OperationDispatchEnforcementContractVersionV5 || r.Operation.Validate() != nil || validateEvidenceIDV2(string(r.EffectID)) != nil || r.IntentRevision == 0 || validateEvidenceIDV2(r.PermitID) != nil || r.PermitFactRevision == 0 || validateEvidenceIDV2(r.AttemptID) != nil || r.ValidatedUnixNano <= 0 || r.ExpiresUnixNano <= r.ValidatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 enforcement receipt identity is incomplete")
	}
	operationDigest, err := r.Operation.DigestV3()
	if err != nil || operationDigest != r.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 enforcement operation digest drifted")
	}
	for _, d := range []core.Digest{r.IntentDigest, r.PermitDigest, r.AdmissionDigest, r.Digest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	if err := validateOperationReviewAuthorizationBasisV5(r.AuthorizationBasis); err != nil {
		return err
	}
	if err := r.SandboxAttempt.Validate(); err != nil {
		return err
	}
	if err := r.Verifier.Validate(); err != nil {
		return err
	}
	if err := r.Sandbox.Validate(); err != nil {
		return err
	}
	if r.Sandbox.Attempt != r.SandboxAttempt || r.Sandbox.AttemptID != r.AttemptID || r.Sandbox.EffectID != r.EffectID || r.Sandbox.IntentRevision != r.IntentRevision || r.Sandbox.IntentDigest != r.IntentDigest || !SameOperationSubjectV3(r.Sandbox.Operation, r.Operation) || r.Sandbox.ProviderBinding != r.Verifier || r.ExpiresUnixNano > r.Sandbox.ExpiresUnixNano || r.ExpiresUnixNano > r.SandboxAttempt.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "V5 enforcement changed Sandbox current facts")
	}
	switch r.Phase {
	case OperationDispatchEnforcementPrepareV4:
		if r.Prepare != nil || r.PreparedAttempt != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 prepare receipt carries execute provenance")
		}
	case OperationDispatchEnforcementExecuteV4:
		if r.Prepare == nil || r.PreparedAttempt == nil || r.Prepare.Validate() != nil || r.PreparedAttempt.Validate() != nil || r.Prepare.Phase != OperationDispatchEnforcementPrepareV4 || r.Prepare.OperationDigest != r.OperationDigest || r.Prepare.EffectID != r.EffectID || r.Prepare.PermitID != r.PermitID || r.Prepare.PermitDigest != r.PermitDigest || r.Prepare.ReviewAuthorization != r.ReviewAuthorization || r.Prepare.AuthorizationBasis != r.AuthorizationBasis || r.Prepare.AttemptID != r.AttemptID || r.PreparedAttempt.OperationDigest != r.OperationDigest || r.PreparedAttempt.IntentID != r.EffectID || r.PreparedAttempt.IntentRevision != r.IntentRevision || r.PreparedAttempt.IntentDigest != r.IntentDigest || r.PreparedAttempt.PermitID != r.PermitID || r.PreparedAttempt.AttemptID != r.AttemptID || r.PreparedAttempt.Provider != r.Verifier || r.ExpiresUnixNano > r.PreparedAttempt.ExpiresUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 execute receipt changed prepare or attempt")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 enforcement phase invalid")
	}
	d, e := r.DigestV5()
	if e != nil || d != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V5 enforcement receipt digest drifted")
	}
	return nil
}
func (r OperationDispatchEnforcementPhaseReceiptV5) DigestV5() (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-enforcement", OperationDispatchEnforcementContractVersionV5, "OperationDispatchEnforcementPhaseReceiptV5", r)
}
func SealOperationDispatchEnforcementPhaseReceiptV5(r OperationDispatchEnforcementPhaseReceiptV5) (OperationDispatchEnforcementPhaseReceiptV5, error) {
	r.ContractVersion = OperationDispatchEnforcementContractVersionV5
	r.Digest = ""
	d, e := r.DigestV5()
	if e != nil {
		return OperationDispatchEnforcementPhaseReceiptV5{}, e
	}
	r.Digest = d
	return r, r.Validate()
}
func (r OperationDispatchEnforcementPhaseReceiptV5) RefV5(journalRevision core.Revision) (OperationDispatchEnforcementPhaseRefV5, error) {
	if err := r.Validate(); err != nil {
		return OperationDispatchEnforcementPhaseRefV5{}, err
	}
	ref := OperationDispatchEnforcementPhaseRefV5{OperationDigest: r.OperationDigest, EffectID: r.EffectID, PermitID: r.PermitID, PermitFactRevision: r.PermitFactRevision, PermitDigest: r.PermitDigest, AdmissionDigest: r.AdmissionDigest, ReviewAuthorization: r.ReviewAuthorization, AuthorizationBasis: r.AuthorizationBasis, AttemptID: r.AttemptID, SandboxAttempt: r.SandboxAttempt, Phase: r.Phase, ReceiptDigest: r.Digest, JournalRevision: journalRevision, ValidatedUnixNano: r.ValidatedUnixNano, ExpiresUnixNano: r.ExpiresUnixNano}
	if r.Prepare != nil {
		ref.PrepareReceiptDigest = r.Prepare.ReceiptDigest
	}
	if r.PreparedAttempt != nil {
		ref.PreparedAttemptDigest = r.PreparedAttempt.Digest
	}
	return ref, ref.Validate()
}

type OperationDispatchEnforcementJournalV5 struct {
	ContractVersion string                                      `json:"contract_version"`
	OperationDigest core.Digest                                 `json:"operation_digest"`
	EffectID        core.EffectIntentID                         `json:"effect_id"`
	PermitID        string                                      `json:"permit_id"`
	AttemptID       string                                      `json:"attempt_id"`
	SandboxAttempt  OperationDispatchSandboxFactRefV4           `json:"sandbox_attempt"`
	Revision        core.Revision                               `json:"revision"`
	Prepare         *OperationDispatchEnforcementPhaseReceiptV5 `json:"prepare,omitempty"`
	Execute         *OperationDispatchEnforcementPhaseReceiptV5 `json:"execute,omitempty"`
	UpdatedUnixNano int64                                       `json:"updated_unix_nano"`
	Digest          core.Digest                                 `json:"digest"`
}

func (j OperationDispatchEnforcementJournalV5) Validate() error {
	if j.ContractVersion != OperationDispatchEnforcementContractVersionV5 || validateEvidenceIDV2(string(j.EffectID)) != nil || validateEvidenceIDV2(j.PermitID) != nil || validateEvidenceIDV2(j.AttemptID) != nil || j.Revision == 0 || j.UpdatedUnixNano <= 0 || j.Prepare == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 enforcement journal is incomplete")
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
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 journal prepare belongs to another dispatch")
	}
	if j.Execute == nil {
		if j.Revision != 1 || j.UpdatedUnixNano != j.Prepare.ValidatedUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "V5 prepare journal revision drifted")
		}
	} else {
		if err := j.Execute.Validate(); err != nil {
			return err
		}
		prepareRef, err := j.Prepare.RefV5(1)
		if err != nil || j.Revision != 2 || j.Execute.Phase != OperationDispatchEnforcementExecuteV4 || j.Execute.Prepare == nil || *j.Execute.Prepare != prepareRef || j.Execute.OperationDigest != j.OperationDigest || j.Execute.EffectID != j.EffectID || j.Execute.PermitID != j.PermitID || j.Execute.AttemptID != j.AttemptID || j.Execute.SandboxAttempt != j.SandboxAttempt || j.UpdatedUnixNano != j.Execute.ValidatedUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "V5 execute journal drifted")
		}
	}
	d, e := j.DigestV5()
	if e != nil || d != j.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V5 enforcement journal digest drifted")
	}
	return nil
}
func (j OperationDispatchEnforcementJournalV5) DigestV5() (core.Digest, error) {
	j.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-enforcement", OperationDispatchEnforcementContractVersionV5, "OperationDispatchEnforcementJournalV5", j)
}
func SealOperationDispatchEnforcementJournalV5(j OperationDispatchEnforcementJournalV5) (OperationDispatchEnforcementJournalV5, error) {
	j.ContractVersion = OperationDispatchEnforcementContractVersionV5
	j.Digest = ""
	d, e := j.DigestV5()
	if e != nil {
		return OperationDispatchEnforcementJournalV5{}, e
	}
	j.Digest = d
	return j, j.Validate()
}
func (j OperationDispatchEnforcementJournalV5) PhaseRefV5(phase OperationDispatchEnforcementPhaseV4) (OperationDispatchEnforcementPhaseRefV5, error) {
	if err := j.Validate(); err != nil {
		return OperationDispatchEnforcementPhaseRefV5{}, err
	}
	switch phase {
	case OperationDispatchEnforcementPrepareV4:
		return j.Prepare.RefV5(1)
	case OperationDispatchEnforcementExecuteV4:
		if j.Execute == nil {
			return OperationDispatchEnforcementPhaseRefV5{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "V5 execute receipt absent")
		}
		return j.Execute.RefV5(2)
	default:
		return OperationDispatchEnforcementPhaseRefV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 phase invalid")
	}
}

type EnforceCurrentOperationDispatchRequestV5 struct {
	Operation                  OperationSubjectV3                      `json:"operation"`
	EffectID                   core.EffectIntentID                     `json:"effect_id"`
	PermitID                   string                                  `json:"permit_id"`
	ExpectedPermitFactRevision core.Revision                           `json:"expected_permit_fact_revision"`
	PermitDigest               core.Digest                             `json:"permit_digest"`
	AdmissionDigest            core.Digest                             `json:"admission_digest"`
	ReviewAuthorization        OperationReviewAuthorizationRefV5       `json:"review_authorization"`
	AuthorizationBasis         OperationReviewAuthorizationBasisV5     `json:"review_authorization_basis"`
	AttemptID                  string                                  `json:"attempt_id"`
	Phase                      OperationDispatchEnforcementPhaseV4     `json:"phase"`
	SandboxAttempt             OperationDispatchSandboxFactRefV4       `json:"sandbox_attempt"`
	SandboxReservation         OperationDispatchSandboxFactRefV4       `json:"sandbox_reservation"`
	SandboxProjectionDigest    core.Digest                             `json:"sandbox_projection_digest"`
	Verifier                   ProviderBindingRefV2                    `json:"verifier"`
	ExpectedJournalRevision    core.Revision                           `json:"expected_journal_revision"`
	Prepare                    *OperationDispatchEnforcementPhaseRefV5 `json:"prepare,omitempty"`
	PreparedAttempt            *PreparedProviderAttemptRefV2           `json:"prepared_attempt,omitempty"`
}

func (r EnforceCurrentOperationDispatchRequestV5) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || validateEvidenceIDV2(r.PermitID) != nil || r.ExpectedPermitFactRevision == 0 || validateEvidenceIDV2(r.AttemptID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 enforcement request identity incomplete")
	}
	for _, d := range []core.Digest{r.PermitDigest, r.AdmissionDigest, r.SandboxProjectionDigest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	if err := validateOperationReviewAuthorizationBasisV5(r.AuthorizationBasis); err != nil {
		return err
	}
	if err := r.SandboxAttempt.Validate(); err != nil {
		return err
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
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 prepare request carries later watermarks")
		}
	case OperationDispatchEnforcementExecuteV4:
		if r.ExpectedJournalRevision != 1 || r.Prepare == nil || r.PreparedAttempt == nil || r.Prepare.Validate() != nil || r.PreparedAttempt.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 execute request lacks prepare watermarks")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 phase invalid")
	}
	return nil
}

type InspectOperationDispatchEnforcementRequestV5 struct {
	Operation OperationSubjectV3                  `json:"operation"`
	EffectID  core.EffectIntentID                 `json:"effect_id"`
	PermitID  string                              `json:"permit_id"`
	Phase     OperationDispatchEnforcementPhaseV4 `json:"phase"`
}

func (r InspectOperationDispatchEnforcementRequestV5) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || validateEvidenceIDV2(r.PermitID) != nil || (r.Phase != OperationDispatchEnforcementPrepareV4 && r.Phase != OperationDispatchEnforcementExecuteV4) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 enforcement Inspect key incomplete")
	}
	return nil
}

type InspectCurrentOperationDispatchEnforcementRequestV5 struct {
	Inspect                 InspectOperationDispatchEnforcementRequestV5 `json:"inspect"`
	PermitDigest            core.Digest                                  `json:"permit_digest"`
	AdmissionDigest         core.Digest                                  `json:"admission_digest"`
	ReviewAuthorization     OperationReviewAuthorizationRefV5            `json:"review_authorization"`
	AuthorizationBasis      OperationReviewAuthorizationBasisV5          `json:"review_authorization_basis"`
	SandboxAttempt          OperationDispatchSandboxFactRefV4            `json:"sandbox_attempt"`
	SandboxProjectionDigest core.Digest                                  `json:"sandbox_projection_digest"`
}

func (r InspectCurrentOperationDispatchEnforcementRequestV5) Validate() error {
	if err := r.Inspect.Validate(); err != nil {
		return err
	}
	for _, d := range []core.Digest{r.PermitDigest, r.AdmissionDigest, r.SandboxProjectionDigest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	if err := validateOperationReviewAuthorizationBasisV5(r.AuthorizationBasis); err != nil {
		return err
	}
	return r.SandboxAttempt.Validate()
}

type CurrentOperationDispatchEnforcementV5 struct {
	Dispatch        CurrentOperationDispatchAuthorizationV5     `json:"dispatch_current"`
	Sandbox         OperationDispatchSandboxCurrentProjectionV4 `json:"sandbox_current"`
	Journal         OperationDispatchEnforcementJournalV5       `json:"journal"`
	Phase           OperationDispatchEnforcementPhaseRefV5      `json:"phase"`
	CheckedUnixNano int64                                       `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                       `json:"expires_unix_nano"`
	Digest          core.Digest                                 `json:"digest"`
}

func (e CurrentOperationDispatchEnforcementV5) Validate() error {
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
	expected, err := e.Journal.PhaseRefV5(e.Phase.Phase)
	if err != nil || expected != e.Phase {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 current phase differs from journal")
	}
	if e.CheckedUnixNano <= 0 || e.ExpiresUnixNano <= e.CheckedUnixNano || e.ExpiresUnixNano > e.Dispatch.ExpiresUnixNano || e.ExpiresUnixNano > e.Sandbox.ExpiresUnixNano || e.Phase.PermitDigest != e.Dispatch.Record.PermitDigest || e.Phase.AdmissionDigest != e.Dispatch.Record.Permit.Admission.Digest || e.Phase.ReviewAuthorization != e.Dispatch.ReviewAuthorization || e.Phase.AuthorizationBasis != e.Dispatch.AuthorizationBasis || e.Phase.SandboxAttempt != e.Sandbox.Attempt {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 current enforcement envelope drifted")
	}
	d, er := e.DigestV5()
	if er != nil || d != e.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V5 current enforcement digest drifted")
	}
	return nil
}
func (e CurrentOperationDispatchEnforcementV5) DigestV5() (core.Digest, error) {
	e.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-enforcement", OperationDispatchEnforcementContractVersionV5, "CurrentOperationDispatchEnforcementV5", e)
}
func SealCurrentOperationDispatchEnforcementV5(e CurrentOperationDispatchEnforcementV5) (CurrentOperationDispatchEnforcementV5, error) {
	e.Digest = ""
	d, er := e.DigestV5()
	if er != nil {
		return CurrentOperationDispatchEnforcementV5{}, er
	}
	e.Digest = d
	return e, e.Validate()
}

type OperationDispatchEnforcementGovernancePortV5 interface {
	EnforceCurrentOperationDispatchV5(context.Context, EnforceCurrentOperationDispatchRequestV5) (CurrentOperationDispatchEnforcementV5, error)
	InspectOperationDispatchEnforcementV5(context.Context, InspectOperationDispatchEnforcementRequestV5) (OperationDispatchEnforcementJournalV5, error)
	InspectCurrentOperationDispatchEnforcementV5(context.Context, InspectCurrentOperationDispatchEnforcementRequestV5) (CurrentOperationDispatchEnforcementV5, error)
}

func minimumEnforcementTimeV5(values ...time.Time) time.Time {
	m := values[0]
	for _, v := range values[1:] {
		if v.Before(m) {
			m = v
		}
	}
	return m
}
