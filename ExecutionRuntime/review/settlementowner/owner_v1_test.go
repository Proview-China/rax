package settlementowner

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func settlementDigestV1(value string) core.Digest { return core.DigestBytes([]byte(value)) }

type settlementFixtureV1 struct {
	now         time.Time
	source      contract.AutoReviewerAttemptV1
	attempt     contract.AutoReviewerAttemptV1
	observation contract.AutoReviewerInvocationObservationV1
	result      contract.ReviewerInvocationResultFactV1
	inspection  runtimeports.OperationInspectionSettlementRefV4
}

func newSettlementFixtureV1(t testing.TB) settlementFixtureV1 {
	t.Helper()
	now := time.Unix(400_010, 0)
	submission := testsupport.OperationSettlementSubmissionV4()
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "review-auto-delegation", Revision: 1, Digest: settlementDigestV1("review-auto-delegation")}
	runtimeAttempt := submission.DomainResult.Attempt
	runtimeAttempt.Delegation = &delegation
	for index := range submission.Evidence {
		submission.Evidence[index].Attempt = runtimeAttempt
	}
	output, err := contract.SealAutoReviewerStructuredOutputV1(contract.AutoReviewerStructuredOutputV1{Resolution: contract.ResolutionAcceptV1, ReasonCodes: []string{"review.auto/settled"}, Evidence: []runtimeports.ReviewEvidenceRefV2{{Ref: "evidence://settled", Classification: "review.auto/settled", Digest: settlementDigestV1("settled-evidence")}}})
	if err != nil {
		t.Fatal(err)
	}
	identity := func(id string) contract.ExactResourceRefV1 {
		return contract.ExactResourceRefV1{ID: id, Revision: 1, Digest: settlementDigestV1(id)}
	}
	result, err := contract.SealReviewerInvocationResultFactV1(contract.ReviewerInvocationResultFactV1{FactIdentityV1: contract.FactIdentityV1{TenantID: submission.TenantID, ID: "review-auto-domain-result", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, CaseID: "review-auto-case", CaseRevision: 7, RoundID: "review-auto-round", RoundRevision: 1, RoundDigest: settlementDigestV1("review-auto-round"), AssignmentID: "review-auto-assignment", AssignmentRevision: 2, AssignmentDigest: settlementDigestV1("review-auto-assignment"), TargetID: "review-auto-target", TargetRevision: 3, TargetDigest: settlementDigestV1("review-auto-target"), AttemptID: runtimeAttempt.AttemptID, ResultSchema: submission.DomainResult.Schema, ResultDigest: output.Digest, ObservationRefs: []string{"review-auto-observation"}})
	if err != nil {
		t.Fatal(err)
	}
	providerObservation := runtimeports.ProviderAttemptObservationRefV2{Delegation: delegation, PreparedAttemptID: "review-auto-prepared", ProviderOperationRef: "review-auto-provider-operation", Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: settlementDigestV1("provider-observation"), PayloadDigest: output.Digest, PayloadRevision: 1, SourceRegistrationID: "review-auto-source", SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: settlementDigestV1("ledger"), Sequence: 1, RecordDigest: settlementDigestV1("record")}, ObservedUnixNano: now.UnixNano()}
	prepared, err := contract.SealAutoReviewerAttemptV1(contract.AutoReviewerAttemptV1{FactIdentityV1: contract.FactIdentityV1{TenantID: submission.TenantID, ID: "review-auto-attempt", Revision: 1, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano()}, IdempotencyKey: "review-auto-idempotency", Case: contract.ExactResourceRefV1{ID: "review-auto-case", Revision: 7, Digest: settlementDigestV1("review-auto-case")}, Round: identity("review-auto-round"), Assignment: contract.ExactResourceRefV1{ID: "review-auto-assignment", Revision: 2, Digest: settlementDigestV1("review-auto-assignment")}, Target: contract.ExactResourceRefV1{ID: "review-auto-target", Revision: 3, Digest: settlementDigestV1("review-auto-target")}, Rubric: identity("review-auto-rubric"), ContextFrameDigest: settlementDigestV1("context"), ReviewerID: "review-auto-reviewer", ReviewerAuthority: runtimeports.AuthorityBindingRefV2{Ref: "review-auto-authority", Revision: 1, Digest: settlementDigestV1("authority"), Epoch: 1}, ReviewerBinding: runtimeports.ReviewComponentBindingRefV2{BindingSetID: "review-auto-binding", BindingSetRevision: 1, ComponentID: "praxis.review/auto-reviewer", ManifestDigest: settlementDigestV1("manifest"), ArtifactDigest: settlementDigestV1("artifact"), Capability: "praxis.review/attest"}, RouteID: "praxis.model/review-route", Operation: submission.Operation, OperationDigest: submission.OperationDigest, InvocationEffect: runtimeports.ReviewInvocationEffectRefV2{EffectID: submission.EffectID, EffectRevision: runtimeAttempt.IntentRevision, EffectKind: "praxis.review/auto-reviewer-invoke", PayloadDigest: settlementDigestV1("payload"), Provider: submission.DomainResult.Owner}, ResultSchema: result.ResultSchema, RoundOrdinal: 1, MaxCostMicros: 1_000, State: contract.AutoReviewerAttemptPreparedV1, ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := contract.SealAutoReviewerInvocationObservationV1(contract.AutoReviewerInvocationObservationV1{FactIdentityV1: contract.FactIdentityV1{TenantID: submission.TenantID, ID: "review-auto-observation", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, AttemptID: prepared.ID, AttemptRevision: prepared.Revision, AttemptDigest: prepared.Digest, OperationDigest: submission.OperationDigest, RuntimeAttempt: runtimeAttempt, ProviderObservation: providerObservation, Output: output, ResultSchema: result.ResultSchema, Tokens: 100, CostMicros: 50, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	attempt := prepared
	attempt.Revision++
	attempt.UpdatedUnixNano = now.UnixNano()
	attempt.State = contract.AutoReviewerAttemptObservedV1
	attempt.InvocationAttempt = refResourceV1(prepared.ExactRef())
	attempt.Observation = refObservationV1(observation.Ref())
	attempt.DomainResult = refResourceV1(result.ExactRef())
	attempt.Digest = ""
	attempt, err = contract.SealAutoReviewerAttemptV1(attempt)
	if err != nil {
		t.Fatal(err)
	}
	// Runtime stores the Review Owner's exact DomainResult Fact and payload.
	submission.DomainResult.ID = result.ID
	submission.DomainResult.Revision = result.Revision
	submission.DomainResult.Digest = result.Digest
	submission.DomainResult.TenantID = result.TenantID
	submission.DomainResult.Attempt = runtimeAttempt
	submission.DomainResult.Schema = result.ResultSchema
	submission.DomainResult.PayloadDigest = result.ResultDigest
	submission.DomainResult.PayloadRevision = 1
	submission.DomainResult.AuthoritativeTime = now.UnixNano()
	submission.Digest = ""
	submission, err = runtimeports.SealOperationSettlementSubmissionV4(submission)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := control.BuildOperationSettlementCommitBundleV4(submission)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{Settlement: bundle.Settlement.RefV4(), Association: bundle.Association.RefV4(), Guard: bundle.Guard.RefV4(), Projection: bundle.Projection.RefV4(), DomainResult: submission.DomainResult, EffectFactRevision: 4, Owner: submission.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	return settlementFixtureV1{now: now, source: prepared, attempt: attempt, observation: observation, result: result, inspection: inspection}
}

func refObservationV1(value contract.AutoReviewerInvocationObservationRefV1) *contract.AutoReviewerInvocationObservationRefV1 {
	return &value
}
func refResourceV1(value contract.ExactResourceRefV1) *contract.ExactResourceRefV1 { return &value }

type settlementStoreV1 struct {
	mu             sync.Mutex
	fixture        settlementFixtureV1
	apply          *contract.DomainApplySettlementFactV1
	loseReply      bool
	cancelOnCreate func()
	recoveryClean  bool
	blockRecovery  bool
	sourceErr      error
}

func (s *settlementStoreV1) InspectAutoReviewerAttemptExactV1(_ context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1) (contract.AutoReviewerAttemptV1, error) {
	if tenant != s.fixture.attempt.TenantID {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Attempt tenant missing")
	}
	if ref == s.fixture.attempt.ExactRef() {
		return s.fixture.attempt, nil
	}
	if ref == s.fixture.source.ExactRef() {
		if s.sourceErr != nil {
			return contract.AutoReviewerAttemptV1{}, s.sourceErr
		}
		return s.fixture.source, nil
	}
	return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Attempt history missing")
}
func (s *settlementStoreV1) InspectAutoReviewerObservationExactV1(context.Context, core.TenantID, contract.AutoReviewerInvocationObservationRefV1) (contract.AutoReviewerInvocationObservationV1, error) {
	return s.fixture.observation, nil
}
func (s *settlementStoreV1) InspectDomainResultExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.ReviewerInvocationResultFactV1, error) {
	return s.fixture.result, nil
}
func (s *settlementStoreV1) CreateApplySettlementV1(_ context.Context, value contract.DomainApplySettlementFactV1) (contract.DomainApplySettlementFactV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyValue := value
	s.apply = &copyValue
	if s.cancelOnCreate != nil {
		s.cancelOnCreate()
	}
	if s.loseReply {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost Apply reply")
	}
	return copyValue, nil
}
func (s *settlementStoreV1) InspectApplySettlementExactV1(ctx context.Context, _ core.TenantID, ref reviewport.ExactFactRefV1) (contract.DomainApplySettlementFactV1, error) {
	if s.blockRecovery {
		<-ctx.Done()
		return contract.DomainApplySettlementFactV1{}, ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recoveryClean = ctx.Err() == nil
	if s.apply == nil || s.apply.ID != ref.ID || s.apply.Revision != ref.Revision || s.apply.Digest != ref.Digest {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Apply missing")
	}
	return *s.apply, nil
}

type settlementRuntimeV1 struct {
	inspection runtimeports.OperationInspectionSettlementRefV4
}

func (r *settlementRuntimeV1) InspectCurrentOperationSettlementV4(context.Context, runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	return r.inspection, nil
}

type sequenceClockV1 struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func (c *sequenceClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	value := c.values[c.index]
	c.index++
	return value
}

func TestSettlementOwnerAppliesExactCurrentV4AndRecoversLostReplyV1(t *testing.T) {
	fixture := newSettlementFixtureV1(t)
	ctx, cancel := context.WithCancel(context.Background())
	store := &settlementStoreV1{fixture: fixture, loseReply: true, cancelOnCreate: cancel}
	runtime := &settlementRuntimeV1{inspection: fixture.inspection}
	clock := &sequenceClockV1{values: []time.Time{fixture.now, fixture.now.Add(time.Second)}}
	owner, err := NewV1(store, runtime, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	apply, err := owner.ApplyV1(ctx, ApplyCommandV1{TenantID: fixture.result.TenantID, Attempt: fixture.attempt.ExactRef(), DomainResult: fixture.result.ExactRef(), ApplyID: "review-auto-apply"})
	if err != nil {
		t.Fatal(err)
	}
	if apply.State != contract.DomainApplyAppliedV1 || apply.RuntimeContractVersion != runtimeports.OperationSettlementContractVersionV4 || apply.RuntimeInspectionDigest != fixture.inspection.Digest || !store.recoveryClean {
		t.Fatalf("unexpected ApplySettlement or cancelled recovery context: %+v clean=%v", apply, store.recoveryClean)
	}
}

func TestSettlementOwnerLostReplyRecoveryIsBoundedAndPreservesOriginalUnknownV1(t *testing.T) {
	fixture := newSettlementFixtureV1(t)
	store := &settlementStoreV1{fixture: fixture, loseReply: true, blockRecovery: true}
	owner, err := NewV1(store, &settlementRuntimeV1{inspection: fixture.inspection}, (&sequenceClockV1{values: []time.Time{fixture.now, fixture.now.Add(time.Second)}}).Now)
	if err != nil {
		t.Fatal(err)
	}
	owner.recoveryTimeout = 10 * time.Millisecond
	started := time.Now()
	_, err = owner.ApplyV1(context.Background(), ApplyCommandV1{TenantID: fixture.result.TenantID, Attempt: fixture.attempt.ExactRef(), DomainResult: fixture.result.ExactRef(), ApplyID: "review-auto-apply-blocked"})
	if !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("blocked recovery replaced the original Unknown: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
		t.Fatalf("blocked ApplySettlement recovery exceeded its bound: %v", elapsed)
	}
}

func TestSettlementOwnerClockRollbackAndDriftWriteZeroV1(t *testing.T) {
	fixture := newSettlementFixtureV1(t)
	for name, testCase := range map[string]struct {
		mutate func(*settlementFixtureV1)
		clocks []time.Time
	}{
		"clock_rollback": {clocks: []time.Time{fixture.now.Add(time.Second), fixture.now}},
		"domain_drift": {mutate: func(value *settlementFixtureV1) {
			value.inspection.DomainResult.PayloadDigest = settlementDigestV1("drift")
		}, clocks: []time.Time{fixture.now, fixture.now.Add(time.Second)}},
		"runtime_attempt_drift": {mutate: func(value *settlementFixtureV1) {
			value.observation.RuntimeAttempt.PermitDigest = settlementDigestV1("other-permit")
			value.observation.Digest = ""
			value.observation, _ = contract.SealAutoReviewerInvocationObservationV1(value.observation)
			value.attempt.Observation = refObservationV1(value.observation.Ref())
			value.attempt.Digest = ""
			value.attempt, _ = contract.SealAutoReviewerAttemptV1(value.attempt)
		}, clocks: []time.Time{fixture.now, fixture.now.Add(time.Second)}},
		"delegation_drift": {mutate: func(value *settlementFixtureV1) {
			delegation := runtimeports.ExecutionDelegationRefV2{ID: "other-delegation", Revision: 1, Digest: settlementDigestV1("other-delegation")}
			value.observation.RuntimeAttempt.Delegation = &delegation
			value.observation.ProviderObservation.Delegation = delegation
			value.observation.Digest = ""
			value.observation, _ = contract.SealAutoReviewerInvocationObservationV1(value.observation)
			value.attempt.Observation = refObservationV1(value.observation.Ref())
			value.attempt.Digest = ""
			value.attempt, _ = contract.SealAutoReviewerAttemptV1(value.attempt)
		}, clocks: []time.Time{fixture.now, fixture.now.Add(time.Second)}},
		"effect_revision_drift": {mutate: func(value *settlementFixtureV1) {
			value.observation.RuntimeAttempt.IntentRevision++
			value.observation.Digest = ""
			value.observation, _ = contract.SealAutoReviewerInvocationObservationV1(value.observation)
			value.attempt.Observation = refObservationV1(value.observation.Ref())
			value.attempt.Digest = ""
			value.attempt, _ = contract.SealAutoReviewerAttemptV1(value.attempt)
		}, clocks: []time.Time{fixture.now, fixture.now.Add(time.Second)}},
		"provider_drift": {mutate: func(value *settlementFixtureV1) {
			value.source.InvocationEffect.Provider.ArtifactDigest = settlementDigestV1("other-provider")
			value.source.Digest = ""
			value.source, _ = contract.SealAutoReviewerAttemptV1(value.source)
			value.observation.AttemptDigest = value.source.Digest
			value.observation.Digest = ""
			value.observation, _ = contract.SealAutoReviewerInvocationObservationV1(value.observation)
			value.attempt.InvocationEffect.Provider = value.source.InvocationEffect.Provider
			value.attempt.InvocationAttempt = refResourceV1(value.source.ExactRef())
			value.attempt.Observation = refObservationV1(value.observation.Ref())
			value.attempt.Digest = ""
			value.attempt, _ = contract.SealAutoReviewerAttemptV1(value.attempt)
		}, clocks: []time.Time{fixture.now, fixture.now.Add(time.Second)}},
		"ttl_crossing": {mutate: func(value *settlementFixtureV1) {}, clocks: []time.Time{fixture.now, time.Unix(0, fixture.inspection.ExpiresUnixNano)}},
	} {
		t.Run(name, func(t *testing.T) {
			value := fixture
			if testCase.mutate != nil {
				testCase.mutate(&value)
			}
			store := &settlementStoreV1{fixture: value}
			owner, err := NewV1(store, &settlementRuntimeV1{inspection: value.inspection}, (&sequenceClockV1{values: testCase.clocks}).Now)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := owner.ApplyV1(context.Background(), ApplyCommandV1{TenantID: value.result.TenantID, Attempt: value.attempt.ExactRef(), DomainResult: value.result.ExactRef(), ApplyID: "review-auto-apply-" + name}); err == nil {
				t.Fatal("invalid settlement path was accepted")
			}
			if store.apply != nil {
				t.Fatal("failed settlement path leaked ApplySettlement")
			}
		})
	}
}

func TestSettlementOwnerRequiresExactPreparedInvocationHistoryV1(t *testing.T) {
	fixture := newSettlementFixtureV1(t)
	for name, mutate := range map[string]func(*settlementStoreV1){
		"missing": func(store *settlementStoreV1) {
			store.sourceErr = core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "source missing")
		},
		"non_prepared": func(store *settlementStoreV1) {
			store.fixture.source.State = contract.AutoReviewerAttemptWaitingInspectV1
		},
		"subject_drift": func(store *settlementStoreV1) {
			store.fixture.source.RouteID = "praxis.model/other-route"
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := &settlementStoreV1{fixture: fixture}
			mutate(store)
			owner, err := NewV1(store, &settlementRuntimeV1{inspection: store.fixture.inspection}, (&sequenceClockV1{values: []time.Time{fixture.now, fixture.now.Add(time.Second)}}).Now)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := owner.ApplyV1(context.Background(), ApplyCommandV1{TenantID: store.fixture.result.TenantID, Attempt: store.fixture.attempt.ExactRef(), DomainResult: store.fixture.result.ExactRef(), ApplyID: "review-auto-apply-source-" + name}); err == nil {
				t.Fatal("invalid invocation history was accepted")
			}
			if store.apply != nil {
				t.Fatal("invalid invocation history leaked ApplySettlement")
			}
		})
	}
}

func TestSettlementOwnerRejectsTypedNilDependenciesV1(t *testing.T) {
	var store *settlementStoreV1
	var runtime *settlementRuntimeV1
	if _, err := NewV1(store, &settlementRuntimeV1{}, time.Now); err == nil {
		t.Fatal("typed-nil Store was accepted")
	}
	if _, err := NewV1(&settlementStoreV1{}, runtime, time.Now); err == nil {
		t.Fatal("typed-nil Runtime reader was accepted")
	}
}
