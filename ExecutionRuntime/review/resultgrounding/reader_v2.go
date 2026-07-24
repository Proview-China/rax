package resultgrounding

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ResultBundleCurrentGroundingContractV2 = "praxis.review/result-bundle-current-grounding-v2"
	resultBundleGroundingDomainV2          = "praxis.review.result-bundle-current-grounding"
	maxReadRecoveryV2                      = 2 * time.Second
)

type ResultBundleCurrentGroundingRequestV2 struct {
	TenantID          core.TenantID                      `json:"tenant_id"`
	Bundle            contract.ExactResourceRefV1        `json:"bundle"`
	Request           contract.ExactResourceRefV1        `json:"request"`
	Target            contract.ExactResourceRefV1        `json:"target"`
	Case              contract.ExactResourceRefV1        `json:"case"`
	Round             contract.ExactResourceRefV1        `json:"round"`
	Assignment        contract.ExactResourceRefV1        `json:"assignment"`
	RunID             core.AgentRunID                    `json:"run_id"`
	ExecutionScope    core.ExecutionScope                `json:"execution_scope"`
	ActionScopeDigest core.Digest                        `json:"action_scope_digest"`
	Evidence          []runtimeports.ReviewEvidenceRefV2 `json:"evidence"`
	EvidenceSetDigest core.Digest                        `json:"evidence_set_digest"`
}

func (r ResultBundleCurrentGroundingRequestV2) Validate() error {
	if r.TenantID == "" || r.RunID == "" || r.ExecutionScope.Validate() != nil || r.ExecutionScope.Identity.TenantID != r.TenantID || r.ActionScopeDigest.Validate() != nil {
		return invalidV2("result grounding request scope is incomplete")
	}
	for _, ref := range []contract.ExactResourceRefV1{r.Bundle, r.Request, r.Target, r.Case, r.Round, r.Assignment} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if !sort.SliceIsSorted(r.Evidence, func(i, j int) bool { return evidenceKeyV2(r.Evidence[i]) < evidenceKeyV2(r.Evidence[j]) }) {
		return invalidCanonicalV2("result grounding evidence is not sorted")
	}
	for i, evidence := range r.Evidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
		if i > 0 && evidenceKeyV2(r.Evidence[i-1]) == evidenceKeyV2(evidence) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "result grounding evidence is duplicated")
		}
	}
	digest, err := contract.ComputeReviewEvidenceDigestV1(r.Evidence)
	if err != nil {
		return err
	}
	if digest != r.EvidenceSetDigest {
		return conflictV2("result grounding evidence digest drifted")
	}
	return nil
}

type ResultBundleGroundingStoredFactsV2 struct {
	Request    contract.ReviewRequestV1      `json:"request"`
	Target     contract.TargetSnapshotV1     `json:"target"`
	Bundle     contract.ReviewResultBundleV2 `json:"bundle"`
	Case       contract.ReviewCaseV1         `json:"case"`
	Round      contract.ReviewRoundV1        `json:"round"`
	Assignment contract.ReviewerAssignmentV1 `json:"assignment"`
}

func (v ResultBundleGroundingStoredFactsV2) Clone() ResultBundleGroundingStoredFactsV2 {
	v.Request.AttachmentEvidence = append([]runtimeports.ReviewEvidenceRefV2(nil), v.Request.AttachmentEvidence...)
	v.Target.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), v.Target.Evidence...)
	v.Bundle = v.Bundle.Clone()
	return v
}

func (v ResultBundleGroundingStoredFactsV2) ValidateAgainst(r ResultBundleCurrentGroundingRequestV2) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if v.Request.Validate() != nil || v.Target.Validate() != nil || v.Bundle.Validate() != nil || v.Case.Validate() != nil || v.Round.Validate() != nil || v.Assignment.Validate() != nil {
		return conflictV2("result grounding stored Review facts are invalid")
	}
	if exact(v.Request.FactIdentityV1) != r.Request || exact(v.Target.FactIdentityV1) != r.Target || exact(v.Bundle.FactIdentityV1) != r.Bundle || exact(v.Case.FactIdentityV1) != r.Case || exact(v.Round.FactIdentityV1) != r.Round || exact(v.Assignment.FactIdentityV1) != r.Assignment {
		return conflictV2("result grounding stored exact refs drifted")
	}
	if v.Bundle.Request != r.Request || v.Bundle.Target != r.Target || v.Request.ResultBundle != nil {
		return conflictV2("result bundle does not bind the exact Request and Target")
	}
	if v.Request.TenantID != r.TenantID || v.Target.TenantID != r.TenantID || v.Bundle.TenantID != r.TenantID || v.Case.TenantID != r.TenantID || v.Round.TenantID != r.TenantID || v.Assignment.TenantID != r.TenantID {
		return core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "result grounding crossed tenant boundary")
	}
	if v.Target.RunID != r.RunID || !runtimeports.SameExecutionScopeV2(v.Target.Scope, r.ExecutionScope) || v.Target.ActionScopeDigest != r.ActionScopeDigest || v.Target.EvidenceSetDigest != r.EvidenceSetDigest || !reflect.DeepEqual(v.Target.Evidence, r.Evidence) {
		return conflictV2("result grounding Run, Scope or Evidence drifted")
	}
	if v.Bundle.EvidenceSetDigest != r.EvidenceSetDigest {
		return conflictV2("result bundle Evidence set drifted from the grounding request")
	}
	if v.Case.TargetID != v.Target.ID || v.Case.TargetRevision != v.Target.Revision || v.Case.TargetDigest != v.Target.Digest || v.Round.CaseID != v.Case.ID || v.Round.TargetID != v.Target.ID || v.Round.TargetRevision != v.Target.Revision || v.Round.TargetDigest != v.Target.Digest || v.Assignment.CaseID != v.Case.ID || v.Assignment.RoundID != v.Round.ID || v.Assignment.TargetID != v.Target.ID || v.Assignment.TargetRevision != v.Target.Revision || v.Assignment.TargetDigest != v.Target.Digest {
		return conflictV2("result grounding Review chain drifted")
	}
	return nil
}

