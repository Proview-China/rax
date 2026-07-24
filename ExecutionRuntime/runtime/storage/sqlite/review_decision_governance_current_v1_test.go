package sqlite

import (
	"context"
	"database/sql"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func TestSQLiteReviewDecisionGovernanceMigratesExistingSchemaV1(t *testing.T) {
	path := testDBPath(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO runtime_binding_schema(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV1, string(core.DigestBytes([]byte(schemaV1))), time.Unix(2_310_000_000, 0).UnixNano()); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, path, func() time.Time { return time.Unix(2_310_000_001, 0) })
	var digest string
	if err := store.db.QueryRow(`SELECT digest FROM runtime_binding_schema WHERE version=?`, schemaVersionV2).Scan(&digest); err != nil || digest != string(core.DigestBytes([]byte(schemaV2))) {
		t.Fatalf("schema v1 did not migrate exactly to v2: %q %v", digest, err)
	}
	var table string
	if err := store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='runtime_review_governance_projection_history'`).Scan(&table); err != nil || table == "" {
		t.Fatalf("schema v2 Governance table is absent after migration: %q %v", table, err)
	}
}

func TestSQLiteReviewDecisionGovernancePublicGatewayRestartAndHistory(t *testing.T) {
	path := testDBPath(t)
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	store := openTestStore(t, path, func() time.Time { return fixture.Now.Add(time.Second) })
	seedSQLiteReviewGovernanceSourcesV1(t, store, fixture)
	proofs := fakes.NewReviewDecisionGovernanceSourceStoreV1()
	proofs.PutTargetV1(fixture.Target)
	proofs.PutAssignmentV1(fixture.Assignment)
	gateway, err := control.NewReviewDecisionGovernanceCurrentGatewayV1(store, proofs, store, store, store, func() time.Time { return fixture.Now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	policyReceipt, err := gateway.PublishReviewDecisionPolicyCurrentV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy})
	if err != nil || !policyReceipt.Created {
		t.Fatalf("Policy first Publish failed: %+v %v", policyReceipt, err)
	}
	authorityReceipt, err := gateway.PublishReviewDecisionAuthorityCurrentV1(context.Background(), ports.ReviewDecisionAuthorityCurrentPublishRequestV1{Value: fixture.Authority})
	if err != nil || !authorityReceipt.Created {
		t.Fatalf("Authority first Publish failed: %+v %v", authorityReceipt, err)
	}
	scopeReceipt, err := gateway.PublishReviewDecisionScopeCurrentV1(context.Background(), ports.ReviewDecisionScopeCurrentPublishRequestV1{Value: fixture.Scope})
	if err != nil || !scopeReceipt.Created {
		t.Fatalf("Scope first Publish failed: %+v %v", scopeReceipt, err)
	}
	if got, err := gateway.InspectCurrentReviewDecisionPolicyV1(context.Background(), fixture.Policy.Subject, fixture.Policy.Ref); err != nil || !reflect.DeepEqual(got, fixture.Policy) {
		t.Fatalf("Policy exact current Inspect drifted: %+v %v", got, err)
	}
	if got, err := gateway.InspectCurrentReviewDecisionAuthorityV1(context.Background(), fixture.Authority.Subject, fixture.Authority.Ref); err != nil || !reflect.DeepEqual(got, fixture.Authority) {
		t.Fatalf("Authority exact current Inspect drifted: %+v %v", got, err)
	}
	if got, err := gateway.InspectCurrentReviewDecisionScopeV1(context.Background(), fixture.Scope.Subject, fixture.Scope.Ref); err != nil || !reflect.DeepEqual(got, fixture.Scope) {
		t.Fatalf("Scope exact current Inspect drifted: %+v %v", got, err)
	}
	aliased, err := store.InspectHistoricalScopeV1(context.Background(), fixture.Scope.Ref)
	if err != nil {
		t.Fatal(err)
	}
	aliased.Fact.SandboxSource.Ref = "mutated-projection-alias"
	if clean, err := store.InspectHistoricalScopeV1(context.Background(), fixture.Scope.Ref); err != nil || clean.Fact.SandboxSource.Ref == "mutated-projection-alias" {
		t.Fatalf("Scope projection Inspect leaked a mutable pointer alias: %+v %v", clean, err)
	}
	next := fixture.NextPolicy(fixture.Now.Add(time.Second))
	if _, err := gateway.PublishReviewDecisionPolicyCurrentV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Previous: &fixture.Policy.Ref, Value: next}); err != nil {
		t.Fatal(err)
	}
	if old, err := gateway.InspectHistoricalReviewDecisionPolicyV1(context.Background(), fixture.Policy.Ref); err != nil || !reflect.DeepEqual(old, fixture.Policy) {
		t.Fatalf("Policy current advance borrowed current index for history: %+v %v", old, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path, func() time.Time { return fixture.Now.Add(3 * time.Second) })
	if got, err := reopened.ResolvePolicyV1(context.Background(), next.Subject); err != nil || got != next.Ref {
		t.Fatalf("restart lost exact Policy current: %+v %v", got, err)
	}
	if got, err := reopened.InspectHistoricalAuthorityV1(context.Background(), fixture.Authority.Ref); err != nil || !reflect.DeepEqual(got, fixture.Authority) {
		t.Fatalf("restart lost Authority history: %+v %v", got, err)
	}
	if got, err := reopened.InspectHistoricalScopeV1(context.Background(), fixture.Scope.Ref); err != nil || !reflect.DeepEqual(got, fixture.Scope) {
		t.Fatalf("restart lost Scope history: %+v %v", got, err)
	}
}

func TestSQLiteReviewDecisionGovernanceLostReplyStageFailureAndTenantIsolation(t *testing.T) {
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	store := openTestStore(t, testDBPath(t), func() time.Time { return fixture.Now.Add(time.Second) })
	store.failNextStageForTest()
	if _, err := store.CommitPolicyV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected staged Policy failure: %v", err)
	}
	if _, err := store.InspectHistoricalPolicyV1(context.Background(), fixture.Policy.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged Policy failure leaked history: %v", err)
	}
	if _, err := store.ResolvePolicyV1(context.Background(), fixture.Policy.Subject); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged Policy failure leaked current: %v", err)
	}
	store.loseNextReplyForTest()
	if _, err := store.CommitPolicyV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost Policy reply must be indeterminate: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if recovered, err := store.InspectHistoricalPolicyV1(context.WithoutCancel(ctx), fixture.Policy.Ref); err != nil || !reflect.DeepEqual(recovered, fixture.Policy) {
		t.Fatalf("lost Policy reply did not recover by exact Inspect: %+v %v", recovered, err)
	}
	replayed, err := store.CommitPolicyV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy})
	if err != nil || replayed.Created {
		t.Fatalf("canonical Policy replay was not idempotent: %+v %v", replayed, err)
	}
	otherTenant := fixture.Policy.Subject
	otherTenant.Target.TenantID = "tenant-other"
	otherTenant.Target.Digest = testDigest(t, "other-target")
	otherTenant.Target.RunID = "run-other"
	otherTenant.Policy.Digest = testDigest(t, "other-policy")
	if _, err := store.ResolvePolicyV1(context.Background(), otherTenant); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("another tenant resolved the first tenant's current index: %v", err)
	}
}

func TestSQLiteReviewDecisionGovernanceConcurrentCanonicalPublish(t *testing.T) {
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	store := openTestStore(t, testDBPath(t), func() time.Time { return fixture.Now.Add(time.Second) })
	results := make(chan error, 64)
	var wait sync.WaitGroup
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, err := store.CommitAuthorityV1(context.Background(), ports.ReviewDecisionAuthorityCurrentPublishRequestV1{Value: fixture.Authority})
			if err == nil && got.Ref != fixture.Authority.Ref {
				err = core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "concurrent Authority replay returned another Ref")
			}
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got, err := store.InspectHistoricalAuthorityV1(context.Background(), fixture.Authority.Ref); err != nil || !reflect.DeepEqual(got, fixture.Authority) {
		t.Fatalf("concurrent Authority Publish did not leave one canonical value: %+v %v", got, err)
	}
}

func TestSQLiteReviewDecisionGovernanceStagedFailureAllDomainsAndRevisionDrift(t *testing.T) {
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	tests := []struct {
		name    string
		commit  func(*Store) error
		inspect func(*Store) error
		resolve func(*Store) error
	}{
		{"policy", func(s *Store) error {
			_, err := s.CommitPolicyV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy})
			return err
		}, func(s *Store) error {
			_, err := s.InspectHistoricalPolicyV1(context.Background(), fixture.Policy.Ref)
			return err
		}, func(s *Store) error {
			_, err := s.ResolvePolicyV1(context.Background(), fixture.Policy.Subject)
			return err
		}},
		{"authority", func(s *Store) error {
			_, err := s.CommitAuthorityV1(context.Background(), ports.ReviewDecisionAuthorityCurrentPublishRequestV1{Value: fixture.Authority})
			return err
		}, func(s *Store) error {
			_, err := s.InspectHistoricalAuthorityV1(context.Background(), fixture.Authority.Ref)
			return err
		}, func(s *Store) error {
			_, err := s.ResolveAuthorityV1(context.Background(), fixture.Authority.Subject)
			return err
		}},
		{"scope", func(s *Store) error {
			_, err := s.CommitScopeV1(context.Background(), ports.ReviewDecisionScopeCurrentPublishRequestV1{Value: fixture.Scope})
			return err
		}, func(s *Store) error {
			_, err := s.InspectHistoricalScopeV1(context.Background(), fixture.Scope.Ref)
			return err
		}, func(s *Store) error {
			_, err := s.ResolveScopeV1(context.Background(), fixture.Scope.Subject)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openTestStore(t, testDBPath(t), func() time.Time { return fixture.Now.Add(time.Second) })
			store.failNextStageForTest()
			if err := test.commit(store); !core.HasCategory(err, core.ErrorUnavailable) {
				t.Fatalf("expected staged failure: %v", err)
			}
			if err := test.inspect(store); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("staged failure leaked history: %v", err)
			}
			if err := test.resolve(store); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("staged failure leaked current: %v", err)
			}
		})
	}
	store := openTestStore(t, testDBPath(t), func() time.Time { return fixture.Now.Add(time.Second) })
	if _, err := store.CommitPolicyV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy}); err != nil {
		t.Fatal(err)
	}
	drift := fixture.Policy
	drift.ExpiresUnixNano--
	drift.Ref.Digest, drift.ProjectionDigest = "", ""
	digest, err := ports.DigestReviewDecisionPolicyCurrentProjectionV1(drift)
	if err != nil {
		t.Fatal(err)
	}
	drift.Ref.Digest, drift.ProjectionDigest = digest, digest
	if _, err := store.CommitPolicyV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: drift}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same Policy ID/revision with another digest must conflict: %v", err)
	}
	if got, err := store.InspectHistoricalPolicyV1(context.Background(), fixture.Policy.Ref); err != nil || !reflect.DeepEqual(got, fixture.Policy) {
		t.Fatalf("revision drift overwrote immutable Policy history: %+v %v", got, err)
	}
}

func TestSQLiteReviewGovernanceSourceOwnerCASDeepCloneAndZeroLeak(t *testing.T) {
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	store := openTestStore(t, testDBPath(t), func() time.Time { return fixture.Now.Add(time.Second) })
	store.failNextStageForTest()
	if _, err := store.CreateReviewPolicyFactV2(context.Background(), fixture.Policy.Fact); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected source staged failure: %v", err)
	}
	if _, err := store.InspectReviewPolicy(context.Background(), fixture.Policy.Fact.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged source failure leaked current: %v", err)
	}
	seedSQLiteReviewGovernanceSourcesV1(t, store, fixture)
	nextPolicy := fixture.Policy.Fact
	expectedPolicy := ports.ReviewPolicyBindingRefV2{Ref: nextPolicy.Ref, Revision: nextPolicy.Revision, Digest: nextPolicy.Digest}
	nextPolicy.Revision++
	nextPolicy.PolicyDecisionRef = "decision-gov-next"
	nextPolicy.Digest = ""
	nextDigest, digestErr := nextPolicy.DigestV2()
	nextPolicy.Digest = mustSQLiteDigest(t, nextDigest, digestErr)
	store.loseNextReplyForTest()
	if _, err := store.CompareAndSwapReviewPolicyFactV2(context.Background(), expectedPolicy, nextPolicy); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost source CAS reply must be indeterminate: %v", err)
	}
	nextPolicyRef := ports.ReviewPolicyBindingRefV2{Ref: nextPolicy.Ref, Revision: nextPolicy.Revision, Digest: nextPolicy.Digest}
	if recovered, err := store.InspectReviewPolicyFactV2(context.Background(), nextPolicyRef); err != nil || !reflect.DeepEqual(recovered, nextPolicy) {
		t.Fatalf("lost source CAS reply did not recover through exact historical Inspect: %+v %v", recovered, err)
	}
	if got, err := store.InspectReviewPolicy(context.Background(), nextPolicy.Ref); err != nil || !reflect.DeepEqual(got, nextPolicy) {
		t.Fatalf("lost source CAS reply did not recover through current Inspect: %+v %v", got, err)
	}
	nextAuthority := fixture.Authority.Fact
	expectedAuthority := fixture.Authority.Subject.Authority
	nextAuthority.Revision++
	nextAuthority.Digest = testDigest(t, "authority-next")
	if _, err := store.CompareAndSwapDispatchAuthorityFactV2(context.Background(), expectedAuthority, nextAuthority); err != nil {
		t.Fatal(err)
	}
	if got, err := store.InspectDispatchAuthority(context.Background(), nextAuthority.Ref); err != nil || !reflect.DeepEqual(got, nextAuthority) {
		t.Fatalf("Authority source CAS/current drifted: %+v %v", got, err)
	}
	nextScope := fixture.Scope.Fact
	expectedScope, err := nextScope.BindingRefV2()
	if err != nil {
		t.Fatal(err)
	}
	nextScope.Revision++
	nextScope.ProjectionWatermark++
	nextScope.Digest = ""
	nextScopeDigest, err := nextScope.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	nextScope.Digest = nextScopeDigest
	if _, err := store.CompareAndSwapExecutionScopeCurrentFactV2(context.Background(), expectedScope, nextScope); err != nil {
		t.Fatal(err)
	}
	if old, err := store.InspectExecutionScopeFactV2(context.Background(), expectedScope); err != nil || !reflect.DeepEqual(old, fixture.Scope.Fact) {
		t.Fatalf("Scope CAS rewrote exact historical source: %+v %v", old, err)
	}
	scope, err := store.InspectCurrentExecutionScope(context.Background(), fixture.Scope.Fact.Ref)
	if err != nil {
		t.Fatal(err)
	}
	scope.SandboxSource.Ref = "mutated-alias"
	again, err := store.InspectCurrentExecutionScope(context.Background(), fixture.Scope.Fact.Ref)
	if err != nil || again.SandboxSource.Ref == "mutated-alias" {
		t.Fatalf("source Reader leaked a mutable pointer alias: %+v %v", again, err)
	}
}

func seedSQLiteReviewGovernanceSourcesV1(t *testing.T, store *Store, fixture testsupport.ReviewDecisionGovernanceFixtureV1) {
	t.Helper()
	if _, err := store.CreateReviewPolicyFactV2(context.Background(), fixture.Policy.Fact); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDispatchAuthorityFactV2(context.Background(), fixture.Authority.Fact); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateExecutionScopeCurrentFactV2(context.Background(), fixture.Scope.Fact); err != nil {
		t.Fatal(err)
	}
}

func mustSQLiteDigest(t *testing.T, digest core.Digest, err error) core.Digest {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestSQLiteReviewDecisionGovernanceInterfaces(t *testing.T) {
	var _ control.ReviewDecisionGovernanceCurrentFactPortV1 = (*Store)(nil)
	var _ ports.ReviewPolicyFactReaderV2 = (*Store)(nil)
	var _ ports.AuthorityFactReaderV2 = (*Store)(nil)
	var _ ports.ExecutionScopeFactReaderV2 = (*Store)(nil)
}
