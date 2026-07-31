package fault_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	sandboxcontract "github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/corepack"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestWorkspaceReadSandboxAdapterV1SourcePayloadSpliceZeroProvider(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*corepack.WorkspaceReadSandboxRequestV1)
	}{
		{name: "source command", mutate: func(v *corepack.WorkspaceReadSandboxRequestV1) {
			v.SourceCommand.ID = "other-tool-command"
		}},
		{name: "payload schema", mutate: func(v *corepack.WorkspaceReadSandboxRequestV1) {
			v.PayloadSchema.Name = "other-schema"
		}},
		{name: "payload revision", mutate: func(v *corepack.WorkspaceReadSandboxRequestV1) {
			v.PayloadRevision++
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
			port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
			request := workspaceReadFaultRequestV1(t, fixture)
			test.mutate(&request)
			request, err := corepack.SealWorkspaceReadSandboxRequestV1(request)
			if err != nil {
				t.Fatal(err)
			}
			adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
			if _, err = adapter.StartOrInspectWorkspaceReadV1(context.Background(), request, fixture.Authorization); err == nil {
				t.Fatal("source/payload splice was accepted")
			}
			if port.ExecuteCalls.Load() != 0 || port.PhysicalReads.Load() != 0 {
				t.Fatalf("source/payload splice reached Sandbox: execute=%d physical=%d", port.ExecuteCalls.Load(), port.PhysicalReads.Load())
			}
		})
	}
	t.Run("same semantic different payload bytes", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		request := workspaceReadFaultRequestV1(t, fixture)
		request.CanonicalInput = append([]byte(" "), request.CanonicalInput...)
		request.PayloadDigest = core.DigestBytes(request.CanonicalInput)
		if _, err := corepack.SealWorkspaceReadSandboxRequestV1(request); err == nil {
			t.Fatal("non-canonical bytes with the same workspace.read semantics were accepted")
		}
		if port.ExecuteCalls.Load() != 0 || port.PhysicalReads.Load() != 0 {
			t.Fatal("same-semantic different source payload reached Sandbox")
		}
	})
	t.Run("payload digest", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		request := workspaceReadFaultRequestV1(t, fixture)
		request.PayloadDigest = testkit.Digest("other-payload")
		if _, err := corepack.SealWorkspaceReadSandboxRequestV1(request); err == nil {
			t.Fatal("payload digest differing from canonical input was accepted")
		}
		if port.ExecuteCalls.Load() != 0 || port.PhysicalReads.Load() != 0 {
			t.Fatal("payload digest splice reached Sandbox")
		}
	})
}

func TestWorkspaceReadSandboxRequestV1RejectsNonCanonicalSchema(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*runtimeports.SchemaRefV2)
	}{
		{name: "uppercase namespace", mutate: func(schema *runtimeports.SchemaRefV2) {
			schema.Namespace = "Tool"
		}},
		{name: "noncanonical semantic version", mutate: func(schema *runtimeports.SchemaRefV2) {
			schema.Version = "1.0"
		}},
		{name: "noncanonical media type", mutate: func(schema *runtimeports.SchemaRefV2) {
			schema.MediaType = "Application/JSON"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
			request := workspaceReadFaultRequestV1(t, fixture)
			test.mutate(&request.PayloadSchema)
			if _, err := corepack.SealWorkspaceReadSandboxRequestV1(request); err == nil {
				t.Fatal("noncanonical Runtime SchemaRefV2 was accepted")
			}
		})
	}
}

func TestWorkspaceReadSandboxAdapterV1SandboxCommandSourcePayloadSpliceZeroProvider(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*sandboxcontract.WorkspaceReadCommandV1)
	}{
		{name: "source command", mutate: func(v *sandboxcontract.WorkspaceReadCommandV1) {
			v.SourceToolCommand.ID = "other-tool-command"
		}},
		{name: "payload schema", mutate: func(v *sandboxcontract.WorkspaceReadCommandV1) {
			v.SourceToolPayloadSchema = "praxis.tool/other-workspace-read-input@v1+json"
		}},
		{name: "payload digest", mutate: func(v *sandboxcontract.WorkspaceReadCommandV1) {
			v.SourceToolPayloadDigest = testkit.SandboxDigestV1("other-source-payload")
		}},
		{name: "payload revision", mutate: func(v *sandboxcontract.WorkspaceReadCommandV1) {
			v.SourceToolPayloadRevision++
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
			port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
			command := fixture.Command
			test.mutate(&command)
			command, err := sandboxcontract.SealWorkspaceReadCommandV1(
				command, command.Meta.ID, fixture.Now, time.Unix(0, command.Meta.ExpiresUnixNano),
			)
			if err != nil {
				t.Fatal(err)
			}
			authorization := fixture.Authorization
			authorization.DomainCommand.ID = command.Meta.ID
			authorization.DomainCommand.Revision = core.Revision(command.Meta.Revision)
			authorization.DomainCommand.Digest = core.Digest("sha256:" + command.Meta.Digest)
			authorization.AuthorizationDigest = ""
			authorization.StableKeyDigest = ""
			authorization, err = runtimeports.SealControlledOperationPhysicalExecutionAuthorizationV3(authorization)
			if err != nil {
				t.Fatal(err)
			}
			port.Command = command
			adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
			if _, err = adapter.StartOrInspectWorkspaceReadV1(
				context.Background(), workspaceReadFaultRequestV1(t, fixture), authorization,
			); err == nil {
				t.Fatal("valid Sandbox Command carrying another Tool source/payload was accepted")
			}
			assertWorkspaceReadZeroProviderV1(t, port)
		})
	}
}

