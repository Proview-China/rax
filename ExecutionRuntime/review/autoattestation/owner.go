// Package autoattestation turns a truthfully applied auto-reviewer result into
// Review-owned Findings and one Attestation. It does not create Runtime facts,
// dispatch a reviewer, or decide a Verdict.
package autoattestation

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	findingIdentityContractV1  = "praxis.review.auto-attestation-finding/v1"
	lostReplyRecoveryTimeoutV1 = 5 * time.Second
)

type OwnerV1 struct {
	store           ownerStoreV2
	auto            reviewport.AutoReviewerStoreV1
	now             func() time.Time
	recoveryTimeout time.Duration
	mu              sync.Mutex
}

type ownerStoreV2 interface {
	reviewport.StoreV1
	reviewport.TraceEventStoreV2
}

func NewV1(store ownerStoreV2, auto reviewport.AutoReviewerStoreV1, now func() time.Time) (*OwnerV1, error) {
	if nilcheck.IsNil(store) || nilcheck.IsNil(auto) || now == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "auto attestation Owner dependencies are required")
	}
	return &OwnerV1{store: store, auto: auto, now: now, recoveryTimeout: lostReplyRecoveryTimeoutV1}, nil
}

type RecordCommandV1 struct {
	TenantID        core.TenantID                       `json:"tenant_id"`
	Attempt         contract.ExactResourceRefV1         `json:"attempt"`
	ApplySettlement contract.DomainApplySettlementRefV1 `json:"apply_settlement"`
	AttestationID   string                              `json:"attestation_id"`
	IdempotencyKey  string                              `json:"idempotency_key"`
	Trace           contract.TraceFactV1                `json:"trace"`
}

type RecordResultV1 struct {
	Case        contract.ReviewCaseV1  `json:"case"`
	Attestation contract.AttestationV1 `json:"attestation"`
	Findings    []contract.FindingV1   `json:"findings"`
}

type findingIdentityInputV1 struct {
	TenantID     core.TenantID                                   `json:"tenant_id"`
	Case         contract.ExactResourceRefV1                     `json:"case"`
	Round        contract.ExactResourceRefV1                     `json:"round"`
	Target       contract.ExactResourceRefV1                     `json:"target"`
	Attempt      contract.ExactResourceRefV1                     `json:"attempt"`
	Observation  contract.AutoReviewerInvocationObservationRefV1 `json:"observation"`
	OutputDigest core.Digest                                     `json:"output_digest"`
	Ordinal      uint32                                          `json:"ordinal"`
	Draft        contract.AutoFindingDraftV1                     `json:"draft"`
}

// DeterministicFindingIDsV1 lets a caller seal the exact Trace before RecordV1.
// IDs bind the exact applied reviewer output and never depend on a local clock.
func DeterministicFindingIDsV1(tenant core.TenantID, attempt contract.AutoReviewerAttemptV1, observation contract.AutoReviewerInvocationObservationV1) ([]string, error) {
	if tenant == "" || tenant != attempt.TenantID || tenant != observation.TenantID {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "auto Finding identity tenant drifted")
	}
	if err := attempt.Validate(); err != nil {
		return nil, err
	}
	if err := observation.Validate(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(observation.Output.Findings))
	for index, draft := range observation.Output.Findings {
		digest, err := core.CanonicalJSONDigest("praxis.review.auto-attestation", findingIdentityContractV1, "FindingIdentityInputV1", findingIdentityInputV1{
			TenantID: tenant, Case: attempt.Case, Round: attempt.Round, Target: attempt.Target,
			Attempt: attempt.ExactRef(), Observation: observation.Ref(), OutputDigest: observation.Output.Digest,
			Ordinal: uint32(index + 1), Draft: draft,
		})
		if err != nil {
			return nil, err
		}
		ids = append(ids, "auto-finding-"+string(digest))
	}
	sort.Strings(ids)
	return ids, nil
}

