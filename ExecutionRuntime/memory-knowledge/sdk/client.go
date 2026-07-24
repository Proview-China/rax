// Package sdk is the public Go boundary for Memory and Knowledge owners. It
// delegates to versioned owner methods and never exposes store maps.
package sdk

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/knowledge"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/memory"
)

type MemoryBackend interface {
	SubmitCandidate(memory.Access, memory.Candidate) (memory.Candidate, error)
	Admit(memory.Access, memory.AdmissionRequest) (memory.AdmissionFact, error)
	Commit(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error)
	Correct(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error)
	Pin(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error)
	Archive(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error)
	Forget(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error)
	Merge(memory.Access, memory.CommitRequest) (memory.Record, contract.DomainResultFact, error)
	InspectRecord(memory.Access, contract.Ref) (memory.Record, error)
	Query(memory.Access, contract.RetrievalQuery) (contract.RetrievalResult, error)
	InspectJob(memory.Access, contract.Ref) (contract.OwnerJobAttemptV1, error)
	ListProjections(memory.Access) ([]memory.Projection, error)
	ExportView(memory.Access, contract.Ref, string, time.Duration) (contract.ExportManifestV1, error)
	ReindexLocal(memory.Access, memory.Projection, contract.ExpectedRevision, contract.IndexDescriptorV1, contract.ExpectedRevision) (memory.Projection, contract.IndexDescriptorV1, error)
	ListIndexDescriptors(memory.Access) ([]contract.IndexDescriptorV1, error)
	WatchChanges(memory.Access, contract.WatchRequestV1) (contract.ChangePageV1, error)
	PublishView(memory.Access, memory.View, contract.ExpectedRevision) (memory.View, error)
	PreparePurge(memory.Access, memory.PurgeRequest) (contract.PurgeIntentV1, error)
	InspectPurge(memory.Access, contract.Ref) (contract.PurgeIntentV1, error)
}

type KnowledgeBackend interface {
	RegisterSource(knowledge.Access, knowledge.SourceInput, contract.ExpectedRevision) (knowledge.Source, error)
	WithdrawSource(knowledge.Access, string, string, contract.ExpectedRevision) (knowledge.Source, knowledge.Tombstone, error)
	DeprecateSource(knowledge.Access, string, string, contract.ExpectedRevision) (knowledge.Source, error)
	GetSource(knowledge.Access, contract.Ref) (knowledge.Source, error)
	CreateSnapshot(knowledge.Access, knowledge.SnapshotInput, contract.ExpectedRevision) (knowledge.Snapshot, error)
	PublishSnapshot(knowledge.Access, contract.Ref, contract.ExpectedRevision) (knowledge.SnapshotPointer, knowledge.Snapshot, error)
	Query(knowledge.Access, contract.RetrievalQuery, knowledge.ContentReader) (contract.RetrievalResult, error)
	InspectJob(knowledge.Access, contract.Ref) (contract.OwnerJobAttemptV1, error)
	PrepareLocalSync(knowledge.Access, knowledge.PrepareLocalSyncRequest) (knowledge.PreparedSyncV1, error)
	FinalizeLocalSync(knowledge.Access, knowledge.FinalizeLocalSyncRequest) (knowledge.FinalizedSyncV1, error)
	ListSources(knowledge.Access) ([]knowledge.Source, error)
	ListProjections(knowledge.Access) ([]knowledge.Projection, error)
	ExportView(knowledge.Access, contract.Ref, string, time.Duration) (contract.ExportManifestV1, error)
	ReindexLocal(knowledge.Access, knowledge.ProjectionInput, contract.ExpectedRevision, contract.IndexDescriptorV1, contract.ExpectedRevision) (knowledge.Projection, contract.IndexDescriptorV1, error)
	ListIndexDescriptors(knowledge.Access) ([]contract.IndexDescriptorV1, error)
	WatchChanges(knowledge.Access, contract.WatchRequestV1) (contract.ChangePageV1, error)
	SubmitCandidate(knowledge.Access, knowledge.CandidateInput, contract.ExpectedRevision) (knowledge.Candidate, error)
	AdmitCandidate(knowledge.Access, contract.Ref, knowledge.AdmissionDecision, string, time.Duration, contract.ExpectedRevision) (knowledge.Admission, error)
	BeginCommit(knowledge.Access, knowledge.CommitRequest) (knowledge.CommitAttempt, error)
	CommitAttempt(knowledge.Access, string) (contract.DomainResultFact, error)
	InspectCommit(knowledge.Access, string, contract.Ref) (knowledge.Inspection, *contract.DomainResultFact, error)
	GetRecord(knowledge.Access, contract.Ref) (knowledge.Record, error)
	CreateView(knowledge.Access, knowledge.ViewInput, contract.ExpectedRevision) (knowledge.View, error)
	PreparePurge(knowledge.Access, knowledge.PurgeRequest) (contract.PurgeIntentV1, error)
	InspectPurge(knowledge.Access, contract.Ref) (contract.PurgeIntentV1, error)
}

