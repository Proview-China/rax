package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	toolsqlite "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestModelToolInjectionLineageV1ExactPositiveAndAliasSafe(t *testing.T) {
	fixture := newSQLiteLineageFixtureV1(t)
	projection, err := fixture.reader.InspectCurrentModelToolInjectionLineageV1(
		context.Background(), fixture.request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Material.Ref != fixture.material.Ref ||
		projection.MaterialSource.ID != fixture.material.Ref.ID ||
		projection.MaterialSource.Digest != fixture.material.Ref.Digest ||
		projection.MaterialSource.Digest == fixture.material.ExpectedInjectionDigest ||
		projection.SurfaceSource.ID != fixture.current.Ref.ID ||
		projection.SurfaceSource.Digest != fixture.current.Ref.Digest ||
		projection.ExpectedInjectionDigest != fixture.material.ExpectedInjectionDigest ||
		projection.ActualCompiledToolsDigest != fixture.compiled.Digest {
		t.Fatalf("lineage projection did not preserve exact owner refs: %#v", projection)
	}
	if projection.MaterialSource.Kind != toolcontract.ModelToolInjectionMaterialSourceKindV1 ||
		projection.SurfaceSource.Kind != toolcontract.ToolSurfaceManifestCurrentSourceKindV1 {
		t.Fatalf("lineage source Kind drifted: material=%q surface=%q", projection.MaterialSource.Kind, projection.SurfaceSource.Kind)
	}
	if err = projection.ValidateCurrent(fixture.material.Ref, fixture.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err = projection.ValidateAgainst(fixture.request, fixture.clock.Now()); err != nil {
		t.Fatal(err)
	}
	changedCall := fixture.request
	changedCall.RouteCall.Request.Model += "-drift"
	if err = projection.ValidateAgainst(changedCall, fixture.clock.Now()); err == nil {
		t.Fatal("Projection accepted a different actual RouteCall")
	}
	original := projection.Clone()
	projection.CompiledTools.Tools[0].Name = "mutated"
	projection.CompiledTools.Tools[0].Parameters[0] = '!'
	projection.Material.Entries[0].ModelName = "mutated"
	projection.Surface.Manifest.Entries[0].ModelName = "mutated"
	again, err := fixture.reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, again) {
		t.Fatal("caller aliases mutated the authoritative lineage")
	}
}

func TestModelToolInjectionLineageV1StoredClosureSpliceFailClosed(t *testing.T) {
	for name, statement := range map[string]string{
		"compiled body": `UPDATE model_tool_injection_material_v1 SET compiled_tools_json=x'7b7d'`,
		"material body": `UPDATE model_tool_injection_material_v1 SET body_json=x'7b7d'`,
		"row digest":    `UPDATE model_tool_injection_material_v1 SET row_digest='sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd'`,
	} {
		t.Run(name, func(t *testing.T) {
			var providerCalls atomic.Int64
			fixture := newSQLiteLineageFixtureV1(t)
			if err := fixture.store.Close(); err != nil {
				t.Fatal(err)
			}
			tamperSQLiteV1(t, fixture.path, statement)
			var err error
			fixture.store, err = toolsqlite.OpenV1(context.Background(), toolsqlite.ConfigV1{
				Path: fixture.path, Clock: fixture.clock.Now, Owner: testkit.Owner(),
			})
			if err != nil {
				if providerCalls.Load() != 0 {
					t.Fatalf("provider was called after Open rejected stored splice: %d", providerCalls.Load())
				}
				return
			}
			t.Cleanup(func() { _ = fixture.store.Close() })
			reader, err := surface.NewModelToolInjectionLineageReaderV1(
				fixture.store, fixture.surfaces, testkit.Owner(), fixture.clock.Now,
			)
			if err != nil {
				t.Fatal(err)
			}
			projection, inspectErr := inspectLineageBeforeProviderV1(
				context.Background(), reader, fixture.request, func() { providerCalls.Add(1) },
			)
			if inspectErr == nil || projection.ContractVersion != "" {
				t.Fatalf("stored closure splice was accepted: projection=%#v err=%v", projection, inspectErr)
			}
			if providerCalls.Load() != 0 {
				t.Fatal("stored closure splice reached a Provider")
			}
		})
	}
}

