package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const MaxDomainResultCurrentTTLV1 = 30 * time.Second

type ApplicationAttemptRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ApplicationAttemptRefV1) Validate() error {
	if ValidateStableID(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return invalid("application attempt exact ref is invalid")
	}
	return nil
}

type OwnerCurrentRefV1 struct {
	Kind            runtimeports.NamespacedNameV2 `json:"kind"`
	ID              string                        `json:"id"`
	Revision        core.Revision                 `json:"revision"`
	Digest          core.Digest                   `json:"digest"`
	Owner           runtimeports.EffectOwnerRefV2 `json:"owner"`
	CheckedUnixNano int64                         `json:"checked_unix_nano"`
	ExpiresUnixNano int64                         `json:"expires_unix_nano"`
}

func (r OwnerCurrentRefV1) Validate(now time.Time) error {
	if runtimeports.ValidateNamespacedNameV2(r.Kind) != nil || !ValidObjectID(r.ID) || r.Revision == 0 || r.Digest.Validate() != nil || validateOwnerRef(r.Owner) != nil || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano {
		return invalid("owner current exact ref is invalid")
	}
	if now.IsZero() || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "owner current exact ref is expired")
	}
	return nil
}

type PendingActionExactRefV2 struct {
	ID            string        `json:"id"`
	Revision      core.Revision `json:"revision"`
	RequestDigest core.Digest   `json:"request_digest"`
}

func (r PendingActionExactRefV2) Validate() error {
	if !ValidObjectID(r.ID) || r.Revision == 0 || r.RequestDigest.Validate() != nil {
		return invalid("pending action exact ref is invalid")
	}
	return nil
}

type ActionCandidateV2 struct {
	ContractVersion          string                        `json:"contract_version"`
	ID                       string                        `json:"id"`
	Revision                 core.Revision                 `json:"revision"`
	Digest                   core.Digest                   `json:"digest"`
	TenantID                 core.TenantID                 `json:"tenant_id"`
	RunID                    string                        `json:"run_id"`
	SessionID                string                        `json:"session_id"`
	TurnID                   string                        `json:"turn_id"`
	PendingAction            PendingActionExactRefV2       `json:"pending_action"`
	SourceCandidate          ObjectRef                     `json:"source_candidate"`
	Capability               ObjectRef                     `json:"capability"`
	Tool                     ObjectRef                     `json:"tool"`
	InputSchema              runtimeports.SchemaRefV2      `json:"input_schema"`
	Payload                  runtimeports.OpaquePayloadV2  `json:"payload"`
	PayloadRevision          core.Revision                 `json:"payload_revision"`
	OperationScopeDigest     core.Digest                   `json:"operation_scope_digest"`
	EffectKind               runtimeports.EffectKindV2     `json:"effect_kind"`
	ExpectedOwner            runtimeports.EffectOwnerRefV2 `json:"expected_owner"`
	ConflictDomain           string                        `json:"conflict_domain"`
	IdempotencyKey           string                        `json:"idempotency_key"`
	CreatedUnixNano          int64                         `json:"created_unix_nano"`
	RequestedExpiresUnixNano int64                         `json:"requested_expires_unix_nano"`
	PendingActionCurrent     OwnerCurrentRefV1             `json:"pending_action_current"`
	SurfaceCurrent           OwnerCurrentRefV1             `json:"surface_current"`
	CapabilityCurrent        OwnerCurrentRefV1             `json:"capability_current"`
	ToolCurrent              OwnerCurrentRefV1             `json:"tool_current"`
	InputSchemaCurrent       OwnerCurrentRefV1             `json:"input_schema_current"`
	SourceCandidateCurrent   OwnerCurrentRefV1             `json:"source_candidate_current"`
}

