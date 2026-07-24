package ports

import (
	"context"
	"reflect"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const ReviewGroundingRouterContractV2 = "praxis.runtime.review-grounding-router/v2"

type ReviewGroundingRouteFamilyV2 string

const (
	ReviewGroundingArtifactRouteV2        ReviewGroundingRouteFamilyV2 = "artifact"
	ReviewGroundingEnvironmentRouteV2     ReviewGroundingRouteFamilyV2 = "environment"
	ReviewGroundingValidationScopeRouteV2 ReviewGroundingRouteFamilyV2 = "validation_scope"
)

type ReviewGroundingRouteDeclarationV2 struct {
	Family   ReviewGroundingRouteFamilyV2 `json:"family"`
	Kind     NamespacedNameV2             `json:"kind"`
	Owner    ReviewGroundingOwnerRefV2    `json:"owner"`
	Required bool                         `json:"required"`
}

type ReviewGroundingRouteRequestV2 struct {
	Family ReviewGroundingRouteFamilyV2 `json:"family"`
	Kind   NamespacedNameV2             `json:"kind"`
	Owner  ReviewGroundingOwnerRefV2    `json:"owner"`
}

type ReviewGroundingRouteRefV2 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

type ReviewGroundingReaderBindingRefV2 struct {
	ID                    string                    `json:"id"`
	Revision              core.Revision             `json:"revision"`
	Route                 ReviewGroundingRouteRefV2 `json:"route"`
	AdapterArtifactDigest core.Digest               `json:"adapter_artifact_digest"`
	Digest                core.Digest               `json:"digest"`
}

type ReviewArtifactResolvedRouteProofV2 struct {
	Declaration   ReviewGroundingRouteDeclarationV2 `json:"declaration"`
	Route         ReviewGroundingRouteRefV2         `json:"route"`
	ReaderBinding ReviewGroundingReaderBindingRefV2 `json:"reader_binding"`
}
type ReviewEnvironmentResolvedRouteProofV2 struct {
	Declaration   ReviewGroundingRouteDeclarationV2 `json:"declaration"`
	Route         ReviewGroundingRouteRefV2         `json:"route"`
	ReaderBinding ReviewGroundingReaderBindingRefV2 `json:"reader_binding"`
}
type ReviewValidationScopeResolvedRouteProofV2 struct {
	Declaration   ReviewGroundingRouteDeclarationV2 `json:"declaration"`
	Route         ReviewGroundingRouteRefV2         `json:"route"`
	ReaderBinding ReviewGroundingReaderBindingRefV2 `json:"reader_binding"`
}

type ReviewArtifactResolvedRouteV2 struct {
	Proof  ReviewArtifactResolvedRouteProofV2 `json:"proof"`
	Reader ReviewArtifactCurrentReaderV2      `json:"-"`
}
type ReviewEnvironmentResolvedRouteV2 struct {
	Proof  ReviewEnvironmentResolvedRouteProofV2 `json:"proof"`
	Reader ReviewEnvironmentCurrentReaderV2      `json:"-"`
}
type ReviewValidationScopeResolvedRouteV2 struct {
	Proof  ReviewValidationScopeResolvedRouteProofV2 `json:"proof"`
	Reader ReviewValidationScopeCurrentReaderV2      `json:"-"`
}

type ReviewArtifactRouteBindingV2 struct {
	Declaration   ReviewGroundingRouteDeclarationV2 `json:"declaration"`
	ReaderBinding ReviewGroundingReaderBindingRefV2 `json:"reader_binding"`
	Reader        ReviewArtifactCurrentReaderV2     `json:"-"`
}
type ReviewEnvironmentRouteBindingV2 struct {
	Declaration   ReviewGroundingRouteDeclarationV2 `json:"declaration"`
	ReaderBinding ReviewGroundingReaderBindingRefV2 `json:"reader_binding"`
	Reader        ReviewEnvironmentCurrentReaderV2  `json:"-"`
}
type ReviewValidationScopeRouteBindingV2 struct {
	Declaration   ReviewGroundingRouteDeclarationV2    `json:"declaration"`
	ReaderBinding ReviewGroundingReaderBindingRefV2    `json:"reader_binding"`
	Reader        ReviewValidationScopeCurrentReaderV2 `json:"-"`
}

type ReviewGroundingRequiredRouteCatalogV2 struct {
	ContractVersion string                              `json:"contract_version"`
	Artifact        []ReviewGroundingRouteDeclarationV2 `json:"artifact"`
	Environment     []ReviewGroundingRouteDeclarationV2 `json:"environment"`
	ValidationScope []ReviewGroundingRouteDeclarationV2 `json:"validation_scope"`
	Digest          core.Digest                         `json:"digest"`
}

type ReviewGroundingReaderResolverV2 interface {
	ResolveReviewArtifactReaderV2(context.Context, ReviewGroundingRouteRequestV2) (ReviewArtifactResolvedRouteV2, error)
	ResolveReviewEnvironmentReaderV2(context.Context, ReviewGroundingRouteRequestV2) (ReviewEnvironmentResolvedRouteV2, error)
	ResolveReviewValidationScopeReaderV2(context.Context, ReviewGroundingRouteRequestV2) (ReviewValidationScopeResolvedRouteV2, error)
}

func (d ReviewGroundingRouteDeclarationV2) Validate() error {
	if !validGroundingFamilyV2(d.Family) || ValidateNamespacedNameV2(d.Kind) != nil || d.Owner.Validate() != nil {
		return groundingInvalidV2("review grounding route declaration is incomplete")
	}
	return nil
}
func (r ReviewGroundingRouteRequestV2) Validate() error {
	return ReviewGroundingRouteDeclarationV2{Family: r.Family, Kind: r.Kind, Owner: r.Owner}.Validate()
}
func (r ReviewGroundingRouteRefV2) Validate() error {
	if r.ID == "" || r.Revision == 0 || r.Digest.Validate() != nil {
		return groundingInvalidV2("review grounding route ref is incomplete")
	}
	return nil
}
func (r ReviewGroundingReaderBindingRefV2) Validate() error {
	if r.ID == "" || r.Revision == 0 || r.Route.Validate() != nil || r.AdapterArtifactDigest.Validate() != nil || r.Digest.Validate() != nil {
		return groundingInvalidV2("review grounding reader binding ref is incomplete")
	}
	copy := r
	copy.Digest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.review-grounding-reader-binding", ReviewGroundingRouterContractV2, "ReviewGroundingReaderBindingRefV2", copy)
	if err != nil {
		return err
	}
	if digest != r.Digest {
		return groundingDigestConflictV2("review grounding reader binding digest drifted")
	}
	return nil
}

func SealReviewGroundingReaderBindingRefV2(r ReviewGroundingReaderBindingRefV2) (ReviewGroundingReaderBindingRefV2, error) {
	if r.ID == "" || r.Revision == 0 || r.Route.Validate() != nil || r.AdapterArtifactDigest.Validate() != nil {
		return ReviewGroundingReaderBindingRefV2{}, groundingInvalidV2("review grounding reader binding input is incomplete")
	}
	provided := r.Digest
	r.Digest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.review-grounding-reader-binding", ReviewGroundingRouterContractV2, "ReviewGroundingReaderBindingRefV2", r)
	if err != nil {
		return ReviewGroundingReaderBindingRefV2{}, err
	}
	if provided != "" && provided != digest {
		return ReviewGroundingReaderBindingRefV2{}, groundingDigestConflictV2("review grounding reader binding supplied digest drifted")
	}
	r.Digest = digest
	return r, r.Validate()
}

func DeriveReviewGroundingRouteRefV2(d ReviewGroundingRouteDeclarationV2) (ReviewGroundingRouteRefV2, error) {
	if err := d.Validate(); err != nil {
		return ReviewGroundingRouteRefV2{}, err
	}
	return deriveGroundingRouteRefV2(d), nil
}

func (p ReviewArtifactResolvedRouteProofV2) Validate() error {
	return validateResolvedProofV2(ReviewGroundingArtifactRouteV2, p.Declaration, p.Route, p.ReaderBinding)
}
func (p ReviewEnvironmentResolvedRouteProofV2) Validate() error {
	return validateResolvedProofV2(ReviewGroundingEnvironmentRouteV2, p.Declaration, p.Route, p.ReaderBinding)
}
func (p ReviewValidationScopeResolvedRouteProofV2) Validate() error {
	return validateResolvedProofV2(ReviewGroundingValidationScopeRouteV2, p.Declaration, p.Route, p.ReaderBinding)
}
func (r ReviewArtifactResolvedRouteV2) Validate() error {
	if r.Proof.Validate() != nil || nilInterfaceV2(r.Reader) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "artifact grounding route has no exact Reader")
	}
	return nil
}
func (r ReviewEnvironmentResolvedRouteV2) Validate() error {
	if r.Proof.Validate() != nil || nilInterfaceV2(r.Reader) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "environment grounding route has no exact Reader")
	}
	return nil
}
func (r ReviewValidationScopeResolvedRouteV2) Validate() error {
	if r.Proof.Validate() != nil || nilInterfaceV2(r.Reader) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "validation scope grounding route has no exact Reader")
	}
	return nil
}