type ResultBundleGroundingStoredFactReaderV2 interface {
	InspectResultBundleGroundingStoredFactsV2(context.Context, ResultBundleCurrentGroundingRequestV2) (ResultBundleGroundingStoredFactsV2, error)
}

type ResultBundleCurrentGroundingProjectionV2 struct {
	ContractVersion                 string                                                                `json:"contract_version"`
	Bundle                          contract.ExactResourceRefV1                                           `json:"bundle"`
	Request                         contract.ExactResourceRefV1                                           `json:"request"`
	Target                          contract.ExactResourceRefV1                                           `json:"target"`
	Context                         contract.ReviewerContextEnvelopeV1                                    `json:"context"`
	OriginalIntent                  contract.ReviewerContextMaterialV1                                    `json:"original_intent"`
	AcceptanceCriteria              []contract.ReviewerContextMaterialV1                                  `json:"acceptance_criteria"`
	ArtifactRoutes                  []runtimeports.ReviewArtifactResolvedRouteProofV2                     `json:"artifact_routes"`
	Artifacts                       []runtimeports.ReviewArtifactCurrentProjectionV2                      `json:"artifacts"`
	EnvironmentRoute                runtimeports.ReviewEnvironmentResolvedRouteProofV2                    `json:"environment_route"`
	Environment                     runtimeports.ReviewEnvironmentCurrentProjectionV2                     `json:"environment"`
	ValidationScopeOwnerAssociation runtimeports.ReviewValidationScopeOwnerAssociationCurrentProjectionV2 `json:"validation_scope_owner_association"`
	ValidationScopeRoute            runtimeports.ReviewValidationScopeResolvedRouteProofV2                `json:"validation_scope_route"`
	ValidationScope                 runtimeports.ReviewValidationScopeCurrentProjectionV2                 `json:"validation_scope"`
	Evidence                        []runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1           `json:"evidence"`
	CheckedUnixNano                 int64                                                                 `json:"checked_unix_nano"`
	ExpiresUnixNano                 int64                                                                 `json:"expires_unix_nano"`
	ProjectionDigest                core.Digest                                                           `json:"projection_digest"`
}

func (p ResultBundleCurrentGroundingProjectionV2) Clone() ResultBundleCurrentGroundingProjectionV2 {
	p.Context = p.Context.Clone()
	p.AcceptanceCriteria = append([]contract.ReviewerContextMaterialV1(nil), p.AcceptanceCriteria...)
	p.ArtifactRoutes = append([]runtimeports.ReviewArtifactResolvedRouteProofV2(nil), p.ArtifactRoutes...)
	p.Artifacts = append([]runtimeports.ReviewArtifactCurrentProjectionV2(nil), p.Artifacts...)
	for i := range p.Artifacts {
		p.Artifacts[i] = p.Artifacts[i].Clone()
	}
	p.Environment = p.Environment.Clone()
	p.ValidationScope = p.ValidationScope.Clone()
	p.Evidence = append([]runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1(nil), p.Evidence...)
	for i := range p.Evidence {
		p.Evidence[i] = runtimeports.CloneReviewEvidenceApplicabilityCurrentSnapshotV1(p.Evidence[i])
	}
	return p
}

func (p ResultBundleCurrentGroundingProjectionV2) Validate() error {
	if p.ContractVersion != ResultBundleCurrentGroundingContractV2 || p.Bundle.Validate() != nil || p.Request.Validate() != nil || p.Target.Validate() != nil || p.Context.Validate() != nil || p.OriginalIntent.Validate() != nil || len(p.AcceptanceCriteria) == 0 || len(p.ArtifactRoutes) == 0 || len(p.ArtifactRoutes) != len(p.Artifacts) || p.EnvironmentRoute.Validate() != nil || p.Environment.Validate() != nil || p.ValidationScopeOwnerAssociation.Validate() != nil || p.ValidationScopeRoute.Validate() != nil || p.ValidationScope.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return invalidV2("result grounding projection is incomplete")
	}
	minimum := p.Context.ExpiresUnixNano
	for i, artifact := range p.Artifacts {
		if artifact.Validate() != nil || p.ArtifactRoutes[i].Validate() != nil || p.ArtifactRoutes[i].Declaration.Owner != artifact.Source.Owner || p.ArtifactRoutes[i].Declaration.Kind != artifact.Source.Kind {
			return conflictV2("result grounding artifact route or projection drifted")
		}
		minimum = minTTL(minimum, artifact.ExpiresUnixNano, artifact.OwnerBinding.ExpiresUnixNano)
	}
	if p.EnvironmentRoute.Declaration.Owner != p.Environment.Source.Owner || p.EnvironmentRoute.Declaration.Kind != p.Environment.Source.Kind {
		return conflictV2("result grounding Environment route or projection drifted")
	}
	if p.ValidationScopeOwnerAssociation.Subject.Source != p.ValidationScope.Source.Source || p.ValidationScopeOwnerAssociation.Owner != p.ValidationScope.Source.Owner || p.ValidationScopeRoute.Declaration.Owner != p.ValidationScope.Source.Owner || p.ValidationScopeRoute.Declaration.Kind != p.ValidationScope.Source.Source.Kind {
		return conflictV2("result grounding Validation Scope association, route or projection drifted")
	}
	minimum = minTTL(minimum, p.Environment.ExpiresUnixNano, p.Environment.OwnerBinding.ExpiresUnixNano, p.ValidationScopeOwnerAssociation.ExpiresUnixNano, p.ValidationScope.ExpiresUnixNano, p.ValidationScope.OwnerBinding.ExpiresUnixNano)
	for _, evidence := range p.Evidence {
		if evidence.Validate() != nil {
			return conflictV2("result grounding Evidence snapshot is invalid")
		}
		minimum = minTTL(minimum, evidence.Projection.ExpiresUnixNano)
	}
	for _, material := range append([]contract.ReviewerContextMaterialV1{p.OriginalIntent}, p.AcceptanceCriteria...) {
		if material.Validate() != nil {
			return conflictV2("result grounding Context material is invalid")
		}
		minimum = minTTL(minimum, material.Source.ExpiresUnixNano)
	}
	// Request/Target/Case/Round/Assignment/Bundle TTLs are exact Review facts,
	// represented by full refs rather than duplicated bodies in this cut. The
	// Owner reader sets their true minimum; standalone validation can prove the
	// sealed expiry does not exceed any embedded external source.
	if p.ExpiresUnixNano > minimum {
		return conflictV2("result grounding projection exceeds an embedded current TTL")
	}
	copy := p.Clone()
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest(resultBundleGroundingDomainV2, ResultBundleCurrentGroundingContractV2, "ResultBundleCurrentGroundingProjectionV2", copy)
	if err != nil {
		return err
	}
	if digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "result grounding projection digest drifted")
	}
	return nil
}