func (o *OwnerV1) RecordV1(ctx context.Context, command RecordCommandV1) (RecordResultV1, error) {
	if o == nil || nilcheck.IsNil(o.store) || nilcheck.IsNil(o.auto) || o.now == nil || ctx == nil || command.TenantID == "" || command.AttestationID == "" || command.IdempotencyKey == "" {
		return RecordResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "auto attestation Record command is incomplete")
	}
	if err := command.Attempt.Validate(); err != nil {
		return RecordResultV1{}, err
	}
	if err := command.ApplySettlement.Validate(); err != nil {
		return RecordResultV1{}, err
	}
	if err := command.Trace.Validate(); err != nil {
		return RecordResultV1{}, err
	}

	// Serializing this Owner instance makes one canonical command single-writer.
	// Independent Owners still converge at Store create-once/CAS boundaries.
	o.mu.Lock()
	defer o.mu.Unlock()

	if replay, ok, err := o.inspectReplayV1(ctx, command); err != nil || ok {
		return replay, err
	}
	baseline := o.now()
	if err := validBaselineV1(baseline); err != nil {
		return RecordResultV1{}, err
	}

	attempt, err := o.auto.InspectAutoReviewerAttemptExactV1(ctx, command.TenantID, command.Attempt)
	if err != nil {
		return RecordResultV1{}, err
	}
	currentAttempt, err := o.auto.InspectAutoReviewerAttemptCurrentV1(ctx, command.TenantID, command.Attempt.ID)
	if err != nil {
		return RecordResultV1{}, err
	}
	if attempt.ExactRef() != command.Attempt || currentAttempt.ExactRef() != command.Attempt || attempt.State != contract.AutoReviewerAttemptObservedV1 || attempt.InvocationAttempt == nil || attempt.Observation == nil || attempt.DomainResult == nil {
		return RecordResultV1{}, driftV1("auto attestation requires the exact current observed Attempt")
	}
	original, err := o.auto.InspectAutoReviewerAttemptExactV1(ctx, command.TenantID, *attempt.InvocationAttempt)
	if err != nil {
		return RecordResultV1{}, err
	}
	if original.ExactRef() != *attempt.InvocationAttempt || original.State != contract.AutoReviewerAttemptPreparedV1 || !sameAttemptSubjectV1(original, attempt) {
		return RecordResultV1{}, driftV1("auto attestation original invocation Attempt drifted")
	}
	observation, err := o.auto.InspectAutoReviewerObservationExactV1(ctx, command.TenantID, *attempt.Observation)
	if err != nil {
		return RecordResultV1{}, err
	}
	if observation.Ref() != *attempt.Observation || observation.AttemptID != attempt.ID || observation.AttemptRevision != original.Revision || observation.AttemptDigest != original.Digest || observation.OperationDigest != attempt.OperationDigest || observation.RuntimeAttempt.OperationDigest != attempt.OperationDigest || observation.RuntimeAttempt.EffectID != attempt.InvocationEffect.EffectID || observation.RuntimeAttempt.IntentRevision != attempt.InvocationEffect.EffectRevision || observation.RuntimeAttempt.Delegation == nil || *observation.RuntimeAttempt.Delegation != observation.ProviderObservation.Delegation || observation.ResultSchema != attempt.ResultSchema {
		return RecordResultV1{}, driftV1("auto attestation Observation drifted from its exact Attempt")
	}
	result, err := o.store.InspectDomainResultExactV1(ctx, command.TenantID, reviewport.ExactV1(attempt.DomainResult.ID, attempt.DomainResult.Revision, attempt.DomainResult.Digest))
	if err != nil {
		return RecordResultV1{}, err
	}
	if err := validateResultV1(attempt, observation, result); err != nil {
		return RecordResultV1{}, err
	}
	apply, err := o.store.InspectApplySettlementExactV1(ctx, command.TenantID, reviewport.ExactV1(command.ApplySettlement.ID, command.ApplySettlement.Revision, command.ApplySettlement.Digest))
	if err != nil {
		return RecordResultV1{}, err
	}
	if apply.Ref() != command.ApplySettlement || apply.State != contract.DomainApplyAppliedV1 || apply.RuntimeContractVersion != runtimeports.OperationSettlementContractVersionV4 || apply.DomainResultID != result.ID || apply.DomainResultDigest != result.Digest {
		return RecordResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "auto attestation requires the exact stored applied Runtime V4 settlement")
	}

	target, caseFact, round, assignment, rubric, err := o.inspectReviewCurrentV1(ctx, command.TenantID, attempt, baseline)
	if err != nil {
		return RecordResultV1{}, err
	}
	if err := rubric.ValidateAutoReviewerOutputV1(observation.Output); err != nil {
		return RecordResultV1{}, err
	}
	terminationChecked := o.now()
	if terminationChecked.IsZero() || terminationChecked.Before(baseline) {
		return RecordResultV1{}, clockRegressionV1("auto attestation clock regressed before termination S1")
	}
	terminationRequest := reviewport.AutoReviewTerminationCurrentRequestV1{TenantID: command.TenantID, Target: attempt.Target, Case: attempt.Case, Rubric: attempt.Rubric, ExpectedRound: attempt.Round, CheckedUnixNano: terminationChecked.UnixNano()}
	terminationCurrent, err := o.auto.InspectAutoReviewTerminationCurrentV1(ctx, terminationRequest)
	if err != nil {
		return RecordResultV1{}, err
	}
	postRead := o.now()
	if postRead.IsZero() || postRead.Before(terminationChecked) {
		return RecordResultV1{}, clockRegressionV1("auto attestation clock regressed during S1")
	}
	if err := terminationCurrent.ValidateCurrent(terminationRequest, postRead); err != nil {
		return RecordResultV1{}, err
	}
	minimum, err := validateCurrentCutV1(postRead, target, caseFact, round, assignment, attempt, observation, rubric)
	if err != nil {
		return RecordResultV1{}, err
	}
	if terminationCurrent.ExpiresUnixNano < minimum {
		minimum = terminationCurrent.ExpiresUnixNano
	}
	minimum, err = validateConditionCutV2(observation.Output, target, postRead, minimum)
	if err != nil {
		return RecordResultV1{}, err
	}

	findings, err := buildFindingsV1(attempt, observation, minimum)
	if err != nil {
		return RecordResultV1{}, err
	}
	if err := validateTraceV1(command.Trace, caseFact, attempt, observation, result, apply, rubric, command.AttestationID, findings); err != nil {
		return RecordResultV1{}, err
	}
	findingTraces, err := findingTracesV2(attempt, observation, findings)
	if err != nil {
		return RecordResultV1{}, err
	}
	for index := range findings {
		created, createErr := o.store.CreateFindingWithTraceV2(ctx, reviewport.CreateFindingWithTraceMutationV2{Finding: findings[index], Trace: findingTraces[index]})
		if createErr != nil {
			if !unknownReplyV1(createErr) {
				return RecordResultV1{}, createErr
			}
			originalUnknown := createErr
			recovery, cancel := o.lostReplyRecoveryContextV1(ctx)
			created, createErr = o.store.InspectFindingExactV1(recovery, command.TenantID, reviewport.ExactV1(findings[index].ID, findings[index].Revision, findings[index].Digest))
			if createErr != nil {
				cancel()
				return RecordResultV1{}, originalUnknown
			}
			if _, createErr = o.store.InspectTraceExactV1(recovery, command.TenantID, reviewport.ExactV1(findingTraces[index].ID, findingTraces[index].Revision, findingTraces[index].Digest)); createErr != nil {
				cancel()
				return RecordResultV1{}, originalUnknown
			}
			cancel()
		}
		if !reflect.DeepEqual(created, findings[index]) {
			return RecordResultV1{}, driftV1("auto Finding create returned different canonical content")
		}
	}

	actual := o.now()
	if actual.IsZero() || actual.Before(postRead) {
		return RecordResultV1{}, clockRegressionV1("auto attestation clock regressed before Record")
	}
	minimum, err = validateCurrentCutV1(actual, target, caseFact, round, assignment, attempt, observation, rubric)
	if err != nil {
		return RecordResultV1{}, err
	}
	if err := terminationCurrent.ValidateCurrent(terminationRequest, actual); err != nil {
		return RecordResultV1{}, err
	}
	if terminationCurrent.ExpiresUnixNano < minimum {
		minimum = terminationCurrent.ExpiresUnixNano
	}
	minimum, err = validateConditionCutV2(observation.Output, target, actual, minimum)
	if err != nil {
		return RecordResultV1{}, err
	}
	for index := range findings {
		if findings[index].ExpiresUnixNano != minimum {
			return RecordResultV1{}, driftV1("auto Finding TTL no longer matches the current cut")
		}
	}
	terminationReached := terminationReachedV1(terminationCurrent, rubric, observation.Output)
	attestation, err := buildAttestationV1(command, attempt, observation, result, minimum, actual, findings, terminationReached)
	if err != nil {
		return RecordResultV1{}, err
	}
	nextState, err := nextCaseStateV1(attestation.Resolution)
	if err != nil {
		return RecordResultV1{}, err
	}
	additional, err := escalationTraceV2(command.Trace, attestation, nextState)
	if err != nil {
		return RecordResultV1{}, err
	}
	nextCase, stored, err := o.store.RecordAttestationV1(ctx, reviewport.RecordAttestationMutationV1{
		Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), Attestation: attestation,
		NextState: nextState, Trace: command.Trace, AdditionalTraces: additional, AutoTerminationCurrent: &terminationCurrent, AutoCheckedUnixNano: actual.UnixNano(),
	})
	if err == nil {
		expectedNext, sealErr := expectedNextCaseV1(caseFact, nextState, attestation.ObservedUnixNano)
		if sealErr != nil {
			return RecordResultV1{}, sealErr
		}
		if !reflect.DeepEqual(stored, attestation) || !reflect.DeepEqual(nextCase, expectedNext) {
			return RecordResultV1{}, driftV1("auto attestation mutation returned different canonical facts")
		}
		return RecordResultV1{Case: nextCase, Attestation: stored, Findings: findings}, nil
	}
	if !unknownReplyV1(err) {
		return RecordResultV1{}, err
	}
	return o.recoverRecordV1(ctx, command, caseFact, nextState, attestation, findings, additional, err)
}

