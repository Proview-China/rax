package runtimeadapter

import (
	"cmp"
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	RestoreContextGenerationExactSchemaV1 = "praxis.context/restored-context-generation/v1"
	RestoreContextFrameExactSchemaV1      = "praxis.context/restored-context-frame/v1"
	RestoreContextResidualExactSchemaV1   = "praxis.context/restore-context-residual/v1"
)

type RestoreContextExactRouteV1 struct {
	ContractVersion string
	SchemaRef       string
	Owner           runtimeports.CheckpointManifestSealOwnerBindingV2
}

func (r RestoreContextExactRouteV1) validateV1() error {
	if r.ContractVersion == "" || r.SchemaRef == "" || r.Owner.Validate() != nil {
		return fmt.Errorf("%w: Context Restore exact route", contract.ErrInvalid)
	}
	return nil
}

// RestoreContextMaterializationAdapterV1 is the Context-owned implementation
// of Application's neutral coordination Port. It rereads Context source facts
// and all current requirements through the Owner service before publishing a
// new Generation. It never activates Runtime or executes a Provider.
type RestoreContextMaterializationAdapterV1 struct {
	owner                 contextports.RestoreContextMaterializationOwnerPortV1
	factOwner             runtimeports.ProviderBindingRefV2
	sourceGenerationRoute RestoreContextExactRouteV1
	sourceFrameRoute      RestoreContextExactRouteV1
	targetGenerationRoute RestoreContextExactRouteV1
	targetFrameRoute      RestoreContextExactRouteV1
	residualRoute         RestoreContextExactRouteV1
	clock                 func() time.Time
}

func NewRestoreContextMaterializationAdapterV1(owner contextports.RestoreContextMaterializationOwnerPortV1, factOwner runtimeports.ProviderBindingRefV2, sourceGenerationRoute, sourceFrameRoute, targetGenerationRoute, targetFrameRoute, residualRoute RestoreContextExactRouteV1, clock func() time.Time) (*RestoreContextMaterializationAdapterV1, error) {
	if restoreAdapterNilV1(owner) || factOwner.Validate() != nil || sourceGenerationRoute.validateV1() != nil || sourceFrameRoute.validateV1() != nil || targetGenerationRoute.validateV1() != nil || targetFrameRoute.validateV1() != nil || residualRoute.validateV1() != nil || clock == nil {
		return nil, fmt.Errorf("%w: Context Restore adapter dependencies", contract.ErrInvalid)
	}
	return &RestoreContextMaterializationAdapterV1{owner: owner, factOwner: factOwner, sourceGenerationRoute: sourceGenerationRoute, sourceFrameRoute: sourceFrameRoute, targetGenerationRoute: targetGenerationRoute, targetFrameRoute: targetFrameRoute, residualRoute: residualRoute, clock: clock}, nil
}