func TestModelToolInjectionLineageV1RouteToolSpliceMatrix(t *testing.T) {
	mutations := map[string]func(*modelinvoker.RouteCall){
		"add": func(call *modelinvoker.RouteCall) {
			extra := cloneModelToolV1(call.Request.Tools[0])
			extra.Name = "z-extra"
			call.Request.Tools = append(call.Request.Tools, extra)
		},
		"remove": func(call *modelinvoker.RouteCall) {
			call.Request.Tools = nil
		},
		"reorder": func(call *modelinvoker.RouteCall) {
			extra := cloneModelToolV1(call.Request.Tools[0])
			extra.Name = "z-after"
			call.Request.Tools = append(call.Request.Tools, extra)
			call.Request.Tools[0], call.Request.Tools[1] = call.Request.Tools[1], call.Request.Tools[0]
		},
		"name": func(call *modelinvoker.RouteCall) {
			call.Request.Tools[0].Name += "-splice"
		},
		"description": func(call *modelinvoker.RouteCall) {
			call.Request.Tools[0].Description += " splice"
		},
		"schema": func(call *modelinvoker.RouteCall) {
			call.Request.Tools[0].Parameters = json.RawMessage(`{"type":"object","properties":{"other":{"type":"string"}},"required":["other"],"additionalProperties":false}`)
		},
		"strict nil": func(call *modelinvoker.RouteCall) {
			call.Request.Tools[0].Strict = nil
		},
		"strict false": func(call *modelinvoker.RouteCall) {
			value := false
			call.Request.Tools[0].Strict = &value
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var providerCalls atomic.Int64
			fixture := newSQLiteLineageFixtureV1(t)
			mutate(&fixture.request.RouteCall)
			projection, err := inspectLineageBeforeProviderV1(
				context.Background(), fixture.reader, fixture.request, func() { providerCalls.Add(1) },
			)
			if err == nil || projection.ContractVersion != "" {
				t.Fatalf("RouteCall Tool splice was accepted: projection=%#v err=%v", projection, err)
			}
			if providerCalls.Load() != 0 {
				t.Fatal("fail-closed lineage validation reached a Provider")
			}
		})
	}
}

func TestModelToolInjectionLineageV1RouteCallDigestBindsNonToolFields(t *testing.T) {
	fixture := newSQLiteLineageFixtureV1(t)
	first, err := fixture.reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	fixture.request.RouteCall.Request.Model = "same-tools-another-model"
	second, err := fixture.reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ActualCompiledToolsDigest != second.ActualCompiledToolsDigest ||
		first.RouteCallDigest == second.RouteCallDigest {
		t.Fatalf("RouteCall non-Tool field was not separated from Tool compilation: first=%#v second=%#v", first, second)
	}
}

