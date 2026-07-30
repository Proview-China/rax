package modelinvokeradapter_test

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/modelinvokeradapter"
	toolsurface "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestInvocationMaterialToolPairV2MapsAuthoritativePairLosslessly(t *testing.T) {
	fixture := newToolPairFixtureV2(t)
	closures := &scriptedClosureReaderV2{values: []closureValueV2{fixture.closure}}
	surfaces := &scriptedSurfaceReaderV2{values: []toolcontract.ToolSurfaceManifestCurrentProjectionV1{fixture.current}}
	pairs := newAuthoritativePairReaderV2(t, fixture, closures, surfaces, fixedPairClockV2(fixture.now))
	adapter := newModelToolPairAdapterV2(t, pairs, fixture.tools, fixedPairClockV2(fixture.now))

	projection, err := adapter.InspectExactInvocationToolPairV2(
		context.Background(),
		fixture.modelMaterial,
		fixture.modelSurface,
		fixture.requestToolsDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	if closures.callCount() != 2 || surfaces.callCount() != 2 {
		t.Fatalf("authoritative S1/S2 reads=%d/%d want=2/2", closures.callCount(), surfaces.callCount())
	}
	if projection.ToolInjectionMaterial != fixture.modelMaterial ||
		projection.ToolSurface != fixture.modelSurface ||
		projection.ExpectedInjectionDigest != fixture.material.ExpectedInjectionDigest ||
		projection.CompiledToolsDigest != fixture.compiled.Digest ||
		projection.RequestToolsDigest != fixture.requestToolsDigest ||
		projection.CheckedUnixNano != fixture.now.UnixNano() ||
		projection.ExpiresUnixNano != fixture.material.ExpiresUnixNano {
		t.Fatalf("authoritative Tool pair was not mapped losslessly: %+v", projection)
	}
	if err := projection.ValidateCurrentV2(
		fixture.modelMaterial,
		fixture.modelSurface,
		fixture.material.ExpectedInjectionDigest,
		fixture.compiled.Digest,
		fixture.requestToolsDigest,
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
}

func TestInvocationMaterialToolPairV2DoesNotTrustCallerPassedDigest(t *testing.T) {
	fixture := newToolPairFixtureV2(t)
	pairs := &countingPairReaderV2{projection: fixture.toolPair}
	adapter := newModelToolPairAdapterV2(t, pairs, fixture.tools, fixedPairClockV2(fixture.now))

	projection, err := adapter.InspectExactInvocationToolPairV2(
		context.Background(),
		fixture.modelMaterial,
		fixture.modelSurface,
		testkit.Digest("caller-selected-digest"),
	)
	if err == nil ||
		projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) ||
		pairs.calls.Load() != 0 {
		t.Fatalf("caller digest reached Tool owner: projection=%+v calls=%d err=%v", projection, pairs.calls.Load(), err)
	}
}

func TestInvocationMaterialToolPairV2ActualRequestToolAxesFailClosed(t *testing.T) {
	fixture := newToolPairFixtureV2(t)
	tests := map[string]func([]modelinvoker.Tool) []modelinvoker.Tool{
		"add": func(tools []modelinvoker.Tool) []modelinvoker.Tool {
			added := cloneToolsV2(tools)
			extra := cloneToolsV2(tools[:1])[0]
			extra.Name = "charlie_alias"
			return append(added, extra)
		},
		"remove": func(tools []modelinvoker.Tool) []modelinvoker.Tool {
			return cloneToolsV2(tools[:1])
		},
		"reorder": func(tools []modelinvoker.Tool) []modelinvoker.Tool {
			reordered := cloneToolsV2(tools)
			reordered[0], reordered[1] = reordered[1], reordered[0]
			return reordered
		},
		"schema": func(tools []modelinvoker.Tool) []modelinvoker.Tool {
			changed := cloneToolsV2(tools)
			changed[0].Parameters = json.RawMessage(`{"type":"object","properties":{"changed":{"type":"string"}},"required":["changed"],"additionalProperties":false}`)
			return changed
		},
		"strict": func(tools []modelinvoker.Tool) []modelinvoker.Tool {
			changed := cloneToolsV2(tools)
			strict := false
			changed[0].Strict = &strict
			return changed
		},
		"alias": func(tools []modelinvoker.Tool) []modelinvoker.Tool {
			changed := cloneToolsV2(tools)
			changed[0].Name = "alpha_spliced_alias"
			return changed
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			closures := &scriptedClosureReaderV2{values: []closureValueV2{fixture.closure}}
			surfaces := &scriptedSurfaceReaderV2{values: []toolcontract.ToolSurfaceManifestCurrentProjectionV1{fixture.current}}
			pairs := newAuthoritativePairReaderV2(t, fixture, closures, surfaces, fixedPairClockV2(fixture.now))
			tools := mutate(fixture.tools)
			adapter, constructorErr := modelinvokeradapter.NewInvocationMaterialToolPairExactReaderV2(
				pairs,
				tools,
				fixedPairClockV2(fixture.now),
			)
			if constructorErr != nil {
				if name != "strict" {
					t.Fatalf("%s unexpectedly failed before authoritative comparison: %v", name, constructorErr)
				}
				return
			}
			digest, digestErr := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(tools)
			if digestErr != nil {
				t.Fatal(digestErr)
			}
			projection, inspectErr := adapter.InspectExactInvocationToolPairV2(
				context.Background(),
				fixture.modelMaterial,
				fixture.modelSurface,
				digest,
			)
			if inspectErr == nil || projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) {
				t.Fatalf("%s splice was accepted: projection=%+v err=%v", name, projection, inspectErr)
			}
		})
	}
}

