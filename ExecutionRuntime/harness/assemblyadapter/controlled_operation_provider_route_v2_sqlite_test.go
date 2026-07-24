package assemblyadapter

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type routeSQLiteClockV2 struct {
	mu       sync.Mutex
	current  time.Time
	sequence []time.Time
}

func (c *routeSQLiteClockV2) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sequence) != 0 {
		value := c.sequence[0]
		c.sequence = c.sequence[1:]
		return value
	}
	return c.current
}

func (c *routeSQLiteClockV2) Set(value time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = value
	c.sequence = nil
}

func (c *routeSQLiteClockV2) SetSequence(values ...time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sequence = append([]time.Time(nil), values...)
}

func openSQLiteRouteStoreV2(t *testing.T, path string, clock func() time.Time) *SQLiteControlledOperationProviderRouteStoreV2 {
	t.Helper()
	store, err := OpenSQLiteControlledOperationProviderRouteStoreV2(context.Background(), SQLiteControlledOperationProviderRouteStoreConfigV2{
		Path: path, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func routeOwnerPublicationV2(t *testing.T, now time.Time) ControlledOperationProviderRouteOwnerArtifactPublicationV2 {
	t.Helper()
	declaration := routeDeclarationV2(t)
	compile := routeCompileResultV2(t, declaration)
	bindings := routeBindingsV2(declaration)
	wiring := routeInventoryV2(t, now, declaration, bindings)
	wiring.ActiveRoutes[0].TransportIdentity = compile.ProviderTransportIdentity
	wiring.ActiveRoutes[0].ProviderIdentity = compile.ProviderIdentity
	wiring.Digest = ""
	var err error
	wiring, err = assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(wiring)
	if err != nil {
		t.Fatal(err)
	}
	record := wiring.ActiveRoutes[0]
	activeDigest, err := core.CanonicalJSONDigest(
		"praxis.harness.controlled-operation-provider-route",
		assemblycontract.ControlledOperationProviderRouteContractVersionV2,
		"ControlledOperationProviderActiveRouteRecordV2",
		record,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ControlledOperationProviderRouteOwnerArtifactPublicationV2{
		Key: ControlledOperationProviderRouteConformanceKeyV2{
			CompileDigest: compile.CompileDigest,
			BindingSetID:  bindings[0].BindingSetID,
			ActiveRouteID: record.RouteID,
			Revision:      1,
		},
		Compile: compile,
		Association: runtimeports.GenerationBindingAssociationRefV1{
			ID: "sqlite-route-owner-association", Revision: 1, Digest: routeDigestV2("sqlite-route-owner-association"),
		},
		ActiveRoute: ControlledOperationProviderActiveRouteCurrentV2{
			Ref: ControlledOperationProviderActiveRouteCurrentRefV2{
				ActiveRouteID: record.RouteID, Revision: 1, Digest: activeDigest,
			},
			Record: record, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
		},
		Wiring: wiring, Bindings: bindings,
	}
}

func successorRouteConformanceV2(t *testing.T, current assemblycontract.ControlledOperationProviderRouteConformanceV2, checked time.Time, ordinal int) assemblycontract.ControlledOperationProviderRouteConformanceV2 {
	t.Helper()
	next := current
	next.Generation.Revision += core.Revision(ordinal)
	next.Generation.Digest = routeDigestV2(fmt.Sprintf("sqlite-generation-successor-%d", ordinal))
	next.BindingSetRevision += core.Revision(ordinal)
	next.BindingSetDigest = routeDigestV2(fmt.Sprintf("sqlite-binding-set-successor-%d", ordinal))
	next.BindingSetCurrentnessDigest = routeDigestV2(fmt.Sprintf("sqlite-binding-current-successor-%d", ordinal))
	bindings := []*runtimeports.ProviderBindingRefV2{
		&next.ToolAdapterBinding, &next.GatewayBinding, &next.ProviderTransportBinding,
		&next.PreparedReaderBinding, &next.BoundaryReaderBinding, &next.ProviderInspectBinding,
		&next.ProviderBinding,
	}
	for index, binding := range bindings {
		binding.BindingSetRevision = next.BindingSetRevision
		binding.ArtifactDigest = routeDigestV2(fmt.Sprintf("sqlite-binding-successor-%d-%d", ordinal, index))
	}
	next.CheckedUnixNano = checked.UnixNano()
	next.ExpiresUnixNano = checked.Add(time.Minute).UnixNano()
	next.ConformanceID = ""
	var err error
	next.ConformanceID, err = assemblycontract.DeriveControlledOperationProviderRouteConformanceIDV2(next.DeclarationRef.RouteID, next.Generation, next.BindingSetID)
	if err != nil {
		t.Fatal(err)
	}
	next.ConformanceDigest = ""
	next, err = assemblycontract.SealControlledOperationProviderRouteConformanceV2(next)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func successorRouteOwnerPublicationV2(t *testing.T, current ControlledOperationProviderRouteOwnerArtifactPublicationV2, checked time.Time) ControlledOperationProviderRouteOwnerArtifactPublicationV2 {
	t.Helper()
	next := current
	next.Key.Revision++
	next.ActiveRoute.Ref.Revision++
	next.ActiveRoute.CheckedUnixNano = checked.UnixNano()
	next.ActiveRoute.ExpiresUnixNano = checked.Add(time.Minute).UnixNano()
	next.Wiring.Revision++
	next.Wiring.CheckedUnixNano = checked.UnixNano()
	next.Wiring.ExpiresUnixNano = checked.Add(time.Minute).UnixNano()
	next.Wiring.Digest = ""
	var err error
	next.Wiring, err = assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(next.Wiring)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestSQLiteControlledOperationProviderRouteFreshClockRejectsExpiredPublishAndS2TTLCrossingV2(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	clock := &routeSQLiteClockV2{current: now}
	store := openSQLiteRouteStoreV2(t, filepath.Join(t.TempDir(), "fresh-clock.db"), clock.Now)
	declaration, conformance := routeConformanceV2(t, now)
	if _, err := store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), declaration, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishControlledOperationProviderRouteConformanceV2(context.Background(), conformance, 0); err != nil {
		t.Fatal(err)
	}
	current, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, 0)
	if err != nil {
		t.Fatal(err)
	}
	clock.SetSequence(time.Unix(0, current.ExpiresUnixNano-1), time.Unix(0, current.ExpiresUnixNano))
	if _, err := store.InspectControlledOperationProviderRouteCurrentV2(context.Background(), current.Ref); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("S2 TTL crossing = %v", err)
	}
	clock.Set(time.Unix(0, conformance.ExpiresUnixNano))
	if _, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, 0); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expired conformance idempotent publish = %v", err)
	}
}

func TestSQLiteControlledOperationProviderRouteActiveCurrentSupersedesOldFullRefV2(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 30, 0, 0, time.UTC)
	clock := &routeSQLiteClockV2{current: now}
	store := openSQLiteRouteStoreV2(t, filepath.Join(t.TempDir(), "active-current.db"), clock.Now)
	firstPublication := routeOwnerPublicationV2(t, now)
	firstRefs, err := store.PublishExactV2(context.Background(), firstPublication, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	clock.Set(now.Add(time.Second))
	secondPublication := successorRouteOwnerPublicationV2(t, firstPublication, now.Add(time.Second))
	secondRefs, err := store.PublishExactV2(context.Background(), secondPublication, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if secondRefs.ActiveRoute.Revision != firstRefs.ActiveRoute.Revision+1 {
		t.Fatalf("active current revision = %d", secondRefs.ActiveRoute.Revision)
	}
	if _, err := store.InspectControlledOperationProviderActiveRouteCurrentV2(context.Background(), firstRefs.ActiveRoute); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("superseded active ref remained current: %v", err)
	}
	if got, err := store.InspectControlledOperationProviderActiveRouteCurrentV2(context.Background(), secondRefs.ActiveRoute); err != nil || got.Ref != secondRefs.ActiveRoute {
		t.Fatalf("successor active current Inspect = %v", err)
	}
	stale := secondPublication
	stale.Key.Revision++
	stale.ActiveRoute.Ref.Revision += 2
	if _, err := store.PublishExactV2(context.Background(), stale, time.Now()); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("non-successor active full ref advanced current: %v", err)
	}
	clock.Set(time.Unix(0, secondPublication.ActiveRoute.ExpiresUnixNano))
	if _, err := store.InspectControlledOperationProviderActiveRouteCurrentV2(context.Background(), secondRefs.ActiveRoute); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expired active current = %v", err)
	}
}

