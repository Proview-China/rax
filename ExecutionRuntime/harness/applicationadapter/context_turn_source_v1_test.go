package applicationadapter

import (
	"testing"
	"time"
)

func TestContextTurnSourceExactMappingAndFailClosed(t *testing.T) {
	fixture := newAssemblerV2Fixture(t)
	projection, err := ContextTurnSourceFromCommittedPendingActionV3(fixture.current, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Turn.Ordinal != fixture.current.Turn || projection.Turn.Digest != fixture.current.TurnApplicability.Digest || projection.Session.Digest != fixture.current.SessionDigest || projection.RunID != fixture.current.Run.RunID {
		t.Fatalf("lossy Session/Turn mapping: %#v", projection)
	}
	if _, err := ContextTurnSourceFromCommittedPendingActionV3(fixture.current, time.Unix(0, fixture.current.ExpiresUnixNano)); err == nil {
		t.Fatal("expired Harness current granted a Context Turn source")
	}
	tampered := fixture.current
	tampered.Turn++
	if _, err := ContextTurnSourceFromCommittedPendingActionV3(tampered, fixture.now); err == nil {
		t.Fatal("tampered Turn was accepted")
	}
}
