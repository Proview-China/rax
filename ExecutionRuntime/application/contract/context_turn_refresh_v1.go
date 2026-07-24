package contract

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ContextTurnRefreshContractVersionV1 = "praxis.application/context-turn-refresh/v1"
	ContextOwnerMemoryV1                = ContextOwnerKindV1("memory")
	ContextOwnerKnowledgeV1             = ContextOwnerKindV1("knowledge")
	ContextSourceCheckS1V1              = ContextSourceCheckPhaseV1("s1")
	ContextSourceCheckS2V1              = ContextSourceCheckPhaseV1("s2")
	MaxContextOwnerRequestBytesV1       = 1 << 20
	MaxContextSourceItemsV1             = 256
)

type ContextOwnerKindV1 string
type ContextSourceCheckPhaseV1 string

type ContextTurnSourceCurrentV1 struct {
	ContractVersion      string                                           `json:"contract_version"`
	ExecutionScopeDigest core.Digest                                      `json:"execution_scope_digest"`
	RunID                core.AgentRunID                                  `json:"run_id"`
	Session              SingleCallSessionCoordinateV1                    `json:"session"`
	SessionApplicability SingleCallSessionApplicabilitySourceCoordinateV1 `json:"session_applicability"`
	Turn                 SingleCallTurnCoordinateV1                       `json:"turn"`
	TurnApplicability    SingleCallTurnApplicabilitySourceCoordinateV1    `json:"turn_applicability"`
	CheckedUnixNano      int64                                            `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                                            `json:"expires_unix_nano"`
	Digest               core.Digest                                      `json:"digest"`
}

func (c ContextTurnSourceCurrentV1) DigestV1() (core.Digest, error) {
	copy := c
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.context-turn-source-current", ContextTurnRefreshContractVersionV1, "ContextTurnSourceCurrentV1", copy)
}

func (c ContextTurnSourceCurrentV1) ValidateCurrent(now time.Time) error {
	if c.ContractVersion != ContextTurnRefreshContractVersionV1 || c.ExecutionScopeDigest.Validate() != nil || !validSingleCallIDV1(string(c.RunID)) || c.Session.Validate() != nil || c.SessionApplicability.Validate() != nil || c.Turn.Validate() != nil || c.TurnApplicability.Validate() != nil || c.CheckedUnixNano <= 0 || c.ExpiresUnixNano <= c.CheckedUnixNano || now.Before(time.Unix(0, c.CheckedUnixNano)) || !now.Before(time.Unix(0, c.ExpiresUnixNano)) || c.Session.CheckedUnixNano != c.CheckedUnixNano || c.Session.ExpiresUnixNano != c.ExpiresUnixNano || c.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context turn source current is incomplete or expired")
	}
	if c.Turn.ID != c.TurnApplicability.ID || c.Turn.Revision != c.TurnApplicability.Revision || c.Turn.Digest != c.TurnApplicability.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "context source Turn and applicability are not the same exact fact")
	}
	d, err := c.DigestV1()
	if err != nil || d != c.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context turn source current digest drifted")
	}
	return nil
}

func SealContextTurnSourceCurrentV1(c ContextTurnSourceCurrentV1) (ContextTurnSourceCurrentV1, error) {
	c.ContractVersion = ContextTurnRefreshContractVersionV1
	c.Digest = ""
	d, err := c.DigestV1()
	if err != nil {
		return ContextTurnSourceCurrentV1{}, err
	}
	c.Digest = d
	return c, nil
}

// ContextRefreshExactRefV1 is a cross-owner coordinate only. It never grants
// currentness; the named Owner reader must still inspect the referenced fact.
type ContextRefreshExactRefV1 struct {
	Kind     runtimeports.NamespacedNameV2 `json:"kind"`
	ID       string                        `json:"id"`
	Revision core.Revision                 `json:"revision"`
	Digest   core.Digest                   `json:"digest"`
}

func (r ContextRefreshExactRefV1) Validate() error {
	if runtimeports.ValidateNamespacedNameV2(r.Kind) != nil || !validSingleCallIDV1(r.ID) || r.Revision == 0 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context refresh exact ref is incomplete")
	}
	return nil
}

// ContextOwnerContentRefV1 mirrors the Owner ContentRef without inventing a
// revision that the Memory/Knowledge contract does not publish.
type ContextOwnerContentRefV1 struct {
	ID        string      `json:"id"`
	Digest    core.Digest `json:"digest"`
	Length    int64       `json:"length"`
	MediaType string      `json:"media_type"`
}

func (r ContextOwnerContentRefV1) Validate() error {
	if !validSingleCallIDV1(r.ID) || r.Digest.Validate() != nil || r.Length <= 0 || strings.TrimSpace(r.MediaType) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context owner content ref is incomplete")
	}
	return nil
}

type ContextOwnerSourceRequestV1 struct {
	ContractVersion       string                                           `json:"contract_version"`
	Owner                 ContextOwnerKindV1                               `json:"owner"`
	SourceSession         SingleCallSessionCoordinateV1                    `json:"source_session"`
	SessionApplicability  SingleCallSessionApplicabilitySourceCoordinateV1 `json:"session_applicability"`
	SourceTurn            SingleCallTurnCoordinateV1                       `json:"source_turn"`
	TurnApplicability     SingleCallTurnApplicabilitySourceCoordinateV1    `json:"turn_applicability"`
	OwnerRequest          []byte                                           `json:"owner_request"`
	OwnerRequestDigest    core.Digest                                      `json:"owner_request_digest"`
	ExpectedOwnerClosure  core.Digest                                      `json:"expected_owner_closure_digest,omitempty"`
	ExpectedStableDigest  core.Digest                                      `json:"expected_stable_digest,omitempty"`
	Phase                 ContextSourceCheckPhaseV1                        `json:"phase"`
	RequestedNotAfterNano int64                                            `json:"requested_not_after_unix_nano"`
	Digest                core.Digest                                      `json:"digest"`
}

func (r ContextOwnerSourceRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.context-owner-source-request", ContextTurnRefreshContractVersionV1, "ContextOwnerSourceRequestV1", copy)
}

func (r ContextOwnerSourceRequestV1) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != ContextTurnRefreshContractVersionV1 || !validContextOwnerV1(r.Owner) || r.SourceSession.Validate() != nil || r.SessionApplicability.Validate() != nil || r.SourceTurn.Validate() != nil || r.TurnApplicability.Validate() != nil || len(r.OwnerRequest) == 0 || len(r.OwnerRequest) > MaxContextOwnerRequestBytesV1 || core.DigestBytes(r.OwnerRequest) != r.OwnerRequestDigest || !validContextSourcePhaseV1(r.Phase) || r.RequestedNotAfterNano <= 0 || !now.Before(time.Unix(0, r.RequestedNotAfterNano)) || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context owner source request is incomplete or expired")
	}
	if r.Phase == ContextSourceCheckS1V1 && (r.ExpectedOwnerClosure != "" || r.ExpectedStableDigest != "") {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "S1 source request cannot preselect a stable digest")
	}
	if r.SourceTurn.ID != r.TurnApplicability.ID || r.SourceTurn.Revision != r.TurnApplicability.Revision || r.SourceTurn.Digest != r.TurnApplicability.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "context Owner source Turn and applicability are not exact")
	}
	if r.Phase == ContextSourceCheckS2V1 && (r.ExpectedOwnerClosure.Validate() != nil || r.ExpectedStableDigest.Validate() != nil) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "S2 source request requires Owner closure and Adapter association digests from S1")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context owner source request digest drifted")
	}
	return nil
}

func SealContextOwnerSourceRequestV1(r ContextOwnerSourceRequestV1) (ContextOwnerSourceRequestV1, error) {
	r.ContractVersion = ContextTurnRefreshContractVersionV1
	r.OwnerRequest = bytes.Clone(r.OwnerRequest)
	r.OwnerRequestDigest = core.DigestBytes(r.OwnerRequest)
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return ContextOwnerSourceRequestV1{}, err
	}
	r.Digest = digest
	return r, nil
}

type ContextOwnerSourceItemV1 struct {
	Rank             uint32                     `json:"rank"`
	ItemDigest       core.Digest                `json:"item_digest"`
	RecordRef        ContextRefreshExactRefV1   `json:"record_ref"`
	StableOwnerChain []ContextRefreshExactRefV1 `json:"stable_owner_chain"`
	ContentRef       ContextOwnerContentRefV1   `json:"content_ref"`
	TokenEstimate    uint64                     `json:"token_estimate"`
	Sensitivity      string                     `json:"sensitivity"`
	CitationDigest   core.Digest                `json:"citation_digest"`
	License          string                     `json:"license,omitempty"`
	ExpiresUnixNano  int64                      `json:"expires_unix_nano"`
	Digest           core.Digest                `json:"digest"`
}

func (i ContextOwnerSourceItemV1) DigestV1() (core.Digest, error) {
	copy := i
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.context-owner-source-item", ContextTurnRefreshContractVersionV1, "ContextOwnerSourceItemV1", copy)
}

func (i ContextOwnerSourceItemV1) ValidateCurrent(now time.Time) error {
	if i.ItemDigest.Validate() != nil || i.RecordRef.Validate() != nil || i.ContentRef.Validate() != nil || i.TokenEstimate == 0 || strings.TrimSpace(i.Sensitivity) == "" || i.CitationDigest.Validate() != nil || i.ExpiresUnixNano <= 0 || !now.Before(time.Unix(0, i.ExpiresUnixNano)) || i.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context owner source item is incomplete or expired")
	}
	seen := make(map[string]struct{}, len(i.StableOwnerChain))
	for _, ref := range i.StableOwnerChain {
		if ref.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context owner source chain contains an invalid ref")
		}
		key := string(ref.Kind) + "\x00" + ref.ID + "\x00" + fmt.Sprint(ref.Revision) + "\x00" + string(ref.Digest)
		if _, ok := seen[key]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "context owner source chain contains a duplicate ref")
		}
		seen[key] = struct{}{}
	}
	digest, err := i.DigestV1()
	if err != nil || digest != i.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context owner source item digest drifted")
	}
	return nil
}

func SealContextOwnerSourceItemV1(i ContextOwnerSourceItemV1) (ContextOwnerSourceItemV1, error) {
	i.StableOwnerChain = slices.Clone(i.StableOwnerChain)
	i.Digest = ""
	digest, err := i.DigestV1()
	if err != nil {
		return ContextOwnerSourceItemV1{}, err
	}
	i.Digest = digest
	return i, nil
}

type ContextOwnerSourceEnvelopeV1 struct {
	ContractVersion         string                                           `json:"contract_version"`
	ID                      string                                           `json:"id"`
	Revision                core.Revision                                    `json:"revision"`
	Owner                   ContextOwnerKindV1                               `json:"owner"`
	SourceSession           SingleCallSessionCoordinateV1                    `json:"source_session"`
	SessionApplicability    SingleCallSessionApplicabilitySourceCoordinateV1 `json:"session_applicability"`
	SourceTurn              SingleCallTurnCoordinateV1                       `json:"source_turn"`
	TurnApplicability       SingleCallTurnApplicabilitySourceCoordinateV1    `json:"turn_applicability"`
	AttemptInspectionRef    ContextRefreshExactRefV1                         `json:"attempt_inspection_ref"`
	CurrentProjectionRef    ContextRefreshExactRefV1                         `json:"current_projection_ref"`
	StableClosureDigest     core.Digest                                      `json:"stable_closure_digest"`
	StableAssociationDigest core.Digest                                      `json:"stable_association_digest"`
	Items                   []ContextOwnerSourceItemV1                       `json:"items"`
	Phase                   ContextSourceCheckPhaseV1                        `json:"phase"`
	CheckedUnixNano         int64                                            `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                                            `json:"expires_unix_nano"`
	Digest                  core.Digest                                      `json:"digest"`
}

