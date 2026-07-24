package conformance

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationDispatchGovernanceCaseV4 struct {
	Gateway           ports.OperationGovernancePortV4
	Issue             ports.IssueGovernedOperationDispatchRequestV4
	V4ThenV3Isolation OperationDispatchCrossVersionCaseV4
	V3ThenV4Isolation OperationDispatchCrossVersionCaseV4
}

// OperationDispatchCrossVersionCaseV4 must use one isolated underlying Fact
// Owner for both public Gateways. The conformance check intentionally mutates
// this fixture, so it must never point at production Effects.
type OperationDispatchCrossVersionCaseV4 struct {
	V3         ports.OperationGovernancePortV3
	V4         ports.OperationGovernancePortV4
	Admissions ports.OperationEffectAdmissionPortV3
	V3Issue    ports.IssueGovernedOperationDispatchRequestV3
	V4Issue    ports.IssueGovernedOperationDispatchRequestV4
}

type OperationDispatchGovernanceReportV4 struct {
	AtomicIssueObserved       bool `json:"atomic_issue_observed"`
	HistoricalInspectObserved bool `json:"historical_inspect_observed"`
	CurrentInspectObserved    bool `json:"current_inspect_observed"`
	BeginObserved             bool `json:"begin_observed"`
	HistoricalRecordExecutes  bool `json:"historical_record_executes"`
	BeginIsFinalExecution     bool `json:"begin_is_final_execution"`
	V3MasqueradesAsV4         bool `json:"v3_masquerades_as_v4"`
	V4ThenV3ConflictObserved  bool `json:"v4_then_v3_conflict_observed"`
	V3ThenV4ConflictObserved  bool `json:"v3_then_v4_conflict_observed"`
	CrossVersionSecondWrite   bool `json:"cross_version_second_write"`
	CrossVersionEffectAtomic  bool `json:"cross_version_effect_atomic"`
	ProductionClaimEligible   bool `json:"production_claim_eligible"`
}

