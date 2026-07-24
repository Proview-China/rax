package testkit

import (
	"context"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

func RuntimeOperation(reservation contract.DomainReservation) runtimeports.OperationSubjectV3 {
	scope := runtimecore.ExecutionScope{
		Identity:       runtimecore.AgentIdentityRef{TenantID: runtimecore.TenantID(reservation.Lease.TenantID), ID: "identity-1", Epoch: 1},
		Lineage:        runtimecore.LineageRef{ID: "lineage-1", PlanDigest: RuntimeDigest("lineage-plan")},
		Instance:       runtimecore.InstanceRef{ID: runtimecore.AgentInstanceID(reservation.Lease.InstanceID), Epoch: runtimecore.Epoch(reservation.Lease.InstanceEpoch)},
		SandboxLease:   &runtimecore.SandboxLeaseRef{ID: runtimecore.SandboxLeaseID(reservation.Lease.LeaseID), Epoch: runtimecore.Epoch(reservation.Lease.LeaseEpoch)},
		AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		panic(err)
	}
	return runtimeports.OperationSubjectV3{
		Kind: "praxis.sandbox/operation", ExecutionScope: scope, ExecutionScopeDigest: scopeDigest,
		CustomOperationID: reservation.OperationID, SubjectRevision: 1,
		CurrentProjectionRef: "sandbox-operation-projection", CurrentProjectionDigest: RuntimeDigest("operation-projection"), CurrentProjectionRevision: 1,
	}
}

type GenerationReader struct {
	Fact runtimeports.GenerationBindingAssociationFactV1
	Err  error
}

func (r *GenerationReader) AssociateGenerationBindingV1(context.Context, runtimeports.GenerationBindingAssociationCandidateV1) (runtimeports.GenerationBindingAssociationFactV1, error) {
	return runtimeports.GenerationBindingAssociationFactV1{}, r.Err
}

func (r *GenerationReader) InspectCurrentGenerationBindingAssociationV1(context.Context, string) (runtimeports.GenerationBindingAssociationFactV1, error) {
	if r.Err != nil {
		return runtimeports.GenerationBindingAssociationFactV1{}, r.Err
	}
	return r.Fact, nil
}

func GenerationAssociation(operation runtimeports.OperationSubjectV3, expires time.Time) runtimeports.GenerationBindingAssociationFactV1 {
	component := runtimeports.GenerationComponentManifestRefV1{ComponentID: "praxis.sandbox/provider", ManifestDigest: RuntimeDigest("provider-manifest"), ArtifactDigest: RuntimeDigest("provider-artifact")}
	generation, err := runtimeports.SealGenerationCurrentProjectionV1(runtimeports.GenerationCurrentProjectionV1{
		Generation:         runtimeports.GenerationArtifactRefV1{ID: "generation-1", Revision: 1, Digest: RuntimeDigest("generation"), InputDigest: RuntimeDigest("generation-input"), ManifestDigest: RuntimeDigest("assembly-manifest"), GraphDigest: RuntimeDigest("generation-graph"), CatalogDigest: RuntimeDigest("generation-catalog")},
		ComponentManifests: []runtimeports.GenerationComponentManifestRefV1{component},
		Extension:          runtimeports.GenerationGovernanceExtensionRefV1{Kind: "praxis.harness/assembly-generation", Contract: runtimeports.SchemaRefV2{Namespace: "praxis.harness", Name: "assembly-generation", Version: "1.0.0", MediaType: "application/json", ContentDigest: RuntimeDigest("generation-schema")}, Digest: RuntimeDigest("generation-extension")},
		State:              runtimeports.GenerationCurrentSealedV1, Current: true, Watermark: 1, ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	binding, err := runtimeports.SealGenerationBindingSetCurrentProjectionV1(runtimeports.GenerationBindingSetCurrentProjectionV1{
		BindingSetID: "binding-set-1", BindingSetRevision: 1, BindingSetDigest: RuntimeDigest("binding-set"), BindingSetSemanticDigest: RuntimeDigest("binding-semantic"),
		PlanDigest: RuntimeDigest("binding-plan"), GovernanceDigest: RuntimeDigest("binding-governance"), ComponentManifestSetDigest: runtimeports.GenerationComponentManifestSetDigestV1(generation.ComponentManifests), CurrentnessDigest: RuntimeDigest("binding-currentness"),
		IssuedUnixNano: FixedNow.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	activationOperation := operation
	activationOperation.Kind = runtimeports.OperationScopeActivationV3
	activationOperation.CustomOperationID = ""
	activationOperation.ActivationAttemptID = "generation-activation"
	activationOperation.CurrentProjectionRef = "generation-activation-projection"
	activationOperation.CurrentProjectionDigest = RuntimeDigest("generation-activation-projection")
	activationDigest, _ := activationOperation.DigestV3()
	activation, err := runtimeports.SealGenerationActivationCurrentProjectionV1(runtimeports.GenerationActivationCurrentProjectionV1{Operation: activationOperation, OperationDigest: activationDigest, Active: true, Watermark: 1, CurrentnessDigest: RuntimeDigest("generation-activation-currentness"), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		panic(err)
	}
	candidate, err := runtimeports.SealGenerationBindingAssociationCandidateV1(runtimeports.GenerationBindingAssociationCandidateV1{AssociationID: "generation-association", Generation: generation, Binding: binding, Activation: activation, RequestedExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		panic(err)
	}
	fact, err := runtimeports.SealGenerationBindingAssociationFactV1(runtimeports.GenerationBindingAssociationFactV1{ID: candidate.AssociationID, Revision: 1, State: runtimeports.GenerationBindingAssociationActiveV1, Candidate: candidate, CandidateDigest: candidate.Digest, CreatedUnixNano: FixedNow.UnixNano(), UpdatedUnixNano: FixedNow.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		panic(err)
	}
	return fact
}

func RuntimeDigest(value string) runtimecore.Digest {
	return runtimecore.Digest("sha256:" + Ref(value).Digest)
}