func (a *RestoreContextMaterializationAdapterV1) MaterializeRestoreContextV1(ctx context.Context, input applicationcontract.RestoreContextMaterializationRequestV1) (runtimeports.RestoreContextMaterializationCurrentProjectionV1, error) {
	if a == nil {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, fmt.Errorf("%w: Context Restore adapter", contract.ErrInvalid)
	}
	now := a.clock()
	if err := input.ValidateCurrent(now); err != nil {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, err
	}
	if !matchesRestoreContextRouteV1(input.Materialization.ContextGeneration, a.sourceGenerationRoute) {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, fmt.Errorf("%w: source Context Generation Owner route", contract.ErrConflict)
	}
	sourceFrames := make([]contract.FactRef, len(input.Materialization.ContextFrames))
	for index, ref := range input.Materialization.ContextFrames {
		if !matchesRestoreContextRouteV1(ref, a.sourceFrameRoute) {
			return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, fmt.Errorf("%w: source Context Frame Owner route", contract.ErrConflict)
		}
		local, err := restoreContextLocalRefV1(ref.ID, uint64(ref.Revision), ref.Digest)
		if err != nil {
			return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, err
		}
		sourceFrames[index] = local
	}
	sort.Slice(sourceFrames, func(i, j int) bool { return compareContextFactRefV1(sourceFrames[i], sourceFrames[j]) < 0 })
	sourceGeneration, err := restoreContextLocalRefV1(input.Materialization.ContextGeneration.ID, uint64(input.Materialization.ContextGeneration.Revision), input.Materialization.ContextGeneration.Digest)
	if err != nil {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, err
	}
	requirements := make([]contract.RestoreContextRequirementRefV1, len(input.Requirements))
	for index, requirement := range input.Requirements {
		local, refErr := restoreContextLocalRefV1(requirement.Ref.ID, uint64(requirement.Ref.Revision), requirement.Ref.Digest)
		if refErr != nil {
			return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, refErr
		}
		routeDigest, digestErr := contract.DigestJSON(requirement.Ref)
		if digestErr != nil {
			return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, digestErr
		}
		kind, kindErr := restoreContextRequirementKindV1(requirement.Kind)
		if kindErr != nil {
			return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, kindErr
		}
		requirements[index] = contract.RestoreContextRequirementRefV1{Kind: kind, Ref: local, RouteDigest: routeDigest}
	}
	target := contract.RestoreContextTargetBindingV1{
		TenantID: string(input.Materialization.Attempt.TenantID), ScopeDigest: contract.Digest(input.Stage.Fact.Operation.ExecutionScopeDigest),
		InstanceID: string(input.Materialization.Identity.TargetInstance.ID), InstanceEpoch: uint64(input.Materialization.Identity.TargetInstance.Epoch),
		LeaseID: string(input.Materialization.Identity.TargetLease.ID), LeaseEpoch: uint64(input.Materialization.Identity.TargetLease.Epoch), FenceEpoch: uint64(input.Materialization.Identity.TargetFenceEpoch),
	}
	request := contract.RestoreContextMaterializationRequestV1{
		ID: input.ID, IdempotencyKey: input.IdempotencyKey, TenantID: target.TenantID,
		Attempt:      restoreContextRuntimeRefV1(input.Materialization.Attempt.ID, uint64(input.Materialization.Attempt.Revision), input.Materialization.Attempt.Digest),
		Eligibility:  restoreContextRuntimeRefV1(input.Materialization.Eligibility.ID, uint64(input.Materialization.Eligibility.Revision), input.Materialization.Eligibility.Digest),
		Stage:        restoreContextRuntimeRefV1(input.Stage.Fact.ID, uint64(input.Stage.Fact.Revision), input.Stage.Fact.Digest),
		SandboxApply: restoreContextRuntimeRefV1(input.SandboxSettlement.Fact.ID, uint64(input.SandboxSettlement.Fact.Revision), input.SandboxSettlement.Fact.Digest),
		SourceScope:  contract.Digest(input.Materialization.SourceScopeDigest), Target: target,
		SourceGeneration: sourceGeneration, SourceFrames: sourceFrames, Requirements: requirements,
		RequestedUnixNano: input.RequestedUnixNano, NotAfterUnixNano: minimumRestoreAdapterTimeV1(input.NotAfterUnixNano, input.Materialization.ExpiresUnixNano, input.Stage.ExpiresUnixNano, input.SandboxSettlement.ExpiresUnixNano),
	}
	fact, err := a.owner.MaterializeRestoreContextV1(ctx, request)
	if err != nil {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, err
	}
	return a.projectV1(fact, input.Materialization, now)
}

func (a *RestoreContextMaterializationAdapterV1) InspectRestoreContextMaterializationV1(ctx context.Context, expected runtimeports.RestoreContextMaterializationRefV1) (runtimeports.RestoreContextMaterializationCurrentProjectionV1, error) {
	return a.InspectRestoreContextMaterializationCurrentV1(ctx, expected)
}

