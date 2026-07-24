package retrieval

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

type Document struct {
	Domain         contract.OwnerDomain
	RecordRef      contract.Ref
	ContentRef     contract.ContentRef
	Text           string
	Scope          string
	Subject        string
	Sensitivity    string
	Current        bool
	SourceRefs     []contract.Ref
	EvidenceRefs   []contract.Ref
	ProjectionRefs []contract.Ref
	ConflictGroup  string
	TrustState     string
	License        string
	SnapshotRef    contract.Ref
	PackageRef     contract.Ref
	RelevanceBPS   int
}

type cursor struct {
	QueryDigest     string `json:"query_digest"`
	ViewDigest      string `json:"view_digest"`
	WatermarkDigest string `json:"watermark_digest"`
	Offset          int    `json:"offset"`
}

func Search(now time.Time, query contract.RetrievalQuery, watermark contract.Ref, docs []Document, base contract.Coverage) (contract.RetrievalResult, error) {
	if query.ID == "" || query.Revision == 0 || query.Domain == "" || query.Purpose == "" || strings.TrimSpace(query.Text) == "" || query.Limit <= 0 || query.Limit > 1000 {
		return contract.RetrievalResult{}, fmt.Errorf("%w: invalid retrieval query", contract.ErrInvalidArgument)
	}
	if err := query.ViewRef.Validate(); err != nil {
		return contract.RetrievalResult{}, err
	}
	if err := watermark.Validate(); err != nil {
		return contract.RetrievalResult{}, err
	}
	if query.RequestedAt.IsZero() || query.ExpiresAt.IsZero() || !query.ExpiresAt.After(now) {
		return contract.RetrievalResult{}, contract.ErrNotCurrent
	}
	qDigest, err := queryDigest(query)
	if err != nil {
		return contract.RetrievalResult{}, err
	}
	offset, err := decodeCursor(query.Cursor, qDigest, query.ViewRef.Digest, watermark.Digest)
	if err != nil {
		return contract.RetrievalResult{}, err
	}
	terms := tokenize(query.Text)
	scopes := make(map[string]struct{}, len(query.Scopes))
	for _, scope := range query.Scopes {
		scopes[scope] = struct{}{}
	}
	var hits []contract.RetrievalHit
	for _, doc := range docs {
		if doc.Domain != query.Domain || !doc.Current {
			continue
		}
		if err := validateDocument(doc); err != nil {
			return contract.RetrievalResult{}, err
		}
		if !allowedScope(scopes, doc.Scope) || !allowedSensitivity(query.SensitivityMax, doc.Sensitivity) {
			continue
		}
		score := scoreTerms(terms, tokenize(doc.Text))
		if score > 0 && doc.RelevanceBPS > 0 {
			score = max(1, score*doc.RelevanceBPS/10_000)
		}
		if score == 0 {
			continue
		}
		summaryDigest, err := canonicalDigest(struct {
			Ref  contract.Ref
			Text string
		}{doc.RecordRef, doc.Text})
		if err != nil {
			return contract.RetrievalResult{}, err
		}
		hits = append(hits, contract.RetrievalHit{
			RecordRef: doc.RecordRef, Score: score, MatchReason: "deterministic_lexical_v1",
			Scope: doc.Scope, Subject: doc.Subject, ConflictGroup: doc.ConflictGroup,
			TrustState: doc.TrustState, License: doc.License, SnapshotRef: doc.SnapshotRef,
			PackageRef: doc.PackageRef, ProjectionRefs: contract.NormalizeRefs(doc.ProjectionRefs),
			Citation: contract.Citation{
				Domain: doc.Domain, RecordRef: doc.RecordRef, SourceRefs: contract.NormalizeRefs(doc.SourceRefs),
				EvidenceRefs: contract.NormalizeRefs(doc.EvidenceRefs), ContentRef: doc.ContentRef,
				RangeStart: 0, RangeEnd: doc.ContentRef.Length, Current: true,
				SummaryDigest: summaryDigest,
			},
		})
	}
	slices.SortFunc(hits, func(a, b contract.RetrievalHit) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		if c := strings.Compare(a.RecordRef.ID, b.RecordRef.ID); c != 0 {
			return c
		}
		if a.RecordRef.Revision > b.RecordRef.Revision {
			return -1
		}
		if a.RecordRef.Revision < b.RecordRef.Revision {
			return 1
		}
		return strings.Compare(a.RecordRef.Digest, b.RecordRef.Digest)
	})
	if offset > len(hits) {
		return contract.RetrievalResult{}, fmt.Errorf("%w: cursor offset", contract.ErrNotCurrent)
	}
	end := min(offset+query.Limit, len(hits))
	page := slices.Clone(hits[offset:end])
	next := ""
	if end < len(hits) {
		next = encodeCursor(cursor{qDigest, query.ViewRef.Digest, watermark.Digest, end})
	}
	coverage := base
	coverage.ProjectionRefs = contract.NormalizeRefs(coverage.ProjectionRefs)
	coverage.DroppedReasons = sortedUnique(coverage.DroppedReasons)
	if coverage.Status == "" {
		coverage.Status = contract.CoverageComplete
	}
	result := contract.RetrievalResult{
		QueryRef: contract.Ref{ID: query.ID, Revision: query.Revision, Digest: qDigest},
		ViewRef:  query.ViewRef, WatermarkRef: watermark, Hits: page, Coverage: coverage,
		NextCursor: next, ObservedAt: now.UTC(),
	}
	result.EvidenceDigest, err = canonicalDigest(citations(page))
	if err != nil {
		return contract.RetrievalResult{}, err
	}
	result.ResultDigest, err = canonicalDigest(struct {
		Query     contract.Ref
		View      contract.Ref
		Watermark contract.Ref
		Hits      []contract.RetrievalHit
		Coverage  contract.Coverage
		Next      string
	}{result.QueryRef, result.ViewRef, result.WatermarkRef, result.Hits, result.Coverage, result.NextCursor})
	if err != nil {
		return contract.RetrievalResult{}, err
	}
	return result, nil
}

