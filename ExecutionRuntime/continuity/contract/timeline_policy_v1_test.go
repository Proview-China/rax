package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

func TestTimelineProjectionPolicyV1CanonicalCurrentAndTerminal(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	current := timelinePolicyV1(t, "policy-a", "scope-a", 1, contract.TimelineProjectionPolicyActiveV1, now, now.Add(time.Minute))
	if err := current.ValidateCurrent(current.Ref, now); err != nil {
		t.Fatal(err)
	}
	if err := current.ValidateCurrent(current.Ref, now.Add(time.Minute)); !contract.HasCode(err, contract.ErrPreconditionFailed) {
		t.Fatalf("exact expiry boundary must fail: %v", err)
	}
	drift := current
	drift.ExpiresUnixNano++
	if err := drift.Validate(); !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("same ref with changed body must conflict: %v", err)
	}
	revoked := timelinePolicyV1(t, "policy-a", "scope-a", 2, contract.TimelineProjectionPolicyRevokedV1, now.Add(time.Second), now.Add(time.Minute))
	if err := contract.ValidateTimelineProjectionPolicySuccessorV1(current, revoked); err != nil {
		t.Fatal(err)
	}
	next := timelinePolicyV1(t, "policy-a", "scope-a", 3, contract.TimelineProjectionPolicyActiveV1, now.Add(2*time.Second), now.Add(time.Minute))
	if err := contract.ValidateTimelineProjectionPolicySuccessorV1(revoked, next); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("terminal policy must not reactivate: %v", err)
	}
}

func timelinePolicyV1(t *testing.T, id, scope string, revision uint64, state contract.TimelineProjectionPolicyStateV1, checked, expires time.Time) contract.TimelineProjectionPolicyCurrentV1 {
	t.Helper()
	value, err := contract.SealTimelineProjectionPolicyCurrentV1(contract.TimelineProjectionPolicyCurrentV1{Ref: contract.TimelineProjectionPolicyRefV1{PolicyID: id, Revision: revision, ScopeDigest: scope}, State: state, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
