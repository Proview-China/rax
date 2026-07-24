package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

type CreateRestorePlanRequestV2 struct {
	Candidate    contract.RestorePlanFactV2 `json:"candidate"`
	ExpectAbsent bool                       `json:"expect_absent"`
}

type CompareAndSwapRestorePlanRequestV2 struct {
	Expected  contract.RestorePlanRefV2   `json:"expected"`
	NextState contract.RestorePlanStateV2 `json:"next_state"`
}

type InspectRestorePlanRequestV2 struct {
	Ref contract.RestorePlanRefV2 `json:"ref"`
}

type InspectCurrentRestorePlanRequestV2 struct {
	TenantID    string                `json:"tenant_id"`
	ScopeDigest string                `json:"scope_digest"`
	PlanID      string                `json:"plan_id"`
	Owner       contract.OwnerBinding `json:"owner"`
}

func (r InspectCurrentRestorePlanRequestV2) Validate() error {
	for field, value := range map[string]string{"tenant_id": r.TenantID, "scope_digest": r.ScopeDigest, "plan_id": r.PlanID} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if r.Owner.ComponentID != contract.ContinuityComponentID || r.Owner.Capability != contract.RestorePlanCapabilityV2 || r.Owner.FactKind != "restore_plan_fact_v2" {
		return contract.NewError(contract.ErrInvalidArgument, "owner_binding", "current reader requires the exact Continuity Restore Plan owner")
	}
	return nil
}

type RestorePlanReaderV2 interface {
	InspectRestorePlanV2(context.Context, InspectRestorePlanRequestV2) (contract.RestorePlanFactV2, error)
	InspectCurrentRestorePlanV2(context.Context, InspectCurrentRestorePlanRequestV2) (contract.RestorePlanFactV2, error)
}

type RestorePlanRepositoryV2 interface {
	RestorePlanReaderV2
	CreateRestorePlanFactV2(context.Context, contract.RestorePlanFactV2) (contract.RestorePlanFactV2, bool, error)
	CompareAndSwapRestorePlanFactV2(context.Context, contract.RestorePlanRefV2, contract.RestorePlanFactV2) (contract.RestorePlanFactV2, bool, error)
}

// RestorePlanGovernancePortV2 owns planning facts only. It has no Runtime
// RestoreAttempt, Eligibility, Review, Permit, Stage, Activate, or Provider
// method by construction.
type RestorePlanGovernancePortV2 interface {
	RestorePlanReaderV2
	CreateRestorePlanV2(context.Context, CreateRestorePlanRequestV2) (contract.RestorePlanFactV2, bool, error)
	CompareAndSwapRestorePlanV2(context.Context, CompareAndSwapRestorePlanRequestV2) (contract.RestorePlanFactV2, bool, error)
}
