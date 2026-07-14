package control

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type RunBundleCreateRequestV2 struct {
	Run  core.AgentRunRecord           `json:"run"`
	Plan ports.RunSettlementPlanFactV2 `json:"plan"`
}

type RunBundleV2 struct {
	Run  core.AgentRunRecord           `json:"run"`
	Plan ports.RunSettlementPlanFactV2 `json:"plan"`
}

// RunBundleCreateRequestV3 is the certified create-once transaction. The
// certification ref is persisted atomically with the pending Run and Plan.
type RunBundleCreateRequestV3 struct {
	Run           core.AgentRunRecord                               `json:"run"`
	Plan          ports.RunSettlementPlanFactV2                     `json:"plan"`
	Certification ports.RunSettlementPlanCertificationAssociationV3 `json:"plan_certification"`
}

type RunBundleV3 struct {
	Run           core.AgentRunRecord                               `json:"run"`
	Plan          ports.RunSettlementPlanFactV2                     `json:"plan"`
	Certification ports.RunSettlementPlanCertificationAssociationV3 `json:"plan_certification"`
}

func (r RunBundleCreateRequestV3) Validate() error {
	if err := (RunBundleCreateRequestV2{Run: r.Run, Plan: r.Plan}).Validate(); err != nil {
		return err
	}
	if err := r.Certification.Validate(); err != nil {
		return err
	}
	expected, err := ports.NewRunSettlementPlanCertificationAssociationV3(r.Run, r.Plan, r.Certification.Certification)
	if err != nil || expected != r.Certification {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "certified Run bundle association does not derive from its Run and Plan")
	}
	return nil
}

type RunBundleFactPortV3 interface {
	CreateRunBundleV3(context.Context, RunBundleCreateRequestV3) (RunBundleV3, error)
	InspectRunBundleV3(context.Context, core.ExecutionScope, core.AgentRunID) (RunBundleV3, error)
}

func (r RunBundleCreateRequestV2) Validate() error {
	if err := r.Run.Validate(); err != nil {
		return err
	}
	if r.Run.Status != core.RunPending || r.Run.Revision != 1 || !r.Run.StartedAt.IsZero() || r.Run.CompletionClaim != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "new V2 run bundle requires a pending revision-one Run without execution claims or timestamps")
	}
	if err := r.Plan.Validate(); err != nil {
		return err
	}
	runIdentity, err := ports.RunIdentityDigestV2(r.Run)
	if err != nil {
		return err
	}
	if r.Plan.RunID != r.Run.ID || r.Plan.RunIdentityDigest != runIdentity || !ports.SameExecutionScopeV2(r.Plan.ExecutionScope, r.Run.Scope) || r.Plan.SessionRef != r.Run.SessionRef {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "Run and settlement Plan bundle identities differ")
	}
	return nil
}

type RunEffectIndexStateV2 string

const (
	RunEffectIndexOpen   RunEffectIndexStateV2 = "open"
	RunEffectIndexFrozen RunEffectIndexStateV2 = "frozen"
)

// RunEffectPartitionV2 is the stable tenant/scope/Run partition key. RunID is
// intentionally not treated as globally unique.
type RunEffectPartitionV2 struct {
	ExecutionScope       core.ExecutionScope `json:"execution_scope"`
	ExecutionScopeDigest core.Digest         `json:"execution_scope_digest"`
	RunID                core.AgentRunID     `json:"run_id"`
	RunIdentityDigest    core.Digest         `json:"run_identity_digest"`
}

func (p RunEffectPartitionV2) Validate() error {
	if err := p.ExecutionScope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(string(p.RunID)) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunEffectIndexConflict, "Run effect partition requires Run identity")
	}
	digest, err := ports.ExecutionScopeDigestV2(p.ExecutionScope)
	if err != nil || digest != p.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "Run effect partition scope digest drifted")
	}
	return p.RunIdentityDigest.Validate()
}

func (p RunEffectPartitionV2) DigestV2() (core.Digest, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunEffectPartitionV2", p)
}

func (f RunEffectIndexFactV2) PartitionV2() RunEffectPartitionV2 {
	return RunEffectPartitionV2{ExecutionScope: f.ExecutionScope, ExecutionScopeDigest: f.ExecutionScopeDigest, RunID: f.RunID, RunIdentityDigest: f.RunIdentityDigest}
}

type RunEffectRefV2 struct {
	EffectID       core.EffectIntentID `json:"effect_id"`
	IntentRevision core.Revision       `json:"intent_revision"`
	IntentDigest   core.Digest         `json:"intent_digest"`
	FactRevision   core.Revision       `json:"fact_revision"`
}

func (r RunEffectRefV2) Validate() error {
	if strings.TrimSpace(string(r.EffectID)) == "" || r.IntentRevision == 0 || r.FactRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunEffectIndexConflict, "run effect ref is incomplete")
	}
	return r.IntentDigest.Validate()
}

type RunEffectIndexFactV2 struct {
	ContractVersion      string                `json:"contract_version"`
	ID                   string                `json:"id"`
	Revision             core.Revision         `json:"revision"`
	RunID                core.AgentRunID       `json:"run_id"`
	RunIdentityDigest    core.Digest           `json:"run_identity_digest"`
	ExecutionScope       core.ExecutionScope   `json:"execution_scope"`
	ExecutionScopeDigest core.Digest           `json:"execution_scope_digest"`
	State                RunEffectIndexStateV2 `json:"state"`
	SegmentCount         uint64                `json:"segment_count"`
	EffectCount          uint64                `json:"effect_count"`
	HeadSegmentDigest    core.Digest           `json:"head_segment_digest"`
	Watermark            uint64                `json:"watermark"`
	CreatedUnixNano      int64                 `json:"created_unix_nano"`
	FrozenUnixNano       int64                 `json:"frozen_unix_nano,omitempty"`
}

