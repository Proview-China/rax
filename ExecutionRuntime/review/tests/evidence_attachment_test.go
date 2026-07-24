package review_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	storesqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func sealedEvidenceAttachmentV1(t *testing.T, now time.Time, view service.ReviewViewV1) contract.EvidenceAttachmentV1 {
	t.Helper()
	value, err := contract.SealEvidenceAttachmentV1(contract.EvidenceAttachmentV1{
		FactIdentityV1:   contract.FactIdentityV1{TenantID: view.Case.TenantID, ID: "attachment-a", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		IdempotencyKey:   "attach-key-a",
		Case:             contract.ExactResourceRefV1{ID: view.Case.ID, Revision: view.Case.Revision, Digest: view.Case.Digest},
		Target:           contract.ExactResourceRefV1{ID: view.Target.ID, Revision: view.Target.Revision, Digest: view.Target.Digest},
		SubmitterID:      "reviewer-a",
		Evidence:         []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("attach-evidence-a")},
		ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestEvidenceAttachmentCreateOnceCurrentAndSQLiteRestartV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_310_000, 0)
	clock := testkit.NewClock(now)
	path := filepath.Join(t.TempDir(), "review.db")
	store, err := storesqlite.Open(ctx, storesqlite.Config{Path: path, Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	testkit.PublishRubric(ctx, store, now, "tenant-a")
	owner, _ := service.New(store, clock.Now)
	view, err := owner.SubmitV1(ctx, submitWithTargetV1(t, now, testkit.Target(now), "case-attach"))
	if err != nil {
		t.Fatal(err)
	}
	value := sealedEvidenceAttachmentV1(t, now, view)
	created, err := owner.AttachEvidenceV1(ctx, value)
	if err != nil || created.Digest != value.Digest {
		t.Fatalf("attach failed: %+v %v", created, err)
	}
	if replay, replayErr := owner.AttachEvidenceV1(ctx, value); replayErr != nil || replay.Digest != value.Digest {
		t.Fatalf("canonical replay failed: %+v %v", replay, replayErr)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storesqlite.Open(ctx, storesqlite.Config{Path: path, Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	inspected, err := store.InspectEvidenceAttachmentExactV1(ctx, value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest))
	if err != nil || inspected.Digest != value.Digest {
		t.Fatalf("restart exact inspect failed: %+v %v", inspected, err)
	}
	byKey, err := store.InspectEvidenceAttachmentByIdempotencyV1(ctx, value.TenantID, value.IdempotencyKey)
	if err != nil || byKey.Digest != value.Digest {
		t.Fatalf("restart idempotency inspect failed: %+v %v", byKey, err)
	}
}

func TestEvidenceAttachmentRejectsStaleCaseAndChangedIdempotencyV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_320_000, 0)
	clock := testkit.NewClock(now)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(ctx, store, now, "tenant-a")
	owner, _ := service.New(store, clock.Now)
	view, err := owner.SubmitV1(ctx, submitWithTargetV1(t, now, testkit.Target(now), "case-attach-stale"))
	if err != nil {
		t.Fatal(err)
	}
	value := sealedEvidenceAttachmentV1(t, now, view)
	if _, err = owner.AttachEvidenceV1(ctx, value); err != nil {
		t.Fatal(err)
	}
	changed := value
	changed.ID = "attachment-b"
	changed.Digest = ""
	changed.Evidence[0].Digest = core.DigestBytes([]byte("changed"))
	changed, _ = contract.SealEvidenceAttachmentV1(changed)
	if _, err = owner.AttachEvidenceV1(ctx, changed); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("changed idempotency payload must conflict: %v", err)
	}
	clock.Advance(time.Second)
	next, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: view.Case.TenantID, ID: view.Case.ID, Revision: view.Case.Revision + 1, CreatedUnixNano: view.Case.CreatedUnixNano, UpdatedUnixNano: clock.Now().UnixNano()}, TargetID: view.Case.TargetID, TargetRevision: view.Case.TargetRevision, TargetDigest: view.Case.TargetDigest, State: contract.CaseAdmittedV1, ExpiresUnixNano: view.Case.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AdvanceCaseForTestV1(ctx, reviewport.ExpectedV1(view.Case.Revision, view.Case.Digest), next); err != nil {
		t.Fatal(err)
	}
	stale := value
	stale.ID, stale.IdempotencyKey, stale.Digest = "attachment-stale", "attach-key-stale", ""
	stale, _ = contract.SealEvidenceAttachmentV1(stale)
	if _, err = owner.AttachEvidenceV1(ctx, stale); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("stale Case must fail closed: %v", err)
	}
}

