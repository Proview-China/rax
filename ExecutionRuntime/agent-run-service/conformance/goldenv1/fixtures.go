// Package goldenv1 builds deterministic cross-language JSON fixtures from the
// sealed AgentRunService V1 Go contracts. It is conformance data, not a second
// wire schema and not a page-facing DTO surface.
package goldenv1

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

const BaseUnixNanoV1 int64 = 1900000000000000000

type scalarCoverageV1 struct {
	Descriptor contract.CrossLanguageWirePrimitivesV1 `json:"descriptor"`
	Uint64Min  contract.WireUint64V1                  `json:"uint64_min"`
	Uint64Max  contract.WireUint64V1                  `json:"uint64_max"`
	Int64Min   contract.WireInt64V1                   `json:"int64_min"`
	Int64Max   contract.WireInt64V1                   `json:"int64_max"`
	UnixNano   contract.WireUnixNanoV1                `json:"unix_nano"`
	ExactRef   contract.ExactRefWireV1                `json:"exact_ref"`
}

// BuildV1 returns a new byte-for-byte stable fixture map on every call.
func BuildV1() map[string][]byte {
	target := targetV1()
	command := cancelCommandV1(target, "command-cancel-1", "idem-cancel-1", "operator_cancel", BaseUnixNanoV1)
	replay := command
	replay.CommandID = "command-cancel-replay-1"
	replay.SubmittedAtUnixNano = contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 1)
	replay.CanonicalPayloadDigest = ""
	replay.RequestDigest = ""
	replay = mustV1(contract.SealAgentRunCommandEnvelopeV1(replay))
	conflict := cancelCommandV1(target, "command-cancel-conflict-1", command.IdempotencyKey, "different_reason", BaseUnixNanoV1+2)
	cancelReceipt := receiptV1(command, BaseUnixNanoV1+10)
	cancelResult := mustV1(contract.SealCommandResultV1(contract.CommandResultV1{
		RequestDigest: command.RequestDigest, TraceID: command.TraceID, Receipt: cancelReceipt,
		Window: windowV1(BaseUnixNanoV1+10, BaseUnixNanoV1+900),
	}))

	stop := stopCommandV1(target)
	stopReceipt := receiptV1(stop, BaseUnixNanoV1+11)
	stopResult := mustV1(contract.SealCommandResultV1(contract.CommandResultV1{
		RequestDigest: stop.RequestDigest, TraceID: stop.TraceID, Receipt: stopReceipt,
		Window: windowV1(BaseUnixNanoV1+11, BaseUnixNanoV1+900),
	}))

	negotiationRequest := mustV1(contract.SealNegotiationRequestV1(contract.NegotiationRequestV1{
		RequestID: "negotiation-1", TraceID: "trace-negotiation-1",
		SupportedVersions:    []string{contract.AgentRunServiceContractVersionV1},
		RequiredCapabilities: []contract.CapabilityV1{contract.CapabilityAgentRunInspectV1, contract.CapabilityCommandInspectOriginalV1},
		OptionalCapabilities: []contract.CapabilityV1{contract.CapabilityAgentHostStopV1, contract.CapabilityAgentRunCancelV1, contract.CapabilityAgentRunWatchV1},
		RequestedUnixNano:    contract.NewWireUnixNanoV1(BaseUnixNanoV1), NotAfterUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 1000),
	}))
	negotiationResult := mustV1(contract.SealNegotiationResultV1(contract.NegotiationResultV1{
		RequestDigest: negotiationRequest.RequestDigest, TraceID: negotiationRequest.TraceID,
		Disposition: contract.NegotiationSelectedV1, SelectedVersion: contract.AgentRunServiceContractVersionV1,
		GrantedCapabilities: append(append([]contract.CapabilityV1{}, negotiationRequest.RequiredCapabilities...), negotiationRequest.OptionalCapabilities...),
		Window:              windowV1(BaseUnixNanoV1+1, BaseUnixNanoV1+900),
	}))

	inspectRequest := mustV1(contract.SealAgentRunInspectRequestV1(contract.AgentRunInspectRequestV1{
		RequestID: "inspect-agent-run-1", TraceID: "trace-inspect-agent-run-1", Target: target,
		RequestedUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1), NotAfterUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 1000),
	}))
	projection := mustV1(contract.SealOwnerProjectionV1(contract.OwnerProjectionV1{
		OwnerDomain: "praxis.runtime", OwnerContract: "praxis.runtime/run-lifecycle-v3",
		CurrentRef: target.RunCurrent, State: "running", Window: windowV1(BaseUnixNanoV1+5, BaseUnixNanoV1+900),
	}))
	inspectResult := mustV1(contract.SealAgentRunInspectResultV1(contract.AgentRunInspectResultV1{
		RequestDigest: inspectRequest.RequestDigest, TraceID: inspectRequest.TraceID, Target: target,
		Disposition: contract.AgentRunInspectObservedV1, Projections: []contract.OwnerProjectionV1{projection},
		Window: windowV1(BaseUnixNanoV1+5, BaseUnixNanoV1+900),
	}))

	commandRef := mustV1(command.CommandRefV1())
	inspectOriginalRequest := mustV1(contract.SealInspectOriginalRequestV1(contract.InspectOriginalRequestV1{
		RequestID: "inspect-original-1", TraceID: command.TraceID, OriginalCommandRef: &commandRef,
		OriginalIdempotencyKey: command.IdempotencyKey, OriginalRequestDigest: command.RequestDigest,
		RequestedUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 20), NotAfterUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 1000),
	}))
	inspectOriginalResult := mustV1(contract.SealInspectOriginalResultV1(contract.InspectOriginalResultV1{
		RequestDigest: inspectOriginalRequest.RequestDigest, TraceID: inspectOriginalRequest.TraceID,
		Disposition: contract.InspectOriginalObservedV1, CommandReceipt: &cancelReceipt,
		Window: windowV1(BaseUnixNanoV1+20, BaseUnixNanoV1+900),
	}))

	stream := exactRefV1(contract.AgentRunEventStreamRefKindV1, "stream-1", 1)
	retention := exactRefV1(contract.AgentRunEventRetentionCurrentRefKindV1, "retention-1", 4)
	watchRequest := watchRequestV1(target, stream, retention, nil, 0, "watch-events-1")
	event := eventV1(stream, target.RunCurrent, 1)
	cursor1 := mustV1(contract.SealAgentRunEventCursorV1(contract.AgentRunEventCursorV1{StreamRef: stream, Sequence: contract.NewWireUint64V1(1)}))
	watchResult := mustV1(contract.SealAgentRunWatchResultV1(contract.AgentRunWatchResultV1{
		RequestDigest: watchRequest.RequestDigest, TraceID: watchRequest.TraceID, StreamRef: stream,
		RetentionCurrentRef: retention, Disposition: contract.AgentRunWatchEventsV1,
		Events: []contract.AgentRunEventV1{event}, NextCursor: &cursor1,
		Window: windowV1(BaseUnixNanoV1+40, BaseUnixNanoV1+900),
	}))
	resyncRequest := watchRequestV1(target, stream, retention, &cursor1, 1, "watch-resync-1")
	resyncFault := contract.FaultV1{
		Code: contract.FaultPreconditionFailedV1, Reason: "event_retention_gap", Message: "event continuity cannot be proven",
		CurrentRef: &retention, TraceID: resyncRequest.TraceID, RetryDirective: contract.RetryInspectV1,
	}
	resyncResult := mustV1(contract.SealAgentRunWatchResultV1(contract.AgentRunWatchResultV1{
		RequestDigest: resyncRequest.RequestDigest, TraceID: resyncRequest.TraceID, StreamRef: stream,
		RetentionCurrentRef: retention, Disposition: contract.AgentRunWatchResyncRequiredV1, Fault: &resyncFault,
		Window: windowV1(BaseUnixNanoV1+40, BaseUnixNanoV1+900),
	}))

	fixtures := map[string]any{
		"wire-primitives-v1.json": scalarCoverageV1{
			Descriptor: contract.CanonicalCrossLanguageWirePrimitivesV1(),
			Uint64Min:  contract.NewWireUint64V1(0), Uint64Max: contract.NewWireUint64V1(^uint64(0)),
			Int64Min: contract.NewWireInt64V1(-1 << 63), Int64Max: contract.NewWireInt64V1(1<<63 - 1),
			UnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1), ExactRef: target.RunCurrent,
		},
		"negotiate-request.json": negotiationRequest, "negotiate-result.json": negotiationResult,
		"inspect-agent-run-request.json": inspectRequest, "inspect-agent-run-result.json": inspectResult,
		"inspect-original-request.json": inspectOriginalRequest, "inspect-original-result.json": inspectOriginalResult,
		"watch-agent-run-request.json": watchRequest, "watch-agent-run-result.json": watchResult,
		"watch-agent-run-resync-request.json": resyncRequest, "watch-agent-run-resync-result.json": resyncResult,
		"cancel-agent-run-request.json": contract.CancelAgentRunRequestV1{Command: command}, "cancel-agent-run-result.json": cancelResult,
		"stop-agent-host-request.json": contract.StopAgentHostRequestV1{Command: stop}, "stop-agent-host-result.json": stopResult,
		"idempotency-original.json": command, "idempotency-replay.json": replay, "idempotency-conflict.json": conflict,
		"fault-map-v1.json": faultMapV1(commandRef, retention),
	}
	encoded := make(map[string][]byte, len(fixtures))
	for name, value := range fixtures {
		payload, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			panic(err)
		}
		encoded[name] = append(payload, '\n')
	}
	return encoded
}

