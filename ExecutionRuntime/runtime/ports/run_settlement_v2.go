package ports

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	RunSettlementContractVersionV2 = "2.0.0"
	MaxRunSettlementRequirementsV2 = 128
)

type RunSettlementRequirementPhaseV2 string

const (
	RunSettlementPhaseCompletion        RunSettlementRequirementPhaseV2 = "run_completion"
	RunSettlementPhaseTerminationReport RunSettlementRequirementPhaseV2 = "termination_report"
)

type RunSettlementDispositionV2 string

const (
	RunSettlementConfirmedSatisfied   RunSettlementDispositionV2 = "confirmed_satisfied"
	RunSettlementConfirmedFailed      RunSettlementDispositionV2 = "confirmed_failed"
	RunSettlementConfirmedNotApplied  RunSettlementDispositionV2 = "confirmed_not_applied"
	RunSettlementUnknown              RunSettlementDispositionV2 = "unknown"
	RunSettlementOperationNotRequired RunSettlementDispositionV2 = "operation_not_required"
)

type RunUnknownResolutionModeV2 string

const (
	RunUnknownBlock                     RunUnknownResolutionModeV2 = "block"
	RunUnknownTerminalizeIndeterminate  RunUnknownResolutionModeV2 = "terminalize_indeterminate"
	RunUnknownTerminalizeReconciliation RunUnknownResolutionModeV2 = "terminalize_needs_reconciliation"
)

type RunClosedFailureModeV2 string

const (
	RunClosedFailureBlock     RunClosedFailureModeV2 = "block"
	RunClosedFailureReconcile RunClosedFailureModeV2 = "terminalize_needs_reconciliation"
)

type RunExecutionTruthV2 string

const (
	RunExecutionTerminalCompleted RunExecutionTruthV2 = "terminal_completed"
	RunExecutionTerminalCancelled RunExecutionTruthV2 = "terminal_cancelled"
	RunExecutionTerminalFailed    RunExecutionTruthV2 = "terminal_failed"
	RunExecutionConfirmedLost     RunExecutionTruthV2 = "confirmed_lost"
	RunExecutionUnknown           RunExecutionTruthV2 = "unknown"
)

const (
	RunRequirementExecutionTruth      NamespacedNameV2 = "runtime/execution-truth"
	RunRequirementEffects             NamespacedNameV2 = "runtime/effects"
	RunRequirementRemoteContinuations NamespacedNameV2 = "runtime/remote-continuations"
	RunRequirementDomainCommits       NamespacedNameV2 = "runtime/domain-commits"
	RunRequirementBudget              NamespacedNameV2 = "runtime/budget"
	RunRequirementCleanup             NamespacedNameV2 = "runtime/cleanup"
	RunRequirementResidual            NamespacedNameV2 = "runtime/residual"
	RunRequirementProviderRetention   NamespacedNameV2 = "runtime/provider-retention"
	RunRequirementClaimAssociation    NamespacedNameV2 = "runtime/claim-association"
)

type RunSettlementPolicyBindingRefV2 struct {
	Ref            string        `json:"ref"`
	Revision       core.Revision `json:"initial_revision"`
	Digest         core.Digest   `json:"initial_digest"`
	SemanticDigest core.Digest   `json:"semantic_digest"`
}

func (r RunSettlementPolicyBindingRefV2) Validate() error {
	if validateEvidenceIDV2(r.Ref) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "run settlement policy ref and revision are required")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	return r.SemanticDigest.Validate()
}

type RunBindingSetRefV2 struct {
	ID             string        `json:"binding_set_id"`
	Revision       core.Revision `json:"initial_binding_set_revision"`
	Digest         core.Digest   `json:"initial_binding_set_digest"`
	SemanticDigest core.Digest   `json:"binding_set_semantic_digest"`
}

func (r RunBindingSetRefV2) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "run binding set ref and revision are required")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	return r.SemanticDigest.Validate()
}

type RunSettlementPlanRefV2 struct {
	ID       string        `json:"plan_id"`
	Revision core.Revision `json:"plan_revision"`
	Digest   core.Digest   `json:"plan_digest"`
}

func (r RunSettlementPlanRefV2) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "run settlement plan ref and revision are required")
	}
	return r.Digest.Validate()
}

type RunExecutionSubjectV2 struct {
	EndpointID     string                       `json:"endpoint_id"`
	EndpointDigest core.Digest                  `json:"endpoint_digest"`
	SessionRef     string                       `json:"session_ref"`
	Binding        EvidenceProducerBindingRefV2 `json:"binding"`
	SubjectDigest  core.Digest                  `json:"subject_digest"`
}

// DeriveRuntimeExecutionSessionRefV2 returns the Runtime-stable execution
// session identity. Provider-native session identifiers are observations and
// must never replace this value in a persisted Run or settlement Plan.
func DeriveRuntimeExecutionSessionRefV2(endpointID string, runID core.AgentRunID) (string, error) {
	if validateEvidenceIDV2(endpointID) != nil || validateEvidenceIDV2(string(runID)) != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "execution endpoint and Run are required for a stable session")
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "RuntimeExecutionSessionIdentityV2", struct {
		EndpointID string          `json:"endpoint_id"`
		RunID      core.AgentRunID `json:"run_id"`
	}{endpointID, runID})
	if err != nil {
		return "", err
	}
	return "runtime-session:" + string(digest), nil
}

