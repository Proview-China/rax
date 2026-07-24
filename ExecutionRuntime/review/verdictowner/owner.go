// Package verdictowner implements the only Wave 1 Verdict Decide/CAS owner.
package verdictowner

import (
	"context"
	"slices"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type Clock func() time.Time

const lostReplyRecoveryTimeoutV1 = 5 * time.Second

type Owner struct {
	store           reviewport.StoreV1
	current         reviewport.DecisionCurrentReaderV1
	clock           Clock
	recoveryTimeout time.Duration
}

func New(store reviewport.StoreV1, current reviewport.DecisionCurrentReaderV1, clock Clock) (*Owner, error) {
	if nilcheck.IsNil(store) || nilcheck.IsNil(current) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "verdict owner requires Store, exact current reader and clock")
	}
	return &Owner{store: store, current: current, clock: clock, recoveryTimeout: lostReplyRecoveryTimeoutV1}, nil
}

// DecideCommandV1 contains no caller-supplied current values or verdict
// content. State, reviewer, evidence, findings, conditions and TTL are derived
// only from the exact DecisionCurrentSnapshotV1.
type DecideCommandV1 struct {
	TenantID      core.TenantID
	CaseID        string
	Expected      reviewport.ExpectedFactV1
	AttestationID string
	VerdictID     string
	Trace         contract.TraceFactV1
	// AdditionalTraces is the V2 closure batch. A resolved decision carries one
	// deterministic Resolved event for the successor Case.
	AdditionalTraces []contract.TraceFactV1
}

