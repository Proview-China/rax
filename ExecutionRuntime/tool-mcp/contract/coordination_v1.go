package contract

import (
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type SingleCallCoordinationStageV1 string

const (
	CoordinationRequestRecordedV1     SingleCallCoordinationStageV1 = "request_recorded"
	CoordinationCandidateRecordedV1   SingleCallCoordinationStageV1 = "candidate_recorded"
	CoordinationReservationRecordedV1 SingleCallCoordinationStageV1 = "reservation_recorded"
	CoordinationRuntimeAttemptBoundV1 SingleCallCoordinationStageV1 = "runtime_attempt_bound"
	CoordinationProviderBoundaryV1    SingleCallCoordinationStageV1 = "provider_boundary_crossed"
	CoordinationProviderObservedV1    SingleCallCoordinationStageV1 = "provider_observed"
	CoordinationDomainResultV1        SingleCallCoordinationStageV1 = "domain_result_recorded"
	CoordinationSettlementAppliedV1   SingleCallCoordinationStageV1 = "settlement_applied"
	CoordinationResultSettledV1       SingleCallCoordinationStageV1 = "result_settled"
)

type SingleCallCanonicalCommandV1 struct {
	TenantID                   core.TenantID                                  `json:"tenant_id"`
	ApplicationRequestID       string                                         `json:"application_request_id"`
	ApplicationRequestRevision core.Revision                                  `json:"application_request_revision"`
	ApplicationRequestDigest   core.Digest                                    `json:"application_request_digest"`
	ActionCoordinateDigest     core.Digest                                    `json:"action_coordinate_digest"`
	OperationScopeDigest       core.Digest                                    `json:"operation_scope_digest"`
	ModelProjection            modelinvoker.ToolCallCandidateObservationRefV1 `json:"model_projection"`
	ObservationDigest          core.Digest                                    `json:"observation_digest"`
	CallID                     string                                         `json:"call_id"`
	CallName                   string                                         `json:"call_name"`
	CanonicalArgumentsDigest   core.Digest                                    `json:"canonical_arguments_digest"`
	PendingAction              PendingActionExactRefV2                        `json:"pending_action"`
	RunID                      string                                         `json:"run_id"`
	SessionID                  string                                         `json:"session_id"`
	TurnID                     string                                         `json:"turn_id"`
	ActionID                   string                                         `json:"action_id"`
	ActionCandidate            ObjectRef                                      `json:"action_candidate"`
	Capability                 ObjectRef                                      `json:"capability"`
	Tool                       ObjectRef                                      `json:"tool"`
	InputSchema                runtimeports.SchemaRefV2                       `json:"input_schema"`
	SourceCandidate            ObjectRef                                      `json:"source_candidate"`
	PayloadDigest              core.Digest                                    `json:"payload_digest"`
	Provider                   runtimeports.ProviderBindingRefV2              `json:"provider"`
	EffectKind                 runtimeports.EffectKindV2                      `json:"effect_kind"`
	PolicyProfile              runtimeports.NamespacedNameV2                  `json:"policy_profile"`
}

func (c SingleCallCanonicalCommandV1) Validate() error {
	if strings.TrimSpace(string(c.TenantID)) == "" || strings.TrimSpace(c.ApplicationRequestID) == "" || len(c.ApplicationRequestID) > MaxStringBytes || c.ApplicationRequestRevision == 0 || c.ApplicationRequestDigest.Validate() != nil || c.ActionCoordinateDigest.Validate() != nil || c.OperationScopeDigest.Validate() != nil || c.ModelProjection.Validate() != nil || c.ObservationDigest.Validate() != nil || strings.TrimSpace(c.CallID) == "" || strings.TrimSpace(c.CallName) == "" || c.CanonicalArgumentsDigest.Validate() != nil || c.PendingAction.Validate() != nil || strings.TrimSpace(c.RunID) == "" || strings.TrimSpace(c.SessionID) == "" || strings.TrimSpace(c.TurnID) == "" || ValidateStableID(c.ActionID) != nil || c.ActionCandidate.Validate() != nil || c.Capability.Validate() != nil || c.Tool.Validate() != nil || c.InputSchema.Validate() != nil || c.SourceCandidate.Validate() != nil || c.PayloadDigest.Validate() != nil || c.Provider.Validate() != nil || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(c.EffectKind)) != nil || runtimeports.ValidateNamespacedNameV2(c.PolicyProfile) != nil {
		return invalid("single-call canonical command is incomplete")
	}
	if c.ModelProjection.ObservationDigest != c.ObservationDigest {
		return conflict("canonical command Model observation digest drifted")
	}
	if c.ActionCandidate.ID != c.ActionID {
		return conflict("canonical command ActionCandidate identity drifted")
	}
	if c.CanonicalArgumentsDigest != c.PayloadDigest {
		return conflict("canonical command Model arguments and PendingAction payload digests drifted")
	}
	if string(c.Provider.Capability) != string(c.EffectKind) {
		return conflict("canonical command Provider capability drifted from Effect kind")
	}
	if c.EffectKind != "praxis.tool/execute" || c.PolicyProfile != "praxis.tool/single-call-action-v1" {
		return invalid("single-call command uses unsupported Effect kind or policy profile")
	}
	return nil
}

