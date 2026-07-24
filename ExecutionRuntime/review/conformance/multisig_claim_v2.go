package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type HumanClaimStoreFixtureV2 struct {
	Claim reviewport.ClaimHumanAssignmentMutationV2
}

// CheckHumanClaimStoreV2 verifies the public compound-mutation contract. The
// caller must prepare the exact open Panel/offered Assignment closure first.
func CheckHumanClaimStoreV2(ctx context.Context, store reviewport.StoreV2, fixture HumanClaimStoreFixtureV2) error {
	first, err := store.ClaimHumanAssignmentV2(ctx, fixture.Claim)
	if err != nil {
		return err
	}
	if first.Panel.Digest != fixture.Claim.NextPanel.Digest || first.Assignment.Digest != fixture.Claim.NextAssignment.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "human claim conformance returned different content")
	}
	replay, err := store.ClaimHumanAssignmentV2(ctx, fixture.Claim)
	if err != nil {
		return err
	}
	if replay.Panel.Digest != first.Panel.Digest || replay.Assignment.Digest != first.Assignment.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human claim conformance replay drifted")
	}
	if _, err = store.InspectHumanPanelExactV2(ctx, fixture.Claim.ExpectedPanel); err != nil {
		return err
	}
	oldAssignment, err := store.InspectHumanPanelAssignmentExactV2(ctx, fixture.Claim.ExpectedAssignment)
	if err != nil {
		return err
	}
	if oldAssignment.State != contract.HumanAssignmentOfferedV2 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "human claim conformance overwrote offered history")
	}
	currentPanel, err := store.InspectHumanPanelCurrentV2(ctx, first.Panel.TenantID, first.Panel.ID)
	if err != nil {
		return err
	}
	currentAssignment, err := store.InspectHumanPanelAssignmentCurrentV2(ctx, first.Assignment.TenantID, first.Assignment.ID)
	if err != nil {
		return err
	}
	if currentPanel.Digest != first.Panel.Digest || currentAssignment.Digest != first.Assignment.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human claim conformance current indexes drifted")
	}
	if _, err = store.InspectTraceExactV1(ctx, fixture.Claim.Trace.TenantID, reviewport.ExactV1(fixture.Claim.Trace.ID, fixture.Claim.Trace.Revision, fixture.Claim.Trace.Digest)); err != nil {
		return err
	}
	return nil
}
