package sqlite_test

import (
	"context"
	"database/sql"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/owner/surfacebinding"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	toolsqlite "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestSQLiteModelToolInjectionMaterialV1RestartExactConcurrentAndExpiry(t *testing.T) {
	path := t.TempDir() + "/tool-owner.db"
	clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
	store := openStoreV1(t, path, clock)
	fixture := sqliteCompileFixtureV1(t, clock)
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	type closure struct {
		compiled surface.CompiledModelToolsV1
		material toolcontract.ModelToolInjectionMaterialV1
	}
	values := make(chan closure, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			compiled, material, err := store.CompileAndEnsureModelToolInjectionMaterialV1(
				context.Background(), fixture.Current.Ref, fixture.Surfaces, fixture.Definitions, fixture.Currents,
			)
			if err == nil {
				values <- closure{compiled: compiled, material: material}
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	close(values)
	var winner closure
	for value := range values {
		if winner.material.Ref == (toolcontract.ModelToolInjectionMaterialRefV1{}) {
			winner = value
			continue
		}
		if !reflect.DeepEqual(winner, value) {
			t.Fatal("64 concurrent compile/persist calls did not converge on one exact closure")
		}
	}
	if winner.material.Ref == (toolcontract.ModelToolInjectionMaterialRefV1{}) {
		t.Fatal("64 concurrent compile/persist calls produced no winner")
	}
	if winner.material.ExpiresUnixNano != fixture.ExpectedExpiresUnixNano {
		t.Fatalf("material expiry did not preserve the natural minimum current bound: got=%d want=%d", winner.material.ExpiresUnixNano, fixture.ExpectedExpiresUnixNano)
	}
	if err := store.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStoreV1(t, path, clock)
	defer store.Close()
	restarted, err := store.InspectExactModelToolInjectionMaterialV1(context.Background(), winner.material.Ref)
	if err != nil || !reflect.DeepEqual(restarted, winner.material) {
		t.Fatalf("restart exact read drifted: equal=%v err=%v", reflect.DeepEqual(restarted, winner.material), err)
	}
	restarted.Entries[0].EffectKinds[0] = "praxis.tool/tampered"
	again, err := store.InspectExactModelToolInjectionMaterialV1(context.Background(), winner.material.Ref)
	if err != nil || again.Entries[0].EffectKinds[0] != winner.material.Entries[0].EffectKinds[0] {
		t.Fatalf("SQLite exact read aliased caller memory: %#v err=%v", again.Entries, err)
	}
	clock.Set(time.Unix(0, winner.material.ExpiresUnixNano))
	if _, err := store.InspectExactModelToolInjectionMaterialV1(context.Background(), winner.material.Ref); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("now==expires was not fail-closed: %v", err)
	}
}

func TestSQLiteModelToolInjectionMaterialV1SpliceFailClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		update string
	}{
		{name: "material id", update: `UPDATE model_tool_injection_material_v1 SET material_id='spliced-material-id'`},
		{name: "revision", update: `UPDATE model_tool_injection_material_v1 SET revision=2`},
		{name: "digest", update: `UPDATE model_tool_injection_material_v1 SET digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'`},
		{name: "surface id", update: `UPDATE model_tool_injection_material_v1 SET surface_id='spliced-surface-id'`},
		{name: "surface revision", update: `UPDATE model_tool_injection_material_v1 SET surface_revision=surface_revision+1`},
		{name: "surface digest", update: `UPDATE model_tool_injection_material_v1 SET surface_digest='sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'`},
		{name: "compiled digest", update: `UPDATE model_tool_injection_material_v1 SET compiled_tools_digest='sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc'`},
		{name: "ttl", update: `UPDATE model_tool_injection_material_v1 SET expires_unix_nano=expires_unix_nano+1`},
		{name: "compiled body", update: `UPDATE model_tool_injection_material_v1 SET compiled_tools_json=x'7b7d'`},
		{name: "material body", update: `UPDATE model_tool_injection_material_v1 SET body_json=x'7b7d'`},
		{name: "row digest", update: `UPDATE model_tool_injection_material_v1 SET row_digest='sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd'`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir() + "/tool-owner.db"
			clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
			store := openStoreV1(t, path, clock)
			_, material := compileAndPersistV1(t, store, clock)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			tamperSQLiteV1(t, path, test.update)
			store = openStoreV1(t, path, clock)
			defer store.Close()
			if _, err := store.InspectExactModelToolInjectionMaterialV1(context.Background(), material.Ref); err == nil {
				t.Fatalf("%s splice was accepted: %v", test.name, err)
			}
		})
	}
	t.Run("exact ref", func(t *testing.T) {
		path := t.TempDir() + "/tool-owner.db"
		clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
		store := openStoreV1(t, path, clock)
		defer store.Close()
		_, material := compileAndPersistV1(t, store, clock)
		spliced := material.Ref
		spliced.Digest = testkit.Digest("spliced-material")
		if _, err := store.InspectExactModelToolInjectionMaterialV1(context.Background(), spliced); err == nil || !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("exact Material Ref splice was accepted: %v", err)
		}
	})
}

