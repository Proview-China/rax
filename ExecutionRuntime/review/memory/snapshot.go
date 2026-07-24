package memory

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const SnapshotContractVersionV1 = "praxis.review.state-snapshot/v1"
const MaxSnapshotBytesV1 = 64 << 20

// SnapshotV1 is the canonical tenant-scoped state of the Review Owner. It is
// used as a transaction staging format by durable backends; it is not an API,
// evidence, authority, or production capability by itself.
type SnapshotV1 struct {
	ContractVersion                 string                                                     `json:"contract_version"`
	TenantID                        core.TenantID                                              `json:"tenant_id"`
	Requests                        map[string]contract.ReviewRequestV1                        `json:"requests"`
	RequestHistory                  map[string]map[core.Revision]contract.ReviewRequestV1      `json:"request_history"`
	RequestByIdempotency            map[string]string                                          `json:"request_by_idempotency"`
	RequestByCase                   map[string]string                                          `json:"request_by_case"`
	ResultBundles                   map[string]contract.ReviewResultBundleV1                   `json:"result_bundles"`
	ResultBundlesV2                 map[string]contract.ReviewResultBundleV2                   `json:"result_bundles_v2,omitempty"`
	Targets                         map[string]contract.TargetSnapshotV1                       `json:"targets"`
	TargetHistory                   map[string]map[core.Revision]contract.TargetSnapshotV1     `json:"target_history"`
	Cases                           map[string]contract.ReviewCaseV1                           `json:"cases"`
	CaseHistory                     map[string]map[core.Revision]contract.ReviewCaseV1         `json:"case_history"`
	CurrentCaseByTarget             map[string]string                                          `json:"current_case_by_target"`
	Rounds                          map[string]contract.ReviewRoundV1                          `json:"rounds"`
	Assignments                     map[string]contract.ReviewerAssignmentV1                   `json:"assignments"`
	AssignmentHistory               map[string]map[core.Revision]contract.ReviewerAssignmentV1 `json:"assignment_history"`
	Findings                        map[string]contract.FindingV1                              `json:"findings"`
	Attestations                    map[string]contract.AttestationV1                          `json:"attestations"`
	Verdicts                        map[string]contract.VerdictV1                              `json:"verdicts"`
	VerdictHistory                  map[string]map[core.Revision]contract.VerdictV1            `json:"verdict_history"`
	Traces                          map[string]contract.TraceFactV1                            `json:"traces"`
	TraceByCase                     map[string][]string                                        `json:"trace_by_case"`
	DomainResults                   map[string]contract.ReviewerInvocationResultFactV1         `json:"domain_results"`
	ApplySettlements                map[string]contract.DomainApplySettlementFactV1            `json:"apply_settlements"`
	BehaviorFeedback                map[string]contract.BehaviorFeedbackCandidateV1            `json:"behavior_feedback"`
	EvidenceAttachments             map[string]contract.EvidenceAttachmentV1                   `json:"evidence_attachments,omitempty"`
	EvidenceAttachmentByIdempotency map[string]string                                          `json:"evidence_attachment_by_idempotency,omitempty"`
	HumanMultiSign                  *HumanMultiSignSnapshotV2                                  `json:"human_multisign_v2,omitempty"`
	Bypass                          *BypassSnapshotV1                                          `json:"bypass_v1,omitempty"`
	Rubrics                         *RubricSnapshotV1                                          `json:"rubrics_v1,omitempty"`
	AutoReviewer                    *AutoReviewerSnapshotV1                                    `json:"auto_reviewer_v1,omitempty"`
	Digest                          core.Digest                                                `json:"digest"`
}

