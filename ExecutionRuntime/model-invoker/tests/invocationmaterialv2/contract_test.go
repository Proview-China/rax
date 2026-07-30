package invocationmaterialv2_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	modelsqlite "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var fixtureNow = time.Unix(1_800_000_000, 0)

const legacySQLiteSchemaV1 = `
CREATE TABLE IF NOT EXISTS model_invoker_schema (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS governed_model_invocation_history (
  invocation_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  attempt_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(invocation_id, revision)
);
CREATE TABLE IF NOT EXISTS governed_model_invocation_current (
  invocation_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  FOREIGN KEY(invocation_id, revision)
    REFERENCES governed_model_invocation_history(invocation_id, revision)
);
CREATE TABLE IF NOT EXISTS governed_model_invocation_attempt_guard (
  attempt_digest TEXT PRIMARY KEY,
  invocation_id TEXT NOT NULL UNIQUE,
  FOREIGN KEY(invocation_id)
    REFERENCES governed_model_invocation_current(invocation_id)
);
CREATE INDEX IF NOT EXISTS governed_model_invocation_current_exact
  ON governed_model_invocation_current(invocation_id, revision, fact_digest, highest_revision);
CREATE INDEX IF NOT EXISTS governed_model_invocation_history_exact
  ON governed_model_invocation_history(invocation_id, revision, fact_digest, attempt_digest);
`

const legacySQLiteSchemaV2 = `
CREATE TABLE IF NOT EXISTS prepared_model_invocation_history (
  prepared_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(prepared_id, revision)
);
CREATE TABLE IF NOT EXISTS prepared_model_invocation_current (
  current_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  current_digest TEXT NOT NULL,
  prepared_id TEXT NOT NULL,
  prepared_revision INTEGER NOT NULL CHECK(prepared_revision > 0),
  prepared_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  FOREIGN KEY(prepared_id, prepared_revision)
    REFERENCES prepared_model_invocation_history(prepared_id, revision)
);
CREATE TABLE IF NOT EXISTS invocation_material_history (
  material_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  material_digest TEXT NOT NULL,
  route_call_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(material_id, revision)
);
CREATE TABLE IF NOT EXISTS governed_model_turn_history (
  turn_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  attempt_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(turn_id, revision)
);
CREATE TABLE IF NOT EXISTS governed_model_turn_current (
  turn_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  FOREIGN KEY(turn_id, revision)
    REFERENCES governed_model_turn_history(turn_id, revision)
);
CREATE TABLE IF NOT EXISTS governed_model_turn_attempt_guard (
  attempt_digest TEXT PRIMARY KEY,
  turn_id TEXT NOT NULL UNIQUE,
  FOREIGN KEY(turn_id) REFERENCES governed_model_turn_current(turn_id)
);
CREATE TABLE IF NOT EXISTS governed_model_turn_tool_call_projection (
  turn_id TEXT PRIMARY KEY,
  turn_revision INTEGER NOT NULL CHECK(turn_revision > 0),
  projection_id TEXT NOT NULL UNIQUE,
  projection_revision INTEGER NOT NULL CHECK(projection_revision > 0),
  projection_digest TEXT NOT NULL,
  observation_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  FOREIGN KEY(turn_id, turn_revision)
    REFERENCES governed_model_turn_history(turn_id, revision)
);
CREATE INDEX IF NOT EXISTS prepared_model_invocation_history_exact
  ON prepared_model_invocation_history(prepared_id, revision, fact_digest);
CREATE INDEX IF NOT EXISTS prepared_model_invocation_current_exact
  ON prepared_model_invocation_current(current_id, revision, current_digest, prepared_id, prepared_revision, prepared_digest);
CREATE INDEX IF NOT EXISTS invocation_material_history_exact
  ON invocation_material_history(material_id, revision, material_digest, route_call_digest);
CREATE INDEX IF NOT EXISTS governed_model_turn_history_exact
  ON governed_model_turn_history(turn_id, revision, fact_digest, attempt_digest);
CREATE INDEX IF NOT EXISTS governed_model_turn_current_exact
  ON governed_model_turn_current(turn_id, revision, fact_digest, highest_revision);
CREATE INDEX IF NOT EXISTS governed_model_turn_projection_exact
  ON governed_model_turn_tool_call_projection(projection_id, projection_revision, projection_digest, observation_digest);
`

type fixtureV2 struct {
	call       modelinvoker.RouteCall
	prepared   modelinvoker.PreparedModelInvocationFactV1
	current    modelinvoker.PreparedModelInvocationCurrentProjectionV1
	closure    modelinvoker.InvocationMaterialExactClosureV2
	authorizer *modelinvoker.InvocationMaterialAuthorizerV2
}

type exactReadersV2 struct {
	expectedInjection core.Digest
	compiledTools     core.Digest
	checkedUnixNano   int64
	expiresUnixNano   int64
}

type driftingReadersV2 struct {
	base          exactReadersV2
	mu            sync.Mutex
	drift         string
	driftAt       int
	contextCalls  int
	toolCalls     int
	providerCalls int
	routeCalls    int
	profileCalls  int
}

type countingRepositoryV2 struct {
	mu           sync.Mutex
	ensureCalls  int
	inspectCalls int
}

type typedNilReadersV2 struct{}

type typedNilRepositoryV2 struct{}

type lostReplyRepositoryV2 struct {
	inner        *modelsqlite.Store
	mu           sync.Mutex
	ensureCalls  int
	inspectCalls int
	persisted    modelinvoker.InvocationMaterialRefV2
	inspected    modelinvoker.InvocationMaterialRefV2
}

func (r exactReadersV2) InspectExactInvocationContextPairV2(
	_ context.Context,
	frame modelinvoker.InvocationMaterialExactSourceRefV1,
	material modelinvoker.InvocationMaterialExactSourceRefV1,
	mappedInput core.Digest,
) (modelinvoker.InvocationMaterialContextPairProjectionV2, error) {
	return modelinvoker.SealInvocationMaterialContextPairProjectionV2(
		modelinvoker.InvocationMaterialContextPairProjectionV2{
			ContextFrame:             frame,
			ContextMaterial:          material,
			ContextMappedInputDigest: mappedInput,
			CheckedUnixNano:          r.checkedUnixNano,
			ExpiresUnixNano:          r.expiresUnixNano,
		},
	)
}

func (r exactReadersV2) InspectExactInvocationToolPairV2(
	_ context.Context,
	injection modelinvoker.InvocationMaterialExactSourceRefV1,
	surface modelinvoker.InvocationMaterialExactSourceRefV1,
	requestTools core.Digest,
) (modelinvoker.InvocationMaterialToolPairProjectionV2, error) {
	return modelinvoker.SealInvocationMaterialToolPairProjectionV2(
		modelinvoker.InvocationMaterialToolPairProjectionV2{
			ToolInjectionMaterial:   injection,
			ToolSurface:             surface,
			ExpectedInjectionDigest: r.expectedInjection,
			CompiledToolsDigest:     r.compiledTools,
			RequestToolsDigest:      requestTools,
			CheckedUnixNano:         r.checkedUnixNano,
			ExpiresUnixNano:         r.expiresUnixNano,
		},
	)
}

func (r exactReadersV2) projection(
	ref modelinvoker.InvocationMaterialExactSourceRefV1,
) modelinvoker.InvocationMaterialExactSourceProjectionV1 {
	return modelinvoker.InvocationMaterialExactSourceProjectionV1{
		Ref:             ref,
		CheckedUnixNano: r.checkedUnixNano,
		ExpiresUnixNano: r.expiresUnixNano,
	}
}

func (r exactReadersV2) InspectExactInvocationProviderInjectionV1(
	_ context.Context,
	ref modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	return r.projection(ref), nil
}

func (r exactReadersV2) InspectExactInvocationRouteV1(
	_ context.Context,
	ref modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	return r.projection(ref), nil
}

func (r exactReadersV2) InspectExactInvocationProfileV1(
	_ context.Context,
	ref modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	return r.projection(ref), nil
}

func (r *driftingReadersV2) InspectExactInvocationContextPairV2(
	ctx context.Context,
	frame modelinvoker.InvocationMaterialExactSourceRefV1,
	material modelinvoker.InvocationMaterialExactSourceRefV1,
	mappedInput core.Digest,
) (modelinvoker.InvocationMaterialContextPairProjectionV2, error) {
	r.mu.Lock()
	r.contextCalls++
	call := r.contextCalls
	r.mu.Unlock()
	projection, err := r.base.InspectExactInvocationContextPairV2(ctx, frame, material, mappedInput)
	if err != nil || r.drift != "context" || call != 2 {
		return projection, err
	}
	projection.ContextFrame.ID += "-drift"
	projection.ProjectionDigest = ""
	return modelinvoker.SealInvocationMaterialContextPairProjectionV2(projection)
}

