package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

func streamRefV1(t *testing.T) contract.ExactRefWireV1 {
	t.Helper()
	return exactRefV1(t, contract.AgentRunEventStreamRefKindV1, "stream-1", 1)
}

func retentionRefV1(t *testing.T) contract.ExactRefWireV1 {
	t.Helper()
	return exactRefV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, "retention-1", 4)
}

func eventV1(t *testing.T, stream contract.ExactRefWireV1, sequence uint64) contract.AgentRunEventV1 {
	t.Helper()
	sequenceWire := contract.NewWireUint64V1(sequence)
	eventID, err := contract.AgentRunEventIDV1(stream, sequenceWire)
	if err != nil {
		t.Fatal(err)
	}
	event, err := contract.SealAgentRunEventV1(contract.AgentRunEventV1{
		StreamRef:        stream,
		Sequence:         sequenceWire,
		EventRef:         contract.ExactRefWireV1{Kind: contract.AgentRunEventRefKindV1, ID: eventID, Revision: sequenceWire},
		SourceOwnerRef:   exactRefV1(t, "praxis.runtime/run-event", "owner-event-"+string(contract.NewWireUint64V1(sequence)), sequence),
		SubjectRef:       exactRefV1(t, contract.AgentRunCurrentKindV1, "run-1", 9),
		Kind:             "run.lifecycle.changed",
		PayloadVersion:   "1.0.0",
		PayloadDigest:    digestV1(t, "payload-"+string(contract.NewWireUint64V1(sequence))),
		OccurredUnixNano: contract.NewWireUnixNanoV1(100 + int64(sequence)),
		ObservedUnixNano: contract.NewWireUnixNanoV1(110 + int64(sequence)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func watchRequestV1(t *testing.T, stream, retention contract.ExactRefWireV1) contract.AgentRunWatchRequestV1 {
	t.Helper()
	request, err := contract.SealAgentRunWatchRequestV1(contract.AgentRunWatchRequestV1{
		RequestID:           "watch-1",
		TraceID:             "trace-1",
		Target:              targetV1(t),
		StreamRef:           stream,
		RetentionCurrentRef: retention,
		AfterSequence:       contract.NewWireUint64V1(0),
		Limit:               contract.NewWireUint64V1(10),
		RequestedUnixNano:   contract.NewWireUnixNanoV1(120),
		NotAfterUnixNano:    contract.NewWireUnixNanoV1(220),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestAgentRunEventV1BindsExactIdentityBodyAndRejectsSplice(t *testing.T) {
	event := eventV1(t, streamRefV1(t), 1)
	if event.EventRef.Digest != event.EventDigest {
		t.Fatalf("event exact ref digest=%s event digest=%s", event.EventRef.Digest, event.EventDigest)
	}
	spliced := event
	spliced.SubjectRef = exactRefV1(t, contract.AgentRunCurrentKindV1, "run-splice", 9)
	if err := spliced.Validate(); err == nil || !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("subject splice err=%v", err)
	}
	spliced = event
	spliced.EventRef.Digest = digestV1(t, "forged-event-ref")
	if err := spliced.Validate(); err == nil || !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("event ref digest splice err=%v", err)
	}
	spliced = event
	spliced.PayloadDigest = digestV1(t, "forged-payload")
	if err := spliced.Validate(); err == nil || !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("payload splice err=%v", err)
	}
	replay := event
	replay.PayloadDigest = digestV1(t, "changed-owner-payload")
	replay.EventRef.Digest = ""
	replay.EventDigest = ""
	replay, err := contract.SealAgentRunEventV1(replay)
	if err != nil {
		t.Fatal(err)
	}
	if replay.EventRef.ID != event.EventRef.ID {
		t.Fatal("same stream sequence changed deterministic event id")
	}
	if err := contract.ValidateAgentRunEventReplayV1(event, replay); err == nil || !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("same sequence content replay err=%v", err)
	}
}

func TestAgentRunEventV1RequiresOpenNamespacedKind(t *testing.T) {
	event := eventV1(t, streamRefV1(t), 1)
	event.Kind = "changed"
	event.EventRef.Digest = ""
	event.EventDigest = ""
	if _, err := contract.SealAgentRunEventV1(event); err == nil || !contract.HasCode(err, contract.FaultInvalidArgumentV1) {
		t.Fatalf("unnamespaced event kind err=%v", err)
	}
	event.Kind = "custom.vendor/run-changed"
	if _, err := contract.SealAgentRunEventV1(event); err != nil {
		t.Fatalf("open namespaced event kind err=%v", err)
	}
}

func TestAgentRunWatchV1ContinuousEventsAdvanceCursor(t *testing.T) {
	stream, retention := streamRefV1(t), retentionRefV1(t)
	request := watchRequestV1(t, stream, retention)
	events := []contract.AgentRunEventV1{eventV1(t, stream, 1), eventV1(t, stream, 2)}
	next, err := contract.SealAgentRunEventCursorV1(contract.AgentRunEventCursorV1{StreamRef: stream, Sequence: contract.NewWireUint64V1(2)})
	if err != nil {
		t.Fatal(err)
	}
	result, err := contract.SealAgentRunWatchResultV1(contract.AgentRunWatchResultV1{
		RequestDigest:       request.RequestDigest,
		TraceID:             request.TraceID,
		StreamRef:           stream,
		RetentionCurrentRef: retention,
		Disposition:         contract.AgentRunWatchEventsV1,
		Events:              events,
		NextCursor:          &next,
		Window:              contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(130), ExpiresUnixNano: contract.NewWireUnixNanoV1(210)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(request); err != nil {
		t.Fatal(err)
	}
	old := result
	old.Window = contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(119), ExpiresUnixNano: contract.NewWireUnixNanoV1(210)}
	old.ResultDigest = ""
	old, err = contract.SealAgentRunWatchResultV1(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.ValidateFor(request); err == nil || !contract.HasCode(err, contract.FaultPreconditionFailedV1) {
		t.Fatalf("old Watch snapshot err=%v", err)
	}
}

func TestAgentRunWatchV1GapRequiresExactResyncCurrent(t *testing.T) {
	stream, retention := streamRefV1(t), retentionRefV1(t)
	request := watchRequestV1(t, stream, retention)
	gapEvent := eventV1(t, stream, 2)
	next, _ := contract.SealAgentRunEventCursorV1(contract.AgentRunEventCursorV1{StreamRef: stream, Sequence: contract.NewWireUint64V1(2)})
	gapResult, err := contract.SealAgentRunWatchResultV1(contract.AgentRunWatchResultV1{
		RequestDigest:       request.RequestDigest,
		TraceID:             request.TraceID,
		StreamRef:           stream,
		RetentionCurrentRef: retention,
		Disposition:         contract.AgentRunWatchEventsV1,
		Events:              []contract.AgentRunEventV1{gapEvent},
		NextCursor:          &next,
		Window:              contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(130), ExpiresUnixNano: contract.NewWireUnixNanoV1(210)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gapResult.ValidateFor(request); err == nil || !contract.HasCode(err, contract.FaultPreconditionFailedV1) {
		t.Fatalf("gap EVENTS err=%v", err)
	}

	fault := contract.FaultV1{
		Code:           contract.FaultPreconditionFailedV1,
		Reason:         "event_retention_gap",
		Message:        "event continuity cannot be proven",
		CurrentRef:     &retention,
		TraceID:        request.TraceID,
		RetryDirective: contract.RetryInspectV1,
	}
	resync, err := contract.SealAgentRunWatchResultV1(contract.AgentRunWatchResultV1{
		RequestDigest:       request.RequestDigest,
		TraceID:             request.TraceID,
		StreamRef:           stream,
		RetentionCurrentRef: retention,
		Disposition:         contract.AgentRunWatchResyncRequiredV1,
		Fault:               &fault,
		Window:              contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(130), ExpiresUnixNano: contract.NewWireUnixNanoV1(210)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := resync.ValidateFor(request); err != nil {
		t.Fatal(err)
	}
	wrongRetention := exactRefV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, "retention-splice", 4)
	resync.Fault.CurrentRef = &wrongRetention
	resync.ResultDigest = ""
	spliced, err := contract.SealAgentRunWatchResultV1(resync)
	if err != nil {
		t.Fatal(err)
	}
	if err := spliced.ValidateFor(request); err == nil || !contract.HasCode(err, contract.FaultPreconditionFailedV1) {
		t.Fatalf("retention current splice err=%v", err)
	}
}

func TestAgentRunWatchV1RejectsCursorRegressionAndUnknownDisposition(t *testing.T) {
	stream, retention := streamRefV1(t), retentionRefV1(t)
	cursor, _ := contract.SealAgentRunEventCursorV1(contract.AgentRunEventCursorV1{StreamRef: stream, Sequence: contract.NewWireUint64V1(1)})
	_, err := contract.SealAgentRunWatchRequestV1(contract.AgentRunWatchRequestV1{
		RequestID:           "watch-regression",
		TraceID:             "trace-1",
		Target:              targetV1(t),
		StreamRef:           stream,
		RetentionCurrentRef: retention,
		AfterSequence:       contract.NewWireUint64V1(2),
		Cursor:              &cursor,
		Limit:               contract.NewWireUint64V1(10),
		RequestedUnixNano:   contract.NewWireUnixNanoV1(120),
		NotAfterUnixNano:    contract.NewWireUnixNanoV1(220),
	})
	if err == nil || !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("cursor regression err=%v", err)
	}

	request := watchRequestV1(t, stream, retention)
	_, err = contract.SealAgentRunWatchResultV1(contract.AgentRunWatchResultV1{
		RequestDigest:       request.RequestDigest,
		TraceID:             request.TraceID,
		StreamRef:           stream,
		RetentionCurrentRef: retention,
		Disposition:         contract.AgentRunWatchDispositionV1("FUTURE_VALUE"),
		Window:              contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(130), ExpiresUnixNano: contract.NewWireUnixNanoV1(210)},
	})
	if err == nil || !contract.HasCode(err, contract.FaultInvalidArgumentV1) {
		t.Fatalf("unknown Watch disposition err=%v", err)
	}
}

func TestAgentRunWatchV1RejectsEventObservedAfterResultCheck(t *testing.T) {
	stream, retention := streamRefV1(t), retentionRefV1(t)
	request := watchRequestV1(t, stream, retention)
	event := eventV1(t, stream, 1)
	event.ObservedUnixNano = contract.NewWireUnixNanoV1(150)
	event.EventRef.Digest = ""
	event.EventDigest = ""
	event, err := contract.SealAgentRunEventV1(event)
	if err != nil {
		t.Fatal(err)
	}
	next, _ := contract.SealAgentRunEventCursorV1(contract.AgentRunEventCursorV1{StreamRef: stream, Sequence: contract.NewWireUint64V1(1)})
	_, err = contract.SealAgentRunWatchResultV1(contract.AgentRunWatchResultV1{
		RequestDigest:       request.RequestDigest,
		TraceID:             request.TraceID,
		StreamRef:           stream,
		RetentionCurrentRef: retention,
		Disposition:         contract.AgentRunWatchEventsV1,
		Events:              []contract.AgentRunEventV1{event},
		NextCursor:          &next,
		Window:              contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(130), ExpiresUnixNano: contract.NewWireUnixNanoV1(210)},
	})
	if err == nil || !contract.HasCode(err, contract.FaultPreconditionFailedV1) {
		t.Fatalf("future-observed event err=%v", err)
	}
}
