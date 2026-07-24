package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
)

type SessionCurrentReaderV4 interface {
	InspectSessionV4(context.Context, contract.RunRef, string) (contract.GovernedSessionV4, error)
}

type SessionFactPortV4 interface {
	SessionCurrentReaderV4
	CreateSessionV4(context.Context, contract.GovernedSessionV4) (contract.GovernedSessionV4, error)
	CompareAndSwapSessionV4(context.Context, contract.SessionCASRequestV4) (contract.GovernedSessionV4, error)
}
