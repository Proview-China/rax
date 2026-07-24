// Package release publishes Sandbox-owned component release candidates without
// claiming that an owner-local fixture is a production deployment.
package release

import (
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ReadinessContractVersionV1 = "praxis.sandbox/component-release-readiness/v1"
	ReadinessProjectionKindV1  = "SandboxProductionReadinessProjectionV1"
	readinessDigestDomainV1    = "praxis.sandbox.component-release-readiness"

	ComponentIDV1         runtimeports.ComponentIDV2    = "praxis/sandbox"
	ComponentKindV1       runtimeports.ComponentKindV2  = "praxis/sandbox"
	LifecycleCapabilityV1 runtimeports.CapabilityNameV2 = "praxis.sandbox/lifecycle-v4"
	LifecycleOwnerV1      runtimeports.CapabilityNameV2 = "praxis.sandbox/lifecycle-owner-v4"
	ExecutionCapabilityV1 runtimeports.CapabilityNameV2 = "praxis.sandbox/execution"
	ExecutionOwnerV1      runtimeports.CapabilityNameV2 = "praxis.sandbox/execution-owner-v1"
)

// SandboxProductionReadinessProjectionV1 is an exact, read-only projection of
// independently current production wiring. It contains no boolean shortcut:
// every required execution, governance, persistence and cleanup surface is an
// exact reference and its expiry participates in the sealed projection.
type SandboxProductionReadinessProjectionV1 struct {
	ContractVersion          string                       `json:"contract_version"`
	ComponentID              runtimeports.ComponentIDV2   `json:"component_id"`
	ReleaseID                string                       `json:"release_id"`
	Revision                 core.Revision                `json:"revision"`
	ArtifactDigest           core.Digest                  `json:"artifact_digest"`
	ManifestDigest           core.Digest                  `json:"manifest_digest"`
	CompositionRef           assemblycontract.ObjectRefV1 `json:"composition_ref"`
	DurableFactStoreRef      assemblycontract.ObjectRefV1 `json:"durable_fact_store_ref"`
	DurableCurrentStoreRef   assemblycontract.ObjectRefV1 `json:"durable_current_store_ref"`
	LeaseCurrentReaderRef    assemblycontract.ObjectRefV1 `json:"lease_current_reader_ref"`
	PolicyCurrentReaderRef   assemblycontract.ObjectRefV1 `json:"policy_current_reader_ref"`
	SandboxCurrentReaderRef  assemblycontract.ObjectRefV1 `json:"sandbox_current_reader_ref"`
	EnforcementGatewayRef    assemblycontract.ObjectRefV1 `json:"enforcement_gateway_ref"`
	EvidenceGovernanceRef    assemblycontract.ObjectRefV1 `json:"evidence_governance_ref"`
	SettlementCurrentRef     assemblycontract.ObjectRefV1 `json:"settlement_current_ref"`
	ProviderTransportRef     assemblycontract.ObjectRefV1 `json:"provider_transport_ref"`
	ProviderInspectRef       assemblycontract.ObjectRefV1 `json:"provider_inspect_ref"`
	DataPlaneJournalRef      assemblycontract.ObjectRefV1 `json:"data_plane_journal_ref"`
	CleanupOwnerRef          assemblycontract.ObjectRefV1 `json:"cleanup_owner_ref"`
	DeploymentAttestationRef assemblycontract.ObjectRefV1 `json:"deployment_attestation_ref"`
	CertificationFactRef     assemblycontract.ObjectRefV1 `json:"certification_fact_ref"`
	CheckedUnixNano          int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                        `json:"expires_unix_nano"`
	Digest                   core.Digest                  `json:"digest"`
}

