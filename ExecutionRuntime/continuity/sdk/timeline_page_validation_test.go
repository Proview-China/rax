package sdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/sdk"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestClientTimelinePageRejectsReaderFilterOrderAndCursorDrift(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	timeline, query, page := sdkTimelinePageFixture(t, now)
	other := query
	other.PolicyWatermark = "policy-watermark-2"
	otherPage, err := timeline.Query(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		mutate func(*contract.TimelinePage)
	}{
		{name: "invalid record", mutate: func(page *contract.TimelinePage) { page.Records[0].Candidate.Digest = "tampered" }},
		{name: "filter mismatch", mutate: func(page *contract.TimelinePage) {
			page.Records[0].Candidate.Scope.IdentityID = "identity-2"
			page.Records[0].Candidate.Digest, _ = page.Records[0].Candidate.CanonicalDigest()
		}},
		{name: "sequence order", mutate: func(page *contract.TimelinePage) { page.Records[0], page.Records[1] = page.Records[1], page.Records[0] }},
		{name: "cursor policy drift", mutate: func(page *contract.TimelinePage) { page.NextCursor = otherPage.NextCursor }},
		{name: "cursor after mismatch", mutate: func(page *contract.TimelinePage) { page.Records = page.Records[:1] }},
		{name: "page over limit", mutate: func(page *contract.TimelinePage) { page.Records = append(page.Records, page.Records[1].Clone()) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneSDKTimelinePage(page)
			test.mutate(&candidate)
			reader := &sdkTimelineReaderFake{delegate: timeline, queryPage: candidate, watchPage: candidate}
			client, err := sdk.New(sdk.Config{Timeline: reader, Clock: func() time.Time { return now }})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.QueryTimeline(context.Background(), query); err == nil {
				t.Fatal("QueryTimeline accepted a drifting Reader page")
			}
			if _, err := client.WatchTimeline(context.Background(), query); err == nil {
				t.Fatal("WatchTimeline accepted a drifting Reader page")
			}
		})
	}
}

func TestClientTimelinePageValidatesAndClonesExactReaderResult(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	timeline, query, page := sdkTimelinePageFixture(t, now)
	reader := &sdkTimelineReaderFake{delegate: timeline, queryPage: page, watchPage: page}
	client, err := sdk.New(sdk.Config{Timeline: reader, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.QueryTimeline(context.Background(), query)
	if err != nil || len(result.Records) != 2 || result.NextCursor == "" {
		t.Fatalf("QueryTimeline = %#v err=%v", result, err)
	}
	result.Records[0].Candidate.ObjectRefs[0] = "mutated"
	again, err := client.QueryTimeline(context.Background(), query)
	if err != nil || again.Records[0].Candidate.ObjectRefs[0] == "mutated" {
		t.Fatal("validated Timeline page aliases Reader storage")
	}
}

func TestClientTimelinePageRejectsInvalidInputCursorBeforeReader(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	timeline, query, page := sdkTimelinePageFixture(t, now)
	reader := &sdkTimelineReaderFake{delegate: timeline, queryPage: page, watchPage: page}
	client, err := sdk.New(sdk.Config{Timeline: reader, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	query.Cursor = "caller-forged-cursor"
	if _, err := client.QueryTimeline(context.Background(), query); err == nil || reader.queryCalls != 0 {
		t.Fatalf("QueryTimeline forwarded an invalid cursor: calls=%d err=%v", reader.queryCalls, err)
	}
	if _, err := client.WatchTimeline(context.Background(), query); err == nil || reader.watchCalls != 0 {
		t.Fatalf("WatchTimeline forwarded an invalid cursor: calls=%d err=%v", reader.watchCalls, err)
	}
}

type sdkTimelineReaderFake struct {
	delegate   *domain.ReferenceTimeline
	queryPage  contract.TimelinePage
	watchPage  contract.TimelinePage
	queryCalls int
	watchCalls int
}

func (r *sdkTimelineReaderFake) Inspect(ctx context.Context, ref string) (contract.TimelineEventRecord, error) {
	return r.delegate.Inspect(ctx, ref)
}

func (r *sdkTimelineReaderFake) Query(context.Context, contract.TimelineQuery) (contract.TimelinePage, error) {
	r.queryCalls++
	return r.queryPage, nil
}

func (r *sdkTimelineReaderFake) Watch(context.Context, contract.TimelineQuery) (contract.TimelinePage, error) {
	r.watchCalls++
	return r.watchPage, nil
}

func sdkTimelinePageFixture(t *testing.T, now time.Time) (*domain.ReferenceTimeline, contract.TimelineQuery, contract.TimelinePage) {
	t.Helper()
	clock := &testkit.Clock{Time: now}
	backend := memory.NewWithClock(func() time.Time { return clock.Time })
	timeline, err := domain.NewReferenceTimeline(backend, clock, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for sequence := uint64(1); sequence <= 2; sequence++ {
		if _, _, err := timeline.Project(context.Background(), testkit.Candidate(sequence, sequence, contract.TrustObservation)); err != nil {
			t.Fatal(err)
		}
	}
	query := contract.TimelineQuery{LedgerScopeDigest: "ledger-scope-1", IdentityID: "identity-1", AuthorityWatermark: "authority-watermark-1", PolicyWatermark: "policy-watermark-1", PageLimit: 2}
	page, err := timeline.Query(context.Background(), query)
	if err != nil || len(page.Records) != 2 {
		t.Fatalf("fixture Query = %#v err=%v", page, err)
	}
	return timeline, query, page
}

func cloneSDKTimelinePage(page contract.TimelinePage) contract.TimelinePage {
	clone := page
	clone.Records = make([]contract.TimelineEventRecord, len(page.Records))
	for index := range page.Records {
		clone.Records[index] = page.Records[index].Clone()
	}
	return clone
}