func (c ReviewGroundingRequiredRouteCatalogV2) Validate() error {
	if c.ContractVersion != ReviewGroundingRouterContractV2 || c.Digest.Validate() != nil {
		return groundingInvalidV2("review grounding route catalog is incomplete")
	}
	for family, values := range map[ReviewGroundingRouteFamilyV2][]ReviewGroundingRouteDeclarationV2{
		ReviewGroundingArtifactRouteV2: c.Artifact, ReviewGroundingEnvironmentRouteV2: c.Environment, ReviewGroundingValidationScopeRouteV2: c.ValidationScope,
	} {
		if !sort.SliceIsSorted(values, func(i, j int) bool { return groundingRouteKeyV2(values[i]) < groundingRouteKeyV2(values[j]) }) {
			return groundingInvalidCanonicalV2("review grounding route catalog is not sorted")
		}
		for i, declaration := range values {
			if declaration.Family != family || declaration.Validate() != nil {
				return groundingInvalidV2("review grounding route catalog declaration drifted")
			}
			if i > 0 && groundingRouteKeyV2(values[i-1]) == groundingRouteKeyV2(declaration) {
				return groundingDuplicateV2("review grounding route catalog contains a duplicate")
			}
		}
	}
	copy := c
	copy.Digest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.review-grounding-route-catalog", ReviewGroundingRouterContractV2, "ReviewGroundingRequiredRouteCatalogV2", copy)
	if err != nil {
		return err
	}
	if digest != c.Digest {
		return groundingDigestConflictV2("review grounding route catalog digest drifted")
	}
	return nil
}

