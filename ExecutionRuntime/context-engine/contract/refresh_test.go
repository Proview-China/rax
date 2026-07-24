package contract_test

import (
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
)

func TestContextTurnRefreshRequestDeterministicAndStrictCardinality(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.Request
	resealed, err := contract.SealContextTurnRefreshRequestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resealed, request) {
		t.Fatalf("same frozen input changed identity\n got: %#v\nwant: %#v", resealed, request)
	}
	request.Cardinality.Memory = 1
	if _, err := contract.SealContextTurnRefreshRequestV1(request); err == nil {
		t.Fatal("Memory source entered N=1 Tool-only refresh")
	}
}

func TestContextTurnRefreshRequestRejectsPrefixAndCacheDrift(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.Request
	request.CacheIdentity.StablePrefix.Digest = contract.DigestBytes([]byte("drift"))
	if _, err := contract.SealContextTurnRefreshRequestV1(request); err == nil {
		t.Fatal("cache prefix drift was accepted")
	}
}
