package contract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	AutoReviewerInvocationContractVersionV1 = "praxis.review.auto-reviewer-invocation/v1"
	AutoReviewTerminationCeilingReasonV1    = "review.auto/termination_ceiling"
)

// AutoReviewerAttemptStateV1 is Review-owned coordination state. It is not a
// Runtime dispatch/settlement state and never grants Provider execution.
type AutoReviewerAttemptStateV1 string

const (
	AutoReviewerAttemptPreparedV1       AutoReviewerAttemptStateV1 = "prepared"
	AutoReviewerAttemptWaitingInspectV1 AutoReviewerAttemptStateV1 = "waiting_inspect"
	AutoReviewerAttemptObservedV1       AutoReviewerAttemptStateV1 = "observed"
	AutoReviewerAttemptFailedClosedV1   AutoReviewerAttemptStateV1 = "failed_closed"
	AutoReviewerAttemptEscalatedV1      AutoReviewerAttemptStateV1 = "escalated_human"
)

// AutoFindingDraftV1 is untrusted structured reviewer output. It becomes a
// Finding Fact only after Review validates it against the exact Rubric and
// the governed invocation has been truthfully settled and applied.
type AutoFindingDraftV1 struct {
	Category string                             `json:"category"`
	Priority string                             `json:"priority"`
	Anchor   string                             `json:"anchor"`
	Claim    string                             `json:"claim"`
	Impact   string                             `json:"impact"`
	Evidence []runtimeports.ReviewEvidenceRefV2 `json:"evidence"`
}

func (f AutoFindingDraftV1) validate() error {
	if invalidText(f.Category) || invalidText(f.Priority) || invalidText(f.Anchor) || invalidText(f.Claim) || invalidText(f.Impact) || len(f.Evidence) == 0 || len(f.Evidence) > MaxListItemsV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "auto reviewer Finding draft is incomplete")
	}
	if !sort.SliceIsSorted(f.Evidence, func(i, j int) bool { return f.Evidence[i].Ref < f.Evidence[j].Ref }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "auto reviewer Finding evidence must be sorted")
	}
	for i, value := range f.Evidence {
		if err := value.Validate(); err != nil {
			return err
		}
		if i > 0 && f.Evidence[i-1].Ref == value.Ref {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "auto reviewer Finding evidence is duplicated")
		}
	}
	return nil
}

// AutoReviewerStructuredOutputV1 is the bounded public output accepted from a
// governed reviewer invocation. It is an Observation until Review publishes a
// DomainResult, applies a Runtime settlement, and records an Attestation.
type AutoReviewerStructuredOutputV1 struct {
	ContractVersion  string                             `json:"contract_version"`
	Resolution       ResolutionV1                       `json:"resolution"`
	ReasonCodes      []string                           `json:"reason_codes"`
	Findings         []AutoFindingDraftV1               `json:"findings"`
	Evidence         []runtimeports.ReviewEvidenceRefV2 `json:"evidence"`
	Conditions       []runtimeports.ReviewConditionV2   `json:"conditions,omitempty"`
	ConditionsDigest core.Digest                        `json:"conditions_digest,omitempty"`
	Digest           core.Digest                        `json:"digest"`
}

func (o AutoReviewerStructuredOutputV1) Clone() AutoReviewerStructuredOutputV1 {
	o.ReasonCodes = append([]string(nil), o.ReasonCodes...)
	o.Findings = append([]AutoFindingDraftV1(nil), o.Findings...)
	for i := range o.Findings {
		o.Findings[i].Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), o.Findings[i].Evidence...)
	}
	o.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), o.Evidence...)
	o.Conditions = append([]runtimeports.ReviewConditionV2(nil), o.Conditions...)
	return o
}

// AutoReviewerStructuredOutputDraftV1 is the exact provider-facing JSON
// shape. Provider output is never trusted to choose Review contract metadata
// or a canonical digest; the host validates this draft against the Review-owned
// schema and Review seals it into AutoReviewerStructuredOutputV1.
type AutoReviewerStructuredOutputDraftV1 struct {
	Resolution  ResolutionV1                       `json:"resolution"`
	ReasonCodes []string                           `json:"reason_codes"`
	Findings    []AutoFindingDraftV1               `json:"findings"`
	Evidence    []runtimeports.ReviewEvidenceRefV2 `json:"evidence"`
	Conditions  []runtimeports.ReviewConditionV2   `json:"conditions,omitempty"`
}