func TestInvocationMaterialToolPairV2StoredCompiledVersusActualToolsMismatch(t *testing.T) {
	fixture := newToolPairFixtureV2(t)
	actual := cloneToolsV2(fixture.tools)
	actual[0].Description = "caller actual bytes differ from Tool stored compilation"
	closures := &scriptedClosureReaderV2{values: []closureValueV2{fixture.closure}}
	surfaces := &scriptedSurfaceReaderV2{values: []toolcontract.ToolSurfaceManifestCurrentProjectionV1{fixture.current}}
	pairs := newAuthoritativePairReaderV2(t, fixture, closures, surfaces, fixedPairClockV2(fixture.now))
	adapter := newModelToolPairAdapterV2(t, pairs, actual, fixedPairClockV2(fixture.now))
	digest, err := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(actual)
	if err != nil {
		t.Fatal(err)
	}
	projection, inspectErr := adapter.InspectExactInvocationToolPairV2(
		context.Background(), fixture.modelMaterial, fixture.modelSurface, digest,
	)
	if inspectErr == nil || projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) {
		t.Fatalf("stored/actual mismatch accepted: projection=%+v err=%v", projection, inspectErr)
	}
}

func TestInvocationMaterialToolPairV2SourceSpliceMatrix(t *testing.T) {
	fixture := newToolPairFixtureV2(t)
	tests := map[string]func(*modelinvoker.InvocationMaterialExactSourceRefV1, *modelinvoker.InvocationMaterialExactSourceRefV1){
		"material owner": func(material, _ *modelinvoker.InvocationMaterialExactSourceRefV1) {
			material.Owner.ID = "spliced-owner"
		},
		"surface owner": func(_, surface *modelinvoker.InvocationMaterialExactSourceRefV1) {
			surface.Owner.ID = "spliced-owner"
		},
		"material kind": func(material, _ *modelinvoker.InvocationMaterialExactSourceRefV1) {
			material.Kind = string(toolcontract.ToolSurfaceManifestCurrentSourceKindV1)
		},
		"surface kind": func(_, surface *modelinvoker.InvocationMaterialExactSourceRefV1) {
			surface.Kind = string(toolcontract.ModelToolInjectionMaterialSourceKindV1)
		},
		"material id": func(material, _ *modelinvoker.InvocationMaterialExactSourceRefV1) {
			material.ID += "-splice"
		},
		"surface id": func(_, surface *modelinvoker.InvocationMaterialExactSourceRefV1) {
			surface.ID += "-splice"
		},
		"material revision": func(material, _ *modelinvoker.InvocationMaterialExactSourceRefV1) {
			material.Revision++
		},
		"surface revision": func(_, surface *modelinvoker.InvocationMaterialExactSourceRefV1) {
			surface.Revision++
		},
		"material digest": func(material, _ *modelinvoker.InvocationMaterialExactSourceRefV1) {
			material.Digest = testkit.Digest("material-splice")
		},
		"surface digest": func(_, surface *modelinvoker.InvocationMaterialExactSourceRefV1) {
			surface.Digest = testkit.Digest("surface-splice")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			pairs := &countingPairReaderV2{projection: fixture.toolPair}
			adapter := newModelToolPairAdapterV2(t, pairs, fixture.tools, fixedPairClockV2(fixture.now))
			material, surface := fixture.modelMaterial, fixture.modelSurface
			mutate(&material, &surface)
			projection, err := adapter.InspectExactInvocationToolPairV2(
				context.Background(), material, surface, fixture.requestToolsDigest,
			)
			if err == nil || projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) {
				t.Fatalf("%s source splice accepted: projection=%+v err=%v", name, projection, err)
			}
		})
	}
}

