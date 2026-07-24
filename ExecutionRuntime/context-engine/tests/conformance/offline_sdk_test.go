package conformance_test

import (
	"reflect"
	"testing"

	contextsdk "github.com/Proview-China/rax/ExecutionRuntime/context-engine/sdk"
)

func TestOfflineSDKDoesNotExposeMutableStoreOrAuthorityV1(t *testing.T) {
	bundleType := reflect.TypeOf(contextsdk.OfflineContentBundleV1{})
	for _, forbidden := range []string{"Put", "Delete", "Commit", "CAS", "SetCurrent", "Settle", "RegisterCapability"} {
		if _, exists := bundleType.MethodByName(forbidden); exists {
			t.Fatalf("offline bundle exposes forbidden method %s", forbidden)
		}
	}
	for _, allowed := range []string{"Items", "Lookup", "ContentSetDigest"} {
		if _, exists := bundleType.MethodByName(allowed); !exists {
			t.Fatalf("offline bundle missing frozen read method %s", allowed)
		}
	}
}
