package projection

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

const ContractVersionV1 = "praxis.memory-knowledge/projection-entry/v1"

type MetadataV1 struct {
	Owner         contract.OwnerDomain `json:"owner"`
	ProjectionRef contract.Ref         `json:"projection_ref"`
	RecordRef     contract.Ref         `json:"record_ref"`
	ContentRef    contract.ContentRef  `json:"content_ref"`
	SourceRefs    []contract.Ref       `json:"source_refs"`
	EvidenceRefs  []contract.Ref       `json:"evidence_refs"`
	Scope         string               `json:"scope"`
	Subject       string               `json:"subject"`
	Sensitivity   string               `json:"sensitivity"`
	ConflictGroup string               `json:"conflict_group,omitempty"`
	TrustState    string               `json:"trust_state,omitempty"`
	License       string               `json:"license,omitempty"`
	SnapshotRef   contract.Ref         `json:"snapshot_ref,omitempty"`
	PackageRef    contract.Ref         `json:"package_ref,omitempty"`
	CreatedAt     time.Time            `json:"created_at"`
	ExpiresAt     time.Time            `json:"expires_at"`
}

func NormalizeMetadataV1(in MetadataV1) MetadataV1 {
	in.SourceRefs = contract.NormalizeRefs(in.SourceRefs)
	in.EvidenceRefs = contract.NormalizeRefs(in.EvidenceRefs)
	in.CreatedAt, in.ExpiresAt = in.CreatedAt.UTC(), in.ExpiresAt.UTC()
	return in
}

func (in MetadataV1) Validate(now time.Time) error {
	if in.Owner != contract.OwnerMemory && in.Owner != contract.OwnerKnowledge || strings.TrimSpace(in.Scope) == "" || strings.TrimSpace(in.Subject) == "" || strings.TrimSpace(in.Sensitivity) == "" || in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: projection metadata", contract.ErrInvalidArgument)
	}
	if in.ProjectionRef.Validate() != nil || in.RecordRef.Validate() != nil || in.ContentRef.Validate() != nil || len(in.SourceRefs) == 0 {
		return fmt.Errorf("%w: projection exact refs", contract.ErrInvalidArgument)
	}
	if !slices.Equal(in.SourceRefs, contract.NormalizeRefs(in.SourceRefs)) || !slices.Equal(in.EvidenceRefs, contract.NormalizeRefs(in.EvidenceRefs)) {
		return fmt.Errorf("%w: noncanonical projection refs", contract.ErrInvalidArgument)
	}
	for _, ref := range append(slices.Clone(in.SourceRefs), in.EvidenceRefs...) {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func Hit(in MetadataV1, score int, reason string) contract.RetrievalHit {
	return contract.RetrievalHit{
		RecordRef: in.RecordRef, Score: score, MatchReason: reason, Scope: in.Scope, Subject: in.Subject,
		ConflictGroup: in.ConflictGroup, TrustState: in.TrustState, License: in.License, SnapshotRef: in.SnapshotRef,
		PackageRef: in.PackageRef, ProjectionRefs: []contract.Ref{in.ProjectionRef},
		Citation: contract.Citation{Domain: in.Owner, RecordRef: in.RecordRef, SourceRefs: slices.Clone(in.SourceRefs), EvidenceRefs: slices.Clone(in.EvidenceRefs), ContentRef: in.ContentRef, RangeEnd: in.ContentRef.Length, Current: true, SummaryDigest: in.ContentRef.Digest},
	}
}

func Tokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func SortedUniqueStrings(in []string) []string {
	out := slices.Clone(in)
	for i := range out {
		out[i] = strings.ToLower(strings.TrimSpace(out[i]))
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if out == nil {
		return []string{}
	}
	return out
}
