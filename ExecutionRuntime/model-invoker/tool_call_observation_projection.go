package modelinvoker

import (
	"encoding/json"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ToolCallCandidateObservationProjectionContractVersionV1 = "praxis.model-invoker.tool-call-observation-projection/v1"
	ToolCallCandidateObservationModelEventKindV1            = "model_tool_call_candidate_observation"
	ToolCallCompatibilityAuthorityV1                        = "compatibility_only_not_gateway_authority"
)

// ToolCallCandidateObservationSourceCoordinateV1 is the immutable Model
// Invoker source coordinate later carried by the public execution-union event.
// It is evidence lineage only and grants no PendingAction or dispatch authority.
type ToolCallCandidateObservationSourceCoordinateV1 struct {
	SourceSequence uint64 `json:"source_sequence"`
	ResponseID     string `json:"response_id,omitempty"`
}

// ToolCallCandidateObservationRefV1 is an exact content-and-source reference
// to one immutable observation projection. Digest covers every field except
// itself, including the observation content digest and source coordinate.
type ToolCallCandidateObservationRefV1 struct {
	ID                string                                         `json:"id"`
	Revision          core.Revision                                  `json:"revision"`
	Digest            core.Digest                                    `json:"digest"`
	InvocationID      string                                         `json:"invocation_id"`
	InvocationDigest  core.Digest                                    `json:"invocation_digest"`
	ObservationDigest core.Digest                                    `json:"observation_digest"`
	Source            ToolCallCandidateObservationSourceCoordinateV1 `json:"source"`
}

// ToolCallCandidateObservationProjectionV1 is the one authoritative public
// union payload for a finalized batch. Individual model_tool_call events are
// compatibility projections only and cannot replace this value or its ref.
type ToolCallCandidateObservationProjectionV1 struct {
	ContractVersion string                            `json:"contract_version"`
	Ref             ToolCallCandidateObservationRefV1 `json:"ref"`
	Observation     ToolCallCandidateObservationV1    `json:"observation"`
}

func (p ToolCallCandidateObservationProjectionV1) Clone() ToolCallCandidateObservationProjectionV1 {
	clone := p
	clone.Observation = p.Observation.Clone()
	return clone
}

func (r ToolCallCandidateObservationRefV1) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision != 1 || strings.TrimSpace(r.InvocationID) == "" || r.Source.SourceSequence == 0 {
		return toolCallObservationError(ErrorInvalidRequest, "tool_call_observation_ref_invalid", "tool call observation ref identity or source coordinate is invalid")
	}
	if r.InvocationDigest.Validate() != nil || r.ObservationDigest.Validate() != nil || r.Digest.Validate() != nil {
		return toolCallObservationError(ErrorInvalidRequest, "tool_call_observation_ref_digest_invalid", "tool call observation ref contains an invalid digest")
	}
	expected, err := digestToolCallCandidateObservationRefV1(r)
	if err != nil {
		return err
	}
	if expected != r.Digest {
		return toolCallObservationError(ErrorMapping, "tool_call_observation_ref_digest_drift", "tool call observation ref digest drifted")
	}
	return nil
}

func (p ToolCallCandidateObservationProjectionV1) Validate() error {
	if p.ContractVersion != ToolCallCandidateObservationProjectionContractVersionV1 {
		return toolCallObservationError(ErrorInvalidRequest, "tool_call_observation_projection_version_invalid", "tool call observation projection version is unsupported")
	}
	if err := p.Observation.Validate(); err != nil {
		return err
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if p.Ref.InvocationDigest != p.Observation.InvocationDigest || p.Ref.ObservationDigest != p.Observation.Digest {
		return toolCallObservationError(ErrorMapping, "tool_call_observation_projection_lineage_drift", "tool call observation projection ref does not match its observation")
	}
	return nil
}

// NewToolCallCandidateObservationProjectionV1 seals one already-finalized
// observation into a deterministic exact public ref. Repeating the same
// invocation, source coordinate, response ID, and observation is idempotent.
func NewToolCallCandidateObservationProjectionV1(invocationID string, sourceSequence uint64, responseID string, observation ToolCallCandidateObservationV1) (ToolCallCandidateObservationProjectionV1, error) {
	if err := observation.Validate(); err != nil {
		return ToolCallCandidateObservationProjectionV1{}, err
	}
	if strings.TrimSpace(invocationID) == "" || sourceSequence == 0 {
		return ToolCallCandidateObservationProjectionV1{}, toolCallObservationError(ErrorInvalidRequest, "tool_call_observation_source_invalid", "invocation ID and positive source sequence are required")
	}
	ref := ToolCallCandidateObservationRefV1{
		Revision: 1, InvocationID: invocationID, InvocationDigest: observation.InvocationDigest,
		ObservationDigest: observation.Digest,
		Source:            ToolCallCandidateObservationSourceCoordinateV1{SourceSequence: sourceSequence, ResponseID: responseID},
	}
	identityDigest, err := core.CanonicalJSONDigest(
		"praxis.model-invoker.tool-call-observation-projection",
		"v1",
		"ToolCallCandidateObservationIdentityV1",
		struct {
			InvocationID      string                                         `json:"invocation_id"`
			ObservationDigest core.Digest                                    `json:"observation_digest"`
			Source            ToolCallCandidateObservationSourceCoordinateV1 `json:"source"`
		}{InvocationID: invocationID, ObservationDigest: observation.Digest, Source: ref.Source},
	)
	if err != nil {
		return ToolCallCandidateObservationProjectionV1{}, err
	}
	ref.ID = "tool-call-observation/" + strings.TrimPrefix(string(identityDigest), "sha256:")
	ref.Digest, err = digestToolCallCandidateObservationRefV1(ref)
	if err != nil {
		return ToolCallCandidateObservationProjectionV1{}, err
	}
	projection := ToolCallCandidateObservationProjectionV1{
		ContractVersion: ToolCallCandidateObservationProjectionContractVersionV1,
		Ref:             ref, Observation: observation.Clone(),
	}
	return projection.Clone(), projection.Validate()
}

func DecodeToolCallCandidateObservationProjectionV1(payload json.RawMessage) (ToolCallCandidateObservationProjectionV1, error) {
	if len(payload) == 0 || len(payload) > core.MaxCanonicalDocumentBytes {
		return ToolCallCandidateObservationProjectionV1{}, toolCallObservationError(ErrorInvalidRequest, "tool_call_observation_projection_size_invalid", "tool call observation projection payload is empty or too large")
	}
	var projection ToolCallCandidateObservationProjectionV1
	if err := core.DecodeStrictJSON(payload, &projection); err != nil {
		return ToolCallCandidateObservationProjectionV1{}, toolCallObservationError(ErrorMapping, "tool_call_observation_projection_json_invalid", "tool call observation projection payload is invalid")
	}
	return projection.Clone(), projection.Validate()
}

func digestToolCallCandidateObservationRefV1(ref ToolCallCandidateObservationRefV1) (core.Digest, error) {
	copy := ref
	copy.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.tool-call-observation-projection",
		"v1",
		"ToolCallCandidateObservationRefV1",
		copy,
	)
}
