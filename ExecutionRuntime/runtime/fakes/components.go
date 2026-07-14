package fakes

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type FakeExecution struct {
	mu              sync.Mutex
	clock           func() time.Time
	descriptor      ports.ComponentDescriptor
	endpoints       map[string]string
	inspectOverride string
}

func NewFakeExecution(id string, conformance ports.ConformanceLevel, clock func() time.Time) (*FakeExecution, error) {
	if clock == nil {
		clock = time.Now
	}
	descriptor, err := fakeDescriptor(id, ports.ComponentHarness, conformance, []string{"preflight", "run", "inspect", "close"}, clock())
	if err != nil {
		return nil, err
	}
	return &FakeExecution{clock: clock, descriptor: descriptor, endpoints: make(map[string]string)}, nil
}

func (f *FakeExecution) Describe(context.Context) (ports.ComponentDescriptor, error) {
	return f.descriptor, nil
}

func (f *FakeExecution) SetInspectState(state string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspectOverride = state
}

func (f *FakeExecution) Preflight(_ context.Context, request ports.ExecutionPreflightRequest) (ports.ExecutionPreflightReport, error) {
	if err := request.ProposedScope.Validate(); err != nil {
		return ports.ExecutionPreflightReport{}, err
	}
	if request.ProposedScope.SandboxLease != nil || request.ProbeBudget.MaxRequests == 0 || request.ProbeBudget.MaxDuration <= 0 || request.ProbeBudget.PossibleCharge || request.ProbeBudget.PossibleMutation {
		return ports.ExecutionPreflightReport{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "foundation fake preflight must be bounded, read-only and lease-free")
	}
	if err := request.RequirementDigest.Validate(); err != nil {
		return ports.ExecutionPreflightReport{}, err
	}
	evidence, _ := core.DigestJSON(map[string]any{"component": f.descriptor.ID, "preflight": true})
	return ports.ExecutionPreflightReport{Accepted: true, Descriptor: f.descriptor, RequirementDigest: request.RequirementDigest, EvidenceDigest: evidence, EvidenceExpiry: f.clock().Add(time.Hour)}, nil
}

func (f *FakeExecution) Open(_ context.Context, request ports.ExecutionOpenRequest) (ports.ExecutionEndpointRef, error) {
	if request.Scope.SandboxLease == nil {
		return ports.ExecutionEndpointRef{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "execution open requires a bound sandbox lease")
	}
	if err := core.ValidateEffectDispatch(request.Intent, request.Fence, core.CurrentFenceFacts{Scope: request.Scope, CapabilityGrantDigest: request.Fence.CapabilityGrantDigest}, f.clock()); err != nil {
		return ports.ExecutionEndpointRef{}, err
	}
	digest, _ := core.DigestJSON(map[string]any{"component": f.descriptor.ID, "instance": request.Scope.Instance})
	endpoint := ports.ExecutionEndpointRef{ComponentID: f.descriptor.ID, EndpointID: fmt.Sprintf("%s/%s", f.descriptor.ID, request.Scope.Instance.ID), Digest: digest}
	state := "ready"
	if f.descriptor.Conformance == ports.ConformanceRestrictedControlled {
		// The restricted adapter cannot observe readiness at Open time. It
		// converges only when Runtime performs the independent ready Inspect.
		state = "starting"
	}
	f.mu.Lock()
	f.endpoints[endpoint.EndpointID] = state
	f.mu.Unlock()
	return endpoint, nil
}

func (f *FakeExecution) Inspect(_ context.Context, request ports.ExecutionInspectRequest) (ports.ExecutionObservation, error) {
	if err := request.Scope.Validate(); err != nil {
		return ports.ExecutionObservation{}, err
	}
	f.mu.Lock()
	state, exists := f.endpoints[request.Endpoint.EndpointID]
	if exists && request.InspectKind == "ready" && state == "starting" {
		state = "ready"
		f.endpoints[request.Endpoint.EndpointID] = state
	}
	if f.inspectOverride != "" {
		state = f.inspectOverride
	}
	f.mu.Unlock()
	if !exists {
		return ports.ExecutionObservation{}, core.NewError(core.ErrorNotFound, core.ReasonReadyEvidenceIncomplete, "execution endpoint is absent")
	}
	return executionObservation(f.descriptor.ID, request.Scope, request.InspectKind+":"+state, f.clock())
}