type MemoryClient struct{ backend MemoryBackend }
type KnowledgeClient struct {
	backend KnowledgeBackend
	content knowledge.ContentReader
}

func NewMemory(backend MemoryBackend) (*MemoryClient, error) {
	if backend == nil {
		return nil, contract.ErrInvalidArgument
	}
	return &MemoryClient{backend: backend}, nil
}
func NewKnowledge(backend KnowledgeBackend, content knowledge.ContentReader) (*KnowledgeClient, error) {
	if backend == nil {
		return nil, contract.ErrInvalidArgument
	}
	return &KnowledgeClient{backend: backend, content: content}, nil
}

type MemoryWriteRequest struct {
	Candidate memory.Candidate
	Admission memory.AdmissionRequest
	Commit    memory.CommitRequest
}
type MemoryWriteResult struct {
	Candidate    memory.Candidate          `json:"candidate"`
	Admission    memory.AdmissionFact      `json:"admission"`
	Record       memory.Record             `json:"record"`
	DomainResult contract.DomainResultFact `json:"domain_result"`
}

func (c *MemoryClient) SubmitCandidate(ctx context.Context, access memory.Access, candidate memory.Candidate) (memory.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return memory.Candidate{}, err
	}
	return c.backend.SubmitCandidate(access, candidate)
}
func (c *MemoryClient) ReviewCandidate(ctx context.Context, access memory.Access, request memory.AdmissionRequest) (memory.AdmissionFact, error) {
	if err := ctx.Err(); err != nil {
		return memory.AdmissionFact{}, err
	}
	return c.backend.Admit(access, request)
}
func (c *MemoryClient) Commit(ctx context.Context, access memory.Access, request memory.CommitRequest) (memory.Record, contract.DomainResultFact, error) {
	if err := ctx.Err(); err != nil {
		return memory.Record{}, contract.DomainResultFact{}, err
	}
	return c.backend.Commit(access, request)
}
func (c *MemoryClient) Correct(ctx context.Context, access memory.Access, request memory.CommitRequest) (memory.Record, contract.DomainResultFact, error) {
	if err := ctx.Err(); err != nil {
		return memory.Record{}, contract.DomainResultFact{}, err
	}
	return c.backend.Correct(access, request)
}