func (f RunEffectIndexFactV2) Validate() error {
	if f.ContractVersion != ports.RunSettlementContractVersionV2 || strings.TrimSpace(f.ID) == "" || len(f.ID) > 128 || strings.TrimSpace(string(f.RunID)) == "" || f.Revision == 0 || f.Watermark == 0 || f.CreatedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunEffectIndexConflict, "run effect index identity and watermark are incomplete")
	}
	if err := f.RunIdentityDigest.Validate(); err != nil {
		return err
	}
	if err := f.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(f.ExecutionScope)
	if err != nil || scopeDigest != f.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "Run effect root execution scope drifted")
	}
	if err := f.ExecutionScopeDigest.Validate(); err != nil {
		return err
	}
	if err := f.HeadSegmentDigest.Validate(); err != nil {
		return err
	}
	if f.SegmentCount == 0 && (f.EffectCount != 0 || f.HeadSegmentDigest != ports.EvidenceGenesisDigestV2) || f.SegmentCount > 0 && f.EffectCount == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "Run effect root counts and chain head are inconsistent")
	}
	switch f.State {
	case RunEffectIndexOpen:
		if f.FrozenUnixNano != 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "open run effect index cannot have frozen time")
		}
	case RunEffectIndexFrozen:
		if f.FrozenUnixNano < f.CreatedUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "frozen run effect index requires ordered time")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "run effect index state is invalid")
	}
	return nil
}

func (f RunEffectIndexFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunEffectIndexFactV2", f)
}

const MaxRunEffectSegmentEntriesV2 = 256

type RunEffectSegmentFactV2 struct {
	ContractVersion      string           `json:"contract_version"`
	ID                   string           `json:"id"`
	RunID                core.AgentRunID  `json:"run_id"`
	RunIdentityDigest    core.Digest      `json:"run_identity_digest"`
	ExecutionScopeDigest core.Digest      `json:"execution_scope_digest"`
	Number               uint64           `json:"number"`
	Revision             core.Revision    `json:"revision"`
	PreviousDigest       core.Digest      `json:"previous_segment_digest"`
	Effects              []RunEffectRefV2 `json:"effects"`
	CreatedUnixNano      int64            `json:"created_unix_nano"`
	UpdatedUnixNano      int64            `json:"updated_unix_nano"`
}

func (f RunEffectSegmentFactV2) Validate() error {
	if f.ContractVersion != ports.RunSettlementContractVersionV2 || strings.TrimSpace(f.ID) == "" || len(f.ID) > 128 || strings.TrimSpace(string(f.RunID)) == "" || f.Number == 0 || f.Revision == 0 || len(f.Effects) == 0 || len(f.Effects) > MaxRunEffectSegmentEntriesV2 || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunEffectIndexConflict, "bounded Run effect segment is incomplete")
	}
	for _, digest := range []core.Digest{f.RunIdentityDigest, f.ExecutionScopeDigest, f.PreviousDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	previous := ""
	for index, ref := range f.Effects {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := string(ref.EffectID)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "segment Effect refs must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func (f RunEffectSegmentFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	if f.Effects == nil {
		f.Effects = []RunEffectRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunEffectSegmentFactV2", f)
}

type RunEffectSegmentPageV2 struct {
	Segments   []RunEffectSegmentFactV2 `json:"segments"`
	NextNumber uint64                   `json:"next_number"`
}

type RunEffectSetRefV2 struct {
	IndexID           string        `json:"index_id"`
	Revision          core.Revision `json:"revision"`
	Digest            core.Digest   `json:"digest"`
	Watermark         uint64        `json:"watermark"`
	SegmentCount      uint64        `json:"segment_count"`
	EffectCount       uint64        `json:"effect_count"`
	HeadSegmentDigest core.Digest   `json:"head_segment_digest"`
}

func (r RunEffectSetRefV2) Validate() error {
	if strings.TrimSpace(r.IndexID) == "" || r.Revision == 0 || r.Watermark == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunEffectIndexConflict, "run effect set ref is incomplete")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	return r.HeadSegmentDigest.Validate()
}

type CreateRunEffectRequestV2 struct {
	Partition             RunEffectPartitionV2 `json:"partition"`
	ExpectedIndexRevision core.Revision        `json:"expected_index_revision"`
	Effect                EffectFactV2         `json:"effect"`
}

type CreateRunEffectResultV2 struct {
	Effect  EffectFactV2           `json:"effect"`
	Index   RunEffectIndexFactV2   `json:"index"`
	Segment RunEffectSegmentFactV2 `json:"segment"`
}

type FreezeRunEffectSetRequestV2 struct {
	Partition             RunEffectPartitionV2 `json:"partition"`
	ExpectedIndexRevision core.Revision        `json:"expected_index_revision"`
	ExpectedRunRevision   core.Revision        `json:"expected_run_revision"`
}

