package contract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type TargetKindV1 string

const (
	TargetIntentV1    TargetKindV1 = "intent"
	TargetActionV1    TargetKindV1 = "action"
	TargetEffectV1    TargetKindV1 = "effect"
	TargetArtifactV1  TargetKindV1 = "artifact"
	TargetWorkStateV1 TargetKindV1 = "work_state"
	TargetOutcomeV1   TargetKindV1 = "outcome"
)

type TargetSnapshotV1 struct {
	FactIdentityV1
	Kind               TargetKindV1                            `json:"kind"`
	PayloadSchema      runtimeports.SchemaRefV2                `json:"payload_schema"`
	PayloadDigest      core.Digest                             `json:"payload_digest"`
	PayloadRevision    core.Revision                           `json:"payload_revision"`
	Scope              core.ExecutionScope                     `json:"scope"`
	RunID              core.AgentRunID                         `json:"run_id"`
	ActionScopeDigest  core.Digest                             `json:"action_scope_digest"`
	IntentID           core.EffectIntentID                     `json:"intent_id,omitempty"`
	IntentRevision     core.Revision                           `json:"intent_revision,omitempty"`
	SubjectDigest      core.Digest                             `json:"subject_digest,omitempty"`
	Policy             runtimeports.ReviewPolicyBindingRefV2   `json:"policy"`
	ActorAuthority     runtimeports.AuthorityBindingRefV2      `json:"actor_authority"`
	CurrentScope       runtimeports.ExecutionScopeBindingRefV2 `json:"current_scope"`
	Evidence           []runtimeports.ReviewEvidenceRefV2      `json:"evidence"`
	EvidenceSetDigest  core.Digest                             `json:"evidence_set_digest"`
	ContextFrameDigest core.Digest                             `json:"context_frame_digest"`
	ExpiresUnixNano    int64                                   `json:"expires_unix_nano"`
}

func (t TargetSnapshotV1) validateShape() error {
	if err := t.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	switch t.Kind {
	case TargetIntentV1, TargetActionV1, TargetEffectV1, TargetArtifactV1, TargetWorkStateV1, TargetOutcomeV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review target kind is unsupported")
	}
	if t.PayloadRevision == 0 || blank(string(t.RunID)) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review target requires an immutable target revision, payload revision and source run")
	}
	if err := t.PayloadSchema.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{t.PayloadDigest, t.ActionScopeDigest, t.EvidenceSetDigest, t.ContextFrameDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := t.Scope.Validate(); err != nil {
		return err
	}
	if err := t.Policy.Validate(); err != nil {
		return err
	}
	if err := t.ActorAuthority.Validate(); err != nil {
		return err
	}
	if err := t.CurrentScope.Validate(); err != nil {
		return err
	}
	if len(t.Evidence) > MaxListItemsV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review target evidence exceeds its bound")
	}
	if !sort.SliceIsSorted(t.Evidence, func(i, j int) bool { return t.Evidence[i].Ref < t.Evidence[j].Ref }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review target evidence must be sorted")
	}
	for _, evidence := range t.Evidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
	}
	expectedEvidence, err := ComputeReviewEvidenceDigestV1(t.Evidence)
	if err != nil {
		return err
	}
	if expectedEvidence != t.EvidenceSetDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "target evidence set digest drifted")
	}
	if t.Kind == TargetEffectV1 {
		if blank(string(t.IntentID)) || t.IntentRevision == 0 || t.SubjectDigest.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectIntentMissing, "effect review target requires exact intent and subject")
		}
	} else if t.IntentID != "" || t.IntentRevision != 0 || t.SubjectDigest != "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "non-effect target cannot carry partial effect intent identity")
	}
	return ValidateExpires(t.CreatedUnixNano, t.ExpiresUnixNano)
}

func (t TargetSnapshotV1) digestValue() TargetSnapshotV1 {
	t.Digest = ""
	return t
}

func SealTargetSnapshotV1(t TargetSnapshotV1) (TargetSnapshotV1, error) {
	t.ContractVersion = ContractVersionV1
	t.Digest = ""
	sort.Slice(t.Evidence, func(i, j int) bool { return t.Evidence[i].Ref < t.Evidence[j].Ref })
	if err := t.validateShape(); err != nil {
		return TargetSnapshotV1{}, err
	}
	digest, err := seal("TargetSnapshotV1", t.digestValue())
	if err != nil {
		return TargetSnapshotV1{}, err
	}
	t.Digest = digest
	return t, t.Validate()
}

func (t TargetSnapshotV1) Validate() error {
	if err := t.validateShape(); err != nil {
		return err
	}
	return validateSealed("TargetSnapshotV1", t.digestValue(), t.Digest)
}

type TargetCurrentnessV1 struct {
	TargetID           string
	TargetRevision     core.Revision
	TargetDigest       core.Digest
	PayloadRevision    core.Revision
	PayloadDigest      core.Digest
	Scope              core.ExecutionScope
	ActionScopeDigest  core.Digest
	Policy             runtimeports.ReviewPolicyBindingRefV2
	ActorAuthority     runtimeports.AuthorityBindingRefV2
	CurrentScope       runtimeports.ExecutionScopeBindingRefV2
	EvidenceSetDigest  core.Digest
	ContextFrameDigest core.Digest
	Now                time.Time
}

func (t TargetSnapshotV1) ValidateCurrent(c TargetCurrentnessV1) error {
	if err := t.Validate(); err != nil {
		return err
	}
	if c.TargetID != t.ID || c.TargetRevision != t.Revision || c.TargetDigest != t.Digest || c.PayloadRevision != t.PayloadRevision || c.PayloadDigest != t.PayloadDigest || c.Scope != t.Scope || c.ActionScopeDigest != t.ActionScopeDigest || c.Policy != t.Policy || c.ActorAuthority != t.ActorAuthority || c.CurrentScope != t.CurrentScope || c.EvidenceSetDigest != t.EvidenceSetDigest || c.ContextFrameDigest != t.ContextFrameDigest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "review target currentness drifted")
	}
	return ValidateNow(c.Now, t.CreatedUnixNano, t.ExpiresUnixNano)
}
