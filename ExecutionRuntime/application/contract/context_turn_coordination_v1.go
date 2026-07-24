package contract

import (
	"bytes"
	"slices"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ContextTurnRefreshPreparedStateV1 = "prepared_pending"
	ContextTurnRefreshAppliedStateV1  = "applied_current"
)

type ContextTurnRefreshPrepareRequestV1 struct {
	ContractVersion       string                                           `json:"contract_version"`
	ID                    string                                           `json:"id"`
	Revision              core.Revision                                    `json:"revision"`
	ExecutionScopeDigest  core.Digest                                      `json:"execution_scope_digest"`
	RunID                 core.AgentRunID                                  `json:"run_id"`
	SourceSession         SingleCallSessionCoordinateV1                    `json:"source_session"`
	SessionApplicability  SingleCallSessionApplicabilitySourceCoordinateV1 `json:"session_applicability"`
	SourceTurn            SingleCallTurnCoordinateV1                       `json:"source_turn"`
	TurnApplicability     SingleCallTurnApplicabilitySourceCoordinateV1    `json:"turn_applicability"`
	ExpectedTargetTurn    uint32                                           `json:"expected_target_turn"`
	OpaqueContextRequest  []byte                                           `json:"opaque_context_request"`
	ContextRequestDigest  core.Digest                                      `json:"context_request_digest"`
	MemoryRequest         *ContextOwnerSourceRequestV1                     `json:"memory_request,omitempty"`
	KnowledgeRequest      *ContextOwnerSourceRequestV1                     `json:"knowledge_request,omitempty"`
	Memory                *ContextOwnerSourceEnvelopeV1                    `json:"memory,omitempty"`
	Knowledge             *ContextOwnerSourceEnvelopeV1                    `json:"knowledge,omitempty"`
	RequestedNotAfterNano int64                                            `json:"requested_not_after_unix_nano"`
	Digest                core.Digest                                      `json:"digest"`
}

func (r ContextTurnRefreshPrepareRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.context-turn-refresh-prepare", ContextTurnRefreshContractVersionV1, "ContextTurnRefreshPrepareRequestV1", copy)
}

func (r ContextTurnRefreshPrepareRequestV1) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != ContextTurnRefreshContractVersionV1 || !validSingleCallIDV1(r.ID) || r.Revision != 1 || r.ExecutionScopeDigest.Validate() != nil || !validSingleCallIDV1(string(r.RunID)) || r.SourceSession.Validate() != nil || r.SessionApplicability.Validate() != nil || r.SourceTurn.Validate() != nil || r.TurnApplicability.Validate() != nil || r.SourceTurn.Ordinal == ^uint32(0) || r.ExpectedTargetTurn != r.SourceTurn.Ordinal+1 || len(r.OpaqueContextRequest) == 0 || len(r.OpaqueContextRequest) > MaxContextOwnerRequestBytesV1 || core.DigestBytes(r.OpaqueContextRequest) != r.ContextRequestDigest || (r.Memory == nil && r.Knowledge == nil) || r.RequestedNotAfterNano <= 0 || !now.Before(time.Unix(0, r.RequestedNotAfterNano)) || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context refresh prepare request is incomplete or expired")
	}
	for owner, envelope := range map[ContextOwnerKindV1]*ContextOwnerSourceEnvelopeV1{ContextOwnerMemoryV1: r.Memory, ContextOwnerKnowledgeV1: r.Knowledge} {
		if envelope == nil {
			continue
		}
		if envelope.Owner != owner || envelope.Phase != ContextSourceCheckS1V1 || envelope.SourceSession != r.SourceSession || envelope.SessionApplicability != r.SessionApplicability || envelope.SourceTurn != r.SourceTurn || envelope.TurnApplicability != r.TurnApplicability || envelope.ValidateCurrent(now) != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "context refresh S1 owner envelope is not exact")
		}
	}
	for owner, pair := range map[ContextOwnerKindV1]struct {
		Request  *ContextOwnerSourceRequestV1
		Envelope *ContextOwnerSourceEnvelopeV1
	}{ContextOwnerMemoryV1: {r.MemoryRequest, r.Memory}, ContextOwnerKnowledgeV1: {r.KnowledgeRequest, r.Knowledge}} {
		if (pair.Request == nil) != (pair.Envelope == nil) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "context refresh Owner request/envelope presence drifted")
		}
		if pair.Request != nil && (pair.Request.Owner != owner || pair.Request.Phase != ContextSourceCheckS1V1 || pair.Request.ValidateCurrent(now) != nil || pair.Request.SourceSession != r.SourceSession || pair.Request.SourceTurn != r.SourceTurn) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "context refresh Owner request is not exact S1")
		}
	}
	d, err := r.DigestV1()
	if err != nil || d != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context refresh prepare request digest drifted")
	}
	return nil
}

