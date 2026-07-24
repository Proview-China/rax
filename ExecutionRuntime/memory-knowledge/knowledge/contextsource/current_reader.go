// Package contextsource implements the Knowledge Owner-local current reader.
// It contains no retrieval, provider, resolver, network, or Context adapter.
package contextsource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	contextsourcev2 "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/internal/contextsourcev2"
)

const (
	ContractVersion             = "praxis.knowledge/context-source-current-reader/v1"
	localAttemptKind            = "knowledge_local_retrieval_attempt"
	currentStateKind            = "knowledge_current_state"
	attemptInspectionKind       = "knowledge_local_attempt_inspection"
	currentProjectionKind       = "knowledge_contribution_current_projection"
	exactContentObservationKind = "knowledge_local_exact_content_observation"
	statePlaneBindingKind       = "knowledge_owner_local_state_plane_binding"
	localExactAccessKind        = "owner_state_plane_local_exact"
)

type StatePlaneBinding struct {
	ContractVersion string               `json:"contract_version"`
	ObjectKind      string               `json:"object_kind"`
	Ref             contract.Ref         `json:"ref"`
	Owner           contract.OwnerDomain `json:"owner"`
	StoreDomain     string               `json:"store_domain"`
	AccessKind      string               `json:"access_kind"`
	RemoteLocator   string               `json:"remote_locator,omitempty"`
	ExpiresAt       time.Time            `json:"expires_at"`
}

type LocalContentReader interface {
	ownerLocalStatePlaneReader()
	StatePlaneBinding() StatePlaneBinding
	Get(contract.ContentRef) ([]byte, error)
}

type StatePlaneContentStore struct {
	mu      sync.RWMutex
	binding StatePlaneBinding
	bodies  map[string][]byte
}

func NewStatePlaneContentStore(binding StatePlaneBinding) (*StatePlaneContentStore, error) {
	if err := validateStatePlaneBinding(binding, time.Time{}); err != nil {
		return nil, err
	}
	return &StatePlaneContentStore{binding: binding, bodies: make(map[string][]byte)}, nil
}

func (*StatePlaneContentStore) ownerLocalStatePlaneReader() {}

func (s *StatePlaneContentStore) StatePlaneBinding() StatePlaneBinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.binding
}

func (s *StatePlaneContentStore) StatePlaneBindingContext(ctx context.Context) (StatePlaneBinding, error) {
	if err := contextsourcev2.RLock(ctx, &s.mu); err != nil {
		return StatePlaneBinding{}, err
	}
	defer s.mu.RUnlock()
	return s.binding, nil
}

func (s *StatePlaneContentStore) PutExact(body []byte, mediaType string) (contract.ContentRef, error) {
	if strings.TrimSpace(mediaType) == "" {
		return contract.ContentRef{}, contract.ErrInvalidArgument
	}
	digest := digestBytes(body)
	ref := contract.ContentRef{ID: digest, Digest: digest, Length: int64(len(body)), MediaType: mediaType}
	s.mu.Lock()
	s.bodies[ref.ID] = bytes.Clone(body)
	s.mu.Unlock()
	return ref, nil
}

func (s *StatePlaneContentStore) Get(ref contract.ContentRef) ([]byte, error) {
	s.mu.RLock()
	body, found := s.bodies[ref.ID]
	body = bytes.Clone(body)
	s.mu.RUnlock()
	if !found {
		return nil, contract.ErrNotFound
	}
	return body, nil
}

