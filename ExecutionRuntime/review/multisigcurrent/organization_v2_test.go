package multisigcurrent_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	organizationcontract "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	organizationcurrent "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/current"
	organizationmemory "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/memory"
	organizationports "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/multisigcurrent"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type organizationFixtureV2 struct {
	now     time.Time
	store   *organizationmemory.Store
	reader  organizationports.ReviewEligibilityCurrentReaderV1
	request reviewport.HumanOrganizationCurrentRequestV2
}

func newOrganizationFixtureV2(t *testing.T) organizationFixtureV2 {
	t.Helper()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	store := organizationmemory.NewStore()
	reviewer := sealOrganizationIdentity(t, "reviewer-a", now, now.Add(time.Hour))
	author := sealOrganizationIdentity(t, "author-a", now, now.Add(55*time.Minute))
	manager := sealOrganizationIdentity(t, "manager-a", now, now.Add(50*time.Minute))
	for _, value := range []organizationcontract.IdentityFactV1{reviewer, author, manager} {
		if err := store.PublishIdentityV1(ctx, nil, value); err != nil {
			t.Fatal(err)
		}
	}
	scope := digestV2("action-scope")
	role, err := organizationcontract.SealRoleGrantV1(organizationcontract.RoleGrantFactV1{FactMetaV1: organizationMetaV2(now, now.Add(40*time.Minute)), Identity: reviewer.ExactRef(), Role: "technical", ScopeDigest: scope})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.PublishRoleGrantV1(ctx, nil, role); err != nil {
		t.Fatal(err)
	}
	delegation, err := organizationcontract.SealDelegationV1(organizationcontract.DelegationFactV1{FactMetaV1: organizationMetaV2(now, now.Add(35*time.Minute)), Delegator: manager.ExactRef(), Delegate: reviewer.ExactRef(), DelegatorSubjectID: "manager-a", DelegateSubjectID: "reviewer-a", Role: "technical", ScopeDigest: scope})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.PublishDelegationV1(ctx, nil, delegation); err != nil {
		t.Fatal(err)
	}
	targetDigest := digestV2("target-a")
	responsibility, err := organizationcontract.SealResponsibilityV1(organizationcontract.ResponsibilityFactV1{FactMetaV1: organizationMetaV2(now, now.Add(45*time.Minute)), SubjectKind: "review-target", SubjectID: "target-a", SubjectDigest: targetDigest, Identity: author.ExactRef()})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.PublishResponsibilityV1(ctx, nil, responsibility); err != nil {
		t.Fatal(err)
	}
	policy := contract.HumanQuorumPolicyBindingV2{TenantID: "tenant-a", Ref: "policy-a", Revision: 1, Digest: digestV2("policy-a"), Domain: "review/human", CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	panel, err := contract.SealHumanReviewPanelV2(contract.HumanReviewPanelV2{
		FactIdentityV1:              contract.FactIdentityV1{TenantID: "tenant-a", Revision: 1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.Add(-time.Minute).UnixNano()},
		Case:                        contract.HumanCaseExactRefV2{TenantID: "tenant-a", ID: "case-a", Revision: 1, Digest: digestV2("case-a")},
		Target:                      contract.HumanTargetExactRefV2{TenantID: "tenant-a", ID: "target-a", Revision: 1, Digest: targetDigest},
		Round:                       contract.HumanRoundExactRefV2{TenantID: "tenant-a", ID: "round-a", Revision: 1, Digest: digestV2("round-a")},
		QuorumPolicy:                policy,
		ResponsibilitySubject:       contract.HumanResponsibilitySubjectRefV2{TenantID: "tenant-a", Ref: responsibility.ID, Revision: responsibility.Revision, Digest: responsibility.Digest, IdentityProof: contract.HumanIdentityProofRefV2{TenantID: "tenant-a", Ref: author.ID, Revision: author.Revision, Digest: author.Digest}},
		State:                       contract.HumanPanelProposedV2,
		AcceptThreshold:             1,
		MaximumPanelSize:            1,
		RoleRequirements:            []contract.HumanRoleRequirementV2{{Role: "technical", Minimum: 1}},
		RejectVetoRoles:             []string{"security"},
		DelegationRequired:          true,
		ProductionSelfReviewAllowed: false,
		MaxPanelDurationNanos:       int64(time.Hour),
		MaxVoteTTLNanos:             int64(10 * time.Minute),
		ExpiresUnixNano:             now.Add(50 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := contract.SealHumanPanelAssignmentV2(contract.HumanPanelAssignmentV2{
		FactIdentityV1:        contract.FactIdentityV1{TenantID: "tenant-a", ID: "assignment-a", Revision: 1, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano()},
		Panel:                 panel.ExactRef(),
		Case:                  panel.Case,
		Round:                 panel.Round,
		Target:                panel.Target,
		ReviewerIdentity:      contract.HumanIdentityProofRefV2{TenantID: "tenant-a", Ref: reviewer.ID, Revision: reviewer.Revision, Digest: reviewer.Digest},
		ReviewerAuthority:     runtimeports.AuthorityBindingRefV2{Ref: "authority-reviewer-a", Digest: digestV2("authority-reviewer-a"), Revision: 1, Epoch: 1},
		ReviewerBinding:       runtimeports.ReviewComponentBindingRefV2{BindingSetID: "binding-a", BindingSetRevision: 1, ComponentID: "review/human", ManifestDigest: digestV2("manifest-a"), ArtifactDigest: digestV2("artifact-a"), Capability: "review/attest"},
		Roles:                 []string{"technical"},
		Delegated:             true,
		DelegatorIdentity:     contract.HumanIdentityProofRefV2{TenantID: "tenant-a", Ref: manager.ID, Revision: manager.Revision, Digest: manager.Digest},
		DelegateIdentity:      contract.HumanIdentityProofRefV2{TenantID: "tenant-a", Ref: reviewer.ID, Revision: reviewer.Revision, Digest: reviewer.Digest},
		DelegationFact:        contract.HumanDelegationFactRefV2{TenantID: "tenant-a", Ref: delegation.ID, Revision: delegation.Revision, Digest: delegation.Digest},
		DelegatedRole:         "technical",
		DelegationScopeDigest: scope,
		State:                 contract.HumanAssignmentOfferedV2,
		ExpiresUnixNano:       now.Add(40 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := organizationcurrent.NewReaderV1(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return organizationFixtureV2{now: now, store: store, reader: reader, request: reviewport.HumanOrganizationCurrentRequestV2{Panel: panel, Assignment: assignment, ReviewerSubjectID: "reviewer-a", DelegatorSubjectID: "manager-a", ActionScopeDigest: scope}}
}

func TestOrganizationCurrentV2ExactS1S2MinimumTTLAndDeepClone(t *testing.T) {
	fixture := newOrganizationFixtureV2(t)
	reader := newReviewOrganizationReaderV2(t, fixture.reader, incrementingClockV2(fixture.now.Add(time.Second)))
	cut, err := reader.InspectHumanOrganizationCurrentV2(context.Background(), []reviewport.HumanOrganizationCurrentRequestV2{fixture.request})
	if err != nil {
		t.Fatal(err)
	}
	if cut.ExpiresUnixNano != fixture.now.Add(35*time.Minute).UnixNano() || len(cut.Items) != 1 {
		t.Fatalf("cut min TTL/items drifted: %+v", cut)
	}
	if cut.Items[0].OwnerProjectionRef.Identity.ID != fixture.request.Assignment.ReviewerIdentity.Ref || cut.Items[0].OwnerProjectionRef.Responsibility.ID != fixture.request.Panel.ResponsibilitySubject.Ref {
		t.Fatal("Organization exact refs were not mapped losslessly")
	}
	cut.Items[0].OwnerProjectionRef.Source.RequiredRoles[0] = "mutated"
	again, err := reader.InspectHumanOrganizationCurrentV2(context.Background(), []reviewport.HumanOrganizationCurrentRequestV2{fixture.request})
	if err != nil {
		t.Fatal(err)
	}
	if again.Items[0].OwnerProjectionRef.Source.RequiredRoles[0] != "technical" {
		t.Fatal("deep-clone boundary leaked a mutable alias")
	}
}

func TestOrganizationCurrentV2ConstructorRejectsTypedNilOwner(t *testing.T) {
	var owner *lostReplyReaderV2
	if _, err := multisigcurrent.NewOrganizationSourceV2(owner, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed nil Owner accepted: %v", err)
	}
}

func TestOrganizationCurrentV2RejectsSubjectScopeVetoAndResponsibilityDrift(t *testing.T) {
	fixture := newOrganizationFixtureV2(t)
	cases := map[string]func(*reviewport.HumanOrganizationCurrentRequestV2){
		"reviewer-subject": func(v *reviewport.HumanOrganizationCurrentRequestV2) { v.ReviewerSubjectID = "other" },
		"scope":            func(v *reviewport.HumanOrganizationCurrentRequestV2) { v.ActionScopeDigest = digestV2("other-scope") },
		"veto": func(v *reviewport.HumanOrganizationCurrentRequestV2) {
			v.Assignment.CanVeto = true
			resealAssignmentV2(t, v)
		},
		"responsibility": func(v *reviewport.HumanOrganizationCurrentRequestV2) {
			v.Panel.ResponsibilitySubject.Ref = "other"
			resealPanelAndAssignmentV2(t, v)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := fixture.request.Clone()
			mutate(&request)
			reader := newReviewOrganizationReaderV2(t, fixture.reader, incrementingClockV2(fixture.now.Add(time.Second)))
			if _, err := reader.InspectHumanOrganizationCurrentV2(context.Background(), []reviewport.HumanOrganizationCurrentRequestV2{request}); err == nil {
				t.Fatal("drift was accepted")
			}
		})
	}
}

type lostReplyReaderV2 struct {
	organizationports.ReviewEligibilityCurrentReaderV1
	once   atomic.Bool
	cancel context.CancelFunc
}

type lostResolveReaderV2 struct {
	organizationports.ReviewEligibilityCurrentReaderV1
	once   atomic.Bool
	calls  atomic.Int64
	cancel context.CancelFunc
}

func (r *lostResolveReaderV2) ResolveCurrentReviewEligibilityV1(ctx context.Context, source organizationcontract.ReviewEligibilitySourceV1) (organizationcontract.ReviewEligibilityCurrentProjectionV1, error) {
	r.calls.Add(1)
	if !r.once.Swap(true) {
		r.cancel()
		return organizationcontract.ReviewEligibilityCurrentProjectionV1{}, organizationports.IndeterminateV1("unknown Resolve response")
	}
	return r.ReviewEligibilityCurrentReaderV1.ResolveCurrentReviewEligibilityV1(ctx, source)
}

func (r *lostReplyReaderV2) InspectCurrentReviewEligibilityV1(ctx context.Context, ref organizationcontract.ReviewEligibilityProjectionRefV1) (organizationcontract.ReviewEligibilityCurrentProjectionV1, error) {
	if !r.once.Swap(true) {
		r.cancel()
		return organizationcontract.ReviewEligibilityCurrentProjectionV1{}, organizationports.IndeterminateV1("lost exact Inspect reply")
	}
	return r.ReviewEligibilityCurrentReaderV1.InspectCurrentReviewEligibilityV1(ctx, ref)
}

func TestOrganizationCurrentV2LostExactReplyUsesDetachedInspect(t *testing.T) {
	fixture := newOrganizationFixtureV2(t)
	ctx, cancel := context.WithCancel(context.Background())
	wrapped := &lostReplyReaderV2{ReviewEligibilityCurrentReaderV1: fixture.reader, cancel: cancel}
	reader := newReviewOrganizationReaderV2(t, wrapped, incrementingClockV2(fixture.now.Add(time.Second)))
	cut, err := reader.InspectHumanOrganizationCurrentV2(ctx, []reviewport.HumanOrganizationCurrentRequestV2{fixture.request})
	if err != nil {
		t.Fatal(err)
	}
	if len(cut.Items) != 1 || !wrapped.once.Load() {
		t.Fatal("lost exact reply was not recovered from the original ref")
	}
}

func TestOrganizationCurrentV2UnknownResolveStartsDetachedNewS1(t *testing.T) {
	fixture := newOrganizationFixtureV2(t)
	ctx, cancel := context.WithCancel(context.Background())
	wrapped := &lostResolveReaderV2{ReviewEligibilityCurrentReaderV1: fixture.reader, cancel: cancel}
	reader := newReviewOrganizationReaderV2(t, wrapped, incrementingClockV2(fixture.now.Add(time.Second)))
	cut, err := reader.InspectHumanOrganizationCurrentV2(ctx, []reviewport.HumanOrganizationCurrentRequestV2{fixture.request})
	if err != nil {
		t.Fatal(err)
	}
	if len(cut.Items) != 1 || wrapped.calls.Load() != 2 {
		t.Fatal("unknown Resolve did not start one detached new S1")
	}
}

func TestOrganizationCurrentV2ClockRollbackAndTTLCrossingFailClosed(t *testing.T) {
	fixture := newOrganizationFixtureV2(t)
	var calls atomic.Int64
	rollback := func() time.Time {
		n := calls.Add(1)
		if n > 8 {
			return fixture.now.Add(time.Second)
		}
		return fixture.now.Add(time.Second + time.Duration(n)*time.Nanosecond)
	}
	reader := newReviewOrganizationReaderV2(t, fixture.reader, rollback)
	if _, err := reader.InspectHumanOrganizationCurrentV2(context.Background(), []reviewport.HumanOrganizationCurrentRequestV2{fixture.request}); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback category=%v", err)
	}
	reader = newReviewOrganizationReaderV2(t, fixture.reader, func() time.Time { return fixture.now.Add(2 * time.Hour) })
	if _, err := reader.InspectHumanOrganizationCurrentV2(context.Background(), []reviewport.HumanOrganizationCurrentRequestV2{fixture.request}); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL crossing category=%v", err)
	}
}

func TestOrganizationCurrentV2OwnerRevocationDriftAndSelfReviewFailClosed(t *testing.T) {
	t.Run("role-revoked", func(t *testing.T) {
		fixture := newOrganizationFixtureV2(t)
		source, _ := fixture.request.OrganizationSourceV1()
		closure, err := fixture.store.ReadReviewEligibilityClosureV1(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		next := closure.Roles[0]
		next.Revision++
		next.UpdatedUnixNano = fixture.now.Add(2 * time.Second).UnixNano()
		next.State = organizationcontract.FactRevokedV1
		next, err = organizationcontract.SealRoleGrantV1(next)
		if err != nil {
			t.Fatal(err)
		}
		expected := closure.Roles[0].ExactRef()
		if err = fixture.store.PublishRoleGrantV1(context.Background(), &expected, next); err != nil {
			t.Fatal(err)
		}
		reader := newReviewOrganizationReaderV2(t, newOwnerReaderAtV2(t, fixture.store, fixture.now.Add(3*time.Second)), incrementingClockV2(fixture.now.Add(3*time.Second)))
		if _, err = reader.InspectHumanOrganizationCurrentV2(context.Background(), []reviewport.HumanOrganizationCurrentRequestV2{fixture.request}); !core.HasCategory(err, core.ErrorPreconditionFailed) {
			t.Fatalf("revoked role category=%v", err)
		}
	})
	t.Run("delegation-revoked", func(t *testing.T) {
		fixture := newOrganizationFixtureV2(t)
		source, _ := fixture.request.OrganizationSourceV1()
		closure, err := fixture.store.ReadReviewEligibilityClosureV1(context.Background(), source)
		if err != nil || closure.Delegation == nil {
			t.Fatal(err)
		}
		next := *closure.Delegation
		next.Revision++
		next.UpdatedUnixNano = fixture.now.Add(2 * time.Second).UnixNano()
		next.State = organizationcontract.FactRevokedV1
		next, err = organizationcontract.SealDelegationV1(next)
		if err != nil {
			t.Fatal(err)
		}
		expected := closure.Delegation.ExactRef()
		if err = fixture.store.PublishDelegationV1(context.Background(), &expected, next); err != nil {
			t.Fatal(err)
		}
		reader := newReviewOrganizationReaderV2(t, newOwnerReaderAtV2(t, fixture.store, fixture.now.Add(3*time.Second)), incrementingClockV2(fixture.now.Add(3*time.Second)))
		if _, err = reader.InspectHumanOrganizationCurrentV2(context.Background(), []reviewport.HumanOrganizationCurrentRequestV2{fixture.request}); !core.HasCategory(err, core.ErrorPreconditionFailed) {
			t.Fatalf("revoked delegation category=%v", err)
		}
	})
	t.Run("production-self-review", func(t *testing.T) {
		fixture := newOrganizationFixtureV2(t)
		source, _ := fixture.request.OrganizationSourceV1()
		closure, err := fixture.store.ReadReviewEligibilityClosureV1(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		next := closure.Responsibility
		next.Revision++
		next.UpdatedUnixNano = fixture.now.Add(2 * time.Second).UnixNano()
		next.Identity = closure.Identity.ExactRef()
		next, err = organizationcontract.SealResponsibilityV1(next)
		if err != nil {
			t.Fatal(err)
		}
		expected := closure.Responsibility.ExactRef()
		if err = fixture.store.PublishResponsibilityV1(context.Background(), &expected, next); err != nil {
			t.Fatal(err)
		}
		reader := newReviewOrganizationReaderV2(t, newOwnerReaderAtV2(t, fixture.store, fixture.now.Add(3*time.Second)), incrementingClockV2(fixture.now.Add(3*time.Second)))
		if _, err = reader.InspectHumanOrganizationCurrentV2(context.Background(), []reviewport.HumanOrganizationCurrentRequestV2{fixture.request}); !core.HasCategory(err, core.ErrorForbidden) {
			t.Fatalf("self-review category=%v", err)
		}
	})
}

func TestOrganizationCurrentV2ConcurrentReadsAreConsistent(t *testing.T) {
	fixture := newOrganizationFixtureV2(t)
	reader := newReviewOrganizationReaderV2(t, fixture.reader, incrementingClockV2(fixture.now.Add(time.Second)))
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cut, err := reader.InspectHumanOrganizationCurrentV2(context.Background(), []reviewport.HumanOrganizationCurrentRequestV2{fixture.request})
			if err == nil && len(cut.Items) != 1 {
				err = core.NewError(core.ErrorInternal, core.ReasonInvalidState, "incomplete concurrent cut")
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
}

func TestOrganizationCurrentV2Conformance(t *testing.T) {
	conformance.RunHumanOrganizationCurrentV2(t, func(t *testing.T) conformance.HumanOrganizationCurrentFixtureV2 {
		fixture := newOrganizationFixtureV2(t)
		return conformance.HumanOrganizationCurrentFixtureV2{Reader: newReviewOrganizationReaderV2(t, fixture.reader, incrementingClockV2(fixture.now.Add(time.Second))), Request: fixture.request}
	})
}

func newReviewOrganizationReaderV2(t *testing.T, owner organizationports.ReviewEligibilityCurrentReaderV1, clock func() time.Time) *multisigcurrent.OrganizationSourceV2 {
	t.Helper()
	reader, err := multisigcurrent.NewOrganizationSourceV2(owner, clock)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func newOwnerReaderAtV2(t *testing.T, store organizationports.StoreV1, now time.Time) organizationports.ReviewEligibilityCurrentReaderV1 {
	t.Helper()
	reader, err := organizationcurrent.NewReaderV1(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func sealOrganizationIdentity(t *testing.T, subject string, now, expires time.Time) organizationcontract.IdentityFactV1 {
	t.Helper()
	v, err := organizationcontract.SealIdentityV1(organizationcontract.IdentityFactV1{FactMetaV1: organizationMetaV2(now, expires), SubjectKind: organizationcontract.SubjectHumanV1, SubjectID: subject, DisplayHandle: subject})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func organizationMetaV2(now, expires time.Time) organizationcontract.FactMetaV1 {
	return organizationcontract.FactMetaV1{TenantID: "tenant-a", Revision: 1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires.UnixNano(), State: organizationcontract.FactActiveV1}
}

func digestV2(value string) core.Digest { return core.DigestBytes([]byte(value)) }

func incrementingClockV2(start time.Time) func() time.Time {
	var tick atomic.Int64
	return func() time.Time { return start.Add(time.Duration(tick.Add(1)) * time.Nanosecond) }
}

func resealAssignmentV2(t *testing.T, request *reviewport.HumanOrganizationCurrentRequestV2) {
	t.Helper()
	request.Assignment.Digest = ""
	sealed, err := contract.SealHumanPanelAssignmentV2(request.Assignment)
	if err != nil {
		t.Fatal(err)
	}
	request.Assignment = sealed
}

func resealPanelAndAssignmentV2(t *testing.T, request *reviewport.HumanOrganizationCurrentRequestV2) {
	t.Helper()
	request.Panel.Digest = ""
	panel, err := contract.SealHumanReviewPanelV2(request.Panel)
	if err != nil {
		t.Fatal(err)
	}
	request.Panel = panel
	request.Assignment.Panel = panel.ExactRef()
	request.Assignment.Case, request.Assignment.Round, request.Assignment.Target = panel.Case, panel.Round, panel.Target
	resealAssignmentV2(t, request)
}
