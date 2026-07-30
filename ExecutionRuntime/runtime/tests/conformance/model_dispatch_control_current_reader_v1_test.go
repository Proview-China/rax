package conformance_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestModelDispatchControlCurrentReaderV1PublicConformance(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 30, 20, 10, 0, 0, time.UTC)
	lease := &core.SandboxLeaseRef{ID: "lease-control-conformance", Epoch: 2}
	planDigest, _ := core.DigestJSON("plan-control-conformance")
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-control-conformance", ID: "agent-control-conformance", Epoch: 3},
		Lineage:  core.LineageRef{ID: "lineage-control-conformance", PlanDigest: planDigest},
		Instance: core.InstanceRef{ID: "instance-control-conformance", Epoch: 4}, SandboxLease: lease, AuthorityEpoch: 5,
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	currentDigest, _ := core.DigestJSON("run-current-control-conformance")
	operation := ports.OperationSubjectV3{
		Kind: ports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: "run-control-conformance",
		SubjectRevision: 1, CurrentProjectionRef: "run-current-control-conformance", CurrentProjectionRevision: 1,
		CurrentProjectionDigest: currentDigest,
	}
	run := core.AgentRunRecord{ID: operation.RunID, Scope: scope, Status: core.RunRunning, Revision: 1, StartedAt: now.Add(-time.Minute)}
	store := fakes.NewFactStore(func() time.Time { return now })
	if _, err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDesiredState(context.Background(), ports.DesiredStateSnapshotV2{Scope: scope, Desired: ports.DesiredRunningV2, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	reader, err := control.NewModelDispatchControlCurrentReaderV1(store, store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	report, err := conformance.CheckModelDispatchControlCurrentReaderV1(context.Background(), reader, conformance.ModelDispatchControlCurrentReaderFixtureV1{
		Operation: operation, EffectID: "effect-control-conformance", CheckedUnixNano: now.UnixNano(),
		ExpectedState: control.ModelDispatchControlDispatchableV1, ExpectedRunRevision: 1, ExpectedDesiredStateRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CurrentProjectionExact || !report.RepeatedReadExact || !report.ReadOnlyCapability || report.ProductionEligible {
		t.Fatalf("unexpected conformance report: %+v", report)
	}
}

func TestModelDispatchControlCurrentReaderV1ConformanceRejectsTypedNil(t *testing.T) {
	t.Parallel()
	var reader *typedNilModelDispatchControlReaderV1
	_, err := conformance.CheckModelDispatchControlCurrentReaderV1(context.Background(), reader, conformance.ModelDispatchControlCurrentReaderFixtureV1{})
	if !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil reader was accepted: %v", err)
	}
}

type typedNilModelDispatchControlReaderV1 struct{}

func (*typedNilModelDispatchControlReaderV1) InspectModelDispatchControlCurrentV1(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (control.ModelDispatchControlCurrentProjectionV1, error) {
	return control.ModelDispatchControlCurrentProjectionV1{}, nil
}
