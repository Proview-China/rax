package owneradapter

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const agentExecutionAvailabilityProofWindowV1 = time.Second

// AgentExecutionAvailabilityCurrentReaderConfigV1 binds one Runtime-neutral
// availability Ref to Host-owned exact readers. No caller may nominate the
// Start's Package Selection binding or Deployment V2 Ref.
type AgentExecutionAvailabilityCurrentReaderConfigV1 struct {
	Owner                         core.OwnerRef
	Availability                  runtimeports.AgentExecutionAvailabilityRefV1
	Host                          hostports.HostV3CurrentReaderV1
	Claims                        hostports.HostStartClaimCurrentReaderV1
	ClaimInputs                   hostports.HostStartClaimInputCurrentReaderV3
	StartPackageSelectionBindings hostports.HostStartPackageSelectionBindingForClaimReaderV1
	DeploymentsV1                 hostports.HostDeploymentCurrentReaderV1
	DeploymentsV2                 hostports.HostDeploymentCurrentReaderV2
	PackageSelections             hostports.HostDeploymentBuilderEvidenceV2
	SystemReadyCurrent            hostports.SystemReadyAvailabilitySourceV2
	SystemReadyFacts              hostports.SystemReadyFactCurrentReaderV2
	SystemReadyCore               hostports.SystemReadyCoreCurrentReadersV2
	ComponentCurrents             hostports.ComponentProductionCurrentReaderRegistryV2
	CleanupClosures               hostports.HostCleanupClosureCurrentReaderV2
	Clock                         func() time.Time
}

// AgentExecutionAvailabilityCurrentReaderV1 is a Host-owned, read-only
// adapter. It creates no Current and returns only Runtime's existing public
// projection.
type AgentExecutionAvailabilityCurrentReaderV1 struct {
	owner                         core.OwnerRef
	availability                  runtimeports.AgentExecutionAvailabilityRefV1
	host                          hostports.HostV3CurrentReaderV1
	claims                        hostports.HostStartClaimCurrentReaderV1
	claimInputs                   hostports.HostStartClaimInputCurrentReaderV3
	startPackageSelectionBindings hostports.HostStartPackageSelectionBindingForClaimReaderV1
	deploymentsV1                 hostports.HostDeploymentCurrentReaderV1
	deploymentsV2                 hostports.HostDeploymentCurrentReaderV2
	packageSelections             hostports.HostDeploymentBuilderEvidenceV2
	systemReady                   hostports.SystemReadyAvailabilitySourceV2
	systemReadyFacts              hostports.SystemReadyFactCurrentReaderV2
	systemReadyCore               hostports.SystemReadyCoreCurrentReadersV2
	components                    hostports.ComponentProductionCurrentReaderRegistryV2
	cleanupClosures               hostports.HostCleanupClosureCurrentReaderV2
	clock                         func() time.Time
	clockMu                       sync.Mutex
	lastClock                     time.Time
}

