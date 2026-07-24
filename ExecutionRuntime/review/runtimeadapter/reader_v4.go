// Package runtimeadapter projects current Review Owner facts into the Runtime
// V4 read-only Review projection. It never creates an Authorization Fact,
// Permit, Begin fact, or provider execution authority.
package runtimeadapter

import (
	"context"
	"reflect"
	"strconv"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const currentSnapshotContractV4 = "praxis.review.runtime-current/v4"

const maxDetachedExactRecoveryV4 = 5 * time.Second

// boundedDetachedExactRecoveryV4 preserves request values while severing the
// caller's cancellation only for one bounded, read-only exact Inspect. The
// timeout is additionally shortened by every subject/snapshot expiry already
// known to the Reader. Logical clocks may differ from wall time, so the
// remaining lifetime is converted to a duration rather than an absolute
// context deadline.
func boundedDetachedExactRecoveryV4(parent context.Context, now time.Time, expiries ...int64) (context.Context, context.CancelFunc, bool) {
	if now.IsZero() {
		return nil, nil, false
	}
	limit := maxDetachedExactRecoveryV4
	for _, expiry := range expiries {
		if expiry <= 0 {
			continue
		}
		remaining := time.Unix(0, expiry).Sub(now)
		if remaining <= 0 {
			return nil, nil, false
		}
		if remaining < limit {
			limit = remaining
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), limit)
	return ctx, cancel, true
}

// ExactCurrentRequestV4 is the complete lookup key passed to the State Plane
// current-fact assembler. Implementations must return one immutable snapshot
// copied from one linearized read; partial or independently timed reads are not
// a valid implementation of CurrentFactSourceV4.
type ExactCurrentRequestV4 struct {
	Operation      runtimeports.OperationSubjectV3 `json:"operation"`
	IntentID       core.EffectIntentID             `json:"intent_id"`
	IntentRevision core.Revision                   `json:"intent_revision"`
	IntentDigest   core.Digest                     `json:"intent_digest"`
	TargetID       string                          `json:"target_id"`
	TargetRevision core.Revision                   `json:"target_revision"`
	TargetDigest   core.Digest                     `json:"target_digest"`
	CaseID         string                          `json:"case_id"`
}

func (r ExactCurrentRequestV4) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if r.IntentID == "" || r.IntentRevision == 0 || r.TargetID == "" || r.TargetRevision == 0 || r.CaseID == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "exact Review current lookup is incomplete")
	}
	for _, digest := range []core.Digest{r.IntentDigest, r.TargetDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// EvidenceBindingV4 binds one Review evidence reference to the exact Evidence
// Ledger record that represents it. A Review digest alone is not a Ledger ref.
type EvidenceBindingV4 struct {
	Review runtimeports.ReviewEvidenceRefV2 `json:"review_evidence"`
	Ledger runtimeports.EvidenceRecordRefV2 `json:"ledger_record"`
}

func (b EvidenceBindingV4) Validate() error {
	if err := b.Review.Validate(); err != nil {
		return err
	}
	return b.Ledger.Validate()
}

// CurrentFactSnapshotV4 is an atomic read model over facts owned by Review and
// the referenced Policy, Authority, Scope, Binding, Satisfaction and Evidence
// owners. Digest is snapshot integrity, not a Runtime Authorization Fact.
type CurrentFactSnapshotV4 struct {
	Revision             core.Revision                             `json:"revision"`
	Target               contract.TargetSnapshotV1                 `json:"target"`
	Case                 contract.ReviewCaseV1                     `json:"case"`
	Verdict              contract.VerdictV1                        `json:"verdict"`
	Rounds               []contract.ReviewRoundV1                  `json:"rounds"`
	Assignments          []contract.ReviewerAssignmentV1           `json:"assignments"`
	Attestations         []contract.AttestationV1                  `json:"attestations"`
	Policy               runtimeports.ReviewPolicyFactV2           `json:"policy"`
	Satisfaction         *runtimeports.ConditionSatisfactionFactV2 `json:"satisfaction,omitempty"`
	DecisionEvidence     []EvidenceBindingV4                       `json:"decision_evidence"`
	SatisfactionEvidence []EvidenceBindingV4                       `json:"satisfaction_evidence,omitempty"`
	ReviewerAuthority    runtimeports.OperationGovernanceFactRefV3 `json:"reviewer_authority"`
	Scope                runtimeports.OperationGovernanceFactRefV3 `json:"scope"`
	Binding              runtimeports.OperationGovernanceFactRefV3 `json:"binding"`
	Current              bool                                      `json:"current"`
	ExpiresUnixNano      int64                                     `json:"expires_unix_nano"`
	Digest               core.Digest                               `json:"digest"`
}

// DigestV4 returns the canonical digest of the complete atomic read model.
func (s CurrentFactSnapshotV4) DigestV4() (core.Digest, error) {
	copyValue := cloneSnapshot(s)
	copyValue.Digest = ""
	return core.CanonicalJSONDigest("praxis.review.runtime-current", currentSnapshotContractV4, "CurrentFactSnapshotV4", copyValue)
}

// SealCurrentFactSnapshotV4 is used by a current-fact assembler after it has
// completed one atomic Inspect. It only seals the read model itself.
func SealCurrentFactSnapshotV4(s CurrentFactSnapshotV4) (CurrentFactSnapshotV4, error) {
	s = cloneSnapshot(s)
	s.Digest = ""
	digest, err := s.DigestV4()
	if err != nil {
		return CurrentFactSnapshotV4{}, err
	}
	s.Digest = digest
	return s, nil
}

// CurrentFactSourceV4 must perform exact Inspect only. It must not create or
// update Review, Runtime, Policy, Satisfaction, or Evidence facts.
type CurrentFactSourceV4 interface {
	InspectReviewCurrentFactsV4(context.Context, ExactCurrentRequestV4) (CurrentFactSnapshotV4, error)
}

type Clock func() time.Time

// ReaderV4 implements runtime/ports.OperationReviewCurrentReaderV4.
type ReaderV4 struct {
	source CurrentFactSourceV4
	clock  Clock
}

func NewReaderV4(source CurrentFactSourceV4, clock Clock) (*ReaderV4, error) {
	if source == nil || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review current reader requires source and clock")
	}
	return &ReaderV4{source: source, clock: clock}, nil
}

