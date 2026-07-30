package bridgecontract

import (
	"bytes"
	"encoding/json"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const ModelTurnDispatchContractVersionV2 = "praxis.harness.exact-model-turn-dispatch/v2"

type ModelTurnDispatchStateV2 string

const (
	ModelTurnDispatchAttemptBoundV2 ModelTurnDispatchStateV2 = "attempt_bound"
	ModelTurnDispatchOutcomeBoundV2 ModelTurnDispatchStateV2 = "outcome_bound"
)

// ModelTurnExactEnvelopeV2 carries only immutable Model-owned coordinates and
// a caller-selected upper lifetime bound. It carries no prompt, Tool payload,
// Provider response, PendingAction, Turn mutation, or authority.
type ModelTurnExactEnvelopeV2 struct {
	ContractVersion           string                                             `json:"contract_version"`
	ID                        string                                             `json:"id"`
	Revision                  core.Revision                                      `json:"revision"`
	Digest                    core.Digest                                        `json:"digest"`
	Material                  modelinvoker.InvocationMaterialRefV2               `json:"material"`
	Command                   modelinvoker.GovernedModelTurnCommandV3            `json:"command"`
	AckRef                    modelinvoker.PreparedModelInvocationCommitAckRefV1 `json:"ack_ref"`
	RequestedNotAfterUnixNano int64                                              `json:"requested_not_after_unix_nano"`
}

func (e ModelTurnExactEnvelopeV2) Validate() error {
	if e.ContractVersion != ModelTurnDispatchContractVersionV2 ||
		strings.TrimSpace(e.ID) == "" || e.Revision != 1 ||
		e.Digest.Validate() != nil {
		return exactModelTurnContractErrorV2(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact model-turn V2 Envelope identity is invalid")
	}
	if err := e.validateBodyV2(); err != nil {
		return err
	}
	id, err := deriveModelTurnExactEnvelopeIDV2(e)
	if err != nil || id != e.ID {
		return exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidDigest, "exact model-turn V2 Envelope ID drifted")
	}
	digest, err := modelTurnExactEnvelopeDigestV2(e)
	if err != nil || digest != e.Digest {
		return exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidDigest, "exact model-turn V2 Envelope digest drifted")
	}
	return nil
}

func (e ModelTurnExactEnvelopeV2) validateBodyV2() error {
	if e.Material.Validate() != nil || e.Command.Validate() != nil ||
		e.AckRef.Validate() != nil ||
		e.Command.MaterialRef != e.Material ||
		e.Command.PreparedRef != e.Material.PreparedRef ||
		e.Command.CurrentRef != e.Material.AuthorizationRef.CurrentRef ||
		e.AckRef.PreparedRef != e.Command.PreparedRef ||
		e.AckRef.CurrentRef != e.Command.CurrentRef ||
		e.Command.AttemptRequestDigest != e.Material.UnifiedRequestDigest ||
		e.Command.RouteCallDigest != e.Material.RouteCallDigest {
		return exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidReference, "exact model-turn V2 Model lineage drifted")
	}
	if e.RequestedNotAfterUnixNano <= 0 ||
		e.RequestedNotAfterUnixNano > e.Material.ExpiresUnixNano ||
		e.RequestedNotAfterUnixNano > e.Command.CurrentRef.ExpiresUnixNano ||
		e.RequestedNotAfterUnixNano > e.Command.CurrentRef.NotAfterUnixNano ||
		e.RequestedNotAfterUnixNano > e.AckRef.ExpiresUnixNano ||
		e.RequestedNotAfterUnixNano > e.AckRef.NotAfterUnixNano {
		return exactModelTurnContractErrorV2(core.ErrorInvalidArgument, core.ReasonBindingExpired, "exact model-turn V2 requested lifetime is invalid")
	}
	return nil
}

func SealModelTurnExactEnvelopeV2(envelope ModelTurnExactEnvelopeV2) (ModelTurnExactEnvelopeV2, error) {
	if envelope.ContractVersion != "" && envelope.ContractVersion != ModelTurnDispatchContractVersionV2 {
		return ModelTurnExactEnvelopeV2{}, exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidReference, "exact model-turn V2 Envelope version drifted")
	}
	if envelope.Revision != 0 && envelope.Revision != 1 {
		return ModelTurnExactEnvelopeV2{}, exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonRevisionConflict, "exact model-turn V2 Envelope revision drifted")
	}
	envelope.ContractVersion = ModelTurnDispatchContractVersionV2
	envelope.Revision = 1
	if err := envelope.validateBodyV2(); err != nil {
		return ModelTurnExactEnvelopeV2{}, err
	}
	id, err := deriveModelTurnExactEnvelopeIDV2(envelope)
	if err != nil {
		return ModelTurnExactEnvelopeV2{}, err
	}
	if envelope.ID != "" && envelope.ID != id {
		return ModelTurnExactEnvelopeV2{}, exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidDigest, "supplied exact model-turn V2 Envelope ID drifted")
	}
	envelope.ID = id
	provided := envelope.Digest
	envelope.Digest = ""
	envelope.Digest, err = modelTurnExactEnvelopeDigestV2(envelope)
	if err != nil {
		return ModelTurnExactEnvelopeV2{}, err
	}
	if provided != "" && provided != envelope.Digest {
		return ModelTurnExactEnvelopeV2{}, exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidDigest, "supplied exact model-turn V2 Envelope digest drifted")
	}
	return envelope, envelope.Validate()
}

