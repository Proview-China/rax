package contract

import "time"

type CoverageStatus string

const (
	CoverageComplete    CoverageStatus = "complete"
	CoveragePartial     CoverageStatus = "partial"
	CoverageNone        CoverageStatus = "none"
	CoverageUnknown     CoverageStatus = "unknown"
	CoverageUnavailable CoverageStatus = "unavailable"
)

type Coverage struct {
	Status         CoverageStatus `json:"status"`
	Expected       int            `json:"expected"`
	Available      int            `json:"available"`
	ProjectionRefs []Ref          `json:"projection_refs"`
	DroppedReasons []string       `json:"dropped_reasons"`
}

type Citation struct {
	Domain        OwnerDomain `json:"domain"`
	RecordRef     Ref         `json:"record_ref"`
	SourceRefs    []Ref       `json:"source_refs"`
	EvidenceRefs  []Ref       `json:"evidence_refs"`
	ContentRef    ContentRef  `json:"content_ref"`
	RangeStart    int64       `json:"range_start"`
	RangeEnd      int64       `json:"range_end"`
	Current       bool        `json:"current"`
	SummaryDigest string      `json:"summary_digest"`
}

type RetrievalQuery struct {
	ID             string      `json:"id"`
	Revision       uint64      `json:"revision"`
	Digest         string      `json:"digest"`
	Domain         OwnerDomain `json:"domain"`
	ViewRef        Ref         `json:"view_ref"`
	Purpose        string      `json:"purpose"`
	Text           string      `json:"text"`
	Scopes         []string    `json:"scopes"`
	SensitivityMax string      `json:"sensitivity_max"`
	Limit          int         `json:"limit"`
	Cursor         string      `json:"cursor,omitempty"`
	RequestedAt    time.Time   `json:"requested_at"`
	ExpiresAt      time.Time   `json:"expires_at"`
}

type RetrievalHit struct {
	RecordRef      Ref      `json:"record_ref"`
	Score          int      `json:"score"`
	MatchReason    string   `json:"match_reason"`
	Scope          string   `json:"scope"`
	Subject        string   `json:"subject"`
	ConflictGroup  string   `json:"conflict_group,omitempty"`
	TrustState     string   `json:"trust_state,omitempty"`
	License        string   `json:"license,omitempty"`
	SnapshotRef    Ref      `json:"snapshot_ref,omitempty"`
	PackageRef     Ref      `json:"package_ref,omitempty"`
	ProjectionRefs []Ref    `json:"projection_refs"`
	Citation       Citation `json:"citation"`
}

type RetrievalResult struct {
	QueryRef       Ref            `json:"query_ref"`
	ViewRef        Ref            `json:"view_ref"`
	WatermarkRef   Ref            `json:"watermark_ref"`
	Hits           []RetrievalHit `json:"hits"`
	Coverage       Coverage       `json:"coverage"`
	NextCursor     string         `json:"next_cursor,omitempty"`
	ResultDigest   string         `json:"result_digest"`
	EvidenceDigest string         `json:"evidence_digest"`
	ObservedAt     time.Time      `json:"observed_at"`
}
