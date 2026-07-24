package production

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/release"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ProofBundleReaderV2 interface {
	InspectProductionProofBundleV2(context.Context, string, core.Revision) (ProductionProofBundleV2, error)
}

// ReadinessVerifierV2 is an additive production reader for the existing
// release Publisher. It never creates provider resources or external facts.
type ReadinessVerifierV2 struct {
	bundles   ProofBundleReaderV2
	resources runtimeports.ResourceCurrentReaderV1
	clock     func() time.Time
}

func NewReadinessVerifierV2(bundles ProofBundleReaderV2, resources runtimeports.ResourceCurrentReaderV1, clock func() time.Time) (*ReadinessVerifierV2, error) {
	if nilLike(bundles) || nilLike(resources) || nilLike(clock) {
		return nil, invalid("production readiness verifier dependencies are incomplete")
	}
	return &ReadinessVerifierV2{bundles: bundles, resources: resources, clock: clock}, nil
}

func (v *ReadinessVerifierV2) InspectMemoryKnowledgeProductionReadinessV1(ctx context.Context, releaseID string, revision core.Revision) (release.ProductionReadinessProjectionV1, error) {
	if v == nil || ctx == nil {
		return release.ProductionReadinessProjectionV1{}, invalid("production readiness verifier or context is nil")
	}
	if err := ctx.Err(); err != nil {
		return release.ProductionReadinessProjectionV1{}, err
	}
	s1, err := v.bundles.InspectProductionProofBundleV2(ctx, releaseID, revision)
	if err != nil {
		return release.ProductionReadinessProjectionV1{}, err
	}
	first := v.clock()
	if s1.ReleaseID != releaseID || s1.Revision != revision {
		return release.ProductionReadinessProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "production proof targets another release")
	}
	if err := s1.ValidateCurrent(first); err != nil {
		return release.ProductionReadinessProjectionV1{}, err
	}
	if err := v.inspectResources(ctx, s1, first); err != nil {
		return release.ProductionReadinessProjectionV1{}, err
	}
	s2, err := v.bundles.InspectProductionProofBundleV2(ctx, releaseID, revision)
	if err != nil {
		return release.ProductionReadinessProjectionV1{}, err
	}
	second := v.clock()
	if second.IsZero() || second.Before(first) {
		return release.ProductionReadinessProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "production verifier clock regressed")
	}
	if s2.Digest != s1.Digest || !reflect.DeepEqual(s2, s1) {
		return release.ProductionReadinessProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "production proof drifted between S1 and S2")
	}
	if err := s2.ValidateCurrent(second); err != nil {
		return release.ProductionReadinessProjectionV1{}, err
	}
	return mapReadiness(s2)
}

func (v *ReadinessVerifierV2) inspectResources(ctx context.Context, bundle ProductionProofBundleV2, now time.Time) error {
	set, err := v.resources.InspectResourceBindingSetCurrentV1(ctx, bundle.ResourceBindingSetRef)
	if err != nil {
		return err
	}
	if err := set.ValidateCurrent(bundle.ResourceBindingSetRef, now); err != nil {
		return err
	}
	expected := []runtimeports.ResourceHandleRefV1{bundle.MemoryFactStoreRef, bundle.MemoryContentStoreRef, bundle.KnowledgeFactStoreRef, bundle.KnowledgeContentStoreRef}
	found := map[runtimeports.ResourceHandleRefV1]bool{}
	for _, binding := range set.Bindings {
		for _, ref := range expected {
			if binding.Handle == ref {
				found[ref] = true
			}
		}
	}
	for _, ref := range expected {
		if !found[ref] {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "durable resource is absent from exact BindingSet")
		}
		current, err := v.resources.InspectResourceHandleCurrentV1(ctx, ref)
		if err != nil {
			return err
		}
		if err := current.ValidateCurrent(ref, now); err != nil {
			return err
		}
	}
	return nil
}

func mapReadiness(bundle ProductionProofBundleV2) (release.ProductionReadinessProjectionV1, error) {
	value := release.ProductionReadinessProjectionV1{
		ReleaseID:                       bundle.ReleaseID,
		Revision:                        bundle.Revision,
		ArtifactDigest:                  bundle.ArtifactDigest,
		ManifestDigest:                  bundle.ManifestDigest,
		DurableMemoryFactStoreRef:       objectRef(bundle.MemoryFactStoreRef.ID, bundle.MemoryFactStoreRef.Revision, bundle.MemoryFactStoreRef.Digest),
		DurableMemoryContentStoreRef:    objectRef(bundle.MemoryContentStoreRef.ID, bundle.MemoryContentStoreRef.Revision, bundle.MemoryContentStoreRef.Digest),
		DurableKnowledgeFactStoreRef:    objectRef(bundle.KnowledgeFactStoreRef.ID, bundle.KnowledgeFactStoreRef.Revision, bundle.KnowledgeFactStoreRef.Digest),
		DurableKnowledgeContentStoreRef: objectRef(bundle.KnowledgeContentStoreRef.ID, bundle.KnowledgeContentStoreRef.Revision, bundle.KnowledgeContentStoreRef.Digest),
		AuthorityPolicyCurrentRef:       currentObjectRef(bundle.AuthorityPolicyCurrentRef),
		CredentialCurrentRef:            currentObjectRef(bundle.CredentialCurrentRef),
		RetrievalIndexCurrentRef:        currentObjectRef(bundle.RetrievalIndexCurrentRef),
		ContextSourceCurrentRef:         currentObjectRef(bundle.ContextSourceCurrentRef),
		SettlementCurrentRef:            currentObjectRef(bundle.SettlementCurrentRef),
		PurgeEffectRef:                  currentObjectRef(bundle.PurgeEffectCurrentRef),
		CleanupOwnerRef:                 currentObjectRef(bundle.CleanupOwnerCurrentRef),
		DeploymentAttestationRef:        currentObjectRef(bundle.DeploymentAttestationRef),
		CertificationFactRef:            currentObjectRef(bundle.CertificationFactRef),
		CheckedUnixNano:                 bundle.CheckedUnixNano,
		ExpiresUnixNano:                 bundle.ExpiresUnixNano,
	}
	return release.SealProductionV1(value)
}

func objectRef(id string, revision core.Revision, digest core.Digest) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: revision, Digest: digest}
}

func currentObjectRef(ref runtimeports.OwnerCurrentRefV1) assemblycontract.ObjectRefV1 {
	return objectRef(ref.ID, ref.Revision, ref.Digest)
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

var _ release.ProductionReadinessReaderV1 = (*ReadinessVerifierV2)(nil)