func (p ResultBundleCurrentGroundingProjectionV2) ValidateCurrent(bundle contract.ExactResourceRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Bundle != bundle {
		return conflictV2("result grounding bundle exact ref drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "result grounding clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return staleV2("result grounding projection expired")
	}
	return nil
}

func SealResultBundleCurrentGroundingProjectionV2(p ResultBundleCurrentGroundingProjectionV2) (ResultBundleCurrentGroundingProjectionV2, error) {
	p.ContractVersion = ResultBundleCurrentGroundingContractV2
	p.ProjectionDigest = ""
	copy := p.Clone()
	digest, err := core.CanonicalJSONDigest(resultBundleGroundingDomainV2, ResultBundleCurrentGroundingContractV2, "ResultBundleCurrentGroundingProjectionV2", copy)
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type ResultBundleGroundingReadRecoveryPolicyV2 struct {
	ReadRecoveryTimeoutNanos int64 `json:"read_recovery_timeout_nanos"`
}

func (p ResultBundleGroundingReadRecoveryPolicyV2) Validate() error {
	timeout := time.Duration(p.ReadRecoveryTimeoutNanos)
	if timeout <= 0 || timeout > maxReadRecoveryV2 {
		return invalidV2("result grounding recovery timeout must be within (0,2s]")
	}
	return nil
}

type ResultBundleCurrentGroundingDependenciesV2 struct {
	Stored                          ResultBundleGroundingStoredFactReaderV2
	Context                         reviewport.ReviewerContextCurrentReaderV1
	Binding                         runtimeports.ReviewBindingAuthoritativeCurrentReaderV1
	Evidence                        runtimeports.ReviewEvidenceApplicabilityCurrentReaderV1
	ValidationScopeOwnerAssociation runtimeports.ReviewValidationScopeOwnerAssociationCurrentReaderV2
	Routes                          runtimeports.ReviewGroundingReaderResolverV2
	Clock                           func() time.Time
}

func (d ResultBundleCurrentGroundingDependenciesV2) Validate() error {
	if nilcheck.IsNil(d.Stored) || nilcheck.IsNil(d.Context) || nilcheck.IsNil(d.Binding) || nilcheck.IsNil(d.Evidence) || nilcheck.IsNil(d.ValidationScopeOwnerAssociation) || nilcheck.IsNil(d.Routes) || d.Clock == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "result grounding requires every exact current Reader and clock")
	}
	return nil
}

type ResultBundleCurrentGroundingReaderV2 interface {
	InspectResultBundleCurrentGroundingV2(context.Context, ResultBundleCurrentGroundingRequestV2) (ResultBundleCurrentGroundingProjectionV2, error)
}

type readerV2 struct {
	policy ResultBundleGroundingReadRecoveryPolicyV2
	deps   ResultBundleCurrentGroundingDependenciesV2
}

type s1ReadErrorV2 struct {
	err     error
	expires int64
}

func (e *s1ReadErrorV2) Error() string { return e.err.Error() }
func (e *s1ReadErrorV2) Unwrap() error { return e.err }

func markS1ReadV2(err error, expires int64) error {
	err = normalizeReadErrorV2(err)
	if err == nil || (!core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorUnavailable)) {
		return err
	}
	return &s1ReadErrorV2{err: err, expires: expires}
}

func NewResultBundleCurrentGroundingReaderV2(p ResultBundleGroundingReadRecoveryPolicyV2, d ResultBundleCurrentGroundingDependenciesV2) (ResultBundleCurrentGroundingReaderV2, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &readerV2{policy: p, deps: d}, nil
}

type artifactCutV2 struct {
	route   runtimeports.ReviewArtifactResolvedRouteV2
	subject runtimeports.ReviewArtifactCurrentSubjectV2
	binding runtimeports.ReviewBindingCurrentProjectionV1
	value   runtimeports.ReviewArtifactCurrentProjectionV2
}
type environmentCutV2 struct {
	route   runtimeports.ReviewEnvironmentResolvedRouteV2
	subject runtimeports.ReviewEnvironmentCurrentSubjectV2
	binding runtimeports.ReviewBindingCurrentProjectionV1
	value   runtimeports.ReviewEnvironmentCurrentProjectionV2
}
type scopeCutV2 struct {
	association runtimeports.ReviewValidationScopeOwnerAssociationCurrentProjectionV2
	route       runtimeports.ReviewValidationScopeResolvedRouteV2
	subject     runtimeports.ReviewValidationScopeCurrentSubjectV2
	binding     runtimeports.ReviewBindingCurrentProjectionV1
	value       runtimeports.ReviewValidationScopeCurrentProjectionV2
}

