package contextsource

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	contextsourcev2 "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/internal/contextsourcev2"
)

const (
	ContractVersionV2             = "praxis.knowledge/context-source-current-reader/v2"
	AttemptCoordinateKindV2       = "knowledge_context_source_attempt_coordinate"
	AttemptInspectionKindV2       = "knowledge_local_attempt_inspection"
	CurrentRequestKindV2          = "knowledge_context_source_current_request"
	ProjectionItemKindV2          = "knowledge_context_source_projection_item"
	CurrentProjectionKindV2       = "knowledge_contribution_current_projection"
	ExactContentRequestKindV2     = "knowledge_local_exact_content_request"
	ExactContentObservationKindV2 = "knowledge_local_exact_content_observation"

	KnowledgeStableClosureDigestDomainV2    = "praxis.knowledge/context-source-current-reader/stable-closure"
	KnowledgeStableClosureContractVersionV2 = "praxis.knowledge/context-source-current-reader/stable-closure/v2"
	KnowledgeStableClosureObjectKindV2      = "knowledge_context_source_stable_closure"
	KnowledgeSetDigestDomainV2              = "praxis.knowledge/context-source-current-reader/set-digest"
	KnowledgeSetDigestContractVersionV2     = "praxis.knowledge/context-source-current-reader/set-digest/v2"
	KnowledgeOrderedItemSetObjectKindV2     = "knowledge_ordered_item_set"
	KnowledgeContentSetObjectKindV2         = "knowledge_content_set"
	KnowledgeSourceSetObjectKindV2          = "knowledge_source_ref_set"
	KnowledgeEvidenceSetObjectKindV2        = "knowledge_evidence_ref_set"
	KnowledgeProjectionSetObjectKindV2      = "knowledge_projection_ref_set"
	KnowledgeCitationSetObjectKindV2        = "knowledge_citation_digest_set"
	KnowledgeLicenseSetObjectKindV2         = "knowledge_license_digest_set"
	KnowledgeConflictSetObjectKindV2        = "knowledge_conflict_digest_set"

	associationContractVersionV2 = "praxis.memory-knowledge/domain-result-association/v2"
	associationObjectKindV2      = "domain_result_association"
	maxV2Items                   = 1024
	maxV2BodyBytes               = int64(16 << 20)
)

type CheckPhaseV2 string

const (
	CheckPhaseS1V2 CheckPhaseV2 = "s1"
	CheckPhaseS2V2 CheckPhaseV2 = "s2"
)

type AttemptCoordinateV2 struct {
	ContractVersion      string       `json:"contract_version"`
	ObjectKind           string       `json:"object_kind"`
	TenantID             string       `json:"tenant_id"`
	IdentityRef          contract.Ref `json:"identity_ref"`
	IdentityEpoch        uint64       `json:"identity_epoch"`
	ExecutionScopeDigest string       `json:"execution_scope_digest"`
	RunID                string       `json:"run_id"`
	SessionRef           contract.Ref `json:"session_ref"`
	SessionEvidenceRef   contract.Ref `json:"session_evidence_ref"`
	SessionCheckedAt     time.Time    `json:"session_checked_at"`
	SessionExpiresAt     time.Time    `json:"session_expires_at"`
	SourceTurnOrdinal    uint32       `json:"source_turn_ordinal"`
	SourceTurnRef        contract.Ref `json:"source_turn_ref"`
	TurnEvidenceRef      contract.Ref `json:"turn_evidence_ref"`
	TurnCheckedAt        time.Time    `json:"turn_checked_at"`
	TurnExpiresAt        time.Time    `json:"turn_expires_at"`
	LegacyTurnID         string       `json:"legacy_turn_id"`
	AttemptRef           contract.Ref `json:"attempt_ref"`
	RequestDigest        string       `json:"request_digest"`
	IdempotencyKey       string       `json:"idempotency_key"`
	ObservationRef       contract.Ref `json:"observation_ref"`
	ResultRef            contract.Ref `json:"result_ref"`
	Digest               string       `json:"digest"`
}

type AttemptInspectionV2 struct {
	ContractVersion string               `json:"contract_version"`
	ObjectKind      string               `json:"object_kind"`
	Ref             contract.Ref         `json:"ref"`
	Owner           contract.OwnerDomain `json:"owner"`
	Coordinate      AttemptCoordinateV2  `json:"coordinate"`
	ObservationRef  *contract.Ref        `json:"observation_ref"`
	ResultRef       *contract.Ref        `json:"result_ref"`
	Status          AttemptStatus        `json:"status"`
	OwnerCheckedAt  time.Time            `json:"owner_checked_at"`
	ExpiresAt       time.Time            `json:"expires_at"`
	Digest          string               `json:"digest"`
}

type CurrentRequestV2 struct {
	ContractVersion         string              `json:"contract_version"`
	ObjectKind              string              `json:"object_kind"`
	Coordinate              AttemptCoordinateV2 `json:"coordinate"`
	CurrentStateRef         contract.Ref        `json:"current_state_ref"`
	ExpectedQueryRef        contract.Ref        `json:"expected_query_ref"`
	ExpectedViewRef         contract.Ref        `json:"expected_view_ref"`
	ExpectedSnapshotRef     contract.Ref        `json:"expected_snapshot_ref"`
	ExpectedPointerRef      contract.Ref        `json:"expected_pointer_ref"`
	AuthorityRef            contract.Ref        `json:"authority_ref"`
	AuthorityEpoch          uint64              `json:"authority_epoch"`
	PolicyRef               contract.Ref        `json:"policy_ref"`
	Purpose                 string              `json:"purpose"`
	Scopes                  []string            `json:"scopes"`
	AllowedLicenses         []string            `json:"allowed_licenses"`
	SensitivityMax          string              `json:"sensitivity_max"`
	CheckPhase              CheckPhaseV2        `json:"check_phase"`
	ExpectedS1ClosureDigest string              `json:"expected_s1_closure_digest"`
	MaxItems                int                 `json:"max_items"`
	MaxBytes                int64               `json:"max_bytes"`
	MaxTokens               int                 `json:"max_tokens"`
	PerItemMaxBytes         int64               `json:"per_item_max_bytes"`
	EstimatorRef            contract.Ref        `json:"estimator_ref"`
	CheckedUpperBound       time.Time           `json:"checked_upper_bound"`
	NotAfter                time.Time           `json:"not_after"`
	ProjectionID            string              `json:"projection_id"`
	ProjectionRevision      uint64              `json:"projection_revision"`
	Digest                  string              `json:"digest"`
}

type ProjectionItemV2 struct {
	ContractVersion   string              `json:"contract_version"`
	ObjectKind        string              `json:"object_kind"`
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
	LicenseDigest     string              `json:"license_digest"`
	TrustState        string              `json:"trust_state"`
	ConflictGroup     string              `json:"conflict_group"`
	ConflictDigest    string              `json:"conflict_digest"`
	DomainResultRef   contract.Ref        `json:"domain_result_ref"`
	AssociationDigest string              `json:"association_digest"`
	ApplicationRef    contract.Ref        `json:"application_ref"`
	TokenEstimate     int                 `json:"token_estimate"`
	EstimatorRef      contract.Ref        `json:"estimator_ref"`
	ExpiresAt         time.Time           `json:"expires_at"`
	Digest            string              `json:"digest"`
}