func emptySnapshotV1(tenant core.TenantID) SnapshotV1 {
	return SnapshotV1{
		ContractVersion: SnapshotContractVersionV1,
		TenantID:        tenant,
		Requests:        map[string]contract.ReviewRequestV1{}, RequestHistory: map[string]map[core.Revision]contract.ReviewRequestV1{}, RequestByIdempotency: map[string]string{}, RequestByCase: map[string]string{}, ResultBundles: map[string]contract.ReviewResultBundleV1{}, ResultBundlesV2: map[string]contract.ReviewResultBundleV2{},
		Targets: map[string]contract.TargetSnapshotV1{}, TargetHistory: map[string]map[core.Revision]contract.TargetSnapshotV1{},
		Cases: map[string]contract.ReviewCaseV1{}, CaseHistory: map[string]map[core.Revision]contract.ReviewCaseV1{}, CurrentCaseByTarget: map[string]string{},
		Rounds: map[string]contract.ReviewRoundV1{}, Assignments: map[string]contract.ReviewerAssignmentV1{}, AssignmentHistory: map[string]map[core.Revision]contract.ReviewerAssignmentV1{},
		Findings: map[string]contract.FindingV1{}, Attestations: map[string]contract.AttestationV1{}, Verdicts: map[string]contract.VerdictV1{}, VerdictHistory: map[string]map[core.Revision]contract.VerdictV1{},
		Traces: map[string]contract.TraceFactV1{}, TraceByCase: map[string][]string{}, DomainResults: map[string]contract.ReviewerInvocationResultFactV1{}, ApplySettlements: map[string]contract.DomainApplySettlementFactV1{}, BehaviorFeedback: map[string]contract.BehaviorFeedbackCandidateV1{}, EvidenceAttachments: map[string]contract.EvidenceAttachmentV1{}, EvidenceAttachmentByIdempotency: map[string]string{},
	}
}

func (s SnapshotV1) digestValue() SnapshotV1 { s.Digest = ""; return s }

func SealSnapshotV1(s SnapshotV1) (SnapshotV1, error) {
	if s.ContractVersion == "" {
		s.ContractVersion = SnapshotContractVersionV1
	}
	s.Digest = ""
	if err := s.validateShape(); err != nil {
		return SnapshotV1{}, err
	}
	digest, err := snapshotDigestV1(s)
	if err != nil {
		return SnapshotV1{}, err
	}
	s.Digest = digest
	return s, s.Validate()
}

func (s SnapshotV1) Validate() error {
	if err := s.validateShape(); err != nil {
		return err
	}
	if err := s.Digest.Validate(); err != nil {
		return err
	}
	digest, err := snapshotDigestV1(s)
	if err != nil {
		return err
	}
	if digest != s.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "review tenant snapshot digest drifted")
	}
	return nil
}

func snapshotDigestV1(s SnapshotV1) (core.Digest, error) {
	payload, err := json.Marshal(s.digestValue())
	if err != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review tenant snapshot is not JSON serializable")
	}
	if len(payload) == 0 || len(payload) > MaxSnapshotBytesV1 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review tenant snapshot exceeds its bounded size")
	}
	framed := make([]byte, 0, len(payload)+len(SnapshotContractVersionV1)+1)
	framed = append(framed, SnapshotContractVersionV1...)
	framed = append(framed, 0)
	framed = append(framed, payload...)
	return core.DigestBytes(framed), nil
}

