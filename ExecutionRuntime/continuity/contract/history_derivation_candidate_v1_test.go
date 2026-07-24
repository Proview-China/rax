package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestHistoryDerivationCandidateIsExactImmutableAndCandidateOnly(t *testing.T) {
	event := testkit.TimelineEvent(1, 1, contract.TrustObservation)
	ref := contract.HistoryDerivationEventRefFromRecordV1(event)
	output := testkit.ContentDeltaSourceV1(testkit.Scope()).Target
	fact, err := contract.NewHistoryDerivationCandidateFactV1(
		"derivation-1", "derivation-request-1", "request-digest-1", testkit.Scope(),
		testkit.HistoryDerivationOwnerV1(), contract.HistoryDerivationSummary,
		[]contract.HistoryDerivationEventRefV1{ref}, output,
		time.Date(2026, 7, 17, 18, 30, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if fact.Authority != contract.HistoryDerivationAuthorityV1 || fact.Revision != 1 {
		t.Fatalf("authority/revision = %q/%d", fact.Authority, fact.Revision)
	}
	for name, mutate := range map[string]func(*contract.HistoryDerivationCandidateFactV1){
		"event digest": func(value *contract.HistoryDerivationCandidateFactV1) {
			value.Sources[0].EvidenceRecordDigest = "changed"
		},
		"projection digest": func(value *contract.HistoryDerivationCandidateFactV1) { value.Sources[0].ProjectionDigest = "changed" },
		"output digest":     func(value *contract.HistoryDerivationCandidateFactV1) { value.Output.ContentDigest = "changed" },
		"authority":         func(value *contract.HistoryDerivationCandidateFactV1) { value.Authority = "authoritative" },
	} {
		t.Run(name, func(t *testing.T) {
			tampered := fact.Clone()
			mutate(&tampered)
			if err := tampered.Validate(); err == nil {
				t.Fatal("tampered candidate accepted")
			}
		})
	}
}

func TestHistoryDerivationCandidateSourceOrderIsExact(t *testing.T) {
	left := contract.HistoryDerivationEventRefFromRecordV1(testkit.TimelineEvent(1, 1, contract.TrustObservation))
	right := contract.HistoryDerivationEventRefFromRecordV1(testkit.TimelineEvent(2, 2, contract.TrustClaim))
	output := testkit.ContentDeltaSourceV1(testkit.Scope()).Target
	a, err := contract.NewHistoryDerivationCandidateFactV1("derivation-1", "request-1", "digest-1", testkit.Scope(), testkit.HistoryDerivationOwnerV1(), contract.HistoryDerivationIndex, []contract.HistoryDerivationEventRefV1{left, right}, output, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, err := contract.NewHistoryDerivationCandidateFactV1("derivation-1", "request-1", "digest-1", testkit.Scope(), testkit.HistoryDerivationOwnerV1(), contract.HistoryDerivationIndex, []contract.HistoryDerivationEventRefV1{right, left}, output, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if a.SourceSetDigest == b.SourceSetDigest {
		t.Fatal("ordered source set was normalized away")
	}
}

func FuzzHistoryDerivationCandidateRejectsCanonicalTamper(f *testing.F) {
	f.Add(uint8(0), "changed")
	f.Add(uint8(3), "authoritative")
	f.Add(uint8(5), "digest-drift")
	f.Fuzz(func(t *testing.T, selector uint8, replacement string) {
		event := testkit.TimelineEvent(1, 1, contract.TrustObservation)
		output := testkit.ContentDeltaSourceV1(testkit.Scope()).Target
		fact, err := contract.NewHistoryDerivationCandidateFactV1(
			"derivation-fuzz", "derivation-request-fuzz", "request-digest-fuzz", testkit.Scope(),
			testkit.HistoryDerivationOwnerV1(), contract.HistoryDerivationProjection,
			[]contract.HistoryDerivationEventRefV1{contract.HistoryDerivationEventRefFromRecordV1(event)}, output,
			time.Date(2026, 7, 17, 19, 30, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatal(err)
		}
		if replacement == "" {
			replacement = "changed"
		}
		tampered := fact.Clone()
		switch selector % 6 {
		case 0:
			if replacement == tampered.Sources[0].EvidenceRecordDigest {
				replacement += "-changed"
			}
			tampered.Sources[0].EvidenceRecordDigest = replacement
		case 1:
			if replacement == tampered.Sources[0].ProjectionDigest {
				replacement += "-changed"
			}
			tampered.Sources[0].ProjectionDigest = replacement
		case 2:
			if replacement == tampered.Output.ContentDigest {
				replacement += "-changed"
			}
			tampered.Output.ContentDigest = replacement
		case 3:
			if replacement == tampered.Authority {
				replacement += "-changed"
			}
			tampered.Authority = replacement
		case 4:
			if replacement == tampered.SourceSetDigest {
				replacement += "-changed"
			}
			tampered.SourceSetDigest = replacement
		case 5:
			if replacement == tampered.Digest {
				replacement += "-changed"
			}
			tampered.Digest = replacement
		}
		if err := tampered.Validate(); err == nil {
			t.Fatal("canonical tamper was accepted")
		}
		if err := fact.Validate(); err != nil {
			t.Fatalf("tamper aliased original Fact: %v", err)
		}
	})
}
