package fakes_test

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationSettlementCurrentReaderV5PublicConformanceUsesGatewayOnly(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "current-reader-conformance")
	if _, err := fixture.gateway.SettleCheckpointPhaseV5(context.Background(), fixture.submission); err != nil {
		t.Fatal(err)
	}
	provider, err := kernel.NewOperationSettlementCurrentReaderFacadeV5(&fixture.gateway)
	if err != nil {
		t.Fatal(err)
	}
	var wired ports.OperationSettlementCurrentReaderProviderV5 = provider
	report, err := conformance.CheckOperationSettlementCurrentReaderV5(context.Background(), conformance.OperationSettlementCurrentReaderCaseV5{
		Provider: wired,
		Request: ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{
			Operation: fixture.submission.Operation,
			EffectID:  fixture.submission.EffectID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CurrentInspectObserved || report.SettleAuthorityUsed || report.ProductionClaimEligible {
		t.Fatalf("current Reader conformance widened authority: %+v", report)
	}
}

func TestOperationSettlementCurrentReaderV5RejectsTypedNil(t *testing.T) {
	var provider *kernel.OperationSettlementCurrentReaderFacadeV5
	_, err := conformance.CheckOperationSettlementCurrentReaderV5(context.Background(), conformance.OperationSettlementCurrentReaderCaseV5{
		Provider: provider,
		Request: ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{
			Operation: ports.OperationSubjectV3{},
			EffectID:  "effect-current-reader-v5",
		},
	})
	if !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil current Reader did not fail closed before request use: %v", err)
	}
}

func TestOperationSettlementCurrentReaderV5WiringRejectsRawFactPort(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "current-reader-wiring")
	providerType := reflect.TypeOf((*ports.OperationSettlementCurrentReaderProviderV5)(nil)).Elem()
	raw := fixture.effect.effect.effect.store
	if reflect.TypeOf(raw).Implements(providerType) {
		t.Fatal("raw Operation Effect/Settlement Fact Store satisfies Gateway-backed wiring provider")
	}
	provider, err := kernel.NewOperationSettlementCurrentReaderFacadeV5(&fixture.gateway)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.TypeOf(provider).Implements(providerType) {
		t.Fatal("Kernel Gateway facade does not satisfy the wiring provider")
	}
	if _, err := kernel.NewOperationSettlementCurrentReaderFacadeV5(nil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("nil Kernel Gateway did not fail closed: %v", err)
	}
	missing := kernel.OperationCheckpointRestoreSettlementGatewayV5{}
	if _, err := kernel.NewOperationSettlementCurrentReaderFacadeV5(&missing); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("Gateway without Fact Owner produced a provider: %v", err)
	}
}

