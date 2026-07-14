package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type RunCoordinationCASRequestV3 struct {
	Scope            core.ExecutionScope            `json:"scope"`
	ID               string                         `json:"id"`
	ExpectedRevision core.Revision                  `json:"expected_revision"`
	Next             contract.RunCoordinationFactV3 `json:"next"`
}

// RunCoordinationFactPortV3 owns only Application recovery watermarks. It
// cannot create or complete Runtime Runs and cannot grant dispatch authority.
type RunCoordinationFactPortV3 interface {
	CreateRunCoordinationV3(context.Context, contract.RunCoordinationFactV3) (contract.RunCoordinationFactV3, error)
	InspectRunCoordinationV3(context.Context, core.ExecutionScope, string) (contract.RunCoordinationFactV3, error)
	CompareAndSwapRunCoordinationV3(context.Context, RunCoordinationCASRequestV3) (contract.RunCoordinationFactV3, error)
}