func (f *FakeExecution) Control(_ context.Context, request ports.ExecutionControlRequest) (ports.ExecutionObservation, error) {
	if err := request.Scope.Validate(); err != nil {
		return ports.ExecutionObservation{}, err
	}
	if strings.TrimSpace(request.CommandKind) == "" {
		return ports.ExecutionObservation{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "execution command kind is required")
	}
	return executionObservation(f.descriptor.ID, request.Scope, "control:"+request.CommandKind, f.clock())
}

func (f *FakeExecution) Close(_ context.Context, request ports.ExecutionCloseRequest) (ports.ExecutionObservation, error) {
	if err := core.ValidateEffectDispatch(request.Intent, request.Fence, core.CurrentFenceFacts{Scope: request.Scope, CapabilityGrantDigest: request.Fence.CapabilityGrantDigest}, f.clock()); err != nil {
		return ports.ExecutionObservation{}, err
	}
	f.mu.Lock()
	if _, exists := f.endpoints[request.Endpoint.EndpointID]; !exists {
		f.mu.Unlock()
		return ports.ExecutionObservation{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "execution endpoint is absent")
	}
	f.endpoints[request.Endpoint.EndpointID] = "closed"
	f.mu.Unlock()
	return executionObservation(f.descriptor.ID, request.Scope, "closed", f.clock())
}

func executionObservation(componentID string, scope core.ExecutionScope, kind string, now time.Time) (ports.ExecutionObservation, error) {
	digest, err := core.DigestJSON(map[string]any{"kind": kind, "instance": scope.Instance})
	if err != nil {
		return ports.ExecutionObservation{}, err
	}
	return ports.ExecutionObservation{SourceComponentID: componentID, SourceEpoch: scope.Instance.Epoch, ObservationKind: kind, Payload: ports.OpaquePayload{Schema: "praxis.fake.execution/v1", Digest: digest}, ObservedAt: now}, nil
}

type fakeSandbox struct {
	lease core.SandboxLeaseRef
	state string
	scope core.ExecutionScope
}

type FakeEnvironment struct {
	mu            sync.Mutex
	clock         func() time.Time
	descriptor    ports.ComponentDescriptor
	leases        map[core.SandboxLeaseID]*fakeSandbox
	faults        FakeEnvironmentFaults
	allocateCalls uint64
	activateCalls uint64
	releaseCalls  uint64
}

type FakeEnvironmentFaults struct {
	LoseAllocateReply bool
	LoseActivateReply bool
	LoseReleaseReply  bool
}

func NewFakeEnvironment(id string, clock func() time.Time) (*FakeEnvironment, error) {
	if clock == nil {
		clock = time.Now
	}
	descriptor, err := fakeDescriptor(id, ports.ComponentSandbox, ports.ConformanceFullyControlled, []string{"allocate", "activate", "inspect", "fence", "release"}, clock())
	if err != nil {
		return nil, err
	}
	return &FakeEnvironment{clock: clock, descriptor: descriptor, leases: make(map[core.SandboxLeaseID]*fakeSandbox)}, nil
}

func (f *FakeEnvironment) Describe(context.Context) (ports.ComponentDescriptor, error) {
	return f.descriptor, nil
}

func (f *FakeEnvironment) SetFaults(faults FakeEnvironmentFaults) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults = faults
}

func (f *FakeEnvironment) OperationCounts() (allocate, activate, release uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.allocateCalls, f.activateCalls, f.releaseCalls
}