func TestOperationSettlementCurrentReaderV5RejectsMaliciousBackendWithoutLeak(t *testing.T) {
	requested := newCheckpointSettlementFixtureV5(t, "current-reader-requested")
	tenantOperation := newOperationSettlementFixtureFromEnforcementV4(t,
		newOperationEnforcementFixtureForScopeV4(t, "settlement-checkpoint-v5-current-reader-returned", "", "tenant-current-reader-returned", "praxis.sandbox/allocate"),
		"checkpoint-v5-current-reader-returned",
	)
	tenantReturned := newCheckpointSettlementFixtureFromOperationV5(t, tenantOperation, "current-reader-returned")
	if _, err := requested.gateway.SettleCheckpointPhaseV5(context.Background(), requested.submission); err != nil {
		t.Fatal(err)
	}
	if _, err := tenantReturned.gateway.SettleCheckpointPhaseV5(context.Background(), tenantReturned.submission); err != nil {
		t.Fatal(err)
	}
	tenantForeign, err := tenantReturned.gateway.InspectCheckpointPhaseSettlementCurrentV5(context.Background(), ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{
		Operation: tenantReturned.submission.Operation,
		EffectID:  tenantReturned.submission.EffectID,
	})
	if err != nil {
		t.Fatal(err)
	}
	scopeForeign := operationSettlementCurrentInspectionWithOperationDriftV5(t, requested, func(operation *ports.OperationSubjectV3) {
		operation.ExecutionScope.Lineage.ID = "lineage-current-reader-drifted"
		operation.ExecutionScope.Lineage.PlanDigest = core.DigestBytes([]byte("lineage-current-reader-drifted"))
	})
	nestedForeign := operationSettlementCurrentInspectionWithOperationDriftV5(t, requested, func(operation *ports.OperationSubjectV3) {
		operation.CurrentProjectionRef = "operation-current-reader-drifted"
		operation.CurrentProjectionRevision++
		operation.CurrentProjectionDigest = core.DigestBytes([]byte("operation-current-reader-drifted"))
	})

	cases := []struct {
		name       string
		inspection ports.OperationCheckpointRestoreSettlementInspectionV5
	}{
		{name: "same-operation-id-wrong-tenant", inspection: tenantForeign},
		{name: "same-operation-id-wrong-scope", inspection: scopeForeign},
		{name: "same-operation-id-wrong-nested-current-ref", inspection: nestedForeign},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			backend := &operationSettlementCurrentMaliciousFactPortV5{
				OperationCheckpointRestoreSettlementFactPortV5: requested.effect.effect.effect.store,
				inspection: testCase.inspection,
			}
			gateway := kernel.OperationCheckpointRestoreSettlementGatewayV5{Facts: backend}
			got, err := gateway.InspectCheckpointPhaseSettlementCurrentV5(context.Background(), ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{Operation: requested.submission.Operation, EffectID: requested.submission.EffectID})
			if !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("malicious %s closure did not conflict: %v", testCase.name, err)
			}
			assertOperationSettlementCurrentZeroV5(t, got)
			if backend.currentReads.Load() != 1 {
				t.Fatalf("malicious %s test performed %d Fact reads", testCase.name, backend.currentReads.Load())
			}
			if backend.commitCalls.Load() != 0 || backend.providerCalls.Load() != 0 || backend.applyCalls.Load() != 0 {
				t.Fatalf("malicious %s read crossed a side-effect boundary: commit=%d provider=%d apply=%d", testCase.name, backend.commitCalls.Load(), backend.providerCalls.Load(), backend.applyCalls.Load())
			}
			if requested.effect.effect.effect.store.CheckpointSettlementV5CommitCount() != 1 || tenantReturned.effect.effect.effect.store.CheckpointSettlementV5CommitCount() != 1 {
				t.Fatalf("malicious %s read changed terminal state", testCase.name)
			}
		})
	}
}

func TestOperationSettlementCurrentReaderV5ValidatesMalformedBeforeRequestDrift(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "current-reader-malformed")
	malformed := operationSettlementCurrentInspectionWithOperationDriftV5(t, fixture, func(operation *ports.OperationSubjectV3) {
		operation.CurrentProjectionRef = "operation-current-reader-malformed-foreign"
		operation.CurrentProjectionDigest = core.DigestBytes([]byte("operation-current-reader-malformed-foreign"))
	})
	malformed.Current = false
	backend := &operationSettlementCurrentMaliciousFactPortV5{OperationCheckpointRestoreSettlementFactPortV5: fixture.effect.effect.effect.store, inspection: malformed}
	gateway := kernel.OperationCheckpointRestoreSettlementGatewayV5{Facts: backend}
	got, err := gateway.InspectCheckpointPhaseSettlementCurrentV5(context.Background(), ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{Operation: fixture.submission.Operation, EffectID: fixture.submission.EffectID})
	if !core.HasCategory(err, core.ErrorInvalidArgument) || core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("malformed Inspection was not rejected before request drift: %v", err)
	}
	assertOperationSettlementCurrentZeroV5(t, got)
	if backend.currentReads.Load() != 1 || backend.commitCalls.Load() != 0 || backend.providerCalls.Load() != 0 || backend.applyCalls.Load() != 0 {
		t.Fatalf("malformed Inspection crossed a side-effect boundary: reads=%d commit=%d provider=%d apply=%d", backend.currentReads.Load(), backend.commitCalls.Load(), backend.providerCalls.Load(), backend.applyCalls.Load())
	}
}

