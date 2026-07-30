package modelinvoker

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	GovernedModelTurnContractVersionV2        = "praxis.model-invoker.governed-model-turn/v2"
	GovernedModelTurnObservationVersionV2     = "praxis.model-invoker.governed-model-turn-observation/v2"
	GovernedModelTurnAttemptContractVersionV2 = "praxis.model-invoker.governed-model-turn-attempt/v2"
	GovernedModelTurnProviderBoundaryKindV2   = "model-provider-tool-turn"
)

type GovernedModelTurnStateV2 string
type GovernedModelTurnOutcomeKindV2 string

const (
	GovernedModelTurnPreparedV2                GovernedModelTurnStateV2 = "prepared"
	GovernedModelTurnProviderBoundaryCrossedV2 GovernedModelTurnStateV2 = "provider_boundary_crossed"
	GovernedModelTurnObservedV2                GovernedModelTurnStateV2 = "observed"
	GovernedModelTurnUnknownV2                 GovernedModelTurnStateV2 = "unknown"
	GovernedModelTurnRejectedNoEffectV2        GovernedModelTurnStateV2 = "rejected_no_effect"
)

const (
	GovernedModelTurnCompletedTextV2     GovernedModelTurnOutcomeKindV2 = "completed_text"
	GovernedModelTurnToolCallCandidateV2 GovernedModelTurnOutcomeKindV2 = "tool_call_candidate"
)

type GovernedModelTurnRefV2 struct {
	ContractVersion        string                       `json:"contract_version"`
	ID                     string                       `json:"id"`
	Revision               core.Revision                `json:"revision"`
	Digest                 core.Digest                  `json:"digest"`
	PreparedRef            PreparedModelInvocationRefV1 `json:"prepared_ref"`
	MaterialRef            InvocationMaterialRefV1      `json:"material_ref"`
	AttemptRequestDigest   core.Digest                  `json:"attempt_request_digest"`
	RouteCallDigest        core.Digest                  `json:"route_call_digest"`
	DispatchSequence       uint64                       `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                       `json:"provider_attempt_ordinal"`
}

type GovernedModelTurnObservationV2 struct {
	ContractVersion      string                                    `json:"contract_version"`
	ID                   string                                    `json:"id"`
	Revision             core.Revision                             `json:"revision"`
	Digest               core.Digest                               `json:"digest"`
	TurnRef              GovernedModelTurnRefV2                    `json:"turn_ref"`
	RouteSelectionDigest core.Digest                               `json:"route_selection_digest"`
	Provider             ProviderID                                `json:"provider"`
	Protocol             Protocol                                  `json:"protocol"`
	ResponseID           string                                    `json:"response_id"`
	Model                string                                    `json:"model"`
	Status               ResponseStatus                            `json:"status"`
	StopReason           StopReason                                `json:"stop_reason"`
	Usage                Usage                                     `json:"usage"`
	OutcomeKind          GovernedModelTurnOutcomeKindV2            `json:"outcome_kind"`
	CompletedText        string                                    `json:"completed_text,omitempty"`
	ToolCallProjection   *ToolCallCandidateObservationProjectionV1 `json:"tool_call_projection,omitempty"`
	ObservedUnixNano     int64                                     `json:"observed_unix_nano"`
	ExpiresUnixNano      int64                                     `json:"expires_unix_nano"`
}

type GovernedModelTurnOutcomeV2 struct {
	ContractVersion        string                                              `json:"contract_version"`
	ID                     string                                              `json:"id"`
	Revision               core.Revision                                       `json:"revision"`
	Digest                 core.Digest                                         `json:"digest"`
	PreparedRef            PreparedModelInvocationRefV1                        `json:"prepared_ref"`
	CurrentRef             PreparedModelInvocationCurrentRefV1                 `json:"current_ref"`
	MaterialRef            InvocationMaterialRefV1                             `json:"material_ref"`
	AttemptRequestDigest   core.Digest                                         `json:"attempt_request_digest"`
	RouteCallDigest        core.Digest                                         `json:"route_call_digest"`
	DispatchSequence       uint64                                              `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                                              `json:"provider_attempt_ordinal"`
	State                  GovernedModelTurnStateV2                            `json:"state"`
	AckRef                 *PreparedModelInvocationCommitAckRefV1              `json:"ack_ref,omitempty"`
	DispatchReceipt        *PreparedModelInvocationDispatchValidationReceiptV1 `json:"dispatch_receipt,omitempty"`
	Observation            *GovernedModelTurnObservationV2                     `json:"observation,omitempty"`
	FailureCode            string                                              `json:"failure_code,omitempty"`
	CreatedUnixNano        int64                                               `json:"created_unix_nano"`
	UpdatedUnixNano        int64                                               `json:"updated_unix_nano"`
	ExpiresUnixNano        int64                                               `json:"expires_unix_nano"`
}

