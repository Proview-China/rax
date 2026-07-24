package assemblyintegration_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblyadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
)

func FuzzBuildCandidateV1NeverAcceptsSelectedDigestOrCurrentnessDrift(f *testing.F) {
	for _, seed := range []byte{0, 1, 2, 3, 4, 255} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, selector byte) {
		fixture := newFixtureV1(t)
		expectSuccess := selector%5 == 0
		switch selector % 5 {
		case 1:
			fixture.request.GenerationCurrentness.Current = false
		case 2:
			fixture.request.Binding.ProjectionDigest = assemblytestkit.Digest(selector)
		case 3:
			fixture.request.Activation.ProjectionDigest = assemblytestkit.Digest(selector)
		case 4:
			fixture.request.Handoff.RequiredExtension = "praxis.harness/missing-extension"
			fixture.request.Handoff.Digest, _ = assemblycontract.HandoffDigestV1(fixture.request.Handoff)
		}
		_, err := assemblyadapter.BuildCandidateV1(fixture.request, fixture.now)
		if expectSuccess && err != nil {
			t.Fatalf("unchanged seed rejected: %v", err)
		}
		if !expectSuccess && err == nil {
			t.Fatal("selected drift was accepted")
		}
	})
}
