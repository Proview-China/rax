package lifecycle

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ConfigV2 struct {
	StartAdmission  *journal.HostStartAdmissionV1
	Journal         *journal.CoordinatorV2
	JournalFacts    ports.JournalFactPortV2
	Definition      ports.DefinitionOperationV2
	Assembler       ports.AssemblyPlanOperationV2
	Compiler        ports.HarnessCompileOperationV2
	Publisher       ports.AssemblyPublisherV2
	PublicationRead ports.AssemblyPublicationInspectorV2
	Inputs          ports.HostV2StageInputsAssemblerV2
	Binding         ports.BindingAdmissionGovernancePortV2
	Control         ports.ControlAdapterConstructionGatewayV2
	Activation      ports.AgentActivationGovernancePortV2
	Generation      ports.GenerationAssociationGovernancePortV2
	Ready           ports.SystemReadyGovernancePortV2
	CleanupPlans    ports.CleanupPlanCurrentReaderV2
	CleanupAttempts ports.CleanupAttemptFactPortV2
	CleanupNodes    ports.CleanupNodeOperationRegistryV2
	Clock           func() time.Time
}

// HostV2 is an injectable reference coordinator. It deliberately has no CLI,
// backend discovery or production composition root.
type HostV2 struct {
	config ConfigV2
}

var _ ports.HostV2 = (*HostV2)(nil)