func NewAgentExecutionAvailabilityCurrentReaderV1(config AgentExecutionAvailabilityCurrentReaderConfigV1) (*AgentExecutionAvailabilityCurrentReaderV1, error) {
	if err := config.Owner.Validate(); err != nil {
		return nil, err
	}
	if err := config.Availability.Validate(); err != nil {
		return nil, err
	}
	if config.Availability.Owner != config.Owner {
		return nil, contract.NewError(contract.ErrorConflict, "availability_owner_drift", "availability exact Ref belongs to another Owner")
	}
	dependencies := []any{
		config.Host,
		config.Claims,
		config.ClaimInputs,
		config.StartPackageSelectionBindings,
		config.DeploymentsV1,
		config.DeploymentsV2,
		config.PackageSelections,
		config.SystemReadyCurrent,
		config.SystemReadyFacts,
		config.SystemReadyCore,
		config.ComponentCurrents,
		config.CleanupClosures,
	}
	for _, dependency := range dependencies {
		if contract.IsTypedNilV1(dependency) {
			return nil, contract.NewError(contract.ErrorInvalidArgument, "availability_current_dependency_missing", "all availability current readers are required")
		}
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &AgentExecutionAvailabilityCurrentReaderV1{
		owner:                         config.Owner,
		availability:                  config.Availability,
		host:                          config.Host,
		claims:                        config.Claims,
		claimInputs:                   config.ClaimInputs,
		startPackageSelectionBindings: config.StartPackageSelectionBindings,
		deploymentsV1:                 config.DeploymentsV1,
		deploymentsV2:                 config.DeploymentsV2,
		packageSelections:             config.PackageSelections,
		systemReady:                   config.SystemReadyCurrent,
		systemReadyFacts:              config.SystemReadyFacts,
		systemReadyCore:               config.SystemReadyCore,
		components:                    config.ComponentCurrents,
		cleanupClosures:               config.CleanupClosures,
		clock:                         config.Clock,
	}, nil
}

func (reader *AgentExecutionAvailabilityCurrentReaderV1) InspectAgentExecutionAvailabilityCurrentV1(
	ctx context.Context,
	expected runtimeports.AgentExecutionAvailabilityRefV1,
) (projection runtimeports.AgentExecutionAvailabilityProjectionV1, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			projection = runtimeports.AgentExecutionAvailabilityProjectionV1{}
			err = contract.NewError(contract.ErrorUnavailable, "availability_current_inspect_panic", fmt.Sprintf("availability current Inspect panicked: %v", recovered))
		}
	}()
	if reader == nil || reader.clock == nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, contract.NewError(contract.ErrorUnavailable, "availability_current_reader_missing", "availability current reader is unavailable")
	}
	if err = availabilityContextV1(ctx); err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	if err = expected.Validate(); err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	if expected != reader.availability || expected.Owner != reader.owner {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, contract.NewError(contract.ErrorConflict, "availability_current_binding_drift", "availability reader is bound to another exact Ref")
	}

	firstNow, err := reader.freshNowV1()
	if err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	first, err := reader.readCurrentClosureV1(ctx, expected, firstNow)
	if err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	if err = availabilityContextV1(ctx); err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}

	secondNow, err := reader.freshNowV1()
	if err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	second, err := reader.readCurrentClosureV1(ctx, expected, secondNow)
	if err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	if !reflect.DeepEqual(first.watermarkV1(), second.watermarkV1()) {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, contract.NewError(contract.ErrorConflict, "availability_current_s1_s2_drift", "availability current closure drifted between S1 and S2")
	}
	if err = availabilityContextV1(ctx); err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}

	sealNow, err := reader.freshNowV1()
	if err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	if sealNow.UnixNano() >= second.proofExpiresUnixNano {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, contract.NewError(contract.ErrorPrecondition, "availability_current_expired", "availability current proof window crossed before return")
	}
	projection, err = second.readyCurrent.ToAgentExecutionAvailabilityV1(reader.owner)
	if err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	if projection.Ref != expected || projection.ExpiresUnixNano > second.ownerExpiresUnixNano {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, contract.NewError(contract.ErrorConflict, "availability_current_ttl_drift", "availability projection exceeds its exact Host closure")
	}
	if projection.State == runtimeports.AgentExecutionAvailabilityReadyV1 {
		if err = projection.ValidateCurrent(expected, sealNow); err != nil {
			return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
		}
	} else if err = projection.Validate(); err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	return projection, nil
}

type availabilityHostWatermarkV1 struct {
	StartClaim     contract.HostStartClaimRefV1
	Journal        contract.ExactRefV1
	Ready          contract.SystemReadyCurrentRefV2
	Availability   runtimeports.AgentExecutionAvailabilityRefV1
	CleanupClosure contract.ExactRefV1
	Phase          contract.HostInspectPhaseV3
}