func (e ContextOwnerSourceEnvelopeV1) stableDigestV1() (core.Digest, error) {
	type stableItem struct {
		Rank             uint32                     `json:"rank"`
		ItemDigest       core.Digest                `json:"item_digest"`
		RecordRef        ContextRefreshExactRefV1   `json:"record_ref"`
		StableOwnerChain []ContextRefreshExactRefV1 `json:"stable_owner_chain"`
		ContentRef       ContextOwnerContentRefV1   `json:"content_ref"`
		TokenEstimate    uint64                     `json:"token_estimate"`
		Sensitivity      string                     `json:"sensitivity"`
		CitationDigest   core.Digest                `json:"citation_digest"`
		License          string                     `json:"license,omitempty"`
	}
	items := make([]stableItem, len(e.Items))
	for index, item := range e.Items {
		items[index] = stableItem{item.Rank, item.ItemDigest, item.RecordRef, slices.Clone(item.StableOwnerChain), item.ContentRef, item.TokenEstimate, item.Sensitivity, item.CitationDigest, item.License}
	}
	body := struct {
		Owner                ContextOwnerKindV1                               `json:"owner"`
		SourceSession        SingleCallSessionCoordinateV1                    `json:"source_session"`
		SessionApplicability SingleCallSessionApplicabilitySourceCoordinateV1 `json:"session_applicability"`
		SourceTurn           SingleCallTurnCoordinateV1                       `json:"source_turn"`
		TurnApplicability    SingleCallTurnApplicabilitySourceCoordinateV1    `json:"turn_applicability"`
		StableClosureDigest  core.Digest                                      `json:"stable_closure_digest"`
		Items                []stableItem                                     `json:"items"`
	}{e.Owner, e.SourceSession, e.SessionApplicability, e.SourceTurn, e.TurnApplicability, e.StableClosureDigest, items}
	return core.CanonicalJSONDigest("praxis.application.context-owner-source-stable-association", ContextTurnRefreshContractVersionV1, "ContextOwnerSourceStableAssociationV1", body)
}

