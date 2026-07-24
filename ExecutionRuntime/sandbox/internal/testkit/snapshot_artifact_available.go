package testkit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

func SnapshotArtifactCommitRequest(reservation contract.SnapshotArtifactReservationV2, current contract.SnapshotArtifactAggregateCurrentIndexV2, suffix string, now time.Time) contract.CommitSnapshotArtifactRequestV2 {
	namespace := contract.SnapshotArtifactExactRefV2{TypeURL: "praxis.sandbox/host-local-namespace/v1", Version: 1, ID: "host-local-namespace-" + suffix, Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: "praxis.sandbox/host-local-namespace/body/v1", Digest: Ref("host-local-namespace-" + suffix).Digest, ExpiresUnixNano: now.Add(3 * time.Hour).UnixNano()}
	storage, err := contract.SealSnapshotStorageArtifactRefV2(contract.SnapshotStorageArtifactRefV2{StorageArtifactID: "storage-artifact-" + suffix, Revision: 1, TenantID: reservation.TenantID, DataDomain: reservation.DataDomain, StorageNamespaceExactRef: namespace, ContentDigest: reservation.ExpectedContentDigest, SchemaRef: reservation.SchemaRef, Length: 4096, EncryptionFactRef: reservation.EncryptionPolicyRef, ResidencyFactRef: reservation.ResidencyPolicyRef, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(90 * time.Minute).UnixNano()})
	if err != nil {
		panic(err)
	}
	return contract.CommitSnapshotArtifactRequestV2{ReservationRef: reservation.ExactRef(), ExpectedAggregateRef: current.HeadAggregateEnvelopeRef, StorageArtifactRef: storage, ProviderObservationRef: Ref("provider-observation-" + suffix), ProviderReceiptRef: Ref("provider-receipt-" + suffix), FormalEvidenceRefs: []contract.Ref{Ref("evidence-" + suffix)}, OwnerInspectionRef: Ref("owner-inspection-" + suffix), SourceAttemptRef: reservation.SourceAttemptRef, RequestedNotAfter: now.Add(2 * time.Hour).UnixNano()}
}

var ErrInjectedSnapshotArtifactAvailableLostReply = errors.New("injected snapshot artifact available reply loss")

type SnapshotArtifactAvailableLostReplyStore struct {
	*SnapshotArtifactMemoryStore
	injected atomic.Bool
}

func NewSnapshotArtifactAvailableLostReplyStore(base *SnapshotArtifactMemoryStore) *SnapshotArtifactAvailableLostReplyStore {
	return &SnapshotArtifactAvailableLostReplyStore{SnapshotArtifactMemoryStore: base}
}

func (s *SnapshotArtifactAvailableLostReplyStore) CommitAvailableSnapshotArtifact(ctx context.Context, bundle contract.SnapshotArtifactAvailableBundleV2) (bool, error) {
	created, err := s.SnapshotArtifactMemoryStore.CommitAvailableSnapshotArtifact(ctx, bundle)
	if err == nil && created && s.injected.CompareAndSwap(false, true) {
		return false, ErrInjectedSnapshotArtifactAvailableLostReply
	}
	return created, err
}

func SnapshotArtifactCommitProjection(request contract.CommitSnapshotArtifactRequestV2, tenantID, dataDomain string, now time.Time) contract.SnapshotArtifactCommitCurrentProjectionV2 {
	value, err := contract.SealSnapshotArtifactCommitCurrentProjectionV2(contract.SnapshotArtifactCommitCurrentProjectionV2{TenantID: tenantID, DataDomain: dataDomain, ReservationRef: request.ReservationRef, ExpectedAggregateRef: request.ExpectedAggregateRef, StorageArtifactRef: request.StorageArtifactRef, ProviderObservationRef: request.ProviderObservationRef, ProviderReceiptRef: request.ProviderReceiptRef, FormalEvidenceRefs: append([]contract.Ref(nil), request.FormalEvidenceRefs...), OwnerInspectionRef: request.OwnerInspectionRef, SourceAttemptRef: request.SourceAttemptRef, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}, now)
	if err != nil {
		panic(err)
	}
	return value
}

type SnapshotArtifactCommitCurrentReader struct {
	mu       sync.Mutex
	Calls    int
	ReadFunc func(int, contract.CommitSnapshotArtifactRequestV2) (contract.SnapshotArtifactCommitCurrentProjectionV2, error)
}

func (r *SnapshotArtifactCommitCurrentReader) InspectSnapshotArtifactCommitCurrentV2(_ context.Context, request contract.CommitSnapshotArtifactRequestV2) (contract.SnapshotArtifactCommitCurrentProjectionV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Calls++
	return r.ReadFunc(r.Calls, request)
}