func CheckOperationDispatchGovernanceV4(ctx context.Context, testCase OperationDispatchGovernanceCaseV4) (OperationDispatchGovernanceReportV4, error) {
	if testCase.Gateway == nil {
		return OperationDispatchGovernanceReportV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V4 operation governance Port is required")
	}
	if err := testCase.Issue.Validate(); err != nil {
		return OperationDispatchGovernanceReportV4{}, err
	}
	if err := validateOperationDispatchCrossVersionCaseV4(testCase.V4ThenV3Isolation); err != nil {
		return OperationDispatchGovernanceReportV4{}, err
	}
	if err := validateOperationDispatchCrossVersionCaseV4(testCase.V3ThenV4Isolation); err != nil {
		return OperationDispatchGovernanceReportV4{}, err
	}
	issued, err := testCase.Gateway.IssueOperationDispatchV4(ctx, testCase.Issue)
	if err != nil {
		return OperationDispatchGovernanceReportV4{}, err
	}
	if err := issued.Validate(); err != nil {
		return OperationDispatchGovernanceReportV4{}, err
	}
	inspect := ports.InspectOperationDispatchRecordRequestV4{Operation: testCase.Issue.Operation, EffectID: testCase.Issue.EffectID, PermitID: testCase.Issue.PermitID}
	historical, err := testCase.Gateway.InspectOperationDispatchRecordV4(ctx, inspect)
	if err != nil {
		return OperationDispatchGovernanceReportV4{}, err
	}
	if historical.Digest != issued.Record.Digest {
		return OperationDispatchGovernanceReportV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 historical Inspect returned another record")
	}
	current, err := testCase.Gateway.InspectCurrentOperationDispatchV4(ctx, ports.InspectCurrentOperationDispatchRequestV4{Inspect: inspect, AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: issued.ReviewAuthorization})
	if err != nil {
		return OperationDispatchGovernanceReportV4{}, err
	}
	if current.Record.Digest != issued.Record.Digest {
		return OperationDispatchGovernanceReportV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 current Inspect returned another record")
	}
	begun, err := testCase.Gateway.BeginOperationDispatchV4(ctx, ports.BeginGovernedOperationDispatchRequestV4{
		Operation: testCase.Issue.Operation, EffectID: testCase.Issue.EffectID,
		ExpectedEffectRevision: issued.Record.EffectFactRevision, PermitID: testCase.Issue.PermitID,
		ExpectedPermitFactRevision: issued.Record.Revision, AdmissionDigest: issued.Record.Permit.Admission.Digest,
		ReviewAuthorization: issued.ReviewAuthorization,
	})
	if err != nil {
		return OperationDispatchGovernanceReportV4{}, err
	}
	if begun.Record.State != ports.OperationPermitBegunV4 {
		return OperationDispatchGovernanceReportV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V4 conformance Begin did not persist begun state")
	}
	v4ThenV3Conflict, v4ThenV3SecondWrite, v4ThenV3Atomic, err := checkV4ThenV3Isolation(ctx, testCase.V4ThenV3Isolation)
	if err != nil {
		return OperationDispatchGovernanceReportV4{}, err
	}
	v3ThenV4Conflict, v3ThenV4SecondWrite, v3ThenV4Atomic, err := checkV3ThenV4Isolation(ctx, testCase.V3ThenV4Isolation)
	if err != nil {
		return OperationDispatchGovernanceReportV4{}, err
	}
	secondWrite := v4ThenV3SecondWrite || v3ThenV4SecondWrite
	effectAtomic := v4ThenV3Atomic && v3ThenV4Atomic
	masquerades := !v4ThenV3Conflict || !v3ThenV4Conflict || secondWrite || !effectAtomic
	return OperationDispatchGovernanceReportV4{
		AtomicIssueObserved: true, HistoricalInspectObserved: true, CurrentInspectObserved: true, BeginObserved: true,
		// Contract/fake tests never turn historical records or host Begin into
		// Provider execution authority, and never make production claims.
		HistoricalRecordExecutes: false, BeginIsFinalExecution: false,
		V3MasqueradesAsV4: masquerades, V4ThenV3ConflictObserved: v4ThenV3Conflict,
		V3ThenV4ConflictObserved: v3ThenV4Conflict, CrossVersionSecondWrite: secondWrite,
		CrossVersionEffectAtomic: effectAtomic,
		ProductionClaimEligible:  false,
	}, nil
}

func validateOperationDispatchCrossVersionCaseV4(testCase OperationDispatchCrossVersionCaseV4) error {
	if testCase.V3 == nil || testCase.V4 == nil || testCase.Admissions == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "isolated V3, V4 and accepted Effect inspection Ports are required")
	}
	if err := testCase.V3Issue.Operation.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(string(testCase.V3Issue.EffectID)) == "" || testCase.V3Issue.ExpectedEffectRevision == 0 || strings.TrimSpace(testCase.V3Issue.PermitID) == "" || strings.TrimSpace(testCase.V3Issue.AttemptID) == "" || testCase.V3Issue.PermitTTL <= 0 || testCase.V3Issue.PermitTTL > ports.MaxDispatchPermitTTL {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "cross-version V3 Issue request is incomplete")
	}
	if err := testCase.V4Issue.Validate(); err != nil {
		return err
	}
	if testCase.V3Issue.PermitID != testCase.V4Issue.PermitID || testCase.V3Issue.EffectID == testCase.V4Issue.EffectID || !ports.SameOperationSubjectV3(testCase.V3Issue.Operation, testCase.V4Issue.Operation) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "cross-version conformance requires distinct accepted Effects sharing one Operation partition and Permit ID")
	}
	return nil
}