type evidenceAttachmentReplyLostStoreV1 struct {
	service.StoreV1
	calls int
}

func (s *evidenceAttachmentReplyLostStoreV1) CreateEvidenceAttachmentV1(ctx context.Context, mutation reviewport.CreateEvidenceAttachmentMutationV1) (contract.EvidenceAttachmentV1, error) {
	s.calls++
	created, err := s.StoreV1.CreateEvidenceAttachmentV1(ctx, mutation)
	if err != nil {
		return created, err
	}
	return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Evidence Attachment reply loss")
}

func TestEvidenceAttachmentLostReplyInspectsExactWithoutSecondMutationV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_330_000, 0)
	base := storetestkit.NewMemoryStoreV1(func() time.Time { return now })
	testkit.PublishRubric(ctx, base, now, "tenant-a")
	owner, _ := service.New(base, func() time.Time { return now })
	view, err := owner.SubmitV1(ctx, submitWithTargetV1(t, now, testkit.Target(now), "case-attach-lost"))
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &evidenceAttachmentReplyLostStoreV1{StoreV1: base}
	owner, _ = service.New(wrapped, func() time.Time { return now })
	value := sealedEvidenceAttachmentV1(t, now, view)
	created, err := owner.AttachEvidenceV1(ctx, value)
	if err != nil || created.Digest != value.Digest || wrapped.calls != 1 {
		t.Fatalf("lost reply recovery calls=%d value=%+v err=%v", wrapped.calls, created, err)
	}
}

func TestEvidenceAttachmentActualPointClockRollbackAndExpiryAreZeroWriteV1(t *testing.T) {
	for _, test := range []struct {
		name   string
		fresh  func(time.Time) time.Time
		reason core.ReasonCode
	}{
		{name: "rollback", fresh: func(now time.Time) time.Time { return now.Add(-time.Second) }, reason: core.ReasonClockRegression},
		{name: "expiry crossing", fresh: func(now time.Time) time.Time { return now.Add(time.Minute) }, reason: core.ReasonReviewVerdictStale},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			now := time.Unix(1_900_340_000, 0)
			base := storetestkit.NewMemoryStoreV1(func() time.Time { return now })
			testkit.PublishRubric(ctx, base, now, "tenant-a")
			owner, _ := service.New(base, func() time.Time { return now })
			view, err := owner.SubmitV1(ctx, submitWithTargetV1(t, now, testkit.Target(now), "case-attach-clock-"+test.name))
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			owner, _ = service.New(base, func() time.Time {
				calls++
				if calls == 1 {
					return now
				}
				return test.fresh(now)
			})
			value := sealedEvidenceAttachmentV1(t, now, view)
			if _, err = owner.AttachEvidenceV1(ctx, value); !core.HasReason(err, test.reason) {
				t.Fatalf("actual-point clock failure reason=%s err=%v", test.reason, err)
			}
			if _, err = base.InspectEvidenceAttachmentExactV1(ctx, value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("actual-point failure leaked attachment: %v", err)
			}
		})
	}
}

func TestEvidenceAttachmentConcurrentCanonicalCreateHasOneFactV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_360_000, 0)
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return now })
	testkit.PublishRubric(ctx, store, now, "tenant-a")
	owner, _ := service.New(store, func() time.Time { return now })
	view, err := owner.SubmitV1(ctx, submitWithTargetV1(t, now, testkit.Target(now), "case-attach-concurrent"))
	if err != nil {
		t.Fatal(err)
	}
	value := sealedEvidenceAttachmentV1(t, now, view)
	const workers = 64
	results := make(chan contract.EvidenceAttachmentV1, workers)
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, createErr := owner.AttachEvidenceV1(ctx, value)
			results <- result
			errors <- createErr
		}()
	}
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("canonical concurrent create failed: %v", err)
		}
	}
	for result := range results {
		if result.Digest != value.Digest {
			t.Fatalf("concurrent create returned drifted fact: %+v", result)
		}
	}
	inspected, err := store.InspectEvidenceAttachmentByIdempotencyV1(ctx, value.TenantID, value.IdempotencyKey)
	if err != nil || inspected.Digest != value.Digest {
		t.Fatalf("concurrent create current fact drifted: %+v %v", inspected, err)
	}
	inspected.Evidence[0].Ref = "mutated-client-alias"
	again, err := store.InspectEvidenceAttachmentExactV1(ctx, value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest))
	if err != nil || again.Evidence[0].Ref != value.Evidence[0].Ref {
		t.Fatalf("returned Evidence Attachment alias mutated store: %+v %v", again, err)
	}
}

