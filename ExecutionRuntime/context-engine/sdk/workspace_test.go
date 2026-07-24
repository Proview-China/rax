package sdk

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func TestOfflineWorkspaceLifecycleAndSealOwnershipV1(t *testing.T) {
	limits := testLimitsV1(OfflineCompileFrameV1)
	input, err := NewOfflineContentBundleV1([]OfflineContentItemV1{itemV1([]byte("input"))}, limits)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := newOfflineWorkspaceV1(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Destroy(); err != nil || workspace.state != workspaceDestroyedV1 {
		t.Fatalf("Destroy(new) failed: %v", err)
	}
	if err := workspace.Destroy(); err != nil {
		t.Fatal("Destroy must be idempotent")
	}

	workspace, err = newOfflineWorkspaceV1(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	work := compileWorkLimitsV1(limits)
	if err := workspace.Begin(context.Background(), work); err != nil {
		t.Fatal(err)
	}
	ref, err := workspace.PutContextV1(context.Background(), []byte("generated"))
	if err != nil {
		t.Fatal(err)
	}
	seal, err := workspace.Seal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wrong := seal
	wrong.id = contract.DigestBytes([]byte("wrong"))
	if _, err := workspace.Export(context.Background(), wrong, limits); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("foreign seal accepted: %v", err)
	}
	bundle, err := workspace.Export(context.Background(), seal, limits)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := bundle.Lookup(ref); !ok || string(value) != "generated" {
		t.Fatal("sealed export missing generated content")
	}
	if err := workspace.Abort(); err != nil || workspace.state != workspaceExportedV1 {
		t.Fatal("Abort(exported) must be harmless")
	}
	if err := workspace.Destroy(); err != nil || workspace.state != workspaceDestroyedV1 {
		t.Fatal("exported workspace did not destroy")
	}
}

func TestOfflineWorkspaceCancellationFaultMatrixV1(t *testing.T) {
	limits := testLimitsV1(OfflineCompileFrameV1)
	input, err := NewOfflineContentBundleV1([]OfflineContentItemV1{itemV1([]byte("input"))}, limits)
	if err != nil {
		t.Fatal(err)
	}
	work := compileWorkLimitsV1(limits)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	workspace, err := newOfflineWorkspaceV1(canceled, input)
	if !errors.Is(err, context.Canceled) || workspace != nil {
		t.Fatalf("canceled constructor returned workspace: %#v %v", workspace, err)
	}
	workspace, err = newOfflineWorkspaceV1(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Begin(canceled, work); !errors.Is(err, context.Canceled) || workspace.state != workspaceNewV1 {
		t.Fatalf("canceled Begin changed state: %v %v", workspace.state, err)
	}
	_ = workspace.Destroy()

	workspace, err = newOfflineWorkspaceV1(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Begin(context.Background(), work); err != nil {
		t.Fatal(err)
	}
	putCtx := &countdownContextV1{Context: context.Background(), remaining: 2}
	if ref, err := workspace.PutContextV1(putCtx, bytes.Repeat([]byte("x"), 256*1024)); !errors.Is(err, context.Canceled) || ref.Validate() == nil || len(workspace.staged) != 0 {
		t.Fatalf("canceled Put exposed partial state: %#v %d %v", ref, len(workspace.staged), err)
	}
	if _, err := workspace.Seal(canceled); !errors.Is(err, context.Canceled) || workspace.state != workspaceOpenV1 {
		t.Fatalf("canceled Seal changed state: %v %v", workspace.state, err)
	}
	ref, err := workspace.PutContextV1(context.Background(), []byte("generated"))
	if err != nil || ref.Validate() != nil {
		t.Fatal(err)
	}
	seal, err := workspace.Seal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	deadline, deadlineCancel := context.WithDeadline(context.Background(), time.Unix(0, 1))
	defer deadlineCancel()
	if bundle, err := workspace.Export(deadline, seal, limits); !errors.Is(err, context.DeadlineExceeded) || bundle.ContentSetDigest() != "" || workspace.state != workspaceSealedV1 {
		t.Fatalf("deadline Export changed state or returned partial bundle: %v %#v %v", workspace.state, bundle, err)
	}
	_ = workspace.Abort()
	if len(workspace.staged) != 0 || workspace.state != workspaceAbortedV1 {
		t.Fatalf("Abort did not make partial refs unreachable: %v %d", workspace.state, len(workspace.staged))
	}
	_ = workspace.Destroy()
}

func TestOfflineWorkspaceAbortMakesPartialRefsUnreachableV1(t *testing.T) {
	limits := testLimitsV1(OfflineCompileFrameV1)
	input, _ := NewOfflineContentBundleV1([]OfflineContentItemV1{}, limits)
	workspace, err := newOfflineWorkspaceV1(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Begin(context.Background(), compileWorkLimitsV1(limits)); err != nil {
		t.Fatal(err)
	}
	ref, err := workspace.PutContextV1(context.Background(), []byte("partial"))
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Abort(); err != nil || workspace.state != workspaceAbortedV1 {
		t.Fatal("abort failed")
	}
	if _, err := workspace.GetContextV1(context.Background(), ref); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("partial ref remained reachable: %v", err)
	}
	if err := workspace.Destroy(); err != nil || workspace.state != workspaceDestroyedV1 {
		t.Fatal("aborted workspace did not destroy")
	}
}