func (c *MemoryClient) Write(ctx context.Context, access memory.Access, request MemoryWriteRequest) (MemoryWriteResult, error) {
	return c.writeKind(ctx, access, request, memory.CandidateCreate)
}
func (c *MemoryClient) Forget(ctx context.Context, access memory.Access, request MemoryWriteRequest) (MemoryWriteResult, error) {
	return c.writeKind(ctx, access, request, memory.CandidateForget)
}
func (c *MemoryClient) Merge(ctx context.Context, access memory.Access, request MemoryWriteRequest) (MemoryWriteResult, error) {
	return c.writeKind(ctx, access, request, memory.CandidateMerge)
}
func (c *MemoryClient) CorrectWrite(ctx context.Context, access memory.Access, request MemoryWriteRequest) (MemoryWriteResult, error) {
	return c.writeKind(ctx, access, request, memory.CandidateCorrection)
}
func (c *MemoryClient) Pin(ctx context.Context, access memory.Access, request MemoryWriteRequest) (MemoryWriteResult, error) {
	return c.writeKind(ctx, access, request, memory.CandidatePin)
}
func (c *MemoryClient) Archive(ctx context.Context, access memory.Access, request MemoryWriteRequest) (MemoryWriteResult, error) {
	return c.writeKind(ctx, access, request, memory.CandidateArchive)
}
func (c *MemoryClient) writeKind(ctx context.Context, access memory.Access, request MemoryWriteRequest, kind memory.CandidateKind) (MemoryWriteResult, error) {
	if request.Candidate.Kind != kind {
		return MemoryWriteResult{}, contract.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return MemoryWriteResult{}, err
	}
	candidate, err := c.backend.SubmitCandidate(access, request.Candidate)
	if err != nil {
		return MemoryWriteResult{}, err
	}
	request.Admission.CandidateRef = candidate.Ref()
	admission, err := c.backend.Admit(access, request.Admission)
	if err != nil {
		return MemoryWriteResult{}, err
	}
	request.Commit.CandidateRef = candidate.Ref()
	request.Commit.AdmissionRef = admission.Ref
	var record memory.Record
	var result contract.DomainResultFact
	switch kind {
	case memory.CandidateCreate:
		record, result, err = c.backend.Commit(access, request.Commit)
	case memory.CandidateCorrection:
		record, result, err = c.backend.Correct(access, request.Commit)
	case memory.CandidatePin:
		record, result, err = c.backend.Pin(access, request.Commit)
	case memory.CandidateArchive:
		record, result, err = c.backend.Archive(access, request.Commit)
	case memory.CandidateForget:
		record, result, err = c.backend.Forget(access, request.Commit)
	case memory.CandidateMerge:
		record, result, err = c.backend.Merge(access, request.Commit)
	default:
		return MemoryWriteResult{}, contract.ErrInvalidArgument
	}
	return MemoryWriteResult{Candidate: candidate, Admission: admission, Record: record, DomainResult: result}, err
}
func (c *MemoryClient) Query(ctx context.Context, access memory.Access, query contract.RetrievalQuery) (contract.RetrievalResult, error) {
	if err := ctx.Err(); err != nil {
		return contract.RetrievalResult{}, err
	}
	return c.backend.Query(access, query)
}
func (c *MemoryClient) Inspect(ctx context.Context, access memory.Access, ref contract.Ref) (memory.Record, error) {
	if err := ctx.Err(); err != nil {
		return memory.Record{}, err
	}
	return c.backend.InspectRecord(access, ref)
}
func (c *MemoryClient) InspectJob(ctx context.Context, access memory.Access, ref contract.Ref) (contract.OwnerJobAttemptV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.OwnerJobAttemptV1{}, err
	}
	return c.backend.InspectJob(access, ref)
}
func (c *MemoryClient) ListProjections(ctx context.Context, access memory.Access) ([]memory.Projection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.backend.ListProjections(access)
}
func (c *MemoryClient) Export(ctx context.Context, access memory.Access, view contract.Ref, id string, ttl time.Duration) (contract.ExportManifestV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ExportManifestV1{}, err
	}
	return c.backend.ExportView(access, view, id, ttl)
}
func (c *MemoryClient) ListIndexes(ctx context.Context, access memory.Access) ([]contract.IndexDescriptorV1, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.backend.ListIndexDescriptors(access)
}
func (c *MemoryClient) Reindex(ctx context.Context, access memory.Access, projection memory.Projection, expectedProjection contract.ExpectedRevision, descriptor contract.IndexDescriptorV1, expectedDescriptor contract.ExpectedRevision) (memory.Projection, contract.IndexDescriptorV1, error) {
	if err := ctx.Err(); err != nil {
		return memory.Projection{}, contract.IndexDescriptorV1{}, err
	}
	return c.backend.ReindexLocal(access, projection, expectedProjection, descriptor, expectedDescriptor)
}
func (c *MemoryClient) Watch(ctx context.Context, access memory.Access, request contract.WatchRequestV1) (contract.ChangePageV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ChangePageV1{}, err
	}
	return c.backend.WatchChanges(access, request)
}
func (c *MemoryClient) PublishView(ctx context.Context, access memory.Access, view memory.View, expected contract.ExpectedRevision) (memory.View, error) {
	if err := ctx.Err(); err != nil {
		return memory.View{}, err
	}
	return c.backend.PublishView(access, view, expected)
}
func (c *MemoryClient) PreparePurge(ctx context.Context, access memory.Access, request memory.PurgeRequest) (contract.PurgeIntentV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.PurgeIntentV1{}, err
	}
	return c.backend.PreparePurge(access, request)
}
func (c *MemoryClient) InspectPurge(ctx context.Context, access memory.Access, exact contract.Ref) (contract.PurgeIntentV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.PurgeIntentV1{}, err
	}
	return c.backend.InspectPurge(access, exact)
}