type RunEffectFactPortV2 interface {
	CreateRunEffectIndexV2(context.Context, RunEffectIndexFactV2) (RunEffectIndexFactV2, error)
	InspectRunEffectIndexV2(context.Context, RunEffectPartitionV2) (RunEffectIndexFactV2, error)
	ListRunEffectSegmentsV2(context.Context, RunEffectPartitionV2, uint64, uint32) (RunEffectSegmentPageV2, error)
	CreateEffectForRunV2(context.Context, CreateRunEffectRequestV2) (CreateRunEffectResultV2, error)
	FreezeRunEffectSetV2(context.Context, FreezeRunEffectSetRequestV2) (RunEffectIndexFactV2, error)
	InspectRunEffectV2(context.Context, RunEffectPartitionV2, core.EffectIntentID) (EffectFactV2, error)
	CompareAndSwapRunEffectV2(context.Context, RunEffectPartitionV2, EffectFactCASRequestV2) (EffectFactV2, error)
	IssueRunDispatchPermitV2(context.Context, RunEffectPartitionV2, IssueDispatchPermitRequestV2) (IssueDispatchPermitResultV2, error)
	InspectRunDispatchPermitV2(context.Context, RunEffectPartitionV2, string) (DispatchPermitFactV2, error)
	BeginRunDispatchV2(context.Context, RunEffectPartitionV2, BeginDispatchRequestV2) (DispatchPermitFactV2, error)
	RecordRunEnforcementReceiptV2(context.Context, RunEffectPartitionV2, RecordEnforcementReceiptRequestV2) (DispatchPermitFactV2, error)
	CompareAndSwapRunDispatchPermitV2(context.Context, RunEffectPartitionV2, DispatchPermitFactCASRequestV2) (DispatchPermitFactV2, error)
	// InspectEffect is the legacy/global ID seam required by EffectFactPortV2.
	// P0.5/P0.6 governed Run paths must use InspectRunEffectV2.
	InspectEffect(context.Context, core.EffectIntentID) (EffectFactV2, error)
}

// RunEffectGovernanceAdapterV2 presents one explicit tenant/Run partition to
// the existing P0.2 gateway without copying facts into the legacy global-ID
// store. Unsupported legacy creation and global lookup methods fail closed.
type RunEffectGovernanceAdapterV2 struct {
	Partition RunEffectPartitionV2
	Facts     RunEffectFactPortV2
}

func (a RunEffectGovernanceAdapterV2) CreateEffect(context.Context, EffectFactV2) (EffectFactV2, error) {
	return EffectFactV2{}, scopedRunEffectOnlyV2()
}
func (a RunEffectGovernanceAdapterV2) InspectEffect(ctx context.Context, id core.EffectIntentID) (EffectFactV2, error) {
	return a.Facts.InspectRunEffectV2(ctx, a.Partition, id)
}
func (a RunEffectGovernanceAdapterV2) InspectEffectByIdempotency(context.Context, ports.EffectStableScopeClassV2, core.Digest, string) (EffectFactV2, error) {
	return EffectFactV2{}, scopedRunEffectOnlyV2()
}
func (a RunEffectGovernanceAdapterV2) InspectConflictDomain(context.Context, ports.ConflictDomainBindingV2) (EffectFactV2, error) {
	return EffectFactV2{}, scopedRunEffectOnlyV2()
}
func (a RunEffectGovernanceAdapterV2) CompareAndSwapEffect(ctx context.Context, request EffectFactCASRequestV2) (EffectFactV2, error) {
	return a.Facts.CompareAndSwapRunEffectV2(ctx, a.Partition, request)
}
func (a RunEffectGovernanceAdapterV2) IssueDispatchPermit(ctx context.Context, request IssueDispatchPermitRequestV2) (IssueDispatchPermitResultV2, error) {
	return a.Facts.IssueRunDispatchPermitV2(ctx, a.Partition, request)
}
func (a RunEffectGovernanceAdapterV2) InspectDispatchPermit(ctx context.Context, id string) (DispatchPermitFactV2, error) {
	return a.Facts.InspectRunDispatchPermitV2(ctx, a.Partition, id)
}
func (a RunEffectGovernanceAdapterV2) BeginDispatch(ctx context.Context, request BeginDispatchRequestV2) (DispatchPermitFactV2, error) {
	return a.Facts.BeginRunDispatchV2(ctx, a.Partition, request)
}
func (a RunEffectGovernanceAdapterV2) RecordEnforcementReceipt(ctx context.Context, request RecordEnforcementReceiptRequestV2) (DispatchPermitFactV2, error) {
	return a.Facts.RecordRunEnforcementReceiptV2(ctx, a.Partition, request)
}
func (a RunEffectGovernanceAdapterV2) CompareAndSwapDispatchPermit(ctx context.Context, request DispatchPermitFactCASRequestV2) (DispatchPermitFactV2, error) {
	return a.Facts.CompareAndSwapRunDispatchPermitV2(ctx, a.Partition, request)
}

func scopedRunEffectOnlyV2() error {
	return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "governed Run Effect adapter forbids legacy global mutation or lookup")
}

var _ EffectFactPortV2 = RunEffectGovernanceAdapterV2{}

type RunSettlementClosureParticipantV2 struct {
	RequirementID     ports.NamespacedNameV2               `json:"requirement_id"`
	RequirementDigest core.Digest                          `json:"requirement_digest"`
	Participant       ports.RunSettlementParticipantRefV2  `json:"participant"`
	ParticipantFact   ports.RunSettlementParticipantFactV2 `json:"participant_fact"`
	PolicyFact        ports.RunSettlementPolicyFactV2      `json:"policy_fact"`
}

func (p RunSettlementClosureParticipantV2) Validate() error {
	if err := ports.ValidateNamespacedNameV2(p.RequirementID); err != nil {
		return err
	}
	if err := p.RequirementDigest.Validate(); err != nil {
		return err
	}
	if p.Participant.RequirementID != p.RequirementID {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementParticipantStale, "closure participant requirement differs")
	}
	if err := p.Participant.Validate(); err != nil {
		return err
	}
	if err := p.ParticipantFact.Validate(); err != nil {
		return err
	}
	ref, err := p.ParticipantFact.RefV2()
	if err != nil || ref != p.Participant || p.ParticipantFact.RequirementID != p.RequirementID || p.ParticipantFact.RequirementDigest != p.RequirementDigest {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementParticipantStale, "closure participant ref does not derive from its exact Owner Fact")
	}
	if err := p.PolicyFact.Validate(); err != nil {
		return err
	}
	if p.PolicyFact.RequirementID != p.RequirementID || p.PolicyFact.RunID != p.ParticipantFact.RunID || p.PolicyFact.PlanID != p.ParticipantFact.Plan.ID || p.PolicyFact.PlanRevision != p.ParticipantFact.Plan.Revision {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementRequirementInvalid, "closure policy does not govern the exact participant requirement")
	}
	return nil
}