func (o *Owner) DecideV1(ctx context.Context, command DecideCommandV1) (contract.ReviewCaseV1, contract.VerdictV1, error) {
	if command.TenantID == "" || command.CaseID == "" || command.AttestationID == "" || command.VerdictID == "" || command.Expected.Revision == 0 || command.Expected.Digest.Validate() != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Verdict Decide command is incomplete")
	}
	currentCase, err := o.store.InspectCaseV1(ctx, command.TenantID, command.CaseID)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	if currentCase.State == contract.CaseResolvedV1 {
		return o.inspectDecideReplay(ctx, command, currentCase)
	}
	if currentCase.Revision != command.Expected.Revision || currentCase.Digest != command.Expected.Digest {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "verdict expected case is stale")
	}
	if currentCase.State != contract.CaseAttestedV1 && currentCase.State != contract.CaseDecidingV1 {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "case is not ready for Decide")
	}
	baseline := o.clock()
	if baseline.IsZero() {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Verdict Decide baseline clock is unavailable")
	}
	snapshot, inspectErr := o.current.InspectDecisionCurrentV1(ctx, reviewport.DecisionCurrentRequestV1{TenantID: command.TenantID, CaseID: command.CaseID, ExpectedCase: command.Expected, AttestationID: command.AttestationID})
	now := o.clock()
	if now.IsZero() || now.Before(baseline) {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Verdict Decide clock regressed across current Inspect")
	}
	if inspectErr != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, inspectErr
	}
	if err := validateDecisionSnapshot(snapshot, command, now); err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	state, err := verdictStateFor(snapshot.Attestation.Resolution)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	findingDigest, err := contract.ComputeFindingSetDigestV1(snapshot.Findings)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	reasons := append([]string(nil), snapshot.Attestation.ReasonCodes...)
	sort.Strings(reasons)
	verdict, err := contract.SealVerdictV1(contract.VerdictV1{
		FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: command.TenantID, ID: command.VerdictID, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		CaseID:         snapshot.Case.ID, CaseRevision: snapshot.Case.Revision, CaseDigest: snapshot.Case.Digest, TargetID: snapshot.Target.ID, TargetRevision: snapshot.Target.Revision, TargetDigest: snapshot.Target.Digest,
		PayloadRevision: snapshot.Target.PayloadRevision, PayloadDigest: snapshot.Target.PayloadDigest, Scope: snapshot.Target.Scope, ActionScopeDigest: snapshot.Target.ActionScopeDigest, TargetEvidenceSetDigest: snapshot.Target.EvidenceSetDigest, ContextFrameDigest: snapshot.Target.ContextFrameDigest,
		IntentID: snapshot.Target.IntentID, IntentRevision: snapshot.Target.IntentRevision, SubjectDigest: snapshot.Target.SubjectDigest, Policy: snapshot.Target.Policy, ActorAuthority: snapshot.Target.ActorAuthority, ReviewerAuthority: snapshot.Attestation.ReviewerAuthority, CurrentScope: snapshot.Target.CurrentScope,
		RoundID: snapshot.Round.ID, RoundRevision: snapshot.Round.Revision, RoundDigest: snapshot.Round.Digest, AssignmentID: snapshot.Assignment.ID, AssignmentRevision: snapshot.Assignment.Revision, AssignmentDigest: snapshot.Assignment.Digest, ReviewerID: snapshot.Attestation.ReviewerID, ReviewerBinding: snapshot.Attestation.ReviewerBinding,
		State: state, AttestationRefs: []string{snapshot.Attestation.ID}, ReasonCodes: reasons, FindingDigest: findingDigest, EvidenceDigest: snapshot.Attestation.EvidenceDigest, Conditions: append([]runtimeports.ReviewConditionV2(nil), snapshot.Attestation.Conditions...), ConditionsDigest: snapshot.Attestation.ConditionsDigest, ExpiresUnixNano: snapshot.ExpiresUnixNano,
	})
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	resolvedTraces, err := resolvedTracesV2(command.Trace, verdict, snapshot.Case, command.AdditionalTraces)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	mutation := reviewport.DecideMutationV1{Expected: command.Expected, Target: exact(snapshot.Target.FactIdentityV1), Round: exact(snapshot.Round.FactIdentityV1), Assignment: exact(snapshot.Assignment.FactIdentityV1), Attestation: exact(snapshot.Attestation.FactIdentityV1), Rubric: snapshot.Rubric.ExactRef(), SnapshotDigest: snapshot.Digest, RubricCheckedUnixNano: now.UnixNano(), Verdict: verdict, Trace: command.Trace, AdditionalTraces: resolvedTraces}
	for _, finding := range snapshot.Findings {
		mutation.Findings = append(mutation.Findings, exact(finding.FactIdentityV1))
	}
	if snapshot.ApplySettlement != nil {
		ref := exact(snapshot.ApplySettlement.FactIdentityV1)
		mutation.ApplySettlement = &ref
	}
	if snapshot.DomainResult != nil {
		ref := exact(snapshot.DomainResult.FactIdentityV1)
		mutation.DomainResult = &ref
	}
	caseFact, storedVerdict, err := o.store.DecideV1(ctx, mutation)
	if err == nil || !unknownOutcome(err) {
		return caseFact, storedVerdict, err
	}
	recoveryCtx, cancel := o.lostReplyRecoveryContextV1(ctx)
	defer cancel()
	return o.inspectLostDecide(recoveryCtx, snapshot.Case, verdict, append([]contract.TraceFactV1{command.Trace}, resolvedTraces...), err)
}

func (o *Owner) inspectDecideReplay(ctx context.Context, command DecideCommandV1, currentCase contract.ReviewCaseV1) (contract.ReviewCaseV1, contract.VerdictV1, error) {
	if currentCase.VerdictID != command.VerdictID || currentCase.Revision != command.Expected.Revision+1 {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Verdict Decide replay changed exact Case or Verdict identity")
	}
	verdict, err := o.store.InspectVerdictExactV1(ctx, command.TenantID, reviewport.ExactV1(currentCase.VerdictID, currentCase.VerdictRevision, currentCase.VerdictDigest))
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	if verdict.CaseID != command.CaseID || !slices.Equal(verdict.AttestationRefs, []string{command.AttestationID}) {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Verdict Decide replay changed canonical attestation")
	}
	previous, err := o.store.InspectCaseExactV1(ctx, command.TenantID, reviewport.ExactV1(command.CaseID, command.Expected.Revision, command.Expected.Digest))
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	resolvedTraces, err := resolvedTracesV2(command.Trace, verdict, previous, command.AdditionalTraces)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	for _, event := range append([]contract.TraceFactV1{command.Trace}, resolvedTraces...) {
		if _, err := o.store.InspectTraceExactV1(ctx, event.TenantID, exact(event.FactIdentityV1)); err != nil {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
		}
	}
	return currentCase, verdict, nil
}

