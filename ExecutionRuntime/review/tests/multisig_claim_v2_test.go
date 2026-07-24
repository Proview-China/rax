package review_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	organizationcontract "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/multisigowner"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	reviewsqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type staticOrganizationCutV2 struct {
	cut   reviewport.HumanOrganizationCurrentCutV2
	err   error
	calls atomic.Int64
}

func (r *staticOrganizationCutV2) InspectHumanOrganizationCurrentV2(_ context.Context, _ []reviewport.HumanOrganizationCurrentRequestV2) (reviewport.HumanOrganizationCurrentCutV2, error) {
	r.calls.Add(1)
	return r.cut.Clone(), r.err
}

type lostReplyClaimStoreV2 struct {
	multiStore
	cancel context.CancelFunc
	calls  atomic.Int64
}

type blockingCanonicalClaimStoreV2 struct {
	multiStore
	calls atomic.Int64
}

func (s *blockingCanonicalClaimStoreV2) InspectHumanPanelAssignmentExactV2(ctx context.Context, _ contract.HumanPanelAssignmentExactRefV2) (contract.HumanPanelAssignmentV2, error) {
	s.calls.Add(1)
	<-ctx.Done()
	return contract.HumanPanelAssignmentV2{}, ctx.Err()
}

func (s *lostReplyClaimStoreV2) ClaimHumanAssignmentV2(ctx context.Context, m reviewport.ClaimHumanAssignmentMutationV2) (reviewport.ClaimHumanAssignmentResultV2, error) {
	s.calls.Add(1)
	result, err := s.multiStore.ClaimHumanAssignmentV2(ctx, m)
	if err == nil {
		if s.cancel != nil {
			s.cancel()
		}
		return reviewport.ClaimHumanAssignmentResultV2{}, indeterminateReply()
	}
	return result, err
}

func claimMutationV2(t *testing.T, f multiFixture) reviewport.ClaimHumanAssignmentMutationV2 {
	t.Helper()
	assignment := f.create.Assignments[0].Clone()
	assignment.Revision++
	assignment.UpdatedUnixNano = f.now.Add(2 * time.Second).UnixNano()
	assignment.State = contract.HumanAssignmentClaimedV2
	assignment.LeaseHolder = "reviewer-a"
	assignment.LeaseExpiresUnixNano = f.now.Add(8 * time.Minute).UnixNano()
	assignment.Digest = ""
	var err error
	assignment, err = contract.SealHumanPanelAssignmentV2(assignment)
	if err != nil {
		t.Fatal(err)
	}
	panel := f.create.OpenPanel.Clone()
	panel.Revision++
	panel.UpdatedUnixNano = f.now.Add(2 * time.Second).UnixNano()
	panel.AssignmentRefs = append([]contract.HumanPanelAssignmentExactRefV2(nil), panel.AssignmentRefs...)
	for index, ref := range panel.AssignmentRefs {
		if ref == f.create.Assignments[0].ExactRef() {
			panel.AssignmentRefs[index] = assignment.ExactRef()
		}
	}
	panel.Digest = ""
	panel, err = contract.SealHumanReviewPanelV2(panel)
	if err != nil {
		t.Fatal(err)
	}
	return reviewport.ClaimHumanAssignmentMutationV2{
		ExpectedPanel:      f.create.OpenPanel.ExactRef(),
		ExpectedAssignment: f.create.Assignments[0].ExactRef(),
		NextPanel:          panel,
		NextAssignment:     assignment,
		Trace:              testClaimTraceV2(t, f, assignment),
	}
}