func TestWorkspaceReadSandboxAdapterV1PreparedPayloadSpliceZeroProvider(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*runtimeports.PreparedProviderAttemptRefV2)
	}{
		{name: "payload schema", mutate: func(v *runtimeports.PreparedProviderAttemptRefV2) {
			v.PayloadSchema = testkit.Schema("other-workspace-read-input")
		}},
		{name: "payload digest", mutate: func(v *runtimeports.PreparedProviderAttemptRefV2) {
			v.PayloadDigest = testkit.Digest("other-workspace-read-payload")
		}},
		{name: "payload revision", mutate: func(v *runtimeports.PreparedProviderAttemptRefV2) {
			v.PayloadRevision++
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
			port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
			authorization := fixture.Authorization
			prepared := authorization.Prepared
			test.mutate(&prepared)
			prepared.Digest = ""
			prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(prepared)
			if err != nil {
				t.Fatal(err)
			}
			authorization.Prepared = prepared
			authorization.AuthorizationDigest = ""
			authorization.StableKeyDigest = ""
			authorization, err = runtimeports.SealControlledOperationPhysicalExecutionAuthorizationV3(authorization)
			if err != nil {
				t.Fatal(err)
			}
			adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
			if _, err = adapter.StartOrInspectWorkspaceReadV1(
				context.Background(), workspaceReadFaultRequestV1(t, fixture), authorization,
			); err == nil {
				t.Fatal("Runtime Prepared payload splice was accepted")
			}
			assertWorkspaceReadZeroProviderV1(t, port)
		})
	}
}

func TestWorkspaceReadSandboxAdapterV1PreActualFailuresZeroProvider(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := adapter.StartOrInspectWorkspaceReadV1(ctx, workspaceReadFaultRequestV1(t, fixture), fixture.Authorization); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled context was not preserved: %v", err)
		}
		assertWorkspaceReadZeroProviderV1(t, port)
	})
	t.Run("clock rollback", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		var calls atomic.Int64
		clock := func() time.Time {
			if calls.Add(1) == 1 {
				return fixture.Now.Add(2 * time.Millisecond)
			}
			return fixture.Now.Add(time.Millisecond)
		}
		adapter := workspaceReadFaultAdapterV1(t, port, clock)
		if _, err := adapter.StartOrInspectWorkspaceReadV1(context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization); err == nil {
			t.Fatal("clock rollback was accepted")
		}
		assertWorkspaceReadZeroProviderV1(t, port)
	})
	t.Run("context canceled after S2", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		ctx, cancel := context.WithCancel(context.Background())
		var calls atomic.Int64
		clock := func() time.Time {
			if calls.Add(1) == 2 {
				cancel()
			}
			return fixture.Now.Add(time.Millisecond)
		}
		adapter := workspaceReadFaultAdapterV1(t, port, clock)
		if _, err := adapter.StartOrInspectWorkspaceReadV1(
			ctx, workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
		); !errors.Is(err, context.Canceled) {
			t.Fatalf("post-S2 context cancellation was not preserved: %v", err)
		}
		if port.CommandCalls.Load() != 2 {
			t.Fatalf("context was not canceled after S2: command calls=%d", port.CommandCalls.Load())
		}
		assertWorkspaceReadZeroProviderV1(t, port)
	})
	t.Run("actual entry clock rollback", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		var calls atomic.Int64
		clock := func() time.Time {
			switch calls.Add(1) {
			case 1:
				return fixture.Now.Add(time.Millisecond)
			case 2:
				return fixture.Now.Add(2 * time.Millisecond)
			default:
				return fixture.Now.Add(time.Millisecond)
			}
		}
		adapter := workspaceReadFaultAdapterV1(t, port, clock)
		if _, err := adapter.StartOrInspectWorkspaceReadV1(
			context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
		); err == nil || !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("actual-entry clock rollback was accepted: %v", err)
		}
		assertWorkspaceReadZeroProviderV1(t, port)
	})
	t.Run("actual entry exact expiry", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		var calls atomic.Int64
		clock := func() time.Time {
			if calls.Add(1) == 3 {
				return time.Unix(0, fixture.Input.RequestedNotAfter)
			}
			return fixture.Now.Add(time.Millisecond)
		}
		adapter := workspaceReadFaultAdapterV1(t, port, clock)
		if _, err := adapter.StartOrInspectWorkspaceReadV1(
			context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
		); err == nil {
			t.Fatal("actual-entry exact expiry was accepted")
		}
		assertWorkspaceReadZeroProviderV1(t, port)
	})
	t.Run("expired", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time {
			return time.Unix(0, fixture.Input.RequestedNotAfter)
		})
		if _, err := adapter.StartOrInspectWorkspaceReadV1(context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization); err == nil {
			t.Fatal("expired request was accepted")
		}
		assertWorkspaceReadZeroProviderV1(t, port)
	})
	t.Run("command unavailable", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		port.CommandErr = core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "reader unavailable")
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
		if _, err := adapter.StartOrInspectWorkspaceReadV1(context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization); err == nil {
			t.Fatal("unavailable command reader was accepted")
		}
		if port.HistoricalCommandCalls.Load() != 0 {
			t.Fatalf("Start path fell back to the historical Command reader: calls=%d", port.HistoricalCommandCalls.Load())
		}
		assertWorkspaceReadZeroProviderV1(t, port)
	})
}