func (r *readerV2) InspectResultBundleCurrentGroundingV2(ctx context.Context, request ResultBundleCurrentGroundingRequestV2) (ResultBundleCurrentGroundingProjectionV2, error) {
	if err := request.Validate(); err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	baseline := r.deps.Clock()
	if baseline.IsZero() {
		return ResultBundleCurrentGroundingProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "result grounding baseline clock is zero")
	}
	guarded := r.withClockWatermarkV2(baseline)
	value, err := guarded.inspectOnceV2(ctx, request)
	var lost *s1ReadErrorV2
	if err == nil || !errors.As(err, &lost) {
		return value, err
	}
	original := lost.err
	now := guarded.deps.Clock()
	timeout := time.Duration(r.policy.ReadRecoveryTimeoutNanos)
	if now.IsZero() {
		return ResultBundleCurrentGroundingProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "result grounding clock regressed before S1 recovery")
	}
	if lost.expires <= now.UnixNano() {
		return ResultBundleCurrentGroundingProjectionV2{}, original
	}
	if ttl := time.Duration(lost.expires - now.UnixNano()); ttl < timeout {
		timeout = ttl
	}
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := deadline.Sub(now); remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		return ResultBundleCurrentGroundingProjectionV2{}, original
	}
	recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	recovered, retryErr := guarded.inspectOnceV2(recovery, request)
	if retryErr != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, original
	}
	return recovered, nil
}

func (r *readerV2) withClockWatermarkV2(baseline time.Time) *readerV2 {
	var mu sync.Mutex
	last := baseline
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now := r.deps.Clock()
		if now.IsZero() || now.Before(last) {
			return time.Time{}
		}
		last = now
		return now
	}
	copy := *r
	copy.deps = r.deps
	copy.deps.Clock = clock
	return &copy
}