func (c ActionCandidateV2) CurrentExpiresUnixNano() int64 {
	values := []int64{c.RequestedExpiresUnixNano, c.PendingActionCurrent.ExpiresUnixNano, c.SurfaceCurrent.ExpiresUnixNano, c.CapabilityCurrent.ExpiresUnixNano, c.ToolCurrent.ExpiresUnixNano, c.InputSchemaCurrent.ExpiresUnixNano, c.SourceCandidateCurrent.ExpiresUnixNano}
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func (c ActionCandidateV2) validateShape() error {
	if c.ContractVersion != ActionContractVersionV2 || ValidateStableID(c.ID) != nil || c.Revision != 1 || strings.TrimSpace(string(c.TenantID)) == "" || strings.TrimSpace(c.RunID) == "" || strings.TrimSpace(c.SessionID) == "" || strings.TrimSpace(c.TurnID) == "" || c.PendingAction.Validate() != nil || c.SourceCandidate.Validate() != nil || c.Capability.Validate() != nil || c.Tool.Validate() != nil || c.InputSchema.Validate() != nil || c.Payload.Validate() != nil || c.PayloadRevision == 0 || c.OperationScopeDigest.Validate() != nil || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(c.EffectKind)) != nil || validateEffectOwner(c.ExpectedOwner) != nil || strings.TrimSpace(c.ConflictDomain) == "" || strings.TrimSpace(c.IdempotencyKey) == "" || c.CreatedUnixNano <= 0 || c.RequestedExpiresUnixNano <= c.CreatedUnixNano {
		return invalid("V2 action candidate identity or typed bindings are invalid")
	}
	if c.Payload.Schema != c.InputSchema {
		return conflict("candidate payload schema differs from exact input schema")
	}
	now := time.Unix(0, c.CreatedUnixNano)
	refs := []OwnerCurrentRefV1{c.PendingActionCurrent, c.SurfaceCurrent, c.CapabilityCurrent, c.ToolCurrent, c.InputSchemaCurrent, c.SourceCandidateCurrent}
	for _, ref := range refs {
		if err := ref.Validate(now); err != nil {
			return err
		}
	}
	if c.PendingActionCurrent.ID != c.PendingAction.ID || c.PendingActionCurrent.Revision != c.PendingAction.Revision || c.PendingActionCurrent.Digest != c.PendingAction.RequestDigest || c.CapabilityCurrent.ID != c.Capability.ID || c.CapabilityCurrent.Revision != c.Capability.Revision || c.CapabilityCurrent.Digest != c.Capability.Digest || c.ToolCurrent.ID != c.Tool.ID || c.ToolCurrent.Revision != c.Tool.Revision || c.ToolCurrent.Digest != c.Tool.Digest || c.SourceCandidateCurrent.ID != c.SourceCandidate.ID || c.SourceCandidateCurrent.Revision != c.SourceCandidate.Revision || c.SourceCandidateCurrent.Digest != c.SourceCandidate.Digest || c.InputSchemaCurrent.Digest != c.InputSchema.ContentDigest {
		return conflict("candidate exact refs drifted from owner-current projections")
	}
	if c.CurrentExpiresUnixNano() <= c.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "candidate current window is empty")
	}
	return nil
}

func (c ActionCandidateV2) ComputeDigest() (core.Digest, error) {
	if err := c.validateShape(); err != nil {
		return "", err
	}
	c.Digest = ""
	return Seal("praxis.tool-mcp.action", ActionContractVersionV2, "ActionCandidateV2", c)
}
func (c ActionCandidateV2) Validate() error {
	if err := c.validateShape(); err != nil {
		return err
	}
	expected, err := c.ComputeDigest()
	if err != nil || c.Digest.Validate() != nil || expected != c.Digest {
		return conflict("V2 action candidate digest drifted")
	}
	return nil
}
func SealActionCandidateV2(c ActionCandidateV2) (ActionCandidateV2, error) {
	c.ContractVersion = ActionContractVersionV2
	c.Revision = 1
	c.Digest = ""
	d, e := c.ComputeDigest()
	if e != nil {
		return ActionCandidateV2{}, e
	}
	c.Digest = d
	return c, c.Validate()
}

type ActionReservationFactV2 struct {
	ContractVersion     string                  `json:"contract_version"`
	ID                  string                  `json:"id"`
	Revision            core.Revision           `json:"revision"`
	Digest              core.Digest             `json:"digest"`
	TenantID            core.TenantID           `json:"tenant_id"`
	Action              ObjectRef               `json:"action"`
	ApplicationAttempt  ApplicationAttemptRefV1 `json:"application_attempt"`
	IntentDigest        core.Digest             `json:"intent_digest"`
	SessionRef          string                  `json:"session_ref"`
	DomainSubjectDigest core.Digest             `json:"domain_subject_digest"`
	ReservedUnixNano    int64                   `json:"reserved_unix_nano"`
	ExpiresUnixNano     int64                   `json:"expires_unix_nano"`
}

