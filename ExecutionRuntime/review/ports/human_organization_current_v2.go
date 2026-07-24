package ports

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"time"

	organizationcontract "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const HumanOrganizationCurrentContractV2 = "praxis.review.human-organization-current/v2"

// HumanOrganizationCurrentRequestV2 binds one Review-owned Assignment to the
// opaque Organization subject coordinates needed to resolve its exact current
// closure. Subject IDs are lookup coordinates only: the adapter must prove
// their stable Organization IDs equal the Review exact refs.
type HumanOrganizationCurrentRequestV2 struct {
	Panel              contract.HumanReviewPanelV2     `json:"panel"`
	Assignment         contract.HumanPanelAssignmentV2 `json:"assignment"`
	ReviewerSubjectID  string                          `json:"reviewer_subject_id"`
	DelegatorSubjectID string                          `json:"delegator_subject_id,omitempty"`
	ActionScopeDigest  core.Digest                     `json:"action_scope_digest"`
}

func (r HumanOrganizationCurrentRequestV2) Clone() HumanOrganizationCurrentRequestV2 {
	r.Panel = r.Panel.Clone()
	r.Assignment = r.Assignment.Clone()
	return r
}

func (r HumanOrganizationCurrentRequestV2) Validate() error {
	if err := r.Panel.Validate(); err != nil {
		return err
	}
	if err := r.Assignment.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.ReviewerSubjectID) == "" || len(r.ReviewerSubjectID) > 512 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human Organization reviewer subject is incomplete")
	}
	if err := r.ActionScopeDigest.Validate(); err != nil {
		return err
	}
	if r.Assignment.TenantID != r.Panel.TenantID || r.Assignment.Case != r.Panel.Case || r.Assignment.Round != r.Panel.Round || r.Assignment.Target != r.Panel.Target || r.Assignment.Panel.TenantID != r.Panel.TenantID || r.Assignment.Panel.ID != r.Panel.ID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human Organization request crosses Panel exact coordinates")
	}
	if r.Panel.State != contract.HumanPanelProposedV2 && !containsHumanAssignmentRef(r.Panel.AssignmentRefs, r.Assignment.ExactRef()) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human Organization request Assignment is absent from Panel")
	}
	if r.Panel.DelegationRequired != r.Assignment.Delegated {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "Panel delegation requirement and Assignment drifted")
	}
	reviewerID, err := organizationcontract.DeriveIdentityIDV1(r.Panel.TenantID, r.ReviewerSubjectID)
	if err != nil {
		return err
	}
	if reviewerID != r.Assignment.ReviewerIdentity.Ref {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "reviewer subject does not derive the Assignment identity")
	}
	responsibilityID, err := organizationcontract.DeriveResponsibilityIDV1(r.Panel.TenantID, "review-target", r.Panel.Target.ID)
	if err != nil {
		return err
	}
	if responsibilityID != r.Panel.ResponsibilitySubject.Ref {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Panel responsibility ref does not derive from its exact Target")
	}
	if r.Assignment.Delegated {
		if strings.TrimSpace(r.DelegatorSubjectID) == "" || len(r.DelegatorSubjectID) > 512 || r.ActionScopeDigest != r.Assignment.DelegationScopeDigest {
			return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "delegated Organization subject or scope drifted")
		}
		delegatorID, deriveErr := organizationcontract.DeriveIdentityIDV1(r.Panel.TenantID, r.DelegatorSubjectID)
		if deriveErr != nil {
			return deriveErr
		}
		if delegatorID != r.Assignment.DelegatorIdentity.Ref {
			return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "delegator subject does not derive the Assignment identity")
		}
		delegationID, deriveErr := organizationcontract.DeriveDelegationIDV1(r.Panel.TenantID, r.DelegatorSubjectID, r.ReviewerSubjectID, r.Assignment.DelegatedRole, r.ActionScopeDigest)
		if deriveErr != nil {
			return deriveErr
		}
		if delegationID != r.Assignment.DelegationFact.Ref {
			return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "delegation source does not derive the Assignment fact")
		}
	} else if r.DelegatorSubjectID != "" {
		return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "direct Assignment cannot carry a delegator subject")
	}
	return nil
}

