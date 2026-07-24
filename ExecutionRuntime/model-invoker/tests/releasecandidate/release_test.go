package releasecandidate_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	. "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/releasecandidate"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var testNowV1 = time.Date(2026, 7, 18, 2, 0, 0, 0, time.UTC)

type fixedClockV1 struct{ value time.Time }

func (clock fixedClockV1) Now() time.Time { return clock.value }

type sequenceClockV1 struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func (clock *sequenceClockV1) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	value := clock.values[clock.index]
	if clock.index+1 < len(clock.values) {
		clock.index++
	}
	return value
}

type releaseStoreV1 struct {
	mu    sync.Mutex
	value assemblercontract.ComponentReleaseV1
	lose  bool
	drift bool
}

func (store *releaseStoreV1) EnsureExactComponentReleaseV1(_ context.Context, value assemblercontract.ComponentReleaseV1) (assemblercontract.ComponentReleaseV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.value = assemblercontract.CloneComponentReleaseV1(value)
	if store.lose {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost release reply")
	}
	return assemblercontract.CloneComponentReleaseV1(value), nil
}
func (store *releaseStoreV1) InspectExactComponentReleaseV1(_ context.Context, ref assemblercontract.ComponentReleaseRefV1) (assemblercontract.ComponentReleaseV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	value := assemblercontract.CloneComponentReleaseV1(store.value)
	if store.drift {
		value.ReleaseID += "-drift"
	}
	if value.RefV1() != ref && !store.drift {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "release ref drift")
	}
	return value, nil
}

func TestBuildReferenceOnlyCandidateAndConformance(t *testing.T) {
	builder := mustBuilderV1(t, fixedClockV1{testNowV1})
	candidate, err := builder.BuildV1(requestV1(1))
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Release.SupportMode != assemblercontract.SupportReferenceOnlyV1 || candidate.Readiness.ProductionEligible {
		t.Fatalf("candidate self-promoted: %+v", candidate.Readiness)
	}
	if len(candidate.Release.ModuleDescriptors) != 1 || len(candidate.Release.CapabilityDescriptors) != 1 || len(candidate.Release.PortSpecs) != 1 || len(candidate.Release.FactoryDescriptors) != 1 {
		t.Fatal("descriptor closure incomplete")
	}
	if candidate.Release.FactoryDescriptors[0].FactoryID != FactoryIDV1 || candidate.Release.FactoryDescriptors[0].OutputCapability != CapabilityV1 {
		t.Fatal("factory identity drift")
	}
	report, err := InspectConformanceV1(candidate, testNowV1)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ReleaseValid || !report.DescriptorClosureValid || !report.ReadinessValid || report.ProductionClaimEligible || !reflect.DeepEqual(report.MissingProductionProofs, RequiredProductionProofsV1()) {
		t.Fatalf("report=%+v", report)
	}
}

func TestCandidateFailClosedOnProductionClaimProofAndDescriptorDrift(t *testing.T) {
	candidate, err := mustBuilderV1(t, fixedClockV1{testNowV1}).BuildV1(requestV1(1))
	if err != nil {
		t.Fatal(err)
	}
	production := candidate
	production.Readiness.ProductionEligible = true
	production.Readiness.Digest, _ = ReadinessDigestV1(production.Readiness)
	if err := production.ValidateCurrentV1(testNowV1); !core.HasReason(err, core.ReasonBindingNotCertified) {
		t.Fatalf("production claim error=%v", err)
	}
	proofDrift := candidate
	proofDrift.Readiness.MissingProductionProofs = proofDrift.Readiness.MissingProductionProofs[1:]
	proofDrift.Readiness.Digest, _ = ReadinessDigestV1(proofDrift.Readiness)
	if err := proofDrift.ValidateCurrentV1(testNowV1); !core.HasReason(err, core.ReasonBindingNotCertified) {
		t.Fatalf("proof drift error=%v", err)
	}
	descriptorDrift := candidate
	descriptorDrift.Release.FactoryDescriptors[0].FactoryID = "factory/model-invoker/drift"
	descriptorDrift.Release, _ = assemblercontract.SealComponentReleaseV1(descriptorDrift.Release)
	if _, err := InspectConformanceV1(descriptorDrift, testNowV1); err == nil {
		t.Fatal("descriptor drift accepted")
	}
}

