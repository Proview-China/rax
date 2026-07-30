package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestDeploymentCurrentV2CreateAdvanceIdempotencyConflictABAAndRestart(t *testing.T) {
	var _ hostports.HostDeploymentCurrentRepositoryV2 = (*Store)(nil)
	if _, exposed := any((*Store)(nil)).(hostports.HostDeploymentCurrentReaderV2); exposed {
		t.Fatal("raw SQLite Store satisfies the public current Reader")
	}
	path := filepath.Join(t.TempDir(), "deployment-v2.db")
	store := openTestStore(t, path)
	first := storedDeploymentFixtureV2(t, "host-deployment-v2", "deployment-v2", 1, 1)
	got, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, first)
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("create=%+v err=%v", got, err)
	}
	if got, err = store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, first); err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("idempotent create=%+v err=%v", got, err)
	}
	next := storedDeploymentAdvanceFixtureV2(t, first, 2)
	if got, err = store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), first.Ref, next); err != nil || !reflect.DeepEqual(got, next) {
		t.Fatalf("advance=%+v err=%v", got, err)
	}
	if got, err = store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), first.Ref, next); err != nil || !reflect.DeepEqual(got, next) {
		t.Fatalf("idempotent lost-reply replay=%+v err=%v", got, err)
	}
	stale := storedDeploymentAdvanceFixtureV2(t, next, 3)
	if _, err = store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), first.Ref, stale); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("stale predecessor accepted: %v", err)
	}
	if historical, inspectErr := store.InspectStoredHostDeploymentExactV2(context.Background(), first.Ref); inspectErr != nil || !reflect.DeepEqual(historical, first) {
		t.Fatalf("append-only history lost first=%+v err=%v", historical, inspectErr)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	current, err := reopened.InspectStoredHostDeploymentCurrentV2(context.Background(), next.Ref.HostID, next.Ref.DeploymentID)
	if err != nil || !reflect.DeepEqual(current, next) {
		t.Fatalf("restart current=%+v err=%v", current, err)
	}
}

func TestDeploymentCurrentV2EightResourceHandlesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deployment-eight-handles.db")
	store := openTestStore(t, path)
	value := storedDeploymentFixtureV2(t, "host-eight", "deployment-eight", 1, 1)
	expires := value.ExpiresUnixNano
	for index := 0; index < 8; index++ {
		id := fmt.Sprintf("state-%d", index)
		value.ResourceHandles = append(value.ResourceHandles, runtimeports.ResourceHandleRefV1{
			Owner: core.OwnerRef{Domain: "praxis.host-test", ID: "resource-owner"},
			ID:    id, Revision: 1, Digest: core.DigestBytes([]byte("resource-" + id)),
			Kind: "praxis/sqlite", ScopeDigest: core.DigestBytes([]byte("scope-" + id)),
			ExpiresUnixNano: expires,
		})
	}
	value.Ref.Digest, value.ProjectionDigest = "", ""
	value, err := contract.SealHostDeploymentCurrentV2(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, value); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	got, err := reopened.InspectStoredHostDeploymentCurrentV2(context.Background(), value.Ref.HostID, value.Ref.DeploymentID)
	if err != nil || !reflect.DeepEqual(got, value) || len(got.ResourceHandles) != 8 {
		t.Fatalf("eight-handle round trip=%+v err=%v", got, err)
	}
}

func TestDeploymentCurrentV2RowAndPointerAxesFailClosed(t *testing.T) {
	t.Run("history row", func(t *testing.T) {
		store := openTestStore(t, filepath.Join(t.TempDir(), "row-splice.db"))
		value := storedDeploymentFixtureV2(t, "host-row-splice", "deployment-row-splice", 1, 1)
		if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, value); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(
			`UPDATE agent_host_deployment_current_history_v2 SET bootstrap_digest=? WHERE host_id=? AND deployment_id=? AND revision=?`,
			string(core.DigestBytes([]byte("other-bootstrap"))), value.Ref.HostID, value.Ref.DeploymentID, value.Ref.Revision,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := store.InspectStoredHostDeploymentExactV2(context.Background(), value.Ref); !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("history row splice accepted: %v", err)
		}
	})
	t.Run("current pointer", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "pointer-splice.db")
		store := openTestStore(t, path)
		value := storedDeploymentFixtureV2(t, "host-pointer-splice", "deployment-pointer-splice", 1, 1)
		if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, value); err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(0)")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(
			`UPDATE agent_host_deployment_current_v2 SET selection_digest=? WHERE host_id=? AND deployment_id=?`,
			string(core.DigestBytes([]byte("other-selection"))), value.Ref.HostID, value.Ref.DeploymentID,
		); err != nil {
			t.Fatal(err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
		reopened := openTestStore(t, path)
		if _, err = reopened.InspectStoredHostDeploymentCurrentV2(context.Background(), value.Ref.HostID, value.Ref.DeploymentID); err == nil {
			t.Fatal("current pointer splice accepted")
		}
	})
}

