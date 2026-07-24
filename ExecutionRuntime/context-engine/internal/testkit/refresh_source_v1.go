package testkit

// This file is an isolated, in-memory test fixture. It is not an Application
// implementation, Tool owner, production adapter, backend, root or SLA.

import (
	"context"
	"fmt"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type SettledActionSourceReaderV1 struct {
	mu         sync.RWMutex
	projection contract.SettledActionContextSourceCurrentV1
	err        error
	readCount  uint64
}

func NewSettledActionSourceReaderV1(projection contract.SettledActionContextSourceCurrentV1) *SettledActionSourceReaderV1 {
	return &SettledActionSourceReaderV1{projection: projection}
}

func (r *SettledActionSourceReaderV1) SetProjection(projection contract.SettledActionContextSourceCurrentV1) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projection = projection
}

func (r *SettledActionSourceReaderV1) SetError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}
func (r *SettledActionSourceReaderV1) ReadCount() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.readCount
}

func (r *SettledActionSourceReaderV1) InspectSettledActionContextSourceCurrentV1(ctx context.Context, request contract.SettledActionContextSourceRequestV1) (contract.SettledActionContextSourceCurrentV1, error) {
	if ctx == nil {
		return contract.SettledActionContextSourceCurrentV1{}, fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return contract.SettledActionContextSourceCurrentV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readCount++
	if r.err != nil {
		return contract.SettledActionContextSourceCurrentV1{}, r.err
	}
	if r.projection.Request != request {
		return contract.SettledActionContextSourceCurrentV1{}, fmt.Errorf("%w: settled action exact request", contract.ErrConflict)
	}
	return cloneJSONV1(r.projection)
}