func NamesV1() []string {
	fixtures := BuildV1()
	names := make([]string, 0, len(fixtures))
	for name := range fixtures {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mustV1[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func digestV1(label string) contract.DigestV1 {
	return mustV1(contract.DigestJSONV1(struct {
		Label string `json:"label"`
	}{label}))
}

func exactRefV1(kind, id string, revision uint64) contract.ExactRefWireV1 {
	return contract.ExactRefWireV1{Kind: kind, ID: id, Revision: contract.NewWireUint64V1(revision), Digest: digestV1(kind + "/" + id)}
}

func targetV1() contract.AgentRunTargetV1 {
	return mustV1(contract.SealAgentRunTargetV1(contract.AgentRunTargetV1{
		HostID: "host-1", StartID: "start-1",
		HostStartClaim: exactRefV1(contract.AgentRunHostStartClaimKindV1, "claim-1", 1),
		ExecutionScope: exactRefV1(contract.AgentRunExecutionScopeKindV1, "scope-1", 7),
		RunCurrent:     exactRefV1(contract.AgentRunCurrentKindV1, "run-1", 9), AuthorityEpoch: contract.NewWireUint64V1(^uint64(0)),
	}))
}

func cancelCommandV1(target contract.AgentRunTargetV1, id, key, reason string, submitted int64) contract.AgentRunCommandEnvelopeV1 {
	return mustV1(contract.SealAgentRunCommandEnvelopeV1(contract.AgentRunCommandEnvelopeV1{
		CommandID: id, TraceID: "trace-cancel-1", Kind: contract.AgentRunCommandCancelRunV1, Target: target,
		Actor: "actor-1", AuthorityRef: exactRefV1("praxis.runtime/authority-current", "authority-1", 7),
		Payload: contract.AgentRunCommandPayloadV1{Reason: reason}, ExpectedCurrent: contract.ExpectedCurrentV1{Ref: target.RunCurrent, Epoch: target.AuthorityEpoch},
		IdempotencyKey: key, SubmittedAtUnixNano: contract.NewWireUnixNanoV1(submitted), NotAfterUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 1000),
	}))
}

func stopCommandV1(target contract.AgentRunTargetV1) contract.AgentRunCommandEnvelopeV1 {
	journal := exactRefV1("praxis.agent-host/journal", "journal-1", 4)
	cleanup := exactRefV1("praxis.agent-host/cleanup-closure", "closure-1", 2)
	return mustV1(contract.SealAgentRunCommandEnvelopeV1(contract.AgentRunCommandEnvelopeV1{
		CommandID: "command-stop-1", TraceID: "trace-stop-1", Kind: contract.AgentRunCommandStopHostV1, Target: target,
		Actor: "actor-1", AuthorityRef: exactRefV1("praxis.agent-host/authority-current", "host-authority-1", 7),
		Payload:         contract.AgentRunCommandPayloadV1{Reason: "operator_stop", HostJournal: &journal, CleanupClosure: &cleanup},
		ExpectedCurrent: contract.ExpectedCurrentV1{Ref: journal, Epoch: target.AuthorityEpoch}, IdempotencyKey: "idem-stop-1",
		SubmittedAtUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1), NotAfterUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 1000),
	}))
}

