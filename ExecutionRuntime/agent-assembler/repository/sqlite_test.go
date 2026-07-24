package repository_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSQLitePlanLostRepliesRestartInspectV1(t *testing.T) {
	fixture := testkit.NewFixture()
	resolved, err := fixture.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/assembler.db"
	s := openAssemblerSQLiteTestV1(t, path)
	s.LoseNextEnsureReplyV1()
	if _, err = s.EnsureExactResolvedAgentPlanV1(context.Background(), resolved.Plan); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost Ensure: %v", err)
	}
	_ = s.Close()
	s = openAssemblerSQLiteTestV1(t, path)
	if got, err := s.InspectExactResolvedAgentPlanV1(context.Background(), resolved.Plan.RefV1()); err != nil || got.Digest != resolved.Plan.Digest {
		t.Fatalf("restart exact: %#v %v", got, err)
	}
	current := sealAssemblerCurrentTestV1(t, contract.CurrentResolvedPlanV1{DefinitionID: fixture.Definition.DefinitionID, Revision: 1, PlanRef: resolved.Plan.RefV1(), UpdatedUnixNano: testkit.Now.UnixNano(), CheckedUnixNano: testkit.Now.UnixNano(), ExpiresUnixNano: resolved.Plan.ValidUntilUnixNano})
	s.LoseNextCASReplyV1()
	if _, err = s.CompareAndSwapCurrentResolvedAgentPlanV1(context.Background(), nil, current); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost CAS: %v", err)
	}
	_ = s.Close()
	s = openAssemblerSQLiteTestV1(t, path)
	defer s.Close()
	if got, err := s.InspectCurrentResolvedAgentPlanV1(context.Background(), fixture.Definition.DefinitionID); err != nil || got.Digest != current.Digest {
		t.Fatalf("restart current: %#v %v", got, err)
	}
	if err = s.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSQLitePlan64IndependentStoresStrictCASAndABAV1(t *testing.T) {
	fixture := testkit.NewFixture()
	resolved, err := fixture.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	planA := resolved.Plan
	planB := assemblerPlanBTestV1(t, planA)
	path := t.TempDir() + "/assembler.db"
	const workers = 64
	stores := make([]*repository.SQLiteV1, workers)
	for i := range stores {
		stores[i] = openAssemblerSQLiteTestV1(t, path)
		defer stores[i].Close()
	}
	runAssembler64V1(t, func(i int) error {
		got, err := stores[i].EnsureExactResolvedAgentPlanV1(context.Background(), planA)
		if err == nil && got.Digest != planA.Digest {
			return fmt.Errorf("plan A drift")
		}
		return err
	}, false)
	runAssembler64V1(t, func(i int) error {
		got, err := stores[i].EnsureExactResolvedAgentPlanV1(context.Background(), planB)
		if err == nil && got.Digest != planB.Digest {
			return fmt.Errorf("plan B drift")
		}
		return err
	}, false)
	checked := testkit.Now.UnixNano()
	currentA := sealAssemblerCurrentTestV1(t, contract.CurrentResolvedPlanV1{DefinitionID: fixture.Definition.DefinitionID, Revision: 1, PlanRef: planA.RefV1(), UpdatedUnixNano: checked, CheckedUnixNano: checked, ExpiresUnixNano: planA.ValidUntilUnixNano})
	storedA, err := stores[0].CompareAndSwapCurrentResolvedAgentPlanV1(context.Background(), nil, currentA)
	if err != nil {
		t.Fatal(err)
	}
	refA := storedA.RefV1()
	currentB := sealAssemblerCurrentTestV1(t, contract.CurrentResolvedPlanV1{DefinitionID: fixture.Definition.DefinitionID, Revision: 2, PlanRef: planB.RefV1(), PreviousRef: &refA, UpdatedUnixNano: checked + 1, CheckedUnixNano: checked + 1, ExpiresUnixNano: planB.ValidUntilUnixNano})
	runAssembler64V1(t, func(i int) error {
		_, err := stores[i].CompareAndSwapCurrentResolvedAgentPlanV1(context.Background(), &refA, currentB)
		return err
	}, true)
	got, err := stores[0].InspectCurrentResolvedAgentPlanV1(context.Background(), fixture.Definition.DefinitionID)
	if err != nil || got.Digest != currentB.Digest {
		t.Fatalf("current B: %#v %v", got, err)
	}
	refB := got.RefV1()
	rollback := sealAssemblerCurrentTestV1(t, contract.CurrentResolvedPlanV1{DefinitionID: fixture.Definition.DefinitionID, Revision: 3, PlanRef: planA.RefV1(), PreviousRef: &refB, UpdatedUnixNano: checked + 2, CheckedUnixNano: checked + 2, ExpiresUnixNano: planA.ValidUntilUnixNano})
	if _, err = stores[0].CompareAndSwapCurrentResolvedAgentPlanV1(context.Background(), &refB, rollback); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("ABA accepted: %v", err)
	}
	var plans, currents int
	raw := openAssemblerRawTestV1(t, path)
	defer raw.Close()
	if err = raw.QueryRow(`SELECT COUNT(*) FROM resolved_plan_history_v1`).Scan(&plans); err != nil {
		t.Fatal(err)
	}
	if err = raw.QueryRow(`SELECT COUNT(*) FROM resolved_plan_current_history_v1`).Scan(&currents); err != nil {
		t.Fatal(err)
	}
	if plans != 2 || currents != 2 {
		t.Fatalf("plans=%d currents=%d", plans, currents)
	}
}

