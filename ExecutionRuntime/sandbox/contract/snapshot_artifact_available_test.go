package contract

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSnapshotStorageArtifactRefV2CanonicalAndOpaque(t *testing.T) {
	now := time.Unix(1_900_300_000, 0).UTC()
	value := snapshotStorageArtifactFixtureV2(t, now)
	if err := value.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"path", "bucket", "object_key", "credential", "mount_handle", "process"} {
		if strings.Contains(strings.ToLower(string(body)), forbidden) {
			t.Fatalf("storage artifact leaked backend locator %q: %s", forbidden, body)
		}
	}

	drifted := value
	drifted.Length++
	if err := drifted.ValidateShape(); err == nil {
		t.Fatal("storage artifact accepted length drift with the old digest")
	}
	drifted = value
	drifted.ExpiresUnixNano++
	if err := drifted.ValidateShape(); err == nil {
		t.Fatal("storage artifact accepted TTL drift with the old digest")
	}
	drifted = value
	drifted.DigestDomain = SnapshotArtifactFactDomain
	if err := drifted.ValidateShape(); err == nil {
		t.Fatal("storage artifact accepted a fact digest domain")
	}
	if err := value.ValidateCurrent(time.Unix(0, value.ExpiresUnixNano)); err == nil {
		t.Fatal("storage artifact remained current at its exact expiry")
	}
}

func TestSnapshotArtifactFactV2ExactBindingAndNoSuccessorBackReference(t *testing.T) {
	now := time.Unix(1_900_300_000, 0).UTC()
	storage := snapshotStorageArtifactFixtureV2(t, now)
	subject := snapshotSubjectRefFixtureV2(t, now)
	reservation := snapshotExactRefFixtureV2(SnapshotArtifactReservationFactTypeURL, SnapshotArtifactReservationFactDomain, "reservation-fact", now.Add(time.Hour))
	fact, err := SealSnapshotArtifactFactV2(SnapshotArtifactFactV2{
		Meta:     Meta{ContractVersion: ContractFamily, ID: "artifact-fact", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()},
		TenantID: "tenant-1", DataDomain: "workspace-checkpoint", ReservationFactRef: reservation,
		ArtifactSubjectRef: subject, StorageArtifactRef: storage, SchemaRef: storage.SchemaRef,
		ContentDigest: storage.ContentDigest, Length: storage.Length, EncryptionFactRef: storage.EncryptionFactRef,
		ResidencyFactRef: storage.ResidencyFactRef, ProviderObservationRef: snapshotRefFixtureV2("provider-observation"),
		ProviderReceiptRef: snapshotRefFixtureV2("provider-receipt"), FormalEvidenceRefs: []Ref{snapshotRefFixtureV2("evidence-1")},
		OwnerInspectionRef: snapshotRefFixtureV2("owner-inspection"), SourceAttemptRef: snapshotRefFixtureV2("source-attempt"),
		RequestedNotAfter: now.Add(time.Hour).UnixNano(), State: SnapshotArtifactAvailable,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fact.ExactRef().TypeURL != SnapshotArtifactFactTypeURL || fact.ExactRef().DigestDomain != SnapshotArtifactFactDomain {
		t.Fatalf("wrong fact exact ref: %#v", fact.ExactRef())
	}
	drifted := fact
	drifted.StorageArtifactRef.ContentDigest = strings.Repeat("a", DigestSizeHex)
	if err := drifted.ValidateShape(); err == nil {
		t.Fatal("artifact fact accepted storage/content drift")
	}
	drifted = fact
	drifted.FormalEvidenceRefs = append(drifted.FormalEvidenceRefs, snapshotRefFixtureV2("evidence-2"))
	if err := drifted.ValidateShape(); err == nil {
		t.Fatal("artifact fact accepted evidence drift with the old digest")
	}
	if err := fact.ValidateCurrent(time.Unix(0, fact.Meta.ExpiresUnixNano)); err == nil {
		t.Fatal("artifact fact remained current at its exact expiry")
	}
}

func snapshotStorageArtifactFixtureV2(t *testing.T, now time.Time) SnapshotStorageArtifactRefV2 {
	t.Helper()
	value, err := SealSnapshotStorageArtifactRefV2(SnapshotStorageArtifactRefV2{
		StorageArtifactID: "storage-artifact-1", Revision: 1, TenantID: "tenant-1", DataDomain: "workspace-checkpoint",
		StorageNamespaceExactRef: snapshotExactRefFixtureV2("praxis.sandbox/storage-namespace/v1", "praxis.sandbox/storage-namespace/body/v1", "namespace-1", now.Add(2*time.Hour)),
		ContentDigest:            strings.Repeat("1", DigestSizeHex), SchemaRef: snapshotRefFixtureV2("workspace-snapshot-schema"), Length: 4096,
		EncryptionFactRef: snapshotRefFixtureV2("encryption-fact"), ResidencyFactRef: snapshotRefFixtureV2("residency-fact"),
		CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func snapshotSubjectRefFixtureV2(t *testing.T, now time.Time) SnapshotArtifactSubjectRefV2 {
	t.Helper()
	identity, err := SealSnapshotArtifactSubjectIdentityV2(SnapshotArtifactSubjectIdentityV2{ArtifactAggregateID: "aggregate-1", TenantID: "tenant-1", DataDomain: "workspace-checkpoint", ReservationID: "reservation-1", SourceAttemptID: "source-attempt"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := SealSnapshotArtifactSubjectRefV2(SnapshotArtifactSubjectRefV2{ArtifactAggregateID: identity.ArtifactAggregateID, Revision: 1, TenantID: identity.TenantID, DataDomain: identity.DataDomain, ReservationID: identity.ReservationID, SourceAttemptID: identity.SourceAttemptID, SchemaRef: snapshotRefFixtureV2("subject-schema"), StableSubjectDigest: identity.StableSubjectDigest, ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func snapshotExactRefFixtureV2(typeURL, domain, id string, expires time.Time) SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: typeURL, Version: SnapshotArtifactVersion, ID: id, Revision: 1, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: strings.Repeat("2", DigestSizeHex), ExpiresUnixNano: expires.UnixNano()}
}

func snapshotRefFixtureV2(id string) Ref {
	digest, _ := Digest("snapshot-artifact-test-ref-v2", id)
	return Ref{ID: id, Revision: 1, Digest: digest}
}