func (e ContextOwnerSourceEnvelopeV1) DigestV1() (core.Digest, error) {
	copy := e
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.context-owner-source-envelope", ContextTurnRefreshContractVersionV1, "ContextOwnerSourceEnvelopeV1", copy)
}

func (e ContextOwnerSourceEnvelopeV1) ValidateCurrent(now time.Time) error {
	if e.ContractVersion != ContextTurnRefreshContractVersionV1 || !validSingleCallIDV1(e.ID) || e.Revision != 1 || !validContextOwnerV1(e.Owner) || e.SourceSession.Validate() != nil || e.SessionApplicability.Validate() != nil || e.SourceTurn.Validate() != nil || e.TurnApplicability.Validate() != nil || e.AttemptInspectionRef.Validate() != nil || e.CurrentProjectionRef.Validate() != nil || e.StableClosureDigest.Validate() != nil || e.StableAssociationDigest.Validate() != nil || len(e.Items) == 0 || len(e.Items) > MaxContextSourceItemsV1 || !validContextSourcePhaseV1(e.Phase) || e.CheckedUnixNano <= 0 || e.ExpiresUnixNano <= e.CheckedUnixNano || now.Before(time.Unix(0, e.CheckedUnixNano)) || !now.Before(time.Unix(0, e.ExpiresUnixNano)) || e.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context owner source envelope is incomplete or expired")
	}
	if e.SourceTurn.ID != e.TurnApplicability.ID || e.SourceTurn.Revision != e.TurnApplicability.Revision || e.SourceTurn.Digest != e.TurnApplicability.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "context owner source envelope Turn applicability is not exact")
	}
	seen := make(map[string]struct{}, len(e.Items))
	for index, item := range e.Items {
		if item.Rank != uint32(index) || item.ValidateCurrent(now) != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "context owner source items are not canonical and current")
		}
		key := item.RecordRef.ID + "\x00" + string(item.RecordRef.Digest)
		if _, ok := seen[key]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "context owner source items contain a duplicate semantic record")
		}
		seen[key] = struct{}{}
	}
	stable, err := e.stableDigestV1()
	if err != nil || stable != e.StableAssociationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context owner source stable association drifted")
	}
	digest, err := e.DigestV1()
	if err != nil || digest != e.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context owner source envelope digest drifted")
	}
	return nil
}

