package dataplaneadapter

import (
	"context"
	"errors"
	"strings"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
)

// WorkspaceReadActualPointAdapterV1 is the concrete Sandbox-internal bridge
// from the Go owner kernel to the Rust Data Plane Unix IPC. It is not a public
// Runtime or product API.
type WorkspaceReadActualPointAdapterV1 struct {
	Client Client
}

func NewWorkspaceReadActualPointAdapterV1(client Client) (*WorkspaceReadActualPointAdapterV1, error) {
	if strings.TrimSpace(client.SocketPath) == "" {
		return nil, errors.New("workspace read Data Plane socket is required")
	}
	return &WorkspaceReadActualPointAdapterV1{Client: client}, nil
}

func (a *WorkspaceReadActualPointAdapterV1) ReadWorkspaceFileV1(ctx context.Context, input kernel.WorkspaceReadActualPointRequestV1) (kernel.WorkspaceReadActualPointResultV1, error) {
	if a == nil {
		return kernel.WorkspaceReadActualPointResultV1{}, errors.New("workspace read Data Plane adapter is unavailable")
	}
	if input.Reservation.ValidateShape() != nil || input.Command.ValidateShape() != nil || input.Workspace.ValidateShape() != nil || input.RuntimeCurrent.Validate() != nil || input.S1CheckedUnixNano <= 0 || input.ExpiresUnixNano <= input.S1CheckedUnixNano {
		return kernel.WorkspaceReadActualPointResultV1{}, kernel.NewWorkspaceReadActualPointErrorV1(kernel.WorkspaceReadEffectNotStartedV1, errors.New("workspace read actual-point input is incomplete"))
	}
	workspace := ExactRefV1{
		ID: input.Workspace.Meta.ID, Revision: input.Workspace.Meta.Revision,
		Digest: input.Workspace.Meta.Digest, ExpiresUnixNano: input.Workspace.Meta.ExpiresUnixNano,
	}
	var expected *ExactRefV1
	if input.Command.ExpectedFileRef != nil {
		expected = &ExactRefV1{
			ID: input.Command.ExpectedFileRef.ID, Revision: input.Command.ExpectedFileRef.Revision,
			Digest: input.Command.ExpectedFileRef.Digest, ExpiresUnixNano: input.Workspace.Meta.ExpiresUnixNano,
		}
	}
	payload, err := NewWorkspaceReadPayloadV1(WorkspaceReadPayloadV1{
		WorkspaceBindingID: input.Workspace.Meta.ID,
		WorkspaceDigest:    input.Workspace.Meta.Digest,
		Workspace:          workspace,
		FileScopeDigest:    input.Command.FileScopeDigest,
		RelativePath:       input.Command.RelativePath,
		StartByte:          input.Command.StartByte,
		MaxBytes:           input.Command.MaxBytes,
		ExpectedFileRef:    expected,
		S1Checked:          true,
	})
	if err != nil {
		return kernel.WorkspaceReadActualPointResultV1{}, kernel.NewWorkspaceReadActualPointErrorV1(kernel.WorkspaceReadEffectNotStartedV1, err)
	}
	prepareCurrent, err := workspaceReadPrepareCurrentV1(input.RuntimeCurrent)
	if err != nil {
		return kernel.WorkspaceReadActualPointResultV1{}, kernel.NewWorkspaceReadActualPointErrorV1(kernel.WorkspaceReadEffectNotStartedV1, err)
	}
	prepareRequest, err := NewDispatchRequestV1(DispatchInput{
		RequestID:         input.Reservation.Meta.ID,
		Current:           prepareCurrent,
		EffectKind:        "praxis.sandbox/workspace-read",
		PayloadSchema:     input.Command.SourceToolPayloadSchema,
		PayloadRevision:   input.Command.SourceToolPayloadRevision,
		Payload:           payload,
		RequestedNotAfter: time.Unix(0, input.ExpiresUnixNano),
	})
	if err != nil {
		return kernel.WorkspaceReadActualPointResultV1{}, kernel.NewWorkspaceReadActualPointErrorV1(kernel.WorkspaceReadEffectNotStartedV1, err)
	}
	if err := a.ensureWorkspaceReadPrepareV1(ctx, prepareRequest); err != nil {
		return kernel.WorkspaceReadActualPointResultV1{}, kernel.NewWorkspaceReadActualPointErrorV1(kernel.WorkspaceReadEffectNotStartedV1, err)
	}
	executeRequest, err := NewDispatchRequestV1(DispatchInput{
		RequestID:         input.Reservation.Meta.ID,
		Current:           input.RuntimeCurrent,
		EffectKind:        "praxis.sandbox/workspace-read",
		PayloadSchema:     input.Command.SourceToolPayloadSchema,
		PayloadRevision:   input.Command.SourceToolPayloadRevision,
		Payload:           payload,
		RequestedNotAfter: time.Unix(0, input.ExpiresUnixNano),
	})
	if err != nil {
		return kernel.WorkspaceReadActualPointResultV1{}, kernel.NewWorkspaceReadActualPointErrorV1(kernel.WorkspaceReadEffectNotStartedV1, err)
	}
	return a.dispatchSealedWorkspaceReadV1(ctx, executeRequest, input.Reservation.StableKeyDigest)
}

