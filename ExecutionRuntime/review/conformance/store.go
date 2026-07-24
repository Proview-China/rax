// Package conformance contains reusable Review Store contract checks.
package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type StoreFixtureV1 struct {
	Target    contract.TargetSnapshotV1
	Case      contract.ReviewCaseV1
	Trace     contract.TraceFactV1
	Next      contract.ReviewCaseV1
	NextTrace contract.TraceFactV1
}

func CheckStoreV1(ctx context.Context, store reviewport.StoreV1, fixture StoreFixtureV1) error {
	mutation := reviewport.CreateTargetCaseMutationV1{Target: fixture.Target, Case: fixture.Case, Trace: fixture.Trace}
	first, err := store.CreateTargetCaseV1(ctx, mutation)
	if err != nil {
		return err
	}
	replayed, err := store.CreateTargetCaseV1(ctx, mutation)
	if err != nil {
		return err
	}
	if first.Digest != replayed.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "create-once replay changed case")
	}
	inspected, err := store.InspectCaseV1(ctx, fixture.Case.TenantID, fixture.Case.ID)
	if err != nil {
		return err
	}
	if inspected.Digest != fixture.Case.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Inspect did not return exact case")
	}
	if _, err := store.InspectTargetExactV1(ctx, fixture.Target.TenantID, reviewport.ExactV1(fixture.Target.ID, fixture.Target.Revision, fixture.Target.Digest)); err != nil {
		return err
	}
	if fixture.Trace.ID != "" {
		if _, err := store.InspectTraceExactV1(ctx, fixture.Trace.TenantID, reviewport.ExactV1(fixture.Trace.ID, fixture.Trace.Revision, fixture.Trace.Digest)); err != nil {
			return err
		}
	}
	transitions, ok := store.(reviewport.CaseTransitionStoreV2)
	if !ok {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Store lacks compound Case transition capability")
	}
	transition := reviewport.TransitionCaseWithTraceMutationV2{Expected: reviewport.ExpectedV1(fixture.Case.Revision, fixture.Case.Digest), Next: fixture.Next, Trace: fixture.NextTrace}
	updated, err := transitions.TransitionCaseWithTraceV2(ctx, transition)
	if err != nil {
		return err
	}
	if updated.Digest != fixture.Next.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "compound transition did not return exact next Case")
	}
	if _, err := store.InspectCaseExactV1(ctx, fixture.Case.TenantID, reviewport.ExactV1(fixture.Case.ID, fixture.Case.Revision, fixture.Case.Digest)); err != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "compound transition overwrote historical Case revision")
	}
	if _, err := store.InspectCaseExactV1(ctx, fixture.Next.TenantID, reviewport.ExactV1(fixture.Next.ID, fixture.Next.Revision, fixture.Next.Digest)); err != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "compound transition did not publish exact next Case revision")
	}
	if _, err := transitions.InspectTraceExactV1(ctx, fixture.NextTrace.TenantID, reviewport.ExactV1(fixture.NextTrace.ID, fixture.NextTrace.Revision, fixture.NextTrace.Digest)); err != nil {
		return err
	}
	replayed, err = transitions.TransitionCaseWithTraceV2(ctx, transition)
	if err != nil || replayed.Digest != updated.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "compound transition replay changed canonical closure")
	}
	return nil
}
