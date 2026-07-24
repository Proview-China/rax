package application

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreStageAuthorizationGatewayV1LostRepliesRecoverOriginalIDs(t *testing.T) {
	fixture := newRestoreStageAuthorizationFixtureV1(t)
	fixture.admission.loseReply = true
	fixture.reviews.loseReply = true
	fixture.dispatch.loseIssueReply = true
	fixture.dispatch.loseBeginReply = true

	value, err := fixture.gateway.AuthorizeRestoreStageV1(context.Background(), fixture.request)
	if err != nil || value.ValidateFor(fixture.request, fixture.now) != nil || value.Dispatch.Record.State != runtimeports.OperationPermitBegunV4 {
		t.Fatalf("authorization failed: value=%+v err=%v", value, err)
	}
	if fixture.admission.admitCalls != 1 || fixture.reviews.createCalls != 1 || fixture.dispatch.issueCalls != 1 || fixture.dispatch.beginCalls != 1 {
		t.Fatalf("unexpected mutation calls: admit=%d review=%d issue=%d begin=%d", fixture.admission.admitCalls, fixture.reviews.createCalls, fixture.dispatch.issueCalls, fixture.dispatch.beginCalls)
	}
	replay, err := fixture.gateway.InspectRestoreStageAuthorizationV1(context.Background(), restoreStageInspectKeyV1(fixture.request))
	if err != nil || replay.Digest != value.Digest {
		t.Fatalf("authorization Inspect replay failed: value=%+v err=%v", replay, err)
	}
	if fixture.admission.admitCalls != 1 || fixture.reviews.createCalls != 1 || fixture.dispatch.issueCalls != 1 || fixture.dispatch.beginCalls != 1 {
		t.Fatal("Inspect replay mutated governance state")
	}
}

func TestRestoreStageAuthorizationGatewayV1S1S2DriftStopsBeforePermit(t *testing.T) {
	fixture := newRestoreStageAuthorizationFixtureV1(t)
	fixture.inputs.drift = true
	if _, err := fixture.gateway.AuthorizeRestoreStageV1(context.Background(), fixture.request); err == nil {
		t.Fatal("trusted input S1/S2 drift was accepted")
	}
	if fixture.dispatch.issueCalls != 0 || fixture.dispatch.beginCalls != 0 {
		t.Fatal("input drift reached Permit or Begin")
	}

	var typedNil *restoreStageAuthorizationInputReaderV1
	if _, err := NewRestoreStageAuthorizationGatewayV1(RestoreStageAuthorizationGatewayConfigV1{Inputs: typedNil, Admission: fixture.admission, Reviews: fixture.reviews, Dispatch: fixture.dispatch, Clock: func() time.Time { return fixture.now }}); err == nil {
		t.Fatal("typed-nil trusted input Reader was accepted")
	}
}

