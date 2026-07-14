// Package runtimeadapter exposes the Harness kernel through Runtime's existing
// ExecutionPort without transferring Runtime fact ownership into Harness.
package runtimeadapter

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	CommandStartRun            = "start_run"
	CommandProvideActionResult = "provide_action_result"
	CommandProvideInput        = "provide_input"
	CommandCancel              = "cancel"

	InspectReady  = "ready"
	InspectState  = "state"
	InspectEvents = "events"
	// InspectCleanup is the only inspection allowed after an endpoint closes.
	// It is a recovery observation for a lost Close reply, not authoritative
	// Runtime cleanup or termination truth.
	InspectCleanup = "cleanup"
)

type StartRunCommand struct {
	RunID  core.AgentRunID            `json:"run_id"`
	Input  runtimeports.OpaquePayload `json:"input"`
	Intent core.EffectIntent          `json:"intent"`
}

type ProvideActionResultCommand struct {
	RunID  core.AgentRunID       `json:"run_id"`
	Result contract.ActionResult `json:"result"`
	Intent core.EffectIntent     `json:"intent"`
}

type ProvideInputCommand struct {
	RunID  core.AgentRunID            `json:"run_id"`
	Input  runtimeports.OpaquePayload `json:"input"`
	Intent core.EffectIntent          `json:"intent"`
}

type CancelCommand struct {
	RunID  core.AgentRunID   `json:"run_id"`
	Intent core.EffectIntent `json:"intent"`
}

type Config struct {
	Manifest contract.Manifest
	// AdapterBindingManifest is the explicit Runtime Binding V2 identity of this
	// adapter. It is intentionally not derived from the legacy Harness
	// manifest: locality, owners and residual policy are governance decisions,
	// not values an adapter may grant to itself.
	AdapterBindingManifest *runtimeports.ComponentManifestV2
	Loop                   *kernel.Loop
	Clock                  func() time.Time
}

type Adapter struct {
	mu        sync.Mutex
	manifest  contract.Manifest
	binding   *runtimeports.ComponentManifestV2
	loop      *kernel.Loop
	clock     func() time.Time
	endpoints map[string]endpoint
}

type endpoint struct {
	ref               runtimeports.ExecutionEndpointRef
	scope             core.ExecutionScope
	requirementDigest core.Digest
	lastRun           core.AgentRunID
	closed            bool
	closeReason       string
}

var _ runtimeports.ExecutionPort = (*Adapter)(nil)
var _ runtimeports.DescriberV2 = (*Adapter)(nil)

func New(config Config) (*Adapter, error) {
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.Loop == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "harness loop is required")
	}
	if err := config.Manifest.Validate(config.Clock()); err != nil {
		return nil, err
	}
	var binding *runtimeports.ComponentManifestV2
	if config.AdapterBindingManifest != nil {
		cloned, err := cloneBindingManifest(*config.AdapterBindingManifest)
		if err != nil {
			return nil, err
		}
		if string(cloned.ComponentID) != config.Manifest.ID || cloned.SemanticVersion != config.Manifest.Version || cloned.ArtifactDigest != config.Manifest.ArtifactDigest || cloned.Conformance != config.Manifest.Conformance {
			return nil, core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "harness legacy and binding v2 identities differ")
		}
		binding = &cloned
	}
	loopManifest := config.Loop.Manifest()
	if loopManifest.ID != config.Manifest.ID || loopManifest.ArtifactDigest != config.Manifest.ArtifactDigest || loopManifest.Bootstrap.ResolvedPlanDigest != config.Manifest.Bootstrap.ResolvedPlanDigest {
		return nil, core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "harness adapter and loop manifests differ")
	}
	return &Adapter{manifest: contract.CloneManifest(config.Manifest), binding: binding, loop: config.Loop, clock: config.Clock, endpoints: make(map[string]endpoint)}, nil
}

// DescribeV2 exposes only an explicitly supplied Binding identity. Legacy
// ExecutionPort compatibility remains usable without it, but such an adapter
// cannot be registered or bound through Runtime Binding V2.
func (a *Adapter) DescribeV2(context.Context) (runtimeports.ComponentManifestV2, error) {
	if a.binding == nil {
		return runtimeports.ComponentManifestV2{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "harness binding v2 manifest was not supplied")
	}
	if err := a.manifest.Validate(a.now()); err != nil {
		return runtimeports.ComponentManifestV2{}, err
	}
	return cloneBindingManifest(*a.binding)
}

