package kernel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

func TestWorkspaceRestoreBundleReaderV1ExactStableProjection(t *testing.T) {
	now := time.Unix(1_950_100_000, 0)
	bundle, fact, content := workspaceRestoreArtifactFixtureV1(t, now)
	artifacts := &workspaceRestoreArtifactReaderFakeV1{fact: fact}
	contentStore := &workspaceRestoreContentReaderFakeV1{ref: fact.StorageArtifactRef, content: content}
	reader, err := NewWorkspaceRestoreBundleReaderV1(artifacts, contentStore, func() time.Time { return now }, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	request := workspaceRestoreReaderRequestV1(fact, now)
	first, err := reader.InspectWorkspaceRestoreBundleCurrentV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reader.InspectWorkspaceRestoreBundleCurrentV1(context.Background(), request)
	if err != nil || second.ProjectionDigest != first.ProjectionDigest || second.CheckedUnixNano != first.CheckedUnixNano || second.ExpiresUnixNano != first.ExpiresUnixNano || second.Bundle.BundleDigest != bundle.BundleDigest {
		t.Fatalf("unstable exact projection first=%#v second=%#v err=%v", first, second, err)
	}
	contentStore.content[0] ^= 0xff
	if _, err := reader.InspectWorkspaceRestoreBundleCurrentV1(context.Background(), request); err == nil {
		t.Fatal("tampered content was accepted")
	}
}

func TestWorkspaceRestoreBundleReaderV1RejectsArtifactSpliceBeforeContent(t *testing.T) {
	now := time.Unix(1_950_100_000, 0)
	_, fact, content := workspaceRestoreArtifactFixtureV1(t, now)
	artifacts := &workspaceRestoreArtifactReaderFakeV1{fact: fact}
	contentStore := &workspaceRestoreContentReaderFakeV1{ref: fact.StorageArtifactRef, content: content}
	reader, _ := NewWorkspaceRestoreBundleReaderV1(artifacts, contentStore, func() time.Time { return now }, 30*time.Minute)
	request := workspaceRestoreReaderRequestV1(fact, now)
	request.TenantID = "other-tenant"
	request.Target.TenantID = "other-tenant"
	if _, err := reader.InspectWorkspaceRestoreBundleCurrentV1(context.Background(), request); err == nil {
		t.Fatal("cross-tenant artifact splice was accepted")
	}
	if contentStore.calls != 0 {
		t.Fatalf("content was read before artifact owner closure: calls=%d", contentStore.calls)
	}
}

type workspaceRestoreArtifactReaderFakeV1 struct {
	fact contract.SnapshotArtifactFactV2
}

func (f *workspaceRestoreArtifactReaderFakeV1) InspectArtifactFact(context.Context, *contract.InspectSnapshotArtifactFactRequestV2) (contract.SnapshotArtifactFactV2, error) {
	return f.fact, nil
}
func (*workspaceRestoreArtifactReaderFakeV1) ReserveArtifact(context.Context, *contract.ReserveArtifactRequestV2) (contract.ReserveArtifactResultV2, error) {
	return contract.ReserveArtifactResultV2{}, errors.New("unsupported")
}
func (*workspaceRestoreArtifactReaderFakeV1) CommitArtifact(context.Context, *contract.CommitSnapshotArtifactRequestV2) (contract.CommitSnapshotArtifactResultV2, error) {
	return contract.CommitSnapshotArtifactResultV2{}, errors.New("unsupported")
}
func (*workspaceRestoreArtifactReaderFakeV1) InspectReservation(context.Context, *contract.InspectSnapshotArtifactReservationRequestV2) (contract.SnapshotArtifactReservationV2, error) {
	return contract.SnapshotArtifactReservationV2{}, errors.New("unsupported")
}
func (*workspaceRestoreArtifactReaderFakeV1) InspectReservationByStableKey(context.Context, *contract.InspectSnapshotArtifactReservationByStableKeyRequestV2) (contract.SnapshotArtifactReservationV2, error) {
	return contract.SnapshotArtifactReservationV2{}, errors.New("unsupported")
}
func (*workspaceRestoreArtifactReaderFakeV1) InspectAggregateHistorical(context.Context, *contract.InspectSnapshotArtifactAggregateHistoricalRequestV2) (contract.SnapshotArtifactAggregateEnvelopeV2, error) {
	return contract.SnapshotArtifactAggregateEnvelopeV2{}, errors.New("unsupported")
}
func (*workspaceRestoreArtifactReaderFakeV1) InspectAggregateCurrent(context.Context, *contract.InspectSnapshotArtifactAggregateCurrentRequestV2) (contract.SnapshotArtifactAggregateCurrentProjectionV2, error) {
	return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, errors.New("unsupported")
}
func (*workspaceRestoreArtifactReaderFakeV1) InspectEntryHistorical(context.Context, *contract.InspectSnapshotArtifactEntryHistoricalRequestV2) (contract.SnapshotArtifactAggregateEntryV2, error) {
	return contract.SnapshotArtifactAggregateEntryV2{}, errors.New("unsupported")
}

type workspaceRestoreContentReaderFakeV1 struct {
	ref     contract.SnapshotStorageArtifactRefV2
	content []byte
	calls   int
}

func (*workspaceRestoreContentReaderFakeV1) PutSnapshotContentV2(context.Context, *contract.PutSnapshotContentRequestV2) (contract.PutSnapshotContentResultV2, error) {
	return contract.PutSnapshotContentResultV2{}, errors.New("unsupported")
}
func (f *workspaceRestoreContentReaderFakeV1) InspectSnapshotContentV2(context.Context, *contract.InspectSnapshotContentRequestV2) (contract.InspectSnapshotContentResultV2, error) {
	f.calls++
	return contract.InspectSnapshotContentResultV2{StorageRef: f.ref, Content: append([]byte(nil), f.content...)}, nil
}

func workspaceRestoreArtifactFixtureV1(t *testing.T, now time.Time) (contract.WorkspaceSnapshotBundleV1, contract.SnapshotArtifactFactV2, []byte) {
	t.Helper()
	bundle, err := contract.SealWorkspaceSnapshotBundleV1(contract.WorkspaceSnapshotBundleV1{SnapshotID: "snapshot", TenantID: "tenant-1", SourceScopeDigest: strings.Repeat("a", contract.DigestSizeHex), Entries: []contract.WorkspaceSnapshotEntryV1{{Path: "file", Kind: contract.WorkspaceSnapshotRegularFile, Content: []byte("payload")}}})
	if err != nil {
		t.Fatal(err)
	}
	content, _ := contract.EncodeWorkspaceSnapshotBundleV1(bundle)
	raw := sha256.Sum256(content)
	ref := func(id string) contract.Ref {
		digest, _ := contract.Digest("workspace-restore-reader-test-ref-v1", id)
		return contract.Ref{ID: id, Revision: 1, Digest: digest}
	}
	exact := func(typeURL, domain, id string, expires time.Time) contract.SnapshotArtifactExactRefV2 {
		digest, _ := contract.Digest("workspace-restore-reader-test-exact-v1", id)
		return contract.SnapshotArtifactExactRefV2{TypeURL: typeURL, Version: 1, ID: id, Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: digest, ExpiresUnixNano: expires.UnixNano()}
	}
	storage, err := contract.SealSnapshotStorageArtifactRefV2(contract.SnapshotStorageArtifactRefV2{StorageArtifactID: "storage", Revision: 1, TenantID: "tenant-1", DataDomain: "workspace-checkpoint", StorageNamespaceExactRef: exact("praxis.sandbox/storage-namespace/v1", "namespace", "namespace", now.Add(2*time.Hour)), ContentDigest: hex.EncodeToString(raw[:]), SchemaRef: ref("schema"), Length: uint64(len(content)), EncryptionFactRef: ref("encryption"), ResidencyFactRef: ref("residency"), CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := contract.SealSnapshotArtifactSubjectIdentityV2(contract.SnapshotArtifactSubjectIdentityV2{ArtifactAggregateID: "aggregate", TenantID: "tenant-1", DataDomain: "workspace-checkpoint", ReservationID: "reservation", SourceAttemptID: "capture-attempt"})
	subject, _ := contract.SealSnapshotArtifactSubjectRefV2(contract.SnapshotArtifactSubjectRefV2{ArtifactAggregateID: "aggregate", Revision: 1, TenantID: "tenant-1", DataDomain: "workspace-checkpoint", ReservationID: "reservation", SourceAttemptID: "capture-attempt", SchemaRef: ref("subject-schema"), StableSubjectDigest: identity.StableSubjectDigest, ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	fact, err := contract.SealSnapshotArtifactFactV2(contract.SnapshotArtifactFactV2{Meta: contract.Meta{ContractVersion: contract.ContractFamily, ID: "artifact-fact", Revision: 1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}, TenantID: "tenant-1", DataDomain: "workspace-checkpoint", ReservationFactRef: exact(contract.SnapshotArtifactReservationFactTypeURL, contract.SnapshotArtifactReservationFactDomain, "reservation-fact", now.Add(time.Hour)), ArtifactSubjectRef: subject, StorageArtifactRef: storage, SchemaRef: storage.SchemaRef, ContentDigest: storage.ContentDigest, Length: storage.Length, EncryptionFactRef: storage.EncryptionFactRef, ResidencyFactRef: storage.ResidencyFactRef, ProviderObservationRef: ref("observation"), ProviderReceiptRef: ref("receipt"), FormalEvidenceRefs: []contract.Ref{ref("evidence")}, OwnerInspectionRef: ref("inspection"), SourceAttemptRef: ref("capture-attempt"), RequestedNotAfter: now.Add(time.Hour).UnixNano(), State: contract.SnapshotArtifactAvailable})
	if err != nil {
		t.Fatal(err)
	}
	return bundle, fact, content
}

func workspaceRestoreReaderRequestV1(fact contract.SnapshotArtifactFactV2, now time.Time) contract.WorkspaceRestoreStageRequestV1 {
	exact := func(typeURL, domain, id string) contract.SnapshotArtifactExactRefV2 {
		digest, _ := contract.Digest("workspace-restore-reader-request-ref-v1", id)
		return contract.SnapshotArtifactExactRefV2{TypeURL: typeURL, Version: 1, ID: id, Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: digest, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	}
	return contract.WorkspaceRestoreStageRequestV1{TenantID: "tenant-1", DispatchAttemptID: "dispatch-attempt", RuntimeRestoreAttempt: exact("praxis.runtime/restore-attempt/v2", "attempt", "attempt"), RestoreEligibility: exact("praxis.runtime/restore-eligibility/v2", "eligibility", "eligibility"), Target: contract.RuntimeLeaseBinding{TenantID: "tenant-1", InstanceID: "new", InstanceEpoch: 2, LeaseID: "lease", LeaseEpoch: 2, FenceEpoch: 2, ScopeDigest: strings.Repeat("b", contract.DigestSizeHex), ObservedRevision: 1, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}, SnapshotArtifactFactRef: fact.ExactRef(), RequestedNotAfter: now.Add(time.Hour).UnixNano()}
}
