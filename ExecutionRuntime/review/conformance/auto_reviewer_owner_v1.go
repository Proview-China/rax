package conformance

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Proview-China/rax/ExecutionRuntime/review/autoreviewer"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// AutoReviewerOwnerScenarioV1 names the fault profiles a reusable Owner
// fixture must provide. Every subject must contain the real autoreviewer.Owner;
// a fixture may replace persistence and invocation boundaries, but it may not
// replace the Owner state machine under test.
type AutoReviewerOwnerScenarioV1 string

const (
	AutoReviewerOwnerHappyV1                AutoReviewerOwnerScenarioV1 = "happy"
	AutoReviewerOwnerUnknownRecoveredV1     AutoReviewerOwnerScenarioV1 = "unknown_recovered"
	AutoReviewerOwnerUnknownPersistentV1    AutoReviewerOwnerScenarioV1 = "unknown_persistent"
	AutoReviewerOwnerBeginLostReplyV1       AutoReviewerOwnerScenarioV1 = "begin_lost_reply"
	AutoReviewerOwnerMarkPrecommitUnknownV1 AutoReviewerOwnerScenarioV1 = "mark_precommit_unknown"
	AutoReviewerOwnerMarkLostReplyV1        AutoReviewerOwnerScenarioV1 = "mark_lost_reply"
	AutoReviewerOwnerRecordLostReplyV1      AutoReviewerOwnerScenarioV1 = "record_lost_reply"
	AutoReviewerOwnerKnownFailureV1         AutoReviewerOwnerScenarioV1 = "known_failure"
	AutoReviewerOwnerBudgetExceededV1       AutoReviewerOwnerScenarioV1 = "budget_exceeded"
	AutoReviewerOwnerTTLCrossingV1          AutoReviewerOwnerScenarioV1 = "ttl_crossing"
	AutoReviewerOwnerClockRollbackV1        AutoReviewerOwnerScenarioV1 = "clock_rollback"
	AutoReviewerOwnerS2DriftV1              AutoReviewerOwnerScenarioV1 = "s2_drift"
	AutoReviewerOwnerConcurrent64V1         AutoReviewerOwnerScenarioV1 = "concurrent_64"
)

// AutoReviewerOwnerStoreV1 is the exact Review-owned persistence surface used
// by autoreviewer.Owner. It is exported here only so backend conformance
// fixtures can supply typed-nil and fault-wrapped dependencies.
type AutoReviewerOwnerStoreV1 interface {
	reviewport.AutoReviewerStoreV1
	reviewport.RubricCurrentReaderV1
	InspectDomainResultExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.ReviewerInvocationResultFactV1, error)
}

type AutoReviewerOwnerStatsV1 struct {
	BeginCalls    int32
	MarkCalls     int32
	RecordCalls   int32
	StartCalls    int32
	InspectCalls  int32
	ProviderCalls int32
	InspectRefs   []contract.ExactResourceRefV1
}

type AutoReviewerOwnerSubjectV1 struct {
	Owner              *autoreviewer.Owner
	Store              AutoReviewerOwnerStoreV1
	Command            autoreviewer.RunCommandV1
	RunContext         context.Context
	OriginalInvocation contract.ExactResourceRefV1
	ExpectedResult     contract.ExactResourceRefV1
	Stats              func() AutoReviewerOwnerStatsV1
}

type AutoReviewerOwnerConstructorFixtureV1 struct {
	ValidStore         AutoReviewerOwnerStoreV1
	ValidInvocation    reviewport.AutoReviewerInvocationPortV1
	TypedNilStore      AutoReviewerOwnerStoreV1
	TypedNilInvocation reviewport.AutoReviewerInvocationPortV1
	Clock              autoreviewer.Clock
}

type AutoReviewerOwnerFixtureFactoryV1 interface {
	NewAutoReviewerOwnerScenarioV1(context.Context, AutoReviewerOwnerScenarioV1) (AutoReviewerOwnerSubjectV1, error)
	NewAutoReviewerOwnerConstructorFixtureV1(context.Context) (AutoReviewerOwnerConstructorFixtureV1, error)
}

