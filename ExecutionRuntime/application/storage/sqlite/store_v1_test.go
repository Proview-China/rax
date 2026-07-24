package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSQLiteStatePlaneLostReplyRestartInspectV1(t *testing.T) {
	now := time.Unix(1_800_200_000, 0)
	active, stopped, coordination, coordinationNext := sqliteFixturesV1(t, now, "restart")
	path := t.TempDir() + "/state.db"
	store := openTestStoreV1(t, path, now)
	store.LoseNextAgentLifecycleEnsureReplyV1()
	if _, err := store.EnsureAgentLifecycleFactV1(context.Background(), active); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lifecycle lost Ensure reply: %v", err)
	}
	_ = store.Close()
	store = openTestStoreV1(t, path, now)
	if got, err := store.InspectAgentLifecycleFactV1(context.Background(), active.LifecycleID); err != nil || got.Digest != active.Digest {
		t.Fatalf("restart lifecycle Inspect: %#v %v", got, err)
	}
	store.LoseNextAgentLifecycleCASReplyV1()
	if _, err := store.CompareAndSwapAgentLifecycleFactV1(context.Background(), applicationports.AgentLifecycleFactCASRequestV1{LifecycleID: active.LifecycleID, ExpectedRevision: active.Revision, ExpectedDigest: active.Digest, Next: stopped}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lifecycle lost CAS reply: %v", err)
	}
	_ = store.Close()
	store = openTestStoreV1(t, path, now)
	if got, err := store.InspectAgentLifecycleFactV1(context.Background(), active.LifecycleID); err != nil || got.Digest != stopped.Digest {
		t.Fatalf("restart lifecycle CAS Inspect: %#v %v", got, err)
	}

	store.LoseNextAgentActivationCoordinationEnsureReplyV1()
	if _, err := store.EnsureAgentActivationCoordinationV1(context.Background(), coordination); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("coordination lost Ensure reply: %v", err)
	}
	_ = store.Close()
	store = openTestStoreV1(t, path, now)
	if got, err := store.InspectAgentActivationCoordinationV1(context.Background(), coordination.ActivationID); err != nil || got.Digest != coordination.Digest {
		t.Fatalf("restart coordination Inspect: %#v %v", got, err)
	}
	store.LoseNextAgentActivationCoordinationCASReplyV1()
	if _, err := store.CompareAndSwapAgentActivationCoordinationV1(context.Background(), applicationports.AgentActivationCoordinationCASRequestV1{ActivationID: coordination.ActivationID, ExpectedRevision: coordination.Revision, ExpectedDigest: coordination.Digest, Next: coordinationNext}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("coordination lost CAS reply: %v", err)
	}
	_ = store.Close()
	store = openTestStoreV1(t, path, now)
	defer store.Close()
	if got, err := store.InspectAgentActivationCoordinationV1(context.Background(), coordination.ActivationID); err != nil || got.Digest != coordinationNext.Digest {
		t.Fatalf("restart coordination CAS Inspect: %#v %v", got, err)
	}
	if err := store.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteStatePlane64IndependentStoresLinearizeV1(t *testing.T) {
	now := time.Unix(1_800_200_100, 0)
	active, stopped, coordination, coordinationNext := sqliteFixturesV1(t, now, "race")
	path := t.TempDir() + "/state.db"
	const workers = 64
	stores := make([]*StoreV1, workers)
	for i := range stores {
		stores[i] = openTestStoreV1(t, path, now)
		defer stores[i].Close()
	}
	run64V1(t, func(i int) error {
		got, err := stores[i].EnsureAgentLifecycleFactV1(context.Background(), active)
		if err == nil && got.Digest != active.Digest {
			return fmt.Errorf("Ensure digest drift")
		}
		return err
	})
	run64V1(t, func(i int) error {
		got, err := stores[i].EnsureAgentActivationCoordinationV1(context.Background(), coordination)
		if err == nil && got.Digest != coordination.Digest {
			return fmt.Errorf("coordination Ensure digest drift")
		}
		return err
	})
	run64AllowConflictV1(t, func(i int) error {
		got, err := stores[i].CompareAndSwapAgentActivationCoordinationV1(context.Background(), applicationports.AgentActivationCoordinationCASRequestV1{ActivationID: coordination.ActivationID, ExpectedRevision: coordination.Revision, ExpectedDigest: coordination.Digest, Next: coordinationNext})
		if err == nil && got.Digest != coordinationNext.Digest {
			return fmt.Errorf("coordination CAS digest drift")
		}
		return err
	})
	run64AllowConflictV1(t, func(i int) error {
		got, err := stores[i].CompareAndSwapAgentLifecycleFactV1(context.Background(), applicationports.AgentLifecycleFactCASRequestV1{LifecycleID: active.LifecycleID, ExpectedRevision: active.Revision, ExpectedDigest: active.Digest, Next: stopped})
		if err == nil && got.Digest != stopped.Digest {
			return fmt.Errorf("CAS digest drift")
		}
		return err
	})
	got, err := stores[0].InspectAgentLifecycleFactV1(context.Background(), active.LifecycleID)
	if err != nil || got.Digest != stopped.Digest {
		t.Fatalf("64 stores current drifted: %#v %v", got, err)
	}
	var history, current int
	if err = stores[0].db.QueryRow(`SELECT COUNT(*) FROM application_fact_history_v1 WHERE fact_type=? AND fact_id=?`, lifecycleKindV1, active.LifecycleID).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if err = stores[0].db.QueryRow(`SELECT COUNT(*) FROM application_fact_current_v1 WHERE fact_type=? AND fact_id=?`, lifecycleKindV1, active.LifecycleID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if history != 2 || current != 1 {
		t.Fatalf("64 stores history=%d current=%d", history, current)
	}
	if gotCoordination, inspectErr := stores[0].InspectAgentActivationCoordinationV1(context.Background(), coordination.ActivationID); inspectErr != nil || gotCoordination.Digest != coordinationNext.Digest {
		t.Fatalf("64 stores coordination current drifted: %#v %v", gotCoordination, inspectErr)
	}
}

func TestSQLiteStatePlaneRejectsABAClockTypePunAndCorruptionV1(t *testing.T) {
	now := time.Unix(1_800_200_200, 0)
	active, stopped, coordination, _ := sqliteFixturesV1(t, now, "negative")
	path := t.TempDir() + "/state.db"
	store := openTestStoreV1(t, path, now)
	if _, err := store.EnsureAgentLifecycleFactV1(context.Background(), active); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndSwapAgentLifecycleFactV1(context.Background(), applicationports.AgentLifecycleFactCASRequestV1{LifecycleID: active.LifecycleID, ExpectedRevision: active.Revision, ExpectedDigest: active.Digest, Next: stopped}); err != nil {
		t.Fatal(err)
	}
	aba := active
	aba.Revision = 3
	aba.PreviousDigest = stopped.Digest
	aba.Digest = ""
	if _, err := contract.SealAgentLifecycleFactV1(aba); err == nil {
		t.Fatal("ABA active state was accepted")
	}
	_ = store.Close()

	regressed := openTestStoreV1(t, path, now.Add(-time.Nanosecond))
	other, _, _, _ := sqliteFixturesV1(t, now, "clock")
	if _, err := regressed.EnsureAgentLifecycleFactV1(context.Background(), other); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock regression accepted: %v", err)
	}
	_ = regressed.Close()

	store = openTestStoreV1(t, path, now)
	payload, _ := json.Marshal(coordination)
	rowDigest, err := core.CanonicalJSONDigest("praxis.application.state-plane", "v1", "ApplicationFactRowV1", rowIdentityV1{lifecycleKindV1, active.LifecycleID, stopped.Revision, stopped.Digest, stopped.PreviousDigest, core.DigestBytes(payload), stopped.CheckedUnixNano, stopped.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec(`UPDATE application_fact_history_v1 SET payload_json=?,row_digest=? WHERE fact_type=? AND fact_id=? AND revision=?`, payload, string(rowDigest), lifecycleKindV1, active.LifecycleID, uint64(stopped.Revision)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectAgentLifecycleFactV1(context.Background(), active.LifecycleID); err == nil {
		t.Fatal("type-punned payload was accepted")
	}
	if _, err = store.db.Exec(`UPDATE application_fact_history_v1 SET payload_json=? WHERE fact_type=? AND fact_id=? AND revision=?`, []byte(`{"unknown":true}`), lifecycleKindV1, active.LifecycleID, uint64(stopped.Revision)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectAgentLifecycleFactV1(context.Background(), active.LifecycleID); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("corrupt row accepted: %v", err)
	}
	_ = store.Close()
}

func TestSQLiteStatePlaneSchemaWALAndForeignKeyV1(t *testing.T) {
	now := time.Unix(1_800_200_300, 0)
	path := t.TempDir() + "/state.db"
	store := openTestStoreV1(t, path, now)
	var journal string
	var fk int
	if err := store.db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" || fk != 1 {
		t.Fatalf("journal=%s fk=%d", journal, fk)
	}
	if _, err := store.db.Exec(`INSERT INTO application_fact_current_v1(fact_type,fact_id,revision,digest,row_digest) VALUES('agent_lifecycle','absent',1,'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb')`); err == nil {
		t.Fatal("foreign key accepted orphan current")
	}
	if _, err := store.db.Exec(`UPDATE application_state_schema_v1 SET digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE version=1`); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	if reopened, err := OpenV1(context.Background(), ConfigV1{Path: path, Clock: func() time.Time { return now }}); err == nil {
		_ = reopened.Close()
		t.Fatal("schema digest drift was accepted")
	}
}

func TestSQLiteStatePlaneStrictSameNextAndNoCommitCASV1(t *testing.T) {
	now := time.Unix(1_800_200_350, 0)
	_, _, current, next := sqliteFixturesV1(t, now, "cas-strict")
	path := t.TempDir() + "/state.db"
	store := openTestStoreV1(t, path, now)
	if _, err := store.EnsureAgentActivationCoordinationV1(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	request := applicationports.AgentActivationCoordinationCASRequestV1{ActivationID: current.ActivationID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next}
	for _, category := range []core.ErrorCategory{core.ErrorConflict, core.ErrorUnavailable, core.ErrorIndeterminate} {
		store.FailNextAgentActivationCoordinationCASBeforeCommitV1(category)
		if _, err := store.CompareAndSwapAgentActivationCoordinationV1(context.Background(), request); !core.HasCategory(err, category) {
			t.Fatalf("no-commit %s: %v", category, err)
		}
		_ = store.Close()
		store = openTestStoreV1(t, path, now)
		got, err := store.InspectAgentActivationCoordinationV1(context.Background(), current.ActivationID)
		if err != nil || got.Digest != current.Digest {
			t.Fatalf("no-commit %s changed current: %#v %v", category, got, err)
		}
	}
	if _, err := store.CompareAndSwapAgentActivationCoordinationV1(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndSwapAgentActivationCoordinationV1(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same-next CAS replay succeeded: %v", err)
	}
	_ = store.Close()
	store = openTestStoreV1(t, path, now)
	defer store.Close()
	got, err := store.InspectAgentActivationCoordinationV1(context.Background(), current.ActivationID)
	if err != nil || got.Digest != next.Digest {
		t.Fatalf("strict CAS restart Inspect: %#v %v", got, err)
	}
}

func run64V1(t *testing.T, call func(int) error) {
	t.Helper()
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := call(i); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func run64AllowConflictV1(t *testing.T, call func(int) error) {
	t.Helper()
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := call(i); err != nil && !core.HasCategory(err, core.ErrorConflict) {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
func openTestStoreV1(t *testing.T, path string, now time.Time) *StoreV1 {
	t.Helper()
	s, err := OpenV1(context.Background(), ConfigV1{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func sqliteFixturesV1(t *testing.T, now time.Time, suffix string) (contract.AgentLifecycleFactV1, contract.AgentLifecycleFactV1, contract.AgentActivationCoordinationFactV1, contract.AgentActivationCoordinationFactV1) {
	t.Helper()
	expiry := now.Add(time.Hour).UnixNano()
	current := func(domain, id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "praxis." + domain, ID: core.OwnerID(domain)}, ContractVersion: "praxis." + domain + "/v1", ID: id + "-" + suffix, Revision: 1, Digest: core.DigestBytes([]byte(domain + id + suffix)), ExpiresUnixNano: expiry}
	}
	start, err := contract.SealAgentActivationStartRequestV1(contract.AgentActivationStartRequestV1{ActivationID: "activation-" + suffix, AttemptID: "attempt-" + suffix, IdempotencyKey: "idempotency-" + suffix, DefinitionCurrent: current("definition", "definition"), PlanCurrent: current("assembler", "plan"), AssemblyCurrent: current("harness", "assembly"), BindingSetCurrent: current("runtime", "binding"), AuthorityCurrent: current("runtime", "authority"), PolicyCurrent: current("policy", "policy"), BudgetCurrent: current("budget", "budget"), CredentialCurrent: current("credential", "credential"), SandboxAdapterBinding: current("sandbox", "adapter"), ExecutionAdapterBinding: current("harness", "adapter"), RequestedNotAfterUnixNano: now.Add(45 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	lease := core.SandboxLeaseRef{ID: core.SandboxLeaseID("lease-" + suffix), Epoch: 1}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant", ID: core.AgentIdentityID("agent-" + suffix), Epoch: 1}, Lineage: core.LineageRef{ID: core.InstanceLineageID("lineage-" + suffix), PlanDigest: start.PlanCurrent.Digest}, Instance: core.InstanceRef{ID: core.AgentInstanceID("instance-" + suffix), Epoch: 1}, SandboxLease: &lease, AuthorityEpoch: 1}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	resultExpiry := now.Add(30 * time.Minute).UnixNano()
	activation, err := contract.SealAgentActivationResultV1(contract.AgentActivationResultV1{ActivationID: start.ActivationID, AttemptID: start.AttemptID, RequestDigest: start.RequestDigest, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, ActivationCurrent: currentAtV1("runtime", "activation-"+suffix, resultExpiry), SandboxLease: lease, SandboxLeaseCurrent: currentAtV1("sandbox", "lease-"+suffix, resultExpiry), SandboxActiveCurrent: currentAtV1("sandbox", "active-"+suffix, resultExpiry), ExecutionReadyCurrent: currentAtV1("harness", "ready-"+suffix, resultExpiry), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: resultExpiry})
	if err != nil {
		t.Fatal(err)
	}
	active, err := contract.SealAgentLifecycleFactV1(contract.AgentLifecycleFactV1{LifecycleID: start.ActivationID, Revision: 1, State: contract.AgentLifecycleFactActiveV1, StartRequest: start, Activation: activation, CheckedUnixNano: activation.CheckedUnixNano, ExpiresUnixNano: activation.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	stop, err := contract.SealAgentTerminationRequestV1(contract.AgentTerminationRequestV1{StopID: "stop-" + suffix, AttemptID: "stop-attempt-" + suffix, IdempotencyKey: "stop-key-" + suffix, ActivationResult: activation.Ref, ActivationCurrent: activation.ActivationCurrent, StopPolicyCurrent: currentAtV1("policy", "stop-policy-"+suffix, resultExpiry), ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, SandboxLease: lease, RequestedNotAfterUnixNano: now.Add(20 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	termExpiry := now.Add(15 * time.Minute).UnixNano()
	termination, err := contract.SealAgentTerminationResultV1(contract.AgentTerminationResultV1{StopID: stop.StopID, AttemptID: stop.AttemptID, RequestDigest: stop.RequestDigest, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, ActivationCurrent: activation.ActivationCurrent, SandboxLease: lease, TerminationCurrent: currentAtV1("runtime", "termination-"+suffix, termExpiry), State: contract.AgentTerminationStoppedV1, Residuals: []contract.AgentTerminationResidualV1{}, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: termExpiry})
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := contract.SealAgentLifecycleFactV1(contract.AgentLifecycleFactV1{LifecycleID: start.ActivationID, Revision: 2, PreviousDigest: active.Digest, State: contract.AgentLifecycleFactStoppedV1, StartRequest: start, Activation: activation, StopRequest: &stop, Termination: &termination, CheckedUnixNano: termination.CheckedUnixNano, ExpiresUnixNano: termination.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	stepReq, err := contract.SealAgentActivationStepRequestV1(contract.AgentActivationStepRequestV1{ActivationID: start.ActivationID, StartRequestDigest: start.RequestDigest, Step: contract.AgentActivationPreflightV1, RequestedNotAfterUnixNano: start.RequestedNotAfterUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	e1, err := contract.SealAgentActivationStepEventV1(contract.AgentActivationStepEventV1{Sequence: 1, Step: stepReq.Step, State: contract.AgentActivationStepIntentRecordedV1, AttemptID: stepReq.AttemptID, RequestDigest: stepReq.RequestDigest, RecordedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	coord, err := contract.SealAgentActivationCoordinationFactV1(contract.AgentActivationCoordinationFactV1{ActivationID: start.ActivationID, Revision: 1, Request: start, Events: []contract.AgentActivationStepEventV1{e1}})
	if err != nil {
		t.Fatal(err)
	}
	e2, err := contract.SealAgentActivationStepEventV1(contract.AgentActivationStepEventV1{Sequence: 2, Step: stepReq.Step, State: contract.AgentActivationStepInvocationRecordedV1, AttemptID: stepReq.AttemptID, RequestDigest: stepReq.RequestDigest, RecordedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	coordNext, err := contract.SealAgentActivationCoordinationFactV1(contract.AgentActivationCoordinationFactV1{ActivationID: start.ActivationID, Revision: 2, Request: start, Events: []contract.AgentActivationStepEventV1{e1, e2}})
	if err != nil {
		t.Fatal(err)
	}
	return active, stopped, coord, coordNext
}

func currentAtV1(domain, id string, expires int64) runtimeports.OwnerCurrentRefV1 {
	return runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "praxis." + domain, ID: core.OwnerID(domain)}, ContractVersion: "praxis." + domain + "/v1", ID: id, Revision: 1, Digest: core.DigestBytes([]byte(domain + id)), ExpiresUnixNano: expires}
}
