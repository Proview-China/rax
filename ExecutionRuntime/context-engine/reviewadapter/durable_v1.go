package reviewadapter

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/reviewcontextstore"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// DurableReviewerContextAdapterV1 is a production-shaped, single-node Context
// Owner capability. Constructing it does not install a host composition root
// and does not claim HA, backup, remote durability or SLA.
type DurableReviewerContextAdapterV1 struct {
	*ReviewerContextAdapterV1
	repository *reviewcontextstore.SQLiteV1
}

var (
	_ reviewport.ReviewerContextPublisherV1     = (*DurableReviewerContextAdapterV1)(nil)
	_ reviewport.ReviewerContextCurrentReaderV1 = (*DurableReviewerContextAdapterV1)(nil)
)

func OpenDurableReviewerContextAdapterV1(ctx context.Context, config reviewcontextstore.SQLiteConfigV1, clock ClockV1) (*DurableReviewerContextAdapterV1, error) {
	if clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "durable Reviewer Context adapter clock is unavailable")
	}
	repository, err := reviewcontextstore.OpenSQLiteV1(ctx, config)
	if err != nil {
		return nil, err
	}
	adapter, err := NewReviewerContextAdapterV1(repository, clock)
	if err != nil {
		_ = repository.Close()
		return nil, err
	}
	return &DurableReviewerContextAdapterV1{ReviewerContextAdapterV1: adapter, repository: repository}, nil
}

func (a *DurableReviewerContextAdapterV1) Close() error {
	if a == nil || a.repository == nil {
		return nil
	}
	return a.repository.Close()
}

func (a *DurableReviewerContextAdapterV1) IntegrityCheckV1(ctx context.Context) error {
	if a == nil || a.repository == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "durable Reviewer Context adapter is unavailable")
	}
	return a.repository.IntegrityCheckV1(ctx)
}
