package contract

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	MCPDiscoveryPageApplyContractVersionV1          = "praxis.tool-mcp.mcp-discovery-page-apply/v1"
	MCPDiscoveryPageAppliedCurrentContractVersionV1 = "praxis.tool-mcp.mcp-discovery-page-applied-current/v1"
	MaxMCPDiscoveryPageAppliedCurrentTTLV1          = 30 * time.Second
)

type MCPDiscoveryPageApplySettlementFactV1 struct {
	ContractVersion string                                          `json:"contract_version"`
	Ref             ObjectRef                                       `json:"ref"`
	Command         ObjectRef                                       `json:"command"`
	DomainResult    ObjectRef                                       `json:"domain_result"`
	Inspection      runtimeports.OperationInspectionSettlementRefV4 `json:"inspection"`
	Owner           runtimeports.EffectOwnerRefV2                   `json:"owner"`
	AppliedUnixNano int64                                           `json:"applied_unix_nano"`
}

func (f MCPDiscoveryPageApplySettlementFactV1) Validate() error {
	if f.ContractVersion != MCPDiscoveryPageApplyContractVersionV1 || f.Ref.Validate() != nil || f.Command.Validate() != nil || f.DomainResult.Validate() != nil || f.AppliedUnixNano <= 0 || f.Inspection.Validate(time.Unix(0, f.AppliedUnixNano)) != nil || validateEffectOwner(f.Owner) != nil || f.Inspection.Owner != f.Owner {
		return invalid("MCP Discovery Page ApplySettlement V1 is invalid")
	}
	id, err := DeriveMCPDiscoveryPageApplySettlementIDV1(f.Command, f.DomainResult, f.Inspection.Settlement)
	if err != nil || id != f.Ref.ID {
		return conflict("MCP Discovery Page ApplySettlement identity drifted")
	}
	digest, err := f.ComputeDigest()
	if err != nil || digest != f.Ref.Digest {
		return conflict("MCP Discovery Page ApplySettlement digest drifted")
	}
	return nil
}
func (f MCPDiscoveryPageApplySettlementFactV1) ComputeDigest() (core.Digest, error) {
	f.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-discovery-page-apply", MCPDiscoveryPageApplyContractVersionV1, "MCPDiscoveryPageApplySettlementFactV1", f)
}
func SealMCPDiscoveryPageApplySettlementFactV1(f MCPDiscoveryPageApplySettlementFactV1) (MCPDiscoveryPageApplySettlementFactV1, error) {
	f.ContractVersion = MCPDiscoveryPageApplyContractVersionV1
	id, err := DeriveMCPDiscoveryPageApplySettlementIDV1(f.Command, f.DomainResult, f.Inspection.Settlement)
	if err != nil {
		return f, err
	}
	f.Ref = ObjectRef{ID: id, Revision: 1}
	f.Ref.Digest, err = f.ComputeDigest()
	if err != nil {
		return f, err
	}
	return f, f.Validate()
}
func DeriveMCPDiscoveryPageApplySettlementIDV1(command, domain ObjectRef, settlement runtimeports.OperationSettlementRefV4) (string, error) {
	if command.Validate() != nil || domain.Validate() != nil || settlement.Validate() != nil {
		return "", invalid("MCP Discovery Page Apply identity input is invalid")
	}
	return StableID("mcp-discovery-page-apply", command.ID, domain.ID, settlement.ID)
}

type MCPDiscoveryPageAppliedCurrentProjectionV1 struct {
	ContractVersion  string                        `json:"contract_version"`
	Command          ObjectRef                     `json:"command"`
	ProtocolReceipt  ObjectRef                     `json:"protocol_receipt"`
	DomainResult     ObjectRef                     `json:"domain_result"`
	ApplySettlement  ObjectRef                     `json:"apply_settlement"`
	Namespace        runtimeports.NamespacedNameV2 `json:"namespace"`
	PageOrdinal      uint32                        `json:"page_ordinal"`
	NextCursor       []byte                        `json:"next_cursor"`
	NextCursorDigest core.Digest                   `json:"next_cursor_digest"`
	Owner            runtimeports.EffectOwnerRefV2 `json:"owner"`
	CheckedUnixNano  int64                         `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                         `json:"expires_unix_nano"`
	Digest           core.Digest                   `json:"digest"`
}

func (p MCPDiscoveryPageAppliedCurrentProjectionV1) Validate(now time.Time) error {
	if p.ContractVersion != MCPDiscoveryPageAppliedCurrentContractVersionV1 || p.Command.Validate() != nil || p.ProtocolReceipt.Validate() != nil || p.DomainResult.Validate() != nil || p.ApplySettlement.Validate() != nil || !runtimeports.IsMCPDiscoveryPageNamespaceV1(p.Namespace) || len(p.NextCursor) > MaxMCPDiscoveryCursorBytesV1 || p.NextCursorDigest != core.DigestBytes(p.NextCursor) || validateEffectOwner(p.Owner) != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || time.Duration(p.ExpiresUnixNano-p.CheckedUnixNano) > MaxMCPDiscoveryPageAppliedCurrentTTLV1 || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Discovery Page applied current is invalid or expired")
	}
	digest, err := p.ComputeDigest()
	if err != nil || digest != p.Digest {
		return conflict("MCP Discovery Page applied current digest drifted")
	}
	return nil
}
func (p MCPDiscoveryPageAppliedCurrentProjectionV1) ComputeDigest() (core.Digest, error) {
	p.Digest = ""
	return Seal("praxis.tool-mcp.mcp-discovery-page-applied-current", MCPDiscoveryPageAppliedCurrentContractVersionV1, "MCPDiscoveryPageAppliedCurrentProjectionV1", p)
}
func SealMCPDiscoveryPageAppliedCurrentProjectionV1(p MCPDiscoveryPageAppliedCurrentProjectionV1, now time.Time) (MCPDiscoveryPageAppliedCurrentProjectionV1, error) {
	p.ContractVersion = MCPDiscoveryPageAppliedCurrentContractVersionV1
	p.NextCursor = append([]byte(nil), p.NextCursor...)
	p.NextCursorDigest = core.DigestBytes(p.NextCursor)
	p.Digest = ""
	digest, err := p.ComputeDigest()
	if err != nil {
		return p, err
	}
	p.Digest = digest
	return p, p.Validate(now)
}

type MCPDiscoveryPageApplyExactReaderV1 interface {
	InspectMCPDiscoveryPageApplySettlementV1(context.Context, ObjectRef) (MCPDiscoveryPageApplySettlementFactV1, error)
	InspectCurrentMCPDiscoveryPageAppliedV1(context.Context, ObjectRef, time.Duration) (MCPDiscoveryPageAppliedCurrentProjectionV1, error)
}
