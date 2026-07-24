package kernel

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceRestoreOwnerLimitsV1 struct {
	MaxAttemptTTL time.Duration
	MaxHistoryTTL time.Duration
}

type WorkspaceRestoreOwnerV1 struct {
	store      ports.WorkspaceRestoreStoreV1
	bundles    ports.WorkspaceRestoreBundleCurrentReaderV1
	governance ports.WorkspaceRestoreGovernanceCurrentReaderV1
	provider   ports.WorkspaceRestoreProviderV1
	clock      func() time.Time
	limits     WorkspaceRestoreOwnerLimitsV1
}

func NewWorkspaceRestoreOwnerV1(store ports.WorkspaceRestoreStoreV1, bundles ports.WorkspaceRestoreBundleCurrentReaderV1, governance ports.WorkspaceRestoreGovernanceCurrentReaderV1, provider ports.WorkspaceRestoreProviderV1, clock func() time.Time, limits WorkspaceRestoreOwnerLimitsV1) (*WorkspaceRestoreOwnerV1, error) {
	if nilInterface(store) || nilInterface(bundles) || nilInterface(governance) || nilInterface(provider) || clock == nil || limits.MaxAttemptTTL <= 0 || limits.MaxHistoryTTL <= 0 {
		return nil, errors.New("workspace restore Owner dependencies and TTL limits are required")
	}
	return &WorkspaceRestoreOwnerV1{store: store, bundles: bundles, governance: governance, provider: provider, clock: clock, limits: limits}, nil
}

// PrepareWorkspaceV1 reserves the exact Owner attempt without invoking the
// materialization Provider. It lets the Runtime-governed path bind a durable
// Sandbox attempt before actual-point enforcement is evaluated.
func (o *WorkspaceRestoreOwnerV1) PrepareWorkspaceV1(ctx context.Context, input *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreAttemptV1, error) {
	if input == nil {
		return contract.WorkspaceRestoreAttemptV1{}, errors.New("workspace restore prepare request is required")
	}
	request := *input
	now := o.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	attempt, _, err := o.prepareAttempt(ctx, request, now)
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	return attempt.Clone(), nil
}