func (r HumanOrganizationCurrentRequestV2) OrganizationSourceV1() (organizationcontract.ReviewEligibilitySourceV1, error) {
	if err := r.Validate(); err != nil {
		return organizationcontract.ReviewEligibilitySourceV1{}, err
	}
	source := organizationcontract.ReviewEligibilitySourceV1{
		TenantID:                    r.Panel.TenantID,
		ReviewerSubjectID:           r.ReviewerSubjectID,
		RequiredRoles:               append([]string(nil), r.Assignment.Roles...),
		ScopeDigest:                 r.ActionScopeDigest,
		ResponsibilitySubjectKind:   "review-target",
		ResponsibilitySubjectID:     r.Panel.Target.ID,
		ResponsibilitySubjectDigest: r.Panel.Target.Digest,
		DelegatorSubjectID:          r.DelegatorSubjectID,
		DelegatedRole:               r.Assignment.DelegatedRole,
		RequireDelegation:           r.Assignment.Delegated,
		Production:                  !r.Panel.ProductionSelfReviewAllowed,
	}
	return source, source.Validate()
}

func (r HumanOrganizationCurrentRequestV2) Digest() (core.Digest, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.review.human-organization-current", HumanOrganizationCurrentContractV2, "HumanOrganizationCurrentRequestV2", r.Clone())
}

// HumanOrganizationAssignmentCurrentV2 is a Review receipt over one exact
// Organization projection. The embedded ref remains Organization-owned and
// carries its full source/fact closure; Review does not copy those fact types.
type HumanOrganizationAssignmentCurrentV2 struct {
	RequestDigest      core.Digest                                           `json:"request_digest"`
	Assignment         contract.HumanPanelAssignmentExactRefV2               `json:"assignment"`
	ReviewerIdentity   contract.HumanIdentityProofRefV2                      `json:"reviewer_identity"`
	OwnerProjectionRef organizationcontract.ReviewEligibilityProjectionRefV1 `json:"owner_projection_ref"`
	CheckedUnixNano    int64                                                 `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                                                 `json:"expires_unix_nano"`
	ProjectionDigest   core.Digest                                           `json:"projection_digest"`
}

func (p HumanOrganizationAssignmentCurrentV2) Clone() HumanOrganizationAssignmentCurrentV2 {
	p.OwnerProjectionRef = p.OwnerProjectionRef.Clone()
	return p
}

func (p HumanOrganizationAssignmentCurrentV2) Validate(request HumanOrganizationCurrentRequestV2, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	want, err := request.Digest()
	if err != nil {
		return err
	}
	if p.RequestDigest != want || p.Assignment != request.Assignment.ExactRef() || p.ReviewerIdentity != request.Assignment.ReviewerIdentity || p.ProjectionDigest != p.OwnerProjectionRef.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Organization receipt exact binding drifted")
	}
	if err := p.OwnerProjectionRef.Validate(); err != nil {
		return err
	}
	source, err := request.OrganizationSourceV1()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(p.OwnerProjectionRef.Source, source) || p.OwnerProjectionRef.TenantID != request.Panel.TenantID || p.OwnerProjectionRef.Identity.TenantID != request.Assignment.ReviewerIdentity.TenantID || p.OwnerProjectionRef.Identity.ID != request.Assignment.ReviewerIdentity.Ref || p.OwnerProjectionRef.Identity.Revision != request.Assignment.ReviewerIdentity.Revision || p.OwnerProjectionRef.Identity.Digest != request.Assignment.ReviewerIdentity.Digest || p.OwnerProjectionRef.Responsibility.TenantID != request.Panel.ResponsibilitySubject.TenantID || p.OwnerProjectionRef.Responsibility.ID != request.Panel.ResponsibilitySubject.Ref || p.OwnerProjectionRef.Responsibility.Revision != request.Panel.ResponsibilitySubject.Revision || p.OwnerProjectionRef.Responsibility.Digest != request.Panel.ResponsibilitySubject.Digest || len(p.OwnerProjectionRef.Roles) != len(request.Assignment.Roles) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Organization Owner ref drifted from the Review request")
	}
	if request.Assignment.Delegated {
		if p.OwnerProjectionRef.Delegation == nil || p.OwnerProjectionRef.Delegation.TenantID != request.Assignment.DelegationFact.TenantID || p.OwnerProjectionRef.Delegation.ID != request.Assignment.DelegationFact.Ref || p.OwnerProjectionRef.Delegation.Revision != request.Assignment.DelegationFact.Revision || p.OwnerProjectionRef.Delegation.Digest != request.Assignment.DelegationFact.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "human Organization delegation ref drifted from the Assignment")
		}
	} else if p.OwnerProjectionRef.Delegation != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "direct Assignment received a delegation ref")
	}
	if p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human Organization receipt is not current")
	}
	return nil
}

type HumanOrganizationCurrentCutV2 struct {
	ContractVersion string                                 `json:"contract_version"`
	TenantID        core.TenantID                          `json:"tenant_id"`
	Items           []HumanOrganizationAssignmentCurrentV2 `json:"items"`
	CheckedUnixNano int64                                  `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                  `json:"expires_unix_nano"`
	Digest          core.Digest                            `json:"digest"`
}

