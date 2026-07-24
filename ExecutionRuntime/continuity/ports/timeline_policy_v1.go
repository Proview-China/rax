package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

type TimelineProjectionPolicyCurrentReaderV1 interface {
	InspectTimelineProjectionPolicyCurrentV1(context.Context, string, string) (contract.TimelineProjectionPolicyCurrentV1, error)
	ValidateTimelineProjectionPolicyCurrentV1(context.Context, contract.TimelineProjectionPolicyCurrentV1) error
}

type TimelineProjectionPolicyRepositoryV1 interface {
	TimelineProjectionPolicyCurrentReaderV1
	CreateTimelineProjectionPolicyV1(context.Context, contract.TimelineProjectionPolicyCurrentV1) (contract.TimelineProjectionPolicyCurrentV1, bool, error)
	InspectTimelineProjectionPolicyV1(context.Context, contract.TimelineProjectionPolicyRefV1) (contract.TimelineProjectionPolicyCurrentV1, error)
	CompareAndSwapTimelineProjectionPolicyV1(context.Context, contract.TimelineProjectionPolicyRefV1, contract.TimelineProjectionPolicyCurrentV1) (contract.TimelineProjectionPolicyCurrentV1, error)
}