func (a *Adapter) Describe(context.Context) (runtimeports.ComponentDescriptor, error) {
	if err := a.manifest.Validate(a.now()); err != nil {
		return runtimeports.ComponentDescriptor{}, err
	}
	descriptor := a.descriptor()
	if err := descriptor.Validate(); err != nil {
		return runtimeports.ComponentDescriptor{}, err
	}
	return descriptor, nil
}

func (a *Adapter) Preflight(_ context.Context, request runtimeports.ExecutionPreflightRequest) (runtimeports.ExecutionPreflightReport, error) {
	if err := request.ProposedScope.Validate(); err != nil {
		return runtimeports.ExecutionPreflightReport{}, err
	}
	if err := request.RequirementDigest.Validate(); err != nil {
		return runtimeports.ExecutionPreflightReport{}, err
	}
	if request.ProposedScope.Lineage.PlanDigest != a.manifest.Bootstrap.ResolvedPlanDigest {
		return runtimeports.ExecutionPreflightReport{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "execution scope is not bound to the harness bootstrap plan")
	}
	if request.ProbeBudget.MaxRequests == 0 || request.ProbeBudget.MaxDuration <= 0 || request.ProbeBudget.PossibleCharge || request.ProbeBudget.PossibleMutation {
		return runtimeports.ExecutionPreflightReport{}, core.NewError(core.ErrorForbidden, core.ReasonPlanInvalid, "minimal harness preflight must be bounded, charge-free and mutation-free")
	}
	if err := a.manifest.Validate(a.now()); err != nil {
		return runtimeports.ExecutionPreflightReport{}, err
	}
	descriptor := a.descriptor()
	evidence, err := core.DigestJSON(struct {
		Manifest    core.Digest         `json:"manifest"`
		Requirement core.Digest         `json:"requirement"`
		Scope       core.ExecutionScope `json:"scope"`
	}{a.manifest.EvidenceDigest, request.RequirementDigest, request.ProposedScope})
	if err != nil {
		return runtimeports.ExecutionPreflightReport{}, err
	}
	return runtimeports.ExecutionPreflightReport{
		Accepted: true, Descriptor: descriptor, RequirementDigest: request.RequirementDigest,
		EvidenceDigest: evidence, EvidenceExpiry: minTime(a.manifest.EvidenceExpiresAt, a.manifest.Bootstrap.EvidenceExpiresAt),
		PossibleResidual: len(a.manifest.Bootstrap.AllowedResiduals) > 0,
		ResidualRef:      strings.Join(a.manifest.Bootstrap.AllowedResiduals, ","),
	}, nil
}

func (a *Adapter) Open(_ context.Context, request runtimeports.ExecutionOpenRequest) (runtimeports.ExecutionEndpointRef, error) {
	if a.binding != nil {
		return runtimeports.ExecutionEndpointRef{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonDispatchPermitInvalid, "binding v2 harness requires the governed execution bridge")
	}
	if err := request.Scope.Validate(); err != nil {
		return runtimeports.ExecutionEndpointRef{}, err
	}
	if request.Scope.Lineage.PlanDigest != a.manifest.Bootstrap.ResolvedPlanDigest {
		return runtimeports.ExecutionEndpointRef{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "execution scope is not bound to the harness bootstrap plan")
	}
	if err := request.RequirementDigest.Validate(); err != nil {
		return runtimeports.ExecutionEndpointRef{}, err
	}
	if err := validateDispatch(request.Scope, request.Intent, request.Fence, a.now()); err != nil {
		return runtimeports.ExecutionEndpointRef{}, err
	}
	endpointID, err := executionEndpointID(a.manifest.ID, request.Scope)
	if err != nil {
		return runtimeports.ExecutionEndpointRef{}, err
	}
	digest, err := core.DigestJSON(struct {
		ID          string              `json:"id"`
		Scope       core.ExecutionScope `json:"scope"`
		Requirement core.Digest         `json:"requirement"`
	}{endpointID, request.Scope, request.RequirementDigest})
	if err != nil {
		return runtimeports.ExecutionEndpointRef{}, err
	}
	ref := runtimeports.ExecutionEndpointRef{ComponentID: a.manifest.ID, EndpointID: endpointID, Digest: digest}
	a.mu.Lock()
	defer a.mu.Unlock()
	if existing, exists := a.endpoints[endpointID]; exists {
		if existing.ref.Digest != ref.Digest || existing.closed {
			return runtimeports.ExecutionEndpointRef{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "harness endpoint identity conflicts with an existing endpoint")
		}
		return existing.ref, nil
	}
	a.endpoints[endpointID] = endpoint{ref: ref, scope: request.Scope, requirementDigest: request.RequirementDigest}
	return ref, nil
}

