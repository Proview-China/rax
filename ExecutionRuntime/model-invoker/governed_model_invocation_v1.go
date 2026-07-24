package modelinvoker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	GovernedModelInvocationContractVersionV1  = "praxis.model-invoker.governed-model-invocation/v1"
	GovernedModelObservationContractVersionV1 = "praxis.model-invoker.governed-model-observation/v1"
	GovernedModelInvocationBindingVersionV1   = "praxis.model-invoker.governed-model-invocation-binding/v1"
	governedModelInvocationCanonicalDomainV1  = "praxis.model-invoker.governed-model-invocation"
	GovernedModelProviderBoundaryKindV1       = "model-provider-invoke"
)

type GovernedModelInvocationStateV1 string

const (
	GovernedModelInvocationPreparedV1                GovernedModelInvocationStateV1 = "prepared"
	GovernedModelInvocationProviderBoundaryCrossedV1 GovernedModelInvocationStateV1 = "provider_boundary_crossed"
	GovernedModelInvocationObservedV1                GovernedModelInvocationStateV1 = "observed"
	GovernedModelInvocationUnknownV1                 GovernedModelInvocationStateV1 = "unknown"
	GovernedModelInvocationRejectedNoEffectV1        GovernedModelInvocationStateV1 = "rejected_no_effect"
)

// GovernedModelInvocationRefV1 is a full exact historical coordinate. The
// request coordinates prevent an ID-only or revision-only read from changing
// the provider attempt being inspected.
type GovernedModelInvocationRefV1 struct {
	ContractVersion        string                       `json:"contract_version"`
	ID                     string                       `json:"id"`
	Revision               core.Revision                `json:"revision"`
	Digest                 core.Digest                  `json:"digest"`
	PreparedRef            PreparedModelInvocationRefV1 `json:"prepared_ref"`
	AttemptRequestDigest   core.Digest                  `json:"attempt_request_digest"`
	RouteCallDigest        core.Digest                  `json:"route_call_digest"`
	DispatchSequence       uint64                       `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                       `json:"provider_attempt_ordinal"`
}

type GovernedModelInvocationObservationV1 struct {
	ContractVersion      string                       `json:"contract_version"`
	ID                   string                       `json:"id"`
	Revision             core.Revision                `json:"revision"`
	Digest               core.Digest                  `json:"digest"`
	InvocationRef        GovernedModelInvocationRefV1 `json:"invocation_ref"`
	RouteID              upstream.RouteID             `json:"route_id"`
	RouteSelectionDigest core.Digest                  `json:"route_selection_digest"`
	Provider             ProviderID                   `json:"provider"`
	Protocol             Protocol                     `json:"protocol"`
	ResponseID           string                       `json:"response_id"`
	Model                string                       `json:"model"`
	Status               ResponseStatus               `json:"status"`
	StopReason           StopReason                   `json:"stop_reason"`
	StructuredOutput     json.RawMessage              `json:"structured_output"`
	Usage                Usage                        `json:"usage"`
	ObservedUnixNano     int64                        `json:"observed_unix_nano"`
	ExpiresUnixNano      int64                        `json:"expires_unix_nano"`
}

type GovernedModelInvocationFactV1 struct {
	ContractVersion        string                                              `json:"contract_version"`
	ID                     string                                              `json:"id"`
	Revision               core.Revision                                       `json:"revision"`
	Digest                 core.Digest                                         `json:"digest"`
	PreparedRef            PreparedModelInvocationRefV1                        `json:"prepared_ref"`
	CurrentRef             PreparedModelInvocationCurrentRefV1                 `json:"current_ref"`
	AttemptRequestDigest   core.Digest                                         `json:"attempt_request_digest"`
	RouteCallDigest        core.Digest                                         `json:"route_call_digest"`
	DispatchSequence       uint64                                              `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                                              `json:"provider_attempt_ordinal"`
	State                  GovernedModelInvocationStateV1                      `json:"state"`
	AckRef                 *PreparedModelInvocationCommitAckRefV1              `json:"ack_ref,omitempty"`
	DispatchReceipt        *PreparedModelInvocationDispatchValidationReceiptV1 `json:"dispatch_receipt,omitempty"`
	Observation            *GovernedModelInvocationObservationV1               `json:"observation,omitempty"`
	FailureCode            string                                              `json:"failure_code,omitempty"`
	CreatedUnixNano        int64                                               `json:"created_unix_nano"`
	UpdatedUnixNano        int64                                               `json:"updated_unix_nano"`
	ExpiresUnixNano        int64                                               `json:"expires_unix_nano"`
}