func TestInvocationMaterialToolPairV2AuthoritativeProjectionSplicesFailClosed(t *testing.T) {
	fixture := newToolPairFixtureV2(t)
	tests := map[string]func(*toolcontract.ModelToolInjectionPairCurrentProjectionV2){
		"material owner": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.MaterialSource.Owner.ID = "splice"
		},
		"surface owner": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.SurfaceSource.Owner.ID = "splice"
		},
		"material kind": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.MaterialSource.Kind = toolcontract.ToolSurfaceManifestCurrentSourceKindV1
		},
		"surface kind": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.SurfaceSource.Kind = toolcontract.ModelToolInjectionMaterialSourceKindV1
		},
		"material ref": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.MaterialSource.ID += "-splice"
		},
		"surface ref": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.SurfaceSource.Revision++
		},
		"expected digest": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.ExpectedInjectionDigest = testkit.Digest("expected-splice")
		},
		"stored compiled": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.StoredCompiledToolsDigest = testkit.Digest("stored-splice")
		},
		"actual compiled": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.ActualCompiledToolsDigest = testkit.Digest("actual-splice")
		},
		"request tools": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.RequestToolsDigest = testkit.Digest("request-splice")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			spliced := fixture.toolPair
			mutate(&spliced)
			pairs := &countingPairReaderV2{projection: spliced}
			adapter := newModelToolPairAdapterV2(t, pairs, fixture.tools, fixedPairClockV2(fixture.now))
			projection, err := adapter.InspectExactInvocationToolPairV2(
				context.Background(),
				fixture.modelMaterial,
				fixture.modelSurface,
				fixture.requestToolsDigest,
			)
			if err == nil || projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) {
				t.Fatalf("%s authoritative splice accepted: projection=%+v err=%v", name, projection, err)
			}
		})
	}
}

func TestInvocationMaterialToolPairV2RejectsResealedSourceExpectedRoleSubstitution(t *testing.T) {
	fixture := newToolPairFixtureV2(t)
	tests := map[string]func(*toolcontract.ModelToolInjectionPairCurrentProjectionV2){
		"expected as material source": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.ExpectedInjectionDigest = p.MaterialSource.Digest
		},
		"expected as surface source": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.ExpectedInjectionDigest = p.SurfaceSource.Digest
		},
		"material as surface source": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.MaterialSource.Digest = p.SurfaceSource.Digest
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			assertResealAndAdapterRejectV2(t, fixture, mutate)
		})
	}
}

func TestInvocationMaterialToolPairV2RejectsResealedCompiledRequestRoleSubstitution(t *testing.T) {
	fixture := newToolPairFixtureV2(t)
	tests := map[string]func(*toolcontract.ModelToolInjectionPairCurrentProjectionV2){
		"compiled as material source": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.StoredCompiledToolsDigest = p.MaterialSource.Digest
			p.ActualCompiledToolsDigest = p.MaterialSource.Digest
		},
		"compiled as surface source": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.StoredCompiledToolsDigest = p.SurfaceSource.Digest
			p.ActualCompiledToolsDigest = p.SurfaceSource.Digest
		},
		"compiled as expected": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.StoredCompiledToolsDigest = p.ExpectedInjectionDigest
			p.ActualCompiledToolsDigest = p.ExpectedInjectionDigest
		},
		"request as material source": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.RequestToolsDigest = p.MaterialSource.Digest
		},
		"request as surface source": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.RequestToolsDigest = p.SurfaceSource.Digest
		},
		"request as expected": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.RequestToolsDigest = p.ExpectedInjectionDigest
		},
		"request as compiled": func(p *toolcontract.ModelToolInjectionPairCurrentProjectionV2) {
			p.RequestToolsDigest = p.StoredCompiledToolsDigest
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			assertResealAndAdapterRejectV2(t, fixture, mutate)
		})
	}
}

func TestInvocationMaterialToolPairV2S1S2DriftFailClosed(t *testing.T) {
	fixture := newToolPairFixtureV2(t)
	s2 := fixture.current
	s2.CheckedUnixNano++
	s2.ProjectionDigest = ""
	var err error
	s2, err = toolcontract.SealToolSurfaceManifestCurrentV1(s2)
	if err != nil {
		t.Fatal(err)
	}
	closures := &scriptedClosureReaderV2{values: []closureValueV2{fixture.closure}}
	surfaces := &scriptedSurfaceReaderV2{
		values: []toolcontract.ToolSurfaceManifestCurrentProjectionV1{fixture.current, s2},
	}
	pairs := newAuthoritativePairReaderV2(t, fixture, closures, surfaces, fixedPairClockV2(fixture.now))
	adapter := newModelToolPairAdapterV2(t, pairs, fixture.tools, fixedPairClockV2(fixture.now))
	projection, inspectErr := adapter.InspectExactInvocationToolPairV2(
		context.Background(),
		fixture.modelMaterial,
		fixture.modelSurface,
		fixture.requestToolsDigest,
	)
	if inspectErr == nil ||
		projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) ||
		closures.callCount() != 2 ||
		surfaces.callCount() != 2 {
		t.Fatalf("S1/S2 drift accepted: projection=%+v closure=%d surface=%d err=%v", projection, closures.callCount(), surfaces.callCount(), inspectErr)
	}
}

