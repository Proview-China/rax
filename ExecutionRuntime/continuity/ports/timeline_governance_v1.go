package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

type PublishTimelineProjectionV1Request struct {
	ExpectedAttempt contract.TimelineProjectionAttemptRefV1  `json:"expected_attempt"`
	VisibleAttempt  contract.TimelineProjectionAttemptFactV1 `json:"visible_attempt"`
	Event           contract.TimelineEventRecord             `json:"event"`
	Current         contract.TimelineProjectionCurrentV1     `json:"current"`
}

// TimelineGovernanceRepositoryV1 is the Continuity Fact Owner persistence
// boundary. Production callers use the domain Controller; implementations
// must nevertheless revalidate every CAS and atomic-publish invariant so a
// direct repository call cannot bypass governance.
type TimelineGovernanceRepositoryV1 interface {
	CreateTimelineProjectionAttemptV1(context.Context, contract.TimelineProjectionAttemptFactV1) (contract.TimelineProjectionAttemptFactV1, bool, error)
	InspectTimelineProjectionAttemptV1(context.Context, contract.TimelineProjectionAttemptRefV1) (contract.TimelineProjectionAttemptFactV1, error)
	InspectCurrentTimelineProjectionAttemptV1(context.Context, string, string) (contract.TimelineProjectionAttemptFactV1, error)
	CompareAndSwapTimelineProjectionAttemptV1(context.Context, contract.TimelineProjectionAttemptRefV1, contract.TimelineProjectionAttemptFactV1) (contract.TimelineProjectionAttemptFactV1, error)
	PublishTimelineProjectionV1(context.Context, PublishTimelineProjectionV1Request) (contract.TimelineProjectionAttemptFactV1, contract.TimelineProjectionCurrentV1, error)
	InspectTimelineProjectionCurrentV1(context.Context, contract.TimelineEventRefV1) (contract.TimelineProjectionCurrentV1, error)
}