func (c HumanOrganizationCurrentCutV2) Clone() HumanOrganizationCurrentCutV2 {
	c.Items = append([]HumanOrganizationAssignmentCurrentV2(nil), c.Items...)
	for i := range c.Items {
		c.Items[i] = c.Items[i].Clone()
	}
	return c
}

func (c HumanOrganizationCurrentCutV2) digestValue() HumanOrganizationCurrentCutV2 {
	c = c.Clone()
	c.Digest = ""
	return c
}

func SealHumanOrganizationCurrentCutV2(c HumanOrganizationCurrentCutV2) (HumanOrganizationCurrentCutV2, error) {
	c = c.Clone()
	c.ContractVersion = HumanOrganizationCurrentContractV2
	c.Digest = ""
	sort.Slice(c.Items, func(i, j int) bool {
		if c.Items[i].Assignment.ID != c.Items[j].Assignment.ID {
			return c.Items[i].Assignment.ID < c.Items[j].Assignment.ID
		}
		if c.Items[i].Assignment.Revision != c.Items[j].Assignment.Revision {
			return c.Items[i].Assignment.Revision < c.Items[j].Assignment.Revision
		}
		return c.Items[i].Assignment.Digest < c.Items[j].Assignment.Digest
	})
	d, err := core.CanonicalJSONDigest("praxis.review.human-organization-current", HumanOrganizationCurrentContractV2, "HumanOrganizationCurrentCutV2", c.digestValue())
	if err != nil {
		return HumanOrganizationCurrentCutV2{}, err
	}
	c.Digest = d
	return c, c.Validate(time.Unix(0, c.CheckedUnixNano))
}

func (c HumanOrganizationCurrentCutV2) Validate(now time.Time) error {
	if c.ContractVersion != HumanOrganizationCurrentContractV2 || strings.TrimSpace(string(c.TenantID)) == "" || len(c.Items) == 0 || len(c.Items) > contract.MaxListItemsV1 || c.CheckedUnixNano <= 0 || c.CheckedUnixNano >= c.ExpiresUnixNano || now.IsZero() || now.UnixNano() < c.CheckedUnixNano || now.UnixNano() >= c.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human Organization current cut is incomplete or stale")
	}
	minimum := int64(0)
	assignments := map[string]struct{}{}
	reviewers := map[string]struct{}{}
	for index, item := range c.Items {
		for _, validate := range []func() error{item.RequestDigest.Validate, item.Assignment.Validate, item.ReviewerIdentity.Validate, item.OwnerProjectionRef.Validate, item.ProjectionDigest.Validate} {
			if err := validate(); err != nil {
				return err
			}
		}
		if item.ProjectionDigest != item.OwnerProjectionRef.Digest || item.CheckedUnixNano <= 0 || item.CheckedUnixNano >= item.ExpiresUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Organization cut item is malformed")
		}
		if item.Assignment.TenantID != c.TenantID || item.ReviewerIdentity.TenantID != c.TenantID || item.CheckedUnixNano > c.CheckedUnixNano || item.ExpiresUnixNano <= c.CheckedUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Organization cut item crosses tenant/window")
		}
		assignmentKey := item.Assignment.ID
		reviewerKey := string(item.ReviewerIdentity.TenantID) + "\x00" + item.ReviewerIdentity.Ref
		if _, exists := assignments[assignmentKey]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human Organization cut duplicates an Assignment")
		}
		if _, exists := reviewers[reviewerKey]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human Organization cut duplicates a reviewer Identity")
		}
		assignments[assignmentKey], reviewers[reviewerKey] = struct{}{}, struct{}{}
		if index > 0 && c.Items[index-1].Assignment.ID >= item.Assignment.ID {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human Organization cut items are not canonical")
		}
		if minimum == 0 || item.ExpiresUnixNano < minimum {
			minimum = item.ExpiresUnixNano
		}
	}
	if c.ExpiresUnixNano != minimum {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Organization cut expiry is not the exact minimum")
	}
	d, err := core.CanonicalJSONDigest("praxis.review.human-organization-current", HumanOrganizationCurrentContractV2, "HumanOrganizationCurrentCutV2", c.digestValue())
	if err != nil {
		return err
	}
	if d != c.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "human Organization current cut digest drifted")
	}
	return nil
}

type HumanOrganizationCurrentReaderV2 interface {
	InspectHumanOrganizationCurrentV2(context.Context, []HumanOrganizationCurrentRequestV2) (HumanOrganizationCurrentCutV2, error)
}

func containsHumanAssignmentRef(values []contract.HumanPanelAssignmentExactRefV2, wanted contract.HumanPanelAssignmentExactRefV2) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
