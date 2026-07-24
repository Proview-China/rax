package ports

import (
	"context"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

var (
	ErrNotFound       = errors.New("sandbox fact not found")
	ErrConflict       = errors.New("sandbox fact conflict")
	ErrStale          = errors.New("sandbox fact stale")
	ErrSourceConflict = errors.New("sandbox observation source conflict")
	ErrUnknownOutcome = errors.New("sandbox provider outcome is unknown; inspect original attempt")
)

// FactStore is the Sandbox Owner persistence boundary. Every mutating method
// must be atomic. Implementations must not infer Runtime Lease/Fence changes.
type FactStore interface {
	CreateReservation(context.Context, contract.DomainReservation) error
	GetReservation(context.Context, string) (contract.DomainReservation, error)
	AppendObservation(context.Context, string, contract.Observation) (accepted bool, err error)
	GetObservation(context.Context, string) (contract.Observation, error)
	CreateInspection(context.Context, contract.InspectionFact) error
	GetInspection(context.Context, string) (contract.InspectionFact, error)
	CreateDomainResult(context.Context, contract.SandboxDomainResultFact) error
	GetDomainResult(context.Context, string) (contract.SandboxDomainResultFact, error)
	GetSettlementBinding(context.Context, contract.Ref) (contract.Ref, error)
	GetProjection(context.Context, string) (contract.EnvironmentProjection, error)
	// CompareAndSwapProjection must atomically bind the projection's opaque
	// settlement ref to its exact domain result ref. Reusing an opaque ref for
	// another result is a conflict, including after later projections advance.
	CompareAndSwapProjection(context.Context, uint64, contract.EnvironmentProjection) error
}
