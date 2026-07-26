package testfixture

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/refstore"
)

type ContextAwareRefStoreV1 struct {
	Store *refstore.Memory
}

func (s ContextAwareRefStoreV1) GetContextV1(ctx context.Context, ref contract.ContentRef) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Store.Get(ref)
}

func (s ContextAwareRefStoreV1) PutContextV1(ctx context.Context, value []byte) (contract.ContentRef, error) {
	if err := ctx.Err(); err != nil {
		return contract.ContentRef{}, err
	}
	return s.Store.Put(value)
}

type FrameConsumptionReaderV1 struct {
	mu        sync.Mutex
	Snapshots []kernel.FrameConsumptionCurrentSnapshotV1
	Errors    []error
	Calls     int
}

func (r *FrameConsumptionReaderV1) InspectFrameConsumptionCurrentV1(ctx context.Context, _ contract.ContextFrameConsumptionRequestV1) (kernel.FrameConsumptionCurrentSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return kernel.FrameConsumptionCurrentSnapshotV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.Calls
	r.Calls++
	if index < len(r.Errors) && r.Errors[index] != nil {
		return kernel.FrameConsumptionCurrentSnapshotV1{}, r.Errors[index]
	}
	if len(r.Snapshots) == 0 {
		return kernel.FrameConsumptionCurrentSnapshotV1{}, contract.ErrUnavailable
	}
	if index >= len(r.Snapshots) {
		index = len(r.Snapshots) - 1
	}
	return cloneSnapshotV1(r.Snapshots[index]), nil
}

type FrameConsumptionFixtureV1 struct {
	Store      *refstore.Memory
	AwareStore ContextAwareRefStoreV1
	Result     kernel.CompileResult
	Generation contract.ContextGeneration
	Snapshot   kernel.FrameConsumptionCurrentSnapshotV1
	Request    contract.ContextFrameConsumptionRequestV1
}

func NewFrameConsumptionFixtureV1() (*FrameConsumptionFixtureV1, error) {
	return NewFrameConsumptionFixtureWithContentV1("authoritative instruction", "semi-stable artifact", "dynamic tail")
}

func NewFrameConsumptionFixtureWithContentV1(stableValue, semiValue, dynamicValue string) (*FrameConsumptionFixtureV1, error) {
	store := refstore.NewMemory()
	instruction, err := store.Put([]byte(stableValue))
	if err != nil {
		return nil, err
	}
	dynamic, err := store.Put([]byte(dynamicValue))
	if err != nil {
		return nil, err
	}
	candidates := []contract.ContextCandidate{
		testkit.Candidate("consumption-instruction", contract.FragmentInstruction, instruction, 20),
		testkit.Candidate("consumption-tail", contract.FragmentConversation, dynamic, 8),
	}
	if semiValue != "" {
		artifact, putErr := store.Put([]byte(semiValue))
		if putErr != nil {
			return nil, putErr
		}
		candidates = append(candidates, testkit.Candidate("consumption-artifact", contract.FragmentArtifactInline, artifact, 10))
	}
	result, err := kernel.Compile(store, kernel.CompileRequest{
		AttemptID: "consumption-attempt-1", ManifestID: "consumption-manifest-1", FrameID: "consumption-frame-1", GenerationID: "consumption-generation-1", Generation: 1,
		Recipe: testkit.Recipe(), Execution: testkit.Execution(), Candidates: candidates, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1_000,
	})
	if err != nil {
		return nil, err
	}
	frameDigest, err := result.Frame.DigestValue()
	if err != nil {
		return nil, err
	}
	manifestDigest, err := result.Manifest.DigestValue()
	if err != nil {
		return nil, err
	}
	frameRef := contract.FactRef{ID: result.Frame.ID, Revision: result.Frame.Revision, Digest: frameDigest}
	manifestRef := contract.FactRef{ID: result.Manifest.ID, Revision: result.Manifest.Revision, Digest: manifestDigest}
	generation := contract.ContextGeneration{
		ContractVersion: contract.Version,
		ID:              result.Frame.GenerationID,
		Revision:        1,
		Ordinal:         result.Frame.Generation,
		RootFrame:       frameRef,
		RetainedAnchors: []contract.FactRef{},
		OpenEffects:     []contract.FactRef{},
		CreatedUnixNano: testkit.Now,
	}
	generationDigest, err := contract.DigestJSON(generation)
	if err != nil {
		return nil, err
	}
	request, err := contract.SealContextFrameConsumptionRequestV1(contract.ContextFrameConsumptionRequestV1{
		DescriptorID:      "consumption-descriptor-1",
		FrameRef:          frameRef,
		ManifestRef:       manifestRef,
		GenerationRef:     contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: generationDigest},
		TenantScopeDigest: testkit.D("tenant-scope"),
		AgentInstanceRef:  contract.FactRef{ID: "agent-instance-1", Revision: 1, Digest: testkit.D("agent-instance")},
		RunID:             testkit.Execution().RunID,
		RunScopeDigest:    testkit.Execution().ScopeDigest,
		PromptAssetRefs:   []contract.PromptAssetRefV1{},
		RecipeRef:         result.Manifest.RecipeRef,
		DisclosureClass:   contract.DisclosureInternalV1,
		CheckedUnixNano:   testkit.Now,
		NotAfterUnixNano:  testkit.Now + 1_000,
	})
	if err != nil {
		return nil, err
	}
	fragmentExpiries := make([]int64, len(result.Manifest.Fragments))
	for index := range fragmentExpiries {
		fragmentExpiries[index] = testkit.Now + 800 - int64(index)*100
	}
	snapshot := kernel.FrameConsumptionCurrentSnapshotV1{
		Manifest:                  result.Manifest,
		Frame:                     result.Frame,
		Generation:                generation,
		GenerationExpiresUnixNano: testkit.Now + 900,
		FragmentSourceExpires:     fragmentExpiries,
		PromptExpiresUnixNano:     testkit.Now + 900,
		RecipeExpiresUnixNano:     testkit.Now + 850,
		DisclosureExpiresUnixNano: testkit.Now + 750,
		AuthorityExpiresUnixNano:  testkit.Now + 650,
	}
	return &FrameConsumptionFixtureV1{
		Store: store, AwareStore: ContextAwareRefStoreV1{Store: store}, Result: result, Generation: generation, Snapshot: snapshot, Request: request,
	}, nil
}

func cloneSnapshotV1(value kernel.FrameConsumptionCurrentSnapshotV1) kernel.FrameConsumptionCurrentSnapshotV1 {
	copy := value
	copy.Manifest.Decisions = append([]contract.AdmissionDecision{}, value.Manifest.Decisions...)
	copy.Manifest.Fragments = append([]contract.ContextFragment{}, value.Manifest.Fragments...)
	copy.Generation.RetainedAnchors = append([]contract.FactRef{}, value.Generation.RetainedAnchors...)
	copy.Generation.OpenEffects = append([]contract.FactRef{}, value.Generation.OpenEffects...)
	copy.FragmentSourceExpires = append([]int64{}, value.FragmentSourceExpires...)
	if value.Frame.SemiStable != nil {
		ref := *value.Frame.SemiStable
		copy.Frame.SemiStable = &ref
	}
	return copy
}
