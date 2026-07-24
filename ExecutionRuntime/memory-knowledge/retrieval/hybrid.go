package retrieval

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

const (
	HybridContractVersionV1        = "praxis.memory-knowledge/hybrid-retrieval/v1"
	HybridRequestObjectKindV1      = "hybrid_retrieval_request"
	ChannelObservationObjectKindV1 = "hybrid_channel_observation"
)

type ChannelBudgetV1 struct {
	Kind   contract.IndexKind `json:"kind"`
	Limit  int                `json:"limit"`
	Weight int                `json:"weight"`
}

type HybridRequestV1 struct {
	ContractVersion string                  `json:"contract_version"`
	ObjectKind      string                  `json:"object_kind"`
	Query           contract.RetrievalQuery `json:"query"`
	Channels        []ChannelBudgetV1       `json:"channels"`
	RRFK            int                     `json:"rrf_k"`
	MaxCandidates   int                     `json:"max_candidates"`
	Digest          string                  `json:"digest"`
}

type ChannelObservationV1 struct {
	ContractVersion string                  `json:"contract_version"`
	ObjectKind      string                  `json:"object_kind"`
	Kind            contract.IndexKind      `json:"kind"`
	ProjectionRef   contract.Ref            `json:"projection_ref"`
	ViewRef         contract.Ref            `json:"view_ref"`
	WatermarkRef    contract.Ref            `json:"watermark_ref"`
	Hits            []contract.RetrievalHit `json:"hits"`
	Coverage        contract.Coverage       `json:"coverage"`
	ObservedAt      time.Time               `json:"observed_at"`
	ExpiresAt       time.Time               `json:"expires_at"`
	Digest          string                  `json:"digest"`
}

func SealHybridRequestV1(in HybridRequestV1) (HybridRequestV1, error) {
	in.ContractVersion = HybridContractVersionV1
	in.ObjectKind = HybridRequestObjectKindV1
	channels, err := normalizeChannelBudgets(in.Channels)
	if err != nil {
		return HybridRequestV1{}, err
	}
	in.Channels = channels
	in.Digest = ""
	digest, err := hybridRequestDigest(in)
	if err != nil {
		return HybridRequestV1{}, err
	}
	in.Digest = digest
	if err := in.Validate(); err != nil {
		return HybridRequestV1{}, err
	}
	return in, nil
}

func (in HybridRequestV1) Validate() error {
	if in.ContractVersion != HybridContractVersionV1 || in.ObjectKind != HybridRequestObjectKindV1 || in.RRFK <= 0 || in.MaxCandidates <= 0 || in.MaxCandidates > 10000 || in.Query.ID == "" || in.Query.Revision == 0 || in.Query.Limit <= 0 || in.Query.Limit > in.MaxCandidates {
		return fmt.Errorf("%w: incomplete hybrid request", contract.ErrInvalidArgument)
	}
	if in.Query.ViewRef.Validate() != nil || in.Query.Domain != contract.OwnerMemory && in.Query.Domain != contract.OwnerKnowledge {
		return fmt.Errorf("%w: hybrid query", contract.ErrInvalidArgument)
	}
	channels, err := normalizeChannelBudgets(in.Channels)
	if err != nil || !slices.Equal(channels, in.Channels) {
		return fmt.Errorf("%w: channel budgets", contract.ErrInvalidArgument)
	}
	copy := in
	copy.Digest = ""
	digest, err := hybridRequestDigest(copy)
	if err != nil {
		return err
	}
	if digest != in.Digest {
		return fmt.Errorf("%w: hybrid request digest", contract.ErrEvidenceConflict)
	}
	return nil
}

func hybridRequestDigest(in HybridRequestV1) (string, error) {
	in.Digest = ""
	in.Query.Digest = ""
	in.Query.Cursor = ""
	return contract.Digest(in)
}

