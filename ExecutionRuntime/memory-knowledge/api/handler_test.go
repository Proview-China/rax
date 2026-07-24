package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/knowledge"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/sdk"
)

func ar(id string) contract.Ref { return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id} }

type resolver struct{ deny bool }

func (r resolver) MemoryAccess(*http.Request) (memory.Access, error) {
	if r.deny {
		return memory.Access{}, contract.ErrScopeDenied
	}
	return memory.Access{TenantID: "tenant", IdentityID: "identity", AuthorityRef: ar("authority"), AuthorityEpoch: 1, PolicyRef: ar("policy")}, nil
}
func (r resolver) KnowledgeAccess(*http.Request) (knowledge.Access, error) {
	if r.deny {
		return knowledge.Access{}, contract.ErrScopeDenied
	}
	return knowledge.Access{TenantID: "tenant", AuthorityRef: ar("authority"), PolicyRef: ar("policy")}, nil
}

type memoryBackend struct{}

type purgeCaptureBackend struct {
	memoryBackend
	request memory.PurgeRequest
}

func (b *purgeCaptureBackend) PreparePurge(_ memory.Access, request memory.PurgeRequest) (contract.PurgeIntentV1, error) {
	b.request = request
	return contract.PurgeIntentV1{}, nil
}

func (memoryBackend) SubmitCandidate(memory.Access, memory.Candidate) (memory.Candidate, error) {
	return memory.Candidate{}, contract.ErrUnsupported
}
func (memoryBackend) Admit(memory.Access, memory.AdmissionRequest) (memory.AdmissionFact, error) {
	return memory.AdmissionFact{}, contract.ErrUnsupported
}
func (memoryBackend) Commit(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error) {
	return memory.Record{}, contract.DomainResultFact{}, contract.ErrUnsupported
}
func (memoryBackend) Correct(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error) {
	return memory.Record{}, contract.DomainResultFact{}, contract.ErrUnsupported
}
func (memoryBackend) Pin(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error) {
	return memory.Record{}, contract.DomainResultFact{}, contract.ErrUnsupported
}
func (memoryBackend) Archive(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error) {
	return memory.Record{}, contract.DomainResultFact{}, contract.ErrUnsupported
}
func (memoryBackend) Forget(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error) {
	return memory.Record{}, contract.DomainResultFact{}, contract.ErrUnsupported
}
func (memoryBackend) Merge(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error) {
	return memory.Record{}, contract.DomainResultFact{}, contract.ErrUnsupported
}
func (memoryBackend) InspectRecord(_ memory.Access, ref contract.Ref) (memory.Record, error) {
	if ref.ID != "record" {
		return memory.Record{}, contract.ErrNotFound
	}
	return memory.Record{Ref: ref, Owner: contract.OwnerMemory, TenantID: "tenant", IdentityID: "identity"}, nil
}
func (memoryBackend) Query(memory.Access, contract.RetrievalQuery) (contract.RetrievalResult, error) {
	return contract.RetrievalResult{}, contract.ErrUnsupported
}
func (memoryBackend) InspectJob(memory.Access, contract.Ref) (contract.OwnerJobAttemptV1, error) {
	return contract.OwnerJobAttemptV1{}, contract.ErrUnsupported
}
func (memoryBackend) ListProjections(memory.Access) ([]memory.Projection, error) {
	return []memory.Projection{}, nil
}
func (memoryBackend) ExportView(memory.Access, contract.Ref, string, time.Duration) (contract.ExportManifestV1, error) {
	return contract.ExportManifestV1{}, contract.ErrUnsupported
}
func (memoryBackend) ReindexLocal(memory.Access, memory.Projection, contract.ExpectedRevision, contract.IndexDescriptorV1, contract.ExpectedRevision) (memory.Projection, contract.IndexDescriptorV1, error) {
	return memory.Projection{}, contract.IndexDescriptorV1{}, contract.ErrUnsupported
}
func (memoryBackend) ListIndexDescriptors(memory.Access) ([]contract.IndexDescriptorV1, error) {
	return []contract.IndexDescriptorV1{}, nil
}
func (memoryBackend) WatchChanges(memory.Access, contract.WatchRequestV1) (contract.ChangePageV1, error) {
	return contract.ChangePageV1{}, contract.ErrUnsupported
}
func (memoryBackend) PublishView(memory.Access, memory.View, contract.ExpectedRevision) (memory.View, error) {
	return memory.View{}, contract.ErrUnsupported
}
func (memoryBackend) PreparePurge(memory.Access, memory.PurgeRequest) (contract.PurgeIntentV1, error) {
	return contract.PurgeIntentV1{}, contract.ErrUnsupported
}
func (memoryBackend) InspectPurge(memory.Access, contract.Ref) (contract.PurgeIntentV1, error) {
	return contract.PurgeIntentV1{}, contract.ErrUnsupported
}