func TestConformanceEvidenceAttachmentMemoryV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_370_000, 0)
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return now })
	testkit.PublishRubric(ctx, store, now, "tenant-a")
	owner, _ := service.New(store, func() time.Time { return now })
	view, err := owner.SubmitV1(ctx, submitWithTargetV1(t, now, testkit.Target(now), "case-attach-conformance"))
	if err != nil {
		t.Fatal(err)
	}
	value := sealedEvidenceAttachmentV1(t, now, view)
	conflict := value
	conflict.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), value.Evidence...)
	conflict.ID, conflict.Digest = "attachment-conflict", ""
	conflict.Evidence[0].Digest = core.DigestBytes([]byte("conflict"))
	conflict, err = contract.SealEvidenceAttachmentV1(conflict)
	if err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckEvidenceAttachmentStoreV1(ctx, store, conformance.EvidenceAttachmentFixtureV1{Attachment: value, Conflict: conflict, CheckedUnixNano: now.UnixNano()}); err != nil {
		t.Fatal(err)
	}
}

func TestConformanceEvidenceAttachmentSQLiteV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_371_000, 0)
	store, err := storesqlite.Open(ctx, storesqlite.Config{Path: filepath.Join(t.TempDir(), "review.db"), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	testkit.PublishRubric(ctx, store, now, "tenant-a")
	owner, _ := service.New(store, func() time.Time { return now })
	view, err := owner.SubmitV1(ctx, submitWithTargetV1(t, now, testkit.Target(now), "case-attach-conformance-sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	value := sealedEvidenceAttachmentV1(t, now, view)
	conflict := value
	conflict.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), value.Evidence...)
	conflict.ID, conflict.Digest = "attachment-conflict-sqlite", ""
	conflict.Evidence[0].Digest = core.DigestBytes([]byte("conflict-sqlite"))
	conflict, err = contract.SealEvidenceAttachmentV1(conflict)
	if err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckEvidenceAttachmentStoreV1(ctx, store, conformance.EvidenceAttachmentFixtureV1{Attachment: value, Conflict: conflict, CheckedUnixNano: now.UnixNano()}); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceAttachmentSQLiteLostReplyConcurrentAndDeepCloneV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_371_500, 0)
	store, err := storesqlite.Open(ctx, storesqlite.Config{Path: filepath.Join(t.TempDir(), "review.db"), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	testkit.PublishRubric(ctx, store, now, "tenant-a")
	owner, _ := service.New(store, func() time.Time { return now })
	view, err := owner.SubmitV1(ctx, submitWithTargetV1(t, now, testkit.Target(now), "case-attach-sqlite-fault"))
	if err != nil {
		t.Fatal(err)
	}
	value := sealedEvidenceAttachmentV1(t, now, view)
	wrapped := &evidenceAttachmentReplyLostStoreV1{StoreV1: store}
	lostOwner, _ := service.New(wrapped, func() time.Time { return now })
	created, err := lostOwner.AttachEvidenceV1(ctx, value)
	if err != nil || created.Digest != value.Digest || wrapped.calls != 1 {
		t.Fatalf("SQLite lost reply recovery calls=%d value=%+v err=%v", wrapped.calls, created, err)
	}

	const workers = 64
	results := make(chan contract.EvidenceAttachmentV1, workers)
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, createErr := owner.AttachEvidenceV1(ctx, value)
			results <- result
			errors <- createErr
		}()
	}
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("SQLite concurrent canonical create failed: %v", err)
		}
	}
	for result := range results {
		if result.Digest != value.Digest {
			t.Fatalf("SQLite concurrent create returned drifted fact: %+v", result)
		}
	}
	inspected, err := store.InspectEvidenceAttachmentExactV1(ctx, value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest))
	if err != nil {
		t.Fatal(err)
	}
	inspected.Evidence[0].Ref = "sqlite-client-alias"
	again, err := store.InspectEvidenceAttachmentExactV1(ctx, value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest))
	if err != nil || again.Evidence[0].Ref != value.Evidence[0].Ref {
		t.Fatalf("SQLite exact Inspect returned a mutable alias: %+v %v", again, err)
	}
}

