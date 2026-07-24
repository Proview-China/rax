package applicationadapter

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceCommitPlanV1 struct {
	Lifecycle         LifecyclePlanV4
	ExpectedView      contract.Ref
	ExpectedChangeSet contract.Ref
}

type WorkspaceCommitResultV1 struct {
	Lifecycle LifecycleResultV4
	ChangeSet contract.WorkspaceChangeSet
}

// WorkspaceCommitFlowV1 is the dedicated governed closure for a captured
// ChangeSet. It re-reads Sandbox current before both actual-point phases and
// advances the Owner current only after Runtime Settlement and Domain Apply.
type WorkspaceCommitFlowV1 struct {
	lifecycle *LifecycleFlowV4
	workspace ports.WorkspaceOwnerStoreV1
}

func NewWorkspaceCommitFlowV1(lifecycle *LifecycleFlowV4, workspace ports.WorkspaceOwnerStoreV1) (*WorkspaceCommitFlowV1, error) {
	if lifecycle == nil || nilLike(workspace) {
		return nil, errors.New("workspace commit flow requires lifecycle closure and Owner current store")
	}
	return &WorkspaceCommitFlowV1{lifecycle: lifecycle, workspace: workspace}, nil
}

func (f *WorkspaceCommitFlowV1) StartOrInspectWorkspaceCommitV1(ctx context.Context, plan WorkspaceCommitPlanV1) (WorkspaceCommitResultV1, error) {
	if f == nil || nilLike(ctx) {
		return WorkspaceCommitResultV1{}, errors.New("workspace commit flow or context is nil")
	}
	reservation, err := f.lifecycle.facts.GetReservation(ctx, plan.Lifecycle.ReservationID)
	if err != nil {
		return WorkspaceCommitResultV1{}, err
	}
	if err := reservation.ValidateCurrent(f.lifecycle.now()); err != nil || reservation.Kind != contract.EffectWorkspaceCommit {
		if err != nil {
			return WorkspaceCommitResultV1{}, err
		}
		return WorkspaceCommitResultV1{}, errors.New("workspace commit reservation is required")
	}
	if err := validateLifecyclePlanV4Mode(plan.Lifecycle, reservation, true); err != nil {
		return WorkspaceCommitResultV1{}, err
	}
	if err := plan.ExpectedView.ValidateShape("expected workspace view"); err != nil {
		return WorkspaceCommitResultV1{}, err
	}
	if err := plan.ExpectedChangeSet.ValidateShape("expected workspace change set"); err != nil {
		return WorkspaceCommitResultV1{}, err
	}
	readCurrent := func(readCtx context.Context) (contract.WorkspaceChangeSet, error) {
		view, err := f.workspace.InspectWorkspaceViewCurrentV1(readCtx, plan.ExpectedView)
		if err != nil {
			return contract.WorkspaceChangeSet{}, err
		}
		set, err := f.workspace.InspectWorkspaceChangeSetCurrentV1(readCtx, plan.ExpectedChangeSet)
		if errors.Is(err, ports.ErrConflict) {
			staged, historyErr := f.workspace.InspectWorkspaceChangeSetHistoryV1(readCtx, plan.ExpectedChangeSet)
			if historyErr != nil {
				return contract.WorkspaceChangeSet{}, err
			}
			set, err = f.workspace.InspectWorkspaceChangeSetOwnerCurrentByIDV1(readCtx, plan.ExpectedChangeSet.ID)
			if err != nil || !isExactWorkspaceCommitSuccessorV1(staged, set) {
				if err != nil {
					return contract.WorkspaceChangeSet{}, err
				}
				return contract.WorkspaceChangeSet{}, errors.New("workspace current is not the exact committed successor")
			}
		} else if err != nil {
			return contract.WorkspaceChangeSet{}, err
		}
		if err := validateWorkspaceCommitBindingsV1(reservation, view, set, plan.Lifecycle.Prepare, plan.Lifecycle.Execute); err != nil {
			return contract.WorkspaceChangeSet{}, err
		}
		return set, nil
	}
	validateCurrent := func(readCtx context.Context) error {
		_, err := readCurrent(readCtx)
		return err
	}
	if _, err := readCurrent(ctx); err != nil {
		return WorkspaceCommitResultV1{}, err
	}
	result, err := f.lifecycle.startOrInspectLifecycleV4(ctx, plan.Lifecycle, true, lifecycleHooksV4{
		beforePrepare: validateCurrent,
		beforeExecute: validateCurrent,
	})
	if err != nil {
		return WorkspaceCommitResultV1{}, err
	}
	observation := result.Execute.Response.ProviderObservation
	if observation == nil || observation.WorkspaceCommit == nil || observation.WorkspaceCommit.State != "committed" {
		return WorkspaceCommitResultV1{}, errors.New("workspace commit actual point lacks a committed exact observation")
	}
	current, err := readCurrent(ctx)
	if err != nil {
		return WorkspaceCommitResultV1{}, err
	}
	settlementRef := contract.RuntimeOperationSettlementRef{
		OpaqueRef:   contract.Ref{ID: result.Settlement.Settlement.ID, Revision: uint64(result.Settlement.Settlement.Revision), Digest: strings.TrimPrefix(string(result.Settlement.Settlement.Digest), "sha256:")},
		OperationID: result.DomainResult.OperationID, AttemptID: result.DomainResult.AttemptID, DomainResultRef: result.DomainResult.Meta.Ref(),
	}
	if current.State == contract.ChangeSetCommitted {
		if current.RuntimeSettlement == nil || *current.RuntimeSettlement != settlementRef || current.CommittedRevision != observation.WorkspaceCommit.CommittedRevision {
			return WorkspaceCommitResultV1{}, errors.New("committed workspace current binds another closure")
		}
		return WorkspaceCommitResultV1{Lifecycle: result, ChangeSet: current}, nil
	}
	next, err := contract.ApplyWorkspaceCommitSettlement(f.lifecycle.now(), current, result.DomainResult, contract.RuntimeOperationSettlementRef{
		OpaqueRef: settlementRef.OpaqueRef, OperationID: settlementRef.OperationID, AttemptID: settlementRef.AttemptID, DomainResultRef: settlementRef.DomainResultRef,
	}, observation.WorkspaceCommit.CommittedRevision)
	if err != nil {
		return WorkspaceCommitResultV1{}, err
	}
	if err := f.workspace.CompareAndSwapWorkspaceChangeSetV1(ctx, current.Meta.Ref(), next); err != nil {
		recovered, inspectErr := f.workspace.InspectWorkspaceChangeSetCurrentV1(context.WithoutCancel(ctx), next.Meta.Ref())
		if inspectErr != nil || !contract.SameRef(recovered.Meta.Ref(), next.Meta.Ref()) {
			return WorkspaceCommitResultV1{}, err
		}
		next = recovered
	}
	return WorkspaceCommitResultV1{Lifecycle: result, ChangeSet: next}, nil
}

