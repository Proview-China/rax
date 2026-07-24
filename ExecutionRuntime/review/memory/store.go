// Package memory provides a concurrency-safe in-memory reference Store.
// It is intended for tests and conformance, not production persistence.
package memory

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type Store struct {
	mu                        sync.RWMutex
	clock                     func() time.Time
	requests                  map[string]contract.ReviewRequestV1
	requestHistory            map[string]map[core.Revision]contract.ReviewRequestV1
	requestKeys               map[string]string
	requestByCase             map[string]string
	resultBundles             map[string]contract.ReviewResultBundleV1
	resultBundlesV2           map[string]contract.ReviewResultBundleV2
	targets                   map[string]contract.TargetSnapshotV1
	targetHistory             map[string]map[core.Revision]contract.TargetSnapshotV1
	cases                     map[string]contract.ReviewCaseV1
	caseHistory               map[string]map[core.Revision]contract.ReviewCaseV1
	caseByTarget              map[string]string
	currentCaseByTargetID     map[string]string
	rounds                    map[string]contract.ReviewRoundV1
	assignments               map[string]contract.ReviewerAssignmentV1
	assignmentHistory         map[string]map[core.Revision]contract.ReviewerAssignmentV1
	findings                  map[string]contract.FindingV1
	attestations              map[string]contract.AttestationV1
	attestationKeys           map[string]string
	verdicts                  map[string]contract.VerdictV1
	verdictHistory            map[string]map[core.Revision]contract.VerdictV1
	traces                    map[string]contract.TraceFactV1
	traceByCase               map[string][]string
	domainResults             map[string]contract.ReviewerInvocationResultFactV1
	applySettlements          map[string]contract.DomainApplySettlementFactV1
	behaviorFeedback          map[string]contract.BehaviorFeedbackCandidateV1
	evidenceAttachments       map[string]contract.EvidenceAttachmentV1
	evidenceAttachmentKeys    map[string]string
	humanPanels               map[string]contract.HumanReviewPanelV2
	humanPanelHistory         map[string]map[core.Revision]contract.HumanReviewPanelV2
	humanAssignments          map[string]contract.HumanPanelAssignmentV2
	humanAssignmentHistory    map[string]map[core.Revision]contract.HumanPanelAssignmentV2
	humanAttestations         map[string]contract.HumanAttestationV2
	humanAttestationKeys      map[string]string
	humanVoteByReviewer       map[string]string
	humanQuorums              map[string]contract.HumanQuorumDecisionV2
	humanQuorumByPanel        map[string]string
	humanVerdicts             map[string]contract.HumanVerdictV2
	humanVerdictHistory       map[string]map[core.Revision]contract.HumanVerdictV2
	humanVerdictByPanel       map[string]string
	humanClaimTraceByRevision map[string]map[core.Revision]contract.FactIdentityV1
	bypassDecisions           map[string]contract.BypassDecisionV1
	bypassDecisionHistory     map[string]map[core.Revision]contract.BypassDecisionV1
	bypassDecisionByCase      map[string]contract.BypassDecisionExactRefV1
	bypassHighestRevision     map[string]core.Revision
	bypassTraceByRevision     map[string]map[core.Revision]contract.FactIdentityV1
	rubricCurrent             map[string]contract.ExactResourceRefV1
	rubricHistory             map[string]map[core.Revision]contract.RubricDefinitionV1
	rubricHighestRevision     map[string]core.Revision
	autoReviewerAttempts      map[string]contract.AutoReviewerAttemptV1
	autoReviewerHistory       map[string]map[core.Revision]contract.AutoReviewerAttemptV1
	autoReviewerKeys          map[string]string
	autoReviewerObservations  map[string]contract.AutoReviewerInvocationObservationV1
}

func NewStore() *Store {
	store, err := NewStoreWithClockV1(time.Now)
	if err != nil {
		panic(err)
	}
	return store
}

// NewStoreWithClockV1 constructs the reference Store with the Review Owner's
// actual-point clock. The Store calls it under its lock; mutation timestamps
// supplied by callers are never accepted as current truth.
func NewStoreWithClockV1(clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "review memory Store requires an actual-point clock")
	}
	return &Store{
		clock:    clock,
		requests: make(map[string]contract.ReviewRequestV1), requestHistory: make(map[string]map[core.Revision]contract.ReviewRequestV1), requestKeys: make(map[string]string), requestByCase: make(map[string]string), resultBundles: make(map[string]contract.ReviewResultBundleV1), resultBundlesV2: make(map[string]contract.ReviewResultBundleV2),
		targets: make(map[string]contract.TargetSnapshotV1), targetHistory: make(map[string]map[core.Revision]contract.TargetSnapshotV1), cases: make(map[string]contract.ReviewCaseV1), caseHistory: make(map[string]map[core.Revision]contract.ReviewCaseV1), caseByTarget: make(map[string]string), currentCaseByTargetID: make(map[string]string), rounds: make(map[string]contract.ReviewRoundV1), assignments: make(map[string]contract.ReviewerAssignmentV1), assignmentHistory: make(map[string]map[core.Revision]contract.ReviewerAssignmentV1), findings: make(map[string]contract.FindingV1), attestations: make(map[string]contract.AttestationV1), attestationKeys: make(map[string]string), verdicts: make(map[string]contract.VerdictV1), verdictHistory: make(map[string]map[core.Revision]contract.VerdictV1), traces: make(map[string]contract.TraceFactV1), traceByCase: make(map[string][]string), domainResults: make(map[string]contract.ReviewerInvocationResultFactV1), applySettlements: make(map[string]contract.DomainApplySettlementFactV1), behaviorFeedback: make(map[string]contract.BehaviorFeedbackCandidateV1), evidenceAttachments: make(map[string]contract.EvidenceAttachmentV1), evidenceAttachmentKeys: make(map[string]string),
		humanPanels: make(map[string]contract.HumanReviewPanelV2), humanPanelHistory: make(map[string]map[core.Revision]contract.HumanReviewPanelV2), humanAssignments: make(map[string]contract.HumanPanelAssignmentV2), humanAssignmentHistory: make(map[string]map[core.Revision]contract.HumanPanelAssignmentV2), humanAttestations: make(map[string]contract.HumanAttestationV2), humanAttestationKeys: make(map[string]string), humanVoteByReviewer: make(map[string]string), humanQuorums: make(map[string]contract.HumanQuorumDecisionV2), humanQuorumByPanel: make(map[string]string), humanVerdicts: make(map[string]contract.HumanVerdictV2), humanVerdictHistory: make(map[string]map[core.Revision]contract.HumanVerdictV2), humanVerdictByPanel: make(map[string]string), humanClaimTraceByRevision: make(map[string]map[core.Revision]contract.FactIdentityV1),
		bypassDecisions: make(map[string]contract.BypassDecisionV1), bypassDecisionHistory: make(map[string]map[core.Revision]contract.BypassDecisionV1), bypassDecisionByCase: make(map[string]contract.BypassDecisionExactRefV1), bypassHighestRevision: make(map[string]core.Revision), bypassTraceByRevision: make(map[string]map[core.Revision]contract.FactIdentityV1),
		rubricCurrent: make(map[string]contract.ExactResourceRefV1), rubricHistory: make(map[string]map[core.Revision]contract.RubricDefinitionV1), rubricHighestRevision: make(map[string]core.Revision),
		autoReviewerAttempts: make(map[string]contract.AutoReviewerAttemptV1), autoReviewerHistory: make(map[string]map[core.Revision]contract.AutoReviewerAttemptV1), autoReviewerKeys: make(map[string]string), autoReviewerObservations: make(map[string]contract.AutoReviewerInvocationObservationV1),
	}, nil
}

var _ reviewport.StoreV1 = (*Store)(nil)
var _ reviewport.TraceEventStoreV2 = (*Store)(nil)
var _ reviewport.StoreV2 = (*Store)(nil)
var _ reviewport.BypassStoreV1 = (*Store)(nil)
var _ reviewport.RubricStoreV1 = (*Store)(nil)
var _ reviewport.AutoReviewerStoreV1 = (*Store)(nil)
var _ reviewport.EvidenceAttachmentStoreV1 = (*Store)(nil)

func key(tenant core.TenantID, id string) string { return string(tenant) + "\x00" + id }
func targetKey(tenant core.TenantID, id string, revision core.Revision, digest core.Digest) string {
	return key(tenant, id) + "\x00" + strconv.FormatUint(uint64(revision), 10) + "\x00" + string(digest)
}

func appendHistory[T any](history map[string]map[core.Revision]T, itemKey string, revision core.Revision, value T) {
	if history[itemKey] == nil {
		history[itemKey] = make(map[core.Revision]T)
	}
	history[itemKey][revision] = value
}

func inspectExact[T any](history map[string]map[core.Revision]T, itemKey string, ref reviewport.ExactFactRefV1, identity func(T) contract.FactIdentityV1, kind string) (T, error) {
	var zero T
	if ref.ID == "" || ref.Revision == 0 || ref.Digest.Validate() != nil {
		return zero, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, kind+" exact ref is incomplete")
	}
	value, ok := history[itemKey][ref.Revision]
	if !ok {
		return zero, notFound(kind)
	}
	fact := identity(value)
	if fact.ID != ref.ID || fact.Revision != ref.Revision || fact.Digest != ref.Digest {
		return zero, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, kind+" exact ref drifted")
	}
	return clone(value)
}

func checkContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "review store request context ended")
	}
	return nil
}

func clone[T any](value T) (T, error) {
	var out T
	payload, err := json.Marshal(value)
	if err != nil {
		return out, core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review reference store could not clone fact")
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return out, core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review reference store could not restore fact")
	}
	return out, nil
}

func expected(current contract.FactIdentityV1, want reviewport.ExpectedFactV1) error {
	if want.Revision == 0 || want.Digest.Validate() != nil || current.Revision != want.Revision || current.Digest != want.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review expected revision or digest is stale")
	}
	return nil
}

func notFound(kind string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, kind+" not found")
}
func exists(kind string) error {
	return core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, kind+" already exists with different content")
}

func createOnce[T interface{ Validate() error }](items map[string]T, itemKey string, value T, digest func(T) core.Digest, kind string) (T, error) {
	var zero T
	if err := value.Validate(); err != nil {
		return zero, err
	}
	if old, ok := items[itemKey]; ok {
		if digest(old) == digest(value) {
			return clone(old)
		}
		return zero, exists(kind)
	}
	copyValue, err := clone(value)
	if err != nil {
		return zero, err
	}
	items[itemKey] = copyValue
	return clone(copyValue)
}

