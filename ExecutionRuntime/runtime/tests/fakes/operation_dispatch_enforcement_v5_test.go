package fakes_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationDispatchEnforcementV5PrepareExecuteLostReplyAndConformance(t *testing.T) {
	fixture := newOperationEnforcementFixtureV5(t, "flow")
	fixture.effect.store.LoseNextEnforcementV5Reply()
	prepared, err := fixture.enforcement.EnforceCurrentOperationDispatchV5(context.Background(), fixture.prepare)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Phase.Phase != ports.OperationDispatchEnforcementPrepareV4 || fixture.effect.store.EnforcementV5CommitCount() != 1 {
		t.Fatalf("V5 prepare did not recover one exact receipt: %#v commits=%d", prepared, fixture.effect.store.EnforcementV5CommitCount())
	}
	historicalOnly := control.OperationDispatchEnforcementGatewayV5{Facts: fixture.effect.store}
	historical, err := historicalOnly.InspectOperationDispatchEnforcementV5(context.Background(), ports.InspectOperationDispatchEnforcementRequestV5{Operation: fixture.prepare.Operation, EffectID: fixture.prepare.EffectID, PermitID: fixture.prepare.PermitID, Phase: ports.OperationDispatchEnforcementPrepareV4})
	if err != nil || historical.Digest != prepared.Journal.Digest {
		t.Fatalf("V5 historical Inspect incorrectly required current dependencies: %#v err=%v", historical, err)
	}

	execute := fixture.prepare
	execute.Phase = ports.OperationDispatchEnforcementExecuteV4
	execute.ExpectedJournalRevision = 1
	execute.Prepare = &prepared.Phase
	execute.PreparedAttempt = preparedAttemptForEnforcementV5(t, fixture, prepared)
	fixture.effect.store.LoseNextEnforcementV5Reply()
	executed, err := fixture.enforcement.EnforceCurrentOperationDispatchV5(context.Background(), execute)
	if err != nil {
		t.Fatal(err)
	}
	if executed.Journal.Revision != 2 || executed.Phase.Phase != ports.OperationDispatchEnforcementExecuteV4 || fixture.effect.store.EnforcementV5CommitCount() != 2 {
		t.Fatalf("V5 execute did not append one exact second slot: %#v commits=%d", executed, fixture.effect.store.EnforcementV5CommitCount())
	}

	conformanceFixture := newOperationEnforcementFixtureV5(t, "conformance")
	preparedAttempt := preparedAttemptForEnforcementV5FromRequest(t, conformanceFixture)
	report, err := conformance.CheckOperationDispatchEnforcementV5(context.Background(), conformance.OperationDispatchEnforcementCaseV5{Gateway: conformanceFixture.enforcement, Prepare: conformanceFixture.prepare, PreparedAttempt: *preparedAttempt})
	if err != nil {
		t.Fatal(err)
	}
	if report.ProviderCalled || report.ProductionClaimEligible {
		t.Fatalf("V5 enforcement conformance overclaimed execution: %#v", report)
	}
}

func TestOperationDispatchEnforcementV5CurrentDriftFailsBeforeJournalWrite(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*operationEnforcementFixtureV5)
	}{
		{name: "review", mutate: func(f *operationEnforcementFixtureV5) {
			f.review.mutateAndSeal(t, f.effect.now, func(value *ports.OperationReviewCurrentProjectionV5) {
				value.Quorum.CurrentnessDigest = core.DigestBytes([]byte("drifted-enforcement-review-v5"))
			})
		}},
		{name: "authority", mutate: func(f *operationEnforcementFixtureV5) {
			f.effect.current.mutate(func(value *ports.OperationGovernanceSnapshotV3) {
				value.Authority.Digest = core.DigestBytes([]byte("drifted-enforcement-authority-v5"))
			})
		}},
		{name: "sandbox", mutate: func(f *operationEnforcementFixtureV5) {
			f.sandbox.mutate(t, func(value *ports.OperationDispatchSandboxCurrentProjectionV4) {
				value.Attempt.Revision++
			})
		}},
		{name: "sandbox_ttl", mutate: func(f *operationEnforcementFixtureV5) {
			f.sandbox.mutate(t, func(value *ports.OperationDispatchSandboxCurrentProjectionV4) {
				value.Attempt.ExpiresUnixNano = f.effect.now.UnixNano()
				value.ExpiresUnixNano = f.effect.now.UnixNano()
			})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newOperationEnforcementFixtureV5(t, "drift-"+tc.name)
			tc.mutate(fixture)
			if _, err := fixture.enforcement.EnforceCurrentOperationDispatchV5(context.Background(), fixture.prepare); err == nil {
				t.Fatal("drifted V5 current facts produced an enforcement receipt")
			}
			if fixture.effect.store.EnforcementV5CommitCount() != 0 {
				t.Fatal("failed V5 actual-point validation changed the journal")
			}
		})
	}
}

