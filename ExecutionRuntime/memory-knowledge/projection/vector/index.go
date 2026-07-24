package vector

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection"
)

const ObjectKindV1 = "vector_projection_entry"

type EntryV1 struct {
	ContractVersion string                `json:"contract_version"`
	ObjectKind      string                `json:"object_kind"`
	Ref             contract.Ref          `json:"ref"`
	Metadata        projection.MetadataV1 `json:"metadata"`
	ModelRef        contract.Ref          `json:"model_ref"`
	Dimension       uint32                `json:"dimension"`
	ChunkStart      int64                 `json:"chunk_start"`
	ChunkEnd        int64                 `json:"chunk_end"`
	Vector          []float64             `json:"vector"`
	Digest          string                `json:"digest"`
}

func SealEntryV1(in EntryV1) (EntryV1, error) {
	in.ContractVersion, in.ObjectKind = projection.ContractVersionV1, ObjectKindV1
	in.Metadata = projection.NormalizeMetadataV1(in.Metadata)
	in.Vector = slices.Clone(in.Vector)
	in.Ref.Digest, in.Digest = "", ""
	d, e := contract.Digest(in)
	if e != nil {
		return EntryV1{}, e
	}
	in.Ref.Digest, in.Digest = d, d
	if e = in.Validate(in.Metadata.CreatedAt); e != nil {
		return EntryV1{}, e
	}
	return in, nil
}
func (in EntryV1) Validate(now time.Time) error {
	if in.ContractVersion != projection.ContractVersionV1 || in.ObjectKind != ObjectKindV1 || in.Ref.Validate() != nil || in.Metadata.Validate(now) != nil || in.ModelRef.Validate() != nil || in.Dimension == 0 || int(in.Dimension) != len(in.Vector) || in.ChunkStart < 0 || in.ChunkEnd <= in.ChunkStart || in.ChunkEnd > in.Metadata.ContentRef.Length {
		return fmt.Errorf("%w: vector entry", contract.ErrInvalidArgument)
	}
	for _, v := range in.Vector {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return contract.ErrInvalidArgument
		}
	}
	c := in
	c.Ref.Digest, c.Digest = "", ""
	d, e := contract.Digest(c)
	if e != nil {
		return e
	}
	if d != in.Digest || in.Ref.Digest != d {
		return fmt.Errorf("%w: vector digest", contract.ErrEvidenceConflict)
	}
	return nil
}
func Search(now time.Time, query []float64, model contract.Ref, limit int, entries []EntryV1) ([]contract.RetrievalHit, error) {
	if len(query) == 0 || model.Validate() != nil || limit <= 0 {
		return nil, contract.ErrInvalidArgument
	}
	for _, v := range query {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, contract.ErrInvalidArgument
		}
	}
	var hits []contract.RetrievalHit
	for _, e := range entries {
		if err := e.Validate(now); err != nil {
			return nil, err
		}
		if !contract.SameRef(e.ModelRef, model) || len(e.Vector) != len(query) {
			continue
		}
		score := cosine(query, e.Vector)
		if score > 0 {
			hits = append(hits, projection.Hit(e.Metadata, int(math.Round(score*1_000_000)), "vector_v1"))
		}
	}
	slices.SortFunc(hits, func(a, b contract.RetrievalHit) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		return strings.Compare(a.RecordRef.ID, b.RecordRef.ID)
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return slices.Clone(hits), nil
}
func cosine(a, b []float64) float64 {
	var dot, aa, bb float64
	for i := range a {
		dot += a[i] * b[i]
		aa += a[i] * a[i]
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / (math.Sqrt(aa) * math.Sqrt(bb))
}
