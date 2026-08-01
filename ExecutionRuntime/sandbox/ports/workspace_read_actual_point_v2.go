package ports

import (
	"errors"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

// WorkspaceReadActualPointBoundaryV1 classifies whether the trusted Data Plane
// crossed the irreversible workspace-read boundary.
type WorkspaceReadActualPointBoundaryV1 string

const (
	WorkspaceReadEffectNotStartedV1     WorkspaceReadActualPointBoundaryV1 = "effect_not_started"
	WorkspaceReadEffectStartedUnknownV1 WorkspaceReadActualPointBoundaryV1 = "effect_started_unknown"
)

// WorkspaceReadActualPointErrorV1 is a neutral boundary error. A journal is
// evidence only; it is not an Owner write capability.
type WorkspaceReadActualPointErrorV1 struct {
	Boundary WorkspaceReadActualPointBoundaryV1
	Cause    error
	Journal  *contract.WorkspaceReadPhysicalJournalRefV2
}

func (e *WorkspaceReadActualPointErrorV1) Error() string {
	if e == nil || e.Cause == nil {
		return "workspace read actual-point failure"
	}
	return e.Cause.Error()
}

func (e *WorkspaceReadActualPointErrorV1) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewWorkspaceReadActualPointErrorV1(boundary WorkspaceReadActualPointBoundaryV1, cause error) error {
	if cause == nil {
		cause = errors.New("workspace read actual-point failure")
	}
	return &WorkspaceReadActualPointErrorV1{Boundary: boundary, Cause: cause}
}

func NewWorkspaceReadActualPointErrorWithJournalV2(boundary WorkspaceReadActualPointBoundaryV1, cause error, journal contract.WorkspaceReadPhysicalJournalRefV2) error {
	if cause == nil {
		cause = errors.New("workspace read actual-point failure")
	}
	if err := journal.Validate(); err != nil {
		return &WorkspaceReadActualPointErrorV1{Boundary: boundary, Cause: cause}
	}
	copy := journal
	return &WorkspaceReadActualPointErrorV1{Boundary: boundary, Cause: cause, Journal: &copy}
}

// WorkspaceReadActualPointRequestV1 contains the already-governed exact S1
// closure. It is neutral transport input and never grants dispatch by itself.
type WorkspaceReadActualPointRequestV1 struct {
	Reservation       contract.WorkspaceReadReservationV1
	Command           contract.WorkspaceReadCommandV1
	Workspace         contract.WorkspaceView
	RuntimeCurrent    runtimeports.CurrentOperationDispatchEnforcementV4
	CurrentQuery      WorkspaceReadCurrentQueryV2
	S1CheckedUnixNano int64
	ExpiresUnixNano   int64
}
