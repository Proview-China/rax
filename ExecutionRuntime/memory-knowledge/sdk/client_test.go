package sdk_test

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/reference"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/sdk"
	"testing"
	"time"
)

func rr(id string) contract.Ref { return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id} }
func TestMemorySDKWriteInspectAndCancellation(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	content := reference.NewStore()
	body, err := content.Put([]byte("sdk memory"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	store := memory.NewStore(contract.ClockFunc(func() time.Time { return now }), content)
	client, err := sdk.NewMemory(store)
	if err != nil {
		t.Fatal(err)
	}
	access := memory.Access{TenantID: "tenant", IdentityID: "identity", AuthorityRef: rr("authority"), AuthorityEpoch: 1, PolicyRef: rr("policy")}
	candidate := memory.SealCandidate(memory.Candidate{Envelope: contract.Envelope{ContractVersion: contract.VersionV1, SchemaRef: "praxis.memory/candidate-v1", ID: "candidate", Revision: 1, TenantID: "tenant", IdentityID: "identity", IdentityEpoch: 1, AuthorityRef: access.AuthorityRef, AuthorityEpoch: 1, PolicyRef: access.PolicyRef, Purpose: "assist", ActionScopeDigest: "sha256:scope", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour), CorrelationID: "sdk"}, Kind: memory.CandidateCreate, ProducerRef: rr("producer"), SourceEpoch: 1, SourceSequence: 1, Scope: "identity_private", Subject: "sdk", ContentRef: &body, SourceRefs: []contract.Ref{rr("source")}, EvidenceRefs: []contract.Ref{rr("evidence")}, Sensitivity: "internal"})
	result, err := client.Write(context.Background(), access, sdk.MemoryWriteRequest{Candidate: candidate, Admission: memory.AdmissionRequest{ID: "admission", Decision: memory.AdmissionCommitReady, ExpiresAt: now.Add(time.Hour), ExpectedRevision: contract.ExpectAbsent()}, Commit: memory.CommitRequest{TenantID: "tenant", AttemptID: "attempt", ResultID: "result", RecordID: "record", OperationRef: rr("operation"), ExpectedRevision: contract.ExpectAbsent()}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Inspect(context.Background(), access, result.Record.Ref)
	if err != nil || !contract.SameRef(got.Ref, result.Record.Ref) {
		t.Fatalf("got=%+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Inspect(ctx, access, result.Record.Ref); err == nil {
		t.Fatal("cancellation ignored")
	}
}
