// Package assemblyadapter maps sealed Harness Assembly artifacts onto the
// Runtime-owned Generation-Binding association Port. It never creates or
// stores Runtime Facts itself.
package assemblyadapter

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ContractVersionV1 = "praxis.harness.assembly.adapter/v1"

type GenerationCurrentnessV1 struct {
	Current         bool          `json:"current"`
	Watermark       core.Revision `json:"watermark"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

type BindingSetExpectationV1 struct {
	ID       string        `json:"binding_set_id"`
	Revision core.Revision `json:"binding_set_revision"`
}

// AssociationRequestV1 combines immutable Harness artifacts with neutral
// Runtime current projections. Binding and Activation remain Runtime-owned
// observations; the request grants no authority by itself.
type AssociationRequestV1 struct {
	ContractVersion          string                                               `json:"contract_version"`
	AssociationID            string                                               `json:"association_id"`
	Handoff                  assemblycontract.AssemblyHandoffV1                   `json:"handoff"`
	Generation               assemblycontract.AssemblyGenerationV1                `json:"generation"`
	Manifest                 assemblycontract.AssemblyManifestV1                  `json:"manifest"`
	Graph                    assemblycontract.CompiledHarnessGraphV1              `json:"graph"`
	GenerationCurrentness    GenerationCurrentnessV1                              `json:"generation_currentness"`
	ExpectedBindingSet       BindingSetExpectationV1                              `json:"expected_binding_set"`
	Binding                  runtimeports.GenerationBindingSetCurrentProjectionV1 `json:"binding"`
	ExpectedActivation       runtimeports.OperationSubjectV3                      `json:"expected_activation"`
	Activation               runtimeports.GenerationActivationCurrentProjectionV1 `json:"activation"`
	RequestedExpiresUnixNano int64                                                `json:"requested_expires_unix_nano"`
}

type AssociationResultV1 struct {
	Candidate          runtimeports.GenerationBindingAssociationCandidateV1 `json:"candidate"`
	Fact               runtimeports.GenerationBindingAssociationFactV1      `json:"fact"`
	Conformance        assemblycontract.AssemblyBindingConformanceV1        `json:"conformance"`
	RecoveredByInspect bool                                                 `json:"recovered_by_inspect"`
}

type Adapter struct {
	associations runtimeports.GenerationBindingAssociationGovernancePortV1
	clock        func() time.Time
}

func New(associations runtimeports.GenerationBindingAssociationGovernancePortV1, clock func() time.Time) (*Adapter, error) {
	if associations == nil || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "versioned Generation-Binding association Port and clock are required")
	}
	return &Adapter{associations: associations, clock: clock}, nil
}

// Associate inspects before creating. This makes a previous successful create
// recoverable without replaying it. After an indeterminate create reply it only
// inspects the original association ID; it never creates a different attempt.
func (a *Adapter) Associate(ctx context.Context, request AssociationRequestV1) (AssociationResultV1, error) {
	if a == nil || a.associations == nil || a.clock == nil {
		return AssociationResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "assembly association Adapter is not initialized")
	}
	now := a.clock()
	candidate, err := BuildCandidateV1(request, now)
	if err != nil {
		return AssociationResultV1{}, err
	}

	existing, inspectErr := a.associations.InspectCurrentGenerationBindingAssociationV1(ctx, candidate.AssociationID)
	if inspectErr == nil {
		return a.finish(request, candidate, existing, true)
	}
	if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return AssociationResultV1{}, inspectErr
	}

	fact, createErr := a.associations.AssociateGenerationBindingV1(ctx, candidate)
	if createErr == nil {
		current, currentErr := a.associations.InspectCurrentGenerationBindingAssociationV1(ctx, candidate.AssociationID)
		if currentErr != nil {
			return AssociationResultV1{}, currentErr
		}
		if current.Digest != fact.Digest {
			return AssociationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "post-create current Inspect returned a different Runtime association Fact")
		}
		return a.finish(request, candidate, current, false)
	}
	if !recoverableCreateReply(createErr) {
		return AssociationResultV1{}, createErr
	}
	recovered, recoveryErr := a.associations.InspectCurrentGenerationBindingAssociationV1(context.WithoutCancel(ctx), candidate.AssociationID)
	if recoveryErr != nil {
		return AssociationResultV1{}, createErr
	}
	return a.finish(request, candidate, recovered, true)
}

func (a *Adapter) finish(request AssociationRequestV1, candidate runtimeports.GenerationBindingAssociationCandidateV1, fact runtimeports.GenerationBindingAssociationFactV1, recovered bool) (AssociationResultV1, error) {
	now := a.clock()
	if err := validateExactFact(candidate, fact, now); err != nil {
		return AssociationResultV1{}, err
	}
	conformance, err := BuildBindingConformanceV1(request.Handoff, fact, now)
	if err != nil {
		return AssociationResultV1{}, err
	}
	return AssociationResultV1{Candidate: candidate, Fact: fact, Conformance: conformance, RecoveredByInspect: recovered}, nil
}

func recoverableCreateReply(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorConflict)
}

// BuildCandidateV1 performs the complete fail-closed, pure mapping step.
func BuildCandidateV1(request AssociationRequestV1, now time.Time) (runtimeports.GenerationBindingAssociationCandidateV1, error) {
	if request.ContractVersion != ContractVersionV1 || request.AssociationID == "" || now.IsZero() {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "association request version, identity and current clock are required")
	}
	if err := validateArtifactChain(request); err != nil {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, err
	}
	components, extension, err := generationSummaries(request.Manifest, request.Handoff.RequiredExtension)
	if err != nil {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, err
	}

	generation, err := runtimeports.SealGenerationCurrentProjectionV1(runtimeports.GenerationCurrentProjectionV1{
		Generation: runtimeports.GenerationArtifactRefV1{
			ID: request.Generation.GenerationID, Revision: request.Generation.Revision, Digest: request.Generation.Digest,
			InputDigest: request.Generation.InputDigest, ManifestDigest: request.Generation.ManifestDigest,
			GraphDigest: request.Generation.GraphDigest, CatalogDigest: request.Handoff.CatalogDigest,
		},
		ComponentManifests: components,
		Extension:          extension,
		State:              runtimeports.GenerationCurrentSealedV1,
		Current:            request.GenerationCurrentness.Current,
		Watermark:          request.GenerationCurrentness.Watermark,
		ExpiresUnixNano:    request.GenerationCurrentness.ExpiresUnixNano,
	})
	if err != nil {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, err
	}
	if err := generation.ValidateCurrent(generation.Generation, now); err != nil {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, err
	}

	if request.ExpectedBindingSet.ID == "" || request.ExpectedBindingSet.Revision == 0 {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonBindingNotCertified, "expected BindingSet identity and revision are required")
	}
	if err := request.Binding.ValidateCurrent(request.ExpectedBindingSet.ID, request.ExpectedBindingSet.Revision, now); err != nil {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, err
	}
	if request.Binding.ComponentManifestSetDigest != runtimeports.GenerationComponentManifestSetDigestV1(components) {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "BindingSet component manifest summary drifted from the sealed Assembly Manifest")
	}

	if err := request.ExpectedActivation.Validate(); err != nil {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, err
	}
	if request.ExpectedActivation.ExecutionScope.Lineage.PlanDigest != request.Manifest.Plan.ResolvedAgentPlan.Digest {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceScopeConflict, "activation execution scope does not bind the resolved Agent Plan")
	}
	if err := request.Activation.ValidateCurrent(request.ExpectedActivation, now); err != nil {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, err
	}
	if request.RequestedExpiresUnixNano <= now.UnixNano() {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "association requested expiry is not current")
	}

	return runtimeports.SealGenerationBindingAssociationCandidateV1(runtimeports.GenerationBindingAssociationCandidateV1{
		AssociationID: request.AssociationID, Generation: generation, Binding: request.Binding,
		Activation: request.Activation, RequestedExpiresUnixNano: request.RequestedExpiresUnixNano,
	})
}

func validateArtifactChain(request AssociationRequestV1) error {
	if err := request.Handoff.Validate(); err != nil {
		return err
	}
	generationDigest, err := assemblycontract.GenerationDigestV1(request.Generation)
	if err != nil || generationDigest != request.Generation.Digest || request.Generation.ContractVersion != assemblycontract.ContractVersionV1 || request.Generation.CompilerVersion != assemblycontract.CompilerVersionV1 || request.Generation.State != assemblycontract.AssemblyStateSealedV1 || request.Generation.Revision == 0 || request.Generation.CreatedUnixNano <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "sealed Assembly Generation is missing or drifted")
	}
	manifestDigest, err := assemblycontract.ManifestDigestV1(request.Manifest)
	if err != nil || manifestDigest != request.Manifest.Digest || request.Manifest.ContractVersion != assemblycontract.ContractVersionV1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Assembly Manifest is missing or drifted")
	}
	graphDigest, err := assemblycontract.GraphDigestV1(request.Graph)
	if err != nil || graphDigest != request.Graph.Digest || request.Graph.ContractVersion != assemblycontract.ContractVersionV1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "compiled Harness Graph is missing or drifted")
	}
	catalogDigest, err := assemblycontract.CatalogDigestV1(request.Manifest.Slots, request.Manifest.HookFaces)
	if err != nil {
		return err
	}
	expectedCatalogDigest, err := assemblycontract.CatalogDigestV1(assemblycontract.SlotCatalogV1(), assemblycontract.HookFaceCatalogV1())
	if err != nil {
		return err
	}
	if catalogDigest != expectedCatalogDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "Assembly artifacts do not use the Harness-owned Slot/Phase catalog")
	}
	if request.Handoff.GenerationRef.ID != request.Generation.GenerationID || request.Handoff.GenerationRef.Revision != request.Generation.Revision || request.Handoff.GenerationRef.Digest != request.Generation.Digest ||
		request.Handoff.ManifestDigest != request.Manifest.Digest || request.Generation.ManifestDigest != request.Manifest.Digest ||
		request.Handoff.GraphDigest != request.Graph.Digest || request.Generation.GraphDigest != request.Graph.Digest ||
		request.Handoff.CatalogDigest != catalogDigest || request.Manifest.CatalogDigest != catalogDigest || request.Graph.CatalogDigest != catalogDigest ||
		request.Generation.InputDigest != request.Manifest.InputDigest || request.Generation.InputDigest != request.Graph.InputDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Generation/Input/Manifest/Graph/Catalog digest chain drifted")
	}
	if err := validateManifestGraphV1(request.Manifest, request.Graph); err != nil {
		return err
	}
	residualDigest, err := assemblycontract.ResidualsDigestV1(request.Manifest.Residuals)
	if err != nil {
		return err
	}
	if request.Generation.ResidualReportDigest != residualDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Assembly Generation residual digest drifted from the Manifest Residual set")
	}
	if !reflect.DeepEqual(request.Handoff.ProviderCandidates, request.Manifest.ProviderBindingCandidates) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "handoff provider candidate summary drifted from the Assembly Manifest")
	}
	return nil
}

func generationSummaries(manifest assemblycontract.AssemblyManifestV1, required runtimeports.NamespacedNameV2) ([]runtimeports.GenerationComponentManifestRefV1, runtimeports.GenerationGovernanceExtensionRefV1, error) {
	if len(manifest.ComponentManifests) == 0 || len(manifest.ComponentManifests) > runtimeports.MaxGenerationBindingComponentsV1 {
		return nil, runtimeports.GenerationGovernanceExtensionRefV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Assembly component manifest set is empty or too large")
	}
	components := make([]runtimeports.GenerationComponentManifestRefV1, 0, len(manifest.ComponentManifests))
	var extension runtimeports.GenerationGovernanceExtensionRefV1
	matchCount := 0
	for _, component := range manifest.ComponentManifests {
		if err := component.Validate(); err != nil {
			return nil, runtimeports.GenerationGovernanceExtensionRefV1{}, err
		}
		manifestDigest, err := component.BindingDigestV2()
		if err != nil {
			return nil, runtimeports.GenerationGovernanceExtensionRefV1{}, err
		}
		components = append(components, runtimeports.GenerationComponentManifestRefV1{ComponentID: component.ComponentID, ManifestDigest: manifestDigest, ArtifactDigest: component.ArtifactDigest})
		for _, candidate := range component.Extensions {
			if candidate.Key != required {
				continue
			}
			if !candidate.Required {
				return nil, runtimeports.GenerationGovernanceExtensionRefV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownRequiredExtension, "Assembly Generation governance extension is not required by its owner manifest")
			}
			matchCount++
			extension = runtimeports.GenerationGovernanceExtensionRefV1{Kind: candidate.Key, Contract: candidate.Payload.Schema, Digest: candidate.Payload.ContentDigest}
		}
	}
	sort.Slice(components, func(i, j int) bool { return components[i].ComponentID < components[j].ComponentID })
	for index := 1; index < len(components); index++ {
		if components[index-1].ComponentID == components[index].ComponentID {
			return nil, runtimeports.GenerationGovernanceExtensionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Assembly component manifest summary contains duplicate component IDs")
		}
	}
	if matchCount != 1 {
		return nil, runtimeports.GenerationGovernanceExtensionRefV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownRequiredExtension, "exactly one required Assembly Generation governance extension must be present")
	}
	if err := extension.Validate(); err != nil {
		return nil, runtimeports.GenerationGovernanceExtensionRefV1{}, err
	}
	return components, extension, nil
}

func validateExactFact(candidate runtimeports.GenerationBindingAssociationCandidateV1, fact runtimeports.GenerationBindingAssociationFactV1, now time.Time) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	if now.IsZero() || fact.State != runtimeports.GenerationBindingAssociationActiveV1 || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Runtime association Fact is not active and current")
	}
	if now.UnixNano() < fact.CreatedUnixNano || now.UnixNano() < fact.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "association conformance clock regressed behind the Runtime Fact")
	}
	if fact.ID != candidate.AssociationID || fact.CandidateDigest != candidate.Digest || fact.Candidate.Digest != candidate.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "association ID already binds different content")
	}
	if err := fact.Candidate.Generation.ValidateCurrent(candidate.Generation.Generation, now); err != nil || fact.Candidate.Generation.ProjectionDigest != candidate.Generation.ProjectionDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Runtime Fact generation projection is stale or drifted")
	}
	if err := fact.Candidate.Binding.ValidateCurrent(candidate.Binding.BindingSetID, candidate.Binding.BindingSetRevision, now); err != nil || fact.Candidate.Binding.ProjectionDigest != candidate.Binding.ProjectionDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Runtime Fact BindingSet projection is stale or drifted")
	}
	if err := fact.Candidate.Activation.ValidateCurrent(candidate.Activation.Operation, now); err != nil || fact.Candidate.Activation.ProjectionDigest != candidate.Activation.ProjectionDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonActivationFactDrift, "Runtime Fact activation projection is stale or drifted")
	}
	return nil
}

func BuildBindingConformanceV1(handoff assemblycontract.AssemblyHandoffV1, fact runtimeports.GenerationBindingAssociationFactV1, now time.Time) (assemblycontract.AssemblyBindingConformanceV1, error) {
	if err := handoff.Validate(); err != nil {
		return assemblycontract.AssemblyBindingConformanceV1{}, err
	}
	if err := validateExactFact(fact.Candidate, fact, now); err != nil {
		return assemblycontract.AssemblyBindingConformanceV1{}, err
	}
	generation := fact.Candidate.Generation.Generation
	if handoff.GenerationRef.ID != generation.ID || handoff.GenerationRef.Revision != generation.Revision || handoff.GenerationRef.Digest != generation.Digest || handoff.ManifestDigest != generation.ManifestDigest || handoff.GraphDigest != generation.GraphDigest || handoff.CatalogDigest != generation.CatalogDigest || handoff.RequiredExtension != fact.Candidate.Generation.Extension.Kind {
		return assemblycontract.AssemblyBindingConformanceV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Runtime association Fact does not conform to the exact Assembly handoff")
	}
	operationDigest, err := fact.Candidate.Activation.Operation.DigestV3()
	if err != nil {
		return assemblycontract.AssemblyBindingConformanceV1{}, err
	}
	association := fact.RefV1()
	extension := fact.Candidate.Generation.Extension
	return assemblycontract.SealBindingConformanceV1(assemblycontract.AssemblyBindingConformanceV1{
		HandoffRef:    assemblycontract.ObjectRefV1{ID: handoff.GenerationRef.ID + "/handoff", Revision: handoff.GenerationRef.Revision, Digest: handoff.Digest},
		GenerationRef: handoff.GenerationRef, Association: &association,
		InputDigest: fact.Candidate.Generation.Generation.InputDigest, ManifestDigest: handoff.ManifestDigest,
		GraphDigest: handoff.GraphDigest, CatalogDigest: handoff.CatalogDigest,
		ComponentManifestSetDigest: runtimeports.GenerationComponentManifestSetDigestV1(fact.Candidate.Generation.ComponentManifests),
		GovernanceExtension:        &extension, GenerationProjectionDigest: fact.Candidate.Generation.ProjectionDigest,
		BindingSetID: fact.Candidate.Binding.BindingSetID, BindingSetRevision: fact.Candidate.Binding.BindingSetRevision,
		BindingSetDigest: fact.Candidate.Binding.BindingSetDigest, BindingSetSemanticDigest: fact.Candidate.Binding.BindingSetSemanticDigest,
		BindingSetCurrentnessDigest: fact.Candidate.Binding.CurrentnessDigest, BindingSetProjectionDigest: fact.Candidate.Binding.ProjectionDigest,
		ActivationOperationDigest: operationDigest, ActivationCurrentnessDigest: fact.Candidate.Activation.CurrentnessDigest,
		ActivationProjectionDigest: fact.Candidate.Activation.ProjectionDigest,
		SchemaDigests:              []core.Digest{}, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: fact.ExpiresUnixNano,
		Current: true, Diagnostics: []assemblycontract.AssemblyDiagnosticV1{},
	}, now.UnixNano())
}