func SealContextTurnRefreshPrepareRequestV1(r ContextTurnRefreshPrepareRequestV1) (ContextTurnRefreshPrepareRequestV1, error) {
	r.ContractVersion = ContextTurnRefreshContractVersionV1
	r.Revision = 1
	r.OpaqueContextRequest = bytes.Clone(r.OpaqueContextRequest)
	r.ContextRequestDigest = core.DigestBytes(r.OpaqueContextRequest)
	r.Digest = ""
	d, err := r.DigestV1()
	if err != nil {
		return ContextTurnRefreshPrepareRequestV1{}, err
	}
	r.Digest = d
	return r, nil
}

type ContextTurnRefreshPreparedV1 struct {
	ContractVersion        string                   `json:"contract_version"`
	AttemptRef             ContextRefreshExactRefV1 `json:"attempt_ref"`
	PendingDomainResultRef ContextRefreshExactRefV1 `json:"pending_domain_result_ref"`
	TransitionProofRef     ContextRefreshExactRefV1 `json:"transition_proof_ref"`
	ManifestRef            ContextRefreshExactRefV1 `json:"manifest_ref"`
	FrameRef               ContextRefreshExactRefV1 `json:"frame_ref"`
	GenerationRef          ContextRefreshExactRefV1 `json:"generation_ref"`
	StableSourceSetDigest  core.Digest              `json:"stable_source_set_digest"`
	S1AssociationSetDigest core.Digest              `json:"s1_association_set_digest"`
	CheckedUnixNano        int64                    `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                    `json:"expires_unix_nano"`
	State                  string                   `json:"state"`
	Digest                 core.Digest              `json:"digest"`
}

func (p ContextTurnRefreshPreparedV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.context-turn-refresh-prepared", ContextTurnRefreshContractVersionV1, "ContextTurnRefreshPreparedV1", copy)
}

func (p ContextTurnRefreshPreparedV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != ContextTurnRefreshContractVersionV1 || p.AttemptRef.Validate() != nil || p.PendingDomainResultRef.Validate() != nil || p.TransitionProofRef.Validate() != nil || p.ManifestRef.Validate() != nil || p.FrameRef.Validate() != nil || p.GenerationRef.Validate() != nil || p.StableSourceSetDigest.Validate() != nil || p.S1AssociationSetDigest.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.Before(time.Unix(0, p.CheckedUnixNano)) || !now.Before(time.Unix(0, p.ExpiresUnixNano)) || p.State != ContextTurnRefreshPreparedStateV1 || p.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context refresh prepared projection is incomplete or expired")
	}
	d, err := p.DigestV1()
	if err != nil || d != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context refresh prepared projection digest drifted")
	}
	return nil
}

func SealContextTurnRefreshPreparedV1(p ContextTurnRefreshPreparedV1) (ContextTurnRefreshPreparedV1, error) {
	p.ContractVersion = ContextTurnRefreshContractVersionV1
	p.State = ContextTurnRefreshPreparedStateV1
	p.Digest = ""
	d, err := p.DigestV1()
	if err != nil {
		return ContextTurnRefreshPreparedV1{}, err
	}
	p.Digest = d
	return p, nil
}

type ContextTurnRefreshApplyRequestV1 struct {
	ContractVersion        string                        `json:"contract_version"`
	Prepared               ContextTurnRefreshPreparedV1  `json:"prepared"`
	MemoryRequest          *ContextOwnerSourceRequestV1  `json:"memory_request,omitempty"`
	KnowledgeRequest       *ContextOwnerSourceRequestV1  `json:"knowledge_request,omitempty"`
	Memory                 *ContextOwnerSourceEnvelopeV1 `json:"memory,omitempty"`
	Knowledge              *ContextOwnerSourceEnvelopeV1 `json:"knowledge,omitempty"`
	S2AssociationSetDigest core.Digest                   `json:"s2_association_set_digest"`
	RequestedNotAfterNano  int64                         `json:"requested_not_after_unix_nano"`
	Digest                 core.Digest                   `json:"digest"`
}

func ContextSourceAssociationSetDigestV1(memory, knowledge *ContextOwnerSourceEnvelopeV1) (core.Digest, error) {
	type member struct {
		Owner                   ContextOwnerKindV1 `json:"owner"`
		StableAssociationDigest core.Digest        `json:"stable_association_digest"`
		EnvelopeDigest          core.Digest        `json:"envelope_digest"`
	}
	members := make([]member, 0, 2)
	if memory != nil {
		members = append(members, member{ContextOwnerMemoryV1, memory.StableAssociationDigest, memory.Digest})
	}
	if knowledge != nil {
		members = append(members, member{ContextOwnerKnowledgeV1, knowledge.StableAssociationDigest, knowledge.Digest})
	}
	return core.CanonicalJSONDigest("praxis.application.context-source-association-set", ContextTurnRefreshContractVersionV1, "ContextSourceAssociationSetV1", members)
}

func StableContextSourceSetDigestV1(memory, knowledge *ContextOwnerSourceEnvelopeV1) (core.Digest, error) {
	type member struct {
		Owner                   ContextOwnerKindV1 `json:"owner"`
		StableAssociationDigest core.Digest        `json:"stable_association_digest"`
	}
	members := make([]member, 0, 2)
	if memory != nil {
		members = append(members, member{ContextOwnerMemoryV1, memory.StableAssociationDigest})
	}
	if knowledge != nil {
		members = append(members, member{ContextOwnerKnowledgeV1, knowledge.StableAssociationDigest})
	}
	return core.CanonicalJSONDigest("praxis.application.context-stable-source-set", ContextTurnRefreshContractVersionV1, "ContextStableSourceSetV1", members)
}

func (r ContextTurnRefreshApplyRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.context-turn-refresh-apply", ContextTurnRefreshContractVersionV1, "ContextTurnRefreshApplyRequestV1", copy)
}

func (r ContextTurnRefreshApplyRequestV1) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != ContextTurnRefreshContractVersionV1 || r.Prepared.ValidateCurrent(now) != nil || (r.Memory == nil && r.Knowledge == nil) || r.S2AssociationSetDigest.Validate() != nil || r.RequestedNotAfterNano <= 0 || !now.Before(time.Unix(0, r.RequestedNotAfterNano)) || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context refresh apply request is incomplete or expired")
	}
	for owner, envelope := range map[ContextOwnerKindV1]*ContextOwnerSourceEnvelopeV1{ContextOwnerMemoryV1: r.Memory, ContextOwnerKnowledgeV1: r.Knowledge} {
		if envelope == nil {
			continue
		}
		if envelope.Owner != owner || envelope.Phase != ContextSourceCheckS2V1 || envelope.ValidateCurrent(now) != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "context refresh S2 owner envelope is not exact")
		}
	}
	for owner, pair := range map[ContextOwnerKindV1]struct {
		Request  *ContextOwnerSourceRequestV1
		Envelope *ContextOwnerSourceEnvelopeV1
	}{ContextOwnerMemoryV1: {r.MemoryRequest, r.Memory}, ContextOwnerKnowledgeV1: {r.KnowledgeRequest, r.Knowledge}} {
		if (pair.Request == nil) != (pair.Envelope == nil) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "context refresh S2 request/envelope presence drifted")
		}
		if pair.Request != nil && (pair.Request.Owner != owner || pair.Request.Phase != ContextSourceCheckS2V1 || pair.Request.ExpectedStableDigest != pair.Envelope.StableAssociationDigest || pair.Request.ValidateCurrent(now) != nil) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "context refresh Owner request is not exact S2")
		}
	}
	stable, err := StableContextSourceSetDigestV1(r.Memory, r.Knowledge)
	if err != nil || stable != r.Prepared.StableSourceSetDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context refresh stable source set drifted between S1 and S2")
	}
	association, err := ContextSourceAssociationSetDigestV1(r.Memory, r.Knowledge)
	if err != nil || association != r.S2AssociationSetDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context refresh S2 association set drifted")
	}
	d, err := r.DigestV1()
	if err != nil || d != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context refresh apply request digest drifted")
	}
	return nil
}

func SealContextTurnRefreshApplyRequestV1(r ContextTurnRefreshApplyRequestV1) (ContextTurnRefreshApplyRequestV1, error) {
	r.ContractVersion = ContextTurnRefreshContractVersionV1
	var err error
	r.S2AssociationSetDigest, err = ContextSourceAssociationSetDigestV1(r.Memory, r.Knowledge)
	if err != nil {
		return ContextTurnRefreshApplyRequestV1{}, err
	}
	r.Digest = ""
	d, err := r.DigestV1()
	if err != nil {
		return ContextTurnRefreshApplyRequestV1{}, err
	}
	r.Digest = d
	return r, nil
}

type ContextTurnRefreshInspectRequestV1 struct {
	ContractVersion string                   `json:"contract_version"`
	AttemptRef      ContextRefreshExactRefV1 `json:"attempt_ref"`
	Digest          core.Digest              `json:"digest"`
}

func (r ContextTurnRefreshInspectRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.context-turn-refresh-inspect", ContextTurnRefreshContractVersionV1, "ContextTurnRefreshInspectRequestV1", copy)
}

func (r ContextTurnRefreshInspectRequestV1) Validate() error {
	if r.ContractVersion != ContextTurnRefreshContractVersionV1 || r.AttemptRef.Validate() != nil || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context refresh inspect request is incomplete")
	}
	d, err := r.DigestV1()
	if err != nil || d != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context refresh inspect request digest drifted")
	}
	return nil
}

func SealContextTurnRefreshInspectRequestV1(r ContextTurnRefreshInspectRequestV1) (ContextTurnRefreshInspectRequestV1, error) {
	r.ContractVersion = ContextTurnRefreshContractVersionV1
	r.Digest = ""
	d, err := r.DigestV1()
	if err != nil {
		return ContextTurnRefreshInspectRequestV1{}, err
	}
	r.Digest = d
	return r, nil
}

type ContextTurnRefreshResultV1 struct {
	ContractVersion        string                    `json:"contract_version"`
	AttemptRef             ContextRefreshExactRefV1  `json:"attempt_ref"`
	PendingDomainResultRef ContextRefreshExactRefV1  `json:"pending_domain_result_ref"`
	TransitionProofRef     ContextRefreshExactRefV1  `json:"transition_proof_ref"`
	ManifestRef            ContextRefreshExactRefV1  `json:"manifest_ref"`
	FrameRef               ContextRefreshExactRefV1  `json:"frame_ref"`
	GenerationRef          ContextRefreshExactRefV1  `json:"generation_ref"`
	StableSourceSetDigest  core.Digest               `json:"stable_source_set_digest"`
	S1AssociationSetDigest core.Digest               `json:"s1_association_set_digest"`
	S2AssociationSetDigest core.Digest               `json:"s2_association_set_digest,omitempty"`
	ApplySettlementRef     *ContextRefreshExactRefV1 `json:"apply_settlement_ref,omitempty"`
	CurrentPointerRef      *ContextRefreshExactRefV1 `json:"current_pointer_ref,omitempty"`
	CheckedUnixNano        int64                     `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                     `json:"expires_unix_nano"`
	State                  string                    `json:"state"`
	Digest                 core.Digest               `json:"digest"`
}

