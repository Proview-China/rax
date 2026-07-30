package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

// CommandJournalDispositionV1 describes whether a command was newly reserved
// or is an exact replay of an already durable original command.
type CommandJournalDispositionV1 string

const (
	CommandJournalReservedV1 CommandJournalDispositionV1 = "RESERVED"
	CommandJournalReplayV1   CommandJournalDispositionV1 = "REPLAY"
)

// CommandJournalEntryV1 binds the exact original envelope to its durable
// receipt. A nil Receipt means that the original owner outcome is not yet
// recorded and callers must inspect rather than execute a second effect.
type CommandJournalEntryV1 struct {
	Command contract.AgentRunCommandEnvelopeV1
	Receipt *contract.AgentRunCommandReceiptV1
}

type CommandJournalV1 interface {
	ReserveCommandV1(context.Context, contract.AgentRunCommandEnvelopeV1) (CommandJournalDispositionV1, CommandJournalEntryV1, error)
	RecordReceiptV1(context.Context, contract.AgentRunCommandEnvelopeV1, contract.AgentRunCommandReceiptV1) error
	InspectReservedCommandV1(context.Context, contract.AgentRunCommandEnvelopeV1) (CommandJournalEntryV1, error)
	InspectCommandV1(context.Context, contract.InspectOriginalRequestV1) (CommandJournalEntryV1, error)
}

// CommandOwnerAdapterV1 executes one already admitted command. Any accepted
// invocation MUST return a sealed receipt for the exact original envelope,
// including UNKNOWN_OUTCOME or INDETERMINATE when the effect cannot be
// determined. Validation, admission, and owner failures must be encoded in a
// REJECTED receipt. A non-nil error means no trustworthy owner receipt could be
// produced; ServiceV1 converts that post-invocation boundary into a durable,
// exact-command UNKNOWN_OUTCOME receipt rather than retrying the effect.
type CommandOwnerAdapterV1 interface {
	ExecuteCommandV1(context.Context, contract.AgentRunCommandEnvelopeV1) (contract.AgentRunCommandReceiptV1, error)
}

type AgentRunInspectOwnerAdapterV1 interface {
	// Implementations return a sealed typed-fault result. Domain not-found or
	// indeterminate outcomes must not be reduced to error.
	InspectAgentRunV1(context.Context, contract.AgentRunInspectRequestV1) (contract.AgentRunInspectResultV1, error)
}

// OriginalAttemptOwnerAdapterV1 is intentionally separate from the horizontal
// command journal. Attempt facts remain owned by the injected domain adapter.
type OriginalAttemptOwnerAdapterV1 interface {
	// Implementations return a sealed typed-fault result. Domain not-found or
	// indeterminate outcomes must not be reduced to error.
	InspectOriginalAttemptV1(context.Context, contract.InspectOriginalRequestV1) (contract.InspectOriginalResultV1, error)
}

type AgentRunEventStreamV1 interface {
	// Implementations return a sealed typed TIMEOUT/RESYNC_REQUIRED/
	// INDETERMINATE result rather than a domain error.
	WatchAgentRunV1(context.Context, contract.AgentRunWatchRequestV1) (contract.AgentRunWatchResultV1, error)
}
