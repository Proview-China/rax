package contract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RubricKindV1 selects one bounded review protocol. It is not a free-form
// prompt name and does not grant execution authority.
type RubricKindV1 string

const (
	RubricActionSafetyV1      RubricKindV1 = "action_safety"
	RubricCodeChangeV1        RubricKindV1 = "code_change"
	RubricWorkStateV1         RubricKindV1 = "work_state"
	RubricArtifactQualityV1   RubricKindV1 = "artifact_quality"
	RubricOutcomeAcceptanceV1 RubricKindV1 = "outcome_acceptance"
	RubricLegalComplianceV1   RubricKindV1 = "legal_compliance"
	RubricFinanceControlV1    RubricKindV1 = "finance_control"
)

type RubricRuleKindV1 string

const (
	RubricRuleRequireEvidenceV1         RubricRuleKindV1 = "require_evidence"
	RubricRuleProhibitPatternV1         RubricRuleKindV1 = "prohibit_pattern"
	RubricRuleRequireExactMatchV1       RubricRuleKindV1 = "require_exact_match"
	RubricRuleRequireTestSignalV1       RubricRuleKindV1 = "require_test_signal"
	RubricRuleRequireApprovalV1         RubricRuleKindV1 = "require_approval"
	RubricRuleRequireAttributionV1      RubricRuleKindV1 = "require_source_attribution"
	RubricRuleRequireClaimCoverageV1    RubricRuleKindV1 = "require_claim_coverage"
	RubricRuleRequireScopeContainmentV1 RubricRuleKindV1 = "require_scope_containment"
)

type RubricReadOnlyCapabilityV1 string

const (
	RubricReadTargetV1       RubricReadOnlyCapabilityV1 = "readonly.target.inspect"
	RubricReadArtifactV1     RubricReadOnlyCapabilityV1 = "readonly.artifact.inspect"
	RubricReadDiffV1         RubricReadOnlyCapabilityV1 = "readonly.diff.inspect"
	RubricReadEvidenceV1     RubricReadOnlyCapabilityV1 = "readonly.evidence.inspect"
	RubricReadSourceV1       RubricReadOnlyCapabilityV1 = "readonly.source.inspect"
	RubricReadTestResultV1   RubricReadOnlyCapabilityV1 = "readonly.test-result.inspect"
	RubricReadPolicyV1       RubricReadOnlyCapabilityV1 = "readonly.policy.inspect"
	RubricReadOrganizationV1 RubricReadOnlyCapabilityV1 = "readonly.organization.inspect"
)

type RubricStateV1 string

const (
	RubricActiveV1  RubricStateV1 = "active"
	RubricRevokedV1 RubricStateV1 = "revoked"
)

type RubricCriterionV1 struct {
	ID                    string                          `json:"id"`
	Title                 string                          `json:"title"`
	Objective             string                          `json:"objective"`
	Priority              string                          `json:"priority"`
	RequiredEvidenceKinds []runtimeports.NamespacedNameV2 `json:"required_evidence_kinds"`
	FailureResolution     ResolutionV1                    `json:"failure_resolution"`
}

func (c RubricCriterionV1) validate() error {
	if invalidID(c.ID) || invalidText(c.Title) || invalidText(c.Objective) || invalidText(c.Priority) || len(c.RequiredEvidenceKinds) == 0 || len(c.RequiredEvidenceKinds) > MaxListItemsV1 || !sort.SliceIsSorted(c.RequiredEvidenceKinds, func(i, j int) bool { return c.RequiredEvidenceKinds[i] < c.RequiredEvidenceKinds[j] }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review rubric criterion is incomplete or non-canonical")
	}
	for i, value := range c.RequiredEvidenceKinds {
		if runtimeports.ValidateNamespacedNameV2(value) != nil || (i > 0 && c.RequiredEvidenceKinds[i-1] == value) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review rubric criterion evidence kinds are invalid or duplicated")
		}
	}
	switch c.FailureResolution {
	case ResolutionRequestChangesV1, ResolutionEscalateHumanV1, ResolutionRejectV1, ResolutionInsufficientEvidenceV1:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review rubric criterion failure resolution is unsupported")
	}
}