func (r ContextTurnRefreshResultV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.context-turn-refresh-result", ContextTurnRefreshContractVersionV1, "ContextTurnRefreshResultV1", copy)
}

func (r ContextTurnRefreshResultV1) Validate() error {
	if r.ContractVersion != ContextTurnRefreshContractVersionV1 || r.AttemptRef.Validate() != nil || r.PendingDomainResultRef.Validate() != nil || r.TransitionProofRef.Validate() != nil || r.ManifestRef.Validate() != nil || r.FrameRef.Validate() != nil || r.GenerationRef.Validate() != nil || r.StableSourceSetDigest.Validate() != nil || r.S1AssociationSetDigest.Validate() != nil || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || (r.State != ContextTurnRefreshPreparedStateV1 && r.State != ContextTurnRefreshAppliedStateV1) || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context refresh result is incomplete")
	}
	if r.State == ContextTurnRefreshAppliedStateV1 {
		if r.ApplySettlementRef == nil || r.CurrentPointerRef == nil || r.ApplySettlementRef.Validate() != nil || r.CurrentPointerRef.Validate() != nil || r.S2AssociationSetDigest.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "applied context refresh result lacks exact settlement/current refs")
		}
	} else if r.ApplySettlementRef != nil || r.CurrentPointerRef != nil || r.S2AssociationSetDigest != "" {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "pending context refresh result claims applied refs")
	}
	d, err := r.DigestV1()
	if err != nil || d != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context refresh result digest drifted")
	}
	return nil
}

