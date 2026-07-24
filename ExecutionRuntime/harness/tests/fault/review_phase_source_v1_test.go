package fault_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewPhaseSourceV1FaultReadRecoveryAndClosedNotFound(t *testing.T) {
	now := time.Unix(1_761_000_000, 0)
	session := faultReviewTerminalSessionV1(t, now)
	request := faultReviewRunRequestV1(t, session)

	t.Run("unavailable same-exact detached inspect", func(t *testing.T) {
		readerPort := &faultReviewSessionReaderV1{session: session, first: core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "read reply unavailable")}
		reader, err := kernel.NewReviewPhaseSourceCurrentReaderV1(faultReviewActionReaderV1{}, readerPort, faultReviewClockV1(now, now.Add(time.Second), now.Add(2*time.Second)))
		if err != nil {
			t.Fatal(err)
		}
		projection, err := reader.InspectReviewPhaseSourceCurrentV1(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if readerPort.calls != 3 || !readerPort.recovered || !reflect.DeepEqual(projection.Source, request.Source) {
			t.Fatalf("fault recovery calls=%d detached=%v", readerPort.calls, readerPort.recovered)
		}
	})

	t.Run("not found is closed and not retried", func(t *testing.T) {
		readerPort := &faultReviewSessionReaderV1{session: session, always: core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "session absent")}
		reader, _ := kernel.NewReviewPhaseSourceCurrentReaderV1(faultReviewActionReaderV1{}, readerPort, func() time.Time { return now })
		projection, err := reader.InspectReviewPhaseSourceCurrentV1(context.Background(), request)
		if !core.HasCategory(err, core.ErrorNotFound) || readerPort.calls != 1 || projection != (contract.ReviewPhaseSourceCurrentProjectionV1{}) {
			t.Fatalf("closed NotFound error=%v calls=%d projection=%#v", err, readerPort.calls, projection)
		}
	})
}

type faultReviewActionReaderV1 struct{}

func (faultReviewActionReaderV1) InspectCommittedPendingActionCurrentV3(context.Context, contract.CommittedPendingActionCurrentRequestV3) (contract.CommittedPendingActionCurrentV3, error) {
	return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "action absent")
}

type faultReviewSessionReaderV1 struct {
	session   contract.GovernedSessionV4
	first     error
	always    error
	calls     int
	recovered bool
}

func (r *faultReviewSessionReaderV1) InspectSessionV4(ctx context.Context, run contract.RunRef, id string) (contract.GovernedSessionV4, error) {
	r.calls++
	if r.always != nil {
		return contract.GovernedSessionV4{}, r.always
	}
	if r.calls == 1 && r.first != nil {
		return contract.GovernedSessionV4{}, r.first
	}
	if ctx.Err() == nil && r.calls == 2 {
		r.recovered = true
	}
	if run.RunID != r.session.Run.RunID || id != r.session.ID {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Inspect changed exact key")
	}
	return r.session.Clone(), nil
}

func faultReviewTerminalSessionV1(t testing.TB, now time.Time) contract.GovernedSessionV4 {
	t.Helper()
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-fault-review", ID: "agent-fault-review", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-fault-review", PlanDigest: core.DigestBytes([]byte("plan-fault-review"))}, Instance: core.InstanceRef{ID: "instance-fault-review", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-fault-review", Epoch: 1}, AuthorityEpoch: 1}
	binding := runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-fault-review", BindingSetRevision: 1, ComponentID: "praxis.harness/test", ManifestDigest: core.DigestBytes([]byte("manifest-fault-review")), ArtifactDigest: core.DigestBytes([]byte("artifact-fault-review")), Capability: "praxis.harness/session"}
	endpoint, err := contract.NewEndpointRefV2("endpoint-fault-review", scope, binding)
	if err != nil {
		t.Fatal(err)
	}
	session, err := contract.SealGovernedSessionV4(contract.GovernedSessionV4{ID: "session-fault-review", Revision: 1, Run: contract.RunRef{Scope: scope, RunID: "run-fault-review"}, Endpoint: endpoint, Phase: contract.SessionTerminalV2, CompletionClaim: contract.ClaimCancelled, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func faultReviewRunRequestV1(t testing.TB, session contract.GovernedSessionV4) contract.ReviewPhaseSourceCurrentRequestV1 {
	t.Helper()
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(session.Run.Scope)
	run := contract.ReviewRunPhaseSourceRefV1{Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, CompletionClaim: session.CompletionClaim}
	ref, err := contract.SealReviewPhaseSourceRefV1(contract.ReviewPhaseSourceRefV1{Kind: contract.ReviewPhaseRunSourceV1, Run: &run})
	if err != nil {
		t.Fatal(err)
	}
	return contract.ReviewPhaseSourceCurrentRequestV1{Source: ref}
}

func faultReviewClockV1(values ...time.Time) func() time.Time {
	index := 0
	return func() time.Time {
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}