func (f *FakeEnvironment) Allocate(_ context.Context, request ports.SandboxAllocateRequest) (ports.SandboxLeaseObservation, error) {
	if err := request.ProposedInstance.Validate(); err != nil {
		return ports.SandboxLeaseObservation{}, err
	}
	if request.Fence.Scope.Instance != request.ProposedInstance || request.Fence.BoundaryScope != core.FenceBoundaryActivation {
		return ports.SandboxLeaseObservation{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "sandbox allocation requires an activation fence for the proposed instance")
	}
	if err := core.ValidateEffectDispatch(request.Intent, request.Fence, core.CurrentFenceFacts{Scope: request.Fence.Scope, CapabilityGrantDigest: request.Fence.CapabilityGrantDigest}, f.clock()); err != nil {
		return ports.SandboxLeaseObservation{}, err
	}
	lease := core.SandboxLeaseRef{ID: core.SandboxLeaseID(fmt.Sprintf("%s/%s", f.descriptor.ID, request.ProposedInstance.ID)), Epoch: 1}
	f.mu.Lock()
	f.allocateCalls++
	if _, exists := f.leases[lease.ID]; exists {
		f.mu.Unlock()
		return ports.SandboxLeaseObservation{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "sandbox lease already exists")
	}
	f.leases[lease.ID] = &fakeSandbox{lease: lease, state: "reserved_quarantined", scope: request.Fence.Scope}
	loseReply := f.faults.LoseAllocateReply
	f.mu.Unlock()
	if loseReply {
		return ports.SandboxLeaseObservation{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost sandbox allocation reply")
	}
	return f.sandboxObservation(lease, "reserved_quarantined")
}

func (f *FakeEnvironment) Activate(_ context.Context, request ports.SandboxActivateRequest) (ports.SandboxLeaseObservation, error) {
	if request.Scope.SandboxLease == nil {
		return ports.SandboxLeaseObservation{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "sandbox activation requires a bound lease")
	}
	if err := core.ValidateEffectDispatch(request.Intent, request.Fence, core.CurrentFenceFacts{Scope: request.Scope, CapabilityGrantDigest: request.Fence.CapabilityGrantDigest}, f.clock()); err != nil {
		return ports.SandboxLeaseObservation{}, err
	}
	f.mu.Lock()
	f.activateCalls++
	entry, exists := f.leases[request.Scope.SandboxLease.ID]
	if !exists || entry.lease != *request.Scope.SandboxLease || entry.state != "reserved_quarantined" {
		f.mu.Unlock()
		return ports.SandboxLeaseObservation{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "sandbox is not reserved and quarantined")
	}
	entry.state = "active"
	entry.scope = request.Scope
	loseReply := f.faults.LoseActivateReply
	f.mu.Unlock()
	if loseReply {
		return ports.SandboxLeaseObservation{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost sandbox activation reply")
	}
	return f.sandboxObservation(entry.lease, "active")
}

func (f *FakeEnvironment) Inspect(_ context.Context, lease core.SandboxLeaseRef) (ports.SandboxLeaseObservation, error) {
	f.mu.Lock()
	entry, exists := f.leases[lease.ID]
	f.mu.Unlock()
	if !exists || entry.lease != lease {
		return ports.SandboxLeaseObservation{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "sandbox lease is absent")
	}
	return f.sandboxObservation(lease, entry.state)
}

func (f *FakeEnvironment) Fence(_ context.Context, request ports.SandboxFenceRequest) (ports.SandboxLeaseObservation, error) {
	f.mu.Lock()
	f.releaseCalls++
	entry, exists := f.leases[request.Lease.ID]
	if !exists || entry.lease != request.Lease {
		f.mu.Unlock()
		return ports.SandboxLeaseObservation{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "sandbox lease is absent")
	}
	entry.state = "fenced"
	f.mu.Unlock()
	return f.sandboxObservation(request.Lease, "fenced")
}

func (f *FakeEnvironment) Release(_ context.Context, request ports.SandboxReleaseRequest) (ports.SandboxLeaseObservation, error) {
	if err := core.ValidateEffectDispatch(request.Intent, request.Fence, core.CurrentFenceFacts{Scope: request.Fence.Scope, CapabilityGrantDigest: request.Fence.CapabilityGrantDigest}, f.clock()); err != nil {
		return ports.SandboxLeaseObservation{}, err
	}
	f.mu.Lock()
	entry, exists := f.leases[request.Lease.ID]
	if !exists || entry.lease != request.Lease {
		f.mu.Unlock()
		return ports.SandboxLeaseObservation{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "sandbox lease is absent")
	}
	entry.state = "released"
	loseReply := f.faults.LoseReleaseReply
	f.mu.Unlock()
	if loseReply {
		return ports.SandboxLeaseObservation{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost sandbox release reply")
	}
	return f.sandboxObservation(request.Lease, "released")
}

func (f *FakeEnvironment) sandboxObservation(lease core.SandboxLeaseRef, state string) (ports.SandboxLeaseObservation, error) {
	digest, err := core.DigestJSON(map[string]any{"lease": lease, "state": state})
	if err != nil {
		return ports.SandboxLeaseObservation{}, err
	}
	return ports.SandboxLeaseObservation{Lease: lease, State: state, EvidenceRef: string(digest), ObservedAt: f.clock()}, nil
}

type FakeEvidence struct {
	mu         sync.Mutex
	clock      func() time.Time
	descriptor ports.ComponentDescriptor
	records    []ports.EvidenceRecord
	observed   map[string]observedEvidence
}

type observedEvidence struct {
	ref    ports.EvidenceRecordRef
	digest core.Digest
}

func NewFakeEvidence(id string, clock func() time.Time) (*FakeEvidence, error) {
	if clock == nil {
		clock = time.Now
	}
	descriptor, err := fakeDescriptor(id, ports.ComponentEvidence, ports.ConformanceFullyControlled, []string{"append_intent", "append_observation", "read"}, clock())
	if err != nil {
		return nil, err
	}
	return &FakeEvidence{clock: clock, descriptor: descriptor, observed: make(map[string]observedEvidence)}, nil
}

func (f *FakeEvidence) Describe(context.Context) (ports.ComponentDescriptor, error) {
	return f.descriptor, nil
}

func (f *FakeEvidence) AppendIntent(_ context.Context, request ports.EvidenceIntentRecord) (ports.EvidenceRecordRef, error) {
	if err := request.Scope.Validate(); err != nil {
		return ports.EvidenceRecordRef{}, err
	}
	if err := request.PayloadDigest.Validate(); err != nil {
		return ports.EvidenceRecordRef{}, err
	}
	return f.append("intent", request.PayloadDigest), nil
}

func (f *FakeEvidence) AppendObservation(_ context.Context, request ports.EvidenceObservationRecord) (ports.EvidenceRecordRef, error) {
	if strings.TrimSpace(request.SourceID) == "" || request.SourceEpoch == 0 || request.SourceSequence == 0 || strings.TrimSpace(request.CausationID) == "" {
		return ports.EvidenceRecordRef{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "observation source identity, sequence and causation are required")
	}
	if err := request.PayloadDigest.Validate(); err != nil {
		return ports.EvidenceRecordRef{}, err
	}
	key := fmt.Sprintf("%s/%d/%d", request.SourceID, request.SourceEpoch, request.SourceSequence)
	f.mu.Lock()
	defer f.mu.Unlock()
	if prior, exists := f.observed[key]; exists {
		if prior.digest != request.PayloadDigest {
			return ports.EvidenceRecordRef{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "source sequence was reused with different observation content")
		}
		return prior.ref, nil
	}
	ref := f.appendLocked("observation", request.PayloadDigest)
	f.observed[key] = observedEvidence{ref: ref, digest: request.PayloadDigest}
	return ref, nil
}

func (f *FakeEvidence) Read(_ context.Context, ref ports.EvidenceRecordRef) (ports.EvidenceRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ref.Scope != f.descriptor.ID || ref.Sequence == 0 || int(ref.Sequence) > len(f.records) {
		return ports.EvidenceRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "evidence record is absent")
	}
	return f.records[ref.Sequence-1], nil
}

func (f *FakeEvidence) append(classification string, digest core.Digest) ports.EvidenceRecordRef {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.appendLocked(classification, digest)
}

func (f *FakeEvidence) appendLocked(classification string, digest core.Digest) ports.EvidenceRecordRef {
	ref := ports.EvidenceRecordRef{Scope: f.descriptor.ID, Sequence: uint64(len(f.records) + 1)}
	f.records = append(f.records, ports.EvidenceRecord{Ref: ref, Classification: classification, PayloadDigest: digest, RecordedAt: f.clock()})
	return ref
}

type FakeCheckpointParticipant struct {
	mu          sync.Mutex
	clock       func() time.Time
	descriptor  ports.ComponentDescriptor
	reports     map[string]ports.CheckpointParticipantReport
	FailPrepare bool
}

func NewFakeCheckpointParticipant(id string, kind ports.ComponentKind, clock func() time.Time) (*FakeCheckpointParticipant, error) {
	if clock == nil {
		clock = time.Now
	}
	descriptor, err := fakeDescriptor(id, kind, ports.ConformanceFullyControlled, []string{"checkpoint", "restore"}, clock())
	if err != nil {
		return nil, err
	}
	return &FakeCheckpointParticipant{clock: clock, descriptor: descriptor, reports: make(map[string]ports.CheckpointParticipantReport)}, nil
}

func (f *FakeCheckpointParticipant) Describe(context.Context) (ports.ComponentDescriptor, error) {
	return f.descriptor, nil
}

func (f *FakeCheckpointParticipant) PrepareCheckpoint(_ context.Context, request ports.CheckpointPrepareRequest) (ports.CheckpointParticipantReport, error) {
	if f.FailPrepare {
		return ports.CheckpointParticipantReport{}, core.NewError(core.ErrorUnavailable, core.ReasonCheckpointInconsistent, "injected checkpoint prepare failure")
	}
	if err := request.Scope.Validate(); err != nil {
		return ports.CheckpointParticipantReport{}, err
	}
	if err := request.Effects.Validate(); err != nil {
		return ports.CheckpointParticipantReport{}, err
	}
	digest, _ := core.DigestJSON(map[string]any{"component": f.descriptor.ID, "barrier": request.BarrierID, "epoch": request.Epoch})
	report := ports.CheckpointParticipantReport{ComponentID: f.descriptor.ID, ComponentKind: f.descriptor.Kind, State: core.CheckpointParticipantPrepared, SnapshotRef: fmt.Sprintf("%s/%s", f.descriptor.ID, request.BarrierID), SnapshotDigest: digest, EvidenceDigest: digest, ObservedAt: f.clock()}
	f.mu.Lock()
	f.reports[request.BarrierID] = report
	f.mu.Unlock()
	return report, nil
}

func (f *FakeCheckpointParticipant) CommitCheckpoint(_ context.Context, request ports.CheckpointCommitRequest) (ports.CheckpointParticipantReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	report, exists := f.reports[request.BarrierID]
	if !exists || report.State != core.CheckpointParticipantPrepared {
		return ports.CheckpointParticipantReport{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint participant was not prepared")
	}
	report.State = core.CheckpointParticipantCommitted
	report.ObservedAt = f.clock()
	f.reports[request.BarrierID] = report
	return report, nil
}

func (f *FakeCheckpointParticipant) AbortCheckpoint(_ context.Context, request ports.CheckpointAbortRequest) (ports.CheckpointParticipantReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	report := f.reports[request.BarrierID]
	report.ComponentID = f.descriptor.ID
	report.ComponentKind = f.descriptor.Kind
	report.State = core.CheckpointParticipantAborted
	report.ObservedAt = f.clock()
	f.reports[request.BarrierID] = report
	return report, nil
}

func (f *FakeCheckpointParticipant) RestoreCheckpoint(_ context.Context, request ports.CheckpointRestoreRequest) (ports.CheckpointParticipantReport, error) {
	if err := request.NewScope.Validate(); err != nil {
		return ports.CheckpointParticipantReport{}, err
	}
	if err := request.SnapshotDigest.Validate(); err != nil {
		return ports.CheckpointParticipantReport{}, err
	}
	digest, _ := core.DigestJSON(map[string]any{"component": f.descriptor.ID, "restore": request.CheckpointID, "instance": request.NewScope.Instance})
	return ports.CheckpointParticipantReport{ComponentID: f.descriptor.ID, ComponentKind: f.descriptor.Kind, State: core.CheckpointParticipantCommitted, SnapshotRef: request.SnapshotRef, SnapshotDigest: request.SnapshotDigest, EvidenceDigest: digest, ObservedAt: f.clock()}, nil
}

func fakeDescriptor(id string, kind ports.ComponentKind, conformance ports.ConformanceLevel, capabilities []string, now time.Time) (ports.ComponentDescriptor, error) {
	digest, err := core.DigestJSON(map[string]any{"id": id, "kind": kind, "version": "fake-v1"})
	if err != nil {
		return ports.ComponentDescriptor{}, err
	}
	evidence, err := core.DigestJSON(map[string]any{"id": id, "certified_at": now.UnixNano()})
	if err != nil {
		return ports.ComponentDescriptor{}, err
	}
	items := make([]ports.Capability, 0, len(capabilities))
	for _, capability := range capabilities {
		items = append(items, ports.Capability{Name: capability, State: ports.CapabilityBound, EvidenceDigest: evidence, EvidenceExpiry: now.Add(24 * time.Hour)})
	}
	return ports.ComponentDescriptor{ID: id, Kind: kind, Version: "fake-v1", ArtifactDigest: digest, ContractVersion: ports.ContractVersion, Conformance: conformance, Capabilities: items}, nil
}