func (f ActionReservationFactV2) validateShape() error {
	if f.ContractVersion != ActionContractVersionV2 || ValidateStableID(f.ID) != nil || f.Revision != 1 || strings.TrimSpace(string(f.TenantID)) == "" || f.Action.Validate() != nil || f.ApplicationAttempt.Validate() != nil || f.IntentDigest.Validate() != nil || strings.TrimSpace(f.SessionRef) == "" || f.DomainSubjectDigest.Validate() != nil || f.ReservedUnixNano <= 0 || f.ExpiresUnixNano <= f.ReservedUnixNano {
		return invalid("V2 reservation is invalid")
	}
	return nil
}
func (f ActionReservationFactV2) ComputeDigest() (core.Digest, error) {
	if err := f.validateShape(); err != nil {
		return "", err
	}
	f.Digest = ""
	return Seal("praxis.tool-mcp.action", ActionContractVersionV2, "ActionReservationFactV2", f)
}
func (f ActionReservationFactV2) Validate() error {
	if err := f.validateShape(); err != nil {
		return err
	}
	d, e := f.ComputeDigest()
	if e != nil || f.Digest.Validate() != nil || d != f.Digest {
		return conflict("V2 reservation digest drifted")
	}
	return nil
}
func SealActionReservationFactV2(f ActionReservationFactV2) (ActionReservationFactV2, error) {
	f.ContractVersion = ActionContractVersionV2
	f.Revision = 1
	f.Digest = ""
	d, e := f.ComputeDigest()
	if e != nil {
		return ActionReservationFactV2{}, e
	}
	f.Digest = d
	return f, f.Validate()
}

type RuntimeAttemptCausalityV1 struct {
	Reservation        ObjectRef                                  `json:"reservation"`
	ApplicationAttempt ApplicationAttemptRefV1                    `json:"application_attempt"`
	Operation          runtimeports.OperationSubjectV3            `json:"operation"`
	OperationDigest    core.Digest                                `json:"operation_digest"`
	Attempt            runtimeports.OperationDispatchAttemptRefV3 `json:"attempt"`
	EffectID           core.EffectIntentID                        `json:"effect_id"`
	EffectRevision     core.Revision                              `json:"effect_revision"`
	IntentDigest       core.Digest                                `json:"intent_digest"`
	Digest             core.Digest                                `json:"digest"`
}

func (c RuntimeAttemptCausalityV1) validateShape() error {
	if c.Reservation.Validate() != nil || c.ApplicationAttempt.Validate() != nil || c.Operation.Validate() != nil || c.OperationDigest.Validate() != nil || c.Attempt.Validate() != nil || strings.TrimSpace(string(c.EffectID)) == "" || c.EffectRevision == 0 || c.IntentDigest.Validate() != nil {
		return invalid("runtime attempt causality is invalid")
	}
	od, e := c.Operation.DigestV3()
	if e != nil || od != c.OperationDigest || c.Attempt.OperationDigest != od || c.Attempt.EffectID != c.EffectID || c.Attempt.IntentRevision != c.EffectRevision || c.Attempt.IntentDigest != c.IntentDigest {
		return conflict("runtime attempt causality combines different attempts")
	}
	return nil
}
func (c RuntimeAttemptCausalityV1) ComputeDigest() (core.Digest, error) {
	if e := c.validateShape(); e != nil {
		return "", e
	}
	c.Digest = ""
	return Seal("praxis.tool-mcp.action", ActionContractVersionV2, "RuntimeAttemptCausalityV1", c)
}
func (c RuntimeAttemptCausalityV1) Validate() error {
	if e := c.validateShape(); e != nil {
		return e
	}
	d, e := c.ComputeDigest()
	if e != nil || c.Digest.Validate() != nil || d != c.Digest {
		return conflict("runtime attempt causality digest drifted")
	}
	return nil
}
func SealRuntimeAttemptCausalityV1(c RuntimeAttemptCausalityV1) (RuntimeAttemptCausalityV1, error) {
	c.Digest = ""
	d, e := c.ComputeDigest()
	if e != nil {
		return RuntimeAttemptCausalityV1{}, e
	}
	c.Digest = d
	return c, c.Validate()
}