func deriveModelTurnExactEnvelopeIDV2(envelope ModelTurnExactEnvelopeV2) (string, error) {
	copy := envelope
	copy.ContractVersion, copy.ID, copy.Revision, copy.Digest = "", "", 0, ""
	digest, err := core.CanonicalJSONDigest(
		"praxis.harness.exact-model-turn",
		ModelTurnDispatchContractVersionV2,
		"ModelTurnExactEnvelopeIdentityV2",
		copy,
	)
	if err != nil {
		return "", err
	}
	return "harness-model-turn-envelope-v2/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func modelTurnExactEnvelopeDigestV2(envelope ModelTurnExactEnvelopeV2) (core.Digest, error) {
	envelope.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.harness.exact-model-turn",
		ModelTurnDispatchContractVersionV2,
		"ModelTurnExactEnvelopeV2",
		envelope,
	)
}

type ModelTurnDispatchRefV2 struct {
	ContractVersion string                                     `json:"contract_version"`
	ID              string                                     `json:"id"`
	Digest          core.Digest                                `json:"digest"`
	Envelope        ModelTurnExactEnvelopeV2                   `json:"envelope"`
	Attempt         modelinvoker.GovernedModelTurnAttemptRefV3 `json:"attempt"`
}

func DeriveModelTurnDispatchRefV2(envelope ModelTurnExactEnvelopeV2) (ModelTurnDispatchRefV2, error) {
	if err := envelope.Validate(); err != nil {
		return ModelTurnDispatchRefV2{}, err
	}
	return deriveModelTurnDispatchRefWithoutValidationV2(envelope)
}

func (r ModelTurnDispatchRefV2) Validate() error {
	if r.ContractVersion != ModelTurnDispatchContractVersionV2 ||
		strings.TrimSpace(r.ID) == "" || r.Digest.Validate() != nil ||
		r.Envelope.Validate() != nil || r.Attempt.Validate() != nil {
		return exactModelTurnContractErrorV2(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact model-turn V2 DispatchRef is invalid")
	}
	derived, err := deriveModelTurnDispatchRefWithoutValidationV2(r.Envelope)
	if err != nil || derived != r {
		return exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidDigest, "exact model-turn V2 DispatchRef drifted")
	}
	return nil
}

func deriveModelTurnDispatchRefWithoutValidationV2(envelope ModelTurnExactEnvelopeV2) (ModelTurnDispatchRefV2, error) {
	attempt, err := modelinvoker.DeriveGovernedModelTurnAttemptRefV3(envelope.Command)
	if err != nil {
		return ModelTurnDispatchRefV2{}, err
	}
	ref := ModelTurnDispatchRefV2{
		ContractVersion: ModelTurnDispatchContractVersionV2,
		Envelope:        envelope,
		Attempt:         attempt,
	}
	identity, err := core.CanonicalJSONDigest(
		"praxis.harness.exact-model-turn",
		ModelTurnDispatchContractVersionV2,
		"ModelTurnDispatchIdentityV2",
		struct {
			Envelope ModelTurnExactEnvelopeV2                   `json:"envelope"`
			Attempt  modelinvoker.GovernedModelTurnAttemptRefV3 `json:"attempt"`
		}{Envelope: envelope, Attempt: attempt},
	)
	if err != nil {
		return ModelTurnDispatchRefV2{}, err
	}
	ref.ID = "harness-model-turn-dispatch-v2/" + strings.TrimPrefix(string(identity), "sha256:")
	ref.Digest, err = ref.digestV2()
	return ref, err
}

func (r ModelTurnDispatchRefV2) digestV2() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.harness.exact-model-turn",
		ModelTurnDispatchContractVersionV2,
		"ModelTurnDispatchRefV2",
		copy,
	)
}

type ModelTurnDispatchFactV2 struct {
	ContractVersion  string                                     `json:"contract_version"`
	Ref              ModelTurnDispatchRefV2                     `json:"ref"`
	Revision         core.Revision                              `json:"revision"`
	State            ModelTurnDispatchStateV2                   `json:"state"`
	Attempt          modelinvoker.GovernedModelTurnAttemptRefV3 `json:"attempt"`
	Outcome          *modelinvoker.GovernedModelTurnRefV3       `json:"outcome,omitempty"`
	NotAfterUnixNano int64                                      `json:"not_after_unix_nano"`
	Digest           core.Digest                                `json:"digest"`
}