func (s *Store) CreateTargetCaseV1(ctx context.Context, mutation reviewport.CreateTargetCaseMutationV1) (contract.ReviewCaseV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	target, value, trace := mutation.Target, mutation.Case, mutation.Trace
	requestPresent := mutation.Request != nil
	bundlePresent := mutation.ResultBundle != nil
	bundleV2Present := mutation.ResultBundleV2 != nil
	if bundlePresent && bundleV2Present {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review admission accepts exactly one Result Bundle version")
	}
	var request contract.ReviewRequestV1
	var bundle contract.ReviewResultBundleV1
	var bundleV2 contract.ReviewResultBundleV2
	if requestPresent {
		request = *mutation.Request
		if err := request.Validate(); err != nil {
			return contract.ReviewCaseV1{}, err
		}
		if request.TenantID != target.TenantID || request.CaseID != value.ID || request.TargetID != target.ID || request.TargetRevision != target.Revision || request.TargetDigest != target.Digest || value.Rubric == nil || request.Rubric != *value.Rubric || request.ExpiresUnixNano != value.ExpiresUnixNano {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "review request does not bind exact Target and Case")
		}
	}
	if bundlePresent {
		bundle = *mutation.ResultBundle
		if err := bundle.Validate(); err != nil {
			return contract.ReviewCaseV1{}, err
		}
		if !requestPresent || request.ResultBundle == nil || bundle.TenantID != request.TenantID || request.ResultBundle.ID != bundle.ID || request.ResultBundle.Revision != bundle.Revision || request.ResultBundle.Digest != bundle.Digest {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "result bundle does not bind the exact Review Request")
		}
	}
	if bundleV2Present {
		bundleV2 = *mutation.ResultBundleV2
		if err := bundleV2.Validate(); err != nil {
			return contract.ReviewCaseV1{}, err
		}
		if !requestPresent || request.ResultBundle != nil || bundleV2.TenantID != request.TenantID || bundleV2.Request.ID != request.ID || bundleV2.Request.Revision != request.Revision || bundleV2.Request.Digest != request.Digest || bundleV2.Target.ID != target.ID || bundleV2.Target.Revision != target.Revision || bundleV2.Target.Digest != target.Digest {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "result bundle V2 does not bind the exact Request and Target")
		}
	} else if !bundlePresent && requestPresent && request.ResultBundle != nil {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "review request result bundle is missing")
	}
	if err := target.Validate(); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	if err := value.Validate(); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	if value.TenantID != target.TenantID || value.TargetID != target.ID || value.TargetRevision != target.Revision || value.TargetDigest != target.Digest {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "case does not bind the exact stored target")
	}
	tracePresent := trace.ID != ""
	if tracePresent {
		if err := reviewport.ValidateRequestedTraceV2(mutation); err != nil {
			return contract.ReviewCaseV1{}, err
		}
	}
	targetCopy, err := clone(target)
	if err != nil {
		return contract.ReviewCaseV1{}, err
	}
	caseCopy, err := clone(value)
	if err != nil {
		return contract.ReviewCaseV1{}, err
	}
	var traceCopy contract.TraceFactV1
	var requestCopy contract.ReviewRequestV1
	var bundleCopy contract.ReviewResultBundleV1
	var bundleV2Copy contract.ReviewResultBundleV2
	if requestPresent {
		requestCopy, err = clone(request)
		if err != nil {
			return contract.ReviewCaseV1{}, err
		}
	}
	if bundlePresent {
		bundleCopy, err = clone(bundle)
		if err != nil {
			return contract.ReviewCaseV1{}, err
		}
	}
	if bundleV2Present {
		bundleV2Copy, err = clone(bundleV2)
		if err != nil {
			return contract.ReviewCaseV1{}, err
		}
	}
	if tracePresent {
		traceCopy, err = clone(trace)
		if err != nil {
			return contract.ReviewCaseV1{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	targetIDKey := key(target.TenantID, target.ID)
	if requestPresent {
		if mutation.RubricCheckedUnixNano <= 0 {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review request admission requires a fresh Rubric currentness clock")
		}
		actualPoint := s.clock()
		if actualPoint.IsZero() || actualPoint.UnixNano() < mutation.RubricCheckedUnixNano {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review request admission clock regressed at the Store linearization point")
		}
		rubricKey := key(request.TenantID, request.Rubric.ID)
		currentRubric, ok := s.rubricCurrent[rubricKey]
		if !ok {
			return contract.ReviewCaseV1{}, notFound("current rubric")
		}
		if !sameRubricRef(currentRubric, request.Rubric) || s.rubricHighestRevision[rubricKey] != request.Rubric.Revision {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review request Rubric current index drifted at admission")
		}
		rubric, ok := s.rubricHistory[rubricKey][request.Rubric.Revision]
		if !ok || !sameRubricRef(rubric.ExactRef(), request.Rubric) {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review request Rubric current index points to missing history")
		}
		if err := rubric.ValidateCurrent(request.Rubric, actualPoint); err != nil {
			return contract.ReviewCaseV1{}, err
		}
		if request.ExpiresUnixNano > rubric.ExpiresUnixNano {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review request exceeds its exact Rubric currentness window")
		}
		requestKey := key(request.TenantID, request.ID)
		idempotencyKey := key(request.TenantID, request.IdempotencyKey)
		if existingID, ok := s.requestKeys[idempotencyKey]; ok {
			existing, exists := s.requests[key(request.TenantID, existingID)]
			if !exists {
				return contract.ReviewCaseV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonReviewCandidateConflict, "request idempotency index points to a missing Request")
			}
			if existing.ID != request.ID || existing.Digest != request.Digest {
				return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "review request idempotency payload changed")
			}
		}
		if existing, ok := s.requests[requestKey]; ok && existing.Digest != request.Digest {
			return contract.ReviewCaseV1{}, exists("review request")
		}
		if existingID, ok := s.requestByCase[key(request.TenantID, request.CaseID)]; ok && existingID != request.ID {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "review Case is already bound to another Request")
		}
		if bundlePresent {
			if existing, ok := s.resultBundles[key(bundle.TenantID, bundle.ID)]; ok && existing.Digest != bundle.Digest {
				return contract.ReviewCaseV1{}, exists("review result bundle")
			}
		}
		if bundleV2Present {
			if existing, ok := s.resultBundlesV2[key(bundleV2.TenantID, bundleV2.ID)]; ok && existing.Digest != bundleV2.Digest {
				return contract.ReviewCaseV1{}, exists("review result bundle V2")
			}
		}
	}
	history := s.targetHistory[targetIDKey]
	if old, ok := history[target.Revision]; ok && old.Digest != target.Digest {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "target revision already exists with another digest")
	}
	exactTargetKey := targetKey(target.TenantID, target.ID, target.Revision, target.Digest)
	if existingID, ok := s.caseByTarget[exactTargetKey]; ok {
		if existingID != value.ID {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "exact target is already bound to another case ID")
		}
		initial := s.caseHistory[key(target.TenantID, existingID)][value.Revision]
		if initial.Digest != value.Digest {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "exact target Case replay changed canonical payload")
		}
		if tracePresent {
			exists, err := s.inspectTraceCreateLocked(traceCopy)
			if err != nil {
				return contract.ReviewCaseV1{}, err
			}
			if !exists {
				return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "exact Target+Case replay introduced a new creation Trace")
			}
		}
		if requestPresent {
			stored, ok := s.requests[key(request.TenantID, request.ID)]
			if !ok || stored.Digest != request.Digest || s.requestKeys[key(request.TenantID, request.IdempotencyKey)] != request.ID {
				return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "exact Target+Case replay changed or introduced Request")
			}
			if bundlePresent {
				stored, ok := s.resultBundles[key(bundle.TenantID, bundle.ID)]
				if !ok || stored.Digest != bundle.Digest {
					return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "exact Target+Case replay changed or introduced Result Bundle")
				}
			}
			if bundleV2Present {
				stored, ok := s.resultBundlesV2[key(bundleV2.TenantID, bundleV2.ID)]
				if !ok || stored.Digest != bundleV2.Digest {
					return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "exact Target+Case replay changed or introduced Result Bundle V2")
				}
			}
		}
		return clone(s.cases[key(target.TenantID, existingID)])
	}
	if old, ok := history[target.Revision]; ok && old.Digest == target.Digest {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonReviewCandidateConflict, "target history exists without its exact Case index")
	}
	var maximumRevision core.Revision
	for revision := range history {
		if revision > maximumRevision {
			maximumRevision = revision
		}
	}
	if maximumRevision != 0 && target.Revision <= maximumRevision {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "new target revision must be strictly greater than all known history")
	}
	if currentTarget, ok := s.targets[targetIDKey]; ok {
		if target.Revision <= currentTarget.Revision {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "new target revision did not advance current target")
		}
		existingCaseID, indexed := s.currentCaseByTargetID[targetIDKey]
		if !indexed {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonReviewCandidateConflict, "current target has no current Case index")
		}
		existing, exists := s.cases[key(target.TenantID, existingCaseID)]
		if !exists || existing.TargetRevision != currentTarget.Revision || existing.TargetDigest != currentTarget.Digest {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonReviewCandidateConflict, "current Target and Case index drifted")
		}
		if existing.State != contract.CaseSupersededV1 {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "new target revision requires explicit supersede-and-create")
		}
	} else if maximumRevision != 0 {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonReviewCandidateConflict, "target history exists without current target")
	}
	caseKey := key(value.TenantID, value.ID)
	if existing, ok := s.cases[caseKey]; ok {
		if existing.TargetID != value.TargetID || existing.TargetRevision != value.TargetRevision || existing.TargetDigest != value.TargetDigest || existing.Digest != value.Digest {
			return contract.ReviewCaseV1{}, exists("review case")
		}
		return clone(existing)
	}
	if tracePresent {
		exists, err := s.inspectTraceCreateLocked(traceCopy)
		if err != nil {
			return contract.ReviewCaseV1{}, err
		}
		if exists {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "creation Trace exists before Target+Case")
		}
	}

	// All validation, index, history and Trace conflict checks are complete.
	// The writes below cannot fail and form the single compound linearization.
	if requestPresent {
		requestKey := key(requestCopy.TenantID, requestCopy.ID)
		s.requests[requestKey] = requestCopy
		appendHistory(s.requestHistory, requestKey, requestCopy.Revision, requestCopy)
		s.requestKeys[key(requestCopy.TenantID, requestCopy.IdempotencyKey)] = requestCopy.ID
		s.requestByCase[key(requestCopy.TenantID, requestCopy.CaseID)] = requestCopy.ID
	}
	if bundlePresent {
		s.resultBundles[key(bundleCopy.TenantID, bundleCopy.ID)] = bundleCopy
	}
	if bundleV2Present {
		s.resultBundlesV2[key(bundleV2Copy.TenantID, bundleV2Copy.ID)] = bundleV2Copy
	}
	s.targets[targetIDKey] = targetCopy
	appendHistory(s.targetHistory, targetIDKey, target.Revision, targetCopy)
	s.cases[caseKey] = caseCopy
	appendHistory(s.caseHistory, caseKey, value.Revision, caseCopy)
	s.caseByTarget[exactTargetKey] = value.ID
	s.currentCaseByTargetID[targetIDKey] = value.ID
	if tracePresent {
		tk := key(traceCopy.TenantID, traceCopy.ID)
		s.traces[tk] = traceCopy
		traceCaseKey := key(traceCopy.TenantID, traceCopy.CaseID)
		s.traceByCase[traceCaseKey] = append(s.traceByCase[traceCaseKey], traceCopy.ID)
	}
	return clone(caseCopy)
}

func (s *Store) InspectRequestExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewRequestV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewRequestV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return inspectExact(s.requestHistory, key(tenant, ref.ID), ref, func(v contract.ReviewRequestV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "review request")
}

func (s *Store) InspectRequestByIdempotencyV1(ctx context.Context, tenant core.TenantID, idempotency string) (contract.ReviewRequestV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewRequestV1{}, err
	}
	if idempotency == "" {
		return contract.ReviewRequestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review request idempotency key is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.requestKeys[key(tenant, idempotency)]
	if !ok {
		return contract.ReviewRequestV1{}, notFound("review request")
	}
	return clone(s.requests[key(tenant, id)])
}

func (s *Store) InspectRequestByCaseV1(ctx context.Context, tenant core.TenantID, caseID string) (contract.ReviewRequestV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewRequestV1{}, err
	}
	if caseID == "" {
		return contract.ReviewRequestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review Case ID is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.requestByCase[key(tenant, caseID)]
	if !ok {
		return contract.ReviewRequestV1{}, notFound("review request")
	}
	return clone(s.requests[key(tenant, id)])
}