type RubricRuleV1 struct {
	ID          string           `json:"id"`
	CriterionID string           `json:"criterion_id"`
	Kind        RubricRuleKindV1 `json:"kind"`
	Subject     string           `json:"subject"`
	Expected    string           `json:"expected"`
	Required    bool             `json:"required"`
}

func (r RubricRuleV1) validate() error {
	if invalidID(r.ID) || invalidID(r.CriterionID) || invalidText(r.Subject) || invalidText(r.Expected) || !r.Required {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review rubric rule is incomplete")
	}
	switch r.Kind {
	case RubricRuleRequireEvidenceV1, RubricRuleProhibitPatternV1, RubricRuleRequireExactMatchV1, RubricRuleRequireTestSignalV1, RubricRuleRequireApprovalV1, RubricRuleRequireAttributionV1, RubricRuleRequireClaimCoverageV1, RubricRuleRequireScopeContainmentV1:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review rubric rule kind is unsupported")
	}
}

type RubricOutputSchemaV1 struct {
	SchemaID                              string         `json:"schema_id"`
	AllowedResolutions                    []ResolutionV1 `json:"allowed_resolutions"`
	RequiredFindingFields                 []string       `json:"required_finding_fields"`
	MaxFindings                           uint32         `json:"max_findings"`
	RequireReasonCodes                    bool           `json:"require_reason_codes"`
	RequireEvidenceRefs                   bool           `json:"require_evidence_refs"`
	RequireConditionsDigestForConditional bool           `json:"require_conditions_digest_for_conditional"`
}

func (s RubricOutputSchemaV1) validate() error {
	if invalidID(s.SchemaID) || len(s.AllowedResolutions) == 0 || len(s.AllowedResolutions) > 6 || s.MaxFindings == 0 || s.MaxFindings > MaxListItemsV1 || !s.RequireReasonCodes || !s.RequireEvidenceRefs || !s.RequireConditionsDigestForConditional {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review rubric output schema is incomplete")
	}
	if !sort.SliceIsSorted(s.AllowedResolutions, func(i, j int) bool { return s.AllowedResolutions[i] < s.AllowedResolutions[j] }) || !sort.StringsAreSorted(s.RequiredFindingFields) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review rubric output schema lists must be sorted")
	}
	allowedResolution := map[ResolutionV1]bool{
		ResolutionAcceptV1: true, ResolutionConditionalV1: true, ResolutionRequestChangesV1: true,
		ResolutionEscalateHumanV1: true, ResolutionRejectV1: true, ResolutionInsufficientEvidenceV1: true,
	}
	for i, value := range s.AllowedResolutions {
		if !allowedResolution[value] || (i > 0 && s.AllowedResolutions[i-1] == value) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review rubric output resolutions are invalid or duplicated")
		}
	}
	requiredFinding := map[string]bool{"anchor": true, "claim": true, "evidence": true, "impact": true, "priority": true}
	if len(s.RequiredFindingFields) != len(requiredFinding) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review rubric output must require the complete Finding shape")
	}
	for i, value := range s.RequiredFindingFields {
		if !requiredFinding[value] || (i > 0 && s.RequiredFindingFields[i-1] == value) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review rubric output Finding fields are invalid or duplicated")
		}
	}
	return nil
}

type RubricTerminationCeilingsV1 struct {
	MaxRounds            uint32 `json:"max_rounds"`
	MaxDurationNanos     int64  `json:"max_duration_nanos"`
	MaxTokens            uint64 `json:"max_tokens"`
	RepeatFindingLimit   uint32 `json:"repeat_finding_limit"`
	RepeatRejectionLimit uint32 `json:"repeat_rejection_limit"`
}

func (c RubricTerminationCeilingsV1) validate() error {
	if c.MaxRounds == 0 || c.MaxRounds > 32 || c.MaxDurationNanos <= 0 || c.MaxDurationNanos > int64(24*time.Hour) || c.MaxTokens == 0 || c.MaxTokens > 10_000_000 || c.RepeatFindingLimit == 0 || c.RepeatFindingLimit > c.MaxRounds || c.RepeatRejectionLimit == 0 || c.RepeatRejectionLimit > c.MaxRounds {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review rubric termination ceilings are outside their bounds")
	}
	return nil
}

