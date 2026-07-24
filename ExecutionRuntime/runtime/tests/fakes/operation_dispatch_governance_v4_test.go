package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationDispatchV4IssueLostReplyHistoricalCurrentBeginAndV3Isolation(t *testing.T) {
	fixture := newOperationDispatchFixtureV4(t, "dispatch-issue")
	fixture.effect.store.LoseNextIssueReply()
	issued, err := fixture.gateway.IssueOperationDispatchV4(context.Background(), fixture.issue)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.effect.store.IssueV4CommitCount() != 1 || issued.Record.State != ports.OperationPermitIssuedV4 {
		t.Fatalf("V4 Issue did not linearize once: record=%#v commits=%d", issued.Record, fixture.effect.store.IssueV4CommitCount())
	}
	inspect := ports.InspectOperationDispatchRecordRequestV4{Operation: fixture.issue.Operation, EffectID: fixture.issue.EffectID, PermitID: fixture.issue.PermitID}
	historical, err := fixture.gateway.InspectOperationDispatchRecordV4(context.Background(), inspect)
	if err != nil || historical.Digest != issued.Record.Digest {
		t.Fatalf("historical Inspect did not recover exact record: %#v err=%v", historical, err)
	}
	current, err := fixture.gateway.InspectCurrentOperationDispatchV4(context.Background(), ports.InspectCurrentOperationDispatchRequestV4{Inspect: inspect, AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: fixture.authorization.RefV4()})
	if err != nil || current.Record.Digest != issued.Record.Digest {
		t.Fatalf("current Inspect did not revalidate exact record: %#v err=%v", current, err)
	}
	if _, err := fixture.effect.store.InspectOperationDispatchPermitV3(context.Background(), fixture.issue.Operation, fixture.issue.PermitID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("V3 Inspect masqueraded V4 Permit as legacy: %v", err)
	}
	if _, err := fixture.effect.store.IssueOperationDispatchPermitV3(context.Background(), control.IssueOperationPermitRequestV3{
		Operation: fixture.issue.Operation, EffectID: fixture.issue.EffectID, ExpectedEffectRevision: fixture.issue.ExpectedEffectRevision,
		Permit: issued.Record.Permit.LegacyPermit, Fence: issued.Record.Fence,
	}); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same Permit ID crossed V3/V4 boundary: %v", err)
	}

	fixture.effect.store.LoseNextBeginReply()
	begun, err := fixture.gateway.BeginOperationDispatchV4(context.Background(), beginOperationDispatchRequestV4(fixture, issued))
	if err != nil {
		t.Fatal(err)
	}
	if begun.Record.State != ports.OperationPermitBegunV4 || begun.Record.Enforcement != nil || fixture.effect.store.BeginV4CommitCount() != 1 {
		t.Fatalf("V4 Begin did not recover exact begun record: %#v commits=%d", begun.Record, fixture.effect.store.BeginV4CommitCount())
	}
}

func TestOperationDispatchV4HistoricalRecordDoesNotSurviveCurrentReviewDrift(t *testing.T) {
	fixture := newOperationDispatchFixtureV4(t, "dispatch-review-drift")
	issued, err := fixture.gateway.IssueOperationDispatchV4(context.Background(), fixture.issue)
	if err != nil {
		t.Fatal(err)
	}
	fixture.review.mutateAndSeal(t, fixture.effect.now, func(value *ports.OperationReviewCurrentProjectionV4) {
		value.Verdict.Revision++
		value.Verdict.Digest = core.DigestBytes([]byte("replacement-verdict"))
		value.CurrentnessDigest = core.DigestBytes([]byte("replacement-currentness"))
	})
	inspect := ports.InspectOperationDispatchRecordRequestV4{Operation: fixture.issue.Operation, EffectID: fixture.issue.EffectID, PermitID: fixture.issue.PermitID}
	if historical, err := fixture.gateway.InspectOperationDispatchRecordV4(context.Background(), inspect); err != nil || historical.Digest != issued.Record.Digest {
		t.Fatalf("Review drift erased historical Permit record: %#v err=%v", historical, err)
	}
	if _, err := fixture.gateway.InspectCurrentOperationDispatchV4(context.Background(), ports.InspectCurrentOperationDispatchRequestV4{Inspect: inspect, AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: fixture.authorization.RefV4()}); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("historical record was treated as current after Review drift: %v", err)
	}
	if _, err := fixture.gateway.BeginOperationDispatchV4(context.Background(), beginOperationDispatchRequestV4(fixture, issued)); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("Begin accepted stale Review Authorization: %v", err)
	}
	if fixture.effect.store.BeginV4CommitCount() != 0 {
		t.Fatal("Review drift changed the Permit Fact owner")
	}
	persisted, err := fixture.effect.store.InspectOperationDispatchPermitV4(context.Background(), fixture.issue.Operation, fixture.issue.PermitID)
	if err != nil || persisted.State != ports.OperationPermitIssuedV4 {
		t.Fatalf("failed Begin changed historical Permit: %#v err=%v", persisted, err)
	}
}