func SealChannelObservationV1(in ChannelObservationV1) (ChannelObservationV1, error) {
	in.ContractVersion = HybridContractVersionV1
	in.ObjectKind = ChannelObservationObjectKindV1
	in.ObservedAt, in.ExpiresAt = in.ObservedAt.UTC(), in.ExpiresAt.UTC()
	in.Hits = slices.Clone(in.Hits)
	for i := range in.Hits {
		in.Hits[i].ProjectionRefs = contract.NormalizeRefs(in.Hits[i].ProjectionRefs)
	}
	in.Coverage.ProjectionRefs = contract.NormalizeRefs(in.Coverage.ProjectionRefs)
	in.Coverage.DroppedReasons = sortedUnique(in.Coverage.DroppedReasons)
	in.Digest = ""
	digest, err := contract.Digest(in)
	if err != nil {
		return ChannelObservationV1{}, err
	}
	in.Digest = digest
	if err := in.Validate(in.ObservedAt); err != nil {
		return ChannelObservationV1{}, err
	}
	return in, nil
}

func (in ChannelObservationV1) Validate(now time.Time) error {
	if in.ContractVersion != HybridContractVersionV1 || in.ObjectKind != ChannelObservationObjectKindV1 || !validIndexKind(in.Kind) || in.ObservedAt.IsZero() || !in.ExpiresAt.After(in.ObservedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: incomplete channel observation", contract.ErrInvalidArgument)
	}
	for _, ref := range []contract.Ref{in.ProjectionRef, in.ViewRef, in.WatermarkRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	seen := make(map[string]struct{}, len(in.Hits))
	for _, hit := range in.Hits {
		if err := validateHybridHit(hit, in.ProjectionRef); err != nil {
			return err
		}
		key := hit.RecordRef.ID
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate channel record", contract.ErrEvidenceConflict)
		}
		seen[key] = struct{}{}
	}
	copy := in
	copy.Digest = ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if digest != in.Digest {
		return fmt.Errorf("%w: channel observation digest", contract.ErrEvidenceConflict)
	}
	return nil
}

