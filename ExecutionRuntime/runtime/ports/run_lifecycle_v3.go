package ports

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const RunLifecycleContractVersionV3 = "3.0.0"

type RunLifecyclePhaseV3 string

const (
	RunLifecyclePendingPreparedV3   RunLifecyclePhaseV3 = "pending_prepared"
	RunLifecycleRunningV3           RunLifecyclePhaseV3 = "running"
	RunLifecycleStoppingV3          RunLifecyclePhaseV3 = "stopping"
	RunLifecycleTerminalCleanupV3   RunLifecyclePhaseV3 = "terminal_cleanup_pending"
	RunLifecycleTerminationClosedV3 RunLifecyclePhaseV3 = "termination_closed"
)

type RunEffectIndexRefV3 struct {
	ID                   string          `json:"id"`
	Revision             core.Revision   `json:"revision"`
	Digest               core.Digest     `json:"digest"`
	RunID                core.AgentRunID `json:"run_id"`
	RunIdentityDigest    core.Digest     `json:"run_identity_digest"`
	ExecutionScopeDigest core.Digest     `json:"execution_scope_digest"`
	Watermark            uint64          `json:"watermark"`
	SegmentCount         uint64          `json:"segment_count"`
	EffectCount          uint64          `json:"effect_count"`
	HeadDigest           core.Digest     `json:"head_digest"`
	Frozen               bool            `json:"frozen"`
}

func (r RunEffectIndexRefV3) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || validateEvidenceIDV2(string(r.RunID)) != nil || r.Revision == 0 || r.Watermark == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunEffectIndexConflict, "Run Effect index ref identity and watermark are required")
	}
	for _, digest := range []core.Digest{r.Digest, r.RunIdentityDigest, r.ExecutionScopeDigest, r.HeadDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if (r.SegmentCount == 0) != (r.EffectCount == 0) || (r.SegmentCount == 0 && r.HeadDigest != EvidenceGenesisDigestV2) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "Run Effect index counts and head digest are inconsistent")
	}
	return nil
}

type RunSettlementPlanLifecycleRefV3 struct {
	RunSettlementPlanRefV2
	RunID                core.AgentRunID `json:"run_id"`
	RunIdentityDigest    core.Digest     `json:"run_identity_digest"`
	ExecutionScopeDigest core.Digest     `json:"execution_scope_digest"`
}

func (r RunSettlementPlanLifecycleRefV3) Validate() error {
	if err := r.RunSettlementPlanRefV2.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.RunID)) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Plan lifecycle ref requires a bounded Run identity")
	}
	if err := r.RunIdentityDigest.Validate(); err != nil {
		return err
	}
	return r.ExecutionScopeDigest.Validate()
}

type RunSettlementClosureRefV3 struct {
	ID                   string          `json:"id"`
	RunID                core.AgentRunID `json:"run_id"`
	RunIdentityDigest    core.Digest     `json:"run_identity_digest"`
	ExecutionScopeDigest core.Digest     `json:"execution_scope_digest"`
	Attempt              uint64          `json:"attempt"`
	Revision             core.Revision   `json:"revision"`
	Digest               core.Digest     `json:"digest"`
}

func (r RunSettlementClosureRefV3) Validate() error {
	return validateRunLifecycleSidecarRefV3(r.ID, r.RunID, r.RunIdentityDigest, r.ExecutionScopeDigest, r.Revision, r.Digest, r.Attempt)
}

type RunSettlementDecisionRefV3 struct {
	ID                   string                    `json:"id"`
	RunID                core.AgentRunID           `json:"run_id"`
	RunIdentityDigest    core.Digest               `json:"run_identity_digest"`
	ExecutionScopeDigest core.Digest               `json:"execution_scope_digest"`
	Revision             core.Revision             `json:"revision"`
	Digest               core.Digest               `json:"digest"`
	Outcome              core.ExecutionOutcome     `json:"outcome"`
	Closure              RunSettlementClosureRefV3 `json:"closure"`
}

func (r RunSettlementDecisionRefV3) Validate() error {
	if err := validateRunLifecycleSidecarRefV3(r.ID, r.RunID, r.RunIdentityDigest, r.ExecutionScopeDigest, r.Revision, r.Digest, 1); err != nil {
		return err
	}
	if !validRunLifecycleOutcomeV3(r.Outcome) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunCompletionConflict, "Decision ref requires a closed Runtime outcome")
	}
	return r.Closure.Validate()
}

