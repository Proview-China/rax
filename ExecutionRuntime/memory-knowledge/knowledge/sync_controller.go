package knowledge

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

const PreparedSyncObjectKindV1 = "knowledge_prepared_sync"

type SyncRecordInput struct {
	Candidate      CandidateInput
	OperationRef   contract.Ref
	AttemptID      string
	ExpectedRecord contract.ExpectedRevision
}

type PrepareLocalSyncRequest struct {
	PlanRef         contract.Ref
	Source          SourceInput
	ExpectedSource  contract.ExpectedRevision
	Package         PackageInput
	ExpectedPackage contract.ExpectedRevision
	Records         []SyncRecordInput
	AdmissionTTL    time.Duration
	PreparedID      string
	ExpiresAt       time.Time
}

type PreparedSyncV1 struct {
	ContractVersion string                      `json:"contract_version"`
	ObjectKind      string                      `json:"object_kind"`
	Ref             contract.Ref                `json:"ref"`
	Owner           contract.OwnerDomain        `json:"owner"`
	TenantID        string                      `json:"tenant_id"`
	PlanRef         contract.Ref                `json:"plan_ref"`
	SourceRef       contract.Ref                `json:"source_ref"`
	PackageRef      contract.Ref                `json:"package_ref"`
	CandidateRefs   []contract.Ref              `json:"candidate_refs"`
	AdmissionRefs   []contract.Ref              `json:"admission_refs"`
	AttemptRefs     []contract.Ref              `json:"attempt_refs"`
	DomainResults   []contract.DomainResultFact `json:"domain_results"`
	RecordRefs      []contract.Ref              `json:"record_refs"`
	CreatedAt       time.Time                   `json:"created_at"`
	ExpiresAt       time.Time                   `json:"expires_at"`
	Digest          string                      `json:"digest"`
}

type FinalizeLocalSyncRequest struct {
	Prepared         PreparedSyncV1
	Projections      []ProjectionInput
	Snapshot         SnapshotInput
	ExpectedSnapshot contract.ExpectedRevision
	ExpectedPointer  contract.ExpectedRevision
}

type FinalizedSyncV1 struct {
	PreparedRef          contract.Ref   `json:"prepared_ref"`
	SettlementRefs       []contract.Ref `json:"settlement_refs"`
	ProjectionRefs       []contract.Ref `json:"projection_refs"`
	ReadySnapshotRef     contract.Ref   `json:"ready_snapshot_ref"`
	PointerRef           contract.Ref   `json:"pointer_ref"`
	PublishedSnapshotRef contract.Ref   `json:"published_snapshot_ref"`
}