type CurrentProjectionV2 struct {
	ContractVersion      string               `json:"contract_version"`
	ObjectKind           string               `json:"object_kind"`
	Ref                  contract.Ref         `json:"ref"`
	Owner                contract.OwnerDomain `json:"owner"`
	Current              bool                 `json:"current"`
	Coordinate           AttemptCoordinateV2  `json:"coordinate"`
	AttemptInspectionRef contract.Ref         `json:"attempt_inspection_ref"`
	CurrentStateRef      contract.Ref         `json:"current_state_ref"`
	StatePlaneBindingRef contract.Ref         `json:"state_plane_binding_ref"`
	QueryRef             contract.Ref         `json:"query_ref"`
	ViewRef              contract.Ref         `json:"view_ref"`
	SnapshotRef          contract.Ref         `json:"snapshot_ref"`
	PointerRef           contract.Ref         `json:"pointer_ref"`
	AuthorityRef         contract.Ref         `json:"authority_ref"`
	AuthorityEpoch       uint64               `json:"authority_epoch"`
	PolicyRef            contract.Ref         `json:"policy_ref"`
	Purpose              string               `json:"purpose"`
	Scopes               []string             `json:"scopes"`
	AllowedLicenses      []string             `json:"allowed_licenses"`
	SensitivityMax       string               `json:"sensitivity_max"`
	Coverage             contract.Coverage    `json:"coverage"`
	NextCursor           string               `json:"next_cursor"`
	ResultDigest         string               `json:"result_digest"`
	EvidenceDigest       string               `json:"evidence_digest"`
	Items                []ProjectionItemV2   `json:"items"`
	OrderedItemSetDigest string               `json:"ordered_item_set_digest"`
	ContentSetDigest     string               `json:"content_set_digest"`
	SourceSetDigest      string               `json:"source_set_digest"`
	EvidenceSetDigest    string               `json:"evidence_set_digest"`
	ProjectionSetDigest  string               `json:"projection_set_digest"`
	CitationSetDigest    string               `json:"citation_set_digest"`
	LicenseSetDigest     string               `json:"license_set_digest"`
	ConflictSetDigest    string               `json:"conflict_set_digest"`
	MaxItems             int                  `json:"max_items"`
	MaxBytes             int64                `json:"max_bytes"`
	MaxTokens            int                  `json:"max_tokens"`
	PerItemMaxBytes      int64                `json:"per_item_max_bytes"`
	EstimatorRef         contract.Ref         `json:"estimator_ref"`
	StableClosureDigest  string               `json:"stable_closure_digest"`
	CheckPhase           CheckPhaseV2         `json:"check_phase"`
	OwnerCheckedAt       time.Time            `json:"owner_checked_at"`
	ExpiresAt            time.Time            `json:"expires_at"`
	Digest               string               `json:"digest"`
}

type ExactContentRequestV2 struct {
	ContractVersion             string              `json:"contract_version"`
	ObjectKind                  string              `json:"object_kind"`
	Coordinate                  AttemptCoordinateV2 `json:"coordinate"`
	Projection                  CurrentProjectionV2 `json:"projection"`
	Rank                        int                 `json:"rank"`
	CheckPhase                  CheckPhaseV2        `json:"check_phase"`
	ExpectedStableClosureDigest string              `json:"expected_stable_closure_digest"`
	MaxBodyBytes                int64               `json:"max_body_bytes"`
	CheckedUpperBound           time.Time           `json:"checked_upper_bound"`
	NotAfter                    time.Time           `json:"not_after"`
	Digest                      string              `json:"digest"`
}

type ExactContentObservationV2 struct {
	ContractVersion      string               `json:"contract_version"`
	ObjectKind           string               `json:"object_kind"`
	Ref                  contract.Ref         `json:"ref"`
	Owner                contract.OwnerDomain `json:"owner"`
	ProjectionRef        contract.Ref         `json:"projection_ref"`
	ProjectionItemDigest string               `json:"projection_item_digest"`
	StatePlaneBindingRef contract.Ref         `json:"state_plane_binding_ref"`
	StableClosureDigest  string               `json:"stable_closure_digest"`
	Coordinate           AttemptCoordinateV2  `json:"coordinate"`
	Rank                 int                  `json:"rank"`
	RecordRef            contract.Ref         `json:"record_ref"`
	PackageRef           contract.Ref         `json:"package_ref"`
	SnapshotRef          contract.Ref         `json:"snapshot_ref"`
	ContentRef           contract.ContentRef  `json:"content_ref"`
	License              string               `json:"license"`
	LicenseDigest        string               `json:"license_digest"`
	DomainResultRef      contract.Ref         `json:"domain_result_ref"`
	AssociationDigest    string               `json:"association_digest"`
	ApplicationRef       contract.Ref         `json:"application_ref"`
	ObservedLength       int64                `json:"observed_length"`
	ObservedMediaType    string               `json:"observed_media_type"`
	ObservedDigest       string               `json:"observed_digest"`
	CheckPhase           CheckPhaseV2         `json:"check_phase"`
	OwnerObservedAt      time.Time            `json:"owner_observed_at"`
	ExpiresAt            time.Time            `json:"expires_at"`
	Digest               string               `json:"digest"`
}

type KnowledgeContextSourceCurrentReaderV2 interface {
	InspectAttempt(context.Context, AttemptCoordinateV2) (AttemptInspectionV2, error)
	InspectForTurn(context.Context, CurrentRequestV2) (CurrentProjectionV2, error)
	ReadContentExact(context.Context, ExactContentRequestV2) (ExactContentObservationV2, []byte, error)
}

type localContentReaderV2 interface {
	LocalContentReader
	StatePlaneBindingContext(context.Context) (StatePlaneBinding, error)
	GetExact(context.Context, contract.ContentRef, int64) ([]byte, error)
}

type CurrentReaderV2 struct {
	store   *Store
	content localContentReaderV2
}

var _ KnowledgeContextSourceCurrentReaderV2 = (*CurrentReaderV2)(nil)

func NewCurrentReaderV2(store *Store) (*CurrentReaderV2, error) {
	if store == nil {
		return nil, contract.ErrInvalidArgument
	}
	content, ok := store.content.(localContentReaderV2)
	if !ok {
		return nil, contract.ErrUnsupported
	}
	return &CurrentReaderV2{store: store, content: content}, nil
}

