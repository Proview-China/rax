package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestSQLiteHostStartV3AtomicLostReplyRestartAndExactInspect(t *testing.T) {
	var _ hostports.HostStartClaimPortV3 = (*Store)(nil)
	path := filepath.Join(t.TempDir(), "claim-v3.db")
	store := openTestStore(t, path)
	input := startInputSQLiteFixtureV3(t, "config-a")
	claim, _ := input.ClaimV1()
	store.loseNextReplyForTest()
	if _, err := store.ClaimOrInspectHostStartV3(context.Background(), claim, input); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost reply=%v", err)
	}
	ref, _ := claim.CurrentRefV1()
	binding, err := store.InspectHostStartClaimInputV3(context.Background(), ref)
	if err != nil || binding.Input.ContentDigest != input.ContentDigest {
		t.Fatalf("inspect=%+v %v", binding, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	binding, err = reopened.InspectHostStartClaimInputV3(context.Background(), ref)
	if err != nil || binding.ClaimRef != ref {
		t.Fatalf("restart=%+v %v", binding, err)
	}
	var version5 string
	if err = reopened.db.QueryRow(`SELECT digest FROM agent_host_schema WHERE version=5`).Scan(&version5); err != nil || version5 != string(core.DigestBytes([]byte(schemaV5))) {
		t.Fatalf("schema v5=%s %v", version5, err)
	}
}

func TestSQLiteHostStartV3SidecarFailureRollsBackClaim(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "claim-v3-rollback.db"))
	if _, err := store.db.Exec(`CREATE TRIGGER reject_v3_sidecar BEFORE INSERT ON agent_host_start_claim_input_bindings_v3 BEGIN SELECT RAISE(ABORT,'reject sidecar'); END;`); err != nil {
		t.Fatal(err)
	}
	input := startInputSQLiteFixtureV3(t, "rollback")
	claim, _ := input.ClaimV1()
	if _, err := store.ClaimOrInspectHostStartV3(context.Background(), claim, input); err == nil {
		t.Fatal("sidecar fault succeeded")
	}
	if _, err := store.InspectHostStartClaimV1(context.Background(), claim.HostID, claim.StartID); !contract.HasCode(err, contract.ErrorNotFound) {
		t.Fatalf("partial claim persisted=%v", err)
	}
}

func TestSQLiteHostStartV3V1BypassRejectedAndExactPartialRepair(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "claim-v3-partial.db"))
	input := startInputSQLiteFixtureV3(t, "partial")
	claim, _ := input.ClaimV1()
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("V1 bypass=%v", err)
	}
	payload, rowDigest, err := encodeRow(hostStartClaimRowV1, claim)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec(`INSERT INTO agent_host_start_claims(host_id,start_id,digest,row_digest,canonical_json) VALUES(?,?,?,?,?)`, claim.HostID, claim.StartID, string(claim.Digest), rowDigest, payload); err != nil {
		t.Fatal(err)
	}
	ref, _ := claim.CurrentRefV1()
	if _, err = store.InspectHostStartClaimInputV3(context.Background(), ref); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("partial inspect=%v", err)
	}
	binding, err := store.ClaimOrInspectHostStartV3(context.Background(), claim, input)
	if err != nil || binding.ClaimRef != ref {
		t.Fatalf("repair=%+v %v", binding, err)
	}
}

func TestSQLiteHostStartV1V2V3ConflictAndSidecarTamper(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "claim-v3-conflict.db"))
	input := startInputSQLiteFixtureV3(t, "v3")
	claim, _ := input.ClaimV1()
	if _, err := store.ClaimOrInspectHostStartV3(context.Background(), claim, input); err != nil {
		t.Fatal(err)
	}
	v2 := claimFixture(t, claim.HostID, claim.StartID, "v2")
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), v2); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("V3 then V2=%v", err)
	}
	if _, err := store.db.Exec(`UPDATE agent_host_start_claim_input_bindings_v3 SET input_digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE host_id=? AND start_id=?`, claim.HostID, claim.StartID); err != nil {
		t.Fatal(err)
	}
	ref, _ := claim.CurrentRefV1()
	if _, err := store.InspectHostStartClaimInputV3(context.Background(), ref); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("tamper=%v", err)
	}
	store2 := openTestStore(t, filepath.Join(t.TempDir(), "claim-v1-first.db"))
	if _, err := store2.ClaimOrInspectHostStartV1(context.Background(), v2); err != nil {
		t.Fatal(err)
	}
	if _, err := store2.ClaimOrInspectHostStartV3(context.Background(), claim, input); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("V2 then V3=%v", err)
	}
}

func TestSQLiteSixtyFourHostStartV3CandidatesOneWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claim-v3-64.db")
	seed := openTestStore(t, path)
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	const workers = 64
	stores := make([]*Store, workers)
	for i := range stores {
		stores[i] = openTestStore(t, path)
		defer stores[i].Close()
	}
	var success, conflict atomic.Uint64
	var wg sync.WaitGroup
	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input := startInputSQLiteFixtureV3(t, fmt.Sprintf("candidate-%d", i))
			claim, _ := input.ClaimV1()
			_, err := stores[i].ClaimOrInspectHostStartV3(context.Background(), claim, input)
			switch {
			case err == nil:
				success.Add(1)
			case contract.HasCode(err, contract.ErrorConflict):
				conflict.Add(1)
			default:
				t.Errorf("candidate %d=%v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if success.Load() != 1 || conflict.Load() != workers-1 {
		t.Fatalf("success=%d conflict=%d", success.Load(), conflict.Load())
	}
}

func startInputSQLiteFixtureV3(t *testing.T, label string) contract.HostStartClaimInputV3 {
	t.Helper()
	digest := func(v string) contract.DigestV1 {
		d, e := contract.DigestJSONV1(v)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	now := time.Unix(2_300_000_000, 0)
	input, err := contract.SealHostStartClaimInputV3(contract.HostStartClaimInputV3{HostID: "host-v3", StartID: "start-v3", DeploymentCurrentRef: contract.HostDeploymentCurrentRefV1{HostID: "host-v3", DeploymentID: "deployment-v3", Revision: 1, BootstrapDigest: digest("bootstrap"), ExpiresUnixNano: now.Add(2 * time.Hour).UnixNano(), Digest: digest("deployment")}, HostConfigDigest: digest(label), DefinitionSourceRef: contract.ExactRefV1{Kind: "praxis.agent-definition/source-current", ID: "source-v3", Revision: 1, Digest: digest("source")}, RequestedOperation: contract.HostStartOperationStartV1, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return input
}
