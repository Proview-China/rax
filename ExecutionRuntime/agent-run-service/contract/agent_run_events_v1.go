package contract

import "strings"

const (
	AgentRunEventStreamRefKindV1           = "praxis.agent-run-service/event-stream"
	AgentRunEventRetentionCurrentRefKindV1 = "praxis.agent-run-service/event-retention-current"
	AgentRunEventRefKindV1                 = "praxis.agent-run-service/event"
	MaxAgentRunWatchEventsV1               = uint64(1000)
)

type AgentRunEventCursorV1 struct {
	ContractVersion string         `json:"contract_version"`
	StreamRef       ExactRefWireV1 `json:"stream_ref"`
	Sequence        WireUint64V1   `json:"sequence"`
	CursorDigest    DigestV1       `json:"cursor_digest"`
}

func SealAgentRunEventCursorV1(cursor AgentRunEventCursorV1) (AgentRunEventCursorV1, error) {
	if err := validateOptionalContractVersionV1(cursor.ContractVersion, AgentRunServiceContractVersionV1); err != nil {
		return AgentRunEventCursorV1{}, err
	}
	cursor.ContractVersion = AgentRunServiceContractVersionV1
	provided := cursor.CursorDigest
	cursor.CursorDigest = ""
	digest, err := cursor.digestV1()
	if err != nil {
		return AgentRunEventCursorV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentRunEventCursorV1{}, NewError(FaultRevisionConflictV1, "event_cursor_digest_drift", "event cursor supplied a wrong digest")
	}
	cursor.CursorDigest = digest
	return cursor, cursor.Validate()
}

func (c AgentRunEventCursorV1) digestV1() (DigestV1, error) {
	clone := c
	clone.CursorDigest = ""
	return DigestJSONV1(struct {
		Domain string                `json:"domain"`
		Type   string                `json:"type"`
		Body   AgentRunEventCursorV1 `json:"body"`
	}{"praxis.agent-run-service.event-cursor", "AgentRunEventCursorV1", clone})
}

func (c AgentRunEventCursorV1) Validate() error {
	if c.ContractVersion != AgentRunServiceContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "event_cursor_version_invalid", "event cursor contract version is invalid")
	}
	if err := c.StreamRef.Validate(); err != nil {
		return err
	}
	if c.StreamRef.Kind != AgentRunEventStreamRefKindV1 {
		return NewError(FaultPreconditionFailedV1, "event_stream_ref_kind_drift", "event cursor stream kind drifted")
	}
	if _, err := c.Sequence.Uint64V1(); err != nil {
		return err
	}
	digest, err := c.digestV1()
	if err != nil || digest != c.CursorDigest {
		return NewError(FaultRevisionConflictV1, "event_cursor_digest_drift", "event cursor digest drifted")
	}
	return nil
}

// AgentRunEventV1 is an owner event projection, not a second event ledger.
type AgentRunEventV1 struct {
	ContractVersion  string         `json:"contract_version"`
	StreamRef        ExactRefWireV1 `json:"stream_ref"`
	Sequence         WireUint64V1   `json:"sequence"`
	EventRef         ExactRefWireV1 `json:"event_ref"`
	SourceOwnerRef   ExactRefWireV1 `json:"source_owner_ref"`
	SubjectRef       ExactRefWireV1 `json:"subject_ref"`
	Kind             string         `json:"kind"`
	PayloadVersion   string         `json:"payload_version"`
	PayloadDigest    DigestV1       `json:"payload_digest"`
	OccurredUnixNano WireUnixNanoV1 `json:"occurred_unix_nano"`
	ObservedUnixNano WireUnixNanoV1 `json:"observed_unix_nano"`
	EventDigest      DigestV1       `json:"event_digest"`
}

