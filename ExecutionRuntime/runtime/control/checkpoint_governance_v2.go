package control

import (
	"reflect"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ValidateCheckpointAttemptTransitionV2 is the Fact Owner transition guard.
// Gateway construction is not authority to skip monotonic revision, immutable
// identity or sidecar-history checks.
func ValidateCheckpointAttemptTransitionV2(current, next ports.CheckpointAttemptFactV2) error {
	if current.Validate() != nil || next.Validate() != nil || next.Revision != current.Revision+1 || next.TenantID != current.TenantID || next.ID != current.ID || !ports.SameExecutionScopeV2(current.Scope, next.Scope) || next.ScopeDigest != current.ScopeDigest || next.RunID != current.RunID || next.RunStableIdentityDigest != current.RunStableIdentityDigest || next.Generation != current.Generation || next.GenerationBinding != current.GenerationBinding || next.BindingSet != current.BindingSet || next.ParticipantSetCertification != current.ParticipantSetCertification || next.Workflow != current.Workflow || next.BarrierPolicy != current.BarrierPolicy || next.BarrierPolicySemanticDigest != current.BarrierPolicySemanticDigest || next.FrozenUnknownAtDeadlineMode != current.FrozenUnknownAtDeadlineMode || next.FrozenAllowNotAppliedAbort != current.FrozenAllowNotAppliedAbort || next.ReconciliationDeadlineUnixNano != current.ReconciliationDeadlineUnixNano || next.CreatedUnixNano != current.CreatedUnixNano || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "checkpoint Attempt transition rewinds revision or immutable identity")
	}
	if current.EffectCut != nil && !reflect.DeepEqual(current.EffectCut, next.EffectCut) || current.FinalizationCut != nil && !reflect.DeepEqual(current.FinalizationCut, next.FinalizationCut) || current.FinalizationInputs != nil && !reflect.DeepEqual(current.FinalizationInputs, next.FinalizationInputs) || current.Consistency != nil && !reflect.DeepEqual(current.Consistency, next.Consistency) {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Attempt transition overwrites immutable history")
	}
	allowed := false
	switch current.State {
	case ports.CheckpointAttemptBarrierAcquiredV2:
		allowed = next.State == ports.CheckpointAttemptCutFrozenV2
	case ports.CheckpointAttemptCutFrozenV2:
		allowed = next.State == ports.CheckpointAttemptFinalizingInputsV2 || next.State == ports.CheckpointAttemptConsistentV2
	case ports.CheckpointAttemptFinalizingInputsV2:
		allowed = next.State == ports.CheckpointAttemptFinalizingInputsV2 || next.State == ports.CheckpointAttemptIncompleteV2 || next.State == ports.CheckpointAttemptAbortedV2 || next.State == ports.CheckpointAttemptIndeterminateV2
	}
	if !allowed {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Attempt transition selects an illegal successor")
	}
	return nil
}

func ValidateCheckpointBarrierTransitionV2(current, next ports.CheckpointBarrierLeaseFactV2) error {
	if current.Validate() != nil || next.Validate() != nil || current.State != ports.CheckpointBarrierActiveV2 || next.State != ports.CheckpointBarrierClosedV2 || next.Revision != current.Revision+1 || next.TenantID != current.TenantID || next.ID != current.ID || next.AttemptID != current.AttemptID || next.ScopeDigest != current.ScopeDigest || next.RunID != current.RunID || next.RunStableIdentityDigest != current.RunStableIdentityDigest || next.Policy != current.Policy || next.AcquiredDispatchWatermark != current.AcquiredDispatchWatermark || next.AcquiredUnixNano != current.AcquiredUnixNano || next.ExpiresUnixNano != current.ExpiresUnixNano || next.ClosedUnixNano < current.AcquiredUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "checkpoint Barrier transition rewinds or changes immutable lease history")
	}
	return nil
}