// GovernedModelInvocationCommandV1 carries the transient semantic RouteCall
// plus exact prepared/current coordinates. The repository stores only its
// canonical digests, never prompt text, credentials, Raw/Native events or
// provider metadata.
type GovernedModelInvocationCommandV1 struct {
	PreparedRef            PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef             PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	AttemptRequestDigest   core.Digest                         `json:"attempt_request_digest"`
	DispatchSequence       uint64                              `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                              `json:"provider_attempt_ordinal"`
	Call                   RouteCall                           `json:"call"`
}

type GovernedModelInvocationResultV1 struct {
	Invocation  GovernedModelInvocationFactV1         `json:"invocation"`
	Observation *GovernedModelInvocationObservationV1 `json:"observation,omitempty"`
}

func (r GovernedModelInvocationResultV1) Validate() error {
	if err := r.Invocation.Validate(); err != nil {
		return err
	}
	if r.Invocation.State != GovernedModelInvocationObservedV1 {
		if r.Observation != nil || r.Invocation.Observation != nil {
			return governedConflictV1("non-observed governed result carries an Observation")
		}
		return nil
	}
	if r.Observation == nil || r.Invocation.Observation == nil {
		return governedConflictV1("observed governed result is missing its Observation")
	}
	if err := r.Observation.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(r.Observation, r.Invocation.Observation) {
		return governedConflictV1("governed result Observation differs from the sealed invocation Fact")
	}
	return nil
}

type GovernedModelInvocationBindingV1 struct {
	ContractVersion        string                              `json:"contract_version"`
	ExecutionID            string                              `json:"execution_id"`
	UnifiedRequestDigest   core.Digest                         `json:"unified_request_digest"`
	PreparedPlanDigest     core.Digest                         `json:"prepared_plan_digest"`
	RouteID                upstream.RouteID                    `json:"route_id"`
	PreparedRef            PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef             PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	DispatchSequence       uint64                              `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                              `json:"provider_attempt_ordinal"`
}

type GovernedModelInvocationBindingRequestV1 struct {
	ExecutionID          string           `json:"execution_id"`
	UnifiedRequestDigest core.Digest      `json:"unified_request_digest"`
	PreparedPlanDigest   core.Digest      `json:"prepared_plan_digest"`
	RouteID              upstream.RouteID `json:"route_id"`
}

type GovernedModelInvocationBindingReaderV1 interface {
	InspectExactGovernedModelInvocationBindingV1(context.Context, GovernedModelInvocationBindingRequestV1) (GovernedModelInvocationBindingV1, error)
}

type GovernedModelInvocationPortV1 interface {
	StartOrInspectGovernedModelInvocationV1(context.Context, GovernedModelInvocationCommandV1) (GovernedModelInvocationResultV1, error)
	InspectExactModelInvocationV1(context.Context, GovernedModelInvocationRefV1) (GovernedModelInvocationResultV1, error)
}

func (r GovernedModelInvocationRefV1) Validate() error {
	if r.ContractVersion != GovernedModelInvocationContractVersionV1 || blankGovernedV1(r.ID) || r.Revision == 0 || r.DispatchSequence == 0 || r.ProviderAttemptOrdinal == 0 {
		return governedInvalidV1("governed invocation exact Ref is incomplete")
	}
	if err := r.Digest.Validate(); err != nil {
		return governedInvalidV1("governed invocation exact Ref digest is invalid")
	}
	if err := r.PreparedRef.Validate(); err != nil {
		return err
	}
	if err := r.AttemptRequestDigest.Validate(); err != nil {
		return err
	}
	if err := r.RouteCallDigest.Validate(); err != nil {
		return err
	}
	expected, err := governedModelInvocationIdentityV1(r.PreparedRef, r.AttemptRequestDigest, r.RouteCallDigest, r.DispatchSequence, r.ProviderAttemptOrdinal)
	if err != nil || expected != r.ID {
		return governedConflictV1("governed invocation exact Ref identity drifted")
	}
	return nil
}