func (r *ReaderV4) InspectOperationReviewCurrentV4(ctx context.Context, intent runtimeports.OperationEffectIntentV3) (runtimeports.OperationReviewCurrentProjectionV4, error) {
	if err := intent.Validate(); err != nil {
		return runtimeports.OperationReviewCurrentProjectionV4{}, err
	}
	started := r.clock()
	if started.IsZero() {
		return runtimeports.OperationReviewCurrentProjectionV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Review current reader clock is unavailable")
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		return runtimeports.OperationReviewCurrentProjectionV4{}, err
	}
	request := ExactCurrentRequestV4{
		Operation:      intent.Operation,
		IntentID:       intent.ID,
		IntentRevision: intent.Revision,
		IntentDigest:   intentDigest,
		TargetID:       intent.Target,
		TargetRevision: intent.Review.CandidateRevision,
		TargetDigest:   intent.Review.CandidateDigest,
		CaseID:         intent.Review.CaseRef,
	}
	if err := request.Validate(); err != nil {
		return runtimeports.OperationReviewCurrentProjectionV4{}, err
	}

	snapshot, err := r.source.InspectReviewCurrentFactsV4(ctx, request)
	now := r.clock()
	if now.IsZero() || now.Before(started) {
		return runtimeports.OperationReviewCurrentProjectionV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review current reader clock regressed across Inspect")
	}
	if err != nil && unknownReadV4(err) {
		// Inspect is read-only, so an unavailable/lost reply is recovered only by
		// repeating the same exact Inspect once. Recovery is detached from the
		// caller cancellation but bounded by five seconds and every already-known
		// subject/snapshot TTL. No alternate attempt is created.
		originalUnknown := err
		recoveryCtx, cancel, ok := boundedDetachedExactRecoveryV4(ctx, now, intent.ExpiresUnixNano, snapshot.ExpiresUnixNano)
		if !ok {
			return runtimeports.OperationReviewCurrentProjectionV4{}, originalUnknown
		}
		recovered, recoveryErr := r.source.InspectReviewCurrentFactsV4(recoveryCtx, request)
		recoveryContextErr := recoveryCtx.Err()
		cancel()
		retriedAt := r.clock()
		if retriedAt.IsZero() || retriedAt.Before(now) {
			return runtimeports.OperationReviewCurrentProjectionV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review current reader clock regressed across lost-reply Inspect recovery")
		}
		now = retriedAt
		if recoveryErr != nil || recoveryContextErr != nil {
			return runtimeports.OperationReviewCurrentProjectionV4{}, originalUnknown
		}
		snapshot, err = recovered, nil
	}
	if err != nil {
		return runtimeports.OperationReviewCurrentProjectionV4{}, err
	}
	snapshot = cloneSnapshot(snapshot)
	return projectCurrentV4(intent, intentDigest, snapshot, now)
}

