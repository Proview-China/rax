// Package observability defines backend-neutral metric observations. Metrics
// are diagnostics, never Memory/Knowledge facts or Runtime outcomes.
package observability

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

const (
	ContractVersionV1    = "praxis.memory-knowledge/metrics/v1"
	SnapshotObjectKindV1 = "memory_knowledge_metric_snapshot"
)

type MetricKind string

const (
	MetricCapacityRecords    MetricKind = "capacity_records"
	MetricQueryLatencyNanos  MetricKind = "query_latency_nanoseconds"
	MetricRecallQualityBPS   MetricKind = "recall_quality_bps"
	MetricNoResultBPS        MetricKind = "no_result_bps"
	MetricConflictBPS        MetricKind = "conflict_bps"
	MetricStaleBPS           MetricKind = "stale_bps"
	MetricScopeDeniedBPS     MetricKind = "scope_denied_bps"
	MetricContextAdoptionBPS MetricKind = "context_adoption_bps"
	MetricTaskEffectBPS      MetricKind = "task_effect_bps"
)

type SampleV1 struct {
	Kind       MetricKind     `json:"kind"`
	Value      int64          `json:"value"`
	Unit       string         `json:"unit"`
	SourceRefs []contract.Ref `json:"source_refs"`
}

type SnapshotV1 struct {
	ContractVersion string               `json:"contract_version"`
	ObjectKind      string               `json:"object_kind"`
	Ref             contract.Ref         `json:"ref"`
	Owner           contract.OwnerDomain `json:"owner"`
	TenantID        string               `json:"tenant_id"`
	BoundaryRef     contract.Ref         `json:"boundary_ref"`
	Samples         []SampleV1           `json:"samples"`
	ObservedAt      time.Time            `json:"observed_at"`
	ExpiresAt       time.Time            `json:"expires_at"`
	Digest          string               `json:"digest"`
}

func SealSnapshotV1(in SnapshotV1) (SnapshotV1, error) {
	in.ContractVersion, in.ObjectKind = ContractVersionV1, SnapshotObjectKindV1
	for i := range in.Samples {
		in.Samples[i].SourceRefs = contract.NormalizeRefs(in.Samples[i].SourceRefs)
	}
	slices.SortFunc(in.Samples, func(a, b SampleV1) int { return strings.Compare(string(a.Kind), string(b.Kind)) })
	in.ObservedAt, in.ExpiresAt = in.ObservedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	digest, err := contract.Digest(in)
	if err != nil {
		return SnapshotV1{}, err
	}
	in.Ref.Digest, in.Digest = digest, digest
	if err := in.Validate(in.ObservedAt); err != nil {
		return SnapshotV1{}, err
	}
	return in, nil
}

func (in SnapshotV1) Validate(now time.Time) error {
	if in.ContractVersion != ContractVersionV1 || in.ObjectKind != SnapshotObjectKindV1 || (in.Owner != contract.OwnerMemory && in.Owner != contract.OwnerKnowledge) || strings.TrimSpace(in.TenantID) == "" || in.Ref.Validate() != nil || in.BoundaryRef.Validate() != nil || len(in.Samples) == 0 || in.ObservedAt.IsZero() || !in.ExpiresAt.After(in.ObservedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: metric snapshot", contract.ErrInvalidArgument)
	}
	previous := MetricKind("")
	for _, sample := range in.Samples {
		if !validMetric(sample.Kind) || sample.Kind <= previous || sample.Value < 0 || strings.TrimSpace(sample.Unit) == "" || !slices.Equal(sample.SourceRefs, contract.NormalizeRefs(sample.SourceRefs)) {
			return fmt.Errorf("%w: metric sample", contract.ErrInvalidArgument)
		}
		if strings.HasSuffix(string(sample.Kind), "_bps") && (sample.Unit != "basis_points" || sample.Value > 10000) {
			return fmt.Errorf("%w: ratio metric", contract.ErrInvalidArgument)
		}
		for _, ref := range sample.SourceRefs {
			if ref.Validate() != nil {
				return contract.ErrInvalidArgument
			}
		}
		previous = sample.Kind
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return fmt.Errorf("%w: metric snapshot digest", contract.ErrEvidenceConflict)
	}
	return nil
}

func validMetric(in MetricKind) bool {
	switch in {
	case MetricCapacityRecords, MetricQueryLatencyNanos, MetricRecallQualityBPS, MetricNoResultBPS, MetricConflictBPS, MetricStaleBPS, MetricScopeDeniedBPS, MetricContextAdoptionBPS, MetricTaskEffectBPS:
		return true
	default:
		return false
	}
}
