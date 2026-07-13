package performance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
)

var (
	benchmarkPlanDigest       string
	benchmarkManifestDigest   string
	benchmarkReplayState      execution.LedgerState
	benchmarkFileSnapshotHash string
)

func BenchmarkProfileCompile(b *testing.B) {
	compiler, input := profileCompileFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		compilation, err := compiler.Compile(input)
		if err != nil {
			b.Fatalf("Compile: %v", err)
		}
		benchmarkPlanDigest = compilation.Plan.Digest
	}
}

func BenchmarkManifestDiff(b *testing.B) {
	profiles, err := profile.RepresentativeProfiles(performanceTime)
	if err != nil {
		b.Fatal(err)
	}
	expected := profiles[0].HarnessCapability.ExpectedManifest
	actual := observedManifest(expected)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		evaluation, err := profile.CompareManifests(expected, actual, profile.ContextSemanticStable, nil)
		if err != nil {
			b.Fatalf("CompareManifests: %v", err)
		}
		benchmarkManifestDigest, err = evaluation.Digest()
		if err != nil {
			b.Fatalf("ManifestEvaluation.Digest: %v", err)
		}
	}
}

func BenchmarkEventReplay(b *testing.B) {
	events := replayEvents(256)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ledger, err := execution.Replay("exec-performance-replay", events)
		if err != nil {
			b.Fatalf("Replay: %v", err)
		}
		benchmarkReplayState = ledger.State()
	}
}

func BenchmarkFileSnapshot(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "snapshot.bin")
	payload := bytes.Repeat([]byte("praxis-file-snapshot\n"), 4096)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		b.Fatal(err)
	}
	observer, err := effect.NewFileObserver(effect.FilePolicy{
		AllowedRoots: []string{root}, MaxFileBytes: int64(len(payload)) + 1,
		MaxCaptureBytes: int64(len(payload)) + 1,
	})
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		snapshot, err := observer.Capture(path)
		if err != nil {
			b.Fatalf("Capture: %v", err)
		}
		benchmarkFileSnapshotHash = snapshot.State.Hash
	}
}
