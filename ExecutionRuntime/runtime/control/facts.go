package control

import "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"

// Deprecated restricted aliases. Application production code imports the
// versioned runtime/ports seam; control remains Fact Owner implementation.
type DesiredExecutionState = ports.DesiredExecutionStateV2

const (
	DesiredStopped = ports.DesiredStoppedV2
	DesiredRunning = ports.DesiredRunningV2
	DesiredFenced  = ports.DesiredFencedV2
)

type DesiredStateSnapshot = ports.DesiredStateSnapshotV2
type DesiredStateMutation = ports.DesiredStateMutationV2
type CommandIntent = ports.ApplicationCommandIntentV2
type OutboxRecord = ports.ApplicationOutboxRecordV2
type CommandAcceptance = ports.ApplicationCommandAcceptanceV2
type CommandFactPort = ports.ApplicationCommandFactPortV2

func ValidDesiredState(value DesiredExecutionState) bool {
	return ports.ValidDesiredExecutionStateV2(value)
}