func (d AutoReviewerStructuredOutputDraftV1) Clone() AutoReviewerStructuredOutputDraftV1 {
	d.ReasonCodes = append([]string(nil), d.ReasonCodes...)
	d.Findings = append([]AutoFindingDraftV1(nil), d.Findings...)
	for i := range d.Findings {
		d.Findings[i].Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), d.Findings[i].Evidence...)
	}
	d.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), d.Evidence...)
	d.Conditions = append([]runtimeports.ReviewConditionV2(nil), d.Conditions...)
	return d
}

func SealAutoReviewerStructuredOutputDraftV1(d AutoReviewerStructuredOutputDraftV1) (AutoReviewerStructuredOutputV1, error) {
	d = d.Clone()
	sortConditionsV2(d.Conditions)
	conditionsDigest := core.Digest("")
	if len(d.Conditions) > 0 {
		var err error
		conditionsDigest, err = runtimeports.DigestReviewConditionsV2(d.Conditions)
		if err != nil {
			return AutoReviewerStructuredOutputV1{}, err
		}
	}
	return SealAutoReviewerStructuredOutputV1(AutoReviewerStructuredOutputV1{
		Resolution: d.Resolution, ReasonCodes: d.ReasonCodes, Findings: d.Findings,
		Evidence: d.Evidence, Conditions: d.Conditions, ConditionsDigest: conditionsDigest,
	})
}

func (o AutoReviewerStructuredOutputV1) digestValue() AutoReviewerStructuredOutputV1 {
	o.Digest = ""
	return o
}

func (o AutoReviewerStructuredOutputV1) validateShape() error {
	if o.ContractVersion != AutoReviewerInvocationContractVersionV1 || len(o.ReasonCodes) == 0 || len(o.ReasonCodes) > MaxListItemsV1 || !sort.StringsAreSorted(o.ReasonCodes) || len(o.Findings) > MaxListItemsV1 || len(o.Evidence) == 0 || len(o.Evidence) > MaxListItemsV1 || !sort.SliceIsSorted(o.Evidence, func(i, j int) bool { return o.Evidence[i].Ref < o.Evidence[j].Ref }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "auto reviewer structured output is incomplete or non-canonical")
	}
	switch o.Resolution {
	case ResolutionAcceptV1, ResolutionConditionalV1, ResolutionRequestChangesV1, ResolutionEscalateHumanV1, ResolutionRejectV1, ResolutionInsufficientEvidenceV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "auto reviewer resolution is unsupported")
	}
	for i, value := range o.ReasonCodes {
		if invalidText(value) || (i > 0 && o.ReasonCodes[i-1] == value) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "auto reviewer reason code is invalid or duplicated")
		}
	}
	for _, finding := range o.Findings {
		if err := finding.validate(); err != nil {
			return err
		}
	}
	for i, value := range o.Evidence {
		if err := value.Validate(); err != nil {
			return err
		}
		if i > 0 && o.Evidence[i-1].Ref == value.Ref {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "auto reviewer output evidence is duplicated")
		}
	}
	if err := validateConditionsSetV2(o.Conditions, o.ConditionsDigest, o.Resolution == ResolutionConditionalV1); err != nil {
		return err
	}
	return nil
}

