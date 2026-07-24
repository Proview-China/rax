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

func TestMCPDiscoveryPageApplyStoreV1PublishesAppliedCurrent(t *testing.T) {
	f := newMCPDiscoveryPageApplyFixtureV1(t)
	apply, err := f.apply.ApplyMCPDiscoveryPageSettlementV1(context.Background(), f.domain.ObjectRef())
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.apply.ApplyMCPDiscoveryPageSettlementV1(context.Background(), f.domain.ObjectRef())
	if err != nil || again.Ref != apply.Ref {
		t.Fatalf("idempotent Apply=%#v err=%v", again, err)
	}
	current, err := f.apply.InspectCurrentMCPDiscoveryPageAppliedV1(context.Background(), f.domain.Command, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if current.DomainResult != f.domain.ObjectRef() || current.ApplySettlement != apply.Ref || current.ProtocolReceipt != f.domain.ProtocolReceipt || current.Namespace != f.domain.Namespace {
		t.Fatalf("applied current drifted: %#v", current)
	}
}

func TestMCPDiscoveryPageApplyStoreV1ConcurrentAndFailsClosed(t *testing.T) {
	t.Run("64_single_winner", func(t *testing.T) {
		f := newMCPDiscoveryPageApplyFixtureV1(t)
		const workers = 64
		refs := make(chan toolcontract.ObjectRef, workers)
		errs := make(chan error, workers)
		var group sync.WaitGroup
		for range workers {
			group.Add(1)
			go func() {
				defer group.Done()
				v, e := f.apply.ApplyMCPDiscoveryPageSettlementV1(context.Background(), f.domain.ObjectRef())
				refs <- v.Ref
				errs <- e
			}()
		}
		group.Wait()
		close(refs)
		close(errs)
		var winner toolcontract.ObjectRef
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		for ref := range refs {
			if winner.ID == "" {
				winner = ref
			} else if ref != winner {
				t.Fatal("multiple Apply winners")
			}
		}
	})
	t.Run("unsettled", func(t *testing.T) {
		f := newMCPDiscoveryPageApplyFixtureV1(t)
		if _, err := f.apply.InspectCurrentMCPDiscoveryPageAppliedV1(context.Background(), f.domain.Command, time.Second); !core.HasReason(err, core.ReasonEffectSettlementMissing) {
			t.Fatalf("unsettled=%v", err)
		}
	})
	t.Run("owner_drift", func(t *testing.T) {
		f := newMCPDiscoveryPageApplyFixtureV1(t)
		f.settlement.current.Owner.ManifestDigest = testDigestV1("other-owner")
		f.settlement.current.Digest = ""
		f.settlement.current, _ = runtimeports.SealOperationInspectionSettlementRefV4(f.settlement.current, f.now)
		if _, err := f.apply.ApplyMCPDiscoveryPageSettlementV1(context.Background(), f.domain.ObjectRef()); !core.HasReason(err, core.ReasonSettlementOwnerMismatch) {
			t.Fatalf("owner drift=%v", err)
		}
	})
	t.Run("closure_drift", func(t *testing.T) {
		f := newMCPDiscoveryPageApplyFixtureV1(t)
		f.settlement.drift = true
		if _, err := f.apply.ApplyMCPDiscoveryPageSettlementV1(context.Background(), f.domain.ObjectRef()); !core.HasReason(err, core.ReasonBindingDrift) {
			t.Fatalf("closure drift=%v", err)
		}
	})
	t.Run("nil_context", func(t *testing.T) {
		f := newMCPDiscoveryPageApplyFixtureV1(t)
		if _, err := f.apply.ApplyMCPDiscoveryPageSettlementV1(nil, f.domain.ObjectRef()); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context=%v", err)
		}
	})
}

type mcpDiscoveryPageApplyFixtureV1 struct {
	now           time.Time
	domainFixture *mcpDiscoveryPageDomainFixtureV1
	domain        toolcontract.MCPDiscoveryPageDomainResultFactV1
	settlement    *mcpConnectSettlementReaderV1
	apply         *MCPDiscoveryPageApplyStoreV1
}

