package resultgrounding

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestResultGroundingConstructorRejectsMissingOwnersAndInvalidRecovery(t *testing.T) {
	if _, err := NewResultBundleCurrentGroundingReaderV2(ResultBundleGroundingReadRecoveryPolicyV2{}, ResultBundleCurrentGroundingDependenciesV2{}); err == nil {
		t.Fatal("zero recovery policy must fail closed")
	}
	if _, err := NewResultBundleCurrentGroundingReaderV2(ResultBundleGroundingReadRecoveryPolicyV2{ReadRecoveryTimeoutNanos: int64(time.Second)}, ResultBundleCurrentGroundingDependenciesV2{}); err == nil {
		t.Fatal("missing external Owner Readers must fail at construction")
	}
	if _, err := NewResultBundleCurrentGroundingReaderV2(ResultBundleGroundingReadRecoveryPolicyV2{ReadRecoveryTimeoutNanos: int64(2*time.Second + time.Nanosecond)}, ResultBundleCurrentGroundingDependenciesV2{}); err == nil {
		t.Fatal("recovery timeout above two seconds must fail closed")
	}
}

func TestRecoverExactReadV2UsesOneBoundedDetachedRetry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	r := &readerV2{
		policy: ResultBundleGroundingReadRecoveryPolicyV2{ReadRecoveryTimeoutNanos: int64(time.Second)},
		deps:   ResultBundleCurrentGroundingDependenciesV2{Clock: func() time.Time { return now }},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	value, err := recoverExactReadV2(r, ctx, now.Add(time.Minute).UnixNano(), func(callCtx context.Context) (string, error) {
		calls++
		if calls == 1 {
			if callCtx.Err() == nil {
				t.Fatal("first call must observe caller cancellation")
			}
			return "", core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "lost reply")
		}
		if callCtx.Err() != nil {
			t.Fatalf("bounded recovery must detach cancellation: %v", callCtx.Err())
		}
		if _, ok := callCtx.Deadline(); !ok {
			t.Fatal("detached recovery must remain deadline bounded")
		}
		return "exact", nil
	})
	if err != nil || value != "exact" || calls != 2 {
		t.Fatalf("exact recovery mismatch: value=%q err=%v calls=%d", value, err, calls)
	}
}

func TestRecoverExactReadV2PreservesOriginalErrorAndDoesNotRetryPastTTL(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	r := &readerV2{policy: ResultBundleGroundingReadRecoveryPolicyV2{ReadRecoveryTimeoutNanos: int64(time.Second)}, deps: ResultBundleCurrentGroundingDependenciesV2{Clock: func() time.Time { return now }}}
	original := core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "original")
	calls := 0
	_, err := recoverExactReadV2(r, context.Background(), now.UnixNano(), func(context.Context) (string, error) {
		calls++
		return "", original
	})
	if err != original || calls != 1 {
		t.Fatalf("expired recovery must preserve original error without retry: err=%v calls=%d", err, calls)
	}
	calls = 0
	_, err = recoverExactReadV2(r, context.Background(), now.Add(time.Minute).UnixNano(), func(context.Context) (string, error) {
		calls++
		if calls == 1 {
			return "", original
		}
		return "", core.NewError(core.ErrorUnavailable, core.ReasonOwnerMissing, "retry failed")
	})
	if err != original || calls != 2 {
		t.Fatalf("failed exact retry must preserve original error: err=%v calls=%d", err, calls)
	}
}
