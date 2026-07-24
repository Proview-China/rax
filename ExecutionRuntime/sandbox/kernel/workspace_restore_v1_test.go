package kernel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceRestoreOwnerV1HappyReplayAndPartial(t *testing.T) {
	for _, residual := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "partial"}[residual], func(t *testing.T) {
			fixture := newWorkspaceRestoreOwnerFixtureV1(t, residual)
			mustPrepareWorkspaceRestoreV1(t, fixture.owner, fixture.request)
			fact, err := fixture.owner.StageWorkspaceV1(context.Background(), &fixture.request)
			if err != nil {
				t.Fatal(err)
			}
			want := contract.WorkspaceRestoreStageCompleteV1
			if residual {
				want = contract.WorkspaceRestoreStagePartialV1
			}
			if fact.State != want || (len(fact.Residuals) != 0) != residual {
				t.Fatalf("fact=%#v", fact)
			}
			replay, err := fixture.owner.StageWorkspaceV1(context.Background(), &fixture.request)
			if err != nil || replay.ExactRef() != fact.ExactRef() || fixture.provider.stageCalls != 1 {
				t.Fatalf("replay=%#v err=%v stage_calls=%d", replay, err, fixture.provider.stageCalls)
			}
		})
	}
}

func TestWorkspaceRestoreOwnerV1LostProviderReplyInspectsOriginalAttempt(t *testing.T) {
	fixture := newWorkspaceRestoreOwnerFixtureV1(t, false)
	mustPrepareWorkspaceRestoreV1(t, fixture.owner, fixture.request)
	fixture.provider.loseReply = true
	fact, err := fixture.owner.StageWorkspaceV1(context.Background(), &fixture.request)
	if err != nil || fact.State != contract.WorkspaceRestoreStageCompleteV1 || fixture.provider.stageCalls != 1 || fixture.provider.inspectCalls == 0 {
		t.Fatalf("fact=%#v err=%v provider=%#v", fact, err, fixture.provider)
	}
}

func TestWorkspaceRestoreOwnerV1UnknownOnlyReconcilesByInspect(t *testing.T) {
	fixture := newWorkspaceRestoreOwnerFixtureV1(t, false)
	mustPrepareWorkspaceRestoreV1(t, fixture.owner, fixture.request)
	fixture.provider.failBeforeMaterialize = true
	if _, err := fixture.owner.StageWorkspaceV1(context.Background(), &fixture.request); !errors.Is(err, ports.ErrUnknownOutcome) {
		t.Fatalf("expected unknown outcome, got %v", err)
	}
	stable, _ := fixture.request.StableKeyDigest()
	attempt, err := fixture.store.InspectWorkspaceRestoreAttemptByStableKeyV1(context.Background(), stable)
	if err != nil || attempt.State != contract.WorkspaceRestoreAttemptReconcileRequiredV1 {
		t.Fatalf("attempt=%#v err=%v", attempt, err)
	}
	stageCalls := fixture.provider.stageCalls
	if _, err := fixture.owner.ReconcileWorkspaceV1(context.Background(), &fixture.request); !errors.Is(err, ports.ErrUnknownOutcome) || fixture.provider.stageCalls != stageCalls {
		t.Fatalf("reconcile dispatched provider: err=%v stage_calls=%d", err, fixture.provider.stageCalls)
	}
	fixture.provider.materializeFor(attempt, fixture.bundle.Bundle)
	fact, err := fixture.owner.ReconcileWorkspaceV1(context.Background(), &fixture.request)
	if err != nil || fact.State != contract.WorkspaceRestoreStageCompleteV1 || fixture.provider.stageCalls != stageCalls {
		t.Fatalf("inspect-only recovery=%#v err=%v stage_calls=%d", fact, err, fixture.provider.stageCalls)
	}
}

func TestWorkspaceRestoreOwnerV1S1S2DriftFailsBeforeProvider(t *testing.T) {
	fixture := newWorkspaceRestoreOwnerFixtureV1(t, false)
	mustPrepareWorkspaceRestoreV1(t, fixture.owner, fixture.request)
	fixture.governance.driftCall = 2
	if _, err := fixture.owner.StageWorkspaceV1(context.Background(), &fixture.request); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("expected S1/S2 conflict, got %v", err)
	}
	if fixture.provider.stageCalls != 0 {
		t.Fatalf("provider was called %d times", fixture.provider.stageCalls)
	}
}