func (c SingleCallCanonicalCommandV1) DigestV1() (core.Digest, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	return Seal("praxis.tool-mcp.single-call-command", CoordinationContractVersionV1, "SingleCallCanonicalCommandV1", c)
}

type SingleCallToolActionCoordinationWatermarkV1 struct {
	ContractVersion            string                                                   `json:"contract_version"`
	ID                         string                                                   `json:"id"`
	Revision                   core.Revision                                            `json:"revision"`
	Digest                     core.Digest                                              `json:"digest"`
	TenantID                   core.TenantID                                            `json:"tenant_id"`
	ApplicationRequestID       string                                                   `json:"application_request_id"`
	ApplicationRequestRevision core.Revision                                            `json:"application_request_revision"`
	ApplicationRequestDigest   core.Digest                                              `json:"application_request_digest"`
	OperationScopeDigest       core.Digest                                              `json:"operation_scope_digest"`
	ModelProjection            modelinvoker.ToolCallCandidateObservationRefV1           `json:"model_projection"`
	ObservationDigest          core.Digest                                              `json:"observation_digest"`
	CanonicalCommandDigest     core.Digest                                              `json:"canonical_command_digest"`
	Stage                      SingleCallCoordinationStageV1                            `json:"stage"`
	Owner                      runtimeports.EffectOwnerRefV2                            `json:"owner"`
	ActionCandidate            *ObjectRef                                               `json:"action_candidate,omitempty"`
	Reservation                *ObjectRef                                               `json:"reservation,omitempty"`
	RuntimeAttempt             *runtimeports.OperationDispatchAttemptRefV3              `json:"runtime_attempt,omitempty"`
	Operation                  *runtimeports.OperationSubjectV3                         `json:"operation,omitempty"`
	OperationDigest            core.Digest                                              `json:"operation_digest,omitempty"`
	ExecuteEnforcement         *runtimeports.OperationDispatchEnforcementPhaseRefV4     `json:"execute_enforcement,omitempty"`
	ExecuteHandoff             *runtimeports.OperationScopeEvidenceProviderHandoffRefV3 `json:"execute_handoff,omitempty"`
	ProviderObservation        *runtimeports.ProviderAttemptObservationRefV2            `json:"provider_observation,omitempty"`
	DomainResult               *ObjectRef                                               `json:"domain_result,omitempty"`
	Apply                      *ObjectRef                                               `json:"apply,omitempty"`
	Result                     *ObjectRef                                               `json:"result,omitempty"`
	CreatedUnixNano            int64                                                    `json:"created_unix_nano"`
	UpdatedUnixNano            int64                                                    `json:"updated_unix_nano"`
	ExpiresUnixNano            int64                                                    `json:"expires_unix_nano"`
}

