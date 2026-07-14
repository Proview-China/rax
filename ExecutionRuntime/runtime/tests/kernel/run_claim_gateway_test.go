package kernel_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRunClaimGatewayPersistsClaimWithoutPromotingRuntimeOutcome(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 20, 0, 0, time.UTC)
	gateway, journal, scope := runClaimFixture(t, now)
	request := completionClaimRequest(t, scope, "run-claim", now.Add(time.Second), 7, core.RunClaimCompleted, "completed")

	result, err := gateway.Ingest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Status != core.RunRunning || result.Run.Outcome != "" || result.Run.CompletionClaim == nil || result.Run.Revision != 2 {
		t.Fatalf("opaque claim was dropped or promoted to Runtime outcome: %+v", result.Run)
	}
	if result.Evidence.Sequence != 1 || result.Run.CompletionClaim.EvidenceSequence != result.Evidence.Sequence {
		t.Fatalf("claim did not bind persisted observation evidence: %+v", result)
	}
	persisted, err := journal.Inspect(context.Background(), scope, "run-claim")
	if err != nil || persisted.Status != core.RunRunning || persisted.Outcome != "" {
		t.Fatalf("claim changed authoritative run lifecycle: record=%+v err=%v", persisted, err)
	}
}

func TestRunClaimGatewayReplayIsIdempotentAndSequenceReuseConflicts(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 21, 0, 0, time.UTC)
	gateway, _, scope := runClaimFixture(t, now)
	request := completionClaimRequest(t, scope, "run-claim", now.Add(time.Second), 8, core.RunClaimCompleted, "completed")
	first, err := gateway.Ingest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := gateway.Ingest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Evidence != first.Evidence || replayed.Run.Revision != first.Run.Revision {
		t.Fatalf("same source event was not idempotent: first=%+v replay=%+v", first, replayed)
	}

	conflict := request
	conflict.Observation.Payload = opaquePayload(t, "different")
	if _, err := gateway.Ingest(context.Background(), conflict); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("source sequence reuse with different content was accepted: %v", err)
	}
}

func TestRunClaimGatewayRejectsStaleEpochBeforeEvidenceAppend(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 22, 0, 0, time.UTC)
	gateway, _, scope := runClaimFixture(t, now)
	stale := completionClaimRequest(t, scope, "run-claim", now.Add(time.Second), 9, core.RunClaimFailed, "failed")
	stale.Observation.SourceEpoch--
	if _, err := gateway.Ingest(context.Background(), stale); !core.HasReason(err, core.ReasonRunClaimUnverified) {
		t.Fatalf("stale epoch completion claim was accepted: %v", err)
	}
	valid := completionClaimRequest(t, scope, "run-claim", now.Add(time.Second), 10, core.RunClaimFailed, "failed")
	result, err := gateway.Ingest(context.Background(), valid)
	if err != nil {
		t.Fatal(err)
	}
	if result.Evidence.Sequence != 1 {
		t.Fatalf("rejected stale claim was written as evidence: %+v", result.Evidence)
	}
}

func runClaimFixture(t *testing.T, now time.Time) (*kernel.RunClaimGateway, *kernel.RunJournal, core.ExecutionScope) {
	t.Helper()
	store := fakes.NewFactStore(func() time.Time { return now })
	journal, err := kernel.NewRunJournal(store)
	if err != nil {
		t.Fatal(err)
	}
	scope := newAggregate(t).Snapshot().Scope
	if _, err := journal.Start(context.Background(), scope, "run-claim", "session-claim", now); err != nil {
		t.Fatal(err)
	}
	evidence, err := fakes.NewFakeEvidence("run-claim-evidence", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := kernel.NewRunClaimGateway(evidence, journal)
	if err != nil {
		t.Fatal(err)
	}
	return gateway, journal, scope
}

func completionClaimRequest(t *testing.T, scope core.ExecutionScope, runID core.AgentRunID, observedAt time.Time, sequence uint64, kind core.RunCompletionClaimKind, content string) kernel.RunClaimIngestRequest {
	t.Helper()
	return kernel.RunClaimIngestRequest{
		Scope: scope, RunID: runID, SourceSequence: sequence, ClaimKind: kind, CausationID: string(runID),
		Observation: ports.ExecutionObservation{
			SourceComponentID: "harness-claim", SourceEpoch: scope.Instance.Epoch,
			ObservationKind: "state:terminal", Payload: opaquePayload(t, content), ObservedAt: observedAt,
		},
	}
}

func opaquePayload(t *testing.T, content string) ports.OpaquePayload {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"completion_claim": content})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := core.DigestJSON(json.RawMessage(payload))
	if err != nil {
		t.Fatal(err)
	}
	return ports.OpaquePayload{Schema: "praxis.test.run-claim/v1", Digest: digest, Payload: payload}
}