func unknownReadV4(err error) bool {
	return core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorUnavailable)
}

func projectCurrentV4(intent runtimeports.OperationEffectIntentV3, intentDigest core.Digest, snapshot CurrentFactSnapshotV4, now time.Time) (runtimeports.OperationReviewCurrentProjectionV4, error) {
	if err := validateSnapshotEnvelope(snapshot, now); err != nil {
		return runtimeports.OperationReviewCurrentProjectionV4{}, err
	}
	target := snapshot.Target
	caseFact := snapshot.Case
	verdict := snapshot.Verdict

	if target.Kind != contract.TargetEffectV1 || target.ID != intent.Target || target.Revision != intent.Review.CandidateRevision || target.Digest != intent.Review.CandidateDigest || target.IntentID != intent.ID || target.IntentRevision != intent.Revision || target.PayloadSchema != intent.Payload.Schema || target.PayloadDigest != intent.Payload.ContentDigest || target.PayloadRevision != intent.PayloadRevision || target.ActionScopeDigest != intent.ActionScopeDigest || !runtimeports.SameExecutionScopeV2(target.Scope, intent.Operation.ExecutionScope) || target.Policy.Digest != intent.Review.PolicyDigest || target.ActorAuthority != intent.Authority {
		return runtimeports.OperationReviewCurrentProjectionV4{}, stale("Review target or Intent drifted")
	}
	if intent.Operation.Kind == runtimeports.OperationScopeRunV3 && target.RunID != intent.Operation.RunID {
		return runtimeports.OperationReviewCurrentProjectionV4{}, stale("Review target source Run drifted")
	}
	if target.CurrentScope.Ref != intent.Operation.CurrentProjectionRef || target.CurrentScope.Revision != intent.Operation.CurrentProjectionRevision || target.CurrentScope.Digest != intent.Operation.CurrentProjectionDigest {
		return runtimeports.OperationReviewCurrentProjectionV4{}, stale("Review target current Scope drifted")
	}
	if caseFact.State != contract.CaseResolvedV1 || caseFact.Revision != verdict.CaseRevision+1 || caseFact.ID != intent.Review.CaseRef || caseFact.TargetID != target.ID || caseFact.TargetRevision != target.Revision || caseFact.TargetDigest != target.Digest || caseFact.VerdictID != verdict.ID || caseFact.VerdictRevision != verdict.Revision || caseFact.VerdictDigest != verdict.Digest {
		return runtimeports.OperationReviewCurrentProjectionV4{}, stale("Review Case is not exactly resolved by the current Verdict")
	}
	currentness := contract.TargetCurrentnessV1{TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, PayloadRevision: target.PayloadRevision, PayloadDigest: target.PayloadDigest, Scope: target.Scope, ActionScopeDigest: target.ActionScopeDigest, Policy: target.Policy, ActorAuthority: target.ActorAuthority, CurrentScope: target.CurrentScope, EvidenceSetDigest: target.EvidenceSetDigest, ContextFrameDigest: target.ContextFrameDigest, Now: now}
	if err := verdict.ValidateCurrent(contract.VerdictCurrentnessV1{Target: currentness, ReviewerAuthority: verdict.ReviewerAuthority, Now: now}); err != nil {
		return runtimeports.OperationReviewCurrentProjectionV4{}, err
	}
	if verdict.CaseID != caseFact.ID || verdict.CaseDigest == "" || verdict.IntentID != intent.ID || verdict.IntentRevision != intent.Revision || verdict.PayloadRevision != intent.PayloadRevision || verdict.PayloadDigest != intent.Payload.ContentDigest || verdict.Policy != target.Policy || verdict.ActorAuthority != intent.Authority {
		return runtimeports.OperationReviewCurrentProjectionV4{}, stale("Review Verdict binding drifted")
	}
	if err := validateCurrentRefs(intent, snapshot, now); err != nil {
		return runtimeports.OperationReviewCurrentProjectionV4{}, err
	}
	if err := validatePolicy(intent, target, verdict, snapshot.Policy, now); err != nil {
		return runtimeports.OperationReviewCurrentProjectionV4{}, err
	}
	decisionEvidence, err := validateDecisionChain(snapshot, now)
	if err != nil {
		return runtimeports.OperationReviewCurrentProjectionV4{}, err
	}

	basis := runtimeports.OperationReviewAuthorizationBasisV4("")
	var satisfaction *runtimeports.OperationReviewConditionSatisfactionV4
	switch verdict.State {
	case contract.VerdictAcceptedV1:
		if snapshot.Satisfaction != nil || len(snapshot.SatisfactionEvidence) != 0 {
			return runtimeports.OperationReviewCurrentProjectionV4{}, stale("accepted Verdict cannot carry condition Satisfaction")
		}
		if snapshot.Policy.OperationNotRequired {
			return runtimeports.OperationReviewCurrentProjectionV4{}, stale("operation_not_required requires an independent current PolicyNotRequired fact and is unsupported")
		}
		basis = runtimeports.OperationReviewBasisAcceptedV4
	case contract.VerdictConditionalV1:
		if snapshot.Policy.OperationNotRequired {
			return runtimeports.OperationReviewCurrentProjectionV4{}, stale("operation-not-required Policy cannot be combined with a conditional Verdict")
		}
		basis = runtimeports.OperationReviewBasisConditionalSatisfiedV4
		value, err := validateSatisfaction(snapshot, intent, now)
		if err != nil {
			return runtimeports.OperationReviewCurrentProjectionV4{}, err
		}
		satisfaction = &value
	case contract.VerdictRejectedV1, contract.VerdictExpiredV1, contract.VerdictRevokedV1, contract.VerdictSupersededV1:
		return runtimeports.OperationReviewCurrentProjectionV4{}, stale("Review Verdict does not authorize this operation")
	default:
		return runtimeports.OperationReviewCurrentProjectionV4{}, stale("unknown Review Verdict state")
	}

	projection := runtimeports.OperationReviewCurrentProjectionV4{
		Operation:         intent.Operation,
		IntentID:          intent.ID,
		IntentRevision:    intent.Revision,
		IntentDigest:      intentDigest,
		PayloadSchema:     intent.Payload.Schema,
		PayloadDigest:     intent.Payload.ContentDigest,
		PayloadRevision:   intent.PayloadRevision,
		Target:            runtimeports.OperationReviewTargetRefV4{Ref: target.ID, Revision: target.Revision, Digest: target.Digest},
		Case:              ownerRef(caseFact.ID, caseFact.Revision, caseFact.Digest, caseFact.ExpiresUnixNano),
		Verdict:           ownerRef(verdict.ID, verdict.Revision, verdict.Digest, verdict.ExpiresUnixNano),
		Basis:             basis,
		Satisfaction:      satisfaction,
		Policy:            ownerRef(snapshot.Policy.Ref, snapshot.Policy.Revision, snapshot.Policy.Digest, snapshot.Policy.ExpiresUnixNano),
		ReviewerAuthority: snapshot.ReviewerAuthority,
		Scope:             snapshot.Scope,
		Binding:           snapshot.Binding,
		DecisionEvidence:  decisionEvidence,
		Current:           true,
		CurrentnessDigest: snapshot.Digest,
		ExpiresUnixNano:   snapshot.ExpiresUnixNano,
	}
	sealed, err := runtimeports.SealOperationReviewCurrentProjectionV4(projection, now)
	if err != nil {
		return runtimeports.OperationReviewCurrentProjectionV4{}, err
	}
	if err := sealed.ValidateAgainstIntent(intent, runtimeports.OperationGovernanceSnapshotV3{Operation: intent.Operation, Active: true, ProjectionWatermark: 1, Binding: snapshot.Binding, CurrentScope: snapshot.Scope, Review: runtimeports.OperationReviewAuthorizationV3{Case: sealed.Case, CandidateDigest: sealed.Target.Digest, CandidateRevision: sealed.Target.Revision, Verdict: sealed.Verdict, Satisfaction: satisfactionRef(satisfaction), ReviewerAuthority: sealed.ReviewerAuthority, PolicyDigest: sealed.Policy.Digest, ExpiresUnixNano: sealed.ExpiresUnixNano}, ExpiresUnixNano: sealed.ExpiresUnixNano}, now); err != nil {
		return runtimeports.OperationReviewCurrentProjectionV4{}, err
	}
	return sealed, nil
}

