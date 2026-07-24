// Package consolidation defines a replayable, backend-neutral Memory
// Consolidator. Its output is proposal material only; the Memory Owner remains
// the sole creator of Candidate, Admission, Record, and Commit facts.
package consolidation

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

const (
	ContractVersionV1 = "praxis.memory/consolidation/v1"
	InputObjectKindV1 = "memory_consolidation_input"
	BatchObjectKindV1 = "memory_consolidation_batch"
)

type InputKind string

const (
	InputTimeline      InputKind = "timeline"
	InputOutcome       InputKind = "outcome"
	InputReviewFinding InputKind = "review_finding"
	InputArtifact      InputKind = "artifact"
	InputToolResult    InputKind = "tool_result"
)

type InputFactV1 struct {
	Kind          InputKind      `json:"kind"`
	FactRef       contract.Ref   `json:"fact_ref"`
	SettlementRef contract.Ref   `json:"settlement_ref"`
	EvidenceRefs  []contract.Ref `json:"evidence_refs"`
}

type InputV1 struct {
	ContractVersion  string        `json:"contract_version"`
	ObjectKind       string        `json:"object_kind"`
	Ref              contract.Ref  `json:"ref"`
	TenantID         string        `json:"tenant_id"`
	ScopeRef         contract.Ref  `json:"scope_ref"`
	PolicyRef        contract.Ref  `json:"policy_ref"`
	TimelineStartRef contract.Ref  `json:"timeline_start_ref"`
	TimelineEndRef   contract.Ref  `json:"timeline_end_ref"`
	Facts            []InputFactV1 `json:"facts"`
	RuleRef          contract.Ref  `json:"rule_ref"`
	ModelRouteRef    contract.Ref  `json:"model_route_ref,omitempty"`
	InputDigest      string        `json:"input_digest"`
	CreatedAt        time.Time     `json:"created_at"`
	ExpiresAt        time.Time     `json:"expires_at"`
	Digest           string        `json:"digest"`
}

type Decision string

const (
	DecisionSubmitCandidate Decision = "submit_candidate"
	DecisionReviewRequired  Decision = "review_required"
	DecisionRejected        Decision = "rejected"
)

type ProposalV1 struct {
	ID           string              `json:"id"`
	Subject      string              `json:"subject"`
	Scope        string              `json:"scope"`
	ContentRef   contract.ContentRef `json:"content_ref"`
	SourceRefs   []contract.Ref      `json:"source_refs"`
	EvidenceRefs []contract.Ref      `json:"evidence_refs"`
	Sensitivity  string              `json:"sensitivity"`
	FutureUse    string              `json:"future_use"`
	Verifiable   bool                `json:"verifiable"`
	RiskFlags    []string            `json:"risk_flags"`
	Decision     Decision            `json:"decision"`
	Reason       string              `json:"reason"`
}

type BatchV1 struct {
	ContractVersion string               `json:"contract_version"`
	ObjectKind      string               `json:"object_kind"`
	Ref             contract.Ref         `json:"ref"`
	Owner           contract.OwnerDomain `json:"owner"`
	InputRef        contract.Ref         `json:"input_ref"`
	JobAttemptRef   contract.Ref         `json:"job_attempt_ref"`
	Proposals       []ProposalV1         `json:"proposals"`
	CreatedAt       time.Time            `json:"created_at"`
	ExpiresAt       time.Time            `json:"expires_at"`
	Digest          string               `json:"digest"`
}