func (r *driftingReadersV2) InspectExactInvocationToolPairV2(
	ctx context.Context,
	injection modelinvoker.InvocationMaterialExactSourceRefV1,
	surface modelinvoker.InvocationMaterialExactSourceRefV1,
	requestTools core.Digest,
) (modelinvoker.InvocationMaterialToolPairProjectionV2, error) {
	r.mu.Lock()
	r.toolCalls++
	call := r.toolCalls
	r.mu.Unlock()
	projection, err := r.base.InspectExactInvocationToolPairV2(ctx, injection, surface, requestTools)
	if err != nil || r.drift != "tool" || call != 2 {
		return projection, err
	}
	projection.ToolSurface.ID += "-drift"
	projection.ProjectionDigest = ""
	return modelinvoker.SealInvocationMaterialToolPairProjectionV2(projection)
}

func (r *driftingReadersV2) InspectExactInvocationProviderInjectionV1(
	ctx context.Context,
	ref modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	r.mu.Lock()
	r.providerCalls++
	call := r.providerCalls
	r.mu.Unlock()
	projection, err := r.base.InspectExactInvocationProviderInjectionV1(ctx, ref)
	if err == nil && r.drift == "provider" && call == r.sourceDriftCall() {
		projection.Ref.ID += "-drift"
	}
	return projection, err
}

func (r *driftingReadersV2) InspectExactInvocationRouteV1(
	ctx context.Context,
	ref modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	r.mu.Lock()
	r.routeCalls++
	call := r.routeCalls
	r.mu.Unlock()
	projection, err := r.base.InspectExactInvocationRouteV1(ctx, ref)
	if err == nil && r.drift == "route" && call == r.sourceDriftCall() {
		projection.Ref.ID += "-drift"
	}
	return projection, err
}

func (r *driftingReadersV2) InspectExactInvocationProfileV1(
	ctx context.Context,
	ref modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	r.mu.Lock()
	r.profileCalls++
	call := r.profileCalls
	r.mu.Unlock()
	projection, err := r.base.InspectExactInvocationProfileV1(ctx, ref)
	if err == nil && r.drift == "profile" && call == r.sourceDriftCall() {
		projection.Ref.ID += "-drift"
	}
	return projection, err
}

func (r *driftingReadersV2) callCounts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.contextCalls, r.toolCalls
}

func (r *driftingReadersV2) sourceCallCounts() (int, int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.providerCalls, r.routeCalls, r.profileCalls
}

func (r *driftingReadersV2) sourceDriftCall() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.driftAt > 0 {
		return r.driftAt
	}
	return 2
}

func (r *countingRepositoryV2) EnsureAuthorizedInvocationMaterialV2(
	_ context.Context,
	_ modelinvoker.InvocationMaterialPersistRequestV2,
) (modelinvoker.InvocationMaterialV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureCalls++
	return modelinvoker.InvocationMaterialV2{}, &modelinvoker.GovernedModelInvocationErrorV1{
		Kind:      modelinvoker.GovernedModelInvocationErrorConflict,
		Operation: "unexpected_ensure",
		Message:   "repository must not be called",
	}
}

func (r *countingRepositoryV2) InspectExactInvocationMaterialV2(
	_ context.Context,
	_ modelinvoker.InvocationMaterialRefV2,
) (modelinvoker.InvocationMaterialV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inspectCalls++
	return modelinvoker.InvocationMaterialV2{}, &modelinvoker.GovernedModelInvocationErrorV1{
		Kind:      modelinvoker.GovernedModelInvocationErrorConflict,
		Operation: "unexpected_inspect",
		Message:   "repository must not be called",
	}
}

func (r *countingRepositoryV2) calls() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ensureCalls, r.inspectCalls
}

func (*typedNilReadersV2) InspectExactInvocationContextPairV2(
	context.Context,
	modelinvoker.InvocationMaterialExactSourceRefV1,
	modelinvoker.InvocationMaterialExactSourceRefV1,
	core.Digest,
) (modelinvoker.InvocationMaterialContextPairProjectionV2, error) {
	panic("typed-nil Context reader was called")
}

func (*typedNilReadersV2) InspectExactInvocationToolPairV2(
	context.Context,
	modelinvoker.InvocationMaterialExactSourceRefV1,
	modelinvoker.InvocationMaterialExactSourceRefV1,
	core.Digest,
) (modelinvoker.InvocationMaterialToolPairProjectionV2, error) {
	panic("typed-nil Tool reader was called")
}

func (*typedNilReadersV2) InspectExactInvocationProviderInjectionV1(
	context.Context,
	modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	panic("typed-nil Provider reader was called")
}

func (*typedNilReadersV2) InspectExactInvocationRouteV1(
	context.Context,
	modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	panic("typed-nil Route reader was called")
}

func (*typedNilReadersV2) InspectExactInvocationProfileV1(
	context.Context,
	modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	panic("typed-nil Profile reader was called")
}

func (*typedNilRepositoryV2) EnsureAuthorizedInvocationMaterialV2(
	context.Context,
	modelinvoker.InvocationMaterialPersistRequestV2,
) (modelinvoker.InvocationMaterialV2, error) {
	panic("typed-nil repository was called")
}

func (*typedNilRepositoryV2) InspectExactInvocationMaterialV2(
	context.Context,
	modelinvoker.InvocationMaterialRefV2,
) (modelinvoker.InvocationMaterialV2, error) {
	panic("typed-nil repository was called")
}

func (r *lostReplyRepositoryV2) EnsureAuthorizedInvocationMaterialV2(
	ctx context.Context,
	request modelinvoker.InvocationMaterialPersistRequestV2,
) (modelinvoker.InvocationMaterialV2, error) {
	material := request.MaterialV2()
	r.mu.Lock()
	r.ensureCalls++
	r.persisted = material.RefV2()
	r.mu.Unlock()
	if _, err := r.inner.EnsureAuthorizedInvocationMaterialV2(ctx, request); err != nil {
		return modelinvoker.InvocationMaterialV2{}, err
	}
	return modelinvoker.InvocationMaterialV2{}, &modelinvoker.GovernedModelInvocationErrorV1{
		Kind:      modelinvoker.GovernedModelInvocationErrorIndeterminate,
		Operation: "ensure_invocation_material_v2",
		Message:   "commit reply was lost",
	}
}

func (r *lostReplyRepositoryV2) InspectExactInvocationMaterialV2(
	ctx context.Context,
	ref modelinvoker.InvocationMaterialRefV2,
) (modelinvoker.InvocationMaterialV2, error) {
	r.mu.Lock()
	r.inspectCalls++
	r.inspected = ref
	r.mu.Unlock()
	return r.inner.InspectExactInvocationMaterialV2(ctx, ref)
}

func (r *lostReplyRepositoryV2) state() (
	int,
	int,
	modelinvoker.InvocationMaterialRefV2,
	modelinvoker.InvocationMaterialRefV2,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ensureCalls, r.inspectCalls, r.persisted, r.inspected
}

func TestAuthorizationAndMaterialIdentityDoNotForkOnLineageOrAuthorizationDigest(t *testing.T) {
	fixture := newFixtureV2(t)
	lineageB := fixture.closure.SourceLineage
	lineageB.ContextFrame = exactOwnedRefV2(
		lineageB.ContextFrame.Owner,
		modelinvoker.InvocationMaterialContextFrameKindV2,
		"frame-b",
		digestV2("context-frame-b"),
	)
	lineageB = mustSealLineageV2(t, lineageB)

	authorizationA := directAuthorizationV2(t, fixture, fixture.closure.SourceLineage)
	authorizationB := directAuthorizationV2(t, fixture, lineageB)
	if authorizationA.ID != authorizationB.ID {
		t.Fatalf("authorization identity forked on lineage: %q != %q", authorizationA.ID, authorizationB.ID)
	}
	if authorizationA.Digest == authorizationB.Digest {
		t.Fatal("authorization body digest did not bind full source lineage")
	}
	if authorizationA.RefV2().SourceLineage != fixture.closure.SourceLineage ||
		authorizationB.RefV2().SourceLineage != lineageB {
		t.Fatal("authorization Ref did not seal the complete source lineage")
	}

	materialA := directMaterialV2(t, fixture, authorizationA)
	materialB := directMaterialV2(t, fixture, authorizationB)
	if materialA.ID != materialB.ID {
		t.Fatalf("material identity forked on authorization digest: %q != %q", materialA.ID, materialB.ID)
	}
	if materialA.Digest == materialB.Digest {
		t.Fatal("material body digest did not bind the authorization body")
	}
	if materialA.RefV2().AuthorizationRef != authorizationA.RefV2() ||
		materialB.RefV2().AuthorizationRef != authorizationB.RefV2() {
		t.Fatal("material Ref does not carry its exact AuthorizationRef")
	}
}

