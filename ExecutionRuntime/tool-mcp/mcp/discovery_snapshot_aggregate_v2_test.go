package mcp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func TestMCPDiscoverySnapshotAggregatorV2SettledTerminalPage(t *testing.T) {
	f := newMCPDiscoverySnapshotFixtureV2(t)
	snapshot, err := f.aggregator.AggregateMCPDiscoverySnapshotV2(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Connection.ID != f.request.Connection.ID || len(snapshot.Tools) != 1 || snapshot.Tools[0].Name != "echo" || len(snapshot.Resources) != 0 || len(snapshot.Prompts) != 0 {
		t.Fatalf("snapshot drifted: %#v", snapshot)
	}
	repository := NewInMemoryMCPCapabilitySnapshotRepositoryV2()
	winner, err := repository.EnsureMCPCapabilitySnapshotV2(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	read, err := repository.InspectMCPCapabilitySnapshotV2(context.Background(), winner.ObjectRef())
	if err != nil || read.Digest != snapshot.Digest {
		t.Fatalf("snapshot exact Inspect=%#v err=%v", read, err)
	}
	read.Tools[0].Name = "mutated"
	again, _ := repository.InspectMCPCapabilitySnapshotV2(context.Background(), winner.ObjectRef())
	if again.Tools[0].Name != "echo" {
		t.Fatal("snapshot repository returned aliased slices")
	}
}

func TestMCPDiscoverySnapshotAggregatorV2FailsClosed(t *testing.T) {
	t.Run("missing_required_page", func(t *testing.T) {
		f := newMCPDiscoverySnapshotFixtureV2(t)
		f.request.AppliedCommands = nil
		if _, err := f.aggregator.AggregateMCPDiscoverySnapshotV2(context.Background(), f.request); !core.HasReason(err, core.ReasonEvidenceUnavailable) {
			t.Fatalf("missing page error=%v", err)
		}
	})
	t.Run("nonterminal_cursor_drift", func(t *testing.T) {
		f := newMCPDiscoverySnapshotFixtureV2(t)
		base := f.aggregator.applied
		f.aggregator.applied = mcpDiscoveryAppliedReaderFuncV2(func(ctx context.Context, exact toolcontract.ObjectRef, ttl time.Duration) (toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1, error) {
			value, err := base.InspectCurrentMCPDiscoveryPageAppliedV1(ctx, exact, ttl)
			if err != nil {
				return value, err
			}
			value.NextCursor = []byte("forged-next")
			return toolcontract.SealMCPDiscoveryPageAppliedCurrentProjectionV1(value, f.now)
		})
		if _, err := f.aggregator.AggregateMCPDiscoverySnapshotV2(context.Background(), f.request); !core.HasReason(err, core.ReasonBindingDrift) {
			t.Fatalf("cursor drift error=%v", err)
		}
	})
	t.Run("wrong_connection_ref", func(t *testing.T) {
		f := newMCPDiscoverySnapshotFixtureV2(t)
		f.request.Connection.Digest = testDigestV1("wrong-connection")
		if _, err := f.aggregator.AggregateMCPDiscoverySnapshotV2(context.Background(), f.request); err == nil {
			t.Fatal("wrong connection Ref was accepted")
		}
	})
	t.Run("clock_rollback", func(t *testing.T) {
		f := newMCPDiscoverySnapshotFixtureV2(t)
		calls := 0
		f.aggregator.clock = func() time.Time {
			calls++
			if calls == 1 {
				return f.now
			}
			return f.now.Add(-time.Nanosecond)
		}
		if _, err := f.aggregator.AggregateMCPDiscoverySnapshotV2(context.Background(), f.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback error=%v", err)
		}
	})
	t.Run("nil_context", func(t *testing.T) {
		f := newMCPDiscoverySnapshotFixtureV2(t)
		if _, err := f.aggregator.AggregateMCPDiscoverySnapshotV2(nil, f.request); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context error=%v", err)
		}
	})
	t.Run("typed_nil_dependency", func(t *testing.T) {
		f := newMCPDiscoverySnapshotFixtureV2(t)
		var commands *InMemoryMCPDiscoveryPageCommandRepositoryV1
		if _, err := NewMCPDiscoverySnapshotAggregatorV2(f.source, f.source, commands, f.apply, f.physical, func() time.Time { return f.now }); err == nil {
			t.Fatal("typed-nil command reader was accepted")
		}
	})
}

func TestMCPDiscoverySnapshotAggregatorV2ConcurrentSingleWinner(t *testing.T) {
	f := newMCPDiscoverySnapshotFixtureV2(t)
	repository := NewInMemoryMCPCapabilitySnapshotRepositoryV2()
	const workers = 64
	var group sync.WaitGroup
	errs := make(chan error, workers)
	refs := make(chan toolcontract.ObjectRef, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			snapshot, err := f.aggregator.AggregateMCPDiscoverySnapshotV2(context.Background(), f.request)
			if err == nil {
				snapshot, err = repository.EnsureMCPCapabilitySnapshotV2(context.Background(), snapshot)
				refs <- snapshot.ObjectRef()
			}
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	close(refs)
	var winner toolcontract.ObjectRef
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for ref := range refs {
		if winner.ID == "" {
			winner = ref
		} else if ref != winner {
			t.Fatalf("multiple snapshot winners: %v != %v", ref, winner)
		}
	}
}

func TestMCPCapabilitySnapshotRepositoryV2SuccessorCASAndHistory(t *testing.T) {
	f := newMCPDiscoverySnapshotFixtureV2(t)
	first, err := f.aggregator.AggregateMCPDiscoverySnapshotV2(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	repository := NewInMemoryMCPCapabilitySnapshotRepositoryV2()
	first, err = repository.EnsureMCPCapabilitySnapshotV2(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	second := toolcontract.CloneMCPCapabilitySnapshotV2(first)
	second.Revision = 2
	second.SourceDigest = testDigestV1("snapshot-successor-source")
	second.CreatedUnixNano = f.now.Add(time.Second).UnixNano()
	second.Digest = ""
	second, err = toolcontract.SealMCPCapabilitySnapshotV2(second)
	if err != nil {
		t.Fatal(err)
	}
	second, err = repository.EnsureMCPCapabilitySnapshotRevisionV2(context.Background(), second, ptrMCPObjectRefV2(first.ObjectRef()))
	if err != nil {
		t.Fatal(err)
	}
	current, err := repository.InspectCurrentMCPCapabilitySnapshotV2(context.Background(), first.ID)
	if err != nil || current.ObjectRef() != second.ObjectRef() {
		t.Fatalf("current=%#v err=%v", current, err)
	}
	historical, err := repository.InspectMCPCapabilitySnapshotV2(context.Background(), first.ObjectRef())
	if err != nil || historical.ObjectRef() != first.ObjectRef() {
		t.Fatalf("history=%#v err=%v", historical, err)
	}
	if _, err = repository.EnsureMCPCapabilitySnapshotV2(context.Background(), first); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("historical rollback error=%v", err)
	}
	if retry, err := repository.EnsureMCPCapabilitySnapshotRevisionV2(context.Background(), second, ptrMCPObjectRefV2(first.ObjectRef())); err != nil || retry.ObjectRef() != second.ObjectRef() {
		t.Fatalf("lost-reply retry=%#v err=%v", retry, err)
	}
	drift := toolcontract.CloneMCPCapabilitySnapshotV2(second)
	drift.SourceDigest = testDigestV1("same-revision-drift")
	drift.Digest = ""
	drift, _ = toolcontract.SealMCPCapabilitySnapshotV2(drift)
	if _, err = repository.EnsureMCPCapabilitySnapshotRevisionV2(context.Background(), drift, ptrMCPObjectRefV2(first.ObjectRef())); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same revision drift error=%v", err)
	}
}

func TestMCPCapabilitySnapshotRepositoryV2TypedNilInspect(t *testing.T) {
	var repository *InMemoryMCPCapabilitySnapshotRepositoryV2
	if _, err := repository.InspectMCPCapabilitySnapshotV2(context.Background(), toolcontract.ObjectRef{}); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil exact Inspect error=%v", err)
	}
	if _, err := repository.InspectCurrentMCPCapabilitySnapshotV2(context.Background(), "snapshot"); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil current Inspect error=%v", err)
	}
}

