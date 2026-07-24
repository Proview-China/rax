package kernel_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewPhaseSourceCurrentReaderV1ActionUsesExactV3Reader(t *testing.T) {
	fixture := newPendingActionReaderV3Fixture(t, pendingActionReaderV3Options{})
	delegate := fixture.newReader(t)
	actions := &recordingReviewActionReaderV1{delegate: delegate}
	sessions := &sequenceReviewSessionReaderV1{}
	clock := &reviewPhaseClockV1{values: []time.Time{fixture.now, fixture.now.Add(2 * time.Second), fixture.now.Add(3 * time.Second)}}
	reader, err := kernel.NewReviewPhaseSourceCurrentReaderV1(actions, sessions, clock.now)
	if err != nil {
		t.Fatal(err)
	}
	action := contract.ReviewActionPhaseSourceRefV1{Subject: fixture.request.Subject.Clone()}
	ref, err := contract.SealReviewPhaseSourceRefV1(contract.ReviewPhaseSourceRefV1{Kind: contract.ReviewPhaseActionSourceV1, Action: &action})
	if err != nil {
		t.Fatal(err)
	}
	request := contract.ReviewPhaseSourceCurrentRequestV1{Source: ref}
	projection, err := reader.InspectReviewPhaseSourceCurrentV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if actions.calls != 2 || sessions.calls != 0 || projection.Action == nil || projection.RunSession != nil || projection.Source.Digest != ref.Digest || projection.ExpiresUnixNano != fixture.now.Add(contract.MaxReviewPhaseSourceProjectionTTLV1).UnixNano() {
		t.Fatalf("action current sequence mismatch: actions=%d sessions=%d projection=%#v", actions.calls, sessions.calls, projection)
	}
	for _, got := range actions.requests {
		if !reflect.DeepEqual(got.Subject, fixture.request.Subject) || got.RequestedNotAfterUnixNano != 0 {
			t.Fatalf("action reader received non-exact request: %#v", got)
		}
	}
	projection.Action.PendingAction.Payload.Inline[0] ^= 1
	if !reflect.DeepEqual(fixture.session, fixture.originalSession) {
		t.Fatal("Review phase projection aliases action Owner state")
	}
	reader, _ = kernel.NewReviewPhaseSourceCurrentReaderV1(actions, sessions, (&reviewPhaseClockV1{values: []time.Time{fixture.now.Add(4 * time.Second), fixture.now.Add(5 * time.Second), fixture.now.Add(6 * time.Second)}}).now)
	fresh, err := reader.InspectReviewPhaseSourceCurrentV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.CheckedUnixNano == projection.CheckedUnixNano || fresh.ClosureDigest != projection.ClosureDigest {
		t.Fatalf("fresh time changed stable action closure: first=%#v second=%#v", projection, fresh)
	}
}

func TestReviewPhaseSourceCurrentReaderV1RunS1S2TTLAndUnsupportedSubagent(t *testing.T) {
	now := time.Unix(1_760_001_000, 0)
	session := terminalReviewSessionV1(t, now, "run")
	request := runReviewPhaseRequestV1(t, session, now.Add(8*time.Second).UnixNano())
	sessions := &sequenceReviewSessionReaderV1{values: []contract.GovernedSessionV4{session, session}}
	actions := &recordingReviewActionReaderV1{}
	clock := &reviewPhaseClockV1{values: []time.Time{now, now.Add(time.Second), now.Add(2 * time.Second)}}
	reader, _ := kernel.NewReviewPhaseSourceCurrentReaderV1(actions, sessions, clock.now)
	projection, err := reader.InspectReviewPhaseSourceCurrentV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if sessions.calls != 2 || actions.calls != 0 || projection.RunSession == nil || projection.Action != nil || projection.ExpiresUnixNano != request.RequestedNotAfterUnixNano {
		t.Fatalf("run current sequence/TTL mismatch: calls=%d projection=%#v", sessions.calls, projection)
	}
	if projection.ClosureDigest.Validate() != nil {
		t.Fatalf("run current closure is invalid: %v", projection.ClosureDigest)
	}

	subagent := contract.ReviewSubagentPhaseSourceRefV1{ParentRun: session.Run, SourceID: "subagent-source", SourceRevision: 1, SourceDigest: testkit.Digest("subagent-source")}
	ref, err := contract.SealReviewPhaseSourceRefV1(contract.ReviewPhaseSourceRefV1{Kind: contract.ReviewPhaseSubagentSourceV1, Subagent: &subagent})
	if err != nil {
		t.Fatal(err)
	}
	reader, _ = kernel.NewReviewPhaseSourceCurrentReaderV1(actions, sessions, func() time.Time { return now })
	_, err = reader.InspectReviewPhaseSourceCurrentV1(context.Background(), contract.ReviewPhaseSourceCurrentRequestV1{Source: ref})
	if !core.HasCategory(err, core.ErrorCapabilityUnavailable) || sessions.calls != 2 || actions.calls != 0 {
		t.Fatalf("subagent fail-closed error=%v session calls=%d action calls=%d", err, sessions.calls, actions.calls)
	}
}