func SealAgentRunEventV1(event AgentRunEventV1) (AgentRunEventV1, error) {
	if err := validateOptionalContractVersionV1(event.ContractVersion, AgentRunServiceContractVersionV1); err != nil {
		return AgentRunEventV1{}, err
	}
	event.ContractVersion = AgentRunServiceContractVersionV1
	providedEventRefDigest, providedEventDigest := event.EventRef.Digest, event.EventDigest
	event.EventRef.Digest = ""
	event.EventDigest = ""
	digest, err := event.digestV1()
	if err != nil {
		return AgentRunEventV1{}, err
	}
	if (providedEventRefDigest != "" && providedEventRefDigest != digest) || (providedEventDigest != "" && providedEventDigest != digest) {
		return AgentRunEventV1{}, NewError(FaultRevisionConflictV1, "event_digest_drift", "Agent Run event supplied a wrong digest")
	}
	event.EventRef.Digest = digest
	event.EventDigest = digest
	return event, event.Validate()
}

func (e AgentRunEventV1) digestV1() (DigestV1, error) {
	clone := e
	clone.EventRef.Digest = ""
	clone.EventDigest = ""
	return DigestJSONV1(struct {
		Domain string          `json:"domain"`
		Type   string          `json:"type"`
		Body   AgentRunEventV1 `json:"body"`
	}{"praxis.agent-run-service.event", "AgentRunEventV1", clone})
}

func AgentRunEventIDV1(streamRef ExactRefWireV1, sequence WireUint64V1) (string, error) {
	if err := streamRef.Validate(); err != nil {
		return "", err
	}
	if streamRef.Kind != AgentRunEventStreamRefKindV1 {
		return "", NewError(FaultPreconditionFailedV1, "event_stream_ref_kind_drift", "event identity stream kind drifted")
	}
	if err := sequence.ValidatePositiveV1("event sequence"); err != nil {
		return "", err
	}
	digest, err := DigestJSONV1(struct {
		Domain   string         `json:"domain"`
		Stream   ExactRefWireV1 `json:"stream"`
		Sequence WireUint64V1   `json:"sequence"`
	}{"praxis.agent-run-service.event-identity", streamRef, sequence})
	if err != nil {
		return "", err
	}
	return "event/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func (e AgentRunEventV1) Validate() error {
	if e.ContractVersion != AgentRunServiceContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "event_version_invalid", "Agent Run event contract version is invalid")
	}
	for _, ref := range []ExactRefWireV1{e.StreamRef, e.EventRef, e.SourceOwnerRef, e.SubjectRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if e.StreamRef.Kind != AgentRunEventStreamRefKindV1 || e.EventRef.Kind != AgentRunEventRefKindV1 {
		return NewError(FaultPreconditionFailedV1, "event_ref_kind_drift", "Agent Run event exact ref kind drifted")
	}
	if err := e.Sequence.ValidatePositiveV1("event sequence"); err != nil {
		return err
	}
	if e.EventRef.Revision != e.Sequence {
		return NewError(FaultRevisionConflictV1, "event_revision_sequence_drift", "event ref revision differs from sequence")
	}
	expectedEventID, err := AgentRunEventIDV1(e.StreamRef, e.Sequence)
	if err != nil || e.EventRef.ID != expectedEventID {
		return NewError(FaultRevisionConflictV1, "event_id_coordinate_drift", "event ref id is not deterministic for stream and sequence")
	}
	if err := ValidateNamespacedIdentifierV1("event kind", e.Kind); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("event payload version", e.PayloadVersion); err != nil {
		return err
	}
	if err := e.PayloadDigest.Validate(); err != nil {
		return err
	}
	if err := e.OccurredUnixNano.ValidatePositiveV1("event occurred UnixNano"); err != nil {
		return err
	}
	if err := e.ObservedUnixNano.ValidatePositiveV1("event observed UnixNano"); err != nil {
		return err
	}
	occurred, _ := e.OccurredUnixNano.Int64V1()
	observed, _ := e.ObservedUnixNano.Int64V1()
	if observed < occurred {
		return NewError(FaultPreconditionFailedV1, "event_observation_clock_regression", "event observation is before owner occurrence")
	}
	digest, err := e.digestV1()
	if err != nil || digest != e.EventDigest || digest != e.EventRef.Digest {
		return NewError(FaultRevisionConflictV1, "event_digest_drift", "Agent Run event digest drifted")
	}
	return nil
}