func TestModelToolInjectionLineageV1RouteCallDigestAxesFailClosed(t *testing.T) {
	mutations := map[string]func(*modelinvoker.RouteCall){
		"route id": func(call *modelinvoker.RouteCall) {
			call.RouteID += ".splice"
		},
		"invocation": func(call *modelinvoker.RouteCall) {
			call.Invocation.Production = !call.Invocation.Production
		},
		"entitlement": func(call *modelinvoker.RouteCall) {
			call.EntitlementState = &upstream.EntitlementState{}
		},
		"provider": func(call *modelinvoker.RouteCall) {
			call.Request.Provider = modelinvoker.ProviderID("splice")
		},
		"protocol": func(call *modelinvoker.RouteCall) {
			call.Request.Protocol = modelinvoker.ProtocolResponses
		},
		"endpoint": func(call *modelinvoker.RouteCall) {
			call.Request.Endpoint = "https://splice.invalid"
		},
		"model": func(call *modelinvoker.RouteCall) {
			call.Request.Model += "-splice"
		},
		"input": func(call *modelinvoker.RouteCall) {
			call.Request.Input[0].Message.Text += " splice"
		},
		"instructions": func(call *modelinvoker.RouteCall) {
			call.Request.Instructions = append(call.Request.Instructions, modelinvoker.Instruction{
				Role: modelinvoker.RoleSystem, Text: "splice",
			})
		},
		"tool choice": func(call *modelinvoker.RouteCall) {
			call.Request.ToolChoice = modelinvoker.ToolChoice{
				Mode: modelinvoker.ToolChoiceFunction, Name: call.Request.Tools[0].Name,
			}
		},
		"parallel tool calls": func(call *modelinvoker.RouteCall) {
			value := true
			call.Request.ParallelToolCalls = &value
		},
		"output constraint": func(call *modelinvoker.RouteCall) {
			call.Request.Output.Type = modelinvoker.OutputJSONObject
		},
		"reasoning": func(call *modelinvoker.RouteCall) {
			call.Request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortLow}
		},
		"state": func(call *modelinvoker.RouteCall) {
			call.Request.State = &modelinvoker.State{
				Kind: modelinvoker.StateServerContinuation, ID: "splice",
			}
		},
		"stream": func(call *modelinvoker.RouteCall) {
			call.Request.Stream = true
		},
		"budget": func(call *modelinvoker.RouteCall) {
			call.Request.Budget.MaxOutputTokens++
		},
		"metadata": func(call *modelinvoker.RouteCall) {
			call.Request.Metadata = modelinvoker.Metadata{"splice": "true"}
		},
		"provider options": func(call *modelinvoker.RouteCall) {
			call.Request.ProviderOptions = modelinvoker.ProviderOptions{
				modelinvoker.ProviderID("splice"): json.RawMessage(`{"splice":true}`),
			}
		},
		"allow degradation": func(call *modelinvoker.RouteCall) {
			call.Request.AllowDegradation = !call.Request.AllowDegradation
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := newSQLiteLineageFixtureV1(t)
			projection := mustLineageProjectionV1(t, fixture)
			request := fixture.request
			mutate(&request.RouteCall)
			var providerCalls atomic.Int64
			if err := validateLineageBeforeProviderV1(
				projection, request, fixture.clock.Now(), func() { providerCalls.Add(1) },
			); err == nil {
				t.Fatal("changed RouteCall digest axis was accepted")
			}
			if providerCalls.Load() != 0 {
				t.Fatal("changed RouteCall digest axis reached a Provider")
			}
		})
	}
}

func TestModelToolInjectionLineageV1ExactRefAxesFailClosed(t *testing.T) {
	fixture := newSQLiteLineageFixtureV1(t)
	mutations := map[string]func(*surface.ModelToolInjectionLineageInspectRequestV1){
		"material id": func(request *surface.ModelToolInjectionLineageInspectRequestV1) {
			request.Material.ID += "-other"
		},
		"material revision": func(request *surface.ModelToolInjectionLineageInspectRequestV1) {
			request.Material.Revision++
		},
		"material digest": func(request *surface.ModelToolInjectionLineageInspectRequestV1) {
			request.Material.Digest = testkit.Digest("other-material")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			request := fixture.request
			mutate(&request)
			if _, err := fixture.reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), request); err == nil {
				t.Fatal("changed exact Material Ref was accepted")
			}
		})
	}
	sourceMutations := map[string]func(*surface.ModelToolInjectionLineageCurrentProjectionV1){
		"material source id": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.MaterialSource.ID += "-splice"
		},
		"material source revision": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.MaterialSource.Revision++
		},
		"material source kind": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.MaterialSource.Kind = toolcontract.ToolSurfaceManifestCurrentSourceKindV1
		},
		"material source digest": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.MaterialSource.Digest = testkit.Digest("other-material-source")
		},
		"material source cross-domain digest": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.MaterialSource.Digest = projection.SurfaceSource.Digest
		},
		"surface source id": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.SurfaceSource.ID += "-splice"
		},
		"surface source revision": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.SurfaceSource.Revision++
		},
		"surface source kind": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.SurfaceSource.Kind = toolcontract.ModelToolInjectionMaterialSourceKindV1
		},
		"surface source digest": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.SurfaceSource.Digest = testkit.Digest("other-surface-source")
		},
		"surface source cross-domain digest": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.SurfaceSource.Digest = projection.MaterialSource.Digest
		},
	}
	for name, mutate := range sourceMutations {
		t.Run(name, func(t *testing.T) {
			projection := mustLineageProjectionV1(t, fixture)
			mutate(&projection)
			projection.ProjectionDigest, _ = projection.ComputeDigest()
			var providerCalls atomic.Int64
			if err := validateLineageBeforeProviderV1(
				projection, fixture.request, fixture.clock.Now(), func() { providerCalls.Add(1) },
			); err == nil {
				t.Fatal("changed lineage source exact coordinate was accepted")
			}
			if providerCalls.Load() != 0 {
				t.Fatal("changed lineage source exact coordinate reached a Provider")
			}
		})
	}
}

