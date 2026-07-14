package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestEvidenceLedgerV2LostReplyThenLaterAppendKeepsReplayRecoverable(t *testing.T) {
	now := time.Unix(90_000, 0)
	store := fakes.NewEvidenceLedgerStoreV2(func() time.Time { return now })
	source := evidenceSourceV2(t, now, "source-registration-a", "custom/source-a", 1)
	if _, err := store.CreateSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	first := evidenceCandidateV2(t, source, "event-a", 1, "correlation-a")
	store.LoseNextAppendReply()
	if _, err := store.Append(context.Background(), ports.EvidenceAppendRequestV2{Candidate: first, ExpectedSourceRevision: 1}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("append reply must be lost after commit: %v", err)
	}
	persisted, err := store.InspectBySource(context.Background(), ports.EvidenceSourceKeyV2{RegistrationID: source.ID, SourceEpoch: source.SourceEpoch, SourceSequence: 1})
	if err != nil || persisted.Ref.Sequence != 1 {
		t.Fatalf("lost append must be inspectable: %v %+v", err, persisted)
	}
	current, err := store.InspectSource(context.Background(), source.ID)
	if err != nil || current.Revision != 2 || current.NextSourceSequence != 2 {
		t.Fatalf("cursor and record must be atomically visible: %v %+v", err, current)
	}
	second := evidenceCandidateV2(t, current, "event-b", 2, "correlation-a")
	if _, err = store.Append(context.Background(), ports.EvidenceAppendRequestV2{Candidate: second, ExpectedSourceRevision: 2}); err != nil {
		t.Fatal(err)
	}
	replayed, err := store.Append(context.Background(), ports.EvidenceAppendRequestV2{Candidate: first, ExpectedSourceRevision: 1})
	if err != nil || replayed.Ref != persisted.Ref {
		t.Fatalf("old replay after later append must return original record: %v %+v", err, replayed)
	}
	changed := first
	changed.CorrelationID = "correlation-changed"
	if _, err = store.Append(context.Background(), ports.EvidenceAppendRequestV2{Candidate: changed, ExpectedSourceRevision: 1}); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("same source key with changed full content must conflict: %v", err)
	}
}