func validateDocument(doc Document) error {
	if err := doc.RecordRef.Validate(); err != nil {
		return err
	}
	if err := doc.ContentRef.Validate(); err != nil {
		return err
	}
	if len(doc.SourceRefs) == 0 {
		return fmt.Errorf("%w: retrieval document has no source citation", contract.ErrInvalidArgument)
	}
	for _, ref := range doc.SourceRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func queryDigest(q contract.RetrievalQuery) (string, error) {
	q.Digest = ""
	q.Cursor = ""
	q.Scopes = sortedUnique(q.Scopes)
	return canonicalDigest(q)
}

func canonicalDigest(value any) (string, error) {
	digest, err := contract.Digest(value)
	if err != nil {
		return "", fmt.Errorf("%w: canonical retrieval digest: %v", contract.ErrInvalidArgument, err)
	}
	return digest, nil
}

func decodeCursor(raw, queryDigest, viewDigest, watermarkDigest string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: cursor encoding", contract.ErrInvalidArgument)
	}
	var c cursor
	if err := contract.StrictDecode(b, &c); err != nil {
		return 0, err
	}
	if c.QueryDigest != queryDigest || c.ViewDigest != viewDigest || c.WatermarkDigest != watermarkDigest || c.Offset < 0 {
		return 0, contract.ErrNotCurrent
	}
	return c.Offset, nil
}

func encodeCursor(c cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func scoreTerms(query, doc []string) int {
	counts := make(map[string]int, len(doc))
	for _, term := range doc {
		counts[term]++
	}
	score := 0
	for _, term := range query {
		score += counts[term]
	}
	return score
}

func allowedScope(allowed map[string]struct{}, scope string) bool {
	if len(allowed) == 0 {
		return false
	}
	_, ok := allowed[scope]
	return ok
}

func allowedSensitivity(maximum, value string) bool {
	rank := map[string]int{"public": 0, "internal": 1, "confidential": 2, "restricted": 3}
	maxRank, okMax := rank[maximum]
	valueRank, okValue := rank[value]
	return okMax && okValue && valueRank <= maxRank
}

func sortedUnique(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

func citations(hits []contract.RetrievalHit) []contract.Citation {
	out := make([]contract.Citation, len(hits))
	for i := range hits {
		out[i] = hits[i].Citation
	}
	return out
}