func TestSQLiteToolSurfaceInvocationBindingV1DurableExactReader(t *testing.T) {
	path := t.TempDir() + "/tool-owner.db"
	clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
	reference, err := surfacebinding.NewInMemoryRepositoryV1(testkit.Owner(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	binding, ack, err := reference.EnsureToolSurfaceInvocationBindingV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	store := openStoreV1(t, path, clock)
	if _, _, err = store.EnsureExactToolSurfaceInvocationBindingV1(context.Background(), binding, ack); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStoreV1(t, path, clock)
	defer store.Close()
	exact, exactAck, err := store.InspectExactToolSurfaceInvocationBindingV1(context.Background(), binding.Ref)
	if err != nil || !reflect.DeepEqual(exact, binding) || !reflect.DeepEqual(exactAck, ack) {
		t.Fatalf("durable exact Binding read drifted: binding=%v ack=%v err=%v", reflect.DeepEqual(exact, binding), reflect.DeepEqual(exactAck, ack), err)
	}
	byInvocation, byInvocationAck, err := store.InspectToolSurfaceInvocationBindingByInvocationV1(context.Background(), binding.Subject.Invocation)
	if err != nil || byInvocation.Ref != binding.Ref || byInvocationAck.Ref != ack.Ref {
		t.Fatalf("durable invocation index drifted: binding=%#v ack=%#v err=%v", byInvocation.Ref, byInvocationAck.Ref, err)
	}
	clock.Set(time.Unix(0, binding.NotAfterUnixNano))
	if _, _, err = store.InspectExactToolSurfaceInvocationBindingV1(context.Background(), binding.Ref); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("Binding exact read accepted now==expires: %v", err)
	}
	if _, _, err = store.InspectToolSurfaceInvocationBindingByInvocationV1(context.Background(), binding.Subject.Invocation); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("Binding secondary-index read accepted now==expires: %v", err)
	}
}