func (r *readerV2) inspectOnceV2(ctx context.Context, request ResultBundleCurrentGroundingRequestV2) (ResultBundleCurrentGroundingProjectionV2, error) {
	if err := request.Validate(); err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	baseline := r.deps.Clock()
	if baseline.IsZero() {
		return ResultBundleCurrentGroundingProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "result grounding baseline clock is zero")
	}
	stored, err := ownerReadV2(r, func() (ResultBundleGroundingStoredFactsV2, error) {
		return r.deps.Stored.InspectResultBundleGroundingStoredFactsV2(ctx, request)
	})
	if err != nil {
		// No trustworthy TTL exists until the exact stored cut is available, so
		// this read is not detached/retried. Raw cancellation/deadline still
		// belongs to the closed Indeterminate class rather than escaping as an
		// untyped transport error.
		return ResultBundleCurrentGroundingProjectionV2{}, normalizeReadErrorV2(err)
	}
	if err := stored.ValidateAgainst(request); err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	contextSubject, err := reviewerContextSubjectV2(stored)
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	contextRef, err := ownerReadV2(r, func() (contract.ReviewerContextEnvelopeRefV1, error) {
		return r.deps.Context.ResolveCurrentReviewerContextV1(ctx, reviewport.ReviewerContextCurrentResolveRequestV1{Subject: contextSubject})
	})
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, stored.Request.ExpiresUnixNano, stored.Target.ExpiresUnixNano, stored.Case.ExpiresUnixNano, stored.Round.ExpiresUnixNano, stored.Assignment.ExpiresUnixNano))
	}
	if contextRef != stored.Bundle.ReviewerContext {
		return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Reviewer Context S1 ref drifted from Bundle")
	}
	contextS1, err := ownerReadV2(r, func() (contract.ReviewerContextEnvelopeV1, error) {
		return r.deps.Context.InspectCurrentReviewerContextV1(ctx, contextSubject, contextRef)
	})
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, stored.Request.ExpiresUnixNano, stored.Target.ExpiresUnixNano, stored.Case.ExpiresUnixNano, stored.Round.ExpiresUnixNano, stored.Assignment.ExpiresUnixNano))
	}
	original, criteria, err := validateContextBundleV2(stored.Bundle, contextS1)
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	bindingSubject := runtimeports.ReviewBindingSubjectV1{TenantID: request.TenantID, AssignmentID: stored.Assignment.ID, AssignmentRevision: stored.Assignment.Revision, AssignmentDigest: stored.Assignment.Digest, ReviewerID: stored.Assignment.ReviewerID, TargetID: stored.Target.ID, TargetRevision: stored.Target.Revision, TargetDigest: stored.Target.Digest}
	artifacts := make([]artifactCutV2, 0, len(stored.Bundle.Artifacts))
	for _, artifact := range stored.Bundle.Artifacts {
		binding, err := r.resolveBindingV2(ctx, artifact.Source.Owner.Binding, bindingSubject)
		if err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano))
		}
		route, err := ownerReadV2(r, func() (runtimeports.ReviewArtifactResolvedRouteV2, error) {
			return r.deps.Routes.ResolveReviewArtifactReaderV2(ctx, runtimeports.ReviewGroundingRouteRequestV2{Family: runtimeports.ReviewGroundingArtifactRouteV2, Kind: artifact.Source.Kind, Owner: artifact.Source.Owner})
		})
		if err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, binding.ExpiresUnixNano))
		}
		subject := runtimeports.ReviewArtifactCurrentSubjectV2{Expected: artifact.Source, Anchors: append([]runtimeports.ReviewArtifactLocatorV2(nil), artifact.Anchors...)}
		ref, err := ownerReadV2(r, func() (runtimeports.ReviewArtifactCurrentProjectionRefV2, error) {
			return route.Reader.ResolveCurrentReviewArtifactV2(ctx, subject)
		})
		if err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, binding.ExpiresUnixNano))
		}
		value, err := ownerReadV2(r, func() (runtimeports.ReviewArtifactCurrentProjectionV2, error) {
			return route.Reader.InspectCurrentReviewArtifactV2(ctx, subject, ref)
		})
		if err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, binding.ExpiresUnixNano))
		}
		if value.OwnerBinding.Ref != binding.Ref || !reflect.DeepEqual(value.OwnerBinding, binding) {
			return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Artifact Owner Binding drifted")
		}
		artifacts = append(artifacts, artifactCutV2{route: route, subject: subject, binding: binding, value: value})
	}
	environmentBinding, err := r.resolveBindingV2(ctx, stored.Bundle.Environment.Owner.Binding, bindingSubject)
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano))
	}
	environmentRoute, err := ownerReadV2(r, func() (runtimeports.ReviewEnvironmentResolvedRouteV2, error) {
		return r.deps.Routes.ResolveReviewEnvironmentReaderV2(ctx, runtimeports.ReviewGroundingRouteRequestV2{Family: runtimeports.ReviewGroundingEnvironmentRouteV2, Kind: stored.Bundle.Environment.Kind, Owner: stored.Bundle.Environment.Owner})
	})
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, environmentBinding.ExpiresUnixNano))
	}
	environmentSubject := runtimeports.ReviewEnvironmentCurrentSubjectV2{Expected: stored.Bundle.Environment}
	environmentRef, err := ownerReadV2(r, func() (runtimeports.ReviewEnvironmentCurrentProjectionRefV2, error) {
		return environmentRoute.Reader.ResolveCurrentReviewEnvironmentV2(ctx, environmentSubject)
	})
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, environmentBinding.ExpiresUnixNano))
	}
	environment, err := ownerReadV2(r, func() (runtimeports.ReviewEnvironmentCurrentProjectionV2, error) {
		return environmentRoute.Reader.InspectCurrentReviewEnvironmentV2(ctx, environmentSubject, environmentRef)
	})
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, environmentBinding.ExpiresUnixNano))
	}
	if !reflect.DeepEqual(environment.OwnerBinding, environmentBinding) {
		return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Environment Owner Binding drifted")
	}
	scopeSubjectIdentity := runtimeports.ReviewValidationScopeOwnerAssociationSubjectV2{Source: stored.Bundle.ValidationScope.Source}
	associationRef, err := ownerReadV2(r, func() (runtimeports.ReviewValidationScopeOwnerAssociationRefV2, error) {
		return r.deps.ValidationScopeOwnerAssociation.ResolveCurrentReviewValidationScopeOwnerAssociationV2(ctx, scopeSubjectIdentity)
	})
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, environment.ExpiresUnixNano))
	}
	association, err := ownerReadV2(r, func() (runtimeports.ReviewValidationScopeOwnerAssociationCurrentProjectionV2, error) {
		return r.deps.ValidationScopeOwnerAssociation.InspectCurrentReviewValidationScopeOwnerAssociationV2(ctx, scopeSubjectIdentity, associationRef)
	})
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, environment.ExpiresUnixNano))
	}
	if association.Owner != stored.Bundle.ValidationScope.Owner {
		return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Validation Scope Owner association drifted")
	}
	scopeBinding, err := r.resolveBindingV2(ctx, association.Owner.Binding, bindingSubject)
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, association.ExpiresUnixNano))
	}
	scopeRoute, err := ownerReadV2(r, func() (runtimeports.ReviewValidationScopeResolvedRouteV2, error) {
		return r.deps.Routes.ResolveReviewValidationScopeReaderV2(ctx, runtimeports.ReviewGroundingRouteRequestV2{Family: runtimeports.ReviewGroundingValidationScopeRouteV2, Kind: stored.Bundle.ValidationScope.Source.Kind, Owner: association.Owner})
	})
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, association.ExpiresUnixNano, scopeBinding.ExpiresUnixNano))
	}
	locatorSetDigest, err := digestBundleLocatorsV2(stored.Bundle)
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	scopeSubject := runtimeports.ReviewValidationScopeCurrentSubjectV2{Expected: stored.Bundle.ValidationScope, CoveredArtifactLocatorSetDigest: locatorSetDigest, EvidenceSetDigest: stored.Bundle.EvidenceSetDigest}
	scopeRef, err := ownerReadV2(r, func() (runtimeports.ReviewValidationScopeCurrentProjectionRefV2, error) {
		return scopeRoute.Reader.ResolveCurrentReviewValidationScopeV2(ctx, scopeSubject)
	})
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, association.ExpiresUnixNano, scopeBinding.ExpiresUnixNano))
	}
	scope, err := ownerReadV2(r, func() (runtimeports.ReviewValidationScopeCurrentProjectionV2, error) {
		return scopeRoute.Reader.InspectCurrentReviewValidationScopeV2(ctx, scopeSubject, scopeRef)
	})
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, association.ExpiresUnixNano, scopeBinding.ExpiresUnixNano))
	}
	if !reflect.DeepEqual(scope.OwnerBinding, scopeBinding) {
		return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Validation Scope Owner Binding drifted")
	}
	evidence := make([]runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, 0, len(request.Evidence))
	for _, item := range request.Evidence {
		subject := runtimeports.ReviewEvidenceApplicabilitySubjectV1{TenantID: request.TenantID, Target: runtimeports.ReviewEvidenceTargetRefV1{ID: stored.Target.ID, Revision: stored.Target.Revision, Digest: stored.Target.Digest}, RunID: request.RunID, Scope: request.ExecutionScope, ActionScopeDigest: request.ActionScopeDigest, ReviewEvidence: item}
		snapshot, err := ownerReadV2(r, func() (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
			return r.deps.Evidence.ResolveReviewEvidenceApplicabilityCurrentV1(ctx, runtimeports.ResolveReviewEvidenceApplicabilityCurrentRequestV1{ContractVersion: runtimeports.ReviewEvidenceCurrentContractVersionV1, Subject: subject})
		})
		if err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, markS1ReadV2(err, minTTL(stored.Bundle.ExpiresUnixNano, contextS1.ExpiresUnixNano, environment.ExpiresUnixNano, association.ExpiresUnixNano, scope.ExpiresUnixNano))
		}
		evidence = append(evidence, snapshot)
	}
	storedS2, err := recoverExactReadV2(r, ctx, minTTL(stored.Bundle.ExpiresUnixNano, stored.Request.ExpiresUnixNano, stored.Target.ExpiresUnixNano, stored.Case.ExpiresUnixNano, stored.Round.ExpiresUnixNano, stored.Assignment.ExpiresUnixNano), func(readCtx context.Context) (ResultBundleGroundingStoredFactsV2, error) {
		return r.deps.Stored.InspectResultBundleGroundingStoredFactsV2(readCtx, request)
	})
	if err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	if !reflect.DeepEqual(stored, storedS2) {
		return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("stored Review facts drifted between S1 and S2")
	}
	contextS2, err := recoverExactReadV2(r, ctx, contextS1.ExpiresUnixNano, func(readCtx context.Context) (contract.ReviewerContextEnvelopeV1, error) {
		return r.deps.Context.InspectCurrentReviewerContextV1(readCtx, contextSubject, contextRef)
	})
	if err != nil || !reflect.DeepEqual(contextS1, contextS2) {
		if err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, err
		}
		return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Reviewer Context drifted between S1 and S2")
	}
	for i := range artifacts {
		value, err := recoverExactReadV2(r, ctx, minTTL(artifacts[i].value.ExpiresUnixNano, artifacts[i].binding.ExpiresUnixNano), func(readCtx context.Context) (runtimeports.ReviewArtifactCurrentProjectionV2, error) {
			return artifacts[i].route.Reader.InspectCurrentReviewArtifactV2(readCtx, artifacts[i].subject, artifacts[i].value.Ref)
		})
		if err != nil || !reflect.DeepEqual(value, artifacts[i].value) {
			if err != nil {
				return ResultBundleCurrentGroundingProjectionV2{}, err
			}
			return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Artifact drifted between S1 and S2")
		}
		binding, err := recoverExactReadV2(r, ctx, artifacts[i].binding.ExpiresUnixNano, func(readCtx context.Context) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
			return r.inspectBindingV2(readCtx, artifacts[i].binding, artifacts[i].subject.Expected.Owner.Binding, bindingSubject)
		})
		if err != nil || !reflect.DeepEqual(binding, artifacts[i].binding) {
			if err != nil {
				return ResultBundleCurrentGroundingProjectionV2{}, err
			}
			return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Artifact Binding drifted between S1 and S2")
		}
	}
	environmentS2, err := recoverExactReadV2(r, ctx, minTTL(environment.ExpiresUnixNano, environmentBinding.ExpiresUnixNano), func(readCtx context.Context) (runtimeports.ReviewEnvironmentCurrentProjectionV2, error) {
		return environmentRoute.Reader.InspectCurrentReviewEnvironmentV2(readCtx, environmentSubject, environment.Ref)
	})
	if err != nil || !reflect.DeepEqual(environmentS2, environment) {
		if err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, err
		}
		return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Environment drifted between S1 and S2")
	}
	environmentBindingS2, err := recoverExactReadV2(r, ctx, environmentBinding.ExpiresUnixNano, func(readCtx context.Context) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
		return r.inspectBindingV2(readCtx, environmentBinding, stored.Bundle.Environment.Owner.Binding, bindingSubject)
	})
	if err != nil || !reflect.DeepEqual(environmentBindingS2, environmentBinding) {
		if err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, err
		}
		return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Environment Binding drifted between S1 and S2")
	}
	associationS2, err := recoverExactReadV2(r, ctx, association.ExpiresUnixNano, func(readCtx context.Context) (runtimeports.ReviewValidationScopeOwnerAssociationCurrentProjectionV2, error) {
		return r.deps.ValidationScopeOwnerAssociation.InspectCurrentReviewValidationScopeOwnerAssociationV2(readCtx, scopeSubjectIdentity, association.Ref)
	})
	if err != nil || !reflect.DeepEqual(associationS2, association) {
		if err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, err
		}
		return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Validation Scope Owner association drifted between S1 and S2")
	}
	scopeS2, err := recoverExactReadV2(r, ctx, minTTL(scope.ExpiresUnixNano, scopeBinding.ExpiresUnixNano), func(readCtx context.Context) (runtimeports.ReviewValidationScopeCurrentProjectionV2, error) {
		return scopeRoute.Reader.InspectCurrentReviewValidationScopeV2(readCtx, scopeSubject, scope.Ref)
	})
	if err != nil || !reflect.DeepEqual(scopeS2, scope) {
		if err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, err
		}
		return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Validation Scope drifted between S1 and S2")
	}
	scopeBindingS2, err := recoverExactReadV2(r, ctx, scopeBinding.ExpiresUnixNano, func(readCtx context.Context) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
		return r.inspectBindingV2(readCtx, scopeBinding, association.Owner.Binding, bindingSubject)
	})
	if err != nil || !reflect.DeepEqual(scopeBindingS2, scopeBinding) {
		if err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, err
		}
		return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Validation Scope Binding drifted between S1 and S2")
	}
	for i := range evidence {
		current, err := recoverExactReadV2(r, ctx, evidence[i].Projection.ExpiresUnixNano, func(readCtx context.Context) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
			return r.deps.Evidence.InspectCurrentReviewEvidenceApplicabilityV1(readCtx, evidence[i].Projection.Ref)
		})
		if err != nil || !reflect.DeepEqual(current, evidence[i]) {
			if err != nil {
				return ResultBundleCurrentGroundingProjectionV2{}, err
			}
			return ResultBundleCurrentGroundingProjectionV2{}, conflictV2("Evidence drifted between S1 and S2")
		}
	}
	now := r.deps.Clock()
	if now.IsZero() || now.Before(baseline) {
		return ResultBundleCurrentGroundingProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "result grounding clock regressed during read")
	}
	if err := contextS1.ValidateCurrent(contextRef, contextSubject, now); err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	minimum := minTTL(stored.Bundle.ExpiresUnixNano, stored.Request.ExpiresUnixNano, stored.Target.ExpiresUnixNano, stored.Case.ExpiresUnixNano, stored.Round.ExpiresUnixNano, stored.Assignment.ExpiresUnixNano, contextS1.ExpiresUnixNano)
	for _, item := range artifacts {
		if err := item.value.ValidateCurrent(item.value.Ref, item.subject, now); err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, err
		}
		if err := item.binding.ValidateCurrent(item.binding.Ref, item.subject.Expected.Owner.Binding, bindingSubject, now); err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, err
		}
		minimum = minTTL(minimum, item.value.ExpiresUnixNano, item.binding.ExpiresUnixNano)
	}
	if err := environment.ValidateCurrent(environment.Ref, environmentSubject, now); err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	if err := environmentBinding.ValidateCurrent(environmentBinding.Ref, stored.Bundle.Environment.Owner.Binding, bindingSubject, now); err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	if err := association.ValidateCurrent(association.Ref, scopeSubjectIdentity, association.Owner, now); err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	if err := scope.ValidateCurrent(scope.Ref, scopeSubject, now); err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	if err := scopeBinding.ValidateCurrent(scopeBinding.Ref, association.Owner.Binding, bindingSubject, now); err != nil {
		return ResultBundleCurrentGroundingProjectionV2{}, err
	}
	minimum = minTTL(minimum, environment.ExpiresUnixNano, environmentBinding.ExpiresUnixNano, association.ExpiresUnixNano, scope.ExpiresUnixNano, scopeBinding.ExpiresUnixNano)
	minimum = minTTL(minimum, original.Source.ExpiresUnixNano)
	for _, criterion := range criteria {
		minimum = minTTL(minimum, criterion.Source.ExpiresUnixNano)
	}
	for _, item := range evidence {
		if err := item.ValidateCurrent(item.Projection.Ref, now); err != nil {
			return ResultBundleCurrentGroundingProjectionV2{}, err
		}
		minimum = minTTL(minimum, item.Projection.ExpiresUnixNano)
	}
	if !now.Before(time.Unix(0, minimum)) {
		return ResultBundleCurrentGroundingProjectionV2{}, staleV2("result grounding minimum TTL crossed")
	}
	out := ResultBundleCurrentGroundingProjectionV2{Bundle: request.Bundle, Request: request.Request, Target: request.Target, Context: contextS1, OriginalIntent: original, AcceptanceCriteria: criteria, EnvironmentRoute: environmentRoute.Proof, Environment: environment, ValidationScopeOwnerAssociation: association, ValidationScopeRoute: scopeRoute.Proof, ValidationScope: scope, Evidence: evidence, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: minimum}
	for _, item := range artifacts {
		out.ArtifactRoutes = append(out.ArtifactRoutes, item.route.Proof)
		out.Artifacts = append(out.Artifacts, item.value)
	}
	return SealResultBundleCurrentGroundingProjectionV2(out)
}

