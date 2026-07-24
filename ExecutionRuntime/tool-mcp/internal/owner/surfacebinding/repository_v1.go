package surfacebinding

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type invocationGateV1 struct {
	mu   sync.Mutex
	refs int
}

type bindingRecordV1 struct {
	request       toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1
	requestDigest core.Digest
	binding       toolcontract.ToolSurfaceInvocationBindingV1
	ack           toolcontract.ToolSurfaceInvocationBindingAckV1
}

// InMemoryRepositoryV1 is the single Tool-owner history and invocation index.
// It is production-neutral: it owns no provider, transport, or composition root.
type InMemoryRepositoryV1 struct {
	mu           sync.RWMutex
	byBindingID  map[string]bindingRecordV1
	byInvocation map[string]toolcontract.ToolSurfaceInvocationBindingRefV1

	gatesMu sync.Mutex
	gates   map[string]*invocationGateV1
	owner   core.OwnerRef
	clock   func() time.Time
}

func NewInMemoryRepositoryV1(owner core.OwnerRef, clock func() time.Time) (*InMemoryRepositoryV1, error) {
	if err := owner.Validate(); err != nil {
		return nil, invalidV1("Tool Surface Invocation Binding Repository Owner is required")
	}
	if clock == nil {
		return nil, invalidV1("Tool Surface Invocation Binding Repository clock is required")
	}
	return &InMemoryRepositoryV1{
		byBindingID:  make(map[string]bindingRecordV1),
		byInvocation: make(map[string]toolcontract.ToolSurfaceInvocationBindingRefV1),
		gates:        make(map[string]*invocationGateV1),
		owner:        owner,
		clock:        clock,
	}, nil
}