func (o *WorkspaceRestoreOwnerV1) StageWorkspaceV1(ctx context.Context, input *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreStageFactV1, error) {
	if input == nil {
		return contract.WorkspaceRestoreStageFactV1{}, errors.New("workspace restore stage request is required")
	}
	request := *input
	now := o.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	stable, err := request.StableKeyDigest()
	if err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	attempt, err := o.store.InspectWorkspaceRestoreAttemptByStableKeyV1(ctx, stable)
	if errors.Is(err, ports.ErrNotFound) {
		attempt, _, err = o.prepareAttempt(ctx, request, now)
	}
	if err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	if attempt.Request != request {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: workspace restore stable key binds different request", ports.ErrConflict)
	}
	if isWorkspaceRestoreFinalV1(attempt.State) {
		return o.factForFinalAttempt(ctx, attempt)
	}
	if attempt.State == contract.WorkspaceRestoreAttemptReconcileRequiredV1 {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: original provider attempt requires exact Inspect", ports.ErrUnknownOutcome)
	}

	if attempt.State == contract.WorkspaceRestoreAttemptPreparedV1 {
		attempt, err = o.bindWorkspaceRestoreGovernanceV1(ctx, attempt, now)
		if err != nil {
			return contract.WorkspaceRestoreStageFactV1{}, err
		}
	}

	ownedInvocation := false
	if attempt.State == contract.WorkspaceRestoreAttemptGovernedV1 {
		attempt, ownedInvocation, err = o.recordWorkspaceRestoreInvocationV1(ctx, attempt)
		if err != nil {
			return contract.WorkspaceRestoreStageFactV1{}, err
		}
	}
	if isWorkspaceRestoreFinalV1(attempt.State) {
		return o.factForFinalAttempt(ctx, attempt)
	}
	if attempt.State == contract.WorkspaceRestoreAttemptReconcileRequiredV1 {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: original provider attempt requires exact Inspect", ports.ErrUnknownOutcome)
	}
	if attempt.State != contract.WorkspaceRestoreAttemptInvocationV1 || attempt.Governance == nil {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: workspace restore attempt is not dispatchable", ports.ErrConflict)
	}
	// The persisted governed Attempt is not authority to execute. Immediately
	// before the Provider actual point, independently re-read the Runtime-owned
	// Restore governance closure and require exact equality with the closure
	// sealed into the Attempt. A Lease/Fence/Scope/Permit change therefore
	// fails closed with zero Provider calls.
	actualPoint, err := o.governance.InspectWorkspaceRestoreGovernanceCurrentV1(ctx, request)
	if err != nil || actualPoint.ValidateCurrent(o.clock()) != nil ||
		actualPoint.ProjectionDigest != attempt.GovernanceProjectionDigest ||
		!reflect.DeepEqual(actualPoint, *attempt.Governance) {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: workspace restore actual-point governance is not exact current", ports.ErrConflict)
	}

	bundleExact, err := o.bundles.InspectWorkspaceRestoreBundleExactV1(ctx, request)
	if err != nil || bundleExact.ValidateShape() != nil || bundleExact.ProjectionDigest != attempt.BundleProjectionDigest || bundleExact.Bundle.BundleDigest != attempt.BundleDigest {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: exact workspace bundle is unavailable for provider invocation", ports.ErrConflict)
	}
	providerRef := attempt.ExactRef()
	providerRequest := contract.WorkspaceRestoreProviderRequestV1{StageAttemptRef: providerRef, RuntimeRestoreAttempt: request.RuntimeRestoreAttempt, Target: request.Target, Bundle: bundleExact.Bundle.Clone()}
	if ownedInvocation {
		if _, stageErr := o.provider.StageWorkspaceRestoreV1(ctx, &providerRequest); stageErr != nil {
			if _, inspectErr := o.provider.InspectWorkspaceRestoreV1(context.WithoutCancel(ctx), &providerRequest); inspectErr != nil {
				_ = o.markWorkspaceRestoreReconcileRequiredV1(context.WithoutCancel(ctx), attempt, providerRef)
				return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: provider stage failed (%v) and exact Inspect did not resolve it (%v)", ports.ErrUnknownOutcome, stageErr, inspectErr)
			}
		}
	}
	inspected, err := o.provider.InspectWorkspaceRestoreV1(context.WithoutCancel(ctx), &providerRequest)
	if err != nil {
		if ownedInvocation {
			_ = o.markWorkspaceRestoreReconcileRequiredV1(context.WithoutCancel(ctx), attempt, providerRef)
		}
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: provider response is not independently inspectable", ports.ErrUnknownOutcome)
	}
	return o.finalizeWorkspaceRestoreV1(ctx, attempt, providerRef, bundleExact.Bundle, inspected.RootRef, o.clock())
}

func (o *WorkspaceRestoreOwnerV1) ReconcileWorkspaceV1(ctx context.Context, input *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreStageFactV1, error) {
	if input == nil {
		return contract.WorkspaceRestoreStageFactV1{}, errors.New("workspace restore reconcile request is required")
	}
	request := *input
	if err := request.ValidateShape(); err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	stable, err := request.StableKeyDigest()
	if err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	attempt, err := o.store.InspectWorkspaceRestoreAttemptByStableKeyV1(ctx, stable)
	if err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	if attempt.Request != request {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: workspace restore stable key binds different request", ports.ErrConflict)
	}
	if isWorkspaceRestoreFinalV1(attempt.State) {
		return o.factForFinalAttempt(ctx, attempt)
	}
	if attempt.State != contract.WorkspaceRestoreAttemptInvocationV1 && attempt.State != contract.WorkspaceRestoreAttemptReconcileRequiredV1 {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: workspace restore has no recorded provider invocation", ports.ErrConflict)
	}
	bundleExact, err := o.bundles.InspectWorkspaceRestoreBundleExactV1(ctx, request)
	if err != nil || bundleExact.ValidateShape() != nil || bundleExact.ProjectionDigest != attempt.BundleProjectionDigest || bundleExact.Bundle.BundleDigest != attempt.BundleDigest {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: exact workspace bundle is unavailable during reconciliation", ports.ErrConflict)
	}
	providerRef := attempt.ExactRef()
	if attempt.State == contract.WorkspaceRestoreAttemptReconcileRequiredV1 {
		providerRef = *attempt.ProviderStageAttemptRef
	}
	providerRequest := contract.WorkspaceRestoreProviderRequestV1{StageAttemptRef: providerRef, RuntimeRestoreAttempt: request.RuntimeRestoreAttempt, Target: request.Target, Bundle: bundleExact.Bundle.Clone()}
	inspected, err := o.provider.InspectWorkspaceRestoreV1(context.WithoutCancel(ctx), &providerRequest)
	if err != nil {
		_ = o.markWorkspaceRestoreReconcileRequiredV1(context.WithoutCancel(ctx), attempt, providerRef)
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: original provider attempt remains unresolved: %v", ports.ErrUnknownOutcome, err)
	}
	return o.finalizeWorkspaceRestoreV1(ctx, attempt, providerRef, bundleExact.Bundle, inspected.RootRef, o.clock())
}