func (s *Store) InspectResultBundleExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewResultBundleV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewResultBundleV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.resultBundles[key(tenant, ref.ID)]
	if !ok {
		return contract.ReviewResultBundleV1{}, notFound("review result bundle")
	}
	if value.Revision != ref.Revision || value.Digest != ref.Digest {
		return contract.ReviewResultBundleV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review result bundle exact ref drifted")
	}
	return clone(value)
}

func (s *Store) InspectResultBundleExactV2(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewResultBundleV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewResultBundleV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.resultBundlesV2[key(tenant, ref.ID)]
	if !ok {
		return contract.ReviewResultBundleV2{}, notFound("review result bundle V2")
	}
	if value.Revision != ref.Revision || value.Digest != ref.Digest {
		return contract.ReviewResultBundleV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "review result bundle V2 exact ref drifted")
	}
	return clone(value)
}

func (s *Store) inspectTraceCreateLocked(t contract.TraceFactV1) (bool, error) {
	if old, ok := s.traces[key(t.TenantID, t.ID)]; ok {
		if old.Digest == t.Digest {
			return true, nil
		}
		return false, exists("review trace")
	}
	caseKey := key(t.TenantID, t.CaseID)
	for _, id := range s.traceByCase[caseKey] {
		old := s.traces[key(t.TenantID, id)]
		if old.SourceID == t.SourceID && old.SourceEpoch == t.SourceEpoch && old.SourceSequence == t.SourceSequence {
			if old.Digest == t.Digest {
				return true, nil
			}
			return false, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review trace source sequence changed content")
		}
	}
	return false, nil
}

func (s *Store) InspectTargetV1(ctx context.Context, tenant core.TenantID, id string) (contract.TargetSnapshotV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.TargetSnapshotV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.targets[key(tenant, id)]
	if !ok {
		return contract.TargetSnapshotV1{}, notFound("review target")
	}
	return clone(v)
}

func (s *Store) InspectTargetExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.TargetSnapshotV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.TargetSnapshotV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return inspectExact(s.targetHistory, key(tenant, ref.ID), ref, func(v contract.TargetSnapshotV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "review target")
}

func (s *Store) InspectCaseByTargetV1(ctx context.Context, tenant core.TenantID, id string, revision core.Revision, digest core.Digest) (contract.ReviewCaseV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	caseID, ok := s.caseByTarget[targetKey(tenant, id, revision, digest)]
	if !ok {
		return contract.ReviewCaseV1{}, notFound("review case by target")
	}
	return clone(s.cases[key(tenant, caseID)])
}

func (s *Store) InspectCaseV1(ctx context.Context, tenant core.TenantID, id string) (contract.ReviewCaseV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.cases[key(tenant, id)]
	if !ok {
		return contract.ReviewCaseV1{}, notFound("review case")
	}
	return clone(value)
}

func (s *Store) InspectCaseExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewCaseV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return inspectExact(s.caseHistory, key(tenant, ref.ID), ref, func(v contract.ReviewCaseV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "review case")
}

// AdvanceCaseForTestV1 is a memory-reference-only setup seam. Production
// StoreV1 does not expose an eventless Case mutation.
func (s *Store) AdvanceCaseForTestV1(ctx context.Context, want reviewport.ExpectedFactV1, next contract.ReviewCaseV1) (contract.ReviewCaseV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(next.TenantID, next.ID)
	current, ok := s.cases[k]
	if !ok {
		return contract.ReviewCaseV1{}, notFound("review case")
	}
	if err := expected(current.FactIdentityV1, want); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	if next.Revision != current.Revision+1 || next.CreatedUnixNano != current.CreatedUnixNano || next.UpdatedUnixNano <= current.UpdatedUnixNano || next.TargetID != current.TargetID || next.TargetRevision != current.TargetRevision || next.TargetDigest != current.TargetDigest || next.ExpiresUnixNano != current.ExpiresUnixNano || !contract.CanTransitionCaseV1(current.State, next.State) {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "review case CAS violates immutable fields or transition")
	}
	copyValue, err := clone(next)
	if err != nil {
		return contract.ReviewCaseV1{}, err
	}
	s.cases[k] = copyValue
	appendHistory(s.caseHistory, k, copyValue.Revision, copyValue)
	return clone(copyValue)
}

func (s *Store) TransitionCaseWithTraceV2(ctx context.Context, m reviewport.TransitionCaseWithTraceMutationV2) (contract.ReviewCaseV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	if err := reviewport.ValidateTransitionCaseTraceV2(m); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(m.Next.TenantID, m.Next.ID)
	current, ok := s.cases[k]
	if !ok {
		return contract.ReviewCaseV1{}, notFound("review case")
	}
	if current.Revision == m.Next.Revision && current.Digest == m.Next.Digest {
		if err := s.inspectCommittedTraceBatchLockedV2([]contract.TraceFactV1{m.Trace}); err != nil {
			return contract.ReviewCaseV1{}, err
		}
		return clone(current)
	}
	if err := expected(current.FactIdentityV1, m.Expected); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	if m.Next.Revision != current.Revision+1 || m.Next.CreatedUnixNano != current.CreatedUnixNano || m.Next.UpdatedUnixNano <= current.UpdatedUnixNano || m.Next.TargetID != current.TargetID || m.Next.TargetRevision != current.TargetRevision || m.Next.TargetDigest != current.TargetDigest || m.Next.ExpiresUnixNano != current.ExpiresUnixNano || !contract.CanTransitionCaseV1(current.State, m.Next.State) {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "compound Case transition violates immutable fields or transition")
	}
	staged, err := s.stageTraceBatchLockedV2([]contract.TraceFactV1{m.Trace})
	if err != nil {
		return contract.ReviewCaseV1{}, err
	}
	copyValue, err := clone(m.Next)
	if err != nil {
		return contract.ReviewCaseV1{}, err
	}
	s.cases[k] = copyValue
	appendHistory(s.caseHistory, k, copyValue.Revision, copyValue)
	s.commitTraceBatchLockedV2(staged)
	return clone(copyValue)
}

func (s *Store) StartRoundV1(ctx context.Context, m reviewport.StartRoundMutationV1) (contract.ReviewCaseV1, contract.ReviewRoundV1, contract.ReviewerAssignmentV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, err
	}
	if err := m.Round.Validate(); err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, err
	}
	if err := m.Assignment.Validate(); err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, err
	}
	if err := m.Trace.Validate(); err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, err
	}
	if m.Round.Rubric == nil || m.Round.Rubric.Validate() != nil || m.Round.Rubric.Digest != m.Round.RubricDigest || m.RubricCheckedUnixNano <= 0 {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "new Review Round requires an exact S2-checked Rubric")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ck := key(m.Round.TenantID, m.Round.CaseID)
	current, ok := s.cases[ck]
	if !ok {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, notFound("review case")
	}
	if err := expected(current.FactIdentityV1, m.Expected); err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, err
	}
	actualPoint := s.clock()
	if actualPoint.IsZero() || actualPoint.UnixNano() < m.RubricCheckedUnixNano {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Round clock regressed at the Store linearization point")
	}
	if current.Rubric == nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Case has no exact Rubric ref")
	}
	rubricKey := key(current.TenantID, current.Rubric.ID)
	rubric, rubricOK := s.rubricHistory[rubricKey][current.Rubric.Revision]
	currentRubric, currentOK := s.rubricCurrent[rubricKey]
	if !rubricOK || !currentOK || m.Round.Rubric == nil || *current.Rubric != *m.Round.Rubric || currentRubric != *current.Rubric || s.rubricHighestRevision[rubricKey] != current.Rubric.Revision || rubric.ValidateCurrent(*current.Rubric, actualPoint) != nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Round exact Rubric is not current at the Store linearization point")
	}
	if current.State != contract.CaseRoutedV1 || m.Round.TenantID != current.TenantID || m.Assignment.TenantID != current.TenantID || m.Round.CaseRevision != current.Revision || m.Round.TargetID != current.TargetID || m.Round.TargetRevision != current.TargetRevision || m.Round.TargetDigest != current.TargetDigest || m.Assignment.CaseID != current.ID || m.Assignment.CaseRevision != current.Revision || m.Assignment.RoundID != m.Round.ID || m.Assignment.RoundRevision != m.Round.Revision || m.Assignment.RoundDigest != m.Round.Digest || m.Round.AssignmentID != m.Assignment.ID || m.Assignment.TargetID != current.TargetID || m.Assignment.TargetRevision != current.TargetRevision || m.Assignment.TargetDigest != current.TargetDigest || m.Trace.CaseID != current.ID || m.Trace.CaseRevision != current.Revision || m.Trace.TargetID != current.TargetID || m.Trace.TargetRevision != current.TargetRevision || m.Trace.TargetDigest != current.TargetDigest || m.Trace.Event != contract.TraceAssignedV1 || m.Round.ExpiresUnixNano > current.ExpiresUnixNano || m.Round.ExpiresUnixNano > rubric.ExpiresUnixNano || m.Assignment.ExpiresUnixNano > m.Round.ExpiresUnixNano {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "round, assignment and case do not bind exact facts")
	}
	rk, ak := key(current.TenantID, m.Round.ID), key(current.TenantID, m.Assignment.ID)
	if _, ok := s.rounds[rk]; ok {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, exists("review round")
	}
	if _, ok := s.assignments[ak]; ok {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, exists("review assignment")
	}
	traceExists, err := s.inspectTraceCreateLocked(m.Trace)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, err
	}
	if traceExists {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "round Trace exists before the compound mutation")
	}
	next := current
	next.Revision++
	next.State = contract.CaseWaitingReviewerV1
	next.CurrentRoundID = m.Round.ID
	next.CurrentAssignment = m.Assignment.ID
	next.UpdatedUnixNano = max64(m.Round.CreatedUnixNano, m.Assignment.CreatedUnixNano)
	next.Digest = ""
	sealed, err := contract.SealReviewCaseV1(next)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, err
	}
	roundCopy, _ := clone(m.Round)
	assignmentCopy, _ := clone(m.Assignment)
	s.rounds[rk] = roundCopy
	s.assignments[ak] = assignmentCopy
	appendHistory(s.assignmentHistory, ak, assignmentCopy.Revision, assignmentCopy)
	s.cases[ck] = sealed
	appendHistory(s.caseHistory, ck, sealed.Revision, sealed)
	traceCopy, _ := clone(m.Trace)
	s.traces[key(traceCopy.TenantID, traceCopy.ID)] = traceCopy
	traceCaseKey := key(traceCopy.TenantID, traceCopy.CaseID)
	s.traceByCase[traceCaseKey] = append(s.traceByCase[traceCaseKey], traceCopy.ID)
	c1, _ := clone(sealed)
	r1, _ := clone(roundCopy)
	a1, _ := clone(assignmentCopy)
	return c1, r1, a1, nil
}

func (s *Store) InspectRoundV1(ctx context.Context, tenant core.TenantID, id string) (contract.ReviewRoundV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewRoundV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.rounds[key(tenant, id)]
	if !ok {
		return contract.ReviewRoundV1{}, notFound("review round")
	}
	return clone(v)
}
func (s *Store) InspectRoundExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewRoundV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewRoundV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.rounds[key(tenant, ref.ID)]
	if !ok {
		return contract.ReviewRoundV1{}, notFound("review round")
	}
	if v.Revision != ref.Revision || v.Digest != ref.Digest {
		return contract.ReviewRoundV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review round exact ref drifted")
	}
	return clone(v)
}
func (s *Store) InspectAssignmentV1(ctx context.Context, tenant core.TenantID, id string) (contract.ReviewerAssignmentV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewerAssignmentV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.assignments[key(tenant, id)]
	if !ok {
		return contract.ReviewerAssignmentV1{}, notFound("review assignment")
	}
	return clone(v)
}
func (s *Store) InspectAssignmentExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewerAssignmentV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewerAssignmentV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return inspectExact(s.assignmentHistory, key(tenant, ref.ID), ref, func(v contract.ReviewerAssignmentV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "review assignment")
}