func SealReviewGroundingRequiredRouteCatalogV2(c ReviewGroundingRequiredRouteCatalogV2) (ReviewGroundingRequiredRouteCatalogV2, error) {
	c.ContractVersion = ReviewGroundingRouterContractV2
	sort.Slice(c.Artifact, func(i, j int) bool { return groundingRouteKeyV2(c.Artifact[i]) < groundingRouteKeyV2(c.Artifact[j]) })
	sort.Slice(c.Environment, func(i, j int) bool {
		return groundingRouteKeyV2(c.Environment[i]) < groundingRouteKeyV2(c.Environment[j])
	})
	sort.Slice(c.ValidationScope, func(i, j int) bool {
		return groundingRouteKeyV2(c.ValidationScope[i]) < groundingRouteKeyV2(c.ValidationScope[j])
	})
	c.Digest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.review-grounding-route-catalog", ReviewGroundingRouterContractV2, "ReviewGroundingRequiredRouteCatalogV2", c)
	if err != nil {
		return ReviewGroundingRequiredRouteCatalogV2{}, err
	}
	c.Digest = digest
	return c, c.Validate()
}

type reviewGroundingReaderResolverV2 struct {
	artifact    map[string]ReviewArtifactRouteBindingV2
	environment map[string]ReviewEnvironmentRouteBindingV2
	scope       map[string]ReviewValidationScopeRouteBindingV2
}