func (o *WorkspaceRestoreOwnerV1) prepareAttempt(ctx context.Context, request contract.WorkspaceRestoreStageRequestV1, now time.Time) (contract.WorkspaceRestoreAttemptV1, contract.WorkspaceRestoreBundleCurrentProjectionV1, error) {
	stable, err := request.StableKeyDigest()
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}
	if existing, err := o.store.InspectWorkspaceRestoreAttemptByStableKeyV1(ctx, stable); err == nil {
		if existing.Request != request {
			return contract.WorkspaceRestoreAttemptV1{}, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, fmt.Errorf("%w: workspace restore stable key binds different request", ports.ErrConflict)
		}
		return existing, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, nil
	} else if !errors.Is(err, ports.ErrNotFound) {
		return contract.WorkspaceRestoreAttemptV1{}, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}

	bundle, err := o.bundles.InspectWorkspaceRestoreBundleCurrentV1(ctx, request)
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}
	if err := bundle.ValidateCurrent(now); err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}
	if !contract.SameSnapshotArtifactExactRef(bundle.SnapshotArtifactFactRef, request.SnapshotArtifactFactRef) || bundle.TenantID != request.TenantID {
		return contract.WorkspaceRestoreAttemptV1{}, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, fmt.Errorf("%w: workspace restore bundle current crosses request coordinates", ports.ErrConflict)
	}
	expires := minimumWorkspaceRestoreTimeV1(request.RequestedNotAfter, bundle.ExpiresUnixNano, request.RuntimeRestoreAttempt.ExpiresUnixNano, request.RestoreEligibility.ExpiresUnixNano, request.Target.ExpiresUnixNano, now.Add(o.limits.MaxAttemptTTL).UnixNano())
	if now.UnixNano() >= expires {
		return contract.WorkspaceRestoreAttemptV1{}, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, fmt.Errorf("%w: workspace restore attempt TTL is exhausted", ports.ErrStale)
	}
	attempt, err := contract.SealWorkspaceRestoreAttemptV1(contract.WorkspaceRestoreAttemptV1{
		Meta:            contract.Meta{ContractVersion: contract.ContractFamily, ID: request.DispatchAttemptID, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires},
		StableKeyDigest: stable, Request: request, BundleProjectionDigest: bundle.ProjectionDigest, BundleDigest: bundle.Bundle.BundleDigest, State: contract.WorkspaceRestoreAttemptPreparedV1,
	})
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}
	created, err := o.store.CreateWorkspaceRestoreAttemptV1(ctx, attempt)
	if err == nil && created {
		return attempt, bundle, nil
	}
	recovered, inspectErr := o.store.InspectWorkspaceRestoreAttemptByStableKeyV1(context.WithoutCancel(ctx), stable)
	if inspectErr == nil && recovered.Request == request {
		return recovered, bundle, nil
	}
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}
	if inspectErr != nil {
		return contract.WorkspaceRestoreAttemptV1{}, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, inspectErr
	}
	return contract.WorkspaceRestoreAttemptV1{}, contract.WorkspaceRestoreBundleCurrentProjectionV1{}, fmt.Errorf("%w: workspace restore create-once winner differs", ports.ErrConflict)
}

