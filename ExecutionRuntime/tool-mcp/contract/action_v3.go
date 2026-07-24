package contract

import (
	"bytes"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ActionContractVersionV3                  = "praxis.tool-mcp.action/v3"
	ModelSourceCandidateHistoricalContractV1 = "praxis.tool.model-source-candidate-historical/v1"
)

const actionCanonicalDomainV3 = "praxis.tool-mcp.action"

// ModelSourceCandidateHistoricalRefV1 is immutable lineage only. It carries
// neither currentness nor authority.
type ModelSourceCandidateHistoricalRefV1 struct {
	ProjectionRef            modelinvoker.ToolCallCandidateObservationRefV1 `json:"projection_ref"`
	CallOrdinal              uint32                                         `json:"call_ordinal"`
	CallID                   string                                         `json:"call_id"`
	CallName                 string                                         `json:"call_name"`
	CanonicalArgumentsDigest core.Digest                                    `json:"canonical_arguments_digest"`
	Digest                   core.Digest                                    `json:"digest"`
}

func (r ModelSourceCandidateHistoricalRefV1) Validate() error {
	if err := r.ProjectionRef.Validate(); err != nil {
		return err
	}
	if r.CallOrdinal != 0 || strings.TrimSpace(r.CallID) == "" || strings.TrimSpace(r.CallName) == "" || r.CanonicalArgumentsDigest.Validate() != nil {
		return invalid("Model Source Candidate historical Ref is invalid")
	}
	digest, err := r.ComputeDigest()
	if err != nil || digest != r.Digest {
		return conflict("Model Source Candidate historical digest drifted")
	}
	return nil
}

func (r ModelSourceCandidateHistoricalRefV1) ComputeDigest() (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest(actionCanonicalDomainV3, ModelSourceCandidateHistoricalContractV1, "ModelSourceCandidateHistoricalRefV1", r)
}

func SealModelSourceCandidateHistoricalRefV1(projection modelinvoker.ToolCallCandidateObservationProjectionV1) (ModelSourceCandidateHistoricalRefV1, error) {
	if err := projection.Validate(); err != nil {
		return ModelSourceCandidateHistoricalRefV1{}, err
	}
	if len(projection.Observation.Calls) != 1 {
		return ModelSourceCandidateHistoricalRefV1{}, conflict("N=1 requires exactly one Model Tool Call")
	}
	call := projection.Observation.Calls[0]
	if call.Ordinal != 0 {
		return ModelSourceCandidateHistoricalRefV1{}, conflict("N=1 Model Tool Call ordinal is not zero")
	}
	result := ModelSourceCandidateHistoricalRefV1{
		ProjectionRef: projection.Ref, CallOrdinal: call.Ordinal, CallID: call.CallID, CallName: call.Name,
		CanonicalArgumentsDigest: core.DigestBytes(call.CanonicalArguments),
	}
	digest, err := result.ComputeDigest()
	if err != nil {
		return ModelSourceCandidateHistoricalRefV1{}, err
	}
	result.Digest = digest
	return result, result.ValidateAgainstProjection(projection)
}

func (r ModelSourceCandidateHistoricalRefV1) ValidateAgainstProjection(projection modelinvoker.ToolCallCandidateObservationProjectionV1) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := projection.Validate(); err != nil {
		return err
	}
	if projection.Ref != r.ProjectionRef || len(projection.Observation.Calls) != 1 {
		return conflict("Model Source Candidate projection exact Ref or cardinality drifted")
	}
	call := projection.Observation.Calls[0]
	if call.Ordinal != r.CallOrdinal || call.CallID != r.CallID || call.Name != r.CallName || core.DigestBytes(call.CanonicalArguments) != r.CanonicalArgumentsDigest {
		return conflict("Model Source Candidate call content drifted")
	}
	return nil
}