// CheckAutoReviewerOwnerV1 verifies the Review Owner coordination semantics,
// not an external Provider. Invocation fixtures may report an Observation or
// unknown outcome, but Inspect is required to remain read-only.
func CheckAutoReviewerOwnerV1(ctx context.Context, factory AutoReviewerOwnerFixtureFactoryV1) error {
	if factory == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Auto Reviewer Owner conformance factory is missing")
	}
	if err := checkAutoReviewerOwnerConstructorV1(ctx, factory); err != nil {
		return err
	}

	checks := []struct {
		name  AutoReviewerOwnerScenarioV1
		check func(context.Context, AutoReviewerOwnerSubjectV1) error
	}{
		{AutoReviewerOwnerHappyV1, checkAutoReviewerOwnerHappyV1},
		{AutoReviewerOwnerUnknownRecoveredV1, checkAutoReviewerOwnerUnknownRecoveredV1},
		{AutoReviewerOwnerUnknownPersistentV1, checkAutoReviewerOwnerUnknownPersistentV1},
		{AutoReviewerOwnerBeginLostReplyV1, checkAutoReviewerOwnerBeginLostReplyV1},
		{AutoReviewerOwnerMarkPrecommitUnknownV1, checkAutoReviewerOwnerMarkPrecommitUnknownV1},
		{AutoReviewerOwnerMarkLostReplyV1, checkAutoReviewerOwnerMarkLostReplyV1},
		{AutoReviewerOwnerRecordLostReplyV1, checkAutoReviewerOwnerRecordLostReplyV1},
		{AutoReviewerOwnerKnownFailureV1, checkAutoReviewerOwnerKnownFailureV1},
		{AutoReviewerOwnerBudgetExceededV1, checkAutoReviewerOwnerBudgetExceededV1},
		{AutoReviewerOwnerTTLCrossingV1, checkAutoReviewerOwnerTTLCrossingV1},
		{AutoReviewerOwnerClockRollbackV1, checkAutoReviewerOwnerClockRollbackV1},
		{AutoReviewerOwnerS2DriftV1, checkAutoReviewerOwnerS2DriftV1},
		{AutoReviewerOwnerConcurrent64V1, checkAutoReviewerOwnerConcurrent64V1},
	}
	for _, item := range checks {
		subject, err := factory.NewAutoReviewerOwnerScenarioV1(ctx, item.name)
		if err != nil {
			return fmt.Errorf("auto reviewer owner %s fixture: %w", item.name, err)
		}
		if err := validateAutoReviewerOwnerSubjectV1(subject); err != nil {
			return fmt.Errorf("auto reviewer owner %s fixture: %w", item.name, err)
		}
		if err := item.check(ctx, subject); err != nil {
			return fmt.Errorf("auto reviewer owner %s: %w", item.name, err)
		}
	}
	return nil
}

func checkAutoReviewerOwnerConstructorV1(ctx context.Context, factory AutoReviewerOwnerFixtureFactoryV1) error {
	fixture, err := factory.NewAutoReviewerOwnerConstructorFixtureV1(ctx)
	if err != nil {
		return fmt.Errorf("auto reviewer owner constructor fixture: %w", err)
	}
	if fixture.ValidStore == nil || fixture.ValidInvocation == nil || fixture.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Auto Reviewer Owner constructor fixture lacks valid dependencies")
	}
	// A plain nil interface would only exercise the trivial nil path. Require
	// the fixture to carry non-nil interfaces whose concrete pointer is nil.
	if fixture.TypedNilStore == nil || fixture.TypedNilInvocation == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Auto Reviewer Owner constructor fixture lacks typed-nil dependencies")
	}
	if _, err := autoreviewer.New(fixture.TypedNilStore, fixture.ValidInvocation, fixture.Clock); !core.HasCategory(err, core.ErrorInvalidArgument) {
		return fmt.Errorf("typed-nil Store was accepted: %w", err)
	}
	if _, err := autoreviewer.New(fixture.ValidStore, fixture.TypedNilInvocation, fixture.Clock); !core.HasCategory(err, core.ErrorInvalidArgument) {
		return fmt.Errorf("typed-nil invocation Port was accepted: %w", err)
	}
	if _, err := autoreviewer.New(fixture.ValidStore, fixture.ValidInvocation, nil); !core.HasCategory(err, core.ErrorInvalidArgument) {
		return fmt.Errorf("nil clock was accepted: %w", err)
	}
	return nil
}