func executionEndpointID(componentID string, scope core.ExecutionScope) (string, error) {
	digest, err := core.DigestJSON(struct {
		Domain      string              `json:"domain"`
		ComponentID string              `json:"component_id"`
		Scope       core.ExecutionScope `json:"scope"`
	}{Domain: "praxis.harness.execution-endpoint/v1", ComponentID: componentID, Scope: scope})
	if err != nil {
		return "", err
	}
	return "harness-endpoint:" + string(digest), nil
}

func (a *Adapter) Inspect(_ context.Context, request runtimeports.ExecutionInspectRequest) (runtimeports.ExecutionObservation, error) {
	current, err := a.lookupEndpoint(request.Scope, request.Endpoint, request.InspectKind == InspectCleanup)
	if err != nil {
		return runtimeports.ExecutionObservation{}, err
	}
	switch request.InspectKind {
	case InspectCleanup:
		state := "open"
		if current.closed {
			state = "closed"
		}
		return a.observation(request.Scope, "cleanup:"+state, map[string]string{"state": state, "reason": current.closeReason})
	case InspectReady:
		if err := a.manifest.Validate(a.now()); err != nil {
			return runtimeports.ExecutionObservation{}, err
		}
		return a.observation(request.Scope, "ready:ready", map[string]string{"state": "ready"})
	case InspectState:
		runID := a.loop.ActiveRun(request.Scope)
		if runID == "" {
			runID = current.lastRun
		}
		if runID == "" {
			return a.observation(request.Scope, "state:idle", map[string]string{"state": "idle"})
		}
		snapshot, err := a.loop.Inspect(contract.RunRef{Scope: request.Scope, RunID: runID})
		if err != nil {
			return runtimeports.ExecutionObservation{}, err
		}
		return a.observation(request.Scope, "state:"+string(snapshot.State.Phase), snapshot)
	case InspectEvents:
		runID := a.loop.ActiveRun(request.Scope)
		if runID == "" {
			runID = current.lastRun
		}
		if runID == "" {
			return a.observation(request.Scope, "events:idle", []contract.Event{})
		}
		events, err := a.loop.Events(contract.RunRef{Scope: request.Scope, RunID: runID})
		if err != nil {
			return runtimeports.ExecutionObservation{}, err
		}
		return a.observation(request.Scope, "events:active", events)
	default:
		return runtimeports.ExecutionObservation{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "unknown harness inspect kind")
	}
}