type RunSettlementClosureFactV2 struct {
	ContractVersion       string                                `json:"contract_version"`
	ID                    string                                `json:"id"`
	Revision              core.Revision                         `json:"revision"`
	RunID                 core.AgentRunID                       `json:"run_id"`
	RunIdentityDigest     core.Digest                           `json:"run_identity_digest"`
	RunRevision           core.Revision                         `json:"run_revision"`
	ExecutionScope        core.ExecutionScope                   `json:"execution_scope"`
	ExecutionScopeDigest  core.Digest                           `json:"execution_scope_digest"`
	Attempt               uint64                                `json:"attempt"`
	PreviousClosureDigest core.Digest                           `json:"previous_closure_digest"`
	Plan                  ports.RunSettlementPlanRefV2          `json:"plan"`
	Claim                 *ports.RunClaimAssociationFactV2      `json:"claim,omitempty"`
	Execution             ports.ExecutionSettlementInspectionV2 `json:"execution"`
	EffectSet             RunEffectSetRefV2                     `json:"effect_set"`
	Participants          []RunSettlementClosureParticipantV2   `json:"participants"`
	CreatedUnixNano       int64                                 `json:"created_unix_nano"`
}

func (f RunSettlementClosureFactV2) Validate() error {
	if f.ContractVersion != ports.RunSettlementContractVersionV2 || strings.TrimSpace(f.ID) == "" || len(f.ID) > 128 || strings.TrimSpace(string(f.RunID)) == "" || f.Revision != 1 || f.RunRevision == 0 || f.Attempt == 0 || f.CreatedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementClosureConflict, "run settlement closure identity is incomplete")
	}
	for _, digest := range []core.Digest{f.RunIdentityDigest, f.ExecutionScopeDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := f.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(f.ExecutionScope)
	if err != nil || scopeDigest != f.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementClosureConflict, "Closure execution scope drifted")
	}
	if f.Attempt == 1 {
		if f.PreviousClosureDigest != ports.EvidenceGenesisDigestV2 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementClosureConflict, "first Closure attempt requires the fixed genesis predecessor")
		}
	} else if err := f.PreviousClosureDigest.Validate(); err != nil {
		return err
	}
	if err := f.Plan.Validate(); err != nil {
		return err
	}
	if f.Claim != nil {
		if err := f.Claim.Validate(); err != nil {
			return err
		}
		if f.Claim.RunID != f.RunID || f.Claim.RunIdentityDigest != f.RunIdentityDigest || f.Claim.ExecutionScopeDigest != f.ExecutionScopeDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "closure claim does not bind the exact Run")
		}
	}
	if err := f.Execution.Validate(); err != nil {
		return err
	}
	if f.Execution.RunID != f.RunID || f.Execution.RunIdentityDigest != f.RunIdentityDigest || f.Execution.ExecutionScopeDigest != f.ExecutionScopeDigest || !ports.SameExecutionScopeV2(f.Execution.ExecutionScope, f.ExecutionScope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "closure execution inspection does not bind the exact Run")
	}
	if err := f.EffectSet.Validate(); err != nil {
		return err
	}
	previous := ""
	for index, participant := range f.Participants {
		if err := participant.Validate(); err != nil {
			return err
		}
		key := string(participant.RequirementID)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "closure participants must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func (f RunSettlementClosureFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	if f.Participants == nil {
		f.Participants = []RunSettlementClosureParticipantV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunSettlementClosureFactV2", f)
}

type RunSettlementClosureRefV2 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
	Attempt  uint64        `json:"attempt"`
}

func (f RunSettlementClosureFactV2) RefV2() (RunSettlementClosureRefV2, error) {
	digest, err := f.DigestV2()
	if err != nil {
		return RunSettlementClosureRefV2{}, err
	}
	return RunSettlementClosureRefV2{ID: f.ID, Revision: f.Revision, Digest: digest, Attempt: f.Attempt}, nil
}

func (r RunSettlementClosureRefV2) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 || r.Attempt == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementClosureConflict, "closure ref is incomplete")
	}
	return r.Digest.Validate()
}

type RunSettlementClosurePointerFactV2 struct {
	ContractVersion      string                    `json:"contract_version"`
	Revision             core.Revision             `json:"revision"`
	RunID                core.AgentRunID           `json:"run_id"`
	RunIdentityDigest    core.Digest               `json:"run_identity_digest"`
	ExecutionScopeDigest core.Digest               `json:"execution_scope_digest"`
	Current              RunSettlementClosureRefV2 `json:"current"`
	UpdatedUnixNano      int64                     `json:"updated_unix_nano"`
}

func (f RunSettlementClosurePointerFactV2) Validate() error {
	if f.ContractVersion != ports.RunSettlementContractVersionV2 || f.Revision == 0 || strings.TrimSpace(string(f.RunID)) == "" || f.UpdatedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementClosureConflict, "current Closure pointer is incomplete")
	}
	if err := f.RunIdentityDigest.Validate(); err != nil {
		return err
	}
	if err := f.ExecutionScopeDigest.Validate(); err != nil {
		return err
	}
	return f.Current.Validate()
}

func (f RunSettlementClosurePointerFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunSettlementClosurePointerFactV2", f)
}

type RunSettlementClosureAttemptResultV2 struct {
	Closure RunSettlementClosureFactV2        `json:"closure"`
	Pointer RunSettlementClosurePointerFactV2 `json:"pointer"`
}