func validateAutoReviewerOwnerSubjectV1(subject AutoReviewerOwnerSubjectV1) error {
	if subject.Owner == nil || subject.Store == nil || subject.Stats == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Auto Reviewer Owner conformance subject is incomplete")
	}
	if subject.RunContext == nil {
		subject.RunContext = context.Background()
	}
	if err := subject.Command.Attempt.Validate(); err != nil {
		return err
	}
	if subject.OriginalInvocation != subject.Command.Attempt.ExactRef() {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Auto Reviewer Owner fixture changed the original invocation ref")
	}
	return subject.ExpectedResult.Validate()
}

func runContextV1(subject AutoReviewerOwnerSubjectV1) context.Context {
	if subject.RunContext != nil {
		return subject.RunContext
	}
	return context.Background()
}

func checkAutoReviewerOwnerHappyV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	first, err := subject.Owner.RunV1(runContextV1(subject), subject.Command)
	if err != nil || first.Attempt.State != contract.AutoReviewerAttemptObservedV1 || first.Observation == nil || first.DomainResult == nil {
		return fmt.Errorf("happy path did not publish the observed closure: %w", err)
	}
	replay, err := subject.Owner.RunV1(context.Background(), subject.Command)
	if err != nil || replay.Attempt.ExactRef() != first.Attempt.ExactRef() || replay.DomainResult == nil || replay.DomainResult.ExactRef() != first.DomainResult.ExactRef() {
		return fmt.Errorf("canonical replay did not exact Inspect the stored closure: %w", err)
	}
	changed := subject.Command
	changed.ResultID += "-drift"
	if _, err := subject.Owner.RunV1(context.Background(), changed); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		return fmt.Errorf("changed DomainResult identity did not conflict: %w", err)
	}
	stats := subject.Stats()
	if stats.StartCalls != 1 || stats.InspectCalls != 0 || stats.ProviderCalls != 1 {
		return fmt.Errorf("happy path invocation counts drifted: %+v", stats)
	}
	return nil
}

func checkAutoReviewerOwnerUnknownRecoveredV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	result, err := subject.Owner.RunV1(runContextV1(subject), subject.Command)
	stats := subject.Stats()
	if err != nil || result.Attempt.State != contract.AutoReviewerAttemptObservedV1 || result.Attempt.InvocationAttempt == nil || *result.Attempt.InvocationAttempt != subject.OriginalInvocation || result.Observation == nil || result.Observation.AttemptRevision != subject.OriginalInvocation.Revision || result.Observation.AttemptDigest != subject.OriginalInvocation.Digest || stats.StartCalls != 1 || stats.InspectCalls != 1 || stats.ProviderCalls != 1 || len(stats.InspectRefs) != 1 || stats.InspectRefs[0] != subject.OriginalInvocation {
		return fmt.Errorf("unknown outcome did not recover by detached exact Inspect: result=%+v stats=%+v err=%w", result, stats, err)
	}
	return nil
}

func checkAutoReviewerOwnerUnknownPersistentV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	first, err := subject.Owner.RunV1(runContextV1(subject), subject.Command)
	if !core.HasCategory(err, core.ErrorIndeterminate) || first.Attempt.State != contract.AutoReviewerAttemptWaitingInspectV1 {
		return fmt.Errorf("first unknown did not persist waiting_inspect: result=%+v err=%w", first, err)
	}
	second, err := subject.Owner.RunV1(context.Background(), subject.Command)
	stats := subject.Stats()
	if !core.HasCategory(err, core.ErrorIndeterminate) || second.Attempt.State != contract.AutoReviewerAttemptWaitingInspectV1 || stats.StartCalls != 1 || stats.InspectCalls != 2 || stats.ProviderCalls != 1 || len(stats.InspectRefs) != 2 || stats.InspectRefs[0] != subject.OriginalInvocation || stats.InspectRefs[1] != subject.OriginalInvocation {
		return fmt.Errorf("waiting_inspect restarted or changed the original attempt: result=%+v stats=%+v err=%w", second, stats, err)
	}
	return nil
}

