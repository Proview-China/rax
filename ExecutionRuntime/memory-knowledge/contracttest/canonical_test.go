package contracttest

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestCanonicalDigestStable(t *testing.T) {
	t.Parallel()
	a := map[string]any{"z": 1, "a": []string{"x", "y"}}
	b := map[string]any{"a": []string{"x", "y"}, "z": 1}
	da, err := contract.Digest(a)
	if err != nil {
		t.Fatal(err)
	}
	db, err := contract.Digest(b)
	if err != nil {
		t.Fatal(err)
	}
	if da != db {
		t.Fatalf("digest depends on map insertion order: %s != %s", da, db)
	}
}

func TestStrictDecodeRejectsUnknownDuplicateAndTrailing(t *testing.T) {
	t.Parallel()
	type payload struct {
		ID string `json:"id"`
	}
	for name, raw := range map[string]string{
		"unknown":   `{"id":"x","extra":1}`,
		"duplicate": `{"id":"x","id":"y"}`,
		"trailing":  `{"id":"x"} {"id":"y"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var out payload
			if err := contract.StrictDecode([]byte(raw), &out); err == nil {
				t.Fatal("expected strict decode error")
			}
		})
	}
}

func TestEnvelopeCurrentnessTTLBoundary(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	ref := contract.Ref{ID: "fact", Revision: 1, Digest: "sha256:x"}
	envelope := contract.Envelope{
		ContractVersion: contract.VersionV1, SchemaRef: "schema:v1", ID: "candidate", Revision: 1,
		TenantID: "tenant", IdentityID: "identity", IdentityEpoch: 1, AuthorityRef: ref,
		AuthorityEpoch: 1, PolicyRef: ref, Purpose: "test", ActionScopeDigest: "scope",
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Minute), ExpiresAt: now,
		Causation: []contract.Ref{},
	}
	if err := envelope.ValidateCurrent(now); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("TTL boundary must be expired, got %v", err)
	}
	envelope.ExpiresAt = now.Add(time.Nanosecond)
	if err := envelope.ValidateCurrent(now); err != nil {
		t.Fatalf("future TTL must be current: %v", err)
	}
}

func TestRuntimeSettlementIsOpaqueRefOnly(t *testing.T) {
	t.Parallel()
	var settlement contract.RuntimeSettlementRef
	if err := contract.StrictDecode([]byte(`{"ref":{"id":"runtime-settlement","revision":1,"digest":"sha256:s"},"outcome":"success"}`), &settlement); err == nil {
		t.Fatal("runtime outcome must not be accepted by the component contract")
	}
}

func TestExpectedRevisionKeepsAbsentDistinctFromZero(t *testing.T) {
	t.Parallel()
	if err := contract.ExpectAbsent().Validate(); err != nil {
		t.Fatal(err)
	}
	if !contract.ExpectAbsent().Matches(false, 0) || contract.ExpectAbsent().Matches(true, 0) {
		t.Fatal("expect_absent matching is incorrect")
	}
	if err := (contract.ExpectedRevision{}).Validate(); err == nil {
		t.Fatal("zero value must not silently mean expect_absent")
	}
	expected := contract.ExpectRevision(3)
	if err := expected.Validate(); err != nil || !expected.Matches(true, 3) || expected.Matches(true, 2) {
		t.Fatalf("expected revision matching is incorrect: %v", err)
	}
}

func TestSettlementApplicationRejectsCrossOwner(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	ref := contract.Ref{ID: "fact", Revision: 1, Digest: "sha256:x"}
	result, err := contract.NewDomainResultFact(contract.OwnerMemory, "result", "attempt", ref, ref, ref, 0, 1, nil, contract.Coverage{Status: contract.CoverageComplete}, "complete", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	settlement := contract.RuntimeSettlementRef{Ref: contract.Ref{ID: "runtime-settlement", Revision: 1, Digest: "sha256:s"}}
	association, err := contract.AssociateDomainResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contract.NewSettlementApplication(contract.OwnerKnowledge, "apply", 1, result, association, settlement, now); !errors.Is(err, contract.ErrSettlementMismatch) {
		t.Fatalf("cross-owner settlement must fail closed, got %v", err)
	}
}

func TestDomainFactsRejectCanonicalDigestTamper(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	ref := contract.Ref{ID: "fact", Revision: 1, Digest: "sha256:x"}
	result, err := contract.NewDomainResultFact(contract.OwnerMemory, "result", "attempt", ref, ref, ref, 0, 1, nil, contract.Coverage{Status: contract.CoverageComplete}, "complete", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("fresh result invalid: %v", err)
	}
	tampered := result
	tampered.CASAfter++
	if err := tampered.Validate(); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tampered domain result accepted: %v", err)
	}

	association, err := contract.AssociateDomainResult(result)
	if err != nil {
		t.Fatal(err)
	}
	application, err := contract.NewSettlementApplication(contract.OwnerMemory, "apply", 1, result, association, contract.RuntimeSettlementRef{Ref: ref}, now)
	if err != nil {
		t.Fatal(err)
	}
	application.AppliedAt = application.AppliedAt.Add(time.Second)
	if err := application.Validate(); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tampered settlement application accepted: %v", err)
	}
}

func TestDomainResultAssociationRejectsWrongRefAndDigest(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	ref := contract.Ref{ID: "fact", Revision: 1, Digest: "sha256:x"}
	result, err := contract.NewDomainResultFact(contract.OwnerKnowledge, "result", "attempt", ref, ref, ref, 0, 1, nil, contract.Coverage{Status: contract.CoverageComplete}, "complete", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	for name, wrong := range map[string]contract.Ref{
		"wrong-ref":    {ID: "other-result", Revision: result.Ref.Revision, Digest: result.Ref.Digest},
		"wrong-digest": {ID: result.Ref.ID, Revision: result.Ref.Revision, Digest: "sha256:tampered"},
	} {
		t.Run(name, func(t *testing.T) {
			association := contract.DomainResultAssociation{DomainResultRef: wrong}
			if err := association.Verify(result); !errors.Is(err, contract.ErrSettlementMismatch) {
				t.Fatalf("wrong association accepted: %v", err)
			}
		})
	}
}
