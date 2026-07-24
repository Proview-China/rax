package projection

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

const (
	BuildContractVersionV1       = "praxis.memory-knowledge/index-build/v1"
	BuildRequestObjectKindV1     = "index_build_request"
	BuildObservationObjectKindV1 = "index_build_observation"
)

type BuildRequestV1 struct {
	ContractVersion string               `json:"contract_version"`
	ObjectKind      string               `json:"object_kind"`
	Owner           contract.OwnerDomain `json:"owner"`
	Kind            contract.IndexKind   `json:"kind"`
	ViewRef         contract.Ref         `json:"view_ref"`
	BoundaryRef     contract.Ref         `json:"boundary_ref"`
	RecordRefs      []contract.Ref       `json:"record_refs"`
	BuilderRef      contract.Ref         `json:"builder_ref"`
	ModelRef        contract.Ref         `json:"model_ref,omitempty"`
	BuilderVersion  string               `json:"builder_version"`
	IndexVersion    string               `json:"index_version"`
	Dimension       int                  `json:"dimension,omitempty"`
	RequestedAt     time.Time            `json:"requested_at"`
	ExpiresAt       time.Time            `json:"expires_at"`
	Digest          string               `json:"digest"`
}

type BuildObservationV1 struct {
	ContractVersion string               `json:"contract_version"`
	ObjectKind      string               `json:"object_kind"`
	Ref             contract.Ref         `json:"ref"`
	Owner           contract.OwnerDomain `json:"owner"`
	Kind            contract.IndexKind   `json:"kind"`
	ViewRef         contract.Ref         `json:"view_ref"`
	BoundaryRef     contract.Ref         `json:"boundary_ref"`
	RecordRefs      []contract.Ref       `json:"record_refs"`
	BuilderRef      contract.Ref         `json:"builder_ref"`
	ModelRef        contract.Ref         `json:"model_ref,omitempty"`
	BuilderVersion  string               `json:"builder_version"`
	IndexVersion    string               `json:"index_version"`
	Dimension       int                  `json:"dimension,omitempty"`
	ArtifactRef     contract.Ref         `json:"artifact_ref"`
	Coverage        contract.Coverage    `json:"coverage"`
	ObservedAt      time.Time            `json:"observed_at"`
	ExpiresAt       time.Time            `json:"expires_at"`
	Digest          string               `json:"digest"`
}

func SealBuildRequestV1(in BuildRequestV1) (BuildRequestV1, error) {
	in.ContractVersion, in.ObjectKind = BuildContractVersionV1, BuildRequestObjectKindV1
	refs, err := canonicalRecordRefs(in.RecordRefs)
	if err != nil {
		return BuildRequestV1{}, err
	}
	in.RecordRefs = refs
	in.RequestedAt, in.ExpiresAt = in.RequestedAt.UTC(), in.ExpiresAt.UTC()
	in.Digest = ""
	digest, err := contract.Digest(in)
	if err != nil {
		return BuildRequestV1{}, err
	}
	in.Digest = digest
	if err := in.Validate(in.RequestedAt); err != nil {
		return BuildRequestV1{}, err
	}
	return in, nil
}

func (in BuildRequestV1) Validate(now time.Time) error {
	if in.ContractVersion != BuildContractVersionV1 || in.ObjectKind != BuildRequestObjectKindV1 || !validBuildShape(in.Owner, in.Kind, in.ModelRef, in.Dimension) || in.ViewRef.Validate() != nil || in.BoundaryRef.Validate() != nil || in.BuilderRef.Validate() != nil || strings.TrimSpace(in.BuilderVersion) == "" || strings.TrimSpace(in.IndexVersion) == "" || in.RequestedAt.IsZero() || !in.ExpiresAt.After(in.RequestedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: index build request", contract.ErrInvalidArgument)
	}
	refs, err := canonicalRecordRefs(in.RecordRefs)
	if err != nil || !slices.Equal(refs, in.RecordRefs) {
		return fmt.Errorf("%w: index build records", contract.ErrInvalidArgument)
	}
	copy := in
	copy.Digest = ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if digest != in.Digest {
		return fmt.Errorf("%w: index build request digest", contract.ErrEvidenceConflict)
	}
	return nil
}

func SealBuildObservationV1(in BuildObservationV1) (BuildObservationV1, error) {
	in.ContractVersion, in.ObjectKind = BuildContractVersionV1, BuildObservationObjectKindV1
	refs, err := canonicalRecordRefs(in.RecordRefs)
	if err != nil {
		return BuildObservationV1{}, err
	}
	in.RecordRefs = refs
	in.Coverage.ProjectionRefs = contract.NormalizeRefs(in.Coverage.ProjectionRefs)
	in.Coverage.DroppedReasons = SortedUniqueStrings(in.Coverage.DroppedReasons)
	in.ObservedAt, in.ExpiresAt = in.ObservedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	digest, err := contract.Digest(in)
	if err != nil {
		return BuildObservationV1{}, err
	}
	in.Ref.Digest, in.Digest = digest, digest
	if err := in.Validate(in.ObservedAt); err != nil {
		return BuildObservationV1{}, err
	}
	return in, nil
}

func (in BuildObservationV1) Validate(now time.Time) error {
	if in.ContractVersion != BuildContractVersionV1 || in.ObjectKind != BuildObservationObjectKindV1 || !validBuildShape(in.Owner, in.Kind, in.ModelRef, in.Dimension) || in.Ref.Validate() != nil || in.ViewRef.Validate() != nil || in.BoundaryRef.Validate() != nil || in.BuilderRef.Validate() != nil || in.ArtifactRef.Validate() != nil || strings.TrimSpace(in.BuilderVersion) == "" || strings.TrimSpace(in.IndexVersion) == "" || in.ObservedAt.IsZero() || !in.ExpiresAt.After(in.ObservedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: index build observation", contract.ErrInvalidArgument)
	}
	refs, err := canonicalRecordRefs(in.RecordRefs)
	if err != nil || !slices.Equal(refs, in.RecordRefs) || in.Coverage.Expected < 0 || in.Coverage.Available < 0 || in.Coverage.Available > in.Coverage.Expected {
		return fmt.Errorf("%w: index build observation records", contract.ErrInvalidArgument)
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return fmt.Errorf("%w: index build observation digest", contract.ErrEvidenceConflict)
	}
	return nil
}

func validBuildShape(owner contract.OwnerDomain, kind contract.IndexKind, model contract.Ref, dimension int) bool {
	if owner != contract.OwnerMemory && owner != contract.OwnerKnowledge {
		return false
	}
	if kind != contract.IndexSkill && kind != contract.IndexLexical && kind != contract.IndexVector && kind != contract.IndexGraph {
		return false
	}
	if kind == contract.IndexVector {
		return dimension > 0 && dimension <= 1<<20 && model.Validate() == nil
	}
	return dimension == 0 && model == (contract.Ref{})
}

func canonicalRecordRefs(in []contract.Ref) ([]contract.Ref, error) {
	if len(in) == 0 {
		return nil, contract.ErrInvalidArgument
	}
	seen := make(map[string]struct{}, len(in))
	for _, ref := range in {
		if ref.Validate() != nil {
			return nil, contract.ErrInvalidArgument
		}
		if _, exists := seen[ref.ID]; exists {
			return nil, contract.ErrEvidenceConflict
		}
		seen[ref.ID] = struct{}{}
	}
	return contract.NormalizeRefs(in), nil
}