func validateSnapshotEnvelope(snapshot CurrentFactSnapshotV4, now time.Time) error {
	if snapshot.Revision == 0 || !snapshot.Current || snapshot.ExpiresUnixNano <= 0 || !now.Before(time.Unix(0, snapshot.ExpiresUnixNano)) {
		return stale("Review current snapshot is absent, inactive or expired")
	}
	if err := snapshot.Digest.Validate(); err != nil {
		return err
	}
	digest, err := snapshot.DigestV4()
	if err != nil || digest != snapshot.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review current snapshot digest drifted")
	}
	if err := snapshot.Target.Validate(); err != nil {
		return err
	}
	if err := snapshot.Case.Validate(); err != nil {
		return err
	}
	if err := snapshot.Verdict.Validate(); err != nil {
		return err
	}
	minimum := minimumExpiry(snapshot)
	if minimum <= 0 || snapshot.ExpiresUnixNano != minimum {
		return stale("Review current snapshot TTL does not equal its shortest current input")
	}
	return nil
}

func validateCurrentRefs(intent runtimeports.OperationEffectIntentV3, snapshot CurrentFactSnapshotV4, now time.Time) error {
	for _, ref := range []runtimeports.OperationGovernanceFactRefV3{snapshot.ReviewerAuthority, snapshot.Scope, snapshot.Binding} {
		if err := ref.Validate(now); err != nil {
			return err
		}
	}
	verdict := snapshot.Verdict
	if snapshot.ReviewerAuthority.Ref != verdict.ReviewerAuthority.Ref || snapshot.ReviewerAuthority.Revision != verdict.ReviewerAuthority.Revision || snapshot.ReviewerAuthority.Digest != verdict.ReviewerAuthority.Digest {
		return stale("Reviewer Authority drifted")
	}
	if snapshot.Scope.Ref != snapshot.Target.CurrentScope.Ref || snapshot.Scope.Revision != snapshot.Target.CurrentScope.Revision || snapshot.Scope.Digest != snapshot.Target.CurrentScope.Digest || snapshot.Scope.Ref != intent.Operation.CurrentProjectionRef || snapshot.Scope.Revision != intent.Operation.CurrentProjectionRevision || snapshot.Scope.Digest != intent.Operation.CurrentProjectionDigest {
		return stale("operation Scope drifted")
	}
	if snapshot.Binding.Ref != intent.Provider.BindingSetID || snapshot.Binding.Revision != intent.Provider.BindingSetRevision {
		return stale("operation Binding drifted")
	}
	return nil
}