func TestSourceLineageRejectsRoleSwapWrongKindAndCrossOwner(t *testing.T) {
	fixture := newFixtureV2(t)
	base := fixture.closure.SourceLineage
	cases := map[string]func(*modelinvoker.InvocationMaterialSourceLineageV2){
		"context role swap": func(lineage *modelinvoker.InvocationMaterialSourceLineageV2) {
			lineage.ContextFrame, lineage.ContextMaterial =
				lineage.ContextMaterial, lineage.ContextFrame
		},
		"tool role swap": func(lineage *modelinvoker.InvocationMaterialSourceLineageV2) {
			lineage.ToolInjectionMaterial, lineage.ToolSurface =
				lineage.ToolSurface, lineage.ToolInjectionMaterial
		},
		"context frame wrong kind": func(lineage *modelinvoker.InvocationMaterialSourceLineageV2) {
			lineage.ContextFrame.Kind = modelinvoker.InvocationMaterialContextMaterialKindV2
		},
		"context material wrong kind": func(lineage *modelinvoker.InvocationMaterialSourceLineageV2) {
			lineage.ContextMaterial.Kind = modelinvoker.InvocationMaterialContextFrameKindV2
		},
		"tool injection wrong kind": func(lineage *modelinvoker.InvocationMaterialSourceLineageV2) {
			lineage.ToolInjectionMaterial.Kind = modelinvoker.InvocationMaterialToolSurfaceKindV2
		},
		"tool surface wrong kind": func(lineage *modelinvoker.InvocationMaterialSourceLineageV2) {
			lineage.ToolSurface.Kind = modelinvoker.InvocationMaterialToolInjectionMaterialKindV2
		},
		"context frame cross owner": func(lineage *modelinvoker.InvocationMaterialSourceLineageV2) {
			lineage.ContextFrame.Owner = ownerV2("context", "different-context-owner")
		},
		"context material cross owner": func(lineage *modelinvoker.InvocationMaterialSourceLineageV2) {
			lineage.ContextMaterial.Owner = ownerV2("context", "different-context-owner")
		},
		"tool injection cross owner": func(lineage *modelinvoker.InvocationMaterialSourceLineageV2) {
			lineage.ToolInjectionMaterial.Owner = ownerV2("tool", "different-tool-owner")
		},
		"tool surface cross owner": func(lineage *modelinvoker.InvocationMaterialSourceLineageV2) {
			lineage.ToolSurface.Owner = ownerV2("tool", "different-tool-owner")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			lineage := base
			lineage.Digest = ""
			mutate(&lineage)
			if _, err := modelinvoker.SealInvocationMaterialSourceLineageV2(lineage); err == nil {
				t.Fatal("invalid source role lineage was accepted")
			}
		})
	}
}

func TestPairProjectionsRejectWrongKindAndCrossOwner(t *testing.T) {
	fixture := newFixtureV2(t)
	lineage := fixture.closure.SourceLineage
	contextCases := map[string]func(*modelinvoker.InvocationMaterialContextPairProjectionV2){
		"role swap": func(projection *modelinvoker.InvocationMaterialContextPairProjectionV2) {
			projection.ContextFrame, projection.ContextMaterial =
				projection.ContextMaterial, projection.ContextFrame
		},
		"wrong kind": func(projection *modelinvoker.InvocationMaterialContextPairProjectionV2) {
			projection.ContextFrame.Kind = modelinvoker.InvocationMaterialContextMaterialKindV2
		},
		"cross owner": func(projection *modelinvoker.InvocationMaterialContextPairProjectionV2) {
			projection.ContextMaterial.Owner = ownerV2("context", "different-context-owner")
		},
	}
	for name, mutate := range contextCases {
		t.Run("context "+name, func(t *testing.T) {
			projection := modelinvoker.InvocationMaterialContextPairProjectionV2{
				ContextFrame:             lineage.ContextFrame,
				ContextMaterial:          lineage.ContextMaterial,
				ContextMappedInputDigest: lineage.ContextMappedInputDigest,
				CheckedUnixNano:          fixtureNow.Add(-time.Minute).UnixNano(),
				ExpiresUnixNano:          fixtureNow.Add(time.Hour).UnixNano(),
			}
			mutate(&projection)
			if _, err := modelinvoker.SealInvocationMaterialContextPairProjectionV2(projection); err == nil {
				t.Fatal("invalid Context source pair was accepted")
			}
		})
	}
	toolCases := map[string]func(*modelinvoker.InvocationMaterialToolPairProjectionV2){
		"role swap": func(projection *modelinvoker.InvocationMaterialToolPairProjectionV2) {
			projection.ToolInjectionMaterial, projection.ToolSurface =
				projection.ToolSurface, projection.ToolInjectionMaterial
		},
		"wrong kind": func(projection *modelinvoker.InvocationMaterialToolPairProjectionV2) {
			projection.ToolSurface.Kind = modelinvoker.InvocationMaterialToolInjectionMaterialKindV2
		},
		"cross owner": func(projection *modelinvoker.InvocationMaterialToolPairProjectionV2) {
			projection.ToolInjectionMaterial.Owner = ownerV2("tool", "different-tool-owner")
		},
	}
	for name, mutate := range toolCases {
		t.Run("tool "+name, func(t *testing.T) {
			projection := modelinvoker.InvocationMaterialToolPairProjectionV2{
				ToolInjectionMaterial:   lineage.ToolInjectionMaterial,
				ToolSurface:             lineage.ToolSurface,
				ExpectedInjectionDigest: lineage.ExpectedInjectionDigest,
				CompiledToolsDigest:     lineage.CompiledToolsDigest,
				RequestToolsDigest:      lineage.RequestToolsDigest,
				CheckedUnixNano:         fixtureNow.Add(-time.Minute).UnixNano(),
				ExpiresUnixNano:         fixtureNow.Add(time.Hour).UnixNano(),
			}
			mutate(&projection)
			if _, err := modelinvoker.SealInvocationMaterialToolPairProjectionV2(projection); err == nil {
				t.Fatal("invalid Tool source pair was accepted")
			}
		})
	}
}

func TestDecodeInvocationMaterialV2RejectsNestedDuplicateUnknownAndTrailingJSON(t *testing.T) {
	fixture := newFixtureV2(t)
	material := directMaterialV2(
		t,
		fixture,
		directAuthorizationV2(t, fixture, fixture.closure.SourceLineage),
	)
	wire, err := modelinvoker.EncodeInvocationMaterialV2(material)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"nested duplicate": bytes.Replace(
			wire,
			[]byte(`"source_lineage":{`),
			[]byte(`"source_lineage":{"contract_version":"duplicate",`),
			1,
		),
		"nested unknown": bytes.Replace(
			wire,
			[]byte(`"source_lineage":{`),
			[]byte(`"source_lineage":{"unknown_field":true,`),
			1,
		),
		"trailing": append(append([]byte(nil), wire...), []byte(` {}`)...),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := modelinvoker.DecodeInvocationMaterialV2(payload); err == nil {
				t.Fatalf("%s JSON was accepted", name)
			}
		})
	}
}

