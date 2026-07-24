package registry_test

import (
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

func TestWhiteboxRegistryDependencyAndCAS(t *testing.T) {
	r := registry.New()
	if _, err := r.SubmitTool(testkit.Tool(), testkit.FixedTime); err == nil {
		t.Fatal("tool without capability was accepted")
	}
	capRecord, err := r.SubmitCapability(testkit.Capability(), testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	toolRecord, err := r.SubmitTool(testkit.Tool(), testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.SubmitPackage(testkit.Package(), testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	capRecord, err = r.Transition("capability", string(testkit.Capability().ID), capRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Transition("capability", string(testkit.Capability().ID), capRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Transition("tool", string(testkit.Tool().ID), toolRecord.RegistryRevision+1, registry.StateAdmitted, testkit.FixedTime); err == nil {
		t.Fatal("stale registry CAS was accepted")
	}
	first, err := r.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Snapshot()
	if err != nil || first.Digest != second.Digest {
		t.Fatalf("registry snapshot is not deterministic: %v", err)
	}
}

func TestWhiteboxRegistryConcurrentCreateOnce(t *testing.T) {
	r := registry.New()
	const workers = 32
	var wg sync.WaitGroup
	errors := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.SubmitCapability(testkit.Capability(), testkit.FixedTime)
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := r.Snapshot()
	if err != nil || len(snapshot.Records) != 1 {
		t.Fatalf("want one create-once record, got %d: %v", len(snapshot.Records), err)
	}
}

func TestWhiteboxRegistryEnforcesDependenciesAndDefensiveCopies(t *testing.T) {
	r := registry.New()
	capability := testkit.Capability()
	if _, err := r.SubmitCapability(capability, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	toolRecord, err := r.SubmitTool(testkit.Tool(), testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Transition("tool", string(testkit.Tool().ID), toolRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime); err == nil {
		t.Fatal("tool was admitted before its capability")
	}
	capability.EffectKinds[0] = "praxis.tool/cancel"
	stored, _, ok := r.ResolveCapability(string(testkit.Capability().ID))
	if !ok || stored.EffectKinds[0] != "praxis.tool/execute" {
		t.Fatal("registry retained a caller-owned capability slice")
	}
	stored.EffectKinds[0] = "praxis.tool/cancel"
	again, _, _ := r.ResolveCapability(string(testkit.Capability().ID))
	if again.EffectKinds[0] != "praxis.tool/execute" {
		t.Fatal("registry exposed an internal capability slice")
	}
}

func BenchmarkRegistryResolveTool(b *testing.B) {
	r := registry.New()
	expected := testkit.Tool()
	if _, err := r.SubmitCapability(testkit.Capability(), testkit.FixedTime); err != nil {
		b.Fatal(err)
	}
	if _, err := r.SubmitTool(expected, testkit.FixedTime); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		tool, _, ok := r.ResolveTool(string(expected.ID))
		if !ok || tool.ID != expected.ID {
			b.Fatal("exact Tool resolve failed")
		}
	}
}
