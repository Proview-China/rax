// Package deployment owns the public HostDeploymentCurrentV2 projection.
// It revalidates every external current at the public boundary and delegates
// only append-only history/current CAS persistence to the raw repository.
package deployment

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"time"

	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type CurrentOwnerConfigV2 struct {
	Builder    hostports.HostDeploymentBuilderEvidenceV2
	Resources  hostports.HostDeploymentResourceCurrentReaderV2
	Services   hostports.HostDeploymentServiceCurrentReaderV2
	Repository hostports.HostDeploymentCurrentRepositoryV2
	Clock      func() time.Time
}

type CurrentOwnerV2 struct {
	builder    hostports.HostDeploymentBuilderEvidenceV2
	resources  hostports.HostDeploymentResourceCurrentReaderV2
	services   hostports.HostDeploymentServiceCurrentReaderV2
	repository hostports.HostDeploymentCurrentRepositoryV2
	clock      func() time.Time
}

func NewCurrentOwnerV2(config CurrentOwnerConfigV2) (*CurrentOwnerV2, error) {
	if contract.IsTypedNilV1(config.Builder) ||
		contract.IsTypedNilV1(config.Resources) ||
		contract.IsTypedNilV1(config.Services) ||
		contract.IsTypedNilV1(config.Repository) {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "host_deployment_v2_dependency_missing", "Host deployment current V2 requires exact Builder, resource, service and repository readers")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &CurrentOwnerV2{
		builder:    config.Builder,
		resources:  config.Resources,
		services:   config.Services,
		repository: config.Repository,
		clock:      config.Clock,
	}, nil
}