// MergeHybridV1 deterministically fuses already-authorized Owner observations.
// It never reads content or upgrades a retrieval hit into an authoritative fact.
func MergeHybridV1(now time.Time, request HybridRequestV1, observations []ChannelObservationV1) (contract.RetrievalResult, error) {
	if err := request.Validate(); err != nil {
		return contract.RetrievalResult{}, err
	}
	budgetByKind := make(map[contract.IndexKind]ChannelBudgetV1, len(request.Channels))
	for _, budget := range request.Channels {
		budgetByKind[budget.Kind] = budget
	}
	type aggregate struct {
		hit   contract.RetrievalHit
		score int
	}
	aggregates := make(map[string]aggregate)
	seenChannels := make(map[contract.IndexKind]struct{})
	coverage := contract.Coverage{Status: contract.CoverageComplete}
	for _, observation := range observations {
		budget, ok := budgetByKind[observation.Kind]
		if !ok {
			return contract.RetrievalResult{}, fmt.Errorf("%w: unrequested channel", contract.ErrScopeDenied)
		}
		if _, duplicate := seenChannels[observation.Kind]; duplicate {
			return contract.RetrievalResult{}, fmt.Errorf("%w: duplicate channel", contract.ErrEvidenceConflict)
		}
		seenChannels[observation.Kind] = struct{}{}
		if err := observation.Validate(now); err != nil {
			return contract.RetrievalResult{}, err
		}
		if !contract.SameRef(observation.ViewRef, request.Query.ViewRef) || len(observation.Hits) > budget.Limit {
			return contract.RetrievalResult{}, contract.ErrNotCurrent
		}
		coverage.Expected += observation.Coverage.Expected
		coverage.Available += observation.Coverage.Available
		coverage.ProjectionRefs = append(coverage.ProjectionRefs, observation.ProjectionRef)
		coverage.ProjectionRefs = append(coverage.ProjectionRefs, observation.Coverage.ProjectionRefs...)
		coverage.DroppedReasons = append(coverage.DroppedReasons, observation.Coverage.DroppedReasons...)
		if observation.Coverage.Status != contract.CoverageComplete {
			coverage.Status = contract.CoveragePartial
		}
		for rank, hit := range observation.Hits {
			key := hit.RecordRef.ID
			value, exists := aggregates[key]
			if exists && !contract.SameRef(value.hit.RecordRef, hit.RecordRef) {
				return contract.RetrievalResult{}, fmt.Errorf("%w: cross-channel record revision drift", contract.ErrEvidenceConflict)
			}
			if exists && (!sameContentRef(value.hit.Citation.ContentRef, hit.Citation.ContentRef) || value.hit.Citation.Domain != hit.Citation.Domain) {
				return contract.RetrievalResult{}, fmt.Errorf("%w: cross-channel citation drift", contract.ErrEvidenceConflict)
			}
			if !exists {
				value.hit = hit
			}
			value.score += budget.Weight * 1_000_000 / (request.RRFK + rank + 1)
			value.hit.ProjectionRefs = contract.NormalizeRefs(append(value.hit.ProjectionRefs, observation.ProjectionRef))
			value.hit.MatchReason = appendReason(value.hit.MatchReason, string(observation.Kind))
			aggregates[key] = value
		}
	}
	for _, budget := range request.Channels {
		if _, ok := seenChannels[budget.Kind]; !ok {
			coverage.Status = contract.CoveragePartial
			coverage.DroppedReasons = append(coverage.DroppedReasons, "channel_unavailable:"+string(budget.Kind))
		}
	}
	hits := make([]contract.RetrievalHit, 0, len(aggregates))
	for _, value := range aggregates {
		value.hit.Score = value.score
		hits = append(hits, value.hit)
	}
	slices.SortFunc(hits, compareHybridHits)
	if len(hits) > request.MaxCandidates {
		hits = hits[:request.MaxCandidates]
	}
	offset, err := decodeHybridCursor(request.Query.Cursor, request.Digest, request.Query.ViewRef.Digest)
	if err != nil || offset > len(hits) {
		if err == nil {
			err = contract.ErrNotCurrent
		}
		return contract.RetrievalResult{}, err
	}
	end := min(offset+request.Query.Limit, len(hits))
	page := slices.Clone(hits[offset:end])
	next := ""
	if end < len(hits) {
		next = encodeHybridCursor(hybridCursor{RequestDigest: request.Digest, ViewDigest: request.Query.ViewRef.Digest, Offset: end})
	}
	coverage.ProjectionRefs = contract.NormalizeRefs(coverage.ProjectionRefs)
	coverage.DroppedReasons = sortedUnique(coverage.DroppedReasons)
	if len(observations) == 0 {
		coverage.Status = contract.CoverageNone
	}
	query := request.Query
	query.Cursor = ""
	query.Digest = ""
	queryDigest, err := contract.Digest(query)
	if err != nil {
		return contract.RetrievalResult{}, err
	}
	result := contract.RetrievalResult{QueryRef: contract.Ref{ID: query.ID, Revision: query.Revision, Digest: queryDigest}, ViewRef: query.ViewRef, Hits: page, Coverage: coverage, NextCursor: next, ObservedAt: now.UTC()}
	if len(observations) != 0 {
		result.WatermarkRef = observations[0].WatermarkRef
		for _, observation := range observations[1:] {
			if !contract.SameRef(result.WatermarkRef, observation.WatermarkRef) {
				return contract.RetrievalResult{}, fmt.Errorf("%w: channel watermark drift", contract.ErrEvidenceConflict)
			}
		}
	} else {
		result.WatermarkRef = contract.Ref{ID: "hybrid/none", Revision: 1, Digest: request.Digest}
	}
	result.EvidenceDigest, err = contract.Digest(citations(page))
	if err != nil {
		return contract.RetrievalResult{}, err
	}
	result.ResultDigest, err = contract.Digest(struct {
		QueryRef     contract.Ref
		ViewRef      contract.Ref
		WatermarkRef contract.Ref
		Hits         []contract.RetrievalHit
		Coverage     contract.Coverage
		NextCursor   string
	}{result.QueryRef, result.ViewRef, result.WatermarkRef, result.Hits, result.Coverage, result.NextCursor})
	if err != nil {
		return contract.RetrievalResult{}, err
	}
	return result, nil
}