func (p SandboxProductionReadinessProjectionV1) references() []assemblycontract.ObjectRefV1 {
	return []assemblycontract.ObjectRefV1{
		p.CompositionRef, p.DurableFactStoreRef, p.DurableCurrentStoreRef,
		p.LeaseCurrentReaderRef, p.PolicyCurrentReaderRef, p.SandboxCurrentReaderRef,
		p.EnforcementGatewayRef, p.EvidenceGovernanceRef, p.SettlementCurrentRef,
		p.ProviderTransportRef, p.ProviderInspectRef, p.DataPlaneJournalRef,
		p.CleanupOwnerRef, p.DeploymentAttestationRef, p.CertificationFactRef,
	}
}

func (p SandboxProductionReadinessProjectionV1) evidenceReferences() []assemblycontract.ObjectRefV1 {
	refs := p.references()
	return refs[:len(refs)-1]
}

func SandboxProductionReadinessDigestV1(value SandboxProductionReadinessProjectionV1) (core.Digest, error) {
	value.ContractVersion = ReadinessContractVersionV1
	value.Digest = ""
	return core.CanonicalJSONDigest(readinessDigestDomainV1, ReadinessContractVersionV1, ReadinessProjectionKindV1, value)
}

func SealSandboxProductionReadinessV1(value SandboxProductionReadinessProjectionV1) (SandboxProductionReadinessProjectionV1, error) {
	value.ContractVersion = ReadinessContractVersionV1
	value.Digest = runtimeports.EvidenceGenesisDigestV2
	digest, err := SandboxProductionReadinessDigestV1(value)
	if err != nil {
		return SandboxProductionReadinessProjectionV1{}, err
	}
	value.Digest = digest
	return value, value.ValidateCurrent(time.Unix(0, value.CheckedUnixNano))
}

func (p SandboxProductionReadinessProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != ReadinessContractVersionV1 || p.ComponentID != ComponentIDV1 || p.ReleaseID == "" || p.Revision == 0 || p.ArtifactDigest.Validate() != nil || p.ManifestDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "sandbox readiness identity is incomplete")
	}
	if now.IsZero() || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "sandbox readiness projection is future-dated or expired")
	}
	seen := make(map[string]struct{}, len(p.references()))
	for _, ref := range p.references() {
		if err := ref.Validate(); err != nil {
			return err
		}
		if _, exists := seen[ref.ID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "sandbox readiness roles alias one exact proof")
		}
		seen[ref.ID] = struct{}{}
	}
	digest, err := SandboxProductionReadinessDigestV1(p)
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "sandbox readiness digest drifted")
	}
	return nil
}

// PublicationRequestV1 identifies one immutable component release revision.
// Promotion from standalone to production therefore requires a higher revision.
type PublicationRequestV1 struct {
	ReleaseID       string                       `json:"release_id"`
	Revision        core.Revision                `json:"revision"`
	SourceRef       assemblycontract.ObjectRefV1 `json:"source_ref"`
	PublisherRef    assemblycontract.ObjectRefV1 `json:"publisher_ref"`
	TrustRef        assemblycontract.ObjectRefV1 `json:"trust_ref"`
	CertificationID string                       `json:"certification_id"`
	ArtifactDigest  core.Digest                  `json:"artifact_digest"`
	CreatedUnixNano int64                        `json:"created_unix_nano"`
	ExpiresUnixNano int64                        `json:"expires_unix_nano"`
}

func (r PublicationRequestV1) Validate(now time.Time) error {
	if r.ReleaseID == "" || r.Revision == 0 || r.CertificationID == "" || r.ArtifactDigest.Validate() != nil || r.SourceRef.Validate() != nil || r.PublisherRef.Validate() != nil || r.TrustRef.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "sandbox release publication request is incomplete")
	}
	if now.IsZero() || r.CreatedUnixNano <= 0 || r.ExpiresUnixNano <= r.CreatedUnixNano || now.UnixNano() < r.CreatedUnixNano || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "sandbox release validity window is future-dated or expired")
	}
	return nil
}

// PublicationResultV1 deliberately separates an assembly candidate from an
// independently certified production release.
type PublicationResultV1 struct {
	Release         assemblercontract.ComponentReleaseV1
	ProductionReady bool
	Readiness       *SandboxProductionReadinessProjectionV1
}
