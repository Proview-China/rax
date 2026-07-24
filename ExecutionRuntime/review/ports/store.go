package ports

import (
	"context"
	"slices"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ExpectedFactV1 struct {
	Revision core.Revision
	Digest   core.Digest
}

type ExactFactRefV1 struct {
	ID       string
	Revision core.Revision
	Digest   core.Digest
}

// CreateTargetCaseMutationV1 is the sole Case creation mutation. A non-empty
// Trace is part of the same atomic create; it is never appended afterward.
type CreateTargetCaseMutationV1 struct {
	Request      *contract.ReviewRequestV1
	ResultBundle *contract.ReviewResultBundleV1
	// ResultBundleV2 is the exact-grounding replacement. At most one Bundle
	// version may be supplied; when present it is committed atomically with
	// Request, Target, Case and optional Trace.
	ResultBundleV2 *contract.ReviewResultBundleV2
	Target         contract.TargetSnapshotV1
	Case           contract.ReviewCaseV1
	Trace          contract.TraceFactV1
	// RubricCheckedUnixNano is produced by the Review service's fresh clock.
	// The Store uses it only while atomically verifying Request.Rubric against
	// the Review-owned current index before publishing any admission facts.
	RubricCheckedUnixNano int64
}

func ValidateRequestedTraceV2(m CreateTargetCaseMutationV1) error {
	if err := m.Trace.Validate(); err != nil {
		return err
	}
	refs := []string{m.Case.ID, m.Target.ID}
	causation := m.Target.ID
	if m.Request != nil {
		refs = append(refs, m.Request.ID)
		causation = m.Request.ID
	}
	slices.Sort(refs)
	if m.Trace.Event != contract.TraceRequestedV1 || m.Trace.TenantID != m.Case.TenantID || m.Trace.CaseID != m.Case.ID || m.Trace.CaseRevision != m.Case.Revision || m.Trace.TargetID != m.Target.ID || m.Trace.TargetRevision != m.Target.Revision || m.Trace.TargetDigest != m.Target.Digest || m.Trace.CausationID != causation || m.Trace.CorrelationID != m.Case.ID || !slices.Equal(m.Trace.FactRefs, refs) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Requested Trace does not bind the exact Request, Target and Case")
	}
	return nil
}

func ExactV1(id string, revision core.Revision, digest core.Digest) ExactFactRefV1 {
	return ExactFactRefV1{ID: id, Revision: revision, Digest: digest}
}

func ExpectedV1(revision core.Revision, digest core.Digest) ExpectedFactV1 {
	return ExpectedFactV1{Revision: revision, Digest: digest}
}

type StartRoundMutationV1 struct {
	Expected   ExpectedFactV1
	Round      contract.ReviewRoundV1
	Assignment contract.ReviewerAssignmentV1
	Trace      contract.TraceFactV1
	// RubricCheckedUnixNano is the Engine S2 clock. The Store never uses it
	// as current truth; it is only the rollback floor for its own fresh clock
	// taken under the Owner lock/transaction.
	RubricCheckedUnixNano int64
}

type ClaimAssignmentMutationV1 struct {
	TenantID             core.TenantID
	ExpectedCase         ExpectedFactV1
	ExpectedAssignment   ExpectedFactV1
	CaseID               string
	AssignmentID         string
	LeaseHolder          string
	LeaseExpiresUnixNano int64
	UpdatedUnixNano      int64
	// Traces is the mandatory atomic event batch and contains exactly one
	// ReviewStarted event bound to the successor Case and Assignment.
	Traces []contract.TraceFactV1 `json:"traces,omitempty"`
}

type RecordAttestationMutationV1 struct {
	Expected    ExpectedFactV1
	Attestation contract.AttestationV1
	NextState   contract.CaseStateV1
	Trace       contract.TraceFactV1
	// AdditionalTraces is empty for ordinary attestations. An escalation
	// closure carries exactly one Escalated event for the successor Case.
	AdditionalTraces       []contract.TraceFactV1 `json:"additional_traces,omitempty"`
	AutoTerminationCurrent *AutoReviewTerminationCurrentProjectionV1
	// AutoCheckedUnixNano is required only for Runtime V4 Auto provenance. It
	// is the Review Owner's fresh actual-point clock used for current/TTL checks.
	AutoCheckedUnixNano int64
}

type DecideMutationV1 struct {
	Expected        ExpectedFactV1
	Target          ExactFactRefV1
	Round           ExactFactRefV1
	Assignment      ExactFactRefV1
	Attestation     ExactFactRefV1
	Rubric          contract.ExactResourceRefV1
	Findings        []ExactFactRefV1
	ApplySettlement *ExactFactRefV1
	DomainResult    *ExactFactRefV1
	SnapshotDigest  core.Digest
	// RubricCheckedUnixNano is the decision-current S2 clock and only serves
	// as a rollback floor for the Store's fresh actual-point clock.
	RubricCheckedUnixNano int64
	Verdict               contract.VerdictV1
	Trace                 contract.TraceFactV1
	// AdditionalTraces carries exactly one Resolved event committed with the
	// Verdict and successor Case.
	AdditionalTraces []contract.TraceFactV1 `json:"additional_traces,omitempty"`
}

