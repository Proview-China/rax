package application

import (
	"context"
	"math"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type RecoveryCoordinatorConfigV2 struct {
	Recovery applicationports.WorkflowJournalRecoveryPortV2
	Facts    applicationports.WorkflowJournalFactPortV2
	Clock    func() time.Time
}

type RecoveryCoordinatorV2 struct{ config RecoveryCoordinatorConfigV2 }

func NewRecoveryCoordinatorV2(config RecoveryCoordinatorConfigV2) (*RecoveryCoordinatorV2, error) {
	if config.Recovery == nil || config.Facts == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "workflow recovery and journal Fact Ports are required")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &RecoveryCoordinatorV2{config: config}, nil
}

func (c *RecoveryCoordinatorV2) ListRecoverableV2(ctx context.Context, request applicationports.WorkflowJournalListRequestV2) ([]contract.WorkflowJournalV2, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return c.config.Recovery.ListWorkflowJournalsV2(ctx, request.Scope, request.AfterID, request.Limit)
}

// AcquireV2 persists or renews only a scheduler claim. It cannot be used as an
// Operation Permit and recovers an unknown reply solely by exact Inspect.
func (c *RecoveryCoordinatorV2) AcquireV2(ctx context.Context, request applicationports.WorkflowJournalClaimRequestV2) (applicationports.WorkflowJournalClaimV2, error) {
	if err := request.Validate(); err != nil {
		return applicationports.WorkflowJournalClaimV2{}, err
	}
	now := c.config.Clock()
	if now.IsZero() || now.UnixNano() <= 0 || request.LeaseNanos > math.MaxInt64-now.UnixNano() {
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "workflow recovery requires an injected clock and explicit positive lease")
	}
	journal, err := c.config.Facts.InspectWorkflowJournalV2(ctx, request.Scope, request.JournalID)
	if err != nil {
		return applicationports.WorkflowJournalClaimV2{}, err
	}
	claim, err := c.config.Recovery.ClaimWorkflowJournalV2(ctx, request)
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) {
			return applicationports.WorkflowJournalClaimV2{}, err
		}
		claim, err = c.config.Recovery.InspectWorkflowJournalClaimV2(context.WithoutCancel(ctx), request.Scope, request.JournalID)
		if err != nil {
			return applicationports.WorkflowJournalClaimV2{}, err
		}
	}
	if validateClaimReceiptV2(claim, journal, request, now) != nil {
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "workflow claim receipt differs from the exact requested CAS")
	}
	return claim, nil
}

func (c *RecoveryCoordinatorV2) ReleaseV2(ctx context.Context, request applicationports.WorkflowJournalReleaseRequestV2) (applicationports.WorkflowJournalClaimV2, error) {
	if err := request.Validate(); err != nil {
		return applicationports.WorkflowJournalClaimV2{}, err
	}
	journal, err := c.config.Facts.InspectWorkflowJournalV2(ctx, request.Scope, request.JournalID)
	if err != nil {
		return applicationports.WorkflowJournalClaimV2{}, err
	}
	released, err := c.config.Recovery.ReleaseWorkflowJournalClaimV2(ctx, request)
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) {
			return applicationports.WorkflowJournalClaimV2{}, err
		}
		released, err = c.config.Recovery.InspectWorkflowJournalClaimV2(context.WithoutCancel(ctx), request.Scope, request.JournalID)
		if err != nil {
			return applicationports.WorkflowJournalClaimV2{}, err
		}
	}
	if err := released.ValidateFor(journal); err != nil || !runtimeports.SameExecutionScopeV2(released.Scope, request.Scope) || released.JournalID != request.JournalID || released.OwnerID != request.OwnerID || released.Epoch != request.Epoch || released.Revision != request.ExpectedRevision+1 || released.State != applicationports.WorkflowJournalClaimReleasedV2 {
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "workflow claim release receipt differs from the exact requested CAS")
	}
	return released, nil
}

func validateClaimReceiptV2(claim applicationports.WorkflowJournalClaimV2, journal contract.WorkflowJournalV2, request applicationports.WorkflowJournalClaimRequestV2, now time.Time) error {
	if err := claim.ValidateFor(journal); err != nil {
		return err
	}
	if !runtimeports.SameExecutionScopeV2(claim.Scope, request.Scope) || claim.JournalID != request.JournalID || claim.OwnerID != request.OwnerID || claim.PolicyDigest != request.PolicyDigest || claim.Revision != request.ExpectedRevision+1 || claim.State != applicationports.WorkflowJournalClaimActiveV2 || claim.UpdatedUnixNano < now.UnixNano() || claim.ExpiresUnixNano-claim.UpdatedUnixNano != request.LeaseNanos {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "workflow claim does not bind requested owner, policy, revision and lifetime")
	}
	return nil
}