func NewReviewGroundingReaderResolverV2(c ReviewGroundingRequiredRouteCatalogV2, artifacts []ReviewArtifactRouteBindingV2, environments []ReviewEnvironmentRouteBindingV2, scopes []ReviewValidationScopeRouteBindingV2) (ReviewGroundingReaderResolverV2, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	out := &reviewGroundingReaderResolverV2{artifact: map[string]ReviewArtifactRouteBindingV2{}, environment: map[string]ReviewEnvironmentRouteBindingV2{}, scope: map[string]ReviewValidationScopeRouteBindingV2{}}
	declaredArtifact, declaredEnvironment, declaredScope := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, d := range c.Artifact {
		declaredArtifact[groundingRouteKeyV2(d)] = d.Required
	}
	for _, d := range c.Environment {
		declaredEnvironment[groundingRouteKeyV2(d)] = d.Required
	}
	for _, d := range c.ValidationScope {
		declaredScope[groundingRouteKeyV2(d)] = d.Required
	}
	for _, value := range artifacts {
		if value.Declaration.Family != ReviewGroundingArtifactRouteV2 || value.Declaration.Validate() != nil || value.ReaderBinding.Validate() != nil || value.ReaderBinding.Route != deriveGroundingRouteRefV2(value.Declaration) || nilInterfaceV2(value.Reader) {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "artifact grounding route binding is invalid")
		}
		key := groundingRouteKeyV2(value.Declaration)
		if _, declared := declaredArtifact[key]; !declared {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "artifact grounding binding is absent from the sealed catalog")
		}
		if _, exists := out.artifact[key]; exists {
			return nil, groundingConflictV2("artifact grounding route is duplicated")
		}
		out.artifact[key] = value
	}
	for _, value := range environments {
		if value.Declaration.Family != ReviewGroundingEnvironmentRouteV2 || value.Declaration.Validate() != nil || value.ReaderBinding.Validate() != nil || value.ReaderBinding.Route != deriveGroundingRouteRefV2(value.Declaration) || nilInterfaceV2(value.Reader) {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "environment grounding route binding is invalid")
		}
		key := groundingRouteKeyV2(value.Declaration)
		if _, declared := declaredEnvironment[key]; !declared {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "environment grounding binding is absent from the sealed catalog")
		}
		if _, exists := out.environment[key]; exists {
			return nil, groundingConflictV2("environment grounding route is duplicated")
		}
		out.environment[key] = value
	}
	for _, value := range scopes {
		if value.Declaration.Family != ReviewGroundingValidationScopeRouteV2 || value.Declaration.Validate() != nil || value.ReaderBinding.Validate() != nil || value.ReaderBinding.Route != deriveGroundingRouteRefV2(value.Declaration) || nilInterfaceV2(value.Reader) {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "validation scope grounding route binding is invalid")
		}
		key := groundingRouteKeyV2(value.Declaration)
		if _, declared := declaredScope[key]; !declared {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "validation scope grounding binding is absent from the sealed catalog")
		}
		if _, exists := out.scope[key]; exists {
			return nil, groundingConflictV2("validation scope grounding route is duplicated")
		}
		out.scope[key] = value
	}
	for _, declaration := range append(append(append([]ReviewGroundingRouteDeclarationV2{}, c.Artifact...), c.Environment...), c.ValidationScope...) {
		var exists bool
		switch declaration.Family {
		case ReviewGroundingArtifactRouteV2:
			_, exists = out.artifact[groundingRouteKeyV2(declaration)]
		case ReviewGroundingEnvironmentRouteV2:
			_, exists = out.environment[groundingRouteKeyV2(declaration)]
		case ReviewGroundingValidationScopeRouteV2:
			_, exists = out.scope[groundingRouteKeyV2(declaration)]
		}
		if declaration.Required && !exists {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "required grounding route has no Reader")
		}
	}
	return out, nil
}