type RunSettlementResolutionV2 struct {
	RequirementID  ports.NamespacedNameV2                `json:"requirement_id"`
	Kind           ports.NamespacedNameV2                `json:"kind"`
	Phase          ports.RunSettlementRequirementPhaseV2 `json:"phase"`
	Disposition    ports.RunSettlementDispositionV2      `json:"disposition"`
	Participant    *ports.RunSettlementParticipantRefV2  `json:"participant,omitempty"`
	Policy         ports.RunSettlementPolicyBindingRefV2 `json:"policy"`
	EvidenceDigest core.Digest                           `json:"evidence_digest"`
}

func (r RunSettlementResolutionV2) Validate() error {
	if err := ports.ValidateNamespacedNameV2(r.RequirementID); err != nil {
		return err
	}
	if err := ports.ValidateNamespacedNameV2(r.Kind); err != nil {
		return err
	}
	if r.Phase != ports.RunSettlementPhaseCompletion && r.Phase != ports.RunSettlementPhaseTerminationReport {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementRequirementInvalid, "resolution phase is invalid")
	}
	switch r.Disposition {
	case ports.RunSettlementConfirmedSatisfied, ports.RunSettlementConfirmedFailed, ports.RunSettlementConfirmedNotApplied, ports.RunSettlementUnknown, ports.RunSettlementOperationNotRequired:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementParticipantMissing, "resolution disposition is invalid")
	}
	if r.Participant != nil {
		if err := r.Participant.Validate(); err != nil {
			return err
		}
	}
	if err := r.Policy.Validate(); err != nil {
		return err
	}
	return r.EvidenceDigest.Validate()
}

type RunSettlementDecisionFactV2 struct {
	ContractVersion        string                            `json:"contract_version"`
	ID                     string                            `json:"id"`
	Revision               core.Revision                     `json:"revision"`
	RunID                  core.AgentRunID                   `json:"run_id"`
	RunIdentityDigest      core.Digest                       `json:"run_identity_digest"`
	ExpectedRunRevision    core.Revision                     `json:"expected_run_revision"`
	ExecutionScopeDigest   core.Digest                       `json:"execution_scope_digest"`
	Plan                   ports.RunSettlementPlanRefV2      `json:"plan"`
	Closure                RunSettlementClosureRefV2         `json:"closure"`
	ClosurePointerRevision core.Revision                     `json:"closure_pointer_revision"`
	Claim                  *ports.RunClaimAssociationFactV2  `json:"claim,omitempty"`
	Execution              ports.RunExecutionInspectionRefV2 `json:"execution"`
	Resolutions            []RunSettlementResolutionV2       `json:"resolutions"`
	Outcome                core.ExecutionOutcome             `json:"outcome"`
	CreatedUnixNano        int64                             `json:"created_unix_nano"`
}

func (f RunSettlementDecisionFactV2) Validate() error {
	if f.ContractVersion != ports.RunSettlementContractVersionV2 || strings.TrimSpace(f.ID) == "" || len(f.ID) > 128 || strings.TrimSpace(string(f.RunID)) == "" || f.Revision != 1 || f.ExpectedRunRevision == 0 || f.ClosurePointerRevision == 0 || f.CreatedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunCompletionConflict, "run settlement decision identity is incomplete")
	}
	for _, digest := range []core.Digest{f.RunIdentityDigest, f.ExecutionScopeDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := f.Plan.Validate(); err != nil {
		return err
	}
	if err := f.Closure.Validate(); err != nil {
		return err
	}
	if f.Claim != nil {
		if err := f.Claim.Validate(); err != nil {
			return err
		}
	}
	if err := f.Execution.Validate(); err != nil {
		return err
	}
	if !validRunOutcomeV2(f.Outcome) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunCompletionConflict, "decision outcome is invalid")
	}
	if len(f.Resolutions) == 0 || len(f.Resolutions) > ports.MaxRunSettlementRequirementsV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementRequirementInvalid, "decision resolutions are empty or exceed bounds")
	}
	previous := ""
	for index, resolution := range f.Resolutions {
		if err := resolution.Validate(); err != nil {
			return err
		}
		if resolution.Phase != ports.RunSettlementPhaseCompletion {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "Decision can contain only run_completion resolutions")
		}
		key := string(resolution.Phase) + "\x00" + string(resolution.RequirementID)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "decision resolutions must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func (f RunSettlementDecisionFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	if f.Resolutions == nil {
		f.Resolutions = []RunSettlementResolutionV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunSettlementDecisionFactV2", f)
}