type InvalidateMutationV1 struct {
	TenantID        core.TenantID
	Expected        ExpectedFactV1
	CaseID          string
	CaseState       contract.CaseStateV1
	VerdictState    contract.VerdictStateV1
	Reason          core.ReasonCode
	UpdatedUnixNano int64
	Trace           contract.TraceFactV1
}

// StoreV1 is the Review Verdict Owner's fact boundary. Compound methods are
// the only allowed linearization points for multi-fact state transitions.
// Case creation has exactly one public entrypoint: CreateTargetCaseV1. A
// Case-only create would bypass the Target identity and current indexes.
type StoreV1 interface {
	RubricStoreV1
	ResolveDecisionCurrentRequestV1(context.Context, DecisionCurrentResolveRequestV1) (DecisionCurrentRequestV1, error)
	InspectDecisionOwnerInputsV1(context.Context, DecisionCurrentRequestV1) (DecisionOwnerInputsV1, error)
	CreateTargetCaseV1(context.Context, CreateTargetCaseMutationV1) (contract.ReviewCaseV1, error)
	InspectRequestExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.ReviewRequestV1, error)
	InspectRequestByIdempotencyV1(context.Context, core.TenantID, string) (contract.ReviewRequestV1, error)
	InspectRequestByCaseV1(context.Context, core.TenantID, string) (contract.ReviewRequestV1, error)
	InspectResultBundleExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.ReviewResultBundleV1, error)
	InspectTargetV1(context.Context, core.TenantID, string) (contract.TargetSnapshotV1, error)
	InspectTargetExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.TargetSnapshotV1, error)
	InspectCaseByTargetV1(context.Context, core.TenantID, string, core.Revision, core.Digest) (contract.ReviewCaseV1, error)
	InspectCaseV1(context.Context, core.TenantID, string) (contract.ReviewCaseV1, error)
	InspectCaseExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.ReviewCaseV1, error)
	StartRoundV1(context.Context, StartRoundMutationV1) (contract.ReviewCaseV1, contract.ReviewRoundV1, contract.ReviewerAssignmentV1, error)
	InspectRoundV1(context.Context, core.TenantID, string) (contract.ReviewRoundV1, error)
	InspectRoundExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.ReviewRoundV1, error)
	InspectAssignmentV1(context.Context, core.TenantID, string) (contract.ReviewerAssignmentV1, error)
	InspectAssignmentExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.ReviewerAssignmentV1, error)
	ClaimAssignmentV1(context.Context, ClaimAssignmentMutationV1) (contract.ReviewCaseV1, contract.ReviewerAssignmentV1, error)

	InspectFindingV1(context.Context, core.TenantID, string) (contract.FindingV1, error)
	InspectFindingExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.FindingV1, error)
	RecordAttestationV1(context.Context, RecordAttestationMutationV1) (contract.ReviewCaseV1, contract.AttestationV1, error)
	InspectAttestationV1(context.Context, core.TenantID, string) (contract.AttestationV1, error)
	InspectAttestationExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.AttestationV1, error)
	InspectAttestationByIdempotencyV1(context.Context, core.TenantID, string) (contract.AttestationV1, error)

	DecideV1(context.Context, DecideMutationV1) (contract.ReviewCaseV1, contract.VerdictV1, error)
	InspectVerdictV1(context.Context, core.TenantID, string) (contract.VerdictV1, error)
	InspectVerdictExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.VerdictV1, error)
	InvalidateV1(context.Context, InvalidateMutationV1) (contract.ReviewCaseV1, *contract.VerdictV1, error)

	InspectTraceExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.TraceFactV1, error)
	ListTraceV1(context.Context, core.TenantID, string) ([]contract.TraceFactV1, error)

	CreateDomainResultV1(context.Context, contract.ReviewerInvocationResultFactV1) (contract.ReviewerInvocationResultFactV1, error)
	InspectDomainResultV1(context.Context, core.TenantID, string) (contract.ReviewerInvocationResultFactV1, error)
	InspectDomainResultExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.ReviewerInvocationResultFactV1, error)
	CreateApplySettlementV1(context.Context, contract.DomainApplySettlementFactV1) (contract.DomainApplySettlementFactV1, error)
	InspectApplySettlementV1(context.Context, core.TenantID, string) (contract.DomainApplySettlementFactV1, error)
	InspectApplySettlementExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.DomainApplySettlementFactV1, error)
	CreateBehaviorFeedbackCandidateV1(context.Context, contract.BehaviorFeedbackCandidateV1) (contract.BehaviorFeedbackCandidateV1, error)
	InspectBehaviorFeedbackCandidateExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.BehaviorFeedbackCandidateV1, error)
}