func ValidateAgentRunEventReplayV1(original, replay AgentRunEventV1) error {
	if err := original.Validate(); err != nil {
		return err
	}
	if err := replay.Validate(); err != nil {
		return err
	}
	if original.StreamRef != replay.StreamRef || original.Sequence != replay.Sequence {
		return NewError(FaultInvalidArgumentV1, "event_replay_coordinate_mismatch", "event replay must use the same stream and sequence")
	}
	if original.EventRef != replay.EventRef || original.EventDigest != replay.EventDigest {
		return NewError(FaultRevisionConflictV1, "event_replay_conflict", "same stream sequence was replayed with different exact event content")
	}
	return nil
}

type AgentRunWatchRequestV1 struct {
	ContractVersion     string                 `json:"contract_version"`
	RequestID           string                 `json:"request_id"`
	TraceID             string                 `json:"trace_id"`
	Target              AgentRunTargetV1       `json:"target"`
	StreamRef           ExactRefWireV1         `json:"stream_ref"`
	RetentionCurrentRef ExactRefWireV1         `json:"retention_current_ref"`
	AfterSequence       WireUint64V1           `json:"after_sequence"`
	Cursor              *AgentRunEventCursorV1 `json:"cursor,omitempty"`
	Limit               WireUint64V1           `json:"limit"`
	RequestedUnixNano   WireUnixNanoV1         `json:"requested_unix_nano"`
	NotAfterUnixNano    WireUnixNanoV1         `json:"not_after_unix_nano"`
	RequestDigest       DigestV1               `json:"request_digest"`
}

func SealAgentRunWatchRequestV1(request AgentRunWatchRequestV1) (AgentRunWatchRequestV1, error) {
	if err := validateOptionalContractVersionV1(request.ContractVersion, AgentRunServiceContractVersionV1); err != nil {
		return AgentRunWatchRequestV1{}, err
	}
	request.ContractVersion = AgentRunServiceContractVersionV1
	provided := request.RequestDigest
	request.RequestDigest = ""
	digest, err := request.digestV1()
	if err != nil {
		return AgentRunWatchRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentRunWatchRequestV1{}, NewError(FaultRevisionConflictV1, "watch_request_digest_drift", "Watch request supplied a wrong digest")
	}
	request.RequestDigest = digest
	return request, request.Validate()
}

func (r AgentRunWatchRequestV1) digestV1() (DigestV1, error) {
	clone := r
	clone.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string                 `json:"domain"`
		Type   string                 `json:"type"`
		Body   AgentRunWatchRequestV1 `json:"body"`
	}{"praxis.agent-run-service.watch", "AgentRunWatchRequestV1", clone})
}