func resolvedTracesV2(primary contract.TraceFactV1, verdict contract.VerdictV1, previous contract.ReviewCaseV1, supplied []contract.TraceFactV1) ([]contract.TraceFactV1, error) {
	if len(supplied) > 1 {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Verdict resolution requires exactly one Resolved Trace")
	}
	if len(supplied) == 1 {
		if supplied[0].Event != contract.TraceResolvedV1 {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Verdict resolution Trace must be Resolved")
		}
		return append([]contract.TraceFactV1(nil), supplied...), nil
	}
	value := primary
	value.ID = primary.ID + "-resolved"
	value.Digest = ""
	value.CaseRevision = previous.Revision + 1
	value.Event = contract.TraceResolvedV1
	value.SourceID = primary.SourceID + "/resolved/" + verdict.ID
	value.SourceSequence = 1
	value.CausationID = verdict.ID
	value.CorrelationID = previous.ID
	value.FactRefs = []string{verdict.ID}
	sealed, err := contract.SealTraceFactV1(value)
	if err != nil {
		return nil, err
	}
	return []contract.TraceFactV1{sealed}, nil
}

func (o *Owner) inspectLostDecide(ctx context.Context, previous contract.ReviewCaseV1, verdict contract.VerdictV1, events []contract.TraceFactV1, original error) (contract.ReviewCaseV1, contract.VerdictV1, error) {
	storedVerdict, err := o.store.InspectVerdictExactV1(ctx, verdict.TenantID, exact(verdict.FactIdentityV1))
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, original
	}
	resolved := previous
	resolved.Revision++
	resolved.State = contract.CaseResolvedV1
	resolved.VerdictID, resolved.VerdictRevision, resolved.VerdictDigest = verdict.ID, verdict.Revision, verdict.Digest
	resolved.UpdatedUnixNano = verdict.UpdatedUnixNano
	resolved.InvalidationReason = ""
	resolved.Digest = ""
	resolved, err = contract.SealReviewCaseV1(resolved)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	storedCase, err := o.store.InspectCaseExactV1(ctx, resolved.TenantID, exact(resolved.FactIdentityV1))
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, original
	}
	for _, event := range events {
		if _, err = o.store.InspectTraceExactV1(ctx, event.TenantID, exact(event.FactIdentityV1)); err != nil {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, original
		}
	}
	return storedCase, storedVerdict, nil
}