// PrepareLocalSync consumes already-local normalized inputs. Acquire/remote
// connector execution is deliberately outside this method and remains governed.
// It stops at DomainResult; Runtime settlement is not fabricated here.
func (s *Store) PrepareLocalSync(access Access, request PrepareLocalSyncRequest) (PreparedSyncV1, error) {
	if err := access.Validate(); err != nil {
		return PreparedSyncV1{}, err
	}
	if request.PlanRef.Validate() != nil || request.ExpectedSource.Validate() != nil || request.ExpectedPackage.Validate() != nil || strings.TrimSpace(request.PreparedID) == "" || len(request.Records) == 0 || request.AdmissionTTL <= 0 || !request.ExpiresAt.After(s.clock.Now()) {
		return PreparedSyncV1{}, contract.ErrInvalidArgument
	}
	source, err := s.RegisterSource(access, request.Source, request.ExpectedSource)
	if err != nil {
		return PreparedSyncV1{}, fmt.Errorf("sync register source: %w", err)
	}
	pkgInput := request.Package
	pkgInput.SourceRefs = []contract.Ref{source.Ref}
	pkg, err := s.PutPackage(access, pkgInput, request.ExpectedPackage)
	if err != nil {
		return PreparedSyncV1{}, fmt.Errorf("sync package: %w", err)
	}
	prepared := PreparedSyncV1{ContractVersion: SyncContractVersionV1, ObjectKind: PreparedSyncObjectKindV1, Ref: contract.Ref{ID: request.PreparedID, Revision: 1}, Owner: contract.OwnerKnowledge, TenantID: access.TenantID, PlanRef: request.PlanRef, SourceRef: source.Ref, PackageRef: pkg.Ref, CreatedAt: s.clock.Now().UTC(), ExpiresAt: request.ExpiresAt.UTC()}
	for _, recordInput := range request.Records {
		candidateInput := recordInput.Candidate
		candidateInput.TenantID = access.TenantID
		candidateInput.Draft.PackageRef = pkg.Ref
		candidateInput.Draft.SourceRefs = contract.NormalizeRefs(append(append([]contract.Ref{}, candidateInput.Draft.SourceRefs...), source.Ref))
		candidate, err := s.SubmitCandidate(access, candidateInput, contract.ExpectAbsent())
		if err != nil {
			return PreparedSyncV1{}, fmt.Errorf("sync candidate %s: %w", candidateInput.ID, err)
		}
		admission, err := s.AdmitCandidate(access, candidate.Ref, AdmissionCommitReady, "knowledge sync validated", request.AdmissionTTL, contract.ExpectAbsent())
		if err != nil {
			return PreparedSyncV1{}, fmt.Errorf("sync admission %s: %w", candidate.Ref.ID, err)
		}
		attempt, err := s.BeginCommit(access, CommitRequest{TenantID: access.TenantID, AttemptID: recordInput.AttemptID, OperationRef: recordInput.OperationRef, CandidateRef: candidate.Ref, AdmissionRef: admission.Ref, ExpectedRecord: recordInput.ExpectedRecord})
		if err != nil {
			return PreparedSyncV1{}, fmt.Errorf("sync begin %s: %w", recordInput.AttemptID, err)
		}
		result, err := s.CommitAttempt(access, attempt.Ref.ID)
		if errors.Is(err, contract.ErrUnknownOutcome) {
			_, inspected, inspectErr := s.InspectCommit(access, attempt.Ref.ID, recordInput.OperationRef)
			if inspectErr != nil || inspected == nil {
				return PreparedSyncV1{}, contract.ErrUnknownOutcome
			}
			result = *inspected
			err = nil
		}
		if err != nil {
			return PreparedSyncV1{}, fmt.Errorf("sync commit %s: %w", attempt.Ref.ID, err)
		}
		prepared.CandidateRefs = append(prepared.CandidateRefs, candidate.Ref)
		prepared.AdmissionRefs = append(prepared.AdmissionRefs, admission.Ref)
		prepared.AttemptRefs = append(prepared.AttemptRefs, attempt.Ref)
		prepared.DomainResults = append(prepared.DomainResults, result)
		prepared.RecordRefs = append(prepared.RecordRefs, result.SubjectRef)
	}
	return sealPreparedSync(prepared)
}

// FinalizeLocalSync first proves every DomainResult has an Owner-applied Runtime
// settlement. Only then are projections/snapshot built and current published.
func (s *Store) FinalizeLocalSync(access Access, request FinalizeLocalSyncRequest) (FinalizedSyncV1, error) {
	if err := access.Validate(); err != nil {
		return FinalizedSyncV1{}, err
	}
	if err := request.Prepared.Validate(s.clock.Now()); err != nil {
		return FinalizedSyncV1{}, err
	}
	if request.Prepared.TenantID != access.TenantID || request.ExpectedSnapshot.Validate() != nil || request.ExpectedPointer.Validate() != nil {
		return FinalizedSyncV1{}, contract.ErrScopeDenied
	}
	result := FinalizedSyncV1{PreparedRef: request.Prepared.Ref}
	for _, domainResult := range request.Prepared.DomainResults {
		application, err := s.InspectSettlement(access, domainResult.Ref)
		if err != nil {
			return FinalizedSyncV1{}, fmt.Errorf("sync settlement %s: %w", domainResult.Ref.ID, err)
		}
		result.SettlementRefs = append(result.SettlementRefs, application.Ref)
	}
	for _, input := range request.Projections {
		input.TenantID = access.TenantID
		input.RecordRefs = slices.Clone(request.Prepared.RecordRefs)
		projection, err := s.PutProjection(access, input, contract.ExpectAbsent())
		if err != nil {
			return FinalizedSyncV1{}, fmt.Errorf("sync projection %s: %w", input.ID, err)
		}
		result.ProjectionRefs = append(result.ProjectionRefs, projection.Ref)
	}
	snapshotInput := request.Snapshot
	snapshotInput.TenantID = access.TenantID
	snapshotInput.SourceRefs = []contract.Ref{request.Prepared.SourceRef}
	snapshotInput.PackageRefs = []contract.Ref{request.Prepared.PackageRef}
	snapshotInput.RecordRefs = slices.Clone(request.Prepared.RecordRefs)
	snapshotInput.ProjectionRefs = slices.Clone(result.ProjectionRefs)
	ready, err := s.CreateSnapshot(access, snapshotInput, request.ExpectedSnapshot)
	if err != nil {
		return FinalizedSyncV1{}, fmt.Errorf("sync snapshot: %w", err)
	}
	pointer, published, err := s.PublishSnapshot(access, ready.Ref, request.ExpectedPointer)
	if err != nil {
		return FinalizedSyncV1{}, fmt.Errorf("sync publish: %w", err)
	}
	result.SettlementRefs = contract.NormalizeRefs(result.SettlementRefs)
	result.ProjectionRefs = contract.NormalizeRefs(result.ProjectionRefs)
	result.ReadySnapshotRef = ready.Ref
	result.PointerRef = pointer.Ref
	result.PublishedSnapshotRef = published.Ref
	return result, nil
}

