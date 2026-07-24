package contract

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	MCPConnectApplySettlementContractVersionV1 = "praxis.tool-mcp.mcp-connect-apply-settlement/v1"
	MCPConnectionAvailabilityContractVersionV1 = "praxis.tool-mcp.mcp-connection-availability/v1"
	MaxMCPConnectionAvailabilityTTLV1          = 30 * time.Second
)

// MCPConnectApplySettlementFactV1 is the Tool/MCP Owner commit after Runtime
// Settlement V4. Runtime only binds the authoritative DomainResult; this fact
// records the Tool/MCP Owner's exact application of that settlement.
type MCPConnectApplySettlementFactV1 struct {
	ContractVersion string                                          `json:"contract_version"`
	Ref             ObjectRef                                       `json:"ref"`
	Connection      MCPConnectionFactRefV2                          `json:"connection"`
	DomainResult    ObjectRef                                       `json:"domain_result"`
	Inspection      runtimeports.OperationInspectionSettlementRefV4 `json:"inspection"`
	Owner           runtimeports.EffectOwnerRefV2                   `json:"owner"`
	AppliedUnixNano int64                                           `json:"applied_unix_nano"`
}

func (f MCPConnectApplySettlementFactV1) Validate() error {
	if f.ContractVersion != MCPConnectApplySettlementContractVersionV1 || f.Ref.Validate() != nil || f.Connection.Validate() != nil || f.DomainResult.Validate() != nil || f.AppliedUnixNano <= 0 || f.Inspection.Validate(time.Unix(0, f.AppliedUnixNano)) != nil || validateEffectOwner(f.Owner) != nil || f.Inspection.Owner != f.Owner {
		return invalid("MCP Connect ApplySettlement V1 is invalid")
	}
	id, err := DeriveMCPConnectApplySettlementIDV1(f.Connection, f.DomainResult, f.Inspection.Settlement)
	if err != nil || id != f.Ref.ID {
		return conflict("MCP Connect ApplySettlement V1 identity drifted")
	}
	digest, err := f.ComputeDigest()
	if err != nil || digest != f.Ref.Digest {
		return conflict("MCP Connect ApplySettlement V1 digest drifted")
	}
	return nil
}

func (f MCPConnectApplySettlementFactV1) ComputeDigest() (core.Digest, error) {
	f.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-connect-apply-settlement", MCPConnectApplySettlementContractVersionV1, "MCPConnectApplySettlementFactV1", f)
}

func SealMCPConnectApplySettlementFactV1(f MCPConnectApplySettlementFactV1) (MCPConnectApplySettlementFactV1, error) {
	f.ContractVersion = MCPConnectApplySettlementContractVersionV1
	id, err := DeriveMCPConnectApplySettlementIDV1(f.Connection, f.DomainResult, f.Inspection.Settlement)
	if err != nil {
		return MCPConnectApplySettlementFactV1{}, err
	}
	if f.Ref.ID != "" && f.Ref.ID != id {
		return MCPConnectApplySettlementFactV1{}, conflict("supplied MCP Connect ApplySettlement V1 ID drifted")
	}
	f.Ref.ID, f.Ref.Revision = id, 1
	provided := f.Ref.Digest
	f.Ref.Digest = ""
	digest, err := f.ComputeDigest()
	if err != nil {
		return MCPConnectApplySettlementFactV1{}, err
	}
	if provided != "" && provided != digest {
		return MCPConnectApplySettlementFactV1{}, conflict("supplied MCP Connect ApplySettlement V1 digest drifted")
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

func DeriveMCPConnectApplySettlementIDV1(connection MCPConnectionFactRefV2, domain ObjectRef, settlement runtimeports.OperationSettlementRefV4) (string, error) {
	if connection.Validate() != nil || domain.Validate() != nil || settlement.Validate() != nil {
		return "", invalid("MCP Connect ApplySettlement V1 identity inputs are invalid")
	}
	return StableID("mcp-connect-apply", connection.ID, domain.ID, settlement.ID)
}

type MCPConnectApplySettlementExactReaderV1 interface {
	InspectMCPConnectApplySettlementV1(context.Context, ObjectRef) (MCPConnectApplySettlementFactV1, error)
}

// MCPConnectionAvailabilityCurrentProjectionV1 is the only connection input
// accepted by post-settlement MCP discovery/invoke adapters. It grants no
// Runtime authority and cannot replace the per-effect Gateway checks.
type MCPConnectionAvailabilityCurrentProjectionV1 struct {
	ContractVersion string                        `json:"contract_version"`
	Connection      MCPConnectionFactRefV2        `json:"connection"`
	ApplySettlement ObjectRef                     `json:"apply_settlement"`
	DomainResult    ObjectRef                     `json:"domain_result"`
	Owner           runtimeports.EffectOwnerRefV2 `json:"owner"`
	CheckedUnixNano int64                         `json:"checked_unix_nano"`
	ExpiresUnixNano int64                         `json:"expires_unix_nano"`
	Digest          core.Digest                   `json:"digest"`
}

func (p MCPConnectionAvailabilityCurrentProjectionV1) Validate(now time.Time) error {
	if p.ContractVersion != MCPConnectionAvailabilityContractVersionV1 || p.Connection.Validate() != nil || p.ApplySettlement.Validate() != nil || p.DomainResult.Validate() != nil || validateEffectOwner(p.Owner) != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || time.Duration(p.ExpiresUnixNano-p.CheckedUnixNano) > MaxMCPConnectionAvailabilityTTLV1 || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Connection availability is invalid or expired")
	}
	digest, err := p.ComputeDigest()
	if err != nil || digest != p.Digest {
		return conflict("MCP Connection availability digest drifted")
	}
	return nil
}

func (p MCPConnectionAvailabilityCurrentProjectionV1) ComputeDigest() (core.Digest, error) {
	p.Digest = ""
	return Seal("praxis.tool-mcp.mcp-connection-availability", MCPConnectionAvailabilityContractVersionV1, "MCPConnectionAvailabilityCurrentProjectionV1", p)
}

func SealMCPConnectionAvailabilityCurrentProjectionV1(p MCPConnectionAvailabilityCurrentProjectionV1, now time.Time) (MCPConnectionAvailabilityCurrentProjectionV1, error) {
	p.ContractVersion = MCPConnectionAvailabilityContractVersionV1
	p.Digest = ""
	digest, err := p.ComputeDigest()
	if err != nil {
		return MCPConnectionAvailabilityCurrentProjectionV1{}, err
	}
	p.Digest = digest
	return p, p.Validate(now)
}

type MCPConnectionAvailabilityCurrentReaderV1 interface {
	InspectCurrentMCPConnectionAvailabilityV1(context.Context, MCPConnectionFactRefV2, time.Duration) (MCPConnectionAvailabilityCurrentProjectionV1, error)
}