func (a *Adapter) Control(ctx context.Context, request runtimeports.ExecutionControlRequest) (runtimeports.ExecutionObservation, error) {
	if a.binding != nil {
		return runtimeports.ExecutionObservation{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonDispatchPermitInvalid, "binding v2 harness requires the governed execution bridge")
	}
	if _, err := a.endpoint(request.Scope, request.Endpoint); err != nil {
		return runtimeports.ExecutionObservation{}, err
	}
	if (request.CommandKind == CommandCancel && !a.manifest.Bootstrap.Controls.Cancel) ||
		(request.CommandKind == CommandProvideInput && !a.manifest.Bootstrap.Controls.ProvideInput) ||
		(request.CommandKind == CommandProvideActionResult && !a.manifest.Bootstrap.Controls.ProvideActionResult) {
		return runtimeports.ExecutionObservation{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMismatch, "harness control is not enabled by the bootstrap plan")
	}
	var snapshot contract.Snapshot
	var err error
	var runID core.AgentRunID
	switch request.CommandKind {
	case CommandStartRun:
		var command StartRunCommand
		if err = DecodeControlPayload(request.Payload, CommandStartRun, &command); err == nil {
			runID = command.RunID
			if request.Fence == nil {
				err = core.NewError(core.ErrorForbidden, core.ReasonEffectFenceStale, "start run requires a current fence")
			} else {
				snapshot, err = a.loop.Start(ctx, kernel.StartRequest{Run: contract.RunRef{Scope: request.Scope, RunID: command.RunID}, Input: command.Input, Intent: command.Intent, Fence: *request.Fence})
			}
		}
	case CommandProvideActionResult:
		var command ProvideActionResultCommand
		if err = DecodeControlPayload(request.Payload, CommandProvideActionResult, &command); err == nil {
			runID = command.RunID
			if request.Fence == nil {
				err = core.NewError(core.ErrorForbidden, core.ReasonEffectFenceStale, "action continuation requires a current fence")
			} else {
				snapshot, err = a.loop.ProvideActionResult(ctx, kernel.ProvideActionResultRequest{Run: contract.RunRef{Scope: request.Scope, RunID: command.RunID}, Result: command.Result, Intent: command.Intent, Fence: *request.Fence})
			}
		}
	case CommandProvideInput:
		var command ProvideInputCommand
		if err = DecodeControlPayload(request.Payload, CommandProvideInput, &command); err == nil {
			runID = command.RunID
			if request.Fence == nil {
				err = core.NewError(core.ErrorForbidden, core.ReasonEffectFenceStale, "input continuation requires a current fence")
			} else {
				snapshot, err = a.loop.ProvideInput(ctx, kernel.ProvideInputRequest{Run: contract.RunRef{Scope: request.Scope, RunID: command.RunID}, Input: command.Input, Intent: command.Intent, Fence: *request.Fence})
			}
		}
	case CommandCancel:
		var command CancelCommand
		if err = DecodeControlPayload(request.Payload, CommandCancel, &command); err == nil {
			runID = command.RunID
			if request.Fence == nil {
				err = core.NewError(core.ErrorForbidden, core.ReasonEffectFenceStale, "cancel requires a current fence")
			} else {
				snapshot, err = a.loop.Cancel(ctx, kernel.CancelRequest{Run: contract.RunRef{Scope: request.Scope, RunID: command.RunID}, Intent: command.Intent, Fence: *request.Fence})
			}
		}
	default:
		return runtimeports.ExecutionObservation{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "unknown harness control command")
	}
	if err != nil {
		return runtimeports.ExecutionObservation{}, err
	}
	a.mu.Lock()
	current := a.endpoints[request.Endpoint.EndpointID]
	current.lastRun = runID
	a.endpoints[request.Endpoint.EndpointID] = current
	a.mu.Unlock()
	return a.observation(request.Scope, "control:"+request.CommandKind+":"+string(snapshot.State.Phase), snapshot)
}

func (a *Adapter) Close(ctx context.Context, request runtimeports.ExecutionCloseRequest) (runtimeports.ExecutionObservation, error) {
	if a.binding != nil {
		return runtimeports.ExecutionObservation{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonDispatchPermitInvalid, "binding v2 harness requires the governed execution bridge")
	}
	current, err := a.endpoint(request.Scope, request.Endpoint)
	if err != nil {
		return runtimeports.ExecutionObservation{}, err
	}
	if strings.TrimSpace(request.Reason) == "" {
		return runtimeports.ExecutionObservation{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "harness close reason is required")
	}
	if err := validateDispatch(request.Scope, request.Intent, request.Fence, a.now()); err != nil {
		return runtimeports.ExecutionObservation{}, err
	}
	if runID := a.loop.ActiveRun(request.Scope); runID != "" {
		if _, err := a.loop.Cancel(ctx, kernel.CancelRequest{Run: contract.RunRef{Scope: request.Scope, RunID: runID}, Intent: request.Intent, Fence: request.Fence}); err != nil {
			return runtimeports.ExecutionObservation{}, err
		}
	}
	a.mu.Lock()
	current.closed = true
	current.closeReason = request.Reason
	a.endpoints[request.Endpoint.EndpointID] = current
	a.mu.Unlock()
	return a.observation(request.Scope, "closed", map[string]string{"reason": request.Reason})
}

func EncodeControlPayload(schema string, value any) (runtimeports.OpaquePayload, error) {
	if strings.TrimSpace(schema) == "" {
		return runtimeports.OpaquePayload{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "control payload schema is required")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return runtimeports.OpaquePayload{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "control payload cannot be serialized")
	}
	digest, err := core.DigestJSON(value)
	if err != nil {
		return runtimeports.OpaquePayload{}, err
	}
	return runtimeports.OpaquePayload{Schema: schema, Digest: digest, Payload: payload}, nil
}