func (r AgentRunWatchRequestV1) Validate() error {
	if r.ContractVersion != AgentRunServiceContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "watch_version_invalid", "Agent Run Watch contract version is invalid")
	}
	for field, value := range map[string]string{"request id": r.RequestID, "trace id": r.TraceID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if err := r.Target.Validate(); err != nil {
		return err
	}
	if err := r.StreamRef.Validate(); err != nil {
		return err
	}
	if r.StreamRef.Kind != AgentRunEventStreamRefKindV1 {
		return NewError(FaultPreconditionFailedV1, "watch_stream_ref_kind_drift", "Watch stream ref kind drifted")
	}
	if err := r.RetentionCurrentRef.Validate(); err != nil {
		return err
	}
	if r.RetentionCurrentRef.Kind != AgentRunEventRetentionCurrentRefKindV1 {
		return NewError(FaultPreconditionFailedV1, "watch_retention_current_kind_drift", "Watch retention current ref kind drifted")
	}
	after, err := r.AfterSequence.Uint64V1()
	if err != nil {
		return err
	}
	limit, err := r.Limit.Uint64V1()
	if err != nil || limit == 0 || limit > MaxAgentRunWatchEventsV1 {
		return NewError(FaultInvalidArgumentV1, "watch_limit_invalid", "Watch limit must be between 1 and 1000")
	}
	if r.Cursor == nil {
		if after != 0 {
			return NewError(FaultPreconditionFailedV1, "watch_cursor_missing", "non-zero afterSequence requires an exact cursor")
		}
	} else {
		if err := r.Cursor.Validate(); err != nil {
			return err
		}
		cursorSequence, _ := r.Cursor.Sequence.Uint64V1()
		if r.Cursor.StreamRef != r.StreamRef || cursorSequence != after {
			return NewError(FaultRevisionConflictV1, "watch_cursor_drift", "Watch cursor differs from stream or afterSequence")
		}
	}
	if err := r.RequestedUnixNano.ValidatePositiveV1("Watch requested UnixNano"); err != nil {
		return err
	}
	if err := r.NotAfterUnixNano.ValidatePositiveV1("Watch not-after UnixNano"); err != nil {
		return err
	}
	requested, _ := r.RequestedUnixNano.Int64V1()
	notAfter, _ := r.NotAfterUnixNano.Int64V1()
	if notAfter <= requested {
		return NewError(FaultInvalidArgumentV1, "watch_window_invalid", "Watch not-after must be after request")
	}
	digest, err := r.digestV1()
	if err != nil || digest != r.RequestDigest {
		return NewError(FaultRevisionConflictV1, "watch_request_digest_drift", "Watch request digest drifted")
	}
	return nil
}

func (r AgentRunWatchRequestV1) ValidateCurrentV1(now WireUnixNanoV1) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := now.ValidatePositiveV1("current UnixNano"); err != nil {
		return err
	}
	requested, _ := r.RequestedUnixNano.Int64V1()
	notAfter, _ := r.NotAfterUnixNano.Int64V1()
	current, _ := now.Int64V1()
	if current < requested {
		return NewError(FaultPreconditionFailedV1, "clock_regression", "Watch was checked before request")
	}
	if current >= notAfter {
		return NewError(FaultPreconditionFailedV1, "watch_request_expired", "Watch reached or exceeded not-after")
	}
	return nil
}

type AgentRunWatchDispositionV1 string

const (
	AgentRunWatchEventsV1         AgentRunWatchDispositionV1 = "EVENTS"
	AgentRunWatchTimeoutV1        AgentRunWatchDispositionV1 = "TIMEOUT"
	AgentRunWatchResyncRequiredV1 AgentRunWatchDispositionV1 = "RESYNC_REQUIRED"
	AgentRunWatchIndeterminateV1  AgentRunWatchDispositionV1 = "INDETERMINATE"
)

type AgentRunWatchResultV1 struct {
	ContractVersion     string                     `json:"contract_version"`
	RequestDigest       DigestV1                   `json:"request_digest"`
	TraceID             string                     `json:"trace_id"`
	StreamRef           ExactRefWireV1             `json:"stream_ref"`
	RetentionCurrentRef ExactRefWireV1             `json:"retention_current_ref"`
	Disposition         AgentRunWatchDispositionV1 `json:"disposition"`
	Events              []AgentRunEventV1          `json:"events"`
	NextCursor          *AgentRunEventCursorV1     `json:"next_cursor,omitempty"`
	Fault               *FaultV1                   `json:"fault,omitempty"`
	Window              WireValidityWindowV1       `json:"window"`
	ResultDigest        DigestV1                   `json:"result_digest"`
}

func SealAgentRunWatchResultV1(result AgentRunWatchResultV1) (AgentRunWatchResultV1, error) {
	if err := validateOptionalContractVersionV1(result.ContractVersion, AgentRunServiceContractVersionV1); err != nil {
		return AgentRunWatchResultV1{}, err
	}
	result.ContractVersion = AgentRunServiceContractVersionV1
	result.Events = append([]AgentRunEventV1(nil), result.Events...)
	provided := result.ResultDigest
	result.ResultDigest = ""
	digest, err := result.digestV1()
	if err != nil {
		return AgentRunWatchResultV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentRunWatchResultV1{}, NewError(FaultRevisionConflictV1, "watch_result_digest_drift", "Watch result supplied a wrong digest")
	}
	result.ResultDigest = digest
	return result, result.Validate()
}