func (r *readerV2) resolveBindingV2(ctx context.Context, source runtimeports.ReviewComponentBindingRefV2, subject runtimeports.ReviewBindingSubjectV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	ref, err := ownerReadV2(r, func() (runtimeports.ReviewBindingProjectionRefV1, error) {
		return r.deps.Binding.ResolveCurrentReviewBindingV1(ctx, runtimeports.ResolveReviewBindingCurrentRequestV1{Source: source, Subject: subject})
	})
	if err != nil {
		return runtimeports.ReviewBindingCurrentProjectionV1{}, err
	}
	return r.inspectBindingV2(ctx, runtimeports.ReviewBindingCurrentProjectionV1{Ref: ref}, source, subject)
}
func (r *readerV2) inspectBindingV2(ctx context.Context, expected runtimeports.ReviewBindingCurrentProjectionV1, source runtimeports.ReviewComponentBindingRefV2, subject runtimeports.ReviewBindingSubjectV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	return ownerReadV2(r, func() (runtimeports.ReviewBindingCurrentProjectionV1, error) {
		return r.deps.Binding.InspectCurrentReviewBindingV1(ctx, runtimeports.InspectCurrentReviewBindingRequestV1{ExpectedRef: expected.Ref, ExpectedSource: source, ExpectedSubject: subject})
	})
}

