package contract

import (
	"cmp"
	"fmt"
	"sort"
	"time"
)

const RestoreContextMaterializationVersionV1 = "praxis.context/restore-materialization/v1"

type RestoreContextTargetBindingV1 struct {
	TenantID      string `json:"tenant_id"`
	ScopeDigest   Digest `json:"scope_digest"`
	InstanceID    string `json:"instance_id"`
	InstanceEpoch uint64 `json:"instance_epoch"`
	LeaseID       string `json:"lease_id"`
	LeaseEpoch    uint64 `json:"lease_epoch"`
	FenceEpoch    uint64 `json:"fence_epoch"`
}

func (b RestoreContextTargetBindingV1) Validate() error {
	if validateID(b.TenantID) != nil || b.ScopeDigest.Validate() != nil || validateID(b.InstanceID) != nil || b.InstanceEpoch == 0 || validateID(b.LeaseID) != nil || b.LeaseEpoch == 0 || b.FenceEpoch == 0 {
		return fmt.Errorf("%w: Restore Context target binding", ErrInvalid)
	}
	return nil
}

type RestoreContextRequirementKindV1 string

const (
	RestoreRequirementProfileV1   RestoreContextRequirementKindV1 = "profile"
	RestoreRequirementToolV1      RestoreContextRequirementKindV1 = "tool_surface"
	RestoreRequirementMCPV1       RestoreContextRequirementKindV1 = "mcp_connection"
	RestoreRequirementReviewV1    RestoreContextRequirementKindV1 = "review_policy"
	RestoreRequirementAuthorityV1 RestoreContextRequirementKindV1 = "authority"
	RestoreRequirementBudgetV1    RestoreContextRequirementKindV1 = "budget"
	RestoreRequirementBindingV1   RestoreContextRequirementKindV1 = "binding"
)

type RestoreContextRequirementRefV1 struct {
	Kind        RestoreContextRequirementKindV1 `json:"kind"`
	Ref         FactRef                         `json:"ref"`
	RouteDigest Digest                          `json:"owner_route_digest"`
}

func (r RestoreContextRequirementRefV1) Validate() error {
	switch r.Kind {
	case RestoreRequirementProfileV1, RestoreRequirementToolV1, RestoreRequirementMCPV1, RestoreRequirementReviewV1, RestoreRequirementAuthorityV1, RestoreRequirementBudgetV1, RestoreRequirementBindingV1:
	default:
		return fmt.Errorf("%w: Restore Context requirement kind", ErrInvalid)
	}
	if r.Ref.Validate() != nil || r.RouteDigest.Validate() != nil {
		return fmt.Errorf("%w: Restore Context requirement exact route", ErrInvalid)
	}
	return nil
}

type RestoreContextMaterializationRequestV1 struct {
	ID                string                           `json:"id"`
	IdempotencyKey    string                           `json:"idempotency_key"`
	TenantID          string                           `json:"tenant_id"`
	Attempt           FactRef                          `json:"restore_attempt"`
	Eligibility       FactRef                          `json:"restore_eligibility"`
	Stage             FactRef                          `json:"stage_domain_result"`
	SandboxApply      FactRef                          `json:"sandbox_apply_settlement"`
	SourceScope       Digest                           `json:"source_scope_digest"`
	Target            RestoreContextTargetBindingV1    `json:"target"`
	SourceGeneration  FactRef                          `json:"source_generation"`
	SourceFrames      []FactRef                        `json:"source_frames"`
	Requirements      []RestoreContextRequirementRefV1 `json:"requirements"`
	RequestedUnixNano int64                            `json:"requested_unix_nano"`
	NotAfterUnixNano  int64                            `json:"not_after_unix_nano"`
}

func (r RestoreContextMaterializationRequestV1) Clone() RestoreContextMaterializationRequestV1 {
	r.SourceFrames = append([]FactRef{}, r.SourceFrames...)
	r.Requirements = append([]RestoreContextRequirementRefV1{}, r.Requirements...)
	return r
}

