package ports_test

import (
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestEvidenceSubjectCurrentV1GoldenAndOneWaySeal(t *testing.T) {
	subject := evidenceSubjectGoldenV1()
	subjectDigest, err := ports.DigestEvidenceSubjectKeyV1(subject)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSubjectLiteralV1(t, string(subjectDigest), "sha256:aee9a1e15b583f37788f293173261d4945e5ab0e3e0cce7980c59dca6de9c005")
	projectionID, err := ports.DeriveEvidenceSubjectProjectionIDV1(subjectDigest)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSubjectLiteralV1(t, projectionID, "sha256:6fe2f3efb4a395cc58f904f74ce45ba36c9597f5ce16621d9d6af224e4e37fe3")
	indexID, err := ports.DeriveEvidenceSubjectCurrentIndexIDV1(subjectDigest)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSubjectLiteralV1(t, indexID, "sha256:cddc693c7eda17f0380135add4dafd8b32078b8c51457654e9a4543ce2840b3a")

	request := ports.EvidenceSubjectMutationRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: subject, Kind: ports.EvidenceSubjectMutationTombstoneCreateV1, Tombstone: &ports.EvidenceTombstoneRefV1{Record: subject.Record, Source: subject.Source, Revision: 1, Digest: evidenceSubjectDigestV1("01")}}
	request, err = ports.SealEvidenceSubjectMutationRequestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ports.DeriveEvidenceSubjectMutationKeyV1(request)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSubjectLiteralV1(t, key.MutationID, "sha256:da80bf64a3ed35c17e24dad24424ac6ca7a3f2c48c8214b212a22efcba871864")
	if key.MutationID != string(key.StableKeyDigest) {
		t.Fatalf("mutation ID and stable key digest differ: %+v", key)
	}
}

func TestEvidenceSubjectCurrentV1ProjectionAndIndexRejectDerivedDrift(t *testing.T) {
	subject := evidenceSubjectGoldenV1()
	subjectDigest, _ := ports.DigestEvidenceSubjectKeyV1(subject)
	absence, err := ports.SealEvidenceTombstoneAbsenceRefV1(ports.EvidenceTombstoneAbsenceRefV1{SubjectKeyDigest: subjectDigest, Revision: 1, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ports.SealEvidenceSubjectCurrentProjectionV1(ports.EvidenceSubjectCurrentProjectionV1{
		Ref: ports.EvidenceSubjectProjectionRefV1{Revision: 1, OwnerWatermark: 1}, Subject: subject,
		SubjectKeyDigest: subjectDigest, Record: subject.Record, Source: subject.Source,
		Causation: []ports.EvidenceCausationRefV2{}, Presence: ports.EvidenceTombstoneAbsentSealedV1,
		Readability: ports.EvidenceSubjectReadableV1, TombstoneAbsence: &absence,
		CheckedUnixNano: 10, ExpiresUnixNano: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	index, err := ports.SealEvidenceSubjectCurrentIndexRefV1(ports.EvidenceSubjectCurrentIndexRefV1{Revision: 1, SubjectKeyDigest: subjectDigest, CurrentProjection: projection.Ref, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	if index.CurrentProjection != projection.Ref {
		t.Fatalf("Index did not bind exact Projection: %+v", index)
	}

	badProjection := projection
	badProjection.Ref.Digest = evidenceSubjectDigestV1("02")
	if _, err = ports.SealEvidenceSubjectCurrentProjectionV1(badProjection); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("wrong nonzero Projection digest must conflict: %v", err)
	}
	badIndex := index
	badIndex.IndexID = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if _, err = ports.SealEvidenceSubjectCurrentIndexRefV1(badIndex); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("wrong nonzero Index ID must conflict: %v", err)
	}
	changed := projection
	changed.CorrelationID = "changed"
	if err = changed.Validate(); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same Projection ref with changed body must conflict: %v", err)
	}
}

func TestEvidenceSubjectCurrentV1ClosedEnums(t *testing.T) {
	for _, value := range []ports.EvidenceSubjectMutationKindV1{ports.EvidenceSubjectMutationSourceRegistrationAdvanceV1, ports.EvidenceSubjectMutationSourcePolicyAdvanceV1, ports.EvidenceSubjectMutationTombstoneCreateV1, ports.EvidenceSubjectMutationReadabilityPolicyAdvanceV1} {
		if err := value.Validate(); err != nil {
			t.Fatalf("closed kind rejected: %q %v", value, err)
		}
	}
	if err := ports.EvidenceSubjectMutationKindV1("custom/advance").Validate(); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("custom mutation kind must reject: %v", err)
	}
	if err := ports.EvidenceTombstonePresenceV1("Present").Validate(); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("presence alias must reject: %v", err)
	}
}

func TestEvidenceSubjectCurrentV1FactOwnerDoesNotSatisfyPublicReader(t *testing.T) {
	owner := reflect.TypeOf((*ports.EvidenceSubjectCurrentFactPortV1)(nil)).Elem()
	reader := reflect.TypeOf((*ports.EvidenceSubjectCurrentReaderV1)(nil)).Elem()
	if owner.Implements(reader) {
		t.Fatal("raw Evidence subject Fact Owner must not satisfy the public current Reader")
	}
	for _, method := range []string{"InspectEvidenceSubjectProjectionV1", "InspectEvidenceSubjectCurrentV1", "ValidateEvidenceSubjectCurrentV1"} {
		if _, ok := owner.MethodByName(method); ok {
			t.Fatalf("raw Evidence subject Fact Owner leaked public Reader method %s", method)
		}
	}
}

func evidenceSubjectGoldenV1() ports.EvidenceSubjectKeyV1 {
	return ports.EvidenceSubjectKeyV1{
		Record: ports.EvidenceRecordRefV2{LedgerScopeDigest: core.Digest("sha256:0000000000000000000000000000000000000000000000000000000000000002"), Sequence: 7, RecordDigest: core.Digest("sha256:0000000000000000000000000000000000000000000000000000000000000003")},
		Source: ports.EvidenceSourceKeyV2{RegistrationID: "reg-a", SourceEpoch: 4, SourceSequence: 7},
	}
}

func evidenceSubjectDigestV1(suffix string) core.Digest {
	value := suffix
	for len(value) < 64 {
		value += "0"
	}
	return core.Digest("sha256:" + value[:64])
}

func assertEvidenceSubjectLiteralV1(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("golden drifted: got=%s want=%s", got, want)
	}
}