// ConfirmRunStartedRequestV3 binds the pending Run to one fully governed,
// settled execution-start Operation. StartedAt and RunRunning are derived by
// the Runtime owner; Application cannot submit them.
type ConfirmRunStartedRequestV3 struct {
	ExecutionScope      core.ExecutionScope            `json:"execution_scope"`
	RunID               core.AgentRunID                `json:"run_id"`
	ExpectedRunRevision core.Revision                  `json:"expected_run_revision"`
	Operation           OperationSubjectV3             `json:"operation_subject"`
	Attempt             GovernedExecutionAttemptRefsV2 `json:"governed_attempt"`
}

// RunStartConfirmationFactV3 is the immutable proof that one exact governed
// execution-start Operation caused the pending->running Run transition. Its
// identity and digest remain stable while the Run later stops or terminates.
type RunStartConfirmationFactV3 struct {
	ContractVersion      string                         `json:"contract_version"`
	ID                   string                         `json:"id"`
	Revision             core.Revision                  `json:"revision"`
	Digest               core.Digest                    `json:"digest"`
	RunID                core.AgentRunID                `json:"run_id"`
	RunIdentityDigest    core.Digest                    `json:"run_identity_digest"`
	ExecutionScope       core.ExecutionScope            `json:"execution_scope"`
	ExecutionScopeDigest core.Digest                    `json:"execution_scope_digest"`
	OperationDigest      core.Digest                    `json:"operation_digest"`
	Attempt              GovernedExecutionAttemptRefsV2 `json:"governed_attempt"`
	RunRevision          core.Revision                  `json:"running_run_revision"`
	StartedUnixNano      int64                          `json:"started_unix_nano"`
}

func (f RunStartConfirmationFactV3) Validate() error {
	if f.ContractVersion != RunSettlementContractVersionV2 || validateEvidenceIDV2(f.ID) != nil || f.Revision != 1 || f.RunRevision < 2 || f.StartedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunConflict, "run start confirmation identity and watermarks are incomplete")
	}
	if validateEvidenceIDV2(string(f.RunID)) != nil || f.RunIdentityDigest.Validate() != nil || f.ExecutionScopeDigest.Validate() != nil || f.OperationDigest.Validate() != nil || f.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunConflict, "run start confirmation digests are incomplete")
	}
	if err := f.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := ExecutionScopeDigestV2(f.ExecutionScope)
	if err != nil || scopeDigest != f.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "run start confirmation scope digest drifted")
	}
	if err := f.Attempt.ValidatePrepared(); err != nil {
		return err
	}
	if f.Attempt.Settlement == nil || f.Attempt.Observation == nil || f.Attempt.Settlement.Disposition != OperationSettlementAppliedV3 || f.Attempt.Admission.OperationDigest != f.OperationDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "run start confirmation requires one exact applied start attempt")
	}
	digest, err := runStartConfirmationDigestV3(f)
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "run start confirmation digest drifted")
	}
	return nil
}

func SealRunStartConfirmationFactV3(f RunStartConfirmationFactV3) (RunStartConfirmationFactV3, error) {
	f.Digest = EvidenceGenesisDigestV2
	digest, err := runStartConfirmationDigestV3(f)
	if err != nil {
		return RunStartConfirmationFactV3{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func runStartConfirmationDigestV3(f RunStartConfirmationFactV3) (core.Digest, error) {
	f.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.run-start", RunSettlementContractVersionV2, "RunStartConfirmationFactV3", f)
}

type RunStartConfirmationEnvelopeV3 struct {
	Run           core.AgentRunRecord                         `json:"run"`
	Certification RunSettlementPlanCertificationAssociationV3 `json:"plan_certification"`
	Confirmation  RunStartConfirmationFactV3                  `json:"confirmation"`
}

func (e RunStartConfirmationEnvelopeV3) Validate() error {
	if err := e.Run.Validate(); err != nil {
		return err
	}
	if err := e.Confirmation.Validate(); err != nil {
		return err
	}
	if err := e.Certification.Validate(); err != nil {
		return err
	}
	identity, err := RunIdentityDigestV2(e.Run)
	if err != nil || e.Run.ID != e.Confirmation.RunID || identity != e.Confirmation.RunIdentityDigest || !SameExecutionScopeV2(e.Run.Scope, e.Confirmation.ExecutionScope) || e.Run.Revision < e.Confirmation.RunRevision || e.Run.StartedAt.UnixNano() != e.Confirmation.StartedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "run start envelope combines a Run with another start confirmation")
	}
	if e.Certification.RunID != e.Run.ID || e.Certification.RunIdentityDigest != identity || e.Certification.ExecutionScopeDigest != e.Confirmation.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "run start envelope lacks the exact certified Run bundle")
	}
	if e.Run.Status != core.RunRunning && e.Run.Status != core.RunStopping && e.Run.Status != core.RunTerminal {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunConflict, "run start envelope requires an already confirmed Run")
	}
	return nil
}

func (r ConfirmRunStartedRequestV3) Validate() error {
	if err := r.ExecutionScope.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.RunID)) != nil || r.ExpectedRunRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunConflict, "run start confirmation identity is incomplete")
	}
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if r.Operation.Kind != OperationScopeRunV3 || r.Operation.RunID != r.RunID || !SameExecutionScopeV2(r.Operation.ExecutionScope, r.ExecutionScope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunConflict, "run start operation belongs to another Run or scope")
	}
	if err := r.Attempt.ValidatePrepared(); err != nil {
		return err
	}
	if r.Attempt.Settlement == nil || r.Attempt.Observation == nil || r.Attempt.Settlement.Disposition != OperationSettlementAppliedV3 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "run start requires exact applied settlement and provider observation")
	}
	operationDigest, err := r.Operation.DigestV3()
	if err != nil || operationDigest != r.Attempt.Admission.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "run start attempt binds another operation")
	}
	return nil
}