func TestModelToolInjectionLineageV1FreezesRouteCallBeforeConcurrentMutation(t *testing.T) {
	fixture := newSQLiteLineageFixtureV1(t)
	budgetTokens := int64(64)
	fixture.request.RouteCall.Request.Instructions = []modelinvoker.Instruction{{
		Role: modelinvoker.RoleSystem, Text: "stable",
	}}
	fixture.request.RouteCall.Request.Input = append(
		fixture.request.RouteCall.Request.Input,
		modelinvoker.FunctionCallInput("call-stable", "tool.example", json.RawMessage(`{"value":"stable"}`)),
		modelinvoker.NamedFunctionResultInput("call-stable", "tool.example", "stable", false),
	)
	fixture.request.RouteCall.Request.Reasoning = &modelinvoker.Reasoning{
		Effort: modelinvoker.ReasoningEffortLow, BudgetTokens: &budgetTokens,
	}
	fixture.request.RouteCall.Request.Metadata = modelinvoker.Metadata{"trace": "stable"}
	fixture.request.RouteCall.Request.ProviderOptions = modelinvoker.ProviderOptions{}
	expected := mustLineageProjectionV1(t, fixture)
	barrier := &snapshotBarrierClosureReaderV1{
		base: fixture.store, entered: make(chan struct{}), release: make(chan struct{}),
	}
	reader, err := surface.NewModelToolInjectionLineageReaderV1(
		barrier, fixture.surfaces, testkit.Owner(), fixture.clock.Now,
	)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.request
	type resultV1 struct {
		projection surface.ModelToolInjectionLineageCurrentProjectionV1
		err        error
	}
	result := make(chan resultV1, 1)
	go func() {
		projection, inspectErr := reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), request)
		result <- resultV1{projection: projection, err: inspectErr}
	}()
	select {
	case <-barrier.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("lineage Reader did not finish freezing RouteCall")
	}
	fixture.request.RouteCall.Request.Input[0].Message.Text = "mutated"
	fixture.request.RouteCall.Request.Input[1].FunctionCall.Arguments[0] = '!'
	fixture.request.RouteCall.Request.Input[2].FunctionResult.Output = "mutated"
	fixture.request.RouteCall.Request.Instructions[0].Text = "mutated"
	fixture.request.RouteCall.Request.Tools[0].Description = "mutated"
	fixture.request.RouteCall.Request.Tools[0].Parameters[0] = '!'
	*fixture.request.RouteCall.Request.Tools[0].Strict = false
	*fixture.request.RouteCall.Request.ParallelToolCalls = true
	fixture.request.RouteCall.Request.Reasoning.Effort = modelinvoker.ReasoningEffortHigh
	*fixture.request.RouteCall.Request.Reasoning.BudgetTokens = 1
	fixture.request.RouteCall.Request.Metadata["trace"] = "mutated"
	close(barrier.release)
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if !reflect.DeepEqual(expected, got.projection) {
			t.Fatalf("concurrent caller mutation mixed RouteCall snapshots: expected=%#v got=%#v", expected, got.projection)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lineage Reader did not finish after concurrent caller mutation")
	}
}