func TestDeploymentCurrentV2LostReplyExactInspectAndStrictJSON(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "deployment-lost.db"))
	first := storedDeploymentFixtureV2(t, "host-lost", "deployment-lost", 1, 1)
	store.loseNextReplyForTest()
	if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, first); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost reply=%v", err)
	}
	if got, err := store.InspectStoredHostDeploymentExactV2(context.Background(), first.Ref); err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("exact recovery=%+v err=%v", got, err)
	}
	if _, err := store.db.Exec(
		`UPDATE agent_host_deployment_current_history_v2
		    SET canonical_json=?,row_digest=row_digest
		  WHERE host_id=? AND deployment_id=? AND revision=?`,
		[]byte(`{"contract_version":"x","contract_version":"y"}`),
		first.Ref.HostID,
		first.Ref.DeploymentID,
		first.Ref.Revision,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectStoredHostDeploymentExactV2(context.Background(), first.Ref); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("duplicate-key JSON accepted: %v", err)
	}
}

func TestDeploymentCurrentV2SelectionAxesAreInHistoryAndPointer(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "deployment-selection-axes.db"))
	first := storedDeploymentFixtureV2(t, "host-axes", "deployment-axes", 1, 1)
	if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, first); err != nil {
		t.Fatal(err)
	}
	axes := []struct {
		name   string
		mutate func(*contract.HostDeploymentCurrentRefV2)
	}{
		{"id", func(ref *contract.HostDeploymentCurrentRefV2) { ref.PackageSelectionRef.SelectionID += "-other" }},
		{"revision", func(ref *contract.HostDeploymentCurrentRefV2) { ref.PackageSelectionRef.Revision++ }},
		{"digest", func(ref *contract.HostDeploymentCurrentRefV2) {
			ref.PackageSelectionRef.Digest = core.DigestBytes([]byte("selection-other"))
		}},
		{"expiry", func(ref *contract.HostDeploymentCurrentRefV2) { ref.PackageSelectionRef.ExpiresUnixNano++ }},
	}
	for _, axis := range axes {
		t.Run(axis.name, func(t *testing.T) {
			spliced := first.Ref
			axis.mutate(&spliced)
			if _, err := store.InspectStoredHostDeploymentExactV2(context.Background(), spliced); err == nil {
				t.Fatal("spliced selection axis resolved exact history")
			}
		})
	}
}

func TestDeploymentCurrentV2IdempotentReplayRejectsForgedExpectedAxes(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "deployment-forged-expected.db"))
	first := storedDeploymentFixtureV2(t, "host-forged", "deployment-forged", 1, 1)
	if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, first); err != nil {
		t.Fatal(err)
	}
	next := storedDeploymentAdvanceFixtureV2(t, first, 2)
	if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), first.Ref, next); err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name   string
		mutate func(*contract.HostDeploymentCurrentRefV2)
	}{
		{"revision", func(ref *contract.HostDeploymentCurrentRefV2) { ref.Revision++ }},
		{"digest", func(ref *contract.HostDeploymentCurrentRefV2) {
			ref.Digest = contract.DigestV1(core.DigestBytes([]byte("wrong-expected")))
		}},
		{"selection id", func(ref *contract.HostDeploymentCurrentRefV2) { ref.PackageSelectionRef.SelectionID += "-wrong" }},
		{"selection revision", func(ref *contract.HostDeploymentCurrentRefV2) { ref.PackageSelectionRef.Revision++ }},
		{"selection digest", func(ref *contract.HostDeploymentCurrentRefV2) {
			ref.PackageSelectionRef.Digest = core.DigestBytes([]byte("wrong-selection"))
		}},
		{"selection expiry", func(ref *contract.HostDeploymentCurrentRefV2) { ref.PackageSelectionRef.ExpiresUnixNano++ }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			forged := first.Ref
			mutation.mutate(&forged)
			if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), forged, next); !contract.HasCode(err, contract.ErrorConflict) {
				t.Fatalf("forged expected replay accepted: %v", err)
			}
		})
	}
}

