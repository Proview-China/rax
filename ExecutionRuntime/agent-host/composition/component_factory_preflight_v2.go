package composition

import (
	"context"
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

type ComponentFactoryPreflightConfigV2 struct {
	Deployments  hostports.HostDeploymentCurrentReaderV2
	Builder      hostports.HostDeploymentBuilderEvidenceV2
	Resources    hostports.HostDeploymentResourceCurrentReaderV2
	Registry     hostports.ComponentFactoryRegistryReaderV2
	Conformance  hostports.ComponentFactoryConformanceCurrentReaderV2
	Dependencies hostports.ComponentDependencyCurrentReaderV2
	Clock        func() time.Time
}

type ComponentFactoryPreflightV2 struct {
	deployments  hostports.HostDeploymentCurrentReaderV2
	builder      hostports.HostDeploymentBuilderEvidenceV2
	resources    hostports.HostDeploymentResourceCurrentReaderV2
	registry     hostports.ComponentFactoryRegistryReaderV2
	conformance  hostports.ComponentFactoryConformanceCurrentReaderV2
	dependencies hostports.ComponentDependencyCurrentReaderV2
	clock        func() time.Time
}

func NewComponentFactoryPreflightV2(config ComponentFactoryPreflightConfigV2) (*ComponentFactoryPreflightV2, error) {
	if contract.IsTypedNilV1(config.Deployments) ||
		contract.IsTypedNilV1(config.Builder) ||
		contract.IsTypedNilV1(config.Resources) ||
		contract.IsTypedNilV1(config.Registry) ||
		contract.IsTypedNilV1(config.Conformance) ||
		contract.IsTypedNilV1(config.Dependencies) {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "component_factory_preflight_dependency_missing", "component factory preflight requires narrow authoritative readers")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &ComponentFactoryPreflightV2{
		deployments:  config.Deployments,
		builder:      config.Builder,
		resources:    config.Resources,
		registry:     config.Registry,
		conformance:  config.Conformance,
		dependencies: config.Dependencies,
		clock:        config.Clock,
	}, nil
}

func (preflight *ComponentFactoryPreflightV2) PreflightComponentFactoryV2(
	ctx context.Context,
	request contract.ComponentFactoryPreflightRequestV2,
) (contract.ComponentFactoryPreflightReceiptV2, error) {
	if preflight == nil || preflight.clock == nil {
		return contract.ComponentFactoryPreflightReceiptV2{}, contract.NewError(contract.ErrorUnavailable, "component_factory_preflight_unavailable", "component factory preflight is unavailable")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.ComponentFactoryPreflightReceiptV2{}, contract.NewError(contract.ErrorUnavailable, "component_factory_preflight_context_ended", "component factory preflight context ended")
	}
	request = contract.CloneComponentFactoryPreflightRequestV2(request)
	n1 := preflight.clock()
	if err := request.ValidateCurrent(n1); err != nil {
		return contract.ComponentFactoryPreflightReceiptV2{}, err
	}
	first, err := preflight.readSnapshotV2(ctx, request, n1)
	if err != nil {
		return contract.ComponentFactoryPreflightReceiptV2{}, err
	}
	n2 := preflight.clock()
	if n2.IsZero() || n2.Before(n1) {
		return contract.ComponentFactoryPreflightReceiptV2{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "component factory preflight S2 clock regressed")
	}
	second, err := preflight.readSnapshotV2(ctx, request, n2)
	if err != nil {
		return contract.ComponentFactoryPreflightReceiptV2{}, err
	}
	if !reflect.DeepEqual(first, second) {
		return contract.ComponentFactoryPreflightReceiptV2{}, contract.NewError(contract.ErrorConflict, "component_factory_preflight_source_drift", "component factory preflight sources changed between S1 and S2")
	}
	sealNow := preflight.clock()
	if sealNow.IsZero() || sealNow.Before(n2) {
		return contract.ComponentFactoryPreflightReceiptV2{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "component factory preflight seal clock regressed")
	}
	expires := minComponentFactoryV2(request.RequestedNotAfterUnixNano, second.expiresUnixNano)
	if second.checkedUnixNano > sealNow.UnixNano() {
		return contract.ComponentFactoryPreflightReceiptV2{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "component factory preflight seal clock predates an authoritative current")
	}
	if second.checkedUnixNano >= expires || sealNow.UnixNano() >= expires {
		return contract.ComponentFactoryPreflightReceiptV2{}, contract.NewError(contract.ErrorPrecondition, "component_factory_preflight_expired", "component factory preflight expired before sealing")
	}
	return contract.SealComponentFactoryPreflightReceiptV2(contract.ComponentFactoryPreflightReceiptV2{
		Ref: contract.ComponentFactoryPreflightReceiptRefV2{
			AttemptID: request.AttemptID, Revision: 1, ExpiresUnixNano: expires,
		},
		HostID: request.HostID, StartID: request.StartID,
		DeploymentRef:     request.DeploymentRef,
		Deployment:        contract.CloneHostDeploymentCurrentV2(second.deployment),
		Request:           contract.CloneComponentFactoryPreflightRequestV2(request),
		Selection:         buildercontract.CloneAgentPackageSelectionCurrentV1(second.selection),
		VerifiedClosure:   buildercontract.CloneVerifiedAgentPackageClosureV1(second.closure),
		PackageRef:        second.closure.Package.RefV1(),
		PublicationRef:    second.closure.PublicationRefV2(),
		ClosureDigest:     contract.DigestV1(second.closure.ClosureDigest),
		PackageDescriptor: second.packageDescriptor,
		RequestDigest:     request.RequestDigest,
		Registration:      second.registration,
		ConformanceRef:    second.conformance.Ref,
		Conformance:       second.conformance,
		ResourceRefs:      request.ResourceRefs,
		ResourceCurrents:  append([]runtimeports.ResourceHandleCurrentV1{}, second.resources...),
		Dependencies:      second.dependencies,
		CheckedUnixNano:   second.checkedUnixNano,
		ExpiresUnixNano:   expires,
	})
}

type componentFactoryPreflightSnapshotV2 struct {
	deployment        contract.HostDeploymentCurrentV2
	selection         buildercontract.AgentPackageSelectionCurrentV1
	closure           buildercontract.VerifiedAgentPackageClosureV1
	registration      contract.ComponentFactoryRegistrationV2
	conformance       contract.ComponentFactoryConformanceCurrentV2
	packageDescriptor assemblycontract.ModuleFactoryDescriptorV1
	resources         []runtimeports.ResourceHandleCurrentV1
	dependencies      []contract.ComponentDependencyCurrentV2
	checkedUnixNano   int64
	expiresUnixNano   int64
}

func (preflight *ComponentFactoryPreflightV2) readSnapshotV2(
	ctx context.Context,
	request contract.ComponentFactoryPreflightRequestV2,
	now time.Time,
) (componentFactoryPreflightSnapshotV2, error) {
	deployment, err := preflight.deployments.InspectHostDeploymentCurrentV2(ctx, request.DeploymentRef)
	if err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if err = deployment.ValidateCurrentV2(request.DeploymentRef, now); err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if deployment.Ref.HostID != request.HostID {
		return componentFactoryPreflightSnapshotV2{}, contract.NewError(contract.ErrorConflict, "component_factory_preflight_host_drift", "deployment belongs to another Host")
	}

	selectionRef := deployment.Ref.PackageSelectionRef
	selection, err := preflight.builder.InspectAgentPackageSelectionExactV1(ctx, selectionRef)
	if err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if err = selection.ValidateCurrent(selectionRef, now); err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	currentSelection, err := preflight.builder.InspectAgentPackageSelectionCurrentV1(ctx, selectionRef.SelectionID)
	if err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if err = currentSelection.ValidateCurrent(selectionRef, now); err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if !reflect.DeepEqual(selection, currentSelection) {
		return componentFactoryPreflightSnapshotV2{}, contract.NewError(contract.ErrorConflict, "component_factory_preflight_selection_drift", "Builder selection exact and current differ")
	}
	closure, err := preflight.builder.LoadVerifiedAgentPackageClosureV1(ctx, selection.PackageRef)
	if err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if err = closure.Validate(); err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if closure.Package.RefV1() != selection.PackageRef ||
		closure.PublicationRefV2() != selection.PublicationRef ||
		closure.ClosureDigest != selection.ClosureDigest {
		return componentFactoryPreflightSnapshotV2{}, contract.NewError(contract.ErrorConflict, "component_factory_preflight_closure_drift", "Builder selection and verified closure differ")
	}
	registration, err := preflight.registry.InspectComponentFactoryRegistrationV2(ctx, request.RegistryKey)
	if err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if err = registration.Validate(); err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if registration.Key != request.RegistryKey {
		return componentFactoryPreflightSnapshotV2{}, contract.NewError(contract.ErrorConflict, "component_factory_preflight_registry_drift", "registry returned another exact registration")
	}
	factoryDescriptor, err := contract.ReadAuthoritativeComponentFactoryPackageDescriptorV2(
		closure,
		registration.Descriptor.Ref.FactoryID,
	)
	if err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if err = contract.ValidateComponentFactoryPackageDescriptorV2(registration.Descriptor, factoryDescriptor); err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}

	conformance, err := preflight.conformance.InspectComponentFactoryConformanceCurrentV2(ctx, registration.ConformanceRef)
	if err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if err = conformance.ValidateCurrent(registration.ConformanceRef, now); err != nil {
		return componentFactoryPreflightSnapshotV2{}, err
	}
	if conformance.FactoryRef != registration.Descriptor.Ref ||
		conformance.DescriptorDigest != registration.Descriptor.DescriptorDigest {
		return componentFactoryPreflightSnapshotV2{}, contract.NewError(contract.ErrorConflict, "component_factory_preflight_conformance_drift", "factory conformance no longer proves the registered descriptor")
	}

	snapshot := componentFactoryPreflightSnapshotV2{
		deployment: deployment, selection: selection, closure: closure,
		registration: registration, conformance: conformance, packageDescriptor: factoryDescriptor,
		resources:    make([]runtimeports.ResourceHandleCurrentV1, 0, len(request.ResourceRefs)),
		dependencies: make([]contract.ComponentDependencyCurrentV2, 0, len(request.DependencyRefs)),
		checkedUnixNano: maxComponentFactoryV2(
			request.RequestedUnixNano,
			deployment.CheckedUnixNano,
			selection.CheckedUnixNano,
			conformance.CheckedUnixNano,
		),
		expiresUnixNano: minComponentFactoryV2(deployment.ExpiresUnixNano, selection.ExpiresUnixNano),
	}
	snapshot.expiresUnixNano = minComponentFactoryV2(snapshot.expiresUnixNano, conformance.ExpiresUnixNano)

	deploymentResources := deployment.ResourceHandles
	for _, expected := range request.ResourceRefs {
		runtimeRef, convertErr := runtimeResourceRefV2(expected)
		if convertErr != nil {
			return componentFactoryPreflightSnapshotV2{}, convertErr
		}
		if !slices.Contains(deploymentResources, runtimeRef) {
			return componentFactoryPreflightSnapshotV2{}, contract.NewError(contract.ErrorConflict, "component_factory_preflight_resource_not_deployed", "factory resource is not in the deployment current")
		}
		current, inspectErr := preflight.resources.InspectResourceHandleCurrentV1(ctx, runtimeRef)
		if inspectErr != nil {
			return componentFactoryPreflightSnapshotV2{}, inspectErr
		}
		if inspectErr = current.ValidateCurrent(runtimeRef, now); inspectErr != nil {
			return componentFactoryPreflightSnapshotV2{}, inspectErr
		}
		snapshot.resources = append(snapshot.resources, current)
		snapshot.checkedUnixNano = maxComponentFactoryV2(snapshot.checkedUnixNano, current.CheckedUnixNano)
		snapshot.expiresUnixNano = minComponentFactoryV2(snapshot.expiresUnixNano, current.ExpiresUnixNano)
	}
	for _, expected := range request.DependencyRefs {
		current, inspectErr := preflight.dependencies.InspectComponentDependencyCurrentV2(ctx, expected)
		if inspectErr != nil {
			return componentFactoryPreflightSnapshotV2{}, inspectErr
		}
		if inspectErr = current.ValidateCurrent(expected, now); inspectErr != nil {
			return componentFactoryPreflightSnapshotV2{}, inspectErr
		}
		snapshot.dependencies = append(snapshot.dependencies, current)
		snapshot.checkedUnixNano = maxComponentFactoryV2(snapshot.checkedUnixNano, current.CheckedUnixNano)
		snapshot.expiresUnixNano = minComponentFactoryV2(snapshot.expiresUnixNano, current.ExpiresUnixNano)
	}
	return snapshot, nil
}

func runtimeResourceRefV2(value contract.ComponentResourceRefV2) (runtimeports.ResourceHandleRefV1, error) {
	ref := runtimeports.ResourceHandleRefV1{
		Owner: core.OwnerRef{Domain: value.OwnerDomain, ID: core.OwnerID(value.OwnerID)},
		ID:    value.ID, Revision: core.Revision(value.Revision), Digest: core.Digest(value.Digest),
		Kind: runtimeports.ResourceHandleKindV1(value.Kind), ScopeDigest: core.Digest(value.ScopeDigest),
		ExpiresUnixNano: value.ExpiresUnixNano,
	}
	if err := ref.Validate(); err != nil {
		return runtimeports.ResourceHandleRefV1{}, err
	}
	return ref, nil
}

func minComponentFactoryV2(left, right int64) int64 {
	if right < left {
		return right
	}
	return left
}

func maxComponentFactoryV2(values ...int64) int64 {
	maximum := int64(0)
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

var _ hostports.ComponentFactoryPreflightV2 = (*ComponentFactoryPreflightV2)(nil)
