package contract

import (
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	SettledTurnDomainResultContractVersionV3        = "praxis.harness.settled-turn-domain-result/v3"
	SettledTurnDomainResultSchemaNamespaceV3        = "praxis.harness"
	SettledTurnDomainResultSchemaNameV3             = "settled-turn-result"
	SettledTurnDomainResultSchemaVersionV3          = "3.0.0"
	settledTurnDomainResultCanonicalDomainV3        = "praxis.harness.settled-turn-domain-result"
	SettledTurnDomainResultFactIDCanonicalDomainV3  = "praxis.harness.settled-turn-domain-result-fact-id"
	SettledTurnDomainResultFactIDCanonicalSubjectV3 = "FactIDSubjectV3"
	SettledTurnDomainResultFactIDPrefixV3           = "settled-turn-fact:v3:"
)

type FactIDSubjectV3 struct {
	SourceKeyDigest core.Digest `json:"source_key_digest"`
}

func DeriveSettledTurnDomainResultFactIDV3(sourceKeyDigest core.Digest) (string, error) {
	if err := sourceKeyDigest.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(
		SettledTurnDomainResultFactIDCanonicalDomainV3,
		SettledTurnDomainResultContractVersionV3,
		SettledTurnDomainResultFactIDCanonicalSubjectV3,
		FactIDSubjectV3{SourceKeyDigest: sourceKeyDigest},
	)
	if err != nil {
		return "", err
	}
	return SettledTurnDomainResultFactIDPrefixV3 + string(digest), nil
}

type SettledTurnDomainResultContentV3 struct {
	Candidate       CandidateRefV2                                 `json:"candidate"`
	ModelProjection modelinvoker.ToolCallCandidateObservationRefV1 `json:"model_projection"`
	PendingAction   PendingActionV2                                `json:"pending_action"`
	Identity        ModelToolCallPendingActionIdentityV1           `json:"identity"`
}

type SettledTurnDomainResultFactRefV3 struct {
	FactID          string                                  `json:"fact_id"`
	Revision        core.Revision                           `json:"revision"`
	FactDigest      core.Digest                             `json:"fact_digest"`
	SourceKeyDigest core.Digest                             `json:"source_key_digest"`
	Schema          runtimeports.SchemaRefV2                `json:"schema"`
	ContentDigest   core.Digest                             `json:"content_digest"`
	IdentityRef     ModelToolCallPendingActionIdentityRefV1 `json:"identity_ref"`
}

func (r SettledTurnDomainResultFactRefV3) Validate() error {
	if strings.TrimSpace(r.FactID) == "" || r.Revision != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "SettledTurn DomainResult fact ref is incomplete")
	}
	if err := r.Schema.Validate(); err != nil {
		return err
	}
	if err := validateSettledTurnSchemaV3(r.Schema); err != nil {
		return err
	}
	for _, d := range []core.Digest{r.FactDigest, r.SourceKeyDigest, r.ContentDigest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	if err := r.IdentityRef.Validate(); err != nil {
		return err
	}
	if r.IdentityRef.DomainResultDigest != r.ContentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Identity ref binds another DomainResult")
	}
	if r.IdentityRef.SourceKeyDigest != r.SourceKeyDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Identity and DomainResult source keys differ")
	}
	expectedFactID, err := DeriveSettledTurnDomainResultFactIDV3(r.SourceKeyDigest)
	if err != nil {
		return err
	}
	expectedIdentityID, err := DeriveModelToolCallPendingActionIdentityIDV1(r.SourceKeyDigest)
	if err != nil {
		return err
	}
	if r.FactID != expectedFactID || r.IdentityRef.ID != expectedIdentityID || expectedFactID == expectedIdentityID || r.FactID == r.IdentityRef.ID {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "SettledTurn Fact and Identity ref IDs are not exact domain-separated derivations")
	}
	return nil
}

type SettledTurnDomainResultFactV3 struct {
	ContractVersion string                                         `json:"contract_version"`
	FactID          string                                         `json:"fact_id"`
	Revision        core.Revision                                  `json:"revision"`
	SourceKey       ModelToolCallPendingActionIdentitySourceKeyV1  `json:"source_key"`
	Candidate       CandidateRefV2                                 `json:"candidate"`
	ModelProjection modelinvoker.ToolCallCandidateObservationRefV1 `json:"model_projection"`
	PendingAction   PendingActionV2                                `json:"pending_action"`
	Identity        ModelToolCallPendingActionIdentityV1           `json:"identity"`
	SettlementOwner runtimeports.ProviderBindingRefV2              `json:"settlement_owner"`
	Schema          runtimeports.SchemaRefV2                       `json:"schema"`
	ContentDigest   core.Digest                                    `json:"content_digest"`
	CreatedUnixNano int64                                          `json:"created_unix_nano"`
	FactDigest      core.Digest                                    `json:"fact_digest"`
}

func (f SettledTurnDomainResultFactV3) Clone() SettledTurnDomainResultFactV3 {
	clone := f
	clone.PendingAction.Payload.Inline = append([]byte(nil), f.PendingAction.Payload.Inline...)
	return clone
}