// ValidateRunSettlementDecisionAgainstClosureV2 is the Run Fact Owner's
// independent provenance gate. It prevents a caller holding the raw FactPort
// from substituting Claim, Execution, participant refs or Outcome while still
// presenting a valid current Closure pointer.
func ValidateRunSettlementDecisionAgainstClosureV2(f RunSettlementDecisionFactV2, closure RunSettlementClosureFactV2, plan ports.RunSettlementPlanFactV2) error {
	if err := f.Validate(); err != nil {
		return err
	}
	closureRef, err := closure.RefV2()
	if err != nil {
		return err
	}
	planRef, err := plan.RefV2()
	if err != nil {
		return err
	}
	executionRef, err := closure.Execution.RefV2()
	if err != nil {
		return err
	}
	if f.Closure != closureRef || f.Plan != planRef || f.Execution != executionRef || f.RunID != closure.RunID || f.RunIdentityDigest != closure.RunIdentityDigest || f.ExecutionScopeDigest != closure.ExecutionScopeDigest || f.ExpectedRunRevision != closure.RunRevision {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunCompletionConflict, "Decision provenance differs from immutable Plan, Closure or Execution inspection")
	}
	if !sameRunClaimAssociationV2(f.Claim, closure.Claim) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "Decision Claim differs from immutable Closure")
	}
	completion := make(map[ports.NamespacedNameV2]ports.RunSettlementRequirementV2)
	for _, requirement := range plan.Requirements {
		if requirement.Phase == ports.RunSettlementPhaseCompletion {
			completion[requirement.ID] = requirement
		}
	}
	if len(f.Resolutions) != len(completion) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "Decision does not resolve every completion requirement")
	}
	reconcile := false
	executionDisposition := ports.RunSettlementUnknown
	for _, resolution := range f.Resolutions {
		requirement, exists := completion[resolution.RequirementID]
		if !exists || resolution.Kind != requirement.Kind || resolution.Phase != requirement.Phase || resolution.Policy != requirement.Policy {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "Decision resolution differs from frozen Plan")
		}
		switch requirement.Kind {
		case ports.RunRequirementExecutionTruth:
			if resolution.Participant != nil || resolution.EvidenceDigest != closure.Execution.PayloadDigest {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "execution resolution differs from Closure inspection")
			}
			executionDisposition = resolution.Disposition
		case ports.RunRequirementEffects:
			if resolution.Participant != nil || resolution.EvidenceDigest != closure.EffectSet.Digest {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "effect resolution differs from frozen Effect set")
			}
		default:
			link, found := closureParticipantByRequirementV2(closure, requirement.ID)
			if !found || resolution.Participant == nil || *resolution.Participant != link.Participant || resolution.EvidenceDigest != link.Participant.Digest || resolution.Disposition != link.Participant.Disposition {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementParticipantStale, "participant resolution differs from immutable Closure")
			}
			if resolution.Disposition == ports.RunSettlementOperationNotRequired && link.Participant.Policy != resolution.Policy {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "not-required resolution differs from participant Policy provenance")
			}
		}
		switch resolution.Disposition {
		case ports.RunSettlementConfirmedFailed, ports.RunSettlementConfirmedNotApplied:
			reconcile = true
		case ports.RunSettlementUnknown:
			if requirement.Kind != ports.RunRequirementExecutionTruth {
				reconcile = true
			}
		}
	}
	expected := core.ExecutionOutcome("")
	if reconcile {
		expected = core.OutcomeNeedsReconciliation
	} else {
		switch closure.Execution.Truth {
		case ports.RunExecutionTerminalCompleted:
			if executionDisposition == ports.RunSettlementConfirmedSatisfied {
				expected = core.OutcomeCompleted
			}
		case ports.RunExecutionTerminalCancelled:
			if executionDisposition == ports.RunSettlementConfirmedSatisfied {
				expected = core.OutcomeCancelled
			}
		case ports.RunExecutionTerminalFailed:
			if executionDisposition == ports.RunSettlementConfirmedSatisfied {
				expected = core.OutcomeFailed
			}
		case ports.RunExecutionConfirmedLost:
			if executionDisposition == ports.RunSettlementConfirmedSatisfied {
				expected = core.OutcomeLost
			} else if executionDisposition == ports.RunSettlementUnknown {
				expected = core.OutcomeIndeterminate
			}
		case ports.RunExecutionUnknown:
			if executionDisposition == ports.RunSettlementUnknown {
				expected = core.OutcomeIndeterminate
			}
		}
	}
	if expected == "" || f.Outcome != expected {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunCompletionConflict, "Decision Outcome is not derivable from exact Closure truth and resolutions")
	}
	return nil
}

func closureParticipantByRequirementV2(closure RunSettlementClosureFactV2, id ports.NamespacedNameV2) (RunSettlementClosureParticipantV2, bool) {
	for _, participant := range closure.Participants {
		if participant.RequirementID == id {
			return participant, true
		}
	}
	return RunSettlementClosureParticipantV2{}, false
}

func sameRunClaimAssociationV2(left, right *ports.RunClaimAssociationFactV2) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

type RunSettlementDecisionRefV2 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (f RunSettlementDecisionFactV2) RefV2() (RunSettlementDecisionRefV2, error) {
	digest, err := f.DigestV2()
	if err != nil {
		return RunSettlementDecisionRefV2{}, err
	}
	return RunSettlementDecisionRefV2{ID: f.ID, Revision: f.Revision, Digest: digest}, nil
}

func (r RunSettlementDecisionRefV2) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunCompletionConflict, "decision ref is incomplete")
	}
	return r.Digest.Validate()
}

type RunTerminationProgressFactV2 struct {
	ContractVersion      string                      `json:"contract_version"`
	ID                   string                      `json:"id"`
	Revision             core.Revision               `json:"revision"`
	RunID                core.AgentRunID             `json:"run_id"`
	ExecutionScope       core.ExecutionScope         `json:"execution_scope"`
	ExecutionScopeDigest core.Digest                 `json:"execution_scope_digest"`
	Decision             RunSettlementDecisionRefV2  `json:"decision"`
	Items                []RunSettlementResolutionV2 `json:"items"`
	UpdatedUnixNano      int64                       `json:"updated_unix_nano"`
}

func (f RunTerminationProgressFactV2) Validate() error {
	if f.ContractVersion != ports.RunSettlementContractVersionV2 || strings.TrimSpace(f.ID) == "" || strings.TrimSpace(string(f.RunID)) == "" || f.Revision == 0 || f.UpdatedUnixNano <= 0 || len(f.Items) == 0 || len(f.Items) > ports.MaxRunSettlementRequirementsV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonTerminationProgressConflict, "termination progress identity and items are incomplete")
	}
	if err := f.Decision.Validate(); err != nil {
		return err
	}
	if err := f.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(f.ExecutionScope)
	if err != nil || scopeDigest != f.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationProgressConflict, "termination Progress execution scope drifted")
	}
	previous := ""
	for index, item := range f.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if item.Phase != ports.RunSettlementPhaseTerminationReport {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationProgressConflict, "progress can only contain termination_report requirements")
		}
		key := string(item.RequirementID)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "termination progress items must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func (f RunTerminationProgressFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	if f.Items == nil {
		f.Items = []RunSettlementResolutionV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunTerminationProgressFactV2", f)
}