func TestModelToolInjectionLineageV1OwnerSpliceAxesFailClosed(t *testing.T) {
	mutations := map[string]func(*surface.ModelToolInjectionLineageCurrentProjectionV1){
		"material owner": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.MaterialOwner.ID = core.OwnerID("other-material-owner")
		},
		"material source owner": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.MaterialSource.Owner.ID = core.OwnerID("other-material-source-owner")
		},
		"surface source owner": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.SurfaceSource.Owner.ID = core.OwnerID("other-surface-source-owner")
		},
		"surface current owner": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.Surface.Owner.ID = core.OwnerID("other-surface-owner")
		},
		"surface manifest owner": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.Surface.Manifest.Owner.ID = core.OwnerID("other-manifest-owner")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var providerCalls atomic.Int64
			fixture := newSQLiteLineageFixtureV1(t)
			projection := mustLineageProjectionV1(t, fixture)
			mutate(&projection)
			projection.ProjectionDigest, _ = projection.ComputeDigest()
			if err := validateLineageBeforeProviderV1(
				projection, fixture.request, fixture.clock.Now(), func() { providerCalls.Add(1) },
			); err == nil {
				t.Fatal("owner splice was accepted")
			}
			if providerCalls.Load() != 0 {
				t.Fatal("owner splice reached a Provider")
			}
		})
	}

	t.Run("configured joint reader owner", func(t *testing.T) {
		var providerCalls atomic.Int64
		fixture := newSQLiteLineageFixtureV1(t)
		other := testkit.Owner()
		other.ID = core.OwnerID("other-configured-owner")
		reader, err := surface.NewModelToolInjectionLineageReaderV1(
			fixture.store, fixture.surfaces, other, fixture.clock.Now,
		)
		if err != nil {
			t.Fatal(err)
		}
		projection, inspectErr := inspectLineageBeforeProviderV1(
			context.Background(), reader, fixture.request, func() { providerCalls.Add(1) },
		)
		if inspectErr == nil || projection.ContractVersion != "" {
			t.Fatalf("configured owner splice was accepted: projection=%#v err=%v", projection, inspectErr)
		}
		if providerCalls.Load() != 0 {
			t.Fatal("configured owner splice reached a Provider")
		}
	})
}

func TestModelToolInjectionLineageV1CheckedCausalityFailClosed(t *testing.T) {
	mutations := map[string]func(*surface.ModelToolInjectionLineageCurrentProjectionV1){
		"before material creation": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.CheckedUnixNano = projection.Material.CreatedUnixNano - 1
		},
		"before surface current": func(projection *surface.ModelToolInjectionLineageCurrentProjectionV1) {
			projection.CheckedUnixNano = projection.Surface.CheckedUnixNano - 1
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := newSQLiteLineageFixtureV1(t)
			projection := mustLineageProjectionV1(t, fixture)
			mutate(&projection)
			projection.ProjectionDigest, _ = projection.ComputeDigest()
			if err := projection.Validate(); err == nil {
				t.Fatal("Projection accepted Checked time before an authoritative source")
			}
			if err := projection.ValidateCurrent(fixture.material.Ref, fixture.clock.Now()); err == nil {
				t.Fatal("ValidateCurrent accepted re-sealed causally early Checked time")
			}
			if err := projection.ValidateAgainst(fixture.request, fixture.clock.Now()); err == nil {
				t.Fatal("ValidateAgainst accepted re-sealed causally early Checked time")
			}
		})
	}
}