func (f GovernedModelInvocationFactV1) RefV1() GovernedModelInvocationRefV1 {
	return GovernedModelInvocationRefV1{ContractVersion: f.ContractVersion, ID: f.ID, Revision: f.Revision, Digest: f.Digest, PreparedRef: f.PreparedRef, AttemptRequestDigest: f.AttemptRequestDigest, RouteCallDigest: f.RouteCallDigest, DispatchSequence: f.DispatchSequence, ProviderAttemptOrdinal: f.ProviderAttemptOrdinal}
}

func (f GovernedModelInvocationFactV1) CloneV1() GovernedModelInvocationFactV1 {
	clone := f
	if f.AckRef != nil {
		value := *f.AckRef
		clone.AckRef = &value
	}
	if f.DispatchReceipt != nil {
		value := f.DispatchReceipt.Clone()
		clone.DispatchReceipt = &value
	}
	if f.Observation != nil {
		value := f.Observation.CloneV1()
		clone.Observation = &value
	}
	return clone
}

func (f GovernedModelInvocationFactV1) Validate() error {
	if err := f.validateShapeV1(); err != nil {
		return err
	}
	expected, err := governedModelInvocationFactDigestV1(f)
	if err != nil || expected != f.Digest {
		return governedConflictV1("governed invocation Fact digest drifted")
	}
	return f.RefV1().Validate()
}

func (f GovernedModelInvocationFactV1) validateShapeV1() error {
	if f.ContractVersion != GovernedModelInvocationContractVersionV1 || blankGovernedV1(f.ID) || f.Revision == 0 || f.DispatchSequence == 0 || f.ProviderAttemptOrdinal == 0 || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano || f.ExpiresUnixNano <= f.CreatedUnixNano {
		return governedInvalidV1("governed invocation Fact fields are invalid")
	}
	if err := f.PreparedRef.Validate(); err != nil {
		return err
	}
	if err := f.CurrentRef.Validate(); err != nil {
		return err
	}
	if f.CurrentRef.Prepared != f.PreparedRef || f.AttemptRequestDigest != f.PreparedRef.UnifiedRequestDigest || f.CurrentRef.ExpiresUnixNano < f.ExpiresUnixNano || f.PreparedRef.InvocationID == "" {
		return governedConflictV1("governed invocation prepared/current/request lineage drifted")
	}
	if err := f.RouteCallDigest.Validate(); err != nil {
		return err
	}
	expectedID, err := governedModelInvocationIdentityV1(f.PreparedRef, f.AttemptRequestDigest, f.RouteCallDigest, f.DispatchSequence, f.ProviderAttemptOrdinal)
	if err != nil || expectedID != f.ID {
		return governedConflictV1("governed invocation ID drifted")
	}
	switch f.State {
	case GovernedModelInvocationPreparedV1:
		if f.Revision != 1 || f.UpdatedUnixNano >= f.ExpiresUnixNano || f.AckRef != nil || f.DispatchReceipt != nil || f.Observation != nil || f.FailureCode != "" {
			return governedConflictV1("prepared invocation claims boundary or result")
		}
	case GovernedModelInvocationProviderBoundaryCrossedV1:
		if f.Revision != 2 || f.UpdatedUnixNano >= f.ExpiresUnixNano || f.AckRef == nil || f.DispatchReceipt == nil || f.Observation != nil || f.FailureCode != "" {
			return governedConflictV1("provider boundary invocation is incomplete")
		}
	case GovernedModelInvocationObservedV1:
		if f.Revision != 3 || f.AckRef == nil || f.DispatchReceipt == nil || f.Observation == nil || f.FailureCode != "" {
			return governedConflictV1("observed invocation is incomplete")
		}
	case GovernedModelInvocationUnknownV1:
		if f.Revision != 3 || f.AckRef == nil || f.DispatchReceipt == nil || f.Observation != nil || blankGovernedV1(f.FailureCode) {
			return governedConflictV1("unknown invocation requires boundary and failure code")
		}
	case GovernedModelInvocationRejectedNoEffectV1:
		if f.Revision != 3 || f.AckRef == nil || f.DispatchReceipt == nil || f.Observation != nil || blankGovernedV1(f.FailureCode) {
			return governedConflictV1("rejected invocation requires boundary and failure code")
		}
	default:
		return governedInvalidV1("governed invocation state is unsupported")
	}
	if f.AckRef != nil {
		if err := f.AckRef.Validate(); err != nil {
			return err
		}
		if f.AckRef.PreparedRef != f.PreparedRef || f.AckRef.CurrentRef != f.CurrentRef || f.ExpiresUnixNano > f.AckRef.ExpiresUnixNano {
			return governedConflictV1("governed invocation ACK lineage drifted")
		}
	}
	if f.DispatchReceipt != nil {
		if err := f.DispatchReceipt.Validate(); err != nil {
			return err
		}
		if f.AckRef == nil || f.DispatchReceipt.PreparedRef != f.PreparedRef || f.DispatchReceipt.CurrentRef != f.CurrentRef || f.DispatchReceipt.AckRef != *f.AckRef || f.DispatchReceipt.DispatchSequence != f.DispatchSequence || f.DispatchReceipt.ProviderAttemptOrdinal != f.ProviderAttemptOrdinal || f.DispatchReceipt.AttemptRequestDigest != f.RouteCallDigest {
			return governedConflictV1("governed invocation dispatch receipt drifted")
		}
	}
	if f.Observation != nil {
		if err := f.Observation.Validate(); err != nil {
			return err
		}
		if f.Observation.InvocationRef.ID != f.ID || f.Observation.InvocationRef.Revision != 2 || f.Observation.InvocationRef.PreparedRef != f.PreparedRef || f.Observation.InvocationRef.AttemptRequestDigest != f.AttemptRequestDigest || f.Observation.InvocationRef.RouteCallDigest != f.RouteCallDigest || f.Observation.InvocationRef.DispatchSequence != f.DispatchSequence || f.Observation.InvocationRef.ProviderAttemptOrdinal != f.ProviderAttemptOrdinal || f.Observation.ExpiresUnixNano > f.ExpiresUnixNano {
			return governedConflictV1("governed invocation Observation lineage drifted")
		}
	}
	return nil
}

