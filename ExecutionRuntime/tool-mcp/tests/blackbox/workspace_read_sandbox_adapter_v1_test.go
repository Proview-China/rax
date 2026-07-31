package blackbox_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	sandboxcontract "github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/corepack"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestWorkspaceReadSandboxAdapterV1StartInspectAndLostReply(t *testing.T) {
	for _, test := range []struct {
		name       string
		executeErr error
	}{
		{name: "normal"},
		{name: "lost provider reply", executeErr: core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "reply lost")},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
			port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
			port.ExecuteErr = test.executeErr
			adapter := workspaceReadAdapterV1(t, port, fixture.Now.Add(time.Millisecond))
			output, err := adapter.StartOrInspectWorkspaceReadV1(context.Background(), workspaceReadRequestV1(t, fixture), fixture.Authorization)
			if err != nil {
				t.Fatal(err)
			}
			if output.Content == nil || *output.Content != "Praxis" || output.File.Path != fixture.Input.RelativePath ||
				output.BytesReturned != 6 || !output.Complete {
				t.Fatalf("unexpected workspace.read output: %#v", output)
			}
			if port.ExecuteCalls.Load() != 1 || port.BindingCalls.Load() != 1 || port.InspectCalls.Load() != 1 ||
				port.PhysicalReads.Load() != 1 || port.HistoricalCommandCalls.Load() != 0 {
				t.Fatalf("Start-or-Inspect calls execute=%d binding=%d inspect=%d physical=%d historical=%d",
					port.ExecuteCalls.Load(), port.BindingCalls.Load(), port.InspectCalls.Load(),
					port.PhysicalReads.Load(), port.HistoricalCommandCalls.Load())
			}
		})
	}
}

