package kernel_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/restorestore"
)

type restoreRequirementsReaderV1 struct {
	mu      sync.Mutex
	current contract.RestoreContextRequirementsCurrentV1
	after   func()
}

func (r *restoreRequirementsReaderV1) InspectRestoreContextRequirementsCurrentV1(_ context.Context, _ contract.RestoreContextMaterializationRequestV1) (contract.RestoreContextRequirementsCurrentV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value := r.current
	value.Proofs = append([]contract.FactRef{}, value.Proofs...)
	value.Residuals = append([]contract.FactRef{}, value.Residuals...)
	if r.after != nil {
		after := r.after
		r.after = nil
		after()
	}
	return value, nil
}

type driftingFrameReaderV1 struct {
	base interface {
		FrameByExactRef(context.Context, contract.FactRef, contract.Digest) (contract.ContextFrame, error)
	}
	mu    sync.Mutex
	reads int
}

func (r *driftingFrameReaderV1) FrameByExactRef(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.ContextFrame, error) {
	value, err := r.base.FrameByExactRef(ctx, ref, scope)
	if err != nil {
		return contract.ContextFrame{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if r.reads > 1 {
		value.Rendered.Ref += ":drift"
	}
	return value, nil
}

func TestRestoreContextMaterializationCreateOnceLostReplyAndConcurrentCASV1(t *testing.T) {
	for _, loseReply := range []bool{false, true} {
		t.Run(map[bool]string{false: "concurrent", true: "lost_reply"}[loseReply], func(t *testing.T) {
			now, request, fixture, requirements := restoreContextFixtureV1(t)
			store := restorestore.NewMemory()
			if loseReply {
				store.LoseNextReplyV1()
			}
			service, err := kernel.NewRestoreContextMaterializationServiceV1(fixture.Metadata, fixture.Metadata, requirements, store, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			const workers = 64
			results := make(chan contract.RestoreContextMaterializationFactV1, workers)
			errs := make(chan error, workers)
			var wait sync.WaitGroup
			for range workers {
				wait.Add(1)
				go func() {
					defer wait.Done()
					value, callErr := service.MaterializeRestoreContextV1(context.Background(), request)
					results <- value
					errs <- callErr
				}()
			}
			wait.Wait()
			close(results)
			close(errs)
			for callErr := range errs {
				if callErr != nil {
					t.Fatalf("materialize: %v", callErr)
				}
			}
			var expected contract.FactRef
			for result := range results {
				if expected == (contract.FactRef{}) {
					expected = result.Ref()
				}
				if result.Ref() != expected {
					t.Fatalf("multiple materialization winners: %v != %v", result.Ref(), expected)
				}
			}
			current, err := store.InspectRestoreContextMaterializationByTargetV1(context.Background(), request.Target)
			if err != nil || current.Ref() != expected {
				t.Fatalf("inspect current: ref=%v err=%v", current.Ref(), err)
			}
			current.TargetFrames[0].ID = "mutated-caller-copy"
			again, err := store.InspectRestoreContextMaterializationByTargetV1(context.Background(), request.Target)
			if err != nil || again.TargetFrames[0].ID == "mutated-caller-copy" {
				t.Fatalf("store leaked caller alias: %#v err=%v", again.TargetFrames, err)
			}
		})
	}
}

func TestRestoreContextMaterializationRejectsDriftResidualAndTargetABA_V1(t *testing.T) {
	now, request, fixture, requirements := restoreContextFixtureV1(t)

	t.Run("S1_S2_source_drift", func(t *testing.T) {
		store := restorestore.NewMemory()
		frames := &driftingFrameReaderV1{base: fixture.Metadata}
		service, err := kernel.NewRestoreContextMaterializationServiceV1(frames, fixture.Metadata, requirements, store, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		if _, err = service.MaterializeRestoreContextV1(context.Background(), request); !errors.Is(err, contract.ErrConflict) {
			t.Fatalf("expected S1/S2 conflict, got %v", err)
		}
	})

	t.Run("residual_is_authoritative_but_blocks_Runtime_activation", func(t *testing.T) {
		current := requirements.current
		current.Proofs = append([]contract.FactRef{}, current.Proofs...)
		current.Residuals = []contract.FactRef{restoreContextRefV1("residual")}
		var err error
		current, err = contract.SealRestoreContextRequirementsCurrentV1(current)
		if err != nil {
			t.Fatal(err)
		}
		withResidual := &restoreRequirementsReaderV1{current: current}
		service, err := kernel.NewRestoreContextMaterializationServiceV1(fixture.Metadata, fixture.Metadata, withResidual, restorestore.NewMemory(), func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		fact, err := service.MaterializeRestoreContextV1(context.Background(), request)
		if err != nil || len(fact.Requirements.Residuals) != 1 {
			t.Fatalf("expected sealed diagnostic residual: fact=%#v err=%v", fact, err)
		}
	})

	t.Run("same_target_changed_request_conflicts_without_ABA", func(t *testing.T) {
		store := restorestore.NewMemory()
		service, err := kernel.NewRestoreContextMaterializationServiceV1(fixture.Metadata, fixture.Metadata, requirements, store, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		if _, err = service.MaterializeRestoreContextV1(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		drifted := request.Clone()
		drifted.IdempotencyKey += ":changed"
		if _, err = service.MaterializeRestoreContextV1(context.Background(), drifted); !errors.Is(err, contract.ErrConflict) {
			t.Fatalf("expected target current conflict, got %v", err)
		}
	})
}

func restoreContextFixtureV1(t *testing.T) (time.Time, contract.RestoreContextMaterializationRequestV1, *testfixture.ParentFrameFixtureV1, *restoreRequirementsReaderV1) {
	t.Helper()
	now := time.Unix(0, 1_700_000_000_000_000_000)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	frameDigest, err := fixture.Frame.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	generationDigest, err := fixture.Generation.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	request := contract.RestoreContextMaterializationRequestV1{
		ID:             "restore-context-materialization-1",
		IdempotencyKey: "restore-context-materialization-key-1",
		TenantID:       "tenant-1",
		Attempt:        restoreContextRefV1("attempt"),
		Eligibility:    restoreContextRefV1("eligibility"),
		Stage:          restoreContextRefV1("stage"),
		SandboxApply:   restoreContextRefV1("sandbox-apply"),
		SourceScope:    fixture.Frame.Execution.ScopeDigest,
		Target: contract.RestoreContextTargetBindingV1{
			TenantID:      "tenant-1",
			ScopeDigest:   contract.DigestBytes([]byte("target-scope")),
			InstanceID:    "instance-restored-2",
			InstanceEpoch: 2,
			LeaseID:       "lease-restored-2",
			LeaseEpoch:    2,
			FenceEpoch:    2,
		},
		SourceGeneration:  contract.FactRef{ID: fixture.Generation.ID, Revision: fixture.Generation.Revision, Digest: generationDigest},
		SourceFrames:      []contract.FactRef{{ID: fixture.Frame.ID, Revision: fixture.Frame.Revision, Digest: frameDigest}},
		Requirements:      restoreContextRequirementRefsV1(),
		RequestedUnixNano: now.Add(-time.Second).UnixNano(),
		NotAfterUnixNano:  now.Add(10 * time.Second).UnixNano(),
	}
	requestDigest, err := request.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	current, err := contract.SealRestoreContextRequirementsCurrentV1(contract.RestoreContextRequirementsCurrentV1{
		RequestDigest:   requestDigest,
		Proofs:          []contract.FactRef{restoreContextRefV1("profile-current-proof")},
		CheckedUnixNano: now.Add(-time.Second).UnixNano(),
		ExpiresUnixNano: now.Add(15 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return now, request, fixture, &restoreRequirementsReaderV1{current: current}
}

func restoreContextRefV1(id string) contract.FactRef {
	return contract.FactRef{ID: id, Revision: 1, Digest: contract.DigestBytes([]byte(id))}
}

func restoreContextRequirementRefsV1() []contract.RestoreContextRequirementRefV1 {
	kinds := []contract.RestoreContextRequirementKindV1{contract.RestoreRequirementProfileV1, contract.RestoreRequirementToolV1, contract.RestoreRequirementMCPV1, contract.RestoreRequirementReviewV1, contract.RestoreRequirementAuthorityV1, contract.RestoreRequirementBudgetV1, contract.RestoreRequirementBindingV1}
	result := make([]contract.RestoreContextRequirementRefV1, len(kinds))
	for index, kind := range kinds {
		result[index] = contract.RestoreContextRequirementRefV1{Kind: kind, Ref: restoreContextRefV1(string(kind)), RouteDigest: contract.DigestBytes([]byte("route:" + string(kind)))}
	}
	return result
}