type RunTerminationProgressRefV3 struct {
	ID                   string                     `json:"id"`
	RunID                core.AgentRunID            `json:"run_id"`
	RunIdentityDigest    core.Digest                `json:"run_identity_digest"`
	ExecutionScopeDigest core.Digest                `json:"execution_scope_digest"`
	Revision             core.Revision              `json:"revision"`
	Digest               core.Digest                `json:"digest"`
	UnresolvedCount      uint32                     `json:"unresolved_count"`
	Decision             RunSettlementDecisionRefV3 `json:"decision"`
}

func (r RunTerminationProgressRefV3) Validate() error {
	if err := validateRunLifecycleSidecarRefV3(r.ID, r.RunID, r.RunIdentityDigest, r.ExecutionScopeDigest, r.Revision, r.Digest, 1); err != nil {
		return err
	}
	return r.Decision.Validate()
}

type RunTerminationReportRefV3 struct {
	ID                   string                      `json:"id"`
	RunID                core.AgentRunID             `json:"run_id"`
	RunIdentityDigest    core.Digest                 `json:"run_identity_digest"`
	ExecutionScopeDigest core.Digest                 `json:"execution_scope_digest"`
	Revision             core.Revision               `json:"revision"`
	Digest               core.Digest                 `json:"digest"`
	Decision             RunSettlementDecisionRefV3  `json:"decision"`
	Progress             RunTerminationProgressRefV3 `json:"progress"`
}

func (r RunTerminationReportRefV3) Validate() error {
	if err := validateRunLifecycleSidecarRefV3(r.ID, r.RunID, r.RunIdentityDigest, r.ExecutionScopeDigest, r.Revision, r.Digest, 1); err != nil {
		return err
	}
	if err := r.Decision.Validate(); err != nil {
		return err
	}
	return r.Progress.Validate()
}

type RunLifecycleEnvelopeV3 struct {
	ContractVersion string                                      `json:"contract_version"`
	Phase           RunLifecyclePhaseV3                         `json:"phase"`
	Run             core.AgentRunRecord                         `json:"run"`
	Plan            RunSettlementPlanLifecycleRefV3             `json:"plan"`
	Certification   RunSettlementPlanCertificationAssociationV3 `json:"plan_certification"`
	EffectIndex     RunEffectIndexRefV3                         `json:"effect_index"`
	Closure         *RunSettlementClosureRefV3                  `json:"closure,omitempty"`
	Decision        *RunSettlementDecisionRefV3                 `json:"decision,omitempty"`
	Progress        *RunTerminationProgressRefV3                `json:"termination_progress,omitempty"`
	Report          *RunTerminationReportRefV3                  `json:"termination_report,omitempty"`
}