func TestReviewPhaseSourceCurrentReaderV1LostReadReplyUsesDetachedSameExactInspect(t *testing.T) {
	now := time.Unix(1_760_002_000, 0)
	session := terminalReviewSessionV1(t, now, "lost")
	request := runReviewPhaseRequestV1(t, session, 0)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	sessions := &sequenceReviewSessionReaderV1{values: []contract.GovernedSessionV4{session, session}, errors: []error{context.Canceled}}
	reader, _ := kernel.NewReviewPhaseSourceCurrentReaderV1(&recordingReviewActionReaderV1{}, sessions, (&reviewPhaseClockV1{values: []time.Time{now, now.Add(time.Second), now.Add(2 * time.Second)}}).now)
	if _, err := reader.InspectReviewPhaseSourceCurrentV1(canceled, request); err != nil {
		t.Fatal(err)
	}
	if sessions.calls != 3 || !sessions.detached || len(sessions.keys) != 3 {
		t.Fatalf("lost reply recovery calls=%d detached=%v", sessions.calls, sessions.detached)
	}
	for _, key := range sessions.keys {
		if key.runID != string(session.Run.RunID) || key.sessionID != session.ID {
			t.Fatalf("recovery changed exact Inspect key: %#v", key)
		}
	}
}

func TestReviewPhaseSourceCurrentReaderV1RejectsDriftRollbackAndTTLCrossing(t *testing.T) {
	now := time.Unix(1_760_003_000, 0)
	session := terminalReviewSessionV1(t, now, "faults")
	request := runReviewPhaseRequestV1(t, session, now.Add(5*time.Second).UnixNano())

	t.Run("s1-s2 drift", func(t *testing.T) {
		drift := session.Clone()
		drift.Revision++
		drift.UpdatedUnixNano++
		drift, _ = contract.SealGovernedSessionV4(drift)
		reader, _ := kernel.NewReviewPhaseSourceCurrentReaderV1(&recordingReviewActionReaderV1{}, &sequenceReviewSessionReaderV1{values: []contract.GovernedSessionV4{session, drift}}, (&reviewPhaseClockV1{values: []time.Time{now, now.Add(time.Second), now.Add(2 * time.Second)}}).now)
		if _, err := reader.InspectReviewPhaseSourceCurrentV1(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("drift error=%v", err)
		}
	})
	t.Run("clock rollback", func(t *testing.T) {
		reader, _ := kernel.NewReviewPhaseSourceCurrentReaderV1(&recordingReviewActionReaderV1{}, &sequenceReviewSessionReaderV1{values: []contract.GovernedSessionV4{session, session}}, (&reviewPhaseClockV1{values: []time.Time{now, now.Add(2 * time.Second), now.Add(time.Second)}}).now)
		if _, err := reader.InspectReviewPhaseSourceCurrentV1(context.Background(), request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("rollback error=%v", err)
		}
	})
	t.Run("ttl crossing", func(t *testing.T) {
		reader, _ := kernel.NewReviewPhaseSourceCurrentReaderV1(&recordingReviewActionReaderV1{}, &sequenceReviewSessionReaderV1{values: []contract.GovernedSessionV4{session, session}}, (&reviewPhaseClockV1{values: []time.Time{now, now.Add(time.Second), now.Add(5 * time.Second)}}).now)
		if _, err := reader.InspectReviewPhaseSourceCurrentV1(context.Background(), request); !core.HasCategory(err, core.ErrorPreconditionFailed) {
			t.Fatalf("TTL crossing error=%v", err)
		}
	})
}