type RunTerminationProgressRefV2 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (f RunTerminationProgressFactV2) RefV2() (RunTerminationProgressRefV2, error) {
	digest, err := f.DigestV2()
	if err != nil {
		return RunTerminationProgressRefV2{}, err
	}
	return RunTerminationProgressRefV2{ID: f.ID, Revision: f.Revision, Digest: digest}, nil
}
func (r RunTerminationProgressRefV2) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonTerminationReportIncomplete, "termination Progress ref is incomplete")
	}
	return r.Digest.Validate()
}

type RunTerminationProgressCASRequestV2 struct {
	ExpectedRevision core.Revision                `json:"expected_revision"`
	Next             RunTerminationProgressFactV2 `json:"next"`
}

func ValidateRunTerminationProgressTransitionV2(current, next RunTerminationProgressFactV2, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if next.Revision != current.Revision+1 || current.ID != next.ID || current.RunID != next.RunID || !ports.SameExecutionScopeV2(current.ExecutionScope, next.ExecutionScope) || current.ExecutionScopeDigest != next.ExecutionScopeDigest || current.Decision != next.Decision || len(current.Items) != len(next.Items) {
		return core.NewError(core.ErrorConflict, core.ReasonTerminationProgressConflict, "termination progress identity, decision or revision drifted")
	}
	if now.IsZero() || next.UpdatedUnixNano < current.UpdatedUnixNano || next.UpdatedUnixNano != now.UnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "termination progress requires a monotonic injected clock")
	}
	for index := range current.Items {
		left, right := current.Items[index], next.Items[index]
		if left.RequirementID != right.RequirementID || left.Kind != right.Kind || left.Phase != right.Phase || left.Policy != right.Policy {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationProgressConflict, "termination requirement identity is immutable")
		}
		if left.Disposition != ports.RunSettlementUnknown && !sameRunSettlementResolutionV2(left, right) {
			return core.NewError(core.ErrorConflict, core.ReasonTerminationProgressConflict, "resolved termination requirement is immutable")
		}
	}
	return nil
}

type CommitRunCompletionRequestV2 struct {
	ExecutionScope      core.ExecutionScope          `json:"execution_scope"`
	ExpectedRunRevision core.Revision                `json:"expected_run_revision"`
	Decision            RunSettlementDecisionFactV2  `json:"decision"`
	InitialProgress     RunTerminationProgressFactV2 `json:"initial_progress"`
}

type CommitRunCompletionResultV2 struct {
	Run      core.AgentRunRecord          `json:"run"`
	Decision RunSettlementDecisionFactV2  `json:"decision"`
	Progress RunTerminationProgressFactV2 `json:"progress"`
}

// CommitRunStartRequestV3 is an internal Run Fact Owner primitive. The Run
// transition and immutable start proof must become visible atomically.
type CommitRunStartRequestV3 struct {
	ExpectedRunRevision core.Revision                    `json:"expected_run_revision"`
	NextRun             core.AgentRunRecord              `json:"next_run"`
	Confirmation        ports.RunStartConfirmationFactV3 `json:"confirmation"`
}

type RunSettlementFactPortV2 interface {
	RunFactPort
	CreateRunBundleV2(context.Context, RunBundleCreateRequestV2) (RunBundleV2, error)
	CommitRunStartV3(context.Context, CommitRunStartRequestV3) (ports.RunStartConfirmationEnvelopeV3, error)
	InspectRunStartConfirmationV3(context.Context, core.ExecutionScope, core.AgentRunID) (ports.RunStartConfirmationEnvelopeV3, error)
	InspectRunSettlementPlanV2(context.Context, core.ExecutionScope, core.AgentRunID) (ports.RunSettlementPlanFactV2, error)
	CreateRunSettlementClosureAttemptV2(context.Context, RunSettlementClosureFactV2) (RunSettlementClosureAttemptResultV2, error)
	InspectRunSettlementClosureAttemptV2(context.Context, core.ExecutionScope, core.AgentRunID, uint64) (RunSettlementClosureFactV2, error)
	InspectCurrentRunSettlementClosureV2(context.Context, core.ExecutionScope, core.AgentRunID) (RunSettlementClosureAttemptResultV2, error)
	CommitRunCompletionV2(context.Context, CommitRunCompletionRequestV2) (CommitRunCompletionResultV2, error)
	InspectRunSettlementDecisionV2(context.Context, core.ExecutionScope, core.AgentRunID) (RunSettlementDecisionFactV2, error)
	InspectRunTerminationProgressV2(context.Context, core.ExecutionScope, core.AgentRunID) (RunTerminationProgressFactV2, error)
	CompareAndSwapRunTerminationProgressV2(context.Context, RunTerminationProgressCASRequestV2) (RunTerminationProgressFactV2, error)
	CreateRunTerminationReportV2(context.Context, RunTerminationReportV2) (RunTerminationReportV2, error)
	InspectRunTerminationReportV2(context.Context, core.ExecutionScope, core.AgentRunID) (RunTerminationReportV2, error)
}