func (a *RestoreContextMaterializationAdapterV1) InspectRestoreContextMaterializationCurrentV1(ctx context.Context, expected runtimeports.RestoreContextMaterializationRefV1) (runtimeports.RestoreContextMaterializationCurrentProjectionV1, error) {
	if a == nil || expected.Validate() != nil || expected.Owner != a.factOwner || !matchesRestoreContextRouteV1(expected.SourceGeneration, a.sourceGenerationRoute) || !matchesRestoreContextRouteV1(expected.TargetGeneration, a.targetGenerationRoute) {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, fmt.Errorf("%w: Context Restore exact inspection coordinates", contract.ErrConflict)
	}
	for _, frame := range expected.TargetFrames {
		if !matchesRestoreContextRouteV1(frame, a.targetFrameRoute) {
			return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, fmt.Errorf("%w: Context Restore target Frame route", contract.ErrConflict)
		}
	}
	local, err := restoreContextLocalRefV1(expected.ID, uint64(expected.Revision), string(expected.Digest))
	if err != nil {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, err
	}
	fact, err := a.owner.InspectRestoreContextMaterializationV1(ctx, local)
	if err != nil {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, err
	}
	materialization := runtimeports.RestoreMaterializationCurrentProjectionV1{
		Attempt: expected.Attempt, Eligibility: expected.Eligibility, SourceScopeDigest: expected.SourceScopeDigest, Identity: expected.Identity,
		ContextGeneration: expected.SourceGeneration,
	}
	projection, err := a.projectV1(fact, materialization, a.clock())
	if err != nil {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(projection.Fact, expected) {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, fmt.Errorf("%w: Context Restore exact fact drifted", contract.ErrConflict)
	}
	return projection, nil
}

func (a *RestoreContextMaterializationAdapterV1) projectV1(fact contract.RestoreContextMaterializationFactV1, source runtimeports.RestoreMaterializationCurrentProjectionV1, now time.Time) (runtimeports.RestoreContextMaterializationCurrentProjectionV1, error) {
	if fact.Validate() != nil || source.Attempt.Validate() != nil || source.Eligibility.Validate() != nil || source.Identity.Validate() != nil || source.SourceScopeDigest.Validate() != nil || source.ContextGeneration.Validate() != nil || now.IsZero() {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, fmt.Errorf("%w: Context Restore projection inputs", contract.ErrInvalid)
	}
	if fact.Attempt != restoreContextRuntimeRefV1(source.Attempt.ID, uint64(source.Attempt.Revision), source.Attempt.Digest) || fact.Eligibility != restoreContextRuntimeRefV1(source.Eligibility.ID, uint64(source.Eligibility.Revision), source.Eligibility.Digest) || fact.SourceGeneration != restoreContextRuntimeRefV1(source.ContextGeneration.ID, uint64(source.ContextGeneration.Revision), runtimecore.Digest(source.ContextGeneration.Digest)) {
		return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, fmt.Errorf("%w: Context Restore source closure", contract.ErrConflict)
	}
	targetGeneration := restoreContextExternalRefV1(a.targetGenerationRoute, fact.Target.TenantID, string(fact.Target.ScopeDigest), fact.TargetGeneration)
	targetFrames := make([]runtimeports.CheckpointExternalExactFactRefV2, len(fact.TargetFrames))
	for index, frame := range fact.TargetFrames {
		targetFrames[index] = restoreContextExternalRefV1(a.targetFrameRoute, fact.Target.TenantID, string(fact.Target.ScopeDigest), frame)
	}
	sortRestoreContextExternalRefsV1(targetFrames)
	residuals := make([]runtimeports.CheckpointExternalExactFactRefV2, len(fact.Requirements.Residuals))
	for index, residual := range fact.Requirements.Residuals {
		residuals[index] = restoreContextExternalRefV1(a.residualRoute, fact.Target.TenantID, string(fact.Target.ScopeDigest), residual)
	}
	sortRestoreContextExternalRefsV1(residuals)
	ref := runtimeports.RestoreContextMaterializationRefV1{
		Owner: a.factOwner, ID: fact.ID, Revision: runtimecore.Revision(fact.Revision), Digest: runtimecore.Digest(fact.Digest), TenantID: source.Attempt.TenantID,
		Attempt: source.Attempt, Eligibility: source.Eligibility, Identity: source.Identity,
		SourceScopeDigest: source.SourceScopeDigest, TargetScopeDigest: runtimecore.Digest(fact.Target.ScopeDigest), SourceGeneration: source.ContextGeneration,
		TargetGeneration: targetGeneration, TargetFrames: targetFrames, CurrentDigest: runtimecore.Digest(fact.CurrentDigest),
	}
	return runtimeports.SealRestoreContextMaterializationCurrentProjectionV1(runtimeports.RestoreContextMaterializationCurrentProjectionV1{Fact: ref, Residuals: residuals, CheckedUnixNano: fact.CreatedUnixNano, ExpiresUnixNano: fact.ExpiresUnixNano}, now)
}

func restoreContextRequirementKindV1(value applicationcontract.RestoreContextRequirementKindV1) (contract.RestoreContextRequirementKindV1, error) {
	switch value {
	case applicationcontract.RestoreContextRequirementProfileV1:
		return contract.RestoreRequirementProfileV1, nil
	case applicationcontract.RestoreContextRequirementToolV1:
		return contract.RestoreRequirementToolV1, nil
	case applicationcontract.RestoreContextRequirementMCPV1:
		return contract.RestoreRequirementMCPV1, nil
	case applicationcontract.RestoreContextRequirementReviewV1:
		return contract.RestoreRequirementReviewV1, nil
	case applicationcontract.RestoreContextRequirementAuthorityV1:
		return contract.RestoreRequirementAuthorityV1, nil
	case applicationcontract.RestoreContextRequirementBudgetV1:
		return contract.RestoreRequirementBudgetV1, nil
	case applicationcontract.RestoreContextRequirementBindingV1:
		return contract.RestoreRequirementBindingV1, nil
	default:
		return "", fmt.Errorf("%w: Context Restore requirement kind", contract.ErrInvalid)
	}
}

func restoreContextLocalRefV1(id string, revision uint64, digest string) (contract.FactRef, error) {
	normalized, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(digest)
	if err != nil {
		return contract.FactRef{}, err
	}
	ref := contract.FactRef{ID: id, Revision: revision, Digest: contract.Digest(normalized)}
	return ref, ref.Validate()
}

func restoreContextRuntimeRefV1(id string, revision uint64, digest runtimecore.Digest) contract.FactRef {
	return contract.FactRef{ID: id, Revision: revision, Digest: contract.Digest(digest)}
}

func restoreContextExternalRefV1(route RestoreContextExactRouteV1, tenant, scope string, ref contract.FactRef) runtimeports.CheckpointExternalExactFactRefV2 {
	return runtimeports.CheckpointExternalExactFactRefV2{ContractVersion: route.ContractVersion, SchemaRef: route.SchemaRef, Owner: route.Owner, TenantID: tenant, ID: ref.ID, Revision: runtimecore.Revision(ref.Revision), Digest: string(ref.Digest), ScopeDigest: scope}
}

func matchesRestoreContextRouteV1(ref runtimeports.CheckpointExternalExactFactRefV2, route RestoreContextExactRouteV1) bool {
	return ref.Validate() == nil && route.validateV1() == nil && ref.ContractVersion == route.ContractVersion && ref.SchemaRef == route.SchemaRef && ref.Owner == route.Owner
}

func sortRestoreContextExternalRefsV1(values []runtimeports.CheckpointExternalExactFactRefV2) {
	sort.Slice(values, func(i, j int) bool { return compareRestoreContextExternalRefV1(values[i], values[j]) < 0 })
}

func compareRestoreContextExternalRefV1(left, right runtimeports.CheckpointExternalExactFactRefV2) int {
	strings := [][2]string{{left.TenantID, right.TenantID}, {left.ScopeDigest, right.ScopeDigest}, {left.ContractVersion, right.ContractVersion}, {left.SchemaRef, right.SchemaRef}, {left.Owner.BindingSetID, right.Owner.BindingSetID}, {left.Owner.ComponentID, right.Owner.ComponentID}, {left.Owner.ManifestDigest, right.Owner.ManifestDigest}, {left.Owner.ArtifactDigest, right.Owner.ArtifactDigest}, {left.Owner.Capability, right.Owner.Capability}, {left.Owner.FactKind, right.Owner.FactKind}, {left.ID, right.ID}, {left.Digest, right.Digest}}
	for _, pair := range strings {
		if result := cmp.Compare(pair[0], pair[1]); result != 0 {
			return result
		}
	}
	if result := cmp.Compare(left.Owner.BindingRevision, right.Owner.BindingRevision); result != 0 {
		return result
	}
	return cmp.Compare(left.Revision, right.Revision)
}

func compareContextFactRefV1(left, right contract.FactRef) int {
	if result := cmp.Compare(left.ID, right.ID); result != 0 {
		return result
	}
	if result := cmp.Compare(left.Revision, right.Revision); result != 0 {
		return result
	}
	return cmp.Compare(left.Digest, right.Digest)
}

func minimumRestoreAdapterTimeV1(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func restoreAdapterNilV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var _ applicationports.RestoreContextMaterializationPortV1 = (*RestoreContextMaterializationAdapterV1)(nil)
var _ runtimeports.RestoreContextMaterializationCurrentReaderV1 = (*RestoreContextMaterializationAdapterV1)(nil)
