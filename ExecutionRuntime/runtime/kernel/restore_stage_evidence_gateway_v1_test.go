package kernel_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreStageEvidenceGatewayLostReplyConcurrentReplayAndExactInspectV1(t *testing.T) {
	fixture := newRestoreStageEvidenceFixtureV1(t)
	fixture.evidence.loseNextReply = true
	first, err := fixture.gateway.PublishRestoreStageEvidenceV1(context.Background(), fixture.request)
	if err != nil || first.Validate() != nil {
		t.Fatalf("lost reply recovery failed: ref=%+v err=%v", first, err)
	}
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ref, callErr := fixture.gateway.PublishRestoreStageEvidenceV1(context.Background(), fixture.request)
			if callErr == nil && ref != first {
				callErr = core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "concurrent replay returned another record")
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
		t.Fatalf("same Restore Stage fact appended %d records", fixture.evidence.writes)
	}
	record, err := fixture.gateway.InspectRestoreStageEvidenceV1(context.Background(), fixture.request)
	if err != nil || record.Ref != first || ports.ValidateRestoreStageEvidenceRecordV1(record, fixture.request.DomainResult) != nil {
		t.Fatalf("exact Inspect failed: record=%+v err=%v", record, err)
	}

	drift := fixture.request
	drift.DomainResult.ID += "-other"
	if _, err := fixture.gateway.InspectRestoreStageEvidenceV1(context.Background(), drift); err == nil {
		t.Fatal("same source key was allowed to prove another DomainResult")
	}
}

func TestRestoreStageEvidenceGatewayS1S2DriftAndTypedNilWriteNothingV1(t *testing.T) {
	fixture := newRestoreStageEvidenceFixtureV1(t)
	fixture.domains.drift = true
	if _, err := fixture.gateway.PublishRestoreStageEvidenceV1(context.Background(), fixture.request); err == nil {
		t.Fatal("Sandbox Evidence S1/S2 drift was accepted")
	}
	if fixture.evidence.writes != 0 {
		t.Fatal("drift wrote Evidence")
	}

	var domains *restoreStageEvidenceDomainReaderV1
	if _, err := kernel.NewRestoreStageEvidenceGatewayV1(domains, fixture.sources, fixture.evidence, func() time.Time { return fixture.now }); err == nil {
		t.Fatal("typed-nil DomainResult Evidence Reader was accepted")
	}
}

type restoreStageEvidenceFixtureV1 struct {
	now      time.Time
	request  ports.PublishRestoreStageEvidenceRequestV1
	domains  *restoreStageEvidenceDomainReaderV1
	sources  *restoreStageEvidenceSourceReaderV1
	evidence *restoreStageEvidenceLedgerV1
	gateway  *kernel.RestoreStageEvidenceGatewayV1
}