func (s *Store) ClaimAssignmentV1(ctx context.Context, m reviewport.ClaimAssignmentMutationV1) (contract.ReviewCaseV1, contract.ReviewerAssignmentV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	if m.UpdatedUnixNano <= 0 || m.LeaseExpiresUnixNano <= m.UpdatedUnixNano || m.LeaseHolder == "" {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonStaleLeaseRevision, "assignment claim requires bounded lease")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ck := key(m.TenantID, m.CaseID)
	currentCase, ok := s.cases[ck]
	if !ok {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, notFound("review case")
	}
	if err := expected(currentCase.FactIdentityV1, m.ExpectedCase); err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	ak := key(currentCase.TenantID, m.AssignmentID)
	assignment, ok := s.assignments[ak]
	if !ok {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, notFound("review assignment")
	}
	if err := expected(assignment.FactIdentityV1, m.ExpectedAssignment); err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	if currentCase.State != contract.CaseWaitingReviewerV1 || currentCase.CurrentAssignment != assignment.ID || assignment.State != contract.AssignmentOfferedV1 || m.LeaseExpiresUnixNano > assignment.ExpiresUnixNano {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorConflict, core.ReasonIdentityLeaseConflict, "assignment cannot be claimed from current state")
	}
	nextAssignment := assignment
	nextAssignment.Revision++
	nextAssignment.State = contract.AssignmentClaimedV1
	nextAssignment.LeaseHolder = m.LeaseHolder
	nextAssignment.LeaseExpiresUnixNano = m.LeaseExpiresUnixNano
	nextAssignment.UpdatedUnixNano = m.UpdatedUnixNano
	nextAssignment.Digest = ""
	sealedAssignment, err := contract.SealReviewerAssignmentV1(nextAssignment)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	nextCase := currentCase
	nextCase.Revision++
	nextCase.State = contract.CaseReviewingV1
	nextCase.UpdatedUnixNano = m.UpdatedUnixNano
	nextCase.Digest = ""
	sealedCase, err := contract.SealReviewCaseV1(nextCase)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	if len(m.Traces) != 1 {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "assignment claim requires exactly one ReviewStarted Trace bound to its successor Case and Assignment")
	}
	if err := reviewport.ValidateClaimAssignmentTraceV2(m.Traces[0], sealedCase, sealedAssignment); err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	traces, err := s.stageTraceBatchLockedV2(m.Traces)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	s.assignments[ak] = sealedAssignment
	s.cases[ck] = sealedCase
	appendHistory(s.assignmentHistory, ak, sealedAssignment.Revision, sealedAssignment)
	appendHistory(s.caseHistory, ck, sealedCase.Revision, sealedCase)
	s.commitTraceBatchLockedV2(traces)
	c, _ := clone(sealedCase)
	a, _ := clone(sealedAssignment)
	return c, a, nil
}

func (s *Store) CreateFindingWithTraceV2(ctx context.Context, mutation reviewport.CreateFindingWithTraceMutationV2) (contract.FindingV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.FindingV1{}, err
	}
	if err := mutation.Finding.Validate(); err != nil {
		return contract.FindingV1{}, err
	}
	if err := mutation.Trace.Validate(); err != nil {
		return contract.FindingV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createFindingLockedV2(mutation.Finding, []contract.TraceFactV1{mutation.Trace})
}

func (s *Store) createFindingLockedV2(v contract.FindingV1, traces []contract.TraceFactV1) (contract.FindingV1, error) {
	caseFact, ok := s.caseHistory[key(v.TenantID, v.CaseID)][v.CaseRevision]
	if !ok || caseFact.TargetID != v.TargetID || caseFact.TargetRevision != v.TargetRevision || caseFact.TargetDigest != v.TargetDigest {
		return contract.FindingV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Finding Case/Target exact history drifted")
	}
	round, ok := s.rounds[key(v.TenantID, v.RoundID)]
	if !ok || round.Revision != v.RoundRevision || round.Digest != v.RoundDigest || round.CaseID != v.CaseID || caseFact.CurrentRoundID != round.ID || round.TargetID != v.TargetID || round.TargetRevision != v.TargetRevision || round.TargetDigest != v.TargetDigest {
		return contract.FindingV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Finding Round exact binding drifted")
	}
	target, ok := s.targetHistory[key(v.TenantID, v.TargetID)][v.TargetRevision]
	if !ok || target.Digest != v.TargetDigest {
		return contract.FindingV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Finding Target exact history drifted")
	}
	if len(traces) != 0 {
		if len(traces) != 1 || !traceBindsHistoricalCaseV2(traces[0], caseFact, contract.TraceFindingV1) || !containsStringV2(traces[0].FactRefs, v.ID) {
			return contract.FindingV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "FindingObserved Trace drifted from its exact Finding or Case")
		}
	}
	fk := key(v.TenantID, v.ID)
	if existing, ok := s.findings[fk]; ok {
		if existing.Digest != v.Digest {
			return contract.FindingV1{}, exists("review finding")
		}
		if err := s.inspectCommittedTraceBatchLockedV2(traces); err != nil {
			return contract.FindingV1{}, err
		}
		return clone(existing)
	}
	staged, err := s.stageTraceBatchLockedV2(traces)
	if err != nil {
		return contract.FindingV1{}, err
	}
	copyValue, err := clone(v)
	if err != nil {
		return contract.FindingV1{}, err
	}
	s.findings[fk] = copyValue
	s.commitTraceBatchLockedV2(staged)
	return clone(copyValue)
}
func (s *Store) InspectFindingV1(ctx context.Context, t core.TenantID, id string) (contract.FindingV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.FindingV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.findings[key(t, id)]
	if !ok {
		return contract.FindingV1{}, notFound("review finding")
	}
	return clone(v)
}
func (s *Store) InspectFindingExactV1(ctx context.Context, t core.TenantID, ref reviewport.ExactFactRefV1) (contract.FindingV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.FindingV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.findings[key(t, ref.ID)]
	if !ok {
		return contract.FindingV1{}, notFound("review finding")
	}
	if v.Revision != ref.Revision || v.Digest != ref.Digest {
		return contract.FindingV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review finding exact ref drifted")
	}
	return clone(v)
}

func (s *Store) RecordAttestationV1(ctx context.Context, m reviewport.RecordAttestationMutationV1) (contract.ReviewCaseV1, contract.AttestationV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	if err := m.Attestation.ValidateProductionAutoProvenanceV4(); err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	if err := m.Trace.Validate(); err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	expectedNextState, err := contract.AttestationNextCaseStateV1(m.Attestation.Resolution)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	if m.NextState != expectedNextState {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "attestation next Case state does not match its Resolution")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idem := key(m.Attestation.TenantID, m.Attestation.IdempotencyKey)
	if existingID, ok := s.attestationKeys[idem]; ok {
		existing := s.attestations[key(m.Attestation.TenantID, existingID)]
		if existing.Digest != m.Attestation.Digest {
			return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "attestation idempotency key has different payload")
		}
		if err := s.inspectCommittedTraceBatchLockedV2(append([]contract.TraceFactV1{m.Trace}, m.AdditionalTraces...)); err != nil {
			return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
		}
		current := s.cases[key(existing.TenantID, existing.CaseID)]
		c, _ := clone(current)
		a, _ := clone(existing)
		return c, a, nil
	}
	ck := key(m.Attestation.TenantID, m.Attestation.CaseID)
	current, ok := s.cases[ck]
	if !ok {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, notFound("review case")
	}
	if err := expected(current.FactIdentityV1, m.Expected); err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	if current.State != contract.CaseReviewingV1 || m.Attestation.CaseRevision != current.Revision || m.Attestation.TargetID != current.TargetID || m.Attestation.TargetRevision != current.TargetRevision || m.Attestation.TargetDigest != current.TargetDigest || m.Attestation.RoundID != current.CurrentRoundID || m.Attestation.AssignmentID != current.CurrentAssignment || !contract.CanTransitionCaseV1(current.State, m.NextState) || m.Trace.CaseID != current.ID || m.Trace.CaseRevision != current.Revision || m.Trace.TargetID != current.TargetID || m.Trace.TargetRevision != current.TargetRevision || m.Trace.TargetDigest != current.TargetDigest || m.Trace.Event != contract.TraceAttestedV1 {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "attestation does not bind the active case/round/assignment")
	}
	round, ok := s.rounds[key(current.TenantID, current.CurrentRoundID)]
	if !ok {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, notFound("review round")
	}
	assignment, ok := s.assignments[key(current.TenantID, current.CurrentAssignment)]
	if !ok {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, notFound("review assignment")
	}
	target, ok := s.targetHistory[key(current.TenantID, current.TargetID)][current.TargetRevision]
	if !ok || target.Digest != current.TargetDigest {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "attestation Target exact fact drifted")
	}
	for _, condition := range m.Attestation.Conditions {
		if condition.ScopeDigest != target.ActionScopeDigest || condition.ExpiresUnixNano <= m.Attestation.ObservedUnixNano || m.Attestation.ExpiresUnixNano > condition.ExpiresUnixNano {
			return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "attestation condition Scope or TTL drifted")
		}
	}
	if round.CaseID != current.ID || round.TargetID != current.TargetID || round.TargetRevision != current.TargetRevision || round.TargetDigest != current.TargetDigest || m.Attestation.RoundRevision != round.Revision || m.Attestation.RoundDigest != round.Digest || assignment.State != contract.AssignmentClaimedV1 || assignment.CaseID != current.ID || assignment.CaseRevision != round.CaseRevision || assignment.RoundID != current.CurrentRoundID || assignment.RoundRevision != round.Revision || assignment.RoundDigest != round.Digest || assignment.TargetID != current.TargetID || assignment.TargetRevision != current.TargetRevision || assignment.TargetDigest != current.TargetDigest || m.Attestation.AssignmentRevision != assignment.Revision || m.Attestation.AssignmentDigest != assignment.Digest || assignment.Route != m.Attestation.Route || assignment.ReviewerID != m.Attestation.ReviewerID || assignment.ReviewerAuthority != m.Attestation.ReviewerAuthority || assignment.ReviewerBinding != m.Attestation.ReviewerBinding || m.Attestation.ObservedUnixNano >= assignment.LeaseExpiresUnixNano || m.Attestation.ObservedUnixNano >= assignment.ExpiresUnixNano || m.Attestation.ExpiresUnixNano > assignment.LeaseExpiresUnixNano || m.Attestation.ExpiresUnixNano > round.ExpiresUnixNano {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonStaleLeaseRevision, "attestation assignment lease or reviewer binding is stale")
	}
	domainResultID := ""
	if m.Attestation.Route == contract.RouteAutoV1 {
		ref := m.Attestation.DomainApplySettlement
		apply, ok := s.applySettlements[key(current.TenantID, ref.ID)]
		if !ok {
			return contract.ReviewCaseV1{}, contract.AttestationV1{}, notFound("review ApplySettlement")
		}
		if apply.Digest != ref.Digest || apply.Revision != ref.Revision || apply.DomainResultID != ref.DomainResultID || apply.DomainResultDigest != ref.DomainResultDigest || apply.RuntimeSettlementID != ref.RuntimeSettlementID || apply.RuntimeSettlementRevision != ref.RuntimeSettlementRevision || apply.RuntimeSettlementDigest != ref.RuntimeSettlementDigest || apply.RuntimeContractVersion != ref.RuntimeContractVersion || apply.RuntimeInspectionDigest != ref.RuntimeInspectionDigest || apply.State != ref.State || apply.State != contract.DomainApplyAppliedV1 {
			return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "auto attestation does not bind stored ApplySettlement")
		}
		result, ok := s.domainResults[key(current.TenantID, apply.DomainResultID)]
		if !ok {
			return contract.ReviewCaseV1{}, contract.AttestationV1{}, notFound("review domain result")
		}
		if result.Digest != apply.DomainResultDigest || result.TenantID != current.TenantID || result.CaseID != current.ID || result.CaseRevision != m.Attestation.CaseRevision || result.RoundID != round.ID || result.RoundRevision != round.Revision || result.RoundDigest != round.Digest || result.AssignmentID != assignment.ID || result.AssignmentRevision != assignment.Revision || result.AssignmentDigest != assignment.Digest || result.TargetID != current.TargetID || result.TargetRevision != current.TargetRevision || result.TargetDigest != current.TargetDigest || result.AttemptID != m.Attestation.ReviewerAttemptID || result.ResultDigest != m.Attestation.ReviewerResultDigest {
			return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto attestation reviewer result drifted across case, round, assignment, target, attempt or result")
		}
		domainResultID = result.ID
		if apply.RuntimeContractVersion == runtimeports.OperationSettlementContractVersionV4 {
			if err := s.validateAutoAttestationProvenanceLockedV1(m, current, round, assignment, result); err != nil {
				return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
			}
		}
	}
	ak := key(m.Attestation.TenantID, m.Attestation.ID)
	if _, ok := s.attestations[ak]; ok {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, exists("review attestation")
	}
	traceExists, err := s.inspectTraceCreateLocked(m.Trace)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	if traceExists {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "attestation Trace exists before the compound mutation")
	}
	next := current
	next.Revision++
	next.State = m.NextState
	next.UpdatedUnixNano = m.Attestation.ObservedUnixNano
	next.Digest = ""
	sealed, err := contract.SealReviewCaseV1(next)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	if err := reviewport.ValidateAttestationTracesV2(m.Trace, m.AdditionalTraces, current, sealed, m.Attestation, domainResultID); err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	traceBatch := append([]contract.TraceFactV1{m.Trace}, m.AdditionalTraces...)
	stagedTraces, err := s.stageTraceBatchLockedV2(traceBatch)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	attCopy, err := clone(m.Attestation)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	s.attestations[ak] = attCopy
	s.attestationKeys[idem] = m.Attestation.ID
	s.cases[ck] = sealed
	appendHistory(s.caseHistory, ck, sealed.Revision, sealed)
	s.commitTraceBatchLockedV2(stagedTraces)
	c, _ := clone(sealed)
	a, _ := clone(attCopy)
	return c, a, nil
}

