package testkit

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type LocalConformance struct {
	mu      sync.RWMutex
	reports map[string]contract.BackendConformanceReport
}

func NewLocalConformance(reports ...contract.BackendConformanceReport) *LocalConformance {
	values := make(map[string]contract.BackendConformanceReport, len(reports))
	for _, report := range reports {
		values[report.BackendRef.ID] = clone(report)
	}
	return &LocalConformance{reports: values}
}

func (c *LocalConformance) Assess(_ context.Context, request ports.BackendConformanceRequest) (contract.BackendConformanceReport, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	report, ok := c.reports[request.Backend.Meta.ID]
	if !ok {
		return contract.BackendConformanceReport{}, ports.ErrNotFound
	}
	if report.ProductionProof {
		return contract.BackendConformanceReport{}, ports.ErrUnsupported
	}
	if !contract.SameRef(report.BackendRef, request.Backend.Meta.Ref()) || !contract.SameRef(report.RequirementRef, request.Requirement.Meta.Ref()) {
		return contract.BackendConformanceReport{}, ports.ErrConflict
	}
	return clone(report), nil
}