// ownerReadV2 places every injected Owner call between two readings from the
// single monotonic clock domain. A rollback that occurs during any read is
// therefore observable even if the underlying Owner returns a superficially
// valid projection.
func ownerReadV2[T any](r *readerV2, read func() (T, error)) (T, error) {
	before := r.deps.Clock()
	if before.IsZero() {
		var zero T
		return zero, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "result grounding clock regressed before Owner read")
	}
	value, err := read()
	after := r.deps.Clock()
	if after.IsZero() || after.Before(before) {
		var zero T
		return zero, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "result grounding clock regressed across Owner read")
	}
	return value, normalizeReadErrorV2(err)
}

// recoverExactReadV2 retries only an exact S2 Inspect. It never calls Resolve,
// never mutates an Owner, and preserves the original unknown/unavailable error
// if the single bounded recovery cannot prove the exact result.
func recoverExactReadV2[T any](r *readerV2, caller context.Context, knownExpires int64, inspect func(context.Context) (T, error)) (T, error) {
	value, err := ownerReadV2(r, func() (T, error) { return inspect(caller) })
	if err == nil || (!core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorUnavailable)) {
		return value, err
	}
	original := err
	now := r.deps.Clock()
	timeout := time.Duration(r.policy.ReadRecoveryTimeoutNanos)
	if now.IsZero() {
		var zero T
		return zero, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "result grounding clock regressed before exact recovery")
	}
	if knownExpires <= now.UnixNano() {
		var zero T
		return zero, original
	}
	if ttl := time.Duration(knownExpires - now.UnixNano()); ttl < timeout {
		timeout = ttl
	}
	if deadline, ok := caller.Deadline(); ok {
		if remaining := deadline.Sub(now); remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		var zero T
		return zero, original
	}
	recovery, cancel := context.WithTimeout(context.WithoutCancel(caller), timeout)
	defer cancel()
	recovered, retryErr := ownerReadV2(r, func() (T, error) { return inspect(recovery) })
	if retryErr != nil {
		var zero T
		return zero, original
	}
	return recovered, nil
}

func normalizeReadErrorV2(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "result grounding Owner read completion is unknown")
	}
	return err
}

type storeFactReaderV2 struct {
	v1 reviewport.StoreV1
	v2 reviewport.ResultBundleStoreV2
}

func NewStoreFactReaderV2(v1 reviewport.StoreV1, v2 reviewport.ResultBundleStoreV2) (ResultBundleGroundingStoredFactReaderV2, error) {
	if nilcheck.IsNil(v1) || nilcheck.IsNil(v2) {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "result grounding stored Reader requires Review V1 and V2 Stores")
	}
	return &storeFactReaderV2{v1: v1, v2: v2}, nil
}