func TestDeploymentCurrentV2PredecessorExpiryCrossingAndCanonicalCorruptionWriteZero(t *testing.T) {
	t.Run("expiry crossing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "deployment-expiry-crossing.db")
		store := openTestStore(t, path)
		first := storedDeploymentFixtureV2(t, "host-expiry", "deployment-expiry", 1, 1)
		first.Ref.PackageSelectionRef.ExpiresUnixNano = testNow.Add(5 * time.Second).UnixNano()
		first.Ref.ExpiresUnixNano = first.Ref.PackageSelectionRef.ExpiresUnixNano
		first.ExpiresUnixNano = first.Ref.ExpiresUnixNano
		first.Ref.Digest, first.ProjectionDigest = "", ""
		var err error
		first, err = contract.SealHostDeploymentCurrentV2(first)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, first); err != nil {
			t.Fatal(err)
		}
		next := contract.CloneHostDeploymentCurrentV2(first)
		next.Ref.Revision++
		next.Ref.PackageSelectionRef = packageSelectionRefFixtureV2(next.Ref.DeploymentID, 2, testNow.Add(time.Hour).UnixNano())
		next.Ref.ExpiresUnixNano = next.Ref.PackageSelectionRef.ExpiresUnixNano
		next.ExpiresUnixNano = next.Ref.ExpiresUnixNano
		next.Ref.Digest, next.ProjectionDigest = "", ""
		next, err = contract.SealHostDeploymentCurrentV2(next)
		if err != nil {
			t.Fatal(err)
		}
		var calls atomic.Int32
		store.clock = func() time.Time {
			if calls.Add(1) == 1 {
				return testNow
			}
			return time.Unix(0, first.ExpiresUnixNano)
		}
		if _, err = store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), first.Ref, next); !contract.HasCode(err, contract.ErrorPrecondition) {
			t.Fatalf("expired predecessor advance accepted: %v", err)
		}
		var count int
		if err = store.db.QueryRow(`SELECT COUNT(*) FROM agent_host_deployment_current_history_v2 WHERE host_id=? AND deployment_id=?`, first.Ref.HostID, first.Ref.DeploymentID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("expiry crossing wrote history count=%d err=%v", count, err)
		}
	})
	t.Run("canonical body corruption", func(t *testing.T) {
		store := openTestStore(t, filepath.Join(t.TempDir(), "deployment-predecessor-corrupt.db"))
		first := storedDeploymentFixtureV2(t, "host-corrupt", "deployment-corrupt", 1, 1)
		if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, first); err != nil {
			t.Fatal(err)
		}
		tampered := contract.CloneHostDeploymentCurrentV2(first)
		tampered.CheckedUnixNano++
		tampered.Ref.Digest, tampered.ProjectionDigest = "", ""
		tampered, err := contract.SealHostDeploymentCurrentV2(tampered)
		if err != nil {
			t.Fatal(err)
		}
		payload, rowDigest, err := encodeRow(deploymentCurrentRowV2, tampered)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.db.Exec(
			`UPDATE agent_host_deployment_current_history_v2 SET canonical_json=?,row_digest=? WHERE host_id=? AND deployment_id=? AND revision=?`,
			payload, rowDigest, first.Ref.HostID, first.Ref.DeploymentID, first.Ref.Revision,
		); err != nil {
			t.Fatal(err)
		}
		next := storedDeploymentAdvanceFixtureV2(t, first, 2)
		if _, err = store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), first.Ref, next); !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("corrupt canonical predecessor accepted: %v", err)
		}
		var count int
		if err = store.db.QueryRow(`SELECT COUNT(*) FROM agent_host_deployment_current_history_v2 WHERE host_id=? AND deployment_id=?`, first.Ref.HostID, first.Ref.DeploymentID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("corrupt predecessor wrote history count=%d err=%v", count, err)
		}
	})
}