func SealGovernedModelInvocationFactV1(f GovernedModelInvocationFactV1) (GovernedModelInvocationFactV1, error) {
	if f.ContractVersion != "" && f.ContractVersion != GovernedModelInvocationContractVersionV1 {
		return GovernedModelInvocationFactV1{}, governedInvalidV1("governed invocation version is invalid")
	}
	f.ContractVersion = GovernedModelInvocationContractVersionV1
	expectedID, err := governedModelInvocationIdentityV1(f.PreparedRef, f.AttemptRequestDigest, f.RouteCallDigest, f.DispatchSequence, f.ProviderAttemptOrdinal)
	if err != nil {
		return GovernedModelInvocationFactV1{}, err
	}
	if f.ID != "" && f.ID != expectedID {
		return GovernedModelInvocationFactV1{}, governedConflictV1("supplied governed invocation ID drifted")
	}
	f.ID = expectedID
	provided := f.Digest
	f.Digest = ""
	if err := f.validateShapeV1(); err != nil {
		return GovernedModelInvocationFactV1{}, err
	}
	f.Digest, err = governedModelInvocationFactDigestV1(f)
	if err != nil {
		return GovernedModelInvocationFactV1{}, err
	}
	if provided != "" && provided != f.Digest {
		return GovernedModelInvocationFactV1{}, governedConflictV1("supplied governed invocation digest drifted")
	}
	return f, f.Validate()
}