func receiptV1(command contract.AgentRunCommandEnvelopeV1, recorded int64) contract.AgentRunCommandReceiptV1 {
	ref := mustV1(command.CommandRefV1())
	return mustV1(contract.SealAgentRunCommandReceiptV1(contract.AgentRunCommandReceiptV1{
		CommandRef: ref, IdempotencyKey: command.IdempotencyKey, CanonicalPayloadDigest: command.CanonicalPayloadDigest,
		OriginalRequestDigest: command.RequestDigest, Status: contract.AgentRunCommandCompletedV1,
		TraceID: command.TraceID, RecordedUnixNano: contract.NewWireUnixNanoV1(recorded),
	}))
}

func watchRequestV1(target contract.AgentRunTargetV1, stream, retention contract.ExactRefWireV1, cursor *contract.AgentRunEventCursorV1, after uint64, id string) contract.AgentRunWatchRequestV1 {
	return mustV1(contract.SealAgentRunWatchRequestV1(contract.AgentRunWatchRequestV1{
		RequestID: id, TraceID: "trace-" + id, Target: target, StreamRef: stream, RetentionCurrentRef: retention,
		AfterSequence: contract.NewWireUint64V1(after), Cursor: cursor, Limit: contract.NewWireUint64V1(1000),
		RequestedUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 20), NotAfterUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 1000),
	}))
}

