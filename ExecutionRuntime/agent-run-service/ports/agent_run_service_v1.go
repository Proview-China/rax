package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

// AgentRunServiceV1 is the horizontal composition/transport skeleton. Owner
// adapters remain responsible for Host lifecycle, Runtime Run/Outcome and
// Application command facts. Implementations return typed domain faults inside
// results; error is reserved for cancellation, decode failure or an inability
// to produce any contract result.
type AgentRunServiceV1 interface {
	NegotiateV1(context.Context, contract.NegotiationRequestV1) (contract.NegotiationResultV1, error)
	InspectAgentRunV1(context.Context, contract.AgentRunInspectRequestV1) (contract.AgentRunInspectResultV1, error)
	InspectOriginalV1(context.Context, contract.InspectOriginalRequestV1) (contract.InspectOriginalResultV1, error)
	WatchAgentRunV1(context.Context, contract.AgentRunWatchRequestV1) (contract.AgentRunWatchResultV1, error)
	CancelAgentRunV1(context.Context, contract.CancelAgentRunRequestV1) (contract.CommandResultV1, error)
	StopAgentHostV1(context.Context, contract.StopAgentHostRequestV1) (contract.CommandResultV1, error)
}

// StrictJSONDecoderV1 is a mandatory transport admission seam. A conforming
// implementation rejects oversized input, duplicate keys, unknown fields and
// trailing documents before any AgentRunServiceV1 method is invoked.
type StrictJSONDecoderV1 interface {
	DecodeStrictV1(payload []byte, target any) error
}
