package contract

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const MCPConnectionFactContractVersionV2 = "praxis.tool-mcp.mcp-connection-fact/v2"

type MCPConnectionFactRefV2 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r MCPConnectionFactRefV2) Validate() error {
	if ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return invalid("MCP Connection Fact Ref V2 is invalid")
	}
	return nil
}

type MCPConnectionFactExactReaderV2 interface {
	InspectMCPConnectionFactV2(context.Context, MCPConnectionFactRefV2) (MCPConnectionFactV2, error)
}

type MCPConnectionFactCurrentReaderV2 interface {
	InspectCurrentMCPConnectionFactV2(context.Context, MCPConnectionFactRefV2) (MCPConnectionFactV2, error)
}

// MCPConnectionFactV2 is Tool/MCP Owner authority. Coordinate.Session is the
// Praxis Session identity. ProviderSessionID is an optional Provider
// observation and never participates in the stable identity.
type MCPConnectionFactV2 struct {
	ContractVersion    string                            `json:"contract_version"`
	Ref                MCPConnectionFactRefV2            `json:"ref"`
	Owner              runtimeports.EffectOwnerRefV2     `json:"owner"`
	Coordinate         MCPConnectionCoordinateV1         `json:"coordinate"`
	Intent             ObjectRef                         `json:"intent"`
	TransportConfig    MCPTransportConfigRefV1           `json:"transport_config"`
	Server             ObjectRef                         `json:"server"`
	ProtocolReceipt    MCPConnectProtocolReceiptRefV1    `json:"protocol_receipt"`
	ProviderTransport  runtimeports.ProviderBindingRefV2 `json:"provider_transport_binding"`
	Provider           runtimeports.ProviderBindingRefV2 `json:"provider_binding"`
	NegotiatedProtocol string                            `json:"negotiated_protocol"`
	ProviderSessionID  string                            `json:"provider_session_id,omitempty"`
	CreatedUnixNano    int64                             `json:"created_unix_nano"`
	ExpiresUnixNano    int64                             `json:"expires_unix_nano"`
}

func (f MCPConnectionFactV2) Validate() error {
	if f.ContractVersion != MCPConnectionFactContractVersionV2 || f.Ref.Validate() != nil || f.Owner.Role != runtimeports.OwnerSettlement || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(f.Owner.ComponentID)) != nil || f.Owner.ManifestDigest.Validate() != nil || f.Coordinate.Validate() != nil || f.Intent.Validate() != nil || f.TransportConfig.Validate() != nil || f.Server.Validate() != nil || f.ProtocolReceipt.Validate() != nil || f.ProviderTransport.Validate() != nil || f.Provider.Validate() != nil || !validProtocolVersion(f.NegotiatedProtocol) || len(f.ProviderSessionID) > 256 || strings.TrimSpace(f.ProviderSessionID) != f.ProviderSessionID || f.CreatedUnixNano <= 0 || f.ExpiresUnixNano <= f.CreatedUnixNano {
		return invalid("MCP Connection Fact V2 is incomplete")
	}
	if f.Server != f.Coordinate.Server || f.ProviderTransport == f.Provider || f.ProviderTransport.BindingSetID != f.Provider.BindingSetID || f.ProviderTransport.BindingSetRevision != f.Provider.BindingSetRevision || string(f.Owner.ComponentID) != string(f.Provider.ComponentID) || f.Owner.ManifestDigest != f.Provider.ManifestDigest {
		return conflict("MCP Connection Fact V2 bindings drifted")
	}
	id, err := DeriveMCPConnectionFactIDV2(f.Coordinate)
	if err != nil || id != f.Ref.ID {
		return conflict("MCP Connection Fact V2 ID drifted")
	}
	digest, err := f.ComputeDigest()
	if err != nil || digest != f.Ref.Digest {
		return conflict("MCP Connection Fact V2 digest drifted")
	}
	return nil
}

func (f MCPConnectionFactV2) ComputeDigest() (core.Digest, error) {
	f.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-connection-fact", MCPConnectionFactContractVersionV2, "MCPConnectionFactV2", f)
}

func SealMCPConnectionFactV2(f MCPConnectionFactV2) (MCPConnectionFactV2, error) {
	f.ContractVersion = MCPConnectionFactContractVersionV2
	id, err := DeriveMCPConnectionFactIDV2(f.Coordinate)
	if err != nil {
		return MCPConnectionFactV2{}, err
	}
	if f.Ref.ID != "" && f.Ref.ID != id {
		return MCPConnectionFactV2{}, conflict("supplied MCP Connection Fact V2 ID drifted")
	}
	f.Ref.ID, f.Ref.Revision = id, 1
	provided := f.Ref.Digest
	f.Ref.Digest = ""
	digest, err := f.ComputeDigest()
	if err != nil {
		return MCPConnectionFactV2{}, err
	}
	if provided != "" && provided != digest {
		return MCPConnectionFactV2{}, conflict("supplied MCP Connection Fact V2 digest drifted")
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

func DeriveMCPConnectionFactIDV2(coordinate MCPConnectionCoordinateV1) (string, error) {
	if coordinate.Validate() != nil {
		return "", invalid("MCP Connection Fact V2 identity coordinate is invalid")
	}
	return StableID("mcp-connection-fact-v2", coordinate.ID)
}
