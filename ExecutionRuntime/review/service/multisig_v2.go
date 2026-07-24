package service

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type HumanMultiSignCommandsV2 interface {
	OpenPanelV2(context.Context, reviewport.CreateHumanPanelMutationV2) (reviewport.CreateHumanPanelResultV2, error)
	SubmitAttestationV2(context.Context, reviewport.RecordHumanAttestationMutationV2) (reviewport.RecordHumanAttestationResultV2, error)
}

type HumanMultiSignClaimCommandsV2 interface {
	ClaimAssignmentV2(context.Context, reviewport.ClaimHumanAssignmentMutationV2, reviewport.HumanOrganizationCurrentRequestV2) (reviewport.ClaimHumanAssignmentResultV2, error)
}

// HumanMultiSignServiceV2 exposes controlled Panel/Vote commands and reads.
// It deliberately has no method accepting a HumanVerdictV2.
type HumanMultiSignServiceV2 struct {
	commands HumanMultiSignCommandsV2
	claims   HumanMultiSignClaimCommandsV2
	store    reviewport.StoreV2
}

func NewHumanMultiSignV2(commands HumanMultiSignCommandsV2, store reviewport.StoreV2) (*HumanMultiSignServiceV2, error) {
	if nilcheck.IsNil(commands) || nilcheck.IsNil(store) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "human multisign service requires commands and Store")
	}
	return &HumanMultiSignServiceV2{commands: commands, store: store}, nil
}

// NewHumanMultiSignProductionV2 requires the Organization-current-backed
// Claim Owner. The legacy constructor remains available to owner-local test
// fixtures but its service deliberately cannot claim an Assignment.
func NewHumanMultiSignProductionV2(commands HumanMultiSignCommandsV2, claims HumanMultiSignClaimCommandsV2, store reviewport.StoreV2) (*HumanMultiSignServiceV2, error) {
	if nilcheck.IsNil(commands) || nilcheck.IsNil(claims) || nilcheck.IsNil(store) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "production human multisign service requires commands, Claim Owner and Store")
	}
	return &HumanMultiSignServiceV2{commands: commands, claims: claims, store: store}, nil
}

type HumanPanelViewV2 struct {
	Panel        contract.HumanReviewPanelV2       `json:"panel"`
	Assignments  []contract.HumanPanelAssignmentV2 `json:"assignments"`
	Attestations []contract.HumanAttestationV2     `json:"attestations"`
	Quorum       *contract.HumanQuorumDecisionV2   `json:"quorum,omitempty"`
	Verdict      *contract.HumanVerdictV2          `json:"verdict,omitempty"`
}

func (s *HumanMultiSignServiceV2) OpenPanelV2(ctx context.Context, m reviewport.CreateHumanPanelMutationV2) (HumanPanelViewV2, error) {
	if err := reviewport.ValidateCreateHumanPanelTraceV2(m); err != nil {
		return HumanPanelViewV2{}, err
	}
	result, err := s.commands.OpenPanelV2(ctx, m)
	if err != nil {
		return HumanPanelViewV2{}, err
	}
	return HumanPanelViewV2{Panel: result.Panel, Assignments: result.Assignments, Attestations: []contract.HumanAttestationV2{}}, nil
}

func (s *HumanMultiSignServiceV2) SubmitAttestationV2(ctx context.Context, m reviewport.RecordHumanAttestationMutationV2) (HumanPanelViewV2, error) {
	if err := reviewport.ValidateRecordHumanAttestationTracesV2(m); err != nil {
		return HumanPanelViewV2{}, err
	}
	result, err := s.commands.SubmitAttestationV2(ctx, m)
	if err != nil {
		return HumanPanelViewV2{}, err
	}
	view, err := s.InspectPanelV2(ctx, result.Panel.TenantID, result.Panel.ID)
	if err != nil {
		return HumanPanelViewV2{}, err
	}
	return view, nil
}

func (s *HumanMultiSignServiceV2) ClaimAssignmentV2(ctx context.Context, mutation reviewport.ClaimHumanAssignmentMutationV2, organization reviewport.HumanOrganizationCurrentRequestV2) (HumanPanelViewV2, error) {
	if nilcheck.IsNil(s.claims) {
		return HumanPanelViewV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "human multisign Claim capability is not composed")
	}
	if err := reviewport.ValidateClaimHumanAssignmentTraceV2(mutation); err != nil {
		return HumanPanelViewV2{}, err
	}
	result, err := s.claims.ClaimAssignmentV2(ctx, mutation, organization)
	if err != nil {
		return HumanPanelViewV2{}, err
	}
	return s.InspectPanelV2(ctx, result.Panel.TenantID, result.Panel.ID)
}

func (s *HumanMultiSignServiceV2) InspectPanelV2(ctx context.Context, tenant core.TenantID, panelID string) (HumanPanelViewV2, error) {
	panel, err := s.store.InspectHumanPanelCurrentV2(ctx, tenant, panelID)
	if err != nil {
		return HumanPanelViewV2{}, err
	}
	assignments, err := s.store.ListHumanPanelAssignmentsV2(ctx, panel.ExactRef())
	if err != nil {
		return HumanPanelViewV2{}, err
	}
	attestations, err := s.store.ListHumanAttestationsByPanelV2(ctx, panel.ExactRef())
	if err != nil {
		return HumanPanelViewV2{}, err
	}
	view := HumanPanelViewV2{Panel: panel, Assignments: assignments, Attestations: attestations}
	if q, e := s.store.InspectHumanQuorumDecisionCurrentByPanelIDV2(ctx, tenant, panelID); e == nil {
		view.Quorum = &q
	} else if !core.HasCategory(e, core.ErrorNotFound) {
		return HumanPanelViewV2{}, e
	}
	// A Verdict binds the pre-terminal Panel revision, so inspect it using the
	// current Panel ID only when the store has an exact indexed value.
	if v, e := s.store.InspectHumanVerdictCurrentByPanelIDV2(ctx, tenant, panelID); e == nil {
		view.Verdict = &v
	} else if !core.HasCategory(e, core.ErrorNotFound) {
		return HumanPanelViewV2{}, e
	}
	return view, nil
}
