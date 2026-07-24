package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ReviewEligibilityClosureV1 struct {
	Identity               contract.IdentityFactV1       `json:"identity"`
	DelegatorIdentity      *contract.IdentityFactV1      `json:"delegator_identity,omitempty"`
	ResponsibilityIdentity contract.IdentityFactV1       `json:"responsibility_identity"`
	Roles                  []contract.RoleGrantFactV1    `json:"roles"`
	Delegation             *contract.DelegationFactV1    `json:"delegation,omitempty"`
	Responsibility         contract.ResponsibilityFactV1 `json:"responsibility"`
}

func (v ReviewEligibilityClosureV1) Clone() ReviewEligibilityClosureV1 {
	v.Roles = append([]contract.RoleGrantFactV1(nil), v.Roles...)
	if v.Delegation != nil {
		d := *v.Delegation
		v.Delegation = &d
	}
	if v.DelegatorIdentity != nil {
		d := *v.DelegatorIdentity
		v.DelegatorIdentity = &d
	}
	return v
}

// StoreV1 owns immutable Organization fact history and full-ref current CAS.
// A nil expected ref is valid only for the first revision.
type StoreV1 interface {
	PublishIdentityV1(context.Context, *contract.IdentityRefV1, contract.IdentityFactV1) error
	PublishRoleGrantV1(context.Context, *contract.RoleGrantRefV1, contract.RoleGrantFactV1) error
	PublishDelegationV1(context.Context, *contract.DelegationRefV1, contract.DelegationFactV1) error
	PublishResponsibilityV1(context.Context, *contract.ResponsibilityRefV1, contract.ResponsibilityFactV1) error

	InspectIdentityV1(context.Context, contract.IdentityRefV1) (contract.IdentityFactV1, error)
	InspectRoleGrantV1(context.Context, contract.RoleGrantRefV1) (contract.RoleGrantFactV1, error)
	InspectDelegationV1(context.Context, contract.DelegationRefV1) (contract.DelegationFactV1, error)
	InspectResponsibilityV1(context.Context, contract.ResponsibilityRefV1) (contract.ResponsibilityFactV1, error)

	ReadReviewEligibilityClosureV1(context.Context, contract.ReviewEligibilitySourceV1) (ReviewEligibilityClosureV1, error)
	CreateOrInspectReviewEligibilityProjectionV1(context.Context, contract.ReviewEligibilityCurrentProjectionV1) (contract.ReviewEligibilityCurrentProjectionV1, error)
	InspectReviewEligibilityProjectionV1(context.Context, contract.ReviewEligibilityProjectionRefV1) (contract.ReviewEligibilityCurrentProjectionV1, error)
}

type ReviewEligibilityCurrentReaderV1 interface {
	ResolveCurrentReviewEligibilityV1(context.Context, contract.ReviewEligibilitySourceV1) (contract.ReviewEligibilityCurrentProjectionV1, error)
	InspectCurrentReviewEligibilityV1(context.Context, contract.ReviewEligibilityProjectionRefV1) (contract.ReviewEligibilityCurrentProjectionV1, error)
}

// Stable IDs allow Store implementations to resolve current indexes without a
// second by-name registry.
func StableIDsForSourceV1(source contract.ReviewEligibilitySourceV1) (string, []string, string, string, error) {
	if err := source.Validate(); err != nil {
		return "", nil, "", "", err
	}
	identityID, err := contract.DeriveIdentityIDV1(source.TenantID, source.ReviewerSubjectID)
	if err != nil {
		return "", nil, "", "", err
	}
	roleIDs := make([]string, len(source.RequiredRoles))
	for i, role := range source.RequiredRoles {
		roleIDs[i], err = contract.DeriveRoleGrantIDV1(source.TenantID, identityID, role, source.ScopeDigest)
		if err != nil {
			return "", nil, "", "", err
		}
	}
	delegationID := ""
	if source.RequireDelegation {
		delegationID, err = contract.DeriveDelegationIDV1(source.TenantID, source.DelegatorSubjectID, source.ReviewerSubjectID, source.DelegatedRole, source.ScopeDigest)
		if err != nil {
			return "", nil, "", "", err
		}
	}
	responsibilityID, err := contract.DeriveResponsibilityIDV1(source.TenantID, source.ResponsibilitySubjectKind, source.ResponsibilitySubjectID)
	if err != nil {
		return "", nil, "", "", err
	}
	return identityID, roleIDs, delegationID, responsibilityID, nil
}

func NotFoundV1(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, message)
}
func ConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}
func IndeterminateV1(message string) error {
	return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, message)
}