func TestSQLiteControlledOperationProviderRouteFactsRestartLostReplyAndCurrentCASV2(t *testing.T) {
	now := time.Date(2026, 7, 23, 4, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "route.db")
	store := openSQLiteRouteStoreV2(t, path, func() time.Time { return now })
	declaration, conformance := routeConformanceV2(t, now)

	store.LoseNextDeclarationReplyV2()
	if _, err := store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), declaration, 0); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("declaration lost reply = %v", err)
	}
	if got, err := store.InspectControlledOperationProviderRouteDeclarationV2(context.Background(), declaration.RefV2()); err != nil || got != declaration {
		t.Fatalf("declaration exact recovery failed: %v", err)
	}
	store.LoseNextConformanceReplyV2()
	if _, err := store.PublishControlledOperationProviderRouteConformanceV2(context.Background(), conformance, 0); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("conformance lost reply = %v", err)
	}
	if got, err := store.InspectControlledOperationProviderRouteConformanceV2(context.Background(), conformance.RefV2()); err != nil || got != conformance {
		t.Fatalf("conformance exact recovery failed: %v", err)
	}
	store.LoseNextCurrentReplyV2()
	if _, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, 0); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("current lost reply = %v", err)
	}
	first, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.IntegrityCheckV2(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openSQLiteRouteStoreV2(t, path, func() time.Time { return now.Add(2 * time.Second) })
	if got, err := reopened.InspectControlledOperationProviderRouteCurrentV2(context.Background(), first.Ref); err != nil || got != first {
		t.Fatalf("current did not survive restart: %v", err)
	}
	secondConformance := successorRouteConformanceV2(t, conformance, now.Add(time.Second), 1)
	if _, err := reopened.PublishControlledOperationProviderRouteConformanceV2(context.Background(), secondConformance, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.PublishControlledOperationProviderRouteCurrentV2(context.Background(), secondConformance, 0); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale expectedPrevious advanced current: %v", err)
	}
	second, err := reopened.PublishControlledOperationProviderRouteCurrentV2(context.Background(), secondConformance, first.Ref.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if second.Ref.Revision != first.Ref.Revision+1 {
		t.Fatalf("current revision = %d", second.Ref.Revision)
	}
	if _, err := reopened.InspectControlledOperationProviderRouteCurrentV2(context.Background(), first.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("historical ref remained current: %v", err)
	}
	if _, err := reopened.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, second.Ref.Revision); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("ABA to older conformance was accepted: %v", err)
	}
}