func checkAutoReviewerOwnerBeginLostReplyV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	result, err := subject.Owner.RunV1(runContextV1(subject), subject.Command)
	stats := subject.Stats()
	if err != nil || result.Attempt.State != contract.AutoReviewerAttemptObservedV1 || stats.BeginCalls != 1 || stats.ProviderCalls != 1 {
		return fmt.Errorf("Begin lost reply was replayed or not recovered: result=%+v stats=%+v err=%w", result, stats, err)
	}
	return nil
}

func checkAutoReviewerOwnerMarkPrecommitUnknownV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	first, err := subject.Owner.RunV1(runContextV1(subject), subject.Command)
	stats := subject.Stats()
	if !core.HasCategory(err, core.ErrorIndeterminate) || first.Attempt.State != contract.AutoReviewerAttemptPreparedV1 || stats.MarkCalls != 1 || stats.StartCalls != 0 || stats.InspectCalls != 0 || stats.ProviderCalls != 0 {
		return fmt.Errorf("pre-commit start-claim uncertainty crossed the invocation boundary: result=%+v stats=%+v err=%w", first, stats, err)
	}
	second, err := subject.Owner.RunV1(context.Background(), subject.Command)
	stats = subject.Stats()
	if err != nil || second.Attempt.State != contract.AutoReviewerAttemptObservedV1 || second.DomainResult == nil || stats.MarkCalls != 2 || stats.StartCalls != 1 || stats.ProviderCalls != 1 {
		return fmt.Errorf("safe start-claim retry did not require the unique applied receipt: result=%+v stats=%+v err=%w", second, stats, err)
	}
	return nil
}

func checkAutoReviewerOwnerMarkLostReplyV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	result, err := subject.Owner.RunV1(runContextV1(subject), subject.Command)
	stats := subject.Stats()
	if !core.HasCategory(err, core.ErrorNotFound) || result.Attempt.State != contract.AutoReviewerAttemptWaitingInspectV1 || stats.MarkCalls != 1 || stats.StartCalls != 0 || stats.InspectCalls != 1 || stats.ProviderCalls != 0 {
		return fmt.Errorf("lost start-claim reply granted invocation authority or lost the inspect-only fence: result=%+v stats=%+v err=%w", result, stats, err)
	}
	return nil
}

func checkAutoReviewerOwnerRecordLostReplyV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	result, err := subject.Owner.RunV1(runContextV1(subject), subject.Command)
	stats := subject.Stats()
	if err != nil || result.Attempt.State != contract.AutoReviewerAttemptObservedV1 || result.DomainResult == nil || stats.RecordCalls != 1 || stats.ProviderCalls != 1 {
		return fmt.Errorf("Record lost reply was replayed or not recovered: result=%+v stats=%+v err=%w", result, stats, err)
	}
	return nil
}

func checkAutoReviewerOwnerKnownFailureV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	first, err := subject.Owner.RunV1(runContextV1(subject), subject.Command)
	if !core.HasCategory(err, core.ErrorForbidden) || first.Attempt.State != contract.AutoReviewerAttemptFailedClosedV1 {
		return fmt.Errorf("known invocation failure did not fail closed: result=%+v err=%w", first, err)
	}
	replay, replayErr := subject.Owner.RunV1(context.Background(), subject.Command)
	stats := subject.Stats()
	if replayErr != nil || replay.Attempt.ExactRef() != first.Attempt.ExactRef() || stats.StartCalls != 1 || stats.InspectCalls != 0 || stats.ProviderCalls != 0 {
		return fmt.Errorf("known failure replay re-entered invocation: result=%+v stats=%+v err=%w", replay, stats, replayErr)
	}
	return nil
}