func TestOperationDispatchV4BeginRereadsEveryCurrentGovernanceDimension(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*operationDispatchFixtureV4)
	}{
		{name: "identity", mutate: func(f *operationDispatchFixtureV4) {
			f.effect.current.mutate(func(v *ports.OperationGovernanceSnapshotV3) {
				v.Identity.Digest = core.DigestBytes([]byte("other-identity"))
			})
		}},
		{name: "binding", mutate: func(f *operationDispatchFixtureV4) {
			f.effect.current.mutate(func(v *ports.OperationGovernanceSnapshotV3) {
				v.Binding.Digest = core.DigestBytes([]byte("other-binding"))
			})
		}},
		{name: "current_scope", mutate: func(f *operationDispatchFixtureV4) {
			f.effect.current.mutate(func(v *ports.OperationGovernanceSnapshotV3) {
				v.CurrentScope.Digest = core.DigestBytes([]byte("other-scope"))
			})
		}},
		{name: "authority", mutate: func(f *operationDispatchFixtureV4) {
			f.effect.current.mutate(func(v *ports.OperationGovernanceSnapshotV3) {
				v.Authority.Digest = core.DigestBytes([]byte("other-authority"))
			})
		}},
		{name: "budget", mutate: func(f *operationDispatchFixtureV4) {
			f.effect.current.mutate(func(v *ports.OperationGovernanceSnapshotV3) {
				v.Budget.Digest = core.DigestBytes([]byte("other-budget"))
			})
		}},
		{name: "policy", mutate: func(f *operationDispatchFixtureV4) {
			f.effect.current.mutate(func(v *ports.OperationGovernanceSnapshotV3) {
				v.Policy.Digest = core.DigestBytes([]byte("other-policy"))
			})
		}},
		{name: "credential_set", mutate: func(f *operationDispatchFixtureV4) {
			f.effect.current.mutate(func(v *ports.OperationGovernanceSnapshotV3) {
				v.Credentials = append(v.Credentials, ports.OperationCredentialCurrentFactV3{})
			})
		}},
		{name: "capability_grant", mutate: func(f *operationDispatchFixtureV4) {
			f.effect.current.mutate(func(v *ports.OperationGovernanceSnapshotV3) {
				v.CapabilityGrantDigest = core.DigestBytes([]byte("other-capability"))
			})
		}},
		{name: "inactive_scope", mutate: func(f *operationDispatchFixtureV4) {
			f.effect.current.mutate(func(v *ports.OperationGovernanceSnapshotV3) { v.Active = false })
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newOperationDispatchFixtureV4(t, "begin-drift-"+testCase.name)
			issued, err := fixture.gateway.IssueOperationDispatchV4(context.Background(), fixture.issue)
			if err != nil {
				t.Fatal(err)
			}
			testCase.mutate(fixture)
			if _, err := fixture.gateway.BeginOperationDispatchV4(context.Background(), beginOperationDispatchRequestV4(fixture, issued)); err == nil {
				t.Fatal("drifted current governance reached raw Begin")
			}
			if fixture.effect.store.BeginV4CommitCount() != 0 {
				t.Fatal("failed current revalidation changed Permit state")
			}
			persisted, err := fixture.effect.store.InspectOperationDispatchPermitV4(context.Background(), fixture.issue.Operation, fixture.issue.PermitID)
			if err != nil || persisted.State != ports.OperationPermitIssuedV4 {
				t.Fatalf("failed Begin did not preserve issued Permit: %#v err=%v", persisted, err)
			}
		})
	}
}