type ToolOutcomeV2 string

const (
	ToolOutcomeSucceededV2 ToolOutcomeV2 = "succeeded"
	ToolOutcomeFailedV2    ToolOutcomeV2 = "failed"
)

type ToolDispositionV2 string

const (
	ToolDispositionConfirmedAppliedV2    ToolDispositionV2 = "confirmed_applied"
	ToolDispositionConfirmedNotAppliedV2 ToolDispositionV2 = "confirmed_not_applied"
)

func ValidateToolOutcomeDispositionV2(o ToolOutcomeV2, d ToolDispositionV2) error {
	if o == ToolOutcomeSucceededV2 && d == ToolDispositionConfirmedAppliedV2 {
		return nil
	}
	if o == ToolOutcomeFailedV2 && (d == ToolDispositionConfirmedAppliedV2 || d == ToolDispositionConfirmedNotAppliedV2) {
		return nil
	}
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "illegal Tool outcome/disposition combination")
}

type ToolDomainResultFactV2 struct {
	ContractVersion      string                                              `json:"contract_version"`
	ID                   string                                              `json:"id"`
	Revision             core.Revision                                       `json:"revision"`
	Digest               core.Digest                                         `json:"digest"`
	TenantID             core.TenantID                                       `json:"tenant_id"`
	OperationScopeDigest core.Digest                                         `json:"operation_scope_digest"`
	Action               ObjectRef                                           `json:"action"`
	Reservation          ObjectRef                                           `json:"reservation"`
	ApplicationAttempt   ApplicationAttemptRefV1                             `json:"application_attempt"`
	Causality            RuntimeAttemptCausalityV1                           `json:"causality"`
	PreparedAttempt      runtimeports.PreparedProviderAttemptRefV2           `json:"prepared_attempt"`
	Observation          runtimeports.ProviderAttemptObservationRefV2        `json:"observation"`
	PrepareEnforcement   runtimeports.OperationDispatchEnforcementPhaseRefV4 `json:"prepare_enforcement"`
	ExecuteEnforcement   runtimeports.OperationDispatchEnforcementPhaseRefV4 `json:"execute_enforcement"`
	PrepareConsumption   runtimeports.OperationScopeEvidenceConsumptionRefV3 `json:"prepare_consumption"`
	ExecuteConsumption   runtimeports.OperationScopeEvidenceConsumptionRefV3 `json:"execute_consumption"`
	Schema               runtimeports.SchemaRefV2                            `json:"schema"`
	PayloadDigest        core.Digest                                         `json:"payload_digest"`
	PayloadRevision      core.Revision                                       `json:"payload_revision"`
	Residuals            []Residual                                          `json:"residuals,omitempty"`
	Owner                runtimeports.EffectOwnerRefV2                       `json:"owner"`
	Outcome              ToolOutcomeV2                                       `json:"outcome"`
	Disposition          ToolDispositionV2                                   `json:"disposition"`
	CreatedUnixNano      int64                                               `json:"created_unix_nano"`
}