func (s SnapshotV1) validateShape() error {
	if s.ContractVersion != SnapshotContractVersionV1 || strings.TrimSpace(string(s.TenantID)) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review tenant snapshot identity is incomplete")
	}
	if s.Requests == nil || s.RequestHistory == nil || s.RequestByIdempotency == nil || s.RequestByCase == nil || s.ResultBundles == nil || s.ResultBundlesV2 == nil || s.Targets == nil || s.TargetHistory == nil || s.Cases == nil || s.CaseHistory == nil || s.CurrentCaseByTarget == nil || s.Rounds == nil || s.Assignments == nil || s.AssignmentHistory == nil || s.Findings == nil || s.Attestations == nil || s.Verdicts == nil || s.VerdictHistory == nil || s.Traces == nil || s.TraceByCase == nil || s.DomainResults == nil || s.ApplySettlements == nil || s.BehaviorFeedback == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review tenant snapshot maps must be present")
	}
	if err := s.HumanMultiSign.validate(s.TenantID); err != nil {
		return err
	}
	if err := s.Bypass.validate(s.TenantID); err != nil {
		return err
	}
	if err := s.Bypass.validateTraces(s.Traces); err != nil {
		return err
	}
	if err := s.Rubrics.validate(s.TenantID); err != nil {
		return err
	}
	if err := s.AutoReviewer.validate(s.TenantID, s.DomainResults); err != nil {
		return err
	}
	if err := validateCurrentHistory(s.TenantID, s.Requests, s.RequestHistory, func(v contract.ReviewRequestV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, func(v contract.ReviewRequestV1) error { return v.Validate() }, "request"); err != nil {
		return err
	}
	for idempotency, requestID := range s.RequestByIdempotency {
		value, ok := s.Requests[requestID]
		if !ok || value.IdempotencyKey != idempotency {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "snapshot request idempotency index drifted")
		}
	}
	if len(s.RequestByIdempotency) != len(s.Requests) {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "snapshot request idempotency index is incomplete")
	}
	for caseID, requestID := range s.RequestByCase {
		value, ok := s.Requests[requestID]
		if !ok || value.CaseID != caseID {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "snapshot Case-to-Request index drifted")
		}
	}
	if len(s.RequestByCase) != len(s.Requests) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "snapshot Case-to-Request index is incomplete")
	}
	for id, value := range s.ResultBundles {
		if err := validateSnapshotFact(s.TenantID, id, value.FactIdentityV1, value.Validate(), "result bundle"); err != nil {
			return err
		}
	}
	for id, value := range s.ResultBundlesV2 {
		if err := validateSnapshotFact(s.TenantID, id, value.FactIdentityV1, value.Validate(), "result bundle V2"); err != nil {
			return err
		}
	}
	for _, request := range s.Requests {
		if request.ResultBundle == nil {
			continue
		}
		bundle, oldOK := s.ResultBundles[request.ResultBundle.ID]
		oldExact := oldOK && bundle.Revision == request.ResultBundle.Revision && bundle.Digest == request.ResultBundle.Digest
		if !oldExact {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "snapshot Request-to-Result-Bundle exact ref drifted")
		}
	}
	for _, bundle := range s.ResultBundlesV2 {
		request, ok := s.Requests[bundle.Request.ID]
		if !ok || request.Revision != bundle.Request.Revision || request.Digest != bundle.Request.Digest || request.ResultBundle != nil {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "snapshot Result-Bundle-V2-to-Request exact ref drifted")
		}
		target, ok := s.Targets[bundle.Target.ID]
		if !ok || target.Revision != bundle.Target.Revision || target.Digest != bundle.Target.Digest || target.TenantID != bundle.TenantID {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "snapshot Result-Bundle-V2-to-Target exact ref drifted")
		}
	}
	if err := validateCurrentHistory(s.TenantID, s.Targets, s.TargetHistory, func(v contract.TargetSnapshotV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, func(v contract.TargetSnapshotV1) error { return v.Validate() }, "target"); err != nil {
		return err
	}
	if err := validateCurrentHistory(s.TenantID, s.Cases, s.CaseHistory, func(v contract.ReviewCaseV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, func(v contract.ReviewCaseV1) error { return v.Validate() }, "case"); err != nil {
		return err
	}
	if err := validateCurrentHistory(s.TenantID, s.Assignments, s.AssignmentHistory, func(v contract.ReviewerAssignmentV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, func(v contract.ReviewerAssignmentV1) error { return v.Validate() }, "assignment"); err != nil {
		return err
	}
	if err := validateCurrentHistory(s.TenantID, s.Verdicts, s.VerdictHistory, func(v contract.VerdictV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, func(v contract.VerdictV1) error { return v.Validate() }, "verdict"); err != nil {
		return err
	}
	for id, value := range s.Rounds {
		if err := validateSnapshotFact(s.TenantID, id, value.FactIdentityV1, value.Validate(), "round"); err != nil {
			return err
		}
	}
	for id, value := range s.Findings {
		if err := validateSnapshotFact(s.TenantID, id, value.FactIdentityV1, value.Validate(), "finding"); err != nil {
			return err
		}
	}
	attestationKeys := make(map[string]string, len(s.Attestations))
	for id, value := range s.Attestations {
		if err := validateSnapshotFact(s.TenantID, id, value.FactIdentityV1, value.Validate(), "attestation"); err != nil {
			return err
		}
		if previous, ok := attestationKeys[value.IdempotencyKey]; ok && previous != id {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "snapshot attestation idempotency key is duplicated")
		}
		attestationKeys[value.IdempotencyKey] = id
	}
	for id, value := range s.Traces {
		if err := validateSnapshotFact(s.TenantID, id, value.FactIdentityV1, value.Validate(), "trace"); err != nil {
			return err
		}
	}
	for id, value := range s.DomainResults {
		if err := validateSnapshotFact(s.TenantID, id, value.FactIdentityV1, value.Validate(), "domain result"); err != nil {
			return err
		}
	}
	for id, value := range s.ApplySettlements {
		if err := validateSnapshotFact(s.TenantID, id, value.FactIdentityV1, value.Validate(), "apply settlement"); err != nil {
			return err
		}
	}
	for id, value := range s.BehaviorFeedback {
		if err := validateSnapshotFact(s.TenantID, id, value.FactIdentityV1, value.Validate(), "behavior feedback candidate"); err != nil {
			return err
		}
		caseValue, ok := s.CaseHistory[value.Case.ID][value.Case.Revision]
		if !ok || caseValue.Digest != value.Case.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "snapshot behavior feedback Case exact ref drifted")
		}
		target, ok := s.TargetHistory[value.Target.ID][value.Target.Revision]
		if !ok || target.Digest != value.Target.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "snapshot behavior feedback Target exact ref drifted")
		}
		verdict, ok := s.VerdictHistory[value.Verdict.ID][value.Verdict.Revision]
		if !ok || verdict.Digest != value.Verdict.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "snapshot behavior feedback Verdict exact ref drifted")
		}
		if caseValue.TargetID != target.ID || caseValue.TargetRevision != target.Revision || caseValue.TargetDigest != target.Digest || caseValue.VerdictID != verdict.ID || caseValue.VerdictRevision != verdict.Revision || caseValue.VerdictDigest != verdict.Digest || caseValue.Revision != verdict.CaseRevision+1 || value.ReviewerID != verdict.ReviewerID || value.ReviewerBinding != verdict.ReviewerBinding || value.Policy != verdict.Policy {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "snapshot behavior feedback provenance drifted")
		}
		findings := make([]contract.FindingV1, 0, len(value.Findings))
		for _, ref := range value.Findings {
			finding, ok := s.Findings[ref.ID]
			if !ok || finding.Revision != ref.Revision || finding.Digest != ref.Digest || finding.CaseID != verdict.CaseID || finding.TargetID != verdict.TargetID || finding.TargetRevision != verdict.TargetRevision || finding.TargetDigest != verdict.TargetDigest || finding.RoundID != verdict.RoundID || finding.RoundRevision != verdict.RoundRevision || finding.RoundDigest != verdict.RoundDigest {
				return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "snapshot behavior feedback Finding provenance drifted")
			}
			findings = append(findings, finding)
		}
		findingDigest, err := contract.ComputeFindingSetDigestV1(findings)
		if err != nil || findingDigest != verdict.FindingDigest {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "snapshot behavior feedback Finding set drifted")
		}
	}
	attachmentKeys := make(map[string]string, len(s.EvidenceAttachments))
	for id, value := range s.EvidenceAttachments {
		if err := validateSnapshotFact(s.TenantID, id, value.FactIdentityV1, value.Validate(), "evidence attachment"); err != nil {
			return err
		}
		if previous, ok := attachmentKeys[value.IdempotencyKey]; ok && previous != id {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "snapshot Evidence Attachment idempotency key is duplicated")
		}
		attachmentKeys[value.IdempotencyKey] = id
		caseValue, ok := s.CaseHistory[value.Case.ID][value.Case.Revision]
		if !ok || caseValue.Digest != value.Case.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "snapshot Evidence Attachment Case provenance is missing")
		}
		target, ok := s.TargetHistory[value.Target.ID][value.Target.Revision]
		if !ok || target.Digest != value.Target.Digest || caseValue.TargetID != target.ID || caseValue.TargetRevision != target.Revision || caseValue.TargetDigest != target.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "snapshot Evidence Attachment Target provenance is missing")
		}
	}
	if len(attachmentKeys) != len(s.EvidenceAttachmentByIdempotency) {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "snapshot Evidence Attachment idempotency index drifted")
	}
	for idempotency, attachmentID := range attachmentKeys {
		if s.EvidenceAttachmentByIdempotency[idempotency] != attachmentID {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "snapshot Evidence Attachment idempotency index drifted")
		}
	}
	for targetID, caseID := range s.CurrentCaseByTarget {
		caseValue, ok := s.Cases[caseID]
		if !ok || caseValue.TargetID != targetID {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "snapshot current target-to-case index drifted")
		}
	}
	seenTrace := make(map[string]struct{}, len(s.Traces))
	for caseID, ids := range s.TraceByCase {
		for _, id := range ids {
			value, ok := s.Traces[id]
			if !ok || value.CaseID != caseID {
				return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "snapshot trace index drifted")
			}
			if _, duplicate := seenTrace[id]; duplicate {
				return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "snapshot trace appears more than once")
			}
			seenTrace[id] = struct{}{}
		}
	}
	if len(seenTrace) != len(s.Traces) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "snapshot trace index is incomplete")
	}
	return nil
}

