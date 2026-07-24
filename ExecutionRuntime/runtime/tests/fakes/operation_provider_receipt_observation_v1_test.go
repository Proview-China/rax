package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type operationReceiptReaderV1 struct {
	value ports.OperationProviderReceiptProjectionV1
}

func (r *operationReceiptReaderV1) InspectOperationProviderReceiptV1(_ context.Context, exact ports.OperationProviderReceiptRefV1) (ports.OperationProviderReceiptProjectionV1, error) {
	if r == nil || r.value.Ref != exact {
		return ports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "receipt not found")
	}
	return r.value, nil
}

type operationReceiptSourceReaderV1 struct {
	value ports.EvidenceSourceRegistrationFactV2
}

func (r *operationReceiptSourceReaderV1) InspectSource(_ context.Context, id string) (ports.EvidenceSourceRegistrationFactV2, error) {
	if r == nil || r.value.ID != id {
		return ports.EvidenceSourceRegistrationFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "source not found")
	}
	return r.value, nil
}

type operationReceiptEvidenceV1 struct {
	mu            sync.Mutex
	records       map[ports.EvidenceSourceKeyV2]ports.EvidenceLedgerRecordV2
	appendCalls   int
	writes        int
	loseNextReply bool
}

func (e *operationReceiptEvidenceV1) RegisterGovernedSource(context.Context, ports.EvidenceSourceRegistrationFactV2) (ports.EvidenceSourceRegistrationFactV2, error) {
	panic("unexpected source mutation")
}
func (e *operationReceiptEvidenceV1) RenewGovernedSource(context.Context, ports.EvidenceSourceCASRequestV2) (ports.EvidenceSourceRegistrationFactV2, error) {
	panic("unexpected source mutation")
}
func (e *operationReceiptEvidenceV1) AppendLateGoverned(context.Context, ports.EvidenceAppendLateRequestV2) (ports.EvidenceLedgerRecordV2, error) {
	panic("unexpected late append")
}
func (e *operationReceiptEvidenceV1) InspectGovernedRecord(_ context.Context, ref ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, record := range e.records {
		if record.Ref == ref {
			return record, nil
		}
	}
	return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "record not found")
}
func (e *operationReceiptEvidenceV1) InspectGovernedBySource(_ context.Context, key ports.EvidenceSourceKeyV2) (ports.EvidenceLedgerRecordV2, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	record, ok := e.records[key]
	if !ok {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "record not found")
	}
	return record, nil
}
func (e *operationReceiptEvidenceV1) AppendGoverned(_ context.Context, request ports.EvidenceAppendRequestV2) (ports.EvidenceLedgerRecordV2, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.appendCalls++
	key := ports.EvidenceSourceKeyV2{RegistrationID: request.Candidate.RegistrationID, SourceEpoch: request.Candidate.SourceEpoch, SourceSequence: request.Candidate.SourceSequence}
	if existing, ok := e.records[key]; ok {
		return existing, nil
	}
	record, err := control.NewEvidenceLedgerRecordV2(request.Candidate, 1, ports.EvidenceGenesisDigestV2, time.Unix(0, request.Candidate.ObservedUnixNano).Add(time.Millisecond))
	if err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	e.records[key], e.writes = record, e.writes+1
	if e.loseNextReply {
		e.loseNextReply = false
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost append reply")
	}
	return record, nil
}

type operationReceiptObservationV1 struct {
	mu    sync.Mutex
	value ports.ProviderAttemptObservationRefV2
	calls int
}

func (o *operationReceiptObservationV1) RecordGovernedProviderObservationV3(_ context.Context, request ports.RecordGovernedProviderObservationRequestV2) (ports.ProviderAttemptObservationRefV2, error) {
	if err := request.Observation.ValidateAgainstPrepared(request.Attempt.Prepared); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	ref, err := request.Observation.RefV2()
	if err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls++
	if o.value.Validate() == nil && o.value != ref {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "observation changed")
	}
	o.value = ref
	return ref, nil
}
func (o *operationReceiptObservationV1) InspectGovernedProviderObservationV3(_ context.Context, delegation ports.ExecutionDelegationRefV2, preparedID string) (ports.ProviderAttemptObservationRefV2, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.value.Delegation != delegation || o.value.PreparedAttemptID != preparedID {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "observation not found")
	}
	return o.value, nil
}

