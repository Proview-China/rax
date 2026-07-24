package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

// SystemReadyAttemptFactPortV2 owns the H4 SystemReady step journal. Create is
// create-once and CAS always binds the exact expected revision and digest.
type SystemReadyAttemptFactPortV2 interface {
	CreateSystemReadyAttemptV2(context.Context, contract.SystemReadyAttemptFactV2) (contract.SystemReadyAttemptFactV2, error)
	CompareAndSwapSystemReadyAttemptV2(context.Context, contract.ExactRefV1, contract.SystemReadyAttemptFactV2) (contract.SystemReadyAttemptFactV2, error)
	InspectSystemReadyAttemptV2(context.Context, contract.SystemReadyAttemptStepKeyV2) (contract.SystemReadyAttemptFactV2, error)
}