func TestWorkspaceReadSandboxAdapterV1LostReplyAndRestartExpiredInspectOnlyRecovery(t *testing.T) {
	fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
	port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
	port.Admission = runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}
	port.ExecuteErr = core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Admission reply lost")
	adapter := workspaceReadAdapterV1(t, port, fixture.Now.Add(time.Millisecond))
	output, err := adapter.StartOrInspectWorkspaceReadV1(
		context.Background(), workspaceReadRequestV1(t, fixture), fixture.Authorization,
	)
	if err != nil || output.Content == nil || *output.Content != "Praxis" {
		t.Fatalf("lost Admission reply did not recover by exact Runtime Attempt: output=%#v err=%v", output, err)
	}
	if port.ExecuteCalls.Load() != 1 || port.RuntimeInspectionCalls.Load() != 1 ||
		port.BindingCalls.Load() != 1 || port.InspectCalls.Load() != 1 || port.PhysicalReads.Load() != 1 {
		t.Fatalf("lost reply recovery drifted: execute=%d runtime=%d binding=%d inspect=%d physical=%d",
			port.ExecuteCalls.Load(), port.RuntimeInspectionCalls.Load(), port.BindingCalls.Load(),
			port.InspectCalls.Load(), port.PhysicalReads.Load())
	}

	late := fixture.Now.Add(20 * time.Second)
	port.ExecuteCalls.Store(0)
	port.PhysicalReads.Store(0)
	port.BindingCalls.Store(0)
	port.InspectCalls.Store(0)
	port.HistoricalCommandCalls.Store(0)
	port.RuntimeInspectionCalls.Store(0)
	port.RuntimeInspection, err = corepack.SealWorkspaceReadRuntimeAdmissionInspectionV1(
		corepack.WorkspaceReadRuntimeAdmissionInspectionV1{
			Attempt: fixture.Authorization.Attempt, Admission: fixture.Admission,
			CheckedUnixNano: late.UnixNano(), ExpiresUnixNano: late.Add(10 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	port.Envelope.CheckedUnixNano = late.UnixNano()
	port.Envelope.ExpiresUnixNano = late.Add(10 * time.Second).UnixNano()
	restarted := workspaceReadAdapterV1(t, port, late)
	recoveryCtx, cancelRecovery := context.WithCancel(context.Background())
	cancelRecovery()
	output, err = restarted.InspectWorkspaceReadRecoveryV1(
		recoveryCtx, workspaceReadRequestV1(t, fixture), fixture.Authorization,
	)
	if err != nil || output.Content == nil || *output.Content != "Praxis" {
		t.Fatalf("expired restart recovery did not inspect the original Attempt: output=%#v err=%v", output, err)
	}
	if port.ExecuteCalls.Load() != 0 || port.PhysicalReads.Load() != 0 ||
		port.RuntimeInspectionCalls.Load() != 1 || port.BindingCalls.Load() != 1 ||
		port.HistoricalCommandCalls.Load() != 1 || port.InspectCalls.Load() != 1 {
		t.Fatalf("restart recovery was not inspect-only: execute=%d physical=%d runtime=%d binding=%d historical=%d inspect=%d",
			port.ExecuteCalls.Load(), port.PhysicalReads.Load(), port.RuntimeInspectionCalls.Load(),
			port.BindingCalls.Load(), port.HistoricalCommandCalls.Load(), port.InspectCalls.Load())
	}
}

func TestWorkspaceReadSandboxAdapterV1Recovery64ConcurrentInspectOnly(t *testing.T) {
	fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
	port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
	adapter := workspaceReadAdapterV1(t, port, fixture.Now.Add(time.Millisecond))
	request := workspaceReadRequestV1(t, fixture)
	const workers = 64
	var wait sync.WaitGroup
	errorsCh := make(chan error, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			output, err := adapter.InspectWorkspaceReadRecoveryV1(context.Background(), request, fixture.Authorization)
			if err == nil && (output.Content == nil || *output.Content != "Praxis") {
				err = errors.New("inspect-only recovery output drifted")
			}
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if port.ExecuteCalls.Load() != 0 || port.PhysicalReads.Load() != 0 ||
		port.RuntimeInspectionCalls.Load() != workers ||
		port.BindingCalls.Load() != workers || port.HistoricalCommandCalls.Load() != workers ||
		port.InspectCalls.Load() != workers {
		t.Fatalf("concurrent recovery crossed execution: execute=%d physical=%d runtime=%d binding=%d historical=%d inspect=%d",
			port.ExecuteCalls.Load(), port.PhysicalReads.Load(), port.RuntimeInspectionCalls.Load(),
			port.BindingCalls.Load(), port.HistoricalCommandCalls.Load(), port.InspectCalls.Load())
	}
}

func TestWorkspaceReadSandboxAdapterV1SameKey64ConcurrentSinglePhysicalRead(t *testing.T) {
	fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
	port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
	adapter := workspaceReadAdapterV1(t, port, fixture.Now.Add(time.Millisecond))
	request := workspaceReadRequestV1(t, fixture)
	const workers = 64
	var wait sync.WaitGroup
	errorsCh := make(chan error, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			output, err := adapter.StartOrInspectWorkspaceReadV1(context.Background(), request, fixture.Authorization)
			if err == nil && (output.Content == nil || *output.Content != "Praxis") {
				err = errors.New("concurrent output drifted")
			}
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if port.ExecuteCalls.Load() != workers || port.PhysicalReads.Load() != 1 {
		t.Fatalf("same key did not converge at Sandbox: entries=%d physical=%d", port.ExecuteCalls.Load(), port.PhysicalReads.Load())
	}
}

func TestWorkspaceReadSandboxAdapterV1UnknownInspectsOriginalAttemptOnly(t *testing.T) {
	fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
	port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
	unknown := port.Projection.Attempt
	unknown.State = sandboxcontract.WorkspaceReadUnknownV1
	unknown.Observation = nil
	unknown.UnknownDigest = testkit.SandboxDigestV1("workspace-read-unknown-v1")
	unknown, err := sandboxcontract.SealWorkspaceReadAttemptV1(
		unknown, unknown.Meta.ID, unknown.Meta.Revision,
		fixture.Now, time.Unix(0, unknown.Meta.ExpiresUnixNano),
	)
	if err != nil {
		t.Fatal(err)
	}
	port.Projection.Attempt = unknown
	port.Projection.Observation = nil
	port.Projection.ProviderReceipt = nil
	adapter := workspaceReadAdapterV1(t, port, fixture.Now.Add(time.Millisecond))
	if _, err := adapter.StartOrInspectWorkspaceReadV1(context.Background(), workspaceReadRequestV1(t, fixture), fixture.Authorization); !errors.Is(err, sandboxports.ErrWorkspaceReadUnknown) {
		t.Fatalf("Unknown outcome was not returned as inspect-only: %v", err)
	}
	if port.ExecuteCalls.Load() != 1 || port.BindingCalls.Load() != 1 || port.InspectCalls.Load() != 1 || port.PhysicalReads.Load() != 1 {
		t.Fatalf("Unknown recovery redispatched or skipped exact Inspect: execute=%d binding=%d inspect=%d physical=%d",
			port.ExecuteCalls.Load(), port.BindingCalls.Load(), port.InspectCalls.Load(), port.PhysicalReads.Load())
	}
}

func TestWorkspaceReadSandboxAdapterV1InlineMaterializationBoundary(t *testing.T) {
	t.Run("exactly 256 KiB", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxWithContentV1(
			testkit.FixedTime, strings.Repeat("x", runtimeports.MaxOpaqueInlineBytes),
		)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		adapter := workspaceReadAdapterV1(t, port, fixture.Now.Add(time.Millisecond))
		output, err := adapter.StartOrInspectWorkspaceReadV1(
			context.Background(), workspaceReadRequestV1(t, fixture), fixture.Authorization,
		)
		if err != nil || output.Content == nil || len(*output.Content) != runtimeports.MaxOpaqueInlineBytes {
			t.Fatalf("256 KiB inline boundary failed: bytes=%d err=%v", len(derefWorkspaceReadContentV1(output.Content)), err)
		}
	})
	t.Run("256 KiB plus one requires Artifact", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxWithContentV1(
			testkit.FixedTime, strings.Repeat("x", runtimeports.MaxOpaqueInlineBytes+1),
		)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		adapter := workspaceReadAdapterV1(t, port, fixture.Now.Add(time.Millisecond))
		output, err := adapter.StartOrInspectWorkspaceReadV1(
			context.Background(), workspaceReadRequestV1(t, fixture), fixture.Authorization,
		)
		if err == nil || !core.HasReason(err, core.ReasonCanonicalLimitExceeded) {
			t.Fatalf("oversized inline result was not rejected for Artifact materialization: output=%#v err=%v", output, err)
		}
		if output.Content != nil {
			t.Fatal("oversized workspace.read produced inline Tool output")
		}
		if port.PhysicalReads.Load() != 1 || port.InspectCalls.Load() != 1 {
			t.Fatalf("oversized result did not preserve exact post-actual Inspect: physical=%d inspect=%d",
				port.PhysicalReads.Load(), port.InspectCalls.Load())
		}
	})
}

func derefWorkspaceReadContentV1(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func workspaceReadAdapterV1(t *testing.T, port *testkit.WorkspaceReadSandboxPortV1, now time.Time) *corepack.WorkspaceReadSandboxAdapterV1 {
	t.Helper()
	adapter, err := corepack.NewWorkspaceReadSandboxAdapterV1(
		port, port, port, port, func() time.Time { return now }, corepack.WorkspaceReadMinimumRecoveryTimeoutV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func workspaceReadRequestV1(t *testing.T, fixture testkit.WorkspaceReadSandboxFixtureV1) corepack.WorkspaceReadSandboxRequestV1 {
	t.Helper()
	request, err := corepack.SealWorkspaceReadSandboxRequestV1(corepack.WorkspaceReadSandboxRequestV1{
		SourceCommand: fixture.SourceCommand, PayloadSchema: fixture.PayloadSchema,
		PayloadDigest: fixture.PayloadDigest, PayloadRevision: 1,
		Input: fixture.Input, CanonicalInput: fixture.CanonicalInput,
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}