func validateDecisionSnapshot(snapshot contract.DecisionCurrentSnapshotV1, command DecideCommandV1, now time.Time) error {
	if err := snapshot.ValidateEnvelope(now); err != nil {
		return err
	}
	if snapshot.Case.TenantID != command.TenantID || snapshot.Case.ID != command.CaseID || snapshot.Case.Revision != command.Expected.Revision || snapshot.Case.Digest != command.Expected.Digest || snapshot.Attestation.ID != command.AttestationID {
		return stale("decision snapshot does not match exact command")
	}
	for _, validate := range []func() error{snapshot.Target.Validate, snapshot.Case.Validate, snapshot.Round.Validate, snapshot.Rubric.Validate, snapshot.Assignment.Validate, snapshot.Attestation.ValidateProductionAutoProvenanceV4} {
		if err := validate(); err != nil {
			return err
		}
	}
	if now.IsZero() || now.UnixNano() < snapshot.Target.UpdatedUnixNano || now.UnixNano() < snapshot.Case.UpdatedUnixNano || now.UnixNano() < snapshot.Round.UpdatedUnixNano || now.UnixNano() < snapshot.Assignment.UpdatedUnixNano || now.UnixNano() < snapshot.Attestation.ObservedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Verdict Decide clock regressed behind inspected facts")
	}
	if snapshot.Case.State != contract.CaseAttestedV1 && snapshot.Case.State != contract.CaseDecidingV1 {
		return stale("decision Case is not current for Decide")
	}
	if snapshot.Case.Revision != snapshot.Attestation.CaseRevision+1 || snapshot.Case.TargetID != snapshot.Target.ID || snapshot.Case.TargetRevision != snapshot.Target.Revision || snapshot.Case.TargetDigest != snapshot.Target.Digest || snapshot.Case.CurrentRoundID != snapshot.Round.ID || snapshot.Case.CurrentAssignment != snapshot.Assignment.ID {
		return stale("Target, Case or Attestation revision chain drifted")
	}
	if snapshot.Case.Rubric == nil || snapshot.Round.Rubric == nil || *snapshot.Case.Rubric != snapshot.Rubric.ExactRef() || *snapshot.Round.Rubric != snapshot.Rubric.ExactRef() || snapshot.Round.RubricDigest != snapshot.Rubric.Digest || snapshot.Round.CaseID != snapshot.Case.ID || snapshot.Round.TargetID != snapshot.Target.ID || snapshot.Round.TargetRevision != snapshot.Target.Revision || snapshot.Round.TargetDigest != snapshot.Target.Digest || snapshot.Round.AssignmentID != snapshot.Assignment.ID {
		return stale("Round subject chain drifted")
	}
	if err := snapshot.Rubric.ValidateCurrent(*snapshot.Round.Rubric, now); err != nil {
		return err
	}
	if snapshot.Assignment.State != contract.AssignmentClaimedV1 || snapshot.Assignment.CaseID != snapshot.Case.ID || snapshot.Assignment.CaseRevision != snapshot.Round.CaseRevision || snapshot.Assignment.RoundID != snapshot.Round.ID || snapshot.Assignment.RoundRevision != snapshot.Round.Revision || snapshot.Assignment.RoundDigest != snapshot.Round.Digest || snapshot.Assignment.TargetID != snapshot.Target.ID || snapshot.Assignment.TargetRevision != snapshot.Target.Revision || snapshot.Assignment.TargetDigest != snapshot.Target.Digest {
		return stale("Assignment subject chain drifted")
	}
	attestation := snapshot.Attestation
	if attestation.CaseID != snapshot.Case.ID || attestation.RoundID != snapshot.Round.ID || attestation.RoundRevision != snapshot.Round.Revision || attestation.RoundDigest != snapshot.Round.Digest || attestation.AssignmentID != snapshot.Assignment.ID || attestation.AssignmentRevision != snapshot.Assignment.Revision || attestation.AssignmentDigest != snapshot.Assignment.Digest || attestation.TargetID != snapshot.Target.ID || attestation.TargetRevision != snapshot.Target.Revision || attestation.TargetDigest != snapshot.Target.Digest || attestation.ReviewerID != snapshot.Assignment.ReviewerID || attestation.ReviewerAuthority != snapshot.Assignment.ReviewerAuthority || attestation.ReviewerBinding != snapshot.Assignment.ReviewerBinding {
		return stale("Attestation subject or reviewer chain drifted")
	}
	for _, condition := range attestation.Conditions {
		if condition.ScopeDigest != snapshot.Target.ActionScopeDigest || condition.ExpiresUnixNano <= now.UnixNano() || attestation.ExpiresUnixNano > condition.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "Attestation condition Scope or TTL drifted")
		}
	}
	if now.UnixNano() >= snapshot.Target.ExpiresUnixNano || now.UnixNano() >= snapshot.Case.ExpiresUnixNano || now.UnixNano() >= snapshot.Round.ExpiresUnixNano || now.UnixNano() >= snapshot.Rubric.ExpiresUnixNano || now.UnixNano() >= snapshot.Assignment.ExpiresUnixNano || now.UnixNano() >= snapshot.Assignment.LeaseExpiresUnixNano || now.UnixNano() >= attestation.ExpiresUnixNano {
		return stale("Review decision input expired")
	}
	for _, finding := range snapshot.Findings {
		if err := finding.Validate(); err != nil {
			return err
		}
		if finding.CaseID != snapshot.Case.ID || finding.CaseRevision != attestation.CaseRevision || finding.RoundID != snapshot.Round.ID || finding.RoundRevision != snapshot.Round.Revision || finding.RoundDigest != snapshot.Round.Digest || finding.TargetID != snapshot.Target.ID || finding.TargetRevision != snapshot.Target.Revision || finding.TargetDigest != snapshot.Target.Digest || now.UnixNano() >= finding.ExpiresUnixNano {
			return stale("Finding subject or TTL drifted")
		}
	}
	if err := validateAutoSettlement(snapshot); err != nil {
		return err
	}
	if err := validateExternalCurrent(snapshot, now); err != nil {
		return err
	}
	minimum := decisionMinimumExpiry(snapshot)
	if minimum <= now.UnixNano() || snapshot.ExpiresUnixNano != minimum {
		return stale("Verdict TTL is not the shortest current input")
	}
	return nil
}