func (owner *CurrentOwnerV2) PublishHostDeploymentCurrentV2(
	ctx context.Context,
	request contract.PublishHostDeploymentCurrentRequestV2,
) (contract.HostDeploymentCurrentV2, error) {
	if err := owner.readyV2(ctx); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	request = contract.ClonePublishHostDeploymentCurrentRequestV2(request)
	baseline := owner.clock()
	if err := request.ValidateCurrentV2(baseline); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}

	first, err := owner.readFreshSourcesV2(ctx, request.PackageSelectionRef, request.ResourceHandles, request.ServiceBindings, baseline)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}

	secondNow := owner.clock()
	if secondNow.IsZero() || secondNow.Before(baseline) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "Host deployment current V2 S2 clock regressed")
	}
	second, err := owner.readFreshSourcesV2(ctx, request.PackageSelectionRef, request.ResourceHandles, request.ServiceBindings, secondNow)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if !first.equalV2(second) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_source_drift", "Host deployment current V2 source changed between S1 and S2")
	}

	sealNow := owner.clock()
	if sealNow.IsZero() || sealNow.Before(secondNow) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "Host deployment current V2 seal clock regressed")
	}
	expires := request.Bootstrap.NotAfterUnixNano
	expires = minV2(expires, request.RequestedNotAfterUnixNano)
	expires = minV2(expires, second.expiresUnixNano)
	if sealNow.UnixNano() >= expires {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorPrecondition, "host_deployment_v2_expired", "Host deployment current V2 expired before sealing")
	}

	revision := uint64(1)
	if !request.ExpectedCurrent.IsZero() {
		if request.ExpectedCurrent.Revision == ^uint64(0) {
			return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorInvalidArgument, "host_deployment_v2_revision_overflow", "Host deployment current V2 revision overflowed")
		}
		revision = request.ExpectedCurrent.Revision + 1
	}
	desired, err := contract.SealHostDeploymentCurrentV2(contract.HostDeploymentCurrentV2{
		Ref: contract.HostDeploymentCurrentRefV2{
			HostID:              request.Bootstrap.HostID,
			DeploymentID:        request.DeploymentID,
			Revision:            revision,
			BootstrapDigest:     request.Bootstrap.ContentDigest,
			PackageSelectionRef: request.PackageSelectionRef,
			ExpiresUnixNano:     expires,
		},
		ResourceHandles: request.ResourceHandles,
		ServiceBindings: request.ServiceBindings,
		CheckedUnixNano: sealNow.UnixNano(),
		ExpiresUnixNano: expires,
	})
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}

	result, writeErr := safeCompareAndSwapV2(ctx, owner.repository, request.ExpectedCurrent, desired)
	if writeErr == nil {
		result = contract.CloneHostDeploymentCurrentV2(result)
		if !reflect.DeepEqual(result, desired) {
			return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_repository_drift", "Host deployment current V2 repository returned another body")
		}
		validated, validateErr := owner.validateStoredCurrentV2(ctx, result)
		if validateErr != nil {
			return contract.HostDeploymentCurrentV2{}, validateErr
		}
		finalWinner, finalErr := owner.repository.InspectStoredHostDeploymentCurrentV2(ctx, desired.Ref.HostID, desired.Ref.DeploymentID)
		if finalErr != nil {
			return contract.HostDeploymentCurrentV2{}, finalErr
		}
		finalWinner = contract.CloneHostDeploymentCurrentV2(finalWinner)
		if finalWinner.Ref != desired.Ref || !reflect.DeepEqual(finalWinner, validated) {
			return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_not_current", "Host deployment current V2 normal publish lost currentness during source validation")
		}
		return contract.CloneHostDeploymentCurrentV2(validated), nil
	}
	if !contract.HasCode(writeErr, contract.ErrorUnknownOutcome) {
		return contract.HostDeploymentCurrentV2{}, writeErr
	}

	// Unknown write outcome is permanently Inspect-only. The original exact
	// desired Ref is inspected; no second CAS, revision or alternate payload is
	// attempted.
	inspected, inspectErr := owner.repository.InspectStoredHostDeploymentExactV2(
		context.WithoutCancel(ctx),
		desired.Ref,
	)
	if inspectErr != nil {
		return contract.HostDeploymentCurrentV2{}, errors.Join(writeErr, inspectErr)
	}
	inspected = contract.CloneHostDeploymentCurrentV2(inspected)
	if !reflect.DeepEqual(inspected, desired) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_unknown_recovery_drift", "Host deployment current V2 unknown recovery observed another body")
	}
	winner, winnerErr := owner.repository.InspectStoredHostDeploymentCurrentV2(
		context.WithoutCancel(ctx),
		desired.Ref.HostID,
		desired.Ref.DeploymentID,
	)
	if winnerErr != nil {
		return contract.HostDeploymentCurrentV2{}, errors.Join(writeErr, winnerErr)
	}
	winner = contract.CloneHostDeploymentCurrentV2(winner)
	if winner.Ref != desired.Ref || !reflect.DeepEqual(winner, desired) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_not_current", "Host deployment current V2 unknown recovery exact history is not the current winner")
	}
	validated, inspectErr := owner.validateStoredCurrentV2(context.WithoutCancel(ctx), inspected)
	if inspectErr != nil {
		return contract.HostDeploymentCurrentV2{}, errors.Join(writeErr, inspectErr)
	}
	finalWinner, finalErr := owner.repository.InspectStoredHostDeploymentCurrentV2(
		context.WithoutCancel(ctx),
		desired.Ref.HostID,
		desired.Ref.DeploymentID,
	)
	if finalErr != nil {
		return contract.HostDeploymentCurrentV2{}, errors.Join(writeErr, finalErr)
	}
	finalWinner = contract.CloneHostDeploymentCurrentV2(finalWinner)
	if finalWinner.Ref != desired.Ref || !reflect.DeepEqual(finalWinner, validated) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_not_current", "Host deployment current V2 unknown recovery lost currentness during source validation")
	}
	return contract.CloneHostDeploymentCurrentV2(validated), nil
}

func (owner *CurrentOwnerV2) InspectHostDeploymentCurrentV2(
	ctx context.Context,
	ref contract.HostDeploymentCurrentRefV2,
) (contract.HostDeploymentCurrentV2, error) {
	if err := owner.readyV2(ctx); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	stored, err := owner.repository.InspectStoredHostDeploymentExactV2(ctx, ref)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	stored = contract.CloneHostDeploymentCurrentV2(stored)
	winner, err := owner.repository.InspectStoredHostDeploymentCurrentV2(ctx, ref.HostID, ref.DeploymentID)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	winner = contract.CloneHostDeploymentCurrentV2(winner)
	if winner.Ref != ref || !reflect.DeepEqual(winner, stored) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_not_current", "Host deployment current V2 exact history is no longer current")
	}
	validated, err := owner.validateStoredCurrentV2(ctx, stored)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	finalWinner, err := owner.repository.InspectStoredHostDeploymentCurrentV2(ctx, ref.HostID, ref.DeploymentID)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	finalWinner = contract.CloneHostDeploymentCurrentV2(finalWinner)
	if finalWinner.Ref != ref || !reflect.DeepEqual(finalWinner, validated) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_not_current", "Host deployment current V2 lost currentness during source validation")
	}
	return contract.CloneHostDeploymentCurrentV2(validated), nil
}