func (r RestoreContextMaterializationRequestV1) ValidateCurrent(now time.Time) error {
	if validateID(r.ID) != nil || validateID(r.IdempotencyKey) != nil || validateID(r.TenantID) != nil || r.Attempt.Validate() != nil || r.Eligibility.Validate() != nil || r.Stage.Validate() != nil || r.SandboxApply.Validate() != nil || r.SourceScope.Validate() != nil || r.Target.Validate() != nil || r.SourceGeneration.Validate() != nil || len(r.SourceFrames) == 0 || len(r.Requirements) == 0 || r.RequestedUnixNano <= 0 || r.NotAfterUnixNano <= r.RequestedUnixNano || r.Target.TenantID != r.TenantID || r.Target.ScopeDigest == r.SourceScope || now.IsZero() || now.UnixNano() < r.RequestedUnixNano || now.UnixNano() >= r.NotAfterUnixNano {
		return fmt.Errorf("%w: Restore Context materialization request", ErrInvalid)
	}
	if !canonicalRestoreFactRefsV1(r.SourceFrames) {
		return fmt.Errorf("%w: Restore Context source Frame set", ErrConflict)
	}
	requirements := append([]RestoreContextRequirementRefV1{}, r.Requirements...)
	sort.Slice(requirements, func(i, j int) bool {
		return compareRestoreRequirementV1(requirements[i], requirements[j]) < 0
	})
	kinds := make(map[RestoreContextRequirementKindV1]struct{}, 7)
	for index, requirement := range requirements {
		if requirement.Validate() != nil || index > 0 && compareRestoreRequirementV1(requirements[index-1], requirement) == 0 {
			return fmt.Errorf("%w: Restore Context requirement set", ErrConflict)
		}
		kinds[requirement.Kind] = struct{}{}
	}
	if len(kinds) != 7 {
		return fmt.Errorf("%w: Restore Context requires Profile, Tool, MCP, Review, Authority, Budget, and Binding current routes", ErrInvalid)
	}
	return nil
}

func (r RestoreContextMaterializationRequestV1) DigestValue() (Digest, error) {
	copy := r.Clone()
	sort.Slice(copy.SourceFrames, func(i, j int) bool { return compareRestoreFactRefV1(copy.SourceFrames[i], copy.SourceFrames[j]) < 0 })
	sort.Slice(copy.Requirements, func(i, j int) bool {
		return compareRestoreRequirementV1(copy.Requirements[i], copy.Requirements[j]) < 0
	})
	return DigestJSON(copy)
}

type RestoreContextRequirementsCurrentV1 struct {
	RequestDigest   Digest    `json:"request_digest"`
	Proofs          []FactRef `json:"proofs"`
	Residuals       []FactRef `json:"residuals"`
	CheckedUnixNano int64     `json:"checked_unix_nano"`
	ExpiresUnixNano int64     `json:"expires_unix_nano"`
	Digest          Digest    `json:"digest"`
}

func SealRestoreContextRequirementsCurrentV1(value RestoreContextRequirementsCurrentV1) (RestoreContextRequirementsCurrentV1, error) {
	value.Proofs = normalizeRestoreFactRefsV1(value.Proofs)
	value.Residuals = normalizeRestoreFactRefsV1(value.Residuals)
	value.Digest = ""
	digest, err := DigestJSON(value)
	if err != nil {
		return RestoreContextRequirementsCurrentV1{}, err
	}
	value.Digest = digest
	return value, value.ValidateCurrent(time.Unix(0, value.CheckedUnixNano))
}

func (v RestoreContextRequirementsCurrentV1) ValidateCurrent(now time.Time) error {
	if v.RequestDigest.Validate() != nil || len(v.Proofs) == 0 || !canonicalRestoreFactRefsV1(v.Proofs) || !canonicalRestoreFactRefsV1(v.Residuals) || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || v.Digest.Validate() != nil || now.IsZero() || now.UnixNano() < v.CheckedUnixNano || now.UnixNano() >= v.ExpiresUnixNano {
		return fmt.Errorf("%w: Restore Context requirements current", ErrInvalid)
	}
	copy := v
	copy.Digest = ""
	digest, err := DigestJSON(copy)
	if err != nil || digest != v.Digest {
		return fmt.Errorf("%w: Restore Context requirements current digest", ErrConflict)
	}
	return nil
}