type availabilityCurrentClosureV1 struct {
	readyCurrent         contract.SystemReadyCurrentV2
	readyFact            contract.SystemReadyFactV2
	claim                contract.HostStartClaimV1
	claimInput           contract.HostStartClaimInputBindingV3
	packageBinding       contract.HostStartPackageSelectionBindingV1
	deploymentV1         contract.HostDeploymentCurrentV1
	deploymentV2         contract.HostDeploymentCurrentV2
	packageSelection     availabilityPackageSelectionWatermarkV1
	coreCurrents         []runtimeports.OwnerCurrentRefV1
	policy               contract.SystemReadySupervisionPolicyCurrentV2
	components           []contract.ComponentProductionCurrentV2
	cleanup              contract.HostCleanupClosureFactV2
	host                 availabilityHostWatermarkV1
	ownerExpiresUnixNano int64
	proofExpiresUnixNano int64
}

func (closure availabilityCurrentClosureV1) watermarkV1() any {
	return struct {
		ReadyCurrent     contract.SystemReadyCurrentV2
		ReadyFact        contract.SystemReadyFactV2
		Claim            contract.HostStartClaimV1
		ClaimInput       contract.HostStartClaimInputBindingV3
		PackageBinding   contract.HostStartPackageSelectionBindingV1
		DeploymentV1     contract.HostDeploymentCurrentV1
		DeploymentV2     contract.HostDeploymentCurrentV2
		PackageSelection availabilityPackageSelectionWatermarkV1
		Core             []runtimeports.OwnerCurrentRefV1
		Policy           contract.SystemReadySupervisionPolicyCurrentV2
		Components       []contract.ComponentProductionCurrentV2
		Cleanup          contract.HostCleanupClosureFactV2
		Host             availabilityHostWatermarkV1
	}{
		closure.readyCurrent,
		closure.readyFact,
		closure.claim,
		closure.claimInput,
		closure.packageBinding,
		closure.deploymentV1,
		closure.deploymentV2,
		closure.packageSelection,
		closure.coreCurrents,
		closure.policy,
		closure.components,
		closure.cleanup,
		closure.host,
	}
}