func (e RunLifecycleEnvelopeV3) Validate() error {
	if e.ContractVersion != RunLifecycleContractVersionV3 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "Run lifecycle envelope contract version is unsupported")
	}
	if err := e.Run.Validate(); err != nil {
		return err
	}
	if err := e.Plan.Validate(); err != nil {
		return err
	}
	if err := e.Certification.Validate(); err != nil {
		return err
	}
	if err := e.EffectIndex.Validate(); err != nil {
		return err
	}
	runIdentity, err := RunIdentityDigestV2(e.Run)
	if err != nil {
		return err
	}
	scopeDigest, err := ExecutionScopeDigestV2(e.Run.Scope)
	if err != nil {
		return err
	}
	if e.Plan.RunID != e.Run.ID || e.EffectIndex.RunID != e.Run.ID || e.Plan.RunIdentityDigest != runIdentity || e.EffectIndex.RunIdentityDigest != runIdentity || e.Plan.ExecutionScopeDigest != scopeDigest || e.EffectIndex.ExecutionScopeDigest != scopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Run, Plan and Effect index lifecycle identities differ")
	}
	if e.Certification.RunID != e.Run.ID || e.Certification.RunIdentityDigest != runIdentity || e.Certification.ExecutionScopeDigest != scopeDigest || e.Certification.Plan != e.Plan.RunSettlementPlanRefV2 {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "lifecycle Plan certification association drifted")
	}
	if e.Closure != nil {
		if err := e.Closure.Validate(); err != nil {
			return err
		}
	}
	if e.Decision != nil {
		if err := e.Decision.Validate(); err != nil {
			return err
		}
	}
	if e.Progress != nil {
		if err := e.Progress.Validate(); err != nil {
			return err
		}
	}
	if e.Report != nil {
		if err := e.Report.Validate(); err != nil {
			return err
		}
	}
	if err := e.validateSidecarRelationshipsV3(runIdentity, scopeDigest); err != nil {
		return err
	}
	switch e.Phase {
	case RunLifecyclePendingPreparedV3:
		if e.Run.Status != core.RunPending || e.EffectIndex.Frozen || e.Closure != nil || e.Decision != nil || e.Progress != nil || e.Report != nil {
			return invalidRunLifecyclePhaseV3("pending lifecycle carries an invalid status or terminal sidecar")
		}
	case RunLifecycleRunningV3:
		if e.Run.Status != core.RunRunning || e.EffectIndex.Frozen || e.Closure != nil || e.Decision != nil || e.Progress != nil || e.Report != nil {
			return invalidRunLifecyclePhaseV3("running lifecycle carries an invalid status or terminal sidecar")
		}
	case RunLifecycleStoppingV3:
		if e.Run.Status != core.RunStopping || e.Decision != nil || e.Progress != nil || e.Report != nil {
			return invalidRunLifecyclePhaseV3("stopping lifecycle carries a terminal Decision, Progress or Report")
		}
	case RunLifecycleTerminalCleanupV3:
		if e.Run.Status != core.RunTerminal || !e.EffectIndex.Frozen || e.Closure == nil || e.Decision == nil || e.Progress == nil || e.Report != nil || e.Decision.Outcome != e.Run.Outcome {
			return invalidRunLifecyclePhaseV3("terminal cleanup lifecycle lacks exact terminal watermarks")
		}
	case RunLifecycleTerminationClosedV3:
		if e.Run.Status != core.RunTerminal || !e.EffectIndex.Frozen || e.Closure == nil || e.Decision == nil || e.Progress == nil || e.Report == nil || e.Progress.UnresolvedCount != 0 || e.Decision.Outcome != e.Run.Outcome {
			return invalidRunLifecyclePhaseV3("closed lifecycle requires exact resolved Progress and Report")
		}
	default:
		return invalidRunLifecyclePhaseV3("unknown Run lifecycle phase")
	}
	return nil
}

func (e RunLifecycleEnvelopeV3) validateSidecarRelationshipsV3(runIdentity, scopeDigest core.Digest) error {
	if e.Closure != nil && (e.Closure.RunID != e.Run.ID || e.Closure.RunIdentityDigest != runIdentity || e.Closure.ExecutionScopeDigest != scopeDigest) {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementClosureConflict, "Closure belongs to another Run identity")
	}
	if e.Decision != nil && (e.Decision.RunID != e.Run.ID || e.Decision.RunIdentityDigest != runIdentity || e.Decision.ExecutionScopeDigest != scopeDigest) {
		return core.NewError(core.ErrorConflict, core.ReasonRunCompletionConflict, "Decision belongs to another Run identity")
	}
	if e.Progress != nil && (e.Progress.RunID != e.Run.ID || e.Progress.RunIdentityDigest != runIdentity || e.Progress.ExecutionScopeDigest != scopeDigest) {
		return core.NewError(core.ErrorConflict, core.ReasonTerminationProgressConflict, "Progress belongs to another Run identity")
	}
	if e.Report != nil && (e.Report.RunID != e.Run.ID || e.Report.RunIdentityDigest != runIdentity || e.Report.ExecutionScopeDigest != scopeDigest) {
		return core.NewError(core.ErrorConflict, core.ReasonTerminationReportIncomplete, "Report belongs to another Run identity")
	}
	if e.Decision != nil && (e.Closure == nil || e.Decision.Closure != *e.Closure) {
		return core.NewError(core.ErrorConflict, core.ReasonRunCompletionConflict, "Decision does not bind the current Closure attempt")
	}
	if e.Progress != nil && (e.Decision == nil || e.Progress.Decision != *e.Decision) {
		return core.NewError(core.ErrorConflict, core.ReasonTerminationProgressConflict, "Progress does not bind the terminal Decision")
	}
	if e.Report != nil && (e.Decision == nil || e.Progress == nil || e.Report.Decision != *e.Decision || e.Report.Progress != *e.Progress) {
		return core.NewError(core.ErrorConflict, core.ReasonTerminationReportIncomplete, "Report does not bind the exact Decision and Progress")
	}
	return nil
}

// CreatePendingRunRequestV3 is restricted to a trusted Runtime Assembler and
// requires an independently persisted, current Plan certification.
type CreatePendingRunRequestV3 struct {
	Run           core.AgentRunRecord                         `json:"run"`
	Plan          RunSettlementPlanFactV2                     `json:"plan"`
	Certification RunSettlementPlanCertificationAssociationV3 `json:"plan_certification"`
	EffectIndexID string                                      `json:"effect_index_id"`
}