// RubricDefinitionV1 is the Review Owner's immutable, append-only definition.
// Policy may select its exact ref; it cannot redefine the criteria, rules,
// output schema, capabilities or termination ceiling.
type RubricDefinitionV1 struct {
	FactIdentityV1
	Kind                        RubricKindV1                 `json:"kind"`
	Name                        string                       `json:"name"`
	Criteria                    []RubricCriterionV1          `json:"criteria"`
	Rules                       []RubricRuleV1               `json:"rules"`
	OutputSchema                RubricOutputSchemaV1         `json:"output_schema"`
	AllowedReadOnlyCapabilities []RubricReadOnlyCapabilityV1 `json:"allowed_readonly_capabilities"`
	Termination                 RubricTerminationCeilingsV1  `json:"termination"`
	State                       RubricStateV1                `json:"state"`
	ExpiresUnixNano             int64                        `json:"expires_unix_nano"`
}

func (r RubricDefinitionV1) ExactRef() ExactResourceRefV1 {
	return ExactResourceRefV1{ID: r.ID, Revision: r.Revision, Digest: r.Digest}
}

func (r RubricDefinitionV1) digestValue() RubricDefinitionV1 { r.Digest = ""; return r }

func (r RubricDefinitionV1) validateShape() error {
	if err := r.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	switch r.Kind {
	case RubricActionSafetyV1, RubricCodeChangeV1, RubricWorkStateV1, RubricArtifactQualityV1, RubricOutcomeAcceptanceV1, RubricLegalComplianceV1, RubricFinanceControlV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review rubric kind is unsupported")
	}
	if invalidText(r.Name) || len(r.Criteria) == 0 || len(r.Criteria) > MaxListItemsV1 || len(r.Rules) == 0 || len(r.Rules) > MaxListItemsV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review rubric criteria or rules are empty or exceed their bounds")
	}
	criteria := make(map[string]struct{}, len(r.Criteria))
	for i, value := range r.Criteria {
		if err := value.validate(); err != nil {
			return err
		}
		if i > 0 && r.Criteria[i-1].ID >= value.ID {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review rubric criteria must be sorted and unique")
		}
		criteria[value.ID] = struct{}{}
	}
	for i, value := range r.Rules {
		if err := value.validate(); err != nil {
			return err
		}
		if _, ok := criteria[value.CriterionID]; !ok {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "review rubric rule references an unknown criterion")
		}
		if i > 0 && r.Rules[i-1].ID >= value.ID {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review rubric rules must be sorted and unique")
		}
	}
	if err := r.OutputSchema.validate(); err != nil {
		return err
	}
	if len(r.AllowedReadOnlyCapabilities) == 0 || len(r.AllowedReadOnlyCapabilities) > MaxListItemsV1 || !sort.SliceIsSorted(r.AllowedReadOnlyCapabilities, func(i, j int) bool { return r.AllowedReadOnlyCapabilities[i] < r.AllowedReadOnlyCapabilities[j] }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review rubric readonly capabilities must be bounded and sorted")
	}
	allowedCapability := map[RubricReadOnlyCapabilityV1]bool{
		RubricReadTargetV1: true, RubricReadArtifactV1: true, RubricReadDiffV1: true, RubricReadEvidenceV1: true,
		RubricReadSourceV1: true, RubricReadTestResultV1: true, RubricReadPolicyV1: true, RubricReadOrganizationV1: true,
	}
	for i, value := range r.AllowedReadOnlyCapabilities {
		if !allowedCapability[value] || (i > 0 && r.AllowedReadOnlyCapabilities[i-1] == value) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review rubric readonly capabilities are invalid or duplicated")
		}
	}
	if err := r.Termination.validate(); err != nil {
		return err
	}
	if r.State != RubricActiveV1 && r.State != RubricRevokedV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review rubric state is unsupported")
	}
	return ValidateExpires(r.CreatedUnixNano, r.ExpiresUnixNano)
}