func TestSQLitePlanRejectsRowAndSchemaCorruptionV1(t *testing.T) {
	fixture := testkit.NewFixture()
	resolved, err := fixture.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/assembler.db"
	s := openAssemblerSQLiteTestV1(t, path)
	if _, err = s.EnsureExactResolvedAgentPlanV1(context.Background(), resolved.Plan); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	raw := openAssemblerRawTestV1(t, path)
	if _, err = raw.Exec(`UPDATE resolved_plan_history_v1 SET payload_json=? WHERE plan_id=? AND revision=?`, []byte(`{"unknown":true}`), resolved.Plan.PlanID, uint64(resolved.Plan.Revision)); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	s = openAssemblerSQLiteTestV1(t, path)
	if _, err = s.InspectExactResolvedAgentPlanV1(context.Background(), resolved.Plan.RefV1()); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("corruption accepted: %v", err)
	}
	_ = s.Close()
	raw = openAssemblerRawTestV1(t, path)
	if _, err = raw.Exec(`UPDATE assembler_schema_v1 SET digest=? WHERE version=1`, string(core.DigestBytes([]byte("drift")))); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	if reopened, err := repository.OpenSQLiteV1(context.Background(), repository.SQLiteConfigV1{Path: path, Clock: func() time.Time { return testkit.Now }}); err == nil {
		_ = reopened.Close()
		t.Fatal("schema drift accepted")
	}
}

func TestSQLitePlanTypedNilFailsClosedV1(t *testing.T) {
	var s *repository.SQLiteV1
	s.LoseNextEnsureReplyV1()
	s.LoseNextCASReplyV1()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	checks := []error{}
	_, e := s.EnsureExactResolvedAgentPlanV1(context.Background(), contract.ResolvedAgentPlanV1{})
	checks = append(checks, e)
	_, e = s.InspectExactResolvedAgentPlanV1(context.Background(), contract.ResolvedAgentPlanRefV1{})
	checks = append(checks, e)
	_, e = s.InspectCurrentResolvedAgentPlanV1(context.Background(), "id")
	checks = append(checks, e)
	_, e = s.CompareAndSwapCurrentResolvedAgentPlanV1(context.Background(), nil, contract.CurrentResolvedPlanV1{})
	checks = append(checks, e)
	checks = append(checks, s.IntegrityCheckV1(context.Background()))
	for i, err := range checks {
		if !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("typed nil method %d: %v", i, err)
		}
	}
}

func openAssemblerSQLiteTestV1(t *testing.T, path string) *repository.SQLiteV1 {
	t.Helper()
	s, err := repository.OpenSQLiteV1(context.Background(), repository.SQLiteConfigV1{Path: path, Clock: func() time.Time { return testkit.Now }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func openAssemblerRawTestV1(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	return db
}
func sealAssemblerCurrentTestV1(t *testing.T, v contract.CurrentResolvedPlanV1) contract.CurrentResolvedPlanV1 {
	t.Helper()
	sealed, err := contract.SealCurrentResolvedPlanV1(v)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
func assemblerPlanBTestV1(t *testing.T, a contract.ResolvedAgentPlanV1) contract.ResolvedAgentPlanV1 {
	t.Helper()
	b := a
	b.PlanID = "resolved/sqlite-plan-b"
	b.BindingPlan.ID = b.PlanID + "-binding"
	var err error
	b.BindingPlan, err = runtimeports.SealBindingPlanV2(b.BindingPlan)
	if err != nil {
		t.Fatal(err)
	}
	b, err = contract.SealResolvedAgentPlanV1(b)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func runAssembler64V1(t *testing.T, call func(int) error, allowConflict bool) {
	t.Helper()
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := call(i); err != nil && !(allowConflict && core.HasCategory(err, core.ErrorConflict)) {
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
