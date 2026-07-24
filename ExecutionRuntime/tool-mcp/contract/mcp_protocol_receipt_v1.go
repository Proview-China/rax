package contract

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	MCPProtocolReceiptContractVersionV1 = "praxis.tool-mcp.mcp-protocol-receipt/v1"
	MaxMCPProtocolReceiptBytesV1        = 1 << 20
)

type MCPProtocolReceiptRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

type MCPProtocolReceiptExactReaderV1 interface {
	InspectMCPProtocolReceiptV1(context.Context, MCPProtocolReceiptRefV1) (MCPProtocolReceiptV1, error)
}

// MCPProtocolReceiptIDReaderV1 returns the immutable receipt selected by the
// Runtime Observation's exact ProviderOperationRef. Callers must still bind
// the returned full Ref/digest to that Observation and the original command.
type MCPProtocolReceiptIDReaderV1 interface {
	InspectMCPProtocolReceiptByIDV1(context.Context, string) (MCPProtocolReceiptV1, error)
}

func (r MCPProtocolReceiptRefV1) Validate() error {
	if ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return invalid("MCP Protocol Receipt Ref is invalid")
	}
	return nil
}

// MCPProtocolReceiptV1 is an external Provider observation. It is not a Tool
// DomainResult, Runtime Settlement, Review Verdict, or evidence of authority.
type MCPProtocolReceiptV1 struct {
	ContractVersion   string                                                        `json:"contract_version"`
	Ref               MCPProtocolReceiptRefV1                                       `json:"ref"`
	Command           MCPExecutionCommandRefV1                                      `json:"command"`
	StableKeyDigest   core.Digest                                                   `json:"stable_key_digest"`
	AdmissionReceipt  runtimeports.ControlledOperationProviderAdmissionReceiptRefV2 `json:"admission_receipt"`
	JSONRPCRequestID  string                                                        `json:"jsonrpc_request_id"`
	ToolError         bool                                                          `json:"tool_error"`
	CanonicalResponse []byte                                                        `json:"canonical_response"`
	ResponseDigest    core.Digest                                                   `json:"response_digest"`
	ObservedUnixNano  int64                                                         `json:"observed_unix_nano"`
}

func (r MCPProtocolReceiptV1) Validate() error {
	if r.ContractVersion != MCPProtocolReceiptContractVersionV1 || r.Ref.Validate() != nil || r.Command.Validate() != nil || r.StableKeyDigest.Validate() != nil || r.AdmissionReceipt.Validate() != nil || !r.AdmissionReceipt.Admitted || r.AdmissionReceipt.StableKeyDigest != r.StableKeyDigest || ValidateStableID(r.JSONRPCRequestID) != nil || len(r.CanonicalResponse) == 0 || len(r.CanonicalResponse) > MaxMCPProtocolReceiptBytesV1 || r.ResponseDigest.Validate() != nil || r.ObservedUnixNano <= 0 {
		return invalid("MCP Protocol Receipt is invalid")
	}
	if core.DigestBytes(r.CanonicalResponse) != r.ResponseDigest {
		return conflict("MCP Protocol Receipt response digest drifted")
	}
	id, err := DeriveMCPProtocolReceiptIDV1(r.Command, r.StableKeyDigest)
	if err != nil || id != r.Ref.ID {
		return conflict("MCP Protocol Receipt ID drifted")
	}
	digest, err := r.ComputeDigest()
	if err != nil || digest != r.Ref.Digest {
		return conflict("MCP Protocol Receipt digest drifted")
	}
	return nil
}

func (r MCPProtocolReceiptV1) ComputeDigest() (core.Digest, error) {
	r.CanonicalResponse = append([]byte(nil), r.CanonicalResponse...)
	r.Ref.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp-protocol-receipt", MCPProtocolReceiptContractVersionV1, "MCPProtocolReceiptV1", r)
}

func SealMCPProtocolReceiptV1(r MCPProtocolReceiptV1) (MCPProtocolReceiptV1, error) {
	r.CanonicalResponse = append([]byte(nil), r.CanonicalResponse...)
	r.ContractVersion = MCPProtocolReceiptContractVersionV1
	id, err := DeriveMCPProtocolReceiptIDV1(r.Command, r.StableKeyDigest)
	if err != nil {
		return MCPProtocolReceiptV1{}, err
	}
	if r.Ref.ID != "" && r.Ref.ID != id {
		return MCPProtocolReceiptV1{}, conflict("supplied MCP Protocol Receipt ID drifted")
	}
	r.Ref.ID, r.Ref.Revision = id, 1
	r.ResponseDigest = core.DigestBytes(r.CanonicalResponse)
	provided := r.Ref.Digest
	r.Ref.Digest = ""
	digest, err := r.ComputeDigest()
	if err != nil {
		return MCPProtocolReceiptV1{}, err
	}
	if provided != "" && provided != digest {
		return MCPProtocolReceiptV1{}, conflict("supplied MCP Protocol Receipt digest drifted")
	}
	r.Ref.Digest = digest
	return r, r.Validate()
}

func DeriveMCPProtocolReceiptIDV1(command MCPExecutionCommandRefV1, stable core.Digest) (string, error) {
	if command.Validate() != nil || stable.Validate() != nil {
		return "", invalid("MCP Protocol Receipt identity inputs are invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp-protocol-receipt", MCPProtocolReceiptContractVersionV1, "MCPProtocolReceiptIdentityV1", struct {
		Command MCPExecutionCommandRefV1 `json:"command"`
		Stable  core.Digest              `json:"stable"`
	}{command, stable})
	if err != nil {
		return "", err
	}
	return "mcp-protocol-receipt-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func CloneMCPProtocolReceiptV1(r MCPProtocolReceiptV1) MCPProtocolReceiptV1 {
	r.CanonicalResponse = append([]byte(nil), r.CanonicalResponse...)
	return r
}
