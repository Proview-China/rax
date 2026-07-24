package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type EvidenceAttachmentFixtureV1 struct {
	Attachment      contract.EvidenceAttachmentV1
	Conflict        contract.EvidenceAttachmentV1
	CheckedUnixNano int64
}

// CheckEvidenceAttachmentStoreV1 is reusable by memory and durable Review
// Owner stores. The fixture's exact Case and Target must already be current.
func CheckEvidenceAttachmentStoreV1(ctx context.Context, store reviewport.EvidenceAttachmentStoreV1, fixture EvidenceAttachmentFixtureV1) error {
	if store == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review Evidence Attachment store is missing")
	}
	mutation := reviewport.CreateEvidenceAttachmentMutationV1{Attachment: fixture.Attachment, CheckedUnixNano: fixture.CheckedUnixNano}
	first, err := store.CreateEvidenceAttachmentV1(ctx, mutation)
	if err != nil {
		return err
	}
	replay, err := store.CreateEvidenceAttachmentV1(ctx, mutation)
	if err != nil || first.Digest != replay.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Evidence Attachment canonical replay drifted")
	}
	exact, err := store.InspectEvidenceAttachmentExactV1(ctx, first.TenantID, reviewport.ExactV1(first.ID, first.Revision, first.Digest))
	if err != nil || exact.Digest != first.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence Attachment exact Inspect drifted")
	}
	byKey, err := store.InspectEvidenceAttachmentByIdempotencyV1(ctx, first.TenantID, first.IdempotencyKey)
	if err != nil || byKey.Digest != first.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Evidence Attachment idempotency Inspect drifted")
	}
	if len(exact.Evidence) == 0 {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceUnavailable, "Evidence Attachment exact Inspect lost evidence refs")
	}
	exact.Evidence[0].Ref = "client-alias-mutation"
	again, err := store.InspectEvidenceAttachmentExactV1(ctx, first.TenantID, reviewport.ExactV1(first.ID, first.Revision, first.Digest))
	if err != nil || len(again.Evidence) == 0 || again.Evidence[0].Ref != first.Evidence[0].Ref {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence Attachment store exposed a mutable alias")
	}
	if fixture.Conflict.ID != "" {
		if _, err := store.CreateEvidenceAttachmentV1(ctx, reviewport.CreateEvidenceAttachmentMutationV1{Attachment: fixture.Conflict, CheckedUnixNano: fixture.CheckedUnixNano}); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Evidence Attachment changed idempotency payload was accepted")
		}
	}
	return nil
}