func workspaceReadPrepareCurrentV1(current runtimeports.CurrentOperationDispatchEnforcementV4) (runtimeports.CurrentOperationDispatchEnforcementV4, error) {
	prepare, err := current.Journal.PhaseRefV4(runtimeports.OperationDispatchEnforcementPrepareV4)
	if err != nil {
		return runtimeports.CurrentOperationDispatchEnforcementV4{}, err
	}
	current.Phase = prepare
	return runtimeports.SealCurrentOperationDispatchEnforcementV4(current)
}

func (a *WorkspaceReadActualPointAdapterV1) ensureWorkspaceReadPrepareV1(ctx context.Context, request DispatchRequestV1) error {
	if _, err := a.Client.Dispatch(ctx, request); err == nil {
		return nil
	} else {
		recovered, inspectErr := a.Client.Inspect(ctx, request)
		if inspectErr == nil && recovered.Accepted {
			return nil
		}
		return err
	}
}

func (a *WorkspaceReadActualPointAdapterV1) dispatchSealedWorkspaceReadV1(ctx context.Context, request DispatchRequestV1, stableKeyDigest string) (kernel.WorkspaceReadActualPointResultV1, error) {
	response, err := a.Client.Dispatch(ctx, request)
	if err != nil {
		recovered, inspectErr := a.Client.Inspect(ctx, request)
		if inspectErr == nil && recovered.Accepted {
			response = recovered
		} else {
			var closed *ClosedError
			if errors.As(err, &closed) && workspaceReadNoEffectEvidenceV1(closed) {
				return kernel.WorkspaceReadActualPointResultV1{}, kernel.NewWorkspaceReadActualPointErrorV1(kernel.WorkspaceReadEffectNotStartedV1, err)
			}
			return kernel.WorkspaceReadActualPointResultV1{}, kernel.NewWorkspaceReadActualPointErrorV1(kernel.WorkspaceReadEffectStartedUnknownV1, err)
		}
	}
	if response.ProviderObservation == nil || response.ProviderObservation.WorkspaceRead == nil || response.ProviderReceipt == nil {
		return kernel.WorkspaceReadActualPointResultV1{}, errors.New("workspace read Data Plane response lacks exact observation and receipt")
	}
	observation := response.ProviderObservation.WorkspaceRead
	receipt := response.ProviderReceipt
	return kernel.WorkspaceReadActualPointResultV1{
		File:              contract.Ref{ID: observation.File.ID, Revision: observation.File.Revision, Digest: observation.File.Digest},
		Content:           observation.Content,
		ContentDigest:     observation.ContentDigest,
		StartByte:         observation.StartByte,
		ReturnedBytes:     observation.ReturnedBytes,
		TotalBytes:        observation.TotalBytes,
		Complete:          observation.Complete,
		ProviderS1Checked: observation.S1Checked,
		ProviderS2Checked: observation.S2Checked,
		PhysicalReadCount: observation.PhysicalReadCount,
		ProviderReceipt: contract.WorkspaceReadReceiptBindingV1{
			ID:              "workspace-read-provider-receipt-" + trimWorkspaceReadDigestV1(receipt.Digest),
			Revision:        1,
			Digest:          receipt.Digest,
			StableKeyDigest: stableKeyDigest,
			CheckedUnixNano: receipt.RecordedUnixNano,
			ExpiresUnixNano: receipt.ExpiresUnixNano,
		},
	}, nil
}

func workspaceReadNoEffectEvidenceV1(closed *ClosedError) bool {
	return closed != nil &&
		closed.EffectBoundary == "effect_not_started" &&
		closed.CrossedActualPoint != nil &&
		!*closed.CrossedActualPoint &&
		closed.PhysicalReadCount != nil &&
		*closed.PhysicalReadCount == 0
}

func trimWorkspaceReadDigestV1(value string) string {
	return strings.TrimPrefix(value, "sha256:")
}

var _ kernel.WorkspaceReadActualPointV1 = (*WorkspaceReadActualPointAdapterV1)(nil)