type RunStartGovernancePortV3 interface {
	ConfirmRunStartedV3(context.Context, ConfirmRunStartedRequestV3) (RunStartConfirmationEnvelopeV3, error)
	InspectRunStartV3(context.Context, core.ExecutionScope, core.AgentRunID) (RunStartConfirmationEnvelopeV3, error)
}

func (s RunExecutionSubjectV2) Validate() error {
	if validateEvidenceIDV2(s.EndpointID) != nil || validateEvidenceIDV2(s.SessionRef) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "execution subject endpoint and session are required")
	}
	if err := s.EndpointDigest.Validate(); err != nil {
		return err
	}
	if err := s.Binding.Validate(); err != nil {
		return err
	}
	// The exact Run is verified by RunSettlementPlanFactV2. This structural
	// check prevents provider-native or caller-chosen session strings from
	// silently becoming the stable subject identity.
	if !strings.HasPrefix(s.SessionRef, "runtime-session:sha256:") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "execution subject requires a Runtime-derived stable session")
	}
	digest, err := s.DigestV2()
	if err != nil {
		return err
	}
	if s.SubjectDigest != digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "execution subject digest is not derived from endpoint, session and Binding")
	}
	return nil
}

func (s RunExecutionSubjectV2) DigestV2() (core.Digest, error) {
	if validateEvidenceIDV2(s.EndpointID) != nil || validateEvidenceIDV2(s.SessionRef) != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "execution subject endpoint and session are required")
	}
	if err := s.EndpointDigest.Validate(); err != nil {
		return "", err
	}
	if err := s.Binding.Validate(); err != nil {
		return "", err
	}
	// BindingSetRevision is a renewable governance watermark. The subject is
	// stable across a semantically identical BindingSet lease renewal; closure
	// attempts bind the exact current revision separately.
	s.SubjectDigest = ""
	s.Binding.BindingSetRevision = 0
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "RunExecutionSubjectV2", s)
}

type RunSettlementRequirementV2 struct {
	ID              NamespacedNameV2                `json:"id"`
	Kind            NamespacedNameV2                `json:"kind"`
	Phase           RunSettlementRequirementPhaseV2 `json:"phase"`
	Owner           EvidenceProducerBindingRefV2    `json:"owner"`
	Schema          SchemaRefV2                     `json:"schema"`
	SubjectSelector NamespacedNameV2                `json:"subject_selector"`
	SubjectDigest   core.Digest                     `json:"subject_digest"`
	Policy          RunSettlementPolicyBindingRefV2 `json:"policy"`
	EvidenceTrust   EvidenceTrustClassV2            `json:"evidence_trust"`
	EvidenceKind    NamespacedNameV2                `json:"evidence_kind"`
}

func (r RunSettlementRequirementV2) Validate() error {
	if err := ValidateNamespacedNameV2(r.ID); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(r.Kind); err != nil {
		return err
	}
	if r.Phase != RunSettlementPhaseCompletion && r.Phase != RunSettlementPhaseTerminationReport {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementRequirementInvalid, "run settlement requirement phase is unknown")
	}
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if err := r.Schema.Validate(); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(r.SubjectSelector); err != nil {
		return err
	}
	if err := r.SubjectDigest.Validate(); err != nil {
		return err
	}
	if r.EvidenceTrust != EvidenceTrustAttestation && r.EvidenceTrust != EvidenceTrustAuthoritativeFact {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceTrustInvalid, "settlement participant Evidence requires attestation or authoritative-fact trust")
	}
	if err := ValidateNamespacedNameV2(r.EvidenceKind); err != nil {
		return err
	}
	return r.Policy.Validate()
}

func RunSettlementEvidenceCorrelationIDV2(planID string, runID core.AgentRunID, requirementID NamespacedNameV2, subject core.Digest) (string, error) {
	if validateEvidenceIDV2(planID) != nil || validateEvidenceIDV2(string(runID)) != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "settlement Evidence correlation requires Plan and Run identity")
	}
	if err := ValidateNamespacedNameV2(requirementID); err != nil {
		return "", err
	}
	if err := subject.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "RunSettlementEvidenceCorrelationV2", struct {
		PlanID        string           `json:"plan_id"`
		RunID         core.AgentRunID  `json:"run_id"`
		RequirementID NamespacedNameV2 `json:"requirement_id"`
		SubjectDigest core.Digest      `json:"subject_digest"`
	}{planID, runID, requirementID, subject})
	if err != nil {
		return "", err
	}
	return "settlement:" + string(digest), nil
}

// RunSettlementEvidenceCausationEventIDV2 binds an attestation to one exact
// participant Fact revision. It prevents another operation under the same Run
// correlation from being replayed as this requirement's evidence.
func RunSettlementEvidenceCausationEventIDV2(planID string, runID core.AgentRunID, requirementID NamespacedNameV2, participantID string, participantRevision core.Revision) (string, error) {
	if validateEvidenceIDV2(planID) != nil || validateEvidenceIDV2(string(runID)) != nil || validateEvidenceIDV2(participantID) != nil || participantRevision == 0 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "settlement Evidence causation requires exact Plan, Run and participant Fact identity")
	}
	if err := ValidateNamespacedNameV2(requirementID); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "RunSettlementEvidenceCausationV2", struct {
		PlanID              string           `json:"plan_id"`
		RunID               core.AgentRunID  `json:"run_id"`
		RequirementID       NamespacedNameV2 `json:"requirement_id"`
		ParticipantID       string           `json:"participant_id"`
		ParticipantRevision core.Revision    `json:"participant_revision"`
	}{planID, runID, requirementID, participantID, participantRevision})
	if err != nil {
		return "", err
	}
	return "settlement-cause:" + string(digest), nil
}