func BuildCheckpointAttemptBundleV2(request ports.CreateCheckpointAttemptRequestV2, policy ports.CheckpointBarrierPolicyCurrentProjectionV2, now time.Time) (ports.CheckpointAttemptBarrierBundleV2, error) {
	return buildCheckpointAttemptBundleV2(request, policy, 0, now)
}

// BuildCheckpointAttemptBundleBoundedV2 derives the immutable Attempt and
// Barrier using the earliest expiry supplied by the current semantic Owners.
// ownerNotAfter is not caller policy: the governance Gateway obtains it from
// an exact CheckpointAttemptInputsCurrentProjectionV2.
func BuildCheckpointAttemptBundleBoundedV2(request ports.CreateCheckpointAttemptRequestV2, policy ports.CheckpointBarrierPolicyCurrentProjectionV2, ownerNotAfter int64, now time.Time) (ports.CheckpointAttemptBarrierBundleV2, error) {
	if ownerNotAfter <= now.UnixNano() {
		return ports.CheckpointAttemptBarrierBundleV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint current Owner inputs expire before Attempt creation")
	}
	return buildCheckpointAttemptBundleV2(request, policy, ownerNotAfter, now)
}

func buildCheckpointAttemptBundleV2(request ports.CreateCheckpointAttemptRequestV2, policy ports.CheckpointBarrierPolicyCurrentProjectionV2, ownerNotAfter int64, now time.Time) (ports.CheckpointAttemptBarrierBundleV2, error) {
	if err := request.Validate(); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := policy.Validate(now); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if policy.Ref != request.BarrierPolicy {
		return ports.CheckpointAttemptBarrierBundleV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "checkpoint Barrier Policy ref drifted")
	}
	nowNanos := now.UnixNano()
	barrierCandidate, ok := checkedAddNanosV2(nowNanos, policy.MaxBarrierTTLUnixNano)
	if !ok {
		return ports.CheckpointAttemptBarrierBundleV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "checkpoint Barrier TTL overflows")
	}
	reconciliationCandidate, ok := checkedAddNanosV2(nowNanos, policy.MaxReconciliationTTLUnixNano)
	if !ok {
		return ports.CheckpointAttemptBarrierBundleV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "checkpoint reconciliation TTL overflows")
	}
	expires := minNanosV2(barrierCandidate, policy.ExpiresUnixNano, policy.AbsoluteNotAfterUnixNano, request.Workflow.NotAfter)
	if ownerNotAfter > 0 {
		expires = minNanosV2(expires, ownerNotAfter)
	}
	deadline := minNanosV2(reconciliationCandidate, expires, policy.ExpiresUnixNano, policy.AbsoluteNotAfterUnixNano, request.Workflow.NotAfter)
	if nowNanos <= 0 || deadline <= nowNanos || deadline > expires {
		return ports.CheckpointAttemptBarrierBundleV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint Barrier and reconciliation deadline cannot be safely derived")
	}
	if request.ExpectedBarrierExpiresUnixNano != 0 && request.ExpectedBarrierExpiresUnixNano != expires {
		return ports.CheckpointAttemptBarrierBundleV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "checkpoint Barrier expected expiry mismatches Owner derivation")
	}
	barrier := ports.CheckpointBarrierLeaseFactV2{
		TenantID: request.Scope.Identity.TenantID, ID: request.BarrierID, AttemptID: request.AttemptID,
		Revision: 1, State: ports.CheckpointBarrierActiveV2, ScopeDigest: request.ScopeDigest,
		RunID: request.RunID, RunStableIdentityDigest: request.RunStableIdentityDigest, Policy: request.BarrierPolicy,
		AcquiredDispatchWatermark: request.AcquiredDispatchWatermark, AcquiredUnixNano: nowNanos, ExpiresUnixNano: expires,
	}
	var err error
	barrier, err = ports.SealCheckpointBarrierLeaseFactV2(barrier)
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	attempt := ports.CheckpointAttemptFactV2{
		TenantID: request.Scope.Identity.TenantID, ID: request.AttemptID, Revision: 1,
		State: ports.CheckpointAttemptBarrierAcquiredV2, Scope: request.Scope, ScopeDigest: request.ScopeDigest,
		RunID: request.RunID, RunStableIdentityDigest: request.RunStableIdentityDigest,
		Generation: request.Generation, GenerationBinding: request.GenerationBinding, BindingSet: request.BindingSet,
		ParticipantSetCertification: request.ParticipantSetCertification, Workflow: request.Workflow,
		BarrierPolicy: request.BarrierPolicy, BarrierPolicySemanticDigest: request.BarrierPolicy.SemanticDigest,
		FrozenUnknownAtDeadlineMode: policy.UnknownAtDeadlineMode, FrozenAllowNotAppliedAbort: policy.AllowConfirmedNotAppliedAbort,
		Barrier: barrier.RefV2(), ReconciliationDeadlineUnixNano: deadline, CreatedUnixNano: nowNanos, UpdatedUnixNano: nowNanos,
	}
	attempt, err = ports.SealCheckpointAttemptFactV2(attempt)
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	bundle := ports.CheckpointAttemptBarrierBundleV2{Attempt: attempt, Barrier: barrier}
	return bundle, bundle.Validate()
}