func validateWorkspaceCommitBindingsV1(reservation contract.DomainReservation, view contract.WorkspaceView, set contract.WorkspaceChangeSet, prepare, execute ProviderPhasePlanV4) error {
	if view.ValidateShape() != nil || set.ValidateShape() != nil {
		// Exact current is checked by the Owner reader with its own clock. This
		// additional guard binds shape without inventing a second clock.
		return errors.New("workspace commit current projection is invalid")
	}
	if !contract.SameRef(set.ViewRef, view.Meta.Ref()) || set.BaseRevision != view.BaseRevision || set.BaseArtifactRef != view.BaseArtifactRef || (set.State != contract.ChangeSetStaged && set.State != contract.ChangeSetCommitted) || set.Meta.ExpiresUnixNano > view.Meta.ExpiresUnixNano || set.Meta.ExpiresUnixNano > view.Lease.ExpiresUnixNano || view.Lease != reservation.Lease {
		return errors.New("workspace current facts drifted from reservation or each other")
	}
	preparePayload, err := workspaceCommitPayloadFromPlanV1(prepare)
	if err != nil {
		return err
	}
	executePayload, err := workspaceCommitPayloadFromPlanV1(execute)
	if err != nil || !workspacePayloadEqualV1(preparePayload, executePayload) {
		return errors.New("workspace commit phases bind different payloads")
	}
	if preparePayload.ChangeSet.ID != set.Meta.ID || preparePayload.ChangeSet.Revision != set.Meta.Revision || strings.TrimPrefix(preparePayload.ChangeSet.Digest, "sha256:") != strings.TrimPrefix(set.Meta.Digest, "sha256:") || preparePayload.ChangeSet.ExpiresUnixNano != set.Meta.ExpiresUnixNano || preparePayload.View.ID != view.Meta.ID || preparePayload.View.Revision != view.Meta.Revision || strings.TrimPrefix(preparePayload.View.Digest, "sha256:") != strings.TrimPrefix(view.Meta.Digest, "sha256:") || preparePayload.View.ExpiresUnixNano != view.Meta.ExpiresUnixNano || strings.TrimPrefix(preparePayload.BaseRevision, "sha256:") != strings.TrimPrefix(set.BaseRevision, "sha256:") || strings.TrimPrefix(preparePayload.FileScopeDigest, "sha256:") != strings.TrimPrefix(view.FileScopeDigest, "sha256:") || !slices.Equal(preparePayload.WriteScopes, view.WriteScopes) || len(preparePayload.Changes) != len(set.Changes) {
		return errors.New("workspace commit payload differs from exact Owner current")
	}
	for index, change := range set.Changes {
		wire := preparePayload.Changes[index]
		if wire.Kind != string(change.Kind) || wire.Path != change.Path || wire.TargetPath != change.TargetPath {
			return errors.New("workspace commit mutation differs from exact ChangeSet")
		}
		if change.BlobRef == nil {
			if wire.BlobID != "" || wire.BlobDigest != "" || wire.Mode != 0 {
				return errors.New("workspace commit mutation added forbidden blob fields")
			}
		} else if wire.BlobID != change.BlobRef.ID || strings.TrimPrefix(wire.BlobDigest, "sha256:") != strings.TrimPrefix(change.BlobRef.Digest, "sha256:") || wire.Mode == 0 {
			return errors.New("workspace commit mutation blob differs from exact ChangeSet")
		}
	}
	return nil
}

