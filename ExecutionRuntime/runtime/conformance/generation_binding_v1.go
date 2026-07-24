package conformance

import (
	"context"
	"reflect"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// GenerationBindingAssociationCurrentReaderCaseV1 proves the capability-
// narrowed public read surface independently from Associate authority.
type GenerationBindingAssociationCurrentReaderCaseV1 struct {
	Reader        ports.GenerationBindingAssociationCurrentReaderV1
	AssociationID string
}

type GenerationBindingAssociationCurrentReaderReportV1 struct {
	CurrentInspectObserved  bool `json:"current_inspect_observed"`
	AssociateAuthorityUsed  bool `json:"associate_authority_used"`
	ProductionClaimEligible bool `json:"production_claim_eligible"`
}

func CheckGenerationBindingAssociationCurrentReaderV1(ctx context.Context, testCase GenerationBindingAssociationCurrentReaderCaseV1) (GenerationBindingAssociationCurrentReaderReportV1, error) {
	if generationBindingReaderNilV1(testCase.Reader) {
		return GenerationBindingAssociationCurrentReaderReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "generation association current Reader is required")
	}
	if strings.TrimSpace(testCase.AssociationID) == "" {
		return GenerationBindingAssociationCurrentReaderReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "generation association ID is required")
	}
	fact, err := testCase.Reader.InspectCurrentGenerationBindingAssociationV1(ctx, testCase.AssociationID)
	if err != nil {
		return GenerationBindingAssociationCurrentReaderReportV1{}, err
	}
	if err := fact.Validate(); err != nil {
		return GenerationBindingAssociationCurrentReaderReportV1{}, err
	}
	if fact.ID != testCase.AssociationID {
		return GenerationBindingAssociationCurrentReaderReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "generation association current Reader returned another association")
	}
	return GenerationBindingAssociationCurrentReaderReportV1{
		CurrentInspectObserved: true,
		// The conformance function is statically typed to the Reader and has no
		// mutation method to call.
		AssociateAuthorityUsed:  false,
		ProductionClaimEligible: false,
	}, nil
}

func generationBindingReaderNilV1(reader ports.GenerationBindingAssociationCurrentReaderV1) bool {
	if reader == nil {
		return true
	}
	value := reflect.ValueOf(reader)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// GenerationBindingAssociationCaseV1 exercises the public governance seam.
// Candidates remain non-authoritative until the Runtime-owned Port returns a
// Fact. The case must use an isolated test scope.
type GenerationBindingAssociationCaseV1 struct {
	Gateway   ports.GenerationBindingAssociationGovernancePortV1
	Candidate ports.GenerationBindingAssociationCandidateV1
}

type GenerationBindingAssociationReportV1 struct {
	RuntimeFactOwnerObserved bool `json:"runtime_fact_owner_observed"`
	CurrentInspectObserved   bool `json:"current_inspect_observed"`
	CandidateIsBindingFact   bool `json:"candidate_is_binding_fact"`
	ProductionClaimEligible  bool `json:"production_claim_eligible"`
}

func CheckGenerationBindingAssociationV1(ctx context.Context, testCase GenerationBindingAssociationCaseV1) (GenerationBindingAssociationReportV1, error) {
	if testCase.Gateway == nil {
		return GenerationBindingAssociationReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "generation association governance Port is required")
	}
	if err := testCase.Candidate.Validate(); err != nil {
		return GenerationBindingAssociationReportV1{}, err
	}
	fact, err := testCase.Gateway.AssociateGenerationBindingV1(ctx, testCase.Candidate)
	if err != nil {
		return GenerationBindingAssociationReportV1{}, err
	}
	if err := fact.Validate(); err != nil {
		return GenerationBindingAssociationReportV1{}, err
	}
	current, err := testCase.Gateway.InspectCurrentGenerationBindingAssociationV1(ctx, fact.ID)
	if err != nil {
		return GenerationBindingAssociationReportV1{}, err
	}
	if current.Digest != fact.Digest || fact.CandidateDigest != testCase.Candidate.Digest {
		return GenerationBindingAssociationReportV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "generation association conformance read different authority")
	}
	return GenerationBindingAssociationReportV1{
		RuntimeFactOwnerObserved: true,
		CurrentInspectObserved:   true,
		CandidateIsBindingFact:   false,
		// This public testkit certifies contract behavior only. It never claims
		// a production backend, durability, process topology, SLA or Binding.
		ProductionClaimEligible: false,
	}, nil
}
