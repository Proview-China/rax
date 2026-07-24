// Package api provides a backend-neutral HTTP command surface. Authentication
// and transport deployment are host concerns; this handler requires an exact
// principal resolver and never accepts authority coordinates from request JSON.
package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/knowledge"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/sdk"
)

const MaxRequestBytes int64 = 1 << 20

type PrincipalResolver interface {
	MemoryAccess(*http.Request) (memory.Access, error)
	KnowledgeAccess(*http.Request) (knowledge.Access, error)
}

type Handler struct {
	resolver  PrincipalResolver
	memory    *sdk.MemoryClient
	knowledge *sdk.KnowledgeClient
}

func NewHandler(resolver PrincipalResolver, memoryClient *sdk.MemoryClient, knowledgeClient *sdk.KnowledgeClient) (*Handler, error) {
	if resolver == nil {
		return nil, contract.ErrInvalidArgument
	}
	return &Handler{resolver: resolver, memory: memoryClient, knowledge: knowledgeClient}, nil
}

type memoryWriteRequest struct {
	Candidate memory.Candidate        `json:"candidate"`
	Admission memory.AdmissionRequest `json:"admission"`
	Commit    memory.CommitRequest    `json:"commit"`
}
type inspectRequest struct {
	Ref contract.Ref `json:"ref"`
}
type registerSourceRequest struct {
	Input    knowledge.SourceInput     `json:"input"`
	Expected contract.ExpectedRevision `json:"expected"`
}
type withdrawSourceRequest struct {
	SourceID string                    `json:"source_id"`
	Reason   string                    `json:"reason"`
	Expected contract.ExpectedRevision `json:"expected"`
}
type buildSnapshotRequest struct {
	Input            knowledge.SnapshotInput   `json:"input"`
	ExpectedSnapshot contract.ExpectedRevision `json:"expected_snapshot"`
	ExpectedPointer  contract.ExpectedRevision `json:"expected_pointer"`
}
type exportRequest struct {
	ViewRef    contract.Ref `json:"view_ref"`
	ID         string       `json:"id"`
	TTLSeconds uint64       `json:"ttl_seconds"`
}
type memoryReindexRequest struct {
	Projection         memory.Projection          `json:"projection"`
	ExpectedProjection contract.ExpectedRevision  `json:"expected_projection"`
	Descriptor         contract.IndexDescriptorV1 `json:"descriptor"`
	ExpectedDescriptor contract.ExpectedRevision  `json:"expected_descriptor"`
}
type knowledgeReindexRequest struct {
	Projection         knowledge.ProjectionInput  `json:"projection"`
	ExpectedProjection contract.ExpectedRevision  `json:"expected_projection"`
	Descriptor         contract.IndexDescriptorV1 `json:"descriptor"`
	ExpectedDescriptor contract.ExpectedRevision  `json:"expected_descriptor"`
}
type memoryViewRequest struct {
	View     memory.View               `json:"view"`
	Expected contract.ExpectedRevision `json:"expected"`
}
type knowledgeViewRequest struct {
	Input    knowledge.ViewInput       `json:"input"`
	Expected contract.ExpectedRevision `json:"expected"`
}
type memoryPurgeRequest struct {
	ID                     string       `json:"id"`
	TargetRef              contract.Ref `json:"target_ref"`
	ScopeRef               contract.Ref `json:"scope_ref"`
	OperationRef           contract.Ref `json:"operation_ref"`
	RequestedByRef         contract.Ref `json:"requested_by_ref"`
	RetentionDecisionRef   contract.Ref `json:"retention_decision_ref"`
	LegalHoldInspectionRef contract.Ref `json:"legal_hold_inspection_ref"`
	NotBefore              time.Time    `json:"not_before"`
	TTLSeconds             uint64       `json:"ttl_seconds"`
}
type knowledgePurgeRequest struct {
	ID                     string       `json:"id"`
	TargetKind             string       `json:"target_kind"`
	TargetRef              contract.Ref `json:"target_ref"`
	ScopeRef               contract.Ref `json:"scope_ref"`
	OperationRef           contract.Ref `json:"operation_ref"`
	RequestedByRef         contract.Ref `json:"requested_by_ref"`
	RetentionDecisionRef   contract.Ref `json:"retention_decision_ref"`
	LegalHoldInspectionRef contract.Ref `json:"legal_hold_inspection_ref"`
	NotBefore              time.Time    `json:"not_before"`
	TTLSeconds             uint64       `json:"ttl_seconds"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, contract.ErrUnsupported)
		return
	}
	switch r.URL.Path {
	case "/v1/memory/query":
		h.memoryQuery(w, r)
	case "/v1/memory/inspect":
		h.memoryInspect(w, r)
	case "/v1/memory/write":
		h.memoryWrite(w, r, false)
	case "/v1/memory/forget":
		h.memoryWrite(w, r, true)
	case "/v1/memory/correct":
		h.memoryLifecycleWrite(w, r, "correct")
	case "/v1/memory/pin":
		h.memoryLifecycleWrite(w, r, "pin")
	case "/v1/memory/archive":
		h.memoryLifecycleWrite(w, r, "archive")
	case "/v1/memory/merge":
		h.memoryMerge(w, r)
	case "/v1/memory/export":
		h.memoryExport(w, r)
	case "/v1/memory/watch":
		h.memoryWatch(w, r)
	case "/v1/memory/reindex":
		h.memoryReindex(w, r)
	case "/v1/memory/view/publish":
		h.memoryPublishView(w, r)
	case "/v1/memory/purge/prepare":
		h.memoryPreparePurge(w, r)
	case "/v1/memory/purge/inspect":
		h.memoryInspectPurge(w, r)
	case "/v1/knowledge/query":
		h.knowledgeQuery(w, r)
	case "/v1/knowledge/source/register":
		h.registerSource(w, r)
	case "/v1/knowledge/source/withdraw":
		h.withdrawSource(w, r)
	case "/v1/knowledge/source/deprecate":
		h.deprecateSource(w, r)
	case "/v1/knowledge/source/list":
		h.listSources(w, r)
	case "/v1/knowledge/source/inspect":
		h.inspectSource(w, r)
	case "/v1/knowledge/record/inspect":
		h.inspectKnowledgeRecord(w, r)
	case "/v1/knowledge/snapshot/build":
		h.buildSnapshot(w, r)
	case "/v1/knowledge/sync/prepare":
		h.prepareSync(w, r)
	case "/v1/knowledge/sync/finalize":
		h.finalizeSync(w, r)
	case "/v1/knowledge/export":
		h.knowledgeExport(w, r)
	case "/v1/knowledge/watch":
		h.knowledgeWatch(w, r)
	case "/v1/knowledge/reindex":
		h.knowledgeReindex(w, r)
	case "/v1/knowledge/view/publish":
		h.knowledgePublishView(w, r)
	case "/v1/knowledge/purge/prepare":
		h.knowledgePreparePurge(w, r)
	case "/v1/knowledge/purge/inspect":
		h.knowledgeInspectPurge(w, r)
	case "/v1/index/status":
		h.indexStatus(w, r)
	default:
		http.NotFound(w, r)
	}
}

type indexStatusRequest struct {
	Domain contract.OwnerDomain `json:"domain"`
}

func (h *Handler) listSources(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.KnowledgeAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var empty struct{}
	if err = decode(r, &empty); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.knowledge.ListSources(r.Context(), access)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) inspectSource(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.KnowledgeAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in inspectRequest
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.knowledge.InspectSource(r.Context(), access, in.Ref)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) inspectKnowledgeRecord(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.KnowledgeAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in inspectRequest
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.knowledge.InspectRecord(r.Context(), access, in.Ref)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) indexStatus(w http.ResponseWriter, r *http.Request) {
	var in indexStatusRequest
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	switch in.Domain {
	case contract.OwnerMemory:
		if h.memory == nil {
			writeError(w, contract.ErrUnsupported)
			return
		}
		access, err := h.resolver.MemoryAccess(r)
		if err != nil {
			writeError(w, err)
			return
		}
		out, err := h.memory.ListIndexes(r.Context(), access)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	case contract.OwnerKnowledge:
		if h.knowledge == nil {
			writeError(w, contract.ErrUnsupported)
			return
		}
		access, err := h.resolver.KnowledgeAccess(r)
		if err != nil {
			writeError(w, err)
			return
		}
		out, err := h.knowledge.ListIndexes(r.Context(), access)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	default:
		writeError(w, contract.ErrInvalidArgument)
	}
}
func (h *Handler) memoryQuery(w http.ResponseWriter, r *http.Request) {
	if h.memory == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, e := h.resolver.MemoryAccess(r)
	if e != nil {
		writeError(w, e)
		return
	}
	var q contract.RetrievalQuery
	if e = decode(r, &q); e == nil {
		var out contract.RetrievalResult
		out, e = h.memory.Query(r.Context(), access, q)
		if e == nil {
			writeJSON(w, http.StatusOK, out)
			return
		}
	}
	writeError(w, e)
}
func (h *Handler) memoryInspect(w http.ResponseWriter, r *http.Request) {
	if h.memory == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, e := h.resolver.MemoryAccess(r)
	if e != nil {
		writeError(w, e)
		return
	}
	var in inspectRequest
	if e = decode(r, &in); e == nil {
		var out memory.Record
		out, e = h.memory.Inspect(r.Context(), access, in.Ref)
		if e == nil {
			writeJSON(w, http.StatusOK, out)
			return
		}
	}
	writeError(w, e)
}
func (h *Handler) memoryWrite(w http.ResponseWriter, r *http.Request, forget bool) {
	if h.memory == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, e := h.resolver.MemoryAccess(r)
	if e != nil {
		writeError(w, e)
		return
	}
	var in memoryWriteRequest
	if e = decode(r, &in); e == nil {
		request := sdk.MemoryWriteRequest{Candidate: in.Candidate, Admission: in.Admission, Commit: in.Commit}
		var out sdk.MemoryWriteResult
		if forget {
			out, e = h.memory.Forget(r.Context(), access, request)
		} else {
			out, e = h.memory.Write(r.Context(), access, request)
		}
		if e == nil {
			writeJSON(w, http.StatusCreated, out)
			return
		}
	}
	writeError(w, e)
}
func (h *Handler) memoryMerge(w http.ResponseWriter, r *http.Request) {
	if h.memory == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.MemoryAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in memoryWriteRequest
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.memory.Merge(r.Context(), access, sdk.MemoryWriteRequest{Candidate: in.Candidate, Admission: in.Admission, Commit: in.Commit})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) memoryLifecycleWrite(w http.ResponseWriter, r *http.Request, operation string) {
	if h.memory == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.MemoryAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in memoryWriteRequest
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	request := sdk.MemoryWriteRequest{Candidate: in.Candidate, Admission: in.Admission, Commit: in.Commit}
	var out sdk.MemoryWriteResult
	switch operation {
	case "correct":
		out, err = h.memory.CorrectWrite(r.Context(), access, request)
	case "pin":
		out, err = h.memory.Pin(r.Context(), access, request)
	case "archive":
		out, err = h.memory.Archive(r.Context(), access, request)
	default:
		err = contract.ErrInvalidArgument
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) memoryExport(w http.ResponseWriter, r *http.Request) {
	if h.memory == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.MemoryAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in exportRequest
	if err = decode(r, &in); err != nil || in.TTLSeconds == 0 || in.TTLSeconds > 86400 {
		writeError(w, contract.ErrInvalidArgument)
		return
	}
	out, err := h.memory.Export(r.Context(), access, in.ViewRef, in.ID, time.Duration(in.TTLSeconds)*time.Second)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) memoryWatch(w http.ResponseWriter, r *http.Request) {
	if h.memory == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.MemoryAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in contract.WatchRequestV1
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.memory.Watch(r.Context(), access, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) memoryReindex(w http.ResponseWriter, r *http.Request) {
	if h.memory == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.MemoryAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in memoryReindexRequest
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	projection, descriptor, err := h.memory.Reindex(r.Context(), access, in.Projection, in.ExpectedProjection, in.Descriptor, in.ExpectedDescriptor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Projection memory.Projection          `json:"projection"`
		Descriptor contract.IndexDescriptorV1 `json:"descriptor"`
	}{projection, descriptor})
}

func (h *Handler) memoryPublishView(w http.ResponseWriter, r *http.Request) {
	if h.memory == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.MemoryAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in memoryViewRequest
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.memory.PublishView(r.Context(), access, in.View, in.Expected)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) memoryPreparePurge(w http.ResponseWriter, r *http.Request) {
	if h.memory == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.MemoryAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in memoryPurgeRequest
	if err = decode(r, &in); err != nil || in.TTLSeconds == 0 || in.TTLSeconds > 86400 {
		writeError(w, contract.ErrInvalidArgument)
		return
	}
	request := memory.PurgeRequest{
		ID: in.ID, TargetRef: in.TargetRef, ScopeRef: in.ScopeRef, OperationRef: in.OperationRef,
		RequestedByRef: in.RequestedByRef, RetentionDecisionRef: in.RetentionDecisionRef,
		LegalHoldInspectionRef: in.LegalHoldInspectionRef, NotBefore: in.NotBefore,
		TTL: time.Duration(in.TTLSeconds) * time.Second,
	}
	out, err := h.memory.PreparePurge(r.Context(), access, request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (h *Handler) memoryInspectPurge(w http.ResponseWriter, r *http.Request) {
	if h.memory == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.MemoryAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in inspectRequest
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.memory.InspectPurge(r.Context(), access, in.Ref)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (h *Handler) knowledgeQuery(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, e := h.resolver.KnowledgeAccess(r)
	if e != nil {
		writeError(w, e)
		return
	}
	var q contract.RetrievalQuery
	if e = decode(r, &q); e == nil {
		var out contract.RetrievalResult
		out, e = h.knowledge.Query(r.Context(), access, q)
		if e == nil {
			writeJSON(w, http.StatusOK, out)
			return
		}
	}
	writeError(w, e)
}
func (h *Handler) registerSource(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, e := h.resolver.KnowledgeAccess(r)
	if e != nil {
		writeError(w, e)
		return
	}
	var in registerSourceRequest
	if e = decode(r, &in); e == nil {
		var out knowledge.Source
		out, e = h.knowledge.RegisterSource(r.Context(), access, in.Input, in.Expected)
		if e == nil {
			writeJSON(w, http.StatusCreated, out)
			return
		}
	}
	writeError(w, e)
}
func (h *Handler) withdrawSource(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, e := h.resolver.KnowledgeAccess(r)
	if e != nil {
		writeError(w, e)
		return
	}
	var in withdrawSourceRequest
	if e = decode(r, &in); e == nil {
		var source knowledge.Source
		var tomb knowledge.Tombstone
		source, tomb, e = h.knowledge.WithdrawSource(r.Context(), access, in.SourceID, in.Reason, in.Expected)
		if e == nil {
			writeJSON(w, http.StatusOK, struct {
				Source    knowledge.Source    `json:"source"`
				Tombstone knowledge.Tombstone `json:"tombstone"`
			}{source, tomb})
			return
		}
	}
	writeError(w, e)
}
func (h *Handler) deprecateSource(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.KnowledgeAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in withdrawSourceRequest
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.knowledge.DeprecateSource(r.Context(), access, in.SourceID, in.Reason, in.Expected)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (h *Handler) buildSnapshot(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, e := h.resolver.KnowledgeAccess(r)
	if e != nil {
		writeError(w, e)
		return
	}
	var in buildSnapshotRequest
	if e = decode(r, &in); e == nil {
		var pointer knowledge.SnapshotPointer
		var snapshot knowledge.Snapshot
		pointer, snapshot, e = h.knowledge.BuildSnapshot(r.Context(), access, in.Input, in.ExpectedSnapshot, in.ExpectedPointer)
		if e == nil {
			writeJSON(w, http.StatusCreated, struct {
				Pointer  knowledge.SnapshotPointer `json:"pointer"`
				Snapshot knowledge.Snapshot        `json:"snapshot"`
			}{pointer, snapshot})
			return
		}
	}
	writeError(w, e)
}
func (h *Handler) prepareSync(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, e := h.resolver.KnowledgeAccess(r)
	if e != nil {
		writeError(w, e)
		return
	}
	var in knowledge.PrepareLocalSyncRequest
	if e = decode(r, &in); e == nil {
		var out knowledge.PreparedSyncV1
		out, e = h.knowledge.PrepareSync(r.Context(), access, in)
		if e == nil {
			writeJSON(w, http.StatusAccepted, out)
			return
		}
	}
	writeError(w, e)
}
func (h *Handler) finalizeSync(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, e := h.resolver.KnowledgeAccess(r)
	if e != nil {
		writeError(w, e)
		return
	}
	var in knowledge.FinalizeLocalSyncRequest
	if e = decode(r, &in); e == nil {
		var out knowledge.FinalizedSyncV1
		out, e = h.knowledge.FinalizeSync(r.Context(), access, in)
		if e == nil {
			writeJSON(w, http.StatusCreated, out)
			return
		}
	}
	writeError(w, e)
}

func (h *Handler) knowledgeExport(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.KnowledgeAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in exportRequest
	if err = decode(r, &in); err != nil || in.TTLSeconds == 0 || in.TTLSeconds > 86400 {
		writeError(w, contract.ErrInvalidArgument)
		return
	}
	out, err := h.knowledge.Export(r.Context(), access, in.ViewRef, in.ID, time.Duration(in.TTLSeconds)*time.Second)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) knowledgeWatch(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.KnowledgeAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in contract.WatchRequestV1
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.knowledge.Watch(r.Context(), access, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) knowledgeReindex(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.KnowledgeAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in knowledgeReindexRequest
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	projection, descriptor, err := h.knowledge.Reindex(r.Context(), access, in.Projection, in.ExpectedProjection, in.Descriptor, in.ExpectedDescriptor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Projection knowledge.Projection       `json:"projection"`
		Descriptor contract.IndexDescriptorV1 `json:"descriptor"`
	}{projection, descriptor})
}

func (h *Handler) knowledgePublishView(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.KnowledgeAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in knowledgeViewRequest
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.knowledge.PublishView(r.Context(), access, in.Input, in.Expected)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) knowledgePreparePurge(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.KnowledgeAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in knowledgePurgeRequest
	if err = decode(r, &in); err != nil || in.TTLSeconds == 0 || in.TTLSeconds > 86400 {
		writeError(w, contract.ErrInvalidArgument)
		return
	}
	request := knowledge.PurgeRequest{
		ID: in.ID, TargetKind: in.TargetKind, TargetRef: in.TargetRef, ScopeRef: in.ScopeRef,
		OperationRef: in.OperationRef, RequestedByRef: in.RequestedByRef,
		RetentionDecisionRef: in.RetentionDecisionRef, LegalHoldInspectionRef: in.LegalHoldInspectionRef,
		NotBefore: in.NotBefore, TTL: time.Duration(in.TTLSeconds) * time.Second,
	}
	out, err := h.knowledge.PreparePurge(r.Context(), access, request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (h *Handler) knowledgeInspectPurge(w http.ResponseWriter, r *http.Request) {
	if h.knowledge == nil {
		writeError(w, contract.ErrUnsupported)
		return
	}
	access, err := h.resolver.KnowledgeAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in inspectRequest
	if err = decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.knowledge.InspectPurge(r.Context(), access, in.Ref)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func decode(r *http.Request, dst any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBytes+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > MaxRequestBytes {
		return fmt.Errorf("%w: request too large", contract.ErrInvalidArgument)
	}
	if err := contract.StrictDecode(body, dst); err != nil {
		return fmt.Errorf("%w: invalid request body", contract.ErrInvalidArgument)
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := marshalJSON(value)
	if err != nil {
		http.Error(w, "{\"error\":\"memory-knowledge: internal error\"}", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case err == nil:
		status = http.StatusInternalServerError
	case errors.Is(err, contract.ErrInvalidArgument):
		status = http.StatusBadRequest
	case errors.Is(err, contract.ErrScopeDenied):
		status = http.StatusForbidden
	case errors.Is(err, contract.ErrSensitivityDenied):
		status = http.StatusForbidden
	case errors.Is(err, contract.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, contract.ErrRevisionConflict), errors.Is(err, contract.ErrEvidenceConflict), errors.Is(err, contract.ErrNotCurrent), errors.Is(err, contract.ErrCandidateRejected), errors.Is(err, contract.ErrUnknownOutcome), errors.Is(err, contract.ErrInspectionIncomplete), errors.Is(err, contract.ErrSettlementMismatch), errors.Is(err, contract.ErrContextUnmaterialized):
		status = http.StatusConflict
	case errors.Is(err, contract.ErrUnsupported):
		status = http.StatusNotImplemented
	}
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: closedError(err)})
}
func closedError(err error) string {
	for _, known := range []error{contract.ErrInvalidArgument, contract.ErrScopeDenied, contract.ErrSensitivityDenied, contract.ErrNotFound, contract.ErrRevisionConflict, contract.ErrEvidenceConflict, contract.ErrNotCurrent, contract.ErrCandidateRejected, contract.ErrUnknownOutcome, contract.ErrInspectionIncomplete, contract.ErrSettlementMismatch, contract.ErrContextUnmaterialized, contract.ErrUnsupported} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	if err == nil {
		return "memory-knowledge: internal error"
	}
	if strings.Contains(err.Error(), contextCanceled) {
		return "memory-knowledge: request cancelled"
	}
	return "memory-knowledge: internal error"
}

const contextCanceled = "context canceled"
