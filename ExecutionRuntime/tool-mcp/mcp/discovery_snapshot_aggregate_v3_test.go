package mcp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func TestMCPDiscoverySnapshotAggregatorV3ProvenanceAndDeepClone(t *testing.T) {
	f := newMCPDiscoverySnapshotFixtureV2(t)
	request := requestV3FromFixture(f)
	snapshot, err := f.aggregator.AggregateMCPDiscoverySnapshotV3(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Pages) != 1 || len(snapshot.ToolMaterials) != 1 || snapshot.ToolMaterials[0].Source != snapshot.Tools[0] || snapshot.ToolMaterials[0].PageReceipt != snapshot.Pages[0].ProtocolReceipt {
		t.Fatalf("snapshot provenance drifted: %#v", snapshot)
	}
	repository := NewInMemoryMCPCapabilitySnapshotRepositoryV3()
	winner, err := repository.EnsureMCPCapabilitySnapshotV3(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	read, err := repository.InspectMCPCapabilitySnapshotV3(context.Background(), winner.ObjectRef())
	if err != nil {
		t.Fatal(err)
	}
	read.Tools[0].Name = "mutated"
	read.ToolMaterials[0].Source.Name = "mutated"
	again, _ := repository.InspectMCPCapabilitySnapshotV3(context.Background(), winner.ObjectRef())
	if again.Tools[0].Name != "echo" || again.ToolMaterials[0].Source.Name != "echo" {
		t.Fatal("Snapshot V3 Repository returned aliased slices")
	}
}

func TestMCPDiscoverySnapshotAggregatorV3RejectsMaterialDrift(t *testing.T) {
	f := newMCPDiscoverySnapshotFixtureV2(t)
	f.physical.mu.Lock()
	for id, entry := range f.physical.entries {
		if len(entry.ToolMaterials) != 0 {
			entry.ToolMaterials[0].Source.Name = "forged"
			f.physical.entries[id] = entry
		}
	}
	f.physical.mu.Unlock()
	if _, err := f.aggregator.AggregateMCPDiscoverySnapshotV3(context.Background(), requestV3FromFixture(f)); err == nil {
		t.Fatal("drifting Tool Material was accepted")
	}
}

func TestMCPCapabilitySnapshotV3RejectsIncompleteProvenance(t *testing.T) {
	f := newMCPDiscoverySnapshotFixtureV2(t)
	snapshot, err := f.aggregator.AggregateMCPDiscoverySnapshotV3(context.Background(), requestV3FromFixture(f))
	if err != nil {
		t.Fatal(err)
	}
	tampered := toolcontract.CloneMCPCapabilitySnapshotV3(snapshot)
	tampered.ToolMaterials = nil
	tampered.ValidationDigest, tampered.Digest = "", ""
	if _, err = toolcontract.SealMCPCapabilitySnapshotV3(tampered); err == nil {
		t.Fatal("Snapshot V3 without Tool provenance was sealed")
	}
	tampered = toolcontract.CloneMCPCapabilitySnapshotV3(snapshot)
	tampered.ToolMaterials[0].PageReceipt = toolcontract.ObjectRef{ID: "other-receipt", Revision: 1, Digest: testDigestV1("other-receipt")}
	tampered.ValidationDigest, tampered.Digest = "", ""
	if _, err = toolcontract.SealMCPCapabilitySnapshotV3(tampered); err == nil {
		t.Fatal("Snapshot V3 cross-page Material was sealed")
	}
}

func TestMCPCapabilitySnapshotRepositoryV3CASLostReplyAndConcurrentWinner(t *testing.T) {
	f := newMCPDiscoverySnapshotFixtureV2(t)
	first, err := f.aggregator.AggregateMCPDiscoverySnapshotV3(context.Background(), requestV3FromFixture(f))
	if err != nil {
		t.Fatal(err)
	}
	repository := NewInMemoryMCPCapabilitySnapshotRepositoryV3()
	first, err = repository.EnsureMCPCapabilitySnapshotV3(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := repository.EnsureMCPCapabilitySnapshotV3(context.Background(), first); err != nil || retry.ObjectRef() != first.ObjectRef() {
		t.Fatalf("lost-reply retry=%#v err=%v", retry, err)
	}
	second := toolcontract.CloneMCPCapabilitySnapshotV3(first)
	second.Revision, second.SourceDigest, second.CreatedUnixNano, second.Digest = 2, testDigestV1("v3-successor"), f.now.Add(time.Second).UnixNano(), ""
	second, err = toolcontract.SealMCPCapabilitySnapshotV3(second)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	var group sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := repository.EnsureMCPCapabilitySnapshotRevisionV3(context.Background(), second, ptrMCPObjectRefV2(first.ObjectRef()))
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
	current, err := repository.InspectCurrentMCPCapabilitySnapshotV3(context.Background(), first.ID)
	if err != nil || current.ObjectRef() != second.ObjectRef() {
		t.Fatalf("current=%#v err=%v", current.ObjectRef(), err)
	}
	if _, err = repository.EnsureMCPCapabilitySnapshotV3(context.Background(), first); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("historical rollback error=%v", err)
	}
}

func TestMCPDiscoverySnapshotAggregatorV3NilCanceledAndClockRollback(t *testing.T) {
	f := newMCPDiscoverySnapshotFixtureV2(t)
	request := requestV3FromFixture(f)
	if _, err := f.aggregator.AggregateMCPDiscoverySnapshotV3(nil, request); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.aggregator.AggregateMCPDiscoverySnapshotV3(ctx, request); err != context.Canceled {
		t.Fatalf("canceled context error=%v", err)
	}
	calls := 0
	f.aggregator.clock = func() time.Time {
		calls++
		if calls == 1 {
			return f.now
		}
		return f.now.Add(-time.Nanosecond)
	}
	if _, err := f.aggregator.AggregateMCPDiscoverySnapshotV3(context.Background(), request); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback error=%v", err)
	}
}

func requestV3FromFixture(f *mcpDiscoverySnapshotFixtureV2) AggregateMCPDiscoverySnapshotRequestV3 {
	return AggregateMCPDiscoverySnapshotRequestV3{
		Connection: f.request.Connection, AppliedCommands: append([]toolcontract.ObjectRef(nil), f.request.AppliedCommands...),
		SnapshotRevision: f.request.SnapshotRevision, Conformance: f.request.Conformance,
		RequestedExpiresUnixNano: f.request.RequestedExpiresUnixNano,
	}
}