func BuildCheckpointEffectCutBundleV2(current ports.CheckpointAttemptBarrierBundleV2, request ports.FreezeCheckpointEffectCutRequestV2, inventory ports.CheckpointEffectInventoryCurrentProjectionV2, now time.Time) (ports.CheckpointEffectCutBundleV2, error) {
	if err := current.Validate(); err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	if err := inventory.Validate(now); err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	if inventory.Attempt != current.Attempt.RefV2() || inventory.Barrier != current.Barrier.RefV2() || inventory.RootDigest != request.EffectInventoryRoot || inventory.Watermark != request.EffectInventoryWatermark || uint64(len(inventory.Entries)) != request.ExpectedEffectCount {
		return ports.CheckpointEffectCutBundleV2{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Effect inventory current projection mismatches expected root, watermark or set")
	}
	if now.IsZero() || now.UnixNano() < current.Attempt.UpdatedUnixNano || !now.Before(time.Unix(0, current.Barrier.ExpiresUnixNano)) {
		return ports.CheckpointEffectCutBundleV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "checkpoint Effect Cut clock or Barrier is stale")
	}
	if current.Attempt.State != ports.CheckpointAttemptBarrierAcquiredV2 || current.Attempt.Revision != request.ExpectedAttemptRevision || current.Barrier.Revision != request.ExpectedBarrierRevision || request.Attempt != current.Attempt.RefV2() || request.Barrier != current.Barrier.RefV2() {
		return ports.CheckpointEffectCutBundleV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "checkpoint Effect Cut expected watermarks drifted")
	}
	cutID, err := deriveCheckpointObjectIDV2("effect-cut", current.Attempt.RefV2(), request.IdempotencyKey)
	if err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	cut := ports.EffectCutFactV2{
		Ref:     ports.EffectCutRefV2{ID: cutID, Revision: 1, Attempt: current.Attempt.RefV2(), RootDigest: request.EffectInventoryRoot, Watermark: request.EffectInventoryWatermark},
		Barrier: current.Barrier.RefV2(), Entries: inventory.Entries, CreatedUnixNano: now.UnixNano(),
	}
	cut, err = ports.SealEffectCutFactV2(cut)
	if err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	next := current.Attempt
	next.Revision++
	next.State = ports.CheckpointAttemptCutFrozenV2
	ref := cut.Ref
	next.EffectCut = &ref
	next.UpdatedUnixNano = now.UnixNano()
	next, err = ports.SealCheckpointAttemptFactV2(next)
	if err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	bundle := ports.CheckpointEffectCutBundleV2{Attempt: next, Cut: cut}
	return bundle, bundle.Validate()
}