func TestMCPCapabilitySnapshotRepositoryV2ConcurrentSuccessorSingleWinner(t *testing.T) {
	f := newMCPDiscoverySnapshotFixtureV2(t)
	first, err := f.aggregator.AggregateMCPDiscoverySnapshotV2(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	repository := NewInMemoryMCPCapabilitySnapshotRepositoryV2()
	first, _ = repository.EnsureMCPCapabilitySnapshotV2(context.Background(), first)
	second := toolcontract.CloneMCPCapabilitySnapshotV2(first)
	second.Revision, second.SourceDigest, second.CreatedUnixNano, second.Digest = 2, testDigestV1("concurrent-successor"), f.now.Add(time.Second).UnixNano(), ""
	second, _ = toolcontract.SealMCPCapabilitySnapshotV2(second)
	const workers = 64
	var group sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := repository.EnsureMCPCapabilitySnapshotRevisionV2(context.Background(), second, ptrMCPObjectRefV2(first.ObjectRef()))
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	current, _ := repository.InspectCurrentMCPCapabilitySnapshotV2(context.Background(), first.ID)
	if current.ObjectRef() != second.ObjectRef() {
		t.Fatalf("successor winner=%#v", current.ObjectRef())
	}
}

func ptrMCPObjectRefV2(value toolcontract.ObjectRef) *toolcontract.ObjectRef { return &value }

type mcpDiscoverySnapshotFixtureV2 struct {
	now        time.Time
	source     *discoveryPageExecutorSourceV1
	apply      *MCPDiscoveryPageApplyStoreV1
	physical   *InMemoryMCPDiscoveryPagePhysicalRepositoryV1
	aggregator *MCPDiscoverySnapshotAggregatorV2
	request    AggregateMCPDiscoverySnapshotRequestV2
}

func newMCPDiscoverySnapshotFixtureV2(t *testing.T) *mcpDiscoverySnapshotFixtureV2 {
	t.Helper()
	applyFixture := newMCPDiscoveryPageApplyFixtureV1(t)
	if _, err := applyFixture.apply.ApplyMCPDiscoveryPageSettlementV1(context.Background(), applyFixture.domain.ObjectRef()); err != nil {
		t.Fatal(err)
	}
	base := applyFixture.domainFixture.executorFixture
	f := &mcpDiscoverySnapshotFixtureV2{now: applyFixture.now, source: base.source, apply: applyFixture.apply, physical: base.entries}
	aggregator, err := NewMCPDiscoverySnapshotAggregatorV2(base.source, base.source, base.executor.commands, applyFixture.apply, base.entries, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	f.aggregator = aggregator
	f.request = AggregateMCPDiscoverySnapshotRequestV2{Connection: base.source.connection.Ref, AppliedCommands: []toolcontract.ObjectRef{applyFixture.domain.Command}, SnapshotRevision: 1, Conformance: "mcp/official-go-sdk-v1", RequestedExpiresUnixNano: f.now.Add(4 * time.Second).UnixNano()}
	return f
}

type mcpDiscoveryAppliedReaderFuncV2 func(context.Context, toolcontract.ObjectRef, time.Duration) (toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1, error)

func (f mcpDiscoveryAppliedReaderFuncV2) InspectMCPDiscoveryPageApplySettlementV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageApplySettlementFactV1, error) {
	return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "unused exact Apply reader")
}
func (f mcpDiscoveryAppliedReaderFuncV2) InspectCurrentMCPDiscoveryPageAppliedV1(ctx context.Context, exact toolcontract.ObjectRef, ttl time.Duration) (toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1, error) {
	return f(ctx, exact, ttl)
}