func NewHostV2(config ConfigV2) (*HostV2, error) {
	dependencies := []struct {
		name  string
		value any
	}{
		{"start admission", config.StartAdmission}, {"journal", config.Journal},
		{"journal facts", config.JournalFacts}, {"definition", config.Definition},
		{"assembler", config.Assembler}, {"compiler", config.Compiler},
		{"publisher", config.Publisher}, {"publication reader", config.PublicationRead}, {"stage inputs", config.Inputs},
		{"binding", config.Binding}, {"control", config.Control},
		{"activation", config.Activation}, {"generation", config.Generation},
		{"system ready", config.Ready},
	}
	for _, dependency := range dependencies {
		if contract.IsTypedNilV1(dependency.value) {
			return nil, contract.NewError(contract.ErrorInvalidArgument, "host_v2_dependency_missing", dependency.name+" is required")
		}
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &HostV2{config: config}, nil
}

func (h *HostV2) StartV2(ctx context.Context, request contract.StartRequestV2) (contract.StartResultV2, error) {
	if h == nil {
		return contract.StartResultV2{}, contract.NewError(contract.ErrorUnavailable, "host_v2_missing", "HostV2 is unavailable")
	}
	if ctx == nil {
		return contract.StartResultV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	now, err := h.freshNowV2(time.Time{})
	if err != nil {
		return contract.StartResultV2{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return contract.StartResultV2{}, err
	}
	claim, err := request.ClaimV1()
	if err != nil {
		return contract.StartResultV2{}, err
	}
	claim, err = h.config.StartAdmission.ClaimV1(ctx, claim)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	current, err := h.config.Journal.EnsureAcceptedV2(ctx, claim)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	if current.Phase == contract.HostClosedV2 || current.Phase == contract.HostDrainingV2 || current.Phase == contract.HostReconcilingV2 || current.Phase == contract.HostIndeterminateV2 {
		return contract.StartResultV2{}, contract.NewError(contract.ErrorPrecondition, "host_v2_not_startable", "HostV2 lifecycle is not resumable by Start")
	}

	current, err = h.ensurePhaseV2(ctx, current, contract.HostValidatingV2)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	var definition contract.DecodedDefinitionV1
	err = h.runOwnerStepV2(ctx, request, contract.HostValidatingV2, "praxis.agent-host/definition", "definition", []contract.HostOperationCoordinateV2{startRequestCoordinateV2(request)},
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (contract.DecodedDefinitionV1, error) {
				return h.config.Definition.StartOrInspectDefinitionV2(callCtx, request)
			}, definitionCoordinateV2, func(value contract.DecodedDefinitionV1, _ time.Time, callErr error) error {
				definition = value
				return validateDefinitionResultV2(value, callErr)
			})
		},
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (contract.DecodedDefinitionV1, error) {
				return h.config.Definition.InspectDefinitionV2(callCtx, request)
			}, definitionCoordinateV2, func(value contract.DecodedDefinitionV1, _ time.Time, callErr error) error {
				definition = value
				return validateDefinitionResultV2(value, callErr)
			})
		})
	if err != nil {
		return contract.StartResultV2{}, err
	}

	current, err = h.reloadAndEnsurePhaseV2(ctx, request, contract.HostResolvingV2)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	var resolved contract.ResolvedAssemblyV1
	err = h.runOwnerStepV2(ctx, request, contract.HostResolvingV2, "praxis.agent-host/assembly-plan", "assembly-plan", []contract.HostOperationCoordinateV2{definitionCoordinateV2(definition)},
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (contract.ResolvedAssemblyV1, error) {
				return h.config.Assembler.StartOrInspectAssemblyPlanV2(callCtx, request, definition)
			}, resolvedCoordinateV2, func(value contract.ResolvedAssemblyV1, _ time.Time, callErr error) error {
				resolved = value
				return validateResolvedResultV2(value, callErr)
			})
		},
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (contract.ResolvedAssemblyV1, error) {
				return h.config.Assembler.InspectAssemblyPlanV2(callCtx, request, definition)
			}, resolvedCoordinateV2, func(value contract.ResolvedAssemblyV1, _ time.Time, callErr error) error {
				resolved = value
				return validateResolvedResultV2(value, callErr)
			})
		})
	if err != nil {
		return contract.StartResultV2{}, err
	}

	current, err = h.reloadAndEnsurePhaseV2(ctx, request, contract.HostCompilingV2)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	var compiled contract.CompiledAssemblyArtifactsV2
	err = h.runOwnerStepV2(ctx, request, contract.HostCompilingV2, "praxis.harness/compile", "compile", []contract.HostOperationCoordinateV2{resolvedCoordinateV2(resolved)},
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (contract.CompiledAssemblyArtifactsV2, error) {
				return h.config.Compiler.StartOrInspectHarnessCompileV2(callCtx, request, resolved)
			}, compiledCoordinateV2, func(value contract.CompiledAssemblyArtifactsV2, now time.Time, callErr error) error {
				compiled = value
				return validateCompiledResultV2(value, now, callErr)
			})
		},
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (contract.CompiledAssemblyArtifactsV2, error) {
				return h.config.Compiler.InspectHarnessCompileV2(callCtx, request, resolved)
			}, compiledCoordinateV2, func(value contract.CompiledAssemblyArtifactsV2, now time.Time, callErr error) error {
				compiled = value
				return validateCompiledResultV2(value, now, callErr)
			})
		})
	if err != nil {
		return contract.StartResultV2{}, err
	}
	publicationRequest, err := h.config.Inputs.BuildAssemblyPublicationRequestV2(ctx, request, compiled)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	publicationNow, err := h.freshNowV2(time.Unix(0, current.UpdatedUnixNano))
	if err != nil {
		return contract.StartResultV2{}, err
	}
	if err = publicationRequest.ValidateAt(publicationNow); err != nil {
		return contract.StartResultV2{}, err
	}
	if publicationRequest.HostID != request.Config.HostID || publicationRequest.StartID != request.StartID || publicationRequest.Artifacts.Digest != compiled.Digest {
		return contract.StartResultV2{}, contract.NewError(contract.ErrorConflict, "assembly_publication_request_splice", "Assembly publication request drifted from HostV2 compile")
	}
	var assembly contract.AssemblyPublicationResultV2
	publicationJournal, inspectJournalErr := h.config.JournalFacts.InspectHostJournalV2(ctx, request.Config.HostID, request.StartID)
	if inspectJournalErr != nil {
		return contract.StartResultV2{}, inspectJournalErr
	}
	publicationComplete, completeErr := publicationAttemptsCompleteV2(publicationJournal, publicationRequest.AttemptID)
	if completeErr != nil {
		return contract.StartResultV2{}, completeErr
	}
	if publicationComplete {
		assembly, err = h.config.PublicationRead.InspectAssemblyPublicationV2(ctx, publicationRequest)
	} else {
		assembly, err = h.config.Publisher.PublishAssemblyV2(ctx, publicationRequest)
	}
	if err != nil {
		return contract.StartResultV2{}, err
	}
	publicationChecked, err := h.freshNowV2(publicationNow)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	if err = assembly.ValidateAt(publicationChecked); err != nil {
		return contract.StartResultV2{}, err
	}

	current, err = h.reloadAndEnsurePhaseV2(ctx, request, contract.HostBindingV2)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	bindingRequest, err := h.config.Inputs.BuildBindingAdmissionRequestV2(ctx, request, definition, resolved, assembly)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	if err = bindingRequest.Validate(); err != nil {
		return contract.StartResultV2{}, err
	}
	if bindingRequest.AssemblyCurrent != assembly.OwnerCurrent {
		return contract.StartResultV2{}, contract.NewError(contract.ErrorConflict, "binding_assembly_current_splice", "Binding admission does not bind the published Assembly current")
	}
	var binding runtimeports.BindingAdmissionResultV1
	err = h.runOwnerStepV2(ctx, request, contract.HostBindingV2, "praxis.runtime/binding-admission", bindingRequest.AttemptID, bindingInputsV2(bindingRequest),
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (runtimeports.BindingAdmissionResultV1, error) {
				return h.config.Binding.StartOrInspectBindingAdmissionV1(callCtx, bindingRequest)
			}, bindingCoordinateV2, func(value runtimeports.BindingAdmissionResultV1, now time.Time, callErr error) error {
				binding = value
				return validateBindingResultV2(value, bindingRequest, now, callErr)
			})
		},
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (runtimeports.BindingAdmissionResultV1, error) {
				return h.config.Binding.InspectBindingAdmissionV1(callCtx, runtimeports.BindingAdmissionInspectRequestV1{AttemptID: bindingRequest.AttemptID, RequestDigest: bindingRequest.RequestDigest})
			}, bindingCoordinateV2, func(value runtimeports.BindingAdmissionResultV1, now time.Time, callErr error) error {
				binding = value
				return validateBindingResultV2(value, bindingRequest, now, callErr)
			})
		})
	if err != nil {
		return contract.StartResultV2{}, err
	}

	current, err = h.reloadAndEnsurePhaseV2(ctx, request, contract.HostConstructingControlV2)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	controlRequests, err := h.config.Inputs.BuildControlAdapterRequestsV2(ctx, request, assembly, binding)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	controlRequests, err = validateControlRequestSetV2(controlRequests, binding, assembly)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	controls := make([]contract.ControlAdapterInstanceV2, len(controlRequests))
	for index := range controlRequests {
		controlRequest := controlRequests[index]
		err = h.runOwnerStepV2(ctx, request, contract.HostConstructingControlV2, "praxis.agent-host/control-adapter", controlRequest.AttemptID, controlInputsV2(controlRequest),
			func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
				return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (contract.ControlAdapterInstanceV2, error) {
					return h.config.Control.StartOrInspectControlAdapterConstructionV2(callCtx, controlRequest)
				}, controlCoordinateV2, func(value contract.ControlAdapterInstanceV2, now time.Time, callErr error) error {
					controls[index] = value
					return validateControlResultV2(value, controlRequest, now, callErr)
				})
			},
			func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
				return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (contract.ControlAdapterInstanceV2, error) {
					return h.config.Control.InspectControlAdapterConstructionV2(callCtx, controlRequest)
				}, controlCoordinateV2, func(value contract.ControlAdapterInstanceV2, now time.Time, callErr error) error {
					controls[index] = value
					return validateControlResultV2(value, controlRequest, now, callErr)
				})
			})
		if err != nil {
			return contract.StartResultV2{}, err
		}
	}

	current, err = h.reloadAndEnsurePhaseV2(ctx, request, contract.HostActivatingV2)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	activationRequest, err := h.config.Inputs.BuildAgentActivationRequestV2(ctx, request, assembly, binding, controls)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	if err = activationRequest.Validate(); err != nil {
		return contract.StartResultV2{}, err
	}
	if activationRequest.AssemblyCurrent != assembly.OwnerCurrent || !sameBindingOwnerCurrentV2(activationRequest.BindingSetCurrent, binding.BindingSet) {
		return contract.StartResultV2{}, contract.NewError(contract.ErrorConflict, "activation_inputs_splice", "Activation request drifted from exact Assembly or BindingSet")
	}
	var activation applicationcontract.AgentActivationResultV1
	err = h.runOwnerStepV2(ctx, request, contract.HostActivatingV2, "praxis.application/agent-activation", activationRequest.AttemptID, activationInputsV2(activationRequest),
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (applicationcontract.AgentActivationResultV1, error) {
				return h.config.Activation.StartOrInspectAgentActivationV1(callCtx, activationRequest)
			}, func(value applicationcontract.AgentActivationResultV1) contract.HostOperationCoordinateV2 {
				return contract.HostV2OwnerCurrentCoordinate(value.Ref)
			}, func(value applicationcontract.AgentActivationResultV1, now time.Time, callErr error) error {
				activation = value
				return validateActivationResultV2(value, activationRequest, now, callErr)
			})
		},
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (applicationcontract.AgentActivationResultV1, error) {
				return h.config.Activation.InspectAgentActivationV1(callCtx, activationRequest)
			}, func(value applicationcontract.AgentActivationResultV1) contract.HostOperationCoordinateV2 {
				return contract.HostV2OwnerCurrentCoordinate(value.Ref)
			}, func(value applicationcontract.AgentActivationResultV1, now time.Time, callErr error) error {
				activation = value
				return validateActivationResultV2(value, activationRequest, now, callErr)
			})
		})
	if err != nil {
		return contract.StartResultV2{}, err
	}

	current, err = h.reloadAndEnsurePhaseV2(ctx, request, contract.HostAssociatingGenerationV2)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	generationCandidate, err := h.config.Inputs.BuildGenerationAssociationCandidateV2(ctx, request, assembly, binding, activation)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	if err = generationCandidate.Validate(); err != nil {
		return contract.StartResultV2{}, err
	}
	if generationCandidate.Binding.BindingSetID != binding.BindingSet.ID || generationCandidate.Binding.BindingSetRevision != binding.BindingSet.Revision || generationCandidate.Binding.BindingSetDigest != binding.BindingSet.Digest {
		return contract.StartResultV2{}, contract.NewError(contract.ErrorConflict, "generation_binding_splice", "Generation association does not bind the admitted BindingSet")
	}
	if generationCandidate.Generation.Generation.ID != assembly.Generation.ID || uint64(generationCandidate.Generation.Generation.Revision) != assembly.Generation.Revision || generationCandidate.Generation.Generation.Digest != core.Digest(assembly.Generation.Digest) {
		return contract.StartResultV2{}, contract.NewError(contract.ErrorConflict, "generation_artifact_splice", "Generation association does not bind the published Generation")
	}
	if generationCandidate.Activation.Operation.Kind != runtimeports.OperationScopeActivationV3 || generationCandidate.Activation.Operation.ActivationAttemptID != activation.AttemptID || generationCandidate.Activation.Operation.ExecutionScopeDigest != activation.ExecutionScopeDigest {
		return contract.StartResultV2{}, contract.NewError(contract.ErrorConflict, "generation_activation_splice", "Generation association does not bind the activated execution scope")
	}
	var generation runtimeports.GenerationBindingAssociationFactV1
	err = h.runOwnerStepV2(ctx, request, contract.HostAssociatingGenerationV2, "praxis.runtime/generation-association", generationCandidate.AssociationID, generationInputsV2(generationCandidate),
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (runtimeports.GenerationBindingAssociationFactV1, error) {
				return h.config.Generation.AssociateGenerationBindingV1(callCtx, generationCandidate)
			}, generationCoordinateV2, func(value runtimeports.GenerationBindingAssociationFactV1, now time.Time, callErr error) error {
				generation = value
				return validateGenerationResultV2(value, generationCandidate, now, callErr)
			})
		},
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (runtimeports.GenerationBindingAssociationFactV1, error) {
				return h.config.Generation.InspectCurrentGenerationBindingAssociationV1(callCtx, generationCandidate.AssociationID)
			}, generationCoordinateV2, func(value runtimeports.GenerationBindingAssociationFactV1, now time.Time, callErr error) error {
				generation = value
				return validateGenerationResultV2(value, generationCandidate, now, callErr)
			})
		})
	if err != nil {
		return contract.StartResultV2{}, err
	}

	current, err = h.reloadAndEnsurePhaseV2(ctx, request, contract.HostVerifyingV2)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	readyRequest, err := h.config.Inputs.BuildSystemReadyRequestV2(ctx, request, claim, definition, resolved, assembly, binding, controls, activation, generation)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	if err = readyRequest.Validate(); err != nil {
		return contract.StartResultV2{}, err
	}
	claimRef, _ := claim.CurrentRefV1()
	if readyRequest.HostID != request.Config.HostID || readyRequest.StartID != request.StartID || readyRequest.Claim != claimRef || readyRequest.Assembly != assembly.OwnerCurrent || readyRequest.Activation != activation.ActivationCurrent || readyRequest.SandboxLease != activation.SandboxLeaseCurrent || readyRequest.SandboxActive != activation.SandboxActiveCurrent || readyRequest.ExecutionReady != activation.ExecutionReadyCurrent {
		return contract.StartResultV2{}, contract.NewError(contract.ErrorConflict, "system_ready_inputs_splice", "SystemReady request drifted from exact HostV2 outputs")
	}
	if err = validateReadyControlSetV2(readyRequest, controlRequests, controls); err != nil {
		return contract.StartResultV2{}, err
	}
	var ready contract.SystemReadyGatewayResultV2
	err = h.runOwnerStepV2(ctx, request, contract.HostVerifyingV2, "praxis.agent-host/system-ready", readyRequest.AttemptID, readyInputsV2(readyRequest),
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (contract.SystemReadyGatewayResultV2, error) {
				return h.config.Ready.StartOrInspectSystemReadyV2(callCtx, readyRequest)
			}, readyCoordinateV2, func(value contract.SystemReadyGatewayResultV2, now time.Time, callErr error) error {
				ready = value
				return validateReadyResultV2(value, now, callErr)
			})
		},
		func(callCtx context.Context) (contract.HostOperationCoordinateV2, error) {
			return timedOwnerCallV2(h, current.UpdatedUnixNano, func() (contract.SystemReadyGatewayResultV2, error) {
				return h.config.Ready.InspectSystemReadyV2(callCtx, contract.SystemReadyInspectRequestV2{HostID: request.Config.HostID, StartID: request.StartID, AttemptID: readyRequest.AttemptID, RequestDigest: readyRequest.RequestDigest})
			}, readyCoordinateV2, func(value contract.SystemReadyGatewayResultV2, now time.Time, callErr error) error {
				ready = value
				return validateReadyResultV2(value, now, callErr)
			})
		})
	if err != nil {
		return contract.StartResultV2{}, err
	}
	// Re-read the exact Ready closure immediately before advancing the Host
	// phase. A structural success reply is not a currentness lease, and a TTL
	// crossing here must leave the Host in verifying rather than publish Ready.
	readyCoordinate := readyCoordinateV2(ready)
	_, err = timedOwnerCallV2(h, current.UpdatedUnixNano, func() (contract.SystemReadyGatewayResultV2, error) {
		return h.config.Ready.InspectSystemReadyV2(context.WithoutCancel(ctx), contract.SystemReadyInspectRequestV2{HostID: request.Config.HostID, StartID: request.StartID, AttemptID: readyRequest.AttemptID, RequestDigest: readyRequest.RequestDigest})
	}, readyCoordinateV2, func(value contract.SystemReadyGatewayResultV2, now time.Time, callErr error) error {
		if err := validateReadyResultV2(value, now, callErr); err != nil {
			return err
		}
		if readyCoordinateV2(value) != readyCoordinate {
			return contract.NewError(contract.ErrorConflict, "system_ready_reread_splice", "SystemReady changed before Host Ready publication")
		}
		ready = value
		return nil
	})
	if err != nil {
		return contract.StartResultV2{}, err
	}
	current, err = h.reloadAndEnsurePhaseV2(ctx, request, contract.HostReadyV2)
	if err != nil {
		return contract.StartResultV2{}, err
	}
	return h.sealStartResultV2(request, current, definition, resolved, compiled, assembly, binding, controls, activation, generation, ready)
}

