package applicationadapter

import (
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	harnesscontract "github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ContextTurnSourceFromCommittedPendingActionV3 is the sole Harness-owned
// nominal mapping from the committed PendingAction current projection into the
// Application neutral Session/Turn coordinate. It does not increment a Turn
// or create a Context fact.
func ContextTurnSourceFromCommittedPendingActionV3(current harnesscontract.CommittedPendingActionCurrentV3, now time.Time) (applicationcontract.ContextTurnSourceCurrentV1, error) {
	if err := current.Validate(now); err != nil {
		return applicationcontract.ContextTurnSourceCurrentV1{}, err
	}
	if current.SessionApplicability.Kind != applicationcontract.SingleCallSessionSourceKindV1 || current.TurnApplicability.Kind != applicationcontract.SingleCallTurnSourceKindV1 {
		return applicationcontract.ContextTurnSourceCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "Harness and Application Session/Turn kinds differ")
	}
	projection, err := applicationcontract.SealContextTurnSourceCurrentV1(applicationcontract.ContextTurnSourceCurrentV1{
		ExecutionScopeDigest: current.ExecutionScopeDigest, RunID: current.Run.RunID,
		Session:              applicationcontract.SingleCallSessionCoordinateV1{ID: current.SessionID, Revision: current.SessionRevision, Digest: current.SessionDigest, Phase: applicationcontract.SingleCallSessionWaitingActionV1, CheckedUnixNano: current.CheckedUnixNano, ExpiresUnixNano: current.ExpiresUnixNano},
		SessionApplicability: applicationcontract.SingleCallSessionApplicabilitySourceCoordinateV1{Kind: current.SessionApplicability.Kind, ID: current.SessionApplicability.ID, Revision: current.SessionApplicability.Revision, Digest: current.SessionApplicability.Digest},
		Turn:                 applicationcontract.SingleCallTurnCoordinateV1{ID: current.TurnApplicability.ID, Ordinal: current.Turn, Revision: current.TurnApplicability.Revision, Digest: current.TurnApplicability.Digest},
		TurnApplicability:    applicationcontract.SingleCallTurnApplicabilitySourceCoordinateV1{Kind: current.TurnApplicability.Kind, ID: current.TurnApplicability.ID, Revision: current.TurnApplicability.Revision, Digest: current.TurnApplicability.Digest},
		CheckedUnixNano:      current.CheckedUnixNano, ExpiresUnixNano: current.ExpiresUnixNano,
	})
	if err != nil {
		return applicationcontract.ContextTurnSourceCurrentV1{}, err
	}
	return projection, projection.ValidateCurrent(now)
}