func (r RunSettlementRequirementV2) DigestV2() (core.Digest, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "RunSettlementRequirementV2", r)
}

type RunClaimModeV2 string

const (
	RunClaimRequiredV2         RunClaimModeV2 = "required"
	RunClaimOptionalByPolicyV2 RunClaimModeV2 = "optional_by_policy"
)

type RunClaimRequirementV2 struct {
	Mode           RunClaimModeV2                   `json:"mode"`
	OmissionPolicy *RunSettlementPolicyBindingRefV2 `json:"omission_policy,omitempty"`
}

func (r RunClaimRequirementV2) Validate() error {
	if r.Mode == RunClaimRequiredV2 {
		if r.OmissionPolicy != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "required Claim cannot carry an omission policy")
		}
		return nil
	}
	if r.Mode != RunClaimOptionalByPolicyV2 || r.OmissionPolicy == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "claim mode is invalid")
	}
	return r.OmissionPolicy.Validate()
}

type RunSettlementPlanFactV2 struct {
	ContractVersion      string                       `json:"contract_version"`
	ID                   string                       `json:"id"`
	Revision             core.Revision                `json:"revision"`
	RunID                core.AgentRunID              `json:"run_id"`
	RunIdentityDigest    core.Digest                  `json:"run_identity_digest"`
	ExecutionScope       core.ExecutionScope          `json:"execution_scope"`
	ExecutionScopeDigest core.Digest                  `json:"execution_scope_digest"`
	SessionRef           string                       `json:"session_ref"`
	LineagePlanDigest    core.Digest                  `json:"lineage_plan_digest"`
	BindingSet           RunBindingSetRefV2           `json:"binding_set"`
	Execution            RunExecutionSubjectV2        `json:"execution"`
	Claim                RunClaimRequirementV2        `json:"claim"`
	Requirements         []RunSettlementRequirementV2 `json:"requirements"`
	CreatedUnixNano      int64                        `json:"created_unix_nano"`
}

func (f RunSettlementPlanFactV2) Validate() error {
	if f.ContractVersion != RunSettlementContractVersionV2 || validateEvidenceIDV2(f.ID) != nil || validateEvidenceIDV2(string(f.RunID)) != nil || f.Revision != 1 || f.CreatedUnixNano <= 0 || validateEvidenceIDV2(f.SessionRef) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "create-once run settlement plan identity is incomplete")
	}
	if err := f.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := ExecutionScopeDigestV2(f.ExecutionScope)
	if err != nil {
		return err
	}
	if f.ExecutionScopeDigest != scopeDigest || f.LineagePlanDigest != f.ExecutionScope.Lineage.PlanDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "run settlement plan scope or lineage drifted")
	}
	if err := f.RunIdentityDigest.Validate(); err != nil {
		return err
	}
	if err := f.BindingSet.Validate(); err != nil {
		return err
	}
	if err := f.Execution.Validate(); err != nil {
		return err
	}
	if f.Execution.SessionRef != f.SessionRef {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "execution subject session differs from run plan")
	}
	expectedSession, err := DeriveRuntimeExecutionSessionRefV2(f.Execution.EndpointID, f.RunID)
	if err != nil || f.SessionRef != expectedSession {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "run Plan session is not derived from endpoint and Run identity")
	}
	if err := f.Claim.Validate(); err != nil {
		return err
	}
	if len(f.Requirements) == 0 || len(f.Requirements) > MaxRunSettlementRequirementsV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "run settlement requirements are empty or exceed the bound")
	}
	previous := ""
	kinds := make(map[NamespacedNameV2]uint32, len(f.Requirements))
	ids := make(map[NamespacedNameV2]struct{}, len(f.Requirements))
	for index, requirement := range f.Requirements {
		if err := requirement.Validate(); err != nil {
			return err
		}
		key := string(requirement.Phase) + "\x00" + string(requirement.ID)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "run settlement requirements must be sorted and unique")
		}
		previous = key
		if _, exists := ids[requirement.ID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonRunSettlementRequirementInvalid, "run settlement requirement ID is duplicated")
		}
		ids[requirement.ID] = struct{}{}
		kinds[requirement.Kind]++
	}
	requiredKinds := map[NamespacedNameV2]RunSettlementRequirementPhaseV2{
		RunRequirementExecutionTruth:      RunSettlementPhaseCompletion,
		RunRequirementEffects:             RunSettlementPhaseCompletion,
		RunRequirementRemoteContinuations: RunSettlementPhaseCompletion,
		RunRequirementDomainCommits:       RunSettlementPhaseCompletion,
		RunRequirementBudget:              RunSettlementPhaseCompletion,
		RunRequirementCleanup:             RunSettlementPhaseTerminationReport,
		RunRequirementResidual:            RunSettlementPhaseTerminationReport,
		RunRequirementProviderRetention:   RunSettlementPhaseTerminationReport,
	}
	for kind, phase := range requiredKinds {
		if kinds[kind] != 1 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "each reserved Runtime governance category must appear exactly once")
		}
		for _, requirement := range f.Requirements {
			if requirement.Kind == kind && requirement.Phase != phase {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "reserved Runtime governance category uses the wrong barrier phase")
			}
		}
	}
	executionRequirementFound := false
	for _, requirement := range f.Requirements {
		if requirement.Kind == RunRequirementExecutionTruth {
			if requirement.Owner != f.Execution.Binding || requirement.SubjectDigest != f.Execution.SubjectDigest {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "execution truth requirement must bind the Plan execution subject and owner")
			}
			executionRequirementFound = true
		}
	}
	if !executionRequirementFound {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "Plan lacks execution truth requirement")
	}
	return nil
}

