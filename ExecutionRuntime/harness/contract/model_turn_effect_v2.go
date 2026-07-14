package contract

import (
	"encoding/json"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ModelTurnEffectPayloadContractV2 = "praxis.harness.model-turn-effect/v2"

// MaxModelTurnEffectEnvelopeBytesV2 is the complete encoded-envelope budget,
// not merely the nested candidate input budget. Candidate validation measures
// the real JSON representation so every accepted candidate is guaranteed to
// fit the Runtime opaque-inline contract after base64 and envelope overhead.
const MaxModelTurnEffectEnvelopeBytesV2 = runtimeports.MaxOpaqueInlineBytes

var (
	modelTurnEffectSchemaDigestV2 = core.DigestBytes([]byte("praxis.harness/model-turn-effect@2.0.0"))
	modelTurnEffectLimitDigestV2  = core.DigestBytes([]byte("praxis.inline/bounded@1.0.0"))
)

// ModelTurnEffectEnvelopeV2 is the deterministic bridge from an immutable
// Harness candidate to one Runtime Operation Effect payload. It grants no
// dispatch authority; Runtime still owns Admission, Permit and Fence facts.
type ModelTurnEffectEnvelopeV2 struct {
	ContractVersion string               `json:"contract_version"`
	Candidate       ModelTurnCandidateV2 `json:"candidate"`
	CandidateDigest core.Digest          `json:"candidate_digest"`
}

func (e ModelTurnEffectEnvelopeV2) Validate(now time.Time) error {
	if e.ContractVersion != ModelTurnEffectPayloadContractV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model turn Effect envelope contract is unsupported")
	}
	if err := e.Candidate.Validate(now); err != nil {
		return err
	}
	digest, err := e.Candidate.DigestV2()
	if err != nil || digest != e.CandidateDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "model turn Effect envelope changed its immutable candidate")
	}
	return nil
}

func ModelTurnEffectSchemaV2() runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{
		Namespace:     "praxis.harness",
		Name:          "model-turn-effect",
		Version:       "2.0.0",
		MediaType:     "application/json",
		ContentDigest: modelTurnEffectSchemaDigestV2,
	}
}

func ModelTurnEffectLimitPolicyV2() runtimeports.OpaqueLimitPolicyRefV2 {
	return runtimeports.OpaqueLimitPolicyRefV2{
		Policy: "praxis.inline/bounded",
		Digest: modelTurnEffectLimitDigestV2,
	}
}

// NewModelTurnEffectPayloadV2 produces the only canonical payload shape a
// governed model-turn adapter may submit. A custom provider can decode it
// without importing Harness kernel or fake packages.
func NewModelTurnEffectPayloadV2(candidate ModelTurnCandidateV2) (runtimeports.OpaquePayloadV2, error) {
	digest, err := candidate.DigestV2()
	if err != nil {
		return runtimeports.OpaquePayloadV2{}, err
	}
	envelope := ModelTurnEffectEnvelopeV2{ContractVersion: ModelTurnEffectPayloadContractV2, Candidate: candidate, CandidateDigest: digest}
	body, err := json.Marshal(envelope)
	if err != nil {
		return runtimeports.OpaquePayloadV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "model turn Effect envelope cannot be encoded")
	}
	payload := runtimeports.OpaquePayloadV2{
		Schema:        ModelTurnEffectSchemaV2(),
		ContentDigest: core.DigestBytes(body),
		Length:        uint64(len(body)),
		Inline:        body,
		LimitPolicy:   ModelTurnEffectLimitPolicyV2(),
	}
	return payload, payload.Validate()
}

func validateModelTurnEffectEncodingBudgetV2(candidate ModelTurnCandidateV2) error {
	// SHA-256 Digests have one canonical fixed-width representation, so this
	// placeholder produces the exact byte length of the final envelope without
	// recursively calling Candidate.DigestV2 from Candidate.Validate.
	envelope := ModelTurnEffectEnvelopeV2{
		ContractVersion: ModelTurnEffectPayloadContractV2,
		Candidate:       candidate,
		CandidateDigest: core.DigestBytes(nil),
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "model turn Effect envelope cannot be size-checked")
	}
	if len(body) == 0 || len(body) > MaxModelTurnEffectEnvelopeBytesV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "model turn candidate exceeds the complete encoded Effect envelope budget")
	}
	return nil
}

func DecodeModelTurnEffectPayloadV2(payload runtimeports.OpaquePayloadV2, now time.Time) (ModelTurnEffectEnvelopeV2, error) {
	if err := payload.Validate(); err != nil {
		return ModelTurnEffectEnvelopeV2{}, err
	}
	if payload.Schema != ModelTurnEffectSchemaV2() || payload.LimitPolicy != ModelTurnEffectLimitPolicyV2() || payload.Inline == nil {
		return ModelTurnEffectEnvelopeV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownSchema, "model turn Effect payload schema, limit policy or locality is not supported")
	}
	var envelope ModelTurnEffectEnvelopeV2
	if err := core.DecodeStrictJSON(payload.Inline, &envelope); err != nil {
		return ModelTurnEffectEnvelopeV2{}, err
	}
	if err := envelope.Validate(now); err != nil {
		return ModelTurnEffectEnvelopeV2{}, err
	}
	return envelope, nil
}