func TestHandlerStrictBodyResolvedAuthorityAndClosedErrors(t *testing.T) {
	client, err := sdk.NewMemory(memoryBackend{})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(resolver{}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"ref":{"id":"record","revision":1,"digest":"sha256:record"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/memory/inspect", strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	unknown := httptest.NewRequest(http.MethodPost, "/v1/memory/inspect", strings.NewReader(`{"ref":{"id":"record","revision":1,"digest":"sha256:record"},"authority_ref":{"id":"forged"}}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, unknown)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("forged authority status=%d body=%s", response.Code, response.Body.String())
	}
	duplicate := httptest.NewRequest(http.MethodPost, "/v1/memory/inspect", strings.NewReader(`{"ref":{"id":"record","revision":1,"digest":"sha256:record"},"ref":{"id":"other","revision":1,"digest":"sha256:other"}}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, duplicate)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("duplicate status=%d", response.Code)
	}
	deniedHandler, _ := NewHandler(resolver{deny: true}, client, nil)
	denied := httptest.NewRequest(http.MethodPost, "/v1/memory/inspect", strings.NewReader(body))
	response = httptest.NewRecorder()
	deniedHandler.ServeHTTP(response, denied)
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), "authority") {
		t.Fatalf("denied leaked: %d %s", response.Code, response.Body.String())
	}
}
func TestHandlerRejectsMethodAndOversizedBody(t *testing.T) {
	handler, _ := NewHandler(resolver{}, nil, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/memory/query", nil))
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("method=%d", response.Code)
	}
	large := strings.Repeat("x", int(MaxRequestBytes)+1)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/memory/query", strings.NewReader(large)))
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("nil client should close unsupported: %d", response.Code)
	}
}

func TestMemoryPurgeWireUsesBoundedTTLSeconds(t *testing.T) {
	backend := &purgeCaptureBackend{}
	client, err := sdk.NewMemory(backend)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(resolver{}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":"purge-1","target_ref":{"id":"record","revision":1,"digest":"sha256:record"},"scope_ref":{"id":"scope","revision":1,"digest":"sha256:scope"},"operation_ref":{"id":"operation","revision":1,"digest":"sha256:operation"},"requested_by_ref":{"id":"requester","revision":1,"digest":"sha256:requester"},"retention_decision_ref":{"id":"retention","revision":1,"digest":"sha256:retention"},"legal_hold_inspection_ref":{"id":"hold","revision":1,"digest":"sha256:hold"},"ttl_seconds":30}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/memory/purge/prepare", strings.NewReader(body)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if backend.request.TTL != 30*time.Second {
		t.Fatalf("ttl=%s", backend.request.TTL)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/memory/purge/prepare", strings.NewReader(strings.Replace(body, `"ttl_seconds":30`, `"ttl":30000000000`, 1))))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("duration wire accepted: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestKnowledgeSourceInspectUsesExactOwnerRef(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	store := knowledge.NewStore(contract.ClockFunc(func() time.Time { return now }))
	access := knowledge.Access{TenantID: "tenant", AuthorityRef: ar("authority"), PolicyRef: ar("policy")}
	source, err := store.RegisterSource(access, knowledge.SourceInput{
		TenantID: "tenant", ID: "source", Version: "v1", AssetRef: ar("asset"),
		ContentDigest: "sha256:content", AuthorityRef: access.AuthorityRef, PolicyRef: access.PolicyRef,
		License: "internal-use", Scope: "project", Sensitivity: "internal", State: knowledge.SourceAvailable,
		Provenance: []contract.Ref{ar("provenance")}, AcquiredAt: now, ValidFrom: now.Add(-time.Hour), ValidTo: now.Add(time.Hour),
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	client, err := sdk.NewKnowledge(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(resolver{}, nil, client)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"ref":{"id":"` + source.Ref.ID + `","revision":1,"digest":"` + source.Ref.Digest + `"}}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/knowledge/source/inspect", strings.NewReader(body)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"source"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/knowledge/source/inspect", strings.NewReader(`{"ref":{"id":"source","revision":1,"digest":"sha256:tampered"}}`)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("tamper status=%d body=%s", response.Code, response.Body.String())
	}
}
