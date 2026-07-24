package knowledge

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	projectioncatalog "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection"
)

type tenantState struct {
	changeSequence uint64
	changes        []contract.ChangeEventV1
	sources        map[string][]Source
	packages       map[string][]Package
	candidates     map[string]Candidate
	producerKeys   map[string]contract.Ref
	admissions     map[string][]Admission
	records        map[string][]Record
	projections    map[string][]Projection
	snapshots      map[string][]Snapshot
	current        *SnapshotPointer
	views          map[string][]View
	attempts       map[string]CommitAttempt
	inspections    map[string]Inspection
	results        map[string]contract.DomainResultFact
	settlements    map[string]contract.SettlementApplication
	settlementUse  map[string]contract.Ref
	tombstones     map[string]Tombstone
	jobs           map[string][]contract.OwnerJobAttemptV1
	syncs          map[string][]SyncAttemptV1
	purgeIntents   map[string]contract.PurgeIntentV1
}

func newTenantState() *tenantState {
	return &tenantState{
		changes: []contract.ChangeEventV1{},
		sources: make(map[string][]Source), packages: make(map[string][]Package),
		candidates: make(map[string]Candidate), producerKeys: make(map[string]contract.Ref),
		admissions: make(map[string][]Admission), records: make(map[string][]Record),
		projections: make(map[string][]Projection), snapshots: make(map[string][]Snapshot),
		views: make(map[string][]View), attempts: make(map[string]CommitAttempt), inspections: make(map[string]Inspection),
		results: make(map[string]contract.DomainResultFact), settlements: make(map[string]contract.SettlementApplication),
		settlementUse: make(map[string]contract.Ref), tombstones: make(map[string]Tombstone),
		jobs:         make(map[string][]contract.OwnerJobAttemptV1),
		syncs:        make(map[string][]SyncAttemptV1),
		purgeIntents: make(map[string]contract.PurgeIntentV1),
	}
}

// Store is the Wave 1 in-memory Knowledge Owner reference store. It owns only
// Knowledge facts; it is not a connector, remote index, Runtime adapter, or a
// production persistence/SLA claim.
type Store struct {
	mu      sync.RWMutex
	clock   contract.Clock
	indexes *projectioncatalog.Catalog
	tenants map[string]*tenantState

	// beforeLinearizedCommit is an in-package fault-injection seam. It runs
	// after every fallible envelope/digest has succeeded and before any
	// authoritative commit state is mutated.
	beforeLinearizedCommit func() error
}

func NewStore(clock contract.Clock) *Store {
	if clock == nil {
		clock = contract.SystemClock{}
	}
	return &Store{clock: clock, indexes: projectioncatalog.NewCatalog(), tenants: make(map[string]*tenantState)}
}

func (s *Store) tenantLocked(tenantID string, create bool) (*tenantState, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("%w: tenant id required", contract.ErrInvalidArgument)
	}
	t := s.tenants[tenantID]
	if t == nil && create {
		t = newTenantState()
		s.tenants[tenantID] = t
	}
	if t == nil {
		return nil, contract.ErrNotFound
	}
	return t, nil
}