func (f RunSettlementPlanFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	copy := f
	if copy.Requirements == nil {
		copy.Requirements = []RunSettlementRequirementV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "RunSettlementPlanFactV2", copy)
}

func (f RunSettlementPlanFactV2) RefV2() (RunSettlementPlanRefV2, error) {
	digest, err := f.DigestV2()
	if err != nil {
		return RunSettlementPlanRefV2{}, err
	}
	return RunSettlementPlanRefV2{ID: f.ID, Revision: f.Revision, Digest: digest}, nil
}

type RunSettlementPolicyStateV2 string

const (
	RunSettlementPolicyActive  RunSettlementPolicyStateV2 = "active"
	RunSettlementPolicyRevoked RunSettlementPolicyStateV2 = "revoked"
	RunSettlementPolicyExpired RunSettlementPolicyStateV2 = "expired"
)

type RunSettlementPolicyFactV2 struct {
	Ref                       string                       `json:"ref"`
	Digest                    core.Digest                  `json:"digest"`
	Revision                  core.Revision                `json:"revision"`
	RunID                     core.AgentRunID              `json:"run_id"`
	PlanID                    string                       `json:"plan_id"`
	PlanRevision              core.Revision                `json:"plan_revision"`
	RequirementID             NamespacedNameV2             `json:"requirement_id"`
	ExecutionScopeDigest      core.Digest                  `json:"execution_scope_digest"`
	ExecutionScope            core.ExecutionScope          `json:"execution_scope"`
	ActionScopeDigest         core.Digest                  `json:"action_scope_digest"`
	PolicyOwner               EvidenceProducerBindingRefV2 `json:"policy_owner"`
	PolicyAuthority           AuthorityBindingRefV2        `json:"policy_authority"`
	SemanticDigest            core.Digest                  `json:"semantic_digest"`
	UnknownMode               RunUnknownResolutionModeV2   `json:"unknown_mode"`
	FailureMode               RunClosedFailureModeV2       `json:"failure_mode"`
	NotAppliedMode            RunClosedFailureModeV2       `json:"not_applied_mode"`
	AllowOperationNotRequired bool                         `json:"allow_operation_not_required"`
	AllowSelfPolicy           bool                         `json:"allow_self_policy"`
	AllowMissingClaim         bool                         `json:"allow_missing_claim"`
	AllowConfirmedLost        bool                         `json:"allow_confirmed_lost"`
	State                     RunSettlementPolicyStateV2   `json:"state"`
	ExpiresUnixNano           int64                        `json:"expires_unix_nano"`
}

func (f RunSettlementPolicyFactV2) Validate() error {
	if err := validateRunSettlementPolicyStructureV2(f); err != nil {
		return err
	}
	semantic, err := runSettlementPolicySemanticDigestV2(f)
	if err != nil || f.SemanticDigest != semantic {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "settlement policy semantic digest is missing or drifted")
	}
	digest, err := runSettlementPolicyDigestV2(f)
	if err != nil || f.Digest != digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "settlement policy fact digest is missing or drifted")
	}
	return nil
}

func validateRunSettlementPolicyStructureV2(f RunSettlementPolicyFactV2) error {
	if validateEvidenceIDV2(f.Ref) != nil || validateEvidenceIDV2(string(f.RunID)) != nil || validateEvidenceIDV2(f.PlanID) != nil || f.Revision == 0 || f.PlanRevision == 0 || f.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementRequirementInvalid, "run settlement policy identity and TTL are required")
	}
	if err := ValidateNamespacedNameV2(f.RequirementID); err != nil {
		return err
	}
	if err := f.ExecutionScopeDigest.Validate(); err != nil {
		return err
	}
	if err := f.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := ExecutionScopeDigestV2(f.ExecutionScope)
	if err != nil || scopeDigest != f.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "settlement policy scope digest drifted")
	}
	if err := f.ActionScopeDigest.Validate(); err != nil {
		return err
	}
	if err := f.PolicyOwner.Validate(); err != nil {
		return err
	}
	if err := f.PolicyAuthority.Validate(); err != nil {
		return err
	}
	if f.UnknownMode != RunUnknownBlock && f.UnknownMode != RunUnknownTerminalizeIndeterminate && f.UnknownMode != RunUnknownTerminalizeReconciliation {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementRequirementInvalid, "run settlement unknown policy is invalid")
	}
	if (f.FailureMode != RunClosedFailureBlock && f.FailureMode != RunClosedFailureReconcile) || (f.NotAppliedMode != RunClosedFailureBlock && f.NotAppliedMode != RunClosedFailureReconcile) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementRequirementInvalid, "settlement failure and not-applied modes are invalid")
	}
	if f.State != RunSettlementPolicyActive && f.State != RunSettlementPolicyRevoked && f.State != RunSettlementPolicyExpired {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "run settlement policy state is invalid")
	}
	if err := f.Digest.Validate(); err != nil {
		return err
	}
	if err := f.SemanticDigest.Validate(); err != nil {
		return err
	}
	return nil
}