func TestOperationDispatchEnforcementV5ClockRollbackAndChangedAttemptFailClosed(t *testing.T) {
	fixture := newOperationEnforcementFixtureV5(t, "clock")
	calls := 0
	fixture.enforcement.Clock = func() time.Time {
		calls++
		if calls == 1 {
			return fixture.effect.now.Add(time.Second)
		}
		return fixture.effect.now
	}
	if _, err := fixture.enforcement.EnforceCurrentOperationDispatchV5(context.Background(), fixture.prepare); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("V5 enforcement accepted a clock rollback: %v", err)
	}
	if fixture.effect.store.EnforcementV5CommitCount() != 0 {
		t.Fatal("clock rollback changed V5 journal")
	}

	fixture = newOperationEnforcementFixtureV5(t, "changed")
	prepared, err := fixture.enforcement.EnforceCurrentOperationDispatchV5(context.Background(), fixture.prepare)
	if err != nil {
		t.Fatal(err)
	}
	execute := fixture.prepare
	execute.Phase = ports.OperationDispatchEnforcementExecuteV4
	execute.ExpectedJournalRevision = 1
	execute.Prepare = &prepared.Phase
	execute.PreparedAttempt = preparedAttemptForEnforcementV5(t, fixture, prepared)
	execute.PreparedAttempt.AttemptID = "changed-attempt"
	if _, err := fixture.enforcement.EnforceCurrentOperationDispatchV5(context.Background(), execute); err == nil {
		t.Fatal("changed prepared attempt reached V5 journal")
	}
	if fixture.effect.store.EnforcementV5CommitCount() != 1 {
		t.Fatal("changed execute appended a second journal slot")
	}
}

func TestOperationDispatchEnforcementV5LostReplyRecoveryIgnoresCanceledMutationContext(t *testing.T) {
	fixture := newOperationEnforcementFixtureV5(t, "canceled-recovery")
	ctx, cancel := context.WithCancel(context.Background())
	fixture.enforcement.Facts = cancelingEnforcementFactPortV5{OperationDispatchEnforcementFactPortV5: fixture.effect.store, cancel: cancel}
	prepared, err := fixture.enforcement.EnforceCurrentOperationDispatchV5(ctx, fixture.prepare)
	if err != nil || prepared.Journal.Revision != 1 {
		t.Fatalf("V5 enforcement did not recover through cancellation-independent exact Inspect: %#v err=%v", prepared, err)
	}
	if fixture.effect.store.EnforcementV5CommitCount() != 1 {
		t.Fatalf("canceled enforcement lost-reply recovery repeated mutation: %d", fixture.effect.store.EnforcementV5CommitCount())
	}
}

type cancelingEnforcementFactPortV5 struct {
	control.OperationDispatchEnforcementFactPortV5
	cancel context.CancelFunc
}

func (p cancelingEnforcementFactPortV5) AppendOperationDispatchEnforcementV5(ctx context.Context, request control.AppendOperationDispatchEnforcementRequestV5) (ports.OperationDispatchEnforcementJournalV5, error) {
	journal, err := p.OperationDispatchEnforcementFactPortV5.AppendOperationDispatchEnforcementV5(ctx, request)
	if err == nil {
		p.cancel()
		return ports.OperationDispatchEnforcementJournalV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected canceled V5 enforcement reply loss")
	}
	return journal, err
}

type operationEnforcementFixtureV5 struct {
	effect        *operationFixtureV3
	review        *operationReviewReaderV5
	authorization ports.OperationReviewAuthorizationFactV5
	dispatch      control.OperationGovernanceGatewayV5
	sandbox       *operationSandboxReaderV4
	enforcement   control.OperationDispatchEnforcementGatewayV5
	prepare       ports.EnforceCurrentOperationDispatchRequestV5
}