func (o *OwnerV1) inspectReplayV1(ctx context.Context, command RecordCommandV1) (RecordResultV1, bool, error) {
	existing, err := o.store.InspectAttestationByIdempotencyV1(ctx, command.TenantID, command.IdempotencyKey)
	if err != nil {
		if core.HasCategory(err, core.ErrorNotFound) {
			return RecordResultV1{}, false, nil
		}
		return RecordResultV1{}, false, err
	}
	if existing.ID != command.AttestationID || existing.AutoProvenance == nil || existing.AutoProvenance.Attempt != command.Attempt || existing.DomainApplySettlement == nil || *existing.DomainApplySettlement != command.ApplySettlement {
		return RecordResultV1{}, true, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "auto attestation replay changed canonical identity")
	}
	if _, err := o.store.InspectTraceExactV1(ctx, command.TenantID, reviewport.ExactV1(command.Trace.ID, command.Trace.Revision, command.Trace.Digest)); err != nil {
		return RecordResultV1{}, true, err
	}
	nextState, err := nextCaseStateV1(existing.Resolution)
	if err != nil {
		return RecordResultV1{}, true, err
	}
	additional, err := escalationTraceV2(command.Trace, existing, nextState)
	if err != nil {
		return RecordResultV1{}, true, err
	}
	for _, event := range additional {
		if _, err := o.store.InspectTraceExactV1(ctx, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); err != nil {
			return RecordResultV1{}, true, err
		}
	}
	attempt, err := o.auto.InspectAutoReviewerAttemptExactV1(ctx, command.TenantID, existing.AutoProvenance.Attempt)
	if err != nil {
		return RecordResultV1{}, true, err
	}
	observation, err := o.auto.InspectAutoReviewerObservationExactV1(ctx, command.TenantID, existing.AutoProvenance.Observation)
	if err != nil {
		return RecordResultV1{}, true, err
	}
	if attempt.DomainResult == nil {
		return RecordResultV1{}, true, driftV1("auto attestation replay lost its exact DomainResult")
	}
	result, err := o.store.InspectDomainResultExactV1(ctx, command.TenantID, reviewport.ExactV1(attempt.DomainResult.ID, attempt.DomainResult.Revision, attempt.DomainResult.Digest))
	if err != nil {
		return RecordResultV1{}, true, err
	}
	apply, err := o.store.InspectApplySettlementExactV1(ctx, command.TenantID, reviewport.ExactV1(command.ApplySettlement.ID, command.ApplySettlement.Revision, command.ApplySettlement.Digest))
	if err != nil {
		return RecordResultV1{}, true, err
	}
	rubric, err := o.store.InspectRubricExactV1(ctx, command.TenantID, existing.AutoProvenance.Rubric)
	if err != nil {
		return RecordResultV1{}, true, err
	}
	historicalCase, err := o.store.InspectCaseExactV1(ctx, command.TenantID, reviewport.ExactV1(attempt.Case.ID, attempt.Case.Revision, attempt.Case.Digest))
	if err != nil {
		return RecordResultV1{}, true, err
	}
	current, err := o.store.InspectCaseV1(ctx, command.TenantID, existing.CaseID)
	if err != nil {
		return RecordResultV1{}, true, err
	}
	findings, err := buildFindingsV1(attempt, observation, existing.ExpiresUnixNano)
	if err != nil {
		return RecordResultV1{}, true, err
	}
	if len(findings) != len(existing.FindingRefs) {
		return RecordResultV1{}, true, driftV1("auto attestation replay Finding set cardinality drifted")
	}
	for index, expected := range findings {
		if expected.ID != existing.FindingRefs[index] {
			return RecordResultV1{}, true, driftV1("auto attestation replay Finding identity drifted")
		}
		value, inspectErr := o.store.InspectFindingExactV1(ctx, command.TenantID, reviewport.ExactV1(expected.ID, expected.Revision, expected.Digest))
		if inspectErr != nil {
			return RecordResultV1{}, true, inspectErr
		}
		if !reflect.DeepEqual(value, expected) {
			return RecordResultV1{}, true, driftV1("auto attestation replay Finding content drifted")
		}
	}
	findingTraces, err := findingTracesV2(attempt, observation, findings)
	if err != nil {
		return RecordResultV1{}, true, err
	}
	for _, event := range findingTraces {
		if _, inspectErr := o.store.InspectTraceExactV1(ctx, command.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); inspectErr != nil {
			return RecordResultV1{}, true, inspectErr
		}
	}
	if err := validateTraceV1(command.Trace, historicalCase, attempt, observation, result, apply, rubric, existing.ID, findings); err != nil {
		return RecordResultV1{}, true, err
	}
	return RecordResultV1{Case: current, Attestation: existing, Findings: findings}, true, nil
}