func DecodeControlPayload(payload runtimeports.OpaquePayload, schema string, target any) error {
	if err := contract.ValidateOpaque(payload); err != nil {
		return err
	}
	if payload.Schema != schema {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "control payload schema does not match command")
	}
	if err := json.Unmarshal(payload.Payload, target); err != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "control payload cannot be decoded")
	}
	digest, err := core.DigestJSON(target)
	if err != nil {
		return err
	}
	if digest != payload.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "control payload digest does not match decoded command")
	}
	return nil
}

func (a *Adapter) descriptor() runtimeports.ComponentDescriptor {
	names := []string{"preflight", "open", "inspect", "control", "close"}
	if a.manifest.Bootstrap.Controls.Cancel {
		names = append(names, CommandCancel)
	}
	if a.manifest.Bootstrap.Controls.ProvideInput {
		names = append(names, CommandProvideInput)
	}
	if a.manifest.Bootstrap.Controls.ProvideActionResult {
		names = append(names, CommandProvideActionResult)
	}
	expiry := minTime(a.manifest.EvidenceExpiresAt, a.manifest.Bootstrap.EvidenceExpiresAt)
	capabilities := make([]runtimeports.Capability, 0, len(names))
	for _, name := range names {
		capabilities = append(capabilities, runtimeports.Capability{Name: name, State: runtimeports.CapabilityBound, EvidenceDigest: a.manifest.EvidenceDigest, EvidenceExpiry: expiry})
	}
	return runtimeports.ComponentDescriptor{
		ID: a.manifest.ID, Kind: runtimeports.ComponentHarness, Version: a.manifest.Version,
		ArtifactDigest: a.manifest.ArtifactDigest, ContractVersion: runtimeports.ContractVersion,
		Conformance: a.manifest.Conformance, Capabilities: capabilities,
	}
}

func (a *Adapter) endpoint(scope core.ExecutionScope, ref runtimeports.ExecutionEndpointRef) (endpoint, error) {
	return a.lookupEndpoint(scope, ref, false)
}

func (a *Adapter) lookupEndpoint(scope core.ExecutionScope, ref runtimeports.ExecutionEndpointRef, allowClosed bool) (endpoint, error) {
	if err := scope.Validate(); err != nil {
		return endpoint{}, err
	}
	if ref.ComponentID != a.manifest.ID || strings.TrimSpace(ref.EndpointID) == "" {
		return endpoint{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "harness endpoint reference is invalid")
	}
	if err := ref.Digest.Validate(); err != nil {
		return endpoint{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	current, exists := a.endpoints[ref.EndpointID]
	if !exists {
		return endpoint{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "harness endpoint does not exist")
	}
	if (!allowClosed && current.closed) || current.ref != ref || !sameScope(current.scope, scope) {
		return endpoint{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonFencedInstance, "harness endpoint is closed, stale or belongs to another scope")
	}
	return current, nil
}

func (a *Adapter) observation(scope core.ExecutionScope, kind string, value any) (runtimeports.ExecutionObservation, error) {
	payload, err := EncodeControlPayload("praxis.harness.observation/v1alpha1", value)
	if err != nil {
		return runtimeports.ExecutionObservation{}, err
	}
	return runtimeports.ExecutionObservation{SourceComponentID: a.manifest.ID, SourceEpoch: scope.Instance.Epoch, ObservationKind: kind, Payload: payload, ObservedAt: a.now()}, nil
}

func (a *Adapter) now() time.Time { return a.clock().UTC() }

func validateDispatch(scope core.ExecutionScope, intent core.EffectIntent, fence core.ExecutionFence, now time.Time) error {
	return core.ValidateEffectDispatch(intent, fence, core.CurrentFenceFacts{Scope: scope, CapabilityGrantDigest: fence.CapabilityGrantDigest}, now)
}

func sameScope(left, right core.ExecutionScope) bool {
	if left.Identity != right.Identity || left.Lineage != right.Lineage || left.Instance != right.Instance || left.AuthorityEpoch != right.AuthorityEpoch {
		return false
	}
	if left.SandboxLease == nil || right.SandboxLease == nil {
		return left.SandboxLease == nil && right.SandboxLease == nil
	}
	return *left.SandboxLease == *right.SandboxLease
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func cloneBindingManifest(manifest runtimeports.ComponentManifestV2) (runtimeports.ComponentManifestV2, error) {
	payload, err := runtimeports.EncodeComponentManifestV2(manifest)
	if err != nil {
		return runtimeports.ComponentManifestV2{}, err
	}
	return runtimeports.DecodeComponentManifestV2(payload)
}