func TestPairedReadersBindEveryExactCoordinateField(t *testing.T) {
	fixture := newFixtureV2(t)
	lineage := fixture.closure.SourceLineage
	contextProjection, err := modelinvoker.SealInvocationMaterialContextPairProjectionV2(
		modelinvoker.InvocationMaterialContextPairProjectionV2{
			ContextFrame:             lineage.ContextFrame,
			ContextMaterial:          lineage.ContextMaterial,
			ContextMappedInputDigest: lineage.ContextMappedInputDigest,
			CheckedUnixNano:          fixtureNow.Add(-time.Minute).UnixNano(),
			ExpiresUnixNano:          fixtureNow.Add(time.Hour).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	toolProjection, err := modelinvoker.SealInvocationMaterialToolPairProjectionV2(
		modelinvoker.InvocationMaterialToolPairProjectionV2{
			ToolInjectionMaterial:   lineage.ToolInjectionMaterial,
			ToolSurface:             lineage.ToolSurface,
			ExpectedInjectionDigest: lineage.ExpectedInjectionDigest,
			CompiledToolsDigest:     lineage.CompiledToolsDigest,
			RequestToolsDigest:      lineage.RequestToolsDigest,
			CheckedUnixNano:         fixtureNow.Add(-time.Minute).UnixNano(),
			ExpiresUnixNano:         fixtureNow.Add(time.Hour).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(modelinvoker.InvocationMaterialExactSourceRefV1) modelinvoker.InvocationMaterialExactSourceRefV1{
		"owner": func(ref modelinvoker.InvocationMaterialExactSourceRefV1) modelinvoker.InvocationMaterialExactSourceRefV1 {
			ref.Owner.ID = core.OwnerID(string(ref.Owner.ID) + "-drift")
			return ref
		},
		"kind": func(ref modelinvoker.InvocationMaterialExactSourceRefV1) modelinvoker.InvocationMaterialExactSourceRefV1 {
			ref.Kind += "-drift"
			return ref
		},
		"id": func(ref modelinvoker.InvocationMaterialExactSourceRefV1) modelinvoker.InvocationMaterialExactSourceRefV1 {
			ref.ID += "-drift"
			return ref
		},
		"revision": func(ref modelinvoker.InvocationMaterialExactSourceRefV1) modelinvoker.InvocationMaterialExactSourceRefV1 {
			ref.Revision++
			return ref
		},
		"digest": func(ref modelinvoker.InvocationMaterialExactSourceRefV1) modelinvoker.InvocationMaterialExactSourceRefV1 {
			ref.Digest = digestV2("drift")
			return ref
		},
	}
	for field, mutate := range mutations {
		t.Run("context_frame_"+field, func(t *testing.T) {
			if err := contextProjection.ValidateCurrentV2(
				mutate(lineage.ContextFrame),
				lineage.ContextMaterial,
				lineage.ContextMappedInputDigest,
				fixtureNow,
			); err == nil {
				t.Fatalf("Context frame accepted drifted %s", field)
			}
		})
		t.Run("context_material_"+field, func(t *testing.T) {
			if err := contextProjection.ValidateCurrentV2(
				lineage.ContextFrame,
				mutate(lineage.ContextMaterial),
				lineage.ContextMappedInputDigest,
				fixtureNow,
			); err == nil {
				t.Fatalf("Context material accepted drifted %s", field)
			}
		})
		t.Run("tool_injection_"+field, func(t *testing.T) {
			if err := toolProjection.ValidateCurrentV2(
				mutate(lineage.ToolInjectionMaterial),
				lineage.ToolSurface,
				lineage.ExpectedInjectionDigest,
				lineage.CompiledToolsDigest,
				lineage.RequestToolsDigest,
				fixtureNow,
			); err == nil {
				t.Fatalf("Tool injection material accepted drifted %s", field)
			}
		})
		t.Run("tool_surface_"+field, func(t *testing.T) {
			if err := toolProjection.ValidateCurrentV2(
				lineage.ToolInjectionMaterial,
				mutate(lineage.ToolSurface),
				lineage.ExpectedInjectionDigest,
				lineage.CompiledToolsDigest,
				lineage.RequestToolsDigest,
				fixtureNow,
			); err == nil {
				t.Fatalf("Tool surface accepted drifted %s", field)
			}
		})
	}
}

func TestAuthorizerS2RejectsPairedReaderCurrentDriftBeforeRepository(t *testing.T) {
	for _, drift := range []string{"context", "tool"} {
		t.Run(drift, func(t *testing.T) {
			fixture := newFixtureV2(t)
			readers := &driftingReadersV2{
				base:  exactReadersForFixtureV2(fixture),
				drift: drift,
			}
			authorizer := mustAuthorizerV2(t, modelinvoker.InvocationMaterialAuthorizerConfigV2{
				ContextPair:       readers,
				ToolPair:          readers,
				ProviderInjection: readers,
				Route:             readers,
				Profile:           readers,
			})
			repository := &countingRepositoryV2{}
			_, err := modelinvoker.AuthorizeAndEnsureInvocationMaterialV2(
				context.Background(),
				authorizer,
				repository,
				fixture.prepared,
				fixture.current,
				fixture.call,
				fixture.closure,
				clockSequenceV2(fixtureNow, fixtureNow, fixtureNow),
			)
			if kind := modelinvoker.GovernedModelInvocationErrorKindOfV1(err); kind != modelinvoker.GovernedModelInvocationErrorConflict {
				t.Fatalf("S2 %s drift returned %q, want conflict: %v", drift, kind, err)
			}
			ensureCalls, inspectCalls := repository.calls()
			if ensureCalls != 0 || inspectCalls != 0 {
				t.Fatalf("repository was called after S2 drift: ensure=%d inspect=%d", ensureCalls, inspectCalls)
			}
			contextCalls, toolCalls := readers.callCounts()
			if contextCalls != 2 {
				t.Fatalf("Context pair read %d times, want 2", contextCalls)
			}
			if drift == "tool" && toolCalls != 2 {
				t.Fatalf("Tool pair read %d times, want 2", toolCalls)
			}
		})
	}
}

func TestAuthorizerS1AndS2RejectProviderRouteAndProfileDriftBeforeRepository(t *testing.T) {
	for _, drift := range []string{"provider", "route", "profile"} {
		for _, stage := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s S%d", drift, stage), func(t *testing.T) {
				fixture := newFixtureV2(t)
				readers := &driftingReadersV2{
					base:    exactReadersForFixtureV2(fixture),
					drift:   drift,
					driftAt: stage,
				}
				authorizer := mustAuthorizerV2(t, modelinvoker.InvocationMaterialAuthorizerConfigV2{
					ContextPair:       readers,
					ToolPair:          readers,
					ProviderInjection: readers,
					Route:             readers,
					Profile:           readers,
				})
				repository := &countingRepositoryV2{}
				_, err := modelinvoker.AuthorizeAndEnsureInvocationMaterialV2(
					context.Background(),
					authorizer,
					repository,
					fixture.prepared,
					fixture.current,
					fixture.call,
					fixture.closure,
					clockSequenceV2(fixtureNow, fixtureNow, fixtureNow),
				)
				if kind := modelinvoker.GovernedModelInvocationErrorKindOfV1(err); kind != modelinvoker.GovernedModelInvocationErrorConflict {
					t.Fatalf("S%d %s drift returned %q, want conflict: %v", stage, drift, kind, err)
				}
				ensureCalls, inspectCalls := repository.calls()
				if ensureCalls != 0 || inspectCalls != 0 {
					t.Fatalf("repository was called after S%d drift: ensure=%d inspect=%d", stage, ensureCalls, inspectCalls)
				}
				providerCalls, routeCalls, profileCalls := readers.sourceCallCounts()
				switch drift {
				case "provider":
					if providerCalls != stage {
						t.Fatalf("Provider source read %d times, want %d", providerCalls, stage)
					}
				case "route":
					if routeCalls != stage {
						t.Fatalf("Route source read %d times, want %d", routeCalls, stage)
					}
				case "profile":
					if profileCalls != stage {
						t.Fatalf("Profile source read %d times, want %d", profileCalls, stage)
					}
				}
			})
		}
	}
}

func TestAuthorizerRejectsTTLCrossingAndClockRollbackBeforeRepository(t *testing.T) {
	tests := []struct {
		name  string
		times []time.Time
	}{
		{
			name:  "paired reader TTL crossing",
			times: []time.Time{fixtureNow, fixtureNow.Add(30 * time.Minute)},
		},
		{
			name:  "clock rollback",
			times: []time.Time{fixtureNow, fixtureNow.Add(-time.Nanosecond)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixtureV2(t)
			repository := &countingRepositoryV2{}
			_, err := modelinvoker.AuthorizeAndEnsureInvocationMaterialV2(
				context.Background(),
				fixture.authorizer,
				repository,
				fixture.prepared,
				fixture.current,
				fixture.call,
				fixture.closure,
				clockSequenceV2(test.times...),
			)
			if kind := modelinvoker.GovernedModelInvocationErrorKindOfV1(err); kind != modelinvoker.GovernedModelInvocationErrorConflict {
				t.Fatalf("%s returned %q, want conflict: %v", test.name, kind, err)
			}
			ensureCalls, inspectCalls := repository.calls()
			if ensureCalls != 0 || inspectCalls != 0 {
				t.Fatalf("repository was called after %s: ensure=%d inspect=%d", test.name, ensureCalls, inspectCalls)
			}
		})
	}
}

func TestInvocationMaterialV2RejectsTypedNilDependencies(t *testing.T) {
	fixture := newFixtureV2(t)
	readers := exactReadersForFixtureV2(fixture)
	var nilReader *typedNilReadersV2
	configs := []struct {
		name   string
		config modelinvoker.InvocationMaterialAuthorizerConfigV2
	}{
		{
			name: "Context reader",
			config: modelinvoker.InvocationMaterialAuthorizerConfigV2{
				ContextPair: nilReader, ToolPair: readers,
				ProviderInjection: readers, Route: readers, Profile: readers,
			},
		},
		{
			name: "Tool reader",
			config: modelinvoker.InvocationMaterialAuthorizerConfigV2{
				ContextPair: readers, ToolPair: nilReader,
				ProviderInjection: readers, Route: readers, Profile: readers,
			},
		},
		{
			name: "Provider reader",
			config: modelinvoker.InvocationMaterialAuthorizerConfigV2{
				ContextPair: readers, ToolPair: readers,
				ProviderInjection: nilReader, Route: readers, Profile: readers,
			},
		},
		{
			name: "Route reader",
			config: modelinvoker.InvocationMaterialAuthorizerConfigV2{
				ContextPair: readers, ToolPair: readers,
				ProviderInjection: readers, Route: nilReader, Profile: readers,
			},
		},
		{
			name: "Profile reader",
			config: modelinvoker.InvocationMaterialAuthorizerConfigV2{
				ContextPair: readers, ToolPair: readers,
				ProviderInjection: readers, Route: readers, Profile: nilReader,
			},
		},
	}
	for _, test := range configs {
		t.Run(test.name, func(t *testing.T) {
			if _, err := modelinvoker.NewInvocationMaterialAuthorizerV2(test.config); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorInvalid {
				t.Fatalf("typed-nil %s was accepted: %v", test.name, err)
			}
		})
	}

	t.Run("repository", func(t *testing.T) {
		var repository *typedNilRepositoryV2
		_, err := modelinvoker.AuthorizeAndEnsureInvocationMaterialV2(
			context.Background(),
			fixture.authorizer,
			repository,
			fixture.prepared,
			fixture.current,
			fixture.call,
			fixture.closure,
			func() time.Time { return fixtureNow },
		)
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorInvalid {
			t.Fatalf("typed-nil repository was accepted: %v", err)
		}
	})

	t.Run("clock", func(t *testing.T) {
		var clock func() time.Time
		_, err := modelinvoker.AuthorizeAndEnsureInvocationMaterialV2(
			context.Background(),
			fixture.authorizer,
			&countingRepositoryV2{},
			fixture.prepared,
			fixture.current,
			fixture.call,
			fixture.closure,
			clock,
		)
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorInvalid {
			t.Fatalf("typed-nil clock was accepted: %v", err)
		}
	})
}

func TestIndeterminateEnsureInspectsPresealedExactWithoutSecondCreate(t *testing.T) {
	store := openStoreV2(t, filepath.Join(t.TempDir(), "model-invoker.db"))
	defer store.Close()
	fixture := newFixtureV2(t)
	repository := &lostReplyRepositoryV2{inner: store}

	material, err := modelinvoker.AuthorizeAndEnsureInvocationMaterialV2(
		context.Background(),
		fixture.authorizer,
		repository,
		fixture.prepared,
		fixture.current,
		fixture.call,
		fixture.closure,
		clockSequenceV2(fixtureNow, fixtureNow, fixtureNow),
	)
	if err != nil {
		t.Fatal(err)
	}
	ensureCalls, inspectCalls, persisted, inspected := repository.state()
	if ensureCalls != 1 || inspectCalls != 1 {
		t.Fatalf("lost reply recovery used ensure=%d inspect=%d, want 1/1", ensureCalls, inspectCalls)
	}
	if material.RefV2() != persisted || persisted != inspected {
		t.Fatalf(
			"lost reply recovery changed exact Ref: material=%#v persisted=%#v inspected=%#v",
			material.RefV2(),
			persisted,
			inspected,
		)
	}
}

func TestSQLiteHistoryCreateOnceRejectsSameIdentityDifferentLineage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-invoker.db")
	store := openStoreV2(t, path)
	fixture := newFixtureV2(t)

	first, err := ensureFixtureV2(store, fixture)
	if err != nil {
		t.Fatal(err)
	}
	changed := fixture
	changed.closure.SourceLineage.ContextFrame = exactOwnedRefV2(
		changed.closure.SourceLineage.ContextFrame.Owner,
		modelinvoker.InvocationMaterialContextFrameKindV2,
		"frame-conflict",
		digestV2("context-frame-conflict"),
	)
	changed.closure.SourceLineage = mustSealLineageV2(t, changed.closure.SourceLineage)
	second, err := ensureFixtureV2(store, changed)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("same identity with different lineage was not Conflict: %#v, %v", second, err)
	}
	if second.ID != "" || second.Digest != "" {
		t.Fatalf("conflicting create returned a material: %#v", second)
	}
	got, err := store.InspectExactInvocationMaterialV2(context.Background(), first.RefV2())
	if err != nil || got.RefV2() != first.RefV2() {
		t.Fatalf("original history was not preserved: %#v, %v", got, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openStoreV2(t, path)
	defer reopened.Close()
	got, err = reopened.InspectExactInvocationMaterialV2(context.Background(), first.RefV2())
	if err != nil || got.RefV2() != first.RefV2() {
		t.Fatalf("exact history did not survive restart: %#v, %v", got, err)
	}
}

func TestSQLiteHistoryIsIdempotentUnder64ConcurrentCreates(t *testing.T) {
	store := openStoreV2(t, filepath.Join(t.TempDir(), "model-invoker.db"))
	defer store.Close()
	fixture := newFixtureV2(t)

	const workers = 64
	results := make(chan modelinvoker.InvocationMaterialV2, workers)
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			material, err := ensureFixtureV2(store, fixture)
			results <- material
			errors <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errors)

	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent create failed: %v", err)
		}
	}
	var want modelinvoker.InvocationMaterialRefV2
	for material := range results {
		if want == (modelinvoker.InvocationMaterialRefV2{}) {
			want = material.RefV2()
			continue
		}
		if material.RefV2() != want {
			t.Fatalf("concurrent create returned a sibling material: %#v != %#v", material.RefV2(), want)
		}
	}
}

