package production

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var fixedNow = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

type bundleReader struct {
	mu     sync.Mutex
	values []ProductionProofBundleV2
	err    error
	calls  int
}

func (r *bundleReader) InspectProductionProofBundleV2(context.Context, string, core.Revision) (ProductionProofBundleV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return ProductionProofBundleV2{}, r.err
	}
	index := r.calls - 1
	if index >= len(r.values) {
		index = len(r.values) - 1
	}
	return r.values[index], nil
}

type resourceReader struct {
	set     runtimeports.ResourceBindingSetV1
	handles map[runtimeports.ResourceHandleRefV1]runtimeports.ResourceHandleCurrentV1
}

func (r *resourceReader) InspectResourceBindingSetCurrentV1(context.Context, runtimeports.ResourceBindingSetRefV1) (runtimeports.ResourceBindingSetV1, error) {
	return r.set, nil
}

func (r *resourceReader) InspectResourceHandleCurrentV1(_ context.Context, ref runtimeports.ResourceHandleRefV1) (runtimeports.ResourceHandleCurrentV1, error) {
	value, ok := r.handles[ref]
	if !ok {
		return runtimeports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "resource absent")
	}
	return value, nil
}

func TestProductionProfilesAndReadinessMapping(t *testing.T) {
	for _, profile := range []AvailabilityProfileV2{ProfileNonHAV2, ProfileHAV2} {
		t.Run(string(profile), func(t *testing.T) {
			bundle, resources := fixture(t, profile)
			verifier, err := NewReadinessVerifierV2(&bundleReader{values: []ProductionProofBundleV2{bundle, bundle}}, resources, func() time.Time { return fixedNow })
			if err != nil {
				t.Fatal(err)
			}
			got, err := verifier.InspectMemoryKnowledgeProductionReadinessV1(context.Background(), bundle.ReleaseID, bundle.Revision)
			if err != nil {
				t.Fatal(err)
			}
			if got.DurableMemoryFactStoreRef.ID != bundle.MemoryFactStoreRef.ID || got.CertificationFactRef.ID != bundle.CertificationFactRef.ID || got.Digest == "" {
				t.Fatalf("mapping drifted: %#v", got)
			}
		})
	}
}

func TestProductionProofFailClosed(t *testing.T) {
	base, resources := fixture(t, ProfileHAV2)
	cases := map[string]func(*ProductionProofBundleV2){
		"tamper": func(v *ProductionProofBundleV2) { v.ReplicaCount++ },
		"expired": func(v *ProductionProofBundleV2) {
			v.CheckedUnixNano = fixedNow.Add(-2 * time.Hour).UnixNano()
			v.ExpiresUnixNano = fixedNow.Add(-time.Hour).UnixNano()
			v.Digest = ""
			*v, _ = SealProductionProofBundleV2(*v)
		},
		"weak quorum": func(v *ProductionProofBundleV2) {
			v.WriteQuorum = 1
			v.Digest = ""
			*v, _ = SealProductionProofBundleV2(*v)
		},
		"missing failover": func(v *ProductionProofBundleV2) {
			v.FailoverProofRef = nil
			v.Digest = ""
			*v, _ = SealProductionProofBundleV2(*v)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			value := base
			mutate(&value)
			verifier, _ := NewReadinessVerifierV2(&bundleReader{values: []ProductionProofBundleV2{value, value}}, resources, func() time.Time { return fixedNow })
			if _, err := verifier.InspectMemoryKnowledgeProductionReadinessV1(context.Background(), base.ReleaseID, base.Revision); err == nil {
				t.Fatal("unsafe proof was accepted")
			}
		})
	}

	drift := base
	drift.CheckedUnixNano++
	drift.Digest = ""
	drift, _ = SealProductionProofBundleV2(drift)
	verifier, _ := NewReadinessVerifierV2(&bundleReader{values: []ProductionProofBundleV2{base, drift}}, resources, func() time.Time { return fixedNow })
	if _, err := verifier.InspectMemoryKnowledgeProductionReadinessV1(context.Background(), base.ReleaseID, base.Revision); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("S1/S2 drift: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	verifier, _ = NewReadinessVerifierV2(&bundleReader{values: []ProductionProofBundleV2{base}}, resources, func() time.Time { return fixedNow })
	if _, err := verifier.InspectMemoryKnowledgeProductionReadinessV1(ctx, base.ReleaseID, base.Revision); err == nil {
		t.Fatal("canceled inspection was accepted")
	}
}