func (h *HostV2) InspectV2(ctx context.Context, request contract.InspectRequestV2) (contract.InspectResultV2, error) {
	if h == nil {
		return contract.InspectResultV2{}, contract.NewError(contract.ErrorUnavailable, "host_v2_missing", "HostV2 is unavailable")
	}
	if ctx == nil {
		return contract.InspectResultV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := request.Validate(); err != nil {
		return contract.InspectResultV2{}, err
	}
	current, err := h.config.Journal.InspectV2(ctx, request.HostID, request.StartID)
	if err != nil {
		return contract.InspectResultV2{}, err
	}
	if current.StartClaimRef != request.StartClaimRef {
		return contract.InspectResultV2{}, contract.NewError(contract.ErrorConflict, "host_v2_inspect_claim_drift", "HostV2 Inspect names another exact Start claim")
	}
	return contract.InspectResultV2{ContractVersion: contract.HostLifecycleContractVersionV2, Journal: current, Phase: current.Phase}, nil
}

// runOwnerStepV2 grants a dispatch token only when this call receives the
// normal exact CAS response that appended intent_recorded. A recovered CAS,
// existing intent, restart, conflict or unknown response is Inspect-only.
func (h *HostV2) runOwnerStepV2(ctx context.Context, request contract.StartRequestV2, phase contract.HostPhaseV2, operationKind, attemptSuffix string, inputs []contract.HostOperationCoordinateV2, start, inspect func(context.Context) (contract.HostOperationCoordinateV2, error)) error {
	requestDigest, err := contract.DigestJSONV1(struct {
		StartDigest contract.DigestV1                    `json:"start_digest"`
		Kind        string                               `json:"kind"`
		Inputs      []contract.HostOperationCoordinateV2 `json:"inputs"`
	}{StartDigest: request.RequestDigest, Kind: operationKind, Inputs: canonicalCoordinatesV2(inputs)})
	if err != nil {
		return err
	}
	attemptID := "host-v2/" + attemptSuffix + "/" + trimDigestV2(requestDigest)
	current, err := h.config.JournalFacts.InspectHostJournalV2(ctx, request.Config.HostID, request.StartID)
	if err != nil {
		return err
	}
	index := findHostOperationV2(current, attemptID)
	owned := false
	if index < 0 {
		if current.Phase != phase {
			return contract.NewError(contract.ErrorConflict, "host_v2_operation_phase_drift", "HostV2 owner operation is absent after its phase")
		}
		now, nowErr := h.freshNowV2(time.Unix(0, current.UpdatedUnixNano))
		if nowErr != nil {
			return nowErr
		}
		attempt, sealErr := contract.SealHostOperationAttemptV2(contract.HostOperationAttemptV2{ContractVersion: contract.HostJournalContractVersionV2, AttemptID: attemptID, Revision: 1, OperationKind: operationKind, Phase: phase, RequestDigest: requestDigest, Inputs: canonicalCoordinatesV2(inputs), State: contract.HostOperationIntentRecordedV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
		if sealErr != nil {
			return sealErr
		}
		next := current
		next.Revision++
		next.UpdatedUnixNano = now.UnixNano()
		next.Operations = append(append([]contract.HostOperationAttemptV2(nil), current.Operations...), attempt)
		next.Digest = ""
		next, sealErr = contract.SealHostJournalV2(next)
		if sealErr != nil {
			return sealErr
		}
		expected, _ := current.RefV2()
		actual, writeErr := safeJournalCASV2(ctx, h.config.JournalFacts, expected, next)
		if writeErr == nil && actual.Digest == next.Digest {
			current, index, owned = actual, len(actual.Operations)-1, true
		} else {
			current, err = h.config.JournalFacts.InspectHostJournalV2(context.WithoutCancel(ctx), request.Config.HostID, request.StartID)
			if err != nil {
				return firstErrorV2(writeErr, err)
			}
			index = findHostOperationV2(current, attemptID)
			if index < 0 {
				return firstErrorV2(writeErr, contract.NewError(contract.ErrorUnknownOutcome, "host_v2_intent_unknown", "HostV2 operation intent outcome is unknown"))
			}
		}
	}
	attempt := current.Operations[index]
	if attempt.OperationKind != operationKind || attempt.RequestDigest != requestDigest {
		return contract.NewError(contract.ErrorConflict, "host_v2_operation_splice", "HostV2 operation attempt drifted")
	}
	if attempt.State == contract.HostOperationResultRecordedV2 {
		actual, inspectErr := safeCoordinateCallV2(context.WithoutCancel(ctx), inspect)
		if inspectErr != nil {
			return inspectErr
		}
		if attempt.Result == nil || *attempt.Result != actual {
			return contract.NewError(contract.ErrorConflict, "host_v2_operation_result_splice", "Owner Inspect drifted from the recorded Host result")
		}
		return nil
	}
	if !owned {
		actual, inspectErr := safeCoordinateCallV2(context.WithoutCancel(ctx), inspect)
		if inspectErr != nil {
			reconcileErr := h.markOwnerStepReconciliationV2(context.WithoutCancel(ctx), request, attemptID, requestDigest)
			return errors.Join(contract.NewError(contract.ErrorUnknownOutcome, "host_v2_operation_inspect_required", "HostV2 operation is permanently Inspect-only until its exact result is visible"), inspectErr, reconcileErr)
		}
		return h.settleOwnerStepV2(context.WithoutCancel(ctx), request, attemptID, requestDigest, actual)
	}
	actual, startErr := safeCoordinateCallV2(ctx, start)
	if startErr != nil {
		inspected, inspectErr := safeCoordinateCallV2(context.WithoutCancel(ctx), inspect)
		if inspectErr == nil {
			return h.settleOwnerStepV2(context.WithoutCancel(ctx), request, attemptID, requestDigest, inspected)
		}
		markErr := h.markOwnerStepReconciliationV2(context.WithoutCancel(ctx), request, attemptID, requestDigest)
		return errors.Join(contract.NewError(contract.ErrorUnknownOutcome, "host_v2_operation_reconciliation_required", "HostV2 Owner call may have started and requires exact Inspect reconciliation"), startErr, inspectErr, markErr)
	}
	return h.settleOwnerStepV2(ctx, request, attemptID, requestDigest, actual)
}

func (h *HostV2) settleOwnerStepV2(ctx context.Context, request contract.StartRequestV2, attemptID string, requestDigest contract.DigestV1, result contract.HostOperationCoordinateV2) error {
	if err := result.Validate(); err != nil {
		return err
	}
	for range 8 {
		current, err := h.config.JournalFacts.InspectHostJournalV2(ctx, request.Config.HostID, request.StartID)
		if err != nil {
			return err
		}
		index := findHostOperationV2(current, attemptID)
		if index < 0 {
			return contract.NewError(contract.ErrorConflict, "host_v2_operation_missing", "HostV2 operation disappeared")
		}
		attempt := current.Operations[index]
		if attempt.RequestDigest != requestDigest {
			return contract.NewError(contract.ErrorConflict, "host_v2_operation_splice", "HostV2 operation digest drifted")
		}
		if attempt.State == contract.HostOperationResultRecordedV2 {
			if attempt.Result != nil && *attempt.Result == result {
				return nil
			}
			return contract.NewError(contract.ErrorConflict, "host_v2_operation_result_splice", "HostV2 operation recorded another result")
		}
		now, err := h.freshNowV2(time.Unix(0, current.UpdatedUnixNano))
		if err != nil {
			return err
		}
		nextAttempt := attempt
		nextAttempt.Revision++
		nextAttempt.State = contract.HostOperationResultRecordedV2
		nextAttempt.Result = &result
		nextAttempt.UpdatedUnixNano = now.UnixNano()
		nextAttempt.Digest = ""
		nextAttempt, err = contract.SealHostOperationAttemptV2(nextAttempt)
		if err != nil {
			return err
		}
		next := current
		next.Revision++
		next.UpdatedUnixNano = now.UnixNano()
		next.Operations = append([]contract.HostOperationAttemptV2(nil), current.Operations...)
		next.Operations[index] = nextAttempt
		next.Digest = ""
		next, err = contract.SealHostJournalV2(next)
		if err != nil {
			return err
		}
		expected, _ := current.RefV2()
		actual, writeErr := safeJournalCASV2(ctx, h.config.JournalFacts, expected, next)
		if writeErr == nil && actual.Digest == next.Digest {
			return nil
		}
		inspected, inspectErr := h.config.JournalFacts.InspectHostJournalV2(context.WithoutCancel(ctx), request.Config.HostID, request.StartID)
		if inspectErr == nil {
			idx := findHostOperationV2(inspected, attemptID)
			if idx >= 0 && inspected.Operations[idx].Digest == nextAttempt.Digest {
				return nil
			}
		}
		if writeErr != nil && !contract.HasCode(writeErr, contract.ErrorConflict) {
			return writeErr
		}
	}
	return contract.NewError(contract.ErrorUnknownOutcome, "host_v2_operation_settlement_contention", "HostV2 operation result did not settle after bounded CAS contention")
}

func (h *HostV2) markOwnerStepUnknownV2(ctx context.Context, request contract.StartRequestV2, attemptID string, requestDigest contract.DigestV1) error {
	current, err := h.config.JournalFacts.InspectHostJournalV2(ctx, request.Config.HostID, request.StartID)
	if err != nil {
		return err
	}
	index := findHostOperationV2(current, attemptID)
	if index < 0 {
		return contract.NewError(contract.ErrorUnknownOutcome, "host_v2_operation_missing", "HostV2 operation intent is missing")
	}
	attempt := current.Operations[index]
	if attempt.RequestDigest != requestDigest || attempt.State != contract.HostOperationIntentRecordedV2 {
		return nil
	}
	now, err := h.freshNowV2(time.Unix(0, current.UpdatedUnixNano))
	if err != nil {
		return err
	}
	nextAttempt := attempt
	nextAttempt.Revision++
	nextAttempt.State = contract.HostOperationOutcomeUnknownV2
	nextAttempt.UpdatedUnixNano = now.UnixNano()
	nextAttempt.Digest = ""
	nextAttempt, err = contract.SealHostOperationAttemptV2(nextAttempt)
	if err != nil {
		return err
	}
	next := current
	next.Revision++
	next.UpdatedUnixNano = now.UnixNano()
	next.Operations = append([]contract.HostOperationAttemptV2(nil), current.Operations...)
	next.Operations[index] = nextAttempt
	next.Digest = ""
	next, err = contract.SealHostJournalV2(next)
	if err != nil {
		return err
	}
	expected, _ := current.RefV2()
	_, err = safeJournalCASV2(ctx, h.config.JournalFacts, expected, next)
	return err
}

func (h *HostV2) markOwnerStepReconciliationV2(ctx context.Context, request contract.StartRequestV2, attemptID string, requestDigest contract.DigestV1) error {
	for range 4 {
		current, err := h.config.JournalFacts.InspectHostJournalV2(ctx, request.Config.HostID, request.StartID)
		if err != nil {
			return err
		}
		index := findHostOperationV2(current, attemptID)
		if index < 0 {
			return contract.NewError(contract.ErrorUnknownOutcome, "host_v2_operation_missing", "HostV2 operation intent is missing")
		}
		attempt := current.Operations[index]
		if attempt.RequestDigest != requestDigest {
			return contract.NewError(contract.ErrorConflict, "host_v2_operation_splice", "HostV2 operation request digest drifted")
		}
		if attempt.State == contract.HostOperationResultRecordedV2 || attempt.State == contract.HostOperationReconciliationRequiredV2 {
			return nil
		}
		nextState := contract.HostOperationOutcomeUnknownV2
		if attempt.State == contract.HostOperationOutcomeUnknownV2 {
			nextState = contract.HostOperationReconciliationRequiredV2
		}
		now, err := h.freshNowV2(time.Unix(0, current.UpdatedUnixNano))
		if err != nil {
			return err
		}
		nextAttempt := attempt
		nextAttempt.Revision++
		nextAttempt.State = nextState
		nextAttempt.UpdatedUnixNano = now.UnixNano()
		nextAttempt.Digest = ""
		nextAttempt, err = contract.SealHostOperationAttemptV2(nextAttempt)
		if err != nil {
			return err
		}
		next := current
		next.Revision++
		next.UpdatedUnixNano = now.UnixNano()
		next.Operations = append([]contract.HostOperationAttemptV2(nil), current.Operations...)
		next.Operations[index] = nextAttempt
		next.Digest = ""
		next, err = contract.SealHostJournalV2(next)
		if err != nil {
			return err
		}
		expected, _ := current.RefV2()
		actual, writeErr := safeJournalCASV2(ctx, h.config.JournalFacts, expected, next)
		if writeErr == nil && actual.Digest == next.Digest {
			continue
		}
		inspected, inspectErr := h.config.JournalFacts.InspectHostJournalV2(context.WithoutCancel(ctx), request.Config.HostID, request.StartID)
		if inspectErr == nil {
			idx := findHostOperationV2(inspected, attemptID)
			if idx >= 0 && inspected.Operations[idx].Digest == nextAttempt.Digest {
				continue
			}
		}
		if writeErr != nil && !contract.HasCode(writeErr, contract.ErrorConflict) {
			return writeErr
		}
	}
	return contract.NewError(contract.ErrorUnknownOutcome, "host_v2_reconciliation_contention", "HostV2 operation could not persist reconciliation_required")
}

func (h *HostV2) reloadAndEnsurePhaseV2(ctx context.Context, request contract.StartRequestV2, phase contract.HostPhaseV2) (contract.HostJournalV2, error) {
	current, err := h.config.JournalFacts.InspectHostJournalV2(ctx, request.Config.HostID, request.StartID)
	if err != nil {
		return contract.HostJournalV2{}, err
	}
	return h.ensurePhaseV2(ctx, current, phase)
}

func (h *HostV2) ensurePhaseV2(ctx context.Context, current contract.HostJournalV2, target contract.HostPhaseV2) (contract.HostJournalV2, error) {
	if hostStartPhaseRankV2(current.Phase) > hostStartPhaseRankV2(target) {
		return current, nil
	}
	for current.Phase != target {
		nextPhase, ok := nextStartPhaseV2(current.Phase)
		if !ok {
			return contract.HostJournalV2{}, contract.NewError(contract.ErrorPrecondition, "host_v2_phase_not_reachable", "HostV2 target phase is not reachable")
		}
		now, err := h.freshNowV2(time.Unix(0, current.UpdatedUnixNano))
		if err != nil {
			return contract.HostJournalV2{}, err
		}
		next := current
		next.Revision++
		next.Phase = nextPhase
		next.UpdatedUnixNano = now.UnixNano()
		next.Digest = ""
		next, err = contract.SealHostJournalV2(next)
		if err != nil {
			return contract.HostJournalV2{}, err
		}
		current, err = h.config.Journal.AdvanceV2(ctx, current, next)
		if err != nil {
			return contract.HostJournalV2{}, err
		}
	}
	return current, nil
}

func hostStartPhaseRankV2(phase contract.HostPhaseV2) int {
	order := []contract.HostPhaseV2{contract.HostAcceptedV2, contract.HostValidatingV2, contract.HostResolvingV2, contract.HostCompilingV2, contract.HostBindingV2, contract.HostConstructingControlV2, contract.HostActivatingV2, contract.HostAssociatingGenerationV2, contract.HostVerifyingV2, contract.HostReadyV2}
	for index, value := range order {
		if value == phase {
			return index
		}
	}
	return -1
}

func (h *HostV2) sealStartResultV2(request contract.StartRequestV2, current contract.HostJournalV2, definition contract.DecodedDefinitionV1, resolved contract.ResolvedAssemblyV1, compiled contract.CompiledAssemblyArtifactsV2, assembly contract.AssemblyPublicationResultV2, binding runtimeports.BindingAdmissionResultV1, controls []contract.ControlAdapterInstanceV2, activation applicationcontract.AgentActivationResultV1, generation runtimeports.GenerationBindingAssociationFactV1, ready contract.SystemReadyGatewayResultV2) (contract.StartResultV2, error) {
	now, err := h.freshNowV2(time.Unix(0, current.UpdatedUnixNano))
	if err != nil {
		return contract.StartResultV2{}, err
	}
	journalRef, err := current.RefV2()
	if err != nil {
		return contract.StartResultV2{}, err
	}
	expires := request.RequestedNotAfterUnixNano
	values := []int64{compiled.ExpiresUnixNano, assembly.OwnerCurrent.ExpiresUnixNano, binding.ExpiresUnixNano, activation.ExpiresUnixNano}
	values = append(values, ready.Fact.ExpiresUnixNano)
	for _, control := range controls {
		values = append(values, control.ExpiresUnixNano)
	}
	for _, value := range values {
		if value < expires {
			expires = value
		}
	}
	return contract.SealStartResultV2(contract.StartResultV2{HostID: request.Config.HostID, StartID: request.StartID, RequestDigest: request.RequestDigest, RequestNotAfterUnixNano: request.RequestedNotAfterUnixNano, Journal: journalRef, Outputs: contract.HostV2StartOutputsV2{Definition: definition, Resolved: resolved, Compiled: compiled, Assembly: assembly, Binding: binding, Controls: append([]contract.ControlAdapterInstanceV2{}, controls...), Activation: activation, GenerationAssociation: generation.RefV1(), Ready: ready}, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
}

func (h *HostV2) freshNowV2(previous time.Time) (time.Time, error) {
	now := h.config.Clock()
	if now.IsZero() || (!previous.IsZero() && now.Before(previous)) {
		return time.Time{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "HostV2 clock regressed")
	}
	return now, nil
}

func timedOwnerCallV2[T any](h *HostV2, previousUnixNano int64, call func() (T, error), coordinate func(T) contract.HostOperationCoordinateV2, validate func(T, time.Time, error) error) (contract.HostOperationCoordinateV2, error) {
	before, err := h.freshNowV2(time.Unix(0, previousUnixNano))
	if err != nil {
		return contract.HostOperationCoordinateV2{}, err
	}
	value, callErr := call()
	result := coordinate(value)
	after, clockErr := h.freshNowV2(before)
	if clockErr != nil {
		return result, clockErr
	}
	return result, validate(value, after, callErr)
}

func nextStartPhaseV2(current contract.HostPhaseV2) (contract.HostPhaseV2, bool) {
	next := map[contract.HostPhaseV2]contract.HostPhaseV2{contract.HostAcceptedV2: contract.HostValidatingV2, contract.HostValidatingV2: contract.HostResolvingV2, contract.HostResolvingV2: contract.HostCompilingV2, contract.HostCompilingV2: contract.HostBindingV2, contract.HostBindingV2: contract.HostConstructingControlV2, contract.HostConstructingControlV2: contract.HostActivatingV2, contract.HostActivatingV2: contract.HostAssociatingGenerationV2, contract.HostAssociatingGenerationV2: contract.HostVerifyingV2, contract.HostVerifyingV2: contract.HostReadyV2}
	value, ok := next[current]
	return value, ok
}

func safeJournalCASV2(ctx context.Context, facts ports.JournalFactPortV2, expected contract.ExactRefV1, next contract.HostJournalV2) (result contract.HostJournalV2, err error) {
	defer func() {
		if recover() != nil {
			result = contract.HostJournalV2{}
			err = contract.NewError(contract.ErrorUnknownOutcome, "host_v2_journal_cas_panic", "HostV2 Journal CAS outcome is unknown after panic")
		}
	}()
	return facts.CompareAndSwapHostJournalV2(ctx, expected, next)
}

func safeCoordinateCallV2(ctx context.Context, call func(context.Context) (contract.HostOperationCoordinateV2, error)) (result contract.HostOperationCoordinateV2, err error) {
	defer func() {
		if recover() != nil {
			result = contract.HostOperationCoordinateV2{}
			err = contract.NewError(contract.ErrorUnknownOutcome, "host_v2_owner_call_panic", "HostV2 Owner call outcome is unknown after panic")
		}
	}()
	return call(ctx)
}

func findHostOperationV2(j contract.HostJournalV2, attemptID string) int {
	for index := range j.Operations {
		if j.Operations[index].AttemptID == attemptID {
			return index
		}
	}
	return -1
}

func publicationAttemptsCompleteV2(j contract.HostJournalV2, attemptID string) (bool, error) {
	steps := []string{"generation", "manifest", "graph", "handoff", "commit"}
	complete := 0
	commitRecorded := false
	for _, step := range steps {
		id := attemptID + "-publication-" + step
		index := findHostOperationV2(j, id)
		if index < 0 {
			continue
		}
		attempt := j.Operations[index]
		if attempt.OperationKind != "praxis.agent-host/assembly-publication-"+step || attempt.Phase != contract.HostCompilingV2 {
			return false, contract.NewError(contract.ErrorConflict, "assembly_publication_journal_splice", "Assembly publication Journal step drifted")
		}
		if attempt.State == contract.HostOperationResultRecordedV2 {
			complete++
			commitRecorded = commitRecorded || step == "commit"
		}
	}
	if commitRecorded && complete != len(steps) {
		return false, contract.NewError(contract.ErrorConflict, "assembly_publication_journal_incomplete", "Assembly publication commit exists without the complete five-step Journal chain")
	}
	return complete == len(steps), nil
}

func canonicalCoordinatesV2(values []contract.HostOperationCoordinateV2) []contract.HostOperationCoordinateV2 {
	result := append([]contract.HostOperationCoordinateV2(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		return left.ContractKind+"\x00"+left.OwnerID+"\x00"+left.ID < right.ContractKind+"\x00"+right.OwnerID+"\x00"+right.ID
	})
	return result
}

func trimDigestV2(value contract.DigestV1) string {
	text := string(value)
	if len(text) > 20 {
		return text[len(text)-20:]
	}
	return text
}

func startRequestCoordinateV2(request contract.StartRequestV2) contract.HostOperationCoordinateV2 {
	return contract.HostOperationCoordinateV2{ContractKind: contract.HostLifecycleContractVersionV2, OwnerID: "praxis.agent-host", ID: request.Config.HostID + "/" + request.StartID, Revision: 1, Digest: request.RequestDigest}
}
func definitionCoordinateV2(value contract.DecodedDefinitionV1) contract.HostOperationCoordinateV2 {
	return contract.HostV2ExactCoordinate("praxis.agent-definition/definition", "praxis.agent-definition", value.Ref)
}
func resolvedCoordinateV2(value contract.ResolvedAssemblyV1) contract.HostOperationCoordinateV2 {
	return contract.HostV2ExactCoordinate("praxis.agent-assembler/resolved-plan", "praxis.agent-assembler", value.PlanRef)
}
func compiledCoordinateV2(value contract.CompiledAssemblyArtifactsV2) contract.HostOperationCoordinateV2 {
	return contract.HostOperationCoordinateV2{ContractKind: contract.CompiledAssemblyArtifactsContractVersionV2, OwnerID: "praxis.harness", ID: value.Compiled.GenerationRef.ID, Revision: value.Compiled.GenerationRef.Revision, Digest: value.Digest}
}
func bindingCoordinateV2(value runtimeports.BindingAdmissionResultV1) contract.HostOperationCoordinateV2 {
	return contract.HostOperationCoordinateV2{ContractKind: runtimeports.BindingAdmissionContractVersionV1, OwnerID: "praxis.runtime", ID: value.BindingSet.ID, Revision: uint64(value.BindingSet.Revision), Digest: contract.DigestV1(value.BindingSet.Digest), Current: true, ExpiresUnixNano: value.ExpiresUnixNano}
}
func controlCoordinateV2(value contract.ControlAdapterInstanceV2) contract.HostOperationCoordinateV2 {
	return contract.HostOperationCoordinateV2{ContractKind: contract.ControlAdapterFactoryContractVersionV2, OwnerID: "praxis.agent-host", ID: value.InstanceRef.ID, Revision: value.InstanceRef.Revision, Digest: contract.DigestV1(value.Digest), Current: true, ExpiresUnixNano: value.ExpiresUnixNano}
}
func generationCoordinateV2(value runtimeports.GenerationBindingAssociationFactV1) contract.HostOperationCoordinateV2 {
	return contract.HostOperationCoordinateV2{ContractKind: runtimeports.GenerationBindingAssociationContractVersionV1, OwnerID: "praxis.runtime", ID: value.ID, Revision: uint64(value.Revision), Digest: contract.DigestV1(value.Digest), Current: true, ExpiresUnixNano: value.ExpiresUnixNano}
}
func readyCoordinateV2(value contract.SystemReadyGatewayResultV2) contract.HostOperationCoordinateV2 {
	return contract.HostOperationCoordinateV2{ContractKind: contract.SystemReadyGatewayContractVersionV2, OwnerID: "praxis.agent-host", ID: value.Current.ID, Revision: uint64(value.Current.Revision), Digest: contract.DigestV1(value.Current.Digest), Current: true, ExpiresUnixNano: value.Current.ExpiresUnixNano}
}

func validateDefinitionResultV2(value contract.DecodedDefinitionV1, err error) error {
	if err != nil {
		return err
	}
	return value.Validate()
}
func validateResolvedResultV2(value contract.ResolvedAssemblyV1, err error) error {
	if err != nil {
		return err
	}
	return value.Validate()
}
func validateCompiledResultV2(value contract.CompiledAssemblyArtifactsV2, now time.Time, err error) error {
	if err != nil {
		return err
	}
	return value.ValidateAt(now)
}
func validateBindingResultV2(value runtimeports.BindingAdmissionResultV1, request runtimeports.BindingAdmissionRequestV1, now time.Time, err error) error {
	if err != nil {
		return err
	}
	return value.ValidateCurrent(request, now)
}
func validateControlResultV2(value contract.ControlAdapterInstanceV2, request contract.ControlAdapterConstructRequestV2, now time.Time, err error) error {
	if err != nil {
		return err
	}
	return value.ValidateCurrent(request, now)
}
func validateActivationResultV2(value applicationcontract.AgentActivationResultV1, request applicationcontract.AgentActivationStartRequestV1, now time.Time, err error) error {
	if err != nil {
		return err
	}
	return value.ValidateFor(request, now)
}
func validateGenerationResultV2(value runtimeports.GenerationBindingAssociationFactV1, candidate runtimeports.GenerationBindingAssociationCandidateV1, now time.Time, err error) error {
	if err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Candidate.Digest != candidate.Digest || value.State != runtimeports.GenerationBindingAssociationActiveV1 || !now.Before(time.Unix(0, value.ExpiresUnixNano)) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "generation association drifted from exact candidate")
	}
	return nil
}
func validateReadyResultV2(value contract.SystemReadyGatewayResultV2, now time.Time, err error) error {
	if err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	if now.IsZero() || !now.Before(time.Unix(0, value.Current.ExpiresUnixNano)) || !now.Before(time.Unix(0, value.Fact.ExpiresUnixNano)) {
		return contract.NewError(contract.ErrorPrecondition, "system_ready_expired", "SystemReady current is expired")
	}
	return nil
}

func bindingInputsV2(request runtimeports.BindingAdmissionRequestV1) []contract.HostOperationCoordinateV2 {
	result := []contract.HostOperationCoordinateV2{contract.HostV2OwnerCurrentCoordinate(request.DefinitionCurrent), contract.HostV2OwnerCurrentCoordinate(request.PlanCurrent), contract.HostV2OwnerCurrentCoordinate(request.AssemblyCurrent), contract.HostV2OwnerCurrentCoordinate(request.CatalogCurrent), contract.HostV2OwnerCurrentCoordinate(request.ResolutionCurrent), contract.HostV2OwnerCurrentCoordinate(request.AuthorityCurrent), contract.HostV2OwnerCurrentCoordinate(request.PolicyCurrent)}
	for _, release := range request.Releases {
		result = append(result, contract.HostV2OwnerCurrentCoordinate(release.Release), contract.HostV2OwnerCurrentCoordinate(release.Certification), contract.HostV2OwnerCurrentCoordinate(release.DeploymentReadiness))
	}
	return canonicalCoordinatesV2(result)
}
func controlInputsV2(request contract.ControlAdapterConstructRequestV2) []contract.HostOperationCoordinateV2 {
	return canonicalCoordinatesV2([]contract.HostOperationCoordinateV2{contract.HostV2OwnerCurrentCoordinate(request.Descriptor.Generation), {ContractKind: contract.ControlAdapterFactoryContractVersionV2, OwnerID: "praxis.agent-host", ID: request.Descriptor.Ref.FactoryID, Revision: uint64(request.Descriptor.Ref.Revision), Digest: contract.DigestV1(request.Descriptor.Ref.Digest)}, {ContractKind: runtimeports.BindingAdmissionContractVersionV1, OwnerID: "praxis.runtime", ID: request.Descriptor.Binding.ID, Revision: uint64(request.Descriptor.Binding.Revision), Digest: contract.DigestV1(request.Descriptor.Binding.Digest), Current: true, ExpiresUnixNano: request.Descriptor.Binding.ExpiresUnixNano}})
}
func activationInputsV2(request applicationcontract.AgentActivationStartRequestV1) []contract.HostOperationCoordinateV2 {
	refs := []runtimeports.OwnerCurrentRefV1{request.DefinitionCurrent, request.PlanCurrent, request.AssemblyCurrent, request.BindingSetCurrent, request.AuthorityCurrent, request.PolicyCurrent, request.BudgetCurrent, request.CredentialCurrent, request.SandboxAdapterBinding, request.ExecutionAdapterBinding}
	result := make([]contract.HostOperationCoordinateV2, 0, len(refs))
	for _, ref := range refs {
		result = append(result, contract.HostV2OwnerCurrentCoordinate(ref))
	}
	return canonicalCoordinatesV2(result)
}
func generationInputsV2(candidate runtimeports.GenerationBindingAssociationCandidateV1) []contract.HostOperationCoordinateV2 {
	return canonicalCoordinatesV2([]contract.HostOperationCoordinateV2{{ContractKind: runtimeports.GenerationBindingAssociationContractVersionV1, OwnerID: "praxis.harness", ID: candidate.Generation.Generation.ID, Revision: uint64(candidate.Generation.Generation.Revision), Digest: contract.DigestV1(candidate.Generation.ProjectionDigest), Current: true, ExpiresUnixNano: candidate.Generation.ExpiresUnixNano}, {ContractKind: runtimeports.GenerationBindingAssociationContractVersionV1, OwnerID: "praxis.runtime", ID: candidate.Binding.BindingSetID, Revision: uint64(candidate.Binding.BindingSetRevision), Digest: contract.DigestV1(candidate.Binding.ProjectionDigest), Current: true, ExpiresUnixNano: candidate.Binding.ExpiresUnixNano}, {ContractKind: runtimeports.GenerationBindingAssociationContractVersionV1, OwnerID: "praxis.runtime", ID: candidate.Activation.Operation.CurrentProjectionRef, Revision: uint64(candidate.Activation.Watermark), Digest: contract.DigestV1(candidate.Activation.ProjectionDigest), Current: true, ExpiresUnixNano: candidate.Activation.ExpiresUnixNano}})
}
func readyInputsV2(request contract.SystemReadyEnsureRequestV2) []contract.HostOperationCoordinateV2 {
	refs := []runtimeports.OwnerCurrentRefV1{request.Definition, request.Plan, request.Assembly, request.BindingSet, request.Activation, request.GenerationBinding, request.ApplicationStart, request.SandboxLease, request.SandboxActive, request.ExecutionReady, request.SupervisionPolicy}
	result := make([]contract.HostOperationCoordinateV2, 0, len(refs)+1)
	for _, ref := range refs {
		result = append(result, contract.HostV2OwnerCurrentCoordinate(ref))
	}
	result = append(result, contract.HostOperationCoordinateV2{ContractKind: contract.HostStartClaimContractVersionV1, OwnerID: "praxis.agent-host", ID: request.Claim.HostID + "/" + request.Claim.StartID, Revision: request.Claim.Revision, Digest: request.Claim.Digest, Current: true, ExpiresUnixNano: request.Claim.ExpiresUnixNano})
	return canonicalCoordinatesV2(result)
}

func bindingContainsControlV2(binding runtimeports.BindingAdmissionResultV1, request contract.ControlAdapterConstructRequestV2) bool {
	for _, ref := range binding.Bindings {
		if ref == request.Descriptor.Binding {
			return binding.ResourceBindingSet == request.Descriptor.ResourceBindingSet
		}
	}
	return false
}

func validateControlRequestSetV2(requests []contract.ControlAdapterConstructRequestV2, binding runtimeports.BindingAdmissionResultV1, assembly contract.AssemblyPublicationResultV2) ([]contract.ControlAdapterConstructRequestV2, error) {
	if len(requests) == 0 || len(requests) != len(binding.Bindings) {
		return nil, contract.NewError(contract.ErrorPrecondition, "control_adapter_set_incomplete", "Control adapter requests must cover every admitted Binding exactly once")
	}
	result := append([]contract.ControlAdapterConstructRequestV2{}, requests...)
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].Descriptor, result[j].Descriptor
		if left.ComponentID != right.ComponentID {
			return left.ComponentID < right.ComponentID
		}
		return left.Ref.FactoryID < right.Ref.FactoryID
	})
	seenBindings := make(map[string]struct{}, len(result))
	seenFactories := make(map[string]struct{}, len(result))
	for index, request := range result {
		if err := request.Validate(); err != nil {
			return nil, err
		}
		if !bindingContainsControlV2(binding, request) || request.Descriptor.Generation != assembly.OwnerCurrent {
			return nil, contract.NewError(contract.ErrorConflict, "control_adapter_binding_splice", "Control adapter request drifted from exact Binding or Generation")
		}
		bindingKey := string(request.Descriptor.Binding.ComponentID) + "\x00" + request.Descriptor.Binding.ID
		if _, exists := seenBindings[bindingKey]; exists {
			return nil, contract.NewError(contract.ErrorConflict, "control_adapter_binding_alias", "Multiple control factories alias one Runtime Binding")
		}
		if _, exists := seenFactories[request.Descriptor.Ref.FactoryID]; exists {
			return nil, contract.NewError(contract.ErrorConflict, "control_adapter_factory_alias", "Control adapter factory is duplicated")
		}
		seenBindings[bindingKey] = struct{}{}
		seenFactories[request.Descriptor.Ref.FactoryID] = struct{}{}
		if index > 0 && result[index-1].Descriptor.ComponentID == request.Descriptor.ComponentID && result[index-1].Descriptor.Ref.FactoryID >= request.Descriptor.Ref.FactoryID {
			return nil, contract.NewError(contract.ErrorConflict, "control_adapter_set_not_canonical", "Control adapter requests are not canonical")
		}
	}
	return result, nil
}

