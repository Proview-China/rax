package kernel_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestModelProviderActualPointGuardV1FailsClosedBeforeReadsWhenDependencyMissing(t *testing.T) {
	request := kernelModelRequestV1(t, time.Unix(1_100_000, 0))
	var boundaryCalls atomic.Int64
	boundary := boundaryReaderV1{calls: &boundaryCalls}
	gateway := kernel.ModelProviderActualPointGuardGatewayV1{ModelBoundary: boundary, Clock: func() time.Time { return time.Unix(1_100_000, 0) }}
	if _, err := gateway.InspectCurrentModelProviderActualPointV1(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("missing dependencies: %v", err)
	}
	if boundaryCalls.Load() != 0 {
		t.Fatal("dependency preflight touched a backend")
	}
}

func TestModelProviderActualPointGuardV1PreservesBoundaryUnavailableAndStops(t *testing.T) {
	request := kernelModelRequestV1(t, time.Unix(1_100_100, 0))
	var boundaryCalls atomic.Int64
	var runCalls atomic.Int64
	gateway := kernel.ModelProviderActualPointGuardGatewayV1{
		ModelBoundary: boundaryReaderV1{calls: &boundaryCalls, err: core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "boundary unavailable")},
		Runs:          runReaderV1{calls: &runCalls},
		Control:       controlReaderV1{},
		Effects:       effectReaderV1{},
		Dispatch:      dispatchReaderV1{},
		Clock:         func() time.Time { return time.Unix(1_100_100, 0) },
	}
	if _, err := gateway.InspectCurrentModelProviderActualPointV1(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("boundary unavailable: %v", err)
	}
	if boundaryCalls.Load() != 1 || runCalls.Load() != 0 {
		t.Fatalf("unexpected read order: boundary=%d run=%d", boundaryCalls.Load(), runCalls.Load())
	}
}

func TestModelProviderActualPointGuardV1PreservesBoundaryIndeterminateAndStops(t *testing.T) {
	request := kernelModelRequestV1(t, time.Unix(1_100_150, 0))
	var boundaryCalls atomic.Int64
	var runCalls atomic.Int64
	gateway := kernel.ModelProviderActualPointGuardGatewayV1{
		ModelBoundary: boundaryReaderV1{calls: &boundaryCalls, err: core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "boundary current is unknown")},
		Runs:          runReaderV1{calls: &runCalls},
		Control:       controlReaderV1{},
		Effects:       effectReaderV1{},
		Dispatch:      dispatchReaderV1{},
		Clock:         func() time.Time { return time.Unix(1_100_150, 0) },
	}
	if _, err := gateway.InspectCurrentModelProviderActualPointV1(context.Background(), request); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("boundary indeterminate: %v", err)
	}
	if boundaryCalls.Load() != 1 || runCalls.Load() != 0 {
		t.Fatalf("indeterminate boundary did not fail closed: boundary=%d run=%d", boundaryCalls.Load(), runCalls.Load())
	}
}

func TestModelProviderActualPointGuardV1TypedNilDependencyFailsBeforeReads(t *testing.T) {
	request := kernelModelRequestV1(t, time.Unix(1_100_200, 0))
	var typedNil *boundaryReaderV1
	gateway := kernel.ModelProviderActualPointGuardGatewayV1{
		ModelBoundary: typedNil,
		Runs:          runReaderV1{},
		Control:       controlReaderV1{},
		Effects:       effectReaderV1{},
		Dispatch:      dispatchReaderV1{},
		Clock:         func() time.Time { return time.Unix(1_100_200, 0) },
	}
	if _, err := gateway.InspectCurrentModelProviderActualPointV1(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed nil dependency: %v", err)
	}
}

func TestModelProviderActualPointGuardV1CancelledContextFailsBeforeReads(t *testing.T) {
	request := kernelModelRequestV1(t, time.Unix(1_100_300, 0))
	var boundaryCalls atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gateway := kernel.ModelProviderActualPointGuardGatewayV1{
		ModelBoundary: boundaryReaderV1{calls: &boundaryCalls},
		Runs:          runReaderV1{},
		Control:       controlReaderV1{},
		Effects:       effectReaderV1{},
		Dispatch:      dispatchReaderV1{},
		Clock:         func() time.Time { return time.Unix(1_100_300, 0) },
	}
	projection, err := gateway.InspectCurrentModelProviderActualPointV1(ctx, request)
	if err != context.Canceled {
		t.Fatalf("cancelled context: %v", err)
	}
	if boundaryCalls.Load() != 0 || projection != (ports.ModelProviderActualPointCurrentProjectionV1{}) {
		t.Fatal("cancelled context read a backend or returned a projection")
	}
}

