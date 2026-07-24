package control

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func BuildRestoreAttemptReservationV2(request ports.CreateRestoreAttemptRequestV2, plan ports.RestorePlanCurrentProjectionV2, now time.Time) (ports.RestoreAttemptFactV2, error) {
	if request.Validate() != nil || now.IsZero() || !now.Before(time.Unix(0, request.RequestedNotAfter)) {
		return ports.RestoreAttemptFactV2{}, restoreControlInvalidV2("Create Restore Attempt inputs are invalid or stale")
	}
	if err := plan.Validate(now); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	if request.RestorePlan != plan.RestorePlan {
		return ports.RestoreAttemptFactV2{}, restoreControlConflictV2("Restore Plan exact ref drifted before reservation")
	}
	expires := minRestoreNanosV2(request.RequestedNotAfter, plan.ExpiresUnixNano)
	if !now.Before(time.Unix(0, expires)) {
		return ports.RestoreAttemptFactV2{}, restoreControlInvalidV2("Restore Attempt reservation TTL is exhausted")
	}
	scope, err := ports.SealCheckpointRestoreOperationScopeV2(ports.CheckpointRestoreOperationScopeV2{
		TenantID: core.TenantID(plan.RestorePlan.TenantID), SourceScopeDigest: plan.SourceScopeDigest,
		RestorePlan: plan.RestorePlan, Consistency: plan.CheckpointConsistency.Ref, AttemptID: request.AttemptID,
		Identity: plan.IdentityProposal, ConflictDomain: plan.ConflictDomain,
	})
	if err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	fact := ports.RestoreAttemptFactV2{
		Ref:   ports.RestoreAttemptRefV2{TenantID: scope.TenantID, ID: request.AttemptID, Revision: 1},
		State: ports.RestoreAttemptReservedV2, OperationScope: scope, IdempotencyKey: request.IdempotencyKey,
		RequestedNotAfter: expires, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(),
	}
	return ports.SealRestoreAttemptFactV2(fact)
}

func ValidateRestoreAttemptTransitionV2(current, next ports.RestoreAttemptFactV2) error {
	if current.Validate() != nil || next.Validate() != nil || next.Ref.Revision != current.Ref.Revision+1 || next.Ref.TenantID != current.Ref.TenantID || next.Ref.ID != current.Ref.ID || next.OperationScope != current.OperationScope || next.IdempotencyKey != current.IdempotencyKey || next.RequestedNotAfter != current.RequestedNotAfter || next.CreatedUnixNano != current.CreatedUnixNano || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Restore Attempt transition changed immutable reservation history")
	}
	if current.State != ports.RestoreAttemptReservedV2 || next.State != ports.RestoreAttemptEligibilityBoundV2 || current.Eligibility != nil || next.Eligibility == nil {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Restore Attempt transition is not the reserved to eligibility-bound edge")
	}
	return nil
}