func (s *Store) RegisterSource(access Access, in SourceInput, expected contract.ExpectedRevision) (Source, error) {
	if err := access.Validate(); err != nil {
		return Source{}, err
	}
	if err := expected.Validate(); err != nil {
		return Source{}, err
	}
	if access.TenantID != in.TenantID || !contract.SameRef(access.AuthorityRef, in.AuthorityRef) || !contract.SameRef(access.PolicyRef, in.PolicyRef) {
		return Source{}, contract.ErrScopeDenied
	}
	if !validID(in.TenantID, in.ID, in.Version, in.ContentDigest, in.License, in.Scope, in.Sensitivity) {
		return Source{}, fmt.Errorf("%w: incomplete source", contract.ErrInvalidArgument)
	}
	if err := in.AssetRef.Validate(); err != nil {
		return Source{}, err
	}
	if err := in.AuthorityRef.Validate(); err != nil {
		return Source{}, err
	}
	if err := in.PolicyRef.Validate(); err != nil {
		return Source{}, err
	}
	if err := validateRefs(in.Provenance); err != nil {
		return Source{}, err
	}
	if in.State != SourceRegistered && in.State != SourceAvailable && in.State != SourceStale && in.State != SourceDeprecated {
		return Source{}, fmt.Errorf("%w: invalid source state", contract.ErrInvalidArgument)
	}
	now := s.clock.Now().UTC()
	if in.AcquiredAt.IsZero() {
		in.AcquiredAt = now
	}
	if in.ValidFrom.IsZero() {
		in.ValidFrom = now
	}
	if in.ValidTo.IsZero() || !now.Before(in.ValidTo) || in.ValidTo.Before(in.ValidFrom) {
		return Source{}, contract.ErrNotCurrent
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(in.TenantID, true)
	if err != nil {
		return Source{}, err
	}
	history := t.sources[in.ID]
	exists := len(history) != 0
	currentRevision := uint64(0)
	if exists {
		currentRevision = history[len(history)-1].Ref.Revision
	}
	if !expected.Matches(exists, currentRevision) {
		return Source{}, contract.ErrRevisionConflict
	}
	revision := currentRevision + 1
	source := Source{
		Ref: contract.Ref{ID: in.ID, Revision: revision}, TenantID: in.TenantID, Owner: contract.OwnerKnowledge,
		Version: in.Version, AssetRef: in.AssetRef, ContentDigest: in.ContentDigest,
		AuthorityRef: in.AuthorityRef, PolicyRef: in.PolicyRef, License: in.License,
		Scope: in.Scope, Sensitivity: in.Sensitivity, State: in.State,
		Provenance: contract.NormalizeRefs(in.Provenance), AcquiredAt: in.AcquiredAt.UTC(),
		ValidFrom: in.ValidFrom.UTC(), ValidTo: in.ValidTo.UTC(), UpdatedAt: now,
	}
	if err := setCanonicalDigest(&source.Ref, source); err != nil {
		return Source{}, err
	}
	changeKind := contract.ChangeSourceRegistered
	previous := contract.Ref{}
	if exists {
		changeKind = contract.ChangeSourceRefreshed
		previous = history[len(history)-1].Ref
	}
	change, err := sealKnowledgeChange(access.TenantID, t.changeSequence+1, changeKind, source.Ref, previous, source.AuthorityRef, source.PolicyRef, source.Scope, source.Sensitivity, source.License, now)
	if err != nil {
		return Source{}, err
	}
	t.sources[in.ID] = append(history, source)
	t.changeSequence++
	t.changes = append(t.changes, change)
	return cloneSource(source), nil
}

func (s *Store) DeprecateSource(access Access, sourceID, reason string, expected contract.ExpectedRevision) (Source, error) {
	if err := access.Validate(); err != nil {
		return Source{}, err
	}
	if err := expected.Validate(); err != nil {
		return Source{}, err
	}
	if !validID(access.TenantID, sourceID, reason) {
		return Source{}, contract.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return Source{}, err
	}
	history := t.sources[sourceID]
	if len(history) == 0 || !expected.Matches(true, history[len(history)-1].Ref.Revision) {
		return Source{}, contract.ErrRevisionConflict
	}
	current := history[len(history)-1]
	if !accessAllows(access, current.AuthorityRef, current.PolicyRef) {
		return Source{}, contract.ErrScopeDenied
	}
	if current.State == SourceWithdrawn {
		return Source{}, contract.ErrNotCurrent
	}
	if current.State == SourceDeprecated {
		return cloneSource(current), nil
	}
	next := cloneSource(current)
	next.Ref = contract.Ref{ID: sourceID, Revision: current.Ref.Revision + 1}
	next.State = SourceDeprecated
	next.UpdatedAt = s.clock.Now().UTC()
	if err := setCanonicalDigest(&next.Ref, next); err != nil {
		return Source{}, err
	}
	change, err := sealKnowledgeChange(access.TenantID, t.changeSequence+1, contract.ChangeSourceDeprecated, next.Ref, current.Ref, next.AuthorityRef, next.PolicyRef, next.Scope, next.Sensitivity, next.License, next.UpdatedAt)
	if err != nil {
		return Source{}, err
	}
	t.sources[sourceID] = append(history, next)
	t.changeSequence++
	t.changes = append(t.changes, change)
	return cloneSource(next), nil
}

func (s *Store) GetSource(access Access, ref contract.Ref) (Source, error) {
	if err := access.Validate(); err != nil {
		return Source{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return Source{}, err
	}
	source, ok := sourceByRef(t, ref)
	if !ok {
		return Source{}, contract.ErrNotFound
	}
	if !accessAllows(access, source.AuthorityRef, source.PolicyRef) {
		return Source{}, contract.ErrScopeDenied
	}
	return cloneSource(source), nil
}

func (s *Store) WithdrawSource(access Access, sourceID, reason string, expected contract.ExpectedRevision) (Source, Tombstone, error) {
	if err := access.Validate(); err != nil {
		return Source{}, Tombstone{}, err
	}
	if err := expected.Validate(); err != nil {
		return Source{}, Tombstone{}, err
	}
	if !validID(access.TenantID, sourceID, reason) {
		return Source{}, Tombstone{}, fmt.Errorf("%w: incomplete withdrawal", contract.ErrInvalidArgument)
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return Source{}, Tombstone{}, err
	}
	history := t.sources[sourceID]
	if len(history) == 0 || !expected.Matches(true, history[len(history)-1].Ref.Revision) {
		return Source{}, Tombstone{}, contract.ErrRevisionConflict
	}
	current := history[len(history)-1]
	if !accessAllows(access, current.AuthorityRef, current.PolicyRef) {
		return Source{}, Tombstone{}, contract.ErrScopeDenied
	}
	if current.State == SourceWithdrawn {
		return cloneSource(current), cloneTombstone(t.tombstones["source/"+sourceID]), nil
	}
	withdrawn := cloneSource(current)
	withdrawn.Ref = contract.Ref{ID: sourceID, Revision: current.Ref.Revision + 1}
	withdrawn.State = SourceWithdrawn
	withdrawn.UpdatedAt = now
	if err := setCanonicalDigest(&withdrawn.Ref, withdrawn); err != nil {
		return Source{}, Tombstone{}, err
	}
	tomb := Tombstone{
		Ref:      contract.Ref{ID: "source/" + sourceID + "/tombstone", Revision: 1},
		TenantID: access.TenantID, Owner: contract.OwnerKnowledge, TargetKind: "source",
		TargetRef: current.Ref, Reason: reason, CreatedAt: now,
	}
	if err := setCanonicalDigest(&tomb.Ref, tomb); err != nil {
		return Source{}, Tombstone{}, err
	}
	change, err := sealKnowledgeChange(access.TenantID, t.changeSequence+1, contract.ChangeSourceWithdrawn, withdrawn.Ref, current.Ref, withdrawn.AuthorityRef, withdrawn.PolicyRef, withdrawn.Scope, withdrawn.Sensitivity, withdrawn.License, now)
	if err != nil {
		return Source{}, Tombstone{}, err
	}
	t.sources[sourceID] = append(history, withdrawn)
	t.tombstones["source/"+sourceID] = tomb
	t.changeSequence++
	t.changes = append(t.changes, change)
	return cloneSource(withdrawn), cloneTombstone(tomb), nil
}

func (s *Store) PutPackage(access Access, in PackageInput, expected contract.ExpectedRevision) (Package, error) {
	if err := access.Validate(); err != nil {
		return Package{}, err
	}
	if err := expected.Validate(); err != nil {
		return Package{}, err
	}
	if access.TenantID != in.TenantID || !contract.SameRef(access.AuthorityRef, in.AuthorityRef) || !contract.SameRef(access.PolicyRef, in.PolicyRef) {
		return Package{}, contract.ErrScopeDenied
	}
	if !validID(in.TenantID, in.ID, in.Version, in.License) || len(in.SourceRefs) == 0 {
		return Package{}, fmt.Errorf("%w: incomplete package", contract.ErrInvalidArgument)
	}
	if err := validateRefs(in.SourceRefs); err != nil {
		return Package{}, err
	}
	if err := in.AuthorityRef.Validate(); err != nil {
		return Package{}, err
	}
	if err := in.PolicyRef.Validate(); err != nil {
		return Package{}, err
	}
	if in.State == "" {
		in.State = PackageReady
	}
	if in.State != PackageReady && in.State != PackageWithdrawn {
		return Package{}, fmt.Errorf("%w: invalid package state", contract.ErrInvalidArgument)
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(in.TenantID, true)
	if err != nil {
		return Package{}, err
	}
	for _, ref := range in.SourceRefs {
		source, ok := sourceByRef(t, ref)
		if !ok || !accessAllows(access, source.AuthorityRef, source.PolicyRef) || source.State == SourceWithdrawn || source.State == SourceDeprecated || !now.Before(source.ValidTo) {
			return Package{}, contract.ErrNotCurrent
		}
	}
	history := t.packages[in.ID]
	exists := len(history) != 0
	revision := uint64(0)
	createdAt := now
	if exists {
		revision = history[len(history)-1].Ref.Revision
		createdAt = history[0].CreatedAt
	}
	if !expected.Matches(exists, revision) {
		return Package{}, contract.ErrRevisionConflict
	}
	pkg := Package{
		Ref: contract.Ref{ID: in.ID, Revision: revision + 1}, TenantID: in.TenantID, Owner: contract.OwnerKnowledge,
		Version: in.Version, SourceRefs: contract.NormalizeRefs(in.SourceRefs), AuthorityRef: in.AuthorityRef,
		PolicyRef: in.PolicyRef, License: in.License, Coverage: cloneCoverage(in.Coverage), State: in.State,
		CreatedAt: createdAt, UpdatedAt: now,
	}
	if err := setCanonicalDigest(&pkg.Ref, pkg); err != nil {
		return Package{}, err
	}
	t.packages[in.ID] = append(history, pkg)
	return clonePackage(pkg), nil
}

func (s *Store) PutProjection(access Access, in ProjectionInput, expected contract.ExpectedRevision) (Projection, error) {
	if err := access.Validate(); err != nil {
		return Projection{}, err
	}
	if err := expected.Validate(); err != nil {
		return Projection{}, err
	}
	if access.TenantID != in.TenantID {
		return Projection{}, contract.ErrScopeDenied
	}
	if !validID(in.TenantID, in.ID, in.Kind, in.BuilderVersion) || len(in.RecordRefs) == 0 {
		return Projection{}, fmt.Errorf("%w: incomplete projection", contract.ErrInvalidArgument)
	}
	if err := validateTTL(in.TTL); err != nil {
		return Projection{}, err
	}
	if err := validateRefs(in.RecordRefs); err != nil {
		return Projection{}, err
	}
	if in.State != ProjectionReady && in.State != ProjectionPartial && in.State != ProjectionStale {
		return Projection{}, fmt.Errorf("%w: invalid projection state", contract.ErrInvalidArgument)
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(in.TenantID, true)
	if err != nil {
		return Projection{}, err
	}
	for _, ref := range in.RecordRefs {
		record, ok := recordByRef(t, ref)
		if !ok {
			return Projection{}, contract.ErrNotFound
		}
		if !accessAllows(access, record.AuthorityRef, record.PolicyRef) {
			return Projection{}, contract.ErrScopeDenied
		}
	}
	history := t.projections[in.ID]
	exists := len(history) != 0
	revision := uint64(0)
	if exists {
		revision = history[len(history)-1].Ref.Revision
	}
	if !expected.Matches(exists, revision) {
		return Projection{}, contract.ErrRevisionConflict
	}
	projection := Projection{
		Ref: contract.Ref{ID: in.ID, Revision: revision + 1}, TenantID: in.TenantID, Owner: contract.OwnerKnowledge,
		Kind: in.Kind, SnapshotRef: in.SnapshotRef, RecordRefs: contract.NormalizeRefs(in.RecordRefs),
		BuilderVersion: in.BuilderVersion, Coverage: cloneCoverage(in.Coverage), State: in.State,
		CreatedAt: now, ExpiresAt: now.Add(in.TTL),
	}
	if err := setCanonicalDigest(&projection.Ref, projection); err != nil {
		return Projection{}, err
	}
	t.projections[in.ID] = append(history, projection)
	return cloneProjection(projection), nil
}

func (s *Store) SubmitCandidate(access Access, in CandidateInput, expected contract.ExpectedRevision) (Candidate, error) {
	if err := access.Validate(); err != nil {
		return Candidate{}, err
	}
	if err := expected.Validate(); err != nil {
		return Candidate{}, err
	}
	if !expected.Absent {
		return Candidate{}, fmt.Errorf("%w: candidates are immutable", contract.ErrUnsupported)
	}
	if access.TenantID != in.TenantID {
		return Candidate{}, contract.ErrScopeDenied
	}
	if !validID(in.TenantID, in.ID, in.ProducerID, in.PayloadDigest) || in.SourceEpoch == 0 || in.SourceSequence == 0 {
		return Candidate{}, fmt.Errorf("%w: incomplete candidate source key", contract.ErrInvalidArgument)
	}
	if err := validateTTL(in.TTL); err != nil {
		return Candidate{}, err
	}
	if in.Kind != CandidateRecord && in.Kind != CandidateCorrection && in.Kind != CandidateWithdraw {
		return Candidate{}, fmt.Errorf("%w: invalid candidate kind", contract.ErrInvalidArgument)
	}
	if in.Kind == CandidateRecord {
		if in.TargetRef.ID != "" {
			return Candidate{}, fmt.Errorf("%w: record candidate has target", contract.ErrInvalidArgument)
		}
	} else if err := in.TargetRef.Validate(); err != nil {
		return Candidate{}, err
	}
	if err := validateDraft(in.Kind, in.Draft); err != nil {
		return Candidate{}, err
	}
	if err := validateRefs(in.EvidenceRefs); err != nil {
		return Candidate{}, err
	}
	now := s.clock.Now().UTC()
	candidate := Candidate{
		Ref: contract.Ref{ID: in.ID, Revision: 1}, TenantID: in.TenantID, Owner: contract.OwnerKnowledge,
		ProducerID: in.ProducerID, SourceEpoch: in.SourceEpoch, SourceSequence: in.SourceSequence,
		Kind: in.Kind, TargetRef: in.TargetRef, Draft: cloneDraft(in.Draft), PayloadDigest: in.PayloadDigest,
		EvidenceRefs: contract.NormalizeRefs(in.EvidenceRefs), RiskFlags: normalizeStrings(in.RiskFlags), CreatedAt: now, ExpiresAt: now.Add(in.TTL),
	}
	if err := setCanonicalDigest(&candidate.Ref, candidate); err != nil {
		return Candidate{}, err
	}
	key := producerKey(in.ProducerID, in.SourceEpoch, in.SourceSequence)
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(in.TenantID, true)
	if err != nil {
		return Candidate{}, err
	}
	if in.Kind == CandidateRecord || in.Kind == CandidateCorrection {
		pkg, ok := packageByRef(t, in.Draft.PackageRef)
		if !ok {
			return Candidate{}, contract.ErrNotFound
		}
		if !accessAllows(access, pkg.AuthorityRef, pkg.PolicyRef) {
			return Candidate{}, contract.ErrScopeDenied
		}
	} else {
		record, ok := recordByRef(t, in.TargetRef)
		if !ok {
			return Candidate{}, contract.ErrNotFound
		}
		if !accessAllows(access, record.AuthorityRef, record.PolicyRef) {
			return Candidate{}, contract.ErrScopeDenied
		}
	}
	if existingRef, ok := t.producerKeys[key]; ok {
		existing := t.candidates[existingRef.ID]
		existingDigest, err := candidateSemanticDigest(existing)
		if err != nil {
			return Candidate{}, err
		}
		candidateDigest, err := candidateSemanticDigest(candidate)
		if err != nil {
			return Candidate{}, err
		}
		if existingDigest != candidateDigest {
			return Candidate{}, contract.ErrEvidenceConflict
		}
		return cloneCandidate(existing), nil
	}
	if _, exists := t.candidates[in.ID]; exists {
		return Candidate{}, contract.ErrAlreadyExists
	}
	t.candidates[in.ID] = candidate
	t.producerKeys[key] = candidate.Ref
	return cloneCandidate(candidate), nil
}

func (s *Store) AdmitCandidate(access Access, candidateRef contract.Ref, decision AdmissionDecision, reason string, ttl time.Duration, expected contract.ExpectedRevision) (Admission, error) {
	if err := access.Validate(); err != nil {
		return Admission{}, err
	}
	if err := expected.Validate(); err != nil {
		return Admission{}, err
	}
	if err := candidateRef.Validate(); err != nil {
		return Admission{}, err
	}
	if err := validateTTL(ttl); err != nil {
		return Admission{}, err
	}
	if !validAdmissionDecision(decision) {
		return Admission{}, fmt.Errorf("%w: invalid admission decision", contract.ErrInvalidArgument)
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return Admission{}, err
	}
	candidate, ok := t.candidates[candidateRef.ID]
	if !ok || !contract.SameRef(candidate.Ref, candidateRef) {
		return Admission{}, contract.ErrNotFound
	}
	if !now.Before(candidate.ExpiresAt) {
		return Admission{}, contract.ErrNotCurrent
	}
	if err := candidateAccess(t, access, candidate); err != nil {
		return Admission{}, err
	}
	if (decision == AdmissionCommitReady || decision == AdmissionMerged) && hasBlockingRisk(candidate.RiskFlags) {
		return Admission{}, contract.ErrCandidateRejected
	}
	id := candidate.Ref.ID + "/admission"
	history := t.admissions[id]
	exists := len(history) != 0
	var existing Admission
	if exists {
		existing = history[len(history)-1]
	}
	if exists && existing.Decision == decision && existing.Reason == reason && contract.SameRef(existing.CandidateRef, candidateRef) {
		return cloneAdmission(existing), nil
	}
	currentRevision := uint64(0)
	if exists {
		currentRevision = existing.Ref.Revision
	}
	if !expected.Matches(exists, currentRevision) {
		return Admission{}, contract.ErrRevisionConflict
	}
	if exists && !validAdmissionTransition(existing.Decision, decision) {
		return Admission{}, contract.ErrCandidateRejected
	}
	admission := Admission{
		Ref: contract.Ref{ID: id, Revision: currentRevision + 1}, TenantID: access.TenantID, Owner: contract.OwnerKnowledge,
		CandidateRef: candidateRef, Decision: decision, Reason: reason, CreatedAt: now, ExpiresAt: now.Add(ttl),
	}
	if err := setCanonicalDigest(&admission.Ref, admission); err != nil {
		return Admission{}, err
	}
	t.admissions[id] = append(history, admission)
	return cloneAdmission(admission), nil
}

func (s *Store) BeginCommit(access Access, req CommitRequest) (CommitAttempt, error) {
	if err := access.Validate(); err != nil {
		return CommitAttempt{}, err
	}
	if access.TenantID != req.TenantID || !validID(req.TenantID, req.AttemptID) {
		return CommitAttempt{}, fmt.Errorf("%w: incomplete commit request", contract.ErrInvalidArgument)
	}
	if err := req.OperationRef.Validate(); err != nil {
		return CommitAttempt{}, err
	}
	if err := req.CandidateRef.Validate(); err != nil {
		return CommitAttempt{}, err
	}
	if err := req.AdmissionRef.Validate(); err != nil {
		return CommitAttempt{}, err
	}
	if err := req.ExpectedRecord.Validate(); err != nil {
		return CommitAttempt{}, err
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(req.TenantID, false)
	if err != nil {
		return CommitAttempt{}, err
	}
	candidate, ok := t.candidates[req.CandidateRef.ID]
	if !ok || !contract.SameRef(candidate.Ref, req.CandidateRef) {
		return CommitAttempt{}, contract.ErrNotFound
	}
	admission, ok := admissionByRef(t, req.AdmissionRef)
	if !ok || !contract.SameRef(admission.CandidateRef, candidate.Ref) {
		return CommitAttempt{}, contract.ErrNotFound
	}
	if admission.Decision != AdmissionCommitReady {
		return CommitAttempt{}, contract.ErrCandidateRejected
	}
	if !now.Before(candidate.ExpiresAt) || !now.Before(admission.ExpiresAt) {
		return CommitAttempt{}, contract.ErrNotCurrent
	}
	if err := candidateAccess(t, access, candidate); err != nil {
		return CommitAttempt{}, err
	}
	if existing, exists := t.attempts[req.AttemptID]; exists {
		if contract.SameRef(existing.OperationRef, req.OperationRef) && contract.SameRef(existing.CandidateRef, req.CandidateRef) && contract.SameRef(existing.AdmissionRef, req.AdmissionRef) && existing.ExpectedRecord == req.ExpectedRecord {
			return cloneAttempt(existing), nil
		}
		return CommitAttempt{}, contract.ErrEvidenceConflict
	}
	attempt := CommitAttempt{
		Ref: contract.Ref{ID: req.AttemptID, Revision: 1}, TenantID: req.TenantID, Owner: contract.OwnerKnowledge,
		OperationRef: req.OperationRef, CandidateRef: req.CandidateRef, AdmissionRef: req.AdmissionRef,
		ExpectedRecord: req.ExpectedRecord, State: AttemptBegun, BegunAt: now, UpdatedAt: now,
	}
	if err := setCanonicalDigest(&attempt.Ref, attempt); err != nil {
		return CommitAttempt{}, err
	}
	t.attempts[req.AttemptID] = attempt
	return cloneAttempt(attempt), nil
}

func (s *Store) CommitAttempt(access Access, attemptID string) (contract.DomainResultFact, error) {
	if err := access.Validate(); err != nil {
		return contract.DomainResultFact{}, err
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return contract.DomainResultFact{}, err
	}
	attempt, ok := t.attempts[attemptID]
	if !ok {
		return contract.DomainResultFact{}, contract.ErrNotFound
	}
	if attempt.State == AttemptApplied {
		result := cloneResult(t.results[attempt.DomainResultRef.ID])
		if err := result.Validate(); err != nil {
			return contract.DomainResultFact{}, err
		}
		return result, nil
	}
	if attempt.State == AttemptFailed {
		return contract.DomainResultFact{}, fmt.Errorf("%w: %s", contract.ErrRevisionConflict, attempt.Failure)
	}
	candidate := t.candidates[attempt.CandidateRef.ID]
	admission, ok := admissionByRef(t, attempt.AdmissionRef)
	if !ok {
		return contract.DomainResultFact{}, contract.ErrNotFound
	}
	if !now.Before(candidate.ExpiresAt) || !now.Before(admission.ExpiresAt) || admission.Decision != AdmissionCommitReady {
		return contract.DomainResultFact{}, contract.ErrNotCurrent
	}
	if err := candidateAccess(t, access, candidate); err != nil {
		return contract.DomainResultFact{}, err
	}
	record, tomb, before, err := prepareRecordCAS(t, candidate, attempt.ExpectedRecord, now)
	if err != nil {
		return contract.DomainResultFact{}, err
	}
	inspectionRevision := uint64(1)
	if previous, exists := t.inspections[attemptID]; exists {
		inspectionRevision = previous.Ref.Revision + 1
	}
	inspection := Inspection{
		Ref: contract.Ref{ID: "knowledge-inspection/" + attemptID, Revision: inspectionRevision}, TenantID: access.TenantID,
		Owner: contract.OwnerKnowledge, AttemptRef: attempt.Ref, OperationRef: attempt.OperationRef,
		Outcome: InspectionApplied, SubjectRef: record.Ref, InspectedAt: now,
	}
	if err := setCanonicalDigest(&inspection.Ref, inspection); err != nil {
		return contract.DomainResultFact{}, err
	}
	coverage := contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}
	evidence := append(slices.Clone(candidate.EvidenceRefs), record.EvidenceRefs...)
	result, err := contract.NewDomainResultFact(contract.OwnerKnowledge, "knowledge-result/"+attemptID, attemptID,
		attempt.OperationRef, record.Ref, inspection.Ref, before, record.Ref.Revision,
		evidence, coverage, "not_required", nil, now)
	if err != nil {
		return contract.DomainResultFact{}, err
	}
	committedAttempt := attempt
	committedAttempt.State = AttemptApplied
	committedAttempt.SubjectRef = record.Ref
	committedAttempt.InspectionRef = inspection.Ref
	committedAttempt.DomainResultRef = result.Ref
	committedAttempt.UpdatedAt = now
	committedAttempt.Ref.Revision++
	if err := setCanonicalDigest(&committedAttempt.Ref, committedAttempt); err != nil {
		return contract.DomainResultFact{}, err
	}
	changeKind := contract.ChangeRecordCommitted
	switch candidate.Kind {
	case CandidateCorrection:
		changeKind = contract.ChangeRecordCorrected
	case CandidateWithdraw:
		changeKind = contract.ChangeRecordWithdrawn
	}
	change, err := sealKnowledgeChange(access.TenantID, t.changeSequence+1, changeKind, record.Ref, record.Corrects, record.AuthorityRef, record.PolicyRef, record.Scope, record.Sensitivity, record.License, now)
	if err != nil {
		return contract.DomainResultFact{}, err
	}
	if s.beforeLinearizedCommit != nil {
		if err := s.beforeLinearizedCommit(); err != nil {
			return contract.DomainResultFact{}, err
		}
	}

	// Single linearization point: no fallible work is allowed below this line.
	t.records[record.Ref.ID] = append(t.records[record.Ref.ID], record)
	if tomb != nil {
		t.tombstones["record/"+record.Ref.ID] = *tomb
	}
	t.inspections[attemptID] = inspection
	// The DomainResult remains result_ready until ApplySettlement. Runtime owns
	// the opaque settlement in between these two domain operations.
	t.results[result.Ref.ID] = result
	t.attempts[attemptID] = committedAttempt
	t.changeSequence++
	t.changes = append(t.changes, change)
	return cloneResult(result), nil
}

func (s *Store) InspectCommit(access Access, attemptID string, operationRef contract.Ref) (Inspection, *contract.DomainResultFact, error) {
	if err := access.Validate(); err != nil {
		return Inspection{}, nil, err
	}
	if err := operationRef.Validate(); err != nil {
		return Inspection{}, nil, err
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return Inspection{}, nil, err
	}
	attempt, ok := t.attempts[attemptID]
	if !ok || !contract.SameRef(attempt.OperationRef, operationRef) {
		return Inspection{}, nil, contract.ErrNotFound
	}
	if err := candidateAccess(t, access, t.candidates[attempt.CandidateRef.ID]); err != nil {
		return Inspection{}, nil, err
	}
	if attempt.State == AttemptApplied {
		inspection := t.inspections[attemptID]
		result := cloneResult(t.results[attempt.DomainResultRef.ID])
		if err := result.Validate(); err != nil {
			return Inspection{}, nil, err
		}
		return cloneInspection(inspection), &result, nil
	}
	inspection := Inspection{
		Ref: contract.Ref{ID: "knowledge-inspection/" + attemptID, Revision: 1}, TenantID: access.TenantID,
		Owner: contract.OwnerKnowledge, AttemptRef: attempt.Ref, OperationRef: operationRef,
		Outcome: InspectionNotApplied, InspectedAt: now,
	}
	if err := setCanonicalDigest(&inspection.Ref, inspection); err != nil {
		return Inspection{}, nil, err
	}
	t.inspections[attemptID] = inspection
	return cloneInspection(inspection), nil, nil
}

func (s *Store) ApplySettlement(access Access, association contract.DomainResultAssociation, settlement contract.RuntimeSettlementRef, expected contract.ExpectedRevision) (contract.SettlementApplication, error) {
	if err := access.Validate(); err != nil {
		return contract.SettlementApplication{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.SettlementApplication{}, err
	}
	if err := association.Validate(); err != nil {
		return contract.SettlementApplication{}, err
	}
	if err := settlement.Validate(); err != nil {
		return contract.SettlementApplication{}, err
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return contract.SettlementApplication{}, err
	}
	resultRef := association.DomainResultRef
	result, ok := t.results[resultRef.ID]
	if !ok {
		return contract.SettlementApplication{}, contract.ErrNotFound
	}
	if err := association.Verify(result); err != nil {
		return contract.SettlementApplication{}, err
	}
	if result.Owner != contract.OwnerKnowledge || result.State != contract.DomainResultReady {
		return contract.SettlementApplication{}, contract.ErrSettlementMismatch
	}
	record, ok := recordByRef(t, result.SubjectRef)
	if !ok || !accessAllows(access, record.AuthorityRef, record.PolicyRef) {
		return contract.SettlementApplication{}, contract.ErrScopeDenied
	}
	settlementKey := refKey(settlement.Ref)
	if usedBy, used := t.settlementUse[settlementKey]; used && !contract.SameRef(usedBy, resultRef) {
		return contract.SettlementApplication{}, contract.ErrSettlementMismatch
	}
	if existing, exists := t.settlements[resultRef.ID]; exists {
		if contract.SameRef(existing.SettlementRef, settlement.Ref) && contract.SameRef(existing.DomainResultRef, resultRef) {
			return cloneApplication(existing), nil
		}
		return contract.SettlementApplication{}, contract.ErrSettlementMismatch
	}
	if !expected.Matches(false, 0) {
		return contract.SettlementApplication{}, contract.ErrRevisionConflict
	}
	application, err := contract.NewSettlementApplication(contract.OwnerKnowledge, "knowledge-settlement/"+resultRef.ID, 1, result, association, settlement, now)
	if err != nil {
		return contract.SettlementApplication{}, err
	}
	t.settlements[resultRef.ID] = application
	t.settlementUse[settlementKey] = resultRef
	return cloneApplication(application), nil
}

func (s *Store) GetRecord(access Access, ref contract.Ref) (Record, error) {
	if err := access.Validate(); err != nil {
		return Record{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return Record{}, err
	}
	record, ok := recordByRef(t, ref)
	if !ok {
		return Record{}, contract.ErrNotFound
	}
	if !accessAllows(access, record.AuthorityRef, record.PolicyRef) {
		return Record{}, contract.ErrScopeDenied
	}
	return cloneRecord(record), nil
}

func (s *Store) CreateSnapshot(access Access, in SnapshotInput, expected contract.ExpectedRevision) (Snapshot, error) {
	if err := access.Validate(); err != nil {
		return Snapshot{}, err
	}
	if err := expected.Validate(); err != nil {
		return Snapshot{}, err
	}
	if access.TenantID != in.TenantID || !validID(in.TenantID, in.ID, in.Version) || len(in.RecordRefs) == 0 {
		return Snapshot{}, fmt.Errorf("%w: incomplete snapshot", contract.ErrInvalidArgument)
	}
	if err := validateRefs(append(append(append(slices.Clone(in.SourceRefs), in.PackageRefs...), in.RecordRefs...), in.ProjectionRefs...)); err != nil {
		return Snapshot{}, err
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(in.TenantID, true)
	if err != nil {
		return Snapshot{}, err
	}
	authorities := make([]contract.Ref, 0)
	policies := make([]contract.Ref, 0)
	for _, ref := range in.SourceRefs {
		source, ok := sourceByRef(t, ref)
		if !ok {
			return Snapshot{}, contract.ErrNotFound
		}
		if !accessAllows(access, source.AuthorityRef, source.PolicyRef) {
			return Snapshot{}, contract.ErrScopeDenied
		}
		authorities = append(authorities, source.AuthorityRef)
		policies = append(policies, source.PolicyRef)
	}
	for _, ref := range in.PackageRefs {
		pkg, ok := packageByRef(t, ref)
		if !ok {
			return Snapshot{}, contract.ErrNotFound
		}
		if !accessAllows(access, pkg.AuthorityRef, pkg.PolicyRef) {
			return Snapshot{}, contract.ErrScopeDenied
		}
		authorities = append(authorities, pkg.AuthorityRef)
		policies = append(policies, pkg.PolicyRef)
	}
	for _, ref := range in.RecordRefs {
		record, ok := recordByRef(t, ref)
		if !ok {
			return Snapshot{}, contract.ErrNotFound
		}
		if !accessAllows(access, record.AuthorityRef, record.PolicyRef) {
			return Snapshot{}, contract.ErrScopeDenied
		}
		authorities = append(authorities, record.AuthorityRef)
		policies = append(policies, record.PolicyRef)
	}
	for _, ref := range in.ProjectionRefs {
		if _, ok := projectionByRef(t, ref); !ok {
			return Snapshot{}, contract.ErrNotFound
		}
	}
	history := t.snapshots[in.ID]
	exists := len(history) != 0
	revision := uint64(0)
	if exists {
		revision = history[len(history)-1].Ref.Revision
	}
	if !expected.Matches(exists, revision) {
		return Snapshot{}, contract.ErrRevisionConflict
	}
	snapshot := Snapshot{
		Ref: contract.Ref{ID: in.ID, Revision: revision + 1}, TenantID: in.TenantID, Owner: contract.OwnerKnowledge,
		Version: in.Version, SourceRefs: contract.NormalizeRefs(in.SourceRefs), PackageRefs: contract.NormalizeRefs(in.PackageRefs),
		RecordRefs: contract.NormalizeRefs(in.RecordRefs), ProjectionRefs: contract.NormalizeRefs(in.ProjectionRefs),
		AuthorityRefs: contract.NormalizeRefs(authorities), PolicyRefs: contract.NormalizeRefs(policies),
		Coverage: cloneCoverage(in.Coverage), State: SnapshotReady, BuiltAt: now,
	}
	manifestDigest, err := snapshotManifestDigest(snapshot)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.ManifestDigest = manifestDigest
	if err := setCanonicalDigest(&snapshot.Ref, snapshot); err != nil {
		return Snapshot{}, err
	}
	t.snapshots[in.ID] = append(history, snapshot)
	return cloneSnapshot(snapshot), nil
}

func (s *Store) PublishSnapshot(access Access, readyRef contract.Ref, expectedPointer contract.ExpectedRevision) (SnapshotPointer, Snapshot, error) {
	if err := access.Validate(); err != nil {
		return SnapshotPointer{}, Snapshot{}, err
	}
	if err := expectedPointer.Validate(); err != nil {
		return SnapshotPointer{}, Snapshot{}, err
	}
	if err := readyRef.Validate(); err != nil {
		return SnapshotPointer{}, Snapshot{}, err
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return SnapshotPointer{}, Snapshot{}, err
	}
	if t.current != nil && t.current.TargetRef.ID == readyRef.ID && t.current.TargetRef.Revision == readyRef.Revision+1 {
		published, ok := snapshotByRef(t, t.current.TargetRef)
		if ok && contract.SameRef(published.BuiltFrom, readyRef) {
			return clonePointer(*t.current), cloneSnapshot(published), nil
		}
	}
	for _, published := range t.snapshots[readyRef.ID] {
		if published.State == SnapshotPublished && contract.SameRef(published.BuiltFrom, readyRef) {
			return SnapshotPointer{}, Snapshot{}, contract.ErrNotCurrent
		}
	}
	ready, ok := snapshotByRef(t, readyRef)
	if !ok || ready.State != SnapshotReady {
		return SnapshotPointer{}, Snapshot{}, contract.ErrNotFound
	}
	if !containsRef(ready.AuthorityRefs, access.AuthorityRef) || !containsRef(ready.PolicyRefs, access.PolicyRef) {
		return SnapshotPointer{}, Snapshot{}, contract.ErrScopeDenied
	}
	exists := t.current != nil
	pointerRevision := uint64(0)
	if exists {
		pointerRevision = t.current.Ref.Revision
	}
	if !expectedPointer.Matches(exists, pointerRevision) {
		return SnapshotPointer{}, Snapshot{}, contract.ErrRevisionConflict
	}
	published := cloneSnapshot(ready)
	published.Ref = contract.Ref{ID: ready.Ref.ID, Revision: ready.Ref.Revision + 1}
	published.State = SnapshotPublished
	published.BuiltFrom = readyRef
	published.PublishedAt = now
	if t.current != nil {
		published.Previous = t.current.TargetRef
	}
	if err := setCanonicalDigest(&published.Ref, published); err != nil {
		return SnapshotPointer{}, Snapshot{}, err
	}
	t.snapshots[published.Ref.ID] = append(t.snapshots[published.Ref.ID], published)
	pointer := SnapshotPointer{
		Ref:      contract.Ref{ID: "knowledge-current-snapshot", Revision: pointerRevision + 1},
		TenantID: access.TenantID, Owner: contract.OwnerKnowledge, TargetRef: published.Ref, UpdatedAt: now,
	}
	if t.current != nil {
		pointer.Previous = t.current.TargetRef
	}
	if err := setCanonicalDigest(&pointer.Ref, pointer); err != nil {
		return SnapshotPointer{}, Snapshot{}, err
	}
	t.current = &pointer
	return clonePointer(pointer), cloneSnapshot(published), nil
}

func (s *Store) CreateView(access Access, in ViewInput, expected contract.ExpectedRevision) (View, error) {
	if err := access.Validate(); err != nil {
		return View{}, err
	}
	if err := expected.Validate(); err != nil {
		return View{}, err
	}
	if access.TenantID != in.TenantID || !contract.SameRef(access.AuthorityRef, in.AuthorityRef) || !contract.SameRef(access.PolicyRef, in.PolicyRef) {
		return View{}, contract.ErrScopeDenied
	}
	if !validID(in.TenantID, in.ID, in.Purpose, in.SensitivityMax) || len(in.Scopes) == 0 || len(in.AllowedLicenses) == 0 {
		return View{}, fmt.Errorf("%w: incomplete view", contract.ErrInvalidArgument)
	}
	if err := validateTTL(in.TTL); err != nil {
		return View{}, err
	}
	if err := in.SnapshotRef.Validate(); err != nil {
		return View{}, err
	}
	if err := in.AuthorityRef.Validate(); err != nil {
		return View{}, err
	}
	if err := in.PolicyRef.Validate(); err != nil {
		return View{}, err
	}
	if err := validateRefs(in.ProjectionRefs); err != nil {
		return View{}, err
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(in.TenantID, true)
	if err != nil {
		return View{}, err
	}
	snapshot, ok := snapshotByRef(t, in.SnapshotRef)
	if !ok || snapshot.State != SnapshotPublished || !containsRef(snapshot.AuthorityRefs, in.AuthorityRef) || !containsRef(snapshot.PolicyRefs, in.PolicyRef) {
		return View{}, contract.ErrNotCurrent
	}
	for _, ref := range in.ProjectionRefs {
		projection, ok := projectionByRef(t, ref)
		if !ok || !now.Before(projection.ExpiresAt) {
			return View{}, contract.ErrNotCurrent
		}
	}
	history := t.views[in.ID]
	exists := len(history) != 0
	revision := uint64(0)
	if exists {
		revision = history[len(history)-1].Ref.Revision
	}
	if !expected.Matches(exists, revision) {
		return View{}, contract.ErrRevisionConflict
	}
	view := View{
		Ref: contract.Ref{ID: in.ID, Revision: revision + 1}, TenantID: in.TenantID, Owner: contract.OwnerKnowledge,
		SnapshotRef: in.SnapshotRef, AuthorityRef: in.AuthorityRef, PolicyRef: in.PolicyRef,
		ProjectionRefs: contract.NormalizeRefs(in.ProjectionRefs), Scopes: normalizeStrings(in.Scopes),
		AllowedLicenses: normalizeStrings(in.AllowedLicenses), SensitivityMax: in.SensitivityMax,
		Purpose: in.Purpose, CurrentOnly: in.CurrentOnly, CreatedAt: now, ExpiresAt: now.Add(in.TTL),
	}
	if err := setCanonicalDigest(&view.Ref, view); err != nil {
		return View{}, err
	}
	t.views[in.ID] = append(history, view)
	return cloneView(view), nil
}

func prepareRecordCAS(t *tenantState, candidate Candidate, expected contract.ExpectedRevision, now time.Time) (Record, *Tombstone, uint64, error) {
	history := t.records[candidate.Draft.ID]
	exists := len(history) != 0
	before := uint64(0)
	if exists {
		before = history[len(history)-1].Ref.Revision
	}
	if !expected.Matches(exists, before) {
		return Record{}, nil, before, contract.ErrRevisionConflict
	}
	if candidate.Kind == CandidateRecord && exists {
		return Record{}, nil, before, contract.ErrRevisionConflict
	}
	if candidate.Kind != CandidateRecord {
		if !exists || !contract.SameRef(history[len(history)-1].Ref, candidate.TargetRef) {
			return Record{}, nil, before, contract.ErrRevisionConflict
		}
	}
	if candidate.Kind == CandidateWithdraw {
		current := cloneRecord(history[len(history)-1])
		current.Ref = contract.Ref{ID: current.Ref.ID, Revision: before + 1}
		current.Status = RecordWithdrawn
		current.TransactionAt = now
		tomb := Tombstone{
			Ref: contract.Ref{ID: "record/" + current.Ref.ID + "/tombstone", Revision: 1}, TenantID: candidate.TenantID,
			Owner: contract.OwnerKnowledge, TargetKind: "record", TargetRef: candidate.TargetRef,
			Reason: candidate.PayloadDigest, CreatedAt: now,
		}
		if err := setCanonicalDigest(&tomb.Ref, tomb); err != nil {
			return Record{}, nil, before, err
		}
		current.WithdrawnBy = tomb.Ref
		if err := setCanonicalDigest(&current.Ref, current); err != nil {
			return Record{}, nil, before, err
		}
		return current, &tomb, before, nil
	}
	pkg, ok := packageByRef(t, candidate.Draft.PackageRef)
	if !ok || pkg.State != PackageReady {
		return Record{}, nil, before, contract.ErrNotCurrent
	}
	for _, sourceRef := range candidate.Draft.SourceRefs {
		source, ok := sourceByRef(t, sourceRef)
		if !ok || source.State == SourceWithdrawn || !now.Before(source.ValidTo) {
			return Record{}, nil, before, contract.ErrNotCurrent
		}
	}
	record := Record{
		Ref: contract.Ref{ID: candidate.Draft.ID, Revision: before + 1}, TenantID: candidate.TenantID, Owner: contract.OwnerKnowledge,
		PackageRef: candidate.Draft.PackageRef, AuthorityRef: pkg.AuthorityRef, PolicyRef: pkg.PolicyRef,
		ContentRef: candidate.Draft.ContentRef, SourceRefs: contract.NormalizeRefs(candidate.Draft.SourceRefs),
		EvidenceRefs: contract.NormalizeRefs(candidate.Draft.EvidenceRefs), Scope: candidate.Draft.Scope,
		Subject: candidate.Draft.Subject, Sensitivity: candidate.Draft.Sensitivity, License: candidate.Draft.License,
		TrustState: candidate.Draft.TrustState, ConflictGroup: candidate.Draft.ConflictGroup,
		Status: RecordActive, ValidFrom: candidate.Draft.ValidFrom.UTC(), ValidTo: candidate.Draft.ValidTo.UTC(), TransactionAt: now,
	}
	if candidate.Kind == CandidateCorrection {
		record.Corrects = candidate.TargetRef
	}
	if err := setCanonicalDigest(&record.Ref, record); err != nil {
		return Record{}, nil, before, err
	}
	return record, nil, before, nil
}

func validateDraft(kind CandidateKind, draft RecordDraft) error {
	if !validID(draft.ID) {
		return fmt.Errorf("%w: record id required", contract.ErrInvalidArgument)
	}
	if kind == CandidateWithdraw {
		return nil
	}
	if err := draft.PackageRef.Validate(); err != nil {
		return err
	}
	if err := draft.ContentRef.Validate(); err != nil {
		return err
	}
	if len(draft.SourceRefs) == 0 || !validID(draft.Scope, draft.Subject, draft.Sensitivity, draft.License, string(draft.TrustState)) {
		return fmt.Errorf("%w: incomplete record draft", contract.ErrInvalidArgument)
	}
	switch draft.TrustState {
	case TrustUnverified, TrustSourceSupported, TrustConflicted:
	default:
		return fmt.Errorf("%w: invalid candidate trust state", contract.ErrInvalidArgument)
	}
	if err := validateRefs(append(slices.Clone(draft.SourceRefs), draft.EvidenceRefs...)); err != nil {
		return err
	}
	if draft.ValidFrom.IsZero() || draft.ValidTo.IsZero() || !draft.ValidTo.After(draft.ValidFrom) {
		return contract.ErrNotCurrent
	}
	return nil
}

func validAdmissionDecision(decision AdmissionDecision) bool {
	switch decision {
	case AdmissionRejected, AdmissionMerged, AdmissionConflictPending, AdmissionReviewRequired, AdmissionCommitReady:
		return true
	default:
		return false
	}
}

func validAdmissionTransition(from, to AdmissionDecision) bool {
	switch from {
	case AdmissionReviewRequired, AdmissionConflictPending:
		return to == AdmissionCommitReady || to == AdmissionRejected
	default:
		return false
	}
}

func producerKey(producer string, epoch, sequence uint64) string {
	return fmt.Sprintf("%s\x00%d\x00%d", producer, epoch, sequence)
}

func candidateSemanticDigest(c Candidate) (string, error) {
	return canonicalDigest(struct {
		ProducerID string
		Epoch      uint64
		Sequence   uint64
		Kind       CandidateKind
		Target     contract.Ref
		Draft      RecordDraft
		Payload    string
		Evidence   []contract.Ref
	}{c.ProducerID, c.SourceEpoch, c.SourceSequence, c.Kind, c.TargetRef, c.Draft, c.PayloadDigest, c.EvidenceRefs})
}

func snapshotManifestDigest(snapshot Snapshot) (string, error) {
	return canonicalDigest(struct {
		Version                                                        string
		Sources, Packages, Records, Projections, Authorities, Policies []contract.Ref
		Coverage                                                       contract.Coverage
	}{snapshot.Version, snapshot.SourceRefs, snapshot.PackageRefs, snapshot.RecordRefs, snapshot.ProjectionRefs, snapshot.AuthorityRefs, snapshot.PolicyRefs, snapshot.Coverage})
}

func canonicalDigest(value any) (string, error) {
	digest, err := contract.Digest(value)
	if err != nil {
		return "", fmt.Errorf("%w: canonical digest: %v", contract.ErrInvalidArgument, err)
	}
	return digest, nil
}

func setCanonicalDigest(ref *contract.Ref, value any) error {
	digest, err := canonicalDigest(value)
	if err != nil {
		return err
	}
	ref.Digest = digest
	return nil
}

func refKey(ref contract.Ref) string {
	return fmt.Sprintf("%s\x00%d\x00%s", ref.ID, ref.Revision, ref.Digest)
}

func containsRef(refs []contract.Ref, want contract.Ref) bool {
	return slices.ContainsFunc(refs, func(ref contract.Ref) bool { return contract.SameRef(ref, want) })
}

func accessAllows(access Access, authority, policy contract.Ref) bool {
	return contract.SameRef(access.AuthorityRef, authority) && contract.SameRef(access.PolicyRef, policy)
}

func candidateAccess(t *tenantState, access Access, candidate Candidate) error {
	if candidate.TenantID != access.TenantID || candidate.Owner != contract.OwnerKnowledge {
		return contract.ErrScopeDenied
	}
	if candidate.Kind == CandidateRecord || candidate.Kind == CandidateCorrection {
		pkg, ok := packageByRef(t, candidate.Draft.PackageRef)
		if !ok {
			return contract.ErrNotFound
		}
		if !accessAllows(access, pkg.AuthorityRef, pkg.PolicyRef) {
			return contract.ErrScopeDenied
		}
		return nil
	}
	record, ok := recordByRef(t, candidate.TargetRef)
	if !ok {
		return contract.ErrNotFound
	}
	if !accessAllows(access, record.AuthorityRef, record.PolicyRef) {
		return contract.ErrScopeDenied
	}
	return nil
}

func sourceByRef(t *tenantState, ref contract.Ref) (Source, bool) {
	for _, item := range t.sources[ref.ID] {
		if contract.SameRef(item.Ref, ref) {
			return item, true
		}
	}
	return Source{}, false
}

func packageByRef(t *tenantState, ref contract.Ref) (Package, bool) {
	for _, item := range t.packages[ref.ID] {
		if contract.SameRef(item.Ref, ref) {
			return item, true
		}
	}
	return Package{}, false
}

func admissionByRef(t *tenantState, ref contract.Ref) (Admission, bool) {
	for _, item := range t.admissions[ref.ID] {
		if contract.SameRef(item.Ref, ref) {
			return item, true
		}
	}
	return Admission{}, false
}

func recordByRef(t *tenantState, ref contract.Ref) (Record, bool) {
	for _, item := range t.records[ref.ID] {
		if contract.SameRef(item.Ref, ref) {
			return item, true
		}
	}
	return Record{}, false
}

func projectionByRef(t *tenantState, ref contract.Ref) (Projection, bool) {
	for _, item := range t.projections[ref.ID] {
		if contract.SameRef(item.Ref, ref) {
			return item, true
		}
	}
	return Projection{}, false
}

func snapshotByRef(t *tenantState, ref contract.Ref) (Snapshot, bool) {
	for _, item := range t.snapshots[ref.ID] {
		if contract.SameRef(item.Ref, ref) {
			return item, true
		}
	}
	return Snapshot{}, false
}

func viewByRef(t *tenantState, ref contract.Ref) (View, bool) {
	for _, item := range t.views[ref.ID] {
		if contract.SameRef(item.Ref, ref) {
			return item, true
		}
	}
	return View{}, false
}

func cloneSource(v Source) Source { v.Provenance = slices.Clone(v.Provenance); return v }
func clonePackage(v Package) Package {
	v.SourceRefs = slices.Clone(v.SourceRefs)
	v.Coverage = cloneCoverage(v.Coverage)
	return v
}
func cloneDraft(v RecordDraft) RecordDraft {
	v.SourceRefs = slices.Clone(v.SourceRefs)
	v.EvidenceRefs = slices.Clone(v.EvidenceRefs)
	return v
}
func cloneCandidate(v Candidate) Candidate {
	v.Draft = cloneDraft(v.Draft)
	v.EvidenceRefs = slices.Clone(v.EvidenceRefs)
	v.RiskFlags = slices.Clone(v.RiskFlags)
	return v
}
func cloneAdmission(v Admission) Admission { return v }
func cloneRecord(v Record) Record {
	v.SourceRefs = slices.Clone(v.SourceRefs)
	v.EvidenceRefs = slices.Clone(v.EvidenceRefs)
	return v
}
func cloneTombstone(v Tombstone) Tombstone { return v }
func cloneProjection(v Projection) Projection {
	v.RecordRefs = slices.Clone(v.RecordRefs)
	v.Coverage = cloneCoverage(v.Coverage)
	return v
}
func cloneSnapshot(v Snapshot) Snapshot {
	v.SourceRefs = slices.Clone(v.SourceRefs)
	v.PackageRefs = slices.Clone(v.PackageRefs)
	v.RecordRefs = slices.Clone(v.RecordRefs)
	v.ProjectionRefs = slices.Clone(v.ProjectionRefs)
	v.AuthorityRefs = slices.Clone(v.AuthorityRefs)
	v.PolicyRefs = slices.Clone(v.PolicyRefs)
	v.Coverage = cloneCoverage(v.Coverage)
	return v
}
func clonePointer(v SnapshotPointer) SnapshotPointer { return v }
func cloneView(v View) View {
	v.ProjectionRefs = slices.Clone(v.ProjectionRefs)
	v.Scopes = slices.Clone(v.Scopes)
	v.AllowedLicenses = slices.Clone(v.AllowedLicenses)
	return v
}
func cloneAttempt(v CommitAttempt) CommitAttempt { return v }
func cloneInspection(v Inspection) Inspection    { return v }
func cloneCoverage(v contract.Coverage) contract.Coverage {
	v.ProjectionRefs = slices.Clone(v.ProjectionRefs)
	v.DroppedReasons = slices.Clone(v.DroppedReasons)
	return v
}
func cloneResult(v contract.DomainResultFact) contract.DomainResultFact {
	v.EvidenceRefs = slices.Clone(v.EvidenceRefs)
	v.Residuals = slices.Clone(v.Residuals)
	v.Coverage = cloneCoverage(v.Coverage)
	return v
}
func cloneApplication(v contract.SettlementApplication) contract.SettlementApplication { return v }
