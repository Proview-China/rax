// Package control retains restricted legacy aliases for command persistence.
// New Application code imports runtime/ports Application*V2 contracts only.
package control

import "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"

type CommandKind = ports.ApplicationCommandKindV2

const (
	CommandStart         = ports.ApplicationCommandStartV2
	CommandResume        = ports.ApplicationCommandResumeV2
	CommandStopInstance  = ports.ApplicationCommandStopInstanceV2
	CommandFence         = ports.ApplicationCommandFenceV2
	CommandRevoke        = ports.ApplicationCommandRevokeV2
	CommandProvideInput  = ports.ApplicationCommandProvideInputV2
	CommandCancelRun     = ports.ApplicationCommandCancelRunV2
	CommandApproveEffect = ports.ApplicationCommandApproveEffectV2
	CommandDenyEffect    = ports.ApplicationCommandDenyEffectV2
)

type CommandStatus = ports.ApplicationCommandStatusV2

const (
	CommandAccepted      = ports.ApplicationCommandAcceptedV2
	CommandRejected      = ports.ApplicationCommandRejectedV2
	CommandExecuting     = ports.ApplicationCommandExecutingV2
	CommandCompleted     = ports.ApplicationCommandCompletedV2
	CommandSuperseded    = ports.ApplicationCommandSupersededV2
	CommandInvalidated   = ports.ApplicationCommandInvalidatedV2
	CommandIndeterminate = ports.ApplicationCommandIndeterminateV2
)

type CommandEnvelope = ports.ApplicationCommandEnvelopeV2
type CommandRecord = ports.ApplicationCommandRecordV2

func Supersedes(newer, older CommandEnvelope) bool { return ports.CommandSupersedesV2(newer, older) }