func (r AgentRunWatchResultV1) digestV1() (DigestV1, error) {
	clone := r
	clone.ResultDigest = ""
	return DigestJSONV1(struct {
		Domain string                `json:"domain"`
		Type   string                `json:"type"`
		Body   AgentRunWatchResultV1 `json:"body"`
	}{"praxis.agent-run-service.watch", "AgentRunWatchResultV1", clone})
}

func (r AgentRunWatchResultV1) Validate() error {
	if r.ContractVersion != AgentRunServiceContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "watch_result_version_invalid", "Watch result contract version is invalid")
	}
	if err := r.RequestDigest.Validate(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("trace id", r.TraceID); err != nil {
		return err
	}
	if err := r.StreamRef.Validate(); err != nil {
		return err
	}
	if r.StreamRef.Kind != AgentRunEventStreamRefKindV1 {
		return NewError(FaultPreconditionFailedV1, "watch_result_stream_kind_drift", "Watch result stream kind drifted")
	}
	if err := r.RetentionCurrentRef.Validate(); err != nil {
		return err
	}
	if r.RetentionCurrentRef.Kind != AgentRunEventRetentionCurrentRefKindV1 {
		return NewError(FaultPreconditionFailedV1, "watch_result_retention_kind_drift", "Watch result retention current ref kind drifted")
	}
	if err := r.Window.Validate(); err != nil {
		return err
	}
	for _, event := range r.Events {
		if err := event.Validate(); err != nil {
			return err
		}
		if event.StreamRef != r.StreamRef {
			return NewError(FaultRevisionConflictV1, "watch_event_stream_drift", "Watch event belongs to another stream")
		}
		eventObserved, _ := event.ObservedUnixNano.Int64V1()
		resultChecked, _ := r.Window.CheckedUnixNano.Int64V1()
		if eventObserved > resultChecked {
			return NewError(FaultPreconditionFailedV1, "watch_event_after_result_check", "Watch event was observed after the result checked watermark")
		}
	}
	if r.NextCursor != nil {
		if err := r.NextCursor.Validate(); err != nil {
			return err
		}
		if r.NextCursor.StreamRef != r.StreamRef {
			return NewError(FaultRevisionConflictV1, "watch_next_cursor_stream_drift", "Watch next cursor belongs to another stream")
		}
	}
	if r.Fault != nil {
		if err := r.Fault.Validate(); err != nil {
			return err
		}
		if r.Fault.TraceID != r.TraceID {
			return NewError(FaultRevisionConflictV1, "watch_fault_trace_drift", "Watch fault trace drifted")
		}
	}
	switch r.Disposition {
	case AgentRunWatchEventsV1:
		if len(r.Events) == 0 || r.NextCursor == nil || r.Fault != nil {
			return NewError(FaultPreconditionFailedV1, "watch_events_result_incomplete", "EVENTS result requires events, next cursor and no fault")
		}
	case AgentRunWatchTimeoutV1:
		if len(r.Events) != 0 || r.NextCursor == nil || r.Fault != nil {
			return NewError(FaultPreconditionFailedV1, "watch_timeout_result_invalid", "TIMEOUT result requires a cursor and no events or fault")
		}
	case AgentRunWatchResyncRequiredV1:
		if len(r.Events) != 0 || r.NextCursor != nil || r.Fault == nil || r.Fault.Code != FaultPreconditionFailedV1 || r.Fault.RetryDirective != RetryInspectV1 || r.Fault.CurrentRef == nil {
			return NewError(FaultPreconditionFailedV1, "watch_resync_result_invalid", "RESYNC_REQUIRED requires an exact current and Inspect instruction")
		}
	case AgentRunWatchIndeterminateV1:
		if len(r.Events) != 0 || r.NextCursor != nil || r.Fault == nil || (r.Fault.Code != FaultUnavailableV1 && r.Fault.Code != FaultInternalV1) {
			return NewError(FaultPreconditionFailedV1, "watch_indeterminate_result_invalid", "INDETERMINATE Watch requires a typed availability fault")
		}
	default:
		return NewError(FaultInvalidArgumentV1, "watch_disposition_invalid", "Watch disposition is invalid")
	}
	digest, err := r.digestV1()
	if err != nil || digest != r.ResultDigest {
		return NewError(FaultRevisionConflictV1, "watch_result_digest_drift", "Watch result digest drifted")
	}
	return nil
}