func (s *Store) PutAttemptV2(in LocalAttempt, coordinate AttemptCoordinateV2, expected contract.ExpectedRevision) (LocalAttempt, AttemptCoordinateV2, error) {
	if err := expected.Validate(); err != nil {
		return LocalAttempt{}, AttemptCoordinateV2{}, err
	}
	sealedAttempt, err := sealAttempt(in)
	if err != nil {
		return LocalAttempt{}, AttemptCoordinateV2{}, err
	}
	coordinate.AttemptRef = sealedAttempt.Ref
	coordinate.ObservationRef = sealedAttempt.ObservationRef
	coordinate.ResultRef = sealedAttempt.ResultRef
	coordinate, err = SealAttemptCoordinateV2(coordinate)
	if err != nil {
		return LocalAttempt{}, AttemptCoordinateV2{}, err
	}
	if err := coordinateMatchesAttemptV2(coordinate, sealedAttempt); err != nil {
		return LocalAttempt{}, AttemptCoordinateV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.attempts[sealedAttempt.Ref.ID]
	if !expected.Matches(len(versions) != 0, lastAttemptRevision(versions)) || sealedAttempt.Ref.Revision != lastAttemptRevision(versions)+1 {
		return LocalAttempt{}, AttemptCoordinateV2{}, contract.ErrRevisionConflict
	}
	if len(s.attemptsV2[sealedAttempt.Ref.ID]) != 0 && sealedAttempt.Ref.Revision <= s.attemptsV2[sealedAttempt.Ref.ID][len(s.attemptsV2[sealedAttempt.Ref.ID])-1].AttemptRef.Revision {
		return LocalAttempt{}, AttemptCoordinateV2{}, contract.ErrRevisionConflict
	}
	s.attempts[sealedAttempt.Ref.ID] = append(versions, cloneAttempt(sealedAttempt))
	s.attemptsV2[sealedAttempt.Ref.ID] = append(s.attemptsV2[sealedAttempt.Ref.ID], coordinate)
	return cloneAttempt(sealedAttempt), coordinate, nil
}

func (r *CurrentReaderV2) InspectAttempt(ctx context.Context, coordinate AttemptCoordinateV2) (AttemptInspectionV2, error) {
	if err := checkContextV2(ctx); err != nil {
		return AttemptInspectionV2{}, err
	}
	if err := coordinate.Validate(); err != nil {
		return AttemptInspectionV2{}, err
	}
	if err := contextsourcev2.RLock(ctx, &r.store.mu); err != nil {
		return AttemptInspectionV2{}, err
	}
	defer r.store.mu.RUnlock()
	now, err := r.store.freshOwnerNow()
	if err != nil {
		return AttemptInspectionV2{}, err
	}
	if err := validateCoordinateCurrentV2(coordinate, now); err != nil {
		return AttemptInspectionV2{}, err
	}
	coordinates := r.store.attemptsV2[coordinate.AttemptRef.ID]
	if len(coordinates) == 0 {
		return sealAttemptInspectionV2(AttemptInspectionV2{ContractVersion: ContractVersionV2, ObjectKind: AttemptInspectionKindV2, Ref: contract.Ref{ID: "knowledge/context-inspection/v2/" + coordinate.AttemptRef.ID, Revision: coordinate.AttemptRef.Revision}, Owner: contract.OwnerKnowledge, Coordinate: coordinate, Status: AttemptNotPersisted, OwnerCheckedAt: now, ExpiresAt: minTime(now.Add(time.Minute), minTime(coordinate.SessionExpiresAt, coordinate.TurnExpiresAt))})
	}
	stored, found := findCoordinateV2(coordinates, coordinate.AttemptRef)
	if !found || !sameCoordinateV2(stored, coordinate) {
		return AttemptInspectionV2{}, contract.ErrEvidenceConflict
	}
	attempt, found := findAttempt(r.store.attempts[coordinate.AttemptRef.ID], coordinate.AttemptRef)
	if !found {
		return AttemptInspectionV2{}, contract.ErrEvidenceConflict
	}
	if err := validateAttempt(attempt); err != nil {
		return AttemptInspectionV2{}, err
	}
	if err := validateCoordinateCurrentV2(stored, now); err != nil || !attempt.ExpiresAt.After(now) {
		return AttemptInspectionV2{}, contract.ErrNotCurrent
	}
	status := AttemptPersistedAndSettled
	if err := validateSettlement(attempt); err != nil {
		status = AttemptPersistedUnsettled
	}
	observation, result := attempt.ObservationRef, attempt.ResultRef
	return sealAttemptInspectionV2(AttemptInspectionV2{ContractVersion: ContractVersionV2, ObjectKind: AttemptInspectionKindV2, Ref: contract.Ref{ID: "knowledge/context-inspection/v2/" + attempt.Ref.ID, Revision: attempt.Ref.Revision}, Owner: contract.OwnerKnowledge, Coordinate: stored, ObservationRef: &observation, ResultRef: &result, Status: status, OwnerCheckedAt: now, ExpiresAt: minTime(attempt.ExpiresAt, minTime(stored.SessionExpiresAt, minTime(stored.TurnExpiresAt, now.Add(time.Minute))))})
}

func (r *CurrentReaderV2) InspectForTurn(ctx context.Context, req CurrentRequestV2) (CurrentProjectionV2, error) {
	if err := checkContextV2(ctx); err != nil {
		return CurrentProjectionV2{}, err
	}
	if err := req.Validate(); err != nil {
		return CurrentProjectionV2{}, err
	}
	if err := contextsourcev2.RLock(ctx, &r.store.mu); err != nil {
		return CurrentProjectionV2{}, err
	}
	defer r.store.mu.RUnlock()
	now, err := r.store.freshOwnerNow()
	if err != nil {
		return CurrentProjectionV2{}, err
	}
	projection, err := r.buildProjectionLocked(ctx, req, now)
	if err != nil {
		return CurrentProjectionV2{}, err
	}
	if err := checkContextV2(ctx); err != nil {
		return CurrentProjectionV2{}, err
	}
	return cloneProjectionV2(projection), nil
}

func (r *CurrentReaderV2) ReadContentExact(ctx context.Context, req ExactContentRequestV2) (ExactContentObservationV2, []byte, error) {
	if err := checkContextV2(ctx); err != nil {
		return ExactContentObservationV2{}, nil, err
	}
	if err := req.Validate(); err != nil {
		return ExactContentObservationV2{}, nil, err
	}
	if err := contextsourcev2.RLock(ctx, &r.store.mu); err != nil {
		return ExactContentObservationV2{}, nil, err
	}
	defer r.store.mu.RUnlock()
	s1Now, err := r.store.freshOwnerNow()
	if err != nil {
		return ExactContentObservationV2{}, nil, err
	}
	if err := validateRequestTimeV2(req.CheckedUpperBound, req.NotAfter, s1Now); err != nil {
		return ExactContentObservationV2{}, nil, err
	}
	current, err := r.rebuildProjectionLocked(ctx, req.Projection, s1Now)
	if err != nil {
		return ExactContentObservationV2{}, nil, err
	}
	if !sameProjectionStableV2(current, req.Projection) || req.ExpectedStableClosureDigest != current.StableClosureDigest || !sameCoordinateV2(req.Coordinate, current.Coordinate) {
		return ExactContentObservationV2{}, nil, contract.ErrEvidenceConflict
	}
	item, ok := projectionItemByRankV2(current.Items, req.Rank)
	if !ok {
		return ExactContentObservationV2{}, nil, contract.ErrNotFound
	}
	if item.ContentRef.Length > req.MaxBodyBytes {
		return ExactContentObservationV2{}, nil, contract.ErrInvalidArgument
	}
	body, err := r.content.GetExact(ctx, item.ContentRef, req.MaxBodyBytes)
	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded || err == contract.ErrInvalidArgument {
			return ExactContentObservationV2{}, nil, err
		}
		return ExactContentObservationV2{}, nil, contract.ErrContextUnmaterialized
	}
	s2Now, err := r.store.freshOwnerNow()
	if err != nil {
		return ExactContentObservationV2{}, nil, err
	}
	if err := validateRequestTimeV2(req.CheckedUpperBound, req.NotAfter, s2Now); err != nil {
		return ExactContentObservationV2{}, nil, err
	}
	after, err := r.rebuildProjectionLocked(ctx, req.Projection, s2Now)
	if err != nil {
		return ExactContentObservationV2{}, nil, err
	}
	if !sameProjectionStableV2(current, after) || !contentMatches(item.ContentRef, body) || int64(len(body)) > req.MaxBodyBytes {
		return ExactContentObservationV2{}, nil, contract.ErrEvidenceConflict
	}
	if err := checkContextV2(ctx); err != nil {
		return ExactContentObservationV2{}, nil, err
	}
	observation := ExactContentObservationV2{ContractVersion: ContractVersionV2, ObjectKind: ExactContentObservationKindV2, Ref: contract.Ref{ID: "knowledge/content-observation/v2/" + req.Projection.Ref.ID + fmt.Sprintf("/%d/%s", req.Rank, req.CheckPhase), Revision: req.Projection.Ref.Revision}, Owner: contract.OwnerKnowledge, ProjectionRef: req.Projection.Ref, ProjectionItemDigest: item.Digest, StatePlaneBindingRef: after.StatePlaneBindingRef, StableClosureDigest: after.StableClosureDigest, Coordinate: after.Coordinate, Rank: item.Rank, RecordRef: item.RecordRef, PackageRef: item.PackageRef, SnapshotRef: item.SnapshotRef, ContentRef: item.ContentRef, License: item.License, LicenseDigest: item.LicenseDigest, DomainResultRef: item.DomainResultRef, AssociationDigest: item.AssociationDigest, ApplicationRef: item.ApplicationRef, ObservedLength: int64(len(body)), ObservedMediaType: item.ContentRef.MediaType, ObservedDigest: digestBytes(body), CheckPhase: req.CheckPhase, OwnerObservedAt: s2Now, ExpiresAt: minTime(after.ExpiresAt, req.NotAfter)}
	observation, err = sealExactContentObservationV2(observation)
	if err != nil {
		return ExactContentObservationV2{}, nil, err
	}
	return observation, bytes.Clone(body), nil
}

