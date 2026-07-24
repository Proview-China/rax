package fault_test

import (
	"testing"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

func sealCandidate(t *testing.T, candidate contract.ActionCandidate) contract.ActionCandidate {
	t.Helper()
	candidate.Digest = ""
	sealed, err := contract.SealActionCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func TestFaultLostReservationReplyInspectsSameFact(t *testing.T) {
	controller := action.NewController()
	candidate := testkit.Candidate()
	if _, err := controller.PutCandidate(candidate, candidate.PendingActionDigest); err != nil {
		t.Fatal(err)
	}
	args := []string{candidate.ID}
	first, err := controller.Reserve(args[0], testkit.Digest("app"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, testkit.FixedTime, testkit.FixedTime.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := controller.InspectReservation(candidate.ID, first.Reservation.ID, first.Reservation.Digest)
	if err != nil || inspected.Digest != first.Reservation.Digest {
		t.Fatalf("lost Reserve reply could not be recovered by exact Inspect: %v", err)
	}
	if _, err := controller.InspectReservation(candidate.ID, first.Reservation.ID, testkit.Digest("wrong-reservation")); err == nil {
		t.Fatal("exact Inspect accepted the wrong reservation digest")
	}
	second, err := controller.Reserve(args[0], testkit.Digest("app"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, testkit.FixedTime, testkit.FixedTime.Add(time.Minute))
	if err != nil || first.Reservation.Digest != second.Reservation.Digest || second.Revision != 2 {
		t.Fatalf("reservation recovery created a new fact: %v", err)
	}
	if _, err := controller.Reserve(args[0], testkit.Digest("different-app"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, testkit.FixedTime, testkit.FixedTime.Add(time.Minute)); err == nil {
		t.Fatal("changed reservation was accepted after uncertain reply")
	}
}

func TestFaultWrongPendingActionDigestRejected(t *testing.T) {
	controller := action.NewController()
	candidate := testkit.Candidate()
	if _, err := controller.PutCandidate(candidate, testkit.Digest("wrong-pending-action-request")); err == nil {
		t.Fatal("ActionCandidate with wrong Harness PendingAction RequestDigest was accepted")
	}
}

func TestFaultSamePendingRefRejectsPayloadCapabilityOrSourceDrift(t *testing.T) {
	controller := action.NewController()
	baseline := testkit.Candidate()
	if _, err := controller.PutCandidate(baseline, baseline.PendingActionDigest); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*contract.ActionCandidate){
		"payload": func(candidate *contract.ActionCandidate) {
			candidate.Payload = testkit.Payload(`{"changed":true}`)
			candidate.PayloadRevision++
		},
		"capability": func(candidate *contract.ActionCandidate) {
			candidate.Capability.ID = "tool/other"
			candidate.Capability.Digest = testkit.Digest("other-capability")
		},
		"source-candidate": func(candidate *contract.ActionCandidate) {
			candidate.SourceCandidateDigest = testkit.Digest("other-source-candidate")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := baseline
			candidate.Payload.Inline = append([]byte(nil), baseline.Payload.Inline...)
			candidate.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), baseline.EffectKinds...)
			candidate.ID, _ = contract.StableID("action", "pending-drift", name)
			mutate(&candidate)
			candidate = sealCandidate(t, candidate)
			if _, err := controller.PutCandidate(candidate, baseline.PendingActionDigest); err == nil {
				t.Fatalf("same PendingAction ref accepted changed %s", name)
			}
		})
	}
}

func TestFaultStaleConnectionCASAndCrossEpochSnapshot(t *testing.T) {
	manager := mcp.NewManager()
	connection := testkit.MCPConnection()
	record, err := manager.Register(connection, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Transition(connection.ID, record.Revision+1, mcp.ConnectionResolving, testkit.FixedTime); err == nil {
		t.Fatal("stale lifecycle CAS was accepted")
	}
	for _, state := range []mcp.ConnectionState{mcp.ConnectionResolving, mcp.ConnectionConnecting, mcp.ConnectionInitializing, mcp.ConnectionDiscovering} {
		record, err = manager.Transition(connection.ID, record.Revision, state, time.Unix(0, record.UpdatedUnixNano).Add(time.Nanosecond))
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot := testkit.MCPSnapshot()
	snapshot.ConnectionEpoch++
	snapshot.Digest = ""
	snapshot, err = contract.SealMCPSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.BindSnapshot(connection.ID, record.Revision, snapshot, time.Unix(0, record.UpdatedUnixNano).Add(time.Nanosecond)); err == nil {
		t.Fatal("cross-epoch snapshot was accepted")
	}
}

func TestFaultExpiredSnapshotBindRejected(t *testing.T) {
	manager := mcp.NewManager()
	connection := testkit.MCPConnection()
	record, err := manager.Register(connection, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []mcp.ConnectionState{mcp.ConnectionResolving, mcp.ConnectionConnecting, mcp.ConnectionInitializing, mcp.ConnectionDiscovering} {
		record, err = manager.Transition(connection.ID, record.Revision, state, time.Unix(0, record.UpdatedUnixNano).Add(time.Nanosecond))
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot := testkit.MCPSnapshot()
	snapshot.ExpiresUnixNano = testkit.FixedTime.Add(30 * time.Minute).UnixNano()
	snapshot, err = contract.SealMCPSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.BindSnapshot(connection.ID, record.Revision, snapshot, testkit.FixedTime.Add(31*time.Minute)); err == nil {
		t.Fatal("expired MCP snapshot was bound")
	}
	inspected, ok := manager.Inspect(connection.ID)
	if !ok || inspected.State != mcp.ConnectionDiscovering || inspected.Snapshot != nil {
		t.Fatal("expired snapshot changed the connection state")
	}
}
