package knowledge

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestConnectorContractsBindFullGovernanceAndInspectOriginalAttempt(t *testing.T) {
	now := time.Date(2026, 7, 17, 7, 0, 0, 0, time.UTC)
	request, err := SealAcquireRequestV1(AcquireRequestV1{Ref: contract.Ref{ID: "acquire-request", Revision: 1}, TenantID: "tenant", SourceKind: SourceCode, SourceSubjectRef: ref("source-subject"), ConnectorRef: ref("connector"), AuthorityRef: ref("authority"), PolicyRef: ref("policy"), ScopeRef: ref("scope"), BudgetRef: ref("budget"), OperationRef: ref("operation"), AttemptRef: ref("attempt"), PermitRef: ref("permit"), PrepareEnforcementRef: ref("prepare-enforcement"), ExecuteEnforcementRef: ref("execute-enforcement"), RequestedAt: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := SealAcquireObservationV1(AcquireObservationV1{Ref: contract.Ref{ID: "observation", Revision: 1}, RequestRef: request.Ref, AttemptRef: request.AttemptRef, ProviderReceiptRef: ref("provider-receipt"), AssetRef: ref("asset"), ContentDigest: "sha256:content", SourceVersion: "commit-1", ProvenanceRefs: []contract.Ref{ref("provenance")}, License: "internal-use", Scope: "project", Sensitivity: "internal", ObservedAt: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := SealAcquireInspectionV1(AcquireInspectionV1{Ref: contract.Ref{ID: "inspection", Revision: 1}, RequestRef: request.Ref, AttemptRef: request.AttemptRef, Outcome: AcquireObserved, ObservationRef: observation.Ref, InspectedAt: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil || inspection.Validate(now) != nil {
		t.Fatalf("inspection=%+v err=%v", inspection, err)
	}
	tampered := request
	tampered.PermitRef = ref("other-permit")
	if err := tampered.Validate(now); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("permit tamper accepted: %v", err)
	}
	missing := request
	missing.Ref = contract.Ref{ID: "missing", Revision: 1}
	missing.ExecuteEnforcementRef = contract.Ref{}
	missing.Digest = ""
	if _, err := SealAcquireRequestV1(missing); !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("missing execution enforcement accepted: %v", err)
	}
}