func (reader *AgentExecutionAvailabilityCurrentReaderV1) readCurrentClosureV1(
	ctx context.Context,
	expected runtimeports.AgentExecutionAvailabilityRefV1,
	now time.Time,
) (availabilityCurrentClosureV1, error) {
	if err := availabilityContextV1(ctx); err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	current, err := reader.systemReady.InspectSystemReadyCurrentForAvailabilityV2(ctx, expected)
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	if err = current.Validate(); err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	projected, err := current.ToAgentExecutionAvailabilityV1(reader.owner)
	if err != nil || projected.Ref != expected {
		return availabilityCurrentClosureV1{}, contract.NewError(contract.ErrorConflict, "availability_current_projection_drift", "SystemReady Current does not bind the requested availability Ref")
	}
	fact, err := reader.systemReadyFacts.InspectSystemReadyFactV2(ctx, current.FactRef)
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	if err = fact.Validate(); err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	if fact.Ref != current.FactRef || fact.HostID != current.HostID || fact.StartID != current.StartID {
		return availabilityCurrentClosureV1{}, contract.NewError(contract.ErrorConflict, "availability_system_ready_splice", "SystemReady Current and Fact coordinates drifted")
	}

	claim, err := reader.claims.InspectHostStartClaimCurrentV1(ctx, fact.HostStartClaim)
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	claimRef, err := claim.CurrentRefV1()
	if err != nil || claimRef != fact.HostStartClaim {
		return availabilityCurrentClosureV1{}, contract.NewError(contract.ErrorConflict, "availability_claim_splice", "SystemReady Fact and HostStart Claim drifted")
	}
	input, err := reader.claimInputs.InspectHostStartClaimInputV3(ctx, claimRef)
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	if err = input.ValidateV3(); err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	if input.ClaimRef != claimRef {
		return availabilityCurrentClosureV1{}, contract.NewError(contract.ErrorConflict, "availability_claim_input_splice", "HostStart Claim Input binds another Claim")
	}
	packageBinding, err := reader.startPackageSelectionBindings.InspectHostStartPackageSelectionBindingForClaimV1(ctx, claimRef)
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	if err = packageBinding.ValidateAgainstClaimInputV1(claim, input); err != nil {
		return availabilityCurrentClosureV1{}, err
	}

	deploymentV1, err := reader.deploymentsV1.InspectHostDeploymentCurrentV1(ctx, input.Input.DeploymentCurrentRef)
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	deploymentV2, err := reader.deploymentsV2.InspectHostDeploymentCurrentV2(ctx, packageBinding.DeploymentCurrentRef)
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	if err = validateDeploymentAssociationV1(deploymentV1, deploymentV2, input.Input.DeploymentCurrentRef, packageBinding.DeploymentCurrentRef); err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	selection, err := reader.inspectPackageSelectionClosureV1(ctx, packageBinding, now, current.State == contract.SystemReadyCurrentReadyV2)
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}

	isReady := current.State == contract.SystemReadyCurrentReadyV2
	if isReady {
		if err = current.ValidateCurrent(current.Ref, now); err != nil {
			return availabilityCurrentClosureV1{}, err
		}
		if err = fact.ValidateCurrent(now); err != nil {
			return availabilityCurrentClosureV1{}, err
		}
		if err = claim.ValidateCurrentV1(now); err != nil {
			return availabilityCurrentClosureV1{}, err
		}
		if err = deploymentV1.ValidateCurrentV1(input.Input.DeploymentCurrentRef, now); err != nil {
			return availabilityCurrentClosureV1{}, err
		}
		if err = packageBinding.ValidateCurrentV1(packageBinding.Ref, now); err != nil {
			return availabilityCurrentClosureV1{}, err
		}
		if err = deploymentV2.ValidateCurrentV2(packageBinding.DeploymentCurrentRef, now); err != nil {
			return availabilityCurrentClosureV1{}, err
		}
	}

	coreCurrents, policy, components, coreExpiry, err := reader.inspectSystemReadyClosureV1(ctx, fact, now, isReady)
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	cleanup, err := reader.cleanupClosures.InspectHostCleanupClosureForStartV2(ctx, current.HostID, current.StartID)
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	if err = cleanup.Validate(); err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	cleanupRef, err := cleanup.RefV2()
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	exactCleanup := contract.ExactRefV1{Kind: contract.HostCleanupClosureRefKindV2, ID: cleanupRef.ClosureID, Revision: cleanupRef.Revision, Digest: cleanupRef.Digest}

	ownerExpiry := current.ExpiresUnixNano
	for _, expiry := range []int64{
		fact.ExpiresUnixNano,
		claimRef.ExpiresUnixNano,
		input.Input.ExpiresUnixNano,
		packageBinding.ExpiresUnixNano,
		deploymentV1.ExpiresUnixNano,
		deploymentV2.ExpiresUnixNano,
		selection.SelectionExpires,
		coreExpiry,
	} {
		ownerExpiry = minAvailabilityExpiryV1(ownerExpiry, expiry)
	}
	if isReady && current.ExpiresUnixNano != ownerExpiry {
		return availabilityCurrentClosureV1{}, contract.NewError(contract.ErrorConflict, "availability_current_ttl_drift", "ready availability TTL is not the exact minimum Host closure")
	}
	requestExpiry := minAvailabilityExpiryV1(ownerExpiry, now.Add(agentExecutionAvailabilityProofWindowV1).UnixNano())
	if requestExpiry <= now.UnixNano() {
		return availabilityCurrentClosureV1{}, contract.NewError(contract.ErrorPrecondition, "availability_current_expired", "availability current closure expired before Host Inspect")
	}
	inspectRequest, err := contract.SealInspectRequestV3(contract.InspectRequestV3{
		HostID:                    current.HostID,
		StartID:                   current.StartID,
		StartClaim:                claimRef,
		RequestedAtUnixNano:       now.UnixNano(),
		RequestedNotAfterUnixNano: requestExpiry,
	})
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	hostResult, err := reader.host.InspectV3(ctx, inspectRequest)
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	observedNow, err := reader.freshNowV1()
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	if err = hostResult.Validate(); err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	actualHostClaim, err := hostResult.StartClaim.CurrentRefV1()
	if err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	if hostResult.RequestDigest != inspectRequest.RequestDigest ||
		hostResult.RequestNotAfterUnixNano != inspectRequest.RequestedNotAfterUnixNano ||
		actualHostClaim != claimRef ||
		hostResult.CheckedUnixNano < inspectRequest.RequestedAtUnixNano ||
		hostResult.CheckedUnixNano > observedNow.UnixNano() ||
		!observedNow.Before(time.Unix(0, hostResult.ExpiresUnixNano)) ||
		hostResult.ExpiresUnixNano > inspectRequest.RequestedNotAfterUnixNano {
		return availabilityCurrentClosureV1{}, contract.NewError(contract.ErrorConflict, "availability_host_inspect_splice", "HostV3 Inspect does not bind the exact short-lived request")
	}
	hostWatermark := availabilityHostWatermarkV1{
		StartClaim:     actualHostClaim,
		Journal:        hostResult.Journal,
		Ready:          hostResult.Ready,
		Availability:   hostResult.Availability,
		CleanupClosure: hostResult.CleanupClosure,
		Phase:          hostResult.Phase,
	}
	if !hostResult.HasReady || !hostResult.HasAvailability || !hostResult.HasCleanupClosure ||
		hostResult.Ready != current.Ref ||
		hostResult.Availability != expected ||
		hostResult.CleanupClosure != exactCleanup {
		return availabilityCurrentClosureV1{}, contract.NewError(contract.ErrorConflict, "availability_host_projection_splice", "HostV3 Inspect spliced Ready, Availability or Cleanup closure")
	}
	if err = validateAvailabilityPhaseV1(current.State, hostResult.Phase); err != nil {
		return availabilityCurrentClosureV1{}, err
	}
	proofExpiry := minAvailabilityExpiryV1(ownerExpiry, hostResult.ExpiresUnixNano)
	return availabilityCurrentClosureV1{
		readyCurrent:         current,
		readyFact:            fact,
		claim:                claim,
		claimInput:           input,
		packageBinding:       packageBinding,
		deploymentV1:         deploymentV1,
		deploymentV2:         deploymentV2,
		packageSelection:     selection,
		coreCurrents:         coreCurrents,
		policy:               policy,
		components:           components,
		cleanup:              cleanup,
		host:                 hostWatermark,
		ownerExpiresUnixNano: ownerExpiry,
		proofExpiresUnixNano: proofExpiry,
	}, nil
}

