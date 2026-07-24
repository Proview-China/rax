package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

const SandboxDomainResultKindV4 runtimeports.NamespacedNameV2 = "praxis.sandbox/domain-result"

type DomainResultFactReaderV4 interface {
	GetDomainResult(context.Context, string) (contract.SandboxDomainResultFact, error)
	GetReservation(context.Context, string) (contract.DomainReservation, error)
}

// DomainResultRuntimeBindingStoreV4 is a Sandbox-owned create-once exact
// binding. It persists the public Runtime ref but owns no Runtime Fact.
type DomainResultRuntimeBindingStoreV4 interface {
	CreateDomainResultRuntimeBindingV4(context.Context, runtimeports.OperationSettlementDomainResultFactRefV4) (runtimeports.OperationSettlementDomainResultFactRefV4, error)
	InspectDomainResultRuntimeBindingV4(context.Context, string) (runtimeports.OperationSettlementDomainResultFactRefV4, error)
}

type BindDomainResultRuntimeV4Request struct {
	EffectKind runtimeports.EffectKindV2
	ResultID   string
	Operation  runtimeports.OperationSubjectV3
	Attempt    runtimeports.OperationDispatchAttemptRefV3
}

type DomainResultCurrentAdapterV4 struct {
	facts    DomainResultFactReaderV4
	bindings DomainResultRuntimeBindingStoreV4
	owner    runtimeports.ProviderBindingRefV2
	schema   runtimeports.SchemaRefV2
	now      func() time.Time
}

func NewDomainResultCurrentAdapterV4(facts DomainResultFactReaderV4, bindings DomainResultRuntimeBindingStoreV4, owner runtimeports.ProviderBindingRefV2, schema runtimeports.SchemaRefV2, now func() time.Time) (*DomainResultCurrentAdapterV4, error) {
	if nilLikeV4(facts) || nilLikeV4(bindings) || nilLikeV4(now) || owner.Validate() != nil || schema.Validate() != nil {
		return nil, errors.New("domain result facts, binding store, owner, schema, and clock are required")
	}
	return &DomainResultCurrentAdapterV4{facts: facts, bindings: bindings, owner: owner, schema: schema, now: now}, nil
}

var _ runtimeports.OperationSettlementDomainResultCurrentReaderV4 = (*DomainResultCurrentAdapterV4)(nil)

func (a *DomainResultCurrentAdapterV4) BindDomainResultRuntimeV4(ctx context.Context, request BindDomainResultRuntimeV4Request) (runtimeports.OperationSettlementDomainResultFactRefV4, error) {
	if a == nil || nilLikeV4(ctx) {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, errors.New("domain result adapter or context is nil")
	}
	now := a.now()
	ref, _, _, err := a.derive(ctx, request.EffectKind, request.ResultID, request.Operation, request.Attempt, now)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, err
	}
	stored, err := a.bindings.CreateDomainResultRuntimeBindingV4(ctx, ref)
	if err != nil {
		recovered, inspectErr := a.bindings.InspectDomainResultRuntimeBindingV4(context.WithoutCancel(ctx), ref.ID)
		if inspectErr != nil || !runtimeports.SameOperationSettlementDomainResultFactRefV4(recovered, ref) {
			return runtimeports.OperationSettlementDomainResultFactRefV4{}, err
		}
		stored = recovered
	}
	if !runtimeports.SameOperationSettlementDomainResultFactRefV4(stored, ref) {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, errors.New("domain result binding owner returned another exact ref")
	}
	return stored, nil
}

func (a *DomainResultCurrentAdapterV4) InspectOperationSettlementDomainResultCurrentV4(ctx context.Context, effectKind runtimeports.EffectKindV2, expected runtimeports.OperationSettlementDomainResultFactRefV4) (runtimeports.OperationSettlementDomainResultCurrentV4, error) {
	if a == nil || nilLikeV4(ctx) {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, errors.New("domain result adapter or context is nil")
	}
	if err := expected.Validate(); err != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, err
	}
	stored, err := a.bindings.InspectDomainResultRuntimeBindingV4(ctx, expected.ID)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, err
	}
	if !runtimeports.SameOperationSettlementDomainResultFactRefV4(stored, expected) {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, errors.New("domain result current read expected another exact binding")
	}
	now := a.now()
	derived, result, reservation, err := a.derive(ctx, effectKind, expected.ID, expected.Operation, expected.Attempt, now)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, err
	}
	if !runtimeports.SameOperationSettlementDomainResultFactRefV4(derived, expected) {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, errors.New("domain result fact or Runtime coordinates drifted")
	}
	expires := minimumExpiry(result.Meta.ExpiresUnixNano, reservation.Meta.ExpiresUnixNano, reservation.Lease.ExpiresUnixNano)
	return runtimeports.SealOperationSettlementDomainResultCurrentV4(runtimeports.OperationSettlementDomainResultCurrentV4{
		EffectKind:      effectKind,
		Fact:            expected,
		CheckedUnixNano: now.UnixNano(),
		ExpiresUnixNano: expires,
	}, now)
}

