package memory

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	projectioncatalog "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection"
)

type ContentReader interface {
	Get(contract.ContentRef) ([]byte, error)
}

type tenantState struct {
	watermark        uint64
	candidates       map[string]Candidate
	candidateSources map[string]string
	admissions       map[string][]AdmissionFact
	records          map[string][]Record
	attempts         map[string]attempt
	results          map[string]contract.DomainResultFact
	settled          map[string]contract.SettlementApplication
	settlementUses   map[string]contract.Ref
	views            map[string][]View
	projections      map[string][]Projection
	jobs             map[string][]contract.OwnerJobAttemptV1
	mergeFacts       map[string]MergeFact
	mergedInto       map[string]contract.Ref
	purgeIntents     map[string]contract.PurgeIntentV1
}

type attempt struct {
	request    CommitRequest
	inspection CommitInspection
	result     contract.DomainResultFact
}

type Store struct {
	mu      sync.RWMutex
	clock   contract.Clock
	content ContentReader
	indexes *projectioncatalog.Catalog
	tenants map[string]*tenantState
	// afterCAS is an in-package fault-injection seam. It is intentionally not
	// exported and cannot become a production backend or external-effect API.
	afterCAS func(CommitInspection) error
}

func NewStore(clock contract.Clock, content ContentReader) *Store {
	if clock == nil {
		clock = contract.SystemClock{}
	}
	return &Store{clock: clock, content: content, indexes: projectioncatalog.NewCatalog(), tenants: make(map[string]*tenantState)}
}

func newTenantState() *tenantState {
	return &tenantState{watermark: 1, candidates: make(map[string]Candidate), candidateSources: make(map[string]string), admissions: make(map[string][]AdmissionFact), records: make(map[string][]Record), attempts: make(map[string]attempt), results: make(map[string]contract.DomainResultFact), settled: make(map[string]contract.SettlementApplication), settlementUses: make(map[string]contract.Ref), views: make(map[string][]View), projections: make(map[string][]Projection), jobs: make(map[string][]contract.OwnerJobAttemptV1), mergeFacts: make(map[string]MergeFact), mergedInto: make(map[string]contract.Ref), purgeIntents: make(map[string]contract.PurgeIntentV1)}
}

func (s *Store) tenant(id string) *tenantState {
	t := s.tenants[id]
	if t == nil {
		t = newTenantState()
		s.tenants[id] = t
	}
	return t
}

func candidateKey(ref contract.Ref) string { return fmt.Sprintf("%s@%d", ref.ID, ref.Revision) }
func candidateSourceKey(c Candidate) string {
	return fmt.Sprintf("%s@%d:%d:%d", c.ProducerRef.ID, c.ProducerRef.Revision, c.SourceEpoch, c.SourceSequence)
}

