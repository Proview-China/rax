package contract

import (
	"context"
	"strconv"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const MCPToolMappingContractVersionV1 = "praxis.tool-mcp.mcp-tool-mapping/v1"

type MCPToolMappingManifestRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r MCPToolMappingManifestRefV1) Validate() error {
	if r.ContractVersion != MCPToolMappingContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return invalid("MCP Tool Mapping Manifest Ref V1 is invalid")
	}
	return nil
}

func (r MCPToolMappingManifestRefV1) ObjectRef() ObjectRef {
	return ObjectRef{ID: r.ID, Revision: r.Revision, Digest: r.Digest}
}

// MCPToolMappingManifestV1 is an explicit, immutable semantic association.
// It never derives governance fields from provider annotations and grants no
// execution authority.
type MCPToolMappingManifestV1 struct {
	ContractVersion     string                        `json:"contract_version"`
	Ref                 MCPToolMappingManifestRefV1   `json:"ref"`
	Owner               core.OwnerRef                 `json:"owner"`
	Snapshot            ObjectRef                     `json:"snapshot"`
	Source              MCPToolObservationV2          `json:"source"`
	SourceMaterial      MCPToolDiscoveryMaterialRefV1 `json:"source_material"`
	Capability          ObjectRef                     `json:"capability"`
	Tool                ObjectRef                     `json:"tool"`
	MappingPolicy       runtimeports.NamespacedNameV2 `json:"mapping_policy"`
	MappingPolicyDigest core.Digest                   `json:"mapping_policy_digest"`
	CreatedUnixNano     int64                         `json:"created_unix_nano"`
}

func (m MCPToolMappingManifestV1) validateShape() error {
	if m.ContractVersion != MCPToolMappingContractVersionV1 || m.Ref.Validate() != nil || m.Owner.Validate() != nil || m.Snapshot.Validate() != nil || m.Source.Validate() != nil || m.SourceMaterial.Validate() != nil || m.Capability.Validate() != nil || m.Tool.Validate() != nil || runtimeports.ValidateNamespacedNameV2(m.MappingPolicy) != nil || m.MappingPolicyDigest.Validate() != nil || m.CreatedUnixNano <= 0 {
		return invalid("MCP Tool Mapping Manifest V1 is incomplete")
	}
	return nil
}

func (m MCPToolMappingManifestV1) Validate() error {
	if err := m.validateShape(); err != nil {
		return err
	}
	id, err := DeriveMCPToolMappingManifestIDV1(m.Snapshot, m.SourceMaterial, m.Tool)
	if err != nil || id != m.Ref.ID {
		return conflict("MCP Tool Mapping Manifest identity drifted")
	}
	digest, err := m.ComputeDigest()
	if err != nil || digest != m.Ref.Digest {
		return conflict("MCP Tool Mapping Manifest digest drifted")
	}
	return nil
}

func (m MCPToolMappingManifestV1) ValidateAgainst(snapshot MCPCapabilitySnapshotV3, material MCPToolDiscoveryMaterialV1, capability CapabilityDescriptor, tool ToolDescriptor) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if snapshot.Validate() != nil || snapshot.ObjectRef() != m.Snapshot || material.Validate() != nil || material.Ref != m.SourceMaterial || material.Source != m.Source || capability.Validate() != nil || tool.ValidateAgainst(capability) != nil {
		return conflict("MCP Tool Mapping source or target closure drifted")
	}
	capabilityRef := ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	toolRef := ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	if capabilityRef != m.Capability || toolRef != m.Tool || tool.Owner != m.Owner || tool.Mechanism != MechanismMCP || tool.ArtifactDigest != material.Ref.Digest || tool.InputSchema.ContentDigest != m.Source.InputSchemaDigest || tool.OutputSchema.ContentDigest != m.Source.OutputSchemaDigest || capability.InputSchema.ContentDigest != m.Source.InputSchemaDigest || capability.OutputSchema.ContentDigest != m.Source.OutputSchemaDigest {
		return conflict("MCP Tool Mapping descriptor semantics drifted")
	}
	if material.Connection.ID != snapshot.Connection.ID || material.Connection.Revision != snapshot.Connection.Revision || material.Connection.Digest != snapshot.Connection.Digest {
		return conflict("MCP Tool Mapping material Connection drifted")
	}
	found := false
	var pageReceipt ObjectRef
	for _, provenance := range snapshot.ToolMaterials {
		if provenance.Source.Name != m.Source.Name {
			continue
		}
		if provenance.Source != m.Source || provenance.Material != m.SourceMaterial {
			return conflict("MCP Tool Mapping Snapshot provenance drifted")
		}
		if found {
			return conflict("MCP Tool Mapping Snapshot provenance is ambiguous")
		}
		found, pageReceipt = true, provenance.PageReceipt
	}
	if !found {
		return core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "MCP Tool Mapping source material is absent from Snapshot V3")
	}
	pageFound := false
	for _, page := range snapshot.Pages {
		if page.ProtocolReceipt != pageReceipt {
			continue
		}
		if page.Namespace != runtimeports.MCPDiscoveryPageToolsNamespaceV1 || page.Command != material.Command {
			return conflict("MCP Tool Mapping Page provenance drifted")
		}
		pageFound = true
	}
	if !pageFound {
		return core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "MCP Tool Mapping Page provenance is absent")
	}
	return nil
}