func newMCPDiscoveryPageApplyFixtureV1(t *testing.T) *mcpDiscoveryPageApplyFixtureV1 {
	t.Helper()
	df := newMCPDiscoveryPageDomainFixtureV1(t)
	domain, err := df.store.CreateMCPDiscoveryPageDomainResultV1(context.Background(), df.request)
	if err != nil {
		t.Fatal(err)
	}
	bundle, current := mcpDiscoveryPageSettlementFixtureV1(t, df.now, df, domain)
	settlement := &mcpConnectSettlementReaderV1{current: current, bundle: bundle}
	f := &mcpDiscoveryPageApplyFixtureV1{now: df.now, domainFixture: df, domain: domain, settlement: settlement}
	apply, err := NewMCPDiscoveryPageApplyStoreV1(df.store, df.store.physical, settlement, domain.PreparedAttempt.Provider, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	f.apply = apply
	return f
}

func mcpDiscoveryPageSettlementFixtureV1(t *testing.T, now time.Time, f *mcpDiscoveryPageDomainFixtureV1, domain toolcontract.MCPDiscoveryPageDomainResultFactV1) (runtimeports.OperationSettlementCommitBundleV4, runtimeports.OperationInspectionSettlementRefV4) {
	t.Helper()
	runtimeDomain := mcpDiscoveryPageRuntimeDomainResultRefV1(domain.PreparedAttempt.Provider, domain)
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
	submission, err := runtimeports.SealOperationSettlementSubmissionV4(runtimeports.OperationSettlementSubmissionV4{ID: "mcp-discovery-page-runtime-settlement", TenantID: domain.TenantID, Operation: domain.Operation, OperationDigest: domain.Attempt.OperationDigest, OperationScopeDigest: scopeSet, EffectID: domain.Attempt.EffectID, ExpectedEffectRevision: 1, Owner: domain.Owner, DomainResult: runtimeDomain, Evidence: []runtimeports.OperationSettlementEvidenceBindingV4{prepare, execute}, IdempotencyKey: "mcp-discovery-page-runtime-settlement", ConflictDomain: testDigestV1("mcp-discovery-page-settlement-conflict"), SettledUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := runtimeports.SealOperationSettlementFactV4(runtimeports.OperationSettlementFactV4{Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	ref := fact.RefV4()
	association, err := runtimeports.SealOperationSettlementEvidenceAssociationV4(runtimeports.OperationSettlementEvidenceAssociationV4{ID: "mcp-discovery-page-runtime-association", Settlement: ref, Prepare: prepare, Execute: execute})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := runtimeports.SealOperationSettlementTerminalGuardV4(runtimeports.OperationSettlementTerminalGuardV4{ID: "mcp-discovery-page-runtime-guard", TenantID: domain.TenantID, OperationDigest: domain.Attempt.OperationDigest, EffectID: domain.Attempt.EffectID, Settlement: ref})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := runtimeports.SealOperationSettlementTerminalProjectionV4(runtimeports.OperationSettlementTerminalProjectionV4{ID: "mcp-discovery-page-runtime-projection", TenantID: domain.TenantID, OperationDigest: domain.Attempt.OperationDigest, EffectID: domain.Attempt.EffectID, Settlement: ref, Association: association.RefV4(), Guard: guard.RefV4(), DomainResult: runtimeDomain})
	if err != nil {
		t.Fatal(err)
	}
	bundle := runtimeports.OperationSettlementCommitBundleV4{Settlement: fact, Association: association, Guard: guard, Projection: projection}
	if err = bundle.Validate(); err != nil {
		t.Fatal(err)
	}
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{Settlement: ref, Association: association.RefV4(), Guard: guard.RefV4(), Projection: projection.RefV4(), DomainResult: runtimeDomain, EffectFactRevision: 2, Owner: domain.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	return bundle, inspection
}