func validateReadyControlSetV2(request contract.SystemReadyEnsureRequestV2, controlRequests []contract.ControlAdapterConstructRequestV2, controls []contract.ControlAdapterInstanceV2) error {
	if len(request.Components) != len(controlRequests) || len(controls) != len(controlRequests) {
		return contract.NewError(contract.ErrorPrecondition, "system_ready_component_set_incomplete", "SystemReady must cover the complete constructed control set")
	}
	byBinding := make(map[string]contract.ControlAdapterInstanceV2, len(controls))
	for index, controlRequest := range controlRequests {
		byBinding[string(controlRequest.Descriptor.Binding.ComponentID)+"\x00"+controlRequest.Descriptor.Binding.ID] = controls[index]
	}
	for _, component := range request.Components {
		key := string(component.Binding.ComponentID) + "\x00" + component.Binding.ID
		control, exists := byBinding[key]
		if !exists || component.ConstructedComponent != control.InstanceRef {
			return contract.NewError(contract.ErrorConflict, "system_ready_constructed_component_splice", "SystemReady component drifted from the exact control adapter result")
		}
		delete(byBinding, key)
	}
	if len(byBinding) != 0 {
		return contract.NewError(contract.ErrorPrecondition, "system_ready_component_set_incomplete", "SystemReady omitted a constructed control adapter")
	}
	return nil
}
func sameBindingOwnerCurrentV2(ref runtimeports.OwnerCurrentRefV1, binding runtimeports.BindingAdmissionBindingSetRefV1) bool {
	return ref.ID == binding.ID && ref.Revision == binding.Revision && ref.Digest == binding.Digest && ref.ExpiresUnixNano == binding.ExpiresUnixNano
}
func firstErrorV2(primary, fallback error) error {
	if primary != nil {
		return primary
	}
	return fallback
}
