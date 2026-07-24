package conformance

import (
	"context"

	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// TraceEventStoreFixtureV2 is a pre-admitted exact Finding mutation. Its
// Target, Case and Round closure must already exist in the Store under test.
type TraceEventStoreFixtureV2 struct {
	Mutation reviewport.CreateFindingWithTraceMutationV2
}

// CheckTraceEventStoreV2 is the reusable Review Owner event-store contract.
// It checks canonical replay, exact/deep-cloned inspection, narrow paging and
// fail-closed cursor drift without granting Timeline or Evidence authority.
func CheckTraceEventStoreV2(ctx context.Context, store reviewport.TraceEventStoreV2, fixture TraceEventStoreFixtureV2) error {
	first, err := store.CreateFindingWithTraceV2(ctx, fixture.Mutation)
	if err != nil {
		return err
	}
	replayed, err := store.CreateFindingWithTraceV2(ctx, fixture.Mutation)
	if err != nil {
		return err
	}
	if first.Digest != fixture.Mutation.Finding.Digest || replayed.Digest != first.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Trace V2 canonical replay changed Finding")
	}
	trace := fixture.Mutation.Trace
	inspected, err := store.InspectTraceExactV1(ctx, trace.TenantID, reviewport.ExactV1(trace.ID, trace.Revision, trace.Digest))
	if err != nil || inspected.Digest != trace.Digest {
		if err != nil {
			return err
		}
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Trace V2 exact Inspect changed Trace")
	}
	if len(inspected.FactRefs) != 0 {
		inspected.FactRefs[0] = "conformance-mutated"
		again, inspectErr := store.InspectTraceExactV1(ctx, trace.TenantID, reviewport.ExactV1(trace.ID, trace.Revision, trace.Digest))
		if inspectErr != nil {
			return inspectErr
		}
		if len(again.FactRefs) == 0 || again.FactRefs[0] == "conformance-mutated" {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Trace V2 exact Inspect exposed a mutable alias")
		}
	}
	page, err := store.ListTracePageV2(ctx, reviewport.ListTracePageRequestV2{TenantID: trace.TenantID, CaseID: trace.CaseID, Limit: reviewport.MaxTracePageV2})
	if err != nil {
		return err
	}
	found := false
	for _, event := range page.Events {
		found = found || event.Digest == trace.Digest
	}
	if !found {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Trace V2 page omitted exact committed Trace")
	}
	drift := reviewport.TracePageAfterV2{SourceID: trace.SourceID, SourceEpoch: trace.SourceEpoch, SourceSequence: trace.SourceSequence, Trace: reviewport.ExactV1(trace.ID, trace.Revision, core.DigestBytes([]byte("trace-v2-cursor-drift")))}
	if _, err = store.ListTracePageV2(ctx, reviewport.ListTracePageRequestV2{TenantID: trace.TenantID, CaseID: trace.CaseID, After: &drift, Limit: 1}); !core.HasCategory(err, core.ErrorConflict) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceCursorInvalid, "Trace V2 drifted exact cursor did not fail closed")
	}
	return nil
}