func (f RunSettlementPolicyFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return f.Digest, nil
}

func (f RunSettlementPolicyFactV2) SemanticDigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return f.SemanticDigest, nil
}

func SealRunSettlementPolicyFactV2(f RunSettlementPolicyFactV2) (RunSettlementPolicyFactV2, error) {
	// Valid placeholder digests allow the structural validator to enforce all
	// non-derived fields before either derived digest is calculated.
	f.Digest, f.SemanticDigest = EvidenceGenesisDigestV2, EvidenceGenesisDigestV2
	if err := validateRunSettlementPolicyStructureV2(f); err != nil {
		return RunSettlementPolicyFactV2{}, err
	}
	semantic, err := runSettlementPolicySemanticDigestV2(f)
	if err != nil {
		return RunSettlementPolicyFactV2{}, err
	}
	f.SemanticDigest = semantic
	digest, err := runSettlementPolicyDigestV2(f)
	if err != nil {
		return RunSettlementPolicyFactV2{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func runSettlementPolicySemanticDigestV2(f RunSettlementPolicyFactV2) (core.Digest, error) {
	f.Digest, f.SemanticDigest = "", ""
	f.Revision, f.ExpiresUnixNano = 0, 0
	f.State = ""
	f.PolicyOwner.BindingSetRevision = 0
	f.PolicyAuthority.Digest, f.PolicyAuthority.Revision = "", 0
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "RunSettlementPolicySemanticV2", f)
}

func runSettlementPolicyDigestV2(f RunSettlementPolicyFactV2) (core.Digest, error) {
	f.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "RunSettlementPolicyFactV2", f)
}

func (f RunSettlementPolicyFactV2) ValidateCurrent(expected RunSettlementPolicyBindingRefV2, plan RunSettlementPlanFactV2, requirement NamespacedNameV2, now time.Time) error {
	digest, err := f.DigestV2()
	if err != nil {
		return err
	}
	semantic, semanticErr := f.SemanticDigestV2()
	if semanticErr != nil || f.SemanticDigest != semantic || f.Ref != expected.Ref || f.Revision < expected.Revision || f.Digest != digest || semantic != expected.SemanticDigest || f.RunID != plan.RunID || f.PlanID != plan.ID || f.PlanRevision != plan.Revision || f.RequirementID != requirement || f.ExecutionScopeDigest != plan.ExecutionScopeDigest || !SameExecutionScopeV2(f.ExecutionScope, plan.ExecutionScope) || f.State != RunSettlementPolicyActive || now.IsZero() || !now.Before(time.Unix(0, f.ExpiresUnixNano)) || f.Revision == expected.Revision && f.Digest != expected.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "run settlement policy is stale or mismatched")
	}
	return nil
}

func requiredPhaseForKindV2(kind NamespacedNameV2) RunSettlementRequirementPhaseV2 {
	switch kind {
	case RunRequirementCleanup, RunRequirementResidual, RunRequirementProviderRetention:
		return RunSettlementPhaseTerminationReport
	default:
		return RunSettlementPhaseCompletion
	}
}

type RunSettlementPolicyReaderV2 interface {
	InspectRunSettlementPolicy(context.Context, string) (RunSettlementPolicyFactV2, error)
}

type RunSettlementParticipantRefV2 struct {
	ID            string                          `json:"id"`
	Revision      core.Revision                   `json:"revision"`
	Digest        core.Digest                     `json:"digest"`
	RequirementID NamespacedNameV2                `json:"requirement_id"`
	Disposition   RunSettlementDispositionV2      `json:"disposition"`
	Policy        RunSettlementPolicyBindingRefV2 `json:"policy,omitempty"`
}

func (r RunSettlementParticipantRefV2) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "run settlement participant ref is incomplete")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(r.RequirementID); err != nil {
		return err
	}
	if !validRunSettlementDispositionV2(r.Disposition) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementParticipantMissing, "participant ref disposition is invalid")
	}
	if r.Disposition == RunSettlementOperationNotRequired {
		if r.Policy.Validate() != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "not-required participant ref requires its exact policy")
		}
	} else if r.Policy != (RunSettlementPolicyBindingRefV2{}) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "ordinary participant ref cannot carry a not-required policy")
	}
	return nil
}

type RunSettlementParticipantFactV2 struct {
	ContractVersion      string                           `json:"contract_version"`
	ID                   string                           `json:"id"`
	Revision             core.Revision                    `json:"revision"`
	RunID                core.AgentRunID                  `json:"run_id"`
	RunIdentityDigest    core.Digest                      `json:"run_identity_digest"`
	ExecutionScope       core.ExecutionScope              `json:"execution_scope"`
	ExecutionScopeDigest core.Digest                      `json:"execution_scope_digest"`
	Plan                 RunSettlementPlanRefV2           `json:"plan"`
	RequirementID        NamespacedNameV2                 `json:"requirement_id"`
	RequirementDigest    core.Digest                      `json:"requirement_digest"`
	SubjectDigest        core.Digest                      `json:"subject_digest"`
	Owner                EvidenceProducerBindingRefV2     `json:"owner"`
	Disposition          RunSettlementDispositionV2       `json:"disposition"`
	Policy               *RunSettlementPolicyBindingRefV2 `json:"policy,omitempty"`
	Evidence             []EvidenceRecordRefV2            `json:"evidence"`
	Payload              *OpaquePayloadV2                 `json:"payload,omitempty"`
	CreatedUnixNano      int64                            `json:"created_unix_nano"`
	ExpiresUnixNano      int64                            `json:"expires_unix_nano"`
}