type hybridCursor struct {
	RequestDigest string `json:"request_digest"`
	ViewDigest    string `json:"view_digest"`
	Offset        int    `json:"offset"`
}

func encodeHybridCursor(in hybridCursor) string {
	body, _ := json.Marshal(in)
	return base64.RawURLEncoding.EncodeToString(body)
}

func decodeHybridCursor(raw, requestDigest, viewDigest string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: hybrid cursor", contract.ErrInvalidArgument)
	}
	var cursor hybridCursor
	if err := contract.StrictDecode(body, &cursor); err != nil {
		return 0, err
	}
	if cursor.RequestDigest != requestDigest || cursor.ViewDigest != viewDigest || cursor.Offset < 0 {
		return 0, contract.ErrNotCurrent
	}
	return cursor.Offset, nil
}

func normalizeChannelBudgets(in []ChannelBudgetV1) ([]ChannelBudgetV1, error) {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b ChannelBudgetV1) int { return strings.Compare(string(a.Kind), string(b.Kind)) })
	for i, channel := range out {
		if !validIndexKind(channel.Kind) || channel.Limit <= 0 || channel.Limit > 10000 || channel.Weight <= 0 || channel.Weight > 1000 {
			return nil, fmt.Errorf("%w: channel budget", contract.ErrInvalidArgument)
		}
		if i > 0 && out[i-1].Kind == channel.Kind {
			return nil, fmt.Errorf("%w: duplicate channel budget", contract.ErrEvidenceConflict)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no channels", contract.ErrInvalidArgument)
	}
	return out, nil
}

func validIndexKind(kind contract.IndexKind) bool {
	return kind == contract.IndexSkill || kind == contract.IndexLexical || kind == contract.IndexVector || kind == contract.IndexGraph
}

func validateHybridHit(hit contract.RetrievalHit, projection contract.Ref) error {
	if hit.RecordRef.Validate() != nil || hit.Citation.RecordRef.Validate() != nil || !contract.SameRef(hit.RecordRef, hit.Citation.RecordRef) || hit.Citation.ContentRef.Validate() != nil || hit.Citation.Domain != contract.OwnerMemory && hit.Citation.Domain != contract.OwnerKnowledge || !containsRef(hit.ProjectionRefs, projection) {
		return fmt.Errorf("%w: invalid channel hit", contract.ErrInvalidArgument)
	}
	return nil
}

func compareHybridHits(a, b contract.RetrievalHit) int {
	if a.Score != b.Score {
		return b.Score - a.Score
	}
	if c := strings.Compare(a.RecordRef.ID, b.RecordRef.ID); c != 0 {
		return c
	}
	if a.RecordRef.Revision != b.RecordRef.Revision {
		if a.RecordRef.Revision > b.RecordRef.Revision {
			return -1
		}
		return 1
	}
	return strings.Compare(a.RecordRef.Digest, b.RecordRef.Digest)
}

func appendReason(current, channel string) string {
	parts := strings.Split(current, "+")
	parts = append(parts, "hybrid_"+channel)
	return strings.Join(sortedUnique(parts), "+")
}

func sameContentRef(a, b contract.ContentRef) bool {
	return a.ID == b.ID && a.Digest == b.Digest && a.Length == b.Length && a.MediaType == b.MediaType
}

func containsRef(refs []contract.Ref, target contract.Ref) bool {
	for _, ref := range refs {
		if contract.SameRef(ref, target) {
			return true
		}
	}
	return false
}