func SealContextOwnerSourceEnvelopeV1(e ContextOwnerSourceEnvelopeV1) (ContextOwnerSourceEnvelopeV1, error) {
	e.ContractVersion = ContextTurnRefreshContractVersionV1
	e.Revision = 1
	e.Items = slices.Clone(e.Items)
	e.StableAssociationDigest = ""
	stable, err := e.stableDigestV1()
	if err != nil {
		return ContextOwnerSourceEnvelopeV1{}, err
	}
	e.StableAssociationDigest = stable
	e.Digest = ""
	digest, err := e.DigestV1()
	if err != nil {
		return ContextOwnerSourceEnvelopeV1{}, err
	}
	e.Digest = digest
	return e, nil
}

type ContextOwnerContentRequestV1 struct {
	ContractVersion       string                       `json:"contract_version"`
	SourceRequest         ContextOwnerSourceRequestV1  `json:"source_request"`
	Envelope              ContextOwnerSourceEnvelopeV1 `json:"envelope"`
	Rank                  uint32                       `json:"rank"`
	MaxBodyBytes          int64                        `json:"max_body_bytes"`
	RequestedNotAfterNano int64                        `json:"requested_not_after_unix_nano"`
	Digest                core.Digest                  `json:"digest"`
}

func (r ContextOwnerContentRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.context-owner-content-request", ContextTurnRefreshContractVersionV1, "ContextOwnerContentRequestV1", copy)
}