func TestWorkspaceReadSandboxAdapterV1PostActualRecoveryNeverRedispatches(t *testing.T) {
	t.Run("caller canceled after entry", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		ctx, cancel := context.WithCancel(context.Background())
		port.OnExecute = cancel
		port.ExecuteErr = context.Canceled
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
		output, err := adapter.StartOrInspectWorkspaceReadV1(ctx, workspaceReadFaultRequestV1(t, fixture), fixture.Authorization)
		if err != nil || output.Content == nil || *output.Content != "Praxis" {
			t.Fatalf("bounded exact recovery did not survive caller cancellation: output=%#v err=%v", output, err)
		}
		if port.ExecuteCalls.Load() != 1 || port.BindingCalls.Load() != 1 || port.InspectCalls.Load() != 1 {
			t.Fatalf("caller cancellation caused redispatch: execute=%d binding=%d inspect=%d",
				port.ExecuteCalls.Load(), port.BindingCalls.Load(), port.InspectCalls.Load())
		}
	})
	t.Run("lost binding reply remains unknown", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		port.BindingErr = core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "binding read lost")
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
		if _, err := adapter.StartOrInspectWorkspaceReadV1(context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization); err == nil {
			t.Fatal("lost binding reply was treated as a deterministic result")
		}
		if port.ExecuteCalls.Load() != 1 || port.BindingCalls.Load() != 1 || port.InspectCalls.Load() != 0 || port.PhysicalReads.Load() != 1 {
			t.Fatalf("lost binding reply changed effect count: execute=%d binding=%d inspect=%d physical=%d",
				port.ExecuteCalls.Load(), port.BindingCalls.Load(), port.InspectCalls.Load(), port.PhysicalReads.Load())
		}
	})
	t.Run("untrusted Execute admission inspects original Runtime Attempt", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		rejected, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(
			runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
				ID: "workspace-read-rejected-no-effect", Revision: 1,
				StableKeyDigest: fixture.Authorization.StableKeyDigest,
				NoEffect:        true,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		wrongKey, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(
			runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
				ID: "workspace-read-wrong-key-admission", Revision: 1,
				StableKeyDigest: testkit.Digest("other-admission-stable-key"),
				Admitted:        true,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, test := range []struct {
			name      string
			admission runtimeports.ControlledOperationProviderAdmissionReceiptRefV2
		}{
			{name: "zero"},
			{name: "malformed admitted", admission: runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
				ID: "workspace-read-malformed-admission", Revision: 1,
				StableKeyDigest: fixture.Authorization.StableKeyDigest,
				Admitted:        true,
				Digest:          testkit.Digest("wrong-admission-digest"),
			}},
			{name: "rejected no effect", admission: rejected},
			{name: "admitted wrong stable key", admission: wrongKey},
		} {
			t.Run(test.name, func(t *testing.T) {
				port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
				port.Admission = test.admission
				adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time {
					return fixture.Now.Add(time.Millisecond)
				})
				output, err := adapter.StartOrInspectWorkspaceReadV1(
					context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
				)
				if err != nil || output.Content == nil || *output.Content != "Praxis" {
					t.Fatalf("untrusted Execute admission did not recover by exact Inspect: output=%#v err=%v", output, err)
				}
				if _, err = adapter.InspectWorkspaceReadRecoveryV1(
					context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
				); err != nil {
					t.Fatalf("explicit inspect-only retry failed: %v", err)
				}
				if port.ExecuteCalls.Load() != 1 ||
					port.RuntimeInspectionCalls.Load() != 2 ||
					port.BindingCalls.Load() != 2 ||
					port.InspectCalls.Load() != 2 ||
					port.PhysicalReads.Load() != 1 {
					t.Fatalf("untrusted admission caused redispatch: execute=%d runtime-inspect=%d binding=%d terminal=%d physical=%d",
						port.ExecuteCalls.Load(), port.RuntimeInspectionCalls.Load(),
						port.BindingCalls.Load(), port.InspectCalls.Load(), port.PhysicalReads.Load())
				}
			})
		}
	})
	t.Run("reservation splice produces zero Tool output", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		observation := *port.Projection.Observation
		observation.Reservation.ID = "other-workspace-read-reservation"
		observation.Reservation.Digest = testkit.SandboxDigestV1("other-workspace-read-reservation")
		var err error
		observation, err = sandboxcontract.SealWorkspaceReadObservationV1(
			observation,
			observation.Meta.ID,
			fixture.Now,
			time.Unix(0, observation.Meta.ExpiresUnixNano),
		)
		if err != nil {
			t.Fatal(err)
		}
		attempt := port.Projection.Attempt
		observationRef := observation.Meta.Ref()
		attempt.Observation = &observationRef
		attempt, err = sandboxcontract.SealWorkspaceReadAttemptV1(
			attempt,
			attempt.Meta.ID,
			attempt.Meta.Revision,
			fixture.Now,
			time.Unix(0, attempt.Meta.ExpiresUnixNano),
		)
		if err != nil {
			t.Fatal(err)
		}
		port.Projection.Attempt = attempt
		port.Projection.Observation = &observation
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
		output, err := adapter.StartOrInspectWorkspaceReadV1(
			context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
		)
		if err == nil || output.Content != nil {
			t.Fatalf("reservation-spliced Observation produced Tool output=%#v err=%v", output, err)
		}
		if port.ExecuteCalls.Load() != 1 || port.InspectCalls.Load() != 1 || port.PhysicalReads.Load() != 1 {
			t.Fatalf("reservation splice changed exact recovery: execute=%d inspect=%d physical=%d",
				port.ExecuteCalls.Load(), port.InspectCalls.Load(), port.PhysicalReads.Load())
		}
	})
	for _, test := range []struct {
		name   string
		mutate func(*sandboxcontract.WorkspaceReadObservationV1)
	}{
		{name: "file ID splice produces zero Tool output", mutate: func(v *sandboxcontract.WorkspaceReadObservationV1) {
			v.File.ID = "other-workspace-read-file"
		}},
		{name: "file revision splice produces zero Tool output", mutate: func(v *sandboxcontract.WorkspaceReadObservationV1) {
			v.File.Revision++
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
			port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
			observation := *port.Projection.Observation
			test.mutate(&observation)
			var err error
			observation, err = sandboxcontract.SealWorkspaceReadObservationV1(
				observation,
				observation.Meta.ID,
				fixture.Now,
				time.Unix(0, observation.Meta.ExpiresUnixNano),
			)
			if err != nil {
				t.Fatal(err)
			}
			attempt := port.Projection.Attempt
			observationRef := observation.Meta.Ref()
			attempt.Observation = &observationRef
			attempt, err = sandboxcontract.SealWorkspaceReadAttemptV1(
				attempt,
				attempt.Meta.ID,
				attempt.Meta.Revision,
				fixture.Now,
				time.Unix(0, attempt.Meta.ExpiresUnixNano),
			)
			if err != nil {
				t.Fatal(err)
			}
			port.Projection.Attempt = attempt
			port.Projection.Observation = &observation
			adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
			output, err := adapter.StartOrInspectWorkspaceReadV1(
				context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
			)
			if err == nil || output.Content != nil {
				t.Fatalf("file-spliced Observation produced Tool output=%#v err=%v", output, err)
			}
			if port.ExecuteCalls.Load() != 1 || port.InspectCalls.Load() != 1 || port.PhysicalReads.Load() != 1 {
				t.Fatalf("file splice changed exact recovery: execute=%d inspect=%d physical=%d",
					port.ExecuteCalls.Load(), port.InspectCalls.Load(), port.PhysicalReads.Load())
			}
		})
	}
	t.Run("indeterminate reply with exact failed Attempt is deterministic", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		attempt := port.Projection.Attempt
		attempt.State = sandboxcontract.WorkspaceReadFailedV1
		attempt.Observation = nil
		attempt.FailureDigest = testkit.SandboxDigestV1("workspace-read-deterministic-failure")
		var err error
		attempt, err = sandboxcontract.SealWorkspaceReadAttemptV1(
			attempt,
			attempt.Meta.ID,
			attempt.Meta.Revision,
			fixture.Now,
			time.Unix(0, attempt.Meta.ExpiresUnixNano),
		)
		if err != nil {
			t.Fatal(err)
		}
		port.Projection.Attempt = attempt
		port.Projection.Observation = nil
		port.Projection.ProviderReceipt = nil
		port.ExecuteErr = core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "provider reply lost")
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
		output, err := adapter.StartOrInspectWorkspaceReadV1(
			context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
		)
		if err == nil ||
			!core.HasCategory(err, core.ErrorPreconditionFailed) ||
			!core.HasReason(err, core.ReasonEffectStateConflict) ||
			core.HasReason(err, core.ReasonEffectUnknownOutcome) ||
			output.Content != nil {
			t.Fatalf("exact failed Attempt did not dominate the lost reply: output=%#v err=%v", output, err)
		}
		if port.ExecuteCalls.Load() != 1 || port.InspectCalls.Load() != 1 || port.PhysicalReads.Load() != 1 {
			t.Fatalf("failed exact Inspect redispatched: execute=%d inspect=%d physical=%d",
				port.ExecuteCalls.Load(), port.InspectCalls.Load(), port.PhysicalReads.Load())
		}
	})
}