func (s *Store) validateAutoAttestationProvenanceLockedV1(m reviewport.RecordAttestationMutationV1, current contract.ReviewCaseV1, round contract.ReviewRoundV1, assignment contract.ReviewerAssignmentV1, result contract.ReviewerInvocationResultFactV1) error {
	attestation := m.Attestation
	if attestation.AutoProvenance == nil || m.AutoTerminationCurrent == nil || m.AutoCheckedUnixNano <= 0 || attestation.ObservedUnixNano != m.AutoCheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "Runtime V4 auto attestation lacks an actual-point provenance check")
	}
	now := time.Unix(0, m.AutoCheckedUnixNano)
	provenance := *attestation.AutoProvenance
	attempt, ok := s.autoReviewerHistory[key(attestation.TenantID, provenance.Attempt.ID)][provenance.Attempt.Revision]
	currentAttempt, currentOK := s.autoReviewerAttempts[key(attestation.TenantID, provenance.Attempt.ID)]
	if !ok || !currentOK || attempt.ExactRef() != provenance.Attempt || currentAttempt.ExactRef() != provenance.Attempt || attempt.State != contract.AutoReviewerAttemptObservedV1 || attempt.Observation == nil || *attempt.Observation != provenance.Observation || attempt.DomainResult == nil || attempt.DomainResult.ID != result.ID || attempt.DomainResult.Revision != result.Revision || attempt.DomainResult.Digest != result.Digest || attempt.Case.ID != current.ID || attempt.Case.Revision != current.Revision || attempt.Case.Digest != current.Digest || attempt.Round.ID != round.ID || attempt.Round.Revision != round.Revision || attempt.Round.Digest != round.Digest || attempt.Assignment.ID != assignment.ID || attempt.Assignment.Revision != assignment.Revision || attempt.Assignment.Digest != assignment.Digest || attempt.Target.ID != current.TargetID || attempt.Target.Revision != current.TargetRevision || attempt.Target.Digest != current.TargetDigest || attempt.ReviewerID != attestation.ReviewerID || attempt.ReviewerAuthority != attestation.ReviewerAuthority || attempt.ReviewerBinding != attestation.ReviewerBinding || attempt.ContextFrameDigest != attestation.ContextFrameDigest || attempt.Rubric != provenance.Rubric {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto attestation exact Attempt provenance drifted")
	}
	observation, ok := s.autoReviewerObservations[key(attestation.TenantID, provenance.Observation.ID)]
	if !ok || observation.Ref() != provenance.Observation || observation.AttemptID != attempt.ID || attempt.InvocationAttempt == nil || observation.AttemptRevision != attempt.InvocationAttempt.Revision || observation.AttemptDigest != attempt.InvocationAttempt.Digest || observation.RuntimeAttempt.AttemptID != result.AttemptID || observation.Output.Digest != result.ResultDigest || observation.ResultSchema != result.ResultSchema {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto attestation exact Observation provenance drifted")
	}
	rubric, ok := s.rubricHistory[key(attestation.TenantID, provenance.Rubric.ID)][provenance.Rubric.Revision]
	currentRubric, rubricCurrent := s.rubricCurrent[key(attestation.TenantID, provenance.Rubric.ID)]
	if !ok || !rubricCurrent || rubric.ExactRef() != provenance.Rubric || currentRubric != provenance.Rubric || rubric.ValidateCurrent(provenance.Rubric, now) != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "auto attestation exact Rubric is not current")
	}
	if err := attempt.ValidateCurrent(now); err != nil {
		return err
	}
	if err := contract.ValidateNow(now, observation.ObservedUnixNano, observation.ExpiresUnixNano); err != nil {
		return err
	}
	terminationS1 := *m.AutoTerminationCurrent
	terminationS1Request := reviewport.AutoReviewTerminationCurrentRequestV1{TenantID: attestation.TenantID, Target: attempt.Target, Case: attempt.Case, Rubric: provenance.Rubric, ExpectedRound: attempt.Round, CheckedUnixNano: terminationS1.CheckedUnixNano}
	if err := terminationS1.ValidateCurrent(terminationS1Request, now); err != nil {
		return err
	}
	terminationS2Request := terminationS1Request
	terminationS2Request.CheckedUnixNano = m.AutoCheckedUnixNano
	terminationS2, err := s.inspectAutoReviewTerminationCurrentLockedV1(terminationS2Request)
	if err != nil {
		return err
	}
	if terminationS1.ClosureDigest != terminationS2.ClosureDigest || terminationS1.Target != terminationS2.Target || terminationS1.Case != terminationS2.Case || terminationS1.Rubric != terminationS2.Rubric || terminationS1.ExpectedRound != terminationS2.ExpectedRound {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "auto review termination history drifted across S1/S2")
	}
	if now.UnixNano() >= current.ExpiresUnixNano || now.UnixNano() >= round.ExpiresUnixNano || now.UnixNano() >= assignment.ExpiresUnixNano || now.UnixNano() >= assignment.LeaseExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "auto attestation current Review facts expired")
	}
	minimum := current.ExpiresUnixNano
	for _, value := range []int64{round.ExpiresUnixNano, assignment.ExpiresUnixNano, assignment.LeaseExpiresUnixNano, attempt.ExpiresUnixNano, observation.ExpiresUnixNano, rubric.ExpiresUnixNano, terminationS2.ExpiresUnixNano} {
		if value < minimum {
			minimum = value
		}
	}
	terminationReached := terminationS2.RepeatedFindingCount >= rubric.Termination.RepeatFindingLimit || terminationS2.RepeatedRejectionCount >= rubric.Termination.RepeatRejectionLimit || (terminationS2.RoundCount >= rubric.Termination.MaxRounds && (observation.Output.Resolution == contract.ResolutionRequestChangesV1 || observation.Output.Resolution == contract.ResolutionInsufficientEvidenceV1))
	contentMatches := attestation.Resolution == observation.Output.Resolution && reflect.DeepEqual(attestation.ReasonCodes, observation.Output.ReasonCodes)
	if terminationReached {
		contentMatches = attestation.Resolution == contract.ResolutionEscalateHumanV1 && m.NextState == contract.CaseWaitingHumanV1 && len(attestation.ReasonCodes) == 1 && attestation.ReasonCodes[0] == contract.AutoReviewTerminationCeilingReasonV1 && attestation.ConditionsDigest == ""
	}
	if attestation.ExpiresUnixNano > minimum || !contentMatches || !reflect.DeepEqual(attestation.Evidence, observation.Output.Evidence) || (!terminationReached && (attestation.ConditionsDigest != observation.Output.ConditionsDigest || !reflect.DeepEqual(attestation.Conditions, observation.Output.Conditions))) || (terminationReached && (len(attestation.Conditions) != 0 || attestation.ConditionsDigest != "")) || len(attestation.FindingRefs) != len(observation.Output.Findings) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto attestation content or TTL drifted from the applied reviewer output")
	}
	for _, id := range attestation.FindingRefs {
		finding, ok := s.findings[key(attestation.TenantID, id)]
		if !ok || finding.CaseID != current.ID || finding.CaseRevision != current.Revision || finding.RoundID != round.ID || finding.RoundRevision != round.Revision || finding.RoundDigest != round.Digest || finding.TargetID != current.TargetID || finding.TargetRevision != current.TargetRevision || finding.TargetDigest != current.TargetDigest || finding.ExpiresUnixNano > minimum {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto attestation Finding provenance drifted")
		}
		matched := false
		for _, draft := range observation.Output.Findings {
			if finding.Category == draft.Category && finding.Priority == draft.Priority && finding.Anchor == draft.Anchor && finding.Claim == draft.Claim && finding.Impact == draft.Impact && reflect.DeepEqual(finding.Evidence, draft.Evidence) {
				matched = true
				break
			}
		}
		if !matched {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto attestation Finding content drifted from reviewer output")
		}
	}
	return nil
}

func (s *Store) InspectAttestationV1(ctx context.Context, t core.TenantID, id string) (contract.AttestationV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.AttestationV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.attestations[key(t, id)]
	if !ok {
		return contract.AttestationV1{}, notFound("review attestation")
	}
	return clone(v)
}
func (s *Store) InspectAttestationExactV1(ctx context.Context, t core.TenantID, ref reviewport.ExactFactRefV1) (contract.AttestationV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.AttestationV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.attestations[key(t, ref.ID)]
	if !ok {
		return contract.AttestationV1{}, notFound("review attestation")
	}
	if v.Revision != ref.Revision || v.Digest != ref.Digest {
		return contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review attestation exact ref drifted")
	}
	return clone(v)
}
func (s *Store) InspectAttestationByIdempotencyV1(ctx context.Context, t core.TenantID, idem string) (contract.AttestationV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.AttestationV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.attestationKeys[key(t, idem)]
	if !ok {
		return contract.AttestationV1{}, notFound("review attestation")
	}
	return clone(s.attestations[key(t, id)])
}