func TestModelToolInjectionLineageV1S1S2TTLContextAndTypedNil(t *testing.T) {
	t.Run("closure drift", func(t *testing.T) {
		fixture := newSQLiteLineageFixtureV1(t)
		drift := &driftingClosureReaderV1{base: fixture.store}
		reader, err := surface.NewModelToolInjectionLineageReaderV1(drift, fixture.surfaces, testkit.Owner(), fixture.clock.Now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request); err == nil {
			t.Fatal("S1/S2 closure drift was accepted")
		}
	})
	t.Run("surface drift", func(t *testing.T) {
		fixture := newSQLiteLineageFixtureV1(t)
		drift := &driftingLineageSurfaceReaderV1{base: fixture.surfaces}
		reader, err := surface.NewModelToolInjectionLineageReaderV1(fixture.store, drift, testkit.Owner(), fixture.clock.Now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request); err == nil {
			t.Fatal("S1/S2 Surface drift was accepted")
		}
	})
	t.Run("final expiry crossing", func(t *testing.T) {
		fixture := newSQLiteLineageFixtureV1(t)
		values := []time.Time{
			testkit.FixedTime.Add(time.Second),
			testkit.FixedTime.Add(2 * time.Second),
			time.Unix(0, fixture.material.ExpiresUnixNano),
		}
		var index atomic.Int64
		clock := func() time.Time {
			at := int(index.Add(1) - 1)
			if at >= len(values) {
				return values[len(values)-1]
			}
			return values[at]
		}
		reader, err := surface.NewModelToolInjectionLineageReaderV1(fixture.store, fixture.surfaces, testkit.Owner(), clock)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request); err == nil {
			t.Fatal("final TTL crossing was accepted")
		}
	})
	t.Run("clock rollback", func(t *testing.T) {
		fixture := newSQLiteLineageFixtureV1(t)
		values := []time.Time{testkit.FixedTime.Add(2 * time.Second), testkit.FixedTime.Add(time.Second)}
		var index atomic.Int64
		clock := func() time.Time {
			at := int(index.Add(1) - 1)
			if at >= len(values) {
				return values[len(values)-1]
			}
			return values[at]
		}
		reader, err := surface.NewModelToolInjectionLineageReaderV1(fixture.store, fixture.surfaces, testkit.Owner(), clock)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request); err == nil {
			t.Fatal("clock rollback was accepted")
		}
	})
	t.Run("context", func(t *testing.T) {
		fixture := newSQLiteLineageFixtureV1(t)
		if _, err := fixture.reader.InspectCurrentModelToolInjectionLineageV1(nil, fixture.request); err == nil {
			t.Fatal("nil context was accepted")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := fixture.reader.InspectCurrentModelToolInjectionLineageV1(ctx, fixture.request); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled context was not preserved: %v", err)
		}
	})
	t.Run("typed nil", func(t *testing.T) {
		var closure *driftingClosureReaderV1
		fixture := newSQLiteLineageFixtureV1(t)
		if _, err := surface.NewModelToolInjectionLineageReaderV1(closure, fixture.surfaces, testkit.Owner(), fixture.clock.Now); err == nil {
			t.Fatal("typed-nil closure Reader was accepted")
		}
		var surfaces *driftingLineageSurfaceReaderV1
		if _, err := surface.NewModelToolInjectionLineageReaderV1(fixture.store, surfaces, testkit.Owner(), fixture.clock.Now); err == nil {
			t.Fatal("typed-nil Surface Reader was accepted")
		}
	})
}

