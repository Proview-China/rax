package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

// ExactCurrentStore is a read-only Sandbox Owner boundary. It deliberately
// exposes no mutation or Provider method.
type ExactCurrentStore interface {
	GetAttempt(context.Context, string) (contract.DomainAttemptFact, error)
	InspectReservationByAttempt(context.Context, string, string, string) (contract.DomainReservation, error)
	GetRuntimeLeaseBinding(context.Context, string) (contract.RuntimeLeaseBindingFact, error)
	GetProjection(context.Context, string) (contract.EnvironmentProjection, error)
	GetRequirement(context.Context, string) (contract.ExecutionRequirement, error)
	GetPolicy(context.Context, string) (contract.PolicyProjection, error)
	GetPlacement(context.Context, string) (contract.PlacementCandidate, error)
	GetBackend(context.Context, string) (contract.BackendDescriptor, error)
	GetSlot(context.Context, string) (contract.SlotCandidate, error)
}