func TestInvocationMaterialToolPairV2ClockTTLTypedNilAndCancellation(t *testing.T) {
	fixture := newToolPairFixtureV2(t)
	t.Run("clock rollback", func(t *testing.T) {
		pairs := newAuthoritativePairReaderV2(
			t,
			fixture,
			&scriptedClosureReaderV2{values: []closureValueV2{fixture.closure}},
			&scriptedSurfaceReaderV2{values: []toolcontract.ToolSurfaceManifestCurrentProjectionV1{fixture.current}},
			scriptedPairClockV2(fixture.now, fixture.now.Add(-time.Nanosecond)),
		)
		adapter := newModelToolPairAdapterV2(t, pairs, fixture.tools, fixedPairClockV2(fixture.now))
		projection, err := adapter.InspectExactInvocationToolPairV2(
			context.Background(), fixture.modelMaterial, fixture.modelSurface, fixture.requestToolsDigest,
		)
		if err == nil || projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) {
			t.Fatalf("clock rollback accepted: projection=%+v err=%v", projection, err)
		}
	})
	t.Run("now equals expiry", func(t *testing.T) {
		expiry := time.Unix(0, fixture.material.ExpiresUnixNano)
		pairs := newAuthoritativePairReaderV2(
			t,
			fixture,
			&scriptedClosureReaderV2{values: []closureValueV2{fixture.closure}},
			&scriptedSurfaceReaderV2{values: []toolcontract.ToolSurfaceManifestCurrentProjectionV1{fixture.current}},
			scriptedPairClockV2(fixture.now, expiry),
		)
		adapter := newModelToolPairAdapterV2(t, pairs, fixture.tools, fixedPairClockV2(expiry))
		projection, err := adapter.InspectExactInvocationToolPairV2(
			context.Background(), fixture.modelMaterial, fixture.modelSurface, fixture.requestToolsDigest,
		)
		if err == nil || projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) {
			t.Fatalf("now==expiry accepted: projection=%+v err=%v", projection, err)
		}
	})
	t.Run("adapter TTL", func(t *testing.T) {
		pairs := &countingPairReaderV2{projection: fixture.toolPair}
		expiry := time.Unix(0, fixture.toolPair.ExpiresUnixNano)
		adapter := newModelToolPairAdapterV2(t, pairs, fixture.tools, fixedPairClockV2(expiry))
		projection, err := adapter.InspectExactInvocationToolPairV2(
			context.Background(), fixture.modelMaterial, fixture.modelSurface, fixture.requestToolsDigest,
		)
		if err == nil || projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) {
			t.Fatalf("adapter now==expiry accepted: projection=%+v err=%v", projection, err)
		}
	})
	t.Run("typed nil", func(t *testing.T) {
		var pairNil *countingPairReaderV2
		if adapter, err := modelinvokeradapter.NewInvocationMaterialToolPairExactReaderV2(
			pairNil,
			fixture.tools,
			fixedPairClockV2(fixture.now),
		); err == nil || adapter != nil {
			t.Fatalf("typed nil pair reader accepted: adapter=%v err=%v", adapter, err)
		}
		var closureNil *scriptedClosureReaderV2
		if reader, err := toolsurface.NewAuthoritativeModelToolInjectionPairCurrentReaderV2(
			closureNil,
			&scriptedSurfaceReaderV2{values: []toolcontract.ToolSurfaceManifestCurrentProjectionV1{fixture.current}},
			fixture.owner,
			fixedPairClockV2(fixture.now),
		); err == nil || reader != nil {
			t.Fatalf("typed nil closure reader accepted: reader=%v err=%v", reader, err)
		}
		var adapterNil *modelinvokeradapter.InvocationMaterialToolPairExactReaderV2
		projection, err := adapterNil.InspectExactInvocationToolPairV2(
			context.Background(), fixture.modelMaterial, fixture.modelSurface, fixture.requestToolsDigest,
		)
		if err == nil || projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) {
			t.Fatalf("typed nil adapter accepted: projection=%+v err=%v", projection, err)
		}
	})
	t.Run("pre-cancel", func(t *testing.T) {
		pairs := &countingPairReaderV2{projection: fixture.toolPair}
		adapter := newModelToolPairAdapterV2(t, pairs, fixture.tools, fixedPairClockV2(fixture.now))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		projection, err := adapter.InspectExactInvocationToolPairV2(
			ctx, fixture.modelMaterial, fixture.modelSurface, fixture.requestToolsDigest,
		)
		if err != context.Canceled ||
			projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) ||
			pairs.calls.Load() != 0 {
			t.Fatalf("pre-cancel: projection=%+v calls=%d err=%v", projection, pairs.calls.Load(), err)
		}
	})
	t.Run("between S1 and S2", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		closures := &scriptedClosureReaderV2{values: []closureValueV2{fixture.closure}}
		surfaces := &scriptedSurfaceReaderV2{
			values: []toolcontract.ToolSurfaceManifestCurrentProjectionV1{fixture.current},
			after:  func(call int) { cancel() },
		}
		pairs := newAuthoritativePairReaderV2(t, fixture, closures, surfaces, fixedPairClockV2(fixture.now))
		adapter := newModelToolPairAdapterV2(t, pairs, fixture.tools, fixedPairClockV2(fixture.now))
		projection, err := adapter.InspectExactInvocationToolPairV2(
			ctx, fixture.modelMaterial, fixture.modelSurface, fixture.requestToolsDigest,
		)
		if err != context.Canceled ||
			projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) ||
			closures.callCount() != 1 ||
			surfaces.callCount() != 1 {
			t.Fatalf("mid-cancel: projection=%+v closure=%d surface=%d err=%v", projection, closures.callCount(), surfaces.callCount(), err)
		}
	})
}