func (o *OwnerV1) inspectReviewCurrentV1(ctx context.Context, tenant core.TenantID, attempt contract.AutoReviewerAttemptV1, now time.Time) (contract.TargetSnapshotV1, contract.ReviewCaseV1, contract.ReviewRoundV1, contract.ReviewerAssignmentV1, contract.RubricDefinitionV1, error) {
	target, err := o.store.InspectTargetExactV1(ctx, tenant, reviewport.ExactV1(attempt.Target.ID, attempt.Target.Revision, attempt.Target.Digest))
	if err != nil {
		return contract.TargetSnapshotV1{}, contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, contract.RubricDefinitionV1{}, err
	}
	caseExact, err := o.store.InspectCaseExactV1(ctx, tenant, reviewport.ExactV1(attempt.Case.ID, attempt.Case.Revision, attempt.Case.Digest))
	if err != nil {
		return contract.TargetSnapshotV1{}, contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, contract.RubricDefinitionV1{}, err
	}
	caseCurrent, err := o.store.InspectCaseV1(ctx, tenant, attempt.Case.ID)
	if err != nil {
		return contract.TargetSnapshotV1{}, contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, contract.RubricDefinitionV1{}, err
	}
	roundExact, err := o.store.InspectRoundExactV1(ctx, tenant, reviewport.ExactV1(attempt.Round.ID, attempt.Round.Revision, attempt.Round.Digest))
	if err != nil {
		return contract.TargetSnapshotV1{}, contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, contract.RubricDefinitionV1{}, err
	}
	roundCurrent, err := o.store.InspectRoundV1(ctx, tenant, attempt.Round.ID)
	if err != nil {
		return contract.TargetSnapshotV1{}, contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, contract.RubricDefinitionV1{}, err
	}
	assignmentExact, err := o.store.InspectAssignmentExactV1(ctx, tenant, reviewport.ExactV1(attempt.Assignment.ID, attempt.Assignment.Revision, attempt.Assignment.Digest))
	if err != nil {
		return contract.TargetSnapshotV1{}, contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, contract.RubricDefinitionV1{}, err
	}
	assignmentCurrent, err := o.store.InspectAssignmentV1(ctx, tenant, attempt.Assignment.ID)
	if err != nil {
		return contract.TargetSnapshotV1{}, contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, contract.RubricDefinitionV1{}, err
	}
	rubric, err := o.store.InspectRubricCurrentV1(ctx, tenant, attempt.Rubric, now)
	if err != nil {
		return contract.TargetSnapshotV1{}, contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, contract.RubricDefinitionV1{}, err
	}
	if !reflect.DeepEqual(caseExact, caseCurrent) || !reflect.DeepEqual(roundExact, roundCurrent) || !reflect.DeepEqual(assignmentExact, assignmentCurrent) {
		return contract.TargetSnapshotV1{}, contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, contract.RubricDefinitionV1{}, driftV1("auto attestation Review current index drifted")
	}
	if (contract.ExactResourceRefV1{ID: target.ID, Revision: target.Revision, Digest: target.Digest}) != attempt.Target || caseExact.TargetID != target.ID || caseExact.TargetRevision != target.Revision || caseExact.TargetDigest != target.Digest || caseExact.State != contract.CaseReviewingV1 || caseExact.CurrentRoundID != roundExact.ID || caseExact.CurrentAssignment != assignmentExact.ID || roundExact.Route != contract.RouteAutoV1 || assignmentExact.Route != contract.RouteAutoV1 || assignmentExact.State != contract.AssignmentClaimedV1 || roundExact.CaseID != caseExact.ID || assignmentExact.CaseID != caseExact.ID || assignmentExact.CaseRevision != roundExact.CaseRevision || assignmentExact.RoundID != roundExact.ID || assignmentExact.RoundRevision != roundExact.Revision || assignmentExact.RoundDigest != roundExact.Digest || roundExact.TargetID != caseExact.TargetID || roundExact.TargetRevision != caseExact.TargetRevision || roundExact.TargetDigest != caseExact.TargetDigest || assignmentExact.TargetID != caseExact.TargetID || assignmentExact.TargetRevision != caseExact.TargetRevision || assignmentExact.TargetDigest != caseExact.TargetDigest || attempt.ContextFrameDigest != roundExact.ContextFrameDigest || attempt.ReviewerID != assignmentExact.ReviewerID || attempt.ReviewerAuthority != assignmentExact.ReviewerAuthority || attempt.ReviewerBinding != assignmentExact.ReviewerBinding || roundExact.RubricDigest != rubric.Digest {
		return contract.TargetSnapshotV1{}, contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, contract.RubricDefinitionV1{}, driftV1("auto attestation Target/Case/Round/Assignment/Rubric binding drifted")
	}
	return target, caseExact, roundExact, assignmentExact, rubric, nil
}