func BuildCheckpointFinalizationCutV2(current ports.CheckpointAttemptBarrierBundleV2, cut ports.EffectCutFactV2, request ports.PrepareCheckpointFinalizationInputsRequestV2, now time.Time) (ports.CheckpointAttemptFactV2, ports.CheckpointFinalizationCutFactV2, error) {
	if current.Validate() != nil || cut.Validate() != nil || request.Validate() != nil || now.IsZero() {
		return ports.CheckpointAttemptFactV2{}, ports.CheckpointFinalizationCutFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCheckpointInconsistent, "checkpoint Finalization Cut inputs are invalid")
	}
	if current.Attempt.Revision != request.ExpectedAttemptRevision || current.Barrier.Revision != request.ExpectedBarrierRevision || request.Attempt != current.Attempt.RefV2() || request.Barrier != current.Barrier.RefV2() || request.EffectCut != cut.Ref || current.Attempt.EffectCut == nil || *current.Attempt.EffectCut != cut.Ref || terminalAttemptStateV2(current.Attempt.State) {
		return ports.CheckpointAttemptFactV2{}, ports.CheckpointFinalizationCutFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "checkpoint Finalization Cut watermarks drifted")
	}
	cutID, err := deriveCheckpointObjectIDV2("finalization-cut", current.Attempt.RefV2(), request.IdempotencyKey)
	if err != nil {
		return ports.CheckpointAttemptFactV2{}, ports.CheckpointFinalizationCutFactV2{}, err
	}
	ref := ports.CheckpointFinalizationCutRefV2{ID: cutID, Revision: 1, Attempt: current.Attempt.RefV2(), EffectCut: cut.Ref, CutUnixNano: now.UnixNano()}
	ref.Digest, err = ref.DigestV2()
	if err != nil {
		return ports.CheckpointAttemptFactV2{}, ports.CheckpointFinalizationCutFactV2{}, err
	}
	fact := ports.CheckpointFinalizationCutFactV2{ContractVersion: ports.CheckpointGovernanceContractVersionV2, Ref: ref, CreatedUnixNano: now.UnixNano()}
	if err := fact.Validate(); err != nil {
		return ports.CheckpointAttemptFactV2{}, ports.CheckpointFinalizationCutFactV2{}, err
	}
	next := current.Attempt
	next.Revision++
	next.State = ports.CheckpointAttemptFinalizingInputsV2
	next.FinalizationCut = &ref
	next.UpdatedUnixNano = now.UnixNano()
	next, err = ports.SealCheckpointAttemptFactV2(next)
	return next, fact, err
}

func BuildCheckpointFinalizationClosureV2(attempt ports.CheckpointAttemptFactV2, barrier ports.CheckpointBarrierLeaseFactV2, cut ports.EffectCutFactV2, finalizationCut ports.CheckpointFinalizationCutFactV2, diagnostics ports.CheckpointDiagnosticsFinalizationSealRefV2, residuals ports.CheckpointResidualsFinalizationSealRefV2, idempotencyKey string, now time.Time) (ports.CheckpointAttemptFactV2, ports.CheckpointFinalizationInputClosureFactV2, error) {
	if attempt.Validate() != nil || barrier.Validate() != nil || cut.Validate() != nil || finalizationCut.Validate() != nil || diagnostics.Validate() != nil || residuals.Validate() != nil || !validBuilderIDV2(idempotencyKey) || now.IsZero() {
		return ports.CheckpointAttemptFactV2{}, ports.CheckpointFinalizationInputClosureFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCheckpointInconsistent, "checkpoint Finalization Closure inputs are invalid")
	}
	closureID, err := deriveCheckpointObjectIDV2("finalization-closure", attempt.RefV2(), idempotencyKey)
	if err != nil {
		return ports.CheckpointAttemptFactV2{}, ports.CheckpointFinalizationInputClosureFactV2{}, err
	}
	ref := ports.CheckpointFinalizationInputClosureRefV2{ID: closureID, Revision: 1, Attempt: attempt.RefV2(), Barrier: barrier.RefV2(), EffectCut: cut.Ref, FinalizationCut: finalizationCut.Ref, DiagnosticsSeal: diagnostics, ResidualsSeal: residuals}
	ref.Digest, err = ref.DigestV2()
	if err != nil {
		return ports.CheckpointAttemptFactV2{}, ports.CheckpointFinalizationInputClosureFactV2{}, err
	}
	fact := ports.CheckpointFinalizationInputClosureFactV2{ContractVersion: ports.CheckpointGovernanceContractVersionV2, Ref: ref, CreatedUnixNano: now.UnixNano()}
	if err := fact.Validate(); err != nil {
		return ports.CheckpointAttemptFactV2{}, ports.CheckpointFinalizationInputClosureFactV2{}, err
	}
	next := attempt
	next.Revision++
	next.State = ports.CheckpointAttemptFinalizingInputsV2
	next.FinalizationInputs = &ref
	next.UpdatedUnixNano = now.UnixNano()
	next, err = ports.SealCheckpointAttemptFactV2(next)
	return next, fact, err
}