func TestModelToolInjectionLineageV1SQLiteRestartLostReplyAndConcurrency(t *testing.T) {
	fixture := newSQLiteLineageFixtureV1(t)
	expected := mustLineageProjectionV1(t, fixture)
	path := fixture.path
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.store = openStoreV1(t, path, fixture.clock)
	t.Cleanup(func() { _ = fixture.store.Close() })
	reader, err := surface.NewModelToolInjectionLineageReaderV1(fixture.store, fixture.surfaces, testkit.Owner(), fixture.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	afterRestart, err := reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request)
	if err != nil || !reflect.DeepEqual(expected, afterRestart) {
		t.Fatalf("SQLite restart changed exact lineage: equal=%v err=%v", reflect.DeepEqual(expected, afterRestart), err)
	}
	lost := &lostReplyClosureReaderV1{base: fixture.store}
	lostReader, err := surface.NewModelToolInjectionLineageReaderV1(lost, fixture.surfaces, testkit.Owner(), fixture.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lostReader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request); err == nil {
		t.Fatal("lost read reply did not surface")
	}
	recovered, err := lostReader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request)
	if err != nil || !reflect.DeepEqual(expected, recovered) {
		t.Fatalf("exact Inspect did not recover lost read reply: equal=%v err=%v", reflect.DeepEqual(expected, recovered), err)
	}

	const workers = 64
	results := make(chan surface.ModelToolInjectionLineageCurrentProjectionV1, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, inspectErr := reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request)
			results <- result
			errs <- inspectErr
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for inspectErr := range errs {
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
	}
	for result := range results {
		if !reflect.DeepEqual(expected, result) {
			t.Fatal("concurrent exact Inspect returned different lineage")
		}
	}
}

type sqliteLineageFixtureV1 struct {
	store    *toolsqlite.StoreV1
	surfaces *surface.InMemoryToolSurfaceManifestCurrentRepositoryV1
	current  toolcontract.ToolSurfaceManifestCurrentProjectionV1
	compiled surface.CompiledModelToolsV1
	material toolcontract.ModelToolInjectionMaterialV1
	request  surface.ModelToolInjectionLineageInspectRequestV1
	reader   *surface.ModelToolInjectionLineageReaderV1
	clock    *testkit.ManualClock
	path     string
}

func newSQLiteLineageFixtureV1(t *testing.T) *sqliteLineageFixtureV1 {
	t.Helper()
	clock := testkit.NewManualClock(testkit.FixedTime.Add(3 * time.Second))
	path := t.TempDir() + "/tool-lineage.sqlite"
	store := openStoreV1(t, path, clock)
	t.Cleanup(func() { _ = store.Close() })
	compileFixture := sqliteCompileFixtureV1(t, clock)
	compiled, material, err := store.CompileAndEnsureModelToolInjectionMaterialV1(
		context.Background(), compileFixture.Current.Ref,
		compileFixture.Surfaces, compileFixture.Definitions, compileFixture.Currents,
	)
	if err != nil {
		t.Fatal(err)
	}
	call := lineageRouteCallV1(compiled.Tools)
	reader, err := surface.NewModelToolInjectionLineageReaderV1(
		store, compileFixture.Surfaces, testkit.Owner(), clock.Now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return &sqliteLineageFixtureV1{
		store: store, surfaces: compileFixture.Surfaces, current: compileFixture.Current,
		compiled: compiled, material: material,
		request: surface.ModelToolInjectionLineageInspectRequestV1{
			Material: material.Ref, RouteCall: call,
		},
		reader: reader, clock: clock, path: path,
	}
}

func lineageRouteCallV1(tools []modelinvoker.Tool) modelinvoker.RouteCall {
	parallel := false
	return modelinvoker.RouteCall{
		RouteID: "openai.direct.payg.responses",
		Invocation: upstream.InvocationContext{
			Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService,
			Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground,
		},
		Request: modelinvoker.Request{
			Model:             "gpt-5.5",
			Input:             []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "invoke tool")},
			Tools:             cloneModelToolsV1(tools),
			ToolChoice:        modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired},
			ParallelToolCalls: &parallel,
			Budget:            modelinvoker.Budget{MaxOutputTokens: 128, Timeout: time.Minute},
		},
	}
}

func cloneModelToolsV1(tools []modelinvoker.Tool) []modelinvoker.Tool {
	result := make([]modelinvoker.Tool, len(tools))
	for index, tool := range tools {
		result[index] = cloneModelToolV1(tool)
	}
	return result
}

func cloneModelToolV1(tool modelinvoker.Tool) modelinvoker.Tool {
	tool.Parameters = append(json.RawMessage(nil), tool.Parameters...)
	if tool.Strict != nil {
		strict := *tool.Strict
		tool.Strict = &strict
	}
	return tool
}

