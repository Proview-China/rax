// Package settlementowner applies Runtime-owned settlement truth to the
// Review domain. It never writes a Runtime fact or interprets Provider output
// as an Attestation/Verdict.
package settlementowner

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type StoreV1 interface {
	InspectAutoReviewerAttemptExactV1(context.Context, core.TenantID, contract.ExactResourceRefV1) (contract.AutoReviewerAttemptV1, error)
	InspectAutoReviewerObservationExactV1(context.Context, core.TenantID, contract.AutoReviewerInvocationObservationRefV1) (contract.AutoReviewerInvocationObservationV1, error)
	InspectDomainResultExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.ReviewerInvocationResultFactV1, error)
	CreateApplySettlementV1(context.Context, contract.DomainApplySettlementFactV1) (contract.DomainApplySettlementFactV1, error)
	InspectApplySettlementExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.DomainApplySettlementFactV1, error)
}

type RuntimeSettlementCurrentReaderV4 interface {
	InspectCurrentOperationSettlementV4(context.Context, runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error)
}

type OwnerV1 struct {
	store           StoreV1
	runtime         RuntimeSettlementCurrentReaderV4
	now             func() time.Time
	recoveryTimeout time.Duration
}

const lostReplyRecoveryTimeoutV1 = 5 * time.Second

func NewV1(store StoreV1, runtime RuntimeSettlementCurrentReaderV4, now func() time.Time) (*OwnerV1, error) {
	if nilcheck.IsNil(store) || nilcheck.IsNil(runtime) || now == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "auto reviewer settlement Owner dependencies are required")
	}
	return &OwnerV1{store: store, runtime: runtime, now: now, recoveryTimeout: lostReplyRecoveryTimeoutV1}, nil
}

type ApplyCommandV1 struct {
	TenantID     core.TenantID               `json:"tenant_id"`
	Attempt      contract.ExactResourceRefV1 `json:"attempt"`
	DomainResult contract.ExactResourceRefV1 `json:"domain_result"`
	ApplyID      string                      `json:"apply_id"`
}