func (m MCPToolMappingManifestV1) ComputeDigest() (core.Digest, error) {
	if m.ContractVersion != MCPToolMappingContractVersionV1 || m.Owner.Validate() != nil || m.Snapshot.Validate() != nil || m.Source.Validate() != nil || m.SourceMaterial.Validate() != nil || m.Capability.Validate() != nil || m.Tool.Validate() != nil || runtimeports.ValidateNamespacedNameV2(m.MappingPolicy) != nil || m.MappingPolicyDigest.Validate() != nil || m.CreatedUnixNano <= 0 {
		return "", invalid("MCP Tool Mapping Manifest source is invalid")
	}
	m.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-tool-mapping", MCPToolMappingContractVersionV1, "MCPToolMappingManifestV1", m)
}

func SealMCPToolMappingManifestV1(m MCPToolMappingManifestV1) (MCPToolMappingManifestV1, error) {
	m.ContractVersion = MCPToolMappingContractVersionV1
	id, err := DeriveMCPToolMappingManifestIDV1(m.Snapshot, m.SourceMaterial, m.Tool)
	if err != nil {
		return MCPToolMappingManifestV1{}, err
	}
	if m.Ref.ID != "" && m.Ref.ID != id {
		return MCPToolMappingManifestV1{}, conflict("supplied MCP Tool Mapping Manifest ID drifted")
	}
	m.Ref = MCPToolMappingManifestRefV1{ContractVersion: MCPToolMappingContractVersionV1, ID: id, Revision: 1}
	m.Ref.Digest, err = m.ComputeDigest()
	if err != nil {
		return MCPToolMappingManifestV1{}, err
	}
	return m, m.Validate()
}

func DeriveMCPToolMappingManifestIDV1(snapshot ObjectRef, material MCPToolDiscoveryMaterialRefV1, tool ObjectRef) (string, error) {
	if snapshot.Validate() != nil || material.Validate() != nil || tool.Validate() != nil {
		return "", invalid("MCP Tool Mapping identity source is invalid")
	}
	return StableID("mcp-tool-mapping", snapshot.ID, strconv.FormatUint(uint64(snapshot.Revision), 10), string(snapshot.Digest), material.ID, string(material.Digest), tool.ID, strconv.FormatUint(uint64(tool.Revision), 10), string(tool.Digest))
}

type MCPToolMappingAdmissionRequestV1 struct {
	ContractVersion                    string                      `json:"contract_version"`
	Mapping                            MCPToolMappingManifestRefV1 `json:"mapping"`
	ExpectedMappingRegistryRevision    core.Revision               `json:"expected_mapping_registry_revision"`
	ExpectedCapabilityRegistryRevision core.Revision               `json:"expected_capability_registry_revision"`
	ExpectedToolRegistryRevision       core.Revision               `json:"expected_tool_registry_revision"`
	RequestedExpiresUnixNano           int64                       `json:"requested_expires_unix_nano"`
}

func (r MCPToolMappingAdmissionRequestV1) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != MCPToolMappingContractVersionV1 || r.Mapping.Validate() != nil || r.ExpectedMappingRegistryRevision == 0 || r.ExpectedCapabilityRegistryRevision == 0 || r.ExpectedToolRegistryRevision == 0 || now.IsZero() {
		return invalid("MCP Tool Mapping Admission request is invalid")
	}
	if r.RequestedExpiresUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Tool Mapping Admission request expired")
	}
	return nil
}

type MCPToolMappingManifestExactReaderV1 interface {
	InspectMCPToolMappingManifestV1(context.Context, MCPToolMappingManifestRefV1) (MCPToolMappingManifestV1, error)
}