func SealContextOwnerContentRequestV1(r ContextOwnerContentRequestV1) (ContextOwnerContentRequestV1, error) {
	r.ContractVersion = ContextTurnRefreshContractVersionV1
	r.Digest = ""
	d, err := r.DigestV1()
	if err != nil {
		return ContextOwnerContentRequestV1{}, err
	}
	r.Digest = d
	return r, nil
}

func (r ContextOwnerContentRequestV1) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != ContextTurnRefreshContractVersionV1 || r.SourceRequest.ValidateCurrent(now) != nil || r.Envelope.ValidateCurrent(now) != nil || r.SourceRequest.Owner != r.Envelope.Owner || r.SourceRequest.SourceSession != r.Envelope.SourceSession || r.SourceRequest.SessionApplicability != r.Envelope.SessionApplicability || r.SourceRequest.SourceTurn != r.Envelope.SourceTurn || r.SourceRequest.TurnApplicability != r.Envelope.TurnApplicability || r.SourceRequest.Phase != r.Envelope.Phase || (r.SourceRequest.Phase == ContextSourceCheckS2V1 && r.SourceRequest.ExpectedStableDigest != r.Envelope.StableAssociationDigest) || int(r.Rank) >= len(r.Envelope.Items) || r.MaxBodyBytes <= 0 || r.MaxBodyBytes > r.Envelope.Items[r.Rank].ContentRef.Length || r.RequestedNotAfterNano <= 0 || !now.Before(time.Unix(0, r.RequestedNotAfterNano)) || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context owner content request is incomplete or expired")
	}
	d, err := r.DigestV1()
	if err != nil || d != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context owner content request digest drifted")
	}
	return nil
}

type ContextOwnerContentObservationV1 struct {
	ContractVersion      string                   `json:"contract_version"`
	ID                   string                   `json:"id"`
	Revision             core.Revision            `json:"revision"`
	Owner                ContextOwnerKindV1       `json:"owner"`
	EnvelopeRef          ContextRefreshExactRefV1 `json:"envelope_ref"`
	ProjectionItemDigest core.Digest              `json:"projection_item_digest"`
	ContentRef           ContextOwnerContentRefV1 `json:"content_ref"`
	ObservedLength       int64                    `json:"observed_length"`
	ObservedDigest       core.Digest              `json:"observed_digest"`
	CheckedUnixNano      int64                    `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                    `json:"expires_unix_nano"`
	Digest               core.Digest              `json:"digest"`
}

func (o ContextOwnerContentObservationV1) DigestV1() (core.Digest, error) {
	copy := o
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.context-owner-content-observation", ContextTurnRefreshContractVersionV1, "ContextOwnerContentObservationV1", copy)
}

func (o ContextOwnerContentObservationV1) ValidateCurrent(now time.Time) error {
	if o.ContractVersion != ContextTurnRefreshContractVersionV1 || !validSingleCallIDV1(o.ID) || o.Revision != 1 || !validContextOwnerV1(o.Owner) || o.EnvelopeRef.Validate() != nil || o.ProjectionItemDigest.Validate() != nil || o.ContentRef.Validate() != nil || o.ObservedLength <= 0 || o.ObservedDigest.Validate() != nil || o.CheckedUnixNano <= 0 || o.ExpiresUnixNano <= o.CheckedUnixNano || now.Before(time.Unix(0, o.CheckedUnixNano)) || !now.Before(time.Unix(0, o.ExpiresUnixNano)) || o.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context owner content observation is incomplete or expired")
	}
	d, err := o.DigestV1()
	if err != nil || d != o.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context owner content observation digest drifted")
	}
	return nil
}

func SealContextOwnerContentObservationV1(o ContextOwnerContentObservationV1) (ContextOwnerContentObservationV1, error) {
	o.ContractVersion = ContextTurnRefreshContractVersionV1
	o.Revision = 1
	o.Digest = ""
	d, err := o.DigestV1()
	if err != nil {
		return ContextOwnerContentObservationV1{}, err
	}
	o.Digest = d
	return o, nil
}

func validContextOwnerV1(owner ContextOwnerKindV1) bool {
	return owner == ContextOwnerMemoryV1 || owner == ContextOwnerKnowledgeV1
}

func validContextSourcePhaseV1(phase ContextSourceCheckPhaseV1) bool {
	return phase == ContextSourceCheckS1V1 || phase == ContextSourceCheckS2V1
}