func organizationReadyClaimFixtureV2(t *testing.T, store multiStore) multiFixture {
	t.Helper()
	f := prepareFixture(t, store)
	tenant := f.target.TenantID
	managerID, err := organizationcontract.DeriveIdentityIDV1(tenant, "manager-a")
	if err != nil {
		t.Fatal(err)
	}
	authorID, err := organizationcontract.DeriveIdentityIDV1(tenant, "author-a")
	if err != nil {
		t.Fatal(err)
	}
	responsibilityID, err := organizationcontract.DeriveResponsibilityIDV1(tenant, "review-target", f.target.ID)
	if err != nil {
		t.Fatal(err)
	}
	proposed := f.create.ProposedPanel.Clone()
	proposed.ResponsibilitySubject = contract.HumanResponsibilitySubjectRefV2{TenantID: tenant, Ref: responsibilityID, Revision: 1, Digest: hd("org-responsibility"), IdentityProof: contract.HumanIdentityProofRefV2{TenantID: tenant, Ref: authorID, Revision: 1, Digest: hd("org-author")}}
	proposed.Digest = ""
	proposed, err = contract.SealHumanReviewPanelV2(proposed)
	if err != nil {
		t.Fatal(err)
	}
	assignments := append([]contract.HumanPanelAssignmentV2(nil), f.create.Assignments...)
	for index := range assignments {
		reviewerSubject := "reviewer-a"
		if index == 1 {
			reviewerSubject = "reviewer-b"
		}
		reviewerID, deriveErr := organizationcontract.DeriveIdentityIDV1(tenant, reviewerSubject)
		if deriveErr != nil {
			t.Fatal(deriveErr)
		}
		delegationID, deriveErr := organizationcontract.DeriveDelegationIDV1(tenant, "manager-a", reviewerSubject, assignments[index].DelegatedRole, assignments[index].DelegationScopeDigest)
		if deriveErr != nil {
			t.Fatal(deriveErr)
		}
		a := assignments[index].Clone()
		a.Panel = proposed.ExactRef()
		a.ReviewerIdentity = contract.HumanIdentityProofRefV2{TenantID: tenant, Ref: reviewerID, Revision: 1, Digest: hd("org-" + reviewerSubject)}
		a.DelegateIdentity = a.ReviewerIdentity
		a.DelegatorIdentity = contract.HumanIdentityProofRefV2{TenantID: tenant, Ref: managerID, Revision: 1, Digest: hd("org-manager")}
		a.DelegationFact = contract.HumanDelegationFactRefV2{TenantID: tenant, Ref: delegationID, Revision: 1, Digest: hd("org-delegation-" + reviewerSubject)}
		a.Digest = ""
		a, err = contract.SealHumanPanelAssignmentV2(a)
		if err != nil {
			t.Fatal(err)
		}
		assignments[index] = a
	}
	open := proposed.Clone()
	open.Revision = proposed.Revision + 1
	open.State = contract.HumanPanelOpenV2
	open.UpdatedUnixNano = proposed.UpdatedUnixNano + 1
	open.AssignmentRefs = []contract.HumanPanelAssignmentExactRefV2{assignments[0].ExactRef(), assignments[1].ExactRef()}
	open.Digest = ""
	open, err = contract.SealHumanReviewPanelV2(open)
	if err != nil {
		t.Fatal(err)
	}
	f.create.ProposedPanel, f.create.Assignments, f.create.OpenPanel = proposed, assignments, open
	if _, err = store.CreateHumanPanelV2(context.Background(), f.create); err != nil {
		t.Fatal(err)
	}
	return f
}

func claimOrganizationRequestV2(f multiFixture) reviewport.HumanOrganizationCurrentRequestV2 {
	return reviewport.HumanOrganizationCurrentRequestV2{Panel: f.create.OpenPanel, Assignment: f.create.Assignments[0], ReviewerSubjectID: "reviewer-a", DelegatorSubjectID: "manager-a", ActionScopeDigest: f.create.Assignments[0].DelegationScopeDigest}
}