func (reader *AgentExecutionAvailabilityCurrentReaderV1) inspectSystemReadyClosureV1(
	ctx context.Context,
	fact contract.SystemReadyFactV2,
	now time.Time,
	requireCurrent bool,
) ([]runtimeports.OwnerCurrentRefV1, contract.SystemReadySupervisionPolicyCurrentV2, []contract.ComponentProductionCurrentV2, int64, error) {
	checks := []struct {
		expected runtimeports.OwnerCurrentRefV1
		inspect  func(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	}{
		{fact.DefinitionCurrent, reader.systemReadyCore.InspectDefinitionCurrentV2},
		{fact.PlanCurrent, reader.systemReadyCore.InspectPlanCurrentV2},
		{fact.AssemblyCurrent, reader.systemReadyCore.InspectAssemblyCurrentV2},
		{fact.BindingSetCurrent, reader.systemReadyCore.InspectBindingSetCurrentV2},
		{fact.ActivationCurrent, reader.systemReadyCore.InspectActivationCurrentV2},
		{fact.GenerationBindingCurrent, reader.systemReadyCore.InspectGenerationBindingCurrentV2},
		{fact.ApplicationStartCurrent, reader.systemReadyCore.InspectApplicationStartCurrentV2},
		{fact.SandboxLeaseCurrent, reader.systemReadyCore.InspectSandboxLeaseCurrentV2},
		{fact.SandboxActiveCurrent, reader.systemReadyCore.InspectSandboxActiveCurrentV2},
		{fact.ExecutionReadyCurrent, reader.systemReadyCore.InspectExecutionReadyCurrentV2},
	}
	actuals := make([]runtimeports.OwnerCurrentRefV1, 0, len(checks))
	minimum := fact.ExpiresUnixNano
	for _, check := range checks {
		if err := availabilityContextV1(ctx); err != nil {
			return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, err
		}
		actual, err := check.inspect(ctx, check.expected)
		if err != nil {
			return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, err
		}
		if actual != check.expected {
			return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, contract.NewError(contract.ErrorConflict, "availability_system_ready_owner_splice", "SystemReady Owner current reader returned another exact Ref")
		}
		if requireCurrent && now.UnixNano() >= actual.ExpiresUnixNano {
			return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, contract.NewError(contract.ErrorPrecondition, "availability_system_ready_owner_expired", "SystemReady Owner current expired")
		}
		minimum = minAvailabilityExpiryV1(minimum, actual.ExpiresUnixNano)
		actuals = append(actuals, actual)
	}
	policy, err := reader.systemReadyCore.InspectSupervisionPolicyCurrentV2(ctx, fact.SupervisionPolicyCurrent)
	if err != nil {
		return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, err
	}
	if err = policy.Validate(); err != nil {
		return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, err
	}
	if policy.Ref != fact.SupervisionPolicyCurrent || policy.MinimumReadyWindowNanos != fact.MinimumReadyWindowNanos ||
		(requireCurrent && (now.UnixNano() < policy.CheckedUnixNano || now.UnixNano() >= policy.ExpiresUnixNano)) {
		return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, contract.NewError(contract.ErrorConflict, "availability_supervision_policy_splice", "SystemReady supervision policy drifted")
	}
	minimum = minAvailabilityExpiryV1(minimum, policy.ExpiresUnixNano)
	components := make([]contract.ComponentProductionCurrentV2, 0, len(fact.Components))
	for _, expected := range fact.Components {
		currentReader, resolveErr := reader.components.ReaderForComponentProductionCurrentV2(expected.Domain)
		if resolveErr != nil {
			return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, resolveErr
		}
		if contract.IsTypedNilV1(currentReader) {
			return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, contract.NewError(contract.ErrorUnavailable, "availability_component_reader_missing", "component production current reader is unavailable")
		}
		actual, inspectErr := currentReader.InspectComponentProductionCurrentV2(ctx, expected)
		if inspectErr != nil {
			return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, inspectErr
		}
		if !reflect.DeepEqual(actual, expected) {
			return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, contract.NewError(contract.ErrorConflict, "availability_component_current_splice", "component production current drifted")
		}
		componentExpiry := componentProductionExpiryV1(actual)
		if requireCurrent && now.UnixNano() >= componentExpiry {
			return nil, contract.SystemReadySupervisionPolicyCurrentV2{}, nil, 0, contract.NewError(contract.ErrorPrecondition, "availability_component_current_expired", "component production current expired")
		}
		minimum = minAvailabilityExpiryV1(minimum, componentExpiry)
		components = append(components, actual)
	}
	return actuals, policy, components, minimum, nil
}

type availabilityPackageSelectionWatermarkV1 struct {
	SelectionID            string
	SelectionRevision      uint64
	SelectionDigest        contract.DigestV1
	SelectionChecked       int64
	SelectionExpires       int64
	PackageID              string
	PackageRevision        uint64
	PackageDigest          contract.DigestV1
	PackageContractVersion string
	PackageSchemaVersion   string
	PublicationID          string
	PublicationRevision    uint64
	PublicationDigest      contract.DigestV1
	ClosureDigest          contract.DigestV1
}

func (reader *AgentExecutionAvailabilityCurrentReaderV1) inspectPackageSelectionClosureV1(
	ctx context.Context,
	binding contract.HostStartPackageSelectionBindingV1,
	now time.Time,
	requireCurrent bool,
) (availabilityPackageSelectionWatermarkV1, error) {
	expected := binding.PackageSelectionRef
	exact, err := reader.packageSelections.InspectAgentPackageSelectionExactV1(ctx, expected)
	if err != nil {
		return availabilityPackageSelectionWatermarkV1{}, err
	}
	current, err := reader.packageSelections.InspectAgentPackageSelectionCurrentV1(ctx, expected.SelectionID)
	if err != nil {
		return availabilityPackageSelectionWatermarkV1{}, err
	}
	if requireCurrent {
		if err = exact.ValidateCurrent(expected, now); err != nil {
			return availabilityPackageSelectionWatermarkV1{}, err
		}
		if err = current.ValidateCurrent(expected, now); err != nil {
			return availabilityPackageSelectionWatermarkV1{}, err
		}
	} else {
		if err = exact.Validate(); err != nil {
			return availabilityPackageSelectionWatermarkV1{}, err
		}
		if err = current.Validate(); err != nil {
			return availabilityPackageSelectionWatermarkV1{}, err
		}
	}
	if exact.Ref != expected || current.Ref != expected || !reflect.DeepEqual(exact, current) {
		return availabilityPackageSelectionWatermarkV1{}, contract.NewError(contract.ErrorConflict, "availability_package_selection_current_splice", "Package Selection exact and current projections differ from the Host binding")
	}
	closure, err := reader.packageSelections.LoadVerifiedAgentPackageClosureV1(ctx, exact.PackageRef)
	if err != nil {
		return availabilityPackageSelectionWatermarkV1{}, err
	}
	if err = closure.Validate(); err != nil {
		return availabilityPackageSelectionWatermarkV1{}, err
	}
	if closure.Package.RefV1() != exact.PackageRef ||
		closure.PublicationRefV2() != exact.PublicationRef ||
		closure.ClosureDigest != exact.ClosureDigest ||
		contract.DigestV1(closure.ClosureDigest) != binding.VerifiedPackageClosureDigest {
		return availabilityPackageSelectionWatermarkV1{}, contract.NewError(contract.ErrorConflict, "availability_package_selection_closure_splice", "Package Selection and verified Package closure drifted from the Host binding")
	}
	return availabilityPackageSelectionWatermarkV1{
		SelectionID:            exact.Ref.SelectionID,
		SelectionRevision:      uint64(exact.Ref.Revision),
		SelectionDigest:        contract.DigestV1(exact.Ref.Digest),
		SelectionChecked:       exact.CheckedUnixNano,
		SelectionExpires:       exact.ExpiresUnixNano,
		PackageID:              exact.PackageRef.PackageID,
		PackageRevision:        uint64(exact.PackageRef.Revision),
		PackageDigest:          contract.DigestV1(exact.PackageRef.Digest),
		PackageContractVersion: exact.PackageRef.ContractVersion,
		PackageSchemaVersion:   exact.PackageRef.SchemaVersion,
		PublicationID:          exact.PublicationRef.PublicationID,
		PublicationRevision:    uint64(exact.PublicationRef.Revision),
		PublicationDigest:      contract.DigestV1(exact.PublicationRef.Digest),
		ClosureDigest:          contract.DigestV1(exact.ClosureDigest),
	}, nil
}

func validateDeploymentAssociationV1(
	v1 contract.HostDeploymentCurrentV1,
	v2 contract.HostDeploymentCurrentV2,
	expectedV1 contract.HostDeploymentCurrentRefV1,
	expectedV2 contract.HostDeploymentCurrentRefV2,
) error {
	if err := v1.ValidateHistoricalV1(); err != nil {
		return err
	}
	if err := v2.ValidateHistoricalV2(); err != nil {
		return err
	}
	if v1.Ref != expectedV1 || v2.Ref != expectedV2 ||
		v1.Ref.HostID != v2.Ref.HostID ||
		v1.Ref.DeploymentID != v2.Ref.DeploymentID ||
		v1.Ref.Revision != uint64(v2.Ref.Revision) ||
		v1.Ref.BootstrapDigest != v2.Ref.BootstrapDigest ||
		v2.ExpiresUnixNano > v1.ExpiresUnixNano ||
		!slices.Equal(v1.ResourceHandles, v2.ResourceHandles) ||
		!slices.Equal(v1.ServiceBindings, v2.ServiceBindings) {
		return contract.NewError(contract.ErrorConflict, "availability_deployment_v1_v2_splice", "Host deployment V1 and V2 exact closures do not describe one additive deployment")
	}
	return nil
}

func validateAvailabilityPhaseV1(state contract.SystemReadyCurrentStateV2, phase contract.HostInspectPhaseV3) error {
	switch state {
	case contract.SystemReadyCurrentReadyV2:
		if phase != contract.HostInspectReadyV3 {
			return contract.NewError(contract.ErrorPrecondition, "availability_host_not_ready", "ready SystemReady Current has a non-ready Host phase")
		}
	case contract.SystemReadyCurrentFencedV2:
		switch phase {
		case contract.HostInspectStoppingV3, contract.HostInspectClosedV3, contract.HostInspectIndeterminateV3:
		default:
			return contract.NewError(contract.ErrorPrecondition, "availability_fence_phase_drift", "fenced SystemReady Current has no draining, stopped or indeterminate Host phase")
		}
	default:
		return contract.NewError(contract.ErrorInvalidArgument, "availability_state_invalid", "SystemReady Current state is unsupported")
	}
	return nil
}

func componentProductionExpiryV1(value contract.ComponentProductionCurrentV2) int64 {
	minimum := value.ReleaseCurrent.ExpiresUnixNano
	for _, expiry := range []int64{
		value.Binding.ExpiresUnixNano,
		value.GenerationCurrent.ExpiresUnixNano,
		value.ActivationCurrent.ExpiresUnixNano,
		value.ProductionCurrent.ExpiresUnixNano,
	} {
		minimum = minAvailabilityExpiryV1(minimum, expiry)
	}
	return minimum
}

func (reader *AgentExecutionAvailabilityCurrentReaderV1) freshNowV1() (time.Time, error) {
	reader.clockMu.Lock()
	defer reader.clockMu.Unlock()
	now := reader.clock()
	if now.IsZero() || (!reader.lastClock.IsZero() && now.Before(reader.lastClock)) {
		return time.Time{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "availability current clock regressed")
	}
	reader.lastClock = now
	return now, nil
}

func availabilityContextV1(ctx context.Context) error {
	if ctx == nil {
		return contract.NewError(contract.ErrorInvalidArgument, "context_missing", "availability current context is required")
	}
	if err := ctx.Err(); err != nil {
		return contract.NewError(contract.ErrorUnavailable, "context_ended", "availability current context ended")
	}
	return nil
}

func minAvailabilityExpiryV1(left, right int64) int64 {
	if right < left {
		return right
	}
	return left
}

var _ runtimeports.AgentExecutionAvailabilityCurrentReaderV1 = (*AgentExecutionAvailabilityCurrentReaderV1)(nil)