func (s *Store) DecideV1(ctx context.Context, m reviewport.DecideMutationV1) (contract.ReviewCaseV1, contract.VerdictV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	if err := m.Verdict.ValidateProductionConditionsV2(); err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	if err := m.Trace.Validate(); err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	if m.SnapshotDigest.Validate() != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "Decide requires exact current snapshot digest")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ck := key(m.Verdict.TenantID, m.Verdict.CaseID)
	current, ok := s.cases[ck]
	if !ok {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, notFound("review case")
	}
	vk := key(m.Verdict.TenantID, m.Verdict.ID)
	if old, found := s.verdicts[vk]; found {
		if old.Digest != m.Verdict.Digest {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, exists("review verdict")
		}
		predecessor, historical := s.caseHistory[ck][m.Expected.Revision]
		if !historical || expected(predecessor.FactIdentityV1, m.Expected) != nil || m.Verdict.CaseID != predecessor.ID || m.Verdict.CaseRevision != predecessor.Revision || m.Verdict.CaseDigest != predecessor.Digest || m.Verdict.TargetID != predecessor.TargetID || m.Verdict.TargetRevision != predecessor.TargetRevision || m.Verdict.TargetDigest != predecessor.TargetDigest {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Verdict replay predecessor drifted")
		}
		successor := predecessor
		successor.Revision++
		successor.State = contract.CaseResolvedV1
		successor.VerdictID = m.Verdict.ID
		successor.VerdictRevision = m.Verdict.Revision
		successor.VerdictDigest = m.Verdict.Digest
		successor.UpdatedUnixNano = m.Verdict.UpdatedUnixNano
		successor.InvalidationReason = ""
		successor.Digest = ""
		successor, err := contract.SealReviewCaseV1(successor)
		if err != nil {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
		}
		storedSuccessor, historical := s.caseHistory[ck][successor.Revision]
		if !historical || !reflect.DeepEqual(storedSuccessor, successor) {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Verdict replay successor history drifted")
		}
		if err := reviewport.ValidateDecisionTracesV2(m.Trace, m.AdditionalTraces, predecessor, successor, m.Verdict); err != nil {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
		}
		if err := s.inspectCommittedTraceBatchLockedV2(append([]contract.TraceFactV1{m.Trace}, m.AdditionalTraces...)); err != nil {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
		}
		c, cloneErr := clone(storedSuccessor)
		if cloneErr != nil {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, cloneErr
		}
		v, cloneErr := clone(old)
		if cloneErr != nil {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, cloneErr
		}
		return c, v, nil
	}
	if err := expected(current.FactIdentityV1, m.Expected); err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	if m.RubricCheckedUnixNano <= 0 || m.Rubric.Validate() != nil || current.Rubric == nil || *current.Rubric != m.Rubric {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Decide requires the Case exact Rubric ref")
	}
	actualPoint := s.clock()
	if actualPoint.IsZero() || actualPoint.UnixNano() < m.RubricCheckedUnixNano {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Decide Rubric clock regressed at the Store linearization point")
	}
	rubricKey := key(current.TenantID, m.Rubric.ID)
	rubric, rubricOK := s.rubricHistory[rubricKey][m.Rubric.Revision]
	if !rubricOK || rubric.ExactRef() != m.Rubric || s.rubricCurrent[rubricKey] != m.Rubric || s.rubricHighestRevision[rubricKey] != m.Rubric.Revision || rubric.ValidateCurrent(m.Rubric, actualPoint) != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Decide exact Rubric is not current at the Store linearization point")
	}
	if (current.State != contract.CaseAttestedV1 && current.State != contract.CaseDecidingV1) || m.Verdict.CaseRevision != current.Revision || m.Verdict.CaseDigest != current.Digest || m.Verdict.TargetID != current.TargetID || m.Verdict.TargetRevision != current.TargetRevision || m.Verdict.TargetDigest != current.TargetDigest || m.Trace.CaseID != current.ID || m.Trace.CaseRevision != current.Revision || m.Trace.TargetID != current.TargetID || m.Trace.TargetRevision != current.TargetRevision || m.Trace.TargetDigest != current.TargetDigest || m.Trace.Event != contract.TraceVerdictV1 {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "verdict does not bind exact deciding case")
	}
	target, ok := s.targetHistory[key(current.TenantID, m.Target.ID)][m.Target.Revision]
	if !ok || target.Digest != m.Target.Digest || target.ID != current.TargetID || target.Revision != current.TargetRevision || target.Digest != current.TargetDigest {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Decide target current fact drifted")
	}
	round, ok := s.rounds[key(current.TenantID, m.Round.ID)]
	if !ok || round.Revision != m.Round.Revision || round.Digest != m.Round.Digest || round.ID != current.CurrentRoundID || round.Rubric == nil || *round.Rubric != m.Rubric || round.RubricDigest != m.Rubric.Digest || round.TargetID != current.TargetID || round.TargetRevision != current.TargetRevision || round.TargetDigest != current.TargetDigest {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Decide round current fact drifted")
	}
	assignment, ok := s.assignments[key(current.TenantID, m.Assignment.ID)]
	if !ok || assignment.Revision != m.Assignment.Revision || assignment.Digest != m.Assignment.Digest || assignment.ID != current.CurrentAssignment || assignment.RoundID != round.ID || assignment.RoundRevision != round.Revision || assignment.RoundDigest != round.Digest || assignment.TargetID != current.TargetID || assignment.TargetRevision != current.TargetRevision || assignment.TargetDigest != current.TargetDigest {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonStaleLeaseRevision, "Decide assignment current fact drifted")
	}
	attestation, ok := s.attestations[key(current.TenantID, m.Attestation.ID)]
	if !ok || attestation.Revision != m.Attestation.Revision || attestation.Digest != m.Attestation.Digest || attestation.CaseID != current.ID || attestation.RoundID != round.ID || attestation.RoundRevision != round.Revision || attestation.RoundDigest != round.Digest || attestation.AssignmentID != assignment.ID || attestation.AssignmentRevision != assignment.Revision || attestation.AssignmentDigest != assignment.Digest || attestation.TargetID != current.TargetID || attestation.TargetRevision != current.TargetRevision || attestation.TargetDigest != current.TargetDigest {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Decide attestation current fact drifted")
	}
	if err := attestation.ValidateProductionAutoProvenanceV4(); err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	if m.Verdict.ConditionsDigest != attestation.ConditionsDigest || !reflect.DeepEqual(m.Verdict.Conditions, attestation.Conditions) {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "Verdict condition set differs from its exact Attestation")
	}
	for _, condition := range m.Verdict.Conditions {
		if condition.ScopeDigest != target.ActionScopeDigest || m.Verdict.ExpiresUnixNano > condition.ExpiresUnixNano {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "Verdict condition Scope or TTL drifted")
		}
	}
	for _, ref := range m.Findings {
		finding, ok := s.findings[key(current.TenantID, ref.ID)]
		if !ok || finding.Revision != ref.Revision || finding.Digest != ref.Digest || finding.CaseID != current.ID || finding.RoundID != round.ID || finding.RoundRevision != round.Revision || finding.RoundDigest != round.Digest || finding.TargetID != current.TargetID || finding.TargetRevision != current.TargetRevision || finding.TargetDigest != current.TargetDigest {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Decide finding current fact drifted")
		}
	}
	if attestation.Route == contract.RouteAutoV1 {
		if m.ApplySettlement == nil || m.DomainResult == nil {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "auto Decide requires exact ApplySettlement and domain result")
		}
		apply, ok := s.applySettlements[key(current.TenantID, m.ApplySettlement.ID)]
		if !ok || apply.Revision != m.ApplySettlement.Revision || apply.Digest != m.ApplySettlement.Digest || apply.State != contract.DomainApplyAppliedV1 {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "auto Decide ApplySettlement is absent, non-applied or drifted")
		}
		result, ok := s.domainResults[key(current.TenantID, m.DomainResult.ID)]
		if !ok || result.Revision != m.DomainResult.Revision || result.Digest != m.DomainResult.Digest || apply.DomainResultID != result.ID || apply.DomainResultDigest != result.Digest || result.CaseID != current.ID || result.CaseRevision != attestation.CaseRevision || result.RoundID != round.ID || result.RoundRevision != round.Revision || result.RoundDigest != round.Digest || result.AssignmentID != assignment.ID || result.AssignmentRevision != assignment.Revision || result.AssignmentDigest != assignment.Digest || result.TargetID != current.TargetID || result.TargetRevision != current.TargetRevision || result.TargetDigest != current.TargetDigest || result.AttemptID != attestation.ReviewerAttemptID || result.ResultDigest != attestation.ReviewerResultDigest {
			return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto Decide reviewer result drifted")
		}
	} else if m.ApplySettlement != nil || m.DomainResult != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "human Decide cannot claim auto settlement facts")
	}
	traceExists, err := s.inspectTraceCreateLocked(m.Trace)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	if traceExists {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "verdict Trace exists before the compound mutation")
	}
	next := current
	next.Revision++
	next.State = contract.CaseResolvedV1
	next.VerdictID = m.Verdict.ID
	next.VerdictRevision = m.Verdict.Revision
	next.VerdictDigest = m.Verdict.Digest
	next.UpdatedUnixNano = m.Verdict.UpdatedUnixNano
	next.InvalidationReason = ""
	next.Digest = ""
	sealed, err := contract.SealReviewCaseV1(next)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	if err := reviewport.ValidateDecisionTracesV2(m.Trace, m.AdditionalTraces, current, sealed, m.Verdict); err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	traceBatch := append([]contract.TraceFactV1{m.Trace}, m.AdditionalTraces...)
	stagedTraces, err := s.stageTraceBatchLockedV2(traceBatch)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	verdictCopy, err := clone(m.Verdict)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	s.verdicts[vk] = verdictCopy
	appendHistory(s.verdictHistory, vk, verdictCopy.Revision, verdictCopy)
	s.cases[ck] = sealed
	appendHistory(s.caseHistory, ck, sealed.Revision, sealed)
	s.commitTraceBatchLockedV2(stagedTraces)
	c, _ := clone(sealed)
	v, _ := clone(verdictCopy)
	return c, v, nil
}

