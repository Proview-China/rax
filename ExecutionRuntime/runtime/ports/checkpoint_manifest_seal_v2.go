package ports

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	CheckpointManifestSealContractVersionV2     = "2.1.0"
	CheckpointManifestSealOwnerContractV2       = "praxis.continuity/checkpoint-manifest-governance/v2"
	CheckpointManifestSealExactSchemaV2         = "praxis.continuity/checkpoint-manifest-seal-fact/v2"
	CheckpointManifestSealOwnerComponentV2      = "praxis/continuity"
	CheckpointManifestSealOwnerCapabilityV2     = "checkpoint-manifest-governance-v2"
	CheckpointManifestSealOwnerFactKindV2       = "checkpoint_manifest_seal_fact_v2"
	CheckpointParticipantClosureExactSchemaV2   = "praxis.runtime/checkpoint-participant-closure-ref/v2"
	CheckpointParticipantClosureExactFactKindV2 = "checkpoint_participant_closure_ref_v2"
)

// CheckpointManifestSealOwnerBindingV2 is the complete, neutral owner identity
// required to inspect a Continuity-owned immutable Seal. It deliberately does
// not copy policy, outcome, currentness, or any domain payload.
type CheckpointManifestSealOwnerBindingV2 struct {
	BindingSetID    string        `json:"binding_set_id"`
	BindingRevision core.Revision `json:"binding_revision"`
	ComponentID     string        `json:"component_id"`
	ManifestDigest  string        `json:"manifest_digest"`
	ArtifactDigest  string        `json:"artifact_digest"`
	Capability      string        `json:"capability"`
	FactKind        string        `json:"fact_kind"`
}

func (b CheckpointManifestSealOwnerBindingV2) Validate() error {
	for _, value := range []string{b.BindingSetID, b.ComponentID, b.ManifestDigest, b.ArtifactDigest, b.Capability, b.FactKind} {
		if !validCheckpointIDV2(value) {
			return checkpointInvalidV2("checkpoint Manifest Seal owner binding is incomplete")
		}
	}
	if b.BindingRevision == 0 {
		return checkpointInvalidV2("checkpoint Manifest Seal owner binding revision is required")
	}
	return nil
}

// CheckpointExternalExactFactRefV2 transports the complete cross-owner lookup
// coordinate without interpreting the referenced Owner's digest encoding.
type CheckpointExternalExactFactRefV2 struct {
	ContractVersion string                               `json:"contract_version"`
	SchemaRef       string                               `json:"schema_ref"`
	Owner           CheckpointManifestSealOwnerBindingV2 `json:"owner"`
	TenantID        string                               `json:"tenant_id"`
	ID              string                               `json:"id"`
	Revision        core.Revision                        `json:"revision"`
	Digest          string                               `json:"digest"`
	ScopeDigest     string                               `json:"scope_digest"`
}

func (r CheckpointExternalExactFactRefV2) Validate() error {
	for _, value := range []string{r.ContractVersion, r.SchemaRef, r.TenantID, r.ID, r.Digest, r.ScopeDigest} {
		if !validCheckpointIDV2(value) {
			return checkpointInvalidV2("checkpoint external exact ref is incomplete")
		}
	}
	if r.Revision == 0 || r.Owner.Validate() != nil {
		return checkpointInvalidV2("checkpoint external exact ref is incomplete")
	}
	return nil
}

// NormalizeCheckpointExternalSHA256DigestV2 maps an Owner's exact SHA-256
// spelling to Runtime's canonical digest spelling. The raw exact spelling is
// retained in CheckpointExternalExactFactRefV2 for Owner lookup.
func NormalizeCheckpointExternalSHA256DigestV2(value string) (core.Digest, error) {
	raw := value
	if strings.HasPrefix(raw, "sha256:") {
		raw = strings.TrimPrefix(raw, "sha256:")
	}
	if len(raw) != 64 || strings.ToLower(raw) != raw {
		return "", checkpointInvalidV2("checkpoint external digest must be lowercase SHA-256")
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return "", checkpointInvalidV2("checkpoint external digest must be lowercase SHA-256")
	}
	return core.Digest("sha256:" + raw), nil
}

