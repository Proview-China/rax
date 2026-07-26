package kernel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestBuildFrameConsumptionDescriptorExactAndTTLMinimum(t *testing.T) {
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	reader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{fixture.Snapshot, fixture.Snapshot}}
	descriptor, err := kernel.BuildFrameConsumptionDescriptorV1(context.Background(), reader, fixture.AwareStore, fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	if reader.Calls != 2 || descriptor.ExpiresUnixNano != testkit.Now+600 {
		t.Fatalf("S1/S2 or TTL min mismatch calls=%d expires=%d", reader.Calls, descriptor.ExpiresUnixNano)
	}
	if descriptor.FrameRef != fixture.Request.FrameRef || descriptor.CacheHint.FrameFingerprint.Validate() != nil || descriptor.CacheHint.StablePrefixFingerprint.Validate() != nil || descriptor.CacheHint.SemiStablePrefixFingerprint == nil {
		t.Fatal("descriptor exact closure incomplete")
	}
}

func TestFrameFingerprintsRespectRegions(t *testing.T) {
	baseline := descriptorFixtureV1(t, "stable", "semi", "dynamic-a")
	dynamic := descriptorFixtureV1(t, "stable", "semi", "dynamic-b")
	if baseline.CacheHint.FrameFingerprint == dynamic.CacheHint.FrameFingerprint {
		t.Fatal("dynamic change did not change frame fingerprint")
	}
	if baseline.CacheHint.StablePrefixFingerprint != dynamic.CacheHint.StablePrefixFingerprint || *baseline.CacheHint.SemiStablePrefixFingerprint != *dynamic.CacheHint.SemiStablePrefixFingerprint {
		t.Fatal("dynamic change broke stable or semi fingerprint")
	}
	stable := descriptorFixtureV1(t, "stable-changed", "semi", "dynamic-a")
	if baseline.CacheHint.FrameFingerprint == stable.CacheHint.FrameFingerprint || baseline.CacheHint.StablePrefixFingerprint == stable.CacheHint.StablePrefixFingerprint {
		t.Fatal("stable change did not change frame and stable fingerprints")
	}
	if *baseline.CacheHint.SemiStablePrefixFingerprint != *stable.CacheHint.SemiStablePrefixFingerprint {
		t.Fatal("stable change broke semi fingerprint")
	}
	semi := descriptorFixtureV1(t, "stable", "semi-changed", "dynamic-a")
	if baseline.CacheHint.FrameFingerprint == semi.CacheHint.FrameFingerprint || *baseline.CacheHint.SemiStablePrefixFingerprint == *semi.CacheHint.SemiStablePrefixFingerprint {
		t.Fatal("semi change did not change frame and semi fingerprints")
	}
	if baseline.CacheHint.StablePrefixFingerprint != semi.CacheHint.StablePrefixFingerprint {
		t.Fatal("semi change broke stable fingerprint")
	}
	withoutSemi := descriptorFixtureV1(t, "stable", "", "dynamic-a")
	if withoutSemi.CacheHint.SemiStablePrefixFingerprint != nil || withoutSemi.CacheHint.SemiStableEligible {
		t.Fatal("frame without semi-stable region exposed a fingerprint")
	}
}

func TestFrameConsumptionS2DriftAndReaderFailureFailClosed(t *testing.T) {
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	drift := fixture.Snapshot
	drift.RecipeExpiresUnixNano--
	reader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{fixture.Snapshot, drift}}
	if _, err = kernel.BuildFrameConsumptionDescriptorV1(context.Background(), reader, fixture.AwareStore, fixture.Request); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("S2 drift error=%v", err)
	}
	reader = &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{fixture.Snapshot}, Errors: []error{nil, contract.ErrUnavailable}}
	if _, err = kernel.BuildFrameConsumptionDescriptorV1(context.Background(), reader, fixture.AwareStore, fixture.Request); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("S2 unavailable error=%v", err)
	}
}

func TestFrameConsumptionTTLBoundaryAndCancel(t *testing.T) {
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	expired := fixture.Snapshot
	expired.FragmentSourceExpires[0] = fixture.Request.CheckedUnixNano
	reader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{expired}}
	if _, err = kernel.BuildFrameConsumptionDescriptorV1(context.Background(), reader, fixture.AwareStore, fixture.Request); !errors.Is(err, contract.ErrExpired) {
		t.Fatalf("TTL boundary error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = kernel.BuildFrameConsumptionDescriptorV1(ctx, reader, fixture.AwareStore, fixture.Request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
}

func descriptorFixtureV1(t *testing.T, stable, semi, dynamic string) contract.ContextFrameConsumptionDescriptorV1 {
	t.Helper()
	fixture, err := testfixture.NewFrameConsumptionFixtureWithContentV1(stable, semi, dynamic)
	if err != nil {
		t.Fatal(err)
	}
	reader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{fixture.Snapshot, fixture.Snapshot}}
	descriptor, err := kernel.BuildFrameConsumptionDescriptorV1(context.Background(), reader, fixture.AwareStore, fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}