func (r AgentRunWatchResultV1) ValidateFor(request AgentRunWatchRequestV1) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if r.RequestDigest != request.RequestDigest || r.TraceID != request.TraceID || r.StreamRef != request.StreamRef || r.RetentionCurrentRef != request.RetentionCurrentRef {
		return NewError(FaultRevisionConflictV1, "watch_result_request_drift", "Watch result belongs to another request or stream")
	}
	if err := r.Window.ValidateWithinRequestV1(request.RequestedUnixNano, request.NotAfterUnixNano); err != nil {
		return err
	}
	after, _ := request.AfterSequence.Uint64V1()
	limit, _ := request.Limit.Uint64V1()
	switch r.Disposition {
	case AgentRunWatchEventsV1:
		if len(r.Events) == 0 || uint64(len(r.Events)) > limit || r.NextCursor == nil || r.Fault != nil {
			return NewError(FaultPreconditionFailedV1, "watch_events_result_incomplete", "EVENTS result requires bounded events, next cursor and no fault")
		}
		expected := after + 1
		if after == ^uint64(0) {
			return NewError(FaultPreconditionFailedV1, "watch_sequence_exhausted", "afterSequence cannot advance beyond uint64 max")
		}
		for _, event := range r.Events {
			sequence, _ := event.Sequence.Uint64V1()
			if sequence != expected {
				return NewError(FaultPreconditionFailedV1, "event_sequence_gap_requires_resync", "event sequence gap must be returned as RESYNC_REQUIRED")
			}
			expected++
		}
		last, _ := r.Events[len(r.Events)-1].Sequence.Uint64V1()
		next, _ := r.NextCursor.Sequence.Uint64V1()
		if next != last {
			return NewError(FaultRevisionConflictV1, "watch_next_cursor_sequence_drift", "next cursor does not bind the last event sequence")
		}
	case AgentRunWatchTimeoutV1:
		if len(r.Events) != 0 || r.NextCursor == nil || r.Fault != nil {
			return NewError(FaultPreconditionFailedV1, "watch_timeout_result_invalid", "TIMEOUT result requires unchanged cursor and no events or fault")
		}
		next, _ := r.NextCursor.Sequence.Uint64V1()
		if next != after {
			return NewError(FaultRevisionConflictV1, "watch_timeout_cursor_advanced", "TIMEOUT cannot advance cursor")
		}
	case AgentRunWatchResyncRequiredV1:
		if len(r.Events) != 0 || r.NextCursor != nil || r.Fault == nil || r.Fault.Code != FaultPreconditionFailedV1 || r.Fault.RetryDirective != RetryInspectV1 || r.Fault.CurrentRef == nil || *r.Fault.CurrentRef != request.RetentionCurrentRef {
			return NewError(FaultPreconditionFailedV1, "watch_resync_result_invalid", "RESYNC_REQUIRED must carry no events/cursor and instruct Inspect")
		}
	case AgentRunWatchIndeterminateV1:
		if len(r.Events) != 0 || r.NextCursor != nil || r.Fault == nil || (r.Fault.Code != FaultUnavailableV1 && r.Fault.Code != FaultInternalV1) {
			return NewError(FaultPreconditionFailedV1, "watch_indeterminate_result_invalid", "INDETERMINATE Watch requires a typed availability fault")
		}
	default:
		return NewError(FaultInvalidArgumentV1, "watch_disposition_invalid", "Watch disposition is invalid")
	}
	return nil
}
