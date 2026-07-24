package review_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
)

func TestConformanceMemoryStoreV1(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	target := testkit.Target(now)
	base, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: target.TenantID, ID: "case-conformance", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, State: contract.CaseRequestedV1, ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	next := base
	next.Revision = 2
	next.State = contract.CaseAdmittedV1
	next.UpdatedUnixNano = now.Add(time.Second).UnixNano()
	next.Digest = ""
	next, err = contract.SealReviewCaseV1(next)
	if err != nil {
		t.Fatal(err)
	}
	trace := testkit.TraceForTarget(now, base.ID, target, contract.TraceRequestedV1, 1)
	if err := conformance.CheckStoreV1(context.Background(), memory.NewStore(), conformance.StoreFixtureV1{Target: target, Case: base, Trace: trace, Next: next, NextTrace: testkit.TransitionTrace(now.Add(time.Second), base, contract.CaseAdmittedV1)}); err != nil {
		t.Fatal(err)
	}
}