func validateAutoSettlement(snapshot contract.DecisionCurrentSnapshotV1) error {
	a := snapshot.Attestation
	if a.Route != contract.RouteAutoV1 {
		if snapshot.ApplySettlement != nil || snapshot.DomainResult != nil {
			return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "human decision snapshot contains auto settlement")
		}
		return nil
	}
	if snapshot.ApplySettlement == nil || snapshot.DomainResult == nil || a.DomainApplySettlement == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "auto decision lacks stored settlement chain")
	}
	apply, result := *snapshot.ApplySettlement, *snapshot.DomainResult
	if err := apply.Validate(); err != nil {
		return err
	}
	if err := result.Validate(); err != nil {
		return err
	}
	ref := a.DomainApplySettlement
	if apply.State != contract.DomainApplyAppliedV1 || ref.State != contract.DomainApplyAppliedV1 || apply.Ref() != *ref || apply.DomainResultID != result.ID || apply.DomainResultDigest != result.Digest || result.TenantID != snapshot.Case.TenantID || result.CaseID != snapshot.Case.ID || result.CaseRevision != a.CaseRevision || result.RoundID != snapshot.Round.ID || result.RoundRevision != snapshot.Round.Revision || result.RoundDigest != snapshot.Round.Digest || result.AssignmentID != snapshot.Assignment.ID || result.AssignmentRevision != snapshot.Assignment.Revision || result.AssignmentDigest != snapshot.Assignment.Digest || result.TargetID != snapshot.Target.ID || result.TargetRevision != snapshot.Target.Revision || result.TargetDigest != snapshot.Target.Digest || result.AttemptID != a.ReviewerAttemptID || result.ResultDigest != a.ReviewerResultDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "auto settlement/result exact binding is non-applied or drifted")
	}
	return nil
}

