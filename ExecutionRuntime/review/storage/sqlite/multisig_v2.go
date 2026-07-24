package sqlite

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var _ reviewport.StoreV2 = (*Store)(nil)

func (s *Store) CreateHumanPanelV2(ctx context.Context, m reviewport.CreateHumanPanelMutationV2) (out reviewport.CreateHumanPanelResultV2, err error) {
	err = s.mutate(ctx, m.ProposedPanel.TenantID, func(state *memory.Store) error { out, err = state.CreateHumanPanelV2(ctx, m); return err })
	return
}
func (s *Store) RecordHumanAttestationV2(ctx context.Context, m reviewport.RecordHumanAttestationMutationV2) (out reviewport.RecordHumanAttestationResultV2, err error) {
	err = s.mutate(ctx, m.Attestation.TenantID, func(state *memory.Store) error { out, err = state.RecordHumanAttestationV2(ctx, m); return err })
	return
}
func (s *Store) BeginHumanPanelDecisionV2(ctx context.Context, m reviewport.BeginHumanPanelDecisionMutationV2) (panel contract.HumanReviewPanelV2, c contract.ReviewCaseV1, err error) {
	err = s.mutate(ctx, m.NextPanel.TenantID, func(state *memory.Store) error { panel, c, err = state.BeginHumanPanelDecisionV2(ctx, m); return err })
	return
}
func (s *Store) DecideHumanPanelV2(ctx context.Context, m reviewport.DecideHumanPanelMutationV2) (out reviewport.DecideHumanPanelResultV2, err error) {
	err = s.mutate(ctx, m.Verdict.TenantID, func(state *memory.Store) error { out, err = state.DecideHumanPanelV2(ctx, m); return err })
	return
}
func (s *Store) InspectHumanPanelCurrentV2(ctx context.Context, t core.TenantID, id string) (out contract.HumanReviewPanelV2, err error) {
	err = s.read(ctx, t, func(state *memory.Store) error { out, err = state.InspectHumanPanelCurrentV2(ctx, t, id); return err })
	return
}
func (s *Store) InspectHumanPanelExactV2(ctx context.Context, ref contract.HumanPanelExactRefV2) (out contract.HumanReviewPanelV2, err error) {
	err = s.read(ctx, ref.TenantID, func(state *memory.Store) error { out, err = state.InspectHumanPanelExactV2(ctx, ref); return err })
	return
}
func (s *Store) ListHumanPanelAssignmentsV2(ctx context.Context, ref contract.HumanPanelExactRefV2) (out []contract.HumanPanelAssignmentV2, err error) {
	err = s.read(ctx, ref.TenantID, func(state *memory.Store) error { out, err = state.ListHumanPanelAssignmentsV2(ctx, ref); return err })
	return
}
func (s *Store) InspectHumanPanelAssignmentCurrentV2(ctx context.Context, t core.TenantID, id string) (out contract.HumanPanelAssignmentV2, err error) {
	err = s.read(ctx, t, func(state *memory.Store) error {
		out, err = state.InspectHumanPanelAssignmentCurrentV2(ctx, t, id)
		return err
	})
	return
}
func (s *Store) InspectHumanPanelAssignmentExactV2(ctx context.Context, ref contract.HumanPanelAssignmentExactRefV2) (out contract.HumanPanelAssignmentV2, err error) {
	err = s.read(ctx, ref.TenantID, func(state *memory.Store) error {
		out, err = state.InspectHumanPanelAssignmentExactV2(ctx, ref)
		return err
	})
	return
}
func (s *Store) InspectHumanAttestationExactV2(ctx context.Context, ref contract.HumanAttestationExactRefV2) (out contract.HumanAttestationV2, err error) {
	err = s.read(ctx, ref.TenantID, func(state *memory.Store) error { out, err = state.InspectHumanAttestationExactV2(ctx, ref); return err })
	return
}
func (s *Store) InspectHumanAttestationByIdempotencyV2(ctx context.Context, t core.TenantID, idem string) (out contract.HumanAttestationV2, err error) {
	err = s.read(ctx, t, func(state *memory.Store) error {
		out, err = state.InspectHumanAttestationByIdempotencyV2(ctx, t, idem)
		return err
	})
	return
}
func (s *Store) ListHumanAttestationsByPanelV2(ctx context.Context, ref contract.HumanPanelExactRefV2) (out []contract.HumanAttestationV2, err error) {
	err = s.read(ctx, ref.TenantID, func(state *memory.Store) error { out, err = state.ListHumanAttestationsByPanelV2(ctx, ref); return err })
	return
}
func (s *Store) InspectHumanQuorumDecisionExactV2(ctx context.Context, ref contract.HumanQuorumDecisionExactRefV2) (out contract.HumanQuorumDecisionV2, err error) {
	err = s.read(ctx, ref.TenantID, func(state *memory.Store) error {
		out, err = state.InspectHumanQuorumDecisionExactV2(ctx, ref)
		return err
	})
	return
}
func (s *Store) InspectHumanQuorumDecisionByPanelV2(ctx context.Context, ref contract.HumanPanelExactRefV2) (out contract.HumanQuorumDecisionV2, err error) {
	err = s.read(ctx, ref.TenantID, func(state *memory.Store) error {
		out, err = state.InspectHumanQuorumDecisionByPanelV2(ctx, ref)
		return err
	})
	return
}
func (s *Store) InspectHumanQuorumDecisionCurrentByPanelIDV2(ctx context.Context, t core.TenantID, panelID string) (out contract.HumanQuorumDecisionV2, err error) {
	err = s.read(ctx, t, func(state *memory.Store) error {
		out, err = state.InspectHumanQuorumDecisionCurrentByPanelIDV2(ctx, t, panelID)
		return err
	})
	return
}
func (s *Store) InspectHumanVerdictExactV2(ctx context.Context, ref contract.HumanVerdictExactRefV2) (out contract.HumanVerdictV2, err error) {
	err = s.read(ctx, ref.TenantID, func(state *memory.Store) error { out, err = state.InspectHumanVerdictExactV2(ctx, ref); return err })
	return
}
func (s *Store) InspectHumanVerdictByPanelV2(ctx context.Context, ref contract.HumanPanelExactRefV2) (out contract.HumanVerdictV2, err error) {
	err = s.read(ctx, ref.TenantID, func(state *memory.Store) error { out, err = state.InspectHumanVerdictByPanelV2(ctx, ref); return err })
	return
}
func (s *Store) InspectHumanVerdictCurrentByPanelIDV2(ctx context.Context, t core.TenantID, panelID string) (out contract.HumanVerdictV2, err error) {
	err = s.read(ctx, t, func(state *memory.Store) error {
		out, err = state.InspectHumanVerdictCurrentByPanelIDV2(ctx, t, panelID)
		return err
	})
	return
}
