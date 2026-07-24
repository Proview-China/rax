package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

type CreateRewindPlanRequestV2 struct {
	Candidate    contract.RewindPlanFactV2 `json:"candidate"`
	ExpectAbsent bool                      `json:"expect_absent"`
}

type CompareAndSwapRewindPlanRequestV2 struct {
	Expected  contract.RewindPlanRefV2   `json:"expected"`
	NextState contract.RewindPlanStateV2 `json:"next_state"`
}

type InspectRewindPlanRequestV2 struct {
	Ref contract.RewindPlanRefV2 `json:"ref"`
}

type InspectCurrentRewindPlanRequestV2 struct {
	TenantID    string                `json:"tenant_id"`
	ScopeDigest string                `json:"scope_digest"`
	PlanID      string                `json:"plan_id"`
	Owner       contract.OwnerBinding `json:"owner"`
}

func (r InspectCurrentRewindPlanRequestV2) Validate() error {
	for field, value := range map[string]string{"tenant_id": r.TenantID, "scope_digest": r.ScopeDigest, "plan_id": r.PlanID} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if r.Owner.ComponentID != contract.ContinuityComponentID || r.Owner.Capability != contract.RewindPlanCapabilityV2 || r.Owner.FactKind != "rewind_plan_fact_v2" {
		return contract.NewError(contract.ErrInvalidArgument, "owner_binding", "current reader requires the exact Continuity Rewind Plan owner")
	}
	return nil
}

type RewindPlanReaderV2 interface {
	InspectRewindPlanV2(context.Context, InspectRewindPlanRequestV2) (contract.RewindPlanFactV2, error)
	InspectCurrentRewindPlanV2(context.Context, InspectCurrentRewindPlanRequestV2) (contract.RewindPlanFactV2, error)
}

type RewindPlanRepositoryV2 interface {
	RewindPlanReaderV2
	CreateRewindPlanFactV2(context.Context, contract.RewindPlanFactV2) (contract.RewindPlanFactV2, bool, error)
	CompareAndSwapRewindPlanFactV2(context.Context, contract.RewindPlanRefV2, contract.RewindPlanFactV2) (contract.RewindPlanFactV2, bool, error)
}

// RewindPlanGovernancePortV2 owns planning facts only. It cannot create a
// Sandbox ChangeSet, Runtime Operation, Review authorization, or filesystem
// effect.
type RewindPlanGovernancePortV2 interface {
	RewindPlanReaderV2
	CreateRewindPlanV2(context.Context, CreateRewindPlanRequestV2) (contract.RewindPlanFactV2, bool, error)
	CompareAndSwapRewindPlanV2(context.Context, CompareAndSwapRewindPlanRequestV2) (contract.RewindPlanFactV2, bool, error)
}