func (f SettledTurnDomainResultFactV3) Validate() error {
	if f.ContractVersion != SettledTurnDomainResultContractVersionV3 || strings.TrimSpace(f.FactID) == "" || f.Revision != 1 || f.CreatedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "SettledTurn DomainResult fact is incomplete")
	}
	if err := f.SourceKey.Validate(); err != nil {
		return err
	}
	if err := f.Candidate.Validate(); err != nil {
		return err
	}
	if err := f.ModelProjection.Validate(); err != nil {
		return err
	}
	if err := f.PendingAction.Validate(); err != nil {
		return err
	}
	if err := f.Identity.Validate(); err != nil {
		return err
	}
	if err := f.SettlementOwner.Validate(); err != nil {
		return err
	}
	if err := validateSettledTurnSchemaV3(f.Schema); err != nil {
		return err
	}
	if f.SourceKey != f.Identity.SourceKey || f.Candidate != f.Identity.SourceCandidate || f.ModelProjection != f.Identity.ModelProjection || f.SettlementOwner != f.Identity.SettlementOwner {
		return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "SettledTurn DomainResult source lineage was spliced")
	}
	if f.PendingAction.Ref != f.Identity.PendingActionRef || f.PendingAction.RequestDigest != f.Identity.PendingActionRequestDigest || f.PendingAction.Payload.Schema != f.Identity.PayloadSchema || f.PendingAction.Payload.ContentDigest != f.Identity.PayloadContentDigest || f.PendingAction.Capability != f.Identity.Capability || f.PendingAction.SourceCandidate != f.Identity.SourceCandidate {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "SettledTurn DomainResult PendingAction and identity differ")
	}
	sourceDigest, _ := f.SourceKey.DigestV1()
	expectedID, err := DeriveSettledTurnDomainResultFactIDV3(sourceDigest)
	if err != nil {
		return err
	}
	if f.FactID != expectedID {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "SettledTurn Fact ID is not source-derived")
	}
	identityID, err := DeriveModelToolCallPendingActionIdentityIDV1(sourceDigest)
	if err != nil {
		return err
	}
	if identityID == expectedID || f.Identity.ID != identityID {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "SettledTurn Fact and Identity IDs are not domain-separated")
	}
	content, err := f.ContentDigestV3()
	if err != nil {
		return err
	}
	if content != f.ContentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "SettledTurn content digest drifted")
	}
	digest, err := f.DigestV3()
	if err != nil {
		return err
	}
	if digest != f.FactDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "SettledTurn fact digest drifted")
	}
	return nil
}

func (f SettledTurnDomainResultFactV3) ContentDigestV3() (core.Digest, error) {
	body := SettledTurnDomainResultContentV3{Candidate: f.Candidate, ModelProjection: f.ModelProjection, PendingAction: f.PendingAction, Identity: f.Identity}
	return core.CanonicalJSONDigest(settledTurnDomainResultCanonicalDomainV3, SettledTurnDomainResultContractVersionV3, "SettledTurnDomainResultContentV3", body)
}

func (f SettledTurnDomainResultFactV3) DigestV3() (core.Digest, error) {
	copy := f
	copy.FactDigest = ""
	return core.CanonicalJSONDigest(settledTurnDomainResultCanonicalDomainV3, SettledTurnDomainResultContractVersionV3, "SettledTurnDomainResultFactV3", copy)
}

func (f SettledTurnDomainResultFactV3) RefV3() (SettledTurnDomainResultFactRefV3, error) {
	if err := f.Validate(); err != nil {
		return SettledTurnDomainResultFactRefV3{}, err
	}
	sourceDigest, _ := f.SourceKey.DigestV1()
	identityRef, err := f.Identity.RefV1(f.ContentDigest)
	if err != nil {
		return SettledTurnDomainResultFactRefV3{}, err
	}
	r := SettledTurnDomainResultFactRefV3{FactID: f.FactID, Revision: f.Revision, FactDigest: f.FactDigest, SourceKeyDigest: sourceDigest, Schema: f.Schema, ContentDigest: f.ContentDigest, IdentityRef: identityRef}
	return r, r.Validate()
}

func SealSettledTurnDomainResultFactV3(identity ModelToolCallPendingActionIdentityV1, pending PendingActionV2, schema runtimeports.SchemaRefV2, createdUnixNano int64) (SettledTurnDomainResultFactV3, error) {
	if err := identity.Validate(); err != nil {
		return SettledTurnDomainResultFactV3{}, err
	}
	if err := pending.Validate(); err != nil {
		return SettledTurnDomainResultFactV3{}, err
	}
	if createdUnixNano != identity.CreatedUnixNano {
		return SettledTurnDomainResultFactV3{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "SettledTurn and identity creation time differ")
	}
	sourceDigest, _ := identity.SourceKey.DigestV1()
	factID, err := DeriveSettledTurnDomainResultFactIDV3(sourceDigest)
	if err != nil {
		return SettledTurnDomainResultFactV3{}, err
	}
	f := SettledTurnDomainResultFactV3{ContractVersion: SettledTurnDomainResultContractVersionV3, FactID: factID, Revision: 1, SourceKey: identity.SourceKey, Candidate: identity.SourceCandidate, ModelProjection: identity.ModelProjection, PendingAction: pending, Identity: identity, SettlementOwner: identity.SettlementOwner, Schema: schema, CreatedUnixNano: createdUnixNano}
	f.ContentDigest, _ = f.ContentDigestV3()
	f.FactDigest, _ = f.DigestV3()
	return f, f.Validate()
}

func validateSettledTurnSchemaV3(schema runtimeports.SchemaRefV2) error {
	if err := schema.Validate(); err != nil {
		return err
	}
	if schema.Namespace != SettledTurnDomainResultSchemaNamespaceV3 || schema.Name != SettledTurnDomainResultSchemaNameV3 || schema.Version != SettledTurnDomainResultSchemaVersionV3 || schema.MediaType != "application/json" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownSchema, "SettledTurn DomainResult requires schema 3.0.0")
	}
	return nil
}