func TestInvocationMaterialToolPairV2FreezesActualToolsAndSupports64ConcurrentReaders(t *testing.T) {
	fixture := newToolPairFixtureV2(t)
	original := cloneToolsV2(fixture.tools)
	closures := &scriptedClosureReaderV2{values: []closureValueV2{fixture.closure}}
	surfaces := &scriptedSurfaceReaderV2{values: []toolcontract.ToolSurfaceManifestCurrentProjectionV1{fixture.current}}
	pairs := newAuthoritativePairReaderV2(t, fixture, closures, surfaces, fixedPairClockV2(fixture.now))
	adapter := newModelToolPairAdapterV2(t, pairs, original, fixedPairClockV2(fixture.now))

	original[0].Name = "caller_mutated_alias"
	original[0].Description = "caller mutated"
	original[0].Parameters[0] = '['
	*original[0].Strict = false

	var expected modelinvoker.InvocationMaterialToolPairProjectionV2
	var expectedSet bool
	var failures atomic.Int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			projection, err := adapter.InspectExactInvocationToolPairV2(
				context.Background(),
				fixture.modelMaterial,
				fixture.modelSurface,
				fixture.requestToolsDigest,
			)
			if err != nil {
				failures.Add(1)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if !expectedSet {
				expected, expectedSet = projection, true
				return
			}
			if projection != expected {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 || !expectedSet {
		t.Fatalf("64 concurrent reads failures=%d expected_set=%v", failures.Load(), expectedSet)
	}
	if closures.callCount() != 128 || surfaces.callCount() != 128 {
		t.Fatalf("64 concurrent reads did not execute exact S1/S2: closure=%d surface=%d", closures.callCount(), surfaces.callCount())
	}
}

func TestInvocationMaterialToolPairV2ToolChoiceAndExecutionStayOutsideToolPair(t *testing.T) {
	requestType := reflect.TypeOf(toolsurface.ModelToolInjectionPairCurrentInspectRequestV2{})
	fields := make(map[string]bool, requestType.NumField())
	for index := 0; index < requestType.NumField(); index++ {
		fields[requestType.Field(index).Name] = true
	}
	if requestType.NumField() != 3 ||
		!fields["Material"] ||
		!fields["Surface"] ||
		!fields["ActualRequestTools"] ||
		fields["ToolChoice"] {
		t.Fatalf("Tool pair request widened into Model ToolChoice or execution: %v", fields)
	}

	fixture := newToolPairFixtureV2(t)
	connected := &connectedBoundarySpyV2{projection: fixture.toolPair}
	adapter := newModelToolPairAdapterV2(t, connected, fixture.tools, fixedPairClockV2(fixture.now))
	if _, err := adapter.InspectExactInvocationToolPairV2(
		context.Background(),
		fixture.modelMaterial,
		fixture.modelSurface,
		fixture.requestToolsDigest,
	); err != nil {
		t.Fatal(err)
	}
	if connected.pairReads.Load() != 1 ||
		connected.providerCalls.Load() != 0 ||
		connected.toolExecutionCalls.Load() != 0 {
		t.Fatalf(
			"connected production dependency calls: pair=%d Provider=%d Tool=%d",
			connected.pairReads.Load(),
			connected.providerCalls.Load(),
			connected.toolExecutionCalls.Load(),
		)
	}
}

func assertResealAndAdapterRejectV2(
	t *testing.T,
	fixture toolPairFixtureV2,
	mutate func(*toolcontract.ModelToolInjectionPairCurrentProjectionV2),
) {
	t.Helper()
	adversarial := fixture.toolPair
	mutate(&adversarial)
	adversarial.ProjectionDigest = ""
	resealed, err := toolcontract.SealModelToolInjectionPairCurrentV2(adversarial)
	if err == nil || resealed != (toolcontract.ModelToolInjectionPairCurrentProjectionV2{}) {
		t.Fatalf("digest role substitution resealed: projection=%+v err=%v", resealed, err)
	}

	forged := forceAdversarialPairProjectionDigestV2(t, adversarial)
	if err := forged.Validate(); err == nil {
		t.Fatal("role-substituted projection passed Validate with a recomputed canonical digest")
	}
	connected := &connectedBoundarySpyV2{projection: forged}
	adapter := newModelToolPairAdapterV2(
		t,
		connected,
		fixture.tools,
		fixedPairClockV2(fixture.now),
	)
	projection, inspectErr := adapter.InspectExactInvocationToolPairV2(
		context.Background(),
		fixture.modelMaterial,
		fixture.modelSurface,
		fixture.requestToolsDigest,
	)
	if inspectErr == nil ||
		projection != (modelinvoker.InvocationMaterialToolPairProjectionV2{}) ||
		connected.pairReads.Load() != 1 ||
		connected.providerCalls.Load() != 0 ||
		connected.toolExecutionCalls.Load() != 0 {
		t.Fatalf(
			"adapter accepted role substitution: projection=%+v pair=%d Provider=%d Tool=%d err=%v",
			projection,
			connected.pairReads.Load(),
			connected.providerCalls.Load(),
			connected.toolExecutionCalls.Load(),
			inspectErr,
		)
	}
}

func forceAdversarialPairProjectionDigestV2(
	t *testing.T,
	projection toolcontract.ModelToolInjectionPairCurrentProjectionV2,
) toolcontract.ModelToolInjectionPairCurrentProjectionV2 {
	t.Helper()
	projection.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest(
		"praxis.tool-mcp.model-tool-injection-pair-current",
		toolcontract.ModelToolInjectionPairCurrentContractVersionV2,
		"ModelToolInjectionPairCurrentProjectionV2",
		projection,
	)
	if err != nil {
		t.Fatal(err)
	}
	projection.ProjectionDigest = digest
	return projection
}

type toolPairFixtureV2 struct {
	now                time.Time
	owner              core.OwnerRef
	tools              []modelinvoker.Tool
	current            toolcontract.ToolSurfaceManifestCurrentProjectionV1
	compiled           toolsurface.CompiledModelToolsV1
	material           toolcontract.ModelToolInjectionMaterialV1
	closure            closureValueV2
	toolPair           toolcontract.ModelToolInjectionPairCurrentProjectionV2
	modelMaterial      modelinvoker.InvocationMaterialExactSourceRefV1
	modelSurface       modelinvoker.InvocationMaterialExactSourceRefV1
	requestToolsDigest core.Digest
}

func newToolPairFixtureV2(t *testing.T) toolPairFixtureV2 {
	t.Helper()
	now := testkit.FixedTime.Add(10 * time.Minute)
	owner := testkit.Owner()
	strictA, strictB := true, true
	tools := []modelinvoker.Tool{
		{
			Name:        "alpha_alias",
			Description: "alpha description",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`),
			Strict:      &strictA,
		},
		{
			Name:        "beta_alias",
			Description: "beta description",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer"}},"required":["count"],"additionalProperties":false}`),
			Strict:      &strictB,
		},
	}
	capability := testkit.Capability()
	baseTool := testkit.Tool()
	entries := make([]toolcontract.ToolSurfaceEntry, 0, len(tools))
	materialEntries := make([]toolcontract.ModelToolInjectionEntryV1, 0, len(tools))
	for index, tool := range tools {
		capabilityRef := toolcontract.ObjectRef{
			ID:       string(capability.ID) + "-" + tool.Name,
			Revision: capability.Revision,
			Digest:   testkit.Digest("capability-" + tool.Name),
		}
		toolRef := toolcontract.ObjectRef{
			ID:       string(baseTool.ID) + "-" + tool.Name,
			Revision: baseTool.Revision,
			Digest:   testkit.Digest("tool-" + tool.Name),
		}
		schemaRef := baseTool.InputSchema
		schemaRef.Name += "-" + tool.Name
		schemaRef.ContentDigest = core.DigestBytes(tool.Parameters)
		descriptionDigest := core.DigestBytes([]byte(tool.Description))
		definition, err := toolcontract.DeriveToolDefinitionMaterialRefV1(
			toolRef,
			schemaRef,
			descriptionDigest,
		)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, toolcontract.ToolSurfaceEntry{
			Capability:        capabilityRef,
			Tool:              toolRef,
			ModelName:         tool.Name,
			InputSchema:       schemaRef,
			DescriptionDigest: descriptionDigest,
			Order:             uint32(index),
			Visibility:        toolcontract.SurfaceVisible,
			Allowed:           true,
			Admission:         toolcontract.AdmissionRequired,
			MechanismDigest:   toolRef.Digest,
			EffectKinds:       append([]runtimeports.NamespacedNameV2(nil), capability.EffectKinds...),
		})
		materialEntries = append(materialEntries, toolcontract.ModelToolInjectionEntryV1{
			Order:                 uint32(index),
			ModelName:             tool.Name,
			CapabilityRef:         capabilityRef,
			ToolRef:               toolRef,
			DefinitionMaterialRef: definition,
			InputSchemaRef:        schemaRef,
			DescriptionDigest:     descriptionDigest,
			Strict:                true,
			Admission:             toolcontract.AdmissionRequired,
			EffectKinds:           append([]runtimeports.NamespacedNameV2(nil), capability.EffectKinds...),
			ReviewProfile:         capability.ReviewProfile,
			AuthorityRequirement:  capability.AuthorityRequirement,
			BudgetRequirement:     capability.BudgetRequirement,
			SandboxRequirement:    capability.SandboxRequirement,
			EvidenceRequirement:   capability.EvidenceRequirement,
		})
	}
	manifest, err := toolcontract.SealSurface(toolcontract.ToolSurfaceManifest{
		ID:                     "model-tool-pair-surface-v2",
		Revision:               1,
		Owner:                  owner,
		ResolvedPlanDigest:     testkit.Digest("pair-plan"),
		ProfileDigest:          testkit.Digest("pair-profile"),
		CapabilityGrantDigest:  testkit.Digest("pair-grant"),
		RegistrySnapshotDigest: testkit.Digest("pair-registry"),
		Entries:                entries,
		Dialect:                toolcontract.ModelToolDialectFunctionCallingV1,
		CreatedUnixNano:        now.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano:        now.Add(20 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := toolcontract.SealToolSurfaceManifestCurrentV1(
		toolcontract.ToolSurfaceManifestCurrentProjectionV1{
			Manifest:        manifest,
			CheckedUnixNano: now.Add(-time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	compiledDigest, err := toolsurface.ComputeRouteCallCompiledToolsDigestV1(
		current.Ref,
		current.Manifest.Dialect,
		tools,
	)
	if err != nil {
		t.Fatal(err)
	}
	compiled := toolsurface.CompiledModelToolsV1{
		ContractVersion: toolsurface.CompiledModelToolsContractVersionV1,
		Surface:         current.Ref,
		Dialect:         string(current.Manifest.Dialect),
		Tools:           cloneToolsV2(tools),
		Digest:          compiledDigest,
	}
	material, err := toolcontract.SealModelToolInjectionMaterialV1(
		toolcontract.ModelToolInjectionMaterialV1{
			Surface:                 current.Ref,
			Entries:                 materialEntries,
			ExpectedInjectionDigest: current.Manifest.ExpectedInjectionDigest,
			CompiledToolsDigest:     compiled.Digest,
			CreatedUnixNano:         now.Add(-30 * time.Second).UnixNano(),
			ExpiresUnixNano:         now.Add(10 * time.Minute).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := compiled.ValidateAgainstMaterialV1(material); err != nil {
		t.Fatal(err)
	}
	materialSource, err := toolcontract.ModelToolInjectionMaterialSourceRefV1(owner, material.Ref)
	if err != nil {
		t.Fatal(err)
	}
	surfaceSource, err := toolcontract.ToolSurfaceManifestSourceRefV1(current)
	if err != nil {
		t.Fatal(err)
	}
	requestToolsDigest, err := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(tools)
	if err != nil {
		t.Fatal(err)
	}
	toolPair, err := toolcontract.SealModelToolInjectionPairCurrentV2(
		toolcontract.ModelToolInjectionPairCurrentProjectionV2{
			MaterialSource:            materialSource,
			SurfaceSource:             surfaceSource,
			ExpectedInjectionDigest:   material.ExpectedInjectionDigest,
			StoredCompiledToolsDigest: compiled.Digest,
			ActualCompiledToolsDigest: compiled.Digest,
			RequestToolsDigest:        requestToolsDigest,
			CheckedUnixNano:           now.UnixNano(),
			ExpiresUnixNano:           material.ExpiresUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return toolPairFixtureV2{
		now:      now,
		owner:    owner,
		tools:    cloneToolsV2(tools),
		current:  current,
		compiled: compiled,
		material: material,
		closure: closureValueV2{
			compiled: compiled,
			material: material,
		},
		toolPair: toolPair,
		modelMaterial: modelinvoker.InvocationMaterialExactSourceRefV1{
			Owner: owner, Kind: string(materialSource.Kind), ID: materialSource.ID,
			Revision: materialSource.Revision, Digest: materialSource.Digest,
		},
		modelSurface: modelinvoker.InvocationMaterialExactSourceRefV1{
			Owner: owner, Kind: string(surfaceSource.Kind), ID: surfaceSource.ID,
			Revision: surfaceSource.Revision, Digest: surfaceSource.Digest,
		},
		requestToolsDigest: requestToolsDigest,
	}
}

func newAuthoritativePairReaderV2(
	t *testing.T,
	fixture toolPairFixtureV2,
	closures toolsurface.ModelToolInjectionExactClosureReaderV1,
	surfaces toolcontract.ToolSurfaceManifestCurrentReaderV1,
	clock func() time.Time,
) *toolsurface.AuthoritativeModelToolInjectionPairCurrentReaderV2 {
	t.Helper()
	reader, err := toolsurface.NewAuthoritativeModelToolInjectionPairCurrentReaderV2(
		closures,
		surfaces,
		fixture.owner,
		clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func newModelToolPairAdapterV2(
	t *testing.T,
	pairs toolsurface.ModelToolInjectionPairCurrentReaderV2,
	tools []modelinvoker.Tool,
	clock func() time.Time,
) *modelinvokeradapter.InvocationMaterialToolPairExactReaderV2 {
	t.Helper()
	adapter, err := modelinvokeradapter.NewInvocationMaterialToolPairExactReaderV2(
		pairs,
		tools,
		clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

type closureValueV2 struct {
	compiled toolsurface.CompiledModelToolsV1
	material toolcontract.ModelToolInjectionMaterialV1
	err      error
}

type scriptedClosureReaderV2 struct {
	mu     sync.Mutex
	values []closureValueV2
	calls  int
	after  func(int)
}

func (r *scriptedClosureReaderV2) InspectExactModelToolInjectionClosureV1(
	ctx context.Context,
	_ toolcontract.ModelToolInjectionMaterialRefV1,
) (toolsurface.CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
	r.mu.Lock()
	index := r.calls
	r.calls++
	if index >= len(r.values) {
		index = len(r.values) - 1
	}
	value := r.values[index]
	after := r.after
	call := r.calls
	r.mu.Unlock()
	if after != nil {
		after(call)
	}
	if err := ctx.Err(); err != nil {
		return toolsurface.CompiledModelToolsV1{}, toolcontract.ModelToolInjectionMaterialV1{}, err
	}
	return value.compiled.Clone(), value.material.Clone(), value.err
}

func (r *scriptedClosureReaderV2) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type scriptedSurfaceReaderV2 struct {
	mu     sync.Mutex
	values []toolcontract.ToolSurfaceManifestCurrentProjectionV1
	errs   []error
	calls  int
	after  func(int)
}

func (r *scriptedSurfaceReaderV2) InspectExactToolSurfaceManifestCurrentV1(
	ctx context.Context,
	_ toolcontract.ToolSurfaceManifestCurrentRefV1,
) (toolcontract.ToolSurfaceManifestCurrentProjectionV1, error) {
	r.mu.Lock()
	index := r.calls
	r.calls++
	if index >= len(r.values) {
		index = len(r.values) - 1
	}
	value := r.values[index]
	var err error
	if len(r.errs) > 0 {
		errIndex := index
		if errIndex >= len(r.errs) {
			errIndex = len(r.errs) - 1
		}
		err = r.errs[errIndex]
	}
	after := r.after
	call := r.calls
	r.mu.Unlock()
	if contextErr := ctx.Err(); contextErr != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, contextErr
	}
	if after != nil {
		after(call)
	}
	return value, err
}

func (r *scriptedSurfaceReaderV2) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type countingPairReaderV2 struct {
	projection toolcontract.ModelToolInjectionPairCurrentProjectionV2
	err        error
	calls      atomic.Int64
}

func (r *countingPairReaderV2) InspectCurrentModelToolInjectionPairV2(
	context.Context,
	toolsurface.ModelToolInjectionPairCurrentInspectRequestV2,
) (toolcontract.ModelToolInjectionPairCurrentProjectionV2, error) {
	r.calls.Add(1)
	return r.projection, r.err
}

// connectedBoundarySpyV2 is the object actually injected into the production
// adapter port. Its extra methods model forbidden connected execution
// surfaces; the adapter's narrow interface cannot reach either method.
type connectedBoundarySpyV2 struct {
	projection         toolcontract.ModelToolInjectionPairCurrentProjectionV2
	err                error
	pairReads          atomic.Int64
	providerCalls      atomic.Int64
	toolExecutionCalls atomic.Int64
}

func (s *connectedBoundarySpyV2) InspectCurrentModelToolInjectionPairV2(
	context.Context,
	toolsurface.ModelToolInjectionPairCurrentInspectRequestV2,
) (toolcontract.ModelToolInjectionPairCurrentProjectionV2, error) {
	s.pairReads.Add(1)
	return s.projection, s.err
}

func (s *connectedBoundarySpyV2) InvokeProviderV2() {
	s.providerCalls.Add(1)
}

func (s *connectedBoundarySpyV2) ExecuteToolV2() {
	s.toolExecutionCalls.Add(1)
}

func cloneToolsV2(tools []modelinvoker.Tool) []modelinvoker.Tool {
	result := append([]modelinvoker.Tool(nil), tools...)
	for index := range result {
		result[index].Parameters = append(json.RawMessage(nil), result[index].Parameters...)
		if result[index].Strict != nil {
			strict := *result[index].Strict
			result[index].Strict = &strict
		}
	}
	return result
}

func fixedPairClockV2(now time.Time) func() time.Time {
	return func() time.Time { return now }
}

func scriptedPairClockV2(values ...time.Time) func() time.Time {
	var mu sync.Mutex
	index := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}