func SealRubricDefinitionV1(r RubricDefinitionV1) (RubricDefinitionV1, error) {
	r.ContractVersion = ContractVersionV1
	r.Digest = ""
	if err := r.validateShape(); err != nil {
		return RubricDefinitionV1{}, err
	}
	digest, err := seal("RubricDefinitionV1", r.digestValue())
	if err != nil {
		return RubricDefinitionV1{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

func (r RubricDefinitionV1) Validate() error {
	if err := r.validateShape(); err != nil {
		return err
	}
	return validateSealed("RubricDefinitionV1", r.digestValue(), r.Digest)
}

func (r RubricDefinitionV1) ValidateCurrent(expected ExactResourceRefV1, now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if r.ExactRef() != expected {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review rubric exact current ref drifted")
	}
	if r.State != RubricActiveV1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review rubric is not active")
	}
	if now.IsZero() || now.UnixNano() < r.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review rubric currentness clock predates its revision")
	}
	return ValidateNow(now, r.CreatedUnixNano, r.ExpiresUnixNano)
}

// ValidateAttestationOutputV1 applies the sealed output schema and criterion
// evidence requirements to one structured Reviewer output. It never executes
// a rule or upgrades the Attestation into a Verdict.
func (r RubricDefinitionV1) ValidateAttestationOutputV1(attestation AttestationV1, findings []FindingV1) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := attestation.Validate(); err != nil {
		return err
	}
	allowed := false
	for _, resolution := range r.OutputSchema.AllowedResolutions {
		if resolution == attestation.Resolution {
			allowed = true
			break
		}
	}
	if !allowed {
		return core.NewError(core.ErrorForbidden, core.ReasonInvalidState, "review Attestation resolution is not allowed by the exact Rubric")
	}
	if len(findings) > int(r.OutputSchema.MaxFindings) || len(attestation.FindingRefs) != len(findings) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review Attestation Finding set does not match the Rubric output schema")
	}
	findingIDs := make(map[string]struct{}, len(findings))
	evidenceKinds := make(map[runtimeports.NamespacedNameV2]struct{}, len(attestation.Evidence))
	for _, evidence := range attestation.Evidence {
		evidenceKinds[evidence.Classification] = struct{}{}
	}
	for _, finding := range findings {
		if err := finding.Validate(); err != nil {
			return err
		}
		if finding.TenantID != attestation.TenantID || finding.CaseID != attestation.CaseID || finding.CaseRevision != attestation.CaseRevision || finding.RoundID != attestation.RoundID || finding.RoundRevision != attestation.RoundRevision || finding.RoundDigest != attestation.RoundDigest || finding.TargetID != attestation.TargetID || finding.TargetRevision != attestation.TargetRevision || finding.TargetDigest != attestation.TargetDigest {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "review Rubric output Finding drifted from the exact Attestation")
		}
		findingIDs[finding.ID] = struct{}{}
		for _, evidence := range finding.Evidence {
			evidenceKinds[evidence.Classification] = struct{}{}
		}
	}
	for _, ref := range attestation.FindingRefs {
		if _, ok := findingIDs[ref]; !ok {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review Attestation references a Finding outside the Rubric output set")
		}
	}
	for _, criterion := range r.Criteria {
		for _, required := range criterion.RequiredEvidenceKinds {
			if _, ok := evidenceKinds[required]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "review Attestation lacks a Rubric-required Evidence kind")
			}
		}
	}
	return nil
}

