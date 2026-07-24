package blackbox_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
)

func TestOwnerLocalRefreshPortBlackBox(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	port := fixture.Service
	prepared, err := port.RefreshContextTurnV1(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	before, err := port.InspectContextTurnRefreshV1(context.Background(), contract.InspectContextTurnRefreshRequestV1{AttemptRef: prepared.AttemptRef})
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != contract.ContextTurnRefreshPendingV1 || before.Current != nil {
		t.Fatal("black-box pending leaked current")
	}
	apply, err := contract.SealApplyContextTurnRefreshRequestV1(contract.ApplyContextTurnRefreshRequestV1{AttemptRef: prepared.AttemptRef, PendingDomainResultRef: prepared.PendingDomainResultRef, ExpectedCurrent: fixture.Request.ExpectedCurrent, CheckedUnixNano: fixture.Now.UnixNano(), NotAfterUnixNano: prepared.ExpiresUnixNano}, fixture.Now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	after, err := port.ApplyContextTurnRefreshV1(context.Background(), apply)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != contract.ContextTurnRefreshAppliedV1 || after.Current == nil || after.ApplySettlementRef == nil {
		t.Fatal("black-box atomic apply absent")
	}
}