func (a *DomainResultCurrentAdapterV4) derive(ctx context.Context, effectKind runtimeports.EffectKindV2, resultID string, operation runtimeports.OperationSubjectV3, attempt runtimeports.OperationDispatchAttemptRefV3, now time.Time) (runtimeports.OperationSettlementDomainResultFactRefV4, contract.SandboxDomainResultFact, contract.DomainReservation, error) {
	zero := runtimeports.OperationSettlementDomainResultFactRefV4{}
	if now.IsZero() || resultID == "" || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(effectKind)) != nil {
		return zero, contract.SandboxDomainResultFact{}, contract.DomainReservation{}, errors.New("domain result current coordinates are incomplete")
	}
	if err := operation.Validate(); err != nil {
		return zero, contract.SandboxDomainResultFact{}, contract.DomainReservation{}, err
	}
	if err := attempt.Validate(); err != nil {
		return zero, contract.SandboxDomainResultFact{}, contract.DomainReservation{}, err
	}
	result, err := a.facts.GetDomainResult(ctx, resultID)
	if err != nil {
		return zero, contract.SandboxDomainResultFact{}, contract.DomainReservation{}, err
	}
	if err := result.ValidateCurrent(now); err != nil {
		return zero, contract.SandboxDomainResultFact{}, contract.DomainReservation{}, fmt.Errorf("domain result currentness: %w", err)
	}
	reservation, err := a.facts.GetReservation(ctx, result.ReservationRef.ID)
	if err != nil {
		return zero, contract.SandboxDomainResultFact{}, contract.DomainReservation{}, err
	}
	if err := reservation.ValidateCurrent(now); err != nil {
		return zero, contract.SandboxDomainResultFact{}, contract.DomainReservation{}, fmt.Errorf("reservation currentness: %w", err)
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		return zero, contract.SandboxDomainResultFact{}, contract.DomainReservation{}, err
	}
	intentDigest := runtimeDigest(reservation.IntentDigest)
	if !contract.SameRef(result.ReservationRef, reservation.Meta.Ref()) || result.OperationID != reservation.OperationID || result.AttemptID != reservation.AttemptID || result.Kind != reservation.Kind || string(result.Kind) != string(effectKind) || result.Lease != reservation.Lease || string(operationDigest) != reservation.OperationSubjectDigest || attempt.OperationDigest != operationDigest || string(attempt.EffectID) != reservation.EffectID || uint64(attempt.IntentRevision) != reservation.IntentRevision || attempt.IntentDigest != intentDigest || attempt.AttemptID != reservation.AttemptID || string(operation.ExecutionScope.Identity.TenantID) != reservation.Lease.TenantID {
		return zero, contract.SandboxDomainResultFact{}, contract.DomainReservation{}, errors.New("domain result, reservation, operation, or attempt binding drifted")
	}
	payloadDigest, err := runtimecore.CanonicalJSONDigest("praxis.sandbox.domain-result", contract.ContractFamily, "SandboxDomainResultPayloadV4", struct {
		Disposition  contract.Disposition         `json:"disposition"`
		Payload      contract.DomainResultPayload `json:"payload"`
		EvidenceRefs []contract.Ref               `json:"evidence_refs"`
	}{result.Disposition, result.Payload, append([]contract.Ref{}, result.EvidenceRefs...)})
	if err != nil {
		return zero, contract.SandboxDomainResultFact{}, contract.DomainReservation{}, err
	}
	ref := runtimeports.OperationSettlementDomainResultFactRefV4{
		Owner: a.owner, Kind: SandboxDomainResultKindV4, ID: result.Meta.ID,
		Revision: runtimecore.Revision(result.Meta.Revision), Digest: runtimeDigest(result.Meta.Digest),
		TenantID: runtimecore.TenantID(reservation.Lease.TenantID), EffectID: runtimecore.EffectIntentID(reservation.EffectID),
		EffectRevision: runtimecore.Revision(reservation.IntentRevision), Operation: operation, OperationDigest: operationDigest,
		Attempt: attempt, Schema: a.schema, PayloadDigest: payloadDigest, PayloadRevision: 1,
		AuthoritativeTime: result.Meta.UpdatedUnixNano,
	}
	if err := ref.Validate(); err != nil {
		return zero, contract.SandboxDomainResultFact{}, contract.DomainReservation{}, err
	}
	return ref, result, reservation, nil
}

func nilLikeV4(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
