package owneradapter

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
	hostsqlite "github.com/Proview-China/rax/ExecutionRuntime/agent-host/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblypublication"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestAssemblyPublicationAdapterV2StagesCommitsAndReturnsExactOwnerCurrent(t *testing.T) {
	fixture := newPublicationAdapterFixtureV2(t, assemblypublication.NewMemoryStoreV2(), journal.NewMemoryHostJournalStoreV2())
	result, err := fixture.adapter.PublishAssemblyV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Recovered || result.OwnerCurrent.ID != fixture.request.Artifacts.ScopeRef || result.Generation != fixture.request.Artifacts.Compiled.GenerationRef || result.Graph != fixture.request.Artifacts.Compiled.Graph.GraphRef {
		t.Fatalf("unexpected publication result: %+v", result)
	}
	if err := result.ValidateAt(fixture.now); err != nil {
		t.Fatal(err)
	}
	journalValue, err := fixture.adapter.journal.InspectHostJournalV2(context.Background(), fixture.request.HostID, fixture.request.StartID)
	if err != nil || len(journalValue.Operations) != 5 {
		t.Fatalf("journal steps=%d err=%v", len(journalValue.Operations), err)
	}
	for _, operation := range journalValue.Operations {
		if operation.State != hostcontract.HostOperationResultRecordedV2 {
			t.Fatalf("unsettled publication operation: %+v", operation)
		}
	}
}

func TestAssemblyPublicationAdapterV2PartialStageNeverBecomesVisible(t *testing.T) {
	store := &publicationFaultStoreV2{OwnerStoreV2: assemblypublication.NewMemoryStoreV2(), failStage: "manifest"}
	fixture := newPublicationAdapterFixtureV2(t, store, journal.NewMemoryHostJournalStoreV2())
	if _, err := fixture.adapter.PublishAssemblyV2(context.Background(), fixture.request); err == nil {
		t.Fatal("partial publication unexpectedly succeeded")
	}
	bundle := publicationBundleForRequestV2(t, fixture.request)
	ref := assemblycontract.AssemblyPublicationRefV2{PublicationID: bundle.Publication.PublicationID, Revision: bundle.Publication.Revision, Digest: bundle.Publication.Digest}
	if _, err := store.InspectHistoricalPublicationV2(context.Background(), ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("partial history = %v", err)
	}
	if _, err := store.InspectCurrentPublicationV2(context.Background(), bundle.Publication.ScopeRef); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("partial current = %v", err)
	}
}