// DeriveCheckpointParticipantClosureExactRefV2 is the only public mapping from
// the Runtime-owned typed closure to the neutral exact ref sealed by
// Continuity. Revision 1 is immutable; Owner fields come only from the typed
// Provider binding and the fact kind is fixed by this contract.
func DeriveCheckpointParticipantClosureExactRefV2(tenantID core.TenantID, scopeDigest string, closure CheckpointParticipantClosureRefV2) (CheckpointExternalExactFactRefV2, error) {
	if strings.TrimSpace(string(tenantID)) == "" || !validCheckpointIDV2(scopeDigest) || closure.Validate() != nil {
		return CheckpointExternalExactFactRefV2{}, checkpointInvalidV2("checkpoint Participant closure exact mapping is incomplete")
	}
	owner := closure.Participant.Owner
	result := CheckpointExternalExactFactRefV2{
		ContractVersion: CheckpointParticipantReservationContractVersionV2,
		SchemaRef:       CheckpointParticipantClosureExactSchemaV2,
		Owner: CheckpointManifestSealOwnerBindingV2{
			BindingSetID: owner.BindingSetID, BindingRevision: owner.BindingSetRevision,
			ComponentID: string(owner.ComponentID), ManifestDigest: string(owner.ManifestDigest),
			ArtifactDigest: string(owner.ArtifactDigest), Capability: string(owner.Capability),
			FactKind: CheckpointParticipantClosureExactFactKindV2,
		},
		TenantID: string(tenantID), ID: closure.ID, Revision: 1,
		Digest: string(closure.Digest), ScopeDigest: scopeDigest,
	}
	return result, result.Validate()
}

type CheckpointManifestSealRefV2 struct {
	ExactLookup        CheckpointExternalExactFactRefV2 `json:"exact_lookup"`
	ID                 string                           `json:"seal_id"`
	Revision           core.Revision                    `json:"revision"`
	Digest             core.Digest                      `json:"digest"`
	ManifestID         string                           `json:"manifest_id"`
	ManifestRevision   core.Revision                    `json:"manifest_revision"`
	ManifestDigest     core.Digest                      `json:"manifest_digest"`
	Attempt            CheckpointAttemptRefV2           `json:"attempt"`
	Barrier            CheckpointBarrierLeaseRefV2      `json:"barrier"`
	EffectCut          EffectCutRefV2                   `json:"effect_cut"`
	FrozenRefSetDigest core.Digest                      `json:"frozen_ref_set_digest"`
}

func (r CheckpointManifestSealRefV2) Validate() error {
	if r.ExactLookup.Validate() != nil || !validCheckpointIDV2(r.ID) || r.Revision != 1 || !validCheckpointIDV2(r.ManifestID) || r.ManifestRevision == 0 || r.Attempt.Validate() != nil || r.Barrier.Validate() != nil || r.EffectCut.Validate() != nil {
		return checkpointInvalidV2("checkpoint Manifest Seal ref is incomplete")
	}
	for _, digest := range []core.Digest{r.Digest, r.ManifestDigest, r.FrozenRefSetDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if r.Attempt.ID != r.Barrier.AttemptID || r.Attempt.ID != r.EffectCut.Attempt.ID {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Manifest Seal belongs to another attempt")
	}
	if r.ExactLookup.ContractVersion != CheckpointManifestSealOwnerContractV2 || r.ExactLookup.SchemaRef != CheckpointManifestSealExactSchemaV2 || r.ExactLookup.Owner.ComponentID != CheckpointManifestSealOwnerComponentV2 || r.ExactLookup.Owner.Capability != CheckpointManifestSealOwnerCapabilityV2 || r.ExactLookup.Owner.FactKind != CheckpointManifestSealOwnerFactKindV2 {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Manifest Seal exact owner coordinate drifted")
	}
	normalized, err := NormalizeCheckpointExternalSHA256DigestV2(r.ExactLookup.Digest)
	if err != nil {
		return err
	}
	if r.ExactLookup.ID != r.ID || r.ExactLookup.Revision != r.Revision || r.ExactLookup.TenantID != string(r.Attempt.TenantID) || normalized != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Manifest Seal exact lookup drifted")
	}
	return nil
}

