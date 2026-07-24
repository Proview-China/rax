package kernel

import (
	"context"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func PrepareContextCompactionV1(ctx context.Context, plan contract.ContextCompactionPlanV1, summary contract.ContextCompactionSummaryV1) (contract.ContextCompactionPreparedV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	if err := plan.Validate(); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	if err := summary.Validate(); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	summaryDigest, err := summary.DigestValue()
	if err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	summaryRef := contract.FactRef{ID: summary.ID, Revision: summary.Revision, Digest: summaryDigest}
	if summaryRef != plan.SummaryRef || summary.SourceGenerationRef != plan.ExpectedCurrent.GenerationRef {
		return contract.ContextCompactionPreparedV1{}, fmt.Errorf("%w: compaction exact source binding", contract.ErrConflict)
	}
	if plan.CheckedUnixNano < summary.CreatedUnixNano || plan.CheckedUnixNano >= summary.ExpiresUnixNano || plan.ExpiresUnixNano > summary.ExpiresUnixNano {
		return contract.ContextCompactionPreparedV1{}, fmt.Errorf("%w: compaction summary currentness", contract.ErrExpired)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	parent := plan.ExpectedCurrent.GenerationRef
	generation := contract.ContextGeneration{
		ContractVersion: contract.Version,
		ID:              plan.TargetGenerationID,
		Revision:        1,
		Ordinal:         plan.ExpectedCurrent.GenerationOrdinal + 1,
		Parent:          &parent,
		RootFrame:       plan.TargetRootFrameRef,
		Summary:         &summary.Summary,
		RetainedAnchors: append([]contract.FactRef(nil), summary.RetainedAnchorRefs...),
		OpenEffects:     append([]contract.FactRef(nil), summary.OpenEffectRefs...),
		CreatedUnixNano: plan.CheckedUnixNano,
	}
	if err := generation.Validate(); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	generationDigest, err := contract.DigestJSON(generation)
	if err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	planRef := contract.FactRef{ID: plan.AttemptID, Revision: plan.Revision, Digest: plan.Digest}
	prepared := contract.ContextCompactionPreparedV1{
		PlanRef:             planRef,
		SummaryRef:          summaryRef,
		Generation:          generation,
		GenerationRef:       contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: generationDigest},
		OutstandingWorkRefs: append([]contract.FactRef(nil), summary.OutstandingWorkRefs...),
		UncompressibleRefs:  append([]contract.FactRef(nil), summary.UncompressibleRefs...),
		PreparedUnixNano:    plan.CheckedUnixNano,
		ExpiresUnixNano:     plan.ExpiresUnixNano,
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	return contract.SealContextCompactionPreparedV1(prepared)
}