type RunTerminationReportV2 struct {
	ContractVersion      string                      `json:"contract_version"`
	ID                   string                      `json:"id"`
	Revision             core.Revision               `json:"revision"`
	RunID                core.AgentRunID             `json:"run_id"`
	RunIdentityDigest    core.Digest                 `json:"run_identity_digest"`
	ExecutionScope       core.ExecutionScope         `json:"execution_scope"`
	ExecutionScopeDigest core.Digest                 `json:"execution_scope_digest"`
	Decision             RunSettlementDecisionRefV2  `json:"decision"`
	Progress             RunTerminationProgressRefV2 `json:"progress"`
	Outcome              core.ExecutionOutcome       `json:"outcome"`
	Items                []RunSettlementResolutionV2 `json:"items"`
	CompletedUnixNano    int64                       `json:"completed_unix_nano"`
}

func BuildRunTerminationReportV2(run core.AgentRunRecord, decision RunSettlementDecisionFactV2, progress RunTerminationProgressFactV2) (RunTerminationReportV2, error) {
	if run.Status != core.RunTerminal || run.ID != decision.RunID || run.Outcome != decision.Outcome || progress.RunID != run.ID || !ports.SameExecutionScopeV2(run.Scope, progress.ExecutionScope) {
		return RunTerminationReportV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationReportIncomplete, "terminal Run, Decision and Progress do not match")
	}
	decisionRef, err := decision.RefV2()
	if err != nil {
		return RunTerminationReportV2{}, err
	}
	if progress.Decision != decisionRef {
		return RunTerminationReportV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationReportIncomplete, "termination progress does not bind decision")
	}
	progressRef, err := progress.RefV2()
	if err != nil {
		return RunTerminationReportV2{}, err
	}
	runIdentity, err := ports.RunIdentityDigestV2(run)
	if err != nil {
		return RunTerminationReportV2{}, err
	}
	for _, item := range progress.Items {
		if item.Disposition == ports.RunSettlementUnknown {
			return RunTerminationReportV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationReportIncomplete, "termination report requirement remains unknown")
		}
	}
	items := append([]RunSettlementResolutionV2{}, progress.Items...)
	identity, _ := core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunTerminationReportIdentityV2", struct {
		RunID    core.AgentRunID             `json:"run_id"`
		Progress RunTerminationProgressRefV2 `json:"progress"`
	}{run.ID, progressRef})
	return RunTerminationReportV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: RunSettlementFactIDV2("termination-report", run.ID, identity), Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScope: run.Scope, ExecutionScopeDigest: progress.ExecutionScopeDigest, Decision: decisionRef, Progress: progressRef, Outcome: run.Outcome, Items: items, CompletedUnixNano: progress.UpdatedUnixNano}, nil
}

func (r RunTerminationReportV2) Validate() error {
	if r.ContractVersion != ports.RunSettlementContractVersionV2 || strings.TrimSpace(r.ID) == "" || r.Revision != 1 || strings.TrimSpace(string(r.RunID)) == "" || !validRunOutcomeV2(r.Outcome) || r.CompletedUnixNano <= 0 || len(r.Items) == 0 || len(r.Items) > ports.MaxRunSettlementRequirementsV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonTerminationReportIncomplete, "termination report is incomplete")
	}
	if err := r.Decision.Validate(); err != nil {
		return err
	}
	if err := r.Progress.Validate(); err != nil {
		return err
	}
	if err := r.RunIdentityDigest.Validate(); err != nil {
		return err
	}
	if err := r.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(r.ExecutionScope)
	if err != nil || scopeDigest != r.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationReportIncomplete, "termination report scope drifted")
	}
	previous := ""
	for index, item := range r.Items {
		if err := item.Validate(); err != nil || item.Phase != ports.RunSettlementPhaseTerminationReport || item.Disposition == ports.RunSettlementUnknown {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationReportIncomplete, "termination report contains unresolved item")
		}
		key := string(item.RequirementID)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "termination report items must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func (r RunTerminationReportV2) DigestV2() (core.Digest, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	if r.Items == nil {
		r.Items = []RunSettlementResolutionV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunTerminationReportV2", r)
}

func BindingSetDigestV2(set BindingSetFactV2) (core.Digest, error) {
	if err := set.Validate(); err != nil {
		return "", err
	}
	copy := set
	if copy.Members == nil {
		copy.Members = []BindingMemberV2{}
	}
	if copy.TopologicalOrder == nil {
		copy.TopologicalOrder = []ports.ComponentIDV2{}
	}
	if copy.Residuals == nil {
		copy.Residuals = []BindingResidualV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.binding", ports.BindingContractVersionV2, "BindingSetFactV2", copy)
}

func SortRunEffectRefsV2(values []RunEffectRefV2) {
	sort.Slice(values, func(i, j int) bool { return values[i].EffectID < values[j].EffectID })
}

func SortRunSettlementResolutionsV2(values []RunSettlementResolutionV2) {
	sort.Slice(values, func(i, j int) bool {
		left := string(values[i].Phase) + "\x00" + string(values[i].RequirementID)
		right := string(values[j].Phase) + "\x00" + string(values[j].RequirementID)
		return left < right
	})
}

func RunSettlementFactIDV2(prefix string, runID core.AgentRunID, digest core.Digest) string {
	identity, err := core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunSettlementFactIdentityV2", struct {
		Prefix string          `json:"prefix"`
		RunID  core.AgentRunID `json:"run_id"`
		Digest core.Digest     `json:"digest"`
	}{prefix, runID, digest})
	if err != nil {
		return ""
	}
	return prefix + ":" + string(identity)
}

func validRunOutcomeV2(outcome core.ExecutionOutcome) bool {
	switch outcome {
	case core.OutcomeCompleted, core.OutcomeCancelled, core.OutcomeFailed, core.OutcomeLost, core.OutcomeIndeterminate, core.OutcomeNeedsReconciliation:
		return true
	default:
		return false
	}
}

func sameRunSettlementResolutionV2(left, right RunSettlementResolutionV2) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunSettlementResolutionV2", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunSettlementResolutionV2", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}