func TestWorkspaceReadSandboxAdapterV1V2RecoverySplicesFailClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testkit.WorkspaceReadSandboxPortV1, testkit.WorkspaceReadSandboxFixtureV1) error
	}{
		{name: "requested origin revision", mutate: func(port *testkit.WorkspaceReadSandboxPortV1, _ testkit.WorkspaceReadSandboxFixtureV1) error {
			port.Envelope.RequestedOriginAttemptRef.Revision++
			return nil
		}},
		{name: "requested origin digest", mutate: func(port *testkit.WorkspaceReadSandboxPortV1, _ testkit.WorkspaceReadSandboxFixtureV1) error {
			port.Envelope.RequestedOriginAttemptRef.Digest = testkit.SandboxDigestV1("other-origin-attempt")
			return nil
		}},
		{name: "Runtime attempt revision axis", mutate: func(port *testkit.WorkspaceReadSandboxPortV1, fixture testkit.WorkspaceReadSandboxFixtureV1) error {
			inspection := port.RuntimeInspection
			inspection.Attempt.IntentRevision++
			sealed, err := corepack.SealWorkspaceReadRuntimeAdmissionInspectionV1(inspection)
			port.RuntimeInspection = sealed
			return err
		}},
		{name: "Runtime attempt digest axis", mutate: func(port *testkit.WorkspaceReadSandboxPortV1, fixture testkit.WorkspaceReadSandboxFixtureV1) error {
			inspection := port.RuntimeInspection
			inspection.Attempt.PermitDigest = testkit.Digest("other-runtime-attempt")
			sealed, err := corepack.SealWorkspaceReadRuntimeAdmissionInspectionV1(inspection)
			port.RuntimeInspection = sealed
			return err
		}},
		{name: "Admission splice", mutate: func(port *testkit.WorkspaceReadSandboxPortV1, fixture testkit.WorkspaceReadSandboxFixtureV1) error {
			admission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(
				runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
					ID: "other-workspace-read-admission", Revision: 1,
					StableKeyDigest: fixture.Authorization.StableKeyDigest, Admitted: true,
				},
			)
			if err != nil {
				return err
			}
			inspection, err := corepack.SealWorkspaceReadRuntimeAdmissionInspectionV1(
				corepack.WorkspaceReadRuntimeAdmissionInspectionV1{
					Attempt: fixture.Authorization.Attempt, Admission: admission,
					CheckedUnixNano: fixture.Now.UnixNano(),
					ExpiresUnixNano: fixture.Now.Add(10 * time.Second).UnixNano(),
				},
			)
			port.RuntimeInspection = inspection
			return err
		}},
		{name: "terminal envelope expiry", mutate: func(port *testkit.WorkspaceReadSandboxPortV1, fixture testkit.WorkspaceReadSandboxFixtureV1) error {
			port.Envelope.CheckedUnixNano = fixture.Now.Add(-20 * time.Second).UnixNano()
			port.Envelope.ExpiresUnixNano = fixture.Now.Add(-10 * time.Second).UnixNano()
			return nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
			port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
			if err := test.mutate(port, fixture); err != nil {
				t.Fatal(err)
			}
			adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
			output, err := adapter.InspectWorkspaceReadRecoveryV1(
				context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
			)
			if err == nil || output.Content != nil {
				t.Fatalf("V2 recovery splice produced Tool output=%#v err=%v", output, err)
			}
			if port.ExecuteCalls.Load() != 0 || port.PhysicalReads.Load() != 0 {
				t.Fatalf("V2 recovery splice crossed execution: execute=%d physical=%d",
					port.ExecuteCalls.Load(), port.PhysicalReads.Load())
			}
		})
	}
}

