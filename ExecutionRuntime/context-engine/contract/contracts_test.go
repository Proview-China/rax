package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestCoreContractsValidateAndBindDigests(t *testing.T) {
	content := contract.ContentRef{Ref: "content-1", Digest: testkit.D("content"), Length: 7}
	candidate := testkit.Candidate("candidate-1", contract.FragmentInstruction, content, 10)
	candidateDigest, err := candidate.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	candidateRef := contract.FactRef{ID: candidate.ID, Revision: candidate.Revision, Digest: candidateDigest}
	recipe := testkit.Recipe()
	recipeDigest, err := recipe.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	sourceSetDigest, err := contract.DigestJSON([]contract.FactRef{candidateRef})
	if err != nil {
		t.Fatal(err)
	}
	manifest := contract.ContextManifest{
		ContractVersion: contract.Version, ID: "manifest-1", Revision: 1, Execution: testkit.Execution(),
		RecipeRef: contract.FactRef{ID: recipe.ID, Revision: recipe.Revision, Digest: recipeDigest}, GenerationID: "generation-1",
		Decisions:    []contract.AdmissionDecision{{CandidateRef: candidateRef, Disposition: contract.AdmissionAdmitted, Reason: "policy_admitted", Region: contract.RegionStablePrefix, Tokens: 10}},
		Fragments:    []contract.ContextFragment{{CandidateRef: candidateRef, Kind: candidate.Kind, Region: contract.RegionStablePrefix, Position: 1, Content: content, Tokens: 10}},
		StableTokens: 10, TotalTokens: 10, SourceSetDigest: sourceSetDigest, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 100,
	}
	manifestDigest, err := manifest.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	frame := contract.ContextFrame{
		ContractVersion: contract.Version, ID: "frame-1", Revision: 1, Execution: testkit.Execution(), ManifestRef: contract.FactRef{ID: manifest.ID, Revision: 1, Digest: manifestDigest},
		GenerationID: "generation-1", Generation: 1, StablePrefix: content, DynamicTail: content, Rendered: content, SourceSetDigest: manifest.SourceSetDigest, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 100,
	}
	frameDigest, err := frame.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	generation := contract.ContextGeneration{
		ContractVersion: contract.Version, ID: "generation-1", Revision: 1, Ordinal: 1, RootFrame: contract.FactRef{ID: frame.ID, Revision: 1, Digest: frameDigest},
		RetainedAnchors: []contract.FactRef{{ID: "anchor-1", Revision: 1, Digest: testkit.D("anchor")}}, CreatedUnixNano: testkit.Now,
	}
	if err := generation.Validate(); err != nil {
		t.Fatal(err)
	}

	badManifest := manifest
	badManifest.TotalTokens++
	if err := badManifest.Validate(); err == nil {
		t.Fatal("manifest accepted inconsistent token totals")
	}
	badManifest = manifest
	badManifest.Fragments = append([]contract.ContextFragment(nil), manifest.Fragments...)
	badManifest.Fragments[0].Tokens++
	badManifest.StableTokens++
	badManifest.TotalTokens++
	if err := badManifest.Validate(); err == nil {
		t.Fatal("manifest accepted fragment not exactly bound to admission")
	}
	badCandidate := candidate
	badCandidate.ContractVersion = "future"
	if err := badCandidate.Validate(); err == nil {
		t.Fatal("candidate accepted unsupported contract")
	}
}

func TestKnowledgeReferenceIsIndependentContextFragmentKind(t *testing.T) {
	content := contract.ContentRef{Ref: "knowledge-content-1", Digest: testkit.D("knowledge-content"), Length: 17}
	knowledge := testkit.Candidate("knowledge-candidate-1", contract.FragmentKnowledgeReference, content, 8)
	if err := knowledge.Validate(); err != nil {
		t.Fatalf("knowledge_reference candidate rejected: %v", err)
	}
	if knowledge.Kind == contract.FragmentMemoryRecall || knowledge.Kind == contract.FragmentArtifactReference {
		t.Fatal("knowledge_reference aliased another owner semantic")
	}
	tampered := knowledge
	tampered.Kind = contract.FragmentKind("knowledge")
	if err := tampered.Validate(); err == nil {
		t.Fatal("unknown knowledge alias was accepted")
	}
}

