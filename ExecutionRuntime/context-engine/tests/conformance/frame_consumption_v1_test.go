package conformance_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func TestFrameConsumptionContractsContainNoModelProjectionOwnership(t *testing.T) {
	for _, value := range []any{
		contract.ContextFrameConsumptionRequestV1{},
		contract.ContextFrameConsumptionDescriptorV1{},
		contract.ContextFragmentCacheKeyV1{},
		contract.ContextFrameCacheKeyV1{},
	} {
		typ := reflect.TypeOf(value)
		for index := 0; index < typ.NumField(); index++ {
			name := strings.ToLower(typ.Field(index).Name)
			for _, forbidden := range []string{"renderer", "modelfamily", "modelprofile", "toolschema", "projectioncache", "providercache"} {
				if strings.Contains(name, forbidden) {
					t.Fatalf("%s illegally owns field %s", typ.Name(), typ.Field(index).Name)
				}
			}
		}
	}
}

func TestContextCacheHintExposesThreeNeutralFingerprintsOnly(t *testing.T) {
	typ := reflect.TypeOf(contract.ContextCacheHintV1{})
	want := map[string]bool{
		"FrameFingerprint": true, "StablePrefixFingerprint": true, "SemiStablePrefixFingerprint": true,
	}
	for name := range want {
		if _, ok := typ.FieldByName(name); !ok {
			t.Fatalf("missing %s", name)
		}
	}
	if _, ok := typ.FieldByName("ProjectionFingerprint"); ok {
		t.Fatal("Context cache hint owns a Model projection fingerprint")
	}
}