func BuildCheckpointConsistencyCommitBundleV2(attempt ports.CheckpointAttemptFactV2, barrier ports.CheckpointBarrierLeaseFactV2, cut ports.EffectCutFactV2, request ports.CommitCheckpointConsistencyRequestV2, manifest ports.CheckpointManifestSealProjectionV2, participants ports.CheckpointParticipantSetCurrentProjectionV2, closures []ports.CheckpointParticipantClosureRefV2, now time.Time) (ports.CheckpointConsistencyCommitBundleV2, error) {
	if attempt.Validate() != nil || barrier.Validate() != nil || cut.Validate() != nil || request.Validate() != nil || manifest.Validate() != nil || now.IsZero() {
		return ports.CheckpointConsistencyCommitBundleV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCheckpointInconsistent, "checkpoint Consistency inputs are invalid")
	}
	if err := participants.Validate(now); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if attempt.Revision != request.ExpectedAttemptRevision || barrier.Revision != request.ExpectedBarrierRevision || request.Attempt != attempt.RefV2() || request.Barrier != barrier.RefV2() || request.EffectCut != cut.Ref || request.ManifestSeal != manifest.Ref || manifest.Ref.Attempt.ID != attempt.ID || manifest.Ref.EffectCut != cut.Ref || manifest.ParticipantSetDigest != attempt.ParticipantSetCertification.Digest || participants.Attempt != attempt.RefV2() || participants.Certification != attempt.ParticipantSetCertification || participants.RootDigest != request.ExpectedParticipantRoot || participants.Watermark != request.ExpectedParticipantWatermark || uint64(len(participants.Participants)) != request.ExpectedParticipantCount || !sameCheckpointClosureSetV2(closures, manifest.ParticipantClosures) || !now.Before(time.Unix(0, barrier.ExpiresUnixNano)) {
		return ports.CheckpointConsistencyCommitBundleV2{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Consistency closure drifted")
	}
	for _, closure := range closures {
		if closure.Validate() != nil || closure.Terminal == nil || closure.Terminal.Phase != ports.CheckpointPhaseCommitV2 || closure.Terminal.PhaseFact.State != ports.CheckpointParticipantCommittedV2 {
			return ports.CheckpointConsistencyCommitBundleV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint Consistency requires every committed Participant closure")
		}
	}
	id, err := deriveCheckpointObjectIDV2("consistency", attempt.RefV2(), request.IdempotencyKey)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	closures = append([]ports.CheckpointParticipantClosureRefV2{}, closures...)
	sort.Slice(closures, func(i, j int) bool { return closures[i].ID < closures[j].ID })
	consistency := ports.CheckpointConsistencyFactV2{ContractVersion: ports.CheckpointGovernanceContractVersionV2, Ref: ports.CheckpointConsistencyRefV2{ID: id, Revision: 1, Attempt: attempt.RefV2()}, Barrier: barrier.RefV2(), EffectCut: cut.Ref, ManifestSeal: manifest.Ref, ParticipantClosures: closures, ParticipantSetDigest: manifest.ParticipantSetDigest, ParticipantRootDigest: participants.RootDigest, ParticipantWatermark: participants.Watermark, ParticipantCount: uint64(len(participants.Participants)), FrozenRefSetDigest: manifest.Ref.FrozenRefSetDigest, CreatedUnixNano: now.UnixNano()}
	nextBarrier := barrier
	nextBarrier.Revision++
	nextBarrier.State = ports.CheckpointBarrierClosedV2
	nextBarrier.ClosedUnixNano = now.UnixNano()
	nextBarrier.CloseReason = core.ReasonCheckpointInconsistent
	nextBarrier, err = ports.SealCheckpointBarrierLeaseFactV2(nextBarrier)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	consistency.Barrier = nextBarrier.RefV2()
	// Recompute after the exact closed Barrier is known.
	consistency, err = ports.SealCheckpointConsistencyFactV2(consistency)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	nextAttempt := attempt
	nextAttempt.Revision++
	nextAttempt.State = ports.CheckpointAttemptConsistentV2
	nextAttempt.Barrier = nextBarrier.RefV2()
	nextAttempt.Consistency = &consistency.Ref
	nextAttempt.FinalizationCut = nil
	nextAttempt.FinalizationInputs = nil
	nextAttempt.UpdatedUnixNano = now.UnixNano()
	nextAttempt, err = ports.SealCheckpointAttemptFactV2(nextAttempt)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	bundle := ports.CheckpointConsistencyCommitBundleV2{Attempt: nextAttempt, Barrier: nextBarrier, Consistency: consistency}
	return bundle, bundle.Validate()
}

