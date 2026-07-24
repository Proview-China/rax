package testfixture

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/refreshstore"
)

type MutableClockV1 struct {
	mu  sync.RWMutex
	now time.Time
}

func NewMutableClockV1(now time.Time) *MutableClockV1 { return &MutableClockV1{now: now} }
func (c *MutableClockV1) Now() time.Time              { c.mu.RLock(); defer c.mu.RUnlock(); return c.now }
func (c *MutableClockV1) Set(now time.Time)           { c.mu.Lock(); defer c.mu.Unlock(); c.now = now }

type RefreshFixtureV1 struct {
	Now            time.Time
	Clock          *MutableClockV1
	Parent         *ParentFrameFixtureV1
	ToolReader     *testkit.SettledActionSourceReaderV1
	ToolProjection contract.SettledActionContextSourceCurrentV1
	Store          *refreshstore.Memory
	Service        *kernel.ContextTurnRefreshServiceV1
	Request        contract.ContextTurnRefreshRequestV1
}

func NewRefreshFixtureV1() (*RefreshFixtureV1, error) {
	return newRefreshFixtureV1(false)
}

func NewRefreshFixtureWithOwnerSourcesV1() (*RefreshFixtureV1, error) {
	return newRefreshFixtureV1(true)
}

func newRefreshFixtureV1(ownerSources bool) (*RefreshFixtureV1, error) {
	now := time.Unix(0, testkit.Now)
	clock := NewMutableClockV1(now)
	recipe := testkit.Recipe()
	recipe.Rules = append(recipe.Rules, contract.FragmentRule{Kind: contract.FragmentToolResult, Region: contract.RegionDynamicTail, MaxTokens: 100, Degradation: contract.DegradeExclude})
	if ownerSources {
		recipe.Rules = append(recipe.Rules,
			contract.FragmentRule{Kind: contract.FragmentMemoryRecall, Region: contract.RegionDynamicTail, MaxTokens: 100, Degradation: contract.DegradeExclude},
			contract.FragmentRule{Kind: contract.FragmentKnowledgeReference, Region: contract.RegionDynamicTail, MaxTokens: 100, Degradation: contract.DegradeExclude},
		)
	}
	parent, err := NewParentFrameFixtureWithRecipeV1(clock.Now, 30*time.Second, recipe)
	if err != nil {
		return nil, err
	}
	toolContent, err := parent.Content.Put([]byte("bounded settled tool summary"))
	if err != nil {
		return nil, err
	}
	toolRequest := contract.SettledActionContextSourceRequestV1{
		ToolResultRef: fact("tool-result-v2"), DomainResultRef: fact("tool-domain-result"), ApplySettlementRef: fact("tool-apply-settlement"), InspectionRef: fact("runtime-v4-inspection"), AssociationRef: fact("verified-association"),
		Execution: parent.Frame.Execution, ActionID: "action-1", AttemptID: "tool-attempt-1",
	}
	toolProjection, err := contract.SealSettledActionContextSourceCurrentV1(contract.SettledActionContextSourceCurrentV1{Request: toolRequest, Content: toolContent, TokenEstimate: 8, Sensitivity: contract.SensitivityInternal, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}, now.UnixNano())
	if err != nil {
		return nil, err
	}
	toolReader := testkit.NewSettledActionSourceReaderV1(toolProjection)
	stableRefs := make([]contract.FactRef, 0)
	for _, fragment := range parent.Manifest.Fragments {
		if fragment.Region == contract.RegionStablePrefix {
			stableRefs = append(stableRefs, fragment.CandidateRef)
		}
	}
	stableDigest, err := contract.DigestJSON(stableRefs)
	if err != nil {
		return nil, err
	}
	cache, err := contract.SealContextStableCacheIdentityV1(contract.ContextStableCacheIdentityV1{ReuseScope: "run", IsolationDigest: testkit.D("isolation"), AuthorityDigest: parent.Frame.Execution.AuthorityDigest, StableSourceSetDigest: stableDigest, RecipeRef: parent.Manifest.RecipeRef, RenderVersion: parent.Recipe.RenderVersion, ModelProfileDigest: testkit.D("model-profile"), HarnessGenerationRef: fact("harness-generation"), ToolSchemaDigest: testkit.D("tool-schema"), StablePrefix: parent.Frame.StablePrefix, SemiStable: parent.Frame.SemiStable, ProviderProfileDigest: testkit.D("provider-profile"), KeyVersion: "v1", ExpiresUnixNano: now.Add(11 * time.Second).UnixNano()}, now.UnixNano())
	if err != nil {
		return nil, err
	}
	request, err := contract.SealContextTurnRefreshRequestV1(contract.ContextTurnRefreshRequestV1{IdempotencyKey: "refresh-idempotency-1", ParentSource: parent.Source, ExpectedCurrent: parent.Pointer, Recipe: parent.Recipe, ToolSource: toolRequest, Cardinality: contract.ContextTurnRefreshSourceCardinalityV1{Tool: 1}, CacheIdentity: cache, CheckedUnixNano: now.UnixNano(), NotAfterUnixNano: now.Add(15 * time.Second).UnixNano()})
	if err != nil {
		return nil, err
	}
	store, err := refreshstore.NewMemoryWithCurrentV1(refreshstore.CurrentStateV1{Binding: parent.Binding, Frame: parent.Frame, Manifest: parent.Manifest, Generation: parent.Generation, Pointer: parent.Pointer})
	if err != nil {
		return nil, err
	}
	authoritativeReader, err := kernel.NewParentFrameCurrentReaderV1(store, store, store, store, store, parent.Content, clock.Now, 30*time.Second)
	if err != nil {
		return nil, err
	}
	parent.Reader = authoritativeReader
	service, err := kernel.NewContextTurnRefreshServiceV1(store, toolReader, parent.Content, clock.Now, 30*time.Second)
	if err != nil {
		return nil, err
	}
	return &RefreshFixtureV1{Now: now, Clock: clock, Parent: parent, ToolReader: toolReader, ToolProjection: toolProjection, Store: store, Service: service, Request: request}, nil
}

func fact(id string) contract.FactRef {
	return contract.FactRef{ID: id, Revision: 1, Digest: testkit.D(id)}
}

func (f *RefreshFixtureV1) Prepare() (contract.ContextTurnRefreshPreparedV1, error) {
	return f.Service.RefreshContextTurnV1(context.Background(), f.Request)
}