func TestOperationDispatchV4RevokedAuthorizationBlocksBeginButPreservesHistory(t *testing.T) {
	fixture := newOperationDispatchFixtureV4(t, "authorization-revoked")
	issued, err := fixture.gateway.IssueOperationDispatchV4(context.Background(), fixture.issue)
	if err != nil {
		t.Fatal(err)
	}
	revoked := fixture.authorization
	revoked.Revision++
	revoked.State = ports.OperationReviewAuthorizationRevokedV4
	revoked.InvalidationReason = core.ReasonReviewVerdictStale
	revoked.UpdatedUnixNano = fixture.effect.now.UnixNano()
	revoked, err = ports.SealOperationReviewAuthorizationFactV4(revoked)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.authorizationStore.CompareAndSwapOperationReviewAuthorizationV4(context.Background(), ports.OperationReviewAuthorizationCASRequestV4{ExpectedRevision: fixture.authorization.Revision, Next: revoked}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.gateway.BeginOperationDispatchV4(context.Background(), beginOperationDispatchRequestV4(fixture, issued)); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("revoked Authorization reached Begin: %v", err)
	}
	if fixture.effect.store.BeginV4CommitCount() != 0 {
		t.Fatal("revoked Authorization changed historical Permit")
	}
}

func TestOperationDispatchV4AuthorizationAndGovernanceDriftCauseZeroIssueWrites(t *testing.T) {
	t.Run("authorization ref", func(t *testing.T) {
		fixture := newOperationDispatchFixtureV4(t, "dispatch-auth-ref")
		fixture.issue.ReviewAuthorization.Digest = core.DigestBytes([]byte("forged-authorization"))
		if _, err := fixture.gateway.IssueOperationDispatchV4(context.Background(), fixture.issue); err == nil {
			t.Fatal("forged Authorization ref was accepted")
		}
		assertOperationDispatchV4NoIssueWrite(t, fixture)
	})
	t.Run("review currentness", func(t *testing.T) {
		fixture := newOperationDispatchFixtureV4(t, "dispatch-review-currentness")
		fixture.review.mutateAndSeal(t, fixture.effect.now, func(value *ports.OperationReviewCurrentProjectionV4) {
			value.CurrentnessDigest = core.DigestBytes([]byte("drifted-currentness"))
		})
		if _, err := fixture.gateway.IssueOperationDispatchV4(context.Background(), fixture.issue); err == nil {
			t.Fatal("drifted Review currentness was accepted")
		}
		assertOperationDispatchV4NoIssueWrite(t, fixture)
	})
	t.Run("authority", func(t *testing.T) {
		fixture := newOperationDispatchFixtureV4(t, "dispatch-authority")
		fixture.effect.current.mutate(func(value *ports.OperationGovernanceSnapshotV3) {
			value.Authority.Digest = core.DigestBytes([]byte("drifted-authority"))
		})
		if _, err := fixture.gateway.IssueOperationDispatchV4(context.Background(), fixture.issue); err == nil {
			t.Fatal("drifted governance was accepted")
		}
		assertOperationDispatchV4NoIssueWrite(t, fixture)
	})
}

