package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestContentIntegrityAuditFactCanonicalAndTamperClosed(t *testing.T) {
	fact := testkit.ContentIntegrityAuditFactV1(contract.ContentIntegrityMetadataAbsent, "object_metadata_absent", time.Date(2026, 7, 17, 17, 0, 0, 0, time.UTC))
	if err := fact.Validate(); err != nil {
		t.Fatal(err)
	}
	copy := fact.Clone()
	copy.Findings[0].DetailCode = "caller_claimed_healthy"
	if err := copy.Validate(); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("tampered audit must fail canonical validation, got %v", err)
	}
}

func TestContentIntegritySubjectsNormalizeAndRejectDuplicates(t *testing.T) {
	values := []contract.ContentIntegritySubjectV1{
		{ObjectID: "object-2", JournalID: "journal-2"},
		{ObjectID: "object-1", JournalID: "journal-1"},
	}
	normalized, err := contract.NormalizeContentIntegritySubjectsV1(values)
	if err != nil || normalized[0].ObjectID != "object-1" {
		t.Fatalf("normalize = (%#v,%v)", normalized, err)
	}
	values = append(values, values[0])
	if _, err := contract.NormalizeContentIntegritySubjectsV1(values); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("duplicate coordinates accepted: %v", err)
	}
}