func validateExternalCurrent(snapshot contract.DecisionCurrentSnapshotV1, now time.Time) error {
	if snapshot.ExternalProof != nil {
		if err := snapshot.ExternalProof.Validate(); err != nil {
			return err
		}
		if snapshot.Binding.ProjectionRef != snapshot.ExternalProof.Binding || snapshot.Binding.ProjectionDigest != snapshot.ExternalProof.Binding.Digest {
			return stale("Binding exact-current proof drifted from the external cut")
		}
	}
	p := snapshot.Policy
	if !p.Active || p.ExpiresUnixNano <= now.UnixNano() || p.PolicyDecisionRef == "" {
		return stale("Review Policy is inactive or expired")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Policy digest drifted")
	}
	if p.Ref != snapshot.Target.Policy.Ref || p.Revision != snapshot.Target.Policy.Revision || p.Digest != snapshot.Target.Policy.Digest || p.Scope != snapshot.Target.Scope || p.RunID != snapshot.Target.RunID || p.CurrentScope != snapshot.Target.CurrentScope || p.ActorAuthorityRef != snapshot.Target.ActorAuthority.Ref || p.ReviewerAuthorityRef != snapshot.Assignment.ReviewerAuthority.Ref {
		return stale("Review Policy exact binding drifted")
	}
	for _, ref := range []runtimeports.OperationGovernanceFactRefV3{snapshot.ActorAuthority, snapshot.ReviewerAuthority, snapshot.Scope} {
		if err := ref.Validate(now); err != nil {
			return err
		}
	}
	if snapshot.ActorAuthority.Ref != snapshot.Target.ActorAuthority.Ref || snapshot.ActorAuthority.Revision != snapshot.Target.ActorAuthority.Revision || snapshot.ActorAuthority.Digest != snapshot.Target.ActorAuthority.Digest || snapshot.ReviewerAuthority.Ref != snapshot.Assignment.ReviewerAuthority.Ref || snapshot.ReviewerAuthority.Revision != snapshot.Assignment.ReviewerAuthority.Revision || snapshot.ReviewerAuthority.Digest != snapshot.Assignment.ReviewerAuthority.Digest || snapshot.Scope.Ref != snapshot.Target.CurrentScope.Ref || snapshot.Scope.Revision != snapshot.Target.CurrentScope.Revision || snapshot.Scope.Digest != snapshot.Target.CurrentScope.Digest {
		return stale("Authority or Scope current ref drifted")
	}
	if err := snapshot.Binding.Validate(snapshot.Assignment.ReviewerBinding, now); err != nil {
		return err
	}
	expected, err := expectedEvidence(snapshot)
	if err != nil {
		return err
	}
	if len(expected) != len(snapshot.Evidence) {
		return stale("Evidence current set is incomplete")
	}
	byRef := make(map[string]contract.DecisionEvidenceCurrentV1, len(snapshot.Evidence))
	for _, current := range snapshot.Evidence {
		if err := current.Validate(now); err != nil {
			return err
		}
		if snapshot.ExternalProof != nil && current.ApplicabilityRef == (runtimeports.ReviewEvidenceApplicabilityRefV1{}) {
			return stale("production Evidence lacks an exact applicability proof")
		}
		if _, ok := byRef[current.Review.Ref]; ok {
			return stale("Evidence current set is duplicated")
		}
		byRef[current.Review.Ref] = current
	}
	for _, evidence := range expected {
		current, ok := byRef[evidence.Ref]
		if !ok || current.Review != evidence {
			return stale("Evidence current exact ref drifted")
		}
	}
	return nil
}

func expectedEvidence(snapshot contract.DecisionCurrentSnapshotV1) ([]runtimeports.ReviewEvidenceRefV2, error) {
	byRef := make(map[string]runtimeports.ReviewEvidenceRefV2)
	add := func(value runtimeports.ReviewEvidenceRefV2) error {
		if old, ok := byRef[value.Ref]; ok && old != value {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "same evidence ref changed content")
		}
		byRef[value.Ref] = value
		return nil
	}
	for _, value := range snapshot.Target.Evidence {
		if err := add(value); err != nil {
			return nil, err
		}
	}
	for _, value := range snapshot.Attestation.Evidence {
		if err := add(value); err != nil {
			return nil, err
		}
	}
	for _, finding := range snapshot.Findings {
		for _, value := range finding.Evidence {
			if err := add(value); err != nil {
				return nil, err
			}
		}
	}
	out := make([]runtimeports.ReviewEvidenceRefV2, 0, len(byRef))
	for _, value := range byRef {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out, nil
}

func decisionMinimumExpiry(snapshot contract.DecisionCurrentSnapshotV1) int64 {
	values := []int64{snapshot.Target.ExpiresUnixNano, snapshot.Case.ExpiresUnixNano, snapshot.Round.ExpiresUnixNano, snapshot.Rubric.ExpiresUnixNano, snapshot.Assignment.ExpiresUnixNano, snapshot.Assignment.LeaseExpiresUnixNano, snapshot.Attestation.ExpiresUnixNano, snapshot.Policy.ExpiresUnixNano, snapshot.ActorAuthority.ExpiresUnixNano, snapshot.ReviewerAuthority.ExpiresUnixNano, snapshot.Scope.ExpiresUnixNano, snapshot.Binding.ExpiresUnixNano}
	for _, finding := range snapshot.Findings {
		values = append(values, finding.ExpiresUnixNano)
	}
	for _, evidence := range snapshot.Evidence {
		values = append(values, evidence.ExpiresUnixNano)
	}
	for _, condition := range snapshot.Attestation.Conditions {
		values = append(values, condition.ExpiresUnixNano)
	}
	minimum := int64(0)
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

func verdictStateFor(resolution contract.ResolutionV1) (contract.VerdictStateV1, error) {
	switch resolution {
	case contract.ResolutionAcceptV1:
		return contract.VerdictAcceptedV1, nil
	case contract.ResolutionRejectV1:
		return contract.VerdictRejectedV1, nil
	case contract.ResolutionConditionalV1:
		return contract.VerdictConditionalV1, nil
	default:
		return "", core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "non-authorizing resolution cannot create Verdict")
	}
}