func TestSQLiteControlledOperationProviderRouteOwnerArtifactsRestartCloneAndLostReplyV2(t *testing.T) {
	now := time.Date(2026, 7, 23, 5, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "owner.db")
	store := openSQLiteRouteStoreV2(t, path, func() time.Time { return now })
	publication := routeOwnerPublicationV2(t, now)
	store.LoseNextOwnerArtifactReplyV2()
	if _, err := store.PublishExactV2(context.Background(), publication, now); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("Owner artifact lost reply = %v", err)
	}
	refs, err := store.InspectControlledOperationProviderRouteOwnerRefsV2(context.Background(), publication.Key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishExactV2(context.Background(), publication, now); err != nil {
		t.Fatalf("exact Owner replay failed: %v", err)
	}
	firstCompile, err := store.InspectVerifiedControlledOperationProviderRouteCompileV2(context.Background(), refs.Compile)
	if err != nil {
		t.Fatal(err)
	}
	firstCompile.Manifest.Modules[0].ModuleID = "mutated"
	againCompile, err := store.InspectVerifiedControlledOperationProviderRouteCompileV2(context.Background(), refs.Compile)
	if err != nil {
		t.Fatal(err)
	}
	if againCompile.Manifest.Modules[0].ModuleID == "mutated" {
		t.Fatal("compile Inspect leaked an alias into durable state")
	}
	firstWiring, err := store.InspectControlledOperationProviderRouteWiringInventoryV2(context.Background(), refs.Wiring)
	if err != nil {
		t.Fatal(err)
	}
	firstWiring.ActiveRoutes[0].RouteID = "mutated"
	againWiring, err := store.InspectControlledOperationProviderRouteWiringInventoryV2(context.Background(), refs.Wiring)
	if err != nil {
		t.Fatal(err)
	}
	if againWiring.ActiveRoutes[0].RouteID == "mutated" {
		t.Fatal("wiring Inspect leaked an alias into durable state")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openSQLiteRouteStoreV2(t, path, func() time.Time { return now.Add(time.Second) })
	if got, err := reopened.InspectControlledOperationProviderRouteOwnerRefsV2(context.Background(), publication.Key); err != nil || got != refs {
		t.Fatalf("Owner refs did not survive restart: %v", err)
	}
	drifted := publication
	drifted.ActiveRoute.ExpiresUnixNano++
	if _, err := reopened.PublishExactV2(context.Background(), drifted, now); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same Owner key changed content: %v", err)
	}
}

func TestSQLiteControlledOperationProviderRouteConcurrentSameCanonicalLinearizesOnceV2(t *testing.T) {
	now := time.Date(2026, 7, 23, 6, 0, 0, 0, time.UTC)
	store := openSQLiteRouteStoreV2(t, filepath.Join(t.TempDir(), "concurrent.db"), func() time.Time { return now })
	declaration, conformance := routeConformanceV2(t, now)
	if _, err := store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), declaration, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishControlledOperationProviderRouteConformanceV2(context.Background(), conformance, 0); err != nil {
		t.Fatal(err)
	}
	var failures atomic.Int32
	refs := make(chan runtimeports.ControlledOperationProviderRouteCurrentRefV2, 64)
	var group sync.WaitGroup
	for range 64 {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, 0)
			if err != nil {
				failures.Add(1)
				return
			}
			refs <- value.Ref
		}()
	}
	group.Wait()
	close(refs)
	if failures.Load() != 0 {
		t.Fatalf("same canonical current failed %d times", failures.Load())
	}
	var first runtimeports.ControlledOperationProviderRouteCurrentRefV2
	for ref := range refs {
		if first == (runtimeports.ControlledOperationProviderRouteCurrentRefV2{}) {
			first = ref
		} else if ref != first {
			t.Fatal("same canonical current produced multiple refs")
		}
	}
	if first.Revision != 1 {
		t.Fatalf("linearized current revision = %d", first.Revision)
	}
}