func TestWorkspaceReadSandboxAdapterV1RecoveryRequiresExactHistoricalCommand(t *testing.T) {
	t.Run("SourceCommand splice", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		request := workspaceReadFaultRequestV1(t, fixture)
		request.SourceCommand.ID = "other-valid-tool-command"
		request.SourceCommand.Digest = testkit.Digest("other-valid-tool-command")
		var err error
		request, err = corepack.SealWorkspaceReadSandboxRequestV1(request)
		if err != nil {
			t.Fatal(err)
		}
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
		output, err := adapter.InspectWorkspaceReadRecoveryV1(context.Background(), request, fixture.Authorization)
		if err == nil || !core.HasCategory(err, core.ErrorConflict) || output.Content != nil {
			t.Fatalf("recovery SourceCommand splice was not rejected as Conflict: output=%#v err=%v", output, err)
		}
		if port.HistoricalCommandCalls.Load() != 1 ||
			port.InspectCalls.Load() != 0 ||
			port.ExecuteCalls.Load() != 0 ||
			port.PhysicalReads.Load() != 0 {
			t.Fatalf("recovery SourceCommand splice crossed the historical proof boundary: historical=%d inspect=%d execute=%d physical=%d",
				port.HistoricalCommandCalls.Load(), port.InspectCalls.Load(),
				port.ExecuteCalls.Load(), port.PhysicalReads.Load())
		}
	})

	t.Run("historical reader unavailable", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		port.HistoricalCommandErr = core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "historical reader unavailable")
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
		output, err := adapter.InspectWorkspaceReadRecoveryV1(
			context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
		)
		if err == nil || output.Content != nil {
			t.Fatalf("unavailable historical reader produced Tool output=%#v err=%v", output, err)
		}
		if port.HistoricalCommandCalls.Load() != 1 ||
			port.InspectCalls.Load() != 0 ||
			port.ExecuteCalls.Load() != 0 ||
			port.PhysicalReads.Load() != 0 {
			t.Fatalf("unavailable historical reader crossed recovery boundary: historical=%d inspect=%d execute=%d physical=%d",
				port.HistoricalCommandCalls.Load(), port.InspectCalls.Load(),
				port.ExecuteCalls.Load(), port.PhysicalReads.Load())
		}
	})

	t.Run("legacy unsealed historical Command", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		port.Command.TenantID = "unsealed-body-splice"
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
		output, err := adapter.InspectWorkspaceReadRecoveryV1(
			context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
		)
		if err == nil || !core.HasCategory(err, core.ErrorConflict) || output.Content != nil {
			t.Fatalf("legacy unsealed historical Command was not rejected as Conflict: output=%#v err=%v", output, err)
		}
		if port.HistoricalCommandCalls.Load() != 1 ||
			port.InspectCalls.Load() != 0 ||
			port.ExecuteCalls.Load() != 0 ||
			port.PhysicalReads.Load() != 0 {
			t.Fatalf("legacy unsealed Command crossed recovery boundary: historical=%d inspect=%d execute=%d physical=%d",
				port.HistoricalCommandCalls.Load(), port.InspectCalls.Load(),
				port.ExecuteCalls.Load(), port.PhysicalReads.Load())
		}
	})
}

func TestWorkspaceReadSandboxAdapterV1RecoveryNotFoundStopsAtEveryReadLevel(t *testing.T) {
	for _, test := range []struct {
		name       string
		mutate     func(*testkit.WorkspaceReadSandboxPortV1)
		runtime    int64
		binding    int64
		historical int64
		terminal   int64
	}{
		{
			name: "Runtime Attempt",
			mutate: func(port *testkit.WorkspaceReadSandboxPortV1) {
				port.RuntimeInspectionErr = sandboxports.ErrNotFound
			},
			runtime: 1,
		},
		{
			name: "Admission binding",
			mutate: func(port *testkit.WorkspaceReadSandboxPortV1) {
				port.BindingErr = sandboxports.ErrNotFound
			},
			runtime: 1, binding: 1,
		},
		{
			name: "historical Command",
			mutate: func(port *testkit.WorkspaceReadSandboxPortV1) {
				port.HistoricalCommandErr = sandboxports.ErrNotFound
			},
			runtime: 1, binding: 1, historical: 1,
		},
		{
			name: "terminal Attempt",
			mutate: func(port *testkit.WorkspaceReadSandboxPortV1) {
				port.InspectionErr = sandboxports.ErrNotFound
			},
			runtime: 1, binding: 1, historical: 1, terminal: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
			port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
			test.mutate(port)
			adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
			output, err := adapter.InspectWorkspaceReadRecoveryV1(
				context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
			)
			if err == nil || output.Content != nil {
				t.Fatalf("%s NotFound produced Tool output=%#v err=%v", test.name, output, err)
			}
			if port.RuntimeInspectionCalls.Load() != test.runtime ||
				port.BindingCalls.Load() != test.binding ||
				port.HistoricalCommandCalls.Load() != test.historical ||
				port.InspectCalls.Load() != test.terminal ||
				port.ExecuteCalls.Load() != 0 ||
				port.PhysicalReads.Load() != 0 {
				t.Fatalf("%s NotFound crossed its read level: runtime=%d binding=%d historical=%d terminal=%d execute=%d physical=%d",
					test.name, port.RuntimeInspectionCalls.Load(), port.BindingCalls.Load(),
					port.HistoricalCommandCalls.Load(), port.InspectCalls.Load(),
					port.ExecuteCalls.Load(), port.PhysicalReads.Load())
			}
		})
	}
}