func TestSQLiteV3SchemaHasHistoryOnlyAndUniqueAuthorizationIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-invoker.db")
	store := openStoreV2(t, path)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='invocation_material_v2_current'`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("forbidden invocation_material_v2_current table exists")
	}
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='invocation_material_v2_history'`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("invocation_material_v2_history table is absent")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM invocation_material_v2_history`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("schema constraint probes leaked %d history rows", count)
	}

	insert := `INSERT INTO invocation_material_v2_history(
		material_id,revision,material_digest,
		prepared_id,prepared_revision,prepared_digest,
		current_id,current_revision,current_digest,
		current_checked_unix_nano,current_expires_unix_nano,current_not_after_unix_nano,
		route_call_digest,authorization_id,source_lineage_digest,authorization_digest,
		expires_unix_nano,canonical_json
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	values := []any{
		"material-a", 1, digestV2("material-a"),
		"prepared", 1, digestV2("prepared"),
		"current", 1, digestV2("current"),
		1, 2, 3,
		digestV2("route"), "authorization-stable", digestV2("lineage-a"), digestV2("authorization-a"),
		fixtureNow.Add(time.Hour).UnixNano(), []byte(`{"material":"a"}`),
	}
	if _, err := db.Exec(insert, values...); err != nil {
		t.Fatal(err)
	}
	values[0] = "material-b"
	values[2] = digestV2("material-b")
	values[14] = digestV2("lineage-b")
	values[15] = digestV2("authorization-b")
	if _, err := db.Exec(insert, values...); err == nil {
		t.Fatal("duplicate authorization_id created a sibling history row")
	}
	invalid := []struct {
		name             string
		revision         int
		preparedRevision int
		expiresUnixNano  int64
	}{
		{name: "revision zero", revision: 0, preparedRevision: 1, expiresUnixNano: 1},
		{name: "revision negative", revision: -1, preparedRevision: 1, expiresUnixNano: 1},
		{name: "prepared revision zero", revision: 1, preparedRevision: 0, expiresUnixNano: 1},
		{name: "prepared revision negative", revision: 1, preparedRevision: -1, expiresUnixNano: 1},
		{name: "expiry zero", revision: 1, preparedRevision: 1, expiresUnixNano: 0},
		{name: "expiry negative", revision: 1, preparedRevision: 1, expiresUnixNano: -1},
	}
	for index, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			id := fmt.Sprintf("invalid-%d", index)
			_, err := db.Exec(
				insert,
				id,
				test.revision,
				digestV2(id),
				"prepared",
				test.preparedRevision,
				digestV2("prepared"),
				"current",
				1,
				digestV2("current"),
				1,
				2,
				3,
				digestV2("route"),
				id+"-authorization",
				digestV2(id+"-lineage"),
				digestV2(id+"-authorization"),
				test.expiresUnixNano,
				[]byte(`{}`),
			)
			if err == nil {
				t.Fatalf("physical schema accepted %s", test.name)
			}
		})
	}
}