func TestEvidenceLedgerV2StrictGapEventConflictAndWatchChain(t *testing.T) {
	now := time.Unix(91_000, 0)
	store := fakes.NewEvidenceLedgerStoreV2(func() time.Time { return now })
	source := evidenceSourceV2(t, now, "source-registration-gap", "custom/source-gap", 1)
	if _, err := store.CreateSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	gap := evidenceCandidateV2(t, source, "event-gap", 2, "correlation-gap")
	if _, err := store.Append(context.Background(), ports.EvidenceAppendRequestV2{Candidate: gap, ExpectedSourceRevision: 1}); !core.HasReason(err, core.ReasonEvidenceSequenceGap) {
		t.Fatalf("strict gap must reject before ledger allocation: %v", err)
	}
	scopeDigest, _ := source.LedgerScope.DigestV2()
	page, err := store.Watch(context.Background(), ports.EvidenceWatchCursorV2{LedgerScopeDigest: scopeDigest}, 1)
	if err != nil || len(page.Records) != 0 {
		t.Fatalf("gap must not allocate ledger sequence: %v %+v", err, page)
	}
	first := evidenceCandidateV2(t, source, "event-shared", 1, "correlation-gap")
	record1, err := store.Append(context.Background(), ports.EvidenceAppendRequestV2{Candidate: first, ExpectedSourceRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	secondSource := evidenceSourceV2(t, now, "source-registration-other", "custom/source-other", 1)
	if _, err = store.CreateSource(context.Background(), secondSource); err != nil {
		t.Fatal(err)
	}
	duplicateEvent := evidenceCandidateV2(t, secondSource, "event-shared", 1, "correlation-gap")
	if _, err = store.Append(context.Background(), ports.EvidenceAppendRequestV2{Candidate: duplicateEvent, ExpectedSourceRevision: 1}); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("event id must be unique within ledger scope: %v", err)
	}
	second := evidenceCandidateV2(t, secondSource, "event-other", 1, "correlation-gap")
	record2, err := store.Append(context.Background(), ports.EvidenceAppendRequestV2{Candidate: second, ExpectedSourceRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if record2.Ref.Sequence != 2 || record2.PreviousRecordDigest != record1.Ref.RecordDigest {
		t.Fatalf("ledger chain must be continuous: %+v %+v", record1, record2)
	}
	page, err = store.Watch(context.Background(), ports.EvidenceWatchCursorV2{LedgerScopeDigest: scopeDigest}, 1)
	if err != nil || len(page.Records) != 1 || page.Next.AfterSequence != 1 {
		t.Fatalf("watch page must be bounded with next cursor: %v %+v", err, page)
	}
	page2, err := store.Watch(context.Background(), page.Next, 1)
	if err != nil || len(page2.Records) != 1 || page2.Records[0].Ref.Sequence != 2 {
		t.Fatalf("next cursor must continue chain: %v %+v", err, page2)
	}
}

func TestEvidenceLedgerV2ConcurrentSourcesAndSourceEpochSingleOwner(t *testing.T) {
	now := time.Unix(92_000, 0)
	store := fakes.NewEvidenceLedgerStoreV2(func() time.Time { return now })
	left := evidenceSourceV2(t, now, "registration-left", "custom/shared-source", 1)
	right := left
	right.ID = "registration-right"
	var successes int
	var mu sync.Mutex
	var wait sync.WaitGroup
	for _, source := range []ports.EvidenceSourceRegistrationFactV2{left, right} {
		source := source
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := store.CreateSource(context.Background(), source); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			} else if !core.HasReason(err, core.ReasonEvidenceConflict) {
				t.Errorf("unexpected source create error: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes != 1 {
		t.Fatalf("same source epoch must have one registration, got %d", successes)
	}
	// High source coordinates must not collide in the idempotency index.
	high := evidenceSourceV2(t, now, "registration-high", "custom/high-source", core.Epoch(^uint64(0)-1))
	if _, err := store.CreateSource(context.Background(), high); err != nil {
		t.Fatal(err)
	}
	candidate := evidenceCandidateV2(t, high, "event-high", 1, "correlation-high")
	if _, err := store.Append(context.Background(), ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectBySource(context.Background(), ports.EvidenceSourceKeyV2{RegistrationID: high.ID, SourceEpoch: high.SourceEpoch, SourceSequence: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceLedgerV2TombstoneIsSeparateAndRecoverable(t *testing.T) {
	now := time.Unix(93_000, 0)
	store := fakes.NewEvidenceLedgerStoreV2(func() time.Time { return now })
	source := evidenceSourceV2(t, now, "registration-tombstone", "custom/source-tombstone", 1)
	if _, err := store.CreateSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	candidate := evidenceCandidateV2(t, source, "event-tombstone", 1, "correlation-tombstone")
	record, err := store.Append(context.Background(), ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	fact := ports.EvidenceTombstoneFactV2{Record: record.Ref, Source: ports.EvidenceSourceKeyV2{RegistrationID: source.ID, SourceEpoch: source.SourceEpoch, SourceSequence: 1}, Causation: []ports.EvidenceCausationRefV2{}, Reason: "runtime/retention", Revision: 1, CreatedUnixNano: now.UnixNano()}
	store.LoseNextTombstoneReply()
	if _, err = store.CreateTombstone(context.Background(), fact); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("lost tombstone reply expected: %v", err)
	}
	inspected, err := store.InspectTombstone(context.Background(), record.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if replayed, err := store.CreateTombstone(context.Background(), fact); err != nil || func() bool { left, _ := replayed.DigestV2(); right, _ := inspected.DigestV2(); return left != right }() {
		t.Fatalf("same tombstone replay must be idempotent: %v %+v", err, replayed)
	}
	original, err := store.InspectRecord(context.Background(), record.Ref)
	if err != nil || original.Ref.RecordDigest != record.Ref.RecordDigest {
		t.Fatalf("tombstone must not mutate ledger chain: %v %+v", err, original)
	}
}

func evidenceSourceV2(t *testing.T, now time.Time, id string, sourceID ports.NamespacedNameV2, epoch core.Epoch) (fact ports.EvidenceSourceRegistrationFactV2) {
	t.Helper()
	defer func() { fact.CurrentScopeWatermark = 1 }()
	scope := evidenceScopeV2(t)
	return ports.EvidenceSourceRegistrationFactV2{ContractVersion: ports.EvidenceContractVersionV2, ID: id, Revision: 1, SourceID: sourceID, SourceEpoch: epoch, LedgerScope: ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionInstance, TenantID: scope.Identity.TenantID, IdentityID: scope.Identity.ID, LineageID: scope.Lineage.ID, InstanceID: scope.Instance.ID}, ExecutionScope: scope, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "current-scope", Digest: evidenceDigestV2(t, "current-scope"), Revision: 1}, Producer: ports.EvidenceProducerBindingRefV2{BindingSetID: "binding-set", BindingSetRevision: 1, ComponentID: "custom/evidence-source", ManifestDigest: evidenceDigestV2(t, "manifest"), ArtifactDigest: evidenceDigestV2(t, "artifact"), Capability: "runtime/evidence-append"}, Authority: ports.AuthorityBindingRefV2{Ref: "authority", Digest: evidenceDigestV2(t, "authority"), Revision: 1, Epoch: scope.AuthorityEpoch}, ActionScopeDigest: evidenceDigestV2(t, "action-scope"), Policy: ports.EvidenceSourcePolicyBindingRefV2{Ref: "source-policy", Digest: evidenceDigestV2(t, "source-policy"), Revision: 1}, ClassMappings: []ports.EvidenceClassMappingV2{{Class: "custom/observation", Trust: ports.EvidenceTrustObservation}}, AllowedKinds: []ports.NamespacedNameV2{"custom/event"}, GapPolicy: ports.EvidenceGapStrictV2, NextSourceSequence: 1, State: ports.EvidenceSourceActive, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
}

func evidenceCandidateV2(t *testing.T, source ports.EvidenceSourceRegistrationFactV2, event string, sequence uint64, correlation string) ports.EvidenceEventCandidateV2 {
	t.Helper()
	configuration, _ := source.ConfigurationDigestV2()
	payload := evidenceDigestV2(t, event+"-payload")
	schema := ports.SchemaRefV2{Namespace: "custom", Name: "evidence", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: evidenceDigestV2(t, "schema")}
	return ports.EvidenceEventCandidateV2{ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: source.LedgerScope, EventID: event, RegistrationID: source.ID, RegistrationRevision: source.Revision, SourceConfigurationDigest: configuration, SourcePolicy: source.Policy, SourceID: source.SourceID, SourceEpoch: source.SourceEpoch, SourceSequence: sequence, TrustClass: ports.EvidenceTrustObservation, EventKind: "custom/event", CustomClass: "custom/observation", ExecutionScope: source.ExecutionScope, Payload: ports.EvidencePayloadRefV2{Schema: schema, ContentDigest: payload, Revision: 1, Length: 1, Ref: "memory://" + event}, Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: correlation, Producer: source.Producer, Authority: source.Authority, ObservedUnixNano: source.CreatedUnixNano}
}

func evidenceScopeV2(t *testing.T) core.ExecutionScope {
	t.Helper()
	return core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-a", ID: "identity-a", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-a", PlanDigest: evidenceDigestV2(t, "plan")}, Instance: core.InstanceRef{ID: "instance-a", Epoch: 1}, AuthorityEpoch: 1}
}
func evidenceDigestV2(t *testing.T, value string) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

var _ = control.ValidateEvidenceAppendV2
