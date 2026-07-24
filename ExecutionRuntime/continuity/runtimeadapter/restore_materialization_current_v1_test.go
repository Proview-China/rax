package runtimeadapter

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimekernel "github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreMaterializationCurrentReaderV1JoinsExactClosure(t *testing.T) {
	fixture := newRestoreMaterializationAdapterFixtureV1(t)
	projection, err := fixture.adapter.InspectRestoreMaterializationCurrentV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	manifest := fixture.plan.seals.manifest
	if projection.Attempt != fixture.request.Attempt || projection.Eligibility != fixture.request.Eligibility || projection.RestorePlan != fixture.plan.expected || projection.ContextGeneration != continuityExactRefToExternalV1(manifest.ContextGenerationRef) {
		t.Fatalf("materialization projection lost exact roots: %+v", projection)
	}
	if len(projection.ContextFrames) != len(manifest.ContextFrameRefs) || len(projection.Memory) != len(manifest.MemoryRefs) || len(projection.Knowledge) != len(manifest.KnowledgeRefs) || len(projection.Snapshots) != 1 {
		t.Fatalf("materialization projection lost sealed artifacts: %+v", projection)
	}
	wantSnapshot := continuityExactRefToExternalV1(*manifest.ParticipantClosures[0].SnapshotRef)
	if !projection.ContainsSnapshotV1(wantSnapshot) {
		t.Fatal("materialization projection does not contain exact participant Snapshot")
	}
}

func TestRestoreMaterializationCurrentReaderV1LostEligibilityReplyRecoversByInspect(t *testing.T) {
	fixture := newRestoreMaterializationAdapterFixtureWithLostReplyV1(t)
	projection, err := fixture.adapter.InspectRestoreMaterializationCurrentV1(context.Background(), fixture.request)
	if err != nil || projection.Attempt != fixture.request.Attempt {
		t.Fatalf("lost Eligibility reply did not recover through exact Inspect: projection=%+v err=%v", projection, err)
	}
}

func TestRestoreMaterializationCurrentReaderV1RejectsOwnerAndClosureSplice(t *testing.T) {
	for name, mutate := range map[string]func(*restoreMaterializationAdapterFixtureV1){
		"attempt ref": func(f *restoreMaterializationAdapterFixtureV1) {
			f.request.Attempt.Digest = runtimecore.DigestBytes([]byte("other-attempt"))
		},
		"manifest context": func(f *restoreMaterializationAdapterFixtureV1) {
			f.plan.seals.manifest.ContextFrameRefs[0].Owner.BindingRevision++
			refreshManifestAndSealMaterializationV1(t, f)
		},
		"manifest snapshot tenant": func(f *restoreMaterializationAdapterFixtureV1) {
			f.plan.seals.manifest.ParticipantClosures[0].SnapshotRef.TenantID = "tenant-other"
			refreshManifestAndSealMaterializationV1(t, f)
		},
		"seal artifact digest": func(f *restoreMaterializationAdapterFixtureV1) {
			f.plan.seals.seal.ArtifactClosureDigest = contract.DigestBytes([]byte("other-artifacts"))
			f.plan.seals.seal.Digest = ""
			digest, err := f.plan.seals.seal.CanonicalDigest()
			if err != nil {
				t.Fatal(err)
			}
			f.plan.seals.seal.Digest = digest
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRestoreMaterializationAdapterFixtureV1(t)
			mutate(&fixture)
			if _, err := fixture.adapter.InspectRestoreMaterializationCurrentV1(context.Background(), fixture.request); err == nil {
				t.Fatal("spliced Restore materialization closure was accepted")
			}
		})
	}
}

