package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var testNow = time.Unix(2_300_000_000, 0)

func TestSQLiteReviewModelInvocationAssociationLostReplyExactRecoveryV1(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "review-association.db"))
	initial, terminal := reviewAssociationFixtureSQLiteV1(t, testNow)
	store.loseNextReplyForTest()
	if _, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), initial); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost Create reply=%v", err)
	}
	if got, err := store.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), initial.RefV1()); err != nil || !reflect.DeepEqual(got, initial) {
		t.Fatalf("Create exact recovery=%+v %v", got, err)
	}
	store.loseNextReplyForTest()
	request := hostports.ReviewModelInvocationAssociationCASRequestV1{Expected: initial.RefV1(), Next: terminal}
	if _, err := store.CompareAndSwapReviewModelInvocationAssociationV1(context.Background(), request); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost CAS reply=%v", err)
	}
	if got, err := store.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), terminal.RefV1()); err != nil || !reflect.DeepEqual(got, terminal) {
		t.Fatalf("CAS exact recovery=%+v %v", got, err)
	}
}

func TestOpenUpgradesVersionOneMetadataWithoutRewritingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade-v1.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE agent_host_schema(version INTEGER PRIMARY KEY,digest TEXT NOT NULL,applied_unix_nano INTEGER NOT NULL); INSERT INTO agent_host_schema(version,digest,applied_unix_nano) VALUES(1,'legacy-v1-digest',1)`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, path)
	defer store.Close()
	var legacy string
	if err = store.db.QueryRow(`SELECT digest FROM agent_host_schema WHERE version=1`).Scan(&legacy); err != nil || legacy != "legacy-v1-digest" {
		t.Fatalf("legacy schema proof changed: %q %v", legacy, err)
	}
	var current string
	if err = store.db.QueryRow(`SELECT digest FROM agent_host_schema WHERE version=3`).Scan(&current); err != nil || current != string(core.DigestBytes([]byte(schemaBaseV3))) {
		t.Fatalf("schema v3 proof missing: %q %v", current, err)
	}
	if err = store.db.QueryRow(`SELECT digest FROM agent_host_schema WHERE version=4`).Scan(&current); err != nil || current != string(core.DigestBytes([]byte(schemaV1))) {
		t.Fatalf("schema v4 proof missing: %q %v", current, err)
	}
	if err = store.db.QueryRow(`SELECT digest FROM agent_host_schema WHERE version=5`).Scan(&current); err != nil || current != string(core.DigestBytes([]byte(schemaV5))) {
		t.Fatalf("schema v5 proof missing: %q %v", current, err)
	}
	if err = store.db.QueryRow(`SELECT digest FROM agent_host_schema WHERE version=6`).Scan(&current); err != nil || current != string(core.DigestBytes([]byte(schemaV6))) {
		t.Fatalf("schema v6 proof missing: %q %v", current, err)
	}
}

func TestOpenRejectsHistoricalSchemaProofDriftBeforePublishingV6(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema-drift.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE agent_host_schema(version INTEGER PRIMARY KEY,digest TEXT NOT NULL,applied_unix_nano INTEGER NOT NULL); INSERT INTO agent_host_schema(version,digest,applied_unix_nano) VALUES(4,'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',1)`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Open(context.Background(), Config{Path: path, Owner: core.OwnerRef{Domain: "praxis.agent-host", ID: "schema-owner"}, Clock: func() time.Time { return testNow }})
	if !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("historical schema proof drift=%v", err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM agent_host_schema WHERE version=6`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("drifted migration published v6 count=%d err=%v", count, err)
	}
}

func TestStoreInterfacesWALRestartAndPermanentClaimConflict(t *testing.T) {
	var _ hostports.HostStartClaimPortV1 = (*Store)(nil)
	var _ hostports.JournalFactPortV2 = (*Store)(nil)
	var _ hostports.SystemReadyFactPortV2 = (*Store)(nil)
	var _ hostports.SystemReadyAttemptFactPortV2 = (*Store)(nil)
	var _ hostports.SystemReadyAvailabilitySourceV2 = (*Store)(nil)
	var _ hostports.CleanupAttemptFactPortV2 = (*Store)(nil)

	path := filepath.Join(t.TempDir(), "agent-host.db")
	store := openTestStore(t, path)
	claim := claimFixture(t, "host-1", "start-1", "config-a")
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	if err := store.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	got, err := reopened.InspectHostStartClaimV1(context.Background(), claim.HostID, claim.StartID)
	if err != nil || !contract.SameHostStartClaimV1(got, claim) {
		t.Fatalf("restart lost claim: %+v %v", got, err)
	}
	changed := claimFixture(t, claim.HostID, claim.StartID, "config-b")
	if _, err = reopened.ClaimOrInspectHostStartV1(context.Background(), changed); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("permanent conflict domain accepted changed content: %v", err)
	}
	ref, _ := claim.CurrentRefV1()
	ref.Digest = digestV1(t, "other-claim")
	if _, err = reopened.InspectHostStartClaimCurrentV1(context.Background(), ref); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("claim exact ref drift accepted: %v", err)
	}
}

func TestClaimLostReplyRecoversByExactInspect(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "claim.db"))
	claim := claimFixture(t, "host-lost", "start-lost", "config-lost")
	store.loseNextReplyForTest()
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost reply = %v", err)
	}
	ref, _ := claim.CurrentRefV1()
	got, err := store.InspectHostStartClaimCurrentV1(context.Background(), ref)
	if err != nil || !contract.SameHostStartClaimV1(got, claim) {
		t.Fatalf("exact inspect did not recover committed claim: %+v %v", got, err)
	}
}

func TestJournalRequiresClaimPersistsCASAndLostReply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.db")
	store := openTestStore(t, path)
	claim := claimFixture(t, "host-journal", "start-journal", "config-journal")
	initial := journalFixture(t, claim)
	if _, err := store.CreateHostJournalV2(context.Background(), initial); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("journal without claim = %v", err)
	}
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateHostJournalV2(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	next := journalSuccessor(t, initial)
	expected, _ := initial.RefV2()
	store.loseNextReplyForTest()
	if _, err := store.CompareAndSwapHostJournalV2(context.Background(), expected, next); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost journal CAS reply = %v", err)
	}
	recovered, err := store.InspectHostJournalV2(context.Background(), initial.HostID, initial.StartID)
	if err != nil || recovered.Digest != next.Digest || recovered.Revision != next.Revision {
		t.Fatalf("journal recovery = %+v %v", recovered, err)
	}
	if _, err = store.CompareAndSwapHostJournalV2(context.Background(), expected, next); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("stale CAS replay succeeded: %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	got, err := reopened.InspectHostJournalV2(context.Background(), initial.HostID, initial.StartID)
	if err != nil || got.Digest != next.Digest {
		t.Fatalf("restart lost journal: %+v %v", got, err)
	}
}

func TestSystemReadyFactCurrentAvailabilityRestartAndExactRefs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready.db")
	store := openTestStore(t, path)
	claim := claimFixture(t, "host-1", "start-1", "host-config")
	fact := readyFactFixture(t, claim)
	if _, err := store.CreateSystemReadyFactV2(context.Background(), fact); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("SystemReady without claim = %v", err)
	}
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	store.loseNextReplyForTest()
	if _, err := store.CreateSystemReadyFactV2(context.Background(), fact); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost Fact reply = %v", err)
	}
	if got, err := store.InspectSystemReadyFactV2(context.Background(), fact.Ref); err != nil || got.Digest != fact.Digest {
		t.Fatalf("Fact recovery = %+v %v", got, err)
	}
	current := readyCurrentFixture(t, fact)
	if _, err := store.CreateSystemReadyCurrentV2(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	projection, err := current.ToAgentExecutionAvailabilityV1(testOwner())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := store.InspectSystemReadyCurrentForAvailabilityV2(context.Background(), projection.Ref); err != nil || got.Ref != current.Ref {
		t.Fatalf("availability exact read = %+v %v", got, err)
	}
	renewed := renewCurrentFixture(t, current)
	store.loseNextReplyForTest()
	if _, err := store.CompareAndSwapSystemReadyCurrentV2(context.Background(), current.Ref, renewed); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost Current reply = %v", err)
	}
	if got, err := store.InspectSystemReadyCurrentV2(context.Background(), renewed.Ref); err != nil || got.Ref != renewed.Ref {
		t.Fatalf("Current recovery = %+v %v", got, err)
	}
	wrong := renewed.Ref
	wrong.Digest = coreDigest(t, "wrong-current")
	if _, err := store.InspectSystemReadyCurrentV2(context.Background(), wrong); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("Current ref drift accepted: %v", err)
	}
	wrongAvailability := projection.Ref
	wrongAvailability.Owner = core.OwnerRef{Domain: "praxis.other", ID: core.OwnerID("other")}
	if _, err := store.InspectSystemReadyCurrentForAvailabilityV2(context.Background(), wrongAvailability); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("availability owner drift accepted: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	if got, err := reopened.InspectSystemReadyCurrentV2(context.Background(), renewed.Ref); err != nil || got.Ref != renewed.Ref {
		t.Fatalf("restart lost Current: %+v %v", got, err)
	}
}

func TestSystemReadyAttemptRequiresClaimLostReplyCASAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready-attempt.db")
	store := openTestStore(t, path)
	claim := claimFixture(t, "host-ready-attempt", "start-ready-attempt", "config-ready-attempt")
	attempt := readyAttemptFixture(t, claim)
	if _, err := store.CreateSystemReadyAttemptV2(context.Background(), attempt); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("attempt without claim = %v", err)
	}
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	store.loseNextReplyForTest()
	if _, err := store.CreateSystemReadyAttemptV2(context.Background(), attempt); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost attempt create reply = %v", err)
	}
	if got, err := store.InspectSystemReadyAttemptV2(context.Background(), attempt.StepKey); err != nil || got.Digest != attempt.Digest {
		t.Fatalf("attempt create recovery = %+v %v", got, err)
	}
	if _, err := store.CreateSystemReadyAttemptV2(context.Background(), attempt); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("exact attempt replay was granted another execution token: %v", err)
	}
	next := attempt
	next.Revision++
	next.State = contract.SystemReadyAttemptOutcomeUnknownV2
	next.UpdatedUnixNano++
	next.Digest = ""
	var err error
	next, err = contract.SealSystemReadyAttemptFactV2(next)
	if err != nil {
		t.Fatal(err)
	}
	store.loseNextReplyForTest()
	if _, err = store.CompareAndSwapSystemReadyAttemptV2(context.Background(), attempt.RefV2(), next); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost attempt CAS reply = %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	got, err := reopened.InspectSystemReadyAttemptV2(context.Background(), next.StepKey)
	if err != nil || got.Digest != next.Digest || got.Revision != next.Revision {
		t.Fatalf("restart lost attempt: %+v %v", got, err)
	}
	if _, err = reopened.CompareAndSwapSystemReadyAttemptV2(context.Background(), attempt.RefV2(), next); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("stale attempt CAS replay succeeded: %v", err)
	}
}

func TestSixtyFourIndependentStoresLinearizeSystemReadyAttemptCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready-attempt-64.db")
	seed := openTestStore(t, path)
	claim := claimFixture(t, "host-ready-attempt-64", "start-ready-attempt-64", "config-ready-attempt-64")
	if _, err := seed.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	const workers = 64
	stores := make([]*Store, workers)
	for i := range stores {
		stores[i] = openTestStore(t, path)
		defer stores[i].Close()
	}
	base := readyAttemptFixture(t, claim)
	var successes atomic.Uint64
	var conflicts atomic.Uint64
	var wg sync.WaitGroup
	for i := range stores {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			candidate := base
			candidate.CreatedUnixNano += int64(index)
			candidate.UpdatedUnixNano = candidate.CreatedUnixNano
			candidate.Digest = ""
			candidate, err := contract.SealSystemReadyAttemptFactV2(candidate)
			if err != nil {
				t.Errorf("seal %d: %v", index, err)
				return
			}
			_, err = stores[index].CreateSystemReadyAttemptV2(context.Background(), candidate)
			switch {
			case err == nil:
				successes.Add(1)
			case contract.HasCode(err, contract.ErrorConflict):
				conflicts.Add(1)
			default:
				t.Errorf("create %d: %v", index, err)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 || conflicts.Load() != workers-1 {
		t.Fatalf("successes=%d conflicts=%d", successes.Load(), conflicts.Load())
	}
}

func TestSixtyFourIndependentStoresLinearizeClaimAndCurrentCAS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	seed := openTestStore(t, path)
	claim := claimFixture(t, "host-64", "start-64", "config-64")
	if _, err := seed.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	fact := readyFactFixtureForSubject(t, claim)
	if _, err := seed.CreateSystemReadyFactV2(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	current := readyCurrentFixture(t, fact)
	if _, err := seed.CreateSystemReadyCurrentV2(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	const workers = 64
	stores := make([]*Store, workers)
	for i := range stores {
		stores[i] = openTestStore(t, path)
		defer stores[i].Close()
	}
	next := renewCurrentFixture(t, current)
	var successes atomic.Int64
	var conflicts atomic.Int64
	var wg sync.WaitGroup
	for i := range stores {
		wg.Add(1)
		go func(store *Store) {
			defer wg.Done()
			_, err := store.CompareAndSwapSystemReadyCurrentV2(context.Background(), current.Ref, next)
			switch {
			case err == nil:
				successes.Add(1)
			case contract.HasCode(err, contract.ErrorConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected CAS error: %v", err)
			}
		}(stores[i])
	}
	wg.Wait()
	if successes.Load() != 1 || conflicts.Load() != workers-1 {
		t.Fatalf("CAS linearization successes=%d conflicts=%d", successes.Load(), conflicts.Load())
	}
}

func TestStoreTypedNilCanceledContextsDeepCopiesAndCorruption(t *testing.T) {
	var nilStore *Store
	claim := claimFixture(t, "host-nil", "start-nil", "config-nil")
	if _, err := nilStore.ClaimOrInspectHostStartV1(context.Background(), claim); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("typed nil mutation = %v", err)
	}
	if _, err := nilStore.InspectHostStartClaimV1(context.Background(), claim.HostID, claim.StartID); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("typed nil read = %v", err)
	}
	store := openTestStore(t, filepath.Join(t.TempDir(), "strict.db"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ClaimOrInspectHostStartV1(ctx, claim); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("canceled mutation = %v", err)
	}
	if _, err := store.InspectHostStartClaimV1(ctx, claim.HostID, claim.StartID); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("canceled read = %v", err)
	}
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	journal := journalFixture(t, claim)
	if _, err := store.CreateHostJournalV2(context.Background(), journal); err != nil {
		t.Fatal(err)
	}
	got, err := store.InspectHostJournalV2(context.Background(), claim.HostID, claim.StartID)
	if err != nil {
		t.Fatal(err)
	}
	got.Operations = append(got.Operations, contract.HostOperationAttemptV2{})
	again, err := store.InspectHostJournalV2(context.Background(), claim.HostID, claim.StartID)
	if err != nil || len(again.Operations) != 0 {
		t.Fatalf("returned Journal aliased durable row: %+v %v", again, err)
	}
	if _, err = store.db.Exec(`UPDATE agent_host_start_claims SET canonical_json=? WHERE host_id=? AND start_id=?`, []byte(`{"host_id":"host-nil","host_id":"forged"}`), claim.HostID, claim.StartID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectHostStartClaimV1(context.Background(), claim.HostID, claim.StartID); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("strict JSON corruption accepted: %v", err)
	}
}

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(context.Background(), Config{Path: path, Owner: testOwner(), Clock: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func (s *Store) loseNextReplyForTest() {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	s.loseNextReply = true
}

func testOwner() core.OwnerRef {
	return core.OwnerRef{Domain: "praxis.agent-host", ID: core.OwnerID("system-ready-owner")}
}

func claimFixture(t *testing.T, hostID, startID, config string) contract.HostStartClaimV1 {
	t.Helper()
	value, err := contract.SealHostStartClaimV1(contract.HostStartClaimV1{ContractVersion: contract.HostStartClaimContractVersionV1, HostContractVersion: contract.ContractVersionV2, HostID: hostID, StartID: startID, ConfigDigest: digestV1(t, config), DefinitionSourceRef: exactRef(t, "praxis.agent-definition/source-current", "definition-source-1"), RequestedOperation: contract.HostStartOperationStartV1, CreatedUnixNano: testNow.UnixNano(), ExpiresUnixNano: testNow.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func journalFixture(t *testing.T, claim contract.HostStartClaimV1) contract.HostJournalV2 {
	t.Helper()
	claimRef, err := claim.RefV1()
	if err != nil {
		t.Fatal(err)
	}
	value, err := contract.SealHostJournalV2(contract.HostJournalV2{ContractVersion: contract.HostJournalContractVersionV2, HostID: claim.HostID, StartID: claim.StartID, Revision: 1, Phase: contract.HostAcceptedV2, StartClaimRef: claimRef, ConfigDigest: claim.ConfigDigest, CreatedUnixNano: testNow.UnixNano(), UpdatedUnixNano: testNow.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func journalSuccessor(t *testing.T, current contract.HostJournalV2) contract.HostJournalV2 {
	t.Helper()
	next := current
	next.Revision++
	next.Phase = contract.HostValidatingV2
	next.UpdatedUnixNano++
	next.Digest = ""
	sealed, err := contract.SealHostJournalV2(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func readyFactFixture(t *testing.T, claim contract.HostStartClaimV1) contract.SystemReadyFactV2 {
	t.Helper()
	return readyFactFixtureForSubject(t, claim)
}

func readyFactFixtureForSubject(t *testing.T, claim contract.HostStartClaimV1) contract.SystemReadyFactV2 {
	t.Helper()
	expires := testNow.Add(time.Hour)
	ownerCurrent := func(domain, id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: domain, ID: core.OwnerID("owner-" + id)}, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: coreDigest(t, domain+":"+id), ExpiresUnixNano: expires.UnixNano()}
	}
	claimRef, err := claim.CurrentRefV1()
	if err != nil {
		t.Fatal(err)
	}
	component := contract.ComponentProductionCurrentV2{Domain: "praxis.test/component", ReleaseCurrent: ownerCurrent("praxis.release", "release-1"), ConstructedComponent: exactRef(t, "praxis.component/instance", "component-1"), Binding: runtimeports.BindingAdmissionBindingRefV1{ComponentID: "praxis.test/component", ID: "binding-1", Revision: 1, Digest: coreDigest(t, "binding-1"), ExpiresUnixNano: expires.UnixNano()}, GenerationCurrent: ownerCurrent("praxis.harness", "generation-1"), ActivationCurrent: ownerCurrent("praxis.runtime", "activation-1"), ProductionCurrent: ownerCurrent("praxis.test", "production-current-1")}
	fact, err := contract.SealSystemReadyFactV2(contract.SystemReadyFactV2{Ref: contract.SystemReadyFactRefV2{Revision: 1, ExpiresUnixNano: testNow.Add(30 * time.Minute).UnixNano()}, HostID: claim.HostID, StartID: claim.StartID, HostStartClaim: claimRef, DefinitionCurrent: ownerCurrent("praxis.agent-definition", "definition-1"), PlanCurrent: ownerCurrent("praxis.agent-assembler", "plan-1"), AssemblyCurrent: ownerCurrent("praxis.harness", "assembly-1"), BindingSetCurrent: ownerCurrent("praxis.runtime", "binding-set-1"), ActivationCurrent: ownerCurrent("praxis.runtime", "activation-1"), GenerationBindingCurrent: ownerCurrent("praxis.runtime", "generation-binding-1"), ApplicationStartCurrent: ownerCurrent("praxis.application", "start-current-1"), SandboxLeaseCurrent: ownerCurrent("praxis.sandbox", "sandbox-lease-1"), SandboxActiveCurrent: ownerCurrent("praxis.sandbox", "sandbox-active-1"), ExecutionReadyCurrent: ownerCurrent("praxis.harness", "execution-ready-1"), SupervisionPolicyCurrent: ownerCurrent("praxis.runtime", "supervision-policy-1"), Components: []contract.ComponentProductionCurrentV2{component}, MinimumReadyWindowNanos: int64(10 * time.Minute), CheckedUnixNano: testNow.UnixNano(), ExpiresUnixNano: testNow.Add(30 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func readyCurrentFixture(t *testing.T, fact contract.SystemReadyFactV2) contract.SystemReadyCurrentV2 {
	t.Helper()
	value, err := contract.SealSystemReadyCurrentV2(contract.SystemReadyCurrentV2{Ref: contract.SystemReadyCurrentRefV2{ID: contract.DeriveSystemReadyCurrentIDV2(fact.HostID, fact.StartID), Revision: 1, Epoch: 1}, HostID: fact.HostID, StartID: fact.StartID, FactRef: fact.Ref, State: contract.SystemReadyCurrentReadyV2, CheckedUnixNano: testNow.UnixNano(), ExpiresUnixNano: fact.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func readyAttemptFixture(t *testing.T, claim contract.HostStartClaimV1) contract.SystemReadyAttemptFactV2 {
	t.Helper()
	fact := readyFactFixture(t, claim)
	current := readyCurrentFixture(t, fact)
	request, err := contract.SealSystemReadyEnsureRequestV2(contract.SystemReadyEnsureRequestV2{
		AttemptID: "ready-attempt-1", HostID: fact.HostID, StartID: fact.StartID, Claim: fact.HostStartClaim,
		Definition: fact.DefinitionCurrent, Plan: fact.PlanCurrent, Assembly: fact.AssemblyCurrent,
		BindingSet: fact.BindingSetCurrent, Activation: fact.ActivationCurrent,
		GenerationBinding: fact.GenerationBindingCurrent, ApplicationStart: fact.ApplicationStartCurrent,
		SandboxLease: fact.SandboxLeaseCurrent, SandboxActive: fact.SandboxActiveCurrent,
		ExecutionReady: fact.ExecutionReadyCurrent, SupervisionPolicy: fact.SupervisionPolicyCurrent,
		Components: fact.Components, MinimumReadyWindowNanos: fact.MinimumReadyWindowNanos, AvailabilityEpoch: current.Ref.Epoch,
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := contract.SealSystemReadyAttemptFactV2(contract.SystemReadyAttemptFactV2{
		StepKey: contract.NewSystemReadyAttemptStepKeyV2(request.HostID, request.StartID, request.AttemptID), Revision: 1,
		Request: request, FactCandidate: fact, CurrentCandidate: current, State: contract.SystemReadyAttemptIntentRecordedV2,
		CreatedUnixNano: testNow.UnixNano(), UpdatedUnixNano: testNow.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return attempt
}

func renewCurrentFixture(t *testing.T, current contract.SystemReadyCurrentV2) contract.SystemReadyCurrentV2 {
	t.Helper()
	next := current
	next.Ref.Revision++
	next.CheckedUnixNano++
	next.Ref.Digest, next.ProjectionDigest = "", ""
	sealed, err := contract.SealSystemReadyCurrentV2(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func exactRef(t *testing.T, kind, id string) contract.ExactRefV1 {
	t.Helper()
	return contract.ExactRefV1{Kind: kind, ID: id, Revision: 1, Digest: digestV1(t, kind+":"+id)}
}

func digestV1(t *testing.T, value string) contract.DigestV1 {
	t.Helper()
	digest, err := contract.DigestJSONV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func coreDigest(t *testing.T, value string) core.Digest {
	t.Helper()
	digest, err := core.CanonicalJSONDigest("fixture", "1.0.0", "Fixture", value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func reviewAssociationFixtureSQLiteV1(t *testing.T, now time.Time) (contract.ReviewModelInvocationAssociationFactV1, contract.ReviewModelInvocationAssociationFactV1) {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	requestDigest := digest("review-command")
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{
		InvocationID: "review-invocation", InvocationDigest: requestDigest, UnifiedRequestDigest: requestDigest,
		RequestToolsDigest: digest("tools"), PreparedPlanDigest: digest("plan"), RouteDigest: digest("route"), ProfileDigest: digest("profile"),
		ActualToolSurfaceDigest: digest("surface"), ActualProviderInjectionDigest: digest("provider"),
		CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{ContractVersion: "1.0.0", ID: "capability", Revision: 1, Digest: digest("capability")},
		RegistrySnapshotRef:   runtimeports.RegistrySnapshotRefV1{Owner: core.OwnerRef{Domain: "registry", ID: "owner"}, ContractVersion: "1.0.0", ID: "registry", Revision: 1, Digest: digest("registry")},
		CreatedUnixNano:       now.Add(-time.Minute).UnixNano(), NotAfterUnixNano: now.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{
		Prepared: prepared.Ref(), CapabilitySnapshotRef: prepared.CapabilitySnapshotRef, RegistrySnapshotRef: prepared.RegistrySnapshotRef,
		ActualToolSurfaceDigest: prepared.ActualToolSurfaceDigest, ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest,
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(8 * time.Minute).UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	strict := true
	call := modelinvoker.RouteCall{RouteID: "openai.direct.payg.responses", Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}, Request: modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "review")}, ToolChoice: modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceNone}, Output: modelinvoker.OutputConstraint{Type: modelinvoker.OutputJSONSchema, Name: "review", Schema: []byte(`{"type":"object"}`), Strict: &strict}}}
	initial, err := contract.SealReviewModelInvocationAssociationV1(contract.ReviewModelInvocationAssociationFactV1{
		Subject:         contract.ReviewModelInvocationAssociationSubjectV1{TenantID: "tenant-a", ReviewAttempt: contract.ReviewAttemptExactCoordinateV1{ID: "attempt-a", Revision: 1, Digest: digest("attempt")}},
		Command:         modelinvoker.GovernedModelInvocationCommandV1{PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), AttemptRequestDigest: requestDigest, DispatchSequence: 1, ProviderAttemptOrdinal: 1, Call: call},
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := contract.SealReviewModelInvocationAssociationSuccessorV1(initial, contract.ReviewModelInvocationAssociationFactV1{State: contract.ReviewModelInvocationAssociationRevokedV1, CheckedUnixNano: now.Add(time.Second).UnixNano(), ExpiresUnixNano: initial.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	return initial, terminal
}