func TestOperationProviderReceiptObservationGatewayLostReplyAndConcurrentReplay(t *testing.T) {
	fixture := newOperationReceiptGatewayFixtureV1(t)
	fixture.evidence.loseNextReply = true
	first, err := fixture.gateway.RecordOperationProviderReceiptObservationV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Validate() != nil || fixture.evidence.writes != 1 {
		t.Fatalf("lost append reply did not recover exact record: ref=%#v writes=%d", first, fixture.evidence.writes)
	}

	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ref, callErr := fixture.gateway.RecordOperationProviderReceiptObservationV1(context.Background(), fixture.request)
			if callErr == nil && ref != first {
				callErr = core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "concurrent replay returned another observation")
			}
			errs <- callErr
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if fixture.evidence.writes != 1 {
		t.Fatalf("same receipt wrote Evidence %d times", fixture.evidence.writes)
	}
	inspected, err := fixture.gateway.InspectOperationProviderReceiptObservationV1(context.Background(), first.Delegation, first.PreparedAttemptID)
	if err != nil || inspected != first {
		t.Fatalf("exact Observation Inspect failed: %#v err=%v", inspected, err)
	}
}

func TestOperationProviderReceiptObservationGatewayFailClosed(t *testing.T) {
	fixture := newOperationReceiptGatewayFixtureV1(t)
	fixture.source.value.NextSourceSequence = 2
	if _, err := fixture.gateway.RecordOperationProviderReceiptObservationV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonEvidenceSourceStale) {
		t.Fatalf("non-first dedicated source passed: %v", err)
	}
	if fixture.evidence.writes != 0 || fixture.observations.calls != 0 {
		t.Fatal("stale source wrote Evidence or Observation")
	}
	if _, err := fixture.gateway.RecordOperationProviderReceiptObservationV1(nil, fixture.request); err == nil {
		t.Fatal("nil context passed")
	}
	var typedNil *operationReceiptReaderV1
	if _, err := kernel.NewOperationProviderReceiptObservationGatewayV1(typedNil, fixture.source, fixture.evidence, fixture.observations, time.Now); err == nil {
		t.Fatal("typed-nil receipt reader passed constructor")
	}
}

type operationReceiptGatewayFixtureV1 struct {
	gateway      *kernel.OperationProviderReceiptObservationGatewayV1
	request      ports.OperationProviderReceiptObservationRequestV1
	source       *operationReceiptSourceReaderV1
	evidence     *operationReceiptEvidenceV1
	observations *operationReceiptObservationV1
}

