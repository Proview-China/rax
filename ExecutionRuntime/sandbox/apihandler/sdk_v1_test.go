package apihandler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/api"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/sdk"
)

type sdkHandlerFakeV1 struct {
	*sdk.Client
	describeCalls      int
	rewindComposeCalls int
	rewindInspectCalls int
}

func (f *sdkHandlerFakeV1) ComposeWorkspaceRewindV1(context.Context, contract.ComposeWorkspaceRewindRequestV1) (ports.WorkspaceRewindCompositionResultV1, error) {
	f.rewindComposeCalls++
	return ports.WorkspaceRewindCompositionResultV1{}, nil
}

func (f *sdkHandlerFakeV1) InspectWorkspaceRewindV1(context.Context, contract.ComposeWorkspaceRewindRequestV1) (ports.WorkspaceRewindCompositionResultV1, error) {
	f.rewindInspectCalls++
	return ports.WorkspaceRewindCompositionResultV1{}, nil
}

func (f *sdkHandlerFakeV1) DescribeBackends() ([]contract.BackendDescriptor, error) {
	f.describeCalls++
	return []contract.BackendDescriptor{testkit.Backend()}, nil
}

func TestSDKHandlerV1ReadOnlyExecutionAndRecoveryDoesNotReplay(t *testing.T) {
	fake := &sdkHandlerFakeV1{}
	handler, err := NewSDKHandlerV1(fake)
	if err != nil {
		t.Fatal(err)
	}
	request := apiRequestForHandlerV1(t, api.ActionDescribeBackendsV1, `{}`)
	outcome, err := handler.Execute(context.Background(), request)
	if err != nil || outcome.State != api.OperationSucceededV1 || fake.describeCalls != 1 || outcome.Result == nil {
		t.Fatalf("outcome=%#v err=%v calls=%d", outcome, err, fake.describeCalls)
	}
	if _, err := handler.Reconcile(context.Background(), request); err == nil || fake.describeCalls != 1 {
		t.Fatalf("read-only recovery replayed execution: err=%v calls=%d", err, fake.describeCalls)
	}
}

func TestSDKHandlerV1InvalidPayloadIsConfirmedFailureAndTypedNilRejected(t *testing.T) {
	fake := &sdkHandlerFakeV1{}
	handler, err := NewSDKHandlerV1(fake)
	if err != nil {
		t.Fatal(err)
	}
	request := apiRequestForHandlerV1(t, api.ActionDescribeBackendsV1, `{"unexpected":true}`)
	outcome, err := handler.Execute(context.Background(), request)
	if err != nil || outcome.State != api.OperationFailedV1 || fake.describeCalls != 0 {
		t.Fatalf("invalid payload outcome=%#v err=%v calls=%d", outcome, err, fake.describeCalls)
	}
	var typedNil *sdkHandlerFakeV1
	if _, err := NewSDKHandlerV1(typedNil); err == nil {
		t.Fatal("typed-nil SDK was accepted")
	}
}

func TestSDKHandlerV1WorkspaceRewindRecoveryInspectsWithoutRecomposing(t *testing.T) {
	fake := &sdkHandlerFakeV1{}
	handler, err := NewSDKHandlerV1(fake)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_100_000_000, 0)
	rewind, err := contract.SealComposeWorkspaceRewindRequestV1(contract.ComposeWorkspaceRewindRequestV1{
		RequestID: "rewind-request", IdempotencyKey: "rewind-key", PlannedChangeSetID: "rewind-set",
		SourceWorkspaceViewRef: testkit.Ref("view"), ExpectedBaseRevision: testkit.Ref("base").Digest,
		ExpectedFileScopeDigest: testkit.Ref("scope").Digest, KeepChangeSetRefs: []contract.Ref{testkit.Ref("keep")},
		RequestedNotAfter: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(rewind)
	if err != nil {
		t.Fatal(err)
	}
	request := apiRequestForHandlerV1(t, api.ActionWorkspaceRewindV1, string(payload))
	outcome, err := handler.Execute(context.Background(), request)
	if err != nil || outcome.State != api.OperationSucceededV1 || fake.rewindComposeCalls != 1 {
		t.Fatalf("rewind execute outcome=%+v err=%v compose=%d", outcome, err, fake.rewindComposeCalls)
	}
	if _, err := handler.Reconcile(context.Background(), request); err != nil || fake.rewindComposeCalls != 1 || fake.rewindInspectCalls != 1 {
		t.Fatalf("rewind recovery err=%v compose=%d inspect=%d", err, fake.rewindComposeCalls, fake.rewindInspectCalls)
	}
}

func apiRequestForHandlerV1(t *testing.T, action api.ActionV1, payload string) api.OperationRequestV1 {
	t.Helper()
	now := time.Unix(2_100_000_000, 0)
	request, err := api.SealOperationRequestV1(api.OperationRequestV1{RequestID: "handler-request", IdempotencyKey: "handler-key", TenantID: "tenant-1", Action: action, PayloadSchema: "praxis.sandbox.api/test/v1", PayloadRevision: 1, Payload: []byte(payload), RequestedUnixNano: now.UnixNano(), RequestedNotAfterUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return request
}
