package knowledge_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/knowledge"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/reference"
)

func TestKnowledgeBlackBoxPublicOwnerPort(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	store := knowledge.NewStore(contract.ClockFunc(func() time.Time { return now }))
	content := reference.NewStore()
	ref := func(id string) contract.Ref {
		return contract.Ref{ID: id, Revision: 1, Digest: contract.MustDigest(id)}
	}
	access := knowledge.Access{TenantID: "tenant", AuthorityRef: ref("authority"), PolicyRef: ref("policy")}
	source, err := store.RegisterSource(access, knowledge.SourceInput{
		TenantID: access.TenantID, ID: "source", Version: "v1", AssetRef: ref("asset"),
		ContentDigest: contract.MustDigest("source-body"), AuthorityRef: access.AuthorityRef,
		PolicyRef: access.PolicyRef, License: "internal-use", Scope: "domain", Sensitivity: "internal",
		State: knowledge.SourceAvailable, Provenance: []contract.Ref{ref("provenance")},
		AcquiredAt: now, ValidFrom: now.Add(-time.Hour), ValidTo: now.Add(time.Hour),
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := store.PutPackage(access, knowledge.PackageInput{
		TenantID: access.TenantID, ID: "package", Version: "v1", SourceRefs: []contract.Ref{source.Ref},
		AuthorityRef: access.AuthorityRef, PolicyRef: access.PolicyRef, License: "internal-use",
		Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1},
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	contentRef, err := content.Put([]byte("alpha public knowledge port"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.SubmitCandidate(access, knowledge.CandidateInput{
		TenantID: access.TenantID, ID: "candidate", ProducerID: "producer", SourceEpoch: 1, SourceSequence: 1,
		Kind: knowledge.CandidateRecord, PayloadDigest: contract.MustDigest("payload"), TTL: time.Hour,
		Draft: knowledge.RecordDraft{
			ID: "record", PackageRef: pkg.Ref, ContentRef: contentRef, SourceRefs: []contract.Ref{source.Ref},
			EvidenceRefs: []contract.Ref{ref("evidence")}, Scope: "domain", Subject: "subject",
			Sensitivity: "internal", License: "internal-use", TrustState: "source_supported",
			ValidFrom: now.Add(-time.Minute), ValidTo: now.Add(time.Hour),
		},
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	admission, err := store.AdmitCandidate(access, candidate.Ref, knowledge.AdmissionCommitReady, "accepted", time.Hour, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginCommit(access, knowledge.CommitRequest{
		TenantID: access.TenantID, AttemptID: "attempt", OperationRef: ref("operation"),
		CandidateRef: candidate.Ref, AdmissionRef: admission.Ref, ExpectedRecord: contract.ExpectAbsent(),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.CommitAttempt(access, attempt.Ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Owner != contract.OwnerKnowledge || result.State != contract.DomainResultReady {
		t.Fatalf("domain result crossed owner or settlement boundary: %+v", result)
	}
	application, err := store.ApplySettlement(access, contract.DomainResultAssociation{DomainResultRef: result.Ref}, contract.RuntimeSettlementRef{Ref: ref("runtime-settlement")}, contract.ExpectAbsent())
	if err != nil || application.Owner != contract.OwnerKnowledge || application.State != contract.DomainResultSettled {
		t.Fatalf("opaque settlement application failed: %+v %v", application, err)
	}
	record, err := store.GetRecord(access, result.SubjectRef)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := store.CreateSnapshot(access, knowledge.SnapshotInput{
		TenantID: access.TenantID, ID: "snapshot", Version: "v1", SourceRefs: []contract.Ref{source.Ref},
		PackageRefs: []contract.Ref{pkg.Ref}, RecordRefs: []contract.Ref{record.Ref},
		Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1},
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, err := store.PublishSnapshot(access, ready.Ref, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.CreateView(access, knowledge.ViewInput{
		TenantID: access.TenantID, ID: "view", SnapshotRef: snapshot.Ref,
		AuthorityRef: access.AuthorityRef, PolicyRef: access.PolicyRef, Scopes: []string{"domain"},
		AllowedLicenses: []string{"internal-use"}, SensitivityMax: "internal", Purpose: "answer",
		CurrentOnly: true, TTL: time.Hour,
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	queryResult, err := store.Query(access, contract.RetrievalQuery{
		ID: "query", Revision: 1, Domain: contract.OwnerKnowledge, ViewRef: view.Ref,
		Purpose: "answer", Text: "knowledge", Scopes: []string{"domain"}, SensitivityMax: "internal",
		Limit: 10, RequestedAt: now, ExpiresAt: now.Add(time.Hour),
	}, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(queryResult.Hits) != 1 || !contract.SameRef(queryResult.Hits[0].Citation.RecordRef, record.Ref) || !contract.SameRef(queryResult.Hits[0].SnapshotRef, snapshot.Ref) {
		t.Fatalf("public query lost exact citation/watermark: %+v", queryResult)
	}
}
