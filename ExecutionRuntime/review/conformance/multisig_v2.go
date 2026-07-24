package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type HumanMultiSignStoreFixtureV2 struct {
	Create reviewport.CreateHumanPanelMutationV2
	Votes  []reviewport.RecordHumanAttestationMutationV2
	Begin  *reviewport.BeginHumanPanelDecisionMutationV2
	Decide *reviewport.DecideHumanPanelMutationV2
}

// CheckHumanMultiSignStoreV2 is reusable by the memory reference Store and the
// SQLite backend. The fixture owns setup of the referenced V1 Case/Target/Round.
func CheckHumanMultiSignStoreV2(ctx context.Context, store reviewport.StoreV2, f HumanMultiSignStoreFixtureV2) error {
	first, err := store.CreateHumanPanelV2(ctx, f.Create)
	if err != nil {
		return err
	}
	replay, err := store.CreateHumanPanelV2(ctx, f.Create)
	if err != nil {
		return err
	}
	if first.Panel.Digest != replay.Panel.Digest || len(first.Assignments) != len(replay.Assignments) {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human Panel canonical replay drifted")
	}
	if _, err := store.InspectHumanPanelExactV2(ctx, f.Create.ProposedPanel.ExactRef()); err != nil {
		return err
	}
	if _, err := store.InspectHumanPanelExactV2(ctx, f.Create.OpenPanel.ExactRef()); err != nil {
		return err
	}
	for _, vote := range f.Votes {
		result, err := store.RecordHumanAttestationV2(ctx, vote)
		if err != nil {
			return err
		}
		again, err := store.RecordHumanAttestationV2(ctx, vote)
		if err != nil {
			return err
		}
		if result.Panel.Digest != again.Panel.Digest || result.Attestation.Digest != again.Attestation.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human vote canonical replay drifted")
		}
		if _, err := store.InspectHumanAttestationExactV2(ctx, vote.Attestation.ExactRef()); err != nil {
			return err
		}
	}
	if f.Begin != nil {
		if _, _, err := store.BeginHumanPanelDecisionV2(ctx, *f.Begin); err != nil {
			return err
		}
	}
	if f.Decide != nil {
		result, err := store.DecideHumanPanelV2(ctx, *f.Decide)
		if err != nil {
			return err
		}
		again, err := store.DecideHumanPanelV2(ctx, *f.Decide)
		if err != nil {
			return err
		}
		if result.Verdict.Digest != again.Verdict.Digest || result.Panel.Digest != again.Panel.Digest || result.Case.Digest != again.Case.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human Verdict canonical replay drifted")
		}
		if _, err := store.InspectHumanVerdictExactV2(ctx, f.Decide.Verdict.ExactRef()); err != nil {
			return err
		}
	}
	return nil
}

// Compile-time API assertion: the reusable suite never accepts a direct
// Verdict writer, only the compound StoreV2 owner boundary.
var _ contract.HumanVerdictV2