func isExactWorkspaceCommitSuccessorV1(staged, current contract.WorkspaceChangeSet) bool {
	return staged.ValidateShape() == nil && current.ValidateShape() == nil && staged.State == contract.ChangeSetStaged && current.State == contract.ChangeSetCommitted &&
		current.Meta.ID == staged.Meta.ID && current.Meta.Revision == staged.Meta.Revision+1 && current.Meta.ExpiresUnixNano <= staged.Meta.ExpiresUnixNano &&
		contract.SameRef(current.ViewRef, staged.ViewRef) && current.BaseRevision == staged.BaseRevision && current.BaseArtifactRef == staged.BaseArtifactRef &&
		reflect.DeepEqual(current.Changes, staged.Changes) && slices.Equal(current.CanonicalPathSet, staged.CanonicalPathSet) && current.RuntimeSettlement != nil && current.CommittedRevision != ""
}

func workspaceCommitPayloadFromPlanV1(plan ProviderPhasePlanV4) (dataplaneadapter.WorkspaceCommitPayloadV1, error) {
	if plan.EffectKind != string(contract.EffectWorkspaceCommit) || plan.Payload.ProviderKind != "workspace_commit" {
		return dataplaneadapter.WorkspaceCommitPayloadV1{}, errors.New("workspace commit plan carries another Effect or Provider")
	}
	var payload dataplaneadapter.WorkspaceCommitPayloadV1
	if err := json.Unmarshal(plan.Payload.ProviderPayload, &payload); err != nil {
		return dataplaneadapter.WorkspaceCommitPayloadV1{}, errors.New("workspace commit Provider payload is malformed")
	}
	return payload, nil
}

func workspacePayloadEqualV1(left, right dataplaneadapter.WorkspaceCommitPayloadV1) bool {
	left.InspectionTarget = nil
	right.InspectionTarget = nil
	return left.WorkspaceBindingID == right.WorkspaceBindingID && left.WorkspaceDigest == right.WorkspaceDigest && left.ChangeSet == right.ChangeSet && left.View == right.View && left.BaseRevision == right.BaseRevision && left.FileScopeDigest == right.FileScopeDigest && slices.Equal(left.WriteScopes, right.WriteScopes) && slices.Equal(left.Changes, right.Changes)
}
