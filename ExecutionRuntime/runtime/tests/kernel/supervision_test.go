package kernel_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
)

func TestSupervisionPolicyHasNoImplicitProductionDefaults(t *testing.T) {
	t.Parallel()
	if err := (kernel.SupervisionPolicy{}).Validate(); !core.HasReason(err, core.ReasonSupervisionPolicyMissing) {
		t.Fatalf("empty supervision policy must be rejected: %v", err)
	}
}

func TestLeaseRenewalDominatesRoutineInspection(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 30, 0, 0, time.UTC)
	policy := supervisionPolicy()
	record := supervisionRecord(t, now, now.Add(time.Hour), policy)
	decision, err := kernel.EvaluateSupervision(record, now.Add(50*time.Minute), policy)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != kernel.SupervisionRenewLease {
		t.Fatalf("lease authority maintenance must dominate overdue inspect: %+v", decision)
	}
}

func TestSupervisionPolicyCannotDriftWhileInstanceRuns(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 30, 30, 0, time.UTC)
	policy := supervisionPolicy()
	record := supervisionRecord(t, now, now.Add(time.Hour), policy)

	drifted := policy
	drifted.MaxConsecutiveFailures++
	if _, err := kernel.EvaluateSupervision(record, now, drifted); !core.HasReason(err, core.ReasonSupervisionPolicyDrift) {
		t.Fatalf("runtime accepted an unbound supervision policy: %v", err)
	}
	if _, _, err := kernel.ApplySupervisionUpdate(record, kernel.SupervisionUpdate{Signal: kernel.SignalHealthy, ObservedAt: now.Add(time.Second)}, drifted); !core.HasReason(err, core.ReasonSupervisionPolicyDrift) {
		t.Fatalf("runtime persisted state under an unbound supervision policy: %v", err)
	}
}

func TestTransientFailuresUseBoundedBackoffThenQuarantine(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 31, 0, 0, time.UTC)
	policy := supervisionPolicy()
	record := supervisionRecord(t, now, now.Add(time.Hour), policy)

	wantDelays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	for index, wantDelay := range wantDelays {
		observed := record.UpdatedAt.Add(time.Second)
		next, decision, err := kernel.ApplySupervisionUpdate(record, kernel.SupervisionUpdate{Signal: kernel.SignalTransientFailure, ObservedAt: observed}, policy)
		if err != nil {
			t.Fatal(err)
		}
		if got := next.NextActionAt.Sub(observed); got != wantDelay {
			t.Fatalf("failure %d backoff: want %s, got %s", index+1, wantDelay, got)
		}
		record = next
		if index < 2 && record.Mode != kernel.SupervisionBackoff {
			t.Fatalf("failure %d should remain in backoff: %+v", index+1, record)
		}
		if index == 2 && (record.Mode != kernel.SupervisionQuarantined || decision.Action != kernel.SupervisionQuarantine) {
			t.Fatalf("failure budget exhaustion must quarantine: record=%+v decision=%+v", record, decision)
		}
	}
	decision, err := kernel.EvaluateSupervision(record, record.NextActionAt, policy)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != kernel.SupervisionInspect || decision.AutomaticSafe {
		t.Fatalf("quarantine may clear only through authoritative inspection: %+v", decision)
	}
}

func TestStaleLeaseRevisionInspectsAndLateSignalCannotMutate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 32, 0, 0, time.UTC)
	policy := supervisionPolicy()
	record := supervisionRecord(t, now, now.Add(time.Hour), policy)
	next, decision, err := kernel.ApplySupervisionUpdate(record, kernel.SupervisionUpdate{Signal: kernel.SignalLeaseRevisionStale, ObservedAt: now.Add(time.Second)}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if next.Mode != kernel.SupervisionQuarantined || decision.Action != kernel.SupervisionInspect || decision.AutomaticSafe {
		t.Fatalf("stale lease CAS must inspect linearized authority: record=%+v decision=%+v", next, decision)
	}
	if _, _, err := kernel.ApplySupervisionUpdate(next, kernel.SupervisionUpdate{Signal: kernel.SignalHealthy, ObservedAt: now}, policy); !core.HasReason(err, core.ReasonLateSupervisionSignal) {
		t.Fatalf("late health result changed newer supervision state: %v", err)
	}
}

