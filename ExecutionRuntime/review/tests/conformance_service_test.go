package review_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
)

func TestConformanceReviewServiceMemoryV1(t *testing.T) {
	now := time.Unix(1_900_600_000, 0)
	target := testkit.Target(now)
	bundle := testkit.ResultBundle(now, target.TenantID, "bundle-service-conformance")
	request := testkit.Request(now, target, "case-service-conformance")
	request.ResultBundle = &contract.ExactResourceRefV1{ID: bundle.ID, Revision: bundle.Revision, Digest: bundle.Digest}
	request.Digest = ""
	request, _ = contract.SealReviewRequestV1(request)
	trace := testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return now })
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, err := service.New(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckServiceV1(context.Background(), owner, conformance.ServiceFixtureV1{Submit: service.SubmitCommandV1{Request: request, ResultBundle: &bundle, Target: target, Trace: trace}}); err != nil {
		t.Fatal(err)
	}
}