func SealAutoReviewerStructuredOutputV1(value AutoReviewerStructuredOutputV1) (AutoReviewerStructuredOutputV1, error) {
	value = value.Clone()
	value.ContractVersion = AutoReviewerInvocationContractVersionV1
	value.Digest = ""
	sort.Strings(value.ReasonCodes)
	sort.Slice(value.Evidence, func(i, j int) bool { return value.Evidence[i].Ref < value.Evidence[j].Ref })
	sortConditionsV2(value.Conditions)
	if len(value.Conditions) > 0 && value.ConditionsDigest == "" {
		var err error
		value.ConditionsDigest, err = runtimeports.DigestReviewConditionsV2(value.Conditions)
		if err != nil {
			return AutoReviewerStructuredOutputV1{}, err
		}
	}
	for index := range value.Findings {
		sort.Slice(value.Findings[index].Evidence, func(i, j int) bool {
			return value.Findings[index].Evidence[i].Ref < value.Findings[index].Evidence[j].Ref
		})
	}
	if err := value.validateShape(); err != nil {
		return AutoReviewerStructuredOutputV1{}, err
	}
	digest, err := seal("AutoReviewerStructuredOutputV1", value.digestValue())
	if err != nil {
		return AutoReviewerStructuredOutputV1{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func (o AutoReviewerStructuredOutputV1) Validate() error {
	if err := o.validateShape(); err != nil {
		return err
	}
	return validateSealed("AutoReviewerStructuredOutputV1", o.digestValue(), o.Digest)
}

// ValidateAutoReviewerOutputV1 validates a Provider observation against the
// exact, Review-owned Rubric. It does not create Findings or a Verdict.
func (r RubricDefinitionV1) ValidateAutoReviewerOutputV1(output AutoReviewerStructuredOutputV1) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := output.Validate(); err != nil {
		return err
	}
	allowed := false
	for _, resolution := range r.OutputSchema.AllowedResolutions {
		if resolution == output.Resolution {
			allowed = true
			break
		}
	}
	if !allowed {
		return core.NewError(core.ErrorForbidden, core.ReasonInvalidState, "auto reviewer resolution is not allowed by the exact Rubric")
	}
	if len(output.Findings) > int(r.OutputSchema.MaxFindings) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "auto reviewer Finding set exceeds the exact Rubric")
	}
	evidenceKinds := make(map[runtimeports.NamespacedNameV2]struct{})
	for _, evidence := range output.Evidence {
		evidenceKinds[evidence.Classification] = struct{}{}
	}
	for _, finding := range output.Findings {
		for _, evidence := range finding.Evidence {
			evidenceKinds[evidence.Classification] = struct{}{}
		}
	}
	for _, criterion := range r.Criteria {
		for _, required := range criterion.RequiredEvidenceKinds {
			if _, ok := evidenceKinds[required]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "auto reviewer output lacks a Rubric-required Evidence kind")
			}
		}
	}
	return nil
}

type AutoReviewerAttemptV1 struct {
	FactIdentityV1
	IdempotencyKey     string             `json:"idempotency_key"`
	Case               ExactResourceRefV1 `json:"case"`
	Round              ExactResourceRefV1 `json:"round"`
	Assignment         ExactResourceRefV1 `json:"assignment"`
	Target             ExactResourceRefV1 `json:"target"`
	Rubric             ExactResourceRefV1 `json:"rubric"`
	ContextFrameDigest core.Digest        `json:"context_frame_digest"`
	// ReviewerContext is optional only for the legacy reference/test path.
	// Production Auto Review must bind the exact Context Owner projection and
	// reread it through ReviewerContextCurrentReaderV1 at S1 and S2.
	ReviewerContext   *ReviewerContextEnvelopeRefV1            `json:"reviewer_context,omitempty"`
	ReviewerID        string                                   `json:"reviewer_id"`
	ReviewerAuthority runtimeports.AuthorityBindingRefV2       `json:"reviewer_authority"`
	ReviewerBinding   runtimeports.ReviewComponentBindingRefV2 `json:"reviewer_binding"`
	RouteID           string                                   `json:"route_id"`
	Operation         runtimeports.OperationSubjectV3          `json:"operation"`
	OperationDigest   core.Digest                              `json:"operation_digest"`
	InvocationEffect  runtimeports.ReviewInvocationEffectRefV2 `json:"invocation_effect"`
	ResultSchema      runtimeports.SchemaRefV2                 `json:"result_schema"`
	RoundOrdinal      uint32                                   `json:"round_ordinal"`
	MaxCostMicros     uint64                                   `json:"max_cost_micros"`
	// InvocationAttempt is the exact immutable Attempt revision handed to the
	// host operation gateway. Successor revisions must keep this ref so an
	// unknown external outcome can only Inspect the original attempt.
	InvocationAttempt *ExactResourceRefV1                     `json:"invocation_attempt,omitempty"`
	State             AutoReviewerAttemptStateV1              `json:"state"`
	Observation       *AutoReviewerInvocationObservationRefV1 `json:"observation,omitempty"`
	DomainResult      *ExactResourceRefV1                     `json:"domain_result,omitempty"`
	TerminationReason string                                  `json:"termination_reason,omitempty"`
	ExpiresUnixNano   int64                                   `json:"expires_unix_nano"`
}