func (r *CurrentReaderV2) buildProjectionLocked(ctx context.Context, req CurrentRequestV2, now time.Time) (CurrentProjectionV2, error) {
	if err := validateRequestTimeV2(req.CheckedUpperBound, req.NotAfter, now); err != nil {
		return CurrentProjectionV2{}, err
	}
	if err := validateCoordinateCurrentV2(req.Coordinate, now); err != nil {
		return CurrentProjectionV2{}, err
	}
	storedCoordinate, found := findCoordinateV2(r.store.attemptsV2[req.Coordinate.AttemptRef.ID], req.Coordinate.AttemptRef)
	if !found || !sameCoordinateV2(storedCoordinate, req.Coordinate) {
		return CurrentProjectionV2{}, contract.ErrEvidenceConflict
	}
	attempt, found := findAttempt(r.store.attempts[req.Coordinate.AttemptRef.ID], req.Coordinate.AttemptRef)
	if !found {
		return CurrentProjectionV2{}, contract.ErrNotFound
	}
	state, found := findCurrentState(r.store.states[req.CurrentStateRef.ID], req.CurrentStateRef)
	if !found {
		return CurrentProjectionV2{}, contract.ErrNotCurrent
	}
	binding, err := r.content.StatePlaneBindingContext(ctx)
	if err != nil {
		return CurrentProjectionV2{}, err
	}
	if err := validateStatePlaneBinding(binding, now); err != nil || !contract.SameRef(binding.Ref, r.store.binding.Ref) {
		return CurrentProjectionV2{}, contract.ErrNotCurrent
	}
	v1req := CurrentRequest{ContractVersion: ContractVersion, Coordinate: coordinateFrom(attempt), RunID: attempt.RunID, TurnID: attempt.TurnID, CurrentStateRef: state.Ref, AuthorityRef: req.AuthorityRef, PolicyRef: req.PolicyRef, Purpose: req.Purpose, Scopes: req.Scopes, AllowedLicenses: req.AllowedLicenses, SensitivityMax: req.SensitivityMax, CheckedAt: req.CheckedUpperBound, NotAfter: req.NotAfter, ProjectionID: req.ProjectionID + "/v1-validation", ProjectionRevision: req.ProjectionRevision}
	v1, err := buildProjection(attempt, state, v1req, now, binding)
	if err != nil {
		return CurrentProjectionV2{}, err
	}
	if !contract.SameRef(req.ExpectedQueryRef, attempt.QueryRef) || !contract.SameRef(req.ExpectedViewRef, attempt.ViewRef) || !contract.SameRef(req.ExpectedSnapshotRef, attempt.SnapshotRef) || !contract.SameRef(req.ExpectedPointerRef, attempt.PointerRef) {
		return CurrentProjectionV2{}, contract.ErrEvidenceConflict
	}
	inspection, err := r.inspectPersistedLocked(storedCoordinate, attempt, now)
	if err != nil {
		return CurrentProjectionV2{}, err
	}
	associationDigest, err := associationDigestV2(attempt)
	if err != nil {
		return CurrentProjectionV2{}, err
	}
	items := make([]ProjectionItemV2, 0, min(len(v1.Items), req.MaxItems))
	var usedBytes int64
	usedTokens := 0
	for _, item := range v1.Items {
		if len(items) == req.MaxItems {
			break
		}
		if item.ContentRef.Length > req.PerItemMaxBytes || usedBytes+item.ContentRef.Length > req.MaxBytes {
			continue
		}
		tokens := int((item.ContentRef.Length + 3) / 4)
		if usedTokens+tokens > req.MaxTokens {
			continue
		}
		licenseDigest, digestErr := contract.Digest(struct {
			License string `json:"license"`
		}{item.License})
		if digestErr != nil {
			return CurrentProjectionV2{}, digestErr
		}
		conflictDigest, digestErr := contract.Digest(struct {
			ConflictGroup string `json:"conflict_group"`
		}{item.ConflictGroup})
		if digestErr != nil {
			return CurrentProjectionV2{}, digestErr
		}
		v2item := ProjectionItemV2{ContractVersion: ContractVersionV2, ObjectKind: ProjectionItemKindV2, Rank: len(items), Score: item.Score, RecordRef: item.RecordRef, PackageRef: item.PackageRef, SnapshotRef: item.SnapshotRef, ContentRef: item.ContentRef, SourceRefs: contract.NormalizeRefs(item.SourceRefs), EvidenceRefs: contract.NormalizeRefs(item.EvidenceRefs), ProjectionRefs: contract.NormalizeRefs(item.ProjectionRefs), CitationDigest: item.CitationDigest, License: item.License, LicenseDigest: licenseDigest, TrustState: item.TrustState, ConflictGroup: item.ConflictGroup, ConflictDigest: conflictDigest, DomainResultRef: item.DomainResultRef, AssociationDigest: associationDigest, ApplicationRef: item.ApplicationRef, TokenEstimate: tokens, EstimatorRef: req.EstimatorRef, ExpiresAt: item.ExpiresAt}
		v2item, err = sealProjectionItemV2(v2item)
		if err != nil {
			return CurrentProjectionV2{}, err
		}
		items = append(items, v2item)
		usedBytes += item.ContentRef.Length
		usedTokens += tokens
	}
	projection := CurrentProjectionV2{ContractVersion: ContractVersionV2, ObjectKind: CurrentProjectionKindV2, Ref: contract.Ref{ID: req.ProjectionID, Revision: req.ProjectionRevision}, Owner: contract.OwnerKnowledge, Current: true, Coordinate: storedCoordinate, AttemptInspectionRef: inspection.Ref, CurrentStateRef: state.Ref, StatePlaneBindingRef: binding.Ref, QueryRef: attempt.QueryRef, ViewRef: attempt.ViewRef, SnapshotRef: attempt.SnapshotRef, PointerRef: attempt.PointerRef, AuthorityRef: attempt.AuthorityRef, AuthorityEpoch: req.AuthorityEpoch, PolicyRef: attempt.PolicyRef, Purpose: attempt.Purpose, Scopes: append([]string{}, attempt.Scopes...), AllowedLicenses: append([]string{}, attempt.AllowedLicenses...), SensitivityMax: attempt.SensitivityMax, Coverage: cloneCoverage(attempt.Coverage), NextCursor: "", ResultDigest: attempt.ResultRef.Digest, EvidenceDigest: attempt.ObservationRef.Digest, Items: items, MaxItems: req.MaxItems, MaxBytes: req.MaxBytes, MaxTokens: req.MaxTokens, PerItemMaxBytes: req.PerItemMaxBytes, EstimatorRef: req.EstimatorRef, CheckPhase: req.CheckPhase, OwnerCheckedAt: now, ExpiresAt: minTime(req.NotAfter, minTime(attempt.ExpiresAt, minTime(state.ExpiresAt, minTime(storedCoordinate.SessionExpiresAt, storedCoordinate.TurnExpiresAt))))}
	projection, err = sealCurrentProjectionV2(projection)
	if err != nil {
		return CurrentProjectionV2{}, err
	}
	if req.CheckPhase == CheckPhaseS2V2 && req.ExpectedS1ClosureDigest != projection.StableClosureDigest {
		return CurrentProjectionV2{}, contract.ErrEvidenceConflict
	}
	return projection, nil
}

func (r *CurrentReaderV2) rebuildProjectionLocked(ctx context.Context, projection CurrentProjectionV2, now time.Time) (CurrentProjectionV2, error) {
	req := CurrentRequestV2{ContractVersion: ContractVersionV2, ObjectKind: CurrentRequestKindV2, Coordinate: projection.Coordinate, CurrentStateRef: projection.CurrentStateRef, ExpectedQueryRef: projection.QueryRef, ExpectedViewRef: projection.ViewRef, ExpectedSnapshotRef: projection.SnapshotRef, ExpectedPointerRef: projection.PointerRef, AuthorityRef: projection.AuthorityRef, AuthorityEpoch: projection.AuthorityEpoch, PolicyRef: projection.PolicyRef, Purpose: projection.Purpose, Scopes: projection.Scopes, AllowedLicenses: projection.AllowedLicenses, SensitivityMax: projection.SensitivityMax, CheckPhase: projection.CheckPhase, ExpectedS1ClosureDigest: projection.StableClosureDigest, MaxItems: projection.MaxItems, MaxBytes: projection.MaxBytes, MaxTokens: projection.MaxTokens, PerItemMaxBytes: projection.PerItemMaxBytes, EstimatorRef: projection.EstimatorRef, CheckedUpperBound: projection.OwnerCheckedAt, NotAfter: projection.ExpiresAt, ProjectionID: projection.Ref.ID, ProjectionRevision: projection.Ref.Revision}
	req, err := SealCurrentRequestV2(req)
	if err != nil {
		return CurrentProjectionV2{}, err
	}
	return r.buildProjectionLocked(ctx, req, now)
}