func (r ContextTurnRefreshResultV1) PreparedV1() (ContextTurnRefreshPreparedV1, error) {
	if err := r.Validate(); err != nil {
		return ContextTurnRefreshPreparedV1{}, err
	}
	return SealContextTurnRefreshPreparedV1(ContextTurnRefreshPreparedV1{
		AttemptRef: r.AttemptRef, PendingDomainResultRef: r.PendingDomainResultRef, TransitionProofRef: r.TransitionProofRef,
		ManifestRef: r.ManifestRef, FrameRef: r.FrameRef, GenerationRef: r.GenerationRef,
		StableSourceSetDigest: r.StableSourceSetDigest, S1AssociationSetDigest: r.S1AssociationSetDigest,
		CheckedUnixNano: r.CheckedUnixNano, ExpiresUnixNano: r.ExpiresUnixNano,
	})
}

func SealContextTurnRefreshResultV1(r ContextTurnRefreshResultV1) (ContextTurnRefreshResultV1, error) {
	r.ContractVersion = ContextTurnRefreshContractVersionV1
	r.Digest = ""
	d, err := r.DigestV1()
	if err != nil {
		return ContextTurnRefreshResultV1{}, err
	}
	r.Digest = d
	return r, nil
}

func cloneSourceEnvelopePtrV1(value *ContextOwnerSourceEnvelopeV1) *ContextOwnerSourceEnvelopeV1 {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Items = slices.Clone(value.Items)
	return &copy
}
