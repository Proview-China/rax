package skill

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection"
)

const ObjectKindV1 = "skill_projection_entry"

type EntryV1 struct {
	ContractVersion string                `json:"contract_version"`
	ObjectKind      string                `json:"object_kind"`
	Ref             contract.Ref          `json:"ref"`
	Metadata        projection.MetadataV1 `json:"metadata"`
	Title           string                `json:"title"`
	Description     string                `json:"description"`
	Keywords        []string              `json:"keywords"`
	UseWhen         []string              `json:"use_when"`
	DoNotUseWhen    []string              `json:"do_not_use_when"`
	DetailRef       contract.ContentRef   `json:"detail_ref"`
	Digest          string                `json:"digest"`
}

func SealEntryV1(in EntryV1) (EntryV1, error) {
	in.ContractVersion, in.ObjectKind = projection.ContractVersionV1, ObjectKindV1
	in.Metadata = projection.NormalizeMetadataV1(in.Metadata)
	in.Keywords = projection.SortedUniqueStrings(in.Keywords)
	in.UseWhen = projection.SortedUniqueStrings(in.UseWhen)
	in.DoNotUseWhen = projection.SortedUniqueStrings(in.DoNotUseWhen)
	in.Ref.Digest, in.Digest = "", ""
	digest, err := contract.Digest(in)
	if err != nil {
		return EntryV1{}, err
	}
	in.Ref.Digest, in.Digest = digest, digest
	if err := in.Validate(in.Metadata.CreatedAt); err != nil {
		return EntryV1{}, err
	}
	return in, nil
}

func (in EntryV1) Validate(now time.Time) error {
	if in.ContractVersion != projection.ContractVersionV1 || in.ObjectKind != ObjectKindV1 || in.Ref.Validate() != nil || in.Metadata.Validate(now) != nil || strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Description) == "" || len(in.Keywords) == 0 || in.DetailRef.Validate() != nil {
		return fmt.Errorf("%w: skill entry", contract.ErrInvalidArgument)
	}
	if !slices.Equal(in.Keywords, projection.SortedUniqueStrings(in.Keywords)) || !slices.Equal(in.UseWhen, projection.SortedUniqueStrings(in.UseWhen)) || !slices.Equal(in.DoNotUseWhen, projection.SortedUniqueStrings(in.DoNotUseWhen)) {
		return fmt.Errorf("%w: skill lists", contract.ErrInvalidArgument)
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if digest != in.Digest || in.Ref.Digest != in.Digest {
		return fmt.Errorf("%w: skill digest", contract.ErrEvidenceConflict)
	}
	return nil
}

func Search(now time.Time, query string, limit int, entries []EntryV1) ([]contract.RetrievalHit, error) {
	if strings.TrimSpace(query) == "" || limit <= 0 {
		return nil, contract.ErrInvalidArgument
	}
	terms := projection.Tokens(query)
	var hits []contract.RetrievalHit
	for _, entry := range entries {
		if err := entry.Validate(now); err != nil {
			return nil, err
		}
		if overlaps(terms, projection.Tokens(strings.Join(entry.DoNotUseWhen, " "))) {
			continue
		}
		score := 4*matches(terms, projection.Tokens(entry.Title)) + 3*matches(terms, entry.Keywords) + 2*matches(terms, projection.Tokens(strings.Join(entry.UseWhen, " "))) + matches(terms, projection.Tokens(entry.Description))
		if score > 0 {
			hits = append(hits, projection.Hit(entry.Metadata, score, "skill_v1"))
		}
	}
	sortHits(hits)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return slices.Clone(hits), nil
}

func matches(query, values []string) int {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	n := 0
	for _, q := range query {
		if _, ok := set[q]; ok {
			n++
		}
	}
	return n
}
func overlaps(a, b []string) bool { return matches(a, b) > 0 }
func sortHits(h []contract.RetrievalHit) {
	slices.SortFunc(h, func(a, b contract.RetrievalHit) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		return strings.Compare(a.RecordRef.ID, b.RecordRef.ID)
	})
}