func SealInputV1(in InputV1) (InputV1, error) {
	in.ContractVersion, in.ObjectKind = ContractVersionV1, InputObjectKindV1
	in.Facts = normalizeFacts(in.Facts)
	in.CreatedAt, in.ExpiresAt = in.CreatedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	d, e := contract.Digest(in)
	if e != nil {
		return InputV1{}, e
	}
	in.Ref.Digest, in.Digest = d, d
	if e = in.Validate(in.CreatedAt); e != nil {
		return InputV1{}, e
	}
	return in, nil
}
func (in InputV1) Validate(now time.Time) error {
	if in.ContractVersion != ContractVersionV1 || in.ObjectKind != InputObjectKindV1 || in.Ref.Validate() != nil || strings.TrimSpace(in.TenantID) == "" || in.ScopeRef.Validate() != nil || in.PolicyRef.Validate() != nil || in.TimelineStartRef.Validate() != nil || in.TimelineEndRef.Validate() != nil || in.RuleRef.Validate() != nil || strings.TrimSpace(in.InputDigest) == "" || len(in.Facts) == 0 || in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: consolidation input", contract.ErrInvalidArgument)
	}
	if in.ModelRouteRef != (contract.Ref{}) && in.ModelRouteRef.Validate() != nil {
		return contract.ErrInvalidArgument
	}
	canonical := normalizeFacts(in.Facts)
	if !slices.EqualFunc(canonical, in.Facts, equalInputFact) {
		return fmt.Errorf("%w: consolidation facts", contract.ErrInvalidArgument)
	}
	seen := map[string]struct{}{}
	for _, fact := range in.Facts {
		if !validInputKind(fact.Kind) || fact.FactRef.Validate() != nil || fact.SettlementRef.Validate() != nil {
			return fmt.Errorf("%w: unsettled input", contract.ErrInvalidArgument)
		}
		key := string(fact.Kind) + ":" + fact.FactRef.ID
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: duplicate input", contract.ErrEvidenceConflict)
		}
		seen[key] = struct{}{}
		for _, r := range fact.EvidenceRefs {
			if r.Validate() != nil {
				return contract.ErrInvalidArgument
			}
		}
	}
	c := in
	c.Ref.Digest, c.Digest = "", ""
	d, e := contract.Digest(c)
	if e != nil {
		return e
	}
	if d != in.Digest || in.Ref.Digest != d {
		return fmt.Errorf("%w: consolidation input digest", contract.ErrEvidenceConflict)
	}
	return nil
}
func SealBatchV1(in BatchV1) (BatchV1, error) {
	in.ContractVersion, in.ObjectKind = ContractVersionV1, BatchObjectKindV1
	in.Owner = contract.OwnerMemory
	in.Proposals = normalizeProposals(in.Proposals)
	in.CreatedAt, in.ExpiresAt = in.CreatedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	d, e := contract.Digest(in)
	if e != nil {
		return BatchV1{}, e
	}
	in.Ref.Digest, in.Digest = d, d
	if e = in.Validate(in.CreatedAt); e != nil {
		return BatchV1{}, e
	}
	return in, nil
}
func (in BatchV1) Validate(now time.Time) error {
	if in.ContractVersion != ContractVersionV1 || in.ObjectKind != BatchObjectKindV1 || in.Owner != contract.OwnerMemory || in.Ref.Validate() != nil || in.InputRef.Validate() != nil || in.JobAttemptRef.Validate() != nil || len(in.Proposals) == 0 || in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: consolidation batch", contract.ErrInvalidArgument)
	}
	canonical := normalizeProposals(in.Proposals)
	if !slices.EqualFunc(canonical, in.Proposals, equalProposal) {
		return fmt.Errorf("%w: proposals not canonical", contract.ErrInvalidArgument)
	}
	seen := map[string]struct{}{}
	for _, p := range in.Proposals {
		if err := validateProposal(p); err != nil {
			return err
		}
		if _, ok := seen[p.ID]; ok {
			return fmt.Errorf("%w: duplicate proposal", contract.ErrEvidenceConflict)
		}
		seen[p.ID] = struct{}{}
	}
	c := in
	c.Ref.Digest, c.Digest = "", ""
	d, e := contract.Digest(c)
	if e != nil {
		return e
	}
	if d != in.Digest || in.Ref.Digest != d {
		return fmt.Errorf("%w: consolidation batch digest", contract.ErrEvidenceConflict)
	}
	return nil
}

func validateProposal(p ProposalV1) error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Subject) == "" || strings.TrimSpace(p.Scope) == "" || p.ContentRef.Validate() != nil || len(p.SourceRefs) == 0 || strings.TrimSpace(p.Sensitivity) == "" || strings.TrimSpace(p.FutureUse) == "" || strings.TrimSpace(p.Reason) == "" || p.Decision != DecisionSubmitCandidate && p.Decision != DecisionReviewRequired && p.Decision != DecisionRejected {
		return fmt.Errorf("%w: proposal", contract.ErrInvalidArgument)
	}
	if p.Decision == DecisionSubmitCandidate && !p.Verifiable {
		return fmt.Errorf("%w: unverifiable auto candidate", contract.ErrCandidateRejected)
	}
	for _, r := range append(slices.Clone(p.SourceRefs), p.EvidenceRefs...) {
		if r.Validate() != nil {
			return contract.ErrInvalidArgument
		}
	}
	return nil
}
func normalizeFacts(in []InputFactV1) []InputFactV1 {
	out := slices.Clone(in)
	for i := range out {
		out[i].EvidenceRefs = contract.NormalizeRefs(out[i].EvidenceRefs)
	}
	slices.SortFunc(out, func(a, b InputFactV1) int {
		if c := strings.Compare(string(a.Kind), string(b.Kind)); c != 0 {
			return c
		}
		if c := strings.Compare(a.FactRef.ID, b.FactRef.ID); c != 0 {
			return c
		}
		if a.FactRef.Revision < b.FactRef.Revision {
			return -1
		}
		if a.FactRef.Revision > b.FactRef.Revision {
			return 1
		}
		return strings.Compare(a.FactRef.Digest, b.FactRef.Digest)
	})
	return out
}
func normalizeProposals(in []ProposalV1) []ProposalV1 {
	out := slices.Clone(in)
	for i := range out {
		out[i].SourceRefs = contract.NormalizeRefs(out[i].SourceRefs)
		out[i].EvidenceRefs = contract.NormalizeRefs(out[i].EvidenceRefs)
		out[i].RiskFlags = sortedUnique(out[i].RiskFlags)
	}
	slices.SortFunc(out, func(a, b ProposalV1) int { return strings.Compare(a.ID, b.ID) })
	return out
}
func sortedUnique(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	out = slices.Compact(out)
	if out == nil {
		return []string{}
	}
	return out
}
func validInputKind(k InputKind) bool {
	return k == InputTimeline || k == InputOutcome || k == InputReviewFinding || k == InputArtifact || k == InputToolResult
}
func equalInputFact(a, b InputFactV1) bool {
	return a.Kind == b.Kind && contract.SameRef(a.FactRef, b.FactRef) && contract.SameRef(a.SettlementRef, b.SettlementRef) && slices.EqualFunc(a.EvidenceRefs, b.EvidenceRefs, contract.SameRef)
}
func equalProposal(a, b ProposalV1) bool {
	da, _ := contract.Digest(a)
	db, _ := contract.Digest(b)
	return da == db
}