type recordingReviewActionReaderV1 struct {
	delegate interface {
		InspectCommittedPendingActionCurrentV3(context.Context, contract.CommittedPendingActionCurrentRequestV3) (contract.CommittedPendingActionCurrentV3, error)
	}
	calls    int
	requests []contract.CommittedPendingActionCurrentRequestV3
}

func (r *recordingReviewActionReaderV1) InspectCommittedPendingActionCurrentV3(ctx context.Context, request contract.CommittedPendingActionCurrentRequestV3) (contract.CommittedPendingActionCurrentV3, error) {
	r.calls++
	r.requests = append(r.requests, request.Clone())
	if r.delegate == nil {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "action absent")
	}
	return r.delegate.InspectCommittedPendingActionCurrentV3(ctx, request)
}

type reviewSessionKeyV1 struct{ runID, sessionID string }
type sequenceReviewSessionReaderV1 struct {
	values   []contract.GovernedSessionV4
	errors   []error
	calls    int
	detached bool
	keys     []reviewSessionKeyV1
}

func (r *sequenceReviewSessionReaderV1) InspectSessionV4(ctx context.Context, run contract.RunRef, id string) (contract.GovernedSessionV4, error) {
	index := r.calls
	r.calls++
	r.keys = append(r.keys, reviewSessionKeyV1{string(run.RunID), id})
	if ctx.Err() == nil && index > 0 {
		r.detached = true
	}
	if index < len(r.errors) && r.errors[index] != nil {
		return contract.GovernedSessionV4{}, r.errors[index]
	}
	valueIndex := index
	if len(r.errors) > 0 {
		valueIndex -= len(r.errors)
	}
	if valueIndex < 0 {
		valueIndex = 0
	}
	if valueIndex >= len(r.values) {
		valueIndex = len(r.values) - 1
	}
	if valueIndex < 0 {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "session absent")
	}
	return r.values[valueIndex].Clone(), nil
}

type reviewPhaseClockV1 struct {
	values []time.Time
	index  int
}

func (c *reviewPhaseClockV1) now() time.Time {
	if c.index >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	value := c.values[c.index]
	c.index++
	return value
}

func terminalReviewSessionV1(t testing.TB, now time.Time, suffix string) contract.GovernedSessionV4 {
	t.Helper()
	base, _ := testkit.GovernedFactsV2(now)
	base.ID += "-" + suffix
	base.Run.RunID = core.AgentRunID(string(base.Run.RunID) + "-" + suffix)
	session, err := contract.SealGovernedSessionV4(contract.GovernedSessionV4{ID: base.ID, Revision: 1, Run: base.Run, Endpoint: base.Endpoint, Phase: contract.SessionTerminalV2, CompletionClaim: contract.ClaimCancelled, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func runReviewPhaseRequestV1(t testing.TB, session contract.GovernedSessionV4, notAfter int64) contract.ReviewPhaseSourceCurrentRequestV1 {
	t.Helper()
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(session.Run.Scope)
	if err != nil {
		t.Fatal(err)
	}
	run := contract.ReviewRunPhaseSourceRefV1{Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, CompletionClaim: session.CompletionClaim}
	ref, err := contract.SealReviewPhaseSourceRefV1(contract.ReviewPhaseSourceRefV1{Kind: contract.ReviewPhaseRunSourceV1, Run: &run})
	if err != nil {
		t.Fatal(err)
	}
	return contract.ReviewPhaseSourceCurrentRequestV1{Source: ref, RequestedNotAfterUnixNano: notAfter}
}