func (o *WorkspaceRestoreOwnerV1) bindWorkspaceRestoreGovernanceV1(ctx context.Context, attempt contract.WorkspaceRestoreAttemptV1, now time.Time) (contract.WorkspaceRestoreAttemptV1, error) {
	bundleS1, err := o.bundles.InspectWorkspaceRestoreBundleCurrentV1(ctx, attempt.Request)
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	governanceS1, err := o.governance.InspectWorkspaceRestoreGovernanceCurrentV1(ctx, attempt.Request)
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	if err := validateWorkspaceRestoreCurrentsV1(attempt.Request, bundleS1, governanceS1, now); err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	bundleS2, err := o.bundles.InspectWorkspaceRestoreBundleCurrentV1(ctx, attempt.Request)
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	governanceS2, err := o.governance.InspectWorkspaceRestoreGovernanceCurrentV1(ctx, attempt.Request)
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	fresh := o.clock()
	if err := validateWorkspaceRestoreCurrentsV1(attempt.Request, bundleS2, governanceS2, fresh); err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	if bundleS1.ProjectionDigest != bundleS2.ProjectionDigest || governanceS1.ProjectionDigest != governanceS2.ProjectionDigest || bundleS2.ProjectionDigest != attempt.BundleProjectionDigest || bundleS2.Bundle.BundleDigest != attempt.BundleDigest {
		return contract.WorkspaceRestoreAttemptV1{}, fmt.Errorf("%w: workspace restore S1/S2 current projection drifted", ports.ErrConflict)
	}
	next := attempt.Clone()
	next.Meta.Revision++
	next.Meta.UpdatedUnixNano = fresh.UnixNano()
	next.State = contract.WorkspaceRestoreAttemptGovernedV1
	next.GovernanceProjectionDigest = governanceS2.ProjectionDigest
	next.Governance = &governanceS2
	next, err = contract.SealWorkspaceRestoreAttemptV1(next)
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	created, casErr := o.store.CASWorkspaceRestoreAttemptV1(ctx, attempt.ExactRef(), next)
	if casErr == nil && created {
		return next, nil
	}
	recovered, inspectErr := o.store.InspectWorkspaceRestoreAttemptByStableKeyV1(context.WithoutCancel(ctx), attempt.StableKeyDigest)
	if inspectErr == nil && recovered.Request == attempt.Request && recovered.State != contract.WorkspaceRestoreAttemptPreparedV1 {
		return recovered, nil
	}
	if casErr != nil {
		return contract.WorkspaceRestoreAttemptV1{}, casErr
	}
	if inspectErr != nil {
		return contract.WorkspaceRestoreAttemptV1{}, inspectErr
	}
	return contract.WorkspaceRestoreAttemptV1{}, fmt.Errorf("%w: workspace restore governance CAS winner differs", ports.ErrConflict)
}

func (o *WorkspaceRestoreOwnerV1) recordWorkspaceRestoreInvocationV1(ctx context.Context, attempt contract.WorkspaceRestoreAttemptV1) (contract.WorkspaceRestoreAttemptV1, bool, error) {
	next := attempt.Clone()
	next.Meta.Revision++
	next.Meta.UpdatedUnixNano = o.clock().UnixNano()
	next.State = contract.WorkspaceRestoreAttemptInvocationV1
	next, err := contract.SealWorkspaceRestoreAttemptV1(next)
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, false, err
	}
	created, casErr := o.store.CASWorkspaceRestoreAttemptV1(ctx, attempt.ExactRef(), next)
	if casErr == nil && created {
		return next, true, nil
	}
	recovered, inspectErr := o.store.InspectWorkspaceRestoreAttemptByStableKeyV1(context.WithoutCancel(ctx), attempt.StableKeyDigest)
	if inspectErr == nil && recovered.Request == attempt.Request && (recovered.State == contract.WorkspaceRestoreAttemptInvocationV1 || recovered.State == contract.WorkspaceRestoreAttemptReconcileRequiredV1 || isWorkspaceRestoreFinalV1(recovered.State)) {
		return recovered, false, nil
	}
	if casErr != nil {
		return contract.WorkspaceRestoreAttemptV1{}, false, casErr
	}
	if inspectErr != nil {
		return contract.WorkspaceRestoreAttemptV1{}, false, inspectErr
	}
	return contract.WorkspaceRestoreAttemptV1{}, false, fmt.Errorf("%w: workspace restore invocation CAS winner differs", ports.ErrConflict)
}