func TestDeploymentCurrentV2SixtyFourIndependentStoresOneWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deployment-64.db")
	seed := openTestStore(t, path)
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	const workers = 64
	stores := make([]*Store, workers)
	candidates := make([]contract.HostDeploymentCurrentV2, workers)
	for index := range stores {
		stores[index] = openTestStore(t, path)
		candidates[index] = storedDeploymentFixtureV2(t, "host-64", "deployment-64", 1, uint64(index+1))
		defer stores[index].Close()
	}
	var successes, conflicts, unavailable atomic.Uint64
	var wait sync.WaitGroup
	for index := range stores {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			candidate := candidates[index]
			_, err := stores[index].CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, candidate)
			switch {
			case err == nil:
				successes.Add(1)
			case contract.HasCode(err, contract.ErrorConflict):
				conflicts.Add(1)
			case contract.HasCode(err, contract.ErrorUnavailable), contract.HasCode(err, contract.ErrorUnknownOutcome):
				unavailable.Add(1)
			default:
				t.Errorf("unexpected concurrent error: %v", err)
			}
		}(index)
	}
	wait.Wait()
	if successes.Load() != 1 || successes.Load()+conflicts.Load()+unavailable.Load() != workers {
		t.Fatalf("success=%d conflict=%d unavailable=%d", successes.Load(), conflicts.Load(), unavailable.Load())
	}
}

func TestDeploymentSchemaV7ProofIgnoresUserProofCoordinatesAndRejectsExtras(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proof-user.db")
	store := openTestStore(t, path)
	value := storedDeploymentFixtureV2(t, "proof-host", "proof-deployment", 1, 1)
	if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, value); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	if got, err := reopened.InspectStoredHostDeploymentCurrentV2(context.Background(), value.Ref.HostID, value.Ref.DeploymentID); err != nil || got.Ref != value.Ref {
		t.Fatalf("real proof coordinate blocked reopen: %+v %v", got, err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		ddl  string
	}{
		{"index", `CREATE INDEX user_extra_deployment_index ON agent_host_deployment_current_v2(digest)`},
		{"trigger", `CREATE TRIGGER user_extra_deployment_trigger AFTER UPDATE ON agent_host_deployment_current_v2 BEGIN SELECT 1; END`},
	} {
		t.Run(test.name, func(t *testing.T) {
			copyPath := filepath.Join(t.TempDir(), test.name+".db")
			source := openTestStore(t, copyPath)
			if err := source.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", copyPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(test.ddl); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			opened, err := Open(context.Background(), Config{Path: copyPath, Owner: testOwner(), Clock: func() time.Time { return testNow }})
			if opened != nil {
				_ = opened.Close()
			}
			if !contract.HasCode(err, contract.ErrorConflict) {
				t.Fatalf("extra %s accepted: %v", test.name, err)
			}
		})
	}
}

func TestDeploymentSchemaV7RejectsWeakCommentOnlySchemaWithCorrectLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weak-schema.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	weak := `
CREATE TABLE agent_host_schema(version INTEGER PRIMARY KEY,digest TEXT NOT NULL,applied_unix_nano INTEGER NOT NULL);
CREATE TABLE agent_host_deployment_current_history_v2(
 host_id TEXT NOT NULL,deployment_id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,
 bootstrap_digest TEXT NOT NULL,selection_id TEXT NOT NULL,selection_revision INTEGER NOT NULL,
 selection_digest TEXT NOT NULL,selection_expires_unix_nano INTEGER NOT NULL,checked_unix_nano INTEGER NOT NULL,
 expires_unix_nano INTEGER NOT NULL,row_digest TEXT NOT NULL,canonical_json BLOB NOT NULL,
 PRIMARY KEY(host_id,deployment_id,revision),
 UNIQUE(host_id,deployment_id,revision,digest),
 UNIQUE(host_id,deployment_id,revision,digest,selection_id,selection_revision,selection_digest,selection_expires_unix_nano)
 /* CHECK(revision > 0) CHECK(selection_revision > 0) CHECK(expires_unix_nano > checked_unix_nano) */
);
CREATE TABLE agent_host_deployment_current_v2(
 host_id TEXT NOT NULL,deployment_id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,
 selection_id TEXT NOT NULL,selection_revision INTEGER NOT NULL,selection_digest TEXT NOT NULL,
 selection_expires_unix_nano INTEGER NOT NULL,row_digest TEXT NOT NULL,
 PRIMARY KEY(host_id,deployment_id)
 /* FOREIGN KEY(host_id,deployment_id,revision,digest,selection_id,selection_revision,selection_digest,selection_expires_unix_nano)
 REFERENCES agent_host_deployment_current_history_v2(host_id,deployment_id,revision,digest,selection_id,selection_revision,selection_digest,selection_expires_unix_nano) */
);
`
	if _, err = db.Exec(weak); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(
		`INSERT INTO agent_host_schema(version,digest,applied_unix_nano) VALUES(7,?,1)`,
		string(core.DigestBytes([]byte(schemaV7))),
	); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(context.Background(), Config{Path: path, Owner: testOwner(), Clock: func() time.Time { return testNow }})
	if opened != nil {
		_ = opened.Close()
	}
	if !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("weak comment-only schema accepted: %v", err)
	}
}