func TestOperationDispatchV4RawOwnerIdempotencyAndChangedContentConflict(t *testing.T) {
	fixture := newOperationDispatchFixtureV4(t, "dispatch-owner-idempotency")
	issued, err := fixture.gateway.IssueOperationDispatchV4(context.Background(), fixture.issue)
	if err != nil {
		t.Fatal(err)
	}
	raw := control.IssueOperationPermitRequestV4{
		Operation: fixture.issue.Operation, EffectID: fixture.issue.EffectID, ExpectedEffectRevision: fixture.issue.ExpectedEffectRevision,
		Permit: issued.Record.Permit, Fence: issued.Record.Fence, ReviewAuthorization: fixture.authorization,
	}
	replayed, err := fixture.effect.store.IssueOperationDispatchPermitV4(context.Background(), raw)
	if err != nil || replayed.Permit.Digest != issued.Record.Digest || fixture.effect.store.IssueV4CommitCount() != 1 {
		t.Fatalf("raw Owner exact replay was not idempotent: %#v err=%v", replayed, err)
	}
	changed := raw
	changed.Permit.LegacyPermit.AttemptID = "changed-attempt"
	changed.Permit, err = ports.SealOperationDispatchPermitV4(changed.Permit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.effect.store.IssueOperationDispatchPermitV4(context.Background(), changed); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same V4 Permit ID changed content did not conflict: %v", err)
	}
	wrongRevision := raw
	wrongRevision.ExpectedEffectRevision++
	if _, err := fixture.effect.store.IssueOperationDispatchPermitV4(context.Background(), wrongRevision); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("idempotent replay accepted another Effect watermark: %v", err)
	}
	wrongEffect := raw
	wrongEffect.EffectID = "another-effect"
	if _, err := fixture.effect.store.IssueOperationDispatchPermitV4(context.Background(), wrongEffect); !core.HasReason(err, core.ReasonDispatchPermitInvalid) {
		t.Fatalf("idempotent replay accepted another Effect ID: %v", err)
	}
	changedGrant := raw
	changedGrant.Permit.LegacyPermit.CredentialGrantDigest = core.DigestBytes([]byte("other-credential-grant"))
	changedGrant.Permit, err = ports.SealOperationDispatchPermitV4(changedGrant.Permit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.effect.store.IssueOperationDispatchPermitV4(context.Background(), changedGrant); !core.HasReason(err, core.ReasonEffectAuthorizationMissing) {
		t.Fatalf("Permit credential grant drifted from Authorization: %v", err)
	}

	nonCurrent := issued
	nonCurrent.Record.State = ports.OperationPermitRevokedV4
	nonCurrent.Record, err = ports.SealOperationDispatchRecordV4(nonCurrent.Record)
	if err != nil {
		t.Fatal(err)
	}
	if err := nonCurrent.Validate(); err == nil {
		t.Fatal("revoked historical Permit was represented as current execution authorization")
	}
}

func TestOperationDispatchV4RawOwnerConcurrentIssueLinearizesOnce(t *testing.T) {
	source := newOperationDispatchFixtureV4(t, "concurrent-owner")
	issued, err := source.gateway.IssueOperationDispatchV4(context.Background(), source.issue)
	if err != nil {
		t.Fatal(err)
	}
	target := newOperationDispatchFixtureV4(t, "concurrent-owner")
	raw := control.IssueOperationPermitRequestV4{
		Operation: target.issue.Operation, EffectID: target.issue.EffectID, ExpectedEffectRevision: target.issue.ExpectedEffectRevision,
		Permit: issued.Record.Permit, Fence: issued.Record.Fence, ReviewAuthorization: target.authorization,
	}
	const workers = 64
	var wait sync.WaitGroup
	errors := make(chan error, workers)
	digests := make(chan core.Digest, workers)
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := target.effect.store.IssueOperationDispatchPermitV4(context.Background(), raw)
			if err == nil {
				digests <- result.Permit.Digest
			}
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	close(digests)
	for err := range errors {
		if err != nil {
			t.Fatalf("exact concurrent Issue was not idempotent: %v", err)
		}
	}
	for digest := range digests {
		if digest != issued.Record.Digest {
			t.Fatalf("concurrent Issue returned another record: %s", digest)
		}
	}
	if target.effect.store.IssueV4CommitCount() != 1 {
		t.Fatalf("concurrent Issue committed %d times", target.effect.store.IssueV4CommitCount())
	}
}