func TestProductionResourceExactAndConcurrent(t *testing.T) {
	bundle, resources := fixture(t, ProfileNonHAV2)
	broken := *resources
	broken.handles = map[runtimeports.ResourceHandleRefV1]runtimeports.ResourceHandleCurrentV1{}
	verifier, _ := NewReadinessVerifierV2(&bundleReader{values: []ProductionProofBundleV2{bundle, bundle}}, &broken, func() time.Time { return fixedNow })
	if _, err := verifier.InspectMemoryKnowledgeProductionReadinessV1(context.Background(), bundle.ReleaseID, bundle.Revision); err == nil {
		t.Fatal("missing exact resource was accepted")
	}

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reader := &bundleReader{values: []ProductionProofBundleV2{bundle, bundle}}
			value, _ := NewReadinessVerifierV2(reader, resources, func() time.Time { return fixedNow })
			_, err := value.InspectMemoryKnowledgeProductionReadinessV1(context.Background(), bundle.ReleaseID, bundle.Revision)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func fixture(t *testing.T, profile AvailabilityProfileV2) (ProductionProofBundleV2, *resourceReader) {
	t.Helper()
	expires := fixedNow.Add(time.Hour).UnixNano()
	owner := core.OwnerRef{Domain: "praxis.deployment", ID: "deployment-owner"}
	current := func(id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: owner, ContractVersion: "praxis.fixture/current/v1", ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires}
	}
	cleanup := current("cleanup-contract")
	deployment := current("deployment-attestation")
	resource := func(id string, kind runtimeports.ResourceHandleKindV1) runtimeports.ResourceHandleCurrentV1 {
		value, err := runtimeports.SealResourceHandleCurrentV1(runtimeports.ResourceHandleCurrentV1{
			Ref:             runtimeports.ResourceHandleRefV1{Owner: owner, ID: id, Revision: 1, Kind: kind, ScopeDigest: core.DigestBytes([]byte(id + "/scope"))},
			CleanupContract: cleanup, DeploymentAttestation: deployment,
			CheckedUnixNano: fixedNow.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires,
		})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	handles := []runtimeports.ResourceHandleCurrentV1{
		resource("memory-fact", MemoryFactStoreKindV2),
		resource("memory-content", MemoryContentStoreKindV2),
		resource("knowledge-fact", KnowledgeFactStoreKindV2),
		resource("knowledge-content", KnowledgeContentStoreKindV2),
	}
	bindings := make([]runtimeports.ResourceBindingV1, 0, len(handles))
	handleMap := make(map[runtimeports.ResourceHandleRefV1]runtimeports.ResourceHandleCurrentV1, len(handles))
	for _, handle := range handles {
		handleMap[handle.Ref] = handle
		bindings = append(bindings, runtimeports.ResourceBindingV1{
			ComponentID: "praxis/memory-knowledge", Handle: handle.Ref,
			CleanupContract: handle.CleanupContract, DeploymentAttestation: handle.DeploymentAttestation,
		})
	}
	set, err := runtimeports.SealResourceBindingSetV1(runtimeports.ResourceBindingSetV1{
		Ref:      runtimeports.ResourceBindingSetRefV1{ID: "memory-knowledge-resource-set", Revision: 1},
		Bindings: bindings, CheckedUnixNano: fixedNow.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle := ProductionProofBundleV2{
		ReleaseID: "praxis.memory-knowledge/release", Revision: 1, Profile: profile,
		ArtifactDigest: core.DigestBytes([]byte("artifact")), ManifestDigest: core.DigestBytes([]byte("manifest")),
		ResourceBindingSetRef: set.Ref, MemoryFactStoreRef: handles[0].Ref, MemoryContentStoreRef: handles[1].Ref,
		KnowledgeFactStoreRef: handles[2].Ref, KnowledgeContentStoreRef: handles[3].Ref,
		AuthorityPolicyCurrentRef: current("authority-policy"), CredentialCurrentRef: current("credential"),
		RetrievalIndexCurrentRef: current("retrieval-index"), ContextSourceCurrentRef: current("context-source"),
		SettlementCurrentRef: current("settlement"), PurgeEffectCurrentRef: current("purge-effect"),
		CleanupOwnerCurrentRef: current("cleanup-owner"), DeploymentAttestationRef: current("deployment-proof"),
		CertificationFactRef: current("certification"), SingleWriterFenceCurrentRef: current("single-writer-fence"),
		RecoveryProofRef: current("recovery-proof"), BackupRestoreProofRef: current("backup-restore-proof"),
		ReplicaCount: 1, WriteQuorum: 1, ReadQuorum: 1,
		CheckedUnixNano: fixedNow.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires,
	}
	if profile == ProfileHAV2 {
		bundle.ReplicaCount, bundle.WriteQuorum, bundle.ReadQuorum = 3, 2, 2
		replication, quorum, failover, monotonic := current("replication"), current("quorum"), current("failover"), current("monotonic-current")
		bundle.ReplicationCurrentRef, bundle.QuorumCurrentRef, bundle.FailoverProofRef, bundle.MonotonicCurrentProofRef = &replication, &quorum, &failover, &monotonic
	}
	bundle, err = SealProductionProofBundleV2(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return bundle, &resourceReader{set: set, handles: handleMap}
}
