package assemblyintegration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblyadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type fixtureV1 struct {
	now     time.Time
	request assemblyadapter.AssociationRequestV1
}

func newFixtureV1(t *testing.T) fixtureV1 {
	return newFixtureWithInputV1(t, nil)
}

func newResidualFixtureV1(t *testing.T) fixtureV1 {
	return newFixtureWithInputV1(t, func(input *assemblycontract.AssemblyInputV1) {
		input.ComponentManifests[0].ResidualClass = runtimeports.ResidualInspectable
		input.Modules[0].ResidualClass = runtimeports.ResidualInspectable
		input.Policy.AllowResidualClasses = []string{string(runtimeports.ResidualInspectable)}
	})
}

func newFixtureWithInputV1(t *testing.T, mutate func(*assemblycontract.AssemblyInputV1)) fixtureV1 {
	t.Helper()
	now := assemblytestkit.Now
	input := assemblytestkit.ValidInput()
	if mutate != nil {
		mutate(&input)
	}
	payload := []byte(`{"assembly_generation":"v1"}`)
	extensionSchema := assemblytestkit.Schema("assembly-generation-extension")
	input.ComponentManifests[0].Extensions = []runtimeports.GovernanceExtensionV2{{
		Key: "praxis.harness/assembly-generation", Required: true,
		Payload: runtimeports.OpaquePayloadV2{
			Schema: extensionSchema, ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload,
			LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.runtime/default-limit", Digest: assemblytestkit.Digest("extension-limit")},
		},
	}}
	manifestDigest, err := input.ComponentManifests[0].BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	input.Modules[0].ComponentManifestRef.Digest = manifestDigest
	input, err = assemblycontract.SealAssemblyInputV1(input)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := assemblycompiler.New().Compile(input)
	if err != nil {
		t.Fatal(err)
	}

	componentRefs := componentRefsV1(t, *compiled.Manifest)
	binding, err := runtimeports.SealGenerationBindingSetCurrentProjectionV1(runtimeports.GenerationBindingSetCurrentProjectionV1{
		BindingSetID: "binding-set/assembly-generation", BindingSetRevision: 7,
		BindingSetDigest: assemblytestkit.Digest("binding-set"), BindingSetSemanticDigest: assemblytestkit.Digest("binding-set-semantic"),
		PlanDigest: compiled.Manifest.Plan.ResolvedAgentPlan.Digest, GovernanceDigest: assemblytestkit.Digest("binding-governance"),
		ComponentManifestSetDigest: runtimeports.GenerationComponentManifestSetDigestV1(componentRefs),
		CurrentnessDigest:          assemblytestkit.Digest("binding-currentness"), IssuedUnixNano: now.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano: now.Add(4 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation := activationSubjectV1(t, compiled.Manifest.Plan.ResolvedAgentPlan.Digest, "activation-attempt-1")
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	activation, err := runtimeports.SealGenerationActivationCurrentProjectionV1(runtimeports.GenerationActivationCurrentProjectionV1{
		Operation: operation, OperationDigest: operationDigest, Active: true, Watermark: 11,
		CurrentnessDigest: assemblytestkit.Digest("activation-currentness"), ExpiresUnixNano: now.Add(3 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixtureV1{now: now, request: assemblyadapter.AssociationRequestV1{
		ContractVersion: assemblyadapter.ContractVersionV1, AssociationID: "association/assembly-generation-1",
		Handoff: *compiled.Handoff, Generation: *compiled.Generation, Manifest: *compiled.Manifest, Graph: *compiled.Graph,
		GenerationCurrentness: assemblyadapter.GenerationCurrentnessV1{Current: true, Watermark: 5, ExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()},
		ExpectedBindingSet:    assemblyadapter.BindingSetExpectationV1{ID: binding.BindingSetID, Revision: binding.BindingSetRevision}, Binding: binding,
		ExpectedActivation: operation, Activation: activation, RequestedExpiresUnixNano: now.Add(90 * time.Second).UnixNano(),
	}}
}

func resealResidualChainV1(t *testing.T, request *assemblyadapter.AssociationRequestV1) {
	t.Helper()
	digest, err := assemblycontract.ResidualsDigestV1(request.Manifest.Residuals)
	if err != nil {
		t.Fatal(err)
	}
	request.Generation.ResidualReportDigest = digest
	resealChainV1(t, request)
}

func activationSubjectV1(t *testing.T, planDigest core.Digest, attempt string) runtimeports.OperationSubjectV3 {
	t.Helper()
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 2},
		Lineage:  core.LineageRef{ID: "lineage-1", PlanDigest: planDigest},
		Instance: core.InstanceRef{ID: "instance-1", Epoch: 3}, AuthorityEpoch: 4,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	return runtimeports.OperationSubjectV3{
		Kind: runtimeports.OperationScopeActivationV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest,
		ActivationAttemptID: attempt, SubjectRevision: 1, CurrentProjectionRef: "activation-current-1",
		CurrentProjectionDigest: assemblytestkit.Digest("activation-current-projection"), CurrentProjectionRevision: 1,
	}
}

func componentRefsV1(t *testing.T, manifest assemblycontract.AssemblyManifestV1) []runtimeports.GenerationComponentManifestRefV1 {
	t.Helper()
	refs := make([]runtimeports.GenerationComponentManifestRefV1, 0, len(manifest.ComponentManifests))
	for _, component := range manifest.ComponentManifests {
		digest, err := component.BindingDigestV2()
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, runtimeports.GenerationComponentManifestRefV1{ComponentID: component.ComponentID, ManifestDigest: digest, ArtifactDigest: component.ArtifactDigest})
	}
	return refs
}

func resealChainV1(t *testing.T, request *assemblyadapter.AssociationRequestV1) {
	t.Helper()
	var err error
	request.Manifest.Digest, err = assemblycontract.ManifestDigestV1(request.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	request.Graph.Digest, err = assemblycontract.GraphDigestV1(request.Graph)
	if err != nil {
		t.Fatal(err)
	}
	request.Generation.ManifestDigest = request.Manifest.Digest
	request.Generation.GraphDigest = request.Graph.Digest
	request.Generation.Digest, err = assemblycontract.GenerationDigestV1(request.Generation)
	if err != nil {
		t.Fatal(err)
	}
	request.Handoff.GenerationRef = assemblycontract.ObjectRefV1{ID: request.Generation.GenerationID, Revision: request.Generation.Revision, Digest: request.Generation.Digest}
	request.Handoff.ManifestDigest = request.Manifest.Digest
	request.Handoff.GraphDigest = request.Graph.Digest
	request.Handoff.CatalogDigest = request.Manifest.CatalogDigest
	request.Handoff.ProviderCandidates = append([]assemblycontract.ProviderBindingCandidateV1(nil), request.Manifest.ProviderBindingCandidates...)
	request.Handoff.Digest, err = assemblycontract.HandoffDigestV1(request.Handoff)
	if err != nil {
		t.Fatal(err)
	}
}

type associationPortV1 struct {
	mu               sync.Mutex
	now              *time.Time
	facts            map[string]runtimeports.GenerationBindingAssociationFactV1
	inspectCalls     int
	associateCalls   int
	lostReplyOnce    bool
	differentCurrent bool
	afterCreate      func()
}

func newAssociationPortV1(now *time.Time) *associationPortV1 {
	return &associationPortV1{now: now, facts: map[string]runtimeports.GenerationBindingAssociationFactV1{}}
}

func (p *associationPortV1) AssociateGenerationBindingV1(_ context.Context, candidate runtimeports.GenerationBindingAssociationCandidateV1) (runtimeports.GenerationBindingAssociationFactV1, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.associateCalls++
	if current, ok := p.facts[candidate.AssociationID]; ok {
		if current.CandidateDigest != candidate.Digest {
			return runtimeports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "same association ID has different content")
		}
		return current, nil
	}
	fact, err := runtimeports.SealGenerationBindingAssociationFactV1(runtimeports.GenerationBindingAssociationFactV1{
		ID: candidate.AssociationID, Revision: 1, State: runtimeports.GenerationBindingAssociationActiveV1,
		Candidate: candidate, CandidateDigest: candidate.Digest, CreatedUnixNano: p.now.UnixNano(), UpdatedUnixNano: p.now.UnixNano(),
		ExpiresUnixNano: minimumExpiryV1(candidate),
	})
	if err != nil {
		return runtimeports.GenerationBindingAssociationFactV1{}, err
	}
	p.facts[candidate.AssociationID] = fact
	if p.afterCreate != nil {
		p.afterCreate()
	}
	if p.lostReplyOnce {
		p.lostReplyOnce = false
		return runtimeports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "injected lost association reply")
	}
	return fact, nil
}

func (p *associationPortV1) InspectCurrentGenerationBindingAssociationV1(_ context.Context, id string) (runtimeports.GenerationBindingAssociationFactV1, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inspectCalls++
	fact, ok := p.facts[id]
	if !ok {
		return runtimeports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "association not found")
	}
	if p.differentCurrent {
		fact.Revision++
		fact.UpdatedUnixNano++
		var err error
		fact, err = runtimeports.SealGenerationBindingAssociationFactV1(fact)
		if err != nil {
			return runtimeports.GenerationBindingAssociationFactV1{}, err
		}
	}
	return fact, nil
}

func minimumExpiryV1(candidate runtimeports.GenerationBindingAssociationCandidateV1) int64 {
	minimum := candidate.RequestedExpiresUnixNano
	for _, expiry := range []int64{candidate.Generation.ExpiresUnixNano, candidate.Binding.ExpiresUnixNano, candidate.Activation.ExpiresUnixNano} {
		if expiry < minimum {
			minimum = expiry
		}
	}
	return minimum
}

var _ runtimeports.GenerationBindingAssociationGovernancePortV1 = (*associationPortV1)(nil)