type ActionCandidateV3 struct {
	ContractVersion          string                              `json:"contract_version"`
	ID                       string                              `json:"id"`
	Revision                 core.Revision                       `json:"revision"`
	Digest                   core.Digest                         `json:"digest"`
	TenantID                 core.TenantID                       `json:"tenant_id"`
	RunID                    string                              `json:"run_id"`
	SessionID                string                              `json:"session_id"`
	TurnID                   string                              `json:"turn_id"`
	PendingAction            PendingActionExactRefV2             `json:"pending_action"`
	SourceCandidate          ModelSourceCandidateHistoricalRefV1 `json:"source_candidate"`
	Surface                  ObjectRef                           `json:"surface"`
	Capability               ObjectRef                           `json:"capability"`
	Tool                     ObjectRef                           `json:"tool"`
	InputSchema              runtimeports.SchemaRefV2            `json:"input_schema"`
	Payload                  runtimeports.OpaquePayloadV2        `json:"payload"`
	PayloadRevision          core.Revision                       `json:"payload_revision"`
	LimitPolicy              runtimeports.OpaqueLimitPolicyRefV2 `json:"limit_policy"`
	InputContractCurrentRef  ToolInputContractCurrentRefV1       `json:"input_contract_current_ref"`
	SurfaceCurrent           ToolSurfaceManifestCurrentRefV1     `json:"surface_current"`
	CapabilityCurrent        ToolRegistryObjectCurrentRefV1      `json:"capability_current"`
	ToolCurrent              ToolRegistryObjectCurrentRefV1      `json:"tool_current"`
	InputSchemaCurrent       ToolInputSchemaCurrentRefV1         `json:"input_schema_current"`
	OperationScopeDigest     core.Digest                         `json:"operation_scope_digest"`
	EffectKind               runtimeports.EffectKindV2           `json:"effect_kind"`
	ExpectedOwner            runtimeports.EffectOwnerRefV2       `json:"expected_owner"`
	ConflictDomain           string                              `json:"conflict_domain"`
	IdempotencyKey           string                              `json:"idempotency_key"`
	CreatedUnixNano          int64                               `json:"created_unix_nano"`
	RequestedExpiresUnixNano int64                               `json:"requested_expires_unix_nano"`
}

func (c ActionCandidateV3) Validate() error {
	if c.ContractVersion != ActionContractVersionV3 || ValidateStableID(c.ID) != nil || c.Revision != 1 || strings.TrimSpace(string(c.TenantID)) == "" || strings.TrimSpace(c.RunID) == "" || strings.TrimSpace(c.SessionID) == "" || strings.TrimSpace(c.TurnID) == "" || c.PendingAction.Validate() != nil || c.PendingAction.Revision != 1 || c.SourceCandidate.Validate() != nil || c.Surface.Validate() != nil || c.Capability.Validate() != nil || c.Tool.Validate() != nil || c.InputSchema.Validate() != nil || c.Payload.Validate() != nil || c.PayloadRevision != 1 || runtimeports.ValidateNamespacedNameV2(c.LimitPolicy.Policy) != nil || c.LimitPolicy.Digest.Validate() != nil || c.InputContractCurrentRef.Validate() != nil || c.SurfaceCurrent.Validate() != nil || c.CapabilityCurrent.Validate() != nil || c.ToolCurrent.Validate() != nil || c.InputSchemaCurrent.Validate() != nil || c.OperationScopeDigest.Validate() != nil || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(c.EffectKind)) != nil || validateEffectOwner(c.ExpectedOwner) != nil || strings.TrimSpace(c.ConflictDomain) == "" || strings.TrimSpace(c.IdempotencyKey) == "" || c.CreatedUnixNano <= 0 || c.RequestedExpiresUnixNano <= c.CreatedUnixNano {
		return invalid("V3 Action Candidate is invalid")
	}
	if c.EffectKind != runtimeports.OperationScopeEvidenceActionEffectKindV3 || c.Payload.Inline == nil || c.Payload.Ref != "" || c.Payload.Schema != c.InputSchema || c.Payload.LimitPolicy != c.LimitPolicy || c.Payload.ContentDigest != c.SourceCandidate.CanonicalArgumentsDigest || c.SurfaceCurrent.ID != c.Surface.ID || c.SurfaceCurrent.Revision != c.Surface.Revision || c.SurfaceCurrent.Digest != c.Surface.Digest || c.InputSchemaCurrent.InputSchema != c.InputSchema || c.InputSchemaCurrent.Authority != c.ToolCurrent {
		return conflict("V3 Action Candidate exact bindings drifted")
	}
	id, err := DeriveActionCandidateIDV3(c)
	if err != nil || id != c.ID {
		return conflict("V3 Action Candidate stable ID drifted")
	}
	digest, err := c.ComputeDigest()
	if err != nil || digest != c.Digest {
		return conflict("V3 Action Candidate digest drifted")
	}
	return nil
}