func (r *CurrentReaderV2) inspectPersistedLocked(coordinate AttemptCoordinateV2, attempt LocalAttempt, now time.Time) (AttemptInspectionV2, error) {
	if err := validateCoordinateCurrentV2(coordinate, now); err != nil || !attempt.ExpiresAt.After(now) {
		return AttemptInspectionV2{}, contract.ErrNotCurrent
	}
	status := AttemptPersistedAndSettled
	if err := validateSettlement(attempt); err != nil {
		status = AttemptPersistedUnsettled
	}
	observation, result := attempt.ObservationRef, attempt.ResultRef
	return sealAttemptInspectionV2(AttemptInspectionV2{ContractVersion: ContractVersionV2, ObjectKind: AttemptInspectionKindV2, Ref: contract.Ref{ID: "knowledge/context-inspection/v2/" + attempt.Ref.ID, Revision: attempt.Ref.Revision}, Owner: contract.OwnerKnowledge, Coordinate: coordinate, ObservationRef: &observation, ResultRef: &result, Status: status, OwnerCheckedAt: now, ExpiresAt: minTime(attempt.ExpiresAt, minTime(coordinate.SessionExpiresAt, minTime(coordinate.TurnExpiresAt, now.Add(time.Minute))))})
}

func SealAttemptCoordinateV2(in AttemptCoordinateV2) (AttemptCoordinateV2, error) {
	in.ContractVersion, in.ObjectKind = ContractVersionV2, AttemptCoordinateKindV2
	in.SessionCheckedAt, in.SessionExpiresAt = in.SessionCheckedAt.UTC(), in.SessionExpiresAt.UTC()
	in.TurnCheckedAt, in.TurnExpiresAt = in.TurnCheckedAt.UTC(), in.TurnExpiresAt.UTC()
	in.Digest = ""
	digest, err := contract.Digest(in)
	if err != nil {
		return AttemptCoordinateV2{}, err
	}
	in.Digest = digest
	if err := in.Validate(); err != nil {
		return AttemptCoordinateV2{}, err
	}
	return in, nil
}