func TestBuilderRejectsTTLClockRollbackDuplicateEvidenceAndTypedNil(t *testing.T) {
	request := requestV1(1)
	request.TTL = time.Millisecond
	if _, err := mustBuilderV1(t, fixedClockV1{testNowV1}).BuildV1(request); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("TTL error=%v", err)
	}
	request = requestV1(1)
	request.EvidenceRefs = append(request.EvidenceRefs, request.EvidenceRefs[0])
	if _, err := mustBuilderV1(t, fixedClockV1{testNowV1}).BuildV1(request); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("duplicate evidence error=%v", err)
	}
	rollback := &sequenceClockV1{values: []time.Time{testNowV1, testNowV1.Add(-time.Second)}}
	if _, err := mustBuilderV1(t, rollback).BuildV1(requestV1(1)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("rollback error=%v", err)
	}
	var typedNil *sequenceClockV1
	if _, err := NewBuilderV1(typedNil); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed nil error=%v", err)
	}
}

func TestCandidateTTLAndReadinessDigestFailClosed(t *testing.T) {
	candidate, err := mustBuilderV1(t, fixedClockV1{testNowV1}).BuildV1(requestV1(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := candidate.ValidateCurrentV1(testNowV1.Add(time.Hour)); !core.HasReason(err, core.ReasonCapabilityExpired) {
		t.Fatalf("expiry error=%v", err)
	}
	drift := candidate
	drift.Readiness.CheckedUnixNano++
	if err := drift.ValidateCurrentV1(testNowV1.Add(time.Second)); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("digest drift error=%v", err)
	}
}

func TestPublisherRecoversLostReplyByExactReadAndRejectsDrift(t *testing.T) {
	builder := mustBuilderV1(t, fixedClockV1{testNowV1})
	store := &releaseStoreV1{lose: true}
	publisher, err := NewPublisherV1(builder, store, store)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := publisher.PublishV1(context.Background(), requestV1(1))
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Release.RefV1() != store.value.RefV1() {
		t.Fatal("lost reply recovered another release")
	}
	store = &releaseStoreV1{lose: true, drift: true}
	publisher, _ = NewPublisherV1(builder, store, store)
	if _, err := publisher.PublishV1(context.Background(), requestV1(1)); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("drift error=%v", err)
	}
	if _, err := publisher.PublishV1(nil, requestV1(1)); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
}

func TestBuildDeterministicAt64Concurrency(t *testing.T) {
	builder := mustBuilderV1(t, fixedClockV1{testNowV1})
	want, err := builder.BuildV1(requestV1(1))
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 64)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := builder.BuildV1(requestV1(1))
			if err != nil {
				errs <- err
				return
			}
			if !reflect.DeepEqual(got, want) {
				errs <- core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "nondeterministic candidate")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func mustBuilderV1(t *testing.T, clock ClockV1) *BuilderV1 {
	t.Helper()
	builder, err := NewBuilderV1(clock)
	if err != nil {
		t.Fatal(err)
	}
	return builder
}
func requestV1(revision core.Revision) RequestV1 {
	return RequestV1{ReleaseID: "model-invoker-release", Revision: revision, ArtifactDigest: core.DigestBytes([]byte("model-invoker-artifact")), SourceRef: refV1("source"), PublisherRef: refV1("publisher"), TrustRef: refV1("trust"), EvidenceRefs: []assemblycontract.ObjectRefV1{refV1("prepared-contract"), refV1("commit-gate-contract"), refV1("harness-bridge-contract")}, TTL: time.Hour}
}
func refV1(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: "model-invoker/" + id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
}