func validatePolicy(intent runtimeports.OperationEffectIntentV3, target contract.TargetSnapshotV1, verdict contract.VerdictV1, policy runtimeports.ReviewPolicyFactV2, now time.Time) error {
	if !policy.Active || policy.ExpiresUnixNano <= now.UnixNano() || policy.PolicyDecisionRef == "" {
		return stale("Review Policy is inactive or expired")
	}
	digest, err := policy.DigestV2()
	if err != nil || digest != policy.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Policy digest drifted")
	}
	if policy.Ref != target.Policy.Ref || policy.Revision != target.Policy.Revision || policy.Digest != target.Policy.Digest || policy.Digest != intent.Review.PolicyDigest || policy.SubjectDigest != target.SubjectDigest || policy.SubjectDigest != verdict.SubjectDigest || !runtimeports.SameExecutionScopeV2(policy.Scope, target.Scope) || policy.RunID != target.RunID || policy.CurrentScope != target.CurrentScope || policy.RiskClass != intent.RiskClass || policy.ActorAuthorityRef != target.ActorAuthority.Ref || policy.ReviewerAuthorityRef != verdict.ReviewerAuthority.Ref {
		return stale("Review Policy, subject, Authority or Scope drifted")
	}
	if target.ActorAuthority.Ref == verdict.ReviewerAuthority.Ref && !policy.AllowSelfReview {
		return stale("Review Policy does not allow self review")
	}
	return nil
}

