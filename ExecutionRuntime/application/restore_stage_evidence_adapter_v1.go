package application

import (
	"context"
	"errors"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreStageEvidenceAdapterV1 maps the Application request to Runtime's
// additive Restore Evidence governance port. Runtime and the Sandbox Owner
// derive all trusted payload/source fields; Application only sequences them.
type RestoreStageEvidenceAdapterV1 struct {
	runtime runtimeports.RestoreStageEvidenceGovernancePortV1
	clock   func() time.Time
}

func NewRestoreStageEvidenceAdapterV1(runtime runtimeports.RestoreStageEvidenceGovernancePortV1, clock func() time.Time) (*RestoreStageEvidenceAdapterV1, error) {
	if restoreExecutionNilV1(runtime) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Restore Stage Evidence Runtime port and clock are required")
	}
	return &RestoreStageEvidenceAdapterV1{runtime: runtime, clock: clock}, nil
}

func (a *RestoreStageEvidenceAdapterV1) PublishRestoreStageEvidenceV1(ctx context.Context, request applicationcontract.RestoreStageEvidenceRequestV1) (runtimeports.EvidenceRecordRefV2, error) {
	if a == nil || restoreExecutionNilV1(ctx) {
		return runtimeports.EvidenceRecordRefV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore Stage Evidence adapter or context is nil")
	}
	now := a.clock()
	if err := request.Validate(now); err != nil {
		return runtimeports.EvidenceRecordRefV2{}, err
	}
	runtimeRequest := runtimeports.PublishRestoreStageEvidenceRequestV1{Governance: request.Governance, DomainResult: request.DomainResult, SourceRegistration: request.SourceRegistration}
	ref, publishErr := a.runtime.PublishRestoreStageEvidenceV1(ctx, runtimeRequest)
	if publishErr != nil {
		record, inspectErr := a.runtime.InspectRestoreStageEvidenceV1(context.WithoutCancel(ctx), runtimeRequest)
		if inspectErr != nil {
			return runtimeports.EvidenceRecordRefV2{}, errors.Join(publishErr, inspectErr)
		}
		ref = record.Ref
	}
	if ref.Validate() != nil {
		return runtimeports.EvidenceRecordRefV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Runtime Restore Stage Evidence returned an invalid exact ref")
	}
	return ref, nil
}

var _ applicationports.RestoreStageEvidencePublisherV1 = (*RestoreStageEvidenceAdapterV1)(nil)
