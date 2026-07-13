package executionunion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
)

func TestRegistryIsImmutableAndDeterministic(t *testing.T) {
	registry := execution.NewRegistry()
	for _, id := range []string{"zeta", "alpha"} {
		if err := registry.Register(context.Background(), &fakeAdapter{id: id, session: newFakeSession(1)}); err != nil {
			t.Fatalf("Register(%s): %v", id, err)
		}
	}
	if err := registry.Register(context.Background(), &fakeAdapter{id: "alpha", session: newFakeSession(1)}); !errors.Is(err, execution.ErrAdapterAlreadyRegistered) {
		t.Fatalf("duplicate adapter error = %v", err)
	}
	descriptors := registry.Descriptors()
	if len(descriptors) != 2 || descriptors[0].Identity.ID != "alpha" || descriptors[1].Identity.ID != "zeta" {
		t.Fatalf("descriptor order = %#v", descriptors)
	}
	descriptors[0].ExecutionKinds[0] = "agent"
	resolved, err := registry.Resolve("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if string(resolved.Descriptor.ExecutionKinds[0]) != "model" {
		t.Fatal("descriptor mutation escaped the registry clone boundary")
	}
}

func TestContextManifestRejectsStaleDigest(t *testing.T) {
	expected := actualManifest()
	actual := actualManifest()
	actual.ID = "actual-with-stale-digest"
	actual.Digest = "sha256:stale"
	if err := execution.CompareContextManifests(expected, actual); !errors.Is(err, execution.ErrPreflightManifestDrift) {
		t.Fatalf("stale manifest digest error = %v", err)
	}
}
