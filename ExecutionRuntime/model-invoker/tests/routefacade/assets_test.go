package routefacade_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestEveryCallableDefaultRouteResolvesThroughV1FacadeWithoutProviderCalls(t *testing.T) {
	routeCatalog, err := catalog.NewDefault(testNow)
	if err != nil {
		t.Fatal(err)
	}
	providersByID := make(map[modelinvoker.ProviderID]*fakeProvider)
	var providers []modelinvoker.Provider
	for _, entry := range routeCatalog.Entries() {
		if !entry.Implementation.Callable {
			continue
		}
		id := modelinvoker.ProviderID(entry.Implementation.AdapterID)
		if _, exists := providersByID[id]; exists {
			continue
		}
		provider := &fakeProvider{id: id}
		providersByID[id] = provider
		providers = append(providers, provider)
	}
	registry, err := modelinvoker.NewRegistry(providers...)
	if err != nil {
		t.Fatal(err)
	}
	base, err := modelinvoker.NewInvoker(registry)
	if err != nil {
		t.Fatal(err)
	}
	routed, err := modelinvoker.NewRouteInvoker(routeCatalog, base, modelinvoker.WithRouteClock(func() time.Time { return testNow }))
	if err != nil {
		t.Fatal(err)
	}

	resolved := 0
	for _, entry := range routeCatalog.Entries() {
		if !entry.Implementation.Callable {
			continue
		}
		model := "runtime-selected-test-model"
		if entry.ModelDiscovery.Method == catalog.ModelDiscoveryStaticCatalog {
			model = entry.Route.Model.ProviderModelRef
			if len(entry.ModelDiscovery.Aliases) > 0 {
				model = entry.ModelDiscovery.Aliases[0].ProviderModelRef
			}
		}
		selection, err := routed.Resolve(callForEntry(entry, model, testNow))
		if err != nil {
			t.Errorf("Resolve(%q) error = %v", entry.ID, err)
			continue
		}
		resolved++
		if selection.RouteID != entry.ID || selection.Identity != entry.Route.Identity() || selection.AdapterID != modelinvoker.ProviderID(entry.Implementation.AdapterID) || selection.EvidenceDigest != entry.Evidence.Digest {
			t.Errorf("Resolve(%q) selection drift = %#v", entry.ID, selection)
		}
	}
	if resolved != 39 {
		t.Fatalf("resolved callable routes = %d, want 39", resolved)
	}
	for id, provider := range providersByID {
		if provider.capabilityCalls != 0 || provider.invokeCalls != 0 || provider.streamCalls != 0 {
			t.Errorf("Resolve contacted provider %q: %#v", id, provider)
		}
	}
}

func callForEntry(entry catalog.Entry, model string, now time.Time) modelinvoker.RouteCall {
	call := modelinvoker.RouteCall{RouteID: entry.ID, Invocation: generalInvocation(), Request: modelinvoker.Request{Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "offline resolve")}}}
	if entry.Route.Offering.Kind != upstream.OfferingTokenPlan && entry.Route.Offering.Kind != upstream.OfferingCodingPlan {
		return call
	}
	remaining := int64(100)
	call.Invocation = upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationInteractiveCoding, Subject: upstream.SubjectPersonal, Tenancy: upstream.TenancySingle, Execution: upstream.ExecutionForeground, ClientIdentity: upstream.ClientIdentity{Name: "praxis-cli", Version: "v1.0.0", UserAgent: "praxis-cli/v1.0.0", Source: upstream.ClientIdentityBuildManifest}}
	call.EntitlementState = &upstream.EntitlementState{OfferingID: entry.Route.Offering.ID, CredentialProfile: entry.Route.Credential.ID, Status: upstream.EntitlementActive, ObservedAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Minute), ExpiresAt: now.Add(24 * time.Hour), RemainingQuota: &remaining}
	return call
}

func TestV1DesignAssetsAreLinkedAndFreezeRequiredBoundaries(t *testing.T) {
	repository := repositoryRoot(t)
	designRoot := filepath.Join(repository, ".properties.rax", "design", "model-invoker")
	assets := map[string][]string{
		"semantic-primitives-v1.md": {
			modelinvoker.SemanticPrimitivesCandidateVersion, "候选", "Runtime Kernel", "Context Engine", "Model Profile", "ProviderOptions",
		},
		"route-invocation-facade-v1.md": {
			modelinvoker.RoutePolicyCandidateVersion, "Policy/Audit", "route_selector_owned", "route_not_callable", "AllowsAutomaticPAYGSwitch=false",
		},
		"provider-cache-transport-boundary-v1.md": {
			"只拥有“缓存相关字段如何安全穿过 Provider边界”的传输合同", "CacheReadTokens", "CacheWriteTokens", "不实现的缓存策略",
		},
		"final-candidate-review.md": {
			"已完成", "候选待联合审核", "明确延期", "需用户决定", "未运行真实验证", "覆盖率重新实测为78.0%",
		},
	}
	for name, required := range assets {
		contents, err := os.ReadFile(filepath.Join(designRoot, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, phrase := range required {
			if !strings.Contains(string(contents), phrase) {
				t.Errorf("%s is missing frozen boundary %q", name, phrase)
			}
		}
	}
	index, err := os.ReadFile(filepath.Join(designRoot, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name := range assets {
		if !strings.Contains(string(index), "./"+name) {
			t.Errorf("design README does not link %s", name)
		}
	}
}

func TestCandidateStatusAndHistoricalDriftCorrectionsStayAccurate(t *testing.T) {
	repository := repositoryRoot(t)
	read := func(relative string) string {
		t.Helper()
		contents, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		return string(contents)
	}

	matrix := read(".properties.rax/design/model-invoker/provider-matrix.md")
	if !strings.Contains(matrix, "| `xai.api` | xAI / 按量 API") || !strings.Contains(matrix, "| `implemented_offline` | `fresh` |") {
		t.Fatal("xAI hand-maintained matrix does not reflect the implemented Responses route")
	}
	if strings.Contains(matrix, "| `xai.direct` |") {
		t.Fatal("stale xai.direct research-only matrix row returned")
	}

	module := read(".properties.rax/module/model-invoker/README.md")
	if !strings.Contains(module, "第三阶段最终记录的全仓合并覆盖率为76.7%") {
		t.Fatal("module README lost the final 76.7% historical record")
	}

	snapshot := read(".properties.rax/memory/model-invoker/20260711-133510-上游调用与统一封装v1完成.md")
	if !strings.Contains(snapshot, "tmp.document/`在现场仍存在且归属不确定") || !strings.Contains(snapshot, "描述失实") {
		t.Fatal("stage snapshot misstates tmp.document live ownership/state")
	}

	semantic := read(".properties.rax/design/model-invoker/semantic-primitives-v1.md")
	policy := read(".properties.rax/design/model-invoker/route-invocation-facade-v1.md")
	if !strings.Contains(semantic, "v1candidate") || strings.Contains(semantic, "状态：已冻结") {
		t.Fatal("semantic primitives are no longer represented as a candidate")
	}
	if !strings.Contains(policy, "不是完整 Route Gateway") {
		t.Fatal("RouteInvoker responsibility drifted back to a complete gateway claim")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve route facade asset test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", "..", ".."))
}