func TestDeploymentSchemaV7RejectsExtraCheckAndCollationDriftWithCorrectLedger(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{
			name: "extra check",
			old:  "bootstrap_digest TEXT NOT NULL,",
			new:  "bootstrap_digest TEXT NOT NULL CHECK(bootstrap_digest <> 'blocked'),",
		},
		{
			name: "column collate nocase",
			old:  "host_id TEXT NOT NULL,",
			new:  "host_id TEXT COLLATE NOCASE NOT NULL,",
		},
		{
			name: "unique collate nocase",
			old:  "selection_digest, selection_expires_unix_nano)",
			new:  "selection_digest COLLATE NOCASE, selection_expires_unix_nano)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ddl-drift.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(schemaV7); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`DROP TABLE agent_host_deployment_current_v2; DROP TABLE agent_host_deployment_current_history_v2`); err != nil {
				t.Fatal(err)
			}
			drifted := strings.Replace(schemaDeltaV7, test.old, test.new, 1)
			if drifted == schemaDeltaV7 {
				t.Fatal("test did not mutate deployment DDL")
			}
			if _, err = db.Exec(drifted); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(
				`INSERT INTO agent_host_schema(version,digest,applied_unix_nano) VALUES(7,?,1)`,
				string(core.DigestBytes([]byte(schemaV7))),
			); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			opened, err := Open(context.Background(), Config{Path: path, Owner: testOwner(), Clock: func() time.Time { return testNow }})
			if opened != nil {
				_ = opened.Close()
			}
			if !contract.HasCode(err, contract.ErrorConflict) {
				t.Fatalf("DDL drift accepted: %v", err)
			}
		})
	}
}

func TestDeploymentSchemaV7IndexXInfoKeyClassAndAuxCollationFailClosed(t *testing.T) {
	if _, err := validateDeploymentIndexXInfoRowV2(
		2,
		0,
		sql.NullString{String: "host_id", Valid: true},
		0,
		sql.NullString{String: "BINARY", Valid: true},
		"proof",
	); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("index_xinfo key=2 accepted: %v", err)
	}
	if _, err := validateDeploymentIndexXInfoRowV2(
		0,
		-1,
		sql.NullString{},
		0,
		sql.NullString{String: "NOCASE", Valid: true},
		"proof",
	); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("index_xinfo auxiliary NOCASE accepted: %v", err)
	}
	if _, err := validateDeploymentIndexXInfoRowV2(
		1,
		0,
		sql.NullString{String: "host_id", Valid: true},
		0,
		sql.NullString{String: "binary", Valid: true},
		"proof",
	); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("index_xinfo key lowercase binary collation accepted: %v", err)
	}
	if _, err := validateDeploymentIndexXInfoRowV2(
		0,
		-1,
		sql.NullString{},
		0,
		sql.NullString{String: "binary", Valid: true},
		"proof",
	); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("index_xinfo auxiliary lowercase binary collation accepted: %v", err)
	}
}