func TestEvidenceAttachmentRejectsTerminalCaseAndTTLAboveCurrentInputsV1(t *testing.T) {
	t.Run("terminal case", func(t *testing.T) {
		ctx := context.Background()
		now := time.Unix(1_900_372_000, 0)
		clock := testkit.NewClock(now)
		store := storetestkit.NewMemoryStoreV1(func() time.Time { return now })
		testkit.PublishRubric(ctx, store, now, "tenant-a")
		owner, _ := service.New(store, clock.Now)
		view, err := owner.SubmitV1(ctx, submitWithTargetV1(t, now, testkit.Target(now), "case-attach-terminal"))
		if err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Second)
		trace := testkit.Trace(clock.Now(), view.Case, contract.TraceCancelledV1, 2, view.Case.ID)
		cancelled, err := owner.CancelV1(ctx, service.CancelCommandV1{TenantID: view.Case.TenantID, CaseID: view.Case.ID, Expected: reviewport.ExpectedV1(view.Case.Revision, view.Case.Digest), Reason: core.ReasonInvalidState, Trace: trace})
		if err != nil {
			t.Fatal(err)
		}
		value := sealedEvidenceAttachmentV1(t, clock.Now(), service.ReviewViewV1{Case: cancelled, Target: view.Target})
		if _, err = owner.AttachEvidenceV1(ctx, value); !core.HasReason(err, core.ReasonInvalidState) {
			t.Fatalf("terminal Case must fail closed: %v", err)
		}
		if _, err = store.InspectEvidenceAttachmentExactV1(ctx, value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("terminal Case leaked an attachment: %v", err)
		}
	})

	t.Run("attachment TTL exceeds Case", func(t *testing.T) {
		ctx := context.Background()
		now := time.Unix(1_900_373_000, 0)
		store := storetestkit.NewMemoryStoreV1(func() time.Time { return now })
		testkit.PublishRubric(ctx, store, now, "tenant-a")
		owner, _ := service.New(store, func() time.Time { return now })
		view, err := owner.SubmitV1(ctx, submitWithTargetV1(t, now, testkit.Target(now), "case-attach-ttl"))
		if err != nil {
			t.Fatal(err)
		}
		value := sealedEvidenceAttachmentV1(t, now, view)
		value.ExpiresUnixNano = view.Case.ExpiresUnixNano + 1
		value.Digest = ""
		value, err = contract.SealEvidenceAttachmentV1(value)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = owner.AttachEvidenceV1(ctx, value); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("attachment TTL above Case must fail closed: %v", err)
		}
		if _, err = store.InspectEvidenceAttachmentExactV1(ctx, value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("TTL conflict leaked an attachment: %v", err)
		}
	})
}

func TestEvidenceAttachmentSnapshotRemainsCompatibleBeforeAttachmentFieldsV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_374_000, 0)
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return now })
	testkit.PublishRubric(ctx, store, now, "tenant-a")
	owner, _ := service.New(store, func() time.Time { return now })
	view, err := owner.SubmitV1(ctx, submitWithTargetV1(t, now, testkit.Target(now), "case-attach-old-snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.ExportSnapshotV1(view.Case.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	legacy := snapshot
	legacy.EvidenceAttachments = nil
	legacy.EvidenceAttachmentByIdempotency = nil
	if err := legacy.Validate(); err != nil || legacy.Digest != snapshot.Digest {
		t.Fatalf("optional empty fields changed the legacy snapshot digest: digest=%s want=%s err=%v", legacy.Digest, snapshot.Digest, err)
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "evidence_attachments") || strings.Contains(string(payload), "evidence_attachment_by_idempotency") {
		t.Fatalf("legacy-compatible snapshot serialized new empty fields: %s", payload)
	}
	var decoded memory.SnapshotV1
	if err := core.DecodeStrictJSON(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	restored, err := memory.NewStoreFromSnapshotV1(decoded)
	if err != nil {
		t.Fatal(err)
	}
	caseFact, err := restored.InspectCaseExactV1(ctx, view.Case.TenantID, reviewport.ExactV1(view.Case.ID, view.Case.Revision, view.Case.Digest))
	if err != nil || caseFact.Digest != view.Case.Digest {
		t.Fatalf("legacy-compatible snapshot lost its exact Case: %+v %v", caseFact, err)
	}
}

func submitWithTargetV1(t *testing.T, now time.Time, target contract.TargetSnapshotV1, caseID string) service.SubmitCommandV1 {
	t.Helper()
	request := testkit.Request(now, target, caseID)
	return service.SubmitCommandV1{Request: request, Target: target, Trace: testkit.TraceForTarget(now, caseID, target, contract.TraceRequestedV1, 1, request.ID)}
}
