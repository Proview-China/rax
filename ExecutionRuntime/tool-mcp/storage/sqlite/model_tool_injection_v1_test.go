package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
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

func TestSQLiteModelToolInjectionMaterialV1AdvancingClockConcurrentConverges(t *testing.T) {
	path := t.TempDir() + "/tool-owner.db"
	sourceClock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
	fixture := sqliteCompileFixtureV1(t, sourceClock)
	var ticks atomic.Int64
	clock := func() time.Time {
		return testkit.FixedTime.Add(time.Second + time.Duration(ticks.Add(1))*time.Nanosecond)
	}
	store, err := toolsqlite.OpenV1(context.Background(), toolsqlite.ConfigV1{
		Path: path, Clock: clock, Owner: testkit.Owner(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const workers = 64
	var wg sync.WaitGroup
	values := make(chan toolcontract.ModelToolInjectionMaterialV1, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, material, err := store.CompileAndEnsureModelToolInjectionMaterialV1(
				context.Background(), fixture.Current.Ref, fixture.Surfaces, fixture.Definitions, fixture.Currents,
			)
			if err == nil {
				values <- material
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(values)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var winner toolcontract.ModelToolInjectionMaterialV1
	for material := range values {
		if winner.Ref == (toolcontract.ModelToolInjectionMaterialRefV1{}) {
			winner = material
		} else if !reflect.DeepEqual(winner, material) {
			t.Fatal("advancing-clock concurrent compile did not converge to one exact Material row")
		}
	}
	if winner.Ref == (toolcontract.ModelToolInjectionMaterialRefV1{}) {
		t.Fatal("advancing-clock concurrent compile produced no winner")
	}
}

func TestSQLiteModelToolInjectionMaterialV1RefreshingCurrentsConvergeAcrossStores(t *testing.T) {
	path := t.TempDir() + "/tool-owner.db"
	sourceClock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
	fixture := sqliteCompileFixtureV1(t, sourceClock)
	clock := &advancingModelToolClockV1{base: testkit.FixedTime.Add(2 * time.Second)}
	currents := &refreshingRegistryCurrentReaderV1{base: fixture.Currents, clock: clock.Now}
	first, err := toolsqlite.OpenV1(context.Background(), toolsqlite.ConfigV1{
		Path: path, Clock: clock.Now, Owner: testkit.Owner(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := toolsqlite.OpenV1(context.Background(), toolsqlite.ConfigV1{
		Path: path, Clock: clock.Now, Owner: testkit.Owner(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	const workers = 64
	var wg sync.WaitGroup
	values := make(chan toolcontract.ModelToolInjectionMaterialV1, workers)
	errs := make(chan error, workers)
	for index := range workers {
		wg.Add(1)
		go func(store *toolsqlite.StoreV1) {
			defer wg.Done()
			_, material, err := store.CompileAndEnsureModelToolInjectionMaterialV1(
				context.Background(), fixture.Current.Ref, fixture.Surfaces, fixture.Definitions, currents,
			)
			if err == nil {
				values <- material
			}
			errs <- err
		}([]*toolsqlite.StoreV1{first, second}[index%2])
	}
	wg.Wait()
	close(values)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var winner toolcontract.ModelToolInjectionMaterialV1
	for material := range values {
		if winner.Ref == (toolcontract.ModelToolInjectionMaterialRefV1{}) {
			winner = material
		} else if !reflect.DeepEqual(winner, material) {
			t.Fatal("refreshing Current projections changed immutable Material identity")
		}
	}
	if winner.CreatedUnixNano != fixture.Current.Manifest.CreatedUnixNano ||
		winner.ExpiresUnixNano != fixture.Current.Manifest.ExpiresUnixNano {
		t.Fatalf("Material lifetime did not bind exact Surface Manifest: created=%d expires=%d", winner.CreatedUnixNano, winner.ExpiresUnixNano)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var rows int
	if err = db.QueryRow(`SELECT COUNT(*) FROM model_tool_injection_material_v1`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("refreshing Current projections produced %d Material rows, want 1", rows)
	}
}

func TestSQLiteModelToolInjectionMaterialV1ExpiredDynamicCurrentFailsClosed(t *testing.T) {
	path := t.TempDir() + "/tool-owner.db"
	sourceClock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
	fixture := sqliteCompileFixtureV1(t, sourceClock)
	clock := &advancingModelToolClockV1{base: testkit.FixedTime.Add(2 * time.Second)}
	currents := &refreshingRegistryCurrentReaderV1{base: fixture.Currents, clock: clock.Now, expired: true}
	store, err := toolsqlite.OpenV1(context.Background(), toolsqlite.ConfigV1{
		Path: path, Clock: clock.Now, Owner: testkit.Owner(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, _, err = store.CompileAndEnsureModelToolInjectionMaterialV1(
		context.Background(), fixture.Current.Ref, fixture.Surfaces, fixture.Definitions, currents,
	); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("expired dynamic Registry Current did not fail closed: %v", err)
	}
}

func TestSQLiteModelToolInjectionMaterialV1PhysicalSchemaFailClosed(t *testing.T) {
	const columns = `
    material_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    surface_id TEXT NOT NULL,
    surface_revision INTEGER NOT NULL,
    surface_digest TEXT NOT NULL,
    compiled_tools_digest TEXT NOT NULL,
    expires_unix_nano INTEGER NOT NULL,
    compiled_tools_json BLOB NOT NULL,
    body_json BLOB NOT NULL,
    row_digest TEXT NOT NULL`
	const exactTable = `CREATE TABLE model_tool_injection_material_v1 (` + columns + `,
    UNIQUE(material_id, revision, digest)
) STRICT`
	for _, test := range []struct {
		name       string
		statements []string
	}{
		{
			name:       "weak same-name table",
			statements: []string{`CREATE TABLE model_tool_injection_material_v1 (material_id TEXT)`},
		},
		{
			name: "missing primary key",
			statements: []string{`CREATE TABLE model_tool_injection_material_v1 (
    material_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    surface_id TEXT NOT NULL,
    surface_revision INTEGER NOT NULL,
    surface_digest TEXT NOT NULL,
    compiled_tools_digest TEXT NOT NULL,
    expires_unix_nano INTEGER NOT NULL,
    compiled_tools_json BLOB NOT NULL,
    body_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    UNIQUE(material_id, revision, digest)
) STRICT`},
		},
		{
			name:       "missing unique closure index",
			statements: []string{`CREATE TABLE model_tool_injection_material_v1 (` + columns + `) STRICT`},
		},
		{
			name:       "missing strict",
			statements: []string{`CREATE TABLE model_tool_injection_material_v1 (` + columns + `, UNIQUE(material_id, revision, digest))`},
		},
		{
			name: "comment cannot fake schema constraint",
			statements: []string{`CREATE TABLE model_tool_injection_material_v1 (
    material_id TEXT PRIMARY KEY /* CHECK(material_id <> '') */,
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    surface_id TEXT NOT NULL,
    surface_revision INTEGER NOT NULL,
    surface_digest TEXT NOT NULL,
    compiled_tools_digest TEXT NOT NULL,
    expires_unix_nano INTEGER NOT NULL,
    compiled_tools_json BLOB NOT NULL,
    body_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    UNIQUE(material_id, revision, digest)
) STRICT`},
		},
		{
			name:       "extra index",
			statements: []string{exactTable, `CREATE INDEX unexpected_material_surface_v1 ON model_tool_injection_material_v1(surface_id)`},
		},
		{
			name: "extra trigger",
			statements: []string{
				exactTable,
				`CREATE TRIGGER unexpected_material_insert_v1 AFTER INSERT ON model_tool_injection_material_v1 BEGIN SELECT 1; END`,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir() + "/tool-owner.db"
			createRawSQLiteSchemaV1(t, path, test.statements...)
			store, err := toolsqlite.OpenV1(context.Background(), toolsqlite.ConfigV1{
				Path: path, Clock: testkit.NewManualClock(testkit.FixedTime.Add(time.Second)).Now, Owner: testkit.Owner(),
			})
			if store != nil {
				_ = store.Close()
			}
			if err == nil || !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("physical schema drift was accepted: %v", err)
			}
		})
	}
}

func TestSQLiteModelToolInjectionMaterialV1SchemaProbeRollsBackAndRepeatedOpenPreservesData(t *testing.T) {
	path := t.TempDir() + "/tool-owner.db"
	clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
	store := openStoreV1(t, path, clock)
	compiled, material := compileAndPersistV1(t, store, clock)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		store = openStoreV1(t, path, clock)
		got, err := store.InspectExactModelToolInjectionMaterialV1(context.Background(), material.Ref)
		if err != nil || !reflect.DeepEqual(got, material) {
			t.Fatalf("repeated Open or rollback probe changed legal data: equal=%v err=%v", reflect.DeepEqual(got, material), err)
		}
		var count int
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRow(`SELECT count(*) FROM model_tool_injection_material_v1`).Scan(&count); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
		_ = db.Close()
		if count != 1 {
			t.Fatalf("schema probe leaked a row beside the legal closure: %d", count)
		}
		if err = store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if compiled.Digest == "" {
		t.Fatal("legal compiled closure fixture was empty")
	}
}

func TestSQLiteModelToolInjectionMaterialV1SpliceFailClosed(t *testing.T) {
	for _, test := range []struct {
		name          string
		update        string
		reorderColumn string
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
		{name: "compiled body equivalent whitespace", update: `UPDATE model_tool_injection_material_v1 SET compiled_tools_json=CAST(x'20'||compiled_tools_json AS BLOB)`},
		{name: "material body", update: `UPDATE model_tool_injection_material_v1 SET body_json=x'7b7d'`},
		{name: "material body equivalent whitespace", update: `UPDATE model_tool_injection_material_v1 SET body_json=CAST(x'20'||body_json AS BLOB)`},
		{name: "row digest", update: `UPDATE model_tool_injection_material_v1 SET row_digest='sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd'`},
		{name: "compiled body equivalent key reorder", reorderColumn: "compiled_tools_json"},
		{name: "material body equivalent key reorder", reorderColumn: "body_json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir() + "/tool-owner.db"
			clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
			store := openStoreV1(t, path, clock)
			_, material := compileAndPersistV1(t, store, clock)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			if test.reorderColumn != "" {
				tamperSQLiteJSONReorderV1(t, path, "model_tool_injection_material_v1", test.reorderColumn)
			} else {
				tamperSQLiteV1(t, path, test.update)
			}
			store, err := toolsqlite.OpenV1(context.Background(), toolsqlite.ConfigV1{
				Path: path, Clock: clock.Now, Owner: testkit.Owner(),
			})
			if err != nil {
				return
			}
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
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	store := openStoreV1(t, path, clock)
	binding, ack, err := store.EnsureToolSurfaceInvocationBindingV1(context.Background(), request)
	if err != nil {
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
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	store := openStoreV1(t, path, clock)
	defer store.Close()

	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	type closure struct {
		binding toolcontract.ToolSurfaceInvocationBindingV1
		ack     toolcontract.ToolSurfaceInvocationBindingAckV1
	}
	values := make(chan closure, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			winner, winnerAck, err := store.EnsureToolSurfaceInvocationBindingV1(context.Background(), request)
			if err == nil {
				values <- closure{binding: winner, ack: winnerAck}
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
	var expected closure
	for value := range values {
		if expected.binding.Ref == (toolcontract.ToolSurfaceInvocationBindingRefV1{}) {
			expected = value
		} else if !reflect.DeepEqual(expected, value) {
			t.Fatal("64 concurrent Binding requests did not converge")
		}
	}
	binding, ack := expected.binding, expected.ack
	exact, exactAck, err := store.InspectToolSurfaceInvocationBindingByInvocationV1(context.Background(), binding.Subject.Invocation)
	if err != nil || !reflect.DeepEqual(exact, binding) || !reflect.DeepEqual(exactAck, ack) {
		t.Fatalf("64 concurrent Binding calls did not converge through the invocation secondary index: binding=%v ack=%v err=%v", reflect.DeepEqual(exact, binding), reflect.DeepEqual(exactAck, ack), err)
	}
}

func TestSQLiteToolSurfaceInvocationBindingV1SameInvocationDifferentRequestConflicts(t *testing.T) {
	path := t.TempDir() + "/tool-owner.db"
	clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
	store := openStoreV1(t, path, clock)
	defer store.Close()
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	if _, _, err := store.EnsureToolSurfaceInvocationBindingV1(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	different := request
	different.RequestedNotAfterUnixNano--
	if _, _, err := store.EnsureToolSurfaceInvocationBindingV1(context.Background(), different); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same invocation with another request payload did not fail closed: %v", err)
	}
}

func TestSQLiteToolSurfaceInvocationBindingV1SpliceFailClosed(t *testing.T) {
	for _, test := range []struct {
		name          string
		update        string
		reorderColumn string
	}{
		{name: "revision", update: `UPDATE tool_surface_invocation_binding_v1 SET revision=2`},
		{name: "digest", update: `UPDATE tool_surface_invocation_binding_v1 SET digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'`},
		{name: "invocation id", update: `UPDATE tool_surface_invocation_binding_v1 SET invocation_id='spliced-invocation'`},
		{name: "invocation digest", update: `UPDATE tool_surface_invocation_binding_v1 SET invocation_digest='sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'`},
		{name: "expiry", update: `UPDATE tool_surface_invocation_binding_v1 SET expires_unix_nano=expires_unix_nano+1`},
		{name: "request digest", update: `UPDATE tool_surface_invocation_binding_v1 SET request_digest='sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd'`},
		{name: "request body", update: `UPDATE tool_surface_invocation_binding_v1 SET request_json=x'7b7d'`},
		{name: "request body equivalent whitespace", update: `UPDATE tool_surface_invocation_binding_v1 SET request_json=CAST(x'20'||request_json AS BLOB)`},
		{name: "binding body", update: `UPDATE tool_surface_invocation_binding_v1 SET binding_json=x'7b7d'`},
		{name: "binding body equivalent whitespace", update: `UPDATE tool_surface_invocation_binding_v1 SET binding_json=CAST(x'20'||binding_json AS BLOB)`},
		{name: "ack body", update: `UPDATE tool_surface_invocation_binding_v1 SET ack_json=x'7b7d'`},
		{name: "ack body equivalent whitespace", update: `UPDATE tool_surface_invocation_binding_v1 SET ack_json=CAST(x'20'||ack_json AS BLOB)`},
		{name: "row digest", update: `UPDATE tool_surface_invocation_binding_v1 SET row_digest='sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc'`},
		{name: "request body equivalent key reorder", reorderColumn: "request_json"},
		{name: "binding body equivalent key reorder", reorderColumn: "binding_json"},
		{name: "ack body equivalent key reorder", reorderColumn: "ack_json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir() + "/tool-owner.db"
			clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
			store := openStoreV1(t, path, clock)
			binding, _, err := store.EnsureToolSurfaceInvocationBindingV1(context.Background(), testkit.ToolSurfaceInvocationBindingRequestV1())
			if err != nil {
				t.Fatal(err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			if test.reorderColumn != "" {
				tamperSQLiteJSONReorderV1(t, path, "tool_surface_invocation_binding_v1", test.reorderColumn)
			} else {
				tamperSQLiteV1(t, path, test.update)
			}
			store = openStoreV1(t, path, clock)
			defer store.Close()
			if _, _, err = store.InspectExactToolSurfaceInvocationBindingV1(context.Background(), binding.Ref); err == nil {
				t.Fatalf("durable Binding %s splice was accepted", test.name)
			}
		})
	}
}

type advancingModelToolClockV1 struct {
	base  time.Time
	ticks atomic.Int64
}

func (c *advancingModelToolClockV1) Now() time.Time {
	return c.base.Add(time.Duration(c.ticks.Add(1)) * time.Nanosecond)
}

type refreshingRegistryCurrentReaderV1 struct {
	base    toolcontract.ToolRegistryObjectCurrentReaderV1
	clock   func() time.Time
	expired bool
}

func (r *refreshingRegistryCurrentReaderV1) ResolveExactToolCapabilityCurrentV1(ctx context.Context, object toolcontract.ObjectRef) (toolcontract.CapabilityDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	value, current, err := r.base.ResolveExactToolCapabilityCurrentV1(ctx, object)
	current, err = r.refresh(current, err)
	return value, current, err
}

func (r *refreshingRegistryCurrentReaderV1) InspectExactToolCapabilityCurrentV1(ctx context.Context, object toolcontract.ObjectRef, expected toolcontract.ToolRegistryObjectCurrentRefV1) (toolcontract.CapabilityDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	value, current, err := r.base.InspectExactToolCapabilityCurrentV1(ctx, object, expected)
	current, err = r.refresh(current, err)
	return value, current, err
}

func (r *refreshingRegistryCurrentReaderV1) ResolveExactToolDescriptorCurrentV1(ctx context.Context, object toolcontract.ObjectRef) (toolcontract.ToolDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	value, current, err := r.base.ResolveExactToolDescriptorCurrentV1(ctx, object)
	current, err = r.refresh(current, err)
	return value, current, err
}

func (r *refreshingRegistryCurrentReaderV1) InspectExactToolDescriptorCurrentV1(ctx context.Context, object toolcontract.ObjectRef, expected toolcontract.ToolRegistryObjectCurrentRefV1) (toolcontract.ToolDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	value, current, err := r.base.InspectExactToolDescriptorCurrentV1(ctx, object, expected)
	current, err = r.refresh(current, err)
	return value, current, err
}

func (r *refreshingRegistryCurrentReaderV1) refresh(current toolcontract.ToolRegistryObjectCurrentProjectionV1, readErr error) (toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	if readErr != nil {
		return toolcontract.ToolRegistryObjectCurrentProjectionV1{}, readErr
	}
	now := r.clock()
	current.CheckedUnixNano = now.UnixNano()
	current.ExpiresUnixNano = now.Add(toolcontract.MaxToolRegistryObjectCurrentTTLV1).UnixNano()
	if r.expired {
		current.CheckedUnixNano = now.Add(-2 * time.Second).UnixNano()
		current.ExpiresUnixNano = now.Add(-time.Second).UnixNano()
	}
	current.ProjectionDigest = ""
	return toolcontract.SealToolRegistryObjectCurrentProjectionV1(current)
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
	store, err := toolsqlite.OpenV1(context.Background(), toolsqlite.ConfigV1{Path: path, Clock: clock.Now, Owner: testkit.Owner()})
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

func createRawSQLiteSchemaV1(t *testing.T, path string, statements ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range statements {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func tamperSQLiteJSONReorderV1(t *testing.T, path, table, column string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var body []byte
	if err = db.QueryRow(fmt.Sprintf("SELECT %s FROM %s", column, table)).Scan(&body); err != nil {
		t.Fatal(err)
	}
	var value map[string]json.RawMessage
	if err = json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	reordered, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(reordered) == string(body) {
		t.Fatalf("%s.%s fixture was already map-key ordered", table, column)
	}
	if _, err = db.Exec(fmt.Sprintf("UPDATE %s SET %s=?", table, column), reordered); err != nil {
		t.Fatal(err)
	}
}