func TestDeploymentSchemaV7SynchronousFullAndDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync.db")
	store := openTestStore(t, path)
	var synchronous int
	if err := store.db.QueryRow(`PRAGMA synchronous`).Scan(&synchronous); err != nil || synchronous != 2 {
		t.Fatalf("synchronous=%d err=%v", synchronous, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	if err := reopened.db.QueryRow(`PRAGMA synchronous`).Scan(&synchronous); err != nil || synchronous != 2 {
		t.Fatalf("reopen synchronous=%d err=%v", synchronous, err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`PRAGMA synchronous=OFF`); err != nil {
		t.Fatal(err)
	}
	drifted := &Store{db: db, owner: testOwner(), clock: func() time.Time { return testNow }}
	if err = drifted.verifyPragmas(context.Background()); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("synchronous drift accepted: %v", err)
	}
	_ = db.Close()
}

func TestDeploymentSchemaV7MigratesRealV6DataWithoutRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migrate-v6.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(schemaV6); err != nil {
		t.Fatal(err)
	}
	for version, schema := range map[int]string{
		3: schemaBaseV3,
		4: schemaV1,
		5: schemaV5,
		6: schemaV6,
	} {
		if _, err = db.Exec(
			`INSERT INTO agent_host_schema(version,digest,applied_unix_nano) VALUES(?,?,1)`,
			version,
			string(core.DigestBytes([]byte(schema))),
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(
		`INSERT INTO agent_host_start_claims(host_id,start_id,digest,row_digest,canonical_json) VALUES('legacy-host','legacy-start','legacy-digest','legacy-row',X'7B7D')`,
	); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, path)
	var digest, rowDigest string
	if err = store.db.QueryRow(
		`SELECT digest,row_digest FROM agent_host_start_claims WHERE host_id='legacy-host' AND start_id='legacy-start'`,
	).Scan(&digest, &rowDigest); err != nil || digest != "legacy-digest" || rowDigest != "legacy-row" {
		t.Fatalf("v6 data rewritten or lost digest=%q row=%q err=%v", digest, rowDigest, err)
	}
	var proof string
	if err = store.db.QueryRow(`SELECT digest FROM agent_host_schema WHERE version=7`).Scan(&proof); err != nil || proof != string(core.DigestBytes([]byte(schemaV7))) {
		t.Fatalf("v7 proof=%q err=%v", proof, err)
	}
}

func storedDeploymentFixtureV2(t *testing.T, hostID, deploymentID string, revision, selectionRevision uint64) contract.HostDeploymentCurrentV2 {
	t.Helper()
	expires := testNow.Add(time.Hour).UnixNano()
	value, err := contract.SealHostDeploymentCurrentV2(contract.HostDeploymentCurrentV2{
		Ref: contract.HostDeploymentCurrentRefV2{
			HostID:          hostID,
			DeploymentID:    deploymentID,
			Revision:        revision,
			BootstrapDigest: contract.DigestV1(core.DigestBytes([]byte("bootstrap-" + hostID))),
			PackageSelectionRef: packageSelectionRefFixtureV2(
				deploymentID,
				selectionRevision,
				expires,
			),
			ExpiresUnixNano: expires,
		},
		ResourceHandles: []runtimeports.ResourceHandleRefV1{},
		ServiceBindings: []contract.HostServiceBindingRefV1{},
		CheckedUnixNano: testNow.UnixNano(),
		ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func packageSelectionRefFixtureV2(id string, revision uint64, expires int64) (ref buildercontract.AgentPackageSelectionCurrentRefV1) {
	ref.SelectionID = "selection/" + id
	ref.Revision = core.Revision(revision)
	ref.Digest = core.DigestBytes([]byte(ref.SelectionID + "-" + time.Duration(revision).String()))
	ref.ExpiresUnixNano = expires
	return ref
}

func storedDeploymentAdvanceFixtureV2(t *testing.T, previous contract.HostDeploymentCurrentV2, selectionRevision uint64) contract.HostDeploymentCurrentV2 {
	t.Helper()
	next := contract.CloneHostDeploymentCurrentV2(previous)
	next.Ref.Revision++
	next.Ref.PackageSelectionRef = packageSelectionRefFixtureV2(next.Ref.DeploymentID, selectionRevision, next.ExpiresUnixNano)
	next.Ref.Digest = ""
	next.ProjectionDigest = ""
	value, err := contract.SealHostDeploymentCurrentV2(next)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
