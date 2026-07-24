package contract

import (
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ModelToolCallPendingActionIdentityContractVersionV1    = "praxis.harness.model-tool-call-pending-action-identity/v1"
	ModelToolCallOrdinalEncodingVersionV1                  = "1.0.0"
	modelToolCallIdentityCanonicalDomainV1                 = "praxis.harness.model-tool-call-pending-action-identity"
	ModelToolCallPendingActionIdentityIDCanonicalDomainV1  = "praxis.harness.model-tool-call-pending-action-identity-id"
	ModelToolCallPendingActionIdentityIDCanonicalSubjectV1 = "IdentityIDSubjectV1"
	ModelToolCallPendingActionIdentityIDPrefixV1           = "mtpa-identity:v1:"
)

type IdentityIDSubjectV1 struct {
	SourceKeyDigest core.Digest `json:"source_key_digest"`
}

func DeriveModelToolCallPendingActionIdentityIDV1(sourceKeyDigest core.Digest) (string, error) {
	if err := sourceKeyDigest.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(
		ModelToolCallPendingActionIdentityIDCanonicalDomainV1,
		ModelToolCallPendingActionIdentityContractVersionV1,
		ModelToolCallPendingActionIdentityIDCanonicalSubjectV1,
		IdentityIDSubjectV1{SourceKeyDigest: sourceKeyDigest},
	)
	if err != nil {
		return "", err
	}
	return ModelToolCallPendingActionIdentityIDPrefixV1 + string(digest), nil
}

type ModelToolCallOrdinalV1 struct {
	EncodingVersion string `json:"encoding_version"`
	Present         bool   `json:"present"`
	Value           uint32 `json:"value"`
}

func (o ModelToolCallOrdinalV1) Validate() error {
	if !o.Present || o.EncodingVersion != ModelToolCallOrdinalEncodingVersionV1 || o.Value != 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "G6A identity requires one explicitly present ordinal zero")
	}
	return nil
}

type ModelToolCallPendingActionIdentitySourceKeyV1 struct {
	ExecutionScopeDigest core.Digest                                    `json:"execution_scope_digest"`
	RunID                string                                         `json:"run_id"`
	SessionID            string                                         `json:"session_id"`
	Turn                 uint32                                         `json:"turn"`
	Candidate            CandidateRefV2                                 `json:"candidate"`
	ModelProjection      modelinvoker.ToolCallCandidateObservationRefV1 `json:"model_projection"`
	CallOrdinal          ModelToolCallOrdinalV1                         `json:"call_ordinal"`
	SettlementOwner      runtimeports.ProviderBindingRefV2              `json:"settlement_owner"`
}

func (k ModelToolCallPendingActionIdentitySourceKeyV1) Validate() error {
	if k.ExecutionScopeDigest.Validate() != nil || strings.TrimSpace(k.RunID) == "" || strings.TrimSpace(k.SessionID) == "" || k.Turn == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "G6A identity source coordinates are incomplete")
	}
	if err := k.Candidate.Validate(); err != nil {
		return err
	}
	if err := k.ModelProjection.Validate(); err != nil {
		return err
	}
	if err := k.CallOrdinal.Validate(); err != nil {
		return err
	}
	return k.SettlementOwner.Validate()
}

func (k ModelToolCallPendingActionIdentitySourceKeyV1) DigestV1() (core.Digest, error) {
	if err := k.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(modelToolCallIdentityCanonicalDomainV1, ModelToolCallPendingActionIdentityContractVersionV1, "ModelToolCallPendingActionIdentitySourceKeyV1", k)
}

type ModelToolCallPendingActionIdentityRefV1 struct {
	ID                         string        `json:"id"`
	Revision                   core.Revision `json:"revision"`
	Digest                     core.Digest   `json:"digest"`
	ModelProjectionID          string        `json:"model_projection_id"`
	ModelProjectionRevision    core.Revision `json:"model_projection_revision"`
	ModelProjectionDigest      core.Digest   `json:"model_projection_digest"`
	PendingActionRef           string        `json:"pending_action_ref"`
	PendingActionRequestDigest core.Digest   `json:"pending_action_request_digest"`
	DomainResultDigest         core.Digest   `json:"domain_result_digest"`
	SourceKeyDigest            core.Digest   `json:"source_key_digest"`
}

