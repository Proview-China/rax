package ports

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
)

type SessionFactPortV3 interface {
	CreateSessionV3(context.Context, contract.GovernedSessionV3) (contract.GovernedSessionV3, error)
	InspectSessionV3(context.Context, contract.RunRef, string) (contract.GovernedSessionV3, error)
	// CompareAndSwapSessionV3 proves the Session-local current-to-next lineage.
	// It does not replace exact owner reads of Model Projection, Identity,
	// DomainResult, or Runtime Settlement facts.
	CompareAndSwapSessionV3(context.Context, contract.SessionCASRequestV3) (contract.GovernedSessionV3, error)
}