func (a AutoReviewerAttemptV1) ExactRef() ExactResourceRefV1 {
	return ExactResourceRefV1{ID: a.ID, Revision: a.Revision, Digest: a.Digest}
}
func (a AutoReviewerAttemptV1) digestValue() AutoReviewerAttemptV1 { a.Digest = ""; return a }

func (a AutoReviewerAttemptV1) validateShape() error {
	if err := a.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if invalidID(a.IdempotencyKey) || invalidID(a.ReviewerID) || invalidID(a.RouteID) || a.ContextFrameDigest.Validate() != nil || a.OperationDigest.Validate() != nil || a.RoundOrdinal == 0 || a.MaxCostMicros == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "auto reviewer attempt identity is incomplete")
	}
	for _, ref := range []ExactResourceRefV1{a.Case, a.Round, a.Assignment, a.Target, a.Rubric} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if a.ReviewerContext != nil {
		if err := a.ReviewerContext.Validate(); err != nil {
			return err
		}
		if a.ReviewerContext.TenantID != a.TenantID {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Auto Reviewer Context belongs to another tenant")
		}
	}
	if err := a.ReviewerAuthority.Validate(); err != nil {
		return err
	}
	if err := a.ReviewerBinding.Validate(); err != nil {
		return err
	}
	if err := a.Operation.Validate(); err != nil {
		return err
	}
	operationDigest, err := a.Operation.DigestV3()
	if err != nil || operationDigest != a.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "auto reviewer Operation digest drifted")
	}
	if err := a.InvocationEffect.Validate(); err != nil {
		return err
	}
	if err := a.ResultSchema.Validate(); err != nil {
		return err
	}
	switch a.State {
	case AutoReviewerAttemptPreparedV1, AutoReviewerAttemptWaitingInspectV1:
		if a.Observation != nil || a.DomainResult != nil || a.TerminationReason != "" {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "pending auto reviewer attempt cannot claim a result")
		}
		if a.State == AutoReviewerAttemptPreparedV1 && a.InvocationAttempt != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "prepared auto reviewer attempt cannot claim an invocation source")
		}
		if a.State == AutoReviewerAttemptWaitingInspectV1 && (a.InvocationAttempt == nil || a.InvocationAttempt.Validate() != nil || a.InvocationAttempt.ID != a.ID || a.InvocationAttempt.Revision >= a.Revision) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "waiting auto reviewer attempt requires the exact original invocation Attempt")
		}
	case AutoReviewerAttemptObservedV1:
		if a.InvocationAttempt == nil || a.InvocationAttempt.Validate() != nil || a.InvocationAttempt.ID != a.ID || a.InvocationAttempt.Revision >= a.Revision || a.Observation == nil || a.DomainResult == nil || a.TerminationReason != "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "observed auto reviewer attempt requires exact Observation and DomainResult")
		}
		if err := a.Observation.Validate(); err != nil {
			return err
		}
		if err := a.DomainResult.Validate(); err != nil {
			return err
		}
	case AutoReviewerAttemptFailedClosedV1, AutoReviewerAttemptEscalatedV1:
		if a.InvocationAttempt == nil || a.InvocationAttempt.Validate() != nil || a.InvocationAttempt.ID != a.ID || a.InvocationAttempt.Revision >= a.Revision || a.Observation != nil || a.DomainResult != nil || invalidText(a.TerminationReason) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "terminated auto reviewer attempt requires one reason and no claimed result")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "auto reviewer attempt state is unsupported")
	}
	return ValidateExpires(a.CreatedUnixNano, a.ExpiresUnixNano)
}

// ReviewerContextSubjectV1 derives the only exact Context Owner subject that
// may be used for this Attempt. It does not resolve "latest" or create a new
// Context projection.
func (a AutoReviewerAttemptV1) ReviewerContextSubjectV1() ReviewerContextSubjectV1 {
	return ReviewerContextSubjectV1{
		TenantID: a.TenantID, Case: a.Case, Round: a.Round,
		Assignment: a.Assignment, Target: a.Target, Rubric: a.Rubric,
		ContextFrameDigest: a.ContextFrameDigest, OutputSchema: a.ResultSchema,
	}
}

