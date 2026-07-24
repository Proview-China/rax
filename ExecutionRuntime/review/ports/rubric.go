package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type PublishRubricMutationV1 struct {
	Expected *contract.ExactResourceRefV1 `json:"expected,omitempty"`
	Next     contract.RubricDefinitionV1  `json:"next"`
}

type RevokeRubricMutationV1 struct {
	Expected contract.ExactResourceRefV1 `json:"expected"`
	Next     contract.RubricDefinitionV1 `json:"next"`
}

// RubricCurrentReaderV1 is the only Request-admission currentness seam. It
// atomically verifies the current index against the caller's full exact ref;
// time currentness is then evaluated with the supplied fresh clock value.
type RubricCurrentReaderV1 interface {
	InspectRubricCurrentV1(context.Context, core.TenantID, contract.ExactResourceRefV1, time.Time) (contract.RubricDefinitionV1, error)
}

type RubricStoreV1 interface {
	RubricCurrentReaderV1
	PublishRubricV1(context.Context, PublishRubricMutationV1) (contract.RubricDefinitionV1, error)
	RevokeRubricV1(context.Context, RevokeRubricMutationV1) (contract.RubricDefinitionV1, error)
	InspectRubricExactV1(context.Context, core.TenantID, contract.ExactResourceRefV1) (contract.RubricDefinitionV1, error)
}