func ValidateGovernedModelInvocationTransitionV1(current, next GovernedModelInvocationFactV1) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.ID != next.ID || current.PreparedRef != next.PreparedRef || current.CurrentRef != next.CurrentRef || current.AttemptRequestDigest != next.AttemptRequestDigest || current.RouteCallDigest != next.RouteCallDigest || current.DispatchSequence != next.DispatchSequence || current.ProviderAttemptOrdinal != next.ProviderAttemptOrdinal || current.CreatedUnixNano != next.CreatedUnixNano || next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano || next.ExpiresUnixNano > current.ExpiresUnixNano {
		return governedConflictV1("governed invocation transition changed immutable lineage")
	}
	switch current.State {
	case GovernedModelInvocationPreparedV1:
		if next.State != GovernedModelInvocationProviderBoundaryCrossedV1 {
			return governedConflictV1("prepared invocation may only cross the provider boundary")
		}
	case GovernedModelInvocationProviderBoundaryCrossedV1:
		if next.State != GovernedModelInvocationObservedV1 && next.State != GovernedModelInvocationUnknownV1 && next.State != GovernedModelInvocationRejectedNoEffectV1 {
			return governedConflictV1("provider boundary may only reach a terminal state")
		}
	default:
		return governedConflictV1("terminal governed invocation cannot transition")
	}
	if next.Observation != nil && next.Observation.InvocationRef != current.RefV1() {
		return governedConflictV1("governed Observation does not bind the exact provider-boundary revision")
	}
	return nil
}

func (o GovernedModelInvocationObservationV1) CloneV1() GovernedModelInvocationObservationV1 {
	clone := o
	clone.StructuredOutput = append(json.RawMessage(nil), o.StructuredOutput...)
	return clone
}

func (o GovernedModelInvocationObservationV1) Validate() error {
	if o.ContractVersion != GovernedModelObservationContractVersionV1 || blankGovernedV1(o.ID) || o.Revision != 1 || o.InvocationRef.Validate() != nil || blankGovernedV1(string(o.RouteID)) || o.RouteSelectionDigest.Validate() != nil || blankGovernedV1(string(o.Provider)) || !o.Protocol.valid() || blankGovernedV1(o.ResponseID) || blankGovernedV1(o.Model) || o.Status != ResponseStatusCompleted || o.StopReason != StopReasonEndTurn || o.ObservedUnixNano <= 0 || o.ExpiresUnixNano <= o.ObservedUnixNano {
		return governedInvalidV1("governed model Observation is incomplete")
	}
	if err := validateStrictJSONObjectV1(o.StructuredOutput); err != nil {
		return err
	}
	if o.Usage.InputTokens < 0 || o.Usage.OutputTokens < 0 || o.Usage.ReasoningTokens < 0 || o.Usage.CacheReadTokens < 0 || o.Usage.CacheWriteTokens < 0 || o.Usage.TotalTokens < 0 || o.Usage.TotalTokens < o.Usage.InputTokens || o.Usage.TotalTokens < o.Usage.OutputTokens {
		return governedInvalidV1("governed model Observation usage is invalid")
	}
	expectedID, err := governedModelObservationIdentityV1(o.InvocationRef)
	if err != nil || expectedID != o.ID {
		return governedConflictV1("governed model Observation ID drifted")
	}
	expected, err := governedModelObservationDigestV1(o)
	if err != nil || expected != o.Digest {
		return governedConflictV1("governed model Observation digest drifted")
	}
	return nil
}

// ResponseV1 reconstructs only provider-neutral sealed output. Raw request,
// Raw response, Native events and provider metadata are intentionally absent.
func (o GovernedModelInvocationObservationV1) ResponseV1() (Response, error) {
	if err := o.Validate(); err != nil {
		return Response{}, err
	}
	return Response{ID: o.ResponseID, Provider: o.Provider, Protocol: o.Protocol, Model: o.Model, Status: o.Status, StopReason: o.StopReason, Output: []OutputItem{{Type: OutputItemText, Text: string(o.StructuredOutput)}}, Usage: o.Usage}, nil
}

func SealGovernedModelInvocationObservationV1(o GovernedModelInvocationObservationV1) (GovernedModelInvocationObservationV1, error) {
	o.ContractVersion = GovernedModelObservationContractVersionV1
	o.Revision = 1
	expectedID, err := governedModelObservationIdentityV1(o.InvocationRef)
	if err != nil {
		return GovernedModelInvocationObservationV1{}, err
	}
	if o.ID != "" && o.ID != expectedID {
		return GovernedModelInvocationObservationV1{}, governedConflictV1("supplied governed Observation ID drifted")
	}
	o.ID = expectedID
	provided := o.Digest
	o.Digest = ""
	if err := validateStrictJSONObjectV1(o.StructuredOutput); err != nil {
		return GovernedModelInvocationObservationV1{}, err
	}
	o.StructuredOutput = append(json.RawMessage(nil), o.StructuredOutput...)
	o.Digest, err = governedModelObservationDigestV1(o)
	if err != nil {
		return GovernedModelInvocationObservationV1{}, err
	}
	if provided != "" && provided != o.Digest {
		return GovernedModelInvocationObservationV1{}, governedConflictV1("supplied governed Observation digest drifted")
	}
	return o, o.Validate()
}