func (w SingleCallToolActionCoordinationWatermarkV1) validateShape() error {
	if w.ContractVersion != CoordinationContractVersionV1 || ValidateStableID(w.ID) != nil || w.Revision == 0 || strings.TrimSpace(string(w.TenantID)) == "" || strings.TrimSpace(w.ApplicationRequestID) == "" || len(w.ApplicationRequestID) > MaxStringBytes || w.ApplicationRequestRevision == 0 || w.ApplicationRequestDigest.Validate() != nil || w.OperationScopeDigest.Validate() != nil || w.ModelProjection.Validate() != nil || w.ObservationDigest.Validate() != nil || w.CanonicalCommandDigest.Validate() != nil || validateEffectOwner(w.Owner) != nil || w.CreatedUnixNano <= 0 || w.UpdatedUnixNano < w.CreatedUnixNano || w.ExpiresUnixNano <= w.UpdatedUnixNano {
		return invalid("single-call coordination watermark is incomplete")
	}
	if w.ModelProjection.ObservationDigest != w.ObservationDigest {
		return conflict("watermark Model observation digest drifted")
	}
	rank := coordinationStageRank(w.Stage)
	if rank < 0 {
		return invalid("watermark stage is invalid")
	}
	if (rank >= 1) != (w.ActionCandidate != nil) || (rank >= 2) != (w.Reservation != nil) || (rank >= 3) != (w.RuntimeAttempt != nil) || (rank >= 3) != (w.Operation != nil) || (rank >= 3) != (w.OperationDigest != "") || (rank >= 4) != (w.ExecuteEnforcement != nil) || (rank >= 4) != (w.ExecuteHandoff != nil) || (rank >= 5) != (w.ProviderObservation != nil) || (rank >= 6) != (w.DomainResult != nil) || (rank >= 7) != (w.Apply != nil) || (rank >= 8) != (w.Result != nil) {
		return conflict("watermark stage and exact refs are not monotonic")
	}
	for _, r := range []*ObjectRef{w.ActionCandidate, w.Reservation, w.DomainResult, w.Apply, w.Result} {
		if r != nil && r.Validate() != nil {
			return invalid("watermark contains invalid Tool exact ref")
		}
	}
	if rank >= 3 {
		if w.RuntimeAttempt.Validate() != nil || w.Operation.Validate() != nil {
			return invalid("watermark Runtime attempt is invalid")
		}
		d, e := w.Operation.DigestV3()
		if e != nil || d != w.OperationDigest || w.RuntimeAttempt.OperationDigest != d {
			return conflict("watermark Runtime attempt operation drifted")
		}
	}
	if rank >= 4 {
		if w.ExecuteEnforcement.Validate() != nil || w.ExecuteHandoff.Validate() != nil || w.ExecuteEnforcement.Phase != runtimeports.OperationDispatchEnforcementExecuteV4 || w.ExecuteEnforcement.AttemptID != w.RuntimeAttempt.AttemptID || w.ExecuteEnforcement.OperationDigest != w.RuntimeAttempt.OperationDigest || w.ExecuteEnforcement.EffectID != w.RuntimeAttempt.EffectID || w.ExpiresUnixNano > w.ExecuteEnforcement.ExpiresUnixNano || w.ExpiresUnixNano > runtimeports.OperationScopeEvidenceFactRefV3(*w.ExecuteHandoff).ExpiresUnixNano {
			return conflict("watermark provider boundary bindings drifted")
		}
	}
	if rank >= 5 && w.ProviderObservation.Validate() != nil {
		return invalid("watermark Provider Observation is invalid")
	}
	return nil
}