func TestWorkspaceReadSandboxAdapterV1RecoveryRejectsReservationTTLClosureSplice(t *testing.T) {
	for _, test := range []struct {
		name              string
		mutateTTL         func(*sandboxcontract.WorkspaceReadTTLClosureV1, testkit.WorkspaceReadSandboxFixtureV1)
		mutateReservation func(*sandboxcontract.WorkspaceReadReservationV1)
		mutateBinding     func(*sandboxports.WorkspaceReadAdmissionAttemptBindingV1)
	}{
		{
			name: "unified not after",
			mutateTTL: func(ttl *sandboxcontract.WorkspaceReadTTLClosureV1, _ testkit.WorkspaceReadSandboxFixtureV1) {
				ttl.UnifiedNotAfterUnixNano++
			},
		},
		{
			name: "Runtime enforcement exceeds authorization upper bound",
			mutateTTL: func(ttl *sandboxcontract.WorkspaceReadTTLClosureV1, fixture testkit.WorkspaceReadSandboxFixtureV1) {
				ttl.RuntimeEnforcementExpiresNano = fixture.Authorization.ExecuteEnforcement.ExpiresUnixNano + 1
			},
		},
		{
			name: "Command requested not after",
			mutateTTL: func(ttl *sandboxcontract.WorkspaceReadTTLClosureV1, _ testkit.WorkspaceReadSandboxFixtureV1) {
				ttl.CommandRequestedNotAfterNano++
			},
		},
		{
			name: "Command expiry",
			mutateTTL: func(ttl *sandboxcontract.WorkspaceReadTTLClosureV1, _ testkit.WorkspaceReadSandboxFixtureV1) {
				ttl.CommandExpiresUnixNano++
			},
		},
		{
			name: "effective expiry differs from binding",
			mutateTTL: func(ttl *sandboxcontract.WorkspaceReadTTLClosureV1, fixture testkit.WorkspaceReadSandboxFixtureV1) {
				ttl.AssociationExpiresUnixNano = fixture.Now.Add(9 * time.Second).UnixNano()
			},
		},
		{
			name: "Reservation stable key",
			mutateReservation: func(reservation *sandboxcontract.WorkspaceReadReservationV1) {
				reservation.StableKeyDigest = testkit.SandboxDigestV1("other-reservation-stable-key")
			},
		},
		{
			name: "Reservation request digest",
			mutateReservation: func(reservation *sandboxcontract.WorkspaceReadReservationV1) {
				reservation.RequestDigest = testkit.SandboxDigestV1("other-reservation-request")
			},
		},
		{
			name: "Reservation attempt ID",
			mutateReservation: func(reservation *sandboxcontract.WorkspaceReadReservationV1) {
				reservation.AttemptID = "other-workspace-read-attempt"
			},
		},
		{
			name: "Admission checked differs from binding created",
			mutateBinding: func(binding *sandboxports.WorkspaceReadAdmissionAttemptBindingV1) {
				binding.CreatedUnixNano--
			},
		},
		{
			name: "Admission expiry differs from binding expiry",
			mutateBinding: func(binding *sandboxports.WorkspaceReadAdmissionAttemptBindingV1) {
				binding.ExpiresUnixNano++
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
			ttl := fixture.Projection.Reservation.TTLClosure
			if test.mutateTTL != nil {
				test.mutateTTL(&ttl, fixture)
			}
			port := workspaceReadFaultPortWithTTLClosureV1(
				t,
				fixture,
				ttl,
				test.mutateReservation,
				test.mutateBinding,
			)
			adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
			output, err := adapter.InspectWorkspaceReadRecoveryV1(
				context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
			)
			if err == nil || !core.HasCategory(err, core.ErrorConflict) || output.Content != nil {
				t.Fatalf("self-consistent Reservation TTL %s splice was not rejected: output=%#v err=%v", test.name, output, err)
			}
			if port.HistoricalCommandCalls.Load() != 1 ||
				port.InspectCalls.Load() != 1 ||
				port.ExecuteCalls.Load() != 0 ||
				port.PhysicalReads.Load() != 0 {
				t.Fatalf("Reservation TTL %s splice crossed the recovery boundary: historical=%d inspect=%d execute=%d physical=%d",
					test.name, port.HistoricalCommandCalls.Load(), port.InspectCalls.Load(),
					port.ExecuteCalls.Load(), port.PhysicalReads.Load())
			}
		})
	}
}

func TestWorkspaceReadSandboxAdapterV1RecoveryRejectsExpectedFileDigestSplice(t *testing.T) {
	fixture := testkit.WorkspaceReadSandboxWithExpectedFileDigestV1(
		testkit.FixedTime,
		"Praxis",
		testkit.SandboxDigestV1("other-expected-workspace-file"),
	)
	port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
	adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
	output, err := adapter.InspectWorkspaceReadRecoveryV1(
		context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
	)
	if err == nil || !core.HasCategory(err, core.ErrorConflict) || output.Content != nil {
		t.Fatalf("ExpectedFile digest splice produced Tool output=%#v err=%v", output, err)
	}
	if port.HistoricalCommandCalls.Load() != 1 ||
		port.InspectCalls.Load() != 1 ||
		port.ExecuteCalls.Load() != 0 ||
		port.PhysicalReads.Load() != 0 {
		t.Fatalf("ExpectedFile digest splice crossed recovery boundary: historical=%d inspect=%d execute=%d physical=%d",
			port.HistoricalCommandCalls.Load(), port.InspectCalls.Load(),
			port.ExecuteCalls.Load(), port.PhysicalReads.Load())
	}
}

