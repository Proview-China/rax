package contract

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	MCPConnectProtocolReceiptContractVersionV1 = "praxis.tool-mcp.mcp-connect-protocol-receipt/v1"
	MaxMCPConnectInitializeReceiptBytesV1      = 1 << 20
)

type MCPConnectProtocolReceiptRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r MCPConnectProtocolReceiptRefV1) Validate() error {
	if ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return invalid("MCP Connect Protocol Receipt Ref is invalid")
	}
	return nil
}

type MCPConnectProtocolReceiptExactReaderV1 interface {
	InspectMCPConnectProtocolReceiptV1(context.Context, MCPConnectProtocolReceiptRefV1) (MCPConnectProtocolReceiptV1, error)
}

type MCPConnectProtocolReceiptIDReaderV1 interface {
	InspectMCPConnectProtocolReceiptByIDV1(context.Context, string) (MCPConnectProtocolReceiptV1, error)
}

// MCPConnectProtocolReceiptV1 is an immutable Provider observation. It is not
// an active Connection, Runtime Settlement, Review Verdict, or execution
// authority.
type MCPConnectProtocolReceiptV1 struct {
	ContractVersion    string                                                        `json:"contract_version"`
	Ref                MCPConnectProtocolReceiptRefV1                                `json:"ref"`
	Intent             ObjectRef                                                     `json:"intent"`
	TransportConfig    MCPTransportConfigRefV1                                       `json:"transport_config"`
	StableKeyDigest    core.Digest                                                   `json:"stable_key_digest"`
	AdmissionReceipt   runtimeports.ControlledOperationProviderAdmissionReceiptRefV2 `json:"admission_receipt"`
	TransportKind      runtimeports.NamespacedNameV2                                 `json:"transport_kind"`
	NegotiatedProtocol string                                                        `json:"negotiated_protocol"`
	ProviderSessionID  string                                                        `json:"provider_session_id,omitempty"`
	InitializeResponse []byte                                                        `json:"initialize_response"`
	ResponseDigest     core.Digest                                                   `json:"response_digest"`
	ObservedUnixNano   int64                                                         `json:"observed_unix_nano"`
}

func (r MCPConnectProtocolReceiptV1) Validate() error {
	if r.ContractVersion != MCPConnectProtocolReceiptContractVersionV1 || r.Ref.Validate() != nil || r.Intent.Validate() != nil || r.TransportConfig.Validate() != nil || r.StableKeyDigest.Validate() != nil || r.AdmissionReceipt.Validate() != nil || !r.AdmissionReceipt.Admitted || r.AdmissionReceipt.StableKeyDigest != r.StableKeyDigest || r.TransportKind != MCPTransportStdioV1 && r.TransportKind != MCPTransportStreamableHTTPV1 || !validProtocolVersion(r.NegotiatedProtocol) || len(r.ProviderSessionID) > 256 || strings.TrimSpace(r.ProviderSessionID) != r.ProviderSessionID || len(r.InitializeResponse) == 0 || len(r.InitializeResponse) > MaxMCPConnectInitializeReceiptBytesV1 || r.ResponseDigest.Validate() != nil || r.ObservedUnixNano <= 0 {
		return invalid("MCP Connect Protocol Receipt is invalid")
	}
	if core.DigestBytes(r.InitializeResponse) != r.ResponseDigest {
		return conflict("MCP Connect initialize response digest drifted")
	}
	id, err := DeriveMCPConnectProtocolReceiptIDV1(r.Intent, r.StableKeyDigest)
	if err != nil || id != r.Ref.ID {
		return conflict("MCP Connect Protocol Receipt ID drifted")
	}
	digest, err := r.ComputeDigest()
	if err != nil || digest != r.Ref.Digest {
		return conflict("MCP Connect Protocol Receipt digest drifted")
	}
	return nil
}

func (r MCPConnectProtocolReceiptV1) ComputeDigest() (core.Digest, error) {
	r.InitializeResponse = append([]byte(nil), r.InitializeResponse...)
	r.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-connect-protocol-receipt", MCPConnectProtocolReceiptContractVersionV1, "MCPConnectProtocolReceiptV1", r)
}

func SealMCPConnectProtocolReceiptV1(r MCPConnectProtocolReceiptV1) (MCPConnectProtocolReceiptV1, error) {
	r.InitializeResponse = append([]byte(nil), r.InitializeResponse...)
	r.ContractVersion = MCPConnectProtocolReceiptContractVersionV1
	id, err := DeriveMCPConnectProtocolReceiptIDV1(r.Intent, r.StableKeyDigest)
	if err != nil {
		return MCPConnectProtocolReceiptV1{}, err
	}
	if r.Ref.ID != "" && r.Ref.ID != id {
		return MCPConnectProtocolReceiptV1{}, conflict("supplied MCP Connect Protocol Receipt ID drifted")
	}
	r.Ref.ID, r.Ref.Revision = id, 1
	r.ResponseDigest = core.DigestBytes(r.InitializeResponse)
	provided := r.Ref.Digest
	r.Ref.Digest = ""
	digest, err := r.ComputeDigest()
	if err != nil {
		return MCPConnectProtocolReceiptV1{}, err
	}
	if provided != "" && provided != digest {
		return MCPConnectProtocolReceiptV1{}, conflict("supplied MCP Connect Protocol Receipt digest drifted")
	}
	r.Ref.Digest = digest
	return r, r.Validate()
}

func DeriveMCPConnectProtocolReceiptIDV1(intent ObjectRef, stable core.Digest) (string, error) {
	if intent.Validate() != nil || stable.Validate() != nil {
		return "", invalid("MCP Connect Protocol Receipt identity inputs are invalid")
	}
	digest, err := Seal("praxis.tool-mcp.mcp-connect-protocol-receipt", MCPConnectProtocolReceiptContractVersionV1, "MCPConnectProtocolReceiptIdentityV1", struct {
		Intent ObjectRef   `json:"intent"`
		Stable core.Digest `json:"stable_key_digest"`
	}{intent, stable})
	if err != nil {
		return "", err
	}
	return StableID("mcp-connect-receipt", string(digest))
}

func CloneMCPConnectProtocolReceiptV1(r MCPConnectProtocolReceiptV1) MCPConnectProtocolReceiptV1 {
	r.InitializeResponse = append([]byte(nil), r.InitializeResponse...)
	return r
}