func checkV4ThenV3Isolation(ctx context.Context, testCase OperationDispatchCrossVersionCaseV4) (bool, bool, bool, error) {
	issued, err := testCase.V4.IssueOperationDispatchV4(ctx, testCase.V4Issue)
	if err != nil {
		return false, false, false, err
	}
	if err := issued.Validate(); err != nil {
		return false, false, false, err
	}
	if issued.Record.EffectFactRevision <= testCase.V4Issue.ExpectedEffectRevision || issued.Record.EffectFactRevision-testCase.V4Issue.ExpectedEffectRevision != 1 {
		return false, true, false, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "first V4 Effect did not atomically advance to dispatch intent")
	}
	_, conflict := testCase.V3.IssueOperationDispatchV3(ctx, testCase.V3Issue)
	conflictObserved := core.HasCategory(conflict, core.ErrorConflict) && core.HasReason(conflict, core.ReasonIdempotencyPayloadMismatch)
	if !conflictObserved {
		return false, conflict == nil, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V3 Issue did not reject a Permit ID already owned by V4")
	}
	historical, err := testCase.V4.InspectOperationDispatchRecordV4(ctx, ports.InspectOperationDispatchRecordRequestV4{Operation: testCase.V4Issue.Operation, EffectID: testCase.V4Issue.EffectID, PermitID: testCase.V4Issue.PermitID})
	if err != nil || historical.Digest != issued.Record.Digest {
		return true, true, false, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "failed V3 Issue changed the existing V4 record")
	}
	_, inspectErr := testCase.V3.InspectOperationDispatchAuthorizationV3(ctx, testCase.V3Issue.Operation, testCase.V3Issue.EffectID, testCase.V3Issue.PermitID)
	if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return true, true, false, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "failed V3 Issue left a V3 Permit record")
	}
	accepted, err := testCase.Admissions.InspectAcceptedOperationEffectV3(ctx, testCase.V3Issue.Operation, testCase.V3Issue.EffectID)
	if err != nil || accepted.FactRevision != testCase.V3Issue.ExpectedEffectRevision {
		return true, true, false, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "failed V3 Issue changed the second accepted Effect")
	}
	return true, false, true, nil
}

func checkV3ThenV4Isolation(ctx context.Context, testCase OperationDispatchCrossVersionCaseV4) (bool, bool, bool, error) {
	issued, err := testCase.V3.IssueOperationDispatchV3(ctx, testCase.V3Issue)
	if err != nil {
		return false, false, false, err
	}
	if err := issued.Validate(); err != nil {
		return false, false, false, err
	}
	if issued.EffectFactRevision <= testCase.V3Issue.ExpectedEffectRevision || issued.EffectFactRevision-testCase.V3Issue.ExpectedEffectRevision != 1 {
		return false, true, false, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "first V3 Effect did not atomically advance to dispatch intent")
	}
	_, conflict := testCase.V4.IssueOperationDispatchV4(ctx, testCase.V4Issue)
	conflictObserved := core.HasCategory(conflict, core.ErrorConflict) && core.HasReason(conflict, core.ReasonIdempotencyPayloadMismatch)
	if !conflictObserved {
		return false, conflict == nil, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V4 Issue did not reject a Permit ID already owned by V3")
	}
	current, err := testCase.V3.InspectOperationDispatchAuthorizationV3(ctx, testCase.V3Issue.Operation, testCase.V3Issue.EffectID, testCase.V3Issue.PermitID)
	if err != nil || current.Attempt != issued.Attempt || current.PermitFactRevision != issued.PermitFactRevision || current.EffectFactRevision != issued.EffectFactRevision {
		return true, true, false, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "failed V4 Issue changed the existing V3 authorization")
	}
	_, inspectErr := testCase.V4.InspectOperationDispatchRecordV4(ctx, ports.InspectOperationDispatchRecordRequestV4{Operation: testCase.V4Issue.Operation, EffectID: testCase.V4Issue.EffectID, PermitID: testCase.V4Issue.PermitID})
	if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return true, true, false, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "failed V4 Issue left a V4 Permit record")
	}
	accepted, err := testCase.Admissions.InspectAcceptedOperationEffectV3(ctx, testCase.V4Issue.Operation, testCase.V4Issue.EffectID)
	if err != nil || accepted != testCase.V4Issue.Admission {
		return true, true, false, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "failed V4 Issue changed the second accepted Effect")
	}
	return true, false, true, nil
}