func validateDecisionChain(snapshot CurrentFactSnapshotV4, now time.Time) ([]runtimeports.EvidenceRecordRefV2, error) {
	if len(snapshot.Attestations) == 0 || len(snapshot.Attestations) != len(snapshot.Verdict.AttestationRefs) || len(snapshot.Assignments) != len(snapshot.Attestations) || len(snapshot.Rounds) != len(snapshot.Attestations) {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Verdict attestation and assignment set is incomplete")
	}
	attestations := make(map[string]contract.AttestationV1, len(snapshot.Attestations))
	assignments := make(map[string]contract.ReviewerAssignmentV1, len(snapshot.Assignments))
	rounds := make(map[string]contract.ReviewRoundV1, len(snapshot.Rounds))
	var reviewEvidence []runtimeports.ReviewEvidenceRefV2
	for _, round := range snapshot.Rounds {
		if err := round.Validate(); err != nil {
			return nil, err
		}
		if _, exists := rounds[round.ID]; exists {
			return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "duplicate Review round")
		}
		if now.UnixNano() >= round.ExpiresUnixNano || round.CaseID != snapshot.Case.ID || round.TargetID != snapshot.Target.ID || round.TargetRevision != snapshot.Target.Revision || round.TargetDigest != snapshot.Target.Digest {
			return nil, stale("Review Round subject or TTL drifted")
		}
		rounds[round.ID] = round
	}
	for _, assignment := range snapshot.Assignments {
		if err := assignment.Validate(); err != nil {
			return nil, err
		}
		if _, exists := assignments[assignment.ID]; exists {
			return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "duplicate reviewer assignment")
		}
		assignments[assignment.ID] = assignment
	}
	for _, attestation := range snapshot.Attestations {
		if err := attestation.ValidateProductionAutoProvenanceV4(); err != nil {
			return nil, err
		}
		if _, exists := attestations[attestation.ID]; exists {
			return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "duplicate Review attestation")
		}
		assignment, ok := assignments[attestation.AssignmentID]
		round, roundOK := rounds[attestation.RoundID]
		if !ok || !roundOK || assignment.State != contract.AssignmentClaimedV1 || now.UnixNano() >= assignment.LeaseExpiresUnixNano || now.UnixNano() >= assignment.ExpiresUnixNano || assignment.CaseID != snapshot.Case.ID || assignment.CaseRevision != round.CaseRevision || assignment.RoundID != round.ID || assignment.RoundRevision != round.Revision || assignment.RoundDigest != round.Digest || assignment.TargetID != snapshot.Target.ID || assignment.TargetRevision != snapshot.Target.Revision || assignment.TargetDigest != snapshot.Target.Digest || assignment.ReviewerID != attestation.ReviewerID || assignment.ReviewerAuthority != attestation.ReviewerAuthority || assignment.ReviewerBinding != attestation.ReviewerBinding || attestation.CaseID != snapshot.Case.ID || attestation.RoundRevision != round.Revision || attestation.RoundDigest != round.Digest || attestation.AssignmentRevision != assignment.Revision || attestation.AssignmentDigest != assignment.Digest || attestation.TargetID != snapshot.Target.ID || attestation.TargetRevision != snapshot.Target.Revision || attestation.TargetDigest != snapshot.Target.Digest || attestation.ContextFrameDigest != snapshot.Target.ContextFrameDigest || attestation.ReviewerAuthority != snapshot.Verdict.ReviewerAuthority || attestation.ReviewerID != snapshot.Verdict.ReviewerID || attestation.ReviewerBinding != snapshot.Verdict.ReviewerBinding || now.UnixNano() >= attestation.ExpiresUnixNano {
			return nil, stale("Review attestation or Assignment lease drifted")
		}
		if snapshot.Verdict.RoundID != round.ID || snapshot.Verdict.RoundRevision != round.Revision || snapshot.Verdict.RoundDigest != round.Digest || snapshot.Verdict.AssignmentID != assignment.ID || snapshot.Verdict.AssignmentRevision != assignment.Revision || snapshot.Verdict.AssignmentDigest != assignment.Digest {
			return nil, stale("Verdict Round or Assignment binding drifted")
		}
		attestations[attestation.ID] = attestation
		reviewEvidence = append(reviewEvidence, attestation.Evidence...)
	}
	for _, id := range snapshot.Verdict.AttestationRefs {
		attestation, ok := attestations[id]
		if !ok {
			return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Verdict attestation set drifted")
		}
		expectedResolution := contract.ResolutionAcceptV1
		switch snapshot.Verdict.State {
		case contract.VerdictConditionalV1:
			expectedResolution = contract.ResolutionConditionalV1
		case contract.VerdictRejectedV1:
			expectedResolution = contract.ResolutionRejectV1
		}
		if attestation.Resolution != expectedResolution {
			return nil, stale("Verdict decision differs from its exact attestation")
		}
		if attestation.ConditionsDigest != snapshot.Verdict.ConditionsDigest || !reflect.DeepEqual(attestation.Conditions, snapshot.Verdict.Conditions) {
			return nil, stale("Verdict condition set differs from its exact attestation")
		}
	}
	evidenceDigest, err := contract.ComputeReviewEvidenceDigestV1(reviewEvidence)
	if err != nil || evidenceDigest != snapshot.Verdict.EvidenceDigest {
		return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Verdict decision evidence drifted")
	}
	return ledgerEvidence(reviewEvidence, snapshot.DecisionEvidence)
}

