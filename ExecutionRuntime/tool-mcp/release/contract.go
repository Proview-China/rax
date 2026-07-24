// Package release publishes Tool and MCP component release candidates without
// upgrading owner-local software evidence into a production claim.
package release

import (
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	LocalReadinessContractVersionV1                                    = "praxis.tool-mcp/component-local-readiness/v1"
	ProductionReadinessContractVersionV1                               = "praxis.tool-mcp/component-production-readiness/v1"
	ComponentIDV1                        runtimeports.ComponentIDV2    = "praxis/tool-mcp"
	ComponentKindV1                      runtimeports.ComponentKindV2  = "praxis/tool-mcp"
	ToolActionCapabilityV1               runtimeports.CapabilityNameV2 = "praxis.tool/single-call-action-v2"
	MCPCallCapabilityV1                  runtimeports.CapabilityNameV2 = "praxis.mcp/governed-tools-call-v1"
)

type LocalReadinessProjectionV1 struct {
	ContractVersion              string                       `json:"contract_version"`
	ComponentID                  runtimeports.ComponentIDV2   `json:"component_id"`
	ReleaseID                    string                       `json:"release_id"`
	Revision                     core.Revision                `json:"revision"`
	ArtifactDigest               core.Digest                  `json:"artifact_digest"`
	G6AP4CurrentRef              assemblycontract.ObjectRefV1 `json:"g6a_p4_current_ref"`
	SurfaceCurrentRef            assemblycontract.ObjectRefV1 `json:"surface_current_ref"`
	SurfaceBindingCurrentRef     assemblycontract.ObjectRefV1 `json:"surface_binding_current_ref"`
	InputContractCurrentRef      assemblycontract.ObjectRefV1 `json:"input_contract_current_ref"`
	ControlledProviderAdapterRef assemblycontract.ObjectRefV1 `json:"controlled_provider_adapter_ref"`
	MCPDiscoveryRef              assemblycontract.ObjectRefV1 `json:"mcp_discovery_ref"`
	MCPLifecycleRef              assemblycontract.ObjectRefV1 `json:"mcp_lifecycle_ref"`
	OfficialSDKCallRef           assemblycontract.ObjectRefV1 `json:"official_sdk_call_ref"`
	CheckedUnixNano              int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano              int64                        `json:"expires_unix_nano"`
	Digest                       core.Digest                  `json:"digest"`
}

func (p LocalReadinessProjectionV1) refs() []assemblycontract.ObjectRefV1 {
	return []assemblycontract.ObjectRefV1{p.G6AP4CurrentRef, p.SurfaceCurrentRef, p.SurfaceBindingCurrentRef, p.InputContractCurrentRef, p.ControlledProviderAdapterRef, p.MCPDiscoveryRef, p.MCPLifecycleRef, p.OfficialSDKCallRef}
}

func LocalReadinessDigestV1(p LocalReadinessProjectionV1) (core.Digest, error) {
	p.ContractVersion = LocalReadinessContractVersionV1
	p.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.component-local-readiness", LocalReadinessContractVersionV1, "LocalReadinessProjectionV1", p)
}

func SealLocalReadinessV1(p LocalReadinessProjectionV1) (LocalReadinessProjectionV1, error) {
	p.ContractVersion = LocalReadinessContractVersionV1
	p.Digest = runtimeports.EvidenceGenesisDigestV2
	d, err := LocalReadinessDigestV1(p)
	if err != nil {
		return LocalReadinessProjectionV1{}, err
	}
	p.Digest = d
	return p, p.ValidateCurrent(time.Unix(0, p.CheckedUnixNano))
}

func (p LocalReadinessProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != LocalReadinessContractVersionV1 || p.ComponentID != ComponentIDV1 || p.ReleaseID == "" || p.Revision == 0 || p.ArtifactDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "tool/MCP local readiness identity is incomplete")
	}
	if err := validateWindow(p.CheckedUnixNano, p.ExpiresUnixNano, now); err != nil {
		return err
	}
	if err := validateDistinctRefs(p.refs()); err != nil {
		return err
	}
	d, err := LocalReadinessDigestV1(p)
	if err != nil || d != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "tool/MCP local readiness digest drifted")
	}
	return nil
}

