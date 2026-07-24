package contracttest

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func frameworkRef(id string) contract.Ref {
	return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id}
}

func TestScopeCoordinateAndViewPolicyCanonicalFailClosed(t *testing.T) {
	now := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	scope, err := contract.SealScopeCoordinateV1(contract.ScopeCoordinateV1{
		ContractVersion: contract.FrameworkContractVersionV1, ObjectKind: contract.ScopeCoordinateObjectKindV1,
		Owner: contract.OwnerMemory, TenantID: "tenant-a", ScopeKind: contract.ScopeIdentityPrivate, ScopeID: "identity-a",
		IdentityRef: frameworkRef("identity-a"), IdentityEpoch: 7, LineageRef: frameworkRef("lineage-a"),
		AuthorityRef: frameworkRef("authority"), AuthorityEpoch: 9, PolicyRef: frameworkRef("policy"), Purpose: "assist", Sensitivity: "internal",
	})
	if err != nil || scope.Validate() != nil {
		t.Fatalf("scope=%+v err=%v", scope, err)
	}
	tampered := scope
	tampered.ScopeID = "identity-b"
	if err := tampered.Validate(); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("scope tamper accepted: %v", err)
	}
	policy, err := contract.SealViewPolicyV1(contract.ViewPolicyV1{
		ContractVersion: contract.FrameworkContractVersionV1, ObjectKind: contract.ViewPolicyObjectKindV1,
		Ref: contract.Ref{ID: "memory-view-policy", Revision: 1}, Owner: contract.OwnerMemory, PrincipalScope: scope,
		Grants: []contract.ScopeGrantV1{
			{ScopeRef: frameworkRef("scope-team"), Disclosure: contract.DisclosureCitationOnly},
			{ScopeRef: frameworkRef("scope-private"), Disclosure: contract.DisclosureFull},
		},
		MaxItems: 8, MaxBytes: 4096, MaxTokens: 1024, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil || policy.Validate(now) != nil || policy.Grants[0].ScopeRef.ID != "scope-private" {
		t.Fatalf("policy=%+v err=%v", policy, err)
	}
	duplicate := policy
	duplicate.Ref = contract.Ref{ID: "duplicate-policy", Revision: 1}
	duplicate.Grants = append(duplicate.Grants, contract.ScopeGrantV1{ScopeRef: frameworkRef("scope-private"), Disclosure: contract.DisclosureDenied})
	duplicate.Digest = ""
	if _, err := contract.SealViewPolicyV1(duplicate); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("duplicate semantic scope accepted: %v", err)
	}
}

func TestIndexDescriptorCanonicalCoverageAndTypeRules(t *testing.T) {
	now := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	descriptor, err := contract.SealIndexDescriptorV1(contract.IndexDescriptorV1{
		ContractVersion: contract.FrameworkContractVersionV1, ObjectKind: contract.IndexDescriptorObjectKindV1,
		Ref: contract.Ref{ID: "memory-vector-v1", Revision: 1}, Owner: contract.OwnerMemory, Kind: contract.IndexVector,
		ViewRef: frameworkRef("memory-view"), BoundaryRef: frameworkRef("memory-watermark"),
		RecordRefs: []contract.Ref{frameworkRef("record-b"), frameworkRef("record-a")}, BuilderRef: frameworkRef("vector-builder"), ModelRef: frameworkRef("embedding-model"),
		BuilderVersion: "v1", IndexVersion: "v1", Dimension: 384, State: contract.IndexPartial,
		Coverage:  contract.Coverage{Status: contract.CoveragePartial, Expected: 3, Available: 2, DroppedReasons: []string{"record_expired"}},
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil || descriptor.Validate(now) != nil || descriptor.RecordRefs[0].ID != "record-a" {
		t.Fatalf("descriptor=%+v err=%v", descriptor, err)
	}
	badGraph := descriptor
	badGraph.Ref = contract.Ref{ID: "graph", Revision: 1}
	badGraph.Kind = contract.IndexGraph
	badGraph.ModelRef = contract.Ref{}
	badGraph.Dimension = 384
	badGraph.Digest = ""
	if _, err := contract.SealIndexDescriptorV1(badGraph); !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("graph accepted vector dimension: %v", err)
	}
	tampered := descriptor
	tampered.Coverage.Available++
	if err := tampered.Validate(now); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("coverage tamper accepted: %v", err)
	}
}

