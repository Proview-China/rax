package contract

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const IndexDescriptorObjectKindV1 = "memory_knowledge_index_descriptor"

type IndexKind string

const (
	IndexSkill   IndexKind = "skill"
	IndexLexical IndexKind = "lexical"
	IndexVector  IndexKind = "vector"
	IndexGraph   IndexKind = "graph"
)

type IndexState string

const (
	IndexBuilding IndexState = "building"
	IndexReady    IndexState = "ready"
	IndexPartial  IndexState = "partial"
	IndexStale    IndexState = "stale"
)

type IndexDescriptorV1 struct {
	ContractVersion string      `json:"contract_version"`
	ObjectKind      string      `json:"object_kind"`
	Ref             Ref         `json:"ref"`
	Owner           OwnerDomain `json:"owner"`
	Kind            IndexKind   `json:"kind"`
	ViewRef         Ref         `json:"view_ref"`
	BoundaryRef     Ref         `json:"boundary_ref"`
	RecordRefs      []Ref       `json:"record_refs"`
	BuilderRef      Ref         `json:"builder_ref"`
	ModelRef        Ref         `json:"model_ref"`
	BuilderVersion  string      `json:"builder_version"`
	IndexVersion    string      `json:"index_version"`
	Dimension       int         `json:"dimension"`
	State           IndexState  `json:"state"`
	Coverage        Coverage    `json:"coverage"`
	CreatedAt       time.Time   `json:"created_at"`
	ExpiresAt       time.Time   `json:"expires_at"`
	Digest          string      `json:"digest"`
}

func SealIndexDescriptorV1(in IndexDescriptorV1) (IndexDescriptorV1, error) {
	in.ContractVersion = FrameworkContractVersionV1
	in.ObjectKind = IndexDescriptorObjectKindV1
	records, err := normalizeSemanticRefs(in.RecordRefs)
	if err != nil {
		return IndexDescriptorV1{}, err
	}
	in.RecordRefs = records
	in.Coverage, err = normalizeCoverageV1(in.Coverage)
	if err != nil {
		return IndexDescriptorV1{}, err
	}
	in.CreatedAt, in.ExpiresAt = in.CreatedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	digest, err := Digest(in)
	if err != nil {
		return IndexDescriptorV1{}, err
	}
	in.Ref.Digest, in.Digest = digest, digest
	if err := in.Validate(in.CreatedAt); err != nil {
		return IndexDescriptorV1{}, err
	}
	return in, nil
}

func (in IndexDescriptorV1) Validate(now time.Time) error {
	if in.ContractVersion != FrameworkContractVersionV1 || in.ObjectKind != IndexDescriptorObjectKindV1 || (in.Owner != OwnerMemory && in.Owner != OwnerKnowledge) || !validIndexKind(in.Kind) || !validIndexState(in.State) || strings.TrimSpace(in.BuilderVersion) == "" || strings.TrimSpace(in.IndexVersion) == "" || in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: incomplete index descriptor", ErrInvalidArgument)
	}
	for _, ref := range []Ref{in.Ref, in.ViewRef, in.BoundaryRef, in.BuilderRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if len(in.RecordRefs) == 0 {
		return fmt.Errorf("%w: empty index", ErrInvalidArgument)
	}
	records, err := normalizeSemanticRefs(in.RecordRefs)
	if err != nil || !slices.Equal(records, in.RecordRefs) {
		return fmt.Errorf("%w: non-canonical record refs", ErrInvalidArgument)
	}
	if in.Kind == IndexVector {
		if in.Dimension <= 0 || in.Dimension > 1<<20 || in.ModelRef.Validate() != nil {
			return fmt.Errorf("%w: vector index model/dimension", ErrInvalidArgument)
		}
	} else if in.Dimension != 0 || in.ModelRef != (Ref{}) {
		return fmt.Errorf("%w: non-vector index carries vector fields", ErrInvalidArgument)
	}
	coverage, err := normalizeCoverageV1(in.Coverage)
	if err != nil || !coverageEqual(coverage, in.Coverage) {
		return fmt.Errorf("%w: non-canonical coverage", ErrInvalidArgument)
	}
	if in.State == IndexReady && in.Coverage.Status != CoverageComplete {
		return fmt.Errorf("%w: ready index has incomplete coverage", ErrEvidenceConflict)
	}
	if in.State == IndexPartial && in.Coverage.Status != CoveragePartial {
		return fmt.Errorf("%w: partial index coverage mismatch", ErrEvidenceConflict)
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return fmt.Errorf("%w: index descriptor digest", ErrEvidenceConflict)
	}
	return nil
}

func normalizeSemanticRefs(in []Ref) ([]Ref, error) {
	seen := make(map[string]struct{}, len(in))
	for _, ref := range in {
		if err := ref.Validate(); err != nil {
			return nil, err
		}
		if _, exists := seen[ref.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate semantic ref", ErrEvidenceConflict)
		}
		seen[ref.ID] = struct{}{}
	}
	out := NormalizeRefs(in)
	if out == nil {
		out = []Ref{}
	}
	return out, nil
}

func normalizeCoverageV1(in Coverage) (Coverage, error) {
	if in.Expected < 0 || in.Available < 0 || in.Available > in.Expected || !validCoverageStatus(in.Status) {
		return Coverage{}, fmt.Errorf("%w: invalid coverage", ErrInvalidArgument)
	}
	refs, err := normalizeSemanticRefsAllowEmpty(in.ProjectionRefs)
	if err != nil {
		return Coverage{}, err
	}
	reasons := slices.Clone(in.DroppedReasons)
	for _, reason := range reasons {
		if reason == "" || reason != strings.TrimSpace(reason) {
			return Coverage{}, fmt.Errorf("%w: dropped reason", ErrInvalidArgument)
		}
	}
	slices.Sort(reasons)
	reasons = slices.Compact(reasons)
	if reasons == nil {
		reasons = []string{}
	}
	in.ProjectionRefs, in.DroppedReasons = refs, reasons
	return in, nil
}

func normalizeSemanticRefsAllowEmpty(in []Ref) ([]Ref, error) {
	if len(in) == 0 {
		return []Ref{}, nil
	}
	return normalizeSemanticRefs(in)
}

func coverageEqual(a, b Coverage) bool {
	return a.Status == b.Status && a.Expected == b.Expected && a.Available == b.Available && slices.Equal(a.ProjectionRefs, b.ProjectionRefs) && slices.Equal(a.DroppedReasons, b.DroppedReasons)
}

func validIndexKind(kind IndexKind) bool {
	return kind == IndexSkill || kind == IndexLexical || kind == IndexVector || kind == IndexGraph
}
func validIndexState(state IndexState) bool {
	return state == IndexBuilding || state == IndexReady || state == IndexPartial || state == IndexStale
}
func validCoverageStatus(status CoverageStatus) bool {
	return status == CoverageComplete || status == CoveragePartial || status == CoverageNone || status == CoverageUnknown || status == CoverageUnavailable
}