func TestModelProviderActualPointGuardV1ConcurrentCancelledCallsStayReadOnly(t *testing.T) {
	request := kernelModelRequestV1(t, time.Unix(1_100_400, 0))
	var boundaryCalls atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gateway := kernel.ModelProviderActualPointGuardGatewayV1{
		ModelBoundary: boundaryReaderV1{calls: &boundaryCalls},
		Runs:          runReaderV1{},
		Control:       controlReaderV1{},
		Effects:       effectReaderV1{},
		Dispatch:      dispatchReaderV1{},
		Clock:         func() time.Time { return time.Unix(1_100_400, 0) },
	}
	var group sync.WaitGroup
	var unexpected atomic.Int64
	for range 64 {
		group.Add(1)
		go func() {
			defer group.Done()
			projection, err := gateway.InspectCurrentModelProviderActualPointV1(ctx, request)
			if err != context.Canceled || projection != (ports.ModelProviderActualPointCurrentProjectionV1{}) {
				unexpected.Add(1)
			}
		}()
	}
	group.Wait()
	if unexpected.Load() != 0 || boundaryCalls.Load() != 0 {
		t.Fatalf("concurrent cancelled guard calls touched current state: unexpected=%d boundary=%d", unexpected.Load(), boundaryCalls.Load())
	}
}

type boundaryReaderV1 struct {
	calls *atomic.Int64
	err   error
}

func (r boundaryReaderV1) InspectCurrentModelProviderBoundaryV1(context.Context, ports.ModelProviderBoundaryCurrentRefV1) (ports.ModelProviderBoundaryCurrentProjectionV1, error) {
	if r.calls != nil {
		r.calls.Add(1)
	}
	return ports.ModelProviderBoundaryCurrentProjectionV1{}, r.err
}

type runReaderV1 struct{ calls *atomic.Int64 }

func (r runReaderV1) InspectRunLifecycleV3(context.Context, core.ExecutionScope, core.AgentRunID) (ports.RunLifecycleEnvelopeV3, error) {
	if r.calls != nil {
		r.calls.Add(1)
	}
	return ports.RunLifecycleEnvelopeV3{}, nil
}
func (runReaderV1) BeginStopRunV3(context.Context, ports.BeginStopRunRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	panic("write method must not be called")
}
func (runReaderV1) StopAndSettleRunV3(context.Context, ports.BeginStopRunRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	panic("write method must not be called")
}
func (runReaderV1) ReconcileRunTerminationV3(context.Context, ports.RunTerminationRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	panic("write method must not be called")
}
func (runReaderV1) InspectRunTerminationV3(context.Context, ports.RunTerminationRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	panic("unexpected termination read")
}

type controlReaderV1 struct{}