func eventV1(stream, subject contract.ExactRefWireV1, sequence uint64) contract.AgentRunEventV1 {
	wireSequence := contract.NewWireUint64V1(sequence)
	id := mustV1(contract.AgentRunEventIDV1(stream, wireSequence))
	return mustV1(contract.SealAgentRunEventV1(contract.AgentRunEventV1{
		StreamRef: stream, Sequence: wireSequence,
		EventRef:       contract.ExactRefWireV1{Kind: contract.AgentRunEventRefKindV1, ID: id, Revision: wireSequence},
		SourceOwnerRef: exactRefV1("praxis.runtime/run-event", fmt.Sprintf("owner-event-%d", sequence), sequence), SubjectRef: subject,
		Kind: "run.lifecycle.changed", PayloadVersion: "1.0.0", PayloadDigest: digestV1(fmt.Sprintf("payload-%d", sequence)),
		OccurredUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 30), ObservedUnixNano: contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 35),
	}))
}

func windowV1(checked, expires int64) contract.WireValidityWindowV1 {
	return contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(checked), ExpiresUnixNano: contract.NewWireUnixNanoV1(expires)}
}

func faultMapV1(commandRef, currentRef contract.ExactRefWireV1) []contract.FaultV1 {
	codes := []contract.FaultCodeV1{
		contract.FaultInvalidArgumentV1, contract.FaultUnauthenticatedV1, contract.FaultForbiddenV1, contract.FaultNotFoundV1,
		contract.FaultRevisionConflictV1, contract.FaultIdempotencyConflictV1, contract.FaultPreconditionFailedV1,
		contract.FaultCapabilityUnavailableV1, contract.FaultUnavailableV1, contract.FaultRateLimitedV1,
		contract.FaultUnknownOutcomeV1, contract.FaultIndeterminateV1, contract.FaultInternalV1,
	}
	faults := make([]contract.FaultV1, 0, len(codes))
	for _, code := range codes {
		fault := contract.FaultV1{Code: code, Reason: "fixture_" + strings.ToLower(string(code)), Message: "stable conformance fixture", TraceID: "trace-fault-map-1", RetryDirective: contract.RetryNoneV1}
		switch code {
		case contract.FaultIdempotencyConflictV1:
			fault.CommandRef = &commandRef
		case contract.FaultUnknownOutcomeV1, contract.FaultIndeterminateV1:
			fault.CommandRef = &commandRef
			fault.RetryDirective = contract.RetryInspectV1
		case contract.FaultRateLimitedV1:
			retryAfter := contract.NewWireUnixNanoV1(BaseUnixNanoV1 + 500)
			fault.RetryDirective = contract.RetryCommandV1
			fault.RetryAfterUnixNano = &retryAfter
		case contract.FaultPreconditionFailedV1:
			fault.CurrentRef = &currentRef
			fault.RetryDirective = contract.RetryInspectV1
		}
		if err := fault.Validate(); err != nil {
			panic(err)
		}
		faults = append(faults, fault)
	}
	return faults
}
