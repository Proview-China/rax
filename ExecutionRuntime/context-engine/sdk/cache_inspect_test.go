package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestInspectCachePlanOfflineExactCodecAndEconomicsV1(t *testing.T) {
	request := cacheInspectRequestFixtureV1(t)
	sealed, err := SealInspectCachePlanRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := EncodeInspectCachePlanRequestV1(context.Background(), sealed)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeInspectCachePlanRequestV1(context.Background(), payload)
	if err != nil || !reflect.DeepEqual(sealed, decoded) {
		t.Fatalf("cache inspect codec drift: %v", err)
	}
	response, err := InspectCachePlanV1(context.Background(), decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !response.Current || !response.EconomicDecision.WorthCreating || response.EconomicDecision.Reason != "expected_savings_positive" || response.ExpiresUnixNano != request.CachePlan.ExpiresUnixNano {
		t.Fatalf("unexpected cache inspection: %#v", response)
	}
	encoded, err := EncodeInspectCachePlanResponseV1(context.Background(), response)
	if err != nil || !bytes.Contains(encoded, []byte(`"worth_creating":true`)) || bytes.Contains(encoded, []byte(`"cache_hit"`)) {
		t.Fatalf("cache inspection encoding drift: %v %q", err, encoded)
	}
}

func TestInspectCachePlanFailClosedMatrixV1(t *testing.T) {
	request := cacheInspectRequestFixtureV1(t)
	payload, err := EncodeInspectCachePlanRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := bytes.Replace(payload, []byte(`"key_version":"v1"`), []byte(`"key_version":"v1","key_version":"v1"`), 1)
	if _, err := DecodeInspectCachePlanRequestV1(context.Background(), duplicate); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("nested duplicate must fail: %v", err)
	}
	var nullDocument map[string]any
	if err := json.Unmarshal(payload, &nullDocument); err != nil {
		t.Fatal(err)
	}
	nullDocument["cache_plan"] = nil
	nullPayload, err := json.Marshal(nullDocument)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeInspectCachePlanRequestV1(context.Background(), nullPayload); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("null cache plan must fail in strict codec: %v", err)
	}

	sealed, err := SealInspectCachePlanRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	tampered := sealed
	tampered.CachePlan.Partition.PrefixDigest = testkit.D("drift")
	if response, err := InspectCachePlanV1(context.Background(), tampered); !errors.Is(err, contract.ErrConflict) || !reflect.DeepEqual(response, InspectCachePlanResponseV1{}) {
		t.Fatalf("request drift must return zero: %#v %v", response, err)
	}

	for name, mutate := range map[string]func(*InspectCachePlanRequestV1){
		"plan_expired": func(value *InspectCachePlanRequestV1) { value.CheckedUnixNano = value.CachePlan.ExpiresUnixNano },
		"profile_expired": func(value *InspectCachePlanRequestV1) {
			value.CheckedUnixNano = value.ProviderCacheProfile.ExpiresUnixNano
		},
		"profile_ref_drift": func(value *InspectCachePlanRequestV1) { value.ProviderCacheProfile.Revision++ },
	} {
		t.Run(name, func(t *testing.T) {
			value := request
			mutate(&value)
			value.Meta.RequestDigest = ""
			value, err := SealInspectCachePlanRequestV1(context.Background(), value)
			if err != nil {
				t.Fatal(err)
			}
			response, err := InspectCachePlanV1(context.Background(), value)
			if err == nil || !reflect.DeepEqual(response, InspectCachePlanResponseV1{}) {
				t.Fatalf("must fail closed: %#v %v", response, err)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if response, err := InspectCachePlanV1(ctx, sealed); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(response, InspectCachePlanResponseV1{}) {
		t.Fatalf("cancel must return zero: %#v %v", response, err)
	}
	response, err := InspectCachePlanV1(context.Background(), sealed)
	if err != nil {
		t.Fatal(err)
	}
	response.EconomicDecision.WorthCreating = false
	if encoded, err := EncodeInspectCachePlanResponseV1(context.Background(), response); !errors.Is(err, contract.ErrConflict) || encoded != nil {
		t.Fatalf("tampered response must not encode: %q %v", encoded, err)
	}
}

func TestInspectCachePlanConcurrentDeterministicV1(t *testing.T) {
	sealed, err := SealInspectCachePlanRequestV1(context.Background(), cacheInspectRequestFixtureV1(t))
	if err != nil {
		t.Fatal(err)
	}
	want, err := InspectCachePlanV1(context.Background(), sealed)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 64)
	for index := 0; index < 64; index++ {
		go func() {
			got, err := InspectCachePlanV1(context.Background(), sealed)
			if err == nil && !reflect.DeepEqual(want, got) {
				err = errors.New("cache inspection drift")
			}
			errs <- err
		}()
	}
	for index := 0; index < 64; index++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func cacheInspectRequestFixtureV1(t *testing.T) InspectCachePlanRequestV1 {
	t.Helper()
	profile := contract.ProviderCacheProfile{
		ContractVersion: contract.Version, ID: "provider-profile-cache", Revision: 1, Provider: "provider",
		RouteID: "route-1", Model: "model-1", RequestControl: true, KeyOwnership: true, TTLControl: true,
		UsageObservable: true, CapabilityDigest: testkit.D("cache-capability"), ExpiresUnixNano: testkit.Now + 1_000,
	}
	profileDigest, err := profile.DigestValue(testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	partition := contract.CachePartition{
		AuditScopeDigest: testkit.D("audit"), ReuseScope: contract.ReuseRun, IsolationDigest: testkit.D("isolation"),
		AuthorityDigest: testkit.D("authority"), Sensitivity: contract.SensitivityInternal, SourceSetDigest: testkit.D("sources"),
		RecipeDigest: testkit.D("recipe"), RenderDigest: testkit.D("render"), ModelProfileDigest: testkit.D("model"),
		HarnessDigest: testkit.D("harness"), ToolSchemaDigest: testkit.D("tools"), PrefixDigest: testkit.D("prefix"),
		ProviderProfileRef: contract.FactRef{ID: profile.ID, Revision: profile.Revision, Digest: profileDigest}, KeyVersion: "v1",
	}
	plan := contract.CachePlan{
		ContractVersion: contract.Version, ID: "cache-plan-inspect", Revision: 1, Partition: partition,
		EligibleTokens: 1_000_000, PredictedReads: 3, ReadCostPerM: 10, WriteCostPerM: 20,
		TTL: 200, CreatedUnixNano: testkit.Now - 100, ExpiresUnixNano: testkit.Now + 100,
	}
	return InspectCachePlanRequestV1{Meta: requestMetaV1(OfflineInspectCachePlanV1, "cache-inspect-1"), CachePlan: plan, ProviderCacheProfile: profile, CheckedUnixNano: testkit.Now}
}