func validateSatisfaction(snapshot CurrentFactSnapshotV4, intent runtimeports.OperationEffectIntentV3, now time.Time) (runtimeports.OperationReviewConditionSatisfactionV4, error) {
	if snapshot.Satisfaction == nil {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "conditional Verdict has no current Satisfaction")
	}
	fact := *snapshot.Satisfaction
	if err := fact.Validate(); err != nil {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, err
	}
	if fact.State != runtimeports.ConditionSatisfied || now.UnixNano() >= fact.ExpiresUnixNano || fact.VerdictID != snapshot.Verdict.ID || fact.VerdictRevision != snapshot.Verdict.Revision || fact.VerdictDigest != snapshot.Verdict.Digest || fact.CandidateDigest != snapshot.Target.Digest || fact.IntentID != intent.ID || fact.IntentRevision != intent.Revision || fact.SubjectDigest != snapshot.Target.SubjectDigest || fact.ConditionsDigest != snapshot.Verdict.ConditionsDigest || fact.Policy != snapshot.Target.Policy || !runtimeports.SameExecutionScopeV2(fact.Scope, snapshot.Target.Scope) || fact.RunID != snapshot.Target.RunID || fact.ActionScopeDigest != snapshot.Target.ActionScopeDigest || fact.CurrentScope != snapshot.Target.CurrentScope {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "condition Satisfaction drifted")
	}
	conditionByID := make(map[runtimeports.NamespacedNameV2]runtimeports.ReviewConditionV2, len(snapshot.Verdict.Conditions))
	for _, condition := range snapshot.Verdict.Conditions {
		conditionByID[condition.ID] = condition
	}
	if len(fact.Proofs) != len(conditionByID) {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "condition proof set is incomplete")
	}
	reviewEvidence := make([]runtimeports.ReviewEvidenceRefV2, 0, len(fact.Proofs))
	for _, proof := range fact.Proofs {
		condition, ok := conditionByID[proof.ConditionID]
		if !ok || proof.ConditionRevision != condition.Revision || proof.ConstraintDigest != condition.ConstraintDigest || proof.Owner != condition.SatisfactionOwner || proof.ScopeDigest != condition.ScopeDigest || proof.Authority != condition.Authority || proof.ExpiresUnixNano > condition.ExpiresUnixNano || proof.ExpiresUnixNano <= now.UnixNano() || proof.ScopeDigest != snapshot.Target.ActionScopeDigest {
			return runtimeports.OperationReviewConditionSatisfactionV4{}, core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "condition proof Scope, Authority or TTL drifted")
		}
		reviewEvidence = append(reviewEvidence, proof.Evidence)
	}
	evidence, err := ledgerEvidence(reviewEvidence, snapshot.SatisfactionEvidence)
	if err != nil {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, err
	}
	factDigest, err := fact.DigestV2()
	if err != nil {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, err
	}
	return runtimeports.OperationReviewConditionSatisfactionV4{Fact: ownerRef(fact.ID, fact.Revision, factDigest, fact.ExpiresUnixNano), ConditionsDigest: fact.ConditionsDigest, Evidence: evidence}, nil
}

func ledgerEvidence(expected []runtimeports.ReviewEvidenceRefV2, bindings []EvidenceBindingV4) ([]runtimeports.EvidenceRecordRefV2, error) {
	if len(expected) == 0 || len(expected) != len(bindings) {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Review evidence has no exact Ledger mapping")
	}
	expectedByRef := make(map[string]runtimeports.ReviewEvidenceRefV2, len(expected))
	for _, value := range expected {
		if err := value.Validate(); err != nil {
			return nil, err
		}
		if _, exists := expectedByRef[value.Ref]; exists {
			return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Review evidence ref is duplicated")
		}
		expectedByRef[value.Ref] = value
	}
	ledger := make([]runtimeports.EvidenceRecordRefV2, 0, len(bindings))
	seenLedger := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return nil, err
		}
		value, ok := expectedByRef[binding.Review.Ref]
		if !ok || value != binding.Review {
			return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review-to-Ledger evidence mapping drifted")
		}
		delete(expectedByRef, binding.Review.Ref)
		key := string(binding.Ledger.LedgerScopeDigest) + "\x00" + strconv.FormatUint(binding.Ledger.Sequence, 10) + "\x00" + string(binding.Ledger.RecordDigest)
		if _, exists := seenLedger[key]; exists {
			return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Ledger evidence mapping is duplicated")
		}
		seenLedger[key] = struct{}{}
		ledger = append(ledger, binding.Ledger)
	}
	if len(expectedByRef) != 0 {
		return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence mapping is incomplete")
	}
	if _, err := runtimeports.DigestOperationReviewEvidenceV4(ledger); err != nil {
		return nil, err
	}
	return ledger, nil
}