func BuildRestoreEligibilityBindBundleV2(current ports.RestoreAttemptFactV2, request ports.IssueRestoreEligibilityRequestV2, plan ports.RestorePlanCurrentProjectionV2, inputs ports.RestoreEligibilityInputsCurrentProjectionV2, now time.Time) (ports.RestoreEligibilityBindBundleV2, error) {
	if current.Validate() != nil || request.Validate() != nil || now.IsZero() {
		return ports.RestoreEligibilityBindBundleV2{}, restoreControlInvalidV2("Issue Restore Eligibility inputs are invalid")
	}
	if current.State != ports.RestoreAttemptReservedV2 || request.Attempt != current.Ref {
		return ports.RestoreEligibilityBindBundleV2{}, restoreControlConflictV2("Restore Attempt is not exact reserved current")
	}
	if err := plan.Validate(now); err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	if err := inputs.Validate(now); err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	if plan.RestorePlan != current.OperationScope.RestorePlan || plan.CheckpointConsistency.Ref != current.OperationScope.Consistency || plan.IdentityProposal != current.OperationScope.Identity || plan.SourceScopeDigest != current.OperationScope.SourceScopeDigest || inputs.Attempt != current.Ref || inputs.OperationScopeDigest != current.OperationScope.Digest || inputs.SourceScopeDigest != current.OperationScope.SourceScopeDigest {
		return ports.RestoreEligibilityBindBundleV2{}, restoreControlConflictV2("Restore Eligibility exact inputs drifted from reserved Attempt")
	}
	expires := minRestoreNanosV2(now.Add(request.RequestedTTL).UnixNano(), current.RequestedNotAfter, plan.ExpiresUnixNano, inputs.ExpiresUnixNano)
	if !now.Before(time.Unix(0, expires)) {
		return ports.RestoreEligibilityBindBundleV2{}, restoreControlInvalidV2("Restore Eligibility TTL is exhausted")
	}
	eligibility, err := ports.SealRestoreEligibilityFactV2(ports.RestoreEligibilityFactV2{
		Ref:   ports.RestoreEligibilityRefV2{TenantID: current.Ref.TenantID, ID: request.EligibilityID, Revision: 1, ExpiresUnixNano: expires},
		State: ports.RestoreEligibilityActiveV2, Attempt: current.Ref, OperationScopeDigest: current.OperationScope.Digest,
		RestorePlan: current.OperationScope.RestorePlan, CheckpointConsistency: current.OperationScope.Consistency, Identity: current.OperationScope.Identity,
		ReviewTarget: inputs.ReviewTarget, ReviewRequirementRefs: inputs.ReviewRequirementRefs, PolicyBasisRefs: inputs.PolicyBasisRefs,
		AuthorityRequirementRefs: inputs.AuthorityRequirementRefs, ScopeRequirementRefs: inputs.ScopeRequirementRefs, BudgetRequirementRefs: inputs.BudgetRequirementRefs, BindingRequirementRefs: inputs.BindingRequirementRefs, ContextRequirementRefs: inputs.ContextRequirementRefs,
		InputsProjectionDigest: inputs.ProjectionDigest, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(),
	})
	if err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	next := current.Clone()
	next.Ref.Revision++
	next.State = ports.RestoreAttemptEligibilityBoundV2
	ref := eligibility.Ref
	next.Eligibility = &ref
	next.UpdatedUnixNano = now.UnixNano()
	next, err = ports.SealRestoreAttemptFactV2(next)
	if err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	if err := ValidateRestoreAttemptTransitionV2(current, next); err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	bundle := ports.RestoreEligibilityBindBundleV2{Attempt: next, Eligibility: eligibility}
	return bundle, bundle.Validate()
}

func ValidateRestoreEligibilityTransitionV2(current, next ports.RestoreEligibilityFactV2, now time.Time) error {
	if current.Validate() != nil || next.Validate() != nil || now.IsZero() || current.State != ports.RestoreEligibilityActiveV2 || next.Ref.Revision != current.Ref.Revision+1 || next.Ref.TenantID != current.Ref.TenantID || next.Ref.ID != current.Ref.ID || next.Ref.ExpiresUnixNano != current.Ref.ExpiresUnixNano || next.Attempt != current.Attempt || next.OperationScopeDigest != current.OperationScopeDigest || next.RestorePlan != current.RestorePlan || next.CheckpointConsistency != current.CheckpointConsistency || next.Identity != current.Identity || next.ReviewTarget != current.ReviewTarget || next.InputsProjectionDigest != current.InputsProjectionDigest || next.CreatedUnixNano != current.CreatedUnixNano || next.UpdatedUnixNano < current.UpdatedUnixNano || !sameRestoreExternalSetsV2(current, next) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Restore Eligibility CAS changed immutable exact bindings")
	}
	if next.State != ports.RestoreEligibilityRevokedV2 && next.State != ports.RestoreEligibilitySupersededV2 && next.State != ports.RestoreEligibilityExpiredV2 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Restore Eligibility may only leave active")
	}
	if next.State == ports.RestoreEligibilityExpiredV2 && now.Before(time.Unix(0, current.Ref.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Restore Eligibility cannot expire before its TTL")
	}
	return nil
}

func sameRestoreExternalSetsV2(a, b ports.RestoreEligibilityFactV2) bool {
	return reflectRestoreRefsV2(a.ReviewRequirementRefs, b.ReviewRequirementRefs) && reflectRestoreRefsV2(a.PolicyBasisRefs, b.PolicyBasisRefs) && reflectRestoreRefsV2(a.AuthorityRequirementRefs, b.AuthorityRequirementRefs) && reflectRestoreRefsV2(a.ScopeRequirementRefs, b.ScopeRequirementRefs) && reflectRestoreRefsV2(a.BudgetRequirementRefs, b.BudgetRequirementRefs) && reflectRestoreRefsV2(a.BindingRequirementRefs, b.BindingRequirementRefs) && reflectRestoreRefsV2(a.ContextRequirementRefs, b.ContextRequirementRefs)
}

func reflectRestoreRefsV2(a, b []ports.CheckpointExternalExactFactRefV2) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func minRestoreNanosV2(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func restoreControlInvalidV2(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, message)
}

func restoreControlConflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, message)
}