func (f ToolDomainResultFactV2) validateShape() error {
	if f.ContractVersion != ResultContractVersionV2 || ValidateStableID(f.ID) != nil || f.Revision != 1 || strings.TrimSpace(string(f.TenantID)) == "" || f.OperationScopeDigest.Validate() != nil || f.Action.Validate() != nil || f.Reservation.Validate() != nil || f.ApplicationAttempt.Validate() != nil || f.Causality.Validate() != nil || f.PreparedAttempt.Validate() != nil || f.Observation.Validate() != nil || f.PrepareEnforcement.Validate() != nil || f.ExecuteEnforcement.Validate() != nil || f.PrepareConsumption.Validate() != nil || f.ExecuteConsumption.Validate() != nil || f.Schema.Validate() != nil || f.PayloadDigest.Validate() != nil || f.PayloadRevision == 0 || len(f.Residuals) > MaxResiduals || validateEffectOwner(f.Owner) != nil || ValidateToolOutcomeDispositionV2(f.Outcome, f.Disposition) != nil || f.CreatedUnixNano <= 0 {
		return invalid("V2 Tool DomainResult fact is invalid")
	}
	if f.PrepareEnforcement.Phase != runtimeports.OperationDispatchEnforcementPrepareV4 || f.ExecuteEnforcement.Phase != runtimeports.OperationDispatchEnforcementExecuteV4 {
		return conflict("DomainResult requires distinct prepare and execute enforcement")
	}
	a := f.Causality.Attempt
	if f.PrepareEnforcement.AttemptID != a.AttemptID || f.ExecuteEnforcement.AttemptID != a.AttemptID || f.PrepareEnforcement.OperationDigest != a.OperationDigest || f.ExecuteEnforcement.OperationDigest != a.OperationDigest || f.PrepareEnforcement.EffectID != a.EffectID || f.ExecuteEnforcement.EffectID != a.EffectID {
		return conflict("DomainResult enforcement belongs to another attempt")
	}
	if f.Causality.Reservation != f.Reservation || f.Causality.ApplicationAttempt != f.ApplicationAttempt {
		return conflict("DomainResult causality differs from reservation chain")
	}
	if a.Delegation == nil || f.Observation.Delegation != *a.Delegation || f.PreparedAttempt.DeclaredDelegation.ID != f.Observation.Delegation.ID || f.PreparedAttempt.DeclaredDelegation.Revision >= f.Observation.Delegation.Revision || f.Observation.PreparedAttemptID != f.PreparedAttempt.ID || f.ExecuteEnforcement.PreparedAttemptDigest != f.PreparedAttempt.Digest || f.PreparedAttempt.OperationDigest != a.OperationDigest || f.PreparedAttempt.IntentID != a.EffectID || f.PreparedAttempt.IntentRevision != a.IntentRevision || f.PreparedAttempt.IntentDigest != a.IntentDigest || f.PreparedAttempt.PermitID != a.PermitID || f.PreparedAttempt.PermitRevision != a.PermitRevision || f.PreparedAttempt.PermitDigest != a.PermitDigest || f.PreparedAttempt.AttemptID != a.AttemptID {
		return conflict("DomainResult Prepared/Observation facts belong to another Runtime Attempt")
	}
	for _, r := range f.Residuals {
		if e := r.Validate(); e != nil {
			return e
		}
	}
	return nil
}
func (f ToolDomainResultFactV2) ComputeDigest() (core.Digest, error) {
	if e := f.validateShape(); e != nil {
		return "", e
	}
	f.Digest = ""
	if f.Residuals == nil {
		f.Residuals = []Residual{}
	}
	return Seal("praxis.tool-mcp.result", ResultContractVersionV2, "ToolDomainResultFactV2", f)
}
func (f ToolDomainResultFactV2) Validate() error {
	if e := f.validateShape(); e != nil {
		return e
	}
	d, e := f.ComputeDigest()
	if e != nil || f.Digest.Validate() != nil || d != f.Digest {
		return conflict("V2 Tool DomainResult digest drifted")
	}
	return nil
}
func SealToolDomainResultFactV2(f ToolDomainResultFactV2) (ToolDomainResultFactV2, error) {
	f.ContractVersion = ResultContractVersionV2
	f.Revision = 1
	f.Digest = ""
	if f.Residuals == nil {
		f.Residuals = []Residual{}
	}
	d, e := f.ComputeDigest()
	if e != nil {
		return ToolDomainResultFactV2{}, e
	}
	f.Digest = d
	return f, f.Validate()
}