func TestRestoreStageAuthorizationInputV1RejectsNonCanonicalOrSplicedTypedPayload(t *testing.T) {
	fixture := newRestoreStageAuthorizationFixtureV1(t)
	noncanonical := fixture.input
	noncanonical.Intent.Payload.Inline = append(append([]byte(nil), noncanonical.Intent.Payload.Inline...), ' ')
	noncanonical.Intent.Payload.Length = uint64(len(noncanonical.Intent.Payload.Inline))
	noncanonical.Intent.Payload.ContentDigest = core.DigestBytes(noncanonical.Intent.Payload.Inline)
	if _, err := applicationcontract.SealRestoreStageAuthorizationInputCurrentProjectionV1(noncanonical, fixture.now); err == nil {
		t.Fatal("noncanonical typed Restore payload was accepted")
	}

	spliced := fixture.input
	payload, err := runtimeports.DecodeRestoreStageOperationPayloadV1(spliced.Intent.Payload.Inline)
	if err != nil {
		t.Fatal(err)
	}
	payload.Eligibility.ID += "-other"
	payload.Eligibility.Digest = core.DigestBytes([]byte("other-eligibility"))
	encoded, err := payload.CanonicalBytesV1()
	if err != nil {
		t.Fatal(err)
	}
	spliced.Intent.Payload.Inline = encoded
	spliced.Intent.Payload.Length = uint64(len(encoded))
	spliced.Intent.Payload.ContentDigest = core.DigestBytes(encoded)
	spliced, err = applicationcontract.SealRestoreStageAuthorizationInputCurrentProjectionV1(spliced, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if spliced.ValidateFor(fixture.request, fixture.now) == nil {
		t.Fatal("typed Restore payload with another Eligibility was accepted")
	}
}

type restoreStageAuthorizationFixtureV1 struct {
	now       time.Time
	request   applicationcontract.RestoreStageActionRequestV1
	input     applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1
	inputs    *restoreStageAuthorizationInputReaderV1
	admission *restoreStageAuthorizationAdmissionV1
	reviews   *restoreStageAuthorizationReviewV1
	dispatch  *restoreStageAuthorizationDispatchV1
	gateway   *RestoreStageAuthorizationGatewayV1
}

func newRestoreStageAuthorizationFixtureV1(t *testing.T) *restoreStageAuthorizationFixtureV1 {
	t.Helper()
	base := newRestoreStageActionGatewayFixtureV1(t)
	return newRestoreStageAuthorizationFixtureForRequestV1(t, base.now, base.request)
}

func newRestoreStageAuthorizationFixtureForRequestV1(t *testing.T, now time.Time, request applicationcontract.RestoreStageActionRequestV1) *restoreStageAuthorizationFixtureV1 {
	t.Helper()
	snapshot := request.Materialization.Snapshots[0]
	seed := buildRestoreStageAuthorizedDispatchV1(t, request, snapshot, now)
	legacy := seed.Dispatch.Record.Permit.LegacyPermit
	operationDigest, err := legacy.Operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	provider := legacy.Provider
	owners := []runtimeports.EffectOwnerRefV2{
		{Role: runtimeports.OwnerCleanup, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
		{Role: runtimeports.OwnerEffect, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
		{Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
	}
	intent := runtimeports.OperationEffectIntentV3{
		ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: legacy.IntentID, Revision: 1, Operation: legacy.Operation,
		Kind: runtimeports.RestoreStageEffectKindV1, RiskClass: "praxis.runtime/high-risk", ActionScopeDigest: core.DigestBytes([]byte("restore-stage-action-scope")),
		PayloadRevision: 1, Target: snapshot.ID, ConflictDomain: legacy.ConflictDomain, Owners: owners, Provider: provider, Authority: legacy.Authority, Review: legacy.Review,
		Budget:      runtimeports.OperationBudgetBindingRefV3{Ref: legacy.Budget.Ref, Digest: legacy.Budget.Digest, Revision: legacy.Budget.Revision, PolicyDigest: legacy.Budget.PolicyDigest, SubjectDigest: operationDigest},
		Policy:      runtimeports.OperationPolicyBindingRefV3{Ref: legacy.Policy.Ref, Digest: legacy.Policy.Digest, Revision: legacy.Policy.Revision, SubjectDigest: operationDigest},
		Idempotency: legacy.Idempotency, CredentialLeases: []runtimeports.CredentialLeaseRefV2{}, RequiresCleanup: true, ExpiresUnixNano: now.Add(20 * time.Second).UnixNano(),
	}
	typedPayload := runtimeports.RestoreStageOperationPayloadV1{ContractVersion: runtimeports.RestoreStageGovernanceContractVersionV1, RestoreAttempt: request.Attempt, Eligibility: request.Eligibility, Identity: request.Materialization.Identity, SnapshotArtifact: snapshot}
	payloadBytes, err := typedPayload.CanonicalBytesV1()
	if err != nil {
		t.Fatal(err)
	}
	intent.Payload = runtimeports.OpaquePayloadV2{Schema: legacy.PayloadSchema, ContentDigest: core.DigestBytes(payloadBytes), Length: uint64(len(payloadBytes)), Inline: payloadBytes, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.runtime/restore-stage-limit", Digest: core.DigestBytes([]byte("restore-stage-limit"))}}
	if err := intent.Validate(); err != nil {
		t.Fatal(err)
	}
	input, err := applicationcontract.SealRestoreStageAuthorizationInputCurrentProjectionV1(applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1{
		RequestDigest: request.Digest, Intent: intent, SnapshotArtifact: snapshot, EvidenceSource: restoreStageEvidenceSourceRefV1("authorization"),
		AuthorizationID: "restore-stage-authorization-real", PermitID: "restore-stage-permit-real", DispatchAttemptID: "restore-stage-dispatch-real", PermitTTL: 15 * time.Second,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(18 * time.Second).UnixNano(),
	}, now)
	if err != nil || input.ValidateFor(request, now) != nil {
		t.Fatalf("authorization input: %v", err)
	}
	admission := &restoreStageAuthorizationAdmissionV1{}
	reviewFact := buildRestoreStageReviewAuthorizationFactV1(t, input, now)
	reviews := &restoreStageAuthorizationReviewV1{value: reviewFact}
	dispatch := &restoreStageAuthorizationDispatchV1{now: now, input: input, review: reviewFact}
	inputs := &restoreStageAuthorizationInputReaderV1{value: input, now: now}
	gateway, err := NewRestoreStageAuthorizationGatewayV1(RestoreStageAuthorizationGatewayConfigV1{Inputs: inputs, Admission: admission, Reviews: reviews, Dispatch: dispatch, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return &restoreStageAuthorizationFixtureV1{now: now, request: request, input: input, inputs: inputs, admission: admission, reviews: reviews, dispatch: dispatch, gateway: gateway}
}

func buildRestoreStageReviewAuthorizationFactV1(t *testing.T, input applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1, now time.Time) runtimeports.OperationReviewAuthorizationFactV4 {
	t.Helper()
	intent := input.Intent
	intentDigest, err := intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	expires := now.Add(15 * time.Second).UnixNano()
	ref := func(id string, digest core.Digest) runtimeports.OperationGovernanceFactRefV3 {
		return runtimeports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: digest, ExpiresUnixNano: expires}
	}
	governance := runtimeports.OperationReviewGovernanceBindingV4{
		SnapshotDigest: core.DigestBytes([]byte("restore-stage-governance-snapshot")), ProjectionWatermark: 1,
		Identity:     ref("restore-stage-identity", core.DigestBytes([]byte("restore-stage-identity"))),
		Binding:      ref(intent.Provider.BindingSetID, core.DigestBytes([]byte("restore-stage-binding"))),
		CurrentScope: ref(intent.Operation.CurrentProjectionRef, intent.Operation.CurrentProjectionDigest),
		Authority:    ref(intent.Authority.Ref, intent.Authority.Digest), Policy: ref(intent.Policy.Ref, intent.Policy.Digest),
		Budget: ref(intent.Budget.Ref, intent.Budget.Digest), CapabilityGrantDigest: core.DigestBytes([]byte("restore-stage-capability")), CredentialGrantDigest: core.DigestBytes([]byte("restore-stage-credential")), ExpiresUnixNano: expires,
	}
	intentBinding := runtimeports.OperationReviewIntentBindingV4{Operation: intent.Operation, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, EffectFactRevision: 1, Target: intent.Target, PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, Provider: intent.Provider, Authority: intent.Authority, ReviewBinding: intent.Review, DispatchPolicy: intent.Policy, IntentExpires: intent.ExpiresUnixNano}
	review, err := runtimeports.SealOperationReviewCurrentProjectionV4(runtimeports.OperationReviewCurrentProjectionV4{
		Operation: intent.Operation, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest,
		PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision,
		Target: runtimeports.OperationReviewTargetRefV4{Ref: intent.Target, Revision: intent.Review.CandidateRevision, Digest: intent.Review.CandidateDigest},
		Case:   ref(intent.Review.CaseRef, core.DigestBytes([]byte("restore-stage-review-case"))), Verdict: ref("restore-stage-verdict", core.DigestBytes([]byte("restore-stage-verdict"))), Basis: runtimeports.OperationReviewBasisAcceptedV4,
		Policy: ref("restore-stage-review-policy", intent.Review.PolicyDigest), ReviewerAuthority: ref("restore-stage-reviewer", core.DigestBytes([]byte("restore-stage-reviewer"))),
		Scope: governance.CurrentScope, Binding: governance.Binding,
		DecisionEvidence: []runtimeports.EvidenceRecordRefV2{{LedgerScopeDigest: core.DigestBytes([]byte("restore-stage-review-ledger")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("restore-stage-review-evidence"))}},
		Current:          true, CurrentnessDigest: core.DigestBytes([]byte("restore-stage-review-current")), ExpiresUnixNano: expires,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryInstance, Scope: intent.Operation.ExecutionScope, CapabilityGrantDigest: governance.CapabilityGrantDigest, EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: intent.Payload.ContentDigest, ExpiresAt: time.Unix(0, expires)}
	fenceDigest, err := runtimeports.DigestOperationExecutionFenceV3(fence, intent.Operation)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := runtimeports.SealOperationReviewAuthorizationFactV4(runtimeports.OperationReviewAuthorizationFactV4{ID: input.AuthorizationID, Revision: 1, State: runtimeports.OperationReviewAuthorizationActiveV4, Intent: intentBinding, Review: review, Governance: governance, Fence: fence, FenceDigest: fenceDigest, RequestedTTLUnixNano: (15 * time.Second).Nanoseconds(), CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

type restoreStageAuthorizationInputReaderV1 struct {
	mu    sync.Mutex
	value applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1
	now   time.Time
	reads int
	drift bool
}

func (r *restoreStageAuthorizationInputReaderV1) InspectRestoreStageAuthorizationInputCurrentV1(_ context.Context, key applicationcontract.RestoreStageActionInspectKeyV1) (applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.RequestDigest != key.RequestDigest {
		return applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "input not found")
	}
	r.reads++
	value := r.value
	if r.drift && r.reads > 1 {
		value.DispatchAttemptID += "-drift"
		var err error
		value, err = applicationcontract.SealRestoreStageAuthorizationInputCurrentProjectionV1(value, r.now)
		if err != nil {
			return applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1{}, err
		}
	}
	return value, nil
}

type restoreStageAuthorizationAdmissionV1 struct {
	mu         sync.Mutex
	value      runtimeports.OperationEffectAdmissionReceiptV3
	admitCalls int
	loseReply  bool
}

func (a *restoreStageAuthorizationAdmissionV1) AdmitOperationEffectV3(_ context.Context, intent runtimeports.OperationEffectIntentV3) (runtimeports.OperationEffectAdmissionReceiptV3, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.admitCalls++
	op, _ := intent.Operation.DigestV3()
	digest, _ := intent.DigestV3()
	a.value = runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: op, EffectID: intent.ID, IntentRevision: intent.Revision, IntentDigest: digest, FactRevision: 1, State: "accepted"}
	if a.loseReply {
		a.loseReply = false
		return runtimeports.OperationEffectAdmissionReceiptV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost Admission reply")
	}
	return a.value, nil
}
func (a *restoreStageAuthorizationAdmissionV1) InspectAcceptedOperationEffectV3(_ context.Context, _ runtimeports.OperationSubjectV3, _ core.EffectIntentID) (runtimeports.OperationEffectAdmissionReceiptV3, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.value.Validate() != nil {
		return runtimeports.OperationEffectAdmissionReceiptV3{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "Admission not found")
	}
	return a.value, nil
}

type restoreStageAuthorizationReviewV1 struct {
	mu          sync.Mutex
	value       runtimeports.OperationReviewAuthorizationFactV4
	created     bool
	createCalls int
	loseReply   bool
}

func (r *restoreStageAuthorizationReviewV1) CreateOperationReviewAuthorizationV4(_ context.Context, _ runtimeports.CreateOperationReviewAuthorizationRequestV4) (runtimeports.OperationReviewAuthorizationFactV4, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createCalls++
	r.created = true
	if r.loseReply {
		r.loseReply = false
		return runtimeports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost Review Authorization reply")
	}
	return r.value, nil
}
func (r *restoreStageAuthorizationReviewV1) InspectCurrentOperationReviewAuthorizationV4(_ context.Context, _ runtimeports.OperationSubjectV3, _ core.EffectIntentID, id string) (runtimeports.OperationReviewAuthorizationFactV4, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.created || r.value.ID != id {
		return runtimeports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "Review Authorization not found")
	}
	return r.value, nil
}

type restoreStageAuthorizationDispatchV1 struct {
	mu                             sync.Mutex
	now                            time.Time
	input                          applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1
	review                         runtimeports.OperationReviewAuthorizationFactV4
	current                        runtimeports.CurrentOperationDispatchAuthorizationV4
	issueCalls, beginCalls         int
	loseIssueReply, loseBeginReply bool
}

func (d *restoreStageAuthorizationDispatchV1) IssueOperationDispatchV4(_ context.Context, request runtimeports.IssueGovernedOperationDispatchRequestV4) (runtimeports.CurrentOperationDispatchAuthorizationV4, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.issueCalls++
	current, err := buildRestoreStageDispatchCurrentV1(d.input, d.review, request.Admission, d.now, false)
	if err != nil {
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	d.current = current
	if d.loseIssueReply {
		d.loseIssueReply = false
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost Permit reply")
	}
	return current, nil
}
func (d *restoreStageAuthorizationDispatchV1) InspectOperationDispatchRecordV4(_ context.Context, request runtimeports.InspectOperationDispatchRecordRequestV4) (runtimeports.OperationDispatchRecordV4, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.current.Record.Validate() != nil || d.current.Record.Permit.LegacyPermit.ID != request.PermitID {
		return runtimeports.OperationDispatchRecordV4{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "Permit not found")
	}
	return d.current.Record, nil
}
func (d *restoreStageAuthorizationDispatchV1) InspectCurrentOperationDispatchV4(_ context.Context, request runtimeports.InspectCurrentOperationDispatchRequestV4) (runtimeports.CurrentOperationDispatchAuthorizationV4, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if currentErr := d.current.Validate(); currentErr != nil || d.current.Record.Permit.Admission.Digest != request.AdmissionDigest || d.current.ReviewAuthorization != request.ReviewAuthorization {
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, fmt.Errorf("%w: current=%v admission_match=%t review_match=%t", core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "current Permit not found"), currentErr, d.current.Record.Permit.Admission.Digest == request.AdmissionDigest, d.current.ReviewAuthorization == request.ReviewAuthorization)
	}
	return d.current, nil
}
func (d *restoreStageAuthorizationDispatchV1) BeginOperationDispatchV4(_ context.Context, _ runtimeports.BeginGovernedOperationDispatchRequestV4) (runtimeports.CurrentOperationDispatchAuthorizationV4, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.beginCalls++
	current, err := buildRestoreStageDispatchCurrentV1(d.input, d.review, d.current.Record.Permit.Admission.Admission, d.now, true)
	if err != nil {
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	d.current = current
	if d.loseBeginReply {
		d.loseBeginReply = false
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost Begin reply")
	}
	return current, nil
}

func buildRestoreStageDispatchCurrentV1(input applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1, authorization runtimeports.OperationReviewAuthorizationFactV4, admission runtimeports.OperationEffectAdmissionReceiptV3, now time.Time, begun bool) (runtimeports.CurrentOperationDispatchAuthorizationV4, error) {
	legacyReview, err := authorization.CompatibilityProjectionV3(now)
	if err != nil {
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	legacyReviewDigest, _ := runtimeports.DigestOperationReviewAuthorizationV3(legacyReview)
	intent := input.Intent
	fenceDigest, _ := runtimeports.DigestOperationExecutionFenceV3(authorization.Fence, intent.Operation)
	legacy := runtimeports.OperationDispatchPermitV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: input.PermitID, Revision: 1, AttemptID: input.DispatchAttemptID, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: admission.IntentDigest, Operation: intent.Operation, PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, ConflictDomain: intent.ConflictDomain, Provider: intent.Provider, EnforcementPoint: intent.Provider, Authority: intent.Authority, Review: intent.Review, ReviewAuthorization: legacyReview, Budget: intent.Budget, Policy: intent.Policy, CapabilityGrantDigest: authorization.Governance.CapabilityGrantDigest, CredentialGrantDigest: authorization.Governance.CredentialGrantDigest, GovernanceSnapshotDigest: authorization.Governance.SnapshotDigest, FenceDigest: fenceDigest, Idempotency: intent.Idempotency, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: authorization.ExpiresUnixNano}
	admitted, err := runtimeports.SealOperationAuthorizedAdmissionV4(runtimeports.OperationAuthorizedAdmissionV4{Admission: admission, Authorization: authorization.RefV4(), PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, ReviewProjectionDigest: authorization.Review.ProjectionDigest, ReviewCurrentnessDigest: authorization.Review.CurrentnessDigest, LegacyReviewProjectionDigest: legacyReviewDigest, GovernanceSnapshotDigest: authorization.Governance.SnapshotDigest, AuthorizationFenceDigest: authorization.FenceDigest, ExpiresUnixNano: authorization.ExpiresUnixNano})
	if err != nil {
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	permit, err := runtimeports.SealOperationDispatchPermitV4(runtimeports.OperationDispatchPermitV4{LegacyPermit: legacy, Admission: admitted})
	if err != nil {
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	state, revision, begunAt := runtimeports.OperationPermitIssuedV4, core.Revision(1), int64(0)
	if begun {
		state, revision, begunAt = runtimeports.OperationPermitBegunV4, 2, now.UnixNano()
	}
	record, err := runtimeports.SealOperationDispatchRecordV4(runtimeports.OperationDispatchRecordV4{Permit: permit, PermitDigest: permit.Digest, Fence: authorization.Fence, State: state, Revision: revision, EffectFactRevision: admission.FactRevision + revision, BegunUnixNano: begunAt})
	if err != nil {
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	current := runtimeports.CurrentOperationDispatchAuthorizationV4{Record: record, ReviewAuthorization: authorization.RefV4(), ReviewProjectionDigest: authorization.Review.ProjectionDigest, ReviewCurrentnessDigest: authorization.Review.CurrentnessDigest, GovernanceSnapshotDigest: authorization.Governance.SnapshotDigest, CheckedUnixNano: now.UnixNano()}
	return current, current.Validate()
}