func TestOperationSettlementCurrentReaderV5PassesBackendUnknownErrorsUnchanged(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "current-reader-errors")
	for _, testCase := range []struct {
		name string
		err  error
	}{
		{name: "unavailable", err: core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "reader unavailable sentinel")},
		{name: "indeterminate", err: core.NewError(core.ErrorIndeterminate, core.ReasonEffectSettlementMissing, "reader indeterminate sentinel")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			backend := &operationSettlementCurrentMaliciousFactPortV5{OperationCheckpointRestoreSettlementFactPortV5: fixture.effect.effect.effect.store, err: testCase.err}
			gateway := kernel.OperationCheckpointRestoreSettlementGatewayV5{Facts: backend}
			got, err := gateway.InspectCheckpointPhaseSettlementCurrentV5(context.Background(), ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{Operation: fixture.submission.Operation, EffectID: fixture.submission.EffectID})
			if err != testCase.err {
				t.Fatalf("backend %s error was translated: got=%v want=%v", testCase.name, err, testCase.err)
			}
			assertOperationSettlementCurrentZeroV5(t, got)
			if backend.currentReads.Load() != 1 || backend.commitCalls.Load() != 0 || backend.providerCalls.Load() != 0 || backend.applyCalls.Load() != 0 {
				t.Fatalf("backend %s error crossed a side-effect boundary", testCase.name)
			}
		})
	}
}

type operationSettlementCurrentMaliciousFactPortV5 struct {
	ports.OperationCheckpointRestoreSettlementFactPortV5
	inspection    ports.OperationCheckpointRestoreSettlementInspectionV5
	err           error
	currentReads  atomic.Int64
	commitCalls   atomic.Int64
	providerCalls atomic.Int64
	applyCalls    atomic.Int64
}

func (p *operationSettlementCurrentMaliciousFactPortV5) InspectCheckpointPhaseSettlementCurrentV5(context.Context, ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5) (ports.OperationCheckpointRestoreSettlementInspectionV5, error) {
	p.currentReads.Add(1)
	return p.inspection, p.err
}

func (p *operationSettlementCurrentMaliciousFactPortV5) CommitCheckpointPhaseSettlementV5(ctx context.Context, bundle ports.OperationCheckpointRestoreSettlementCommitBundleV5) (ports.OperationCheckpointRestoreSettlementCommitBundleV5, error) {
	p.commitCalls.Add(1)
	return p.OperationCheckpointRestoreSettlementFactPortV5.CommitCheckpointPhaseSettlementV5(ctx, bundle)
}

func operationSettlementCurrentInspectionWithOperationDriftV5(t *testing.T, fixture checkpointSettlementFixtureV5, mutate func(*ports.OperationSubjectV3)) ports.OperationCheckpointRestoreSettlementInspectionV5 {
	t.Helper()
	submission := fixture.submission
	mutate(&submission.Operation)
	scopeDigest, err := ports.ExecutionScopeDigestV2(submission.Operation.ExecutionScope)
	if err != nil {
		t.Fatal(err)
	}
	submission.Operation.ExecutionScopeDigest = scopeDigest
	operationDigest, err := submission.Operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	submission.OperationDigest = operationDigest
	submission.DomainResult.Operation = submission.Operation
	submission.DomainResult.OperationDigest = operationDigest
	submission.DispatchAttempt.OperationDigest = operationDigest
	submission.Enforcement.OperationDigest = operationDigest
	submission.Handoff.Attempt = submission.DispatchAttempt
	submission.Handoff.Digest, err = submission.Handoff.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	submission.Evidence.Handoff = submission.Handoff
	submission.Evidence.Digest, err = submission.Evidence.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := control.BuildOperationCheckpointRestoreSettlementBundleV5(submission)
	if err != nil {
		t.Fatal(err)
	}
	inspection := ports.OperationCheckpointRestoreSettlementInspectionV5{Bundle: bundle, Current: true, CheckedUnixNano: fixture.now.UnixNano()}
	if err := inspection.Validate(); err != nil {
		t.Fatal(err)
	}
	return inspection
}

func assertOperationSettlementCurrentZeroV5(t *testing.T, got ports.OperationCheckpointRestoreSettlementInspectionV5) {
	t.Helper()
	if !reflect.DeepEqual(got, ports.OperationCheckpointRestoreSettlementInspectionV5{}) {
		t.Fatalf("current Reader leaked a non-zero Inspection: %#v", got)
	}
}