func (w SingleCallToolActionCoordinationWatermarkV1) ComputeDigest() (core.Digest, error) {
	if err := w.validateShape(); err != nil {
		return "", err
	}
	w.Digest = ""
	return Seal("praxis.tool-mcp.single-call-coordination", CoordinationContractVersionV1, "SingleCallToolActionCoordinationWatermarkV1", w)
}
func (w SingleCallToolActionCoordinationWatermarkV1) Validate() error {
	if err := w.validateShape(); err != nil {
		return err
	}
	d, e := w.ComputeDigest()
	if e != nil || w.Digest.Validate() != nil || d != w.Digest {
		return conflict("coordination watermark digest drifted")
	}
	return nil
}
func SealCoordinationWatermarkV1(w SingleCallToolActionCoordinationWatermarkV1) (SingleCallToolActionCoordinationWatermarkV1, error) {
	w.ContractVersion = CoordinationContractVersionV1
	w.Digest = ""
	d, e := w.ComputeDigest()
	if e != nil {
		return SingleCallToolActionCoordinationWatermarkV1{}, e
	}
	w.Digest = d
	return w, w.Validate()
}

func coordinationStageRank(s SingleCallCoordinationStageV1) int {
	switch s {
	case CoordinationRequestRecordedV1:
		return 0
	case CoordinationCandidateRecordedV1:
		return 1
	case CoordinationReservationRecordedV1:
		return 2
	case CoordinationRuntimeAttemptBoundV1:
		return 3
	case CoordinationProviderBoundaryV1:
		return 4
	case CoordinationProviderObservedV1:
		return 5
	case CoordinationDomainResultV1:
		return 6
	case CoordinationSettlementAppliedV1:
		return 7
	case CoordinationResultSettledV1:
		return 8
	default:
		return -1
	}
}

type ToolProviderBoundarySourceRefV1 struct {
	WatermarkID       string        `json:"watermark_id"`
	WatermarkRevision core.Revision `json:"watermark_revision"`
	WatermarkDigest   core.Digest   `json:"watermark_digest"`
}

func (r ToolProviderBoundarySourceRefV1) Validate() error {
	if ValidateStableID(r.WatermarkID) != nil || r.WatermarkRevision == 0 || r.WatermarkDigest.Validate() != nil {
		return invalid("Tool provider boundary source ref is invalid")
	}
	return nil
}
func (r ToolProviderBoundarySourceRefV1) RuntimeRefV1() (runtimeports.OperationProviderBoundaryRefV1, error) {
	if err := r.Validate(); err != nil {
		return runtimeports.OperationProviderBoundaryRefV1{}, err
	}
	v := runtimeports.OperationProviderBoundaryRefV1{ID: r.WatermarkID, Revision: r.WatermarkRevision, Digest: r.WatermarkDigest}
	return v, v.Validate()
}

func (w SingleCallToolActionCoordinationWatermarkV1) BoundarySourceRefV1() (ToolProviderBoundarySourceRefV1, error) {
	if w.Validate() != nil || coordinationStageRank(w.Stage) < coordinationStageRank(CoordinationProviderBoundaryV1) {
		return ToolProviderBoundarySourceRefV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "watermark has not crossed the Provider boundary")
	}
	return ToolProviderBoundarySourceRefV1{WatermarkID: w.ID, WatermarkRevision: w.Revision, WatermarkDigest: w.Digest}, nil
}

func MinUnixNanoV1(values ...int64) int64 {
	minimum := int64(0)
	for _, v := range values {
		if v > 0 && (minimum == 0 || v < minimum) {
			minimum = v
		}
	}
	return minimum
}
func IsCoordinationCurrentV1(w SingleCallToolActionCoordinationWatermarkV1, now time.Time) bool {
	return !now.IsZero() && now.UnixNano() >= w.CreatedUnixNano && now.UnixNano() < w.ExpiresUnixNano
}