func SealAutoReviewerAttemptV1(value AutoReviewerAttemptV1) (AutoReviewerAttemptV1, error) {
	value.ContractVersion = ContractVersionV1
	value.Digest = ""
	if err := value.validateShape(); err != nil {
		return AutoReviewerAttemptV1{}, err
	}
	digest, err := seal("AutoReviewerAttemptV1", value.digestValue())
	if err != nil {
		return AutoReviewerAttemptV1{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func (a AutoReviewerAttemptV1) Validate() error {
	if err := a.validateShape(); err != nil {
		return err
	}
	return validateSealed("AutoReviewerAttemptV1", a.digestValue(), a.Digest)
}

func (a AutoReviewerAttemptV1) ValidateCurrent(now time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	return ValidateNow(now, a.CreatedUnixNano, a.ExpiresUnixNano)
}

type AutoReviewerInvocationObservationRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r AutoReviewerInvocationObservationRefV1) Validate() error {
	if invalidID(r.ID) || r.Revision != 1 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "auto reviewer Observation ref is incomplete")
	}
	return nil
}

type AutoReviewerInvocationObservationV1 struct {
	FactIdentityV1
	AttemptID           string                                       `json:"attempt_id"`
	AttemptRevision     core.Revision                                `json:"attempt_revision"`
	AttemptDigest       core.Digest                                  `json:"attempt_digest"`
	OperationDigest     core.Digest                                  `json:"operation_digest"`
	RuntimeAttempt      runtimeports.OperationDispatchAttemptRefV3   `json:"runtime_attempt"`
	ProviderObservation runtimeports.ProviderAttemptObservationRefV2 `json:"provider_observation"`
	Output              AutoReviewerStructuredOutputV1               `json:"output"`
	ResultSchema        runtimeports.SchemaRefV2                     `json:"result_schema"`
	Tokens              uint64                                       `json:"tokens"`
	CostMicros          uint64                                       `json:"cost_micros"`
	ObservedUnixNano    int64                                        `json:"observed_unix_nano"`
	ExpiresUnixNano     int64                                        `json:"expires_unix_nano"`
}

func (o AutoReviewerInvocationObservationV1) Ref() AutoReviewerInvocationObservationRefV1 {
	return AutoReviewerInvocationObservationRefV1{ID: o.ID, Revision: o.Revision, Digest: o.Digest}
}
func (o AutoReviewerInvocationObservationV1) digestValue() AutoReviewerInvocationObservationV1 {
	o.Output = o.Output.Clone()
	o.Digest = ""
	return o
}

func (o AutoReviewerInvocationObservationV1) Clone() AutoReviewerInvocationObservationV1 {
	o.Output = o.Output.Clone()
	return o
}
func (o AutoReviewerInvocationObservationV1) validateShape() error {
	if err := o.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if o.Revision != 1 || invalidID(o.AttemptID) || o.AttemptRevision == 0 || o.AttemptDigest.Validate() != nil || o.OperationDigest.Validate() != nil || o.ObservedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "auto reviewer Observation is incomplete")
	}
	if err := o.RuntimeAttempt.Validate(); err != nil {
		return err
	}
	if err := o.ProviderObservation.Validate(); err != nil {
		return err
	}
	if err := o.Output.Validate(); err != nil {
		return err
	}
	if err := o.ResultSchema.Validate(); err != nil {
		return err
	}
	if o.Tokens == 0 || o.CostMicros == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "auto reviewer usage must be explicit and positive")
	}
	return ValidateExpires(o.ObservedUnixNano, o.ExpiresUnixNano)
}

func SealAutoReviewerInvocationObservationV1(value AutoReviewerInvocationObservationV1) (AutoReviewerInvocationObservationV1, error) {
	value = value.Clone()
	value.ContractVersion = ContractVersionV1
	value.Digest = ""
	if err := value.validateShape(); err != nil {
		return AutoReviewerInvocationObservationV1{}, err
	}
	digest, err := seal("AutoReviewerInvocationObservationV1", value.digestValue())
	if err != nil {
		return AutoReviewerInvocationObservationV1{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func (o AutoReviewerInvocationObservationV1) Validate() error {
	if err := o.validateShape(); err != nil {
		return err
	}
	return validateSealed("AutoReviewerInvocationObservationV1", o.digestValue(), o.Digest)
}