type GovernedModelTurnCommandV2 struct {
	PreparedRef            PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef             PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	MaterialRef            InvocationMaterialRefV1             `json:"material_ref"`
	AttemptRequestDigest   core.Digest                         `json:"attempt_request_digest"`
	RouteCallDigest        core.Digest                         `json:"route_call_digest"`
	DispatchSequence       uint64                              `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                              `json:"provider_attempt_ordinal"`
}

type GovernedModelTurnAttemptRefV2 struct {
	ContractVersion        string                              `json:"contract_version"`
	ID                     string                              `json:"id"`
	Digest                 core.Digest                         `json:"digest"`
	PreparedRef            PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef             PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	MaterialRef            InvocationMaterialRefV1             `json:"material_ref"`
	AttemptRequestDigest   core.Digest                         `json:"attempt_request_digest"`
	RouteCallDigest        core.Digest                         `json:"route_call_digest"`
	DispatchSequence       uint64                              `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                              `json:"provider_attempt_ordinal"`
}

type GovernedModelTurnMutationV2 struct {
	Outcome GovernedModelTurnOutcomeV2 `json:"outcome"`
	Applied bool                       `json:"applied"`
}
type GovernedModelTurnCASV2 struct {
	Expected GovernedModelTurnRefV2     `json:"expected"`
	Next     GovernedModelTurnOutcomeV2 `json:"next"`
}

type GovernedModelTurnRepositoryV2 interface {
	CreateGovernedModelTurnV2(context.Context, GovernedModelTurnOutcomeV2) (GovernedModelTurnMutationV2, error)
	CompareAndSwapGovernedModelTurnV2(context.Context, GovernedModelTurnCASV2) (GovernedModelTurnMutationV2, error)
	CompareAndSwapObservedGovernedModelTurnV2(context.Context, GovernedModelTurnCASV2) (GovernedModelTurnMutationV2, error)
	InspectGovernedModelTurnAttemptV2(context.Context, GovernedModelTurnAttemptRefV2) (GovernedModelTurnOutcomeV2, error)
	InspectExactGovernedModelTurnV2(context.Context, GovernedModelTurnRefV2) (GovernedModelTurnOutcomeV2, error)
	InspectCurrentGovernedModelTurnV2(context.Context, string) (GovernedModelTurnOutcomeV2, error)
	InspectExactGovernedModelTurnToolCallProjectionV2(context.Context, ToolCallCandidateObservationRefV1) (ToolCallCandidateObservationProjectionV1, error)
}
type GovernedModelTurnPortV2 interface {
	StartOrInspectGovernedModelTurnV2(context.Context, GovernedModelTurnCommandV2) (GovernedModelTurnOutcomeV2, error)
	InspectGovernedModelTurnAttemptV2(context.Context, GovernedModelTurnAttemptRefV2) (GovernedModelTurnOutcomeV2, error)
	InspectExactGovernedModelTurnV2(context.Context, GovernedModelTurnRefV2) (GovernedModelTurnOutcomeV2, error)
}