func (s *Store) SubmitCandidate(access Access, candidate Candidate) (Candidate, error) {
	if err := access.validate(); err != nil {
		return Candidate{}, err
	}
	if candidate.Envelope.TenantID != access.TenantID || candidate.Envelope.IdentityID != access.IdentityID {
		return Candidate{}, contract.ErrScopeDenied
	}
	if candidate.Envelope.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(candidate.Envelope.AuthorityRef, access.AuthorityRef) || !contract.SameRef(candidate.Envelope.PolicyRef, access.PolicyRef) {
		return Candidate{}, contract.ErrNotCurrent
	}
	if err := candidate.Validate(s.clock.Now()); err != nil {
		return Candidate{}, err
	}
	if candidate.ContentRef != nil {
		if s.content == nil {
			return Candidate{}, contract.ErrNotFound
		}
		if _, err := s.content.Get(*candidate.ContentRef); err != nil {
			return Candidate{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tenant(access.TenantID)
	key := candidateKey(candidate.Ref())
	sourceKey := candidateSourceKey(candidate)
	if existingKey, ok := t.candidateSources[sourceKey]; ok {
		existing := t.candidates[existingKey]
		if contract.SameRef(existing.Ref(), candidate.Ref()) {
			return cloneCandidate(existing), nil
		}
		return Candidate{}, contract.ErrEvidenceConflict
	}
	if existing, ok := t.candidates[key]; ok {
		if contract.SameRef(existing.Ref(), candidate.Ref()) {
			return cloneCandidate(existing), nil
		}
		return Candidate{}, contract.ErrEvidenceConflict
	}
	t.candidates[key] = cloneCandidate(candidate)
	t.candidateSources[sourceKey] = key
	return cloneCandidate(candidate), nil
}

func (s *Store) Admit(access Access, request AdmissionRequest) (AdmissionFact, error) {
	if err := access.validate(); err != nil {
		return AdmissionFact{}, err
	}
	if strings.TrimSpace(request.ID) == "" || request.CandidateRef.Validate() != nil || !request.ExpiresAt.After(s.clock.Now()) {
		return AdmissionFact{}, fmt.Errorf("%w: incomplete admission", contract.ErrInvalidArgument)
	}
	if err := request.ExpectedRevision.Validate(); err != nil {
		return AdmissionFact{}, err
	}
	switch request.Decision {
	case AdmissionRejected, AdmissionReviewRequired, AdmissionCommitReady:
		if request.MergeTarget.ID != "" {
			return AdmissionFact{}, fmt.Errorf("%w: unexpected merge target", contract.ErrInvalidArgument)
		}
	case AdmissionMerged:
		if err := request.MergeTarget.Validate(); err != nil {
			return AdmissionFact{}, err
		}
	default:
		return AdmissionFact{}, fmt.Errorf("%w: invalid admission decision", contract.ErrInvalidArgument)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tenant(access.TenantID)
	c, ok := t.candidates[candidateKey(request.CandidateRef)]
	if !ok || !contract.SameRef(c.Ref(), request.CandidateRef) || c.Envelope.IdentityID != access.IdentityID {
		return AdmissionFact{}, contract.ErrNotFound
	}
	if c.Envelope.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(c.Envelope.AuthorityRef, access.AuthorityRef) || !contract.SameRef(c.Envelope.PolicyRef, access.PolicyRef) {
		return AdmissionFact{}, contract.ErrNotCurrent
	}
	if (request.Decision == AdmissionCommitReady || request.Decision == AdmissionMerged) && hasBlockingRisk(c.RiskFlags) {
		return AdmissionFact{}, contract.ErrCandidateRejected
	}
	versions := t.admissions[request.ID]
	exists := len(versions) > 0
	current := uint64(0)
	if exists {
		current = versions[len(versions)-1].Ref.Revision
	}
	if !request.ExpectedRevision.Matches(exists, current) {
		return AdmissionFact{}, contract.ErrRevisionConflict
	}
	if exists {
		previous := versions[len(versions)-1]
		if !contract.SameRef(previous.CandidateRef, request.CandidateRef) {
			return AdmissionFact{}, contract.ErrEvidenceConflict
		}
		if previous.Decision != AdmissionReviewRequired || request.Decision != AdmissionCommitReady && request.Decision != AdmissionRejected {
			return AdmissionFact{}, fmt.Errorf("%w: illegal admission transition", contract.ErrRevisionConflict)
		}
	}
	fact := AdmissionFact{Ref: contract.Ref{ID: request.ID, Revision: current + 1}, Owner: contract.OwnerMemory, TenantID: access.TenantID, CandidateRef: request.CandidateRef, Decision: request.Decision, MergeTarget: request.MergeTarget, Reason: request.Reason, CreatedAt: s.clock.Now().UTC(), ExpiresAt: request.ExpiresAt.UTC()}
	digest, err := contract.Digest(fact)
	if err != nil {
		return AdmissionFact{}, fmt.Errorf("canonical memory admission: %w", err)
	}
	fact.Ref.Digest = digest
	t.admissions[request.ID] = append(versions, fact)
	return fact, nil
}

func (s *Store) Commit(access Access, request CommitRequest) (Record, contract.DomainResultFact, error) {
	if err := access.validate(); err != nil {
		return Record{}, contract.DomainResultFact{}, err
	}
	if request.TenantID != access.TenantID || strings.TrimSpace(request.AttemptID) == "" || strings.TrimSpace(request.ResultID) == "" || strings.TrimSpace(request.RecordID) == "" {
		return Record{}, contract.DomainResultFact{}, contract.ErrScopeDenied
	}
	if err := request.ExpectedRevision.Validate(); err != nil {
		return Record{}, contract.DomainResultFact{}, err
	}
	for _, ref := range []contract.Ref{request.CandidateRef, request.AdmissionRef, request.OperationRef} {
		if err := ref.Validate(); err != nil {
			return Record{}, contract.DomainResultFact{}, err
		}
	}
	s.mu.Lock()
	t := s.tenant(access.TenantID)
	if existing, ok := t.attempts[request.AttemptID]; ok {
		existingDigest, err := contract.Digest(existing.request)
		if err != nil {
			s.mu.Unlock()
			return Record{}, contract.DomainResultFact{}, fmt.Errorf("canonical stored memory request: %w", err)
		}
		requestDigest, err := contract.Digest(request)
		if err != nil {
			s.mu.Unlock()
			return Record{}, contract.DomainResultFact{}, fmt.Errorf("canonical memory request: %w", err)
		}
		if existingDigest != requestDigest {
			s.mu.Unlock()
			return Record{}, contract.DomainResultFact{}, contract.ErrEvidenceConflict
		}
		if err := existing.result.Validate(); err != nil {
			s.mu.Unlock()
			return Record{}, contract.DomainResultFact{}, err
		}
		if !attemptAllowed(t, existing, access) {
			s.mu.Unlock()
			return Record{}, contract.DomainResultFact{}, contract.ErrNotFound
		}
		record := findRecordByRef(t, existing.inspection.RecordRef)
		s.mu.Unlock()
		return cloneRecord(record), cloneDomainResult(existing.result), nil
	}
	candidate, ok := t.candidates[candidateKey(request.CandidateRef)]
	if !ok || !contract.SameRef(candidate.Ref(), request.CandidateRef) || candidate.Envelope.IdentityID != access.IdentityID {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, contract.ErrNotFound
	}
	if candidate.Envelope.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(candidate.Envelope.AuthorityRef, access.AuthorityRef) || !contract.SameRef(candidate.Envelope.PolicyRef, access.PolicyRef) {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, contract.ErrNotCurrent
	}
	admissions := t.admissions[request.AdmissionRef.ID]
	if len(admissions) == 0 {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, contract.ErrNotFound
	}
	admission := admissions[len(admissions)-1]
	if !contract.SameRef(admission.Ref, request.AdmissionRef) || !contract.SameRef(admission.CandidateRef, request.CandidateRef) {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, contract.ErrNotCurrent
	}
	if !admission.Current(s.clock.Now()) {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, contract.ErrNotCurrent
	}
	if admission.Decision != AdmissionCommitReady && !(candidate.Kind == CandidateMerge && admission.Decision == AdmissionMerged && contract.SameRef(admission.MergeTarget, candidate.TargetRecordRef)) {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, contract.ErrCandidateRejected
	}
	if _, collision := t.results[request.ResultID]; collision {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, contract.ErrAlreadyExists
	}
	versions := t.records[request.RecordID]
	exists := len(versions) != 0
	currentRevision := uint64(0)
	if exists {
		currentRevision = versions[len(versions)-1].Ref.Revision
		current := versions[len(versions)-1]
		if current.IdentityID != access.IdentityID || current.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(current.AuthorityRef, access.AuthorityRef) || !contract.SameRef(current.PolicyRef, access.PolicyRef) {
			s.mu.Unlock()
			return Record{}, contract.DomainResultFact{}, contract.ErrNotFound
		}
	}
	if !request.ExpectedRevision.Matches(exists, currentRevision) {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, contract.ErrRevisionConflict
	}
	if candidate.Kind == CandidateCreate && exists || candidate.Kind != CandidateCreate && !exists {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, contract.ErrRevisionConflict
	}
	if candidate.Kind != CandidateCreate && (!contract.SameRef(candidate.TargetRecordRef, versions[len(versions)-1].Ref) || candidate.TargetRecordRef.ID != request.RecordID) {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, contract.ErrRevisionConflict
	}
	var mergeFact MergeFact
	if candidate.Kind == CandidateMerge {
		for _, sourceRef := range candidate.MergeSourceRefs {
			source := findRecordByRef(t, sourceRef)
			if source.Ref.ID == "" || source.IdentityID != access.IdentityID || source.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(source.AuthorityRef, access.AuthorityRef) || !contract.SameRef(source.PolicyRef, access.PolicyRef) {
				s.mu.Unlock()
				return Record{}, contract.DomainResultFact{}, contract.ErrNotFound
			}
			latest := t.records[sourceRef.ID]
			if len(latest) == 0 || !contract.SameRef(latest[len(latest)-1].Ref, sourceRef) {
				s.mu.Unlock()
				return Record{}, contract.DomainResultFact{}, contract.ErrNotCurrent
			}
		}
	}
	before := currentRevision
	after := before + 1
	now := s.clock.Now().UTC()
	nextWatermark := t.watermark + 1
	record := Record{Ref: contract.Ref{ID: request.RecordID, Revision: after}, Owner: contract.OwnerMemory, TenantID: access.TenantID, IdentityID: access.IdentityID, AuthorityRef: candidate.Envelope.AuthorityRef, AuthorityEpoch: candidate.Envelope.AuthorityEpoch, PolicyRef: candidate.Envelope.PolicyRef, Purpose: candidate.Envelope.Purpose, ActionScopeDigest: candidate.Envelope.ActionScopeDigest, Kind: string(candidate.Kind), Scope: candidate.Scope, Subject: candidate.Subject, ContentRef: cloneContentRef(candidate.ContentRef), SourceRefs: contract.NormalizeRefs(candidate.SourceRefs), EvidenceRefs: contract.NormalizeRefs(candidate.EvidenceRefs), Sensitivity: candidate.Sensitivity, RetentionRef: candidate.RetentionRef, LegalHoldRef: candidate.LegalHoldRef, DecayPolicyRef: candidate.DecayPolicyRef, DecayHalfLifeSeconds: candidate.DecayHalfLifeSeconds, Status: RecordActive, Watermark: nextWatermark, CreatedAt: now, ExpiresAt: candidate.Envelope.ExpiresAt.UTC()}
	if candidate.Kind != CandidateCreate {
		current := versions[len(versions)-1]
		record.Corrects = candidate.TargetRecordRef
		if candidate.Kind == CandidatePin || candidate.Kind == CandidateArchive || candidate.Kind == CandidateForget {
			if candidate.Scope != current.Scope || candidate.Subject != current.Subject || candidate.Sensitivity != current.Sensitivity {
				s.mu.Unlock()
				return Record{}, contract.DomainResultFact{}, contract.ErrEvidenceConflict
			}
			record.ContentRef = cloneContentRef(current.ContentRef)
			record.SourceRefs = contract.NormalizeRefs(current.SourceRefs)
			record.EvidenceRefs = contract.NormalizeRefs(append(append([]contract.Ref{}, current.EvidenceRefs...), candidate.EvidenceRefs...))
			record.Pinned = current.Pinned
			record.RetentionRef = current.RetentionRef
			record.LegalHoldRef = current.LegalHoldRef
			record.DecayPolicyRef = current.DecayPolicyRef
			record.DecayHalfLifeSeconds = current.DecayHalfLifeSeconds
			record.ExpiresAt = current.ExpiresAt
		}
		switch candidate.Kind {
		case CandidatePin:
			record.Pinned = true
			record.RetentionRef = candidate.RetentionRef
			if candidate.LegalHoldRef != (contract.Ref{}) {
				record.LegalHoldRef = candidate.LegalHoldRef
			}
		case CandidateArchive:
			record.Status = RecordArchived
		case CandidateTombstone, CandidateForget:
			if candidate.Kind == CandidateForget && current.LegalHoldRef != (contract.Ref{}) {
				s.mu.Unlock()
				return Record{}, contract.DomainResultFact{}, contract.ErrScopeDenied
			}
			record.Status = RecordTombstoned
			record.ContentRef = nil
		case CandidateMerge:
			record.MergeSourceRefs = contract.NormalizeRefs(candidate.MergeSourceRefs)
		}
	}
	recordDigest, err := contract.Digest(record)
	if err != nil {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, fmt.Errorf("canonical memory record: %w", err)
	}
	record.Ref.Digest = recordDigest
	if candidate.Kind == CandidateMerge {
		mergeFact = MergeFact{Ref: contract.Ref{ID: "memory/merge/" + candidate.Envelope.ID, Revision: 1}, Owner: contract.OwnerMemory, TenantID: access.TenantID, TargetRef: record.Ref, SourceRefs: contract.NormalizeRefs(candidate.MergeSourceRefs), CandidateRef: candidate.Ref(), CreatedAt: now}
		mergeDigest, mergeErr := contract.Digest(mergeFact)
		if mergeErr != nil {
			s.mu.Unlock()
			return Record{}, contract.DomainResultFact{}, mergeErr
		}
		mergeFact.Ref.Digest = mergeDigest
	}
	inspection := CommitInspection{Ref: contract.Ref{ID: "inspect/" + request.AttemptID, Revision: 1}, Owner: contract.OwnerMemory, TenantID: access.TenantID, IdentityID: access.IdentityID, AttemptID: request.AttemptID, OperationRef: request.OperationRef, RecordRef: record.Ref, State: InspectionApplied, ObservedAt: now}
	inspectionDigest, err := contract.Digest(inspection)
	if err != nil {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, fmt.Errorf("canonical memory inspection: %w", err)
	}
	inspection.Ref.Digest = inspectionDigest
	coverage := contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}
	result, err := contract.NewDomainResultFact(contract.OwnerMemory, request.ResultID, request.AttemptID, request.OperationRef, record.Ref, inspection.Ref, before, after, record.EvidenceRefs, coverage, "local_complete", nil, now)
	if err != nil {
		s.mu.Unlock()
		return Record{}, contract.DomainResultFact{}, err
	}
	t.watermark = nextWatermark
	t.records[request.RecordID] = append(versions, record)
	t.attempts[request.AttemptID] = attempt{request: request, inspection: inspection, result: result}
	t.results[result.Ref.ID] = result
	if candidate.Kind == CandidateMerge {
		t.mergeFacts[mergeFact.Ref.ID] = mergeFact
		for _, sourceRef := range mergeFact.SourceRefs {
			if sourceRef.ID != request.RecordID {
				t.mergedInto[sourceRef.ID] = record.Ref
			}
		}
	}
	fault := s.afterCAS
	s.mu.Unlock()
	if fault != nil {
		if err := fault(inspection); err != nil {
			return Record{}, contract.DomainResultFact{}, contract.ErrUnknownOutcome
		}
	}
	return cloneRecord(record), cloneDomainResult(result), nil
}

func (s *Store) Correct(access Access, request CommitRequest) (Record, contract.DomainResultFact, error) {
	return s.commitKind(access, request, CandidateCorrection)
}

func (s *Store) Tombstone(access Access, request CommitRequest) (Record, contract.DomainResultFact, error) {
	return s.commitKind(access, request, CandidateTombstone)
}

func (s *Store) Pin(access Access, request CommitRequest) (Record, contract.DomainResultFact, error) {
	return s.commitKind(access, request, CandidatePin)
}

func (s *Store) Archive(access Access, request CommitRequest) (Record, contract.DomainResultFact, error) {
	return s.commitKind(access, request, CandidateArchive)
}

func (s *Store) Forget(access Access, request CommitRequest) (Record, contract.DomainResultFact, error) {
	return s.commitKind(access, request, CandidateForget)
}

func (s *Store) Merge(access Access, request CommitRequest) (Record, contract.DomainResultFact, error) {
	return s.commitKind(access, request, CandidateMerge)
}

func (s *Store) InspectMerge(access Access, ref contract.Ref) (MergeFact, error) {
	if err := access.validate(); err != nil {
		return MergeFact{}, err
	}
	if err := ref.Validate(); err != nil {
		return MergeFact{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t := s.tenants[access.TenantID]
	if t == nil {
		return MergeFact{}, contract.ErrNotFound
	}
	fact, ok := t.mergeFacts[ref.ID]
	if !ok {
		return MergeFact{}, contract.ErrNotFound
	}
	if !contract.SameRef(fact.Ref, ref) {
		return MergeFact{}, contract.ErrEvidenceConflict
	}
	if err := fact.Validate(); err != nil {
		return MergeFact{}, err
	}
	target := findRecordByRef(t, fact.TargetRef)
	if target.IdentityID != access.IdentityID || target.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(target.AuthorityRef, access.AuthorityRef) || !contract.SameRef(target.PolicyRef, access.PolicyRef) {
		return MergeFact{}, contract.ErrNotFound
	}
	fact.SourceRefs = append([]contract.Ref{}, fact.SourceRefs...)
	return fact, nil
}

func (s *Store) commitKind(access Access, request CommitRequest, kind CandidateKind) (Record, contract.DomainResultFact, error) {
	s.mu.RLock()
	t := s.tenants[access.TenantID]
	var got CandidateKind
	if t != nil {
		if c, ok := t.candidates[candidateKey(request.CandidateRef)]; ok {
			got = c.Kind
		}
	}
	s.mu.RUnlock()
	if got != kind {
		return Record{}, contract.DomainResultFact{}, fmt.Errorf("%w: candidate kind", contract.ErrInvalidArgument)
	}
	return s.Commit(access, request)
}

func (s *Store) InspectCommit(access Access, attemptID string) (CommitInspection, contract.DomainResultFact, error) {
	if err := access.validate(); err != nil {
		return CommitInspection{}, contract.DomainResultFact{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t := s.tenants[access.TenantID]
	if t == nil {
		return CommitInspection{}, contract.DomainResultFact{}, contract.ErrNotFound
	}
	a, ok := t.attempts[attemptID]
	if !ok || !attemptAllowed(t, a, access) {
		return CommitInspection{}, contract.DomainResultFact{}, contract.ErrNotFound
	}
	if err := a.result.Validate(); err != nil {
		return CommitInspection{}, contract.DomainResultFact{}, err
	}
	return a.inspection, cloneDomainResult(a.result), nil
}

func (s *Store) ApplySettlement(access Access, request SettlementRequest) (contract.SettlementApplication, error) {
	if err := access.validate(); err != nil {
		return contract.SettlementApplication{}, err
	}
	if request.TenantID != access.TenantID {
		return contract.SettlementApplication{}, contract.ErrScopeDenied
	}
	if err := request.ExpectedRevision.Validate(); err != nil {
		return contract.SettlementApplication{}, err
	}
	if err := request.Settlement.Validate(); err != nil {
		return contract.SettlementApplication{}, err
	}
	if err := request.Association.Validate(); err != nil {
		return contract.SettlementApplication{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tenant(access.TenantID)
	result, ok := t.results[request.Association.DomainResultRef.ID]
	if !ok {
		return contract.SettlementApplication{}, contract.ErrNotFound
	}
	if err := request.Association.Verify(result); err != nil {
		return contract.SettlementApplication{}, err
	}
	if result.Owner != contract.OwnerMemory {
		return contract.SettlementApplication{}, contract.ErrSettlementMismatch
	}
	if !resultOwnedByAccess(t, result.Ref, access) {
		return contract.SettlementApplication{}, contract.ErrNotFound
	}
	existing, exists := t.settled[result.Ref.ID]
	revision := uint64(0)
	if exists {
		revision = existing.Ref.Revision
		if contract.SameRef(existing.SettlementRef, request.Settlement.Ref) {
			return existing, nil
		}
	}
	if !request.ExpectedRevision.Matches(exists, revision) {
		return contract.SettlementApplication{}, contract.ErrRevisionConflict
	}
	settlementKey := candidateKey(request.Settlement.Ref)
	if otherResult, used := t.settlementUses[settlementKey]; used && !contract.SameRef(otherResult, result.Ref) {
		return contract.SettlementApplication{}, contract.ErrSettlementMismatch
	}
	application, err := contract.NewSettlementApplication(contract.OwnerMemory, "memory/settlement/"+result.Ref.ID, revision+1, result, request.Association, request.Settlement, s.clock.Now())
	if err != nil {
		return contract.SettlementApplication{}, err
	}
	t.settled[result.Ref.ID] = application
	t.settlementUses[settlementKey] = result.Ref
	return application, nil
}

func (s *Store) InspectRecord(access Access, ref contract.Ref) (Record, error) {
	if err := access.validate(); err != nil {
		return Record{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t := s.tenants[access.TenantID]
	if t == nil {
		return Record{}, contract.ErrNotFound
	}
	r := findRecordByRef(t, ref)
	if r.Ref.ID == "" || r.IdentityID != access.IdentityID || r.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(r.AuthorityRef, access.AuthorityRef) || !contract.SameRef(r.PolicyRef, access.PolicyRef) {
		return Record{}, contract.ErrNotFound
	}
	return cloneRecord(r), nil
}

func (s *Store) CurrentWatermark(access Access) (Watermark, error) {
	if err := access.validate(); err != nil {
		return Watermark{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tenant(access.TenantID)
	return makeWatermark(access.TenantID, t.watermark), nil
}

func makeWatermark(tenant string, seq uint64) Watermark {
	w := Watermark{Ref: contract.Ref{ID: "memory/watermark/" + tenant, Revision: seq}, TenantID: tenant, Sequence: seq}
	digest, _ := contract.Digest(w) // Watermark has no fallible JSON field.
	w.Ref.Digest = digest
	return w
}

func findRecordByRef(t *tenantState, ref contract.Ref) Record {
	for _, r := range t.records[ref.ID] {
		if contract.SameRef(r.Ref, ref) {
			return r
		}
	}
	return Record{}
}

func resultOwnedByAccess(t *tenantState, ref contract.Ref, access Access) bool {
	for _, a := range t.attempts {
		if contract.SameRef(a.result.Ref, ref) {
			return attemptAllowed(t, a, access)
		}
	}
	return false
}

func attemptAllowed(t *tenantState, a attempt, access Access) bool {
	if a.inspection.IdentityID != access.IdentityID {
		return false
	}
	candidate, ok := t.candidates[candidateKey(a.request.CandidateRef)]
	return ok && candidate.Envelope.AuthorityEpoch == access.AuthorityEpoch && contract.SameRef(candidate.Envelope.AuthorityRef, access.AuthorityRef) && contract.SameRef(candidate.Envelope.PolicyRef, access.PolicyRef)
}

func cloneCandidate(c Candidate) Candidate {
	c.SourceRefs = append([]contract.Ref(nil), c.SourceRefs...)
	c.EvidenceRefs = append([]contract.Ref(nil), c.EvidenceRefs...)
	c.MergeSourceRefs = append([]contract.Ref(nil), c.MergeSourceRefs...)
	c.RiskFlags = append([]string(nil), c.RiskFlags...)
	c.Envelope.Causation = append([]contract.Ref(nil), c.Envelope.Causation...)
	c.ContentRef = cloneContentRef(c.ContentRef)
	return c
}

func cloneRecord(r Record) Record {
	r.SourceRefs = append([]contract.Ref(nil), r.SourceRefs...)
	r.EvidenceRefs = append([]contract.Ref(nil), r.EvidenceRefs...)
	r.ContentRef = cloneContentRef(r.ContentRef)
	r.MergeSourceRefs = append([]contract.Ref(nil), r.MergeSourceRefs...)
	return r
}

func cloneDomainResult(result contract.DomainResultFact) contract.DomainResultFact {
	result.EvidenceRefs = append([]contract.Ref(nil), result.EvidenceRefs...)
	result.Coverage.ProjectionRefs = append([]contract.Ref(nil), result.Coverage.ProjectionRefs...)
	result.Coverage.DroppedReasons = append([]string(nil), result.Coverage.DroppedReasons...)
	result.Residuals = append([]string(nil), result.Residuals...)
	return result
}

func cloneContentRef(ref *contract.ContentRef) *contract.ContentRef {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}
