package control

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type AppendOperationDispatchEnforcementRequestV5 struct {
	Operation               ports.OperationSubjectV3                         `json:"operation"`
	EffectID                core.EffectIntentID                              `json:"effect_id"`
	PermitID                string                                           `json:"permit_id"`
	ExpectedJournalRevision core.Revision                                    `json:"expected_journal_revision"`
	Receipt                 ports.OperationDispatchEnforcementPhaseReceiptV5 `json:"receipt"`
}
type OperationDispatchEnforcementFactPortV5 interface {
	AppendOperationDispatchEnforcementV5(context.Context, AppendOperationDispatchEnforcementRequestV5) (ports.OperationDispatchEnforcementJournalV5, error)
	InspectOperationDispatchEnforcementV5(context.Context, ports.OperationSubjectV3, core.EffectIntentID, string) (ports.OperationDispatchEnforcementJournalV5, error)
}