func (o *Owner) InspectCurrentV1(ctx context.Context, tenant core.TenantID, id string) (contract.VerdictV1, error) {
	verdict, err := o.store.InspectVerdictV1(ctx, tenant, id)
	if err != nil {
		return contract.VerdictV1{}, err
	}
	caseFact, err := o.store.InspectCaseV1(ctx, tenant, verdict.CaseID)
	if err != nil {
		return contract.VerdictV1{}, err
	}
	if caseFact.State != contract.CaseResolvedV1 || caseFact.VerdictID != verdict.ID || caseFact.VerdictRevision != verdict.Revision || caseFact.VerdictDigest != verdict.Digest {
		return contract.VerdictV1{}, stale("Verdict is not the exact current Case resolution")
	}
	snapshot, err := o.current.InspectDecisionCurrentV1(ctx, reviewport.DecisionCurrentRequestV1{TenantID: tenant, CaseID: caseFact.ID, ExpectedCase: reviewport.ExpectedV1(verdict.CaseRevision, verdict.CaseDigest), AttestationID: verdict.AttestationRefs[0]})
	if err != nil {
		return contract.VerdictV1{}, err
	}
	now := o.clock()
	command := DecideCommandV1{TenantID: tenant, CaseID: verdict.CaseID, Expected: reviewport.ExpectedV1(snapshot.Case.Revision, snapshot.Case.Digest), AttestationID: verdict.AttestationRefs[0], VerdictID: verdict.ID}
	if err := validateDecisionSnapshot(snapshot, command, now); err != nil {
		return contract.VerdictV1{}, err
	}
	if verdict.ExpiresUnixNano != snapshot.ExpiresUnixNano || verdict.ReviewerID != snapshot.Attestation.ReviewerID || verdict.ReviewerBinding != snapshot.Attestation.ReviewerBinding {
		return contract.VerdictV1{}, stale("Verdict current snapshot drifted")
	}
	return verdict, nil
}