func NewModelTurnDispatchAttemptFactV2(ref ModelTurnDispatchRefV2) (ModelTurnDispatchFactV2, error) {
	fact := ModelTurnDispatchFactV2{
		ContractVersion:  ModelTurnDispatchContractVersionV2,
		Ref:              ref,
		Revision:         1,
		State:            ModelTurnDispatchAttemptBoundV2,
		Attempt:          ref.Attempt,
		NotAfterUnixNano: ref.Envelope.RequestedNotAfterUnixNano,
	}
	return sealModelTurnDispatchFactV2(fact)
}

func BindModelTurnDispatchOutcomeV2(current ModelTurnDispatchFactV2, outcome modelinvoker.GovernedModelTurnRefV3) (ModelTurnDispatchFactV2, error) {
	if err := current.Validate(); err != nil {
		return ModelTurnDispatchFactV2{}, err
	}
	if current.State == ModelTurnDispatchOutcomeBoundV2 {
		if current.Outcome != nil && *current.Outcome == outcome {
			return current, nil
		}
		return ModelTurnDispatchFactV2{}, exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "exact model-turn V2 Dispatch already binds another Outcome")
	}
	if !sameModelTurnAttemptAndOutcomeV2(current.Attempt, outcome) {
		return ModelTurnDispatchFactV2{}, exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidReference, "Model V3 Outcome differs from stable exact attempt")
	}
	next := current
	next.Revision = 2
	next.State = ModelTurnDispatchOutcomeBoundV2
	next.Outcome = &outcome
	next.Digest = ""
	return sealModelTurnDispatchFactV2(next)
}

func (f ModelTurnDispatchFactV2) Validate() error {
	if f.ContractVersion != ModelTurnDispatchContractVersionV2 ||
		f.Ref.Validate() != nil || f.Attempt.Validate() != nil ||
		f.Attempt != f.Ref.Attempt ||
		f.NotAfterUnixNano != f.Ref.Envelope.RequestedNotAfterUnixNano ||
		f.Digest.Validate() != nil {
		return exactModelTurnContractErrorV2(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact model-turn V2 Dispatch Fact is invalid")
	}
	switch f.State {
	case ModelTurnDispatchAttemptBoundV2:
		if f.Revision != 1 || f.Outcome != nil {
			return exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidState, "attempt-bound model-turn V2 Fact carries an Outcome")
		}
	case ModelTurnDispatchOutcomeBoundV2:
		if f.Revision != 2 || f.Outcome == nil ||
			!sameModelTurnAttemptAndOutcomeV2(f.Attempt, *f.Outcome) {
			return exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidState, "outcome-bound model-turn V2 Fact is incomplete")
		}
	default:
		return exactModelTurnContractErrorV2(core.ErrorInvalidArgument, core.ReasonInvalidState, "exact model-turn V2 Dispatch state is unsupported")
	}
	digest, err := f.digestV2()
	if err != nil || digest != f.Digest {
		return exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidDigest, "exact model-turn V2 Dispatch Fact digest drifted")
	}
	return nil
}

func (f ModelTurnDispatchFactV2) CloneV2() ModelTurnDispatchFactV2 {
	clone := f
	if f.Outcome != nil {
		outcome := *f.Outcome
		clone.Outcome = &outcome
	}
	return clone
}

func EncodeModelTurnDispatchFactV2(fact ModelTurnDispatchFactV2) ([]byte, error) {
	if err := fact.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(fact)
}

func DecodeModelTurnDispatchFactV2(payload []byte) (ModelTurnDispatchFactV2, error) {
	var fact ModelTurnDispatchFactV2
	if err := core.DecodeStrictJSON(payload, &fact); err != nil {
		return ModelTurnDispatchFactV2{}, exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 Dispatch payload is not strict JSON")
	}
	canonical, err := json.Marshal(fact)
	if err != nil || !bytes.Equal(canonical, payload) || fact.Validate() != nil {
		return ModelTurnDispatchFactV2{}, exactModelTurnContractErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 Dispatch payload failed canonical validation")
	}
	return fact.CloneV2(), nil
}

func sealModelTurnDispatchFactV2(fact ModelTurnDispatchFactV2) (ModelTurnDispatchFactV2, error) {
	fact.Digest = ""
	digest, err := fact.digestV2()
	if err != nil {
		return ModelTurnDispatchFactV2{}, err
	}
	fact.Digest = digest
	return fact, fact.Validate()
}

func (f ModelTurnDispatchFactV2) digestV2() (core.Digest, error) {
	copy := f.CloneV2()
	copy.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.harness.exact-model-turn",
		ModelTurnDispatchContractVersionV2,
		"ModelTurnDispatchFactV2",
		copy,
	)
}

func sameModelTurnAttemptAndOutcomeV2(attempt modelinvoker.GovernedModelTurnAttemptRefV3, outcome modelinvoker.GovernedModelTurnRefV3) bool {
	return attempt.Validate() == nil && outcome.Validate() == nil &&
		outcome.AttemptRefV3() == attempt
}

func exactModelTurnContractErrorV2(category core.ErrorCategory, reason core.ReasonCode, message string) error {
	return core.NewError(category, reason, message)
}