func newRestoreStageEvidenceFixtureV1(t *testing.T) *restoreStageEvidenceFixtureV1 {
	t.Helper()
	now := time.Unix(2_010_000_000, 0)
	governance, domain, _, _, err := runtimefakes.BuildRestoreStageSettlementFixtureV1("evidence-gateway", now)
	if err != nil {
		t.Fatal(err)
	}
	domainEvidence, err := ports.SealRestoreStageDomainEvidenceCurrentProjectionV1(ports.RestoreStageDomainEvidenceCurrentProjectionV1{
		Domain:  domain,
		Payload: ports.EvidencePayloadRefV2{Schema: domain.Fact.PayloadSchema, ContentDigest: domain.Fact.PayloadDigest, Revision: domain.Fact.PayloadRevision, Length: 512, Ref: "sandbox-fact://workspace-restore-stage/" + domain.Fact.ID},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	scope := domain.Fact.Operation.ExecutionScope
	producer := ports.EvidenceProducerBindingRefV2(domain.Fact.Owner)
	source := ports.EvidenceSourceRegistrationFactV2{
		ContractVersion: ports.EvidenceContractVersionV2, ID: "restore-stage-evidence-registration", Revision: 1,
		SourceID: ports.RestoreStageEvidenceSourceIDV1, SourceEpoch: 1,
		LedgerScope:    ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionInstance, TenantID: scope.Identity.TenantID, IdentityID: scope.Identity.ID, LineageID: scope.Lineage.ID, InstanceID: scope.Instance.ID},
		ExecutionScope: scope, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "restore-stage-scope-current", Digest: core.DigestBytes([]byte("restore-stage-scope-current")), Revision: 1}, CurrentScopeWatermark: 1,
		Producer: producer, Authority: ports.AuthorityBindingRefV2{Ref: "restore-stage-evidence-authority", Digest: core.DigestBytes([]byte("restore-stage-evidence-authority")), Revision: 1, Epoch: scope.AuthorityEpoch},
		ActionScopeDigest: domain.Fact.OperationDigest,
		Policy:            ports.EvidenceSourcePolicyBindingRefV2{Ref: "restore-stage-evidence-policy", Digest: core.DigestBytes([]byte("restore-stage-evidence-policy")), Revision: 1},
		ClassMappings:     []ports.EvidenceClassMappingV2{{Class: ports.RestoreStageEvidenceClassV1, Trust: ports.EvidenceTrustAuthoritativeFact}},
		AllowedKinds:      []ports.NamespacedNameV2{ports.RestoreStageEvidenceEventKindV1}, GapPolicy: ports.EvidenceGapStrictV2,
		NextSourceSequence: 1, State: ports.EvidenceSourceActive, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
	if err := source.Validate(); err != nil {
		t.Fatal(err)
	}
	ref, err := control.NewEvidenceSourceRegistrationRefV1(source)
	if err != nil {
		t.Fatal(err)
	}
	request := ports.PublishRestoreStageEvidenceRequestV1{Governance: governance, DomainResult: domain.Fact, SourceRegistration: ref}
	if err := request.Validate(now); err != nil {
		t.Fatal(err)
	}
	domains := &restoreStageEvidenceDomainReaderV1{value: domainEvidence, now: now}
	sources := &restoreStageEvidenceSourceReaderV1{value: source}
	evidence := &restoreStageEvidenceLedgerV1{records: make(map[ports.EvidenceSourceKeyV2]ports.EvidenceLedgerRecordV2), now: now}
	gateway, err := kernel.NewRestoreStageEvidenceGatewayV1(domains, sources, evidence, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return &restoreStageEvidenceFixtureV1{now: now, request: request, domains: domains, sources: sources, evidence: evidence, gateway: gateway}
}

type restoreStageEvidenceDomainReaderV1 struct {
	mu    sync.Mutex
	value ports.RestoreStageDomainEvidenceCurrentProjectionV1
	now   time.Time
	reads int
	drift bool
}

func (r *restoreStageEvidenceDomainReaderV1) InspectRestoreStageDomainEvidenceCurrentV1(_ context.Context, expected ports.RestoreStageDomainResultFactRefV1) (ports.RestoreStageDomainEvidenceCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !ports.SameRestoreStageDomainResultFactRefV1(r.value.Domain.Fact, expected) {
		return ports.RestoreStageDomainEvidenceCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "DomainResult not found")
	}
	r.reads++
	value := r.value
	if r.drift && r.reads > 1 {
		value.Payload.Ref += "-drift"
		var err error
		value, err = ports.SealRestoreStageDomainEvidenceCurrentProjectionV1(value, r.now)
		if err != nil {
			return ports.RestoreStageDomainEvidenceCurrentProjectionV1{}, err
		}
	}
	return value, nil
}

type restoreStageEvidenceSourceReaderV1 struct {
	value ports.EvidenceSourceRegistrationFactV2
}

func (r *restoreStageEvidenceSourceReaderV1) InspectSource(_ context.Context, id string) (ports.EvidenceSourceRegistrationFactV2, error) {
	if r == nil || r.value.ID != id {
		return ports.EvidenceSourceRegistrationFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "source not found")
	}
	return r.value, nil
}

type restoreStageEvidenceLedgerV1 struct {
	mu            sync.Mutex
	records       map[ports.EvidenceSourceKeyV2]ports.EvidenceLedgerRecordV2
	now           time.Time
	writes        int
	loseNextReply bool
}

func (e *restoreStageEvidenceLedgerV1) RegisterGovernedSource(context.Context, ports.EvidenceSourceRegistrationFactV2) (ports.EvidenceSourceRegistrationFactV2, error) {
	panic("unexpected source mutation")
}
func (e *restoreStageEvidenceLedgerV1) RenewGovernedSource(context.Context, ports.EvidenceSourceCASRequestV2) (ports.EvidenceSourceRegistrationFactV2, error) {
	panic("unexpected source mutation")
}
func (e *restoreStageEvidenceLedgerV1) AppendLateGoverned(context.Context, ports.EvidenceAppendLateRequestV2) (ports.EvidenceLedgerRecordV2, error) {
	panic("unexpected late append")
}
func (e *restoreStageEvidenceLedgerV1) InspectGovernedRecord(_ context.Context, ref ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, record := range e.records {
		if record.Ref == ref {
			return record, nil
		}
	}
	return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "record not found")
}
func (e *restoreStageEvidenceLedgerV1) InspectGovernedBySource(_ context.Context, key ports.EvidenceSourceKeyV2) (ports.EvidenceLedgerRecordV2, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	record, ok := e.records[key]
	if !ok {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "record not found")
	}
	return record, nil
}
func (e *restoreStageEvidenceLedgerV1) AppendGoverned(_ context.Context, request ports.EvidenceAppendRequestV2) (ports.EvidenceLedgerRecordV2, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := ports.EvidenceSourceKeyV2{RegistrationID: request.Candidate.RegistrationID, SourceEpoch: request.Candidate.SourceEpoch, SourceSequence: request.Candidate.SourceSequence}
	if current, ok := e.records[key]; ok {
		return current, nil
	}
	record, err := control.NewEvidenceLedgerRecordV2(request.Candidate, 1, ports.EvidenceGenesisDigestV2, e.now)
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
