package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type BypassStoreFixtureV1 struct {
	Create reviewport.CreateBypassDecisionMutationV1
	Next   reviewport.BypassDecisionCASMutationV1
}

// CheckBypassStoreV1 checks canonical replay, exact historical inspection,
// current-by-exact-Case and append-only CAS behavior without assuming a backend.
func CheckBypassStoreV1(ctx context.Context, store reviewport.BypassStoreV1, f BypassStoreFixtureV1) error {
	first, err := store.CreateBypassDecisionV1(ctx, f.Create)
	if err != nil {
		return err
	}
	replay, err := store.CreateBypassDecisionV1(ctx, f.Create)
	if err != nil {
		return err
	}
	if first.Digest != replay.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "bypass create replay drifted")
	}
	if _, err := store.InspectBypassDecisionExactV1(ctx, first.ExactRef()); err != nil {
		return err
	}
	current, err := store.InspectCurrentBypassDecisionByCaseV1(ctx, first.Case)
	if err != nil || current.Digest != first.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "bypass current Case index did not return exact Decision")
	}
	next, err := store.CompareAndSwapBypassDecisionV1(ctx, f.Next)
	if err != nil {
		return err
	}
	again, err := store.CompareAndSwapBypassDecisionV1(ctx, f.Next)
	if err != nil {
		return err
	}
	if next.Digest != again.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "bypass CAS replay drifted")
	}
	if _, err := store.InspectBypassDecisionExactV1(ctx, first.ExactRef()); err != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "bypass CAS hid historical Decision")
	}
	if _, err := store.InspectBypassDecisionExactV1(ctx, next.ExactRef()); err != nil {
		return err
	}
	current, err = store.InspectCurrentBypassDecisionByCaseV1(ctx, next.Case)
	if err != nil || current.Digest != next.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "bypass CAS did not advance the exact Case index")
	}
	return nil
}

var _ contract.BypassDecisionV1