func newOperationEnforcementFixtureV5(t *testing.T, suffix string) *operationEnforcementFixtureV5 {
	t.Helper()
	scope := core.ExecutionScope{
		Identity:     core.AgentIdentityRef{TenantID: "tenant-enforcement-v5", ID: "identity-enforcement-v5", Epoch: 1},
		Lineage:      core.LineageRef{ID: "lineage-enforcement-v5", PlanDigest: core.DigestBytes([]byte("lineage-enforcement-v5"))},
		Instance:     core.InstanceRef{ID: "instance-enforcement-v5", Epoch: 1},
		SandboxLease: &core.SandboxLeaseRef{ID: "lease-enforcement-v5", Epoch: 1}, AuthorityEpoch: 1,
	}
	effect := newOperationFixtureForRunAndKindV3(t, "run-enforcement-v5", &scope, "")
	reviewGateway, reviewStore, review := operationReviewAuthorizationForEffectV5(t, effect)
	effect.store.BindOperationReviewAuthorizationV5(reviewGateway)
	authorization, err := reviewGateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(effect, "authorization-"+suffix, ports.OperationReviewBasisAcceptedQuorumV5))
	if err != nil {
		t.Fatal(err)
	}
	_ = reviewStore
	admissions := control.OperationEffectAdmissionGatewayV3{Effects: effect.store}
	admission, err := admissions.InspectAcceptedOperationEffectV3(context.Background(), effect.intent.Operation, effect.intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	dispatch := control.OperationGovernanceGatewayV5{Effects: effect.store, Admissions: admissions, Reviews: reviewGateway, Current: effect.current, Clock: func() time.Time { return effect.now }}
	issue := ports.IssueGovernedOperationDispatchRequestV5{Operation: effect.intent.Operation, EffectID: effect.intent.ID, ExpectedEffectRevision: effect.accepted.Revision, Admission: admission, ReviewAuthorization: authorization.RefV5(), AuthorizationBasis: ports.OperationReviewBasisAcceptedQuorumV5, PermitID: "permit-" + suffix, AttemptID: "attempt-" + suffix, PermitTTL: 10 * time.Second}
	issued, err := dispatch.IssueOperationDispatchV5(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	begun, err := dispatch.BeginOperationDispatchV5(context.Background(), ports.BeginGovernedOperationDispatchRequestV5{Operation: issue.Operation, EffectID: issue.EffectID, ExpectedEffectRevision: issued.Record.EffectFactRevision, PermitID: issue.PermitID, ExpectedPermitFactRevision: issued.Record.Revision, AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: authorization.RefV5(), AuthorizationBasis: issue.AuthorizationBasis})
	if err != nil {
		t.Fatal(err)
	}
	sandboxProjection := sandboxProjectionForEnforcementV5(t, effect, begun)
	sandbox := &operationSandboxReaderV4{value: sandboxProjection}
	enforcement := control.OperationDispatchEnforcementGatewayV5{Dispatch: dispatch, Sandbox: sandbox, Facts: effect.store, Clock: func() time.Time { return effect.now }}
	prepare := ports.EnforceCurrentOperationDispatchRequestV5{
		Operation: effect.intent.Operation, EffectID: effect.intent.ID, PermitID: issue.PermitID,
		ExpectedPermitFactRevision: begun.Record.Revision, PermitDigest: begun.Record.PermitDigest,
		AdmissionDigest: begun.Record.Permit.Admission.Digest, ReviewAuthorization: authorization.RefV5(), AuthorizationBasis: issue.AuthorizationBasis,
		AttemptID: issue.AttemptID, Phase: ports.OperationDispatchEnforcementPrepareV4,
		SandboxAttempt: sandboxProjection.Attempt, SandboxReservation: sandboxProjection.Reservation,
		SandboxProjectionDigest: sandboxProjection.ProjectionDigest, Verifier: begun.Record.Permit.EnforcementPoint,
	}
	return &operationEnforcementFixtureV5{effect: effect, review: review, authorization: authorization, dispatch: dispatch, sandbox: sandbox, enforcement: enforcement, prepare: prepare}
}

func operationReviewAuthorizationForEffectV5(t *testing.T, fixture *operationFixtureV3) (kernel.OperationReviewAuthorizationGatewayV5, *fakes.OperationReviewAuthorizationStoreV5, *operationReviewReaderV5) {
	t.Helper()
	now := fixture.now
	expires := now.Add(30 * time.Second).UnixNano()
	intentDigest, err := fixture.intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	ref := func(id string, digest core.Digest) ports.OperationGovernanceFactRefV3 {
		return ports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: digest, ExpiresUnixNano: expires}
	}
	tenant := fixture.intent.Operation.ExecutionScope.Identity.TenantID
	q, err := ports.SealOperationReviewQuorumCurrentProjectionV5(ports.OperationReviewQuorumCurrentProjectionV5{
		Operation: fixture.intent.Operation, IntentID: fixture.intent.ID, IntentRevision: fixture.intent.Revision, IntentDigest: intentDigest,
		PayloadSchema: fixture.intent.Payload.Schema, PayloadDigest: fixture.intent.Payload.ContentDigest, PayloadRevision: fixture.intent.PayloadRevision,
		Target:         ports.OperationReviewTargetRefV4{Ref: fixture.intent.Target, Revision: fixture.intent.Review.CandidateRevision, Digest: fixture.intent.Review.CandidateDigest},
		Case:           ports.OperationReviewCaseRefV5{TenantID: tenant, ID: fixture.intent.Review.CaseRef, Revision: 1, Digest: core.DigestBytes([]byte("case-enforcement-v5")), ExpiresUnixNano: expires},
		Panel:          ports.OperationReviewPanelRefV5{TenantID: tenant, ID: "panel-enforcement-v5", Revision: 1, Digest: core.DigestBytes([]byte("panel-enforcement-v5")), ExpiresUnixNano: expires},
		QuorumDecision: ports.OperationReviewQuorumDecisionRefV5{TenantID: tenant, ID: "quorum-enforcement-v5", Revision: 1, Digest: core.DigestBytes([]byte("quorum-enforcement-v5")), ExpiresUnixNano: expires},
		Verdict:        ports.OperationReviewVerdictRefV5{TenantID: tenant, ID: "verdict-enforcement-v5", Revision: 1, Digest: core.DigestBytes([]byte("verdict-enforcement-v5")), ExpiresUnixNano: expires},
		QuorumPolicy:   ref("quorum-policy-enforcement-v5", fixture.intent.Review.PolicyDigest), ReviewerSetDigest: core.DigestBytes([]byte("reviewer-set-enforcement-v5")),
		AcceptCount: 1, Threshold: 1, SatisfiedRoleCounts: []ports.OperationReviewRoleCountV5{{Role: "security", Count: 1, Required: 1}},
		ReviewerAuthorityRefs: []ports.OperationGovernanceFactRefV3{ref("reviewer-authority-enforcement-v5", core.DigestBytes([]byte("reviewer-authority-enforcement-v5")))},
		BindingRefs:           []ports.OperationGovernanceFactRefV3{ref("review-binding-enforcement-v5", core.DigestBytes([]byte("review-binding-enforcement-v5")))},
		ScopeRef:              fixture.current.snapshot.CurrentScope,
		DecisionEvidence:      []ports.EvidenceRecordRefV2{{LedgerScopeDigest: core.DigestBytes([]byte("review-ledger-enforcement-v5")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("review-evidence-enforcement-v5"))}},
		Basis:                 ports.OperationReviewBasisAcceptedQuorumV5, Current: true, CurrentnessDigest: core.DigestBytes([]byte("review-currentness-enforcement-v5")), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ports.SealOperationReviewCurrentProjectionV5(ports.OperationReviewCurrentProjectionV5{Basis: ports.OperationReviewBasisAcceptedQuorumV5, Quorum: &q}, now)
	if err != nil {
		t.Fatal(err)
	}
	reader := &operationReviewReaderV5{value: projection}
	store := fakes.NewOperationReviewAuthorizationStoreV5(func() time.Time { return now })
	gateway := kernel.OperationReviewAuthorizationGatewayV5{Facts: store, Effects: fixture.store, Governance: fixture.current, Reviews: reader, Clock: func() time.Time { return now }}
	return gateway, store, reader
}

func sandboxProjectionForEnforcementV5(t *testing.T, fixture *operationFixtureV3, begun ports.CurrentOperationDispatchAuthorizationV5) ports.OperationDispatchSandboxCurrentProjectionV4 {
	t.Helper()
	expires := fixture.now.Add(8 * time.Second).UnixNano()
	ref := func(id string) ports.OperationDispatchSandboxFactRefV4 {
		return ports.OperationDispatchSandboxFactRefV4{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires}
	}
	lease := *fixture.intent.Operation.ExecutionScope.SandboxLease
	projection, err := ports.SealOperationDispatchSandboxCurrentProjectionV4(ports.OperationDispatchSandboxCurrentProjectionV4{
		Operation: fixture.intent.Operation, OperationDigest: mustOperationDigestForEnforcementV4(t, fixture.intent.Operation), EffectID: fixture.intent.ID,
		IntentRevision: fixture.intent.Revision, IntentDigest: begun.Record.Permit.IntentDigest, AttemptID: begun.Record.Permit.AttemptID,
		Attempt: ref(begun.Record.Permit.AttemptID), Reservation: ref("sandbox-reservation-v5"), SandboxLease: lease,
		RuntimeLease: ports.OperationDispatchRuntimeLeaseBindingV4{Ref: ref("runtime-lease-binding-v5"), Lease: lease, Instance: fixture.intent.Operation.ExecutionScope.Instance, FenceEpoch: 1, ScopeDigest: fixture.intent.Operation.ExecutionScopeDigest, ObservedRevision: 1},
		Generation:   ports.GenerationBindingAssociationRefV1{ID: "generation-association-v5", Revision: 1, Digest: core.DigestBytes([]byte("generation-association-v5"))},
		Placement:    ref("sandbox-placement-v5"), Backend: ref("sandbox-backend-v5"), Slot: ref("sandbox-slot-v5"), ProviderBinding: begun.Record.Permit.EnforcementPoint,
		Current: true, ProjectionRevision: 1, ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func preparedAttemptForEnforcementV5(t *testing.T, fixture *operationEnforcementFixtureV5, prepared ports.CurrentOperationDispatchEnforcementV5) *ports.PreparedProviderAttemptRefV2 {
	t.Helper()
	return preparedAttemptForEnforcementV5WithPermit(t, fixture, prepared.Dispatch.Record.Permit)
}

func preparedAttemptForEnforcementV5FromRequest(t *testing.T, fixture *operationEnforcementFixtureV5) *ports.PreparedProviderAttemptRefV2 {
	t.Helper()
	record, err := fixture.dispatch.InspectOperationDispatchRecordV5(context.Background(), ports.InspectOperationDispatchRecordRequestV5{Operation: fixture.prepare.Operation, EffectID: fixture.prepare.EffectID, PermitID: fixture.prepare.PermitID})
	if err != nil {
		t.Fatal(err)
	}
	return preparedAttemptForEnforcementV5WithPermit(t, fixture, record.Permit)
}

func preparedAttemptForEnforcementV5WithPermit(t *testing.T, fixture *operationEnforcementFixtureV5, permit ports.OperationDispatchPermitV5) *ports.PreparedProviderAttemptRefV2 {
	t.Helper()
	delegation := ports.ExecutionDelegationRefV2{ID: "delegation-enforcement-v5", Revision: 1, Digest: core.DigestBytes([]byte("delegation-enforcement-v5"))}
	id, err := ports.DerivePreparedProviderAttemptIDV2(delegation.ID, permit.ID, permit.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	value, err := ports.SealPreparedProviderAttemptRefV2(ports.PreparedProviderAttemptRefV2{
		ID: id, Revision: 1, DeclaredDelegation: delegation, OperationDigest: mustOperationDigestForEnforcementV4(t, permit.Operation),
		IntentID: permit.IntentID, IntentRevision: permit.IntentRevision, IntentDigest: permit.IntentDigest,
		PermitID: permit.ID, PermitRevision: permit.Revision, PermitDigest: permit.Digest, AttemptID: permit.AttemptID,
		Provider: permit.EnforcementPoint, PayloadSchema: permit.PayloadSchema, PayloadDigest: permit.PayloadDigest, PayloadRevision: permit.PayloadRevision,
		PreparedUnixNano: fixture.effect.now.UnixNano(), ExpiresUnixNano: fixture.effect.now.Add(7 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &value
}

var _ ports.OperationDispatchEnforcementGovernancePortV5 = control.OperationDispatchEnforcementGatewayV5{}
var _ control.OperationDispatchEnforcementFactPortV5 = (*fakes.OperationEffectStoreV3)(nil)