func TestCacheAndInjectionContracts(t *testing.T) {
	profile := contract.ProviderCacheProfile{ContractVersion: contract.Version, ID: "profile-1", Revision: 1, Provider: "provider", RouteID: "route-1", Model: "model-1", CapabilityDigest: testkit.D("capability"), ExpiresUnixNano: testkit.Now + 100}
	if err := profile.Validate(testkit.Now); err != nil {
		t.Fatal(err)
	}
	partition := contract.CachePartition{
		AuditScopeDigest: testkit.D("audit"), ReuseScope: contract.ReuseRun, IsolationDigest: testkit.D("isolation"), AuthorityDigest: testkit.D("authority"), Sensitivity: contract.SensitivityInternal,
		SourceSetDigest: testkit.D("sources"), RecipeDigest: testkit.D("recipe"), RenderDigest: testkit.D("render"), ModelProfileDigest: testkit.D("model"), HarnessDigest: testkit.D("harness"), ToolSchemaDigest: testkit.D("tools"), PrefixDigest: testkit.D("prefix"),
		ProviderProfileRef: contract.FactRef{ID: profile.ID, Revision: profile.Revision, Digest: testkit.D("profile")}, KeyVersion: "v1",
	}
	if _, err := partition.DigestValue(); err != nil {
		t.Fatal(err)
	}
	entry := contract.CacheEntry{ContractVersion: contract.Version, ID: "entry-1", Revision: 1, PartitionDigest: testkit.D("partition"), KeyDigest: testkit.D("key"), PrefixDigest: testkit.D("prefix"), AuthorityDigest: testkit.D("authority"), State: contract.CacheEntryCurrent, InvalidationGeneration: 1, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 100}
	if err := entry.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (contract.ProviderCacheUsageObservation{ObservationID: "usage-1", ObservedUnixNano: testkit.Now}).Validate(); err != nil {
		t.Fatal(err)
	}

	frameRef := contract.FactRef{ID: "frame-1", Revision: 1, Digest: testkit.D("frame")}
	field := contract.InjectionField{Path: "messages.system", Digest: testkit.D("system"), Required: true}
	expected := contract.ExpectedInjectionManifest{ContractVersion: contract.Version, ID: "expected-1", Revision: 1, Execution: testkit.Execution(), FrameRef: frameRef, Fields: []contract.InjectionField{field}, CapabilityRef: contract.FactRef{ID: "capability-1", Revision: 1, Digest: testkit.D("capability")}, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 100}
	if err := expected.Validate(); err != nil {
		t.Fatal(err)
	}
	observation := contract.ProviderActualInjectionObservation{ContractVersion: contract.Version, ID: "observation-1", Revision: 1, Execution: testkit.Execution(), FrameRef: frameRef, RouteID: "route-1", AttemptID: "attempt-1", SourceSequence: 1, Fields: []contract.InjectionField{field}, ObservedUnixNano: testkit.Now + 1}
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	observationRef, err := observation.Ref(contract.ObservationFidelityComplete)
	if err != nil {
		t.Fatal(err)
	}
	actual := contract.HarnessActualInjectionManifest{ContractVersion: contract.Version, ID: "actual-1", Revision: 1, Execution: testkit.Execution(), FrameRef: frameRef, RouteID: "route-1", AttemptID: "attempt-1", Fields: []contract.InjectionField{field}, ObservationRefs: []contract.ActualInjectionObservationRef{observationRef}, CreatedUnixNano: testkit.Now + 2}
	if err := actual.Validate(); err != nil {
		t.Fatal(err)
	}
	fact := contract.InjectionConformanceFact{ContractVersion: contract.Version, ID: "conformance-1", Revision: 1, ExpectedRef: contract.FactRef{ID: expected.ID, Revision: 1, Digest: testkit.D("expected")}, ActualRef: contract.FactRef{ID: actual.ID, Revision: 1, Digest: testkit.D("actual")}, State: contract.InjectionMatched, Reason: "exact_match", InspectedUnixNano: testkit.Now + 3}
	if err := fact.Validate(); err != nil {
		t.Fatal(err)
	}
}