func BuildCheckpointFinalizationBundleV2(attempt ports.CheckpointAttemptFactV2, barrier ports.CheckpointBarrierLeaseFactV2, inputs ports.CheckpointFinalizationInputClosureFactV2, diagnostics ports.CheckpointDiagnosticsFinalizationSealProjectionV2, residuals ports.CheckpointResidualsFinalizationSealProjectionV2, now time.Time) (ports.CheckpointAttemptFinalizationBundleV2, error) {
	if attempt.Validate() != nil || barrier.Validate() != nil || inputs.Validate() != nil || diagnostics.Validate() != nil || residuals.Validate() != nil || now.IsZero() || !ports.SameCheckpointDiagnosticsFinalizationSealRefV2(inputs.Ref.DiagnosticsSeal, diagnostics.Ref) || !ports.SameCheckpointResidualsFinalizationSealRefV2(inputs.Ref.ResidualsSeal, residuals.Ref) {
		return ports.CheckpointAttemptFinalizationBundleV2{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Finalization inputs drifted")
	}
	state, err := DeriveCheckpointFinalizationStateV2(attempt, diagnostics.Ref, residuals.Ref, now)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	nextBarrier := barrier
	nextBarrier.Revision++
	nextBarrier.State = ports.CheckpointBarrierClosedV2
	nextBarrier.ClosedUnixNano = now.UnixNano()
	nextBarrier.CloseReason = core.ReasonCheckpointInconsistent
	nextBarrier, err = ports.SealCheckpointBarrierLeaseFactV2(nextBarrier)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	nextAttempt := attempt
	nextAttempt.Revision++
	nextAttempt.State = state
	nextAttempt.Barrier = nextBarrier.RefV2()
	nextAttempt.UpdatedUnixNano = now.UnixNano()
	nextAttempt, err = ports.SealCheckpointAttemptFactV2(nextAttempt)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	bundle := ports.CheckpointAttemptFinalizationBundleV2{Attempt: nextAttempt, Barrier: nextBarrier, Inputs: inputs}
	return bundle, bundle.Validate()
}

func DeriveCheckpointFinalizationStateV2(attempt ports.CheckpointAttemptFactV2, diagnostics ports.CheckpointDiagnosticsFinalizationSealRefV2, residuals ports.CheckpointResidualsFinalizationSealRefV2, now time.Time) (ports.CheckpointAttemptStateV2, error) {
	if attempt.Validate() != nil || diagnostics.Validate() != nil || residuals.Validate() != nil || now.IsZero() || diagnostics.Attempt.ID != attempt.ID || residuals.Attempt.ID != attempt.ID {
		return "", core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Finalization classification inputs drifted")
	}
	classifications := append([]ports.CheckpointFinalizationClassificationEntryV2{}, diagnostics.Classifications.Entries...)
	classifications = append(classifications, residuals.Classifications.Entries...)
	state := ports.CheckpointAttemptIncompleteV2
	hasUnknown := false
	hasIncomplete := false
	hasNotApplied := false
	for _, classification := range classifications {
		switch classification.Classification {
		case ports.CheckpointClassificationUnknownV2:
			hasUnknown = true
		case ports.CheckpointClassificationIncompleteV2:
			hasIncomplete = true
		case ports.CheckpointClassificationConfirmedNotAppliedV2:
			hasNotApplied = true
		}
	}
	if hasUnknown {
		if now.UnixNano() < attempt.ReconciliationDeadlineUnixNano {
			return "", core.NewError(core.ErrorPreconditionFailed, core.ReasonRemoteResidualUnresolved, "checkpoint residuals remain unknown before reconciliation deadline")
		}
		state = ports.CheckpointAttemptIndeterminateV2
	} else if hasIncomplete || len(classifications) == 0 {
		state = ports.CheckpointAttemptIncompleteV2
	} else if hasNotApplied && attempt.FrozenAllowNotAppliedAbort {
		state = ports.CheckpointAttemptAbortedV2
	} else {
		state = ports.CheckpointAttemptIncompleteV2
	}
	return state, nil
}

func CheckpointCanonicalDigestV2(discriminator string, value any) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.runtime.checkpoint-governance", ports.CheckpointGovernanceContractVersionV2, discriminator, value)
}

