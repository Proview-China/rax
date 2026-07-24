package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestSQLiteDefinitionRepositoryConformanceV1(t *testing.T) {
	now := time.Unix(1_800_220_000, 0)
	s := openDefinitionSQLiteTestV1(t, t.TempDir()+"/definition.db", now)
	defer s.Close()
	conformance.RunRepositoryConformanceV1(t, s, conformance.CatalogV1(), now)
	if err := s.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteDefinitionLostReplyRestartHighestCheckedRevokeAndABAV1(t *testing.T) {
	now := time.Unix(1_800_220_100, 0)
	path := t.TempDir() + "/definition.db"
	definition := definitionSQLiteFixtureV1(t, now)
	s := openDefinitionSQLiteTestV1(t, path, now)
	s.LoseNextCreateReply()
	if _, err := s.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: definition}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("lost reply: %v", err)
	}
	_ = s.Close()
	s = openDefinitionSQLiteTestV1(t, path, now)
	if got, err := s.InspectDefinitionRevisionV1(context.Background(), definition.DefinitionID, definition.Revision); err != nil || got.Digest != definition.Digest {
		t.Fatalf("restart exact: %#v %v", got, err)
	}
	high := now.Add(2 * time.Hour).UnixNano()
	expired, err := s.InspectCurrentDefinitionV1(context.Background(), definition.DefinitionID, high)
	if err != nil || expired.State != contract.DefinitionCurrentExpiredV1 {
		t.Fatalf("highest expiry: %#v %v", expired, err)
	}
	_ = s.Close()
	s = openDefinitionSQLiteTestV1(t, path, now)
	if _, err = s.InspectCurrentDefinitionV1(context.Background(), definition.DefinitionID, high-1); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("restart clock rollback: %v", err)
	}
	revokedAt := high + int64(time.Minute)
	revoked, err := s.RevokeDefinitionV1(context.Background(), ports.RevokeDefinitionRequestV1{DefinitionID: definition.DefinitionID, ExpectedCurrentRevision: 1, RevokedUnixNano: revokedAt, Reason: "operator revoke"})
	if err != nil || revoked.State != contract.DefinitionCurrentRevokedV1 {
		t.Fatalf("revoke: %#v %v", revoked, err)
	}
	source2 := contract.CloneSourceV1(definition.AgentDefinitionSourceV1)
	source2.Revision = 2
	source2.ChangeReason = "ABA after revoke"
	revision2, err := contract.SealDefinitionV1(source2, conformance.CatalogV1(), now.Add(time.Minute).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: revision2, ExpectedCurrentRevision: revoked.Revision}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("revoked ABA accepted: %v", err)
	}
	_ = s.Close()
}

func TestSQLiteDefinition64IndependentStoresLinearizeV1(t *testing.T) {
	now := time.Unix(1_800_220_200, 0)
	path := t.TempDir() + "/definition.db"
	definition := definitionSQLiteFixtureV1(t, now)
	const workers = 64
	stores := make([]*SQLiteRepositoryV1, workers)
	for i := range stores {
		stores[i] = openDefinitionSQLiteTestV1(t, path, now)
		defer stores[i].Close()
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, err := stores[i].CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: definition})
			if err == nil && got.Definition.Digest != definition.Digest {
				err = fmt.Errorf("digest drift")
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var history, current int
	if err := stores[0].db.QueryRow(`SELECT COUNT(*) FROM definition_history_v1 WHERE definition_id=?`, definition.DefinitionID).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if err := stores[0].db.QueryRow(`SELECT COUNT(*) FROM definition_current_v1 WHERE definition_id=?`, definition.DefinitionID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if history != 1 || current != 1 {
		t.Fatalf("history=%d current=%d", history, current)
	}
}

func TestSQLiteDefinitionRejectsRowAndSchemaCorruptionV1(t *testing.T) {
	now := time.Unix(1_800_220_300, 0)
	path := t.TempDir() + "/definition.db"
	definition := definitionSQLiteFixtureV1(t, now)
	s := openDefinitionSQLiteTestV1(t, path, now)
	if _, err := s.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: definition}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE definition_history_v1 SET payload_json=? WHERE definition_id=? AND revision=1`, []byte(`{"unknown":true}`), definition.DefinitionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InspectDefinitionRevisionV1(context.Background(), definition.DefinitionID, 1); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("corruption accepted: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE definition_schema_v1 SET digest=? WHERE version=1`, string(core.DigestBytes([]byte("drift")))); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if reopened, err := OpenSQLiteRepositoryV1(context.Background(), SQLiteConfigV1{Path: path, Catalog: conformance.CatalogV1(), Clock: func() time.Time { return now }}); err == nil {
		_ = reopened.Close()
		t.Fatal("schema drift accepted")
	}
}

func TestSQLiteDefinitionTypedNilFailsClosedV1(t *testing.T) {
	var s *SQLiteRepositoryV1
	s.LoseNextCreateReply()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	checks := []error{}
	_, e := s.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{})
	checks = append(checks, e)
	_, e = s.InspectDefinitionRevisionV1(context.Background(), "id", 1)
	checks = append(checks, e)
	_, e = s.InspectExactDefinitionV1(context.Background(), contract.AgentDefinitionRefV1{})
	checks = append(checks, e)
	_, e = s.InspectCurrentDefinitionV1(context.Background(), "id", 1)
	checks = append(checks, e)
	_, e = s.RevokeDefinitionV1(context.Background(), ports.RevokeDefinitionRequestV1{})
	checks = append(checks, e)
	checks = append(checks, s.IntegrityCheckV1(context.Background()))
	for i, err := range checks {
		if !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("typed nil method %d: %v", i, err)
		}
	}
	recorder := &sqliteConformanceRecorderV1{}
	conformance.RunRepositoryConformanceV1(recorder, s, conformance.CatalogV1(), time.Unix(1_800_220_400, 0))
	if !recorder.called {
		t.Fatal("conformance accepted typed nil SQLite repository")
	}
}

type sqliteConformanceRecorderV1 struct{ called bool }

func (*sqliteConformanceRecorderV1) Helper()                 {}
func (r *sqliteConformanceRecorderV1) Fatalf(string, ...any) { r.called = true }

func openDefinitionSQLiteTestV1(t *testing.T, path string, now time.Time) *SQLiteRepositoryV1 {
	t.Helper()
	s, err := OpenSQLiteRepositoryV1(context.Background(), SQLiteConfigV1{Path: path, Catalog: conformance.CatalogV1(), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func definitionSQLiteFixtureV1(t *testing.T, now time.Time) contract.AgentDefinitionV1 {
	t.Helper()
	v, err := contract.SealDefinitionV1(conformance.SourceV1(now), conformance.CatalogV1(), now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return v
}