func TestWorkspaceReadSandboxAdapterV1RecoveryRejectsHistoricalRuntimeCoordinateSplice(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*sandboxcontract.WorkspaceReadCommandV1)
	}{
		{
			name: "dispatch digest",
			mutate: func(command *sandboxcontract.WorkspaceReadCommandV1) {
				command.DispatchDigest = testkit.SandboxDigestV1("other-runtime-dispatch")
			},
		},
		{
			name: "tenant ID",
			mutate: func(command *sandboxcontract.WorkspaceReadCommandV1) {
				command.TenantID = "other-tenant"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
			command := fixture.Command
			test.mutate(&command)
			command, err := sandboxcontract.SealWorkspaceReadCommandV1(
				command,
				command.Meta.ID,
				fixture.Now,
				time.Unix(0, command.Meta.ExpiresUnixNano),
			)
			if err != nil {
				t.Fatal(err)
			}

			authorization := fixture.Authorization
			authorization.DomainCommand.Digest = core.Digest("sha256:" + command.Meta.Digest)
			authorization.StableKeyDigest = ""
			authorization.AuthorizationDigest = ""
			authorization, err = runtimeports.SealControlledOperationPhysicalExecutionAuthorizationV3(authorization)
			if err != nil {
				t.Fatal(err)
			}
			admission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(
				runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
					ID: fixture.Admission.ID, Revision: fixture.Admission.Revision,
					StableKeyDigest: authorization.StableKeyDigest, Admitted: true,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			runtimeInspection, err := corepack.SealWorkspaceReadRuntimeAdmissionInspectionV1(
				corepack.WorkspaceReadRuntimeAdmissionInspectionV1{
					Attempt: authorization.Attempt, Admission: admission,
					CheckedUnixNano: fixture.RuntimeInspection.CheckedUnixNano,
					ExpiresUnixNano: fixture.RuntimeInspection.ExpiresUnixNano,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			binding := fixture.Binding
			binding.AdmissionReceipt = admission
			binding.Command = command.Meta.Ref()
			binding.AuthorizationDigest = authorization.AuthorizationDigest
			binding.StableKeyDigest = authorization.StableKeyDigest
			binding.DomainCommand = authorization.DomainCommand
			binding, err = sandboxports.SealWorkspaceReadAdmissionAttemptBindingV1(binding)
			if err != nil {
				t.Fatal(err)
			}

			port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
			port.Command = command
			port.RuntimeInspection = runtimeInspection
			port.Binding = binding
			adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time { return fixture.Now.Add(time.Millisecond) })
			output, err := adapter.InspectWorkspaceReadRecoveryV1(
				context.Background(), workspaceReadFaultRequestV1(t, fixture), authorization,
			)
			if err == nil || !core.HasCategory(err, core.ErrorConflict) || output.Content != nil {
				t.Fatalf("historical %s splice was not rejected as Conflict: output=%#v err=%v", test.name, output, err)
			}
			if port.HistoricalCommandCalls.Load() != 1 ||
				port.InspectCalls.Load() != 0 ||
				port.ExecuteCalls.Load() != 0 ||
				port.PhysicalReads.Load() != 0 {
				t.Fatalf("historical %s splice crossed recovery boundary: historical=%d inspect=%d execute=%d physical=%d",
					test.name, port.HistoricalCommandCalls.Load(), port.InspectCalls.Load(),
					port.ExecuteCalls.Load(), port.PhysicalReads.Load())
			}
		})
	}
}

func TestWorkspaceReadSandboxAdapterV1TypedNilFailClosed(t *testing.T) {
	var typedNil *testkit.WorkspaceReadSandboxPortV1
	if adapter, err := corepack.NewWorkspaceReadSandboxAdapterV1(
		typedNil, typedNil, typedNil, typedNil, time.Now, corepack.WorkspaceReadMinimumRecoveryTimeoutV1,
	); err == nil || adapter != nil {
		t.Fatalf("typed-nil dependencies were accepted: adapter=%#v err=%v", adapter, err)
	}
	fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
	port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
	if adapter, err := corepack.NewWorkspaceReadSandboxAdapterV1(
		port, port, typedNil, port, time.Now, corepack.WorkspaceReadMinimumRecoveryTimeoutV1,
	); err == nil || adapter != nil {
		t.Fatalf("typed-nil historical Command reader was accepted: adapter=%#v err=%v", adapter, err)
	}
	for _, timeout := range []time.Duration{
		corepack.WorkspaceReadMinimumRecoveryTimeoutV1 - time.Nanosecond,
		corepack.WorkspaceReadMaximumRecoveryTimeoutV1 + time.Nanosecond,
	} {
		if adapter, err := corepack.NewWorkspaceReadSandboxAdapterV1(port, port, port, port, time.Now, timeout); err == nil || adapter != nil {
			t.Fatalf("out-of-bounds recovery timeout %s was accepted", timeout)
		}
	}
	adapter, err := corepack.NewWorkspaceReadSandboxAdapterV1(
		port,
		port,
		port,
		corepack.UnsupportedWorkspaceReadRuntimeAdmissionCurrentReaderV1{},
		func() time.Time { return fixture.Now.Add(time.Millisecond) },
		corepack.WorkspaceReadMinimumRecoveryTimeoutV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	output, err := adapter.InspectWorkspaceReadRecoveryV1(
		context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
	)
	if err == nil || output.Content != nil || port.ExecuteCalls.Load() != 0 || port.PhysicalReads.Load() != 0 {
		t.Fatalf("unsupported Runtime reader did not fail closed without execution: output=%#v err=%v execute=%d physical=%d",
			output, err, port.ExecuteCalls.Load(), port.PhysicalReads.Load())
	}
}

func TestWorkspaceReadSandboxAdapterV1FreezesAuthorizationPointersBeforeS1(t *testing.T) {
	fixture := testkit.WorkspaceReadSandboxV1(testkit.FixedTime)
	if fixture.Authorization.Operation.ExecutionScope.SandboxLease == nil ||
		fixture.Authorization.Attempt.Delegation == nil {
		t.Fatal("fixture lacks the V3 authorization pointers under test")
	}
	originalLease := *fixture.Authorization.Operation.ExecutionScope.SandboxLease
	originalDelegation := *fixture.Authorization.Attempt.Delegation
	port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
	snapshotReady := make(chan struct{})
	continueCall := make(chan struct{})
	var once sync.Once
	clock := func() time.Time {
		once.Do(func() {
			close(snapshotReady)
			<-continueCall
		})
		return fixture.Now.Add(time.Millisecond)
	}
	adapter := workspaceReadFaultAdapterV1(t, port, clock)
	request := workspaceReadFaultRequestV1(t, fixture)
	result := make(chan error, 1)
	go func() {
		output, err := adapter.StartOrInspectWorkspaceReadV1(
			context.Background(), request, fixture.Authorization,
		)
		if err == nil && (output.Content == nil || *output.Content != "Praxis") {
			err = errors.New("authorization snapshot produced a drifted output")
		}
		result <- err
	}()
	<-snapshotReady
	fixture.Authorization.Operation.ExecutionScope.SandboxLease.Epoch++
	fixture.Authorization.Attempt.Delegation.Revision++
	close(continueCall)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	executed := port.ExecutedAuthorizationV1()
	fixture.Authorization.Operation.ExecutionScope.SandboxLease.Epoch++
	fixture.Authorization.Attempt.Delegation.Revision++
	if executed.Operation.ExecutionScope.SandboxLease == fixture.Authorization.Operation.ExecutionScope.SandboxLease ||
		executed.Attempt.Delegation == fixture.Authorization.Attempt.Delegation ||
		*executed.Operation.ExecutionScope.SandboxLease != originalLease ||
		*executed.Attempt.Delegation != originalDelegation {
		t.Fatalf("physical entry did not receive the frozen authorization snapshot: executed=%#v", executed)
	}
}

func TestWorkspaceReadSandboxAdapterV1FreezesExpectedFileRefAfterReaderReturn(t *testing.T) {
	expectedDigest := testkit.SandboxDigestV1("workspace-file-tool-adapter-v1")
	t.Run("current reader", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxWithExpectedFileDigestV1(
			testkit.FixedTime,
			"Praxis",
			expectedDigest,
		)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		original := *port.Command.ExpectedFileRef
		port.OnExecute = func() {
			port.Command.ExpectedFileRef.Digest = testkit.SandboxDigestV1("mutated-after-current-read")
		}
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time {
			return fixture.Now.Add(time.Millisecond)
		})
		output, err := adapter.StartOrInspectWorkspaceReadV1(
			context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
		)
		if err != nil || output.Content == nil || *output.Content != "Praxis" {
			t.Fatalf("current Command alias mutation changed the frozen proof: output=%#v err=%v", output, err)
		}
		if original == *port.Command.ExpectedFileRef {
			t.Fatal("current Command adversary did not mutate the source pointer")
		}
	})
	t.Run("historical reader", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxWithExpectedFileDigestV1(
			testkit.FixedTime,
			"Praxis",
			expectedDigest,
		)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		original := *port.Command.ExpectedFileRef
		port.OnInspect = func() {
			port.Command.ExpectedFileRef.Digest = testkit.SandboxDigestV1("mutated-after-historical-read")
		}
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time {
			return fixture.Now.Add(time.Millisecond)
		})
		output, err := adapter.InspectWorkspaceReadRecoveryV1(
			context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
		)
		if err != nil || output.Content == nil || *output.Content != "Praxis" {
			t.Fatalf("historical Command alias mutation changed the frozen proof: output=%#v err=%v", output, err)
		}
		if original == *port.Command.ExpectedFileRef {
			t.Fatal("historical Command adversary did not mutate the source pointer")
		}
	})
	t.Run("concurrent mutation after S2 clone", func(t *testing.T) {
		fixture := testkit.WorkspaceReadSandboxWithExpectedFileDigestV1(
			testkit.FixedTime,
			"Praxis",
			expectedDigest,
		)
		port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
		stop := make(chan struct{})
		done := make(chan struct{})
		port.OnExecute = func() {
			go func() {
				defer close(done)
				for {
					select {
					case <-stop:
						return
					default:
						port.Command.ExpectedFileRef.Digest = testkit.SandboxDigestV1("concurrent-alias-mutation")
					}
				}
			}()
		}
		adapter := workspaceReadFaultAdapterV1(t, port, func() time.Time {
			return fixture.Now.Add(time.Millisecond)
		})
		output, err := adapter.StartOrInspectWorkspaceReadV1(
			context.Background(), workspaceReadFaultRequestV1(t, fixture), fixture.Authorization,
		)
		close(stop)
		<-done
		if err != nil || output.Content == nil || *output.Content != "Praxis" {
			t.Fatalf("concurrent Command alias mutation changed the frozen proof: output=%#v err=%v", output, err)
		}
	})
}