func newOperationReceiptGatewayFixtureV1(t *testing.T) operationReceiptGatewayFixtureV1 {
	t.Helper()
	runtimeFixture := newOperationFixtureForRunAndKindV3(t, "run-provider-receipt", nil, ports.OperationScopeEvidenceActionEffectKindV3)
	declared := beginAndDeclareOperationV3(t, runtimeFixture, "provider-receipt")
	provider := newGovernedProviderV2(runtimeFixture, declared.dispatch)
	preparation, err := provider.Prepare(context.Background(), ports.PrepareGovernedExecutionRequestV2{Delegation: declared.declaredRef, Intent: runtimeFixture.intent, Permit: declared.begun.Permit, Fence: declared.begun.Fence})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := declared.delegationGateway.CommitPreparedExecutionV2(context.Background(), ports.CommitPreparedExecutionRequestV2{Declared: declared.declaredRef, Intent: runtimeFixture.intent, Permit: declared.begun.Permit, Fence: declared.begun.Fence, Preparation: preparation})
	if err != nil {
		t.Fatal(err)
	}
	operationDigest, _ := runtimeFixture.intent.Operation.DigestV3()
	intentDigest, _ := runtimeFixture.intent.DigestV3()
	attemptRef := ports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: runtimeFixture.intent.ID, IntentRevision: runtimeFixture.intent.Revision, IntentDigest: intentDigest, PermitID: declared.begun.Permit.ID, PermitRevision: declared.begun.Permit.Revision, PermitDigest: declared.begun.PermitDigest, AttemptID: declared.begun.Permit.AttemptID, Delegation: &prepared.Delegation}
	attempt := ports.GovernedExecutionAttemptRefsV2{Admission: ports.OperationEffectAdmissionReceiptV3{OperationDigest: operationDigest, EffectID: runtimeFixture.intent.ID, IntentRevision: runtimeFixture.intent.Revision, IntentDigest: intentDigest, FactRevision: runtimeFixture.accepted.Revision, State: "accepted"}, PermitID: declared.begun.Permit.ID, PermitRevision: declared.begun.Permit.Revision, PermitDigest: declared.begun.PermitDigest, AttemptID: declared.begun.Permit.AttemptID, Delegation: prepared.Delegation, Prepared: prepared.Prepared, Enforcement: prepared.Enforcement}
	owner := ports.EffectOwnerRefV2{Role: ports.OwnerSettlement, ComponentID: runtimeFixture.intent.Provider.ComponentID, ManifestDigest: runtimeFixture.intent.Provider.ManifestDigest}
	receiptRef := ports.OperationProviderReceiptRefV1{Owner: owner, Kind: "praxis.mcp/protocol-receipt", ID: "provider-receipt-operation", Revision: 1, Digest: core.DigestBytes([]byte("provider-receipt-operation"))}
	payload := opaquePayloadV3("provider-response")
	payload.Inline, payload.Ref = nil, "mcp-receipt://provider-receipt-operation"
	projection, err := ports.SealOperationProviderReceiptProjectionV1(ports.OperationProviderReceiptProjectionV1{Ref: receiptRef, Operation: runtimeFixture.intent.Operation, OperationDigest: operationDigest, Prepared: prepared.Prepared, Attempt: attemptRef, Provider: runtimeFixture.intent.Provider, ProviderOperationRef: receiptRef.ID, Payload: payload, PayloadRevision: 1, ObservedUnixNano: runtimeFixture.now.UnixNano(), CheckedUnixNano: runtimeFixture.now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	sourceFact := ports.EvidenceSourceRegistrationFactV2{ContractVersion: ports.EvidenceContractVersionV2, ID: "provider-receipt-source-operation", Revision: 1, SourceID: ports.OperationProviderReceiptEvidenceSourceIDV1, SourceEpoch: 1, LedgerScope: ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionEffect, TenantID: runtimeFixture.intent.Operation.ExecutionScope.Identity.TenantID, IdentityID: runtimeFixture.intent.Operation.ExecutionScope.Identity.ID, LineageID: runtimeFixture.intent.Operation.ExecutionScope.Lineage.ID, InstanceID: runtimeFixture.intent.Operation.ExecutionScope.Instance.ID, RunID: runtimeFixture.intent.Operation.RunID, EffectID: runtimeFixture.intent.ID}, ExecutionScope: runtimeFixture.intent.Operation.ExecutionScope, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "provider-receipt-current-scope", Digest: core.DigestBytes([]byte("provider-receipt-current-scope")), Revision: 1}, CurrentScopeWatermark: 1, Producer: ports.EvidenceProducerBindingRefV2(runtimeFixture.intent.Provider), Authority: runtimeFixture.intent.Authority, ActionScopeDigest: runtimeFixture.intent.ActionScopeDigest, Policy: ports.EvidenceSourcePolicyBindingRefV2{Ref: "provider-receipt-policy", Digest: core.DigestBytes([]byte("provider-receipt-policy")), Revision: 1}, ClassMappings: []ports.EvidenceClassMappingV2{{Class: ports.OperationProviderReceiptEvidenceClassV1, Trust: ports.EvidenceTrustObservation}}, AllowedKinds: []ports.NamespacedNameV2{ports.OperationProviderReceiptEvidenceKindV1}, GapPolicy: ports.EvidenceGapStrictV2, NextSourceSequence: 1, State: ports.EvidenceSourceActive, CreatedUnixNano: runtimeFixture.now.Add(-time.Second).UnixNano(), UpdatedUnixNano: runtimeFixture.now.UnixNano(), ExpiresUnixNano: runtimeFixture.now.Add(time.Minute).UnixNano()}
	sourceRef, err := control.NewEvidenceSourceRegistrationRefV1(sourceFact)
	if err != nil {
		t.Fatal(err)
	}
	receipts := &operationReceiptReaderV1{projection}
	sources := &operationReceiptSourceReaderV1{sourceFact}
	evidence := &operationReceiptEvidenceV1{records: make(map[ports.EvidenceSourceKeyV2]ports.EvidenceLedgerRecordV2)}
	observations := &operationReceiptObservationV1{}
	gateway, err := kernel.NewOperationProviderReceiptObservationGatewayV1(receipts, sources, evidence, observations, func() time.Time { return runtimeFixture.now })
	if err != nil {
		t.Fatal(err)
	}
	request := ports.OperationProviderReceiptObservationRequestV1{Intent: runtimeFixture.intent, Permit: declared.begun.Permit, Fence: declared.begun.Fence, Attempt: attempt, Receipt: receiptRef, SourceRegistration: sourceRef}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	return operationReceiptGatewayFixtureV1{gateway: gateway, request: request, source: sources, evidence: evidence, observations: observations}
}