func (f RunSettlementParticipantFactV2) Validate() error {
	if f.ContractVersion != RunSettlementContractVersionV2 || validateEvidenceIDV2(f.ID) != nil || validateEvidenceIDV2(string(f.RunID)) != nil || f.Revision == 0 || f.CreatedUnixNano <= 0 || f.ExpiresUnixNano <= f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementParticipantMissing, "run settlement participant identity and lifetime are incomplete")
	}
	if err := f.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := ExecutionScopeDigestV2(f.ExecutionScope)
	if err != nil || scopeDigest != f.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementParticipantStale, "participant execution scope drifted")
	}
	for _, digest := range []core.Digest{f.RunIdentityDigest, f.ExecutionScopeDigest, f.RequirementDigest, f.SubjectDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := f.Plan.Validate(); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(f.RequirementID); err != nil {
		return err
	}
	if err := f.Owner.Validate(); err != nil {
		return err
	}
	if !validRunSettlementDispositionV2(f.Disposition) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementParticipantMissing, "participant disposition is invalid")
	}
	if f.Disposition == RunSettlementOperationNotRequired {
		if f.Policy == nil || f.Policy.Validate() != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "operation_not_required requires exact policy")
		}
	} else if f.Policy != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "ordinary participant disposition cannot carry not-required policy")
	}
	if f.Disposition != RunSettlementOperationNotRequired && len(f.Evidence) == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "participant disposition requires at least one exact Evidence record")
	}
	previous := ""
	for index, ref := range f.Evidence {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := string(ref.LedgerScopeDigest) + "\x00" + strconv.FormatUint(ref.Sequence, 10) + "\x00" + string(ref.RecordDigest)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "participant evidence refs must be sorted and unique")
		}
		previous = key
	}
	if f.Payload != nil {
		if err := f.Payload.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (f RunSettlementParticipantFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	if f.Evidence == nil {
		f.Evidence = []EvidenceRecordRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "RunSettlementParticipantFactV2", f)
}

func (f RunSettlementParticipantFactV2) RefV2() (RunSettlementParticipantRefV2, error) {
	digest, err := f.DigestV2()
	if err != nil {
		return RunSettlementParticipantRefV2{}, err
	}
	var policy RunSettlementPolicyBindingRefV2
	if f.Policy != nil {
		policy = *f.Policy
	}
	return RunSettlementParticipantRefV2{ID: f.ID, Revision: f.Revision, Digest: digest, RequirementID: f.RequirementID, Disposition: f.Disposition, Policy: policy}, nil
}

type RunSettlementParticipantInspectRequestV2 struct {
	RunID                core.AgentRunID              `json:"run_id"`
	RunIdentityDigest    core.Digest                  `json:"run_identity_digest"`
	ExecutionScope       core.ExecutionScope          `json:"execution_scope"`
	ExecutionScopeDigest core.Digest                  `json:"execution_scope_digest"`
	Plan                 RunSettlementPlanRefV2       `json:"plan"`
	RequirementID        NamespacedNameV2             `json:"requirement_id"`
	RequirementDigest    core.Digest                  `json:"requirement_digest"`
	SubjectDigest        core.Digest                  `json:"subject_digest"`
	Owner                EvidenceProducerBindingRefV2 `json:"owner"`
}

type RunSettlementParticipantPortV2 interface {
	InspectRunSettlementParticipant(context.Context, RunSettlementParticipantInspectRequestV2) (RunSettlementParticipantFactV2, error)
}

type RunExecutionInspectionRefV2 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r RunExecutionInspectionRefV2) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonExecutionInspectionInvalid, "execution inspection ref is incomplete")
	}
	return r.Digest.Validate()
}

type RunExecutionInspectionRequestV2 struct {
	RunID               core.AgentRunID       `json:"run_id"`
	RunIdentityDigest   core.Digest           `json:"run_identity_digest"`
	ExpectedRunRevision core.Revision         `json:"expected_run_revision"`
	ExecutionScope      core.ExecutionScope   `json:"execution_scope"`
	Subject             RunExecutionSubjectV2 `json:"subject"`
}

type ExecutionSettlementInspectionV2 struct {
	ContractVersion      string                `json:"contract_version"`
	ID                   string                `json:"id"`
	Revision             core.Revision         `json:"revision"`
	RunID                core.AgentRunID       `json:"run_id"`
	RunIdentityDigest    core.Digest           `json:"run_identity_digest"`
	RunRevision          core.Revision         `json:"run_revision"`
	ExecutionScope       core.ExecutionScope   `json:"execution_scope"`
	ExecutionScopeDigest core.Digest           `json:"execution_scope_digest"`
	Subject              RunExecutionSubjectV2 `json:"subject"`
	Truth                RunExecutionTruthV2   `json:"truth"`
	SourceEpoch          core.Epoch            `json:"source_epoch"`
	SourceSequence       uint64                `json:"source_sequence"`
	PayloadDigest        core.Digest           `json:"payload_digest"`
	Evidence             EvidenceRecordRefV2   `json:"evidence"`
	InspectedUnixNano    int64                 `json:"inspected_unix_nano"`
	ExpiresUnixNano      int64                 `json:"expires_unix_nano"`
}