func (r ModelToolCallPendingActionIdentityRefV1) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision != 1 || strings.TrimSpace(r.ModelProjectionID) == "" || r.ModelProjectionRevision != 1 || strings.TrimSpace(r.PendingActionRef) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "G6A identity ref is incomplete")
	}
	for _, d := range []core.Digest{r.Digest, r.ModelProjectionDigest, r.PendingActionRequestDigest, r.DomainResultDigest, r.SourceKeyDigest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	expectedID, err := DeriveModelToolCallPendingActionIdentityIDV1(r.SourceKeyDigest)
	if err != nil {
		return err
	}
	if r.ID != expectedID {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "G6A identity ref ID is not source-derived")
	}
	return nil
}

type ModelToolCallPendingActionIdentityV1 struct {
	ContractVersion            string                                         `json:"contract_version"`
	ID                         string                                         `json:"id"`
	Revision                   core.Revision                                  `json:"revision"`
	SourceKey                  ModelToolCallPendingActionIdentitySourceKeyV1  `json:"source_key"`
	ModelProjection            modelinvoker.ToolCallCandidateObservationRefV1 `json:"model_projection"`
	CallOrdinal                ModelToolCallOrdinalV1                         `json:"call_ordinal"`
	SettlementOwner            runtimeports.ProviderBindingRefV2              `json:"settlement_owner"`
	CallID                     string                                         `json:"call_id"`
	CallName                   string                                         `json:"call_name"`
	CanonicalArgumentsDigest   core.Digest                                    `json:"canonical_arguments_digest"`
	PendingActionRef           string                                         `json:"pending_action_ref"`
	PendingActionRequestDigest core.Digest                                    `json:"pending_action_request_digest"`
	PayloadSchema              runtimeports.SchemaRefV2                       `json:"payload_schema"`
	PayloadContentDigest       core.Digest                                    `json:"payload_content_digest"`
	Capability                 runtimeports.CapabilityNameV2                  `json:"capability"`
	SourceCandidate            CandidateRefV2                                 `json:"source_candidate"`
	CreatedUnixNano            int64                                          `json:"created_unix_nano"`
	NotAfterUnixNano           int64                                          `json:"not_after_unix_nano"`
	Digest                     core.Digest                                    `json:"digest"`
}

func (i ModelToolCallPendingActionIdentityV1) Clone() ModelToolCallPendingActionIdentityV1 { return i }

func (i ModelToolCallPendingActionIdentityV1) Validate() error {
	if i.ContractVersion != ModelToolCallPendingActionIdentityContractVersionV1 || strings.TrimSpace(i.ID) == "" || i.Revision != 1 || strings.TrimSpace(i.CallID) == "" || strings.TrimSpace(i.CallName) == "" || strings.TrimSpace(i.PendingActionRef) == "" || i.CreatedUnixNano <= 0 || i.NotAfterUnixNano <= i.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "G6A identity is incomplete")
	}
	if err := i.SourceKey.Validate(); err != nil {
		return err
	}
	if i.SourceKey.ModelProjection != i.ModelProjection || i.SourceKey.CallOrdinal != i.CallOrdinal || i.SourceKey.Candidate != i.SourceCandidate || i.SourceKey.SettlementOwner != i.SettlementOwner {
		return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "G6A identity source key was spliced")
	}
	if err := i.PayloadSchema.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(i.Capability)); err != nil {
		return err
	}
	for _, d := range []core.Digest{i.CanonicalArgumentsDigest, i.PendingActionRequestDigest, i.PayloadContentDigest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	if i.CanonicalArgumentsDigest != i.PayloadContentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "G6A identity arguments and PendingAction payload differ")
	}
	sourceDigest, err := i.SourceKey.DigestV1()
	if err != nil {
		return err
	}
	expectedID, err := DeriveModelToolCallPendingActionIdentityIDV1(sourceDigest)
	if err != nil {
		return err
	}
	if i.ID != expectedID {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "G6A identity ID is not source-derived")
	}
	digest, err := i.DigestV1()
	if err != nil {
		return err
	}
	if digest != i.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "G6A identity digest drifted")
	}
	return nil
}

