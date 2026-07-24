package graph

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection"
)

const ObjectKindV1 = "graph_projection_edge"

type EdgeV1 struct {
	ContractVersion string                `json:"contract_version"`
	ObjectKind      string                `json:"object_kind"`
	Ref             contract.Ref          `json:"ref"`
	Metadata        projection.MetadataV1 `json:"metadata"`
	From            string                `json:"from"`
	Relation        string                `json:"relation"`
	To              string                `json:"to"`
	ConfidenceBPS   uint16                `json:"confidence_bps"`
	ValidFrom       time.Time             `json:"valid_from"`
	ValidTo         time.Time             `json:"valid_to"`
	TransactionAt   time.Time             `json:"transaction_at"`
	Digest          string                `json:"digest"`
}

func SealEdgeV1(in EdgeV1) (EdgeV1, error) {
	in.ContractVersion, in.ObjectKind = projection.ContractVersionV1, ObjectKindV1
	in.Metadata = projection.NormalizeMetadataV1(in.Metadata)
	in.From = strings.TrimSpace(strings.ToLower(in.From))
	in.Relation = strings.TrimSpace(strings.ToLower(in.Relation))
	in.To = strings.TrimSpace(strings.ToLower(in.To))
	in.ValidFrom, in.ValidTo, in.TransactionAt = in.ValidFrom.UTC(), in.ValidTo.UTC(), in.TransactionAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	d, e := contract.Digest(in)
	if e != nil {
		return EdgeV1{}, e
	}
	in.Ref.Digest, in.Digest = d, d
	if e = in.Validate(in.Metadata.CreatedAt); e != nil {
		return EdgeV1{}, e
	}
	return in, nil
}
func (in EdgeV1) Validate(now time.Time) error {
	if in.ContractVersion != projection.ContractVersionV1 || in.ObjectKind != ObjectKindV1 || in.Ref.Validate() != nil || in.Metadata.Validate(now) != nil || in.From == "" || in.Relation == "" || in.To == "" || in.ConfidenceBPS > 10000 || in.ValidFrom.IsZero() || !in.ValidTo.After(in.ValidFrom) || in.TransactionAt.IsZero() {
		return fmt.Errorf("%w: graph edge", contract.ErrInvalidArgument)
	}
	c := in
	c.Ref.Digest, c.Digest = "", ""
	d, e := contract.Digest(c)
	if e != nil {
		return e
	}
	if d != in.Digest || in.Ref.Digest != d {
		return fmt.Errorf("%w: graph digest", contract.ErrEvidenceConflict)
	}
	return nil
}
func Search(now time.Time, entities []string, relation string, limit int, edges []EdgeV1) ([]contract.RetrievalHit, error) {
	if len(entities) == 0 || limit <= 0 {
		return nil, contract.ErrInvalidArgument
	}
	wanted := make(map[string]struct{}, len(entities))
	for _, e := range entities {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			return nil, contract.ErrInvalidArgument
		}
		wanted[e] = struct{}{}
	}
	relation = strings.ToLower(strings.TrimSpace(relation))
	var hits []contract.RetrievalHit
	for _, e := range edges {
		if err := e.Validate(now); err != nil {
			return nil, err
		}
		if now.Before(e.ValidFrom) || !e.ValidTo.After(now) || relation != "" && e.Relation != relation {
			continue
		}
		_, from := wanted[e.From]
		_, to := wanted[e.To]
		if from || to {
			hits = append(hits, projection.Hit(e.Metadata, int(e.ConfidenceBPS), "graph_v1"))
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