type ProductionReadinessProjectionV1 struct {
	ContractVersion          string                       `json:"contract_version"`
	ComponentID              runtimeports.ComponentIDV2   `json:"component_id"`
	ReleaseID                string                       `json:"release_id"`
	Revision                 core.Revision                `json:"revision"`
	ArtifactDigest           core.Digest                  `json:"artifact_digest"`
	ManifestDigest           core.Digest                  `json:"manifest_digest"`
	DurableActionStoreRef    assemblycontract.ObjectRefV1 `json:"durable_action_store_ref"`
	DurableBindingStoreRef   assemblycontract.ObjectRefV1 `json:"durable_binding_store_ref"`
	DurableSurfaceStoreRef   assemblycontract.ObjectRefV1 `json:"durable_surface_store_ref"`
	DurableMCPStoreRef       assemblycontract.ObjectRefV1 `json:"durable_mcp_store_ref"`
	CredentialCurrentRef     assemblycontract.ObjectRefV1 `json:"credential_current_ref"`
	ProviderTransportRef     assemblycontract.ObjectRefV1 `json:"provider_transport_ref"`
	ProviderCurrentRef       assemblycontract.ObjectRefV1 `json:"provider_current_ref"`
	ControlledActualPointRef assemblycontract.ObjectRefV1 `json:"controlled_actual_point_ref"`
	EvidenceGovernanceRef    assemblycontract.ObjectRefV1 `json:"evidence_governance_ref"`
	SettlementCurrentRef     assemblycontract.ObjectRefV1 `json:"settlement_current_ref"`
	MCPLifecycleCurrentRef   assemblycontract.ObjectRefV1 `json:"mcp_lifecycle_current_ref"`
	MCPInspectRef            assemblycontract.ObjectRefV1 `json:"mcp_inspect_ref"`
	CleanupOwnerRef          assemblycontract.ObjectRefV1 `json:"cleanup_owner_ref"`
	DeploymentAttestationRef assemblycontract.ObjectRefV1 `json:"deployment_attestation_ref"`
	CertificationFactRef     assemblycontract.ObjectRefV1 `json:"certification_fact_ref"`
	CheckedUnixNano          int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                        `json:"expires_unix_nano"`
	Digest                   core.Digest                  `json:"digest"`
}

func (p ProductionReadinessProjectionV1) refs() []assemblycontract.ObjectRefV1 {
	return []assemblycontract.ObjectRefV1{p.DurableActionStoreRef, p.DurableBindingStoreRef, p.DurableSurfaceStoreRef, p.DurableMCPStoreRef, p.CredentialCurrentRef, p.ProviderTransportRef, p.ProviderCurrentRef, p.ControlledActualPointRef, p.EvidenceGovernanceRef, p.SettlementCurrentRef, p.MCPLifecycleCurrentRef, p.MCPInspectRef, p.CleanupOwnerRef, p.DeploymentAttestationRef, p.CertificationFactRef}
}
func (p ProductionReadinessProjectionV1) evidenceRefs() []assemblycontract.ObjectRefV1 {
	return p.refs()[:len(p.refs())-1]
}
func ProductionReadinessDigestV1(p ProductionReadinessProjectionV1) (core.Digest, error) {
	p.ContractVersion = ProductionReadinessContractVersionV1
	p.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.component-production-readiness", ProductionReadinessContractVersionV1, "ProductionReadinessProjectionV1", p)
}
func SealProductionReadinessV1(p ProductionReadinessProjectionV1) (ProductionReadinessProjectionV1, error) {
	p.ContractVersion = ProductionReadinessContractVersionV1
	p.Digest = runtimeports.EvidenceGenesisDigestV2
	d, e := ProductionReadinessDigestV1(p)
	if e != nil {
		return ProductionReadinessProjectionV1{}, e
	}
	p.Digest = d
	return p, p.ValidateCurrent(time.Unix(0, p.CheckedUnixNano))
}
func (p ProductionReadinessProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != ProductionReadinessContractVersionV1 || p.ComponentID != ComponentIDV1 || p.ReleaseID == "" || p.Revision == 0 || p.ArtifactDigest.Validate() != nil || p.ManifestDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "tool/MCP production readiness identity is incomplete")
	}
	if e := validateWindow(p.CheckedUnixNano, p.ExpiresUnixNano, now); e != nil {
		return e
	}
	if e := validateDistinctRefs(p.refs()); e != nil {
		return e
	}
	d, e := ProductionReadinessDigestV1(p)
	if e != nil || d != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "tool/MCP production readiness digest drifted")
	}
	return nil
}

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
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "tool/MCP publication request is incomplete")
	}
	return validateWindow(r.CreatedUnixNano, r.ExpiresUnixNano, now)
}

type PublicationResultV1 struct {
	Release             assemblercontract.ComponentReleaseV1
	LocalReady          bool
	ProductionReady     bool
	LocalReadiness      *LocalReadinessProjectionV1
	ProductionReadiness *ProductionReadinessProjectionV1
}

func validateWindow(checked, expires int64, now time.Time) error {
	if now.IsZero() || checked <= 0 || expires <= checked || now.UnixNano() < checked {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "tool/MCP readiness clock is invalid")
	}
	if !now.Before(time.Unix(0, expires)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "tool/MCP readiness expired")
	}
	return nil
}
func validateDistinctRefs(refs []assemblycontract.ObjectRefV1) error {
	seen := map[string]struct{}{}
	for _, r := range refs {
		if e := r.Validate(); e != nil {
			return e
		}
		if _, ok := seen[r.ID]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "tool/MCP readiness roles alias one proof")
		}
		seen[r.ID] = struct{}{}
	}
	return nil
}