func validateResultV1(attempt contract.AutoReviewerAttemptV1, observation contract.AutoReviewerInvocationObservationV1, result contract.ReviewerInvocationResultFactV1) error {
	if result.ExactRef() != *attempt.DomainResult || result.TenantID != attempt.TenantID || result.CaseID != attempt.Case.ID || result.CaseRevision != attempt.Case.Revision || result.RoundID != attempt.Round.ID || result.RoundRevision != attempt.Round.Revision || result.RoundDigest != attempt.Round.Digest || result.AssignmentID != attempt.Assignment.ID || result.AssignmentRevision != attempt.Assignment.Revision || result.AssignmentDigest != attempt.Assignment.Digest || result.TargetID != attempt.Target.ID || result.TargetRevision != attempt.Target.Revision || result.TargetDigest != attempt.Target.Digest || result.AttemptID != observation.RuntimeAttempt.AttemptID || result.ResultSchema != observation.ResultSchema || result.ResultDigest != observation.Output.Digest || len(result.ObservationRefs) != 1 || result.ObservationRefs[0] != observation.ID {
		return driftV1("auto attestation DomainResult drifted from exact provenance")
	}
	return nil
}

func validateCurrentCutV1(now time.Time, target contract.TargetSnapshotV1, c contract.ReviewCaseV1, r contract.ReviewRoundV1, a contract.ReviewerAssignmentV1, attempt contract.AutoReviewerAttemptV1, observation contract.AutoReviewerInvocationObservationV1, rubric contract.RubricDefinitionV1) (int64, error) {
	if err := contract.ValidateNow(now, target.CreatedUnixNano, target.ExpiresUnixNano); err != nil {
		return 0, err
	}
	if err := contract.ValidateNow(now, c.CreatedUnixNano, c.ExpiresUnixNano); err != nil {
		return 0, err
	}
	if err := contract.ValidateNow(now, r.CreatedUnixNano, r.ExpiresUnixNano); err != nil {
		return 0, err
	}
	if err := contract.ValidateNow(now, a.CreatedUnixNano, a.ExpiresUnixNano); err != nil {
		return 0, err
	}
	if now.UnixNano() >= a.LeaseExpiresUnixNano {
		return 0, core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleLeaseRevision, "auto attestation assignment lease expired")
	}
	if err := attempt.ValidateCurrent(now); err != nil {
		return 0, err
	}
	if err := contract.ValidateNow(now, observation.ObservedUnixNano, observation.ExpiresUnixNano); err != nil {
		return 0, err
	}
	if err := rubric.ValidateCurrent(attempt.Rubric, now); err != nil {
		return 0, err
	}
	minimum := target.ExpiresUnixNano
	if c.ExpiresUnixNano < minimum {
		minimum = c.ExpiresUnixNano
	}
	for _, value := range []int64{r.ExpiresUnixNano, a.ExpiresUnixNano, a.LeaseExpiresUnixNano, attempt.ExpiresUnixNano, observation.ExpiresUnixNano, rubric.ExpiresUnixNano} {
		if value < minimum {
			minimum = value
		}
	}
	return minimum, nil
}