// EvidenceSubjectDigestV2 is the review-independent, evidence-ref-independent
// subject committed by Execution Evidence. It includes the closed execution
// truth plus every inspection identity/governance coordinate, while excluding
// PayloadDigest and Evidence to avoid a digest cycle with the ledger record.
func (f ExecutionSettlementInspectionV2) EvidenceSubjectDigestV2() (core.Digest, error) {
	copy := f
	copy.PayloadDigest = ""
	copy.Evidence = EvidenceRecordRefV2{}
	if copy.ContractVersion != RunSettlementContractVersionV2 || validateEvidenceIDV2(copy.ID) != nil || validateEvidenceIDV2(string(copy.RunID)) != nil || copy.Revision == 0 || copy.RunRevision == 0 || copy.SourceEpoch == 0 || copy.SourceSequence == 0 || copy.InspectedUnixNano <= 0 || copy.ExpiresUnixNano <= copy.InspectedUnixNano {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonExecutionInspectionInvalid, "execution inspection Evidence subject identity is incomplete")
	}
	if err := copy.ExecutionScope.Validate(); err != nil {
		return "", err
	}
	scopeDigest, err := ExecutionScopeDigestV2(copy.ExecutionScope)
	if err != nil || scopeDigest != copy.ExecutionScopeDigest || copy.SourceEpoch != copy.ExecutionScope.Instance.Epoch {
		return "", core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "execution inspection Evidence subject scope drifted")
	}
	if err := copy.Subject.Validate(); err != nil {
		return "", err
	}
	if err := copy.RunIdentityDigest.Validate(); err != nil {
		return "", err
	}
	if !validRunExecutionTruthV2(copy.Truth) {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonExecutionInspectionInvalid, "execution inspection Evidence subject truth is invalid")
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "ExecutionSettlementEvidenceSubjectV2", copy)
}

func RunExecutionInspectionEvidenceCausationIDV2(f ExecutionSettlementInspectionV2) (string, error) {
	subject, err := f.EvidenceSubjectDigestV2()
	if err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "ExecutionSettlementEvidenceCausationV2", struct {
		InspectionID string              `json:"inspection_id"`
		Revision     core.Revision       `json:"revision"`
		Truth        RunExecutionTruthV2 `json:"truth"`
		Subject      core.Digest         `json:"subject_digest"`
	}{f.ID, f.Revision, f.Truth, subject})
	if err != nil {
		return "", err
	}
	return "execution-inspection:" + string(digest), nil
}

func (f ExecutionSettlementInspectionV2) Validate() error {
	if f.ContractVersion != RunSettlementContractVersionV2 || validateEvidenceIDV2(f.ID) != nil || validateEvidenceIDV2(string(f.RunID)) != nil || f.Revision == 0 || f.RunRevision == 0 || f.SourceEpoch == 0 || f.SourceSequence == 0 || f.InspectedUnixNano <= 0 || f.ExpiresUnixNano <= f.InspectedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonExecutionInspectionInvalid, "execution inspection identity, sequence and TTL are incomplete")
	}
	if err := f.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := ExecutionScopeDigestV2(f.ExecutionScope)
	if err != nil {
		return err
	}
	if f.ExecutionScopeDigest != scopeDigest || f.SourceEpoch != f.ExecutionScope.Instance.Epoch {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "execution inspection scope or instance epoch drifted")
	}
	if err := f.Subject.Validate(); err != nil {
		return err
	}
	if err := f.RunIdentityDigest.Validate(); err != nil {
		return err
	}
	if err := f.PayloadDigest.Validate(); err != nil {
		return err
	}
	evidenceSubject, err := f.EvidenceSubjectDigestV2()
	if err != nil || f.PayloadDigest != evidenceSubject {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "execution inspection payload does not bind its exact identity and truth")
	}
	if err := f.Evidence.Validate(); err != nil {
		return err
	}
	if !validRunExecutionTruthV2(f.Truth) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonExecutionInspectionInvalid, "execution inspection truth is invalid")
	}
	return nil
}

func (f ExecutionSettlementInspectionV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", RunSettlementContractVersionV2, "ExecutionSettlementInspectionV2", f)
}

func (f ExecutionSettlementInspectionV2) RefV2() (RunExecutionInspectionRefV2, error) {
	digest, err := f.DigestV2()
	if err != nil {
		return RunExecutionInspectionRefV2{}, err
	}
	return RunExecutionInspectionRefV2{ID: f.ID, Revision: f.Revision, Digest: digest}, nil
}

type RunExecutionSettlementInspectorV2 interface {
	InspectRunExecutionV2(context.Context, RunExecutionInspectionRequestV2) (ExecutionSettlementInspectionV2, error)
}

func validRunSettlementDispositionV2(value RunSettlementDispositionV2) bool {
	switch value {
	case RunSettlementConfirmedSatisfied, RunSettlementConfirmedFailed, RunSettlementConfirmedNotApplied, RunSettlementUnknown, RunSettlementOperationNotRequired:
		return true
	default:
		return false
	}
}

func validRunExecutionTruthV2(value RunExecutionTruthV2) bool {
	switch value {
	case RunExecutionTerminalCompleted, RunExecutionTerminalCancelled, RunExecutionTerminalFailed, RunExecutionConfirmedLost, RunExecutionUnknown:
		return true
	default:
		return false
	}
}

func SortRunSettlementRequirementsV2(values []RunSettlementRequirementV2) {
	sort.Slice(values, func(i, j int) bool {
		left := string(values[i].Phase) + "\x00" + string(values[i].ID)
		right := string(values[j].Phase) + "\x00" + string(values[j].ID)
		return strings.Compare(left, right) < 0
	})
}