func (s *Store) InspectSettlement(access Access, resultRef contract.Ref) (contract.SettlementApplication, error) {
	if err := access.Validate(); err != nil {
		return contract.SettlementApplication{}, err
	}
	if err := resultRef.Validate(); err != nil {
		return contract.SettlementApplication{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return contract.SettlementApplication{}, err
	}
	application, ok := t.settlements[resultRef.ID]
	if !ok {
		return contract.SettlementApplication{}, contract.ErrNotFound
	}
	resultFact, ok := t.results[resultRef.ID]
	if !ok || !contract.SameRef(application.DomainResultRef, resultFact.Ref) {
		return contract.SettlementApplication{}, contract.ErrEvidenceConflict
	}
	record, ok := recordByRef(t, resultFact.SubjectRef)
	if !ok || !accessAllows(access, record.AuthorityRef, record.PolicyRef) {
		return contract.SettlementApplication{}, contract.ErrScopeDenied
	}
	if err := application.Validate(); err != nil {
		return contract.SettlementApplication{}, err
	}
	return application, nil
}

func sealPreparedSync(in PreparedSyncV1) (PreparedSyncV1, error) {
	in.CandidateRefs = contract.NormalizeRefs(in.CandidateRefs)
	in.AdmissionRefs = contract.NormalizeRefs(in.AdmissionRefs)
	in.AttemptRefs = contract.NormalizeRefs(in.AttemptRefs)
	in.RecordRefs = contract.NormalizeRefs(in.RecordRefs)
	slices.SortFunc(in.DomainResults, func(a, b contract.DomainResultFact) int { return strings.Compare(a.Ref.ID, b.Ref.ID) })
	in.Ref.Digest, in.Digest = "", ""
	d, e := contract.Digest(in)
	if e != nil {
		return PreparedSyncV1{}, e
	}
	in.Ref.Digest, in.Digest = d, d
	if e = in.Validate(in.CreatedAt); e != nil {
		return PreparedSyncV1{}, e
	}
	return in, nil
}
func (in PreparedSyncV1) Validate(now time.Time) error {
	if in.ContractVersion != SyncContractVersionV1 || in.ObjectKind != PreparedSyncObjectKindV1 || in.Owner != contract.OwnerKnowledge || in.Ref.Validate() != nil || strings.TrimSpace(in.TenantID) == "" || in.PlanRef.Validate() != nil || in.SourceRef.Validate() != nil || in.PackageRef.Validate() != nil || len(in.DomainResults) == 0 || len(in.RecordRefs) != len(in.DomainResults) || in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: prepared sync", contract.ErrInvalidArgument)
	}
	for i := range in.DomainResults {
		if err := in.DomainResults[i].Validate(); err != nil {
			return err
		}
		if !containsRef(in.RecordRefs, in.DomainResults[i].SubjectRef) {
			return fmt.Errorf("%w: prepared result subject", contract.ErrEvidenceConflict)
		}
	}
	c := in
	c.Ref.Digest, c.Digest = "", ""
	d, e := contract.Digest(c)
	if e != nil {
		return e
	}
	if d != in.Digest || in.Ref.Digest != d {
		return fmt.Errorf("%w: prepared sync digest", contract.ErrEvidenceConflict)
	}
	return nil
}
