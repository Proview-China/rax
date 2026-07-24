package mcp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func TestMCPConnectApplyStoreV1PublishesAvailability(t *testing.T) {
	f := newMCPConnectApplyFixtureV1(t)
	apply, err := f.apply.ApplyMCPConnectSettlementV1(context.Background(), f.domain.ObjectRef())
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.apply.ApplyMCPConnectSettlementV1(context.Background(), f.domain.ObjectRef())
	if err != nil {
		t.Fatal(err)
	}
	if apply.Ref != again.Ref || apply.DomainResult != f.domain.ObjectRef() || apply.Connection != f.domain.Connection || apply.Inspection.Digest != f.settlement.current.Digest {
		t.Fatalf("MCP Connect ApplySettlement lost exact closure: %#v", apply)
	}
	availability, err := f.apply.InspectCurrentMCPConnectionAvailabilityV1(context.Background(), f.connection.Ref, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if availability.Connection != f.connection.Ref || availability.ApplySettlement != apply.Ref || availability.DomainResult != f.domain.ObjectRef() {
		t.Fatalf("MCP Connection availability drifted: %#v", availability)
	}
}

func TestMCPConnectApplyStoreV1ConcurrentSingleWinner(t *testing.T) {
	f := newMCPConnectApplyFixtureV1(t)
	const workers = 64
	refs := make(chan toolcontract.ObjectRef, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fact, err := f.apply.ApplyMCPConnectSettlementV1(context.Background(), f.domain.ObjectRef())
			refs <- fact.Ref
			errs <- err
		}()
	}
	wg.Wait()
	close(refs)
	close(errs)
	var winner toolcontract.ObjectRef
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for ref := range refs {
		if winner.ID == "" {
			winner = ref
		} else if ref != winner {
			t.Fatalf("concurrent ApplySettlement winners drifted: %#v != %#v", ref, winner)
		}
	}
}

func TestMCPConnectApplyStoreV1FailsClosed(t *testing.T) {
	t.Run("unsettled_connection", func(t *testing.T) {
		f := newMCPConnectApplyFixtureV1(t)
		if _, err := f.apply.InspectCurrentMCPConnectionAvailabilityV1(context.Background(), f.connection.Ref, time.Second); !core.HasReason(err, core.ReasonEffectSettlementMissing) {
			t.Fatalf("unsettled availability error=%v", err)
		}
	})

	t.Run("nil_context", func(t *testing.T) {
		f := newMCPConnectApplyFixtureV1(t)
		if _, err := f.apply.ApplyMCPConnectSettlementV1(nil, f.domain.ObjectRef()); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context error=%v", err)
		}
	})

	t.Run("typed_nil", func(t *testing.T) {
		f := newMCPConnectApplyFixtureV1(t)
		var settlements *mcpConnectSettlementReaderV1
		if _, err := NewMCPConnectApplyStoreV1(f.domainFixture.store, f.domainFixture.connections, settlements, func() time.Time { return f.now }); err == nil {
			t.Fatal("typed-nil settlement reader was accepted")
		}
	})

	t.Run("settlement_owner_drift", func(t *testing.T) {
		f := newMCPConnectApplyFixtureV1(t)
		f.settlement.current.Owner.ManifestDigest = testDigestV1("another-settlement-owner")
		f.settlement.current.Digest = ""
		var err error
		f.settlement.current, err = runtimeports.SealOperationInspectionSettlementRefV4(f.settlement.current, f.now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.apply.ApplyMCPConnectSettlementV1(context.Background(), f.domain.ObjectRef()); !core.HasReason(err, core.ReasonSettlementOwnerMismatch) {
			t.Fatalf("settlement Owner drift error=%v", err)
		}
	})

	t.Run("closure_drift_between_reads", func(t *testing.T) {
		f := newMCPConnectApplyFixtureV1(t)
		f.settlement.drift = true
		if _, err := f.apply.ApplyMCPConnectSettlementV1(context.Background(), f.domain.ObjectRef()); !core.HasReason(err, core.ReasonBindingDrift) {
			t.Fatalf("S1/S2 closure drift error=%v", err)
		}
	})

	t.Run("expired_settlement", func(t *testing.T) {
		f := newMCPConnectApplyFixtureV1(t)
		f.now = f.now.Add(20 * time.Second)
		if _, err := f.apply.ApplyMCPConnectSettlementV1(context.Background(), f.domain.ObjectRef()); err == nil {
			t.Fatal("expired Runtime settlement was applied")
		}
	})
}