func (s *Store) InspectVerdictV1(ctx context.Context, t core.TenantID, id string) (contract.VerdictV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.VerdictV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.verdicts[key(t, id)]
	if !ok {
		return contract.VerdictV1{}, notFound("review verdict")
	}
	return clone(v)
}
func (s *Store) InspectVerdictExactV1(ctx context.Context, t core.TenantID, ref reviewport.ExactFactRefV1) (contract.VerdictV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.VerdictV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return inspectExact(s.verdictHistory, key(t, ref.ID), ref, func(v contract.VerdictV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "review verdict")
}

func (s *Store) InvalidateV1(ctx context.Context, m reviewport.InvalidateMutationV1) (contract.ReviewCaseV1, *contract.VerdictV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	if err := m.Trace.Validate(); err != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	if m.Reason == "" || m.UpdatedUnixNano <= 0 {
		return contract.ReviewCaseV1{}, nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review invalidation requires reason and time")
	}
	switch m.CaseState {
	case contract.CaseExpiredV1, contract.CaseRevokedV1, contract.CaseSupersededV1, contract.CaseCancelledV1:
	default:
		return contract.ReviewCaseV1{}, nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unsupported review invalidation state")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ck := key(m.TenantID, m.CaseID)
	current, ok := s.cases[ck]
	if !ok {
		return contract.ReviewCaseV1{}, nil, notFound("review case")
	}
	if err := expected(current.FactIdentityV1, m.Expected); err != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	if m.UpdatedUnixNano <= current.UpdatedUnixNano {
		return contract.ReviewCaseV1{}, nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review invalidation time did not advance")
	}
	if current.State == contract.CaseExpiredV1 || current.State == contract.CaseRevokedV1 || current.State == contract.CaseSupersededV1 {
		return contract.ReviewCaseV1{}, nil, core.NewError(core.ErrorConflict, core.ReasonTerminalInstance, "review case is already invalidated")
	}
	expectedTraceEvent := contract.TraceRevokedV1
	if m.CaseState == contract.CaseExpiredV1 {
		expectedTraceEvent = contract.TraceExpiredV1
	}
	if m.CaseState == contract.CaseSupersededV1 {
		expectedTraceEvent = contract.TraceSupersededV1
	}
	if m.CaseState == contract.CaseCancelledV1 {
		expectedTraceEvent = contract.TraceCancelledV1
	}
	if err := reviewport.ValidateInvalidationTraceV2(m.Trace, current, expectedTraceEvent, m.UpdatedUnixNano); err != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	stagedTraces, err := s.stageTraceBatchLockedV2([]contract.TraceFactV1{m.Trace})
	if err != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	var updatedVerdict *contract.VerdictV1
	var vk string
	if current.VerdictID != "" {
		vk = key(current.TenantID, current.VerdictID)
		oldVerdict := s.verdicts[vk]
		if oldVerdict.ID == "" {
			return contract.ReviewCaseV1{}, nil, core.NewError(core.ErrorIndeterminate, core.ReasonReviewVerdictMissing, "resolved case references missing verdict")
		}
		nextVerdict := oldVerdict
		nextVerdict.Revision++
		nextVerdict.State = m.VerdictState
		nextVerdict.UpdatedUnixNano = m.UpdatedUnixNano
		nextVerdict.InvalidationReason = m.Reason
		nextVerdict.Digest = ""
		sealedVerdict, err := contract.SealVerdictV1(nextVerdict)
		if err != nil {
			return contract.ReviewCaseV1{}, nil, err
		}
		copyVerdict, cloneErr := clone(sealedVerdict)
		if cloneErr != nil {
			return contract.ReviewCaseV1{}, nil, cloneErr
		}
		updatedVerdict = &copyVerdict
	}
	next := current
	next.Revision++
	next.State = m.CaseState
	next.VerdictID = ""
	next.VerdictRevision = 0
	next.VerdictDigest = ""
	next.UpdatedUnixNano = m.UpdatedUnixNano
	next.InvalidationReason = m.Reason
	next.Digest = ""
	sealedCase, err := contract.SealReviewCaseV1(next)
	if err != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	caseCopy, err := clone(sealedCase)
	if err != nil {
		return contract.ReviewCaseV1{}, nil, err
	}
	// This is the sole commit section: every object has already been validated,
	// sealed, cloned and staged under the same Owner lock.
	if updatedVerdict != nil {
		s.verdicts[vk] = *updatedVerdict
		appendHistory(s.verdictHistory, vk, updatedVerdict.Revision, *updatedVerdict)
	}
	s.cases[ck] = caseCopy
	appendHistory(s.caseHistory, ck, caseCopy.Revision, caseCopy)
	s.commitTraceBatchLockedV2(stagedTraces)
	c, _ := clone(caseCopy)
	return c, updatedVerdict, nil
}

// InjectTraceForTestV1 is a memory-reference-only conflict-injection seam. It
// is intentionally absent from StoreV1 and from the production SQLite Store.
func (s *Store) InjectTraceForTestV1(ctx context.Context, t contract.TraceFactV1) (contract.TraceFactV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.TraceFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendTraceLocked(t)
}
func (s *Store) InspectTraceExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.TraceFactV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.TraceFactV1{}, err
	}
	if ref.ID == "" || ref.Revision == 0 || ref.Digest.Validate() != nil {
		return contract.TraceFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review trace exact ref is incomplete")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.traces[key(tenant, ref.ID)]
	if !ok {
		return contract.TraceFactV1{}, notFound("review trace")
	}
	if value.Revision != ref.Revision || value.Digest != ref.Digest {
		return contract.TraceFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review trace exact ref drifted")
	}
	return clone(value)
}
func (s *Store) appendTraceLocked(t contract.TraceFactV1) (contract.TraceFactV1, error) {
	if err := t.Validate(); err != nil {
		return contract.TraceFactV1{}, err
	}
	tk := key(t.TenantID, t.ID)
	if old, ok := s.traces[tk]; ok {
		if old.Digest == t.Digest {
			return clone(old)
		}
		return contract.TraceFactV1{}, exists("review trace")
	}
	caseKey := key(t.TenantID, t.CaseID)
	for _, id := range s.traceByCase[caseKey] {
		old := s.traces[key(t.TenantID, id)]
		if old.SourceID == t.SourceID && old.SourceEpoch == t.SourceEpoch && old.SourceSequence == t.SourceSequence {
			if old.Digest == t.Digest {
				return clone(old)
			}
			return contract.TraceFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review trace source sequence changed content")
		}
	}
	copyValue, _ := clone(t)
	s.traces[tk] = copyValue
	s.traceByCase[caseKey] = append(s.traceByCase[caseKey], t.ID)
	return clone(copyValue)
}

func traceBindsCaseV2(trace contract.TraceFactV1, value contract.ReviewCaseV1, event contract.TraceEventV1) bool {
	return trace.TenantID == value.TenantID && trace.CaseID == value.ID && trace.CaseRevision == value.Revision && trace.TargetID == value.TargetID && trace.TargetRevision == value.TargetRevision && trace.TargetDigest == value.TargetDigest && trace.Event == event
}

func traceBindsHistoricalCaseV2(trace contract.TraceFactV1, value contract.ReviewCaseV1, event contract.TraceEventV1) bool {
	return traceBindsCaseV2(trace, value, event)
}

func containsStringV2(values []string, expected string) bool {
	index := sort.SearchStrings(values, expected)
	return index < len(values) && values[index] == expected
}

// stageTraceBatchLockedV2 validates the complete event batch before any domain
// fact or Trace index is mutated. It detects conflicts both with committed
// history and within the proposed batch.
func (s *Store) stageTraceBatchLockedV2(values []contract.TraceFactV1) ([]contract.TraceFactV1, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) > contract.MaxListItemsV1 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review Trace batch exceeds its bound")
	}
	byID := make(map[string]contract.TraceFactV1, len(values))
	bySource := make(map[string]contract.TraceFactV1, len(values))
	staged := make([]contract.TraceFactV1, 0, len(values))
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return nil, err
		}
		idKey := key(value.TenantID, value.ID)
		if old, ok := s.traces[idKey]; ok {
			if old.Digest == value.Digest {
				return nil, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "review Trace exists before its compound mutation")
			}
			return nil, exists("review trace")
		}
		if old, ok := byID[idKey]; ok {
			if old.Digest == value.Digest {
				return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review Trace batch repeats an event")
			}
			return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review Trace batch ID changed content")
		}
		sourceKey := string(value.TenantID) + "\x00" + value.CaseID + "\x00" + value.SourceID + "\x00" + strconv.FormatUint(uint64(value.SourceEpoch), 10) + "\x00" + strconv.FormatUint(value.SourceSequence, 10)
		for _, id := range s.traceByCase[key(value.TenantID, value.CaseID)] {
			old := s.traces[key(value.TenantID, id)]
			if old.SourceID == value.SourceID && old.SourceEpoch == value.SourceEpoch && old.SourceSequence == value.SourceSequence {
				return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review Trace source sequence already belongs to committed content")
			}
		}
		if _, ok := bySource[sourceKey]; ok {
			return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review Trace batch source sequence is duplicated")
		}
		copyValue, err := clone(value)
		if err != nil {
			return nil, err
		}
		byID[idKey], bySource[sourceKey] = copyValue, copyValue
		staged = append(staged, copyValue)
	}
	return staged, nil
}

func (s *Store) commitTraceBatchLockedV2(values []contract.TraceFactV1) {
	for _, value := range values {
		s.traces[key(value.TenantID, value.ID)] = value
		caseKey := key(value.TenantID, value.CaseID)
		s.traceByCase[caseKey] = append(s.traceByCase[caseKey], value.ID)
	}
}

func (s *Store) inspectCommittedTraceBatchLockedV2(values []contract.TraceFactV1) error {
	for _, expected := range values {
		actual, ok := s.traces[key(expected.TenantID, expected.ID)]
		if !ok {
			return notFound("review trace")
		}
		if actual.Revision != expected.Revision || actual.Digest != expected.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "review compound mutation replay changed its exact Trace batch")
		}
	}
	return nil
}

func (s *Store) ListTraceV1(ctx context.Context, t core.TenantID, caseID string) ([]contract.TraceFactV1, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := append([]string{}, s.traceByCase[key(t, caseID)]...)
	sort.Slice(ids, func(i, j int) bool {
		a, b := s.traces[key(t, ids[i])], s.traces[key(t, ids[j])]
		return traceLessV2(a, b)
	})
	out := make([]contract.TraceFactV1, 0, len(ids))
	for _, id := range ids {
		v, _ := clone(s.traces[key(t, id)])
		out = append(out, v)
	}
	return out, nil
}

func tracePageAfterV2(value contract.TraceFactV1) reviewport.TracePageAfterV2 {
	return reviewport.TracePageAfterV2{SourceID: value.SourceID, SourceEpoch: value.SourceEpoch, SourceSequence: value.SourceSequence, Trace: reviewport.ExactV1(value.ID, value.Revision, value.Digest)}
}

func sameTracePageAfterV2(value contract.TraceFactV1, after reviewport.TracePageAfterV2) bool {
	return value.SourceID == after.SourceID && value.SourceEpoch == after.SourceEpoch && value.SourceSequence == after.SourceSequence && value.ID == after.Trace.ID && value.Revision == after.Trace.Revision && value.Digest == after.Trace.Digest
}

func traceLessV2(a, b contract.TraceFactV1) bool {
	if a.SourceSequence != b.SourceSequence {
		return a.SourceSequence < b.SourceSequence
	}
	if a.SourceID != b.SourceID {
		return a.SourceID < b.SourceID
	}
	if a.SourceEpoch != b.SourceEpoch {
		return a.SourceEpoch < b.SourceEpoch
	}
	return a.ID < b.ID
}

func (s *Store) ListTracePageV2(ctx context.Context, request reviewport.ListTracePageRequestV2) (reviewport.ListTracePageResultV2, error) {
	if err := checkContext(ctx); err != nil {
		return reviewport.ListTracePageResultV2{}, err
	}
	if err := request.Validate(); err != nil {
		return reviewport.ListTracePageResultV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.cases[key(request.TenantID, request.CaseID)]; !ok {
		return reviewport.ListTracePageResultV2{}, notFound("review case")
	}
	ids := append([]string(nil), s.traceByCase[key(request.TenantID, request.CaseID)]...)
	sort.Slice(ids, func(i, j int) bool {
		a, b := s.traces[key(request.TenantID, ids[i])], s.traces[key(request.TenantID, ids[j])]
		return traceLessV2(a, b)
	})
	start := 0
	if request.After != nil {
		found := false
		for i, id := range ids {
			if sameTracePageAfterV2(s.traces[key(request.TenantID, id)], *request.After) {
				start, found = i+1, true
				break
			}
		}
		if !found {
			return reviewport.ListTracePageResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceCursorInvalid, "review Trace page position is not exact committed history")
		}
	}
	end := start + request.Limit
	if end > len(ids) {
		end = len(ids)
	}
	result := reviewport.ListTracePageResultV2{Events: make([]contract.TraceFactV1, 0, end-start)}
	for _, id := range ids[start:end] {
		value, err := clone(s.traces[key(request.TenantID, id)])
		if err != nil {
			return reviewport.ListTracePageResultV2{}, err
		}
		result.Events = append(result.Events, value)
	}
	if end < len(ids) && len(result.Events) != 0 {
		next := tracePageAfterV2(result.Events[len(result.Events)-1])
		result.Next = &next
	}
	return result, nil
}