func (o *Owner) InvalidateV1(ctx context.Context, tenant core.TenantID, caseID string, expected reviewport.ExpectedFactV1, caseState contract.CaseStateV1, verdictState contract.VerdictStateV1, reason core.ReasonCode, trace contract.TraceFactV1) (contract.ReviewCaseV1, *contract.VerdictV1, error) {
	previous, err := o.store.InspectCaseExactV1(ctx, tenant, reviewport.ExactV1(caseID, expected.Revision, expected.Digest))
	if err != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	now := o.clock()
	if now.IsZero() {
		return contract.ReviewCaseV1{}, nil, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "invalidation clock is unavailable")
	}
	expectedCase := previous
	expectedCase.Revision++
	expectedCase.State = caseState
	expectedCase.VerdictID, expectedCase.VerdictRevision, expectedCase.VerdictDigest = "", 0, ""
	expectedCase.UpdatedUnixNano = now.UnixNano()
	expectedCase.InvalidationReason = reason
	expectedCase.Digest = ""
	expectedCase, err = contract.SealReviewCaseV1(expectedCase)
	if err != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	var expectedVerdict *contract.VerdictV1
	if previous.VerdictID != "" {
		old, err := o.store.InspectVerdictExactV1(ctx, tenant, reviewport.ExactV1(previous.VerdictID, previous.VerdictRevision, previous.VerdictDigest))
		if err != nil {
			return contract.ReviewCaseV1{}, nil, err
		}
		next := old
		next.Revision++
		next.State = verdictState
		next.UpdatedUnixNano = now.UnixNano()
		next.InvalidationReason = reason
		next.Digest = ""
		next, err = contract.SealVerdictV1(next)
		if err != nil {
			return contract.ReviewCaseV1{}, nil, err
		}
		expectedVerdict = &next
	}
	caseFact, verdict, err := o.store.InvalidateV1(ctx, reviewport.InvalidateMutationV1{TenantID: tenant, Expected: expected, CaseID: caseID, CaseState: caseState, VerdictState: verdictState, Reason: reason, UpdatedUnixNano: now.UnixNano(), Trace: trace})
	if err == nil || !unknownOutcome(err) {
		return caseFact, verdict, err
	}
	// The mutation is never repeated. Recovery is a bounded detached exact
	// Inspect of the expected Case, optional Verdict and mandatory Trace.
	recoveryCtx, cancel := o.lostReplyRecoveryContextV1(ctx)
	defer cancel()
	current, inspectErr := o.store.InspectCaseExactV1(recoveryCtx, tenant, exact(expectedCase.FactIdentityV1))
	if inspectErr != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	if _, inspectErr = o.store.InspectTraceExactV1(recoveryCtx, trace.TenantID, exact(trace.FactIdentityV1)); inspectErr != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	if expectedVerdict == nil {
		return current, nil, nil
	}
	inspectedVerdict, inspectErr := o.store.InspectVerdictExactV1(recoveryCtx, tenant, exact(expectedVerdict.FactIdentityV1))
	if inspectErr != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	return current, &inspectedVerdict, nil
}

func (o *Owner) lostReplyRecoveryContextV1(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := o.recoveryTimeout
	if timeout <= 0 || timeout > lostReplyRecoveryTimeoutV1 {
		timeout = lostReplyRecoveryTimeoutV1
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}
func (o *Owner) RevokeV1(ctx context.Context, tenant core.TenantID, caseID string, expected reviewport.ExpectedFactV1, reason core.ReasonCode, trace contract.TraceFactV1) (contract.ReviewCaseV1, *contract.VerdictV1, error) {
	return o.InvalidateV1(ctx, tenant, caseID, expected, contract.CaseRevokedV1, contract.VerdictRevokedV1, reason, trace)
}
func (o *Owner) SupersedeV1(ctx context.Context, tenant core.TenantID, caseID string, expected reviewport.ExpectedFactV1, reason core.ReasonCode, trace contract.TraceFactV1) (contract.ReviewCaseV1, *contract.VerdictV1, error) {
	return o.InvalidateV1(ctx, tenant, caseID, expected, contract.CaseSupersededV1, contract.VerdictSupersededV1, reason, trace)
}
func (o *Owner) ExpireV1(ctx context.Context, tenant core.TenantID, caseID string, expected reviewport.ExpectedFactV1, reason core.ReasonCode, trace contract.TraceFactV1) (contract.ReviewCaseV1, *contract.VerdictV1, error) {
	current, err := o.store.InspectCaseV1(ctx, tenant, caseID)
	if err != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	deadline := current.ExpiresUnixNano
	if current.VerdictID != "" {
		verdict, err := o.store.InspectVerdictExactV1(ctx, tenant, reviewport.ExactV1(current.VerdictID, current.VerdictRevision, current.VerdictDigest))
		if err != nil {
			return contract.ReviewCaseV1{}, nil, err
		}
		if verdict.ExpiresUnixNano < deadline {
			deadline = verdict.ExpiresUnixNano
		}
	}
	now := o.clock()
	if now.IsZero() || now.UnixNano() < deadline {
		return contract.ReviewCaseV1{}, nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "case has not expired")
	}
	return o.InvalidateV1(ctx, tenant, caseID, expected, contract.CaseExpiredV1, contract.VerdictExpiredV1, reason, trace)
}

func exact(fact contract.FactIdentityV1) reviewport.ExactFactRefV1 {
	return reviewport.ExactV1(fact.ID, fact.Revision, fact.Digest)
}
func unknownOutcome(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}
func stale(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, message)
}