func TestSQLiteV2DatabaseMigratesToV3WithoutChangingExistingData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "model-invoker-v2.db")
	fixture := newFixtureV2(t)
	prepared := fixture.prepared
	current := fixture.current
	preparedWire, err := modelinvoker.EncodePreparedModelInvocationFactV1(prepared)
	if err != nil {
		t.Fatal(err)
	}
	currentWire, err := modelinvoker.EncodePreparedModelInvocationCurrentV1(current)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(legacySQLiteSchemaV1 + legacySQLiteSchemaV2); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO model_invoker_schema(version,digest,applied_unix_nano) VALUES(?,?,?),(?,?,?)`,
		1,
		string(core.DigestBytes([]byte(legacySQLiteSchemaV1))),
		int64(1),
		2,
		string(core.DigestBytes([]byte(legacySQLiteSchemaV2))),
		int64(2),
	); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO prepared_model_invocation_history(
		   prepared_id,revision,fact_digest,canonical_json
		 ) VALUES(?,?,?,?)`,
		prepared.ID,
		prepared.Revision,
		string(prepared.Digest),
		preparedWire,
	); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO prepared_model_invocation_current(
		   current_id,revision,current_digest,
		   prepared_id,prepared_revision,prepared_digest,canonical_json
		 ) VALUES(?,?,?,?,?,?,?)`,
		current.ID,
		current.Revision,
		string(current.Digest),
		current.Prepared.ID,
		current.Prepared.Revision,
		string(current.Prepared.Digest),
		currentWire,
	); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	var maxVersion int
	if err := db.QueryRow(`SELECT MAX(version) FROM model_invoker_schema`).Scan(&maxVersion); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if maxVersion != 2 {
		_ = db.Close()
		t.Fatalf("legacy database version=%d, want 2", maxVersion)
	}
	legacySchema := snapshotLegacySQLiteObjectsV2(t, db)
	legacyLedger := snapshotLegacySQLiteLedgerV2(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	migrated := openStoreV2(t, path)
	gotPrepared, err := migrated.InspectExactPreparedModelInvocationV1(ctx, prepared.Ref())
	if err != nil || gotPrepared != prepared {
		t.Fatalf("prepared V1 changed across V2-to-V3 migration: %#v, %v", gotPrepared, err)
	}
	gotCurrent, err := migrated.InspectExactPreparedModelInvocationCurrentV1(ctx, current.Ref())
	if err != nil || gotCurrent != current {
		t.Fatalf("prepared current V1 changed across V2-to-V3 migration: %#v, %v", gotCurrent, err)
	}
	material, err := ensureFixtureV2(migrated, fixture)
	if err != nil {
		t.Fatal(err)
	}
	gotMaterial, err := migrated.InspectExactInvocationMaterialV2(ctx, material.RefV2())
	if err != nil || !reflect.DeepEqual(gotMaterial, material) {
		t.Fatalf("V2 material was not exact after migration: %#v, %v", gotMaterial, err)
	}
	if err := migrated.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = sql.Open("sqlite", fmt.Sprintf("file:%s", path))
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotLegacySQLiteObjectsV2(t, db); !reflect.DeepEqual(got, legacySchema) {
		_ = db.Close()
		t.Fatal("legacy V1/V2 physical schema changed during V3 migration")
	}
	if got := snapshotLegacySQLiteLedgerV2(t, db); !reflect.DeepEqual(got, legacyLedger) {
		_ = db.Close()
		t.Fatal("legacy V1/V2 schema ledger changed during V3 migration")
	}
	var v3LedgerRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM model_invoker_schema WHERE version=3`).Scan(&v3LedgerRows); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if v3LedgerRows != 1 {
		_ = db.Close()
		t.Fatalf("V3 migration ledger rows=%d, want 1", v3LedgerRows)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openStoreV2(t, path)
	defer reopened.Close()
	if got, err := reopened.InspectExactPreparedModelInvocationV1(ctx, prepared.Ref()); err != nil || got != prepared {
		t.Fatalf("prepared V1 changed after repeated Open: %#v, %v", got, err)
	}
	if got, err := reopened.InspectExactPreparedModelInvocationCurrentV1(ctx, current.Ref()); err != nil || got != current {
		t.Fatalf("prepared current V1 changed after repeated Open: %#v, %v", got, err)
	}
	if got, err := reopened.InspectExactInvocationMaterialV2(ctx, material.RefV2()); err != nil || !reflect.DeepEqual(got, material) {
		t.Fatalf("V2 material changed after repeated Open: %#v, %v", got, err)
	}
}

func TestSQLiteV3RejectsWeakPrecreatedHistoryDespiteCorrectLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-invoker.db")
	store := openStoreV2(t, path)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		DROP TABLE invocation_material_v2_history;
		CREATE TABLE invocation_material_v2_history (
			material_id TEXT,
			revision INTEGER,
			material_digest TEXT,
			prepared_id TEXT,
			prepared_revision INTEGER,
			prepared_digest TEXT,
			current_id TEXT,
			current_revision INTEGER,
			current_digest TEXT,
			current_checked_unix_nano INTEGER,
			current_expires_unix_nano INTEGER,
			current_not_after_unix_nano INTEGER,
			route_call_digest TEXT,
			authorization_id TEXT,
			source_lineage_digest TEXT,
			authorization_digest TEXT,
			expires_unix_nano INTEGER,
			canonical_json BLOB
		);
	`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if reopened != nil {
		_ = reopened.Close()
		t.Fatal("weak precreated V3 history was accepted")
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("weak V3 history did not fail closed as Conflict: %v", err)
	}
}

func TestSQLiteV3RejectsCommentForgedChecksDespiteExactShapeAndLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-invoker.db")
	store := openStoreV2(t, path)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		DROP TABLE invocation_material_v2_history;
		CREATE TABLE invocation_material_v2_history (
		  material_id TEXT NOT NULL,
		  revision INTEGER NOT NULL CHECK(1 /* CHECK(revision > 0) */),
		  material_digest TEXT NOT NULL,
		  prepared_id TEXT NOT NULL,
		  prepared_revision INTEGER NOT NULL CHECK(1 /* CHECK(prepared_revision > 0) */),
		  prepared_digest TEXT NOT NULL,
		  current_id TEXT NOT NULL,
		  current_revision INTEGER NOT NULL CHECK(1 /* CHECK(current_revision > 0) */),
		  current_digest TEXT NOT NULL,
		  current_checked_unix_nano INTEGER NOT NULL CHECK(1 /* CHECK(current_checked_unix_nano > 0) */),
		  current_expires_unix_nano INTEGER NOT NULL CHECK(1 /* CHECK(current_expires_unix_nano > 0) */),
		  current_not_after_unix_nano INTEGER NOT NULL CHECK(1 /* CHECK(current_not_after_unix_nano > 0) */),
		  route_call_digest TEXT NOT NULL,
		  authorization_id TEXT NOT NULL UNIQUE,
		  source_lineage_digest TEXT NOT NULL,
		  authorization_digest TEXT NOT NULL,
		  expires_unix_nano INTEGER NOT NULL CHECK(1 /* CHECK(expires_unix_nano > 0) */),
		  canonical_json BLOB NOT NULL,
		  PRIMARY KEY(material_id, revision)
		);
	`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if reopened != nil {
		_ = reopened.Close()
		t.Fatal("comment-forged CHECK constraints were accepted")
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("comment-forged CHECK constraints did not fail closed as Conflict: %v", err)
	}
}

func TestSQLiteV3AcceptsEquivalentQuotedIdentifiersAndRealComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-invoker.db")
	store := openStoreV2(t, path)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		DROP TABLE invocation_material_v2_history;
		CREATE TABLE "invocation_material_v2_history" (
		  "material_id" TEXT NOT NULL,
		  "revision" INTEGER NOT NULL /* required positive revision */ CHECK("revision" > 0),
		  "material_digest" TEXT NOT NULL,
		  "prepared_id" TEXT NOT NULL,
		  "prepared_revision" INTEGER NOT NULL -- required positive prepared revision
		    CHECK("prepared_revision" > 0),
		  "prepared_digest" TEXT NOT NULL,
		  "current_id" TEXT NOT NULL,
		  "current_revision" INTEGER NOT NULL CHECK("current_revision" > 0),
		  "current_digest" TEXT NOT NULL,
		  "current_checked_unix_nano" INTEGER NOT NULL CHECK("current_checked_unix_nano" > 0),
		  "current_expires_unix_nano" INTEGER NOT NULL CHECK("current_expires_unix_nano" > 0),
		  "current_not_after_unix_nano" INTEGER NOT NULL CHECK("current_not_after_unix_nano" > 0),
		  "route_call_digest" TEXT NOT NULL,
		  "authorization_id" TEXT NOT NULL UNIQUE,
		  "source_lineage_digest" TEXT NOT NULL,
		  "authorization_digest" TEXT NOT NULL,
		  "expires_unix_nano" INTEGER NOT NULL CHECK("expires_unix_nano" > 0),
		  "canonical_json" BLOB NOT NULL,
		  PRIMARY KEY("material_id", "revision")
		);
	`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatalf("semantically exact quoted/commented DDL was rejected: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteV3RejectsUniqueConstraintMasqueradingAsCheckProof(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-invoker.db")
	store := openStoreV2(t, path)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		DROP TABLE invocation_material_v2_history;
		CREATE TABLE invocation_material_v2_history (
		  material_id TEXT NOT NULL,
		  revision INTEGER NOT NULL CHECK(1 /* CHECK(revision > 0) */),
		  material_digest TEXT NOT NULL,
		  prepared_id TEXT NOT NULL,
		  prepared_revision INTEGER NOT NULL CHECK(1 /* CHECK(prepared_revision > 0) */),
		  prepared_digest TEXT NOT NULL,
		  current_id TEXT NOT NULL,
		  current_revision INTEGER NOT NULL CHECK(1 /* CHECK(current_revision > 0) */),
		  current_digest TEXT NOT NULL,
		  current_checked_unix_nano INTEGER NOT NULL CHECK(1 /* CHECK(current_checked_unix_nano > 0) */),
		  current_expires_unix_nano INTEGER NOT NULL CHECK(1 /* CHECK(current_expires_unix_nano > 0) */),
		  current_not_after_unix_nano INTEGER NOT NULL CHECK(1 /* CHECK(current_not_after_unix_nano > 0) */),
		  route_call_digest TEXT NOT NULL,
		  authorization_id TEXT NOT NULL UNIQUE,
		  source_lineage_digest TEXT NOT NULL,
		  authorization_digest TEXT NOT NULL,
		  expires_unix_nano INTEGER NOT NULL CHECK(1 /* CHECK(expires_unix_nano > 0) */),
		  canonical_json BLOB NOT NULL,
		  PRIMARY KEY(material_id, revision)
		);
		CREATE UNIQUE INDEX invocation_material_v2_history_probe_collision
		  ON invocation_material_v2_history(material_digest);
		INSERT INTO invocation_material_v2_history(
		  material_id,revision,material_digest,
		  prepared_id,prepared_revision,prepared_digest,
		  current_id,current_revision,current_digest,
		  current_checked_unix_nano,current_expires_unix_nano,current_not_after_unix_nano,
		  route_call_digest,authorization_id,source_lineage_digest,authorization_digest,
		  expires_unix_nano,canonical_json
		) VALUES(
		  'preseed',1,'probe-material-digest',
		  'prepared',1,'prepared-digest',
		  'current',1,'current-digest',
		  1,2,3,
		  'route-digest','preseed-authorization','lineage-digest','authorization-digest',
		  1,x'7b7d'
		);
	`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if reopened != nil {
		_ = reopened.Close()
		t.Fatal("UNIQUE interference was accepted as CHECK enforcement")
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("UNIQUE interference did not fail closed as Conflict: %v", err)
	}
}

