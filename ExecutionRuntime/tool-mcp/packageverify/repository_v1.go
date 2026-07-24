package packageverify

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type RepositoryV1 struct {
	mu           sync.RWMutex
	clock        ClockV1
	observations map[string]toolcontract.ToolPackageVerificationObservationV1
	facts        map[string]toolcontract.ToolPackageVerificationFactV1
	factByObs    map[string]string
	currents     map[string]toolcontract.ToolPackageVerificationCurrentProjectionV1
}

func NewRepositoryV1(clock ClockV1) (*RepositoryV1, error) {
	if isNilV1(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "package verification Repository clock is nil")
	}
	return &RepositoryV1{
		clock: clock, observations: make(map[string]toolcontract.ToolPackageVerificationObservationV1),
		facts: make(map[string]toolcontract.ToolPackageVerificationFactV1), factByObs: make(map[string]string),
		currents: make(map[string]toolcontract.ToolPackageVerificationCurrentProjectionV1),
	}, nil
}

func (r *RepositoryV1) EnsureToolPackageVerificationObservationV1(ctx context.Context, request toolcontract.ToolPackageVerificationObservationEnsureRequestV1) (toolcontract.ToolPackageVerificationObservationV1, error) {
	if isNilV1(ctx) || r == nil || isNilV1(r.clock) || request.Validate() != nil {
		return toolcontract.ToolPackageVerificationObservationV1{}, packageInvalidV1("package observation Ensure request is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolPackageVerificationObservationV1{}, err
	}
	id, err := toolcontract.DeriveToolPackageVerificationObservationIDV1(request.Subject, request.TrustPolicyCurrent)
	if err != nil {
		return toolcontract.ToolPackageVerificationObservationV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if winner, ok := r.observations[id]; ok {
		if !reflect.DeepEqual(winner.Request, request) {
			return toolcontract.ToolPackageVerificationObservationV1{}, packageConflictV1("package observation stable ID binds different request")
		}
		return winner, nil
	}
	now := r.clock.Now()
	if now.IsZero() {
		return toolcontract.ToolPackageVerificationObservationV1{}, packageClockV1("package observation clock is invalid")
	}
	observation, err := toolcontract.SealToolPackageVerificationObservationV1(toolcontract.ToolPackageVerificationObservationV1{
		Request: request, ObservedUnixNano: now.UnixNano(),
	})
	if err != nil {
		return toolcontract.ToolPackageVerificationObservationV1{}, err
	}
	r.observations[id] = observation
	return observation, nil
}

func (r *RepositoryV1) InspectToolPackageVerificationObservationBySubjectV1(ctx context.Context, subject toolcontract.ToolPackageVerificationSubjectV1, policy runtimeports.SupplyChainTrustPolicyCurrentRefV1) (toolcontract.ToolPackageVerificationObservationV1, error) {
	id, err := toolcontract.DeriveToolPackageVerificationObservationIDV1(subject, policy)
	if err != nil {
		return toolcontract.ToolPackageVerificationObservationV1{}, err
	}
	return r.inspectObservationV1(ctx, id, nil)
}

func (r *RepositoryV1) InspectExactToolPackageVerificationObservationV1(ctx context.Context, exact toolcontract.ToolPackageVerificationObservationRefV1) (toolcontract.ToolPackageVerificationObservationV1, error) {
	if exact.Validate() != nil {
		return toolcontract.ToolPackageVerificationObservationV1{}, packageInvalidV1("package observation exact Ref is invalid")
	}
	return r.inspectObservationV1(ctx, exact.ID, &exact)
}

func (r *RepositoryV1) inspectObservationV1(ctx context.Context, id string, exact *toolcontract.ToolPackageVerificationObservationRefV1) (toolcontract.ToolPackageVerificationObservationV1, error) {
	if isNilV1(ctx) || r == nil {
		return toolcontract.ToolPackageVerificationObservationV1{}, packageInvalidV1("package observation Inspect context or Repository is nil")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolPackageVerificationObservationV1{}, err
	}
	r.mu.RLock()
	winner, ok := r.observations[id]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.ToolPackageVerificationObservationV1{}, packageNotFoundV1("package observation was not found")
	}
	if exact != nil && winner.Ref != *exact {
		return toolcontract.ToolPackageVerificationObservationV1{}, packageConflictV1("package observation exact Ref drifted")
	}
	return winner, winner.Validate()
}

func (r *RepositoryV1) EnsureToolPackageVerificationFactV1(ctx context.Context, request toolcontract.ToolPackageVerificationFactEnsureRequestV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	if isNilV1(ctx) || r == nil || isNilV1(r.clock) || request.Validate() != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, packageInvalidV1("package fact Ensure request is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	id, err := toolcontract.DeriveToolPackageVerificationFactIDV1(request.Observation, request.Subject.ArtifactBinding.Package)
	if err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	observation, ok := r.observations[request.Observation.ID]
	if !ok || observation.Ref != request.Observation || observation.Request.Subject != request.Subject {
		return toolcontract.ToolPackageVerificationFactV1{}, packageConflictV1("package fact Observation or Subject drifted")
	}
	if winner, ok := r.facts[id]; ok {
		if winner.Observation != request.Observation || winner.Package != request.Subject.ArtifactBinding.Package || winner.PackageRegistry != request.Subject.PackageRegistry || winner.TrustPolicy != request.Subject.TrustPolicy {
			return toolcontract.ToolPackageVerificationFactV1{}, packageConflictV1("package fact stable ID binds different content")
		}
		return winner, nil
	}
	now := r.clock.Now()
	if now.IsZero() || now.UnixNano() < observation.ObservedUnixNano {
		return toolcontract.ToolPackageVerificationFactV1{}, packageClockV1("package fact clock regressed")
	}
	fact, err := toolcontract.SealToolPackageVerificationFactV1(toolcontract.ToolPackageVerificationFactV1{
		Package: request.Subject.ArtifactBinding.Package, PackageRegistry: request.Subject.PackageRegistry,
		ArtifactBindingDigest: request.Subject.ArtifactBinding.BindingDigest, TrustPolicy: request.Subject.TrustPolicy,
		Observation: request.Observation, SignerIdentityDigest: observation.Request.SignerIdentityDigest,
		PredicateType: observation.Request.PredicateType, VerifierConformance: observation.Request.VerifierConformance,
		VerifiedUnixNano: now.UnixNano(),
	})
	if err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	r.facts[id] = fact
	r.factByObs[request.Observation.ID] = id
	return fact, nil
}

func (r *RepositoryV1) InspectToolPackageVerificationFactByObservationV1(ctx context.Context, observation toolcontract.ToolPackageVerificationObservationRefV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	if observation.Validate() != nil || isNilV1(ctx) || r == nil {
		return toolcontract.ToolPackageVerificationFactV1{}, packageInvalidV1("package fact Observation Inspect request is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	r.mu.RLock()
	id, ok := r.factByObs[observation.ID]
	winner := r.facts[id]
	r.mu.RUnlock()
	if !ok || winner.Observation != observation {
		return toolcontract.ToolPackageVerificationFactV1{}, packageNotFoundV1("package fact was not found by Observation")
	}
	return winner, winner.Validate()
}

func (r *RepositoryV1) InspectExactToolPackageVerificationFactV1(ctx context.Context, exact toolcontract.ToolPackageVerificationFactRefV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	if exact.Validate() != nil || isNilV1(ctx) || r == nil {
		return toolcontract.ToolPackageVerificationFactV1{}, packageInvalidV1("package fact exact Inspect request is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	r.mu.RLock()
	winner, ok := r.facts[exact.ID]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.ToolPackageVerificationFactV1{}, packageNotFoundV1("package fact was not found")
	}
	if winner.Ref != exact {
		return toolcontract.ToolPackageVerificationFactV1{}, packageConflictV1("package fact exact Ref drifted")
	}
	return winner, winner.Validate()
}

func (r *RepositoryV1) ensureCurrentV1(ctx context.Context, projection toolcontract.ToolPackageVerificationCurrentProjectionV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	if isNilV1(ctx) || r == nil || projection.Validate() != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, packageInvalidV1("package verification current projection is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if winner, ok := r.currents[projection.Ref.ID]; ok {
		if winner.Ref != projection.Ref || !reflect.DeepEqual(winner.Issuance, projection.Issuance) {
			return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, packageConflictV1("package current stable ID binds different content")
		}
		return winner.Clone(), nil
	}
	r.currents[projection.Ref.ID] = projection.Clone()
	return projection.Clone(), nil
}

func (r *RepositoryV1) InspectToolPackageVerificationCurrentByIssuanceV1(ctx context.Context, issuance toolcontract.ToolPackageVerificationCurrentIssuanceV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	id, err := toolcontract.DeriveToolPackageVerificationCurrentIDV1(issuance)
	if err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	return r.inspectCurrentV1(ctx, id, nil)
}

func (r *RepositoryV1) InspectCurrentToolPackageVerificationV1(ctx context.Context, exact toolcontract.ToolPackageVerificationCurrentRefV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	if exact.Validate() != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, packageInvalidV1("package current exact Ref is invalid")
	}
	return r.inspectCurrentV1(ctx, exact.ID, &exact)
}

func (r *RepositoryV1) inspectCurrentV1(ctx context.Context, id string, exact *toolcontract.ToolPackageVerificationCurrentRefV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	if isNilV1(ctx) || r == nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, packageInvalidV1("package current Inspect context or Repository is nil")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	r.mu.RLock()
	winner, ok := r.currents[id]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, packageNotFoundV1("package current was not found")
	}
	if exact != nil && winner.Ref != *exact {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, packageConflictV1("package current exact Ref drifted")
	}
	return winner.Clone(), winner.Validate()
}

func packageInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func packageConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}

func packageNotFoundV1(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, message)
}

func packageClockV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, message)
}

var _ toolcontract.ToolPackageVerificationRepositoryV1 = (*RepositoryV1)(nil)
