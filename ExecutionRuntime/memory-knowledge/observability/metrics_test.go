package observability_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/observability"
)

func mr(id string) contract.Ref { return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id} }

func TestMetricSnapshotCanonicalAndTamperClosed(t *testing.T) {
	now := time.Date(2026, 7, 17, 7, 0, 0, 0, time.UTC)
	snapshot, err := observability.SealSnapshotV1(observability.SnapshotV1{Ref: contract.Ref{ID: "metrics", Revision: 1}, Owner: contract.OwnerMemory, TenantID: "tenant", BoundaryRef: mr("watermark"), Samples: []observability.SampleV1{{Kind: observability.MetricStaleBPS, Value: 250, Unit: "basis_points", SourceRefs: []contract.Ref{mr("index")}}, {Kind: observability.MetricCapacityRecords, Value: 42, Unit: "records", SourceRefs: []contract.Ref{mr("record-set")}}}, ObservedAt: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil || snapshot.Samples[0].Kind != observability.MetricCapacityRecords {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	tampered := snapshot
	tampered.Samples[0].Value++
	if err := tampered.Validate(now); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("metric tamper accepted: %v", err)
	}
	badRatio := snapshot
	badRatio.Ref = contract.Ref{ID: "bad", Revision: 1}
	badRatio.Samples[1].Value = 10001
	badRatio.Digest = ""
	if _, err := observability.SealSnapshotV1(badRatio); !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("invalid ratio accepted: %v", err)
	}
}
