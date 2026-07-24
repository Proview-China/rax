package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestArtifactRefRequiresExactLowerParentAndDeepClone(t *testing.T) {
	source := testkit.ArtifactSourceProjectionV1("evidence-1", "evidence-digest-1")
	if err := source.Artifact.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := source.Artifact.Clone()
	clone.ParentRevisionRef.Digest = "mutated"
	if source.Artifact.ParentRevisionRef.Digest == "mutated" {
		t.Fatal("ArtifactRef clone aliases parent exact ref")
	}
	for name, mutate := range map[string]func(*contract.ArtifactRefV1){
		"same revision":  func(value *contract.ArtifactRefV1) { value.ParentRevisionRef.Revision = value.ArtifactFactRef.Revision },
		"other owner":    func(value *contract.ArtifactRefV1) { value.ParentRevisionRef.Owner.ComponentID = "other-owner" },
		"other artifact": func(value *contract.ArtifactRefV1) { value.ParentRevisionRef.ID = "artifact-2" },
		"other tenant":   func(value *contract.ArtifactRefV1) { value.ParentRevisionRef.TenantID = "tenant-2" },
	} {
		t.Run(name, func(t *testing.T) {
			value := source.Artifact.Clone()
			mutate(&value)
			if err := value.Validate(); !contract.HasCode(err, contract.ErrRevisionConflict) {
				t.Fatalf("parent splice error = %v", err)
			}
		})
	}
}

func TestArtifactRelationCanonicalTamperAndOwnerBoundary(t *testing.T) {
	now := time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)
	source := testkit.ArtifactSourceProjectionV1("evidence-1", "evidence-digest-1")
	fact, err := contract.NewArtifactRelationFactV1("relation-1", "request-1", testkit.Scope(), testkit.ArtifactRelationOwnerV1(), source, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := fact.Validate(); err != nil {
		t.Fatal(err)
	}
	tampered := fact.Clone()
	tampered.SourceProjection.Artifact.StorageDigest = "tampered"
	if err := tampered.Validate(); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("canonical tamper error = %v", err)
	}
	wrongOwner := testkit.ArtifactRelationOwnerV1()
	wrongOwner.ComponentID = "praxis/runtime"
	if _, err := contract.NewArtifactRelationFactV1("relation-2", "request-2", testkit.Scope(), wrongOwner, source, now); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("wrong owner error = %v", err)
	}
}

func TestArtifactRelationSourceRejectsCrossTenantAndEvidenceDrift(t *testing.T) {
	for name, mutate := range map[string]func(*contract.ArtifactRelationSourceProjectionV1){
		"related tenant": func(value *contract.ArtifactRelationSourceProjectionV1) { value.RelatedFactRef.TenantID = "tenant-2" },
		"artifact tenant": func(value *contract.ArtifactRelationSourceProjectionV1) {
			value.Artifact.ArtifactFactRef.TenantID = "tenant-2"
		},
		"evidence ref": func(value *contract.ArtifactRelationSourceProjectionV1) {
			value.Artifact.OriginEvidenceRecordRef = "other-evidence"
		},
		"scope": func(value *contract.ArtifactRelationSourceProjectionV1) {
			value.SourceProjectionRef.ScopeDigest = "other-scope"
		},
		"source owner": func(value *contract.ArtifactRelationSourceProjectionV1) {
			value.SourceProjectionRef.Owner.ComponentID = "other-owner"
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := testkit.ArtifactSourceProjectionV1("evidence-1", "evidence-digest-1")
			mutate(&value)
			if err := value.Validate(); !contract.HasCode(err, contract.ErrRevisionConflict) {
				t.Fatalf("source splice error = %v", err)
			}
		})
	}
}