type mcpConnectApplyFixtureV1 struct {
	now           time.Time
	domainFixture *mcpConnectDomainResultFixtureV1
	domain        toolcontract.MCPConnectDomainResultFactV1
	connection    toolcontract.MCPConnectionFactV2
	settlement    *mcpConnectSettlementReaderV1
	apply         *MCPConnectApplyStoreV1
}

func newMCPConnectApplyFixtureV1(t *testing.T) *mcpConnectApplyFixtureV1 {
	t.Helper()
	domainFixture := newMCPConnectDomainResultFixtureV1(t)
	domain, err := domainFixture.store.CreateMCPConnectDomainResultV1(context.Background(), domainFixture.request)
	if err != nil {
		t.Fatal(err)
	}
	bundle, current := mcpConnectSettlementFixtureV1(t, domainFixture.now, domainFixture, domain)
	settlement := &mcpConnectSettlementReaderV1{current: current, bundle: bundle}
	fixture := &mcpConnectApplyFixtureV1{now: domainFixture.now, domainFixture: domainFixture, domain: domain, connection: domainFixture.connection, settlement: settlement}
	apply, err := NewMCPConnectApplyStoreV1(domainFixture.store, domainFixture.connections, settlement, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	fixture.apply = apply
	return fixture
}

func mcpConnectSettlementFixtureV1(t *testing.T, now time.Time, f *mcpConnectDomainResultFixtureV1, domain toolcontract.MCPConnectDomainResultFactV1) (runtimeports.OperationSettlementCommitBundleV4, runtimeports.OperationInspectionSettlementRefV4) {
	t.Helper()
	runtimeDomain := mcpConnectRuntimeDomainResultRefV1(f.connection.Provider, domain)
	prepareScope, err := runtimeports.DigestOperationSettlementEvidenceScopeV4(f.prepare.qualification.Scope)
	if err != nil {
		t.Fatal(err)
	}
	executeScope, err := runtimeports.DigestOperationSettlementEvidenceScopeV4(f.execute.qualification.Scope)
	if err != nil {
		t.Fatal(err)
	}
	prepare := runtimeports.OperationSettlementEvidenceBindingV4{Phase: runtimeports.OperationDispatchEnforcementPrepareV4, Consumption: f.prepare.consumption.RefV3(), IssuedQualification: f.prepare.consumption.Qualification, FinalQualification: f.prepare.qualification.RefV3(), Record: f.prepare.record.Ref, CandidateDigest: f.prepare.record.CandidateDigest, Handoff: f.prepare.handoff.RefV3(), Attempt: domain.Attempt, EnforcementPhase: domain.PrepareEnforcement, OperationScopeDigest: prepareScope}
	execute := runtimeports.OperationSettlementEvidenceBindingV4{Phase: runtimeports.OperationDispatchEnforcementExecuteV4, Consumption: f.execute.consumption.RefV3(), IssuedQualification: f.execute.consumption.Qualification, FinalQualification: f.execute.qualification.RefV3(), Record: f.execute.record.Ref, CandidateDigest: f.execute.record.CandidateDigest, Handoff: f.execute.handoff.RefV3(), Attempt: domain.Attempt, EnforcementPhase: domain.ExecuteEnforcement, OperationScopeDigest: executeScope}
	scopeSet, err := runtimeports.DigestOperationSettlementScopeSetV4([]runtimeports.OperationSettlementEvidenceBindingV4{prepare, execute})
	if err != nil {
		t.Fatal(err)
	}
	submission, err := runtimeports.SealOperationSettlementSubmissionV4(runtimeports.OperationSettlementSubmissionV4{ID: "mcp-connect-runtime-settlement", TenantID: domain.TenantID, Operation: domain.Operation, OperationDigest: domain.Attempt.OperationDigest, OperationScopeDigest: scopeSet, EffectID: domain.Attempt.EffectID, ExpectedEffectRevision: 1, Owner: domain.Owner, DomainResult: runtimeDomain, Evidence: []runtimeports.OperationSettlementEvidenceBindingV4{prepare, execute}, IdempotencyKey: "mcp-connect-runtime-settlement", ConflictDomain: testDigestV1("mcp-connect-settlement-conflict"), SettledUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	settlementFact, err := runtimeports.SealOperationSettlementFactV4(runtimeports.OperationSettlementFactV4{Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	settlementRef := settlementFact.RefV4()
	association, err := runtimeports.SealOperationSettlementEvidenceAssociationV4(runtimeports.OperationSettlementEvidenceAssociationV4{ID: "mcp-connect-runtime-association", Settlement: settlementRef, Prepare: prepare, Execute: execute})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := runtimeports.SealOperationSettlementTerminalGuardV4(runtimeports.OperationSettlementTerminalGuardV4{ID: "mcp-connect-runtime-guard", TenantID: domain.TenantID, OperationDigest: domain.Attempt.OperationDigest, EffectID: domain.Attempt.EffectID, Settlement: settlementRef})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := runtimeports.SealOperationSettlementTerminalProjectionV4(runtimeports.OperationSettlementTerminalProjectionV4{ID: "mcp-connect-runtime-projection", TenantID: domain.TenantID, OperationDigest: domain.Attempt.OperationDigest, EffectID: domain.Attempt.EffectID, Settlement: settlementRef, Association: association.RefV4(), Guard: guard.RefV4(), DomainResult: runtimeDomain})
	if err != nil {
		t.Fatal(err)
	}
	bundle := runtimeports.OperationSettlementCommitBundleV4{Settlement: settlementFact, Association: association, Guard: guard, Projection: projection}
	if err = bundle.Validate(); err != nil {
		t.Fatal(err)
	}
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{Settlement: settlementRef, Association: association.RefV4(), Guard: guard.RefV4(), Projection: projection.RefV4(), DomainResult: runtimeDomain, EffectFactRevision: 2, Owner: domain.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	return bundle, inspection
}

type mcpConnectSettlementReaderV1 struct {
	mu      sync.Mutex
	current runtimeports.OperationInspectionSettlementRefV4
	bundle  runtimeports.OperationSettlementCommitBundleV4
	calls   int
	drift   bool
}

func (r *mcpConnectSettlementReaderV1) InspectCurrentOperationSettlementV4(_ context.Context, request runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	if request.Operation != r.current.DomainResult.Operation || request.EffectID != r.current.Settlement.EffectID {
		return runtimeports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "MCP Connect Runtime settlement not found")
	}
	return r.current, nil
}

func (r *mcpConnectSettlementReaderV1) InspectOperationSettlementClosureV4(_ context.Context, request runtimeports.InspectOperationSettlementRequestV4) (runtimeports.OperationSettlementCommitBundleV4, error) {
	if request.Operation != r.current.DomainResult.Operation || request.SettlementID != r.bundle.Settlement.RefV4().ID {
		return runtimeports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "MCP Connect Runtime settlement closure not found")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	value := r.bundle
	if r.drift && r.calls > 1 {
		value.Settlement.Submission.IdempotencyKey = "mcp-connect-runtime-settlement-drift"
	}
	return value, nil
}

var _ MCPConnectSettlementInspectionReaderV1 = (*mcpConnectSettlementReaderV1)(nil)