func (r *reviewGroundingReaderResolverV2) ResolveReviewArtifactReaderV2(ctx context.Context, request ReviewGroundingRouteRequestV2) (ReviewArtifactResolvedRouteV2, error) {
	if err := ctx.Err(); err != nil {
		return ReviewArtifactResolvedRouteV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "artifact grounding route context ended")
	}
	if request.Validate() != nil || request.Family != ReviewGroundingArtifactRouteV2 {
		return ReviewArtifactResolvedRouteV2{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "artifact grounding route request is undeclared")
	}
	value, ok := r.artifact[groundingRouteRequestKeyV2(request)]
	if !ok {
		return ReviewArtifactResolvedRouteV2{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "artifact grounding route is undeclared")
	}
	return ReviewArtifactResolvedRouteV2{Proof: ReviewArtifactResolvedRouteProofV2{Declaration: value.Declaration, Route: value.ReaderBinding.Route, ReaderBinding: value.ReaderBinding}, Reader: value.Reader}, nil
}
func (r *reviewGroundingReaderResolverV2) ResolveReviewEnvironmentReaderV2(ctx context.Context, request ReviewGroundingRouteRequestV2) (ReviewEnvironmentResolvedRouteV2, error) {
	if err := ctx.Err(); err != nil {
		return ReviewEnvironmentResolvedRouteV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "environment grounding route context ended")
	}
	if request.Validate() != nil || request.Family != ReviewGroundingEnvironmentRouteV2 {
		return ReviewEnvironmentResolvedRouteV2{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "environment grounding route request is undeclared")
	}
	value, ok := r.environment[groundingRouteRequestKeyV2(request)]
	if !ok {
		return ReviewEnvironmentResolvedRouteV2{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "environment grounding route is undeclared")
	}
	return ReviewEnvironmentResolvedRouteV2{Proof: ReviewEnvironmentResolvedRouteProofV2{Declaration: value.Declaration, Route: value.ReaderBinding.Route, ReaderBinding: value.ReaderBinding}, Reader: value.Reader}, nil
}
func (r *reviewGroundingReaderResolverV2) ResolveReviewValidationScopeReaderV2(ctx context.Context, request ReviewGroundingRouteRequestV2) (ReviewValidationScopeResolvedRouteV2, error) {
	if err := ctx.Err(); err != nil {
		return ReviewValidationScopeResolvedRouteV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "validation scope grounding route context ended")
	}
	if request.Validate() != nil || request.Family != ReviewGroundingValidationScopeRouteV2 {
		return ReviewValidationScopeResolvedRouteV2{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "validation scope grounding route request is undeclared")
	}
	value, ok := r.scope[groundingRouteRequestKeyV2(request)]
	if !ok {
		return ReviewValidationScopeResolvedRouteV2{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "validation scope grounding route is undeclared")
	}
	return ReviewValidationScopeResolvedRouteV2{Proof: ReviewValidationScopeResolvedRouteProofV2{Declaration: value.Declaration, Route: value.ReaderBinding.Route, ReaderBinding: value.ReaderBinding}, Reader: value.Reader}, nil
}

func deriveGroundingRouteRefV2(d ReviewGroundingRouteDeclarationV2) ReviewGroundingRouteRefV2 {
	digest, _ := core.CanonicalJSONDigest("praxis.runtime.review-grounding-route", ReviewGroundingRouterContractV2, "ReviewGroundingRouteDeclarationV2", d)
	return ReviewGroundingRouteRefV2{ID: string(digest), Revision: 1, Digest: digest}
}
func validateResolvedProofV2(family ReviewGroundingRouteFamilyV2, declaration ReviewGroundingRouteDeclarationV2, route ReviewGroundingRouteRefV2, binding ReviewGroundingReaderBindingRefV2) error {
	if declaration.Validate() != nil || declaration.Family != family || route.Validate() != nil || binding.Validate() != nil || route != deriveGroundingRouteRefV2(declaration) || binding.Route != route {
		return groundingConflictV2("review grounding resolved route proof drifted")
	}
	return nil
}
func groundingRouteKeyV2(d ReviewGroundingRouteDeclarationV2) string {
	// Required is a sealed catalog obligation, not a caller coordinate. The
	// identity below binds every routable field, including the complete
	// BindingSet/revision/manifest/artifact and SourceContract, while allowing
	// a request (which has no Required field) to resolve the exact declaration.
	identity := ReviewGroundingRouteRequestV2{Family: d.Family, Kind: d.Kind, Owner: d.Owner}
	digest, _ := core.CanonicalJSONDigest("praxis.runtime.review-grounding-route-key", ReviewGroundingRouterContractV2, "ReviewGroundingRouteRequestV2", identity)
	return string(digest)
}
func groundingRouteRequestKeyV2(r ReviewGroundingRouteRequestV2) string {
	return groundingRouteKeyV2(ReviewGroundingRouteDeclarationV2{Family: r.Family, Kind: r.Kind, Owner: r.Owner})
}
func validGroundingFamilyV2(v ReviewGroundingRouteFamilyV2) bool {
	return v == ReviewGroundingArtifactRouteV2 || v == ReviewGroundingEnvironmentRouteV2 || v == ReviewGroundingValidationScopeRouteV2
}
func nilInterfaceV2(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