func organizationCutForClaimV2(t *testing.T, request reviewport.HumanOrganizationCurrentRequestV2, checked, expires time.Time) reviewport.HumanOrganizationCurrentCutV2 {
	t.Helper()
	source, err := request.OrganizationSourceV1()
	if err != nil {
		t.Fatal(err)
	}
	roles := make([]organizationcontract.RoleGrantRefV1, 0, len(request.Assignment.Roles))
	for _, role := range request.Assignment.Roles {
		roles = append(roles, organizationcontract.RoleGrantRefV1{TenantID: request.Panel.TenantID, ID: "role-" + role, Revision: 1, Digest: hd("org-role-" + role)})
	}
	var delegation *organizationcontract.DelegationRefV1
	if request.Assignment.Delegated {
		value := organizationcontract.DelegationRefV1{TenantID: request.Assignment.DelegationFact.TenantID, ID: request.Assignment.DelegationFact.Ref, Revision: request.Assignment.DelegationFact.Revision, Digest: request.Assignment.DelegationFact.Digest}
		delegation = &value
	}
	projectionDigest := hd("organization-projection-" + request.Assignment.ID)
	ref := organizationcontract.ReviewEligibilityProjectionRefV1{
		TenantID: request.Panel.TenantID, ID: "organization-projection-" + request.Assignment.ID, Source: source,
		Identity: organizationcontract.IdentityRefV1{TenantID: request.Assignment.ReviewerIdentity.TenantID, ID: request.Assignment.ReviewerIdentity.Ref, Revision: request.Assignment.ReviewerIdentity.Revision, Digest: request.Assignment.ReviewerIdentity.Digest},
		Roles:    roles, Delegation: delegation,
		Responsibility: organizationcontract.ResponsibilityRefV1{TenantID: request.Panel.ResponsibilitySubject.TenantID, ID: request.Panel.ResponsibilitySubject.Ref, Revision: request.Panel.ResponsibilitySubject.Revision, Digest: request.Panel.ResponsibilitySubject.Digest},
		Digest:         projectionDigest,
	}
	requestDigest, err := request.Digest()
	if err != nil {
		t.Fatal(err)
	}
	cut, err := reviewport.SealHumanOrganizationCurrentCutV2(reviewport.HumanOrganizationCurrentCutV2{
		TenantID:        request.Panel.TenantID,
		Items:           []reviewport.HumanOrganizationAssignmentCurrentV2{{RequestDigest: requestDigest, Assignment: request.Assignment.ExactRef(), ReviewerIdentity: request.Assignment.ReviewerIdentity, OwnerProjectionRef: ref, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano(), ProjectionDigest: projectionDigest}},
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return cut
}

func claimWithExactLeaseV2(t *testing.T, f multiFixture, expiry time.Time) reviewport.ClaimHumanAssignmentMutationV2 {
	t.Helper()
	claim := claimMutationV2(t, f)
	claim.NextAssignment.LeaseExpiresUnixNano = expiry.UnixNano()
	claim.NextAssignment.Digest = ""
	var err error
	claim.NextAssignment, err = contract.SealHumanPanelAssignmentV2(claim.NextAssignment)
	if err != nil {
		t.Fatal(err)
	}
	claim.NextPanel.AssignmentRefs = append([]contract.HumanPanelAssignmentExactRefV2(nil), f.create.OpenPanel.AssignmentRefs...)
	for index, ref := range claim.NextPanel.AssignmentRefs {
		if ref == claim.ExpectedAssignment {
			claim.NextPanel.AssignmentRefs[index] = claim.NextAssignment.ExactRef()
		}
	}
	claim.NextPanel.Digest = ""
	claim.NextPanel, err = contract.SealHumanReviewPanelV2(claim.NextPanel)
	if err != nil {
		t.Fatal(err)
	}
	claim.Trace = testClaimTraceV2(t, f, claim.NextAssignment)
	return claim
}

func testClaimTraceV2(t *testing.T, f multiFixture, assignment contract.HumanPanelAssignmentV2) contract.TraceFactV1 {
	t.Helper()
	trace, err := contract.SealTraceFactV1(contract.TraceFactV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: assignment.TenantID, ID: "trace-claim-" + assignment.ID, Revision: 1, CreatedUnixNano: assignment.UpdatedUnixNano, UpdatedUnixNano: assignment.UpdatedUnixNano},
		CaseID:         f.caseWaiting.ID, CaseRevision: f.caseWaiting.Revision, TargetID: f.target.ID, TargetRevision: f.target.Revision, TargetDigest: f.target.Digest,
		Event: contract.TraceStartedV1, SourceID: "review.claim-owner", SourceEpoch: 1, SourceSequence: 20, CausationID: assignment.ID, CorrelationID: f.create.OpenPanel.ID, FactRefs: []string{assignment.ID, f.create.OpenPanel.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	return trace
}

func openPanelForClaimV2(t *testing.T, store multiStore) multiFixture {
	t.Helper()
	f := prepareFixture(t, store)
	if _, err := store.CreateHumanPanelV2(context.Background(), f.create); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestHumanMultiSignClaimV2MemoryConcurrentCanonical(t *testing.T) {
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := openPanelForClaimV2(t, store)
	claim := claimMutationV2(t, f)
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := store.ClaimHumanAssignmentV2(context.Background(), claim)
			if err == nil && (got.Panel.Digest != claim.NextPanel.Digest || got.Assignment.Digest != claim.NextAssignment.Digest) {
				err = core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "claim replay returned a different closure")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	current, err := store.InspectHumanPanelAssignmentCurrentV2(context.Background(), claim.NextAssignment.TenantID, claim.NextAssignment.ID)
	if err != nil || current.Digest != claim.NextAssignment.Digest {
		t.Fatalf("claim current Assignment drifted: %v", err)
	}
	old, err := store.InspectHumanPanelAssignmentExactV2(context.Background(), claim.ExpectedAssignment)
	if err != nil || old.State != contract.HumanAssignmentOfferedV2 {
		t.Fatalf("claim overwrote Assignment history: %v", err)
	}
}

func TestHumanMultiSignClaimV2ReusableConformance(t *testing.T) {
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := openPanelForClaimV2(t, store)
	if err := conformance.CheckHumanClaimStoreV2(context.Background(), store, conformance.HumanClaimStoreFixtureV2{Claim: claimMutationV2(t, f)}); err != nil {
		t.Fatal(err)
	}
}

func TestHumanMultiSignClaimV2SQLiteRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-claim.sqlite")
	store, err := reviewsqlite.Open(context.Background(), reviewsqlite.Config{Path: path, Clock: func() time.Time { return time.Unix(1_900_000_010, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	f := openPanelForClaimV2(t, store)
	claim := claimMutationV2(t, f)
	if _, err = store.ClaimHumanAssignmentV2(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = reviewsqlite.Open(context.Background(), reviewsqlite.Config{Path: path, Clock: func() time.Time { return time.Unix(1_900_000_010, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.InspectHumanPanelExactV2(context.Background(), claim.ExpectedPanel); err != nil {
		t.Fatalf("restart lost historical Panel: %v", err)
	}
	if _, err = store.InspectHumanPanelAssignmentExactV2(context.Background(), claim.NextAssignment.ExactRef()); err != nil {
		t.Fatalf("restart lost claimed Assignment: %v", err)
	}
}

func TestHumanMultiSignClaimV2StagedTraceConflictZeroWrite(t *testing.T) {
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := openPanelForClaimV2(t, store)
	claim := claimMutationV2(t, f)
	conflict := claim.Trace
	conflict.FactRefs = []string{"different"}
	conflict.Digest = ""
	var err error
	conflict, err = contract.SealTraceFactV1(conflict)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.InjectTraceForTestV1(context.Background(), conflict); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimHumanAssignmentV2(context.Background(), claim); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("claim Trace conflict accepted: %v", err)
	}
	if _, err = store.InspectHumanPanelAssignmentExactV2(context.Background(), claim.NextAssignment.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed claim leaked Assignment history: %v", err)
	}
	panel, err := store.InspectHumanPanelCurrentV2(context.Background(), claim.ExpectedPanel.TenantID, claim.ExpectedPanel.ID)
	if err != nil || panel.Digest != claim.ExpectedPanel.Digest {
		t.Fatalf("failed claim moved Panel current: %v", err)
	}
}

func TestHumanMultiSignClaimV2RejectsChangedReplayAndStalePanel(t *testing.T) {
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := openPanelForClaimV2(t, store)
	claim := claimMutationV2(t, f)
	if _, err := store.ClaimHumanAssignmentV2(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	changed := claim
	changed.NextAssignment = claim.NextAssignment.Clone()
	changed.NextAssignment.LeaseExpiresUnixNano--
	changed.NextAssignment.Digest = ""
	changed.NextAssignment, _ = contract.SealHumanPanelAssignmentV2(changed.NextAssignment)
	if _, err := store.ClaimHumanAssignmentV2(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("changed claim replay accepted: %v", err)
	}
	changedTrace := claim
	changedTrace.Trace.ID += "-changed"
	changedTrace.Trace.SourceSequence++
	changedTrace.Trace.Digest = ""
	changedTrace.Trace, _ = contract.SealTraceFactV1(changedTrace.Trace)
	if _, err := store.ClaimHumanAssignmentV2(context.Background(), changedTrace); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("claim replay changed exact Trace: %v", err)
	}
	second := f.create.Assignments[1]
	stale := claim
	stale.ExpectedAssignment = second.ExactRef()
	stale.NextAssignment = second
	stale.NextAssignment.Revision++
	stale.NextAssignment.UpdatedUnixNano = f.now.Add(3 * time.Second).UnixNano()
	stale.NextAssignment.State = contract.HumanAssignmentClaimedV2
	stale.NextAssignment.LeaseHolder = "reviewer-b"
	stale.NextAssignment.LeaseExpiresUnixNano = f.now.Add(8 * time.Minute).UnixNano()
	stale.NextAssignment.Digest = ""
	stale.NextAssignment, _ = contract.SealHumanPanelAssignmentV2(stale.NextAssignment)
	if _, err := store.ClaimHumanAssignmentV2(context.Background(), stale); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale Panel claim accepted: %v", err)
	}
}

func TestHumanMultiSignPanelCreateRejectsPreclaimedAssignment(t *testing.T) {
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := prepareFixture(t, store)
	bad := f.create
	bad.Assignments = append([]contract.HumanPanelAssignmentV2(nil), bad.Assignments...)
	a := bad.Assignments[0].Clone()
	a.State = contract.HumanAssignmentClaimedV2
	a.LeaseHolder = "reviewer-a"
	a.LeaseExpiresUnixNano = f.now.Add(8 * time.Minute).UnixNano()
	a.Digest = ""
	var err error
	a, err = contract.SealHumanPanelAssignmentV2(a)
	if err != nil {
		t.Fatal(err)
	}
	bad.Assignments[0] = a
	bad.OpenPanel.AssignmentRefs = []contract.HumanPanelAssignmentExactRefV2{a.ExactRef(), bad.Assignments[1].ExactRef()}
	bad.OpenPanel.Digest = ""
	bad.OpenPanel, err = contract.SealHumanReviewPanelV2(bad.OpenPanel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateHumanPanelV2(context.Background(), bad); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("preclaimed Assignment admitted at Panel create: %v", err)
	}
	if _, err = store.InspectHumanPanelCurrentV2(context.Background(), bad.OpenPanel.TenantID, bad.OpenPanel.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("preclaimed Panel failure leaked current: %v", err)
	}
}

func TestHumanMultiSignClaimOwnerV2OrganizationCurrentTTLAndReplay(t *testing.T) {
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := organizationReadyClaimFixtureV2(t, store)
	request := claimOrganizationRequestV2(f)
	baseline := f.now.Add(3 * time.Second)
	reader := &staticOrganizationCutV2{cut: organizationCutForClaimV2(t, request, baseline.Add(-time.Nanosecond), f.now.Add(30*time.Minute))}
	clock := func() func() time.Time {
		var calls atomic.Int64
		return func() time.Time { return baseline.Add(time.Duration(calls.Add(1)-1) * time.Nanosecond) }
	}()
	owner, err := multisigowner.NewClaimOwnerV2(store, reader, clock)
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Unix(0, f.now.Add(2*time.Second).UnixNano()+f.create.OpenPanel.MaxVoteTTLNanos)
	claim := claimWithExactLeaseV2(t, f, expiry)
	first, err := owner.ClaimAssignmentV2(context.Background(), claim, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Assignment.LeaseExpiresUnixNano != expiry.UnixNano() || first.Assignment.LeaseHolder != request.ReviewerSubjectID {
		t.Fatalf("claim lease did not bind principal/min TTL: %+v", first.Assignment)
	}
	again, err := owner.ClaimAssignmentV2(context.Background(), claim, request)
	if err != nil {
		t.Fatal(err)
	}
	if again.Assignment.Digest != first.Assignment.Digest || again.Panel.Digest != first.Panel.Digest {
		t.Fatal("same canonical claim replay returned a different closure")
	}
}

func TestHumanMultiSignClaimOwnerV2InitialCanonicalInspectRespectsCallerCancellation(t *testing.T) {
	base := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := organizationReadyClaimFixtureV2(t, base)
	request := claimOrganizationRequestV2(f)
	baseline := f.now.Add(3 * time.Second)
	reader := &staticOrganizationCutV2{cut: organizationCutForClaimV2(t, request, baseline.Add(-time.Nanosecond), f.now.Add(30*time.Minute))}
	store := &blockingCanonicalClaimStoreV2{multiStore: base}
	owner, err := multisigowner.NewClaimOwnerV2(store, reader, func() time.Time { return baseline })
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Unix(0, f.now.Add(2*time.Second).UnixNano()+f.create.OpenPanel.MaxVoteTTLNanos)
	claim := claimWithExactLeaseV2(t, f, expiry)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, err = owner.ClaimAssignmentV2(ctx, claim, request)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("initial exact Inspect did not preserve caller cancellation: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= 500*time.Millisecond || store.calls.Load() != 1 {
		t.Fatalf("cancelled initial exact Inspect blocked or retried: elapsed=%v calls=%d", elapsed, store.calls.Load())
	}
}

func TestHumanMultiSignClaimOwnerV2LostReplyUsesDetachedExactInspect(t *testing.T) {
	base := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := organizationReadyClaimFixtureV2(t, base)
	request := claimOrganizationRequestV2(f)
	baseline := f.now.Add(3 * time.Second)
	reader := &staticOrganizationCutV2{cut: organizationCutForClaimV2(t, request, baseline.Add(-time.Nanosecond), f.now.Add(30*time.Minute))}
	ctx, cancel := context.WithCancel(context.Background())
	store := &lostReplyClaimStoreV2{multiStore: base, cancel: cancel}
	var calls atomic.Int64
	owner, err := multisigowner.NewClaimOwnerV2(store, reader, func() time.Time { return baseline.Add(time.Duration(calls.Add(1)-1) * time.Nanosecond) })
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Unix(0, f.now.Add(2*time.Second).UnixNano()+f.create.OpenPanel.MaxVoteTTLNanos)
	claim := claimWithExactLeaseV2(t, f, expiry)
	result, err := owner.ClaimAssignmentV2(ctx, claim, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Assignment.Digest != claim.NextAssignment.Digest || store.calls.Load() != 1 {
		t.Fatalf("lost reply did not recover the exact claim once: calls=%d", store.calls.Load())
	}
}

func TestHumanMultiSignClaimOwnerV2FailClosedBeforeWrite(t *testing.T) {
	tests := map[string]func(*reviewport.ClaimHumanAssignmentMutationV2, *reviewport.HumanOrganizationCurrentRequestV2, *staticOrganizationCutV2){
		"principal-drift": func(_ *reviewport.ClaimHumanAssignmentMutationV2, request *reviewport.HumanOrganizationCurrentRequestV2, _ *staticOrganizationCutV2) {
			request.ReviewerSubjectID = "other-reviewer"
		},
		"lease-ttl-drift": func(claim *reviewport.ClaimHumanAssignmentMutationV2, _ *reviewport.HumanOrganizationCurrentRequestV2, _ *staticOrganizationCutV2) {
			claim.NextAssignment.LeaseExpiresUnixNano--
			claim.NextAssignment.Digest = ""
			claim.NextAssignment, _ = contract.SealHumanPanelAssignmentV2(claim.NextAssignment)
		},
		"organization-projection-drift": func(_ *reviewport.ClaimHumanAssignmentMutationV2, _ *reviewport.HumanOrganizationCurrentRequestV2, reader *staticOrganizationCutV2) {
			reader.cut.Items[0].Assignment.Digest = hd("drift")
			reader.cut.Digest = ""
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
			f := organizationReadyClaimFixtureV2(t, store)
			request := claimOrganizationRequestV2(f)
			baseline := f.now.Add(3 * time.Second)
			reader := &staticOrganizationCutV2{cut: organizationCutForClaimV2(t, request, baseline.Add(-time.Nanosecond), f.now.Add(30*time.Minute))}
			expiry := time.Unix(0, f.now.Add(2*time.Second).UnixNano()+f.create.OpenPanel.MaxVoteTTLNanos)
			claim := claimWithExactLeaseV2(t, f, expiry)
			mutate(&claim, &request, reader)
			var calls atomic.Int64
			owner, err := multisigowner.NewClaimOwnerV2(store, reader, func() time.Time { return baseline.Add(time.Duration(calls.Add(1)-1) * time.Nanosecond) })
			if err != nil {
				t.Fatal(err)
			}
			if _, err = owner.ClaimAssignmentV2(context.Background(), claim, request); err == nil {
				t.Fatal("invalid claim was accepted")
			}
			if _, err = store.InspectHumanPanelAssignmentExactV2(context.Background(), claim.NextAssignment.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("invalid claim leaked Assignment: %v", err)
			}
		})
	}
}

func TestHumanMultiSignClaimOwnerV2ClockRollbackZeroWrite(t *testing.T) {
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := organizationReadyClaimFixtureV2(t, store)
	request := claimOrganizationRequestV2(f)
	baseline := f.now.Add(3 * time.Second)
	reader := &staticOrganizationCutV2{cut: organizationCutForClaimV2(t, request, baseline.Add(-time.Nanosecond), f.now.Add(30*time.Minute))}
	values := []time.Time{baseline, baseline.Add(-time.Nanosecond)}
	var index atomic.Int64
	owner, err := multisigowner.NewClaimOwnerV2(store, reader, func() time.Time {
		value := index.Add(1) - 1
		if value >= int64(len(values)) {
			return values[len(values)-1]
		}
		return values[value]
	})
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Unix(0, f.now.Add(2*time.Second).UnixNano()+f.create.OpenPanel.MaxVoteTTLNanos)
	claim := claimWithExactLeaseV2(t, f, expiry)
	if _, err = owner.ClaimAssignmentV2(context.Background(), claim, request); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback accepted: %v", err)
	}
	if _, err = store.InspectHumanPanelAssignmentExactV2(context.Background(), claim.NextAssignment.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("clock rollback leaked Assignment: %v", err)
	}
}