type CheckpointManifestSealProjectionV2 struct {
	ContractVersion       string                              `json:"contract_version"`
	Ref                   CheckpointManifestSealRefV2         `json:"ref"`
	ParticipantSetDigest  core.Digest                         `json:"participant_set_digest"`
	ParticipantClosures   []CheckpointParticipantClosureRefV2 `json:"participant_closures"`
	ContextClosureDigest  core.Digest                         `json:"context_closure_digest"`
	ArtifactClosureDigest core.Digest                         `json:"artifact_closure_digest"`
	SealDigest            core.Digest                         `json:"seal_digest"`
}

func (p CheckpointManifestSealProjectionV2) Validate() error {
	if p.ContractVersion != CheckpointManifestSealContractVersionV2 || p.Ref.Validate() != nil || p.ParticipantSetDigest.Validate() != nil || p.ContextClosureDigest.Validate() != nil || p.ArtifactClosureDigest.Validate() != nil || p.SealDigest.Validate() != nil || len(p.ParticipantClosures) == 0 || len(p.ParticipantClosures) > MaxCheckpointParticipantClosuresV2 {
		return checkpointInvalidV2("checkpoint Manifest Seal projection is incomplete")
	}
	for index, closure := range p.ParticipantClosures {
		if err := closure.Validate(); err != nil {
			return err
		}
		if index > 0 && closure.ID <= p.ParticipantClosures[index-1].ID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Manifest Seal closures must be sorted and unique")
		}
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.SealDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Manifest Seal projection drifted")
	}
	return nil
}

func (p CheckpointManifestSealProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.SealDigest = ""
	copy.ParticipantClosures = normalizeCheckpointClosureRefsV2(copy.ParticipantClosures)
	return core.CanonicalJSONDigest("praxis.runtime.checkpoint-manifest-seal", CheckpointManifestSealContractVersionV2, "CheckpointManifestSealProjectionV2", copy)
}

type InspectCheckpointManifestSealRequestV2 struct {
	Ref                          CheckpointManifestSealRefV2         `json:"ref"`
	ExpectedParticipantSetDigest core.Digest                         `json:"expected_participant_set_digest"`
	ExpectedParticipantClosures  []CheckpointParticipantClosureRefV2 `json:"expected_participant_closures"`
}

func (r InspectCheckpointManifestSealRequestV2) Validate() error {
	if r.Ref.Validate() != nil || r.ExpectedParticipantSetDigest.Validate() != nil || len(r.ExpectedParticipantClosures) == 0 || len(r.ExpectedParticipantClosures) > MaxCheckpointParticipantClosuresV2 {
		return checkpointInvalidV2("inspect checkpoint Manifest Seal request is incomplete")
	}
	for index, closure := range r.ExpectedParticipantClosures {
		if closure.Validate() != nil {
			return checkpointInvalidV2("inspect checkpoint Manifest Seal closure is invalid")
		}
		if index > 0 && closure.ID <= r.ExpectedParticipantClosures[index-1].ID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "inspect Manifest Seal closures must be sorted and unique")
		}
	}
	return nil
}

type CheckpointManifestSealReaderV2 interface {
	InspectCheckpointManifestSealV2(context.Context, InspectCheckpointManifestSealRequestV2) (CheckpointManifestSealProjectionV2, error)
}