func (in AttemptCoordinateV2) Validate() error {
	if in.ContractVersion != ContractVersionV2 || in.ObjectKind != AttemptCoordinateKindV2 || strings.TrimSpace(in.TenantID) == "" || in.IdentityEpoch == 0 || strings.TrimSpace(in.ExecutionScopeDigest) == "" || strings.TrimSpace(in.RunID) == "" || in.SourceTurnOrdinal == 0 || strings.TrimSpace(in.LegacyTurnID) == "" || in.LegacyTurnID != in.SourceTurnRef.ID || strings.TrimSpace(in.RequestDigest) == "" || strings.TrimSpace(in.IdempotencyKey) == "" {
		return contract.ErrInvalidArgument
	}
	for _, ref := range []contract.Ref{in.IdentityRef, in.SessionRef, in.SessionEvidenceRef, in.SourceTurnRef, in.TurnEvidenceRef, in.AttemptRef, in.ObservationRef, in.ResultRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if in.SessionCheckedAt.IsZero() || !in.SessionExpiresAt.After(in.SessionCheckedAt) || in.TurnCheckedAt.IsZero() || !in.TurnExpiresAt.After(in.TurnCheckedAt) {
		return contract.ErrInvalidArgument
	}
	copy := in
	copy.Digest = ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if digest != in.Digest {
		return contract.ErrEvidenceConflict
	}
	return nil
}

func SealCurrentRequestV2(in CurrentRequestV2) (CurrentRequestV2, error) {
	in.ContractVersion, in.ObjectKind = ContractVersionV2, CurrentRequestKindV2
	var err error
	in.Scopes, err = contextsourcev2.NormalizeStrings(in.Scopes, false)
	if err != nil {
		return CurrentRequestV2{}, err
	}
	in.AllowedLicenses, err = contextsourcev2.NormalizeStrings(in.AllowedLicenses, false)
	if err != nil {
		return CurrentRequestV2{}, err
	}
	in.CheckedUpperBound, in.NotAfter = in.CheckedUpperBound.UTC(), in.NotAfter.UTC()
	in.Digest = ""
	digest, err := contract.Digest(in)
	if err != nil {
		return CurrentRequestV2{}, err
	}
	in.Digest = digest
	if err := in.Validate(); err != nil {
		return CurrentRequestV2{}, err
	}
	return in, nil
}

func (in CurrentRequestV2) Validate() error {
	if in.ContractVersion != ContractVersionV2 || in.ObjectKind != CurrentRequestKindV2 || in.MaxItems <= 0 || in.MaxItems > maxV2Items || in.MaxBytes <= 0 || in.MaxBytes > maxV2BodyBytes || in.MaxTokens <= 0 || in.PerItemMaxBytes <= 0 || in.PerItemMaxBytes > in.MaxBytes || strings.TrimSpace(in.Purpose) == "" || strings.TrimSpace(in.SensitivityMax) == "" || strings.TrimSpace(in.ProjectionID) == "" || in.ProjectionRevision == 0 || (in.CheckPhase != CheckPhaseS1V2 && in.CheckPhase != CheckPhaseS2V2) || (in.CheckPhase == CheckPhaseS2V2 && strings.TrimSpace(in.ExpectedS1ClosureDigest) == "") {
		return contract.ErrInvalidArgument
	}
	if err := in.Coordinate.Validate(); err != nil {
		return err
	}
	for _, ref := range []contract.Ref{in.CurrentStateRef, in.ExpectedQueryRef, in.ExpectedViewRef, in.ExpectedSnapshotRef, in.ExpectedPointerRef, in.AuthorityRef, in.PolicyRef, in.EstimatorRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if in.AuthorityEpoch == 0 || in.CheckedUpperBound.IsZero() || !in.NotAfter.After(in.CheckedUpperBound) {
		return contract.ErrInvalidArgument
	}
	scopes, err := contextsourcev2.NormalizeStrings(in.Scopes, false)
	if err != nil || !slices.Equal(scopes, in.Scopes) {
		return contract.ErrInvalidArgument
	}
	licenses, err := contextsourcev2.NormalizeStrings(in.AllowedLicenses, false)
	if err != nil || !slices.Equal(licenses, in.AllowedLicenses) {
		return contract.ErrInvalidArgument
	}
	copy := in
	copy.Digest = ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if digest != in.Digest {
		return contract.ErrEvidenceConflict
	}
	return nil
}

func SealExactContentRequestV2(in ExactContentRequestV2) (ExactContentRequestV2, error) {
	in.ContractVersion, in.ObjectKind = ContractVersionV2, ExactContentRequestKindV2
	in.CheckedUpperBound, in.NotAfter = in.CheckedUpperBound.UTC(), in.NotAfter.UTC()
	in.Digest = ""
	digest, err := contract.Digest(in)
	if err != nil {
		return ExactContentRequestV2{}, err
	}
	in.Digest = digest
	if err := in.Validate(); err != nil {
		return ExactContentRequestV2{}, err
	}
	return in, nil
}

func (in ExactContentRequestV2) Validate() error {
	if in.ContractVersion != ContractVersionV2 || in.ObjectKind != ExactContentRequestKindV2 || in.Rank < 0 || in.MaxBodyBytes <= 0 || in.MaxBodyBytes > maxV2BodyBytes || (in.CheckPhase != CheckPhaseS1V2 && in.CheckPhase != CheckPhaseS2V2) || strings.TrimSpace(in.ExpectedStableClosureDigest) == "" || in.CheckedUpperBound.IsZero() || !in.NotAfter.After(in.CheckedUpperBound) {
		return contract.ErrInvalidArgument
	}
	if err := in.Coordinate.Validate(); err != nil {
		return err
	}
	if err := in.Projection.Validate(); err != nil {
		return err
	}
	if !sameCoordinateV2(in.Coordinate, in.Projection.Coordinate) || in.ExpectedStableClosureDigest != in.Projection.StableClosureDigest || in.CheckPhase != in.Projection.CheckPhase {
		return contract.ErrEvidenceConflict
	}
	copy := in
	copy.Digest = ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if digest != in.Digest {
		return contract.ErrEvidenceConflict
	}
	return nil
}

func (in AttemptInspectionV2) Validate() error {
	if in.ContractVersion != ContractVersionV2 || in.ObjectKind != AttemptInspectionKindV2 || in.Owner != contract.OwnerKnowledge || in.OwnerCheckedAt.IsZero() || !in.ExpiresAt.After(in.OwnerCheckedAt) {
		return contract.ErrInvalidArgument
	}
	if err := in.Ref.Validate(); err != nil {
		return err
	}
	if err := in.Coordinate.Validate(); err != nil {
		return err
	}
	if in.Status == AttemptNotPersisted {
		if in.ObservationRef != nil || in.ResultRef != nil {
			return contract.ErrEvidenceConflict
		}
	} else if in.Status == AttemptPersistedAndSettled || in.Status == AttemptPersistedUnsettled {
		if in.ObservationRef == nil || in.ResultRef == nil || in.ObservationRef.Validate() != nil || in.ResultRef.Validate() != nil {
			return contract.ErrEvidenceConflict
		}
	} else {
		return contract.ErrInvalidArgument
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return contract.ErrEvidenceConflict
	}
	return nil
}

func (in ProjectionItemV2) Validate() error {
	if in.ContractVersion != ContractVersionV2 || in.ObjectKind != ProjectionItemKindV2 || in.Rank < 0 || in.TokenEstimate < 0 || strings.TrimSpace(in.CitationDigest) == "" || strings.TrimSpace(in.License) == "" || strings.TrimSpace(in.LicenseDigest) == "" || strings.TrimSpace(in.TrustState) == "" || strings.TrimSpace(in.ConflictDigest) == "" || strings.TrimSpace(in.AssociationDigest) == "" || in.ExpiresAt.IsZero() {
		return contract.ErrInvalidArgument
	}
	if err := in.RecordRef.Validate(); err != nil {
		return err
	}
	if err := in.PackageRef.Validate(); err != nil {
		return err
	}
	if err := in.SnapshotRef.Validate(); err != nil {
		return err
	}
	if err := in.ContentRef.Validate(); err != nil {
		return err
	}
	for _, ref := range []contract.Ref{in.DomainResultRef, in.ApplicationRef, in.EstimatorRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	for _, refs := range [][]contract.Ref{in.SourceRefs, in.EvidenceRefs, in.ProjectionRefs} {
		normalized, err := contextsourcev2.NormalizeRefs(refs)
		if err != nil || !slices.Equal(normalized, refs) {
			return contract.ErrInvalidArgument
		}
	}
	copy := in
	copy.Digest = ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if digest != in.Digest {
		return contract.ErrEvidenceConflict
	}
	return nil
}

func (in CurrentProjectionV2) Validate() error {
	if in.ContractVersion != ContractVersionV2 || in.ObjectKind != CurrentProjectionKindV2 || in.Owner != contract.OwnerKnowledge || !in.Current || in.OwnerCheckedAt.IsZero() || !in.ExpiresAt.After(in.OwnerCheckedAt) || (in.CheckPhase != CheckPhaseS1V2 && in.CheckPhase != CheckPhaseS2V2) {
		return contract.ErrInvalidArgument
	}
	if err := in.Ref.Validate(); err != nil {
		return err
	}
	if err := in.Coordinate.Validate(); err != nil {
		return err
	}
	for _, ref := range []contract.Ref{in.AttemptInspectionRef, in.CurrentStateRef, in.StatePlaneBindingRef, in.QueryRef, in.ViewRef, in.SnapshotRef, in.PointerRef, in.AuthorityRef, in.PolicyRef, in.EstimatorRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	for i, item := range in.Items {
		if err := item.Validate(); err != nil || item.Rank != i {
			return contract.ErrEvidenceConflict
		}
	}
	licenses, err := contextsourcev2.NormalizeStrings(in.AllowedLicenses, false)
	if err != nil || !slices.Equal(licenses, in.AllowedLicenses) {
		return contract.ErrInvalidArgument
	}
	copy := cloneProjectionV2(in)
	if err := populateProjectionDigestsV2(&copy); err != nil {
		return err
	}
	if copy.StableClosureDigest != in.StableClosureDigest || copy.OrderedItemSetDigest != in.OrderedItemSetDigest || copy.ContentSetDigest != in.ContentSetDigest || copy.SourceSetDigest != in.SourceSetDigest || copy.EvidenceSetDigest != in.EvidenceSetDigest || copy.ProjectionSetDigest != in.ProjectionSetDigest || copy.CitationSetDigest != in.CitationSetDigest || copy.LicenseSetDigest != in.LicenseSetDigest || copy.ConflictSetDigest != in.ConflictSetDigest {
		return contract.ErrEvidenceConflict
	}
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return contract.ErrEvidenceConflict
	}
	return nil
}

func (in ExactContentObservationV2) Validate() error {
	if in.ContractVersion != ContractVersionV2 || in.ObjectKind != ExactContentObservationKindV2 || in.Owner != contract.OwnerKnowledge || in.Rank < 0 || strings.TrimSpace(in.License) == "" || strings.TrimSpace(in.LicenseDigest) == "" || in.ObservedLength < 0 || in.OwnerObservedAt.IsZero() || !in.ExpiresAt.After(in.OwnerObservedAt) || in.ObservedDigest != in.ContentRef.Digest || in.ObservedMediaType != in.ContentRef.MediaType || (in.CheckPhase != CheckPhaseS1V2 && in.CheckPhase != CheckPhaseS2V2) {
		return contract.ErrInvalidArgument
	}
	if err := in.Coordinate.Validate(); err != nil {
		return err
	}
	for _, ref := range []contract.Ref{in.Ref, in.ProjectionRef, in.StatePlaneBindingRef, in.RecordRef, in.PackageRef, in.SnapshotRef, in.DomainResultRef, in.ApplicationRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if err := in.ContentRef.Validate(); err != nil {
		return err
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return contract.ErrEvidenceConflict
	}
	return nil
}

func sealAttemptInspectionV2(in AttemptInspectionV2) (AttemptInspectionV2, error) {
	in.Ref.Digest, in.Digest = "", ""
	d, err := contract.Digest(in)
	if err != nil {
		return AttemptInspectionV2{}, err
	}
	in.Ref.Digest, in.Digest = d, d
	return in, in.Validate()
}
func sealProjectionItemV2(in ProjectionItemV2) (ProjectionItemV2, error) {
	var err error
	in.SourceRefs, err = contextsourcev2.NormalizeRefs(in.SourceRefs)
	if err != nil {
		return ProjectionItemV2{}, err
	}
	in.EvidenceRefs, err = contextsourcev2.NormalizeRefs(in.EvidenceRefs)
	if err != nil {
		return ProjectionItemV2{}, err
	}
	in.ProjectionRefs, err = contextsourcev2.NormalizeRefs(in.ProjectionRefs)
	if err != nil {
		return ProjectionItemV2{}, err
	}
	in.Digest = ""
	d, err := contract.Digest(in)
	if err != nil {
		return ProjectionItemV2{}, err
	}
	in.Digest = d
	return in, in.Validate()
}
func sealCurrentProjectionV2(in CurrentProjectionV2) (CurrentProjectionV2, error) {
	var err error
	in.Scopes, err = contextsourcev2.NormalizeStrings(in.Scopes, false)
	if err != nil {
		return CurrentProjectionV2{}, err
	}
	in.AllowedLicenses, err = contextsourcev2.NormalizeStrings(in.AllowedLicenses, false)
	if err != nil {
		return CurrentProjectionV2{}, err
	}
	if err := populateProjectionDigestsV2(&in); err != nil {
		return CurrentProjectionV2{}, err
	}
	in.Ref.Digest, in.Digest = "", ""
	d, err := contract.Digest(in)
	if err != nil {
		return CurrentProjectionV2{}, err
	}
	in.Ref.Digest, in.Digest = d, d
	return in, in.Validate()
}
func sealExactContentObservationV2(in ExactContentObservationV2) (ExactContentObservationV2, error) {
	in.Ref.Digest, in.Digest = "", ""
	d, err := contract.Digest(in)
	if err != nil {
		return ExactContentObservationV2{}, err
	}
	in.Ref.Digest, in.Digest = d, d
	return in, in.Validate()
}

type stableItemV2 struct {
	ContractVersion   string              `json:"contract_version"`
	ObjectKind        string              `json:"object_kind"`
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
	LicenseDigest     string              `json:"license_digest"`
	TrustState        string              `json:"trust_state"`
	ConflictGroup     string              `json:"conflict_group"`
	ConflictDigest    string              `json:"conflict_digest"`
	DomainResultRef   contract.Ref        `json:"domain_result_ref"`
	AssociationDigest string              `json:"association_digest"`
	ApplicationRef    contract.Ref        `json:"application_ref"`
	TokenEstimate     int                 `json:"token_estimate"`
	EstimatorRef      contract.Ref        `json:"estimator_ref"`
}
type stableClosureV2 struct {
	DigestDomain         string               `json:"digest_domain"`
	ContractVersion      string               `json:"contract_version"`
	ObjectKind           string               `json:"object_kind"`
	Owner                contract.OwnerDomain `json:"owner"`
	TenantID             string               `json:"tenant_id"`
	IdentityRef          contract.Ref         `json:"identity_ref"`
	IdentityEpoch        uint64               `json:"identity_epoch"`
	ExecutionScopeDigest string               `json:"execution_scope_digest"`
	RunID                string               `json:"run_id"`
	SessionRef           contract.Ref         `json:"session_ref"`
	SessionEvidenceRef   contract.Ref         `json:"session_evidence_ref"`
	SourceTurnOrdinal    uint32               `json:"source_turn_ordinal"`
	SourceTurnRef        contract.Ref         `json:"source_turn_ref"`
	TurnEvidenceRef      contract.Ref         `json:"turn_evidence_ref"`
	LegacyTurnID         string               `json:"legacy_turn_id"`
	AttemptRef           contract.Ref         `json:"attempt_ref"`
	RequestDigest        string               `json:"request_digest"`
	IdempotencyKey       string               `json:"idempotency_key"`
	ObservationRef       contract.Ref         `json:"observation_ref"`
	ResultRef            contract.Ref         `json:"result_ref"`
	CurrentStateRef      contract.Ref         `json:"current_state_ref"`
	StatePlaneBindingRef contract.Ref         `json:"state_plane_binding_ref"`
	QueryRef             contract.Ref         `json:"query_ref"`
	ViewRef              contract.Ref         `json:"view_ref"`
	SnapshotRef          contract.Ref         `json:"snapshot_ref"`
	PointerRef           contract.Ref         `json:"pointer_ref"`
	AuthorityRef         contract.Ref         `json:"authority_ref"`
	AuthorityEpoch       uint64               `json:"authority_epoch"`
	PolicyRef            contract.Ref         `json:"policy_ref"`
	Purpose              string               `json:"purpose"`
	Scopes               []string             `json:"scopes"`
	AllowedLicenses      []string             `json:"allowed_licenses"`
	SensitivityMax       string               `json:"sensitivity_max"`
	Coverage             contract.Coverage    `json:"coverage"`
	NextCursor           string               `json:"next_cursor"`
	ResultDigest         string               `json:"result_digest"`
	EvidenceDigest       string               `json:"evidence_digest"`
	Items                []stableItemV2       `json:"items"`
	OrderedItemSetDigest string               `json:"ordered_item_set_digest"`
	ContentSetDigest     string               `json:"content_set_digest"`
	SourceSetDigest      string               `json:"source_set_digest"`
	EvidenceSetDigest    string               `json:"evidence_set_digest"`
	ProjectionSetDigest  string               `json:"projection_set_digest"`
	CitationSetDigest    string               `json:"citation_set_digest"`
	LicenseSetDigest     string               `json:"license_set_digest"`
	ConflictSetDigest    string               `json:"conflict_set_digest"`
	MaxItems             int                  `json:"max_items"`
	MaxBytes             int64                `json:"max_bytes"`
	MaxTokens            int                  `json:"max_tokens"`
	PerItemMaxBytes      int64                `json:"per_item_max_bytes"`
	EstimatorRef         contract.Ref         `json:"estimator_ref"`
}
type itemSetBodyV2 struct {
	DigestDomain    string         `json:"digest_domain"`
	ContractVersion string         `json:"contract_version"`
	ObjectKind      string         `json:"object_kind"`
	Items           []stableItemV2 `json:"items"`
}
type contentSetBodyV2 struct {
	DigestDomain    string                `json:"digest_domain"`
	ContractVersion string                `json:"contract_version"`
	ObjectKind      string                `json:"object_kind"`
	ContentRefs     []contract.ContentRef `json:"content_refs"`
}
type refSetBodyV2 struct {
	DigestDomain    string         `json:"digest_domain"`
	ContractVersion string         `json:"contract_version"`
	ObjectKind      string         `json:"object_kind"`
	Refs            []contract.Ref `json:"refs"`
}
type stringSetBodyV2 struct {
	DigestDomain    string   `json:"digest_domain"`
	ContractVersion string   `json:"contract_version"`
	ObjectKind      string   `json:"object_kind"`
	Values          []string `json:"values"`
}

func populateProjectionDigestsV2(p *CurrentProjectionV2) error {
	stable := make([]stableItemV2, len(p.Items))
	contents := make([]contract.ContentRef, 0, len(p.Items))
	var sources, evidence, projections []contract.Ref
	citations := make([]string, 0, len(p.Items))
	licenses := make([]string, 0, len(p.Items))
	conflicts := make([]string, 0, len(p.Items))
	records := make(map[string]struct{}, len(p.Items))
	for i, item := range p.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if item.Rank != i {
			return contract.ErrEvidenceConflict
		}
		if _, exists := records[item.RecordRef.ID]; exists {
			return contract.ErrEvidenceConflict
		}
		records[item.RecordRef.ID] = struct{}{}
		stable[i] = stableItemV2{ContractVersion: item.ContractVersion, ObjectKind: item.ObjectKind, Rank: item.Rank, Score: item.Score, RecordRef: item.RecordRef, PackageRef: item.PackageRef, SnapshotRef: item.SnapshotRef, ContentRef: item.ContentRef, SourceRefs: contract.NormalizeRefs(item.SourceRefs), EvidenceRefs: contract.NormalizeRefs(item.EvidenceRefs), ProjectionRefs: contract.NormalizeRefs(item.ProjectionRefs), CitationDigest: item.CitationDigest, License: item.License, LicenseDigest: item.LicenseDigest, TrustState: item.TrustState, ConflictGroup: item.ConflictGroup, ConflictDigest: item.ConflictDigest, DomainResultRef: item.DomainResultRef, AssociationDigest: item.AssociationDigest, ApplicationRef: item.ApplicationRef, TokenEstimate: item.TokenEstimate, EstimatorRef: item.EstimatorRef}
		contents = append(contents, item.ContentRef)
		sources = append(sources, item.SourceRefs...)
		evidence = append(evidence, item.EvidenceRefs...)
		projections = append(projections, item.ProjectionRefs...)
		citations = append(citations, item.CitationDigest)
		licenses = append(licenses, item.LicenseDigest)
		conflicts = append(conflicts, item.ConflictDigest)
	}
	if !slices.IsSortedFunc(stable, func(a, b stableItemV2) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		if c := strings.Compare(a.RecordRef.ID, b.RecordRef.ID); c != 0 {
			return c
		}
		if a.RecordRef.Revision > b.RecordRef.Revision {
			return -1
		}
		if a.RecordRef.Revision < b.RecordRef.Revision {
			return 1
		}
		return strings.Compare(a.RecordRef.Digest, b.RecordRef.Digest)
	}) {
		return contract.ErrEvidenceConflict
	}
	slices.SortFunc(contents, func(a, b contract.ContentRef) int {
		if c := strings.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		if c := strings.Compare(a.Digest, b.Digest); c != 0 {
			return c
		}
		if a.Length < b.Length {
			return -1
		}
		if a.Length > b.Length {
			return 1
		}
		return strings.Compare(a.MediaType, b.MediaType)
	})
	contents = slices.Compact(contents)
	var err error
	sources, err = contextsourcev2.NormalizeRefs(sources)
	if err != nil {
		return err
	}
	evidence, err = contextsourcev2.NormalizeRefs(evidence)
	if err != nil {
		return err
	}
	projections, err = contextsourcev2.NormalizeRefs(projections)
	if err != nil {
		return err
	}
	citations, err = contextsourcev2.NormalizeStrings(citations, false)
	if err != nil {
		return err
	}
	licenses, err = contextsourcev2.NormalizeStrings(licenses, false)
	if err != nil {
		return err
	}
	conflicts, err = contextsourcev2.NormalizeStrings(conflicts, false)
	if err != nil {
		return err
	}
	p.OrderedItemSetDigest, err = contract.Digest(itemSetBodyV2{KnowledgeSetDigestDomainV2, KnowledgeSetDigestContractVersionV2, KnowledgeOrderedItemSetObjectKindV2, stable})
	if err != nil {
		return err
	}
	p.ContentSetDigest, err = contract.Digest(contentSetBodyV2{KnowledgeSetDigestDomainV2, KnowledgeSetDigestContractVersionV2, KnowledgeContentSetObjectKindV2, contents})
	if err != nil {
		return err
	}
	p.SourceSetDigest, err = contract.Digest(refSetBodyV2{KnowledgeSetDigestDomainV2, KnowledgeSetDigestContractVersionV2, KnowledgeSourceSetObjectKindV2, sources})
	if err != nil {
		return err
	}
	p.EvidenceSetDigest, err = contract.Digest(refSetBodyV2{KnowledgeSetDigestDomainV2, KnowledgeSetDigestContractVersionV2, KnowledgeEvidenceSetObjectKindV2, evidence})
	if err != nil {
		return err
	}
	p.ProjectionSetDigest, err = contract.Digest(refSetBodyV2{KnowledgeSetDigestDomainV2, KnowledgeSetDigestContractVersionV2, KnowledgeProjectionSetObjectKindV2, projections})
	if err != nil {
		return err
	}
	p.CitationSetDigest, err = contract.Digest(stringSetBodyV2{KnowledgeSetDigestDomainV2, KnowledgeSetDigestContractVersionV2, KnowledgeCitationSetObjectKindV2, citations})
	if err != nil {
		return err
	}
	p.LicenseSetDigest, err = contract.Digest(stringSetBodyV2{KnowledgeSetDigestDomainV2, KnowledgeSetDigestContractVersionV2, KnowledgeLicenseSetObjectKindV2, licenses})
	if err != nil {
		return err
	}
	p.ConflictSetDigest, err = contract.Digest(stringSetBodyV2{KnowledgeSetDigestDomainV2, KnowledgeSetDigestContractVersionV2, KnowledgeConflictSetObjectKindV2, conflicts})
	if err != nil {
		return err
	}
	c := p.Coordinate
	body := stableClosureV2{KnowledgeStableClosureDigestDomainV2, KnowledgeStableClosureContractVersionV2, KnowledgeStableClosureObjectKindV2, p.Owner, c.TenantID, c.IdentityRef, c.IdentityEpoch, c.ExecutionScopeDigest, c.RunID, c.SessionRef, c.SessionEvidenceRef, c.SourceTurnOrdinal, c.SourceTurnRef, c.TurnEvidenceRef, c.LegacyTurnID, c.AttemptRef, c.RequestDigest, c.IdempotencyKey, c.ObservationRef, c.ResultRef, p.CurrentStateRef, p.StatePlaneBindingRef, p.QueryRef, p.ViewRef, p.SnapshotRef, p.PointerRef, p.AuthorityRef, p.AuthorityEpoch, p.PolicyRef, p.Purpose, p.Scopes, p.AllowedLicenses, p.SensitivityMax, normalizeCoverageV2(p.Coverage), p.NextCursor, p.ResultDigest, p.EvidenceDigest, stable, p.OrderedItemSetDigest, p.ContentSetDigest, p.SourceSetDigest, p.EvidenceSetDigest, p.ProjectionSetDigest, p.CitationSetDigest, p.LicenseSetDigest, p.ConflictSetDigest, p.MaxItems, p.MaxBytes, p.MaxTokens, p.PerItemMaxBytes, p.EstimatorRef}
	p.StableClosureDigest, err = contract.Digest(body)
	return err
}

func associationDigestV2(attempt LocalAttempt) (string, error) {
	if err := validateSettlement(attempt); err != nil {
		return "", contract.ErrSettlementMismatch
	}
	body := struct {
		ContractVersion string               `json:"contract_version"`
		ObjectKind      string               `json:"object_kind"`
		Owner           contract.OwnerDomain `json:"owner"`
		DomainResultRef contract.Ref         `json:"domain_result_ref"`
	}{associationContractVersionV2, associationObjectKindV2, contract.OwnerKnowledge, attempt.DomainResult.Ref}
	return contract.Digest(body)
}
func coordinateMatchesAttemptV2(c AttemptCoordinateV2, a LocalAttempt) error {
	if c.TenantID != a.TenantID || c.ExecutionScopeDigest != a.ExecutionScopeDigest || c.RunID != a.RunID || c.LegacyTurnID != a.TurnID || !contract.SameRef(c.AttemptRef, a.Ref) || c.RequestDigest != a.RequestDigest || c.IdempotencyKey != a.IdempotencyKey || !contract.SameRef(c.ObservationRef, a.ObservationRef) || !contract.SameRef(c.ResultRef, a.ResultRef) {
		return contract.ErrEvidenceConflict
	}
	return nil
}
func validateCoordinateCurrentV2(c AttemptCoordinateV2, now time.Time) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if !c.SessionExpiresAt.After(now) || !c.TurnExpiresAt.After(now) {
		return contract.ErrNotCurrent
	}
	return nil
}
func validateRequestTimeV2(upper, notAfter, now time.Time) error {
	if upper.IsZero() || !notAfter.After(upper) {
		return contract.ErrInvalidArgument
	}
	if !notAfter.After(now) {
		return contract.ErrNotCurrent
	}
	return nil
}
func findCoordinateV2(values []AttemptCoordinateV2, ref contract.Ref) (AttemptCoordinateV2, bool) {
	for _, v := range values {
		if contract.SameRef(v.AttemptRef, ref) {
			return v, true
		}
	}
	return AttemptCoordinateV2{}, false
}
func sameCoordinateV2(a, b AttemptCoordinateV2) bool {
	return a.Validate() == nil && b.Validate() == nil && a.Digest == b.Digest
}
func projectionItemByRankV2(items []ProjectionItemV2, rank int) (ProjectionItemV2, bool) {
	for _, item := range items {
		if item.Rank == rank {
			return item, true
		}
	}
	return ProjectionItemV2{}, false
}
func sameProjectionStableV2(a, b CurrentProjectionV2) bool {
	return a.StableClosureDigest == b.StableClosureDigest && a.OrderedItemSetDigest == b.OrderedItemSetDigest && a.ContentSetDigest == b.ContentSetDigest && a.SourceSetDigest == b.SourceSetDigest && a.EvidenceSetDigest == b.EvidenceSetDigest && a.ProjectionSetDigest == b.ProjectionSetDigest && a.CitationSetDigest == b.CitationSetDigest && a.LicenseSetDigest == b.LicenseSetDigest && a.ConflictSetDigest == b.ConflictSetDigest
}
func checkContextV2(ctx context.Context) error {
	if ctx == nil {
		return contract.ErrInvalidArgument
	}
	return ctx.Err()
}
func cloneProjectionV2(v CurrentProjectionV2) CurrentProjectionV2 {
	v.Scopes = append([]string{}, v.Scopes...)
	v.AllowedLicenses = append([]string{}, v.AllowedLicenses...)
	v.Coverage = cloneCoverage(v.Coverage)
	v.Items = append([]ProjectionItemV2{}, v.Items...)
	for i := range v.Items {
		v.Items[i].SourceRefs = append([]contract.Ref{}, v.Items[i].SourceRefs...)
		v.Items[i].EvidenceRefs = append([]contract.Ref{}, v.Items[i].EvidenceRefs...)
		v.Items[i].ProjectionRefs = append([]contract.Ref{}, v.Items[i].ProjectionRefs...)
	}
	return v
}

func normalizeCoverageV2(v contract.Coverage) contract.Coverage {
	v.ProjectionRefs = contract.NormalizeRefs(v.ProjectionRefs)
	v.DroppedReasons, _ = contextsourcev2.NormalizeStrings(v.DroppedReasons, false)
	if v.ProjectionRefs == nil {
		v.ProjectionRefs = []contract.Ref{}
	}
	if v.DroppedReasons == nil {
		v.DroppedReasons = []string{}
	}
	return v
}