func TestSQLiteV3RejectsAlwaysAbortTriggerDespiteValidPhysicalDDL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-invoker.db")
	store := openStoreV2(t, path)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER invocation_material_v2_history_abort
		BEFORE INSERT ON invocation_material_v2_history
		BEGIN
		  SELECT RAISE(ABORT, 'all writes blocked');
		END;
	`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if reopened != nil {
		_ = reopened.Close()
		t.Fatal("always-abort trigger was accepted as CHECK enforcement")
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("always-abort trigger did not fail closed as Conflict: %v", err)
	}
}

func TestSQLiteV3RejectsGeneratedColumnsAndExtraIndexes(t *testing.T) {
	cases := []struct {
		name string
		ddl  string
	}{
		{
			name: "generated hidden column",
			ddl: `
				ALTER TABLE invocation_material_v2_history
				ADD COLUMN generated_probe TEXT
				GENERATED ALWAYS AS (material_id) VIRTUAL;
			`,
		},
		{
			name: "extra unique index",
			ddl: `
				CREATE UNIQUE INDEX invocation_material_v2_history_extra_unique
				ON invocation_material_v2_history(material_digest);
			`,
		},
		{
			name: "extra partial index",
			ddl: `
				CREATE INDEX invocation_material_v2_history_extra_partial
				ON invocation_material_v2_history(material_id)
				WHERE revision > 0;
			`,
		},
		{
			name: "extra expression index",
			ddl: `
				CREATE INDEX invocation_material_v2_history_extra_expression
				ON invocation_material_v2_history(lower(material_id));
			`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "model-invoker.db")
			store := openStoreV2(t, path)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(test.ddl); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := modelsqlite.Open(
				context.Background(),
				modelsqlite.Config{Path: path},
			)
			if reopened != nil {
				_ = reopened.Close()
				t.Fatalf("%s was accepted", test.name)
			}
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
				t.Fatalf("%s did not fail closed as Conflict: %v", test.name, err)
			}
		})
	}
}

func TestSQLiteInspectRecomputesEveryHistoryCoordinate(t *testing.T) {
	cases := []struct {
		name     string
		column   string
		value    any
		wantKind modelinvoker.GovernedModelInvocationErrorKindV1
	}{
		{name: "material id", column: "material_id", value: "material-drift", wantKind: modelinvoker.GovernedModelInvocationErrorNotFound},
		{name: "revision", column: "revision", value: 2, wantKind: modelinvoker.GovernedModelInvocationErrorNotFound},
		{name: "material digest", column: "material_digest", value: digestV2("material-drift"), wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "prepared id", column: "prepared_id", value: "prepared-drift", wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "prepared revision", column: "prepared_revision", value: 2, wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "prepared digest", column: "prepared_digest", value: digestV2("prepared-drift"), wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "current id", column: "current_id", value: "current-drift", wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "current revision", column: "current_revision", value: 2, wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "current digest", column: "current_digest", value: digestV2("current-drift"), wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "current checked", column: "current_checked_unix_nano", value: fixtureNow.Add(-2 * time.Minute).UnixNano(), wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "current expires", column: "current_expires_unix_nano", value: fixtureNow.Add(2 * time.Hour).UnixNano(), wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "current not after", column: "current_not_after_unix_nano", value: fixtureNow.Add(3 * time.Hour).UnixNano(), wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "route call digest", column: "route_call_digest", value: digestV2("route-drift"), wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "authorization id", column: "authorization_id", value: "authorization-drift", wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "source lineage digest", column: "source_lineage_digest", value: digestV2("lineage-drift"), wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "authorization digest", column: "authorization_digest", value: digestV2("authorization-drift"), wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "expiry", column: "expires_unix_nano", value: fixtureNow.Add(3 * time.Hour).UnixNano(), wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
		{name: "canonical JSON", column: "canonical_json", value: []byte(`{"drift":true}`), wantKind: modelinvoker.GovernedModelInvocationErrorConflict},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "model-invoker.db")
			store := openStoreV2(t, path)
			fixture := newFixtureV2(t)
			material, err := ensureFixtureV2(store, fixture)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
			if err != nil {
				t.Fatal(err)
			}
			statement := fmt.Sprintf(
				"UPDATE invocation_material_v2_history SET %s=? WHERE material_id=? AND revision=?",
				test.column,
			)
			if _, err := db.Exec(statement, test.value, material.ID, material.Revision); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := openStoreV2(t, path)
			defer reopened.Close()
			_, err = reopened.InspectExactInvocationMaterialV2(
				context.Background(),
				material.RefV2(),
			)
			if kind := modelinvoker.GovernedModelInvocationErrorKindOfV1(err); kind != test.wantKind {
				t.Fatalf("tampered %s returned %q, want %q: %v", test.column, kind, test.wantKind, err)
			}
		})
	}
}

func newFixtureV2(t *testing.T) fixtureV2 {
	t.Helper()
	strict := true
	parallel := false
	call := modelinvoker.RouteCall{
		RouteID: "test.governed.route",
		Invocation: upstream.InvocationContext{
			Usage:     upstream.InvocationGeneralAPI,
			Subject:   upstream.SubjectService,
			Tenancy:   upstream.TenancyMulti,
			Execution: upstream.ExecutionForeground,
		},
		Request: modelinvoker.Request{
			Model: "test-model",
			Input: []modelinvoker.InputItem{
				modelinvoker.MessageInput(modelinvoker.RoleUser, "read README"),
			},
			Tools: []modelinvoker.Tool{{
				Name:        "workspace.read",
				Description: "read one bounded file",
				Parameters:  []byte(`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}},"additionalProperties":false}`),
				Strict:      &strict,
			}},
			ToolChoice:        modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired},
			ParallelToolCalls: &parallel,
			Budget:            modelinvoker.Budget{MaxOutputTokens: 256, Timeout: time.Minute},
		},
	}
	requestTools, err := modelinvoker.DigestGovernedModelTurnRequestToolsV2(call)
	if err != nil {
		t.Fatal(err)
	}
	contextMapped, err := modelinvoker.DigestGovernedModelTurnContextV2(call)
	if err != nil {
		t.Fatal(err)
	}
	requestDigest := digestV2("unified-request")
	routeDigest := digestV2("route")
	profileDigest := digestV2("profile")
	expectedInjection := digestV2("expected-injection")
	providerDigest := digestV2("provider-injection")
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(
		modelinvoker.PreparedModelInvocationFactV1{
			InvocationID:                  "invocation-material-v2-test",
			InvocationDigest:              requestDigest,
			UnifiedRequestDigest:          requestDigest,
			RequestToolsDigest:            requestTools,
			PreparedPlanDigest:            digestV2("prepared-plan"),
			RouteDigest:                   routeDigest,
			ProfileDigest:                 profileDigest,
			ActualToolSurfaceDigest:       expectedInjection,
			ActualProviderInjectionDigest: providerDigest,
			CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{
				ContractVersion: "1.0.0",
				ID:              "capability-snapshot",
				Revision:        1,
				Digest:          digestV2("capability-snapshot"),
			},
			RegistrySnapshotRef: runtimeports.RegistrySnapshotRefV1{
				Owner:           ownerV2("registry", "registry-owner"),
				ContractVersion: "1.0.0",
				ID:              "registry-snapshot",
				Revision:        1,
				Digest:          digestV2("registry-snapshot"),
			},
			CreatedUnixNano:  fixtureNow.Add(-2 * time.Minute).UnixNano(),
			NotAfterUnixNano: fixtureNow.Add(2 * time.Hour).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(
		modelinvoker.PreparedModelInvocationCurrentProjectionV1{
			Prepared:                      prepared.Ref(),
			CapabilitySnapshotRef:         prepared.CapabilitySnapshotRef,
			RegistrySnapshotRef:           prepared.RegistrySnapshotRef,
			ActualToolSurfaceDigest:       prepared.ActualToolSurfaceDigest,
			ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest,
			CheckedUnixNano:               fixtureNow.Add(-time.Minute).UnixNano(),
			ExpiresUnixNano:               fixtureNow.Add(time.Hour).UnixNano(),
			NotAfterUnixNano:              prepared.NotAfterUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	contextOwner := ownerV2("context", "context-owner")
	toolOwner := ownerV2("tool", "tool-owner")
	lineage := mustSealLineageV2(t, modelinvoker.InvocationMaterialSourceLineageV2{
		ContextFrame: exactOwnedRefV2(
			contextOwner,
			modelinvoker.InvocationMaterialContextFrameKindV2,
			"frame",
			digestV2("context-frame"),
		),
		ContextMaterial: exactOwnedRefV2(
			contextOwner,
			modelinvoker.InvocationMaterialContextMaterialKindV2,
			"material",
			digestV2("context-material"),
		),
		ToolInjectionMaterial: exactOwnedRefV2(
			toolOwner,
			modelinvoker.InvocationMaterialToolInjectionMaterialKindV2,
			"injection",
			digestV2("tool-injection"),
		),
		ToolSurface: exactOwnedRefV2(
			toolOwner,
			modelinvoker.InvocationMaterialToolSurfaceKindV2,
			"surface",
			digestV2("tool-surface-current"),
		),
		ContextMappedInputDigest: contextMapped,
		ExpectedInjectionDigest:  expectedInjection,
		CompiledToolsDigest:      digestV2("compiled-tools"),
		RequestToolsDigest:       requestTools,
	})
	seenSourceDigests := map[core.Digest]struct{}{}
	for _, ref := range []modelinvoker.InvocationMaterialExactSourceRefV1{
		lineage.ContextFrame,
		lineage.ContextMaterial,
		lineage.ToolInjectionMaterial,
		lineage.ToolSurface,
	} {
		if _, exists := seenSourceDigests[ref.Digest]; exists {
			t.Fatal("fixture collapsed two source roles onto one exact digest")
		}
		seenSourceDigests[ref.Digest] = struct{}{}
	}
	closure := modelinvoker.InvocationMaterialExactClosureV2{
		SourceLineage:     lineage,
		ProviderInjection: exactRefV2("model", "provider", providerDigest),
		Route:             exactRefV2("model", "route", routeDigest),
		Profile:           exactRefV2("model", "profile", profileDigest),
	}
	readers := exactReadersV2{
		expectedInjection: expectedInjection,
		compiledTools:     lineage.CompiledToolsDigest,
		checkedUnixNano:   fixtureNow.Add(-30 * time.Second).UnixNano(),
		expiresUnixNano:   fixtureNow.Add(30 * time.Minute).UnixNano(),
	}
	authorizer, err := modelinvoker.NewInvocationMaterialAuthorizerV2(
		modelinvoker.InvocationMaterialAuthorizerConfigV2{
			ContextPair:       readers,
			ToolPair:          readers,
			ProviderInjection: readers,
			Route:             readers,
			Profile:           readers,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return fixtureV2{
		call:       call,
		prepared:   prepared,
		current:    current,
		closure:    closure,
		authorizer: authorizer,
	}
}

func directAuthorizationV2(
	t *testing.T,
	fixture fixtureV2,
	lineage modelinvoker.InvocationMaterialSourceLineageV2,
) modelinvoker.InvocationMaterialAuthorizationV2 {
	t.Helper()
	routeCallDigest, err := modelinvoker.DigestGovernedModelTurnRouteCallV2(fixture.call)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := modelinvoker.SealInvocationMaterialAuthorizationV2(
		modelinvoker.InvocationMaterialAuthorizationV2{
			PreparedRef:          fixture.prepared.Ref(),
			CurrentRef:           fixture.current.Ref(),
			RouteCallDigest:      routeCallDigest,
			SourceLineage:        lineage,
			ProviderInjectionRef: fixture.closure.ProviderInjection,
			RouteRef:             fixture.closure.Route,
			ProfileRef:           fixture.closure.Profile,
			AuthorizedUnixNano:   fixtureNow.UnixNano(),
			ExpiresUnixNano:      fixtureNow.Add(20 * time.Minute).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return authorization
}

func directMaterialV2(
	t *testing.T,
	fixture fixtureV2,
	authorization modelinvoker.InvocationMaterialAuthorizationV2,
) modelinvoker.InvocationMaterialV2 {
	t.Helper()
	material, err := modelinvoker.SealInvocationMaterialV2(
		modelinvoker.InvocationMaterialV2{
			PreparedRef:          fixture.prepared.Ref(),
			UnifiedRequestDigest: fixture.prepared.UnifiedRequestDigest,
			PreparedPlanDigest:   fixture.prepared.PreparedPlanDigest,
			RouteDigest:          fixture.prepared.RouteDigest,
			ProfileDigest:        fixture.prepared.ProfileDigest,
			Authorization:        authorization,
			Call:                 fixture.call,
			CreatedUnixNano:      fixtureNow.Add(time.Second).UnixNano(),
			ExpiresUnixNano:      fixtureNow.Add(10 * time.Minute).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return material
}

func ensureFixtureV2(
	store *modelsqlite.Store,
	fixture fixtureV2,
) (modelinvoker.InvocationMaterialV2, error) {
	return modelinvoker.AuthorizeAndEnsureInvocationMaterialV2(
		context.Background(),
		fixture.authorizer,
		store,
		fixture.prepared,
		fixture.current,
		fixture.call,
		fixture.closure,
		func() time.Time { return fixtureNow },
	)
}

func exactReadersForFixtureV2(fixture fixtureV2) exactReadersV2 {
	return exactReadersV2{
		expectedInjection: fixture.closure.SourceLineage.ExpectedInjectionDigest,
		compiledTools:     fixture.closure.SourceLineage.CompiledToolsDigest,
		checkedUnixNano:   fixtureNow.Add(-30 * time.Second).UnixNano(),
		expiresUnixNano:   fixtureNow.Add(30 * time.Minute).UnixNano(),
	}
}

func mustAuthorizerV2(
	t *testing.T,
	config modelinvoker.InvocationMaterialAuthorizerConfigV2,
) *modelinvoker.InvocationMaterialAuthorizerV2 {
	t.Helper()
	authorizer, err := modelinvoker.NewInvocationMaterialAuthorizerV2(config)
	if err != nil {
		t.Fatal(err)
	}
	return authorizer
}

func clockSequenceV2(values ...time.Time) func() time.Time {
	index := 0
	return func() time.Time {
		if len(values) == 0 {
			return time.Time{}
		}
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}

func mustSealLineageV2(
	t *testing.T,
	lineage modelinvoker.InvocationMaterialSourceLineageV2,
) modelinvoker.InvocationMaterialSourceLineageV2 {
	t.Helper()
	lineage.Digest = ""
	sealed, err := modelinvoker.SealInvocationMaterialSourceLineageV2(lineage)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func openStoreV2(t *testing.T, path string) *modelsqlite.Store {
	t.Helper()
	store, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func snapshotLegacySQLiteObjectsV2(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`
		SELECT type,name,tbl_name,COALESCE(sql,'')
		FROM sqlite_master
		WHERE name NOT LIKE 'sqlite_%'
		  AND name NOT LIKE 'invocation_material_v2_%'
		ORDER BY type,name
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var snapshot []string
	for rows.Next() {
		var kind, name, table, definition string
		if err := rows.Scan(&kind, &name, &table, &definition); err != nil {
			t.Fatal(err)
		}
		snapshot = append(snapshot, kind+"\x00"+name+"\x00"+table+"\x00"+definition)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func snapshotLegacySQLiteLedgerV2(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`
		SELECT version,digest,applied_unix_nano
		FROM model_invoker_schema
		WHERE version<=2
		ORDER BY version
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var snapshot []string
	for rows.Next() {
		var version, applied int64
		var digest string
		if err := rows.Scan(&version, &digest, &applied); err != nil {
			t.Fatal(err)
		}
		snapshot = append(snapshot, fmt.Sprintf("%d\x00%s\x00%d", version, digest, applied))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func exactRefV2(domain, id string, digest core.Digest) modelinvoker.InvocationMaterialExactSourceRefV1 {
	return exactOwnedRefV2(
		ownerV2(domain, id+"-owner"),
		domain+"-source",
		id,
		digest,
	)
}

func exactOwnedRefV2(
	owner core.OwnerRef,
	kind string,
	id string,
	digest core.Digest,
) modelinvoker.InvocationMaterialExactSourceRefV1 {
	return modelinvoker.InvocationMaterialExactSourceRefV1{
		Owner:    owner,
		Kind:     kind,
		ID:       id,
		Revision: 1,
		Digest:   digest,
	}
}

func ownerV2(domain, id string) core.OwnerRef {
	return core.OwnerRef{Domain: domain, ID: core.OwnerID(id)}
}

func digestV2(value string) core.Digest {
	return core.DigestBytes([]byte(value))
}