func mustLineageProjectionV1(t *testing.T, fixture *sqliteLineageFixtureV1) surface.ModelToolInjectionLineageCurrentProjectionV1 {
	t.Helper()
	projection, err := fixture.reader.InspectCurrentModelToolInjectionLineageV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func inspectLineageBeforeProviderV1(
	ctx context.Context,
	reader surface.ModelToolInjectionLineageCurrentReaderV1,
	request surface.ModelToolInjectionLineageInspectRequestV1,
	provider func(),
) (surface.ModelToolInjectionLineageCurrentProjectionV1, error) {
	projection, err := reader.InspectCurrentModelToolInjectionLineageV1(ctx, request)
	if err != nil {
		return surface.ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	if err = projection.ValidateAgainst(request, time.Unix(0, projection.CheckedUnixNano)); err != nil {
		return surface.ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	provider()
	return projection, nil
}

func validateLineageBeforeProviderV1(
	projection surface.ModelToolInjectionLineageCurrentProjectionV1,
	request surface.ModelToolInjectionLineageInspectRequestV1,
	now time.Time,
	provider func(),
) error {
	if err := projection.ValidateAgainst(request, now); err != nil {
		return err
	}
	provider()
	return nil
}

type driftingClosureReaderV1 struct {
	base  surface.ModelToolInjectionExactClosureReaderV1
	calls atomic.Int64
}

func (r *driftingClosureReaderV1) InspectExactModelToolInjectionClosureV1(
	ctx context.Context,
	ref toolcontract.ModelToolInjectionMaterialRefV1,
) (surface.CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
	compiled, material, err := r.base.InspectExactModelToolInjectionClosureV1(ctx, ref)
	if err == nil && r.calls.Add(1) == 2 {
		material.Entries[0].ModelName += "-drift"
	}
	return compiled, material, err
}

type snapshotBarrierClosureReaderV1 struct {
	base    surface.ModelToolInjectionExactClosureReaderV1
	entered chan struct{}
	release chan struct{}
	once    atomic.Bool
}

func (r *snapshotBarrierClosureReaderV1) InspectExactModelToolInjectionClosureV1(
	ctx context.Context,
	ref toolcontract.ModelToolInjectionMaterialRefV1,
) (surface.CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
	if !r.once.Swap(true) {
		close(r.entered)
		select {
		case <-ctx.Done():
			return surface.CompiledModelToolsV1{}, toolcontract.ModelToolInjectionMaterialV1{}, ctx.Err()
		case <-r.release:
		}
	}
	return r.base.InspectExactModelToolInjectionClosureV1(ctx, ref)
}

type driftingLineageSurfaceReaderV1 struct {
	base  toolcontract.ToolSurfaceManifestCurrentReaderV1
	calls atomic.Int64
}

func (r *driftingLineageSurfaceReaderV1) InspectExactToolSurfaceManifestCurrentV1(
	ctx context.Context,
	ref toolcontract.ToolSurfaceManifestCurrentRefV1,
) (toolcontract.ToolSurfaceManifestCurrentProjectionV1, error) {
	projection, err := r.base.InspectExactToolSurfaceManifestCurrentV1(ctx, ref)
	if err == nil && r.calls.Add(1) == 2 {
		projection.CheckedUnixNano++
	}
	return projection, err
}

type lostReplyClosureReaderV1 struct {
	base surface.ModelToolInjectionExactClosureReaderV1
	once atomic.Bool
}

func (r *lostReplyClosureReaderV1) InspectExactModelToolInjectionClosureV1(
	ctx context.Context,
	ref toolcontract.ModelToolInjectionMaterialRefV1,
) (surface.CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
	compiled, material, err := r.base.InspectExactModelToolInjectionClosureV1(ctx, ref)
	if err == nil && !r.once.Swap(true) {
		return surface.CompiledModelToolsV1{}, toolcontract.ModelToolInjectionMaterialV1{},
			core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "simulated lost read reply")
	}
	return compiled, material, err
}