func validateSnapshotFact(tenant core.TenantID, mapID string, identity contract.FactIdentityV1, factErr error, kind string) error {
	if factErr != nil {
		return factErr
	}
	if identity.TenantID != tenant || identity.ID != mapID {
		return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "snapshot "+kind+" owner or ID drifted")
	}
	return nil
}

func validateCurrentHistory[T any](tenant core.TenantID, current map[string]T, history map[string]map[core.Revision]T, identity func(T) contract.FactIdentityV1, validate func(T) error, kind string) error {
	for id, revisions := range history {
		if len(revisions) == 0 {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "snapshot "+kind+" history is empty")
		}
		for revision, value := range revisions {
			fact := identity(value)
			if err := validateSnapshotFact(tenant, id, fact, validate(value), kind); err != nil {
				return err
			}
			if fact.Revision != revision {
				return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "snapshot "+kind+" history revision drifted")
			}
		}
	}
	for id, value := range current {
		fact := identity(value)
		if err := validateSnapshotFact(tenant, id, fact, validate(value), kind); err != nil {
			return err
		}
		historical, ok := history[id][fact.Revision]
		if !ok || identity(historical).Digest != fact.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "snapshot current "+kind+" is absent from history")
		}
	}
	return nil
}

func (s *Store) ExportSnapshotV1(tenant core.TenantID) (SnapshotV1, error) {
	if strings.TrimSpace(string(tenant)) == "" {
		return SnapshotV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "snapshot tenant is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := emptySnapshotV1(tenant)
	prefix := string(tenant) + "\x00"
	copyCurrent := func(id string) string { return strings.TrimPrefix(id, prefix) }
	for itemKey, value := range s.requests {
		if strings.HasPrefix(itemKey, prefix) {
			out.Requests[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, revisions := range s.requestHistory {
		if strings.HasPrefix(itemKey, prefix) {
			out.RequestHistory[copyCurrent(itemKey)], _ = clone(revisions)
		}
	}
	for itemKey, requestID := range s.requestKeys {
		if strings.HasPrefix(itemKey, prefix) {
			out.RequestByIdempotency[copyCurrent(itemKey)] = requestID
		}
	}
	for itemKey, requestID := range s.requestByCase {
		if strings.HasPrefix(itemKey, prefix) {
			out.RequestByCase[copyCurrent(itemKey)] = requestID
		}
	}
	for itemKey, value := range s.resultBundles {
		if strings.HasPrefix(itemKey, prefix) {
			out.ResultBundles[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, value := range s.resultBundlesV2 {
		if strings.HasPrefix(itemKey, prefix) {
			out.ResultBundlesV2[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, value := range s.targets {
		if strings.HasPrefix(itemKey, prefix) {
			out.Targets[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, revisions := range s.targetHistory {
		if strings.HasPrefix(itemKey, prefix) {
			out.TargetHistory[copyCurrent(itemKey)], _ = clone(revisions)
		}
	}
	for itemKey, value := range s.cases {
		if strings.HasPrefix(itemKey, prefix) {
			out.Cases[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, revisions := range s.caseHistory {
		if strings.HasPrefix(itemKey, prefix) {
			out.CaseHistory[copyCurrent(itemKey)], _ = clone(revisions)
		}
	}
	for itemKey, caseID := range s.currentCaseByTargetID {
		if strings.HasPrefix(itemKey, prefix) {
			out.CurrentCaseByTarget[copyCurrent(itemKey)] = caseID
		}
	}
	for itemKey, value := range s.rounds {
		if strings.HasPrefix(itemKey, prefix) {
			out.Rounds[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, value := range s.assignments {
		if strings.HasPrefix(itemKey, prefix) {
			out.Assignments[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, revisions := range s.assignmentHistory {
		if strings.HasPrefix(itemKey, prefix) {
			out.AssignmentHistory[copyCurrent(itemKey)], _ = clone(revisions)
		}
	}
	for itemKey, value := range s.findings {
		if strings.HasPrefix(itemKey, prefix) {
			out.Findings[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, value := range s.attestations {
		if strings.HasPrefix(itemKey, prefix) {
			out.Attestations[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, value := range s.verdicts {
		if strings.HasPrefix(itemKey, prefix) {
			out.Verdicts[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, revisions := range s.verdictHistory {
		if strings.HasPrefix(itemKey, prefix) {
			out.VerdictHistory[copyCurrent(itemKey)], _ = clone(revisions)
		}
	}
	for itemKey, value := range s.traces {
		if strings.HasPrefix(itemKey, prefix) {
			out.Traces[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, ids := range s.traceByCase {
		if strings.HasPrefix(itemKey, prefix) {
			out.TraceByCase[copyCurrent(itemKey)], _ = clone(ids)
		}
	}
	for itemKey, value := range s.domainResults {
		if strings.HasPrefix(itemKey, prefix) {
			out.DomainResults[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, value := range s.applySettlements {
		if strings.HasPrefix(itemKey, prefix) {
			out.ApplySettlements[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, value := range s.behaviorFeedback {
		if strings.HasPrefix(itemKey, prefix) {
			out.BehaviorFeedback[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, value := range s.evidenceAttachments {
		if strings.HasPrefix(itemKey, prefix) {
			out.EvidenceAttachments[copyCurrent(itemKey)], _ = clone(value)
		}
	}
	for itemKey, attachmentID := range s.evidenceAttachmentKeys {
		if strings.HasPrefix(itemKey, prefix) {
			out.EvidenceAttachmentByIdempotency[copyCurrent(itemKey)] = attachmentID
		}
	}
	out.HumanMultiSign = s.exportHumanMultiSignSnapshotV2(tenant)
	out.Bypass = s.exportBypassSnapshotV1(tenant)
	out.Rubrics = s.exportRubricSnapshotV1(tenant)
	out.AutoReviewer = s.exportAutoReviewerSnapshotV1(tenant)
	return SealSnapshotV1(out)
}

func NewStoreFromSnapshotV1(snapshot SnapshotV1) (*Store, error) {
	return NewStoreFromSnapshotWithClockV1(snapshot, time.Now)
}

// NewStoreFromSnapshotWithClockV1 restores durable state while preserving the
// Owner clock used for actual-point Rubric currentness checks.
func NewStoreFromSnapshotWithClockV1(snapshot SnapshotV1, clock func() time.Time) (*Store, error) {
	// V2 bundles were added as an optional append-only map. Normalizing a
	// missing legacy field preserves the old canonical JSON because the field
	// is omitempty, while every live Store still owns a non-nil map.
	if snapshot.ResultBundlesV2 == nil {
		snapshot.ResultBundlesV2 = map[string]contract.ReviewResultBundleV2{}
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	s, err := NewStoreWithClockV1(clock)
	if err != nil {
		return nil, err
	}
	tenant := snapshot.TenantID
	for id, value := range snapshot.Requests {
		s.requests[key(tenant, id)], _ = clone(value)
	}
	for id, revisions := range snapshot.RequestHistory {
		s.requestHistory[key(tenant, id)], _ = clone(revisions)
	}
	for idempotency, requestID := range snapshot.RequestByIdempotency {
		s.requestKeys[key(tenant, idempotency)] = requestID
	}
	for caseID, requestID := range snapshot.RequestByCase {
		s.requestByCase[key(tenant, caseID)] = requestID
	}
	for id, value := range snapshot.ResultBundles {
		s.resultBundles[key(tenant, id)], _ = clone(value)
	}
	for id, value := range snapshot.ResultBundlesV2 {
		s.resultBundlesV2[key(tenant, id)], _ = clone(value)
	}
	for id, value := range snapshot.Targets {
		s.targets[key(tenant, id)], _ = clone(value)
	}
	for id, revisions := range snapshot.TargetHistory {
		s.targetHistory[key(tenant, id)], _ = clone(revisions)
	}
	for id, value := range snapshot.Cases {
		s.cases[key(tenant, id)], _ = clone(value)
		exactKey := targetKey(tenant, value.TargetID, value.TargetRevision, value.TargetDigest)
		if previous, exists := s.caseByTarget[exactKey]; exists && previous != id {
			return nil, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "snapshot exact target binds multiple cases")
		}
		s.caseByTarget[exactKey] = id
	}
	for id, revisions := range snapshot.CaseHistory {
		s.caseHistory[key(tenant, id)], _ = clone(revisions)
	}
	for targetID, caseID := range snapshot.CurrentCaseByTarget {
		s.currentCaseByTargetID[key(tenant, targetID)] = caseID
	}
	for id, value := range snapshot.Rounds {
		s.rounds[key(tenant, id)], _ = clone(value)
	}
	for id, value := range snapshot.Assignments {
		s.assignments[key(tenant, id)], _ = clone(value)
	}
	for id, revisions := range snapshot.AssignmentHistory {
		s.assignmentHistory[key(tenant, id)], _ = clone(revisions)
	}
	for id, value := range snapshot.Findings {
		s.findings[key(tenant, id)], _ = clone(value)
	}
	for id, value := range snapshot.Attestations {
		s.attestations[key(tenant, id)], _ = clone(value)
		s.attestationKeys[key(tenant, value.IdempotencyKey)] = id
	}
	for id, value := range snapshot.Verdicts {
		s.verdicts[key(tenant, id)], _ = clone(value)
	}
	for id, revisions := range snapshot.VerdictHistory {
		s.verdictHistory[key(tenant, id)], _ = clone(revisions)
	}
	for id, value := range snapshot.Traces {
		s.traces[key(tenant, id)], _ = clone(value)
	}
	for caseID, ids := range snapshot.TraceByCase {
		s.traceByCase[key(tenant, caseID)], _ = clone(ids)
	}
	for id, value := range snapshot.DomainResults {
		s.domainResults[key(tenant, id)], _ = clone(value)
	}
	for id, value := range snapshot.ApplySettlements {
		s.applySettlements[key(tenant, id)], _ = clone(value)
	}
	for id, value := range snapshot.BehaviorFeedback {
		s.behaviorFeedback[key(tenant, id)], _ = clone(value)
	}
	for id, value := range snapshot.EvidenceAttachments {
		s.evidenceAttachments[key(tenant, id)], _ = clone(value)
	}
	for idempotency, attachmentID := range snapshot.EvidenceAttachmentByIdempotency {
		s.evidenceAttachmentKeys[key(tenant, idempotency)] = attachmentID
	}
	s.importHumanMultiSignSnapshotV2(tenant, snapshot.HumanMultiSign)
	s.importBypassSnapshotV1(tenant, snapshot.Bypass)
	s.importRubricSnapshotV1(tenant, snapshot.Rubrics)
	s.importAutoReviewerSnapshotV1(tenant, snapshot.AutoReviewer)
	return s, nil
}

func (s SnapshotV1) CaseIDsV1(states []contract.CaseStateV1) []string {
	allowed := make(map[contract.CaseStateV1]bool, len(states))
	for _, state := range states {
		allowed[state] = true
	}
	ids := make([]string, 0, len(s.Cases))
	for id, value := range s.Cases {
		if len(allowed) == 0 || allowed[value.State] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