func validateWorkspaceRestoreCurrentsV1(request contract.WorkspaceRestoreStageRequestV1, bundle contract.WorkspaceRestoreBundleCurrentProjectionV1, governance contract.WorkspaceRestoreGovernanceCurrentProjectionV1, now time.Time) error {
	if err := bundle.ValidateCurrent(now); err != nil {
		return err
	}
	if err := governance.ValidateCurrent(now); err != nil {
		return err
	}
	if !contract.SameSnapshotArtifactExactRef(bundle.SnapshotArtifactFactRef, request.SnapshotArtifactFactRef) || bundle.TenantID != request.TenantID || !governance.MatchesRequest(request) {
		return fmt.Errorf("%w: workspace restore Owner current projection crosses request coordinates", ports.ErrConflict)
	}
	return nil
}

func (o *WorkspaceRestoreOwnerV1) markWorkspaceRestoreReconcileRequiredV1(ctx context.Context, attempt contract.WorkspaceRestoreAttemptV1, providerRef contract.SnapshotArtifactExactRefV2) error {
	if attempt.State == contract.WorkspaceRestoreAttemptReconcileRequiredV1 || isWorkspaceRestoreFinalV1(attempt.State) {
		return nil
	}
	if attempt.State != contract.WorkspaceRestoreAttemptInvocationV1 || providerRef != attempt.ExactRef() {
		return ports.ErrConflict
	}
	now := o.clock()
	next := attempt.Clone()
	next.Meta.Revision++
	next.Meta.UpdatedUnixNano = now.UnixNano()
	next.State = contract.WorkspaceRestoreAttemptReconcileRequiredV1
	next.ProviderStageAttemptRef = &providerRef
	next, err := contract.SealWorkspaceRestoreAttemptV1(next)
	if err != nil {
		return err
	}
	_, err = o.store.CASWorkspaceRestoreAttemptV1(ctx, attempt.ExactRef(), next)
	return err
}

func (o *WorkspaceRestoreOwnerV1) finalizeWorkspaceRestoreV1(ctx context.Context, attempt contract.WorkspaceRestoreAttemptV1, providerRef contract.SnapshotArtifactExactRefV2, bundle contract.WorkspaceSnapshotBundleV1, root contract.WorkspaceRootRefV1, now time.Time) (contract.WorkspaceRestoreStageFactV1, error) {
	if now.IsZero() || attempt.Governance == nil || root.StageAttemptRef != providerRef || root.RuntimeRestoreAttempt != attempt.Request.RuntimeRestoreAttempt || root.Target != attempt.Request.Target || root.BundleDigest != attempt.BundleDigest {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: independently inspected workspace root crosses exact attempt", ports.ErrConflict)
	}
	state := contract.WorkspaceRestoreStageCompleteV1
	attemptState := contract.WorkspaceRestoreAttemptStagedV1
	if len(bundle.Excluded) != 0 {
		state = contract.WorkspaceRestoreStagePartialV1
		attemptState = contract.WorkspaceRestoreAttemptPartialV1
	}
	fact, err := contract.SealWorkspaceRestoreStageFactV1(contract.WorkspaceRestoreStageFactV1{
		Meta:     contract.Meta{ContractVersion: contract.ContractFamily, ID: attempt.Meta.ID + "-fact", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(o.limits.MaxHistoryTTL).UnixNano()},
		TenantID: attempt.Request.TenantID, AttemptRef: providerRef, RuntimeRestoreAttempt: attempt.Request.RuntimeRestoreAttempt, RestoreEligibility: attempt.Request.RestoreEligibility, Target: attempt.Request.Target, SnapshotArtifactFactRef: attempt.Request.SnapshotArtifactFactRef,
		BundleDigest: bundle.BundleDigest, RootRef: root, Governance: *attempt.Governance, State: state, Residuals: append([]contract.WorkspaceSnapshotExcludedV1(nil), bundle.Excluded...),
	})
	if err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	next := attempt.Clone()
	next.Meta.Revision++
	next.Meta.UpdatedUnixNano = now.UnixNano()
	next.State = attemptState
	next.RootRef = &root
	next.ProviderStageAttemptRef = &providerRef
	factRef := fact.ExactRef()
	next.StageFactRef = &factRef
	next, err = contract.SealWorkspaceRestoreAttemptV1(next)
	if err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	created, err := o.store.CommitWorkspaceRestoreStageV1(ctx, attempt.ExactRef(), next, fact)
	if err == nil && created {
		return fact.Clone(), nil
	}
	recoveredAttempt, inspectErr := o.store.InspectWorkspaceRestoreAttemptByStableKeyV1(context.WithoutCancel(ctx), attempt.StableKeyDigest)
	if inspectErr == nil && isWorkspaceRestoreFinalV1(recoveredAttempt.State) {
		return o.factForFinalAttempt(context.WithoutCancel(ctx), recoveredAttempt)
	}
	if err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	if inspectErr != nil {
		return contract.WorkspaceRestoreStageFactV1{}, inspectErr
	}
	return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: workspace restore final CAS winner differs", ports.ErrConflict)
}