func TestFencedOrExpiredSupervisionCannotRevive(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 33, 0, 0, time.UTC)
	policy := supervisionPolicy()
	record := supervisionRecord(t, now, now.Add(time.Hour), policy)
	fenced, decision, err := kernel.ApplySupervisionUpdate(record, kernel.SupervisionUpdate{Signal: kernel.SignalAuthorityRevoked, ObservedAt: now.Add(time.Second)}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != kernel.SupervisionFenceAndStop || fenced.Mode != kernel.SupervisionFenced {
		t.Fatalf("authority revocation did not fence: record=%+v decision=%+v", fenced, decision)
	}
	if _, _, err := kernel.ApplySupervisionUpdate(fenced, kernel.SupervisionUpdate{Signal: kernel.SignalHealthy, ObservedAt: now.Add(2 * time.Second)}, policy); !core.HasReason(err, core.ReasonFencedInstance) {
		t.Fatalf("fenced instance regained authority: %v", err)
	}
	if _, _, err := kernel.ApplySupervisionUpdate(record, kernel.SupervisionUpdate{Signal: kernel.SignalHealthy, ObservedAt: now.Add(time.Hour)}, policy); !core.HasReason(err, core.ReasonIdentityLeaseStateInvalid) {
		t.Fatalf("expired lease accepted late health signal: %v", err)
	}
}

func TestSupervisionRemainsBoundedAcrossLongRunningSchedule(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 34, 0, 0, time.UTC)
	policy := supervisionPolicy()
	policy.InspectionInterval = time.Minute
	record := supervisionRecord(t, now, now.Add(2*time.Hour), policy)

	for range 10_000 {
		now = record.NextActionAt
		decision, err := kernel.EvaluateSupervision(record, now, policy)
		if err != nil {
			t.Fatal(err)
		}
		var update kernel.SupervisionUpdate
		switch decision.Action {
		case kernel.SupervisionInspect:
			update = kernel.SupervisionUpdate{Signal: kernel.SignalHealthy, ObservedAt: now}
		case kernel.SupervisionRenewLease:
			update = kernel.SupervisionUpdate{Signal: kernel.SignalLeaseRenewed, ObservedAt: now, NewLeaseExpiresAt: now.Add(2 * time.Hour)}
		default:
			t.Fatalf("unexpected long-running action at revision %d: %+v", record.Revision, decision)
		}
		record, _, err = kernel.ApplySupervisionUpdate(record, update, policy)
		if err != nil {
			t.Fatal(err)
		}
	}
	if record.Mode != kernel.SupervisionNormal || record.ConsecutiveFailures != 0 || record.Revision != 10_001 {
		t.Fatalf("long-running supervision drifted: %+v", record)
	}
}

func supervisionPolicy() kernel.SupervisionPolicy {
	return kernel.SupervisionPolicy{
		InspectionInterval: 10 * time.Minute, LeaseRenewLeadTime: 15 * time.Minute,
		RetryBaseDelay: time.Second, RetryMaxDelay: 4 * time.Second,
		MaxConsecutiveFailures: 3,
	}
}

func supervisionRecord(t *testing.T, now, leaseExpiry time.Time, policy kernel.SupervisionPolicy) kernel.SupervisionRecord {
	t.Helper()
	digest, err := policy.Digest()
	if err != nil {
		t.Fatal(err)
	}
	scope := supervisionScope(t)
	record, err := kernel.NewSupervisionRecord(scope, digest, leaseExpiry, now, policy)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func supervisionScope(t *testing.T) core.ExecutionScope {
	t.Helper()
	plan, err := core.DigestJSON("plan-supervision")
	if err != nil {
		t.Fatal(err)
	}
	return core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-supervision", Epoch: 4},
		Lineage:        core.LineageRef{ID: "lineage-supervision", PlanDigest: plan},
		Instance:       core.InstanceRef{ID: "instance-supervision", Epoch: 8},
		SandboxLease:   &core.SandboxLeaseRef{ID: "sandbox-supervision", Epoch: 2},
		AuthorityEpoch: 3,
	}
}
