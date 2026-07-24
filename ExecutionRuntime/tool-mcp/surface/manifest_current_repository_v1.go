package surface

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type toolSurfaceManifestHistoryKeyV1 struct {
	ID       string
	Revision core.Revision
}

type toolSurfaceManifestGateV1 struct {
	mu   sync.Mutex
	refs int
}

type InMemoryToolSurfaceManifestCurrentRepositoryV1 struct {
	mu      sync.RWMutex
	history map[toolSurfaceManifestHistoryKeyV1]toolcontract.ToolSurfaceManifestCurrentProjectionV1
	current map[string]toolcontract.ToolSurfaceManifestCurrentRefV1

	gatesMu sync.Mutex
	gates   map[string]*toolSurfaceManifestGateV1
	clock   func() time.Time
}

func NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock func() time.Time) (*InMemoryToolSurfaceManifestCurrentRepositoryV1, error) {
	if clock == nil {
		return nil, toolSurfaceManifestInvalidV1("Tool Surface Manifest current Repository clock is required")
	}
	return &InMemoryToolSurfaceManifestCurrentRepositoryV1{
		history: make(map[toolSurfaceManifestHistoryKeyV1]toolcontract.ToolSurfaceManifestCurrentProjectionV1),
		current: make(map[string]toolcontract.ToolSurfaceManifestCurrentRefV1),
		gates:   make(map[string]*toolSurfaceManifestGateV1),
		clock:   clock,
	}, nil
}

func (r *InMemoryToolSurfaceManifestCurrentRepositoryV1) EnsureExactToolSurfaceManifestCurrentV1(ctx context.Context, request toolcontract.ToolSurfaceManifestCurrentEnsureRequestV1) (toolcontract.ToolSurfaceManifestCurrentProjectionV1, error) {
	if r == nil || r.clock == nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolSurfaceManifestUnavailableV1("Tool Surface Manifest current Repository is unavailable")
	}
	if err := toolSurfaceManifestContextErrorV1(ctx); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	request = cloneToolSurfaceManifestEnsureRequestV1(request)
	if err := request.Validate(); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	first := r.clock()
	if err := validateToolSurfaceManifestTimeV1(request.Manifest, first); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}

	release := r.acquireToolSurfaceManifestGateV1(request.Manifest.ID)
	defer release()
	if err := toolSurfaceManifestContextErrorV1(ctx); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	second := r.clock()
	if err := validateMonotonicToolSurfaceManifestTimeV1(request.Manifest, first, second); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	if err := toolSurfaceManifestContextErrorV1(ctx); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := toolSurfaceManifestContextErrorV1(ctx); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	commitNow := r.clock()
	if err := validateMonotonicToolSurfaceManifestTimeV1(request.Manifest, second, commitNow); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	if err := toolSurfaceManifestContextErrorV1(ctx); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}

	key := toolSurfaceManifestHistoryKeyV1{ID: request.Manifest.ID, Revision: request.Manifest.Revision}
	if winner, exists := r.history[key]; exists {
		return r.validateHistoryWinnerLockedV1(request, winner, commitNow)
	}

	current, currentExists := r.current[request.Manifest.ID]
	if request.Manifest.Revision == 1 {
		if currentExists {
			return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolSurfaceManifestConflictV1("initial Tool Surface Manifest current already has a current winner")
		}
	} else {
		if !currentExists || current != request.ExpectedCurrent {
			return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolSurfaceManifestConflictV1("Tool Surface Manifest successor ExpectedCurrent lost exact CAS")
		}
		previous, exists := r.history[toolSurfaceManifestHistoryKeyV1{ID: current.ID, Revision: current.Revision}]
		if !exists || previous.Ref != current {
			return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolSurfaceManifestConflictV1("Tool Surface Manifest current index and history drifted")
		}
		if err := previous.ValidateCurrent(current, commitNow); err != nil {
			return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
		}
		if previous.Owner != request.Manifest.Owner {
			return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolSurfaceManifestConflictV1("Tool Surface Manifest ID cannot cross Owner")
		}
	}

	projection, err := toolcontract.SealToolSurfaceManifestCurrentV1(toolcontract.ToolSurfaceManifestCurrentProjectionV1{
		ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1,
		Manifest:        cloneToolSurfaceManifestV1(request.Manifest), Owner: request.Manifest.Owner,
		CheckedUnixNano: commitNow.UnixNano(), ExpiresUnixNano: request.Manifest.ExpiresUnixNano,
	})
	if err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	if err := projection.ValidateCurrent(projection.Ref, commitNow); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	if err := toolSurfaceManifestContextErrorV1(ctx); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	r.history[key] = cloneToolSurfaceManifestProjectionV1(projection)
	r.current[projection.Ref.ID] = projection.Ref
	return cloneToolSurfaceManifestProjectionV1(projection), nil
}

