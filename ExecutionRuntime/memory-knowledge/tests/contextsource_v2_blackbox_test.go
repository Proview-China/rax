package tests_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	knowledge "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/knowledge/contextsource"
	memory "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/memory/contextsource"
)

func exactRef(id string) contract.Ref {
	return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id}
}

func TestContextSourceV2PublicNominalsStayOwnerSeparated(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	memoryCoordinate, err := memory.SealAttemptCoordinateV2(memory.AttemptCoordinateV2{
		ContractVersion: memory.ContractVersionV2, ObjectKind: memory.AttemptCoordinateKindV2,
		TenantID: "tenant", IdentityRef: exactRef("identity"), IdentityEpoch: 1, ExecutionScopeDigest: "sha256:scope", RunID: "run",
		SessionRef: exactRef("session"), SessionEvidenceRef: exactRef("session-evidence"), SessionCheckedAt: now, SessionExpiresAt: now.Add(time.Hour),
		SourceTurnOrdinal: 3, SourceTurnRef: exactRef("turn"), TurnEvidenceRef: exactRef("turn-evidence"), TurnCheckedAt: now, TurnExpiresAt: now.Add(time.Hour), LegacyTurnID: "turn",
		AttemptRef: exactRef("memory-attempt"), RequestDigest: "sha256:request", IdempotencyKey: "memory-key", ObservationRef: exactRef("memory-observation"), ResultRef: exactRef("memory-result"),
	})
	if err != nil || memoryCoordinate.Validate() != nil {
		t.Fatalf("memory public coordinate invalid: %+v %v", memoryCoordinate, err)
	}
	knowledgeCoordinate, err := knowledge.SealAttemptCoordinateV2(knowledge.AttemptCoordinateV2{
		ContractVersion: knowledge.ContractVersionV2, ObjectKind: knowledge.AttemptCoordinateKindV2,
		TenantID: "tenant", IdentityRef: exactRef("identity"), IdentityEpoch: 1, ExecutionScopeDigest: "sha256:scope", RunID: "run",
		SessionRef: exactRef("session"), SessionEvidenceRef: exactRef("session-evidence"), SessionCheckedAt: now, SessionExpiresAt: now.Add(time.Hour),
		SourceTurnOrdinal: 3, SourceTurnRef: exactRef("turn"), TurnEvidenceRef: exactRef("turn-evidence"), TurnCheckedAt: now, TurnExpiresAt: now.Add(time.Hour), LegacyTurnID: "turn",
		AttemptRef: exactRef("knowledge-attempt"), RequestDigest: "sha256:request", IdempotencyKey: "knowledge-key", ObservationRef: exactRef("knowledge-observation"), ResultRef: exactRef("knowledge-result"),
	})
	if err != nil || knowledgeCoordinate.Validate() != nil {
		t.Fatalf("knowledge public coordinate invalid: %+v %v", knowledgeCoordinate, err)
	}
	if memoryCoordinate.ObjectKind == knowledgeCoordinate.ObjectKind || memoryCoordinate.ContractVersion == knowledgeCoordinate.ContractVersion {
		t.Fatal("Memory and Knowledge V2 owner nominals collapsed onto one wire contract")
	}
	tampered := memoryCoordinate
	tampered.SourceTurnOrdinal++
	if err := tampered.Validate(); err == nil {
		t.Fatal("canonical coordinate tamper accepted")
	}
}