func (controlReaderV1) InspectModelDispatchControlCurrentV1(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (control.ModelDispatchControlCurrentProjectionV1, error) {
	return control.ModelDispatchControlCurrentProjectionV1{}, nil
}

type effectReaderV1 struct{}

func (effectReaderV1) CreateOperationEffectV3(context.Context, control.OperationEffectFactV3) (control.OperationEffectFactV3, error) {
	panic("write method must not be called")
}
func (effectReaderV1) InspectOperationEffectV3(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (control.OperationEffectFactV3, error) {
	return control.OperationEffectFactV3{}, nil
}
func (effectReaderV1) CompareAndSwapOperationEffectV3(context.Context, ports.OperationSubjectV3, control.OperationEffectCASRequestV3) (control.OperationEffectFactV3, error) {
	panic("write method must not be called")
}
func (effectReaderV1) IssueOperationDispatchPermitV3(context.Context, control.IssueOperationPermitRequestV3) (control.IssueOperationPermitResultV3, error) {
	panic("write method must not be called")
}
func (effectReaderV1) InspectOperationDispatchPermitV3(context.Context, ports.OperationSubjectV3, string) (control.OperationDispatchPermitFactV3, error) {
	panic("unexpected legacy read")
}
func (effectReaderV1) BeginOperationDispatchV3(context.Context, control.BeginOperationDispatchRequestV3) (control.OperationDispatchPermitFactV3, error) {
	panic("write method must not be called")
}
func (effectReaderV1) RecordOperationEnforcementV3(context.Context, control.RecordOperationEnforcementRequestV3) (control.OperationDispatchPermitFactV3, error) {
	panic("write method must not be called")
}

type dispatchReaderV1 struct{}

func (dispatchReaderV1) IssueOperationDispatchV4(context.Context, ports.IssueGovernedOperationDispatchRequestV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	panic("write method must not be called")
}
func (dispatchReaderV1) InspectOperationDispatchRecordV4(context.Context, ports.InspectOperationDispatchRecordRequestV4) (ports.OperationDispatchRecordV4, error) {
	panic("historical-only read must not replace current Inspect")
}
func (dispatchReaderV1) InspectCurrentOperationDispatchV4(context.Context, ports.InspectCurrentOperationDispatchRequestV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	return ports.CurrentOperationDispatchAuthorizationV4{}, nil
}
func (dispatchReaderV1) BeginOperationDispatchV4(context.Context, ports.BeginGovernedOperationDispatchRequestV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	panic("write method must not be called")
}

func kernelModelRequestV1(t *testing.T, now time.Time) ports.InspectCurrentModelProviderActualPointRequestV1 {
	t.Helper()
	lease := &core.SandboxLeaseRef{ID: "lease-kernel", Epoch: 3}
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-kernel", ID: "agent-kernel", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-kernel", PlanDigest: kernelDigestV1("plan")},
		Instance:       core.InstanceRef{ID: "instance-kernel", Epoch: 2},
		SandboxLease:   lease,
		AuthorityEpoch: 1,
	}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	operation := ports.OperationSubjectV3{
		Kind: ports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: "run-kernel",
		SubjectRevision: 1, CurrentProjectionRef: "run-current-kernel", CurrentProjectionRevision: 1, CurrentProjectionDigest: kernelDigestV1("run-current"),
	}
	operationDigest, _ := operation.DigestV3()
	attempt := ports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: "effect-kernel", IntentRevision: 1, IntentDigest: kernelDigestV1("intent"),
		PermitID: "permit-kernel", PermitRevision: 1, PermitDigest: kernelDigestV1("permit"), AttemptID: "attempt-kernel",
	}
	boundary, err := ports.SealModelProviderBoundaryCurrentRefV1(ports.ModelProviderBoundaryCurrentRefV1{
		Owner: core.OwnerRef{Domain: "praxis.model", ID: "model-invoker"}, ID: "boundary-kernel", Revision: 1,
		OperationDigest: operationDigest, EffectID: attempt.EffectID, RuntimeAttempt: attempt, DispatchSequence: 1, ProviderAttemptOrdinal: 1,
		AttemptRequestDigest: kernelDigestV1("request"), AcknowledgementDigest: kernelDigestV1("ack"), ExpiresUnixNano: now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ports.InspectCurrentModelProviderActualPointRequestV1{
		Operation: operation, EffectID: attempt.EffectID, ExpectedEffectRevision: 3, PermitID: attempt.PermitID, ExpectedPermitFactRevision: 2,
		PermitDigest: attempt.PermitDigest, AdmissionDigest: kernelDigestV1("admission"),
		ReviewAuthorization: ports.OperationReviewAuthorizationRefV4{ID: "review-kernel", Revision: 1, Digest: kernelDigestV1("review")},
		Attempt:             attempt, Verifier: kernelProviderBindingV1(), FenceDigest: kernelDigestV1("fence"), ModelBoundary: boundary, RequestedNotAfterUnixNano: now.Add(time.Second).UnixNano(),
	}
}

func kernelProviderBindingV1() ports.ProviderBindingRefV2 {
	return ports.ProviderBindingRefV2{
		BindingSetID: "binding-kernel", BindingSetRevision: 1, ComponentID: "praxis.model/provider",
		ManifestDigest: kernelDigestV1("manifest"), ArtifactDigest: kernelDigestV1("artifact"), Capability: ports.ModelInvokeCapabilityV1,
	}
}

func kernelDigestV1(value string) core.Digest {
	digest, _ := core.DigestJSON(value)
	return digest
}