func TestSQLiteControlledOperationProviderRouteConcurrentDifferentSuccessorsOnlyOneWinsV2(t *testing.T) {
	now := time.Date(2026, 7, 23, 6, 30, 0, 0, time.UTC)
	storeNow := now.Add(100 * time.Millisecond)
	store := openSQLiteRouteStoreV2(t, filepath.Join(t.TempDir(), "concurrent-drift.db"), func() time.Time { return storeNow })
	declaration, conformance := routeConformanceV2(t, now)
	if _, err := store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), declaration, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishControlledOperationProviderRouteConformanceV2(context.Background(), conformance, 0); err != nil {
		t.Fatal(err)
	}
	first, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, 0)
	if err != nil {
		t.Fatal(err)
	}
	successors := make([]assemblycontract.ControlledOperationProviderRouteConformanceV2, 64)
	for index := range successors {
		successors[index] = successorRouteConformanceV2(t, conformance, now.Add(time.Duration(index+1)*time.Millisecond), index+1)
		if _, err := store.PublishControlledOperationProviderRouteConformanceV2(context.Background(), successors[index], 0); err != nil {
			t.Fatal(err)
		}
	}
	var successes atomic.Int32
	var conflicts atomic.Int32
	var group sync.WaitGroup
	for index := range successors {
		group.Add(1)
		go func(value assemblycontract.ControlledOperationProviderRouteConformanceV2) {
			defer group.Done()
			_, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), value, first.Ref.Revision)
			switch {
			case err == nil:
				successes.Add(1)
			case core.HasCategory(err, core.ErrorConflict):
				conflicts.Add(1)
			default:
				t.Errorf("different successor returned %v", err)
			}
		}(successors[index])
	}
	group.Wait()
	if successes.Load() != 1 || conflicts.Load() != 63 {
		t.Fatalf("different successors: successes=%d conflicts=%d", successes.Load(), conflicts.Load())
	}
}

func TestSQLiteControlledOperationProviderRouteRejectsContextClockAndCorruptRowsV2(t *testing.T) {
	now := time.Date(2026, 7, 23, 7, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "guards.db")
	store := openSQLiteRouteStoreV2(t, path, func() time.Time { return now })
	declaration, _ := routeConformanceV2(t, now)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.PublishControlledOperationProviderRouteDeclarationV2(canceled, declaration, 0); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("canceled mutation = %v", err)
	}
	if _, err := store.InspectControlledOperationProviderRouteDeclarationV2(canceled, declaration.RefV2()); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("canceled read = %v", err)
	}
	if _, err := store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), declaration, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE harness_route_declaration_history_v2 SET row_digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE route_id=?`, declaration.RouteID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectControlledOperationProviderRouteDeclarationV2(context.Background(), declaration.RefV2()); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("tampered row was accepted: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSQLiteControlledOperationProviderRouteStoreV2(context.Background(), SQLiteControlledOperationProviderRouteStoreConfigV2{
		Path: path, Clock: func() time.Time { return now.Add(-time.Nanosecond) },
	}); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback reopen = %v", err)
	}
}

func TestSQLiteControlledOperationProviderRouteRejectsCurrentIndexDigestDriftV2(t *testing.T) {
	now := time.Date(2026, 7, 23, 7, 30, 0, 0, time.UTC)
	store := openSQLiteRouteStoreV2(t, filepath.Join(t.TempDir(), "current-index-drift.db"), func() time.Time { return now })
	declaration, conformance := routeConformanceV2(t, now)
	if _, err := store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), declaration, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishControlledOperationProviderRouteConformanceV2(context.Background(), conformance, 0); err != nil {
		t.Fatal(err)
	}
	current, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, 0)
	if err != nil {
		t.Fatal(err)
	}
	store.db.SetMaxOpenConns(1)
	if _, err := store.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE harness_route_current_v2 SET digest=? WHERE current_id=?`, routeDigestV2("tampered-current-index"), current.Ref.CurrentID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectControlledOperationProviderRouteCurrentV2(context.Background(), current.Ref); err == nil {
		t.Fatal("tampered current index digest was accepted")
	}
}