type RestoredContextFrameV1 struct {
	ID              string                        `json:"id"`
	Revision        uint64                        `json:"revision"`
	Digest          Digest                        `json:"digest"`
	Target          RestoreContextTargetBindingV1 `json:"target"`
	SourceFrame     FactRef                       `json:"source_frame"`
	StablePrefix    ContentRef                    `json:"stable_prefix"`
	SemiStable      *ContentRef                   `json:"semi_stable,omitempty"`
	DynamicTail     ContentRef                    `json:"dynamic_tail"`
	Rendered        ContentRef                    `json:"rendered"`
	CreatedUnixNano int64                         `json:"created_unix_nano"`
	ExpiresUnixNano int64                         `json:"expires_unix_nano"`
}

func SealRestoredContextFrameV1(value RestoredContextFrameV1) (RestoredContextFrameV1, error) {
	value.Revision = 1
	value.Digest = ""
	digest, err := DigestJSON(value)
	if err != nil {
		return RestoredContextFrameV1{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func (f RestoredContextFrameV1) Validate() error {
	if validateID(f.ID) != nil || f.Revision != 1 || f.Digest.Validate() != nil || f.Target.Validate() != nil || f.SourceFrame.Validate() != nil || f.StablePrefix.Validate() != nil || f.DynamicTail.Validate() != nil || f.Rendered.Validate() != nil || validateTimes(f.CreatedUnixNano, f.ExpiresUnixNano) != nil || f.SemiStable != nil && f.SemiStable.Validate() != nil {
		return fmt.Errorf("%w: restored Context Frame", ErrInvalid)
	}
	copy := f
	copy.Digest = ""
	digest, err := DigestJSON(copy)
	if err != nil || digest != f.Digest {
		return fmt.Errorf("%w: restored Context Frame digest", ErrConflict)
	}
	return nil
}

func (f RestoredContextFrameV1) Ref() FactRef {
	return FactRef{ID: f.ID, Revision: f.Revision, Digest: f.Digest}
}

type RestoredContextGenerationV1 struct {
	ID               string                        `json:"id"`
	Revision         uint64                        `json:"revision"`
	Digest           Digest                        `json:"digest"`
	Target           RestoreContextTargetBindingV1 `json:"target"`
	SourceGeneration FactRef                       `json:"source_generation"`
	Frames           []FactRef                     `json:"frames"`
	CreatedUnixNano  int64                         `json:"created_unix_nano"`
}

func SealRestoredContextGenerationV1(value RestoredContextGenerationV1) (RestoredContextGenerationV1, error) {
	value.Revision = 1
	value.Frames = normalizeRestoreFactRefsV1(value.Frames)
	value.Digest = ""
	digest, err := DigestJSON(value)
	if err != nil {
		return RestoredContextGenerationV1{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func (g RestoredContextGenerationV1) Validate() error {
	if validateID(g.ID) != nil || g.Revision != 1 || g.Digest.Validate() != nil || g.Target.Validate() != nil || g.SourceGeneration.Validate() != nil || len(g.Frames) == 0 || !canonicalRestoreFactRefsV1(g.Frames) || g.CreatedUnixNano <= 0 {
		return fmt.Errorf("%w: restored Context Generation", ErrInvalid)
	}
	copy := g
	copy.Digest = ""
	digest, err := DigestJSON(copy)
	if err != nil || digest != g.Digest {
		return fmt.Errorf("%w: restored Context Generation digest", ErrConflict)
	}
	return nil
}

func (g RestoredContextGenerationV1) Ref() FactRef {
	return FactRef{ID: g.ID, Revision: g.Revision, Digest: g.Digest}
}

type RestoreContextMaterializationFactV1 struct {
	ID               string                              `json:"id"`
	Revision         uint64                              `json:"revision"`
	Digest           Digest                              `json:"digest"`
	RequestDigest    Digest                              `json:"request_digest"`
	Target           RestoreContextTargetBindingV1       `json:"target"`
	Attempt          FactRef                             `json:"restore_attempt"`
	Eligibility      FactRef                             `json:"restore_eligibility"`
	Stage            FactRef                             `json:"stage_domain_result"`
	SandboxApply     FactRef                             `json:"sandbox_apply_settlement"`
	SourceGeneration FactRef                             `json:"source_generation"`
	TargetGeneration FactRef                             `json:"target_generation"`
	TargetFrames     []FactRef                           `json:"target_frames"`
	Requirements     RestoreContextRequirementsCurrentV1 `json:"requirements"`
	CurrentDigest    Digest                              `json:"current_digest"`
	CreatedUnixNano  int64                               `json:"created_unix_nano"`
	ExpiresUnixNano  int64                               `json:"expires_unix_nano"`
}

func SealRestoreContextMaterializationFactV1(value RestoreContextMaterializationFactV1) (RestoreContextMaterializationFactV1, error) {
	value.Revision = 1
	value.TargetFrames = normalizeRestoreFactRefsV1(value.TargetFrames)
	value.Digest = ""
	digest, err := DigestJSON(value)
	if err != nil {
		return RestoreContextMaterializationFactV1{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func (f RestoreContextMaterializationFactV1) Validate() error {
	if validateID(f.ID) != nil || f.Revision != 1 || f.Digest.Validate() != nil || f.RequestDigest.Validate() != nil || f.Target.Validate() != nil || f.Attempt.Validate() != nil || f.Eligibility.Validate() != nil || f.Stage.Validate() != nil || f.SandboxApply.Validate() != nil || f.SourceGeneration.Validate() != nil || f.TargetGeneration.Validate() != nil || len(f.TargetFrames) == 0 || !canonicalRestoreFactRefsV1(f.TargetFrames) || f.Requirements.ValidateCurrent(time.Unix(0, f.CreatedUnixNano)) != nil || f.CurrentDigest.Validate() != nil || validateTimes(f.CreatedUnixNano, f.ExpiresUnixNano) != nil || f.ExpiresUnixNano > f.Requirements.ExpiresUnixNano {
		return fmt.Errorf("%w: Restore Context materialization fact", ErrInvalid)
	}
	copy := f
	copy.Digest = ""
	digest, err := DigestJSON(copy)
	if err != nil || digest != f.Digest {
		return fmt.Errorf("%w: Restore Context materialization fact digest", ErrConflict)
	}
	return nil
}

func (f RestoreContextMaterializationFactV1) Ref() FactRef {
	return FactRef{ID: f.ID, Revision: f.Revision, Digest: f.Digest}
}

func normalizeRestoreFactRefsV1(values []FactRef) []FactRef {
	result := append([]FactRef{}, values...)
	sort.Slice(result, func(i, j int) bool { return compareRestoreFactRefV1(result[i], result[j]) < 0 })
	return result
}

func canonicalRestoreFactRefsV1(values []FactRef) bool {
	for index, ref := range values {
		if ref.Validate() != nil || index > 0 && compareRestoreFactRefV1(values[index-1], ref) >= 0 {
			return false
		}
	}
	return true
}

func compareRestoreFactRefV1(left, right FactRef) int {
	if result := cmp.Compare(left.ID, right.ID); result != 0 {
		return result
	}
	if result := cmp.Compare(left.Revision, right.Revision); result != 0 {
		return result
	}
	return cmp.Compare(left.Digest, right.Digest)
}
func compareRestoreRequirementV1(left, right RestoreContextRequirementRefV1) int {
	if result := cmp.Compare(left.Kind, right.Kind); result != 0 {
		return result
	}
	if result := compareRestoreFactRefV1(left.Ref, right.Ref); result != 0 {
		return result
	}
	return cmp.Compare(left.RouteDigest, right.RouteDigest)
}