func (i ModelToolCallPendingActionIdentityV1) DigestV1() (core.Digest, error) {
	copy := i
	copy.Digest = ""
	return core.CanonicalJSONDigest(modelToolCallIdentityCanonicalDomainV1, ModelToolCallPendingActionIdentityContractVersionV1, "ModelToolCallPendingActionIdentityV1", copy)
}

func (i ModelToolCallPendingActionIdentityV1) RefV1(domainResultDigest core.Digest) (ModelToolCallPendingActionIdentityRefV1, error) {
	if err := i.Validate(); err != nil {
		return ModelToolCallPendingActionIdentityRefV1{}, err
	}
	if err := domainResultDigest.Validate(); err != nil {
		return ModelToolCallPendingActionIdentityRefV1{}, err
	}
	sourceDigest, _ := i.SourceKey.DigestV1()
	r := ModelToolCallPendingActionIdentityRefV1{ID: i.ID, Revision: i.Revision, Digest: i.Digest, ModelProjectionID: i.ModelProjection.ID, ModelProjectionRevision: i.ModelProjection.Revision, ModelProjectionDigest: i.ModelProjection.Digest, PendingActionRef: i.PendingActionRef, PendingActionRequestDigest: i.PendingActionRequestDigest, DomainResultDigest: domainResultDigest, SourceKeyDigest: sourceDigest}
	return r, r.Validate()
}

func SealModelToolCallPendingActionIdentityV1(source ModelToolCallPendingActionIdentitySourceKeyV1, projection modelinvoker.ToolCallCandidateObservationProjectionV1, pending PendingActionV2, createdUnixNano, notAfterUnixNano int64) (ModelToolCallPendingActionIdentityV1, error) {
	if err := source.Validate(); err != nil {
		return ModelToolCallPendingActionIdentityV1{}, err
	}
	if err := projection.Validate(); err != nil {
		return ModelToolCallPendingActionIdentityV1{}, err
	}
	if projection.Ref != source.ModelProjection || len(projection.Observation.Calls) != 1 {
		return ModelToolCallPendingActionIdentityV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "G6A identity requires the exact one-call Model projection")
	}
	if err := pending.Validate(); err != nil {
		return ModelToolCallPendingActionIdentityV1{}, err
	}
	call := projection.Observation.Calls[0]
	if call.Ordinal != source.CallOrdinal.Value || pending.SourceCandidate != source.Candidate || core.DigestBytes(call.CanonicalArguments) != pending.Payload.ContentDigest {
		return ModelToolCallPendingActionIdentityV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "G6A Model call and PendingAction are not exact")
	}
	sourceDigest, _ := source.DigestV1()
	id, err := DeriveModelToolCallPendingActionIdentityIDV1(sourceDigest)
	if err != nil {
		return ModelToolCallPendingActionIdentityV1{}, err
	}
	i := ModelToolCallPendingActionIdentityV1{ContractVersion: ModelToolCallPendingActionIdentityContractVersionV1, ID: id, Revision: 1, SourceKey: source, ModelProjection: source.ModelProjection, CallOrdinal: source.CallOrdinal, SettlementOwner: source.SettlementOwner, CallID: call.CallID, CallName: call.Name, CanonicalArgumentsDigest: core.DigestBytes(call.CanonicalArguments), PendingActionRef: pending.Ref, PendingActionRequestDigest: pending.RequestDigest, PayloadSchema: pending.Payload.Schema, PayloadContentDigest: pending.Payload.ContentDigest, Capability: pending.Capability, SourceCandidate: pending.SourceCandidate, CreatedUnixNano: createdUnixNano, NotAfterUnixNano: notAfterUnixNano}
	i.Digest, _ = i.DigestV1()
	return i, i.Validate()
}

func ValidateModelToolCallPendingActionIdentityExactV1(identity ModelToolCallPendingActionIdentityV1, projection modelinvoker.ToolCallCandidateObservationProjectionV1, pending PendingActionV2) error {
	resealed, err := SealModelToolCallPendingActionIdentityV1(identity.SourceKey, projection, pending, identity.CreatedUnixNano, identity.NotAfterUnixNano)
	if err != nil {
		return err
	}
	if resealed != identity {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "G6A identity is not the exact Model/PendingAction mapping")
	}
	return nil
}