func (o *WorkspaceRestoreOwnerV1) factForFinalAttempt(ctx context.Context, attempt contract.WorkspaceRestoreAttemptV1) (contract.WorkspaceRestoreStageFactV1, error) {
	if attempt.StageFactRef == nil {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: final workspace restore attempt lacks fact ref", ports.ErrConflict)
	}
	fact, err := o.store.InspectWorkspaceRestoreStageFactV1(ctx, *attempt.StageFactRef)
	if err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	if fact.ValidateShape() != nil || fact.ExactRef() != *attempt.StageFactRef || fact.RootRef != *attempt.RootRef || fact.BundleDigest != attempt.BundleDigest || fact.RuntimeRestoreAttempt != attempt.Request.RuntimeRestoreAttempt || attempt.ProviderStageAttemptRef == nil || fact.AttemptRef != *attempt.ProviderStageAttemptRef || attempt.Governance == nil || fact.Governance.ProjectionDigest != attempt.GovernanceProjectionDigest {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: workspace restore final fact drifted", ports.ErrConflict)
	}
	return fact.Clone(), nil
}

func (o *WorkspaceRestoreOwnerV1) InspectWorkspaceRestoreAttemptV1(ctx context.Context, input *contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreAttemptV1, error) {
	if input == nil {
		return contract.WorkspaceRestoreAttemptV1{}, errors.New("workspace restore attempt inspect ref is required")
	}
	if err := input.ValidateShape("workspace restore attempt"); err != nil || input.TypeURL != contract.WorkspaceRestoreAttemptTypeURLV1 {
		return contract.WorkspaceRestoreAttemptV1{}, errors.New("workspace restore attempt inspect ref is invalid")
	}
	attempt, err := o.store.InspectWorkspaceRestoreAttemptV1(ctx, *input)
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	if attempt.ValidateShape() != nil || attempt.ExactRef() != *input {
		return contract.WorkspaceRestoreAttemptV1{}, fmt.Errorf("%w: workspace restore attempt exact Inspect drifted", ports.ErrConflict)
	}
	return attempt.Clone(), nil
}

func (o *WorkspaceRestoreOwnerV1) InspectWorkspaceRestoreStageFactV1(ctx context.Context, input *contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreStageFactV1, error) {
	if input == nil {
		return contract.WorkspaceRestoreStageFactV1{}, errors.New("workspace restore stage fact inspect ref is required")
	}
	fact, err := o.store.InspectWorkspaceRestoreStageFactV1(ctx, *input)
	if err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	if fact.ValidateShape() != nil || fact.ExactRef() != *input {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: workspace restore stage fact exact Inspect drifted", ports.ErrConflict)
	}
	return fact.Clone(), nil
}

func isWorkspaceRestoreFinalV1(state contract.WorkspaceRestoreAttemptStateV1) bool {
	return state == contract.WorkspaceRestoreAttemptStagedV1 || state == contract.WorkspaceRestoreAttemptPartialV1
}

func minimumWorkspaceRestoreTimeV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

var _ ports.WorkspaceRestoreOwnerPortV1 = (*WorkspaceRestoreOwnerV1)(nil)
