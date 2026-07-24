package failure_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/reviewadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/reviewcontextstore"
	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type lostReplyRepositoryV1 struct {
	reviewcontextstore.RepositoryV1
	commits atomic.Int64
	reads   atomic.Int64
}

func TestDurableReviewerContextFaultLostReplyIsExactInspectOnlyV1(t *testing.T) {
	now := time.Unix(1_932_000_000, 0)
	value, err := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1("durable-fault-lost"), 1, now, now.Add(time.Hour), "durable-fault-lost")
	if err != nil {
		t.Fatal(err)
	}
	sqliteRepository, err := reviewcontextstore.OpenSQLiteV1(context.Background(), reviewcontextstore.SQLiteConfigV1{Path: filepath.Join(t.TempDir(), "reviewer-context.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqliteRepository.Close() }()
	repository := &lostReplyRepositoryV1{RepositoryV1: sqliteRepository}
	adapter, err := reviewadapter.NewReviewerContextAdapterV1(repository, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := adapter.PublishReviewerContextV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: value})
	if err != nil || receipt.Ref != value.Ref {
		t.Fatalf("durable lost reply exact recovery: %#v %v", receipt, err)
	}
	if repository.commits.Load() != 1 || repository.reads.Load() != 1 {
		t.Fatalf("durable lost reply repeated mutation: commits=%d reads=%d", repository.commits.Load(), repository.reads.Load())
	}
}

func (r *lostReplyRepositoryV1) CommitV1(ctx context.Context, request reviewport.ReviewerContextPublishRequestV1) (reviewport.ReviewerContextPublishReceiptV1, error) {
	r.commits.Add(1)
	if _, err := r.RepositoryV1.CommitV1(ctx, request); err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, err
	}
	return reviewport.ReviewerContextPublishReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost reply")
}

func (r *lostReplyRepositoryV1) InspectHistoricalV1(ctx context.Context, ref reviewcontract.ReviewerContextEnvelopeRefV1) (reviewcontract.ReviewerContextEnvelopeV1, error) {
	r.reads.Add(1)
	return r.RepositoryV1.InspectHistoricalV1(ctx, ref)
}

func TestReviewerContextFaultLostReplyNeverRepeatsMutationV1(t *testing.T) {
	now := time.Unix(1_932_000_000, 0)
	value, err := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1("fault-lost"), 1, now, now.Add(time.Hour), "fault-lost")
	if err != nil {
		t.Fatal(err)
	}
	repository := &lostReplyRepositoryV1{RepositoryV1: reviewcontextstore.NewMemoryV1()}
	adapter, err := reviewadapter.NewReviewerContextAdapterV1(repository, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.PublishReviewerContextV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: value}); err != nil {
		t.Fatal(err)
	}
	if repository.commits.Load() != 1 || repository.reads.Load() != 1 {
		t.Fatalf("lost reply recovery was not inspect-only: commits=%d reads=%d", repository.commits.Load(), repository.reads.Load())
	}
}

type opaqueReadRepositoryV1 struct {
	reviewcontextstore.RepositoryV1
}

func (r opaqueReadRepositoryV1) ResolveV1(context.Context, reviewcontract.ReviewerContextSubjectV1) (reviewcontract.ReviewerContextEnvelopeRefV1, error) {
	return reviewcontract.ReviewerContextEnvelopeRefV1{}, errors.New("opaque backend error")
}

func TestReviewerContextFaultOpaqueReadIsIndeterminateV1(t *testing.T) {
	now := time.Unix(1_932_000_000, 0)
	subject := testfixture.ReviewerContextSubjectV1("fault-read")
	adapter, err := reviewadapter.NewReviewerContextAdapterV1(opaqueReadRepositoryV1{RepositoryV1: reviewcontextstore.NewMemoryV1()}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if ref, err := adapter.ResolveCurrentReviewerContextV1(context.Background(), reviewport.ReviewerContextCurrentResolveRequestV1{Subject: subject}); !core.HasCategory(err, core.ErrorIndeterminate) || ref.ID != "" {
		t.Fatalf("opaque backend error escaped closed set: %#v %v", ref, err)
	}
}

func TestReviewerContextFaultTTLAndClockRollbackReturnZeroV1(t *testing.T) {
	now := time.Unix(1_932_000_000, 0)
	for _, tc := range []struct {
		name   string
		clocks []time.Time
		reason core.ReasonCode
	}{
		{name: "expiry", clocks: []time.Time{now.Add(59 * time.Minute), now.Add(time.Hour)}, reason: core.ReasonReviewVerdictStale},
		{name: "rollback", clocks: []time.Time{now.Add(time.Minute), now.Add(time.Second)}, reason: core.ReasonClockRegression},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clock := testfixture.NewSequenceClockV1(tc.clocks...)
			value, buildErr := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1("fault-"+tc.name), 1, now, now.Add(time.Hour), tc.name)
			if buildErr != nil {
				t.Fatal(buildErr)
			}
			store := reviewcontextstore.NewMemoryV1()
			if _, commitErr := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: value}); commitErr != nil {
				t.Fatal(commitErr)
			}
			adapter, newErr := reviewadapter.NewReviewerContextAdapterV1(store, clock.Now)
			if newErr != nil {
				t.Fatal(newErr)
			}
			if got, inspectErr := adapter.InspectCurrentReviewerContextV1(context.Background(), value.Subject, value.Ref); !core.HasReason(inspectErr, tc.reason) || got.Ref.ID != "" {
				t.Fatalf("actual-point fault returned nonzero: %#v %v", got, inspectErr)
			}
		})
	}
}
