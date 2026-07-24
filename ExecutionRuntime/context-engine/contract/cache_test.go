package contract_test

import (
	"errors"
	"math"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestProviderCacheProfileExpiredIsTypedExpired(t *testing.T) {
	profile := contract.ProviderCacheProfile{ContractVersion: contract.Version, ID: "profile-expired", Revision: 1, Provider: "provider", RouteID: "route", Model: "model", CapabilityDigest: testkit.D("capability"), ExpiresUnixNano: testkit.Now}
	if err := profile.Validate(testkit.Now); !errors.Is(err, contract.ErrExpired) {
		t.Fatalf("want expired, got %v", err)
	}
}

func TestOfflineCacheEconomics(t *testing.T) {
	p := cachePlan()
	p.EligibleTokens, p.PredictedReads, p.ReadCostPerM, p.WriteCostPerM = 1_000_000, 3, 10, 20
	decision, err := contract.CompareCacheEconomics(p)
	if err != nil || !decision.WorthCreating || decision.ExpectedRead != 30 || decision.ExpectedCost != 20 {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
	p.PredictedReads = 1
	decision, err = contract.CompareCacheEconomics(p)
	if err != nil || decision.WorthCreating {
		t.Fatalf("negative economics should not create: %#v err=%v", decision, err)
	}
}

func TestCacheKeyCoversEveryPartitionDimension(t *testing.T) {
	base := cachePlan().Partition
	want, err := base.KeyDigest()
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(*contract.CachePartition){
		func(p *contract.CachePartition) { p.AuditScopeDigest = testkit.D("audit-2") },
		func(p *contract.CachePartition) { p.ReuseScope = contract.ReuseInstance },
		func(p *contract.CachePartition) { p.IsolationDigest = testkit.D("isolation-2") },
		func(p *contract.CachePartition) { p.AuthorityDigest = testkit.D("authority-2") },
		func(p *contract.CachePartition) { p.Sensitivity = contract.SensitivityConfidential },
		func(p *contract.CachePartition) { p.SourceSetDigest = testkit.D("sources-2") },
		func(p *contract.CachePartition) { p.RecipeDigest = testkit.D("recipe-2") },
		func(p *contract.CachePartition) { p.RenderDigest = testkit.D("render-2") },
		func(p *contract.CachePartition) { p.ModelProfileDigest = testkit.D("model-2") },
		func(p *contract.CachePartition) { p.HarnessDigest = testkit.D("harness-2") },
		func(p *contract.CachePartition) { p.ToolSchemaDigest = testkit.D("tools-2") },
		func(p *contract.CachePartition) { p.PrefixDigest = testkit.D("prefix-2") },
		func(p *contract.CachePartition) { p.ProviderProfileRef.Digest = testkit.D("provider-profile-2") },
		func(p *contract.CachePartition) { p.KeyVersion = "v2" },
	}
	for index, mutate := range mutations {
		changed := base
		mutate(&changed)
		got, err := changed.KeyDigest()
		if err != nil {
			t.Fatalf("mutation %d: %v", index, err)
		}
		if got == want {
			t.Fatalf("partition mutation %d did not change cache key", index)
		}
	}
}

func TestCachePlanCurrentnessBindsExactProviderProfile(t *testing.T) {
	profile := contract.ProviderCacheProfile{
		ContractVersion: contract.Version, ID: "provider-profile", Revision: 2, Provider: "provider", RouteID: "route-1", Model: "model-1",
		CapabilityDigest: testkit.D("capability"), ExpiresUnixNano: testkit.Now + 1_000,
	}
	profileDigest, err := profile.DigestValue(testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	plan := cachePlan()
	plan.Partition.ProviderProfileRef = contract.FactRef{ID: profile.ID, Revision: profile.Revision, Digest: profileDigest}
	plan.CreatedUnixNano = testkit.Now - 100
	plan.ExpiresUnixNano = testkit.Now + 100
	plan.TTL = 200
	if err := plan.ValidateCurrent(profile, testkit.Now); err != nil {
		t.Fatal(err)
	}
	stale := profile
	stale.Revision++
	if err := plan.ValidateCurrent(stale, testkit.Now); err == nil {
		t.Fatal("stale provider profile revision was accepted")
	}
	if err := plan.ValidateCurrent(profile, plan.ExpiresUnixNano); err == nil {
		t.Fatal("expired cache plan was current")
	}
	plan.ExpiresUnixNano = profile.ExpiresUnixNano + 1
	plan.TTL = plan.ExpiresUnixNano - plan.CreatedUnixNano
	if err := plan.ValidateCurrent(profile, testkit.Now); err == nil {
		t.Fatal("plan outliving provider profile was accepted")
	}
}

func TestCacheEconomicsSaturatesOverflow(t *testing.T) {
	plan := cachePlan()
	plan.EligibleTokens = math.MaxUint64
	plan.PredictedReads = math.MaxUint64
	plan.ReadCostPerM = math.MaxUint64
	decision, err := contract.CompareCacheEconomics(plan)
	if err != nil {
		t.Fatal(err)
	}
	if decision.ExpectedRead != math.MaxUint64 {
		t.Fatalf("overflow wrapped instead of saturating: %d", decision.ExpectedRead)
	}
}

func TestCacheEconomicsLargeArithmeticDoesNotWrapOrPreDivideSaturate(t *testing.T) {
	plan := cachePlan()
	plan.EligibleTokens = uint64(1) << 63
	plan.PredictedReads = 0
	plan.ReadCostPerM = 0
	plan.WriteCostPerM = 2
	plan.KeepaliveCost = 0
	decision, err := contract.CompareCacheEconomics(plan)
	if err != nil {
		t.Fatal(err)
	}
	const want = uint64(18_446_744_073_709)
	if decision.ExpectedCost != want || decision.WorthCreating {
		t.Fatalf("exact post-division cost drift: %#v want=%d", decision, want)
	}

	plan.PredictedReads = math.MaxUint64
	plan.ReadCostPerM = math.MaxUint64
	plan.WriteCostPerM = 1
	decision, err = contract.CompareCacheEconomics(plan)
	if err != nil {
		t.Fatal(err)
	}
	if decision.ExpectedRead != math.MaxUint64 || !decision.WorthCreating {
		t.Fatalf("read savings did not saturate safely: %#v", decision)
	}

	plan.PredictedReads = 0
	plan.ReadCostPerM = 0
	plan.WriteCostPerM = math.MaxUint64
	plan.KeepaliveCost = math.MaxUint64
	decision, err = contract.CompareCacheEconomics(plan)
	if err != nil {
		t.Fatal(err)
	}
	if decision.ExpectedCost != math.MaxUint64 || decision.WorthCreating {
		t.Fatalf("write cost wrapped instead of saturating: %#v", decision)
	}
}

func cachePlan() contract.CachePlan {
	d := testkit.D
	partition := contract.CachePartition{
		AuditScopeDigest: d("audit"), ReuseScope: contract.ReuseRun, IsolationDigest: d("isolation"), AuthorityDigest: d("authority"), Sensitivity: contract.SensitivityInternal,
		SourceSetDigest: d("sources"), RecipeDigest: d("recipe"), RenderDigest: d("render"), ModelProfileDigest: d("model"), HarnessDigest: d("harness"), ToolSchemaDigest: d("tools"), PrefixDigest: d("prefix"),
		ProviderProfileRef: contract.FactRef{ID: "provider-profile", Revision: 1, Digest: d("provider-profile")}, KeyVersion: "v1",
	}
	return contract.CachePlan{ContractVersion: contract.Version, ID: "plan-1", Revision: 1, Partition: partition, EligibleTokens: 1, TTL: 100, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 100}
}
