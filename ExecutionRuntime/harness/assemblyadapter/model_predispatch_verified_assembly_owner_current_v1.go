package assemblyadapter

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ModelPreDispatchVerifiedAssemblyOwnerCurrentContractVersionV1 = "praxis.harness.model-predispatch-verified-assembly-owner-current/v1"

	modelPreDispatchVerifiedAssemblyOwnerCurrentIDDomainV1         = "praxis.harness.model-predispatch-verified-assembly-owner-current-id/v1"
	modelPreDispatchVerifiedAssemblyOwnerCurrentCompileDomainV1    = "praxis.harness.model-predispatch-verified-assembly-owner-current-compile/v1"
	modelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDomainV1 = "praxis.harness.model-predispatch-verified-assembly-owner-current-projection/v1"

	modelPreDispatchVerifiedAssemblyOwnerCurrentIDDiscriminatorV1         = "ModelPreDispatchVerifiedAssemblyOwnerCurrentIdentityV1"
	modelPreDispatchVerifiedAssemblyOwnerCurrentCompileDiscriminatorV1    = "ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileV1"
	modelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDiscriminatorV1 = "ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1"
	modelPreDispatchVerifiedAssemblyOwnerCurrentIDPrefixV1                = "mpva-owner-current:v1:"
)

type ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1) Validate() error {
	if !strings.HasPrefix(r.ID, modelPreDispatchVerifiedAssemblyOwnerCurrentIDPrefixV1) || r.Revision == 0 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "verified Assembly Owner-current Ref is incomplete")
	}
	return nil
}

type ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1 struct {
	ContractVersion  string                                            `json:"contract_version"`
	Ref              ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1 `json:"ref"`
	Compile          assemblycontract.CompileResultV1                  `json:"compile"`
	CompileDigest    core.Digest                                       `json:"compile_digest"`
	Conformance      assemblycontract.AssemblyBindingConformanceV1     `json:"conformance"`
	CheckedUnixNano  int64                                             `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                             `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                       `json:"projection_digest"`
}

type ModelPreDispatchVerifiedAssemblyOwnerCurrentIdentityCanonicalV1 struct {
	ContractVersion string `json:"contract_version"`
	GenerationID    string `json:"generation_id"`
	HandoffID       string `json:"handoff_id"`
}

type ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileCanonicalV1 struct {
	Compile assemblycontract.CompileResultV1 `json:"compile"`
}

type modelPreDispatchVerifiedAssemblyCompileCanonicalBodyV1 struct {
	Generation  *assemblycontract.AssemblyGenerationV1   `json:"generation"`
	Manifest    *assemblycontract.AssemblyManifestV1     `json:"manifest"`
	Graph       *assemblycontract.CompiledHarnessGraphV1 `json:"graph"`
	Handoff     *assemblycontract.AssemblyHandoffV1      `json:"handoff"`
	Diagnostics []assemblycontract.AssemblyDiagnosticV1  `json:"diagnostics"`
	Residuals   []assemblycontract.ResidualReportV1      `json:"residuals"`
}

func newModelPreDispatchVerifiedAssemblyCompileCanonicalBodyV1(compile assemblycontract.CompileResultV1) modelPreDispatchVerifiedAssemblyCompileCanonicalBodyV1 {
	return modelPreDispatchVerifiedAssemblyCompileCanonicalBodyV1{
		Generation: compile.Generation, Manifest: compile.Manifest, Graph: compile.Graph, Handoff: compile.Handoff,
		Diagnostics: compile.Diagnostics, Residuals: compile.Residuals,
	}
}

func (c ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileCanonicalV1) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Compile modelPreDispatchVerifiedAssemblyCompileCanonicalBodyV1 `json:"compile"`
	}{Compile: newModelPreDispatchVerifiedAssemblyCompileCanonicalBodyV1(c.Compile)})
}

type ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionCanonicalV1 struct {
	ContractVersion string                                            `json:"contract_version"`
	Ref             ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1 `json:"ref"`
	Compile         assemblycontract.CompileResultV1                  `json:"compile"`
	CompileDigest   core.Digest                                       `json:"compile_digest"`
	Conformance     assemblycontract.AssemblyBindingConformanceV1     `json:"conformance"`
	CheckedUnixNano int64                                             `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                             `json:"expires_unix_nano"`
}

func (p ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionCanonicalV1) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ContractVersion string                                                 `json:"contract_version"`
		Ref             ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1      `json:"ref"`
		Compile         modelPreDispatchVerifiedAssemblyCompileCanonicalBodyV1 `json:"compile"`
		CompileDigest   core.Digest                                            `json:"compile_digest"`
		Conformance     assemblycontract.AssemblyBindingConformanceV1          `json:"conformance"`
		CheckedUnixNano int64                                                  `json:"checked_unix_nano"`
		ExpiresUnixNano int64                                                  `json:"expires_unix_nano"`
	}{p.ContractVersion, p.Ref, newModelPreDispatchVerifiedAssemblyCompileCanonicalBodyV1(p.Compile), p.CompileDigest, p.Conformance, p.CheckedUnixNano, p.ExpiresUnixNano})
}

func DeriveModelPreDispatchVerifiedAssemblyOwnerCurrentIDV1(generationID, handoffID string) (string, error) {
	if generationID == "" || handoffID == "" {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "verified Assembly identity coordinates are required")
	}
	digest, err := core.CanonicalJSONDigest(
		modelPreDispatchVerifiedAssemblyOwnerCurrentIDDomainV1,
		ModelPreDispatchVerifiedAssemblyOwnerCurrentContractVersionV1,
		modelPreDispatchVerifiedAssemblyOwnerCurrentIDDiscriminatorV1,
		ModelPreDispatchVerifiedAssemblyOwnerCurrentIdentityCanonicalV1{
			ContractVersion: ModelPreDispatchVerifiedAssemblyOwnerCurrentContractVersionV1,
			GenerationID:    generationID,
			HandoffID:       handoffID,
		},
	)
	if err != nil {
		return "", err
	}
	return modelPreDispatchVerifiedAssemblyOwnerCurrentIDPrefixV1 + string(digest), nil
}

func ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileDigestV1(compile assemblycontract.CompileResultV1) (core.Digest, error) {
	clone, err := cloneModelPreDispatchVerifiedAssemblyV1(compile)
	if err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(
		modelPreDispatchVerifiedAssemblyOwnerCurrentCompileDomainV1,
		ModelPreDispatchVerifiedAssemblyOwnerCurrentContractVersionV1,
		modelPreDispatchVerifiedAssemblyOwnerCurrentCompileDiscriminatorV1,
		ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileCanonicalV1{Compile: clone},
	)
}

func ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDigestV1(projection ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1) (core.Digest, error) {
	clone, err := cloneModelPreDispatchVerifiedAssemblyV1(projection)
	if err != nil {
		return "", err
	}
	clone.Ref.Digest = ""
	clone.ProjectionDigest = ""
	return core.CanonicalJSONDigest(
		modelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDomainV1,
		ModelPreDispatchVerifiedAssemblyOwnerCurrentContractVersionV1,
		modelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDiscriminatorV1,
		ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionCanonicalV1{
			ContractVersion: clone.ContractVersion,
			Ref:             clone.Ref,
			Compile:         clone.Compile,
			CompileDigest:   clone.CompileDigest,
			Conformance:     clone.Conformance,
			CheckedUnixNano: clone.CheckedUnixNano,
			ExpiresUnixNano: clone.ExpiresUnixNano,
		},
	)
}

func SealModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1(projection ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, now time.Time) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	if now.IsZero() {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "verified Assembly seal requires time")
	}
	clone, err := cloneModelPreDispatchVerifiedAssemblyV1(projection)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if clone.ContractVersion != "" && clone.ContractVersion != ModelPreDispatchVerifiedAssemblyOwnerCurrentContractVersionV1 {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "verified Assembly contract version drifted")
	}
	if clone.Compile.Generation == nil || clone.Compile.Handoff == nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "verified Assembly compile lineage is incomplete")
	}
	clone.ContractVersion = ModelPreDispatchVerifiedAssemblyOwnerCurrentContractVersionV1
	expectedID, err := DeriveModelPreDispatchVerifiedAssemblyOwnerCurrentIDV1(clone.Compile.Generation.GenerationID, clone.Compile.Handoff.GenerationRef.ID+"/handoff")
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if clone.Ref.ID != "" && clone.Ref.ID != expectedID {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly stable ID drifted")
	}
	if clone.Ref.Revision == 0 {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "verified Assembly revision is required")
	}
	clone.Ref.ID = expectedID
	compileDigest, err := ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileDigestV1(clone.Compile)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if clone.CompileDigest != "" && clone.CompileDigest != compileDigest {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "verified Assembly compile digest drifted")
	}
	clone.CompileDigest = compileDigest
	if clone.CheckedUnixNano != 0 && clone.CheckedUnixNano != clone.Conformance.ObservedUnixNano {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly checked time drifted from conformance")
	}
	if clone.ExpiresUnixNano != 0 && clone.ExpiresUnixNano != clone.Conformance.ExpiresUnixNano {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly expiry drifted from conformance")
	}
	clone.CheckedUnixNano = clone.Conformance.ObservedUnixNano
	clone.ExpiresUnixNano = clone.Conformance.ExpiresUnixNano
	clone.Ref.Digest = ""
	clone.ProjectionDigest = ""
	projectionDigest, err := ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDigestV1(clone)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if projection.Ref.Digest != "" && projection.Ref.Digest != projectionDigest {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "verified Assembly Ref digest drifted")
	}
	if projection.ProjectionDigest != "" && projection.ProjectionDigest != projectionDigest {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "verified Assembly projection digest drifted")
	}
	clone.Ref.Digest = projectionDigest
	clone.ProjectionDigest = projectionDigest
	if err := clone.ValidateCurrent(clone.Ref, now); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	return clone, nil
}

func (p ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1) Validate() error {
	return validateModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1(p, time.Unix(0, p.CheckedUnixNano))
}

func (p ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1) ValidateCurrent(expected ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1, now time.Time) error {
	if now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "verified Assembly current validation requires time")
	}
	if err := validateModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1(p, now); err != nil {
		return err
	}
	if p.Ref != expected {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly current Ref drifted")
	}
	if now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "verified Assembly current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "verified Assembly current projection expired")
	}
	return nil
}

func validateModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1(p ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, now time.Time) error {
	if p.ContractVersion != ModelPreDispatchVerifiedAssemblyOwnerCurrentContractVersionV1 || p.Ref.Validate() != nil || p.CompileDigest.Validate() != nil || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "verified Assembly Owner-current projection is incomplete")
	}
	if p.Compile.Generation == nil || p.Compile.Manifest == nil || p.Compile.Graph == nil || p.Compile.Handoff == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "verified Assembly compile requires Generation, Manifest, Graph and Handoff")
	}
	if err := validateArtifactChain(AssociationRequestV1{Generation: *p.Compile.Generation, Manifest: *p.Compile.Manifest, Graph: *p.Compile.Graph, Handoff: *p.Compile.Handoff}); err != nil {
		return err
	}
	diagnosticsDigest, err := assemblycontract.DiagnosticsDigestV1(p.Compile.Diagnostics)
	if err != nil || diagnosticsDigest != p.Compile.Generation.DiagnosticDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly diagnostics drifted from Generation")
	}
	residualDigest, err := assemblycontract.ResidualsDigestV1(p.Compile.Residuals)
	if err != nil || residualDigest != p.Compile.Generation.ResidualReportDigest || !reflect.DeepEqual(p.Compile.Residuals, p.Compile.Manifest.Residuals) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly residuals drifted from Generation or Manifest")
	}
	if err := p.Conformance.Validate(now.UnixNano()); err != nil {
		return err
	}
	generationRef := assemblycontract.ObjectRefV1{ID: p.Compile.Generation.GenerationID, Revision: p.Compile.Generation.Revision, Digest: p.Compile.Generation.Digest}
	handoffRef := assemblycontract.ObjectRefV1{ID: p.Compile.Handoff.GenerationRef.ID + "/handoff", Revision: p.Compile.Handoff.GenerationRef.Revision, Digest: p.Compile.Handoff.Digest}
	if p.Conformance.GenerationRef != generationRef || p.Conformance.HandoffRef != handoffRef ||
		p.Conformance.InputDigest != p.Compile.Generation.InputDigest || p.Conformance.ManifestDigest != p.Compile.Manifest.Digest ||
		p.Conformance.GraphDigest != p.Compile.Graph.Digest || p.Conformance.CatalogDigest != p.Compile.Manifest.CatalogDigest ||
		p.Compile.Manifest.CatalogDigest != p.Compile.Graph.CatalogDigest || p.Compile.Manifest.CatalogDigest != p.Compile.Handoff.CatalogDigest ||
		p.CheckedUnixNano != p.Conformance.ObservedUnixNano || p.ExpiresUnixNano != p.Conformance.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly conformance lineage drifted")
	}
	if p.Conformance.Association == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "verified Assembly requires Runtime association-path conformance")
	}
	expectedID, err := DeriveModelPreDispatchVerifiedAssemblyOwnerCurrentIDV1(p.Compile.Generation.GenerationID, handoffRef.ID)
	if err != nil || p.Ref.ID != expectedID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly stable ID does not bind exact Generation/Handoff")
	}
	compileDigest, err := ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileDigestV1(p.Compile)
	if err != nil || compileDigest != p.CompileDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "verified Assembly compile digest drifted")
	}
	projectionDigest, err := ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDigestV1(p)
	if err != nil || projectionDigest != p.ProjectionDigest || p.Ref.Digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "verified Assembly projection digest drifted")
	}
	return nil
}

type ModelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1 interface {
	InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(context.Context, ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error)
	modelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1()
}

type ModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1 interface {
	ModelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1
	EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Context, ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error)
	CompareAndSwapModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Context, ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1, ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error)
	InspectHistoricalModelPreDispatchVerifiedAssemblyOwnerV1(context.Context, ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error)
}

type InMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1 struct {
	mu      sync.RWMutex
	history map[string]map[core.Revision]ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1
	current map[string]ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1
	clock   func() time.Time
}

func NewInMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(clock func() time.Time) (*InMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1, error) {
	if clock == nil {
		return nil, componentMissingModelPreDispatchAssemblyV1("verified Assembly Owner-current Store clock is required")
	}
	return &InMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1{
		history: make(map[string]map[core.Revision]ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1),
		current: make(map[string]ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1),
		clock:   clock,
	}, nil
}

func (*InMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) modelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1() {
}

func (s *InMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx context.Context, next ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	return s.writeModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx, ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1{}, next, true)
}

func (s *InMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) CompareAndSwapModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx context.Context, expected ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1, next ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	return s.writeModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx, expected, next, false)
}

func (s *InMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) writeModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx context.Context, expected ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1, next ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, ensure bool) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	if s == nil || s.clock == nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("verified Assembly Owner-current Store is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if !ensure {
		if err := expected.Validate(); err != nil {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
		}
	}
	next, err := cloneModelPreDispatchVerifiedAssemblyV1(next)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	s.mu.Lock()
	now := s.clock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		s.mu.Unlock()
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if now.IsZero() {
		s.mu.Unlock()
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "verified Assembly Owner-current Store clock is unavailable")
	}
	if err := next.ValidateCurrent(next.Ref, now); err != nil {
		s.mu.Unlock()
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		s.mu.Unlock()
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if revisions := s.history[next.Ref.ID]; revisions != nil {
		if stored, exists := revisions[next.Ref.Revision]; exists {
			if stored.ProjectionDigest != next.ProjectionDigest || !reflect.DeepEqual(stored, next) {
				s.mu.Unlock()
				return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "verified Assembly revision already stores different content")
			}
			if s.current[next.Ref.ID] != next.Ref {
				s.mu.Unlock()
				return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "verified Assembly historical revision is no longer current")
			}
			s.mu.Unlock()
			return cloneModelPreDispatchVerifiedAssemblyV1(stored)
		}
	}
	current, exists := s.current[next.Ref.ID]
	if ensure {
		if exists || next.Ref.Revision != 1 {
			s.mu.Unlock()
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "verified Assembly initial Ensure conflicted")
		}
	} else if !exists || current != expected || next.Ref.ID != expected.ID || next.Ref.Revision != expected.Revision+1 {
		s.mu.Unlock()
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "verified Assembly full Ref CAS conflicted")
	}
	if s.history[next.Ref.ID] == nil {
		s.history[next.Ref.ID] = make(map[core.Revision]ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1)
	}
	s.history[next.Ref.ID][next.Ref.Revision] = next
	s.current[next.Ref.ID] = next.Ref
	s.mu.Unlock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	return cloneModelPreDispatchVerifiedAssemblyV1(next)
}

func (s *InMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) InspectHistoricalModelPreDispatchVerifiedAssemblyOwnerV1(ctx context.Context, ref ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	if s == nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("verified Assembly historical Store is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	stored, exists := s.history[ref.ID][ref.Revision]
	s.mu.RUnlock()
	if !exists || stored.Ref != ref {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "verified Assembly historical revision is absent")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	return cloneModelPreDispatchVerifiedAssemblyV1(stored)
}

func (s *InMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(ctx context.Context, ref ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	if s == nil || s.clock == nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("verified Assembly current Reader is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	current := s.current[ref.ID]
	stored, exists := s.history[ref.ID][ref.Revision]
	s.mu.RUnlock()
	if current != ref || !exists || stored.Ref != ref {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "verified Assembly exact Ref is not current")
	}
	now := s.clock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if err := stored.ValidateCurrent(ref, now); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	confirmedCurrent := s.current[ref.ID]
	confirmed, confirmedExists := s.history[ref.ID][ref.Revision]
	s.mu.RUnlock()
	if confirmedCurrent != ref || !confirmedExists || !reflect.DeepEqual(confirmed, stored) {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly current changed during exact read")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	return cloneModelPreDispatchVerifiedAssemblyV1(stored)
}

func cloneModelPreDispatchVerifiedAssemblyV1[T any](value T) (T, error) {
	var clone T
	payload, err := json.Marshal(value)
	if err != nil {
		return clone, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "verified Assembly value cannot be cloned")
	}
	if err := core.DecodeStrictJSON(payload, &clone); err != nil {
		return clone, err
	}
	return clone, nil
}

var _ ModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1 = (*InMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1)(nil)
