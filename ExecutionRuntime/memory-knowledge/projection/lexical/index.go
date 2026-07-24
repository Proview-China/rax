package lexical

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection"
)

const ObjectKindV1 = "lexical_projection_entry"

type EntryV1 struct {
	ContractVersion string                `json:"contract_version"`
	ObjectKind      string                `json:"object_kind"`
	Ref             contract.Ref          `json:"ref"`
	Metadata        projection.MetadataV1 `json:"metadata"`
	TermFrequency   map[string]uint32     `json:"term_frequency"`
	TokenCount      uint32                `json:"token_count"`
	Digest          string                `json:"digest"`
}

func BuildEntryV1(ref contract.Ref, metadata projection.MetadataV1, text string) (EntryV1, error) {
	terms := projection.Tokens(text)
	freq := make(map[string]uint32)
	for _, term := range terms {
		freq[term]++
	}
	return SealEntryV1(EntryV1{Ref: ref, Metadata: metadata, TermFrequency: freq, TokenCount: uint32(len(terms))})
}
func SealEntryV1(in EntryV1) (EntryV1, error) {
	in.ContractVersion, in.ObjectKind = projection.ContractVersionV1, ObjectKindV1
	in.Metadata = projection.NormalizeMetadataV1(in.Metadata)
	in.TermFrequency = maps.Clone(in.TermFrequency)
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
	if in.ContractVersion != projection.ContractVersionV1 || in.ObjectKind != ObjectKindV1 || in.Ref.Validate() != nil || in.Metadata.Validate(now) != nil || len(in.TermFrequency) == 0 || in.TokenCount == 0 {
		return fmt.Errorf("%w: lexical entry", contract.ErrInvalidArgument)
	}
	var total uint32
	for term, n := range in.TermFrequency {
		if term != strings.ToLower(strings.TrimSpace(term)) || term == "" || n == 0 {
			return contract.ErrInvalidArgument
		}
		total += n
	}
	if total != in.TokenCount {
		return fmt.Errorf("%w: token count", contract.ErrEvidenceConflict)
	}
	c := in
	c.Ref.Digest, c.Digest = "", ""
	d, e := contract.Digest(c)
	if e != nil {
		return e
	}
	if d != in.Digest || in.Ref.Digest != d {
		return fmt.Errorf("%w: lexical digest", contract.ErrEvidenceConflict)
	}
	return nil
}
func Search(now time.Time, query string, limit int, entries []EntryV1) ([]contract.RetrievalHit, error) {
	if strings.TrimSpace(query) == "" || limit <= 0 {
		return nil, contract.ErrInvalidArgument
	}
	terms := projection.Tokens(query)
	var hits []contract.RetrievalHit
	for _, e := range entries {
		if err := e.Validate(now); err != nil {
			return nil, err
		}
		score := 0
		for _, term := range terms {
			score += int(e.TermFrequency[term])
		}
		if score > 0 {
			hits = append(hits, projection.Hit(e.Metadata, score, "lexical_v1"))
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
