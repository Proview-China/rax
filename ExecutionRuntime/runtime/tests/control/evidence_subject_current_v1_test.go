package control_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestEvidenceSubjectCurrentV1BuilderProducesExactOneWayBundle(t *testing.T) {
	now := time.Unix(2_200_000_000, 0)
	request, projection := evidenceSubjectBundleInputV1(t)
	commit, sealed, index, err := control.NewEvidenceSubjectMutationBundleV1(request, projection, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := commit.Validate(); err != nil {
		t.Fatal(err)
	}
	if sealed.Ref != index.CurrentProjection || commit.NewProjection != sealed.Ref || commit.NewIndex != index {
		t.Fatalf("bundle relationships drifted: %+v %+v %+v", commit, sealed.Ref, index)
	}
	if sealed.Ref.Revision != 1 || index.Revision != 1 || sealed.PreviousProjection != nil || index.PreviousProjection != nil {
		t.Fatalf("first publish shape drifted: %+v %+v", sealed, index)
	}
	bad := projection
	bad.Ref.OwnerWatermark = 0
	if _, _, _, err = control.NewEvidenceSubjectMutationBundleV1(request, bad, now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("missing owner watermark must reject: %v", err)
	}
}

func evidenceSubjectBundleInputV1(t *testing.T) (ports.EvidenceSubjectMutationRequestV1, ports.EvidenceSubjectCurrentProjectionV1) {
	t.Helper()
	subject := ports.EvidenceSubjectKeyV1{Record: ports.EvidenceRecordRefV2{LedgerScopeDigest: subjectDigestV1("11"), Sequence: 1, RecordDigest: subjectDigestV1("12")}, Source: ports.EvidenceSourceKeyV2{RegistrationID: "registration-subject", SourceEpoch: 1, SourceSequence: 1}}
	subjectDigest, err := ports.DigestEvidenceSubjectKeyV1(subject)
	if err != nil {
		t.Fatal(err)
	}
	absence, err := ports.SealEvidenceTombstoneAbsenceRefV1(ports.EvidenceTombstoneAbsenceRefV1{SubjectKeyDigest: subjectDigest, Revision: 1, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	registration := ports.EvidenceSourceRegistrationRefV1{RegistrationID: subject.Source.RegistrationID, Revision: 1, FactDigest: subjectDigestV1("13"), ConfigurationDigest: subjectDigestV1("14"), SourceID: "custom/source", SourceEpoch: 1}
	request, err := ports.SealEvidenceSubjectMutationRequestV1(ports.EvidenceSubjectMutationRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: subject, Kind: ports.EvidenceSubjectMutationSourceRegistrationAdvanceV1, Registration: &registration})
	if err != nil {
		t.Fatal(err)
	}
	projection := ports.EvidenceSubjectCurrentProjectionV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Ref: ports.EvidenceSubjectProjectionRefV1{OwnerWatermark: 1}, Subject: subject, SubjectKeyDigest: subjectDigest, Record: subject.Record, Source: subject.Source, CandidateDigest: subjectDigestV1("15"), Registration: registration, RegistrationState: ports.EvidenceSourceActive, RegistrationExpiresUnixNano: 30, Presence: ports.EvidenceTombstoneAbsentSealedV1, Readability: ports.EvidenceSubjectReadableV1, TombstoneAbsence: &absence, Causation: []ports.EvidenceCausationRefV2{}, CheckedUnixNano: 10, ExpiresUnixNano: 30}
	return request, projection
}

func subjectDigestV1(prefix string) core.Digest {
	value := prefix
	for len(value) < 64 {
		value += "0"
	}
	return core.Digest("sha256:" + value[:64])
}