// NewBaselineRubricDefinitionV1 constructs one bounded Review-owned protocol.
// The result is structured rules and data, never a universal prompt.
func NewBaselineRubricDefinitionV1(identity FactIdentityV1, kind RubricKindV1, expiresUnixNano int64) (RubricDefinitionV1, error) {
	definition := RubricDefinitionV1{
		FactIdentityV1: identity,
		Kind:           kind,
		OutputSchema: RubricOutputSchemaV1{
			SchemaID:              "praxis.review/attestation-v1",
			AllowedResolutions:    []ResolutionV1{ResolutionAcceptV1, ResolutionConditionalV1, ResolutionEscalateHumanV1, ResolutionInsufficientEvidenceV1, ResolutionRejectV1, ResolutionRequestChangesV1},
			RequiredFindingFields: []string{"anchor", "claim", "evidence", "impact", "priority"},
			MaxFindings:           64, RequireReasonCodes: true, RequireEvidenceRefs: true, RequireConditionsDigestForConditional: true,
		},
		Termination: RubricTerminationCeilingsV1{MaxRounds: 3, MaxDurationNanos: int64(10 * time.Minute), MaxTokens: 64_000, RepeatFindingLimit: 2, RepeatRejectionLimit: 2},
		State:       RubricActiveV1, ExpiresUnixNano: expiresUnixNano,
	}
	criterion := func(id, title, objective string, evidence runtimeports.NamespacedNameV2, failure ResolutionV1) RubricCriterionV1 {
		return RubricCriterionV1{ID: id, Title: title, Objective: objective, Priority: "blocking", RequiredEvidenceKinds: []runtimeports.NamespacedNameV2{evidence}, FailureResolution: failure}
	}
	rule := func(id, criterionID string, kind RubricRuleKindV1, subject, expected string) RubricRuleV1 {
		return RubricRuleV1{ID: id, CriterionID: criterionID, Kind: kind, Subject: subject, Expected: expected, Required: true}
	}
	switch kind {
	case RubricActionSafetyV1:
		definition.Name = "Action safety"
		definition.Criteria = []RubricCriterionV1{
			criterion("authority", "Authority", "Verify the exact action is admitted by current authority.", "review.authority/current", ResolutionRejectV1),
			criterion("scope", "Scope", "Verify the exact action remains inside the current execution scope.", "review.scope/current", ResolutionRejectV1),
		}
		definition.Rules = []RubricRuleV1{
			rule("authority-exact", "authority", RubricRuleRequireExactMatchV1, "target.actor_authority", "current.authority"),
			rule("scope-contained", "scope", RubricRuleRequireScopeContainmentV1, "target.action_scope", "current.scope"),
		}
		definition.AllowedReadOnlyCapabilities = []RubricReadOnlyCapabilityV1{RubricReadEvidenceV1, RubricReadPolicyV1, RubricReadTargetV1}
	case RubricCodeChangeV1:
		definition.Name = "Code change"
		definition.Criteria = []RubricCriterionV1{
			criterion("correctness", "Correctness", "Find concrete defects introduced by the exact change.", "review.code/diff", ResolutionRequestChangesV1),
			criterion("verification", "Verification", "Require relevant tests or an explicit evidence gap.", "review.test/result", ResolutionInsufficientEvidenceV1),
		}
		definition.Rules = []RubricRuleV1{
			rule("diff-grounded", "correctness", RubricRuleRequireEvidenceV1, "finding.anchor", "exact.diff.location"),
			rule("tests-required", "verification", RubricRuleRequireTestSignalV1, "changed.behavior", "relevant.test.result"),
		}
		definition.AllowedReadOnlyCapabilities = []RubricReadOnlyCapabilityV1{RubricReadDiffV1, RubricReadEvidenceV1, RubricReadSourceV1, RubricReadTestResultV1}
	case RubricWorkStateV1:
		definition.Name = "Work state"
		definition.Criteria = []RubricCriterionV1{
			criterion("intent-fidelity", "Intent fidelity", "Verify current work remains faithful to the frozen human intent.", "review.work-state/trace", ResolutionRequestChangesV1),
			criterion("unresolved-risk", "Unresolved risk", "Surface unresolved blockers and unsupported claims.", "review.work-state/evidence", ResolutionEscalateHumanV1),
		}
		definition.Rules = []RubricRuleV1{
			rule("intent-exact", "intent-fidelity", RubricRuleRequireExactMatchV1, "work_state.intent_digest", "target.intent_digest"),
			rule("risk-grounded", "unresolved-risk", RubricRuleRequireEvidenceV1, "work_state.risk", "exact.evidence"),
		}
		definition.AllowedReadOnlyCapabilities = []RubricReadOnlyCapabilityV1{RubricReadEvidenceV1, RubricReadSourceV1, RubricReadTargetV1}
	case RubricArtifactQualityV1:
		definition.Name = "Artifact quality"
		definition.Criteria = []RubricCriterionV1{
			criterion("artifact-integrity", "Artifact integrity", "Verify the exact artifact revision is readable and internally coherent.", "review.artifact/current", ResolutionRequestChangesV1),
			criterion("quality-evidence", "Quality evidence", "Require evidence for declared artifact quality checks.", "review.artifact/validation", ResolutionInsufficientEvidenceV1),
		}
		definition.Rules = []RubricRuleV1{
			rule("artifact-exact", "artifact-integrity", RubricRuleRequireExactMatchV1, "artifact.ref", "target.artifact_ref"),
			rule("quality-grounded", "quality-evidence", RubricRuleRequireEvidenceV1, "artifact.claim", "validation.evidence"),
		}
		definition.AllowedReadOnlyCapabilities = []RubricReadOnlyCapabilityV1{RubricReadArtifactV1, RubricReadEvidenceV1, RubricReadSourceV1}
	case RubricOutcomeAcceptanceV1:
		definition.Name = "Outcome acceptance"
		definition.Criteria = []RubricCriterionV1{
			criterion("acceptance-coverage", "Acceptance coverage", "Map every required acceptance criterion to the exact outcome.", "review.outcome/acceptance", ResolutionRequestChangesV1),
			criterion("claim-grounding", "Claim grounding", "Require exact evidence for each material outcome claim.", "review.outcome/evidence", ResolutionInsufficientEvidenceV1),
		}
		definition.Rules = []RubricRuleV1{
			rule("claims-grounded", "claim-grounding", RubricRuleRequireEvidenceV1, "outcome.claim", "exact.evidence"),
			rule("criteria-covered", "acceptance-coverage", RubricRuleRequireClaimCoverageV1, "acceptance.criteria", "outcome.claims"),
		}
		definition.AllowedReadOnlyCapabilities = []RubricReadOnlyCapabilityV1{RubricReadArtifactV1, RubricReadEvidenceV1, RubricReadTargetV1, RubricReadTestResultV1}
	case RubricLegalComplianceV1:
		definition.Name = "Legal compliance"
		definition.Criteria = []RubricCriterionV1{
			criterion("legal-authority", "Legal authority", "Require the current accountable legal authority for the exact scope.", "review.organization/authority", ResolutionEscalateHumanV1),
			criterion("source-attribution", "Source attribution", "Require attributable sources for material legal claims.", "review.legal/source", ResolutionInsufficientEvidenceV1),
		}
		definition.Rules = []RubricRuleV1{
			rule("legal-approval", "legal-authority", RubricRuleRequireApprovalV1, "legal.scope", "current.accountable_authority"),
			rule("legal-sources", "source-attribution", RubricRuleRequireAttributionV1, "legal.claim", "exact.source"),
		}
		definition.AllowedReadOnlyCapabilities = []RubricReadOnlyCapabilityV1{RubricReadEvidenceV1, RubricReadOrganizationV1, RubricReadPolicyV1, RubricReadSourceV1}
	case RubricFinanceControlV1:
		definition.Name = "Finance control"
		definition.Criteria = []RubricCriterionV1{
			criterion("financial-authority", "Financial authority", "Require current accountable authority for the exact financial scope.", "review.organization/authority", ResolutionRejectV1),
			criterion("financial-evidence", "Financial evidence", "Require exact source and calculation evidence for material financial claims.", "review.finance/evidence", ResolutionInsufficientEvidenceV1),
		}
		definition.Rules = []RubricRuleV1{
			rule("finance-approval", "financial-authority", RubricRuleRequireApprovalV1, "finance.scope", "current.accountable_authority"),
			rule("finance-grounding", "financial-evidence", RubricRuleRequireEvidenceV1, "finance.claim", "exact.source_and_calculation"),
		}
		definition.AllowedReadOnlyCapabilities = []RubricReadOnlyCapabilityV1{RubricReadEvidenceV1, RubricReadOrganizationV1, RubricReadPolicyV1, RubricReadSourceV1}
	default:
		return RubricDefinitionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review baseline rubric kind is unsupported")
	}
	return SealRubricDefinitionV1(definition)
}