func validateConditionCutV2(output contract.AutoReviewerStructuredOutputV1, target contract.TargetSnapshotV1, now time.Time, minimum int64) (int64, error) {
	if err := output.Validate(); err != nil {
		return 0, err
	}
	for _, condition := range output.Conditions {
		if condition.ScopeDigest != target.ActionScopeDigest {
			return 0, core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "auto Condition scope drifted from the exact Target")
		}
		if condition.ExpiresUnixNano <= now.UnixNano() {
			return 0, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "auto Condition expired before Attestation")
		}
		if condition.ExpiresUnixNano < minimum {
			minimum = condition.ExpiresUnixNano
		}
	}
	return minimum, nil
}

func buildFindingsV1(attempt contract.AutoReviewerAttemptV1, observation contract.AutoReviewerInvocationObservationV1, expires int64) ([]contract.FindingV1, error) {
	ids, err := DeterministicFindingIDsV1(attempt.TenantID, attempt, observation)
	if err != nil {
		return nil, err
	}
	// IDs are sorted for Attestation canonical form; match drafts through the
	// same deterministic identity calculation instead of relying on sort order.
	byID := make(map[string]contract.AutoFindingDraftV1, len(ids))
	for index, draft := range observation.Output.Findings {
		digest, digestErr := core.CanonicalJSONDigest("praxis.review.auto-attestation", findingIdentityContractV1, "FindingIdentityInputV1", findingIdentityInputV1{TenantID: attempt.TenantID, Case: attempt.Case, Round: attempt.Round, Target: attempt.Target, Attempt: attempt.ExactRef(), Observation: observation.Ref(), OutputDigest: observation.Output.Digest, Ordinal: uint32(index + 1), Draft: draft})
		if digestErr != nil {
			return nil, digestErr
		}
		byID["auto-finding-"+string(digest)] = draft
	}
	values := make([]contract.FindingV1, 0, len(ids))
	for _, id := range ids {
		draft := byID[id]
		value, sealErr := contract.SealFindingV1(contract.FindingV1{FactIdentityV1: contract.FactIdentityV1{TenantID: attempt.TenantID, ID: id, Revision: 1, CreatedUnixNano: observation.ObservedUnixNano, UpdatedUnixNano: observation.ObservedUnixNano}, CaseID: attempt.Case.ID, CaseRevision: attempt.Case.Revision, RoundID: attempt.Round.ID, RoundRevision: attempt.Round.Revision, RoundDigest: attempt.Round.Digest, TargetID: attempt.Target.ID, TargetRevision: attempt.Target.Revision, TargetDigest: attempt.Target.Digest, Category: draft.Category, Priority: draft.Priority, Anchor: draft.Anchor, Claim: draft.Claim, Impact: draft.Impact, Evidence: append([]runtimeports.ReviewEvidenceRefV2{}, draft.Evidence...), Status: contract.FindingOpenV1, ExpiresUnixNano: expires})
		if sealErr != nil {
			return nil, sealErr
		}
		values = append(values, value)
	}
	return values, nil
}