func TestRestoreMaterializationCurrentReaderV1FailsClosedOnExpiryAndTypedNil(t *testing.T) {
	fixture := newRestoreMaterializationAdapterFixtureV1(t)
	fixture.now = time.Unix(0, fixture.request.Eligibility.ExpiresUnixNano)
	fixture.adapter.Clock = func() time.Time { return fixture.now }
	if _, err := fixture.adapter.InspectRestoreMaterializationCurrentV1(context.Background(), fixture.request); err == nil {
		t.Fatal("expired Eligibility was accepted for materialization")
	}

	fixture = newRestoreMaterializationAdapterFixtureV1(t)
	var typedNil *restorePlanReaderAdapterV2
	fixture.adapter.Plans = typedNil
	if _, err := fixture.adapter.InspectRestoreMaterializationCurrentV1(context.Background(), fixture.request); !runtimecore.HasCategory(err, runtimecore.ErrorUnavailable) {
		t.Fatalf("typed-nil exact Reader did not fail closed: %v", err)
	}
}

type restoreMaterializationAdapterFixtureV1 struct {
	now      time.Time
	plan     restorePlanAdapterFixtureV2
	store    *runtimefakes.RestoreGovernanceStoreV2
	gateway  runtimekernel.RestoreGovernanceGatewayV2
	reserved runtimeports.RestoreAttemptFactV2
	request  runtimeports.InspectRestoreMaterializationCurrentRequestV1
	adapter  RestoreMaterializationCurrentReaderV1
}

func newRestoreMaterializationAdapterFixtureV1(t *testing.T) restoreMaterializationAdapterFixtureV1 {
	return newRestoreMaterializationAdapterFixtureWithModeV1(t, false)
}

func newRestoreMaterializationAdapterFixtureWithLostReplyV1(t *testing.T) restoreMaterializationAdapterFixtureV1 {
	return newRestoreMaterializationAdapterFixtureWithModeV1(t, true)
}

func newRestoreMaterializationAdapterFixtureWithModeV1(t *testing.T, loseEligibilityReply bool) restoreMaterializationAdapterFixtureV1 {
	t.Helper()
	plan := newRestorePlanAdapterFixtureV2(t)
	store := runtimefakes.NewRestoreGovernanceStoreV2()
	gateway := runtimekernel.RestoreGovernanceGatewayV2{
		Facts: store, Plans: plan.adapter,
		Inputs: restoreEligibilityInputsAdapterV2{now: plan.now, tenant: runtimecore.TenantID(plan.expected.TenantID), scope: runtimecore.Digest(plan.expected.ScopeDigest)},
		Clock:  func() time.Time { return plan.now },
	}
	reserved, err := gateway.CreateRestoreAttemptV2(context.Background(), runtimeports.CreateRestoreAttemptRequestV2{
		AttemptID: "restore-attempt-materialization-adapter", IdempotencyKey: "restore-attempt-materialization-adapter-key",
		RestorePlan: plan.expected, RequestedNotAfter: plan.now.Add(5 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if loseEligibilityReply {
		store.LoseNextRestoreReplyV2()
	}
	bundle, err := gateway.IssueRestoreEligibilityV2(context.Background(), runtimeports.IssueRestoreEligibilityRequestV2{EligibilityID: "restore-eligibility-materialization-adapter", Attempt: reserved.Ref, RequestedTTL: 2 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	adapter := RestoreMaterializationCurrentReaderV1{Restore: gateway, PlanCurrent: plan.adapter, Plans: plan.plan, Manifests: plan.seals, Clock: func() time.Time { return plan.now }}
	return restoreMaterializationAdapterFixtureV1{
		now: plan.now, plan: plan, store: store, gateway: gateway, reserved: reserved,
		request: runtimeports.InspectRestoreMaterializationCurrentRequestV1{Attempt: bundle.Attempt.Ref, Eligibility: bundle.Eligibility.Ref}, adapter: adapter,
	}
}

func refreshManifestAndSealMaterializationV1(t *testing.T, fixture *restoreMaterializationAdapterFixtureV1) {
	t.Helper()
	manifest := &fixture.plan.seals.manifest
	manifest.Digest = ""
	digest, err := manifest.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	manifest.Digest = digest
	fixture.plan.seals.seal = fixture.plan.seals.seal.Clone()
}
