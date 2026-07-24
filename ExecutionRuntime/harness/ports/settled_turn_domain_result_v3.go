package ports

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
)

type SettledTurnDomainResultRepositoryV3 interface {
	EnsureExact(context.Context, contract.SettledTurnDomainResultFactV3) (contract.SettledTurnDomainResultFactV3, error)
	InspectExact(context.Context, contract.SettledTurnDomainResultFactRefV3) (contract.SettledTurnDomainResultFactV3, error)
}

type SettledTurnDomainResultReaderV3 interface {
	InspectExact(context.Context, contract.SettledTurnDomainResultFactRefV3) (contract.SettledTurnDomainResultFactV3, error)
}