func (c *KnowledgeClient) RegisterSource(ctx context.Context, access knowledge.Access, input knowledge.SourceInput, expected contract.ExpectedRevision) (knowledge.Source, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.Source{}, err
	}
	return c.backend.RegisterSource(access, input, expected)
}
func (c *KnowledgeClient) InspectSource(ctx context.Context, access knowledge.Access, exact contract.Ref) (knowledge.Source, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.Source{}, err
	}
	return c.backend.GetSource(access, exact)
}
func (c *KnowledgeClient) SubmitCandidate(ctx context.Context, access knowledge.Access, input knowledge.CandidateInput, expected contract.ExpectedRevision) (knowledge.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.Candidate{}, err
	}
	return c.backend.SubmitCandidate(access, input, expected)
}
func (c *KnowledgeClient) ReviewCandidate(ctx context.Context, access knowledge.Access, candidate contract.Ref, decision knowledge.AdmissionDecision, reason string, ttl time.Duration, expected contract.ExpectedRevision) (knowledge.Admission, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.Admission{}, err
	}
	return c.backend.AdmitCandidate(access, candidate, decision, reason, ttl, expected)
}
func (c *KnowledgeClient) BeginCommit(ctx context.Context, access knowledge.Access, request knowledge.CommitRequest) (knowledge.CommitAttempt, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.CommitAttempt{}, err
	}
	return c.backend.BeginCommit(access, request)
}
func (c *KnowledgeClient) Commit(ctx context.Context, access knowledge.Access, attemptID string) (contract.DomainResultFact, error) {
	if err := ctx.Err(); err != nil {
		return contract.DomainResultFact{}, err
	}
	return c.backend.CommitAttempt(access, attemptID)
}
func (c *KnowledgeClient) InspectCommit(ctx context.Context, access knowledge.Access, attemptID string, operation contract.Ref) (knowledge.Inspection, *contract.DomainResultFact, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.Inspection{}, nil, err
	}
	return c.backend.InspectCommit(access, attemptID, operation)
}
func (c *KnowledgeClient) InspectRecord(ctx context.Context, access knowledge.Access, exact contract.Ref) (knowledge.Record, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.Record{}, err
	}
	return c.backend.GetRecord(access, exact)
}
func (c *KnowledgeClient) WithdrawSource(ctx context.Context, access knowledge.Access, id, reason string, expected contract.ExpectedRevision) (knowledge.Source, knowledge.Tombstone, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.Source{}, knowledge.Tombstone{}, err
	}
	return c.backend.WithdrawSource(access, id, reason, expected)
}
func (c *KnowledgeClient) DeprecateSource(ctx context.Context, access knowledge.Access, id, reason string, expected contract.ExpectedRevision) (knowledge.Source, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.Source{}, err
	}
	return c.backend.DeprecateSource(access, id, reason, expected)
}
func (c *KnowledgeClient) Query(ctx context.Context, access knowledge.Access, query contract.RetrievalQuery) (contract.RetrievalResult, error) {
	if err := ctx.Err(); err != nil {
		return contract.RetrievalResult{}, err
	}
	if c.content == nil {
		return contract.RetrievalResult{}, contract.ErrNotFound
	}
	return c.backend.Query(access, query, c.content)
}
func (c *KnowledgeClient) BuildSnapshot(ctx context.Context, access knowledge.Access, input knowledge.SnapshotInput, expectedSnapshot contract.ExpectedRevision, expectedPointer contract.ExpectedRevision) (knowledge.SnapshotPointer, knowledge.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.SnapshotPointer{}, knowledge.Snapshot{}, err
	}
	ready, err := c.backend.CreateSnapshot(access, input, expectedSnapshot)
	if err != nil {
		return knowledge.SnapshotPointer{}, knowledge.Snapshot{}, err
	}
	return c.backend.PublishSnapshot(access, ready.Ref, expectedPointer)
}
func (c *KnowledgeClient) PrepareSync(ctx context.Context, access knowledge.Access, request knowledge.PrepareLocalSyncRequest) (knowledge.PreparedSyncV1, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.PreparedSyncV1{}, err
	}
	return c.backend.PrepareLocalSync(access, request)
}
func (c *KnowledgeClient) FinalizeSync(ctx context.Context, access knowledge.Access, request knowledge.FinalizeLocalSyncRequest) (knowledge.FinalizedSyncV1, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.FinalizedSyncV1{}, err
	}
	return c.backend.FinalizeLocalSync(access, request)
}
func (c *KnowledgeClient) InspectJob(ctx context.Context, access knowledge.Access, ref contract.Ref) (contract.OwnerJobAttemptV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.OwnerJobAttemptV1{}, err
	}
	return c.backend.InspectJob(access, ref)
}
func (c *KnowledgeClient) ListSources(ctx context.Context, access knowledge.Access) ([]knowledge.Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.backend.ListSources(access)
}
func (c *KnowledgeClient) ListProjections(ctx context.Context, access knowledge.Access) ([]knowledge.Projection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.backend.ListProjections(access)
}
func (c *KnowledgeClient) Export(ctx context.Context, access knowledge.Access, view contract.Ref, id string, ttl time.Duration) (contract.ExportManifestV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ExportManifestV1{}, err
	}
	return c.backend.ExportView(access, view, id, ttl)
}
func (c *KnowledgeClient) ListIndexes(ctx context.Context, access knowledge.Access) ([]contract.IndexDescriptorV1, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.backend.ListIndexDescriptors(access)
}
func (c *KnowledgeClient) Reindex(ctx context.Context, access knowledge.Access, projection knowledge.ProjectionInput, expectedProjection contract.ExpectedRevision, descriptor contract.IndexDescriptorV1, expectedDescriptor contract.ExpectedRevision) (knowledge.Projection, contract.IndexDescriptorV1, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.Projection{}, contract.IndexDescriptorV1{}, err
	}
	return c.backend.ReindexLocal(access, projection, expectedProjection, descriptor, expectedDescriptor)
}
func (c *KnowledgeClient) Watch(ctx context.Context, access knowledge.Access, request contract.WatchRequestV1) (contract.ChangePageV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ChangePageV1{}, err
	}
	return c.backend.WatchChanges(access, request)
}
func (c *KnowledgeClient) PublishView(ctx context.Context, access knowledge.Access, input knowledge.ViewInput, expected contract.ExpectedRevision) (knowledge.View, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.View{}, err
	}
	return c.backend.CreateView(access, input, expected)
}
func (c *KnowledgeClient) PreparePurge(ctx context.Context, access knowledge.Access, request knowledge.PurgeRequest) (contract.PurgeIntentV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.PurgeIntentV1{}, err
	}
	return c.backend.PreparePurge(access, request)
}
func (c *KnowledgeClient) InspectPurge(ctx context.Context, access knowledge.Access, exact contract.Ref) (contract.PurgeIntentV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.PurgeIntentV1{}, err
	}
	return c.backend.InspectPurge(access, exact)
}