func (b GovernedModelInvocationBindingV1) ValidateAgainstV1(expected GovernedModelInvocationBindingRequestV1) error {
	if b.ContractVersion != GovernedModelInvocationBindingVersionV1 || blankGovernedV1(b.ExecutionID) || b.DispatchSequence == 0 || b.ProviderAttemptOrdinal == 0 || b.PreparedRef.Validate() != nil || b.CurrentRef.Validate() != nil || b.UnifiedRequestDigest.Validate() != nil || b.PreparedPlanDigest.Validate() != nil || blankGovernedV1(string(b.RouteID)) {
		return governedInvalidV1("governed direct binding is incomplete")
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if b.ExecutionID != expected.ExecutionID || b.UnifiedRequestDigest != expected.UnifiedRequestDigest || b.PreparedPlanDigest != expected.PreparedPlanDigest || b.RouteID != expected.RouteID || b.PreparedRef.InvocationID != b.ExecutionID || b.PreparedRef.UnifiedRequestDigest != b.UnifiedRequestDigest || b.CurrentRef.Prepared != b.PreparedRef {
		return governedConflictV1("governed direct binding differs from exact request")
	}
	return nil
}

func (r GovernedModelInvocationBindingRequestV1) Validate() error {
	if blankGovernedV1(r.ExecutionID) || blankGovernedV1(string(r.RouteID)) || r.UnifiedRequestDigest.Validate() != nil || r.PreparedPlanDigest.Validate() != nil {
		return governedInvalidV1("governed binding request is incomplete")
	}
	return nil
}

func DigestGovernedRouteCallV1(call RouteCall) (core.Digest, error) {
	if err := validateGovernedRouteCallV1(call); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(governedModelInvocationCanonicalDomainV1, "v1", "GovernedRouteCallV1", call)
}

func DigestGovernedRouteSelectionV1(selection RouteSelection) (core.Digest, error) {
	if blankGovernedV1(string(selection.RouteID)) || blankGovernedV1(selection.Model) || blankGovernedV1(string(selection.AdapterID)) {
		return "", governedInvalidV1("governed route selection is incomplete")
	}
	return core.CanonicalJSONDigest(governedModelInvocationCanonicalDomainV1, "v1", "GovernedRouteSelectionV1", selection.Clone())
}

func validateGovernedRouteCallV1(call RouteCall) error {
	if blankGovernedV1(string(call.RouteID)) || call.Invocation == (upstream.InvocationContext{}) || blankGovernedV1(call.Request.Model) || len(call.Request.Input) == 0 || call.Request.Stream || call.Request.Provider != "" || call.Request.Protocol != ProtocolAuto || strings.TrimSpace(call.Request.Endpoint) != "" || call.Request.State != nil || len(call.Request.ProviderOptions) != 0 {
		return governedInvalidV1("governed RouteCall must be synchronous, RouteID-owned and continuation-free")
	}
	// Governed V1 is the read-only structured-output surface. Tool calls remain
	// candidates on legacy paths and are deliberately absent at this boundary.
	if len(call.Request.Tools) != 0 || (call.Request.ToolChoice.Mode != ToolChoiceNone && call.Request.ToolChoice.Mode != ToolChoiceAuto) || call.Request.ToolChoice.Name != "" || call.Request.Output.Type != OutputJSONSchema || call.Request.Output.Strict == nil || !*call.Request.Output.Strict {
		return governedInvalidV1("governed RouteCall requires no tools and one strict JSON schema output")
	}
	if err := validateStrictJSONObjectV1(call.Request.Output.Schema); err != nil {
		return err
	}
	if _, err := compileGovernedOutputSchemaV1(call.Request.Output.Schema); err != nil {
		return err
	}
	for _, item := range call.Request.Input {
		if item.FunctionCall != nil {
			if err := validateStrictJSONObjectV1(item.FunctionCall.Arguments); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateStrictJSONObjectV1(raw json.RawMessage) error {
	var value map[string]json.RawMessage
	if err := core.DecodeStrictJSON(raw, &value); err != nil || value == nil {
		return governedInvalidV1("structured JSON must be one strict object without duplicate keys")
	}
	return nil
}

// ValidateGovernedStructuredOutputV1 validates one strict provider-neutral
// JSON object against the exact schema carried by the governed RouteCall.
// External schema references are rejected so validation cannot introduce an
// implicit network/file effect at the provider result boundary.
func ValidateGovernedStructuredOutputV1(schemaDocument, output json.RawMessage) error {
	schema, err := compileGovernedOutputSchemaV1(schemaDocument)
	if err != nil {
		return err
	}
	if err := validateStrictJSONObjectV1(output); err != nil {
		return err
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(output))
	if err != nil {
		return governedInvalidV1("structured output cannot be decoded for schema validation")
	}
	if err := schema.Validate(value); err != nil {
		return governedConflictV1("structured output does not satisfy the exact governed JSON Schema")
	}
	return nil
}

func compileGovernedOutputSchemaV1(document json.RawMessage) (*jsonschema.Schema, error) {
	if err := validateStrictJSONObjectV1(document); err != nil {
		return nil, err
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return nil, governedInvalidV1("governed JSON Schema cannot be decoded")
	}
	if err := rejectGovernedExternalSchemaReferencesV1(value); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	const resource = "urn:praxis:model-invoker:governed-output-schema:v1"
	if err := compiler.AddResource(resource, value); err != nil {
		return nil, governedInvalidV1("governed JSON Schema resource is invalid")
	}
	schema, err := compiler.Compile(resource)
	if err != nil {
		return nil, governedInvalidV1("governed JSON Schema cannot be compiled")
	}
	return schema, nil
}

func rejectGovernedExternalSchemaReferencesV1(value any) error {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			switch key {
			case "$id":
				if id, ok := child.(string); !ok || id != "" {
					return governedInvalidV1("governed JSON Schema must not redefine its resource ID")
				}
			case "$ref", "$dynamicRef", "$recursiveRef":
				ref, ok := child.(string)
				if !ok || !strings.HasPrefix(ref, "#") {
					return governedInvalidV1("governed JSON Schema references must stay inside the sealed document")
				}
			}
			if err := rejectGovernedExternalSchemaReferencesV1(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range node {
			if err := rejectGovernedExternalSchemaReferencesV1(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func governedModelInvocationIdentityV1(prepared PreparedModelInvocationRefV1, request, route core.Digest, sequence uint64, ordinal uint32) (string, error) {
	if err := prepared.Validate(); err != nil {
		return "", err
	}
	if request.Validate() != nil || route.Validate() != nil || sequence == 0 || ordinal == 0 {
		return "", governedInvalidV1("governed invocation identity inputs are invalid")
	}
	digest, err := core.CanonicalJSONDigest(governedModelInvocationCanonicalDomainV1, "v1", "GovernedModelInvocationIdentityV1", struct {
		Prepared PreparedModelInvocationRefV1 `json:"prepared"`
		Request  core.Digest                  `json:"request"`
		Route    core.Digest                  `json:"route"`
		Sequence uint64                       `json:"sequence"`
		Ordinal  uint32                       `json:"provider_attempt_ordinal"`
	}{prepared, request, route, sequence, ordinal})
	if err != nil {
		return "", err
	}
	return "governed-model-invocation/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func governedModelObservationIdentityV1(ref GovernedModelInvocationRefV1) (string, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(governedModelInvocationCanonicalDomainV1, "v1", "GovernedModelObservationIdentityV1", ref)
	if err != nil {
		return "", err
	}
	return "governed-model-observation/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func governedModelInvocationFactDigestV1(f GovernedModelInvocationFactV1) (core.Digest, error) {
	f.Digest = ""
	return core.CanonicalJSONDigest(governedModelInvocationCanonicalDomainV1, "v1", "GovernedModelInvocationFactV1", f)
}

func governedModelObservationDigestV1(o GovernedModelInvocationObservationV1) (core.Digest, error) {
	o.Digest = ""
	return core.CanonicalJSONDigest(governedModelInvocationCanonicalDomainV1, "v1", "GovernedModelInvocationObservationV1", o)
}

type GovernedModelInvocationErrorKindV1 string

const (
	GovernedModelInvocationErrorInvalid       GovernedModelInvocationErrorKindV1 = "invalid"
	GovernedModelInvocationErrorConflict      GovernedModelInvocationErrorKindV1 = "conflict"
	GovernedModelInvocationErrorNotFound      GovernedModelInvocationErrorKindV1 = "authoritative_not_found"
	GovernedModelInvocationErrorUnavailable   GovernedModelInvocationErrorKindV1 = "unavailable"
	GovernedModelInvocationErrorIndeterminate GovernedModelInvocationErrorKindV1 = "indeterminate"
)

type GovernedModelInvocationErrorV1 struct {
	Kind               GovernedModelInvocationErrorKindV1
	Operation, Message string
	Err                error
}

func (e *GovernedModelInvocationErrorV1) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Operation == "" {
		return e.Message
	}
	return fmt.Sprintf("governed model invocation %s: %s", e.Operation, e.Message)
}
func (e *GovernedModelInvocationErrorV1) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
func (e *GovernedModelInvocationErrorV1) Is(target error) bool {
	other, ok := target.(*GovernedModelInvocationErrorV1)
	return ok && e != nil && other != nil && (other.Kind == "" || e.Kind == other.Kind)
}
func GovernedModelInvocationErrorKindOfV1(err error) GovernedModelInvocationErrorKindV1 {
	var typed *GovernedModelInvocationErrorV1
	if errors.As(err, &typed) && typed != nil {
		return typed.Kind
	}
	return ""
}

func governedErrorV1(kind GovernedModelInvocationErrorKindV1, operation, message string, err error) error {
	return &GovernedModelInvocationErrorV1{Kind: kind, Operation: operation, Message: message, Err: err}
}
func governedInvalidV1(message string) error {
	return governedErrorV1(GovernedModelInvocationErrorInvalid, "validate", message, nil)
}
func governedConflictV1(message string) error {
	return governedErrorV1(GovernedModelInvocationErrorConflict, "validate", message, nil)
}
func blankGovernedV1(value string) bool {
	return !utf8.ValidString(value) || strings.TrimSpace(value) == ""
}
func minGovernedExpiryV1(values ...int64) int64 {
	result := int64(0)
	for _, value := range values {
		if value > 0 && (result == 0 || value < result) {
			result = value
		}
	}
	return result
}

func NewPreparedGovernedModelInvocationForGatewayV1(command GovernedModelInvocationCommandV1, routeDigest core.Digest, now time.Time) (GovernedModelInvocationFactV1, error) {
	if now.IsZero() {
		return GovernedModelInvocationFactV1{}, governedInvalidV1("governed invocation clock is zero")
	}
	if command.AttemptRequestDigest != command.PreparedRef.UnifiedRequestDigest || command.CurrentRef.Prepared != command.PreparedRef || command.DispatchSequence == 0 || command.ProviderAttemptOrdinal == 0 {
		return GovernedModelInvocationFactV1{}, governedConflictV1("governed command lineage drifted")
	}
	expires := minGovernedExpiryV1(command.CurrentRef.ExpiresUnixNano, command.CurrentRef.NotAfterUnixNano)
	// The prepared revision must be bit-for-bit stable across delayed retries.
	// CurrentRef.CheckedUnixNano is part of the exact sealed input, while a
	// caller's wall clock is not; using the latter would turn a canonical retry
	// into changed content under the same derived ID.
	created := command.CurrentRef.CheckedUnixNano
	if created <= 0 || now.UnixNano() < created || now.UnixNano() >= expires {
		return GovernedModelInvocationFactV1{}, governedConflictV1("governed command is not current at creation")
	}
	return SealGovernedModelInvocationFactV1(GovernedModelInvocationFactV1{Revision: 1, PreparedRef: command.PreparedRef, CurrentRef: command.CurrentRef, AttemptRequestDigest: command.AttemptRequestDigest, RouteCallDigest: routeDigest, DispatchSequence: command.DispatchSequence, ProviderAttemptOrdinal: command.ProviderAttemptOrdinal, State: GovernedModelInvocationPreparedV1, CreatedUnixNano: created, UpdatedUnixNano: created, ExpiresUnixNano: expires})
}