func TestSQLiteToolSurfaceInvocationBindingV1ConcurrentSameBinding(t *testing.T) {
	path := t.TempDir() + "/tool-owner.db"
	clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
	reference, err := surfacebinding.NewInMemoryRepositoryV1(testkit.Owner(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	binding, ack, err := reference.EnsureToolSurfaceInvocationBindingV1(context.Background(), testkit.ToolSurfaceInvocationBindingRequestV1())
	if err != nil {
		t.Fatal(err)
	}
	store := openStoreV1(t, path, clock)
	defer store.Close()

	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			winner, winnerAck, err := store.EnsureExactToolSurfaceInvocationBindingV1(context.Background(), binding, ack)
			if err == nil && (!reflect.DeepEqual(winner, binding) || !reflect.DeepEqual(winnerAck, ack)) {
				err = core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "concurrent Binding winner drifted")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	exact, exactAck, err := store.InspectToolSurfaceInvocationBindingByInvocationV1(context.Background(), binding.Subject.Invocation)
	if err != nil || !reflect.DeepEqual(exact, binding) || !reflect.DeepEqual(exactAck, ack) {
		t.Fatalf("64 concurrent Binding calls did not converge through the invocation secondary index: binding=%v ack=%v err=%v", reflect.DeepEqual(exact, binding), reflect.DeepEqual(exactAck, ack), err)
	}
}

func TestSQLiteToolSurfaceInvocationBindingV1SpliceFailClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		update string
	}{
		{name: "revision", update: `UPDATE tool_surface_invocation_binding_v1 SET revision=2`},
		{name: "digest", update: `UPDATE tool_surface_invocation_binding_v1 SET digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'`},
		{name: "invocation id", update: `UPDATE tool_surface_invocation_binding_v1 SET invocation_id='spliced-invocation'`},
		{name: "invocation digest", update: `UPDATE tool_surface_invocation_binding_v1 SET invocation_digest='sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'`},
		{name: "expiry", update: `UPDATE tool_surface_invocation_binding_v1 SET expires_unix_nano=expires_unix_nano+1`},
		{name: "binding body", update: `UPDATE tool_surface_invocation_binding_v1 SET binding_json=x'7b7d'`},
		{name: "ack body", update: `UPDATE tool_surface_invocation_binding_v1 SET ack_json=x'7b7d'`},
		{name: "row digest", update: `UPDATE tool_surface_invocation_binding_v1 SET row_digest='sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc'`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir() + "/tool-owner.db"
			clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
			reference, err := surfacebinding.NewInMemoryRepositoryV1(testkit.Owner(), clock.Now)
			if err != nil {
				t.Fatal(err)
			}
			binding, ack, err := reference.EnsureToolSurfaceInvocationBindingV1(context.Background(), testkit.ToolSurfaceInvocationBindingRequestV1())
			if err != nil {
				t.Fatal(err)
			}
			store := openStoreV1(t, path, clock)
			if _, _, err = store.EnsureExactToolSurfaceInvocationBindingV1(context.Background(), binding, ack); err != nil {
				t.Fatal(err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			tamperSQLiteV1(t, path, test.update)
			store = openStoreV1(t, path, clock)
			defer store.Close()
			if _, _, err = store.InspectExactToolSurfaceInvocationBindingV1(context.Background(), binding.Ref); err == nil {
				t.Fatalf("durable Binding %s splice was accepted", test.name)
			}
		})
	}
}

func sqliteMaterialV1(t *testing.T) toolcontract.ModelToolInjectionMaterialV1 {
	t.Helper()
	capability, tool := testkit.Capability(), testkit.Tool()
	schema := []byte(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`)
	input := tool.InputSchema
	input.ContentDigest = core.DigestBytes(schema)
	description := core.DigestBytes([]byte("Example tool"))
	toolRef := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	definition, err := toolcontract.DeriveToolDefinitionMaterialRefV1(toolRef, input, description)
	if err != nil {
		t.Fatal(err)
	}
	capabilityRef := toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	entry := toolcontract.ModelToolInjectionEntryV1{
		ModelName: "example", CapabilityRef: capabilityRef, ToolRef: toolRef,
		DefinitionMaterialRef: definition, InputSchemaRef: input, DescriptionDigest: description,
		Strict: true, Admission: toolcontract.AdmissionRequired,
		EffectKinds:   append([]runtimeports.NamespacedNameV2(nil), capability.EffectKinds...),
		ReviewProfile: capability.ReviewProfile, AuthorityRequirement: capability.AuthorityRequirement,
		BudgetRequirement: capability.BudgetRequirement, SandboxRequirement: capability.SandboxRequirement,
		EvidenceRequirement: capability.EvidenceRequirement,
	}
	expected, err := toolcontract.ComputeExpectedInjectionDigest([]toolcontract.ToolSurfaceEntry{{
		Capability: capabilityRef, Tool: toolRef, ModelName: entry.ModelName, InputSchema: input,
		DescriptionDigest: description, Visibility: toolcontract.SurfaceVisible, Allowed: true,
		Admission: entry.Admission, MechanismDigest: tool.Digest, EffectKinds: entry.EffectKinds,
	}})
	if err != nil {
		t.Fatal(err)
	}
	material, err := toolcontract.SealModelToolInjectionMaterialV1(toolcontract.ModelToolInjectionMaterialV1{
		Surface: toolcontract.ToolSurfaceManifestCurrentRefV1{
			ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1,
			ID:              "surface-sqlite-material-v1", Revision: 1, Digest: testkit.Digest("surface-sqlite-material"),
		},
		Entries:                 []toolcontract.ModelToolInjectionEntryV1{entry},
		ExpectedInjectionDigest: expected, CompiledToolsDigest: testkit.Digest("compiled-tools"),
		CreatedUnixNano: testkit.FixedTime.UnixNano(), ExpiresUnixNano: testkit.FixedTime.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return material
}

func openStoreV1(t *testing.T, path string, clock *testkit.ManualClock) *toolsqlite.StoreV1 {
	t.Helper()
	store, err := toolsqlite.OpenV1(context.Background(), toolsqlite.ConfigV1{Path: path, Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func tamperSQLiteV1(t *testing.T, path, statement string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(statement); err != nil {
		t.Fatal(err)
	}
}
