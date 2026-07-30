package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	assemblycontract "github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSQLiteHostStartPackageSelectionBindingLostReplyRestartAndExactInspectV1(t *testing.T) {
	var _ hostports.HostStartClaimPackageSelectionPortV1 = (*Store)(nil)
	path := filepath.Join(t.TempDir(), "start-package-selection.db")
	store := openTestStore(t, path)
	claim, input, deployment, _, binding := sqliteStartPackageSelectionFixtureV1(t, "lost")
	if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, deployment); err != nil {
		t.Fatal(err)
	}
	store.loseNextReplyForTest()
	if _, err := store.ClaimOrInspectHostStartPackageSelectionV1(context.Background(), claim, input.Input, binding); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost reply=%v", err)
	}
	actual, err := store.InspectHostStartPackageSelectionBindingV1(context.Background(), binding.Ref)
	if err != nil || actual.BindingDigest != binding.BindingDigest {
		t.Fatalf("exact Inspect=%+v %v", actual, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	defer reopened.Close()
	actual, err = reopened.InspectHostStartPackageSelectionBindingForClaimV1(context.Background(), binding.ClaimRef)
	if err != nil || actual.Ref != binding.Ref {
		t.Fatalf("restart ForClaim=%+v %v", actual, err)
	}
}

func TestSQLiteHostStartPackageSelectionBindingRollbackConflictAndTamperV1(t *testing.T) {
	t.Run("sidecar failure rolls back all three rows", func(t *testing.T) {
		store := openTestStore(t, filepath.Join(t.TempDir(), "rollback.db"))
		defer store.Close()
		claim, input, deployment, _, binding := sqliteStartPackageSelectionFixtureV1(t, "rollback")
		if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, deployment); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`CREATE TRIGGER reject_start_selection BEFORE INSERT ON agent_host_start_package_selection_bindings_v1 BEGIN SELECT RAISE(ABORT,'reject association'); END;`); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ClaimOrInspectHostStartPackageSelectionV1(context.Background(), claim, input.Input, binding); err == nil {
			t.Fatal("association failure committed")
		}
		if _, err := store.InspectHostStartClaimV1(context.Background(), claim.HostID, claim.StartID); !contract.HasCode(err, contract.ErrorNotFound) {
			t.Fatalf("partial Claim persisted=%v", err)
		}
	})
	t.Run("immutable conflict and row splice", func(t *testing.T) {
		store := openTestStore(t, filepath.Join(t.TempDir(), "conflict.db"))
		defer store.Close()
		claim, input, deployment, _, binding := sqliteStartPackageSelectionFixtureV1(t, "conflict")
		if _, err := store.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, deployment); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ClaimOrInspectHostStartPackageSelectionV1(context.Background(), claim, input.Input, binding); err != nil {
			t.Fatal(err)
		}
		changed := binding
		changed.VerifiedPackageClosureDigest = contract.DigestV1(core.DigestBytes([]byte("another-valid-closure")))
		changed.Ref.Digest, changed.BindingDigest = "", ""
		changed, err := contract.SealHostStartPackageSelectionBindingV1(changed)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.ClaimOrInspectHostStartPackageSelectionV1(context.Background(), claim, input.Input, changed); !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("association replacement=%v", err)
		}
		if _, err = store.db.Exec(`UPDATE agent_host_start_package_selection_bindings_v1 SET closure_digest=? WHERE host_id=? AND start_id=?`,
			string(changed.VerifiedPackageClosureDigest), claim.HostID, claim.StartID); err != nil {
			t.Fatal(err)
		}
		if _, err = store.InspectHostStartPackageSelectionBindingV1(context.Background(), binding.Ref); !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("row splice=%v", err)
		}
	})
}

func TestSQLiteHostStartPackageSelectionBindingEightHandlesSixtyFourSameClaimV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eight-handles.db")
	seed := openTestStore(t, path)
	claim, input, deployment, _, binding := sqliteStartPackageSelectionFixtureV1(t, "concurrent")
	if _, err := seed.CompareAndSwapStoredHostDeploymentCurrentV2(context.Background(), contract.HostDeploymentCurrentRefV2{}, deployment); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	const handles = 8
	stores := make([]*Store, handles)
	for index := range stores {
		stores[index] = openTestStore(t, path)
		defer stores[index].Close()
	}
	var successes atomic.Int64
	var wait sync.WaitGroup
	for index := range 64 {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			actual, err := stores[index%handles].ClaimOrInspectHostStartPackageSelectionV1(context.Background(), claim, input.Input, binding)
			if err != nil {
				t.Errorf("caller %d=%v", index, err)
				return
			}
			if actual.Ref != binding.Ref {
				t.Errorf("caller %d Ref=%+v", index, actual.Ref)
				return
			}
			successes.Add(1)
		}(index)
	}
	wait.Wait()
	if successes.Load() != 64 {
		t.Fatalf("successes=%d", successes.Load())
	}
	var rows int
	if err := stores[0].db.QueryRow(`SELECT COUNT(1) FROM agent_host_start_package_selection_bindings_v1`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("association rows=%d err=%v", rows, err)
	}
}