func (owner *CurrentOwnerV2) validateStoredCurrentV2(
	ctx context.Context,
	stored contract.HostDeploymentCurrentV2,
) (contract.HostDeploymentCurrentV2, error) {
	baseline := owner.clock()
	if err := stored.ValidateCurrentV2(stored.Ref, baseline); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	first, err := owner.readFreshSourcesV2(ctx, stored.Ref.PackageSelectionRef, stored.ResourceHandles, stored.ServiceBindings, baseline)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	secondNow := owner.clock()
	if secondNow.IsZero() || secondNow.Before(baseline) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "Host deployment current V2 Inspect clock regressed")
	}
	second, err := owner.readFreshSourcesV2(ctx, stored.Ref.PackageSelectionRef, stored.ResourceHandles, stored.ServiceBindings, secondNow)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if !first.equalV2(second) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_source_drift", "Host deployment current V2 source changed during Inspect")
	}
	if stored.ExpiresUnixNano > second.expiresUnixNano {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_ttl_widened", "Host deployment current V2 TTL exceeds freshly read source current")
	}
	freshNow := owner.clock()
	if freshNow.IsZero() || freshNow.Before(secondNow) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "Host deployment current V2 final Inspect clock regressed")
	}
	if err = stored.ValidateCurrentV2(stored.Ref, freshNow); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	return contract.CloneHostDeploymentCurrentV2(stored), nil
}

type sourceSnapshotV2 struct {
	selection       buildercontract.AgentPackageSelectionCurrentV1
	packageRef      buildercontract.AgentPackageRefV1
	publicationRef  assemblycontract.AssemblyPublicationRefV2
	closureDigest   core.Digest
	resourceCurrent []runtimeports.ResourceHandleCurrentV1
	serviceCurrent  []contract.HostServiceBindingCurrentV2
	expiresUnixNano int64
}

func (snapshot sourceSnapshotV2) equalV2(other sourceSnapshotV2) bool {
	return reflect.DeepEqual(snapshot.selection, other.selection) &&
		snapshot.packageRef == other.packageRef &&
		snapshot.publicationRef == other.publicationRef &&
		snapshot.closureDigest == other.closureDigest &&
		slices.Equal(snapshot.resourceCurrent, other.resourceCurrent) &&
		slices.Equal(snapshot.serviceCurrent, other.serviceCurrent) &&
		snapshot.expiresUnixNano == other.expiresUnixNano
}