func TestOperationDispatchV4PermitIDCannotCrossFromV3(t *testing.T) {
	source := newOperationDispatchFixtureV4(t, "v3-first")
	issued, err := source.gateway.IssueOperationDispatchV4(context.Background(), source.issue)
	if err != nil {
		t.Fatal(err)
	}
	target := newOperationDispatchFixtureV4(t, "v3-first")
	if _, err := target.effect.store.IssueOperationDispatchPermitV3(context.Background(), control.IssueOperationPermitRequestV3{
		Operation: target.issue.Operation, EffectID: target.issue.EffectID, ExpectedEffectRevision: target.issue.ExpectedEffectRevision,
		Permit: issued.Record.Permit.LegacyPermit, Fence: issued.Record.Fence,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := target.effect.store.IssueOperationDispatchPermitV4(context.Background(), control.IssueOperationPermitRequestV4{
		Operation: target.issue.Operation, EffectID: target.issue.EffectID, ExpectedEffectRevision: target.issue.ExpectedEffectRevision,
		Permit: issued.Record.Permit, Fence: issued.Record.Fence, ReviewAuthorization: target.authorization,
	}); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("V3 Permit ID was silently upgraded to V4: %v", err)
	}
	if target.effect.store.IssueV4CommitCount() != 0 {
		t.Fatal("cross-version conflict created a V4 Permit")
	}
}

func TestOperationDispatchV4ConformanceNeverClaimsExecutionOrProduction(t *testing.T) {
	fixture := newOperationDispatchFixtureV4(t, "dispatch-conformance")
	v4ThenV3 := newOperationDispatchFixtureV4(t, "conformance-v4-then-v3")
	v4ThenV3Second := addSecondOperationDispatchEffectV4(t, v4ThenV3, "conformance-v4-then-v3-second")
	v3ThenV4 := newOperationDispatchFixtureV4(t, "conformance-v3-then-v4")
	v3ThenV4Second := addSecondOperationDispatchEffectV4(t, v3ThenV4, "conformance-v3-then-v4-second")
	report, err := conformance.CheckOperationDispatchGovernanceV4(context.Background(), conformance.OperationDispatchGovernanceCaseV4{
		Gateway: fixture.gateway,
		Issue:   fixture.issue,
		V4ThenV3Isolation: conformance.OperationDispatchCrossVersionCaseV4{
			V3: v4ThenV3.effect.gateway, V4: v4ThenV3.gateway, Admissions: v4ThenV3Second.admissions,
			V3Issue: v4ThenV3Second.v3Issue, V4Issue: v4ThenV3.issue,
		},
		V3ThenV4Isolation: conformance.OperationDispatchCrossVersionCaseV4{
			V3: v3ThenV4.effect.gateway, V4: v3ThenV4.gateway, Admissions: v3ThenV4Second.admissions,
			V3Issue: operationDispatchIssueV3ForV4(v3ThenV4), V4Issue: v3ThenV4Second.v4Issue,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.AtomicIssueObserved || !report.HistoricalInspectObserved || !report.CurrentInspectObserved || !report.BeginObserved || !report.V4ThenV3ConflictObserved || !report.V3ThenV4ConflictObserved || !report.CrossVersionEffectAtomic || report.CrossVersionSecondWrite || report.HistoricalRecordExecutes || report.BeginIsFinalExecution || report.V3MasqueradesAsV4 || report.ProductionClaimEligible {
		t.Fatalf("V4 conformance exceeded its authority: %#v", report)
	}
}

func TestOperationDispatchV4ConformanceRejectsDisconnectedCrossVersionStores(t *testing.T) {
	fixture := newOperationDispatchFixtureV4(t, "conformance-disconnected-main")
	v4Owner := newOperationDispatchFixtureV4(t, "conformance-disconnected-cross")
	_ = addSecondOperationDispatchEffectV4(t, v4Owner, "conformance-disconnected-cross-second")
	v3Owner := newOperationDispatchFixtureV4(t, "conformance-disconnected-cross")
	v3OwnerSecond := addSecondOperationDispatchEffectV4(t, v3Owner, "conformance-disconnected-cross-second")
	validReverse := newOperationDispatchFixtureV4(t, "conformance-disconnected-reverse")
	validReverseSecond := addSecondOperationDispatchEffectV4(t, validReverse, "conformance-disconnected-reverse-second")
	_, err := conformance.CheckOperationDispatchGovernanceV4(context.Background(), conformance.OperationDispatchGovernanceCaseV4{
		Gateway: fixture.gateway,
		Issue:   fixture.issue,
		V4ThenV3Isolation: conformance.OperationDispatchCrossVersionCaseV4{
			V3: v3Owner.effect.gateway, V4: v4Owner.gateway, Admissions: v3OwnerSecond.admissions,
			V3Issue: v3OwnerSecond.v3Issue, V4Issue: v4Owner.issue,
		},
		V3ThenV4Isolation: conformance.OperationDispatchCrossVersionCaseV4{
			V3: validReverse.effect.gateway, V4: validReverse.gateway, Admissions: validReverseSecond.admissions,
			V3Issue: operationDispatchIssueV3ForV4(validReverse), V4Issue: validReverseSecond.v4Issue,
		},
	})
	if !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("conformance accepted independent stores that both wrote the same Permit ID: %v", err)
	}
}

type operationDispatchFixtureV4 struct {
	effect             *operationFixtureV3
	review             *operationReviewReaderV4
	authorizationStore *fakes.OperationReviewAuthorizationStoreV4
	authorization      ports.OperationReviewAuthorizationFactV4
	gateway            control.OperationGovernanceGatewayV4
	issue              ports.IssueGovernedOperationDispatchRequestV4
}

type secondOperationDispatchEffectV4 struct {
	admissions ports.OperationEffectAdmissionPortV3
	v3Issue    ports.IssueGovernedOperationDispatchRequestV3
	v4Issue    ports.IssueGovernedOperationDispatchRequestV4
}

type operationReviewRouterV4 struct {
	values map[core.EffectIntentID]ports.OperationReviewCurrentProjectionV4
}

func (r operationReviewRouterV4) InspectOperationReviewCurrentV4(_ context.Context, intent ports.OperationEffectIntentV3) (ports.OperationReviewCurrentProjectionV4, error) {
	value, exists := r.values[intent.ID]
	if !exists {
		return ports.OperationReviewCurrentProjectionV4{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "operation Review projection not found")
	}
	return value, nil
}

func newOperationDispatchFixtureV4(t *testing.T, suffix string) *operationDispatchFixtureV4 {
	t.Helper()
	effect, reviewGateway, authorizationStore, review := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
	effect.store.BindOperationReviewAuthorizationFactsV4(authorizationStore)
	authorization, err := reviewGateway.CreateOperationReviewAuthorizationV4(context.Background(), ports.CreateOperationReviewAuthorizationRequestV4{
		AuthorizationID: "authorization-" + suffix, Operation: effect.intent.Operation, EffectID: effect.intent.ID,
		ExpectedEffectRevision: effect.accepted.Revision, RequestedTTL: 20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	admissionGateway := control.OperationEffectAdmissionGatewayV3{Effects: effect.store}
	admission, err := admissionGateway.InspectAcceptedOperationEffectV3(context.Background(), effect.intent.Operation, effect.intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	gateway := control.OperationGovernanceGatewayV4{
		Effects: effect.store, Admissions: admissionGateway, Reviews: reviewGateway,
		Current: effect.current, Clock: func() time.Time { return effect.now },
	}
	issue := ports.IssueGovernedOperationDispatchRequestV4{
		Operation: effect.intent.Operation, EffectID: effect.intent.ID, ExpectedEffectRevision: effect.accepted.Revision,
		Admission: admission, ReviewAuthorization: authorization.RefV4(), PermitID: "permit-" + suffix,
		AttemptID: "attempt-" + suffix, PermitTTL: 10 * time.Second,
	}
	return &operationDispatchFixtureV4{effect: effect, review: review, authorizationStore: authorizationStore, authorization: authorization, gateway: gateway, issue: issue}
}

func addSecondOperationDispatchEffectV4(t *testing.T, fixture *operationDispatchFixtureV4, suffix string) secondOperationDispatchEffectV4 {
	t.Helper()
	intent := fixture.effect.intent
	intent.ID = core.EffectIntentID("operation-effect-" + suffix)
	intent.Idempotency.Key = "operation-key-" + suffix
	proposed, err := control.NewProposedOperationEffectFactV3(intent, fixture.effect.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.effect.store.CreateOperationEffectV3(context.Background(), proposed); err != nil {
		t.Fatal(err)
	}
	accepted := proposed
	accepted.State = control.OperationEffectAcceptedV3
	accepted.Revision++
	accepted.UpdatedUnixNano = fixture.effect.now.UnixNano()
	if _, err := fixture.effect.store.CompareAndSwapOperationEffectV3(context.Background(), intent.Operation, control.OperationEffectCASRequestV3{ExpectedRevision: proposed.Revision, Next: accepted}); err != nil {
		t.Fatal(err)
	}
	admissions := control.OperationEffectAdmissionGatewayV3{Effects: fixture.effect.store}
	admission, err := admissions.InspectAcceptedOperationEffectV3(context.Background(), intent.Operation, intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	fixture.review.mu.Lock()
	originalReview := fixture.review.value
	fixture.review.mu.Unlock()
	secondReview := originalReview
	secondReview.IntentID = intent.ID
	secondReview.IntentDigest = intentDigest
	secondReview.CurrentnessDigest = core.DigestBytes([]byte("review-currentness-" + suffix))
	secondReview, err = ports.SealOperationReviewCurrentProjectionV4(secondReview, fixture.effect.now)
	if err != nil {
		t.Fatal(err)
	}
	reviews := operationReviewRouterV4{values: map[core.EffectIntentID]ports.OperationReviewCurrentProjectionV4{
		fixture.effect.intent.ID: originalReview,
		intent.ID:                secondReview,
	}}
	reviewGateway := kernel.OperationReviewAuthorizationGatewayV4{
		Facts: fixture.authorizationStore, Effects: fixture.effect.store, Governance: fixture.effect.current,
		Reviews: reviews, Clock: func() time.Time { return fixture.effect.now },
	}
	fixture.gateway.Reviews = reviewGateway
	authorization, err := reviewGateway.CreateOperationReviewAuthorizationV4(context.Background(), ports.CreateOperationReviewAuthorizationRequestV4{
		AuthorizationID: "authorization-" + suffix, Operation: intent.Operation, EffectID: intent.ID,
		ExpectedEffectRevision: accepted.Revision, RequestedTTL: 20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	v4Issue := fixture.issue
	v4Issue.EffectID = intent.ID
	v4Issue.ExpectedEffectRevision = accepted.Revision
	v4Issue.Admission = admission
	v4Issue.ReviewAuthorization = authorization.RefV4()
	v4Issue.AttemptID = "attempt-" + suffix
	v3Issue := ports.IssueGovernedOperationDispatchRequestV3{
		Operation: intent.Operation, EffectID: intent.ID, ExpectedEffectRevision: accepted.Revision,
		PermitID: fixture.issue.PermitID, AttemptID: "legacy-attempt-" + suffix, PermitTTL: fixture.issue.PermitTTL,
	}
	return secondOperationDispatchEffectV4{admissions: admissions, v3Issue: v3Issue, v4Issue: v4Issue}
}

func beginOperationDispatchRequestV4(fixture *operationDispatchFixtureV4, issued ports.CurrentOperationDispatchAuthorizationV4) ports.BeginGovernedOperationDispatchRequestV4 {
	return ports.BeginGovernedOperationDispatchRequestV4{
		Operation: fixture.issue.Operation, EffectID: fixture.issue.EffectID, ExpectedEffectRevision: issued.Record.EffectFactRevision,
		PermitID: fixture.issue.PermitID, ExpectedPermitFactRevision: issued.Record.Revision,
		AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: fixture.authorization.RefV4(),
	}
}

func operationDispatchIssueV3ForV4(fixture *operationDispatchFixtureV4) ports.IssueGovernedOperationDispatchRequestV3 {
	return ports.IssueGovernedOperationDispatchRequestV3{
		Operation: fixture.issue.Operation, EffectID: fixture.issue.EffectID,
		ExpectedEffectRevision: fixture.issue.ExpectedEffectRevision,
		PermitID:               fixture.issue.PermitID, AttemptID: "legacy-" + fixture.issue.AttemptID,
		PermitTTL: fixture.issue.PermitTTL,
	}
}

func assertOperationDispatchV4NoIssueWrite(t *testing.T, fixture *operationDispatchFixtureV4) {
	t.Helper()
	if fixture.effect.store.IssueV4CommitCount() != 0 {
		t.Fatal("failed V4 Issue changed the Permit Fact owner")
	}
	effect, err := fixture.effect.store.InspectOperationEffectV3(context.Background(), fixture.issue.Operation, fixture.issue.EffectID)
	if err != nil || effect.State != control.OperationEffectAcceptedV3 || effect.Revision != fixture.issue.ExpectedEffectRevision {
		t.Fatalf("failed V4 Issue changed accepted Effect: %#v err=%v", effect, err)
	}
}

var _ ports.OperationReviewAuthorizationGovernancePortV4 = kernel.OperationReviewAuthorizationGatewayV4{}
var _ ports.OperationGovernancePortV4 = control.OperationGovernanceGatewayV4{}
var _ control.OperationEffectDispatchFactPortV4 = (*fakes.OperationEffectStoreV3)(nil)