func minimumExpiry(snapshot CurrentFactSnapshotV4) int64 {
	values := []int64{snapshot.Target.ExpiresUnixNano, snapshot.Case.ExpiresUnixNano, snapshot.Verdict.ExpiresUnixNano, snapshot.Policy.ExpiresUnixNano, snapshot.ReviewerAuthority.ExpiresUnixNano, snapshot.Scope.ExpiresUnixNano, snapshot.Binding.ExpiresUnixNano}
	for _, round := range snapshot.Rounds {
		values = append(values, round.ExpiresUnixNano)
	}
	for _, assignment := range snapshot.Assignments {
		values = append(values, assignment.ExpiresUnixNano, assignment.LeaseExpiresUnixNano)
	}
	for _, attestation := range snapshot.Attestations {
		values = append(values, attestation.ExpiresUnixNano)
		for _, condition := range attestation.Conditions {
			values = append(values, condition.ExpiresUnixNano)
		}
	}
	for _, condition := range snapshot.Verdict.Conditions {
		values = append(values, condition.ExpiresUnixNano)
	}
	if snapshot.Satisfaction != nil {
		values = append(values, snapshot.Satisfaction.ExpiresUnixNano)
		for _, proof := range snapshot.Satisfaction.Proofs {
			values = append(values, proof.ExpiresUnixNano)
		}
	}
	minimum := int64(0)
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

func ownerRef(id string, revision core.Revision, digest core.Digest, expires int64) runtimeports.OperationGovernanceFactRefV3 {
	return runtimeports.OperationGovernanceFactRefV3{Ref: id, Revision: revision, Digest: digest, ExpiresUnixNano: expires}
}

func satisfactionRef(value *runtimeports.OperationReviewConditionSatisfactionV4) *runtimeports.OperationGovernanceFactRefV3 {
	if value == nil {
		return nil
	}
	ref := value.Fact
	return &ref
}

func stale(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, message)
}

func cloneSnapshot(value CurrentFactSnapshotV4) CurrentFactSnapshotV4 {
	value.Rounds = append([]contract.ReviewRoundV1(nil), value.Rounds...)
	value.Assignments = append([]contract.ReviewerAssignmentV1(nil), value.Assignments...)
	value.Attestations = append([]contract.AttestationV1(nil), value.Attestations...)
	for index := range value.Attestations {
		value.Attestations[index].ReasonCodes = append([]string(nil), value.Attestations[index].ReasonCodes...)
		value.Attestations[index].FindingRefs = append([]string(nil), value.Attestations[index].FindingRefs...)
		value.Attestations[index].Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), value.Attestations[index].Evidence...)
		value.Attestations[index].Conditions = append([]runtimeports.ReviewConditionV2(nil), value.Attestations[index].Conditions...)
	}
	value.DecisionEvidence = append([]EvidenceBindingV4(nil), value.DecisionEvidence...)
	value.SatisfactionEvidence = append([]EvidenceBindingV4(nil), value.SatisfactionEvidence...)
	value.Target.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), value.Target.Evidence...)
	value.Verdict.AttestationRefs = append([]string(nil), value.Verdict.AttestationRefs...)
	value.Verdict.ReasonCodes = append([]string(nil), value.Verdict.ReasonCodes...)
	value.Verdict.Conditions = append([]runtimeports.ReviewConditionV2(nil), value.Verdict.Conditions...)
	if value.Satisfaction != nil {
		copyFact := *value.Satisfaction
		copyFact.Proofs = append([]runtimeports.ReviewConditionProofV2(nil), value.Satisfaction.Proofs...)
		value.Satisfaction = &copyFact
	}
	return value
}

var _ runtimeports.OperationReviewCurrentReaderV4 = (*ReaderV4)(nil)