func TestOwnerJobAttemptTransitionAndUnknownInspectOnly(t *testing.T) {
	now := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	reserved, err := contract.SealOwnerJobAttemptV1(contract.OwnerJobAttemptV1{
		ContractVersion: contract.FrameworkContractVersionV1, ObjectKind: contract.OwnerJobAttemptObjectKindV1,
		Ref: contract.Ref{ID: "knowledge-sync-attempt", Revision: 1}, Owner: contract.OwnerKnowledge, Kind: contract.JobKnowledgeSync,
		TenantID: "tenant-a", AuthorityRef: frameworkRef("authority"), PolicyRef: frameworkRef("policy"), ScopeRef: frameworkRef("scope"),
		OperationRef: frameworkRef("operation"), AttemptRef: frameworkRef("attempt"), SubjectRef: frameworkRef("source"), InputDigest: "sha256:input",
		State: contract.JobReserved, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	begun := reserved
	begun.Ref = contract.Ref{ID: reserved.Ref.ID, Revision: 2}
	begun.State = contract.JobBegun
	begun.UpdatedAt = now.Add(time.Second)
	begun.Digest = ""
	begun, err = contract.SealOwnerJobAttemptV1(begun)
	if err != nil || contract.ValidateOwnerJobTransitionV1(reserved, begun) != nil {
		t.Fatalf("begin transition failed: %+v %v", begun, err)
	}
	unknown := begun
	unknown.Ref = contract.Ref{ID: begun.Ref.ID, Revision: 3}
	unknown.State = contract.JobUnknownOutcome
	unknown.UpdatedAt = now.Add(2 * time.Second)
	unknown.Digest = ""
	unknown, err = contract.SealOwnerJobAttemptV1(unknown)
	if err != nil || contract.ValidateOwnerJobTransitionV1(begun, unknown) != nil {
		t.Fatalf("unknown transition failed: %+v %v", unknown, err)
	}
	rebegun := unknown
	rebegun.Ref = contract.Ref{ID: unknown.Ref.ID, Revision: 4}
	rebegun.State = contract.JobBegun
	rebegun.UpdatedAt = now.Add(3 * time.Second)
	rebegun.Digest = ""
	rebegun, err = contract.SealOwnerJobAttemptV1(rebegun)
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateOwnerJobTransitionV1(unknown, rebegun); !errors.Is(err, contract.ErrUnknownOutcome) {
		t.Fatalf("unknown outcome was blindly restarted: %v", err)
	}
	observed := unknown
	observed.Ref = contract.Ref{ID: unknown.Ref.ID, Revision: 4}
	observed.State = contract.JobObserved
	observed.ObservationRef = frameworkRef("inspection-observation")
	observed.UpdatedAt = now.Add(3 * time.Second)
	observed.Digest = ""
	observed, err = contract.SealOwnerJobAttemptV1(observed)
	if err != nil || contract.ValidateOwnerJobTransitionV1(unknown, observed) != nil {
		t.Fatalf("inspect recovery failed: %+v %v", observed, err)
	}
}

func TestOwnerJobCanCloseResidualBeforeBeginWithoutForgedObservation(t *testing.T) {
	now := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	reserved, err := contract.SealOwnerJobAttemptV1(contract.OwnerJobAttemptV1{
		Ref: contract.Ref{ID: "purge-reservation", Revision: 1}, Owner: contract.OwnerMemory, Kind: contract.JobPurge,
		TenantID: "tenant-a", AuthorityRef: frameworkRef("authority"), PolicyRef: frameworkRef("policy"), ScopeRef: frameworkRef("scope"),
		OperationRef: frameworkRef("operation"), AttemptRef: frameworkRef("attempt"), SubjectRef: frameworkRef("record"), InputDigest: "sha256:input",
		State: contract.JobReserved, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	residual := reserved
	residual.Ref = contract.Ref{ID: reserved.Ref.ID, Revision: 2}
	residual.State = contract.JobResidual
	residual.Residuals = []string{"legal_hold_active"}
	residual.UpdatedAt = now.Add(time.Second)
	residual.Digest = ""
	residual, err = contract.SealOwnerJobAttemptV1(residual)
	if err != nil || contract.ValidateOwnerJobTransitionV1(reserved, residual) != nil || residual.ObservationRef != (contract.Ref{}) {
		t.Fatalf("pre-begin residual failed: %+v %v", residual, err)
	}
}