func (o GovernedModelTurnOutcomeV2) RefV2() GovernedModelTurnRefV2 {
	return GovernedModelTurnRefV2{ContractVersion: o.ContractVersion, ID: o.ID, Revision: o.Revision, Digest: o.Digest, PreparedRef: o.PreparedRef, MaterialRef: o.MaterialRef, AttemptRequestDigest: o.AttemptRequestDigest, RouteCallDigest: o.RouteCallDigest, DispatchSequence: o.DispatchSequence, ProviderAttemptOrdinal: o.ProviderAttemptOrdinal}
}
func (o GovernedModelTurnOutcomeV2) CloneV2() GovernedModelTurnOutcomeV2 {
	payload, _ := json.Marshal(o)
	var clone GovernedModelTurnOutcomeV2
	_ = core.DecodeStrictJSON(payload, &clone)
	return clone
}
func (o GovernedModelTurnObservationV2) CloneV2() GovernedModelTurnObservationV2 {
	clone := o
	if o.ToolCallProjection != nil {
		value := o.ToolCallProjection.Clone()
		clone.ToolCallProjection = &value
	}
	return clone
}
func (r GovernedModelTurnRefV2) Validate() error {
	if r.ContractVersion != GovernedModelTurnContractVersionV2 || strings.TrimSpace(r.ID) == "" || r.Revision == 0 || r.Digest.Validate() != nil || r.PreparedRef.Validate() != nil || r.MaterialRef.Validate() != nil || r.AttemptRequestDigest.Validate() != nil || r.RouteCallDigest.Validate() != nil || r.AttemptRequestDigest != r.PreparedRef.UnifiedRequestDigest || r.RouteCallDigest != r.MaterialRef.RouteCallDigest || r.DispatchSequence == 0 || r.ProviderAttemptOrdinal == 0 {
		return governedInvalidV1("governed model turn exact Ref is invalid")
	}
	id, err := governedModelTurnIdentityV2(r.PreparedRef, r.MaterialRef, r.DispatchSequence, r.ProviderAttemptOrdinal)
	if err != nil || id != r.ID {
		return governedConflictV1("governed model turn exact Ref identity drifted")
	}
	return nil
}
func (r GovernedModelTurnAttemptRefV2) Validate() error {
	if r.ContractVersion != GovernedModelTurnAttemptContractVersionV2 || strings.TrimSpace(r.ID) == "" || r.Digest.Validate() != nil || r.PreparedRef.Validate() != nil || r.CurrentRef.Validate() != nil || r.MaterialRef.Validate() != nil || r.CurrentRef.Prepared != r.PreparedRef || r.MaterialRef.PreparedRef != r.PreparedRef || r.AttemptRequestDigest != r.PreparedRef.UnifiedRequestDigest || r.RouteCallDigest != r.MaterialRef.RouteCallDigest || r.DispatchSequence == 0 || r.ProviderAttemptOrdinal == 0 {
		return governedInvalidV1("governed model turn AttemptRef is invalid")
	}
	id, err := governedModelTurnIdentityV2(r.PreparedRef, r.MaterialRef, r.DispatchSequence, r.ProviderAttemptOrdinal)
	if err != nil || id != r.ID {
		return governedConflictV1("governed model turn AttemptRef identity drifted")
	}
	digest, err := governedModelTurnAttemptRefDigestV2(r)
	if err != nil || digest != r.Digest {
		return governedConflictV1("governed model turn AttemptRef digest drifted")
	}
	return nil
}
func (o GovernedModelTurnOutcomeV2) ValidateAgainstAttemptRefV2(ref GovernedModelTurnAttemptRefV2) error {
	if o.Validate() != nil || ref.Validate() != nil || o.ID != ref.ID || o.PreparedRef != ref.PreparedRef || o.CurrentRef != ref.CurrentRef || o.MaterialRef != ref.MaterialRef || o.AttemptRequestDigest != ref.AttemptRequestDigest || o.RouteCallDigest != ref.RouteCallDigest || o.DispatchSequence != ref.DispatchSequence || o.ProviderAttemptOrdinal != ref.ProviderAttemptOrdinal {
		return governedConflictV1("governed model turn differs from stable AttemptRef")
	}
	return nil
}
func (o GovernedModelTurnOutcomeV2) Validate() error {
	if o.ContractVersion != GovernedModelTurnContractVersionV2 || o.CreatedUnixNano <= 0 || o.UpdatedUnixNano < o.CreatedUnixNano || o.ExpiresUnixNano <= o.CreatedUnixNano || o.PreparedRef.Validate() != nil || o.CurrentRef.Validate() != nil || o.MaterialRef.Validate() != nil || o.CurrentRef.Prepared != o.PreparedRef || o.MaterialRef.PreparedRef != o.PreparedRef || o.AttemptRequestDigest != o.PreparedRef.UnifiedRequestDigest || o.RouteCallDigest != o.MaterialRef.RouteCallDigest || o.ExpiresUnixNano > o.CurrentRef.ExpiresUnixNano || o.ExpiresUnixNano > o.MaterialRef.ExpiresUnixNano {
		return governedInvalidV1("governed model turn outcome fields are invalid")
	}
	switch o.State {
	case GovernedModelTurnPreparedV2:
		if o.Revision != 1 || o.AckRef != nil || o.DispatchReceipt != nil || o.Observation != nil || o.FailureCode != "" {
			return governedConflictV1("prepared model turn claims an effect")
		}
	case GovernedModelTurnProviderBoundaryCrossedV2:
		if o.Revision != 2 || o.AckRef == nil || o.DispatchReceipt == nil || o.Observation != nil || o.FailureCode != "" {
			return governedConflictV1("provider-boundary model turn is incomplete")
		}
	case GovernedModelTurnObservedV2:
		if o.Revision != 3 || o.AckRef == nil || o.DispatchReceipt == nil || o.Observation == nil || o.FailureCode != "" {
			return governedConflictV1("observed model turn is incomplete")
		}
	case GovernedModelTurnUnknownV2:
		if o.Revision != 3 || o.AckRef == nil || o.DispatchReceipt == nil || o.Observation != nil || strings.TrimSpace(o.FailureCode) == "" {
			return governedConflictV1("terminal model turn failure is incomplete")
		}
	case GovernedModelTurnRejectedNoEffectV2:
		if o.Revision != 2 || o.AckRef != nil || o.DispatchReceipt != nil || o.Observation != nil || strings.TrimSpace(o.FailureCode) == "" {
			return governedConflictV1("rejected no-effect turn must terminate before provider boundary")
		}
	default:
		return governedInvalidV1("governed model turn state is unsupported")
	}
	if o.AckRef != nil {
		if err := o.AckRef.Validate(); err != nil {
			return err
		}
		if o.AckRef.PreparedRef != o.PreparedRef || o.AckRef.CurrentRef != o.CurrentRef {
			return governedConflictV1("model turn ACK lineage drifted")
		}
	}
	if o.DispatchReceipt != nil {
		if err := o.DispatchReceipt.Validate(); err != nil {
			return err
		}
		if o.AckRef == nil || o.DispatchReceipt.AckRef != *o.AckRef || o.DispatchReceipt.PreparedRef != o.PreparedRef || o.DispatchReceipt.CurrentRef != o.CurrentRef || o.DispatchReceipt.DispatchSequence != o.DispatchSequence || o.DispatchReceipt.ProviderAttemptOrdinal != o.ProviderAttemptOrdinal || o.DispatchReceipt.AttemptRequestDigest != o.AttemptRequestDigest {
			return governedConflictV1("model turn dispatch receipt drifted")
		}
	}
	if o.Observation != nil {
		if err := o.Observation.Validate(); err != nil {
			return err
		}
		if o.Observation.TurnRef != o.RefAtBoundaryV2() {
			return governedConflictV1("model turn observation does not bind boundary revision")
		}
	}
	expected, err := governedModelTurnDigestV2(o)
	if err != nil || expected != o.Digest {
		return governedConflictV1("governed model turn digest drifted")
	}
	return o.RefV2().Validate()
}
func (o GovernedModelTurnOutcomeV2) RefAtBoundaryV2() GovernedModelTurnRefV2 {
	r := o.RefV2()
	if o.Observation != nil {
		r = o.Observation.TurnRef
	}
	return r
}
func (o GovernedModelTurnObservationV2) Validate() error {
	if o.ContractVersion != GovernedModelTurnObservationVersionV2 || strings.TrimSpace(o.ID) == "" || o.Revision != 1 || o.TurnRef.Validate() != nil || o.TurnRef.Revision != 2 || o.RouteSelectionDigest.Validate() != nil || strings.TrimSpace(string(o.Provider)) == "" || !o.Protocol.valid() || strings.TrimSpace(o.ResponseID) == "" || strings.TrimSpace(o.Model) == "" || o.Status != ResponseStatusCompleted || o.ObservedUnixNano <= 0 || o.ExpiresUnixNano <= o.ObservedUnixNano {
		return governedInvalidV1("governed model turn Observation is incomplete")
	}
	switch o.OutcomeKind {
	case GovernedModelTurnCompletedTextV2:
		if o.StopReason != StopReasonEndTurn || strings.TrimSpace(o.CompletedText) == "" || o.ToolCallProjection != nil {
			return governedConflictV1("completed_text payload is invalid")
		}
	case GovernedModelTurnToolCallCandidateV2:
		if o.StopReason != StopReasonToolCall || o.CompletedText != "" || o.ToolCallProjection == nil || o.ToolCallProjection.Validate() != nil || len(o.ToolCallProjection.Observation.Calls) != 1 {
			return governedConflictV1("tool_call_candidate payload is invalid")
		}
		if o.ToolCallProjection.Ref.InvocationID != o.TurnRef.PreparedRef.InvocationID || o.ToolCallProjection.Ref.InvocationDigest != o.TurnRef.PreparedRef.InvocationDigest || o.ToolCallProjection.Ref.Source.SourceSequence != o.TurnRef.DispatchSequence || o.ToolCallProjection.Ref.Source.ResponseID != o.ResponseID {
			return governedConflictV1("tool call projection lineage drifted")
		}
	default:
		return governedInvalidV1("model turn outcome kind is unsupported")
	}
	id, err := governedModelTurnObservationIdentityV2(o.TurnRef)
	if err != nil || id != o.ID {
		return governedConflictV1("model turn Observation identity drifted")
	}
	digest, err := governedModelTurnObservationDigestV2(o)
	if err != nil || digest != o.Digest {
		return governedConflictV1("model turn Observation digest drifted")
	}
	return nil
}
func SealGovernedModelTurnOutcomeV2(o GovernedModelTurnOutcomeV2) (GovernedModelTurnOutcomeV2, error) {
	o.ContractVersion = GovernedModelTurnContractVersionV2
	id, err := governedModelTurnIdentityV2(o.PreparedRef, o.MaterialRef, o.DispatchSequence, o.ProviderAttemptOrdinal)
	if err != nil {
		return GovernedModelTurnOutcomeV2{}, err
	}
	if o.ID != "" && o.ID != id {
		return GovernedModelTurnOutcomeV2{}, governedConflictV1("model turn ID drifted")
	}
	o.ID = id
	provided := o.Digest
	o.Digest = ""
	o = o.CloneV2()
	o.Digest, err = governedModelTurnDigestV2(o)
	if err != nil {
		return GovernedModelTurnOutcomeV2{}, err
	}
	if provided != "" && provided != o.Digest {
		return GovernedModelTurnOutcomeV2{}, governedConflictV1("model turn digest drifted")
	}
	if err := o.Validate(); err != nil {
		return GovernedModelTurnOutcomeV2{}, err
	}
	return o, nil
}
func SealGovernedModelTurnObservationV2(o GovernedModelTurnObservationV2) (GovernedModelTurnObservationV2, error) {
	o.ContractVersion = GovernedModelTurnObservationVersionV2
	o.Revision = 1
	id, err := governedModelTurnObservationIdentityV2(o.TurnRef)
	if err != nil {
		return GovernedModelTurnObservationV2{}, err
	}
	if o.ID != "" && o.ID != id {
		return GovernedModelTurnObservationV2{}, governedConflictV1("model turn Observation ID drifted")
	}
	o.ID = id
	provided := o.Digest
	o.Digest = ""
	o = o.CloneV2()
	o.Digest, err = governedModelTurnObservationDigestV2(o)
	if err != nil {
		return GovernedModelTurnObservationV2{}, err
	}
	if provided != "" && provided != o.Digest {
		return GovernedModelTurnObservationV2{}, governedConflictV1("model turn Observation digest drifted")
	}
	if err := o.Validate(); err != nil {
		return GovernedModelTurnObservationV2{}, err
	}
	return o, nil
}
func NewPreparedGovernedModelTurnV2(command GovernedModelTurnCommandV2, now time.Time) (GovernedModelTurnOutcomeV2, error) {
	if now.IsZero() || command.PreparedRef.Validate() != nil || command.CurrentRef.Validate() != nil || command.MaterialRef.Validate() != nil || command.CurrentRef.Prepared != command.PreparedRef || command.MaterialRef.PreparedRef != command.PreparedRef || command.AttemptRequestDigest != command.PreparedRef.UnifiedRequestDigest || command.RouteCallDigest != command.MaterialRef.RouteCallDigest || command.DispatchSequence == 0 || command.ProviderAttemptOrdinal == 0 {
		return GovernedModelTurnOutcomeV2{}, governedInvalidV1("governed model turn command is invalid")
	}
	expires := minGovernedExpiryV1(command.CurrentRef.ExpiresUnixNano, command.CurrentRef.NotAfterUnixNano, command.MaterialRef.ExpiresUnixNano)
	return SealGovernedModelTurnOutcomeV2(GovernedModelTurnOutcomeV2{Revision: 1, PreparedRef: command.PreparedRef, CurrentRef: command.CurrentRef, MaterialRef: command.MaterialRef, AttemptRequestDigest: command.AttemptRequestDigest, RouteCallDigest: command.RouteCallDigest, DispatchSequence: command.DispatchSequence, ProviderAttemptOrdinal: command.ProviderAttemptOrdinal, State: GovernedModelTurnPreparedV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
}
func DeriveGovernedModelTurnAttemptRefV2(command GovernedModelTurnCommandV2) (GovernedModelTurnAttemptRefV2, error) {
	if command.PreparedRef.Validate() != nil || command.CurrentRef.Validate() != nil || command.MaterialRef.Validate() != nil || command.CurrentRef.Prepared != command.PreparedRef || command.MaterialRef.PreparedRef != command.PreparedRef || command.AttemptRequestDigest != command.PreparedRef.UnifiedRequestDigest || command.RouteCallDigest != command.MaterialRef.RouteCallDigest || command.DispatchSequence == 0 || command.ProviderAttemptOrdinal == 0 {
		return GovernedModelTurnAttemptRefV2{}, governedInvalidV1("governed model turn command is invalid")
	}
	id, err := governedModelTurnIdentityV2(command.PreparedRef, command.MaterialRef, command.DispatchSequence, command.ProviderAttemptOrdinal)
	if err != nil {
		return GovernedModelTurnAttemptRefV2{}, err
	}
	ref := GovernedModelTurnAttemptRefV2{
		ContractVersion: GovernedModelTurnAttemptContractVersionV2,
		ID:              id, PreparedRef: command.PreparedRef, CurrentRef: command.CurrentRef, MaterialRef: command.MaterialRef,
		AttemptRequestDigest: command.AttemptRequestDigest, RouteCallDigest: command.RouteCallDigest,
		DispatchSequence: command.DispatchSequence, ProviderAttemptOrdinal: command.ProviderAttemptOrdinal,
	}
	ref.Digest, err = governedModelTurnAttemptRefDigestV2(ref)
	if err != nil {
		return GovernedModelTurnAttemptRefV2{}, err
	}
	return ref, ref.Validate()
}
func (c GovernedModelTurnCASV2) Validate() error {
	if err := c.Expected.Validate(); err != nil {
		return err
	}
	if err := c.Next.Validate(); err != nil {
		return err
	}
	if c.Expected.ID != c.Next.ID || c.Next.Revision != c.Expected.Revision+1 {
		return governedConflictV1("model turn CAS coordinates are not adjacent")
	}
	return nil
}
func ValidateGovernedModelTurnTransitionV2(current GovernedModelTurnOutcomeV2, next GovernedModelTurnOutcomeV2) error {
	if current.Validate() != nil || next.Validate() != nil {
		return governedInvalidV1("model turn transition input is invalid")
	}
	if current.ID != next.ID || current.PreparedRef != next.PreparedRef || current.CurrentRef != next.CurrentRef || current.MaterialRef != next.MaterialRef || current.AttemptRequestDigest != next.AttemptRequestDigest || current.RouteCallDigest != next.RouteCallDigest || current.DispatchSequence != next.DispatchSequence || current.ProviderAttemptOrdinal != next.ProviderAttemptOrdinal || current.CreatedUnixNano != next.CreatedUnixNano || next.ExpiresUnixNano > current.ExpiresUnixNano || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return governedConflictV1("model turn transition changed immutable lineage")
	}
	switch current.State {
	case GovernedModelTurnPreparedV2:
		if next.State != GovernedModelTurnProviderBoundaryCrossedV2 && next.State != GovernedModelTurnRejectedNoEffectV2 {
			return governedConflictV1("prepared turn may only cross provider boundary")
		}
		if next.State == GovernedModelTurnRejectedNoEffectV2 && next.ExpiresUnixNano != current.ExpiresUnixNano {
			return governedConflictV1("rejected turn changed the prepared expiry")
		}
	case GovernedModelTurnProviderBoundaryCrossedV2:
		if next.State != GovernedModelTurnObservedV2 && next.State != GovernedModelTurnUnknownV2 {
			return governedConflictV1("provider boundary may only become terminal")
		}
		if next.ExpiresUnixNano != current.ExpiresUnixNano {
			return governedConflictV1("terminal turn changed the provider-boundary expiry")
		}
	default:
		return governedConflictV1("terminal turn cannot transition")
	}
	return nil
}
func governedModelTurnIdentityV2(p PreparedModelInvocationRefV1, m InvocationMaterialRefV1, s uint64, o uint32) (string, error) {
	d, err := core.CanonicalJSONDigest("praxis.model-invoker.governed-model-turn", "v2", "GovernedModelTurnIdentityV2", struct {
		Prepared PreparedModelInvocationRefV1 `json:"prepared"`
		Material InvocationMaterialRefV1      `json:"material"`
		Sequence uint64                       `json:"sequence"`
		Ordinal  uint32                       `json:"ordinal"`
	}{p, m, s, o})
	if err != nil {
		return "", err
	}
	return "governed-model-turn/" + strings.TrimPrefix(string(d), "sha256:"), nil
}
func governedModelTurnObservationIdentityV2(r GovernedModelTurnRefV2) (string, error) {
	d, err := core.CanonicalJSONDigest("praxis.model-invoker.governed-model-turn", "v2", "GovernedModelTurnObservationIdentityV2", r)
	if err != nil {
		return "", err
	}
	return "governed-model-turn-observation/" + strings.TrimPrefix(string(d), "sha256:"), nil
}
func governedModelTurnDigestV2(o GovernedModelTurnOutcomeV2) (core.Digest, error) {
	o.Digest = ""
	return core.CanonicalJSONDigest("praxis.model-invoker.governed-model-turn", "v2", "GovernedModelTurnOutcomeV2", o)
}
func governedModelTurnObservationDigestV2(o GovernedModelTurnObservationV2) (core.Digest, error) {
	o.Digest = ""
	return core.CanonicalJSONDigest("praxis.model-invoker.governed-model-turn", "v2", "GovernedModelTurnObservationV2", o)
}
func governedModelTurnAttemptRefDigestV2(r GovernedModelTurnAttemptRefV2) (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest("praxis.model-invoker.governed-model-turn", "v2", "GovernedModelTurnAttemptRefV2", r)
}

var _ = reflect.DeepEqual
