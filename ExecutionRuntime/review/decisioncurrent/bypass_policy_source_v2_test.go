package decisioncurrent

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestBypassPolicySourceV2ExactCutLostReplyAndDrift(t *testing.T) {
	t.Run("exact", func(t *testing.T) {
		fixture := newBypassCurrentFixtureV5(t)
		source, err := NewBypassPolicySourceV2(fixture.policy, fixture.clock.Now)
		if err != nil {
			t.Fatal(err)
		}
		proof, err := source.ReadBypassCurrentV1(context.Background(), fixture.decision, fixture.clock.Now())
		if err != nil || proof != fixture.decision.ExternalProof || fixture.policy.inspectCalls != 2 {
			t.Fatalf("Policy V2 exact cut failed: proof=%+v err=%v calls=%d", proof, err, fixture.policy.inspectCalls)
		}
	})
	t.Run("lost exact inspect", func(t *testing.T) {
		fixture := newBypassCurrentFixtureV5(t)
		ctx, cancel := context.WithCancel(context.Background())
		fixture.policy.loseInspect, fixture.policy.cancel = true, cancel
		source, _ := NewBypassPolicySourceV2(fixture.policy, fixture.clock.Now)
		proof, err := source.ReadBypassCurrentV1(ctx, fixture.decision, fixture.clock.Now())
		if err != nil || proof.Digest == "" || fixture.policy.inspectCalls != 3 || ctx.Err() == nil {
			t.Fatalf("Policy V2 lost exact reply was not recovered once: proof=%+v err=%v calls=%d ctx=%v", proof, err, fixture.policy.inspectCalls, ctx.Err())
		}
	})
	t.Run("S2 drift", func(t *testing.T) {
		fixture := newBypassCurrentFixtureV5(t)
		fixture.policy.driftAt = 2
		source, _ := NewBypassPolicySourceV2(fixture.policy, fixture.clock.Now)
		if proof, err := source.ReadBypassCurrentV1(context.Background(), fixture.decision, fixture.clock.Now()); err == nil || proof.Digest != "" {
			t.Fatalf("Policy V2 S2 drift reached a proof: proof=%+v err=%v", proof, err)
		}
	})
	t.Run("clock rollback", func(t *testing.T) {
		fixture := newBypassCurrentFixtureV5(t)
		fixture.clock.rollbackAt = fixture.clock.calls + 2
		source, _ := NewBypassPolicySourceV2(fixture.policy, fixture.clock.Now)
		if proof, err := source.ReadBypassCurrentV1(context.Background(), fixture.decision, time.Time{}); !core.HasReason(err, core.ReasonClockRegression) || proof.Digest != "" {
			t.Fatalf("Policy V2 clock rollback reached a proof: proof=%+v err=%v", proof, err)
		}
	})
}
