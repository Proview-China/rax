package conformance

import (
	"context"
	"fmt"

	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
)

type AutoReviewerStoreFixtureV1 struct {
	Begin         reviewport.BeginAutoReviewerAttemptMutationV1
	DirectObserve reviewport.RecordAutoReviewerObservationMutationV1
	Claim         reviewport.MarkAutoReviewerWaitingInspectMutationV1
	Observe       reviewport.RecordAutoReviewerObservationMutationV1
}

// CheckAutoReviewerStoreV1 is reusable by every Review-owned backend. It
// checks create-once, atomic Observation+DomainResult, historical exact reads,
// lost-reply canonical replay, and deep-clone behavior.
func CheckAutoReviewerStoreV1(ctx context.Context, store reviewport.AutoReviewerStoreV1, fixture AutoReviewerStoreFixtureV1) error {
	created, err := store.BeginAutoReviewerAttemptV1(ctx, fixture.Begin)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if replay, err := store.BeginAutoReviewerAttemptV1(ctx, fixture.Begin); err != nil || replay.Digest != created.Digest {
		return fmt.Errorf("begin replay: %w", err)
	}
	if fixture.DirectObserve.Expected != created.ExactRef() {
		return fmt.Errorf("fixture DirectObserve does not expect created Attempt")
	}
	if _, _, err := store.RecordAutoReviewerObservationV1(ctx, fixture.DirectObserve); err == nil {
		return fmt.Errorf("prepared Attempt bypassed the persistent start claim")
	}
	if current, err := store.InspectAutoReviewerAttemptCurrentV1(ctx, created.TenantID, created.ID); err != nil || current.ExactRef() != created.ExactRef() {
		return fmt.Errorf("rejected prepared-to-observed transition changed current: %w", err)
	}
	if _, err := store.InspectAutoReviewerObservationExactV1(ctx, created.TenantID, fixture.DirectObserve.Observation.Ref()); err == nil {
		return fmt.Errorf("rejected prepared-to-observed transition leaked Observation")
	}
	if fixture.Claim.Expected != created.ExactRef() {
		return fmt.Errorf("fixture Claim does not expect created Attempt")
	}
	claim, err := store.MarkAutoReviewerWaitingInspectV1(ctx, fixture.Claim)
	if err != nil || !claim.Applied || claim.Attempt.ExactRef() != fixture.Claim.Next.ExactRef() {
		return fmt.Errorf("persistent start claim: receipt=%+v err=%w", claim, err)
	}
	if replay, err := store.MarkAutoReviewerWaitingInspectV1(ctx, fixture.Claim); err != nil || replay.Applied || replay.Attempt.ExactRef() != claim.Attempt.ExactRef() {
		return fmt.Errorf("persistent start claim replay: receipt=%+v err=%w", replay, err)
	}
	if fixture.Observe.Expected != claim.Attempt.ExactRef() {
		return fmt.Errorf("fixture Observe does not expect waiting_inspect Attempt")
	}
	observed, result, err := store.RecordAutoReviewerObservationV1(ctx, fixture.Observe)
	if err != nil {
		return fmt.Errorf("record Observation: %w", err)
	}
	if replay, replayResult, err := store.RecordAutoReviewerObservationV1(ctx, fixture.Observe); err != nil || replay.Digest != observed.Digest || replayResult.Digest != result.Digest {
		return fmt.Errorf("record Observation replay: %w", err)
	}
	if historical, err := store.InspectAutoReviewerAttemptExactV1(ctx, created.TenantID, created.ExactRef()); err != nil || historical.Digest != created.Digest {
		return fmt.Errorf("historical exact Attempt: %w", err)
	}
	copyValue, err := store.InspectAutoReviewerObservationExactV1(ctx, observed.TenantID, fixture.Observe.Observation.Ref())
	if err != nil {
		return fmt.Errorf("exact Observation: %w", err)
	}
	copyValue.Output.ReasonCodes[0] = "conformance/mutated"
	again, err := store.InspectAutoReviewerObservationExactV1(ctx, observed.TenantID, fixture.Observe.Observation.Ref())
	if err != nil || again.Output.ReasonCodes[0] == "conformance/mutated" {
		return fmt.Errorf("Observation deep clone: %w", err)
	}
	return nil
}