func (r CreatePendingRunRequestV3) Validate() error {
	if err := r.Run.Validate(); err != nil {
		return err
	}
	if r.Run.Status != core.RunPending || r.Run.Revision != 1 || !r.Run.StartedAt.IsZero() || r.Run.Outcome != "" || r.Run.CompletionClaim != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunConflict, "public Run create accepts only pending revision-one facts")
	}
	if err := r.Plan.Validate(); err != nil {
		return err
	}
	if err := r.Certification.Validate(); err != nil {
		return err
	}
	expectedCertification, err := NewRunSettlementPlanCertificationAssociationV3(r.Run, r.Plan, r.Certification.Certification)
	if err != nil || expectedCertification != r.Certification {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "pending Run certification association does not derive from its Run and Plan")
	}
	identity, err := RunIdentityDigestV2(r.Run)
	if err != nil || r.Plan.RunID != r.Run.ID || r.Plan.RunIdentityDigest != identity || !SameExecutionScopeV2(r.Plan.ExecutionScope, r.Run.Scope) || r.Plan.SessionRef != r.Run.SessionRef {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "pending Run and immutable Plan identity differ")
	}
	if strings.TrimSpace(r.EffectIndexID) == "" || len(r.EffectIndexID) > 128 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunEffectIndexConflict, "bounded Effect index identity is required")
	}
	return nil
}

type BeginStopRunRequestV3 struct {
	ExecutionScope      core.ExecutionScope `json:"execution_scope"`
	RunID               core.AgentRunID     `json:"run_id"`
	ExpectedRunRevision core.Revision       `json:"expected_run_revision"`
}

func (r BeginStopRunRequestV3) Validate() error {
	if err := r.ExecutionScope.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.RunID)) != nil || r.ExpectedRunRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunConflict, "BeginStop requires a bounded Run and expected revision")
	}
	return nil
}

type RunTerminationRequestV3 struct {
	ExecutionScope core.ExecutionScope `json:"execution_scope"`
	RunID          core.AgentRunID     `json:"run_id"`
}

func (r RunTerminationRequestV3) Validate() error {
	if err := r.ExecutionScope.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.RunID)) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "termination request requires a bounded Run identity")
	}
	return nil
}

func validateRunLifecycleSidecarRefV3(id string, runID core.AgentRunID, runIdentity, scopeDigest core.Digest, revision core.Revision, digest core.Digest, ordinal uint64) error {
	if validateEvidenceIDV2(id) != nil || validateEvidenceIDV2(string(runID)) != nil || revision == 0 || ordinal == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Run lifecycle sidecar ref identity is incomplete")
	}
	for _, value := range []core.Digest{runIdentity, scopeDigest, digest} {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func invalidRunLifecyclePhaseV3(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, message)
}

func validRunLifecycleOutcomeV3(outcome core.ExecutionOutcome) bool {
	switch outcome {
	case core.OutcomeCompleted, core.OutcomeCancelled, core.OutcomeFailed, core.OutcomeLost, core.OutcomeIndeterminate, core.OutcomeNeedsReconciliation:
		return true
	default:
		return false
	}
}

// TrustedRunAssemblerPortV3 is deliberately separate from the
// Application-facing lifecycle Port. Components can publish declarations but
// cannot certify Plans or create Runs.
type TrustedRunAssemblerPortV3 interface {
	CreatePendingRunV3(context.Context, CreatePendingRunRequestV3) (RunLifecycleEnvelopeV3, error)
}

// RunLifecycleGovernancePortV3 is Application-facing after a trusted
// Assembler has created the pending Run. It never accepts an Outcome. Claims
// and observations remain evidence; Runtime derives the terminal Decision.
type RunLifecycleGovernancePortV3 interface {
	BeginStopRunV3(context.Context, BeginStopRunRequestV3) (RunLifecycleEnvelopeV3, error)
	StopAndSettleRunV3(context.Context, BeginStopRunRequestV3) (RunLifecycleEnvelopeV3, error)
	InspectRunLifecycleV3(context.Context, core.ExecutionScope, core.AgentRunID) (RunLifecycleEnvelopeV3, error)
	ReconcileRunTerminationV3(context.Context, RunTerminationRequestV3) (RunLifecycleEnvelopeV3, error)
	InspectRunTerminationV3(context.Context, RunTerminationRequestV3) (RunLifecycleEnvelopeV3, error)
}