func (owner *CurrentOwnerV2) readFreshSourcesV2(
	ctx context.Context,
	selectionRef buildercontract.AgentPackageSelectionCurrentRefV1,
	resourceRefs []runtimeports.ResourceHandleRefV1,
	serviceRefs []contract.HostServiceBindingRefV1,
	now time.Time,
) (sourceSnapshotV2, error) {
	if now.IsZero() {
		return sourceSnapshotV2{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "Host deployment current V2 source clock is unavailable")
	}
	exactSelection, err := owner.builder.InspectAgentPackageSelectionExactV1(ctx, selectionRef)
	if err != nil {
		return sourceSnapshotV2{}, err
	}
	if err = exactSelection.ValidateCurrent(selectionRef, now); err != nil {
		return sourceSnapshotV2{}, err
	}
	currentSelection, err := owner.builder.InspectAgentPackageSelectionCurrentV1(ctx, selectionRef.SelectionID)
	if err != nil {
		return sourceSnapshotV2{}, err
	}
	if err = currentSelection.ValidateCurrent(selectionRef, now); err != nil {
		return sourceSnapshotV2{}, err
	}
	if !reflect.DeepEqual(exactSelection, currentSelection) {
		return sourceSnapshotV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_selection_drift", "Builder package selection exact and current projections differ")
	}

	closure, err := owner.builder.LoadVerifiedAgentPackageClosureV1(ctx, exactSelection.PackageRef)
	if err != nil {
		return sourceSnapshotV2{}, err
	}
	if err = closure.Validate(); err != nil {
		return sourceSnapshotV2{}, err
	}
	if closure.Package.RefV1() != exactSelection.PackageRef ||
		closure.PublicationRefV2() != exactSelection.PublicationRef ||
		closure.ClosureDigest != exactSelection.ClosureDigest {
		return sourceSnapshotV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_closure_drift", "Builder package selection and verified closure differ")
	}

	snapshot := sourceSnapshotV2{
		selection:       exactSelection,
		packageRef:      closure.Package.RefV1(),
		publicationRef:  closure.PublicationRefV2(),
		closureDigest:   closure.ClosureDigest,
		resourceCurrent: make([]runtimeports.ResourceHandleCurrentV1, 0, len(resourceRefs)),
		serviceCurrent:  make([]contract.HostServiceBindingCurrentV2, 0, len(serviceRefs)),
		expiresUnixNano: exactSelection.ExpiresUnixNano,
	}
	for _, ref := range resourceRefs {
		current, inspectErr := owner.resources.InspectResourceHandleCurrentV1(ctx, ref)
		if inspectErr != nil {
			return sourceSnapshotV2{}, inspectErr
		}
		if inspectErr = current.ValidateCurrent(ref, now); inspectErr != nil {
			return sourceSnapshotV2{}, inspectErr
		}
		snapshot.resourceCurrent = append(snapshot.resourceCurrent, current)
		snapshot.expiresUnixNano = minV2(snapshot.expiresUnixNano, current.ExpiresUnixNano)
	}
	for _, ref := range serviceRefs {
		current, inspectErr := owner.services.InspectHostServiceBindingCurrentV2(ctx, ref)
		if inspectErr != nil {
			return sourceSnapshotV2{}, inspectErr
		}
		if inspectErr = current.ValidateCurrentV2(ref, now); inspectErr != nil {
			return sourceSnapshotV2{}, inspectErr
		}
		snapshot.serviceCurrent = append(snapshot.serviceCurrent, current)
		snapshot.expiresUnixNano = minV2(snapshot.expiresUnixNano, current.ExpiresUnixNano)
	}
	return snapshot, nil
}

func (owner *CurrentOwnerV2) readyV2(ctx context.Context) error {
	if owner == nil ||
		contract.IsTypedNilV1(owner.builder) ||
		contract.IsTypedNilV1(owner.resources) ||
		contract.IsTypedNilV1(owner.services) ||
		contract.IsTypedNilV1(owner.repository) ||
		owner.clock == nil {
		return contract.NewError(contract.ErrorUnavailable, "host_deployment_v2_owner_unavailable", "Host deployment current V2 Owner is unavailable")
	}
	if ctx == nil {
		return contract.NewError(contract.ErrorInvalidArgument, "context_missing", "Host deployment current V2 requires a context")
	}
	if ctx.Err() != nil {
		return contract.NewError(contract.ErrorUnavailable, "context_ended", "Host deployment current V2 context ended")
	}
	return nil
}

func safeCompareAndSwapV2(
	ctx context.Context,
	repository hostports.HostDeploymentCurrentRepositoryV2,
	expected contract.HostDeploymentCurrentRefV2,
	next contract.HostDeploymentCurrentV2,
) (result contract.HostDeploymentCurrentV2, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = contract.HostDeploymentCurrentV2{}
			err = contract.NewError(contract.ErrorUnknownOutcome, "host_deployment_v2_repository_panic", "Host deployment current V2 repository mutation panicked")
		}
	}()
	return repository.CompareAndSwapStoredHostDeploymentCurrentV2(ctx, expected, next)
}

func minV2(left, right int64) int64 {
	if right < left {
		return right
	}
	return left
}

var _ hostports.HostDeploymentCurrentOwnerV2 = (*CurrentOwnerV2)(nil)