func (o *OwnerV1) ApplyV1(ctx context.Context, command ApplyCommandV1) (contract.DomainApplySettlementFactV1, error) {
	if o == nil || nilcheck.IsNil(o.store) || nilcheck.IsNil(o.runtime) || o.now == nil || ctx == nil || command.TenantID == "" || command.ApplyID == "" {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "auto reviewer settlement Apply command is incomplete")
	}
	if err := command.Attempt.Validate(); err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	if err := command.DomainResult.Validate(); err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	baseline := o.now()
	if baseline.IsZero() || baseline.UnixNano() <= 0 {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "auto reviewer settlement baseline clock is invalid")
	}
	attempt, err := o.store.InspectAutoReviewerAttemptExactV1(ctx, command.TenantID, command.Attempt)
	if err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	if attempt.State != contract.AutoReviewerAttemptObservedV1 || attempt.DomainResult == nil || attempt.Observation == nil || *attempt.DomainResult != command.DomainResult {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "auto reviewer Attempt has no exact observed DomainResult")
	}
	if attempt.InvocationAttempt == nil {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "auto reviewer Attempt has no exact invocation source")
	}
	invocationAttempt, err := o.store.InspectAutoReviewerAttemptExactV1(ctx, command.TenantID, *attempt.InvocationAttempt)
	if err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	if invocationAttempt.ExactRef() != *attempt.InvocationAttempt || invocationAttempt.State != contract.AutoReviewerAttemptPreparedV1 || !sameAttemptSubjectV1(invocationAttempt, attempt) {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer original invocation Attempt history drifted")
	}
	observation, err := o.store.InspectAutoReviewerObservationExactV1(ctx, command.TenantID, *attempt.Observation)
	if err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	if attempt.InvocationAttempt == nil || observation.TenantID != attempt.TenantID || observation.AttemptID != attempt.ID || observation.AttemptRevision != attempt.InvocationAttempt.Revision || observation.AttemptDigest != attempt.InvocationAttempt.Digest || observation.OperationDigest != attempt.OperationDigest || observation.RuntimeAttempt.OperationDigest != attempt.OperationDigest || observation.RuntimeAttempt.EffectID != attempt.InvocationEffect.EffectID || observation.RuntimeAttempt.IntentRevision != attempt.InvocationEffect.EffectRevision || observation.RuntimeAttempt.Delegation == nil || *observation.RuntimeAttempt.Delegation != observation.ProviderObservation.Delegation {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer Observation drifted from the exact Attempt history")
	}
	result, err := o.store.InspectDomainResultExactV1(ctx, command.TenantID, reviewport.ExactV1(command.DomainResult.ID, command.DomainResult.Revision, command.DomainResult.Digest))
	if err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	if result.TenantID != attempt.TenantID || result.CaseID != attempt.Case.ID || result.CaseRevision != attempt.Case.Revision || result.RoundID != attempt.Round.ID || result.RoundRevision != attempt.Round.Revision || result.RoundDigest != attempt.Round.Digest || result.AssignmentID != attempt.Assignment.ID || result.AssignmentRevision != attempt.Assignment.Revision || result.AssignmentDigest != attempt.Assignment.Digest || result.TargetID != attempt.Target.ID || result.TargetRevision != attempt.Target.Revision || result.TargetDigest != attempt.Target.Digest || result.AttemptID != observation.RuntimeAttempt.AttemptID || result.ResultSchema != attempt.ResultSchema || result.ResultDigest != observation.Output.Digest || len(result.ObservationRefs) != 1 || result.ObservationRefs[0] != observation.ID {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer DomainResult drifted from its exact Observation")
	}
	inspection, err := o.runtime.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: attempt.Operation, EffectID: attempt.InvocationEffect.EffectID})
	if err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	fresh := o.now()
	if fresh.IsZero() || fresh.Before(baseline) {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "auto reviewer settlement clock regressed during current Inspect")
	}
	if err := attempt.ValidateCurrent(fresh); err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	if err := inspection.Validate(fresh); err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	if inspection.Settlement.OperationDigest != attempt.OperationDigest || inspection.Settlement.EffectID != attempt.InvocationEffect.EffectID || inspection.DomainResult.EffectID != attempt.InvocationEffect.EffectID || inspection.DomainResult.EffectRevision != attempt.InvocationEffect.EffectRevision || inspection.DomainResult.Owner != attempt.InvocationEffect.Provider || !reflect.DeepEqual(inspection.DomainResult.Attempt, observation.RuntimeAttempt) {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Runtime V4 settlement belongs to another auto reviewer operation")
	}
	apply, err := contract.ApplyRuntimeSettlementV4(command.ApplyID, result, inspection, fresh)
	if err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	created, err := o.store.CreateApplySettlementV1(ctx, apply)
	if err == nil {
		return created, nil
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorUnavailable) {
		return contract.DomainApplySettlementFactV1{}, err
	}
	// A lost mutation reply never replays Create. Recovery is one bounded,
	// detached exact read of the original canonical ApplySettlement.
	recoveryCtx, cancel := o.lostReplyRecoveryContextV1(ctx, fresh, attempt.ExpiresUnixNano, inspection.ExpiresUnixNano)
	defer cancel()
	recovered, inspectErr := o.store.InspectApplySettlementExactV1(recoveryCtx, apply.TenantID, reviewport.ExactV1(apply.ID, apply.Revision, apply.Digest))
	if inspectErr != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	if !reflect.DeepEqual(recovered, apply) {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "auto reviewer ApplySettlement recovery returned different canonical content")
	}
	return recovered, nil
}

func (o *OwnerV1) lostReplyRecoveryContextV1(ctx context.Context, now time.Time, expiries ...int64) (context.Context, context.CancelFunc) {
	timeout := o.recoveryTimeout
	if timeout <= 0 || timeout > lostReplyRecoveryTimeoutV1 {
		timeout = lostReplyRecoveryTimeoutV1
	}
	for _, expiry := range expiries {
		if expiry <= 0 {
			continue
		}
		remaining := time.Unix(0, expiry).Sub(now)
		if remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

func sameAttemptSubjectV1(left, right contract.AutoReviewerAttemptV1) bool {
	return left.TenantID == right.TenantID && left.ID == right.ID && left.IdempotencyKey == right.IdempotencyKey && left.Case == right.Case && left.Round == right.Round && left.Assignment == right.Assignment && left.Target == right.Target && left.Rubric == right.Rubric && left.ContextFrameDigest == right.ContextFrameDigest && left.ReviewerID == right.ReviewerID && left.ReviewerAuthority == right.ReviewerAuthority && left.ReviewerBinding == right.ReviewerBinding && left.RouteID == right.RouteID && reflect.DeepEqual(left.Operation, right.Operation) && left.OperationDigest == right.OperationDigest && left.InvocationEffect == right.InvocationEffect && left.ResultSchema == right.ResultSchema && left.RoundOrdinal == right.RoundOrdinal && left.MaxCostMicros == right.MaxCostMicros && left.CreatedUnixNano == right.CreatedUnixNano && left.ExpiresUnixNano == right.ExpiresUnixNano
}