type ToolDomainResultCurrentProjectionV1 struct {
	ContractVersion    string                                              `json:"contract_version"`
	Fact               ObjectRef                                           `json:"fact"`
	CausalityDigest    core.Digest                                         `json:"causality_digest"`
	Observation        runtimeports.ProviderAttemptObservationRefV2        `json:"observation"`
	PrepareEnforcement runtimeports.OperationDispatchEnforcementPhaseRefV4 `json:"prepare_enforcement"`
	ExecuteEnforcement runtimeports.OperationDispatchEnforcementPhaseRefV4 `json:"execute_enforcement"`
	PrepareConsumption runtimeports.OperationScopeEvidenceConsumptionRefV3 `json:"prepare_consumption"`
	ExecuteConsumption runtimeports.OperationScopeEvidenceConsumptionRefV3 `json:"execute_consumption"`
	Owner              runtimeports.EffectOwnerRefV2                       `json:"owner"`
	CheckedUnixNano    int64                                               `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                                               `json:"expires_unix_nano"`
	Digest             core.Digest                                         `json:"digest"`
}

func (p ToolDomainResultCurrentProjectionV1) ComputeDigest() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	return Seal("praxis.tool-mcp.result", ResultContractVersionV2, "ToolDomainResultCurrentProjectionV1", copy)
}
func (p ToolDomainResultCurrentProjectionV1) Validate(now time.Time) error {
	if p.ContractVersion != ResultContractVersionV2 || p.Fact.Validate() != nil || p.CausalityDigest.Validate() != nil || p.Observation.Validate() != nil || p.PrepareEnforcement.Validate() != nil || p.ExecuteEnforcement.Validate() != nil || p.PrepareConsumption.Validate() != nil || p.ExecuteConsumption.Validate() != nil || validateEffectOwner(p.Owner) != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || time.Duration(p.ExpiresUnixNano-p.CheckedUnixNano) > MaxDomainResultCurrentTTLV1 || now.IsZero() || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Tool DomainResult current projection is invalid or expired")
	}
	d, e := p.ComputeDigest()
	if e != nil || p.Digest.Validate() != nil || d != p.Digest {
		return conflict("Tool DomainResult current projection digest drifted")
	}
	return nil
}

type ToolApplySettlementFactV2 struct {
	ContractVersion      string                                          `json:"contract_version"`
	ID                   string                                          `json:"id"`
	Revision             core.Revision                                   `json:"revision"`
	Digest               core.Digest                                     `json:"digest"`
	TenantID             core.TenantID                                   `json:"tenant_id"`
	OperationScopeDigest core.Digest                                     `json:"operation_scope_digest"`
	Action               ObjectRef                                       `json:"action"`
	Reservation          ObjectRef                                       `json:"reservation"`
	DomainResult         ObjectRef                                       `json:"domain_result"`
	Inspection           runtimeports.OperationInspectionSettlementRefV4 `json:"inspection"`
	Outcome              ToolOutcomeV2                                   `json:"outcome"`
	Disposition          ToolDispositionV2                               `json:"disposition"`
	Owner                runtimeports.EffectOwnerRefV2                   `json:"owner"`
	AppliedUnixNano      int64                                           `json:"applied_unix_nano"`
}

func (f ToolApplySettlementFactV2) validateShape() error {
	now := time.Unix(0, f.AppliedUnixNano)
	if f.ContractVersion != ResultContractVersionV2 || ValidateStableID(f.ID) != nil || f.Revision != 1 || strings.TrimSpace(string(f.TenantID)) == "" || f.OperationScopeDigest.Validate() != nil || f.Action.Validate() != nil || f.Reservation.Validate() != nil || f.DomainResult.Validate() != nil || f.Inspection.Validate(now) != nil || ValidateToolOutcomeDispositionV2(f.Outcome, f.Disposition) != nil || validateEffectOwner(f.Owner) != nil || f.AppliedUnixNano <= 0 {
		return invalid("V2 Tool ApplySettlement fact is invalid")
	}
	return nil
}
func (f ToolApplySettlementFactV2) ComputeDigest() (core.Digest, error) {
	if e := f.validateShape(); e != nil {
		return "", e
	}
	f.Digest = ""
	return Seal("praxis.tool-mcp.result", ResultContractVersionV2, "ToolApplySettlementFactV2", f)
}
func (f ToolApplySettlementFactV2) Validate() error {
	if e := f.validateShape(); e != nil {
		return e
	}
	d, e := f.ComputeDigest()
	if e != nil || f.Digest.Validate() != nil || d != f.Digest {
		return conflict("V2 Tool ApplySettlement digest drifted")
	}
	return nil
}
func SealToolApplySettlementFactV2(f ToolApplySettlementFactV2) (ToolApplySettlementFactV2, error) {
	f.ContractVersion = ResultContractVersionV2
	f.Revision = 1
	f.Digest = ""
	d, e := f.ComputeDigest()
	if e != nil {
		return ToolApplySettlementFactV2{}, e
	}
	f.Digest = d
	return f, f.Validate()
}

type ToolResultV2 struct {
	ContractVersion   string                                          `json:"contract_version"`
	ID                string                                          `json:"id"`
	Revision          core.Revision                                   `json:"revision"`
	Digest            core.Digest                                     `json:"digest"`
	Action            ObjectRef                                       `json:"action"`
	Reservation       ObjectRef                                       `json:"reservation"`
	DomainResult      ObjectRef                                       `json:"domain_result"`
	Apply             ObjectRef                                       `json:"apply"`
	Inspection        runtimeports.OperationInspectionSettlementRefV4 `json:"inspection"`
	Outcome           ToolOutcomeV2                                   `json:"outcome"`
	Disposition       ToolDispositionV2                               `json:"disposition"`
	Schema            runtimeports.SchemaRefV2                        `json:"schema"`
	PayloadDigest     core.Digest                                     `json:"payload_digest"`
	PayloadRevision   core.Revision                                   `json:"payload_revision"`
	Artifacts         []ObjectRef                                     `json:"artifacts,omitempty"`
	Residuals         []Residual                                      `json:"residuals,omitempty"`
	FinalizedUnixNano int64                                           `json:"finalized_unix_nano"`
}

func (r ToolResultV2) validateShape() error {
	if r.ContractVersion != ResultContractVersionV2 || ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Action.Validate() != nil || r.Reservation.Validate() != nil || r.DomainResult.Validate() != nil || r.Apply.Validate() != nil || r.Inspection.Validate(time.Unix(0, r.FinalizedUnixNano)) != nil || ValidateToolOutcomeDispositionV2(r.Outcome, r.Disposition) != nil || r.Schema.Validate() != nil || r.PayloadDigest.Validate() != nil || r.PayloadRevision == 0 || len(r.Artifacts) > MaxResiduals || len(r.Residuals) > MaxResiduals || r.FinalizedUnixNano <= 0 {
		return invalid("V2 ToolResult is invalid")
	}
	for _, a := range r.Artifacts {
		if e := a.Validate(); e != nil {
			return e
		}
	}
	for _, x := range r.Residuals {
		if e := x.Validate(); e != nil {
			return e
		}
	}
	return nil
}
func (r ToolResultV2) ComputeDigest() (core.Digest, error) {
	if e := r.validateShape(); e != nil {
		return "", e
	}
	r.Digest = ""
	if r.Artifacts == nil {
		r.Artifacts = []ObjectRef{}
	}
	if r.Residuals == nil {
		r.Residuals = []Residual{}
	}
	return Seal("praxis.tool-mcp.result", ResultContractVersionV2, "ToolResultV2", r)
}
func (r ToolResultV2) Validate() error {
	if e := r.validateShape(); e != nil {
		return e
	}
	d, e := r.ComputeDigest()
	if e != nil || r.Digest.Validate() != nil || d != r.Digest {
		return conflict("V2 ToolResult digest drifted")
	}
	return nil
}
func SealToolResultV2(r ToolResultV2) (ToolResultV2, error) {
	r.ContractVersion = ResultContractVersionV2
	r.Revision = 1
	r.Digest = ""
	if r.Artifacts == nil {
		r.Artifacts = []ObjectRef{}
	}
	if r.Residuals == nil {
		r.Residuals = []Residual{}
	}
	d, e := r.ComputeDigest()
	if e != nil {
		return ToolResultV2{}, e
	}
	r.Digest = d
	return r, r.Validate()
}

func validateEffectOwner(owner runtimeports.EffectOwnerRefV2) error {
	if owner.Role != runtimeports.OwnerSettlement || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(owner.ComponentID)) != nil || owner.ManifestDigest.Validate() != nil {
		return invalid("exact Tool Settlement Owner is invalid")
	}
	return nil
}

func validateOwnerRef(owner runtimeports.EffectOwnerRefV2) error {
	if (owner.Role != runtimeports.OwnerEffect && owner.Role != runtimeports.OwnerSettlement && owner.Role != runtimeports.OwnerCleanup) || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(owner.ComponentID)) != nil || owner.ManifestDigest.Validate() != nil {
		return invalid("exact Owner ref is invalid")
	}
	return nil
}