func TestWorkspaceRestoreOwnerV1ActualPointDriftFailsBeforeProvider(t *testing.T) {
	fixture := newWorkspaceRestoreOwnerFixtureV1(t, false)
	mustPrepareWorkspaceRestoreV1(t, fixture.owner, fixture.request)
	fixture.governance.driftCall = 3
	if _, err := fixture.owner.StageWorkspaceV1(context.Background(), &fixture.request); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("expected actual-point current conflict, got %v", err)
	}
	if fixture.provider.stageCalls != 0 || fixture.provider.inspectCalls != 0 {
		t.Fatalf("actual-point drift reached Provider: stage=%d inspect=%d", fixture.provider.stageCalls, fixture.provider.inspectCalls)
	}
}

func TestWorkspaceRestoreOwnerV1LostStoreRepliesRecoverExactWinner(t *testing.T) {
	fixture := newWorkspaceRestoreOwnerFixtureV1(t, false)
	lost := &testkit.WorkspaceRestoreLostReplyStoreV1{Base: fixture.store, LoseCreateOnce: true, LoseCommitOnce: true}
	owner, err := NewWorkspaceRestoreOwnerV1(lost, fixture.bundles, fixture.governance, fixture.provider, func() time.Time { return fixture.now }, WorkspaceRestoreOwnerLimitsV1{MaxAttemptTTL: time.Hour, MaxHistoryTTL: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	mustPrepareWorkspaceRestoreV1(t, owner, fixture.request)
	fact, err := owner.StageWorkspaceV1(context.Background(), &fixture.request)
	if err != nil || fact.State != contract.WorkspaceRestoreStageCompleteV1 {
		t.Fatalf("lost reply recovery=%#v err=%v", fact, err)
	}
}

func TestWorkspaceRestoreOwnerV1LostInvocationCASReplyNeverRedispatches(t *testing.T) {
	fixture := newWorkspaceRestoreOwnerFixtureV1(t, false)
	lost := &testkit.WorkspaceRestoreLostReplyStoreV1{Base: fixture.store, LoseCASStateOnce: contract.WorkspaceRestoreAttemptInvocationV1}
	owner, err := NewWorkspaceRestoreOwnerV1(lost, fixture.bundles, fixture.governance, fixture.provider, func() time.Time { return fixture.now }, WorkspaceRestoreOwnerLimitsV1{MaxAttemptTTL: time.Hour, MaxHistoryTTL: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	mustPrepareWorkspaceRestoreV1(t, owner, fixture.request)
	if _, err := owner.StageWorkspaceV1(context.Background(), &fixture.request); !errors.Is(err, ports.ErrUnknownOutcome) {
		t.Fatalf("lost invocation CAS reply must become inspect-only unknown, got %v", err)
	}
	if fixture.provider.stageCalls != 0 || fixture.provider.inspectCalls == 0 {
		t.Fatalf("lost invocation CAS caused dispatch: stage=%d inspect=%d", fixture.provider.stageCalls, fixture.provider.inspectCalls)
	}
	if _, err := owner.StageWorkspaceV1(context.Background(), &fixture.request); !errors.Is(err, ports.ErrUnknownOutcome) || fixture.provider.stageCalls != 0 {
		t.Fatalf("retry must remain inspect-only: err=%v stage=%d", err, fixture.provider.stageCalls)
	}
}

func mustPrepareWorkspaceRestoreV1(t *testing.T, owner *WorkspaceRestoreOwnerV1, request contract.WorkspaceRestoreStageRequestV1) contract.WorkspaceRestoreAttemptV1 {
	t.Helper()
	attempt, err := owner.PrepareWorkspaceV1(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.State != contract.WorkspaceRestoreAttemptPreparedV1 {
		t.Fatalf("prepared attempt state=%s", attempt.State)
	}
	return attempt
}

type workspaceRestoreOwnerFixtureV1 struct {
	now        time.Time
	request    contract.WorkspaceRestoreStageRequestV1
	bundle     contract.WorkspaceRestoreBundleCurrentProjectionV1
	bundles    *workspaceRestoreBundleReaderV1
	governance *workspaceRestoreGovernanceReaderV1
	provider   *workspaceRestoreProviderFakeV1
	store      *testkit.WorkspaceRestoreMemoryStoreV1
	owner      *WorkspaceRestoreOwnerV1
}

func newWorkspaceRestoreOwnerFixtureV1(t *testing.T, residual bool) *workspaceRestoreOwnerFixtureV1 {
	t.Helper()
	now := time.Unix(1_950_000_000, 0)
	bundleValue := contract.WorkspaceSnapshotBundleV1{SnapshotID: "snapshot-1", TenantID: "tenant-1", SourceScopeDigest: strings.Repeat("a", contract.DigestSizeHex), Entries: []contract.WorkspaceSnapshotEntryV1{{Path: "file", Kind: contract.WorkspaceSnapshotRegularFile, Content: []byte("payload")}}}
	if residual {
		bundleValue.Excluded = []contract.WorkspaceSnapshotExcludedV1{{Path: "link", Kind: contract.WorkspaceSnapshotExcludedSymlink, Reason: contract.WorkspaceSnapshotResidualUnsupportedKind}}
	}
	sealedBundle, err := contract.SealWorkspaceSnapshotBundleV1(bundleValue)
	if err != nil {
		t.Fatal(err)
	}
	artifactRef := workspaceRestoreExactRefV1(contract.SnapshotArtifactFactTypeURL, contract.SnapshotArtifactFactDomain, "artifact-fact", now)
	storage := workspaceRestoreStorageRefV1(t, sealedBundle, now)
	bundleProjection, err := contract.SealWorkspaceRestoreBundleCurrentProjectionV1(contract.WorkspaceRestoreBundleCurrentProjectionV1{TenantID: "tenant-1", SnapshotArtifactFactRef: artifactRef, StorageArtifactRef: storage, Bundle: sealedBundle, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	target := contract.RuntimeLeaseBinding{TenantID: "tenant-1", InstanceID: "instance-new", InstanceEpoch: 2, LeaseID: "lease-new", LeaseEpoch: 2, FenceEpoch: 2, ScopeDigest: strings.Repeat("b", contract.DigestSizeHex), ObservedRevision: 1, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	request := contract.WorkspaceRestoreStageRequestV1{TenantID: "tenant-1", DispatchAttemptID: "dispatch-attempt", RuntimeRestoreAttempt: workspaceRestoreExactRefV1("praxis.runtime/restore-attempt/v2", "praxis.runtime/restore-attempt/body/v2", "runtime-attempt", now), RestoreEligibility: workspaceRestoreExactRefV1("praxis.runtime/restore-eligibility/v2", "praxis.runtime/restore-eligibility/body/v2", "eligibility", now), Target: target, SnapshotArtifactFactRef: artifactRef, RequestedNotAfter: now.Add(time.Hour).UnixNano()}
	governanceProjection, err := contract.SealWorkspaceRestoreGovernanceCurrentProjectionV1(contract.WorkspaceRestoreGovernanceCurrentProjectionV1{
		TenantID: "tenant-1", RuntimeRestoreAttempt: request.RuntimeRestoreAttempt, RestoreEligibility: request.RestoreEligibility, Target: target,
		ActionAdmissionRef: workspaceRestoreExactRefV1("praxis.runtime/action-admission/v1", "admission", "admission", now), ReviewAuthorizationRef: workspaceRestoreExactRefV1("praxis.runtime/review-authorization/v5", "authorization", "authorization", now), DispatchPermitRef: workspaceRestoreExactRefV1("praxis.runtime/dispatch-permit/v1", "permit", "permit", now), BeginRef: workspaceRestoreExactRefV1("praxis.runtime/operation-begin/v3", "begin", "begin", now), EnforcementRef: workspaceRestoreExactRefV1("praxis.runtime/enforcement/v1", "enforcement", request.DispatchAttemptID, now),
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := testkit.NewWorkspaceRestoreMemoryStoreV1()
	bundles := &workspaceRestoreBundleReaderV1{value: bundleProjection}
	governance := &workspaceRestoreGovernanceReaderV1{value: governanceProjection}
	provider := &workspaceRestoreProviderFakeV1{}
	owner, err := NewWorkspaceRestoreOwnerV1(store, bundles, governance, provider, func() time.Time { return now }, WorkspaceRestoreOwnerLimitsV1{MaxAttemptTTL: time.Hour, MaxHistoryTTL: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return &workspaceRestoreOwnerFixtureV1{now: now, request: request, bundle: bundleProjection, bundles: bundles, governance: governance, provider: provider, store: store, owner: owner}
}

type workspaceRestoreBundleReaderV1 struct {
	value contract.WorkspaceRestoreBundleCurrentProjectionV1
}

func (r *workspaceRestoreBundleReaderV1) InspectWorkspaceRestoreBundleCurrentV1(context.Context, contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, error) {
	return r.value, nil
}
func (r *workspaceRestoreBundleReaderV1) InspectWorkspaceRestoreBundleExactV1(context.Context, contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, error) {
	return r.value, nil
}

type workspaceRestoreGovernanceReaderV1 struct {
	mu        sync.Mutex
	value     contract.WorkspaceRestoreGovernanceCurrentProjectionV1
	calls     int
	driftCall int
}

func (r *workspaceRestoreGovernanceReaderV1) InspectWorkspaceRestoreGovernanceCurrentV1(context.Context, contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreGovernanceCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.driftCall > 0 && r.calls == r.driftCall {
		value := r.value
		value.EnforcementRef = workspaceRestoreExactRefV1("praxis.runtime/enforcement/v1", "enforcement", "drift", time.Unix(0, value.CheckedUnixNano+int64(time.Second)))
		sealed, _ := contract.SealWorkspaceRestoreGovernanceCurrentProjectionV1(value)
		return sealed, nil
	}
	return r.value, nil
}

type workspaceRestoreProviderFakeV1 struct {
	mu                    sync.Mutex
	stageCalls            int
	inspectCalls          int
	loseReply             bool
	failBeforeMaterialize bool
	root                  *contract.WorkspaceRootRefV1
}

func (p *workspaceRestoreProviderFakeV1) StageWorkspaceRestoreV1(_ context.Context, input *contract.WorkspaceRestoreProviderRequestV1) (contract.WorkspaceRestoreProviderResultV1, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stageCalls++
	if p.failBeforeMaterialize {
		return contract.WorkspaceRestoreProviderResultV1{}, errors.New("injected pre-materialize failure")
	}
	root := p.rootFor(*input)
	p.root = &root
	if p.loseReply {
		p.loseReply = false
		return contract.WorkspaceRestoreProviderResultV1{}, errors.New("injected lost provider reply")
	}
	return contract.WorkspaceRestoreProviderResultV1{RootRef: root, Created: true}, nil
}

func (p *workspaceRestoreProviderFakeV1) InspectWorkspaceRestoreV1(_ context.Context, input *contract.WorkspaceRestoreProviderRequestV1) (contract.WorkspaceRestoreProviderResultV1, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inspectCalls++
	if p.root == nil {
		return contract.WorkspaceRestoreProviderResultV1{}, ports.ErrNotFound
	}
	want := p.rootFor(*input)
	if *p.root != want {
		return contract.WorkspaceRestoreProviderResultV1{}, ports.ErrConflict
	}
	return contract.WorkspaceRestoreProviderResultV1{RootRef: *p.root}, nil
}

func (p *workspaceRestoreProviderFakeV1) rootFor(input contract.WorkspaceRestoreProviderRequestV1) contract.WorkspaceRootRefV1 {
	root, _ := contract.SealWorkspaceRootRefV1(contract.WorkspaceRootRefV1{ID: "workspace-root-test", TenantID: input.Target.TenantID, RestoreAttemptID: input.RuntimeRestoreAttempt.ID, RuntimeRestoreAttempt: input.RuntimeRestoreAttempt, StageAttemptRef: input.StageAttemptRef, Target: input.Target, BundleDigest: input.Bundle.BundleDigest})
	return root
}

func (p *workspaceRestoreProviderFakeV1) materializeFor(attempt contract.WorkspaceRestoreAttemptV1, bundle contract.WorkspaceSnapshotBundleV1) {
	p.mu.Lock()
	defer p.mu.Unlock()
	providerRef := attempt.ExactRef()
	if attempt.ProviderStageAttemptRef != nil {
		providerRef = *attempt.ProviderStageAttemptRef
	}
	root := p.rootFor(contract.WorkspaceRestoreProviderRequestV1{StageAttemptRef: providerRef, RuntimeRestoreAttempt: attempt.Request.RuntimeRestoreAttempt, Target: attempt.Request.Target, Bundle: bundle})
	p.root = &root
}

func workspaceRestoreStorageRefV1(t *testing.T, bundle contract.WorkspaceSnapshotBundleV1, now time.Time) contract.SnapshotStorageArtifactRefV2 {
	t.Helper()
	encoded, _ := contract.EncodeWorkspaceSnapshotBundleV1(bundle)
	digest := sha256.Sum256(encoded)
	ref := func(id string) contract.Ref {
		d, _ := contract.Digest("workspace-restore-test-ref-v1", id)
		return contract.Ref{ID: id, Revision: 1, Digest: d}
	}
	value, err := contract.SealSnapshotStorageArtifactRefV2(contract.SnapshotStorageArtifactRefV2{StorageArtifactID: "storage-1", Revision: 1, TenantID: "tenant-1", DataDomain: "workspace-checkpoint", StorageNamespaceExactRef: workspaceRestoreExactRefV1("praxis.sandbox/storage-namespace/v1", "namespace", "namespace", now.Add(time.Hour)), ContentDigest: hex.EncodeToString(digest[:]), SchemaRef: ref("schema"), Length: uint64(len(encoded)), EncryptionFactRef: ref("encryption"), ResidencyFactRef: ref("residency"), CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func workspaceRestoreExactRefV1(typeURL, domain, id string, now time.Time) contract.SnapshotArtifactExactRefV2 {
	digest, _ := contract.Digest("workspace-restore-test-exact-ref-v1", struct{ TypeURL, ID string }{typeURL, id})
	return contract.SnapshotArtifactExactRefV2{TypeURL: typeURL, Version: 1, ID: id, Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: digest, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
}
