package reviewer

import (
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const contextDomainV1 = "praxis.review.reviewer-context"

type ContextFrameV1 struct {
	ContractVersion          string        `json:"contract_version"`
	TenantID                 core.TenantID `json:"tenant_id"`
	CaseID                   string        `json:"case_id"`
	RoundID                  string        `json:"round_id"`
	TargetDigest             core.Digest   `json:"target_digest"`
	OriginalIntentDigest     core.Digest   `json:"original_intent_digest"`
	StableRulesDigest        core.Digest   `json:"stable_rules_digest"`
	ConfirmedDecisionsDigest core.Digest   `json:"confirmed_decisions_digest"`
	EvidenceSetDigest        core.Digest   `json:"evidence_set_digest"`
	RubricDigest             core.Digest   `json:"rubric_digest"`
	OutputSchemaDigest       core.Digest   `json:"output_schema_digest"`
	AllowedReadCapabilities  []string      `json:"allowed_read_capabilities"`
	ReadOnly                 bool          `json:"read_only"`
	CreatedUnixNano          int64         `json:"created_unix_nano"`
	ExpiresUnixNano          int64         `json:"expires_unix_nano"`
	Digest                   core.Digest   `json:"digest"`
}

func (f ContextFrameV1) digestValue() ContextFrameV1 { f.Digest = ""; return f }

func (f ContextFrameV1) validateShape() error {
	if f.ContractVersion != contract.ContractVersionV1 || strings.TrimSpace(string(f.TenantID)) == "" || strings.TrimSpace(f.CaseID) == "" || strings.TrimSpace(f.RoundID) == "" || !f.ReadOnly || f.CreatedUnixNano <= 0 || f.ExpiresUnixNano <= f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "reviewer context must be bounded, identified and read-only")
	}
	for _, d := range []core.Digest{f.TargetDigest, f.OriginalIntentDigest, f.StableRulesDigest, f.ConfirmedDecisionsDigest, f.EvidenceSetDigest, f.RubricDigest, f.OutputSchemaDigest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	if len(f.AllowedReadCapabilities) == 0 || len(f.AllowedReadCapabilities) > contract.MaxListItemsV1 || !sort.StringsAreSorted(f.AllowedReadCapabilities) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "reviewer read capabilities must be bounded and sorted")
	}
	for i, capability := range f.AllowedReadCapabilities {
		if strings.TrimSpace(capability) == "" || strings.Contains(capability, "write") || strings.Contains(capability, "dispatch") || strings.Contains(capability, "commit") || strings.Contains(capability, "spawn") || (i > 0 && f.AllowedReadCapabilities[i-1] == capability) {
			return core.NewError(core.ErrorForbidden, core.ReasonUnknownCapability, "reviewer context contains a forbidden or duplicate capability")
		}
	}
	return nil
}

func SealContextFrameV1(f ContextFrameV1) (ContextFrameV1, error) {
	f.ContractVersion = contract.ContractVersionV1
	f.Digest = ""
	if err := f.validateShape(); err != nil {
		return ContextFrameV1{}, err
	}
	digest, err := core.CanonicalJSONDigest(contextDomainV1, contract.ContractVersionV1, "ContextFrameV1", f.digestValue())
	if err != nil {
		return ContextFrameV1{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func (f ContextFrameV1) Validate() error {
	if err := f.validateShape(); err != nil {
		return err
	}
	if err := f.Digest.Validate(); err != nil {
		return err
	}
	expected, err := core.CanonicalJSONDigest(contextDomainV1, contract.ContractVersionV1, "ContextFrameV1", f.digestValue())
	if err != nil {
		return err
	}
	if expected != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "reviewer context digest drifted")
	}
	return nil
}

func (f ContextFrameV1) ValidateCurrent(now time.Time, target core.Digest) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if target != f.TargetDigest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "reviewer context belongs to another target")
	}
	if now.IsZero() || now.UnixNano() < f.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "reviewer context clock regressed")
	}
	if now.UnixNano() >= f.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "reviewer context expired")
	}
	return nil
}