func checkAutoReviewerOwnerBudgetExceededV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	result, err := subject.Owner.RunV1(runContextV1(subject), subject.Command)
	if !core.HasReason(err, core.ReasonBudgetBindingStale) || result.Attempt.State != contract.AutoReviewerAttemptEscalatedV1 {
		return fmt.Errorf("budget overrun did not escalate: result=%+v err=%w", result, err)
	}
	return checkAutoReviewerOwnerNoResultV1(subject)
}

func checkAutoReviewerOwnerTTLCrossingV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	if _, err := subject.Owner.RunV1(runContextV1(subject), subject.Command); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		return fmt.Errorf("TTL crossing did not fail closed: %w", err)
	}
	return checkAutoReviewerOwnerNoResultV1(subject)
}

func checkAutoReviewerOwnerClockRollbackV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	if _, err := subject.Owner.RunV1(runContextV1(subject), subject.Command); !core.HasReason(err, core.ReasonClockRegression) {
		return fmt.Errorf("clock rollback did not fail closed: %w", err)
	}
	return checkAutoReviewerOwnerNoResultV1(subject)
}

func checkAutoReviewerOwnerS2DriftV1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	result, err := subject.Owner.RunV1(runContextV1(subject), subject.Command)
	if !core.HasCategory(err, core.ErrorConflict) || result.Attempt.State != contract.AutoReviewerAttemptWaitingInspectV1 {
		return fmt.Errorf("S2 drift did not fail before Review result: result=%+v err=%w", result, err)
	}
	return checkAutoReviewerOwnerNoResultV1(subject)
}

func checkAutoReviewerOwnerConcurrent64V1(_ context.Context, subject AutoReviewerOwnerSubjectV1) error {
	var successes atomic.Int32
	var waiting atomic.Int32
	errorsSeen := make(chan error, 64)
	var group sync.WaitGroup
	for index := 0; index < 64; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := subject.Owner.RunV1(context.Background(), subject.Command)
			if err == nil && result.Attempt.State == contract.AutoReviewerAttemptObservedV1 {
				successes.Add(1)
				return
			}
			if core.HasCategory(err, core.ErrorNotFound) && result.Attempt.State == contract.AutoReviewerAttemptWaitingInspectV1 {
				waiting.Add(1)
				return
			}
			errorsSeen <- fmt.Errorf("state=%s: %w", result.Attempt.State, err)
		}()
	}
	group.Wait()
	close(errorsSeen)
	current, err := subject.Store.InspectAutoReviewerAttemptCurrentV1(context.Background(), subject.Command.Attempt.TenantID, subject.Command.Attempt.ID)
	stats := subject.Stats()
	if err == nil && current.State == contract.AutoReviewerAttemptObservedV1 && successes.Load()+waiting.Load() == 64 && stats.StartCalls == 1 && stats.ProviderCalls == 1 {
		for index := 0; index < 64; index++ {
			replay, replayErr := subject.Owner.RunV1(context.Background(), subject.Command)
			if replayErr != nil || replay.Attempt.State != contract.AutoReviewerAttemptObservedV1 || replay.DomainResult == nil {
				return fmt.Errorf("post-closure replay %d did not converge by Review exact Inspect: result=%+v err=%w", index, replay, replayErr)
			}
		}
		stats = subject.Stats()
		if stats.StartCalls == 1 && stats.ProviderCalls == 1 {
			return nil
		}
	}
	var first error
	for seen := range errorsSeen {
		if first == nil {
			first = seen
		}
	}
	return fmt.Errorf("64 callers violated the single-start fence: current=%s successes=%d waiting=%d stats=%+v inspect=%w first=%v", current.State, successes.Load(), waiting.Load(), stats, err, first)
}

func checkAutoReviewerOwnerNoResultV1(subject AutoReviewerOwnerSubjectV1) error {
	if _, err := subject.Store.InspectDomainResultExactV1(context.Background(), subject.Command.Attempt.TenantID, reviewport.ExactV1(subject.ExpectedResult.ID, subject.ExpectedResult.Revision, subject.ExpectedResult.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		return fmt.Errorf("failed path leaked DomainResult: %w", err)
	}
	return nil
}