func (c ActionCandidateV3) ValidateAgainstInputContract(input ToolInputContractCurrentProjectionV1) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := input.Validate(); err != nil {
		return err
	}
	s := input.BindingSubject
	if c.InputContractCurrentRef != input.Ref || c.PendingAction != s.PendingAction || c.Surface != s.Surface || c.Capability != s.Capability || c.Tool != s.Tool || c.InputSchema != s.InputSchema || c.LimitPolicy != s.LimitPolicy || c.SurfaceCurrent != input.SurfaceCurrent.Ref || c.CapabilityCurrent != input.CapabilityCurrent.Ref || c.ToolCurrent != input.ToolCurrent.Ref || c.InputSchemaCurrent != input.InputSchemaCurrent || c.OperationScopeDigest != s.OperationScopeDigest || c.ExpectedOwner != s.ExpectedOwner || c.Payload.Schema != s.InputSchema || c.Payload.LimitPolicy != s.LimitPolicy {
		return conflict("V3 Action Candidate differs from exact Tool Input Contract")
	}
	return nil
}

func (c ActionCandidateV3) ValidateAgainstModelProjection(projection modelinvoker.ToolCallCandidateObservationProjectionV1) error {
	if err := c.SourceCandidate.ValidateAgainstProjection(projection); err != nil {
		return err
	}
	call := projection.Observation.Calls[0]
	if c.SourceCandidate.CallName != call.Name || !bytes.Equal(c.Payload.Inline, call.CanonicalArguments) || c.Payload.ContentDigest != core.DigestBytes(call.CanonicalArguments) {
		return conflict("V3 Action Candidate payload differs from Model canonical arguments")
	}
	return nil
}

func (c ActionCandidateV3) ComputeDigest() (core.Digest, error) {
	c = CloneActionCandidateV3(c)
	c.Digest = ""
	return core.CanonicalJSONDigest(actionCanonicalDomainV3, ActionContractVersionV3, "ActionCandidateV3", c)
}

func SealActionCandidateV3(c ActionCandidateV3) (ActionCandidateV3, error) {
	c = CloneActionCandidateV3(c)
	c.ContractVersion = ActionContractVersionV3
	c.Revision = 1
	id, err := DeriveActionCandidateIDV3(c)
	if err != nil {
		return ActionCandidateV3{}, err
	}
	if c.ID != "" && c.ID != id {
		return ActionCandidateV3{}, conflict("supplied V3 Action Candidate ID drifted")
	}
	c.ID = id
	provided := c.Digest
	c.Digest = ""
	digest, err := c.ComputeDigest()
	if err != nil {
		return ActionCandidateV3{}, err
	}
	if provided != "" && provided != digest {
		return ActionCandidateV3{}, conflict("supplied V3 Action Candidate digest drifted")
	}
	c.Digest = digest
	return c, c.Validate()
}

func DeriveActionCandidateIDV3(c ActionCandidateV3) (string, error) {
	identity := struct {
		TenantID                core.TenantID                       `json:"tenant_id"`
		RunID                   string                              `json:"run_id"`
		SessionID               string                              `json:"session_id"`
		TurnID                  string                              `json:"turn_id"`
		PendingAction           PendingActionExactRefV2             `json:"pending_action"`
		SourceCandidate         ModelSourceCandidateHistoricalRefV1 `json:"source_candidate"`
		InputContractCurrentRef ToolInputContractCurrentRefV1       `json:"input_contract_current_ref"`
	}{c.TenantID, c.RunID, c.SessionID, c.TurnID, c.PendingAction, c.SourceCandidate, c.InputContractCurrentRef}
	if strings.TrimSpace(string(identity.TenantID)) == "" || strings.TrimSpace(identity.RunID) == "" || strings.TrimSpace(identity.SessionID) == "" || strings.TrimSpace(identity.TurnID) == "" || identity.PendingAction.Validate() != nil || identity.SourceCandidate.Validate() != nil || identity.InputContractCurrentRef.Validate() != nil {
		return "", invalid("V3 Action Candidate identity is invalid")
	}
	digest, err := core.CanonicalJSONDigest(actionCanonicalDomainV3, ActionContractVersionV3, "ActionCandidateV3Identity", identity)
	if err != nil {
		return "", err
	}
	return "tool-action-candidate-v3-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func (c ActionCandidateV3) ObjectRef() ObjectRef {
	return ObjectRef{ID: c.ID, Revision: c.Revision, Digest: c.Digest}
}

func CloneActionCandidateV3(c ActionCandidateV3) ActionCandidateV3 {
	c.Payload.Inline = append([]byte(nil), c.Payload.Inline...)
	return c
}