func TestAssemblyPublicationAdapterV2RejectsArtifactSpliceClockTTLAndStaleCAS(t *testing.T) {
	fixture := newPublicationAdapterFixtureV2(t, assemblypublication.NewMemoryStoreV2(), journal.NewMemoryHostJournalStoreV2())
	splice := fixture.request
	splice.Artifacts.Compiled.ManifestRef.Digest = hostcontract.DigestV1(core.DigestBytes([]byte("splice")))
	splice.Artifacts.Digest = ""
	sealed, err := hostcontract.SealCompiledAssemblyArtifactsV2(splice.Artifacts)
	if err == nil {
		splice.Artifacts = sealed
		_, err = fixture.adapter.PublishAssemblyV2(context.Background(), splice)
	}
	if err == nil {
		t.Fatal("object splice was accepted")
	}
	if _, err = fixture.adapter.PublishAssemblyV2(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	stale := fixture.request
	stale.AttemptID = "publication-stale"
	if _, err = fixture.adapter.PublishAssemblyV2(context.Background(), stale); err == nil {
		t.Fatal("stale predecessor replay succeeded")
	}
	clock := fixture.now.Add(-time.Nanosecond)
	rollback, _ := NewAssemblyPublicationAdapterV2(assemblypublication.NewMemoryStoreV2(), journal.NewMemoryHostJournalStoreV2(), fixture.owner, func() time.Time { return clock })
	if _, err = rollback.PublishAssemblyV2(context.Background(), fixture.request); err == nil {
		t.Fatal("clock rollback succeeded")
	}
	expired := fixture.request
	expired.RequestedExpiresUnixNano = fixture.now.UnixNano()
	if _, err = fixture.adapter.PublishAssemblyV2(context.Background(), expired); err == nil {
		t.Fatal("expired request succeeded")
	}
}

func TestAssemblyPublicationAdapterV2TTLPassingMidSequenceFailsBeforeNextWrite(t *testing.T) {
	store := &publicationFaultStoreV2{OwnerStoreV2: assemblypublication.NewMemoryStoreV2()}
	facts := journal.NewMemoryHostJournalStoreV2()
	now := assemblytestkit.Now.Add(time.Second)
	request := publicationRequestFixtureV2(t, now)
	request.RequestedExpiresUnixNano = now.Add(time.Second).UnixNano()
	createMemoryHostJournalV2(t, facts, request, now)
	clock := now
	store.afterGeneration = func() { clock = now.Add(2 * time.Second) }
	owner := core.OwnerRef{Domain: "praxis.harness", ID: "assembly-publication-owner"}
	adapter, err := NewAssemblyPublicationAdapterV2(store, facts, owner, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.PublishAssemblyV2(context.Background(), request); err == nil {
		t.Fatal("TTL crossing did not fail closed")
	}
	if store.manifestCalls.Load() != 0 || store.commitCalls.Load() != 0 {
		t.Fatalf("writes after expiry manifest=%d commit=%d", store.manifestCalls.Load(), store.commitCalls.Load())
	}
}

func TestAssemblyPublicationAdapterV2SixtyFourIndependentAdaptersDispatchEachStepOnce(t *testing.T) {
	store := &publicationFaultStoreV2{OwnerStoreV2: assemblypublication.NewMemoryStoreV2()}
	facts := journal.NewMemoryHostJournalStoreV2()
	fixture := newPublicationAdapterFixtureV2(t, store, facts)
	start := make(chan struct{})
	var wait sync.WaitGroup
	var successes atomic.Int64
	for index := 0; index < 64; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			adapter, err := NewAssemblyPublicationAdapterV2(store, facts, fixture.owner, func() time.Time { return fixture.now })
			if err != nil {
				return
			}
			<-start
			if _, err = adapter.PublishAssemblyV2(context.Background(), fixture.request); err == nil {
				successes.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()
	if successes.Load() == 0 {
		t.Fatal("no independent adapter completed the publication")
	}
	if store.generationCalls.Load() != 1 || store.manifestCalls.Load() != 1 || store.graphCalls.Load() != 1 || store.handoffCalls.Load() != 1 || store.commitCalls.Load() != 1 {
		t.Fatalf("dispatch counts generation=%d manifest=%d graph=%d handoff=%d commit=%d", store.generationCalls.Load(), store.manifestCalls.Load(), store.graphCalls.Load(), store.handoffCalls.Load(), store.commitCalls.Load())
	}
}

func TestAssemblyPublicationAdapterV2SQLiteCommitLostReplyRestartInspectsOnly(t *testing.T) {
	ctx := context.Background()
	now := assemblytestkit.Now.Add(time.Second)
	harnessPath := filepath.Join(t.TempDir(), "harness.db")
	hostPath := filepath.Join(t.TempDir(), "host.db")
	harnessStore, err := assemblypublication.OpenSQLiteStoreV2(ctx, assemblypublication.SQLiteStoreConfigV2{Path: harnessPath, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	owner := core.OwnerRef{Domain: "praxis.harness", ID: "assembly-publication-owner"}
	hostStore, err := hostsqlite.Open(ctx, hostsqlite.Config{Path: hostPath, Owner: owner, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	request := publicationRequestFixtureV2(t, now)
	createSQLiteHostJournalV2(t, hostStore, request, now)
	fault := &publicationFaultStoreV2{OwnerStoreV2: harnessStore, loseCommitReply: true, failCommitInspectOnce: true}
	adapter, _ := NewAssemblyPublicationAdapterV2(fault, hostStore, owner, func() time.Time { return now })
	if _, err = adapter.PublishAssemblyV2(ctx, request); err == nil {
		t.Fatal("lost commit reply plus unavailable Inspect unexpectedly succeeded")
	}
	if err = harnessStore.Close(); err != nil {
		t.Fatal(err)
	}
	if err = hostStore.Close(); err != nil {
		t.Fatal(err)
	}
	harnessStore, err = assemblypublication.OpenSQLiteStoreV2(ctx, assemblypublication.SQLiteStoreConfigV2{Path: harnessPath, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer harnessStore.Close()
	hostStore, err = hostsqlite.Open(ctx, hostsqlite.Config{Path: hostPath, Owner: owner, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer hostStore.Close()
	restarted, _ := NewAssemblyPublicationAdapterV2(harnessStore, hostStore, owner, func() time.Time { return now })
	result, err := restarted.PublishAssemblyV2(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Recovered {
		t.Fatal("restart did not report Inspect recovery")
	}
}

type publicationAdapterFixtureV2 struct {
	adapter *AssemblyPublicationAdapterV2
	request hostcontract.AssemblyPublicationRequestV2
	now     time.Time
	owner   core.OwnerRef
}

func newPublicationAdapterFixtureV2(t *testing.T, store assemblypublication.OwnerStoreV2, facts *journal.MemoryHostJournalStoreV2) publicationAdapterFixtureV2 {
	t.Helper()
	now := assemblytestkit.Now.Add(time.Second)
	request := publicationRequestFixtureV2(t, now)
	createMemoryHostJournalV2(t, facts, request, now)
	owner := core.OwnerRef{Domain: "praxis.harness", ID: "assembly-publication-owner"}
	adapter, err := NewAssemblyPublicationAdapterV2(store, facts, owner, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return publicationAdapterFixtureV2{adapter: adapter, request: request, now: now, owner: owner}
}

func publicationRequestFixtureV2(t *testing.T, now time.Time) hostcontract.AssemblyPublicationRequestV2 {
	t.Helper()
	input := assemblytestkit.ValidInput()
	compiled, err := assemblycompiler.New().Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	generation, manifest, graph, handoff := *compiled.Generation, *compiled.Manifest, *compiled.Graph, *compiled.Handoff
	hostGraph, err := constructionGraphV1(manifest, graph, generation)
	if err != nil {
		t.Fatal(err)
	}
	hostCompiled := hostcontract.CompiledAssemblyV1{GenerationRef: generationRefV1(generation), ManifestRef: artifactRefV1(ManifestKindV1, generation.GenerationID+"/manifest", uint64(generation.Revision), manifest.Digest), Graph: hostGraph, HandoffRef: artifactRefV1(HandoffKindV1, generation.GenerationID+"/handoff", uint64(generation.Revision), handoff.Digest)}
	artifacts, err := hostcontract.SealCompiledAssemblyArtifactsV2(hostcontract.CompiledAssemblyArtifactsV2{ScopeRef: input.ScopeRef, InputRef: inputRefV1(input), Compiled: hostCompiled, Harness: compiled, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return hostcontract.AssemblyPublicationRequestV2{ContractVersion: hostcontract.AssemblyPublicationAdapterContractVersionV2, HostID: "host-publication", StartID: "start-publication", AttemptID: "publication-attempt", Artifacts: artifacts, RequestedExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()}
}

func createMemoryHostJournalV2(t *testing.T, facts *journal.MemoryHostJournalStoreV2, request hostcontract.AssemblyPublicationRequestV2, now time.Time) {
	t.Helper()
	claim := hostcontract.ExactRefV1{Kind: "praxis.agent-host/host-start-claim", ID: "claim/publication", Revision: 1, Digest: hostDigestV2(t, "claim")}
	j, err := hostcontract.SealHostJournalV2(hostcontract.HostJournalV2{ContractVersion: hostcontract.HostJournalContractVersionV2, HostID: request.HostID, StartID: request.StartID, Revision: 1, Phase: hostcontract.HostCompilingV2, StartClaimRef: claim, ConfigDigest: hostDigestV2(t, "config"), CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = facts.CreateHostJournalV2(context.Background(), j); err != nil {
		t.Fatal(err)
	}
}

func createSQLiteHostJournalV2(t *testing.T, store *hostsqlite.Store, request hostcontract.AssemblyPublicationRequestV2, now time.Time) {
	t.Helper()
	definition := hostcontract.ExactRefV1{Kind: "praxis.agent-definition/definition", ID: "definition/publication", Revision: 1, Digest: hostDigestV2(t, "definition")}
	claim, err := hostcontract.SealHostStartClaimV1(hostcontract.HostStartClaimV1{ContractVersion: hostcontract.HostStartClaimContractVersionV1, HostContractVersion: hostcontract.ContractVersionV2, HostID: request.HostID, StartID: request.StartID, ConfigDigest: hostDigestV2(t, "config"), DefinitionSourceRef: definition, RequestedOperation: hostcontract.HostStartOperationStartV1, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	claimRef, _ := claim.RefV1()
	j, err := hostcontract.SealHostJournalV2(hostcontract.HostJournalV2{ContractVersion: hostcontract.HostJournalContractVersionV2, HostID: request.HostID, StartID: request.StartID, Revision: 1, Phase: hostcontract.HostCompilingV2, StartClaimRef: claimRef, ConfigDigest: claim.ConfigDigest, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateHostJournalV2(context.Background(), j); err != nil {
		t.Fatal(err)
	}
}

func publicationBundleForRequestV2(t *testing.T, request hostcontract.AssemblyPublicationRequestV2) assemblycontract.AssemblyPublicationBundleV2 {
	t.Helper()
	bundle, err := assemblycontract.NewAssemblyPublicationBundleV2(request.Artifacts.ScopeRef, request.Artifacts.Harness)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}
func hostDigestV2(t *testing.T, value string) hostcontract.DigestV1 {
	t.Helper()
	digest, err := hostcontract.DigestJSONV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

type publicationFaultStoreV2 struct {
	assemblypublication.OwnerStoreV2
	failStage                                                             string
	loseCommitReply                                                       bool
	failCommitInspectOnce                                                 bool
	afterGeneration                                                       func()
	generationCalls, manifestCalls, graphCalls, handoffCalls, commitCalls atomic.Int64
}

func (s *publicationFaultStoreV2) StageGenerationV2(c context.Context, id string, v assemblycontract.AssemblyGenerationV1) error {
	s.generationCalls.Add(1)
	if s.failStage == "generation" {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected")
	}
	err := s.OwnerStoreV2.StageGenerationV2(c, id, v)
	if err == nil && s.afterGeneration != nil {
		s.afterGeneration()
	}
	return err
}
func (s *publicationFaultStoreV2) StageManifestV2(c context.Context, id string, v assemblycontract.AssemblyManifestV1) error {
	s.manifestCalls.Add(1)
	if s.failStage == "manifest" {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected")
	}
	return s.OwnerStoreV2.StageManifestV2(c, id, v)
}
func (s *publicationFaultStoreV2) StageGraphV2(c context.Context, id string, v assemblycontract.CompiledHarnessGraphV1) error {
	s.graphCalls.Add(1)
	if s.failStage == "graph" {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected")
	}
	return s.OwnerStoreV2.StageGraphV2(c, id, v)
}
func (s *publicationFaultStoreV2) StageHandoffV2(c context.Context, id string, v assemblycontract.AssemblyHandoffV1) error {
	s.handoffCalls.Add(1)
	if s.failStage == "handoff" {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected")
	}
	return s.OwnerStoreV2.StageHandoffV2(c, id, v)
}
func (s *publicationFaultStoreV2) CommitPublicationCurrentV2(c context.Context, r assemblypublication.CommitPublicationCurrentRequestV2) (assemblycontract.AssemblyPublicationCurrentV2, error) {
	s.commitCalls.Add(1)
	v, err := s.OwnerStoreV2.CommitPublicationCurrentV2(c, r)
	if err == nil && s.loseCommitReply {
		s.loseCommitReply = false
		return assemblycontract.AssemblyPublicationCurrentV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected lost reply")
	}
	return v, err
}
func (s *publicationFaultStoreV2) InspectCommittedPublicationCurrentV2(c context.Context, r assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationCurrentV2, error) {
	if s.failCommitInspectOnce {
		s.failCommitInspectOnce = false
		return assemblycontract.AssemblyPublicationCurrentV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected inspect unavailable")
	}
	return s.OwnerStoreV2.InspectCommittedPublicationCurrentV2(c, r)
}