func (s *Store) CreateDomainResultV1(ctx context.Context, v contract.ReviewerInvocationResultFactV1) (contract.ReviewerInvocationResultFactV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewerInvocationResultFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return createOnce(s.domainResults, key(v.TenantID, v.ID), v, func(x contract.ReviewerInvocationResultFactV1) core.Digest { return x.Digest }, "review domain result")
}
func (s *Store) InspectDomainResultV1(ctx context.Context, t core.TenantID, id string) (contract.ReviewerInvocationResultFactV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewerInvocationResultFactV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.domainResults[key(t, id)]
	if !ok {
		return contract.ReviewerInvocationResultFactV1{}, notFound("review domain result")
	}
	return clone(v)
}
func (s *Store) InspectDomainResultExactV1(ctx context.Context, t core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewerInvocationResultFactV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ReviewerInvocationResultFactV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.domainResults[key(t, ref.ID)]
	if !ok {
		return contract.ReviewerInvocationResultFactV1{}, notFound("review domain result")
	}
	if v.Revision != ref.Revision || v.Digest != ref.Digest {
		return contract.ReviewerInvocationResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review domain result exact ref drifted")
	}
	return clone(v)
}
func (s *Store) CreateApplySettlementV1(ctx context.Context, v contract.DomainApplySettlementFactV1) (contract.DomainApplySettlementFactV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	if err := v.Validate(); err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result, ok := s.domainResults[key(v.TenantID, v.DomainResultID)]
	if !ok {
		return contract.DomainApplySettlementFactV1{}, notFound("review domain result")
	}
	if result.Digest != v.DomainResultDigest {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "ApplySettlement references another domain result")
	}
	return createOnce(s.applySettlements, key(v.TenantID, v.ID), v, func(x contract.DomainApplySettlementFactV1) core.Digest { return x.Digest }, "review ApplySettlement")
}
func (s *Store) InspectApplySettlementV1(ctx context.Context, t core.TenantID, id string) (contract.DomainApplySettlementFactV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.applySettlements[key(t, id)]
	if !ok {
		return contract.DomainApplySettlementFactV1{}, notFound("review ApplySettlement")
	}
	return clone(v)
}
func (s *Store) InspectApplySettlementExactV1(ctx context.Context, t core.TenantID, ref reviewport.ExactFactRefV1) (contract.DomainApplySettlementFactV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.DomainApplySettlementFactV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.applySettlements[key(t, ref.ID)]
	if !ok {
		return contract.DomainApplySettlementFactV1{}, notFound("review ApplySettlement")
	}
	if v.Revision != ref.Revision || v.Digest != ref.Digest {
		return contract.DomainApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review ApplySettlement exact ref drifted")
	}
	return clone(v)
}

func (s *Store) CreateBehaviorFeedbackCandidateV1(ctx context.Context, value contract.BehaviorFeedbackCandidateV1) (contract.BehaviorFeedbackCandidateV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.BehaviorFeedbackCandidateV1{}, err
	}
	if err := value.Validate(); err != nil {
		return contract.BehaviorFeedbackCandidateV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	caseValue, err := inspectExact(s.caseHistory, key(value.TenantID, value.Case.ID), reviewport.ExactV1(value.Case.ID, value.Case.Revision, value.Case.Digest), func(v contract.ReviewCaseV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "review case")
	if err != nil {
		return contract.BehaviorFeedbackCandidateV1{}, err
	}
	target, err := inspectExact(s.targetHistory, key(value.TenantID, value.Target.ID), reviewport.ExactV1(value.Target.ID, value.Target.Revision, value.Target.Digest), func(v contract.TargetSnapshotV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "review target")
	if err != nil {
		return contract.BehaviorFeedbackCandidateV1{}, err
	}
	verdict, err := inspectExact(s.verdictHistory, key(value.TenantID, value.Verdict.ID), reviewport.ExactV1(value.Verdict.ID, value.Verdict.Revision, value.Verdict.Digest), func(v contract.VerdictV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "review verdict")
	if err != nil {
		return contract.BehaviorFeedbackCandidateV1{}, err
	}
	if caseValue.TargetID != target.ID || caseValue.TargetRevision != target.Revision || caseValue.TargetDigest != target.Digest || caseValue.VerdictID != verdict.ID || caseValue.VerdictRevision != verdict.Revision || caseValue.VerdictDigest != verdict.Digest || caseValue.Revision != verdict.CaseRevision+1 || value.ReviewerID != verdict.ReviewerID || value.ReviewerBinding != verdict.ReviewerBinding || value.Policy != verdict.Policy {
		return contract.BehaviorFeedbackCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "behavior feedback provenance drifted from exact Review facts")
	}
	findings := make([]contract.FindingV1, 0, len(value.Findings))
	for _, ref := range value.Findings {
		finding, ok := s.findings[key(value.TenantID, ref.ID)]
		if !ok {
			return contract.BehaviorFeedbackCandidateV1{}, notFound("review finding")
		}
		if finding.Revision != ref.Revision || finding.Digest != ref.Digest {
			return contract.BehaviorFeedbackCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "behavior feedback finding exact ref drifted")
		}
		if finding.CaseID != verdict.CaseID || finding.TargetID != verdict.TargetID || finding.TargetRevision != verdict.TargetRevision || finding.TargetDigest != verdict.TargetDigest || finding.RoundID != verdict.RoundID || finding.RoundRevision != verdict.RoundRevision || finding.RoundDigest != verdict.RoundDigest {
			return contract.BehaviorFeedbackCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "behavior feedback finding provenance drifted")
		}
		findings = append(findings, finding)
	}
	findingDigest, err := contract.ComputeFindingSetDigestV1(findings)
	if err != nil {
		return contract.BehaviorFeedbackCandidateV1{}, err
	}
	if findingDigest != verdict.FindingDigest {
		return contract.BehaviorFeedbackCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "behavior feedback finding set does not match Verdict")
	}
	return createOnce(s.behaviorFeedback, key(value.TenantID, value.ID), value, func(v contract.BehaviorFeedbackCandidateV1) core.Digest { return v.Digest }, "behavior feedback candidate")
}

func (s *Store) InspectBehaviorFeedbackCandidateExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.BehaviorFeedbackCandidateV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.BehaviorFeedbackCandidateV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.behaviorFeedback[key(tenant, ref.ID)]
	if !ok {
		return contract.BehaviorFeedbackCandidateV1{}, notFound("behavior feedback candidate")
	}
	if value.Revision != ref.Revision || value.Digest != ref.Digest {
		return contract.BehaviorFeedbackCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "behavior feedback candidate exact ref drifted")
	}
	return clone(value)
}

func (s *Store) CreateEvidenceAttachmentV1(ctx context.Context, mutation reviewport.CreateEvidenceAttachmentMutationV1) (contract.EvidenceAttachmentV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.EvidenceAttachmentV1{}, err
	}
	value := mutation.Attachment
	if err := value.Validate(); err != nil {
		return contract.EvidenceAttachmentV1{}, err
	}
	if mutation.CheckedUnixNano <= 0 {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review Evidence Attachment actual-point clock is unavailable")
	}
	copyValue, err := clone(value)
	if err != nil {
		return contract.EvidenceAttachmentV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	attachmentKey := key(value.TenantID, value.ID)
	idempotencyKey := key(value.TenantID, value.IdempotencyKey)
	if existingID, ok := s.evidenceAttachmentKeys[idempotencyKey]; ok {
		existing := s.evidenceAttachments[key(value.TenantID, existingID)]
		if existing.Digest == value.Digest {
			return clone(existing)
		}
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "review Evidence Attachment idempotency key changed content")
	}
	if existing, ok := s.evidenceAttachments[attachmentKey]; ok {
		if existing.Digest == value.Digest {
			return clone(existing)
		}
		return contract.EvidenceAttachmentV1{}, exists("review Evidence Attachment")
	}
	caseValue, ok := s.cases[key(value.TenantID, value.Case.ID)]
	if !ok {
		return contract.EvidenceAttachmentV1{}, notFound("current review case")
	}
	if caseValue.Revision != value.Case.Revision || caseValue.Digest != value.Case.Digest {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review Evidence Attachment Case is not current")
	}
	switch caseValue.State {
	case contract.CaseResolvedV1, contract.CaseExpiredV1, contract.CaseRevokedV1, contract.CaseSupersededV1, contract.CaseCancelledV1:
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "review Evidence Attachment Case is terminal")
	}
	target, ok := s.targets[key(value.TenantID, value.Target.ID)]
	if !ok {
		return contract.EvidenceAttachmentV1{}, notFound("current review target")
	}
	if target.Revision != value.Target.Revision || target.Digest != value.Target.Digest {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review Evidence Attachment Target is not current")
	}
	if caseValue.TargetID != target.ID || caseValue.TargetRevision != target.Revision || caseValue.TargetDigest != target.Digest {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "review Evidence Attachment Target drifted from current Case")
	}
	if s.currentCaseByTargetID[key(value.TenantID, target.ID)] != caseValue.ID {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review Evidence Attachment Case is not the Target current index")
	}
	if mutation.CheckedUnixNano < value.ObservedUnixNano || mutation.CheckedUnixNano >= value.ExpiresUnixNano || mutation.CheckedUnixNano >= caseValue.ExpiresUnixNano || mutation.CheckedUnixNano >= target.ExpiresUnixNano {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review Evidence Attachment current input expired at the actual point")
	}
	if value.ExpiresUnixNano > caseValue.ExpiresUnixNano || value.ExpiresUnixNano > target.ExpiresUnixNano {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "review Evidence Attachment TTL exceeds Case or Target")
	}
	s.evidenceAttachments[attachmentKey] = copyValue
	s.evidenceAttachmentKeys[idempotencyKey] = value.ID
	return clone(copyValue)
}

func (s *Store) InspectEvidenceAttachmentExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.EvidenceAttachmentV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.EvidenceAttachmentV1{}, err
	}
	if ref.ID == "" || ref.Revision == 0 || ref.Digest.Validate() != nil {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review Evidence Attachment exact ref is incomplete")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.evidenceAttachments[key(tenant, ref.ID)]
	if !ok {
		return contract.EvidenceAttachmentV1{}, notFound("review Evidence Attachment")
	}
	if value.Revision != ref.Revision || value.Digest != ref.Digest {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review Evidence Attachment exact ref drifted")
	}
	return clone(value)
}

func (s *Store) InspectEvidenceAttachmentByIdempotencyV1(ctx context.Context, tenant core.TenantID, idempotency string) (contract.EvidenceAttachmentV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.EvidenceAttachmentV1{}, err
	}
	if strings.TrimSpace(idempotency) == "" || len(idempotency) > 512 {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review Evidence Attachment idempotency key is invalid")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.evidenceAttachmentKeys[key(tenant, idempotency)]
	if !ok {
		return contract.EvidenceAttachmentV1{}, notFound("review Evidence Attachment")
	}
	return clone(s.evidenceAttachments[key(tenant, id)])
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
