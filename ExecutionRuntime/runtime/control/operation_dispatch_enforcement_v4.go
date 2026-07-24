package control

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type AppendOperationDispatchEnforcementRequestV4 struct {
	Operation               ports.OperationSubjectV3                         `json:"operation"`
	EffectID                core.EffectIntentID                              `json:"effect_id"`
	PermitID                string                                           `json:"permit_id"`
	ExpectedJournalRevision core.Revision                                    `json:"expected_journal_revision"`
	Receipt                 ports.OperationDispatchEnforcementPhaseReceiptV4 `json:"receipt"`
}

// OperationDispatchEnforcementFactPortV4 is the Runtime Effect Owner's raw
// sidecar primitive. It must share the Permit partition and linearization lock.
// Application and Provider code use the public governance Port instead.
type OperationDispatchEnforcementFactPortV4 interface {
	AppendOperationDispatchEnforcementV4(context.Context, AppendOperationDispatchEnforcementRequestV4) (ports.OperationDispatchEnforcementJournalV4, error)
	InspectOperationDispatchEnforcementV4(context.Context, ports.OperationSubjectV3, core.EffectIntentID, string) (ports.OperationDispatchEnforcementJournalV4, error)
}