func TestSQLiteMigratesV7ToV8WithoutRewritingV7ProofV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migrate-v7-v8.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(schemaV7); err != nil {
		t.Fatal(err)
	}
	v7Proof := string(core.DigestBytes([]byte(schemaV7)))
	if _, err = db.Exec(`INSERT INTO agent_host_schema(version,digest,applied_unix_nano) VALUES(7,?,1)`, v7Proof); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, path)
	defer store.Close()
	var actualV7, actualV8 string
	if err = store.db.QueryRow(`SELECT digest FROM agent_host_schema WHERE version=7`).Scan(&actualV7); err != nil || actualV7 != v7Proof {
		t.Fatalf("v7 proof=%q err=%v", actualV7, err)
	}
	if err = store.db.QueryRow(`SELECT digest FROM agent_host_schema WHERE version=8`).Scan(&actualV8); err != nil || actualV8 != string(core.DigestBytes([]byte(schemaV8))) {
		t.Fatalf("v8 proof=%q err=%v", actualV8, err)
	}
}

func sqliteStartPackageSelectionFixtureV1(
	t *testing.T,
	label string,
) (
	contract.HostStartClaimV1,
	contract.HostStartClaimInputBindingV3,
	contract.HostDeploymentCurrentV2,
	buildercontract.AgentPackageSelectionCurrentV1,
	contract.HostStartPackageSelectionBindingV1,
) {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(label + "/" + value)) }
	expires := testNow.Add(time.Hour).UnixNano()
	selection, err := buildercontract.SealAgentPackageSelectionCurrentV1(buildercontract.AgentPackageSelectionCurrentV1{
		Ref: buildercontract.AgentPackageSelectionCurrentRefV1{
			SelectionID: "selection/" + label, Revision: 1, ExpiresUnixNano: expires,
		},
		PackageRef: buildercontract.AgentPackageRefV1{
			PackageID: "agent-package-" + label, Revision: 1, Digest: digest("package"),
			ContractVersion: buildercontract.ContractVersionV1, SchemaVersion: buildercontract.SchemaVersionV1,
		},
		PublicationRef: assemblycontract.AssemblyPublicationRefV2{
			PublicationID: "publication-" + label, Revision: 1, Digest: digest("publication"),
		},
		ClosureDigest: digest("closure"), CheckedUnixNano: testNow.UnixNano(),
		ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := contract.SealHostDeploymentCurrentV2(contract.HostDeploymentCurrentV2{
		Ref: contract.HostDeploymentCurrentRefV2{
			HostID: "host-" + label, DeploymentID: "deployment-" + label, Revision: 1,
			BootstrapDigest: contract.DigestV1(digest("bootstrap")), PackageSelectionRef: selection.Ref, ExpiresUnixNano: expires,
		},
		ResourceHandles: []runtimeports.ResourceHandleRefV1{},
		ServiceBindings: []contract.HostServiceBindingRefV1{},
		CheckedUnixNano: testNow.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	inputValue, err := contract.SealHostStartClaimInputV3(contract.HostStartClaimInputV3{
		HostID: deployment.Ref.HostID, StartID: "start-" + label,
		DeploymentCurrentRef: contract.HostDeploymentCurrentRefV1{
			HostID: deployment.Ref.HostID, DeploymentID: deployment.Ref.DeploymentID, Revision: deployment.Ref.Revision,
			BootstrapDigest: deployment.Ref.BootstrapDigest, ExpiresUnixNano: expires,
			Digest: contract.DigestV1(digest("deployment-v1")),
		},
		HostConfigDigest: contract.DigestV1(digest("config")),
		DefinitionSourceRef: contract.ExactRefV1{
			Kind: "praxis.agent-definition/source-current", ID: "source-" + label, Revision: 1, Digest: contract.DigestV1(digest("source")),
		},
		RequestedOperation: contract.HostStartOperationStartV1,
		CreatedUnixNano:    testNow.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano:    expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := inputValue.ClaimV1()
	if err != nil {
		t.Fatal(err)
	}
	input, err := contract.NewHostStartClaimInputBindingV3(claim, inputValue)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := contract.NewHostStartPackageSelectionBindingV1(claim, input, deployment, selection, testNow.Add(time.Minute).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return claim, input, deployment, selection, binding
}