func (r *InMemoryRepositoryV1) EnsureToolSurfaceInvocationBindingV1(ctx context.Context, request toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	if r == nil || r.clock == nil || r.owner.Validate() != nil {
		return zeroV1(unavailableV1("Tool Surface Invocation Binding Repository is unavailable"))
	}
	if err := contextErrorV1(ctx); err != nil {
		return zeroV1(err)
	}
	request = cloneRequestV1(request)
	if err := request.Validate(); err != nil {
		return zeroV1(err)
	}
	requestDigest, err := request.ComputeDigest()
	if err != nil {
		return zeroV1(err)
	}

	first := r.clock()
	if err := validateRequestCurrentV1(request, first); err != nil {
		return zeroV1(err)
	}
	release := r.acquireGateV1(invocationKeyV1(request.Invocation))
	defer release()
	if err := contextErrorV1(ctx); err != nil {
		return zeroV1(err)
	}
	second := r.clock()
	if err := validateMonotonicRequestV1(request, first, second); err != nil {
		return zeroV1(err)
	}

	subject, err := toolcontract.SealToolSurfaceInvocationBindingSubjectV1(request)
	if err != nil {
		return zeroV1(err)
	}
	deadline, _ := ctx.Deadline()
	notAfter := toolcontract.ToolSurfaceInvocationBindingNotAfterV1(subject, deadline)
	if second.UnixNano() >= notAfter {
		return zeroV1(expiredV1("Tool Surface Invocation Binding expired before commit"))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := contextErrorV1(ctx); err != nil {
		return zeroV1(err)
	}
	commitNow := r.clock()
	if err := validateMonotonicRequestV1(request, second, commitNow); err != nil {
		return zeroV1(err)
	}
	if commitNow.UnixNano() >= notAfter {
		return zeroV1(expiredV1("Tool Surface Invocation Binding expired at commit"))
	}

	key := invocationKeyV1(request.Invocation)
	if exact, exists := r.byInvocation[key]; exists {
		winner, ok := r.byBindingID[exact.ID]
		if !ok || winner.binding.Ref != exact {
			return zeroV1(conflictV1("Tool Surface Invocation Binding invocation index drifted"))
		}
		if err := request.ValidateAgainst(winner.binding); err != nil {
			return zeroV1(err)
		}
		if winner.requestDigest != requestDigest {
			return zeroV1(core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Surface Invocation Binding request canonical digest differs from winner"))
		}
		if err := winner.binding.ValidateCurrent(commitNow); err != nil {
			return zeroV1(err)
		}
		if err := winner.ack.ValidateAgainst(winner.binding, commitNow); err != nil {
			return zeroV1(err)
		}
		return cloneRecordV1(winner)
	}

	binding, err := toolcontract.SealToolSurfaceInvocationBindingV1(toolcontract.ToolSurfaceInvocationBindingV1{
		Ref: toolcontract.ToolSurfaceInvocationBindingRefV1{Owner: r.owner}, Subject: subject,
		CreatedUnixNano: commitNow.UnixNano(), NotAfterUnixNano: notAfter,
	})
	if err != nil {
		return zeroV1(err)
	}
	ack, err := toolcontract.SealToolSurfaceInvocationBindingAckV1(toolcontract.ToolSurfaceInvocationBindingAckV1{
		BindingRef: binding.Ref, Invocation: binding.Subject.Invocation,
		PreparedFactRef: binding.Subject.PreparedFactRef, PreparedCurrentRef: binding.Subject.PreparedCurrentRef,
		CheckedUnixNano: commitNow.UnixNano(), NotAfterUnixNano: binding.NotAfterUnixNano,
	})
	if err != nil {
		return zeroV1(err)
	}
	if err := ack.ValidateAgainst(binding, commitNow); err != nil {
		return zeroV1(err)
	}
	if _, exists := r.byBindingID[binding.Ref.ID]; exists {
		return zeroV1(conflictV1("Tool Surface Invocation Binding ID already stores another winner"))
	}
	record := cloneRecordValueV1(bindingRecordV1{request: request, requestDigest: requestDigest, binding: binding, ack: ack})
	r.byBindingID[binding.Ref.ID] = record
	r.byInvocation[key] = binding.Ref
	return cloneRecordV1(record)
}

func (r *InMemoryRepositoryV1) InspectToolSurfaceInvocationBindingByInvocationV1(ctx context.Context, invocation toolcontract.ToolSurfaceInvocationCoordinateV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	if r == nil || r.clock == nil {
		return zeroV1(unavailableV1("Tool Surface Invocation Binding Repository is unavailable"))
	}
	if err := contextErrorV1(ctx); err != nil {
		return zeroV1(err)
	}
	if err := invocation.Validate(); err != nil {
		return zeroV1(err)
	}
	r.mu.RLock()
	exact, exists := r.byInvocation[invocationKeyV1(invocation)]
	record, recordExists := r.byBindingID[exact.ID]
	r.mu.RUnlock()
	if !exists || !recordExists {
		return zeroV1(notFoundV1("Tool Surface Invocation Binding invocation is absent"))
	}
	if exact != record.binding.Ref || record.binding.Subject.Invocation != invocation {
		return zeroV1(conflictV1("Tool Surface Invocation Binding invocation index drifted"))
	}
	return r.validateAndCloneRecordV1(ctx, record)
}

func (r *InMemoryRepositoryV1) InspectExactToolSurfaceInvocationBindingV1(ctx context.Context, exact toolcontract.ToolSurfaceInvocationBindingRefV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	if r == nil || r.clock == nil {
		return zeroV1(unavailableV1("Tool Surface Invocation Binding Repository is unavailable"))
	}
	if err := contextErrorV1(ctx); err != nil {
		return zeroV1(err)
	}
	if err := exact.Validate(); err != nil {
		return zeroV1(err)
	}
	r.mu.RLock()
	record, exists := r.byBindingID[exact.ID]
	r.mu.RUnlock()
	if !exists {
		return zeroV1(notFoundV1("Tool Surface Invocation Binding exact Ref is absent"))
	}
	if record.binding.Ref != exact {
		return zeroV1(conflictV1("Tool Surface Invocation Binding exact Ref drifted"))
	}
	return r.validateAndCloneRecordV1(ctx, record)
}

func (r *InMemoryRepositoryV1) validateAndCloneRecordV1(ctx context.Context, record bindingRecordV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	if err := record.binding.Validate(); err != nil {
		return zeroV1(conflictV1("stored Tool Surface Invocation Binding is non-canonical"))
	}
	if err := record.ack.Validate(); err != nil {
		return zeroV1(conflictV1("stored Tool Surface Invocation Binding Ack is non-canonical"))
	}
	if err := record.request.ValidateAgainst(record.binding); err != nil {
		return zeroV1(err)
	}
	digest, err := record.request.ComputeDigest()
	if err != nil || digest != record.requestDigest {
		return zeroV1(conflictV1("stored Tool Surface Invocation Binding request canonical digest drifted"))
	}
	if err := contextErrorV1(ctx); err != nil {
		return zeroV1(err)
	}
	r.mu.RLock()
	confirmed, exists := r.byBindingID[record.binding.Ref.ID]
	current, currentExists := r.byInvocation[invocationKeyV1(record.binding.Subject.Invocation)]
	r.mu.RUnlock()
	if !exists || !currentExists || current != record.binding.Ref || !reflect.DeepEqual(confirmed, record) {
		return zeroV1(conflictV1("Tool Surface Invocation Binding changed during exact inspection"))
	}
	return cloneRecordV1(record)
}

func (r *InMemoryRepositoryV1) acquireGateV1(key string) func() {
	r.gatesMu.Lock()
	gate := r.gates[key]
	if gate == nil {
		gate = &invocationGateV1{}
		r.gates[key] = gate
	}
	gate.refs++
	r.gatesMu.Unlock()
	gate.mu.Lock()
	return func() {
		gate.mu.Unlock()
		r.gatesMu.Lock()
		gate.refs--
		if gate.refs == 0 && r.gates[key] == gate {
			delete(r.gates, key)
		}
		r.gatesMu.Unlock()
	}
}

func validateRequestCurrentV1(request toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1, now time.Time) error {
	if now.IsZero() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Invocation Binding clock is unavailable")
	}
	if err := request.PreparedCurrent.ValidateCurrent(request.PreparedCurrentRef, now); err != nil {
		return err
	}
	if err := request.SurfaceCurrent.ValidateCurrent(request.SurfaceCurrent.Ref, now); err != nil {
		return err
	}
	if err := request.AssemblyCurrent.ValidateCurrent(request.AssemblyCurrentRef, now); err != nil {
		return err
	}
	if !now.Before(time.Unix(0, request.RequestedNotAfterUnixNano)) || !now.Before(time.Unix(0, request.PreparedHistoricalFact.NotAfterUnixNano)) {
		return expiredV1("Tool Surface Invocation Binding request is no longer current")
	}
	return nil
}

func validateMonotonicRequestV1(request toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1, previous, now time.Time) error {
	if now.Before(previous) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Invocation Binding clock regressed")
	}
	return validateRequestCurrentV1(request, now)
}

func contextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return invalidV1("Tool Surface Invocation Binding context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func invocationKeyV1(invocation toolcontract.ToolSurfaceInvocationCoordinateV1) string {
	return invocation.InvocationID + "\x00" + string(invocation.InvocationDigest)
}

func cloneRecordV1(record bindingRecordV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	record = cloneRecordValueV1(record)
	return record.binding, record.ack, nil
}

func cloneRecordValueV1(record bindingRecordV1) bindingRecordV1 {
	record.request = cloneRequestV1(record.request)
	record.binding.Subject.SurfaceCurrent = cloneSurfaceProjectionV1(record.binding.Subject.SurfaceCurrent)
	return record
}

func cloneRequestV1(request toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1) toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1 {
	request.SurfaceCurrent = cloneSurfaceProjectionV1(request.SurfaceCurrent)
	return request
}

func cloneSurfaceProjectionV1(projection toolcontract.ToolSurfaceManifestCurrentProjectionV1) toolcontract.ToolSurfaceManifestCurrentProjectionV1 {
	projection.Manifest.Entries = append([]toolcontract.ToolSurfaceEntry(nil), projection.Manifest.Entries...)
	for index := range projection.Manifest.Entries {
		projection.Manifest.Entries[index].EffectKinds = append([]runtimeports.NamespacedNameV2(nil), projection.Manifest.Entries[index].EffectKinds...)
	}
	projection.Manifest.Residuals = append([]toolcontract.Residual(nil), projection.Manifest.Residuals...)
	return projection
}

func zeroV1(err error) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	return toolcontract.ToolSurfaceInvocationBindingV1{}, toolcontract.ToolSurfaceInvocationBindingAckV1{}, err
}

func invalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, message)
}

func conflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}

func notFoundV1(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, message)
}

func unavailableV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, message)
}

func expiredV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, message)
}

var _ toolcontract.ToolSurfaceInvocationBindingRepositoryV1 = (*InMemoryRepositoryV1)(nil)