func findingTracesV2(attempt contract.AutoReviewerAttemptV1, observation contract.AutoReviewerInvocationObservationV1, findings []contract.FindingV1) ([]contract.TraceFactV1, error) {
	values := make([]contract.TraceFactV1, 0, len(findings))
	for index, finding := range findings {
		refs := []string{attempt.ID, observation.ID, finding.ID}
		sort.Strings(refs)
		value, err := contract.SealTraceFactV1(contract.TraceFactV1{
			FactIdentityV1: contract.FactIdentityV1{TenantID: finding.TenantID, ID: "trace-" + finding.ID, Revision: 1, CreatedUnixNano: finding.CreatedUnixNano, UpdatedUnixNano: finding.UpdatedUnixNano},
			CaseID:         finding.CaseID, CaseRevision: finding.CaseRevision, TargetID: finding.TargetID, TargetRevision: finding.TargetRevision, TargetDigest: finding.TargetDigest,
			Event: contract.TraceFindingV1, SourceID: "praxis.review/auto-finding/" + attempt.ID, SourceEpoch: 1, SourceSequence: uint64(index + 1),
			CausationID: observation.ID, CorrelationID: finding.CaseID, FactRefs: refs,
		})
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func buildAttestationV1(command RecordCommandV1, attempt contract.AutoReviewerAttemptV1, observation contract.AutoReviewerInvocationObservationV1, result contract.ReviewerInvocationResultFactV1, expires int64, now time.Time, findings []contract.FindingV1, terminationReached bool) (contract.AttestationV1, error) {
	evidence := append([]runtimeports.ReviewEvidenceRefV2{}, observation.Output.Evidence...)
	evidenceDigest, err := contract.ComputeReviewEvidenceDigestV1(evidence)
	if err != nil {
		return contract.AttestationV1{}, err
	}
	findingRefs := make([]string, 0, len(findings))
	for _, value := range findings {
		findingRefs = append(findingRefs, value.ID)
	}
	sort.Strings(findingRefs)
	provenance := &contract.AutoReviewerAttestationProvenanceV1{Attempt: attempt.ExactRef(), Observation: observation.Ref(), Rubric: attempt.Rubric}
	apply := command.ApplySettlement
	resolution := observation.Output.Resolution
	reasonCodes := append([]string{}, observation.Output.ReasonCodes...)
	conditionsDigest := observation.Output.ConditionsDigest
	conditions := append([]runtimeports.ReviewConditionV2(nil), observation.Output.Conditions...)
	if terminationReached {
		resolution = contract.ResolutionEscalateHumanV1
		reasonCodes = []string{contract.AutoReviewTerminationCeilingReasonV1}
		conditionsDigest = ""
		conditions = nil
	}
	return contract.SealAttestationV1(contract.AttestationV1{FactIdentityV1: contract.FactIdentityV1{TenantID: command.TenantID, ID: command.AttestationID, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, IdempotencyKey: command.IdempotencyKey, CaseID: attempt.Case.ID, CaseRevision: attempt.Case.Revision, RoundID: attempt.Round.ID, RoundRevision: attempt.Round.Revision, RoundDigest: attempt.Round.Digest, AssignmentID: attempt.Assignment.ID, AssignmentRevision: attempt.Assignment.Revision, AssignmentDigest: attempt.Assignment.Digest, TargetID: attempt.Target.ID, TargetRevision: attempt.Target.Revision, TargetDigest: attempt.Target.Digest, ContextFrameDigest: attempt.ContextFrameDigest, Route: contract.RouteAutoV1, ReviewerID: attempt.ReviewerID, ReviewerAuthority: attempt.ReviewerAuthority, ReviewerBinding: attempt.ReviewerBinding, Resolution: resolution, ReasonCodes: reasonCodes, FindingRefs: findingRefs, Evidence: evidence, EvidenceDigest: evidenceDigest, Conditions: conditions, ConditionsDigest: conditionsDigest, DomainApplySettlement: &apply, ReviewerAttemptID: result.AttemptID, ReviewerResultDigest: result.ResultDigest, AutoProvenance: provenance, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
}

func terminationReachedV1(current reviewport.AutoReviewTerminationCurrentProjectionV1, rubric contract.RubricDefinitionV1, output contract.AutoReviewerStructuredOutputV1) bool {
	return current.RepeatedFindingCount >= rubric.Termination.RepeatFindingLimit || current.RepeatedRejectionCount >= rubric.Termination.RepeatRejectionLimit || (current.RoundCount >= rubric.Termination.MaxRounds && (output.Resolution == contract.ResolutionRequestChangesV1 || output.Resolution == contract.ResolutionInsufficientEvidenceV1))
}

func validateTraceV1(trace contract.TraceFactV1, c contract.ReviewCaseV1, attempt contract.AutoReviewerAttemptV1, observation contract.AutoReviewerInvocationObservationV1, result contract.ReviewerInvocationResultFactV1, apply contract.DomainApplySettlementFactV1, rubric contract.RubricDefinitionV1, attestationID string, findings []contract.FindingV1) error {
	refs := []string{attempt.ID, observation.ID, result.ID, apply.ID, rubric.ID, attestationID}
	for _, value := range findings {
		refs = append(refs, value.ID)
	}
	sort.Strings(refs)
	if trace.TenantID != c.TenantID || trace.CaseID != c.ID || trace.CaseRevision != c.Revision || trace.TargetID != c.TargetID || trace.TargetRevision != c.TargetRevision || trace.TargetDigest != c.TargetDigest || trace.Event != contract.TraceAttestedV1 || !reflect.DeepEqual(trace.FactRefs, refs) {
		return driftV1("auto attestation Trace does not bind the exact provenance set")
	}
	return nil
}

func nextCaseStateV1(resolution contract.ResolutionV1) (contract.CaseStateV1, error) {
	switch resolution {
	case contract.ResolutionAcceptV1, contract.ResolutionConditionalV1, contract.ResolutionRejectV1:
		return contract.CaseAttestedV1, nil
	case contract.ResolutionRequestChangesV1:
		return contract.CaseWaitingRevisionV1, nil
	case contract.ResolutionEscalateHumanV1:
		return contract.CaseWaitingHumanV1, nil
	case contract.ResolutionInsufficientEvidenceV1:
		return contract.CaseWaitingEvidenceV1, nil
	default:
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "auto attestation resolution is unsupported")
	}
}

func (o *OwnerV1) recoverRecordV1(ctx context.Context, command RecordCommandV1, previous contract.ReviewCaseV1, nextState contract.CaseStateV1, attestation contract.AttestationV1, findings []contract.FindingV1, additional []contract.TraceFactV1, originalUnknown error) (RecordResultV1, error) {
	recoveryCtx, cancel := o.lostReplyRecoveryContextV1(ctx)
	defer cancel()
	stored, err := o.store.InspectAttestationExactV1(recoveryCtx, command.TenantID, reviewport.ExactV1(attestation.ID, attestation.Revision, attestation.Digest))
	if err != nil {
		return RecordResultV1{}, originalUnknown
	}
	next, err := expectedNextCaseV1(previous, nextState, attestation.ObservedUnixNano)
	if err != nil {
		return RecordResultV1{}, err
	}
	storedCase, err := o.store.InspectCaseExactV1(recoveryCtx, command.TenantID, reviewport.ExactV1(next.ID, next.Revision, next.Digest))
	if err != nil {
		return RecordResultV1{}, originalUnknown
	}
	if !reflect.DeepEqual(stored, attestation) || !reflect.DeepEqual(storedCase, next) {
		return RecordResultV1{}, driftV1("auto attestation lost-reply recovery returned different canonical facts")
	}
	for _, event := range append([]contract.TraceFactV1{command.Trace}, additional...) {
		if _, err = o.store.InspectTraceExactV1(recoveryCtx, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); err != nil {
			return RecordResultV1{}, originalUnknown
		}
	}
	return RecordResultV1{Case: storedCase, Attestation: stored, Findings: findings}, nil
}

func (o *OwnerV1) lostReplyRecoveryContextV1(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := o.recoveryTimeout
	if timeout <= 0 || timeout > lostReplyRecoveryTimeoutV1 {
		timeout = lostReplyRecoveryTimeoutV1
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

func escalationTraceV2(primary contract.TraceFactV1, attestation contract.AttestationV1, next contract.CaseStateV1) ([]contract.TraceFactV1, error) {
	if next != contract.CaseWaitingHumanV1 {
		return nil, nil
	}
	if primary.SourceSequence == ^uint64(0) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "auto escalation Trace sequence overflowed")
	}
	value := primary
	value.ID = primary.ID + "-escalated"
	value.Digest = ""
	value.CaseRevision++
	value.Event = contract.TraceEscalatedV1
	value.SourceSequence++
	value.CausationID = attestation.ID
	value.FactRefs = append([]string(nil), primary.FactRefs...)
	index := sort.SearchStrings(value.FactRefs, attestation.ID)
	if index >= len(value.FactRefs) || value.FactRefs[index] != attestation.ID {
		value.FactRefs = append(value.FactRefs, attestation.ID)
		sort.Strings(value.FactRefs)
	}
	sealed, err := contract.SealTraceFactV1(value)
	if err != nil {
		return nil, err
	}
	return []contract.TraceFactV1{sealed}, nil
}

func expectedNextCaseV1(previous contract.ReviewCaseV1, nextState contract.CaseStateV1, updated int64) (contract.ReviewCaseV1, error) {
	next := previous
	next.Revision++
	next.State = nextState
	next.UpdatedUnixNano = updated
	next.Digest = ""
	return contract.SealReviewCaseV1(next)
}

func sameAttemptSubjectV1(left, right contract.AutoReviewerAttemptV1) bool {
	return left.TenantID == right.TenantID && left.ID == right.ID && left.IdempotencyKey == right.IdempotencyKey && left.Case == right.Case && left.Round == right.Round && left.Assignment == right.Assignment && left.Target == right.Target && left.Rubric == right.Rubric && left.ContextFrameDigest == right.ContextFrameDigest && sameReviewerContextRefV1(left.ReviewerContext, right.ReviewerContext) && left.ReviewerID == right.ReviewerID && left.ReviewerAuthority == right.ReviewerAuthority && left.ReviewerBinding == right.ReviewerBinding && left.RouteID == right.RouteID && reflect.DeepEqual(left.Operation, right.Operation) && left.OperationDigest == right.OperationDigest && left.InvocationEffect == right.InvocationEffect && left.ResultSchema == right.ResultSchema && left.RoundOrdinal == right.RoundOrdinal && left.MaxCostMicros == right.MaxCostMicros && left.CreatedUnixNano == right.CreatedUnixNano && left.ExpiresUnixNano == right.ExpiresUnixNano
}

func sameReviewerContextRefV1(left, right *contract.ReviewerContextEnvelopeRefV1) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func validBaselineV1(now time.Time) error {
	if now.IsZero() || now.UnixNano() <= 0 {
		return clockRegressionV1("auto attestation baseline clock is invalid")
	}
	return nil
}

func unknownReplyV1(err error) bool {
	return core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorUnavailable) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func driftV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, message)
}
func clockRegressionV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, message)
}
func unknownV1(message string) error {
	return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, message)
}
