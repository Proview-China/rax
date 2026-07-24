// Package bypassowner owns the independent Review BypassDecision lifecycle.
// It never creates a Verdict, Attestation, Runtime Authorization or Permit.
package bypassowner

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type Clock func() time.Time

// ExternalCurrentCutV1 must perform Policy/Authority/Scope/Binding/Evidence
// S1 -> exact Inspect -> S2 and return the immutable exact read receipt. It is
// read-only and has no external Owner mutation capability.
type ExternalCurrentCutV1 interface {
	ReadBypassCurrentV1(context.Context, contract.BypassDecisionV1, time.Time) (contract.BypassExternalCurrentProofV1, error)
}

type storeV1 interface {
	reviewport.StoreV1
	reviewport.BypassStoreV1
}

type Owner struct {
	store           storeV1
	external        ExternalCurrentCutV1
	clock           Clock
	recoveryTimeout time.Duration
}

const lostReplyRecoveryTimeoutV1 = 5 * time.Second

func New(store storeV1, external ExternalCurrentCutV1, clock Clock) (*Owner, error) {
	if nilcheck.IsNil(store) || nilcheck.IsNil(external) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Bypass Owner requires Store, external-current cut and clock")
	}
	return &Owner{store: store, external: external, clock: clock, recoveryTimeout: lostReplyRecoveryTimeoutV1}, nil
}

func (o *Owner) CreateV1(ctx context.Context, mutation reviewport.CreateBypassDecisionMutationV1) (contract.BypassDecisionV1, error) {
	baseline := o.clock()
	if baseline.IsZero() {
		return contract.BypassDecisionV1{}, clockError()
	}
	decision := mutation.Decision
	if err := decision.Validate(); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	target, err := o.store.InspectTargetExactV1(ctx, decision.TenantID, reviewport.ExactV1(decision.Target.ID, decision.Target.Revision, decision.Target.Digest))
	if err != nil {
		return contract.BypassDecisionV1{}, err
	}
	reviewCase, err := o.store.InspectCaseExactV1(ctx, decision.TenantID, reviewport.ExactV1(decision.Case.ID, decision.Case.Revision, decision.Case.Digest))
	if err != nil {
		return contract.BypassDecisionV1{}, err
	}
	if target.BypassExactRefV1() != decision.Target || reviewCase.BypassExactRefV1() != decision.Case || reviewCase.TargetID != target.ID || reviewCase.TargetRevision != target.Revision || reviewCase.TargetDigest != target.Digest || reviewCase.State != contract.CaseRoutedV1 {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Bypass Decision does not bind the exact routed Case and Target")
	}
	if decision.PayloadRevision != target.PayloadRevision || decision.PayloadDigest != target.PayloadDigest || decision.Scope != target.Scope || decision.RunID != target.RunID || decision.ActionScopeDigest != target.ActionScopeDigest || decision.IntentID != target.IntentID || decision.IntentRevision != target.IntentRevision || decision.SubjectDigest != target.SubjectDigest || decision.Policy != target.Policy || decision.ActorAuthority != target.ActorAuthority || decision.CurrentScope != target.CurrentScope || decision.TargetEvidenceSetDigest != target.EvidenceSetDigest {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Bypass Decision target fields drifted")
	}
	proof, err := o.external.ReadBypassCurrentV1(ctx, decision, baseline)
	if err != nil {
		return contract.BypassDecisionV1{}, err
	}
	if err := proof.ValidateCurrent(baseline); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	if proof.Digest != decision.ExternalProof.Digest || proof.Policy != decision.PolicyCurrentProjection {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Bypass external-current proof drifted")
	}
	wantExpiry := minPositive(target.ExpiresUnixNano, reviewCase.ExpiresUnixNano, proof.ExpiresUnixNano)
	if decision.ExpiresUnixNano != wantExpiry {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Bypass Decision expiry is not the exact current-input minimum")
	}
	now := o.clock()
	if now.IsZero() || now.Before(baseline) {
		return contract.BypassDecisionV1{}, clockError()
	}
	if err := proof.ValidateCurrent(now); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	if err := target.ValidateCurrent(contract.TargetCurrentnessV1{
		TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest,
		PayloadRevision: target.PayloadRevision, PayloadDigest: target.PayloadDigest,
		Scope: target.Scope, ActionScopeDigest: target.ActionScopeDigest, Policy: target.Policy,
		ActorAuthority: target.ActorAuthority, CurrentScope: target.CurrentScope,
		EvidenceSetDigest: target.EvidenceSetDigest, ContextFrameDigest: target.ContextFrameDigest, Now: now,
	}); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	created, err := o.store.CreateBypassDecisionV1(ctx, mutation)
	if err == nil || (!core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorUnavailable)) {
		return created, err
	}
	detached, cancel := o.lostReplyRecoveryContextV1(ctx, now, decision.ExpiresUnixNano)
	defer cancel()
	recovered, inspectErr := o.store.InspectBypassDecisionExactV1(detached, decision.ExactRef())
	if inspectErr != nil {
		return contract.BypassDecisionV1{}, err
	}
	if !reflect.DeepEqual(recovered, decision) {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Bypass Decision lost-reply recovery found different content")
	}
	if mutation.Trace.ID != "" {
		if _, inspectErr := o.store.InspectTraceExactV1(detached, mutation.Trace.TenantID, reviewport.ExactV1(mutation.Trace.ID, mutation.Trace.Revision, mutation.Trace.Digest)); inspectErr != nil {
			return contract.BypassDecisionV1{}, err
		}
	}
	now = o.clock()
	if now.IsZero() || now.Before(baseline) {
		return contract.BypassDecisionV1{}, clockError()
	}
	if err := recovered.ValidateCurrent(decision.Target, decision.Case, decision.PolicyCurrentProjection, now); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	return recovered, nil
}

func (o *Owner) lostReplyRecoveryContextV1(ctx context.Context, now time.Time, expiresUnixNano int64) (context.Context, context.CancelFunc) {
	timeout := o.recoveryTimeout
	if timeout <= 0 || timeout > lostReplyRecoveryTimeoutV1 {
		timeout = lostReplyRecoveryTimeoutV1
	}
	if expiresUnixNano > 0 {
		if remaining := time.Unix(0, expiresUnixNano).Sub(now); remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

func minPositive(values ...int64) int64 {
	minimum := int64(0)
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

func clockError() error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Bypass Owner clock is zero or regressed")
}