func (r *InMemoryToolSurfaceManifestCurrentRepositoryV1) InspectExactToolSurfaceManifestCurrentV1(ctx context.Context, exact toolcontract.ToolSurfaceManifestCurrentRefV1) (toolcontract.ToolSurfaceManifestCurrentProjectionV1, error) {
	if r == nil || r.clock == nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolSurfaceManifestUnavailableV1("Tool Surface Manifest current Repository is unavailable")
	}
	if err := toolSurfaceManifestContextErrorV1(ctx); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	if err := exact.Validate(); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}

	r.mu.RLock()
	current, currentExists := r.current[exact.ID]
	winner, historyExists := r.history[toolSurfaceManifestHistoryKeyV1{ID: exact.ID, Revision: exact.Revision}]
	r.mu.RUnlock()
	if !currentExists || !historyExists {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolSurfaceManifestNotFoundV1("Tool Surface Manifest current exact Ref is absent")
	}
	if current != exact || winner.Ref != exact {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolSurfaceManifestConflictV1("Tool Surface Manifest exact Ref is not current")
	}
	if err := toolSurfaceManifestContextErrorV1(ctx); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	now := r.clock()
	if err := winner.ValidateCurrent(exact, now); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	if err := toolSurfaceManifestContextErrorV1(ctx); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}

	r.mu.RLock()
	confirmedCurrent, currentExists := r.current[exact.ID]
	confirmedWinner, historyExists := r.history[toolSurfaceManifestHistoryKeyV1{ID: exact.ID, Revision: exact.Revision}]
	r.mu.RUnlock()
	if !currentExists || !historyExists || confirmedCurrent != exact || !reflect.DeepEqual(confirmedWinner, winner) {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolSurfaceManifestConflictV1("Tool Surface Manifest current changed during exact inspection")
	}
	return cloneToolSurfaceManifestProjectionV1(winner), nil
}

func (r *InMemoryToolSurfaceManifestCurrentRepositoryV1) validateHistoryWinnerLockedV1(request toolcontract.ToolSurfaceManifestCurrentEnsureRequestV1, winner toolcontract.ToolSurfaceManifestCurrentProjectionV1, now time.Time) (toolcontract.ToolSurfaceManifestCurrentProjectionV1, error) {
	if err := winner.Validate(); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolSurfaceManifestConflictV1("stored Tool Surface Manifest current winner is non-canonical")
	}
	if !reflect.DeepEqual(winner.Manifest, request.Manifest) {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Surface Manifest current revision stores different content")
	}
	current, exists := r.current[winner.Ref.ID]
	if !exists || current != winner.Ref {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "historical Tool Surface Manifest revision is no longer current")
	}
	if request.Manifest.Revision > 1 {
		previous, exists := r.history[toolSurfaceManifestHistoryKeyV1{ID: request.Manifest.ID, Revision: request.Manifest.Revision - 1}]
		if !exists || previous.Ref != request.ExpectedCurrent {
			return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolSurfaceManifestConflictV1("Tool Surface Manifest current replay changed ExpectedCurrent")
		}
	}
	if err := winner.ValidateCurrent(winner.Ref, now); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	return cloneToolSurfaceManifestProjectionV1(winner), nil
}

func (r *InMemoryToolSurfaceManifestCurrentRepositoryV1) acquireToolSurfaceManifestGateV1(id string) func() {
	r.gatesMu.Lock()
	gate := r.gates[id]
	if gate == nil {
		gate = &toolSurfaceManifestGateV1{}
		r.gates[id] = gate
	}
	gate.refs++
	r.gatesMu.Unlock()
	gate.mu.Lock()
	return func() {
		gate.mu.Unlock()
		r.gatesMu.Lock()
		gate.refs--
		if gate.refs == 0 && r.gates[id] == gate {
			delete(r.gates, id)
		}
		r.gatesMu.Unlock()
	}
}

func validateToolSurfaceManifestTimeV1(manifest toolcontract.ToolSurfaceManifest, now time.Time) error {
	if now.IsZero() || now.UnixNano() < manifest.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Manifest current clock is unavailable or regressed")
	}
	if !now.Before(time.Unix(0, manifest.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Surface Manifest expired before current commit")
	}
	return nil
}

func validateMonotonicToolSurfaceManifestTimeV1(manifest toolcontract.ToolSurfaceManifest, previous, now time.Time) error {
	if err := validateToolSurfaceManifestTimeV1(manifest, now); err != nil {
		return err
	}
	if now.Before(previous) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Manifest current clock regressed between validation points")
	}
	return nil
}

func toolSurfaceManifestContextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return toolSurfaceManifestInvalidV1("Tool Surface Manifest current context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func cloneToolSurfaceManifestEnsureRequestV1(request toolcontract.ToolSurfaceManifestCurrentEnsureRequestV1) toolcontract.ToolSurfaceManifestCurrentEnsureRequestV1 {
	request.Manifest = cloneToolSurfaceManifestV1(request.Manifest)
	return request
}

func cloneToolSurfaceManifestProjectionV1(projection toolcontract.ToolSurfaceManifestCurrentProjectionV1) toolcontract.ToolSurfaceManifestCurrentProjectionV1 {
	projection.Manifest = cloneToolSurfaceManifestV1(projection.Manifest)
	return projection
}

func cloneToolSurfaceManifestV1(manifest toolcontract.ToolSurfaceManifest) toolcontract.ToolSurfaceManifest {
	manifest.Entries = append([]toolcontract.ToolSurfaceEntry(nil), manifest.Entries...)
	for index := range manifest.Entries {
		manifest.Entries[index].EffectKinds = append([]runtimeports.NamespacedNameV2(nil), manifest.Entries[index].EffectKinds...)
	}
	manifest.Residuals = append([]toolcontract.Residual(nil), manifest.Residuals...)
	return manifest
}

func toolSurfaceManifestInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, message)
}

func toolSurfaceManifestConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}

func toolSurfaceManifestNotFoundV1(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, message)
}

func toolSurfaceManifestUnavailableV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, message)
}

var _ toolcontract.ToolSurfaceManifestCurrentRepositoryV1 = (*InMemoryToolSurfaceManifestCurrentRepositoryV1)(nil)