func checkedAddNanosV2(left, right int64) (int64, bool) {
	if right <= 0 || left > int64(^uint64(0)>>1)-right {
		return 0, false
	}
	return left + right, true
}

func minNanosV2(values ...int64) int64 {
	result := int64(^uint64(0) >> 1)
	for _, value := range values {
		if value < result {
			result = value
		}
	}
	return result
}

func deriveCheckpointObjectIDV2(kind string, attempt ports.CheckpointAttemptRefV2, idempotencyKey string) (string, error) {
	digest, err := CheckpointCanonicalDigestV2("CheckpointObjectIdentityV2", struct {
		Kind    string                       `json:"kind"`
		Attempt ports.CheckpointAttemptRefV2 `json:"attempt"`
		Key     string                       `json:"idempotency_key"`
	}{kind, attempt, idempotencyKey})
	if err != nil {
		return "", err
	}
	return kind + ":" + string(digest), nil
}

func sameCheckpointClosureSetV2(left, right []ports.CheckpointParticipantClosureRefV2) bool {
	if len(left) != len(right) {
		return false
	}
	a := append([]ports.CheckpointParticipantClosureRefV2{}, left...)
	b := append([]ports.CheckpointParticipantClosureRefV2{}, right...)
	sort.Slice(a, func(i, j int) bool { return a[i].ID < a[j].ID })
	sort.Slice(b, func(i, j int) bool { return b[i].ID < b[j].ID })
	for index := range a {
		if a[index].Digest != b[index].Digest || a[index].ID != b[index].ID {
			return false
		}
	}
	return true
}

func terminalAttemptStateV2(state ports.CheckpointAttemptStateV2) bool {
	return state == ports.CheckpointAttemptConsistentV2 || state == ports.CheckpointAttemptIncompleteV2 || state == ports.CheckpointAttemptAbortedV2 || state == ports.CheckpointAttemptIndeterminateV2
}

func validBuilderIDV2(value string) bool { return len(value) > 0 && len(value) <= 192 }
