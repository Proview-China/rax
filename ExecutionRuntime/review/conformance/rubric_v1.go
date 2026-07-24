package conformance

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type RubricStoreFixtureV1 struct {
	Now       time.Time
	Create    reviewport.PublishRubricMutationV1
	Supersede reviewport.PublishRubricMutationV1
	Revoke    reviewport.RevokeRubricMutationV1
}

// CheckRubricStoreV1 is reusable across memory and durable Review stores.
// It verifies canonical replay, append-only exact history, full-ref current
// CAS, supersede, terminal revoke, and no resurrection through stale refs.
func CheckRubricStoreV1(ctx context.Context, store reviewport.RubricStoreV1, f RubricStoreFixtureV1) error {
	created, err := store.PublishRubricV1(ctx, f.Create)
	if err != nil {
		return err
	}
	replayed, err := store.PublishRubricV1(ctx, f.Create)
	if err != nil || replayed.Digest != created.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "rubric create replay drifted")
	}
	if _, err := store.InspectRubricCurrentV1(ctx, created.TenantID, created.ExactRef(), f.Now); err != nil {
		return err
	}
	next, err := store.PublishRubricV1(ctx, f.Supersede)
	if err != nil {
		return err
	}
	if _, err := store.InspectRubricExactV1(ctx, created.TenantID, created.ExactRef()); err != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "rubric supersede hid historical revision")
	}
	if _, err := store.InspectRubricCurrentV1(ctx, next.TenantID, next.ExactRef(), f.Now); err != nil {
		return err
	}
	if _, err := store.InspectRubricCurrentV1(ctx, created.TenantID, created.ExactRef(), f.Now); !core.HasCategory(err, core.ErrorConflict) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric stale current ref was accepted")
	}
	revoked, err := store.RevokeRubricV1(ctx, f.Revoke)
	if err != nil {
		return err
	}
	if _, err := store.InspectRubricExactV1(ctx, revoked.TenantID, revoked.ExactRef()); err != nil {
		return err
	}
	if _, err := store.InspectRubricCurrentV1(ctx, revoked.TenantID, revoked.ExactRef(), f.Now); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "revoked rubric remained current-active")
	}
	return nil
}

var _ contract.RubricDefinitionV1
