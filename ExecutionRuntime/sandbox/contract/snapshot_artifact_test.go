package contract

import (
	"reflect"
	"testing"
	"time"
)

func TestSnapshotArtifactStableIdentityHasNoCurrentOrTTLFields(t *testing.T) {
	t.Parallel()
	typeOf := reflect.TypeOf(SnapshotArtifactSubjectIdentityV2{})
	for _, forbidden := range []string{"Revision", "ExpiresUnixNano", "RequestedNotAfter", "Current"} {
		if _, found := typeOf.FieldByName(forbidden); found {
			t.Fatalf("stable subject identity exposes forbidden field %s", forbidden)
		}
	}
	identity, err := SealSnapshotArtifactSubjectIdentityV2(SnapshotArtifactSubjectIdentityV2{
		ArtifactAggregateID: "aggregate-1", TenantID: "tenant-1", DataDomain: "workspace",
		ReservationID: "reservation-1", SourceAttemptID: "attempt-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.ValidateShape(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotArtifactSubjectExactDigestBindsTTL(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	identity, err := SealSnapshotArtifactSubjectIdentityV2(SnapshotArtifactSubjectIdentityV2{
		ArtifactAggregateID: "aggregate-1", TenantID: "tenant-1", DataDomain: "workspace",
		ReservationID: "reservation-1", SourceAttemptID: "attempt-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	base := SnapshotArtifactSubjectRefV2{
		ArtifactAggregateID: identity.ArtifactAggregateID, Revision: 1, TenantID: identity.TenantID,
		DataDomain: identity.DataDomain, ReservationID: identity.ReservationID, SourceAttemptID: identity.SourceAttemptID,
		SchemaRef: snapshotArtifactTestRef("subject-schema"), StableSubjectDigest: identity.StableSubjectDigest,
		ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	}
	first, err := SealSnapshotArtifactSubjectRefV2(base)
	if err != nil {
		t.Fatal(err)
	}
	base.ExpiresUnixNano = now.Add(2 * time.Hour).UnixNano()
	second, err := SealSnapshotArtifactSubjectRefV2(base)
	if err != nil {
		t.Fatal(err)
	}
	if first.SubjectDigest == second.SubjectDigest {
		t.Fatal("different legal TTLs produced the same subject digest")
	}
	tampered := first
	tampered.ExpiresUnixNano++
	if err := tampered.ValidateShape(); err == nil {
		t.Fatal("tampered subject TTL reused an old digest")
	}
	if err := first.ValidateCurrent(time.Unix(0, first.ExpiresUnixNano)); err == nil {
		t.Fatal("now == expires authorized subject current")
	}
}

func TestSnapshotArtifactReserveShapeRequiresExplicitAbsent(t *testing.T) {
	t.Parallel()
	request := snapshotArtifactTestRequest()
	if err := request.ValidateShape(); err != nil {
		t.Fatal(err)
	}
	request.ExpectedAggregateRef = SnapshotArtifactOptionalAggregateRefV2{Presence: SnapshotArtifactPresent}
	if err := request.ValidateShape(); err == nil {
		t.Fatal("typed-nil present expected aggregate passed validation")
	}
	request = snapshotArtifactTestRequest()
	request.ExpectedAggregateRef.Ref = &SnapshotArtifactAggregateRefV2{}
	if err := request.ValidateShape(); err == nil {
		t.Fatal("absent expected aggregate carried a value")
	}
}

func snapshotArtifactTestRequest() ReserveArtifactRequestV2 {
	now := time.Unix(1_700_000_000, 0)
	return ReserveArtifactRequestV2{
		TenantID: "tenant-1", DataDomain: "workspace", SourceOperationID: "operation-1", SourceEffectID: "effect-1",
		SourceAttemptRef: snapshotArtifactTestRef("attempt-1"), SchemaRef: snapshotArtifactTestRef("schema-1"),
		ExpectedContentDigest: snapshotArtifactTestRef("content-1").Digest,
		RetentionPolicyRef:    snapshotArtifactTestRef("retention-1"), EncryptionPolicyRef: snapshotArtifactTestRef("encryption-1"),
		ResidencyPolicyRef:   snapshotArtifactTestRef("residency-1"),
		ExpectedAggregateRef: SnapshotArtifactOptionalAggregateRefV2{Presence: SnapshotArtifactAbsent},
		RequestedNotAfter:    now.Add(time.Hour).UnixNano(),
	}
}

func snapshotArtifactTestRef(id string) Ref {
	digest, err := Digest("snapshot-artifact-test-ref", id)
	if err != nil {
		panic(err)
	}
	return Ref{ID: id, Revision: 1, Digest: digest}
}