func (s *StatePlaneContentStore) GetExact(ctx context.Context, ref contract.ContentRef, maxBodyBytes int64) ([]byte, error) {
	if ctx == nil || maxBodyBytes <= 0 {
		return nil, contract.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ref.Length > maxBodyBytes {
		return nil, contract.ErrInvalidArgument
	}
	if err := contextsourcev2.RLock(ctx, &s.mu); err != nil {
		return nil, err
	}
	body, found := s.bodies[ref.ID]
	body = bytes.Clone(body)
	s.mu.RUnlock()
	if !found {
		return nil, contract.ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBodyBytes {
		return nil, contract.ErrInvalidArgument
	}
	return body, nil
}

func (s *StatePlaneContentStore) EvictExact(ref contract.ContentRef) {
	s.mu.Lock()
	delete(s.bodies, ref.ID)
	s.mu.Unlock()
}

type KnowledgeContextSourceCurrentReaderV1 interface {
	InspectAttempt(AttemptCoordinate) (AttemptInspection, error)
	InspectForTurn(CurrentRequest) (CurrentProjection, error)
	ReadContentExact(ExactContentRequest) (ExactContentObservation, []byte, error)
}

type AttemptStatus string

const (
	AttemptPersistedAndSettled AttemptStatus = "persisted_and_settled"
	AttemptPersistedUnsettled  AttemptStatus = "persisted_unsettled"
	AttemptNotPersisted        AttemptStatus = "confirmed_not_persisted"
)

type StoredItem struct {
	Rank              int                 `json:"rank"`
	Score             int                 `json:"score"`
	RecordRef         contract.Ref        `json:"record_ref"`
	PackageRef        contract.Ref        `json:"package_ref"`
	SnapshotRef       contract.Ref        `json:"snapshot_ref"`
	ContentRef        contract.ContentRef `json:"content_ref"`
	SourceRefs        []contract.Ref      `json:"source_refs"`
	EvidenceRefs      []contract.Ref      `json:"evidence_refs"`
	ProjectionRefs    []contract.Ref      `json:"projection_refs"`
	CitationDigest    string              `json:"citation_digest"`
	License           string              `json:"license"`
	TrustState        string              `json:"trust_state"`
	ConflictGroup     string              `json:"conflict_group,omitempty"`
	RecordExpiresAt   time.Time           `json:"record_expires_at"`
	SourceExpiresAt   time.Time           `json:"source_expires_at"`
	ProjectionExpires time.Time           `json:"projection_expires_at"`
}

type LocalAttempt struct {
	ContractVersion      string                           `json:"contract_version"`
	ObjectKind           string                           `json:"object_kind"`
	Ref                  contract.Ref                     `json:"ref"`
	Owner                contract.OwnerDomain             `json:"owner"`
	TenantID             string                           `json:"tenant_id"`
	ExecutionScopeDigest string                           `json:"execution_scope_digest"`
	RunID                string                           `json:"run_id"`
	TurnID               string                           `json:"turn_id"`
	RequestDigest        string                           `json:"request_digest"`
	IdempotencyKey       string                           `json:"idempotency_key"`
	ObservationRef       contract.Ref                     `json:"observation_ref"`
	ResultRef            contract.Ref                     `json:"result_ref"`
	QueryRef             contract.Ref                     `json:"query_ref"`
	ViewRef              contract.Ref                     `json:"view_ref"`
	SnapshotRef          contract.Ref                     `json:"snapshot_ref"`
	PointerRef           contract.Ref                     `json:"pointer_ref"`
	AuthorityRef         contract.Ref                     `json:"authority_ref"`
	PolicyRef            contract.Ref                     `json:"policy_ref"`
	Purpose              string                           `json:"purpose"`
	Scopes               []string                         `json:"scopes"`
	AllowedLicenses      []string                         `json:"allowed_licenses"`
	SensitivityMax       string                           `json:"sensitivity_max"`
	Coverage             contract.Coverage                `json:"coverage"`
	Items                []StoredItem                     `json:"items"`
	DomainResult         contract.DomainResultFact        `json:"domain_result"`
	Association          contract.DomainResultAssociation `json:"association"`
	Application          contract.SettlementApplication   `json:"application"`
	ObservedAt           time.Time                        `json:"observed_at"`
	ExpiresAt            time.Time                        `json:"expires_at"`
}

type CurrentItem struct {
	RecordRef         contract.Ref        `json:"record_ref"`
	PackageRef        contract.Ref        `json:"package_ref"`
	SnapshotRef       contract.Ref        `json:"snapshot_ref"`
	ContentRef        contract.ContentRef `json:"content_ref"`
	SourceRefs        []contract.Ref      `json:"source_refs"`
	ProjectionRefs    []contract.Ref      `json:"projection_refs"`
	License           string              `json:"license"`
	TrustState        string              `json:"trust_state"`
	ConflictGroup     string              `json:"conflict_group,omitempty"`
	Active            bool                `json:"active"`
	Withdrawn         bool                `json:"withdrawn"`
	PoisoningCleared  bool                `json:"poisoning_cleared"`
	RecordExpiresAt   time.Time           `json:"record_expires_at"`
	SourceExpiresAt   time.Time           `json:"source_expires_at"`
	ProjectionExpires time.Time           `json:"projection_expires_at"`
}

type CurrentState struct {
	ContractVersion string               `json:"contract_version"`
	ObjectKind      string               `json:"object_kind"`
	Ref             contract.Ref         `json:"ref"`
	Owner           contract.OwnerDomain `json:"owner"`
	TenantID        string               `json:"tenant_id"`
	AuthorityRef    contract.Ref         `json:"authority_ref"`
	PolicyRef       contract.Ref         `json:"policy_ref"`
	Purpose         string               `json:"purpose"`
	Scopes          []string             `json:"scopes"`
	AllowedLicenses []string             `json:"allowed_licenses"`
	SensitivityMax  string               `json:"sensitivity_max"`
	ViewRef         contract.Ref         `json:"view_ref"`
	SnapshotRef     contract.Ref         `json:"snapshot_ref"`
	PointerRef      contract.Ref         `json:"pointer_ref"`
	Items           []CurrentItem        `json:"items"`
	ExpiresAt       time.Time            `json:"expires_at"`
}

type AttemptCoordinate struct {
	ContractVersion      string
	TenantID             string
	ExecutionScopeDigest string
	RunID                string
	TurnID               string
	AttemptRef           contract.Ref
	RequestDigest        string
	IdempotencyKey       string
	ObservationRef       contract.Ref
	ResultRef            contract.Ref
}

type AttemptInspection struct {
	ContractVersion string               `json:"contract_version"`
	ObjectKind      string               `json:"object_kind"`
	Ref             contract.Ref         `json:"ref"`
	Owner           contract.OwnerDomain `json:"owner"`
	AttemptRef      contract.Ref         `json:"attempt_ref"`
	RunID           string               `json:"run_id"`
	TurnID          string               `json:"turn_id"`
	ObservationRef  contract.Ref         `json:"observation_ref,omitempty"`
	ResultRef       contract.Ref         `json:"result_ref,omitempty"`
	Status          AttemptStatus        `json:"status"`
	ObservedAt      time.Time            `json:"observed_at"`
	ExpiresAt       time.Time            `json:"expires_at"`
}

type CurrentRequest struct {
	ContractVersion         string
	Coordinate              AttemptCoordinate
	RunID                   string
	TurnID                  string
	CurrentStateRef         contract.Ref
	AuthorityRef            contract.Ref
	PolicyRef               contract.Ref
	Purpose                 string
	Scopes                  []string
	AllowedLicenses         []string
	SensitivityMax          string
	CheckedAt               time.Time
	NotAfter                time.Time
	ProjectionID            string
	ProjectionRevision      uint64
	ExpectedS1ClosureDigest string
}

type ProjectionItem struct {
	Rank            int                 `json:"rank"`
	Score           int                 `json:"score"`
	RecordRef       contract.Ref        `json:"record_ref"`
	PackageRef      contract.Ref        `json:"package_ref"`
	SnapshotRef     contract.Ref        `json:"snapshot_ref"`
	ContentRef      contract.ContentRef `json:"content_ref"`
	SourceRefs      []contract.Ref      `json:"source_refs"`
	EvidenceRefs    []contract.Ref      `json:"evidence_refs"`
	ProjectionRefs  []contract.Ref      `json:"projection_refs"`
	CitationDigest  string              `json:"citation_digest"`
	License         string              `json:"license"`
	TrustState      string              `json:"trust_state"`
	ConflictGroup   string              `json:"conflict_group,omitempty"`
	DomainResultRef contract.Ref        `json:"domain_result_ref"`
	ApplicationRef  contract.Ref        `json:"application_ref"`
	ExpiresAt       time.Time           `json:"expires_at"`
}

type CurrentProjection struct {
	ContractVersion      string               `json:"contract_version"`
	ObjectKind           string               `json:"object_kind"`
	Ref                  contract.Ref         `json:"ref"`
	Owner                contract.OwnerDomain `json:"owner"`
	Current              bool                 `json:"current"`
	AttemptRef           contract.Ref         `json:"attempt_ref"`
	ObservationRef       contract.Ref         `json:"observation_ref"`
	ResultRef            contract.Ref         `json:"result_ref"`
	CurrentStateRef      contract.Ref         `json:"current_state_ref"`
	StatePlaneBindingRef contract.Ref         `json:"state_plane_binding_ref"`
	RunID                string               `json:"run_id"`
	TurnID               string               `json:"turn_id"`
	ViewRef              contract.Ref         `json:"view_ref"`
	SnapshotRef          contract.Ref         `json:"snapshot_ref"`
	PointerRef           contract.Ref         `json:"pointer_ref"`
	AuthorityRef         contract.Ref         `json:"authority_ref"`
	PolicyRef            contract.Ref         `json:"policy_ref"`
	Purpose              string               `json:"purpose"`
	Scopes               []string             `json:"scopes"`
	AllowedLicenses      []string             `json:"allowed_licenses"`
	SensitivityMax       string               `json:"sensitivity_max"`
	Coverage             contract.Coverage    `json:"coverage"`
	Items                []ProjectionItem     `json:"items"`
	ClosureDigest        string               `json:"closure_digest"`
	CheckedAt            time.Time            `json:"checked_at"`
	ExpiresAt            time.Time            `json:"expires_at"`
}

type ExactContentRequest struct {
	ContractVersion string
	Projection      CurrentProjection
	RunID           string
	TurnID          string
	Rank            int
	CheckedAt       time.Time
	NotAfter        time.Time
}

type ExactContentObservation struct {
	ContractVersion      string               `json:"contract_version"`
	ObjectKind           string               `json:"object_kind"`
	Ref                  contract.Ref         `json:"ref"`
	Owner                contract.OwnerDomain `json:"owner"`
	ProjectionRef        contract.Ref         `json:"projection_ref"`
	StatePlaneBindingRef contract.Ref         `json:"state_plane_binding_ref"`
	ClosureDigest        string               `json:"closure_digest"`
	RunID                string               `json:"run_id"`
	TurnID               string               `json:"turn_id"`
	Rank                 int                  `json:"rank"`
	RecordRef            contract.Ref         `json:"record_ref"`
	ContentRef           contract.ContentRef  `json:"content_ref"`
	License              string               `json:"license"`
	ObservedLength       int64                `json:"observed_length"`
	ObservedMedia        string               `json:"observed_media_type"`
	ObservedDigest       string               `json:"observed_digest"`
	ObservedAt           time.Time            `json:"observed_at"`
	ExpiresAt            time.Time            `json:"expires_at"`
}

type Store struct {
	mu           sync.RWMutex
	clockMu      sync.Mutex
	lastOwnerNow time.Time
	clock        contract.Clock
	content      LocalContentReader
	binding      StatePlaneBinding
	attempts     map[string][]LocalAttempt
	attemptsV2   map[string][]AttemptCoordinateV2
	states       map[string][]CurrentState
}

var _ KnowledgeContextSourceCurrentReaderV1 = (*Store)(nil)

func NewStore(clock contract.Clock, content LocalContentReader) (*Store, error) {
	if clock == nil {
		clock = contract.SystemClock{}
	}
	if content == nil {
		return nil, contract.ErrInvalidArgument
	}
	binding := content.StatePlaneBinding()
	if err := validateStatePlaneBinding(binding, time.Time{}); err != nil {
		return nil, err
	}
	return &Store{clock: clock, content: content, binding: binding, attempts: make(map[string][]LocalAttempt), attemptsV2: make(map[string][]AttemptCoordinateV2), states: make(map[string][]CurrentState)}, nil
}

func (s *Store) PutAttempt(in LocalAttempt, expected contract.ExpectedRevision) (LocalAttempt, error) {
	if err := expected.Validate(); err != nil {
		return LocalAttempt{}, err
	}
	sealed, err := sealAttempt(in)
	if err != nil {
		return LocalAttempt{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.attempts[sealed.Ref.ID]
	current := lastAttemptRevision(versions)
	if !expected.Matches(len(versions) != 0, current) || sealed.Ref.Revision != current+1 {
		return LocalAttempt{}, contract.ErrRevisionConflict
	}
	s.attempts[sealed.Ref.ID] = append(versions, cloneAttempt(sealed))
	return cloneAttempt(sealed), nil
}

func (s *Store) PublishCurrent(in CurrentState, expected contract.ExpectedRevision) (CurrentState, error) {
	if err := expected.Validate(); err != nil {
		return CurrentState{}, err
	}
	sealed, err := sealCurrentState(in)
	if err != nil {
		return CurrentState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.states[sealed.Ref.ID]
	current := lastStateRevision(versions)
	if !expected.Matches(len(versions) != 0, current) || sealed.Ref.Revision != current+1 {
		return CurrentState{}, contract.ErrRevisionConflict
	}
	s.states[sealed.Ref.ID] = append(versions, cloneCurrentState(sealed))
	return cloneCurrentState(sealed), nil
}

func (s *Store) InspectAttempt(c AttemptCoordinate) (AttemptInspection, error) {
	if err := validateCoordinate(c); err != nil {
		return AttemptInspection{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	checked, err := s.freshOwnerNow()
	if err != nil {
		return AttemptInspection{}, err
	}
	versions := s.attempts[c.AttemptRef.ID]
	attempt, found := findAttempt(versions, c.AttemptRef)
	if len(versions) == 0 {
		return sealInspection(AttemptInspection{ContractVersion: ContractVersion, ObjectKind: attemptInspectionKind, Ref: contract.Ref{ID: "knowledge/context-inspection/" + c.AttemptRef.ID, Revision: c.AttemptRef.Revision}, Owner: contract.OwnerKnowledge, AttemptRef: c.AttemptRef, RunID: c.RunID, TurnID: c.TurnID, Status: AttemptNotPersisted, ObservedAt: checked, ExpiresAt: checked.Add(time.Minute)})
	}
	if !found {
		return AttemptInspection{}, contract.ErrEvidenceConflict
	}
	if err := validateAttempt(attempt); err != nil {
		return AttemptInspection{}, err
	}
	if !attempt.ExpiresAt.After(checked) {
		return AttemptInspection{}, contract.ErrNotCurrent
	}
	if attempt.TenantID != c.TenantID || attempt.ExecutionScopeDigest != c.ExecutionScopeDigest || attempt.RunID != c.RunID || attempt.TurnID != c.TurnID || attempt.RequestDigest != c.RequestDigest || attempt.IdempotencyKey != c.IdempotencyKey || !contract.SameRef(attempt.ObservationRef, c.ObservationRef) || !contract.SameRef(attempt.ResultRef, c.ResultRef) {
		return AttemptInspection{}, contract.ErrEvidenceConflict
	}
	status := AttemptPersistedAndSettled
	if err := validateSettlement(attempt); err != nil {
		status = AttemptPersistedUnsettled
	}
	return sealInspection(AttemptInspection{ContractVersion: ContractVersion, ObjectKind: attemptInspectionKind, Ref: contract.Ref{ID: "knowledge/context-inspection/" + attempt.Ref.ID, Revision: attempt.Ref.Revision}, Owner: contract.OwnerKnowledge, AttemptRef: attempt.Ref, RunID: attempt.RunID, TurnID: attempt.TurnID, ObservationRef: attempt.ObservationRef, ResultRef: attempt.ResultRef, Status: status, ObservedAt: checked, ExpiresAt: minTime(attempt.ExpiresAt, checked.Add(time.Minute))})
}

func (s *Store) InspectForTurn(req CurrentRequest) (CurrentProjection, error) {
	if err := validateCurrentRequest(req); err != nil {
		return CurrentProjection{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ownerNow, err := s.freshOwnerNow()
	if err != nil {
		return CurrentProjection{}, err
	}
	liveBinding := s.content.StatePlaneBinding()
	if err := validateStatePlaneBinding(liveBinding, ownerNow); err != nil || !contract.SameRef(liveBinding.Ref, s.binding.Ref) {
		return CurrentProjection{}, contract.ErrNotCurrent
	}
	attempt, found := findAttempt(s.attempts[req.Coordinate.AttemptRef.ID], req.Coordinate.AttemptRef)
	if !found {
		return CurrentProjection{}, contract.ErrNotFound
	}
	state, found := findCurrentState(s.states[req.CurrentStateRef.ID], req.CurrentStateRef)
	if !found {
		return CurrentProjection{}, contract.ErrNotCurrent
	}
	return buildProjection(attempt, state, req, ownerNow, s.binding)
}

func (s *Store) ReadContentExact(req ExactContentRequest) (ExactContentObservation, []byte, error) {
	if req.ContractVersion != ContractVersion || strings.TrimSpace(req.RunID) == "" || strings.TrimSpace(req.TurnID) == "" || req.RunID != req.Projection.RunID || req.TurnID != req.Projection.TurnID || req.CheckedAt.IsZero() {
		return ExactContentObservation{}, nil, contract.ErrInvalidArgument
	}
	if err := validateProjection(req.Projection); err != nil {
		return ExactContentObservation{}, nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	s1Now, err := s.freshOwnerNow()
	if err != nil {
		return ExactContentObservation{}, nil, err
	}
	if err := s.recheckExactLocked(req, s1Now); err != nil {
		return ExactContentObservation{}, nil, err
	}
	item, ok := itemByRank(req.Projection.Items, req.Rank)
	if !ok {
		return ExactContentObservation{}, nil, contract.ErrNotFound
	}
	if s.content == nil {
		return ExactContentObservation{}, nil, contract.ErrContextUnmaterialized
	}
	body, err := s.content.Get(item.ContentRef)
	if err != nil {
		return ExactContentObservation{}, nil, contract.ErrContextUnmaterialized
	}
	s2Now, err := s.freshOwnerNow()
	if err != nil {
		return ExactContentObservation{}, nil, err
	}
	if err := s.recheckExactLocked(req, s2Now); err != nil {
		return ExactContentObservation{}, nil, err
	}
	if !contentMatches(item.ContentRef, body) {
		return ExactContentObservation{}, nil, contract.ErrEvidenceConflict
	}
	body = bytes.Clone(body)
	observation := ExactContentObservation{ContractVersion: ContractVersion, ObjectKind: exactContentObservationKind, Ref: contract.Ref{ID: "knowledge/content-observation/" + req.Projection.Ref.ID + fmt.Sprintf("/%d", req.Rank), Revision: req.Projection.Ref.Revision}, Owner: contract.OwnerKnowledge, ProjectionRef: req.Projection.Ref, StatePlaneBindingRef: s.binding.Ref, ClosureDigest: req.Projection.ClosureDigest, RunID: req.RunID, TurnID: req.TurnID, Rank: req.Rank, RecordRef: item.RecordRef, ContentRef: item.ContentRef, License: item.License, ObservedLength: int64(len(body)), ObservedMedia: item.ContentRef.MediaType, ObservedDigest: digestBytes(body), ObservedAt: s2Now, ExpiresAt: minTime(req.Projection.ExpiresAt, req.NotAfter)}
	sealed, err := sealContentObservation(observation)
	if err != nil {
		return ExactContentObservation{}, nil, err
	}
	return sealed, body, nil
}

func (s *Store) recheckExactLocked(req ExactContentRequest, ownerNow time.Time) error {
	if !req.NotAfter.After(ownerNow) || !req.Projection.ExpiresAt.After(ownerNow) || !contract.SameRef(req.Projection.StatePlaneBindingRef, s.binding.Ref) {
		return contract.ErrNotCurrent
	}
	liveBinding := s.content.StatePlaneBinding()
	if err := validateStatePlaneBinding(liveBinding, ownerNow); err != nil || !contract.SameRef(liveBinding.Ref, s.binding.Ref) {
		return contract.ErrContextUnmaterialized
	}
	attempt, found := findAttempt(s.attempts[req.Projection.AttemptRef.ID], req.Projection.AttemptRef)
	if !found {
		return contract.ErrNotFound
	}
	state, found := findCurrentState(s.states[req.Projection.CurrentStateRef.ID], req.Projection.CurrentStateRef)
	if !found {
		return contract.ErrNotCurrent
	}
	_, err := buildProjection(attempt, state, CurrentRequest{ContractVersion: ContractVersion, Coordinate: coordinateFrom(attempt), RunID: req.RunID, TurnID: req.TurnID, CurrentStateRef: state.Ref, AuthorityRef: req.Projection.AuthorityRef, PolicyRef: req.Projection.PolicyRef, Purpose: req.Projection.Purpose, Scopes: req.Projection.Scopes, AllowedLicenses: req.Projection.AllowedLicenses, SensitivityMax: req.Projection.SensitivityMax, CheckedAt: req.CheckedAt, NotAfter: req.Projection.ExpiresAt, ProjectionID: req.Projection.Ref.ID, ProjectionRevision: req.Projection.Ref.Revision, ExpectedS1ClosureDigest: req.Projection.ClosureDigest}, ownerNow, s.binding)
	return err
}

func buildProjection(attempt LocalAttempt, state CurrentState, req CurrentRequest, ownerNow time.Time, binding StatePlaneBinding) (CurrentProjection, error) {
	if req.CheckedAt.After(ownerNow) {
		return CurrentProjection{}, contract.ErrNotCurrent
	}
	if err := validateAttempt(attempt); err != nil {
		return CurrentProjection{}, err
	}
	if err := validateSettlement(attempt); err != nil {
		return CurrentProjection{}, contract.ErrNotCurrent
	}
	if err := validateCurrentState(state); err != nil {
		return CurrentProjection{}, err
	}
	if !attempt.ExpiresAt.After(ownerNow) || !state.ExpiresAt.After(ownerNow) || !req.NotAfter.After(ownerNow) {
		return CurrentProjection{}, contract.ErrNotCurrent
	}
	if attempt.TenantID != state.TenantID || attempt.TenantID != req.Coordinate.TenantID || attempt.ExecutionScopeDigest != req.Coordinate.ExecutionScopeDigest || attempt.RunID != req.RunID || attempt.TurnID != req.TurnID || req.RunID != req.Coordinate.RunID || req.TurnID != req.Coordinate.TurnID || !contract.SameRef(attempt.Ref, req.Coordinate.AttemptRef) || attempt.RequestDigest != req.Coordinate.RequestDigest || attempt.IdempotencyKey != req.Coordinate.IdempotencyKey || !contract.SameRef(attempt.ObservationRef, req.Coordinate.ObservationRef) || !contract.SameRef(attempt.ResultRef, req.Coordinate.ResultRef) {
		return CurrentProjection{}, contract.ErrEvidenceConflict
	}
	if !contract.SameRef(attempt.ViewRef, state.ViewRef) || !contract.SameRef(attempt.SnapshotRef, state.SnapshotRef) || !contract.SameRef(attempt.PointerRef, state.PointerRef) || !contract.SameRef(attempt.AuthorityRef, req.AuthorityRef) || !contract.SameRef(state.AuthorityRef, req.AuthorityRef) || !contract.SameRef(attempt.PolicyRef, req.PolicyRef) || !contract.SameRef(state.PolicyRef, req.PolicyRef) || attempt.Purpose != req.Purpose || state.Purpose != req.Purpose || !sameStrings(attempt.Scopes, req.Scopes) || !sameStrings(state.Scopes, req.Scopes) || !sameStrings(attempt.AllowedLicenses, req.AllowedLicenses) || !sameStrings(state.AllowedLicenses, req.AllowedLicenses) || attempt.SensitivityMax != req.SensitivityMax || state.SensitivityMax != req.SensitivityMax {
		return CurrentProjection{}, contract.ErrNotCurrent
	}
	currentByID := make(map[string]CurrentItem, len(state.Items))
	for _, item := range state.Items {
		currentByID[item.RecordRef.ID] = item
	}
	items := make([]ProjectionItem, 0, len(attempt.Items))
	expires := minTime(attempt.ExpiresAt, state.ExpiresAt, binding.ExpiresAt, req.NotAfter)
	for _, stored := range attempt.Items {
		current, ok := currentByID[stored.RecordRef.ID]
		if !ok || !current.Active || current.Withdrawn || !current.PoisoningCleared || !contract.SameRef(current.RecordRef, stored.RecordRef) || !contract.SameRef(current.PackageRef, stored.PackageRef) || !contract.SameRef(current.SnapshotRef, stored.SnapshotRef) || current.ContentRef != stored.ContentRef || !sameRefs(current.SourceRefs, stored.SourceRefs) || !sameRefs(current.ProjectionRefs, stored.ProjectionRefs) || current.License != stored.License || current.TrustState != stored.TrustState || current.ConflictGroup != stored.ConflictGroup || !slices.Contains(req.AllowedLicenses, stored.License) {
			return CurrentProjection{}, contract.ErrNotCurrent
		}
		itemExpires := minTime(stored.RecordExpiresAt, stored.SourceExpiresAt, stored.ProjectionExpires, current.RecordExpiresAt, current.SourceExpiresAt, current.ProjectionExpires, binding.ExpiresAt, req.NotAfter)
		if !itemExpires.After(ownerNow) {
			return CurrentProjection{}, contract.ErrNotCurrent
		}
		expires = minTime(expires, itemExpires)
		items = append(items, ProjectionItem{Rank: stored.Rank, Score: stored.Score, RecordRef: stored.RecordRef, PackageRef: stored.PackageRef, SnapshotRef: stored.SnapshotRef, ContentRef: stored.ContentRef, SourceRefs: slices.Clone(stored.SourceRefs), EvidenceRefs: slices.Clone(stored.EvidenceRefs), ProjectionRefs: slices.Clone(stored.ProjectionRefs), CitationDigest: stored.CitationDigest, License: stored.License, TrustState: stored.TrustState, ConflictGroup: stored.ConflictGroup, DomainResultRef: attempt.DomainResult.Ref, ApplicationRef: attempt.Application.Ref, ExpiresAt: itemExpires})
	}
	projection := CurrentProjection{ContractVersion: ContractVersion, ObjectKind: currentProjectionKind, Ref: contract.Ref{ID: req.ProjectionID, Revision: req.ProjectionRevision}, Owner: contract.OwnerKnowledge, Current: true, AttemptRef: attempt.Ref, ObservationRef: attempt.ObservationRef, ResultRef: attempt.ResultRef, CurrentStateRef: state.Ref, StatePlaneBindingRef: binding.Ref, RunID: req.RunID, TurnID: req.TurnID, ViewRef: state.ViewRef, SnapshotRef: state.SnapshotRef, PointerRef: state.PointerRef, AuthorityRef: state.AuthorityRef, PolicyRef: state.PolicyRef, Purpose: state.Purpose, Scopes: slices.Clone(state.Scopes), AllowedLicenses: slices.Clone(state.AllowedLicenses), SensitivityMax: state.SensitivityMax, Coverage: normalizeCoverage(attempt.Coverage), Items: items, CheckedAt: ownerNow, ExpiresAt: expires}
	closure, err := closureDigest(projection)
	if err != nil {
		return CurrentProjection{}, err
	}
	projection.ClosureDigest = closure
	if req.ExpectedS1ClosureDigest != "" && req.ExpectedS1ClosureDigest != closure {
		return CurrentProjection{}, contract.ErrNotCurrent
	}
	return sealProjection(projection)
}

func sealAttempt(in LocalAttempt) (LocalAttempt, error) {
	if in.ContractVersion == "" {
		in.ContractVersion = ContractVersion
	}
	if in.ObjectKind == "" {
		in.ObjectKind = localAttemptKind
	}
	in.Owner = contract.OwnerKnowledge
	in.Scopes = normalizeStrings(in.Scopes)
	in.AllowedLicenses = normalizeStrings(in.AllowedLicenses)
	in.Coverage = normalizeCoverage(in.Coverage)
	in.Items = cloneStoredItems(in.Items)
	if hasDuplicateStoredRecord(in.Items) {
		return LocalAttempt{}, contract.ErrInvalidArgument
	}
	slices.SortFunc(in.Items, compareStoredItems)
	for i := range in.Items {
		in.Items[i].Rank = i
		in.Items[i].SourceRefs = contract.NormalizeRefs(in.Items[i].SourceRefs)
		in.Items[i].EvidenceRefs = contract.NormalizeRefs(in.Items[i].EvidenceRefs)
		in.Items[i].ProjectionRefs = contract.NormalizeRefs(in.Items[i].ProjectionRefs)
	}
	in.Ref.Digest = ""
	digest, err := contract.Digest(in)
	if err != nil {
		return LocalAttempt{}, fmt.Errorf("canonical knowledge local attempt: %w", err)
	}
	in.Ref.Digest = digest
	if err := validateAttempt(in); err != nil {
		return LocalAttempt{}, err
	}
	return in, nil
}

func validateAttempt(in LocalAttempt) error {
	if in.ContractVersion != ContractVersion || in.ObjectKind != localAttemptKind || in.Owner != contract.OwnerKnowledge || strings.TrimSpace(in.Ref.ID) == "" || in.Ref.Revision == 0 || strings.TrimSpace(in.Ref.Digest) == "" || strings.TrimSpace(in.TenantID) == "" || strings.TrimSpace(in.ExecutionScopeDigest) == "" || strings.TrimSpace(in.RunID) == "" || strings.TrimSpace(in.TurnID) == "" || strings.TrimSpace(in.RequestDigest) == "" || strings.TrimSpace(in.IdempotencyKey) == "" || strings.TrimSpace(in.Purpose) == "" || len(in.Scopes) == 0 || len(in.AllowedLicenses) == 0 || strings.TrimSpace(in.SensitivityMax) == "" || in.ObservedAt.IsZero() || in.ExpiresAt.IsZero() || len(in.Items) > 64 || hasDuplicateStoredRecord(in.Items) {
		return contract.ErrInvalidArgument
	}
	for _, ref := range []contract.Ref{in.ObservationRef, in.ResultRef, in.QueryRef, in.ViewRef, in.SnapshotRef, in.PointerRef, in.AuthorityRef, in.PolicyRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if err := in.DomainResult.Validate(); err != nil {
		return err
	}
	copy := cloneAttempt(in)
	copy.Ref.Digest = ""
	digest, err := contract.Digest(copy)
	if err != nil || digest != in.Ref.Digest {
		return contract.ErrEvidenceConflict
	}
	for i, item := range in.Items {
		if item.Rank != i || (i > 0 && compareStoredItems(in.Items[i-1], item) >= 0) || item.RecordRef.Validate() != nil || item.PackageRef.Validate() != nil || item.SnapshotRef.Validate() != nil || item.ContentRef.Validate() != nil || validateRefs(item.SourceRefs, true) != nil || validateRefs(item.EvidenceRefs, true) != nil || validateRefs(item.ProjectionRefs, true) != nil || strings.TrimSpace(item.CitationDigest) == "" || strings.TrimSpace(item.License) == "" || strings.TrimSpace(item.TrustState) == "" || !item.RecordExpiresAt.After(in.ObservedAt) || !item.SourceExpiresAt.After(in.ObservedAt) || !item.ProjectionExpires.After(in.ObservedAt) {
			return contract.ErrInvalidArgument
		}
	}
	return nil
}

func validateSettlement(in LocalAttempt) error {
	if in.DomainResult.Owner != contract.OwnerKnowledge || !contract.SameRef(in.DomainResult.SubjectRef, in.ResultRef) {
		return contract.ErrSettlementMismatch
	}
	if err := in.Association.Verify(in.DomainResult); err != nil {
		return err
	}
	if err := in.Application.Validate(); err != nil {
		return err
	}
	if in.Application.Owner != contract.OwnerKnowledge || !contract.SameRef(in.Application.DomainResultRef, in.DomainResult.Ref) {
		return contract.ErrSettlementMismatch
	}
	return nil
}

func sealCurrentState(in CurrentState) (CurrentState, error) {
	if in.ContractVersion == "" {
		in.ContractVersion = ContractVersion
	}
	if in.ObjectKind == "" {
		in.ObjectKind = currentStateKind
	}
	in.Owner = contract.OwnerKnowledge
	in.Scopes = normalizeStrings(in.Scopes)
	in.AllowedLicenses = normalizeStrings(in.AllowedLicenses)
	in.Items = cloneCurrentItems(in.Items)
	if hasDuplicateCurrentRecord(in.Items) {
		return CurrentState{}, contract.ErrInvalidArgument
	}
	slices.SortFunc(in.Items, compareCurrentItems)
	for i := range in.Items {
		in.Items[i].SourceRefs = contract.NormalizeRefs(in.Items[i].SourceRefs)
		in.Items[i].ProjectionRefs = contract.NormalizeRefs(in.Items[i].ProjectionRefs)
	}
	in.Ref.Digest = ""
	digest, err := contract.Digest(in)
	if err != nil {
		return CurrentState{}, fmt.Errorf("canonical knowledge current state: %w", err)
	}
	in.Ref.Digest = digest
	if err := validateCurrentState(in); err != nil {
		return CurrentState{}, err
	}
	return in, nil
}

func validateCurrentState(in CurrentState) error {
	if in.ContractVersion != ContractVersion || in.ObjectKind != currentStateKind || in.Owner != contract.OwnerKnowledge || strings.TrimSpace(in.Ref.ID) == "" || in.Ref.Revision == 0 || strings.TrimSpace(in.Ref.Digest) == "" || strings.TrimSpace(in.TenantID) == "" || strings.TrimSpace(in.Purpose) == "" || len(in.Scopes) == 0 || len(in.AllowedLicenses) == 0 || strings.TrimSpace(in.SensitivityMax) == "" || in.ExpiresAt.IsZero() || hasDuplicateCurrentRecord(in.Items) {
		return contract.ErrInvalidArgument
	}
	for _, ref := range []contract.Ref{in.AuthorityRef, in.PolicyRef, in.ViewRef, in.SnapshotRef, in.PointerRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	copy := cloneCurrentState(in)
	copy.Ref.Digest = ""
	digest, err := contract.Digest(copy)
	if err != nil || digest != in.Ref.Digest {
		return contract.ErrEvidenceConflict
	}
	for i, item := range in.Items {
		if (i > 0 && compareCurrentItems(in.Items[i-1], item) >= 0) || item.RecordRef.Validate() != nil || item.PackageRef.Validate() != nil || item.SnapshotRef.Validate() != nil || item.ContentRef.Validate() != nil || validateRefs(item.SourceRefs, true) != nil || validateRefs(item.ProjectionRefs, true) != nil || strings.TrimSpace(item.License) == "" || strings.TrimSpace(item.TrustState) == "" || item.RecordExpiresAt.IsZero() || item.SourceExpiresAt.IsZero() || item.ProjectionExpires.IsZero() {
			return contract.ErrInvalidArgument
		}
	}
	return nil
}

func validateCoordinate(c AttemptCoordinate) error {
	if c.ContractVersion != ContractVersion || strings.TrimSpace(c.TenantID) == "" || strings.TrimSpace(c.ExecutionScopeDigest) == "" || strings.TrimSpace(c.RunID) == "" || strings.TrimSpace(c.TurnID) == "" || strings.TrimSpace(c.RequestDigest) == "" || strings.TrimSpace(c.IdempotencyKey) == "" {
		return contract.ErrInvalidArgument
	}
	for _, ref := range []contract.Ref{c.AttemptRef, c.ObservationRef, c.ResultRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validateCurrentRequest(req CurrentRequest) error {
	if err := validateCoordinate(req.Coordinate); err != nil {
		return err
	}
	if req.ContractVersion != ContractVersion || strings.TrimSpace(req.RunID) == "" || strings.TrimSpace(req.TurnID) == "" || req.RunID != req.Coordinate.RunID || req.TurnID != req.Coordinate.TurnID || req.CurrentStateRef.Validate() != nil || req.AuthorityRef.Validate() != nil || req.PolicyRef.Validate() != nil || strings.TrimSpace(req.Purpose) == "" || len(req.Scopes) == 0 || len(req.AllowedLicenses) == 0 || strings.TrimSpace(req.SensitivityMax) == "" || req.CheckedAt.IsZero() || !req.NotAfter.After(req.CheckedAt) || strings.TrimSpace(req.ProjectionID) == "" || req.ProjectionRevision == 0 {
		return contract.ErrInvalidArgument
	}
	return nil
}

func sealProjection(in CurrentProjection) (CurrentProjection, error) {
	if in.ContractVersion == "" {
		in.ContractVersion = ContractVersion
	}
	if in.ObjectKind == "" {
		in.ObjectKind = currentProjectionKind
	}
	in.Scopes = normalizeStrings(in.Scopes)
	in.AllowedLicenses = normalizeStrings(in.AllowedLicenses)
	in.Coverage = normalizeCoverage(in.Coverage)
	in.Items = cloneProjectionItems(in.Items)
	in.Ref.Digest = ""
	digest, err := contract.Digest(in)
	if err != nil {
		return CurrentProjection{}, err
	}
	in.Ref.Digest = digest
	if err := validateProjection(in); err != nil {
		return CurrentProjection{}, err
	}
	return in, nil
}

func validateProjection(in CurrentProjection) error {
	if in.ContractVersion != ContractVersion || in.ObjectKind != currentProjectionKind || in.Owner != contract.OwnerKnowledge || !in.Current || in.Ref.Validate() != nil || in.AttemptRef.Validate() != nil || in.ObservationRef.Validate() != nil || in.ResultRef.Validate() != nil || in.CurrentStateRef.Validate() != nil || in.StatePlaneBindingRef.Validate() != nil || strings.TrimSpace(in.RunID) == "" || strings.TrimSpace(in.TurnID) == "" || in.ViewRef.Validate() != nil || in.SnapshotRef.Validate() != nil || in.PointerRef.Validate() != nil || in.AuthorityRef.Validate() != nil || in.PolicyRef.Validate() != nil || strings.TrimSpace(in.Purpose) == "" || len(in.Scopes) == 0 || len(in.AllowedLicenses) == 0 || strings.TrimSpace(in.SensitivityMax) == "" || strings.TrimSpace(in.ClosureDigest) == "" || in.CheckedAt.IsZero() || !in.ExpiresAt.After(in.CheckedAt) || hasDuplicateProjectionRecord(in.Items) {
		return contract.ErrInvalidArgument
	}
	copy := in
	copy.Ref.Digest = ""
	digest, err := contract.Digest(copy)
	if err != nil || digest != in.Ref.Digest {
		return contract.ErrEvidenceConflict
	}
	closure, err := closureDigest(in)
	if err != nil || closure != in.ClosureDigest {
		return contract.ErrEvidenceConflict
	}
	for i, item := range in.Items {
		if item.Rank != i || (i > 0 && compareProjectionItems(in.Items[i-1], item) >= 0) || item.RecordRef.Validate() != nil || item.PackageRef.Validate() != nil || item.SnapshotRef.Validate() != nil || item.ContentRef.Validate() != nil || item.DomainResultRef.Validate() != nil || item.ApplicationRef.Validate() != nil || validateRefs(item.SourceRefs, true) != nil || validateRefs(item.EvidenceRefs, true) != nil || validateRefs(item.ProjectionRefs, true) != nil {
			return contract.ErrInvalidArgument
		}
	}
	return nil
}

func closureDigest(in CurrentProjection) (string, error) {
	type closure struct {
		ContractVersion                                           string
		ObjectKind                                                string
		Owner                                                     contract.OwnerDomain
		AttemptRef, ObservationRef, ResultRef, CurrentStateRef    contract.Ref
		StatePlaneBindingRef                                      contract.Ref
		RunID, TurnID                                             string
		ViewRef, SnapshotRef, PointerRef, AuthorityRef, PolicyRef contract.Ref
		Purpose                                                   string
		Scopes, AllowedLicenses                                   []string
		SensitivityMax                                            string
		Coverage                                                  contract.Coverage
		Items                                                     []ProjectionItem
		ExpiresAt                                                 time.Time
	}
	return contract.Digest(closure{ContractVersion: in.ContractVersion, ObjectKind: in.ObjectKind, Owner: in.Owner, AttemptRef: in.AttemptRef, ObservationRef: in.ObservationRef, ResultRef: in.ResultRef, CurrentStateRef: in.CurrentStateRef, StatePlaneBindingRef: in.StatePlaneBindingRef, RunID: in.RunID, TurnID: in.TurnID, ViewRef: in.ViewRef, SnapshotRef: in.SnapshotRef, PointerRef: in.PointerRef, AuthorityRef: in.AuthorityRef, PolicyRef: in.PolicyRef, Purpose: in.Purpose, Scopes: normalizeStrings(in.Scopes), AllowedLicenses: normalizeStrings(in.AllowedLicenses), SensitivityMax: in.SensitivityMax, Coverage: normalizeCoverage(in.Coverage), Items: cloneProjectionItems(in.Items), ExpiresAt: in.ExpiresAt})
}

func sealInspection(in AttemptInspection) (AttemptInspection, error) {
	in.Ref.Digest = ""
	digest, err := contract.Digest(in)
	if err != nil {
		return AttemptInspection{}, err
	}
	in.Ref.Digest = digest
	if in.ContractVersion != ContractVersion || in.ObjectKind != attemptInspectionKind || in.Owner != contract.OwnerKnowledge || in.Ref.Validate() != nil || in.AttemptRef.Validate() != nil || strings.TrimSpace(in.RunID) == "" || strings.TrimSpace(in.TurnID) == "" || in.ObservedAt.IsZero() || !in.ExpiresAt.After(in.ObservedAt) {
		return AttemptInspection{}, contract.ErrInvalidArgument
	}
	return in, nil
}

func sealContentObservation(in ExactContentObservation) (ExactContentObservation, error) {
	in.Ref.Digest = ""
	digest, err := contract.Digest(in)
	if err != nil {
		return ExactContentObservation{}, err
	}
	in.Ref.Digest = digest
	if in.ContractVersion != ContractVersion || in.ObjectKind != exactContentObservationKind || in.Owner != contract.OwnerKnowledge || in.Ref.Validate() != nil || in.ProjectionRef.Validate() != nil || in.StatePlaneBindingRef.Validate() != nil || strings.TrimSpace(in.ClosureDigest) == "" || strings.TrimSpace(in.RunID) == "" || strings.TrimSpace(in.TurnID) == "" || in.RecordRef.Validate() != nil || in.ContentRef.Validate() != nil || strings.TrimSpace(in.License) == "" || in.ObservedAt.IsZero() || !in.ExpiresAt.After(in.ObservedAt) || in.ObservedLength != in.ContentRef.Length || in.ObservedDigest != in.ContentRef.Digest || in.ObservedMedia != in.ContentRef.MediaType {
		return ExactContentObservation{}, contract.ErrInvalidArgument
	}
	return in, nil
}

func coordinateFrom(in LocalAttempt) AttemptCoordinate {
	return AttemptCoordinate{ContractVersion: ContractVersion, TenantID: in.TenantID, ExecutionScopeDigest: in.ExecutionScopeDigest, RunID: in.RunID, TurnID: in.TurnID, AttemptRef: in.Ref, RequestDigest: in.RequestDigest, IdempotencyKey: in.IdempotencyKey, ObservationRef: in.ObservationRef, ResultRef: in.ResultRef}
}

func NewStatePlaneBinding(id string, revision uint64, storeDomain string, expiresAt time.Time) (StatePlaneBinding, error) {
	in := StatePlaneBinding{ContractVersion: ContractVersion, ObjectKind: statePlaneBindingKind, Ref: contract.Ref{ID: id, Revision: revision}, Owner: contract.OwnerKnowledge, StoreDomain: storeDomain, AccessKind: localExactAccessKind, ExpiresAt: expiresAt.UTC()}
	in.Ref.Digest = ""
	digest, err := contract.Digest(in)
	if err != nil {
		return StatePlaneBinding{}, err
	}
	in.Ref.Digest = digest
	if err := validateStatePlaneBinding(in, time.Time{}); err != nil {
		return StatePlaneBinding{}, err
	}
	return in, nil
}

func validateStatePlaneBinding(in StatePlaneBinding, ownerNow time.Time) error {
	if in.ContractVersion != ContractVersion || in.ObjectKind != statePlaneBindingKind || in.Owner != contract.OwnerKnowledge || in.Ref.Validate() != nil || strings.TrimSpace(in.StoreDomain) == "" || in.AccessKind != localExactAccessKind || strings.TrimSpace(in.RemoteLocator) != "" || in.ExpiresAt.IsZero() {
		return contract.ErrInvalidArgument
	}
	copy := in
	copy.Ref.Digest = ""
	digest, err := contract.Digest(copy)
	if err != nil || digest != in.Ref.Digest {
		return contract.ErrEvidenceConflict
	}
	if !ownerNow.IsZero() && !in.ExpiresAt.After(ownerNow) {
		return contract.ErrNotCurrent
	}
	return nil
}

func (s *Store) freshOwnerNow() (time.Time, error) {
	now := s.clock.Now().UTC()
	if now.IsZero() {
		return time.Time{}, contract.ErrNotCurrent
	}
	s.clockMu.Lock()
	defer s.clockMu.Unlock()
	if !s.lastOwnerNow.IsZero() && now.Before(s.lastOwnerNow) {
		return time.Time{}, contract.ErrNotCurrent
	}
	s.lastOwnerNow = now
	return now, nil
}

func findAttempt(versions []LocalAttempt, ref contract.Ref) (LocalAttempt, bool) {
	for _, item := range versions {
		if contract.SameRef(item.Ref, ref) {
			return cloneAttempt(item), true
		}
	}
	return LocalAttempt{}, false
}

func findCurrentState(versions []CurrentState, ref contract.Ref) (CurrentState, bool) {
	if len(versions) == 0 || !contract.SameRef(versions[len(versions)-1].Ref, ref) {
		return CurrentState{}, false
	}
	return cloneCurrentState(versions[len(versions)-1]), true
}

func itemByRank(items []ProjectionItem, rank int) (ProjectionItem, bool) {
	for _, item := range items {
		if item.Rank == rank {
			return item, true
		}
	}
	return ProjectionItem{}, false
}

func contentMatches(ref contract.ContentRef, body []byte) bool {
	return ref.Length == int64(len(body)) && strings.EqualFold(ref.Digest, digestBytes(body)) && ref.ID == ref.Digest
}

func digestBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func minTime(values ...time.Time) time.Time {
	var result time.Time
	for _, value := range values {
		if !value.IsZero() && (result.IsZero() || value.Before(result)) {
			result = value
		}
	}
	return result
}

func sameStrings(a, b []string) bool { return slices.Equal(normalizeStrings(a), normalizeStrings(b)) }
func sameRefs(a, b []contract.Ref) bool {
	return slices.EqualFunc(contract.NormalizeRefs(a), contract.NormalizeRefs(b), contract.SameRef)
}
func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}

func validateRefs(values []contract.Ref, required bool) error {
	if required && len(values) == 0 {
		return contract.ErrInvalidArgument
	}
	for _, ref := range values {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func compareRecordRef(a, b contract.Ref) int {
	if c := strings.Compare(a.ID, b.ID); c != 0 {
		return c
	}
	if a.Revision > b.Revision {
		return -1
	}
	if a.Revision < b.Revision {
		return 1
	}
	return strings.Compare(a.Digest, b.Digest)
}

func compareStoredItems(a, b StoredItem) int {
	if a.Score > b.Score {
		return -1
	}
	if a.Score < b.Score {
		return 1
	}
	return compareRecordRef(a.RecordRef, b.RecordRef)
}

func compareCurrentItems(a, b CurrentItem) int { return compareRecordRef(a.RecordRef, b.RecordRef) }

func compareProjectionItems(a, b ProjectionItem) int {
	if a.Score > b.Score {
		return -1
	}
	if a.Score < b.Score {
		return 1
	}
	return compareRecordRef(a.RecordRef, b.RecordRef)
}

func hasDuplicateStoredRecord(values []StoredItem) bool {
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		if _, exists := seen[item.RecordRef.ID]; exists {
			return true
		}
		seen[item.RecordRef.ID] = struct{}{}
	}
	return false
}

func hasDuplicateCurrentRecord(values []CurrentItem) bool {
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		if _, exists := seen[item.RecordRef.ID]; exists {
			return true
		}
		seen[item.RecordRef.ID] = struct{}{}
	}
	return false
}

func hasDuplicateProjectionRecord(values []ProjectionItem) bool {
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		if _, exists := seen[item.RecordRef.ID]; exists {
			return true
		}
		seen[item.RecordRef.ID] = struct{}{}
	}
	return false
}
func normalizeCoverage(v contract.Coverage) contract.Coverage {
	v.ProjectionRefs = contract.NormalizeRefs(v.ProjectionRefs)
	v.DroppedReasons = normalizeStrings(v.DroppedReasons)
	return v
}

func cloneCoverage(v contract.Coverage) contract.Coverage {
	v.ProjectionRefs = slices.Clone(v.ProjectionRefs)
	v.DroppedReasons = slices.Clone(v.DroppedReasons)
	return v
}

func cloneAttempt(v LocalAttempt) LocalAttempt {
	v.Scopes = slices.Clone(v.Scopes)
	v.AllowedLicenses = slices.Clone(v.AllowedLicenses)
	v.Coverage = cloneCoverage(v.Coverage)
	v.Items = cloneStoredItems(v.Items)
	v.DomainResult.EvidenceRefs = slices.Clone(v.DomainResult.EvidenceRefs)
	v.DomainResult.Residuals = slices.Clone(v.DomainResult.Residuals)
	v.DomainResult.Coverage = cloneCoverage(v.DomainResult.Coverage)
	return v
}

func cloneStoredItems(values []StoredItem) []StoredItem {
	out := slices.Clone(values)
	for i := range out {
		out[i].SourceRefs = slices.Clone(out[i].SourceRefs)
		out[i].EvidenceRefs = slices.Clone(out[i].EvidenceRefs)
		out[i].ProjectionRefs = slices.Clone(out[i].ProjectionRefs)
	}
	return out
}

func cloneCurrentState(v CurrentState) CurrentState {
	v.Scopes = slices.Clone(v.Scopes)
	v.AllowedLicenses = slices.Clone(v.AllowedLicenses)
	v.Items = cloneCurrentItems(v.Items)
	return v
}
func cloneCurrentItems(values []CurrentItem) []CurrentItem {
	out := slices.Clone(values)
	for i := range out {
		out[i].SourceRefs = slices.Clone(out[i].SourceRefs)
		out[i].ProjectionRefs = slices.Clone(out[i].ProjectionRefs)
	}
	return out
}
func cloneProjectionItems(values []ProjectionItem) []ProjectionItem {
	out := slices.Clone(values)
	for i := range out {
		out[i].SourceRefs = slices.Clone(out[i].SourceRefs)
		out[i].EvidenceRefs = slices.Clone(out[i].EvidenceRefs)
		out[i].ProjectionRefs = slices.Clone(out[i].ProjectionRefs)
	}
	return out
}
func lastAttemptRevision(values []LocalAttempt) uint64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1].Ref.Revision
}
func lastStateRevision(values []CurrentState) uint64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1].Ref.Revision
}