func (r *storeFactReaderV2) InspectResultBundleGroundingStoredFactsV2(ctx context.Context, request ResultBundleCurrentGroundingRequestV2) (ResultBundleGroundingStoredFactsV2, error) {
	if err := request.Validate(); err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	var out ResultBundleGroundingStoredFactsV2
	var err error
	if out.Request, err = r.v1.InspectRequestExactV1(ctx, request.TenantID, toStoreRef(request.Request)); err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	if out.Target, err = r.v1.InspectTargetExactV1(ctx, request.TenantID, toStoreRef(request.Target)); err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	if out.Bundle, err = r.v2.InspectResultBundleExactV2(ctx, request.TenantID, toStoreRef(request.Bundle)); err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	if out.Case, err = r.v1.InspectCaseExactV1(ctx, request.TenantID, toStoreRef(request.Case)); err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	if out.Round, err = r.v1.InspectRoundExactV1(ctx, request.TenantID, toStoreRef(request.Round)); err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	if out.Assignment, err = r.v1.InspectAssignmentExactV1(ctx, request.TenantID, toStoreRef(request.Assignment)); err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	currentRequest, err := r.v1.InspectRequestByCaseV1(ctx, request.TenantID, out.Case.ID)
	if err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	currentTarget, err := r.v1.InspectTargetV1(ctx, request.TenantID, out.Target.ID)
	if err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	currentCase, err := r.v1.InspectCaseV1(ctx, request.TenantID, out.Case.ID)
	if err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	currentRound, err := r.v1.InspectRoundV1(ctx, request.TenantID, out.Round.ID)
	if err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	currentAssignment, err := r.v1.InspectAssignmentV1(ctx, request.TenantID, out.Assignment.ID)
	if err != nil {
		return ResultBundleGroundingStoredFactsV2{}, err
	}
	if exact(currentRequest.FactIdentityV1) != request.Request || exact(currentTarget.FactIdentityV1) != request.Target || exact(currentCase.FactIdentityV1) != request.Case || exact(currentRound.FactIdentityV1) != request.Round || exact(currentAssignment.FactIdentityV1) != request.Assignment {
		return ResultBundleGroundingStoredFactsV2{}, conflictV2("result grounding Review current index drifted")
	}
	return out.Clone(), out.ValidateAgainst(request)
}

func reviewerContextSubjectV2(v ResultBundleGroundingStoredFactsV2) (contract.ReviewerContextSubjectV1, error) {
	if v.Round.Rubric == nil {
		return contract.ReviewerContextSubjectV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "result grounding requires exact Round Rubric")
	}
	return contract.ReviewerContextSubjectV1{TenantID: v.Target.TenantID, Case: exact(v.Case.FactIdentityV1), Round: exact(v.Round.FactIdentityV1), Assignment: exact(v.Assignment.FactIdentityV1), Target: exact(v.Target.FactIdentityV1), Rubric: *v.Round.Rubric, ContextFrameDigest: v.Round.ContextFrameDigest, OutputSchema: v.Target.PayloadSchema}, nil
}
func validateContextBundleV2(bundle contract.ReviewResultBundleV2, envelope contract.ReviewerContextEnvelopeV1) (contract.ReviewerContextMaterialV1, []contract.ReviewerContextMaterialV1, error) {
	sources := make([]contract.ReviewerContextSourceRefV1, 0, len(envelope.Materials))
	var original contract.ReviewerContextMaterialV1
	var criteria []contract.ReviewerContextMaterialV1
	for _, material := range envelope.Materials {
		sources = append(sources, material.Source)
		switch material.Kind {
		case contract.ReviewerContextOriginalIntentV1:
			original = material
		case contract.ReviewerContextAcceptanceCriterionV1:
			criteria = append(criteria, material)
		}
	}
	sort.Slice(sources, func(i, j int) bool { return contextSourceKeyV2(sources[i]) < contextSourceKeyV2(sources[j]) })
	sort.Slice(criteria, func(i, j int) bool {
		return contextSourceKeyV2(criteria[i].Source) < contextSourceKeyV2(criteria[j].Source)
	})
	criterionSources := make([]contract.ReviewerContextSourceRefV1, len(criteria))
	for i := range criteria {
		criterionSources[i] = criteria[i].Source
	}
	if original.Source != bundle.OriginalIntent || !reflect.DeepEqual(criterionSources, bundle.AcceptanceCriteria) || !reflect.DeepEqual(sources, bundle.ReviewerContextSources) {
		return contract.ReviewerContextMaterialV1{}, nil, conflictV2("Bundle Context sources drifted from Owner envelope")
	}
	return original, criteria, nil
}
func digestBundleLocatorsV2(bundle contract.ReviewResultBundleV2) (core.Digest, error) {
	var values []runtimeports.ReviewArtifactLocatorV2
	for _, artifact := range bundle.Artifacts {
		values = append(values, artifact.Anchors...)
	}
	sort.Slice(values, func(i, j int) bool {
		return string(values[i].Kind)+values[i].Schema.Key()+string(values[i].LocatorDigest) < string(values[j].Kind)+values[j].Schema.Key()+string(values[j].LocatorDigest)
	})
	return core.CanonicalJSONDigest("praxis.review.artifact-locator-set/v2", "2.0.0", "ReviewArtifactLocatorSetV2", values)
}
func exact(v contract.FactIdentityV1) contract.ExactResourceRefV1 {
	return contract.ExactResourceRefV1{ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}
func toStoreRef(v contract.ExactResourceRefV1) reviewport.ExactFactRefV1 {
	return reviewport.ExactV1(v.ID, v.Revision, v.Digest)
}
func evidenceKeyV2(v runtimeports.ReviewEvidenceRefV2) string {
	return v.Ref + "\x00" + string(v.Classification) + "\x00" + string(v.Digest)
}
func contextSourceKeyV2(v contract.ReviewerContextSourceRefV1) string {
	return string(v.Owner) + "\x00" + v.ID + "\x00" + string(v.Digest)
}
func minTTL(first int64, values ...int64) int64 {
	out := first
	for _, value := range values {
		if value < out {
			out = value
		}
	}
	return out
}
func invalidV2(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}
func invalidCanonicalV2(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, message)
}
func conflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}
func staleV2(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, message)
}