func workspaceReadFaultRequestV1(t *testing.T, fixture testkit.WorkspaceReadSandboxFixtureV1) corepack.WorkspaceReadSandboxRequestV1 {
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

func workspaceReadFaultAdapterV1(t *testing.T, port *testkit.WorkspaceReadSandboxPortV1, clock func() time.Time) *corepack.WorkspaceReadSandboxAdapterV1 {
	t.Helper()
	adapter, err := corepack.NewWorkspaceReadSandboxAdapterV1(
		port, port, port, port, clock, corepack.WorkspaceReadMinimumRecoveryTimeoutV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func workspaceReadFaultPortWithTTLClosureV1(
	t *testing.T,
	fixture testkit.WorkspaceReadSandboxFixtureV1,
	ttl sandboxcontract.WorkspaceReadTTLClosureV1,
	mutateReservation func(*sandboxcontract.WorkspaceReadReservationV1),
	mutateBinding func(*sandboxports.WorkspaceReadAdmissionAttemptBindingV1),
) *testkit.WorkspaceReadSandboxPortV1 {
	t.Helper()
	ttl, err := sandboxcontract.SealWorkspaceReadTTLClosureV1(ttl)
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Unix(0, ttl.EffectiveExpiresUnixNano)
	reservation := fixture.Projection.Reservation
	reservation.TTLClosure = ttl
	if mutateReservation != nil {
		mutateReservation(&reservation)
	}
	reservation, err = sandboxcontract.SealWorkspaceReadReservationV1(
		reservation,
		reservation.Meta.ID,
		fixture.Now,
		expires,
	)
	if err != nil {
		t.Fatal(err)
	}
	admissionReceipt := fixture.Projection.AdmissionReceipt
	admissionReceipt.StableKeyDigest = reservation.StableKeyDigest
	providerReceipt := *fixture.Projection.ProviderReceipt
	providerReceipt.StableKeyDigest = reservation.StableKeyDigest
	observation := *fixture.Projection.Observation
	observation.Reservation = reservation.Meta.Ref()
	observation.AdmissionReceipt = admissionReceipt
	observation.ProviderReceipt = providerReceipt
	observation, err = sandboxcontract.SealWorkspaceReadObservationV1(
		observation,
		observation.Meta.ID,
		fixture.Now,
		expires,
	)
	if err != nil {
		t.Fatal(err)
	}
	attempt := fixture.Projection.Attempt
	attempt.Reservation = reservation.Meta.Ref()
	attempt.StableKeyDigest = reservation.StableKeyDigest
	attempt.RequestDigest = reservation.RequestDigest
	attempt.AdmissionReceipt = admissionReceipt
	observationRef := observation.Meta.Ref()
	attempt.Observation = &observationRef
	attempt, err = sandboxcontract.SealWorkspaceReadAttemptV1(
		attempt,
		attempt.Meta.ID,
		attempt.Meta.Revision,
		fixture.Now,
		expires,
	)
	if err != nil {
		t.Fatal(err)
	}
	projection := fixture.Projection
	projection.Reservation = reservation
	projection.AdmissionReceipt = admissionReceipt
	projection.Observation = &observation
	projection.ProviderReceipt = &providerReceipt
	projection.Attempt = attempt
	if err = projection.ValidateShape(); err != nil {
		t.Fatal(err)
	}
	envelope := fixture.Envelope
	envelope.CurrentProjection = projection
	envelope, err = sandboxports.SealWorkspaceReadInspectionEnvelopeV2(envelope)
	if err != nil {
		t.Fatal(err)
	}
	port := testkit.WorkspaceReadSandboxPortFromFixtureV1(fixture)
	binding := fixture.Binding
	if mutateBinding != nil {
		mutateBinding(&binding)
		binding, err = sandboxports.SealWorkspaceReadAdmissionAttemptBindingV1(binding)
		if err != nil {
			t.Fatal(err)
		}
	}
	port.Binding = binding
	port.Projection = projection
	port.Envelope = envelope
	return port
}

func assertWorkspaceReadZeroProviderV1(t *testing.T, port *testkit.WorkspaceReadSandboxPortV1) {
	t.Helper()
	if port.ExecuteCalls.Load() != 0 || port.PhysicalReads.Load() != 0 {
		t.Fatalf("pre-actual failure reached Sandbox: execute=%d physical=%d", port.ExecuteCalls.Load(), port.PhysicalReads.Load())
	}
}
