package contract

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	MCPDiscoveryPageDomainResultContractVersionV1 = "praxis.tool-mcp.mcp-discovery-page-domain-result/v1"
	MCPDiscoveryPageDomainResultRuntimeKindV1     = runtimeports.NamespacedNameV2("praxis.mcp/discovery-page-domain-result")
	MaxMCPDiscoveryPageDomainResultCurrentTTLV1   = 30 * time.Second
)

type MCPDiscoveryPageDomainResultFactV1 struct {
	ContractVersion      string                                              `json:"contract_version"`
	ID                   string                                              `json:"id"`
	Revision             core.Revision                                       `json:"revision"`
	Digest               core.Digest                                         `json:"digest"`
	TenantID             core.TenantID                                       `json:"tenant_id"`
	Operation            runtimeports.OperationSubjectV3                     `json:"operation"`
	OperationScopeDigest core.Digest                                         `json:"operation_scope_digest"`
	Connection           MCPConnectionFactRefV2                              `json:"connection"`
	Command              ObjectRef                                           `json:"command"`
	ProtocolReceipt      ObjectRef                                           `json:"protocol_receipt"`
	Namespace            runtimeports.NamespacedNameV2                       `json:"namespace"`
	CursorDigest         core.Digest                                         `json:"cursor_digest"`
	PageOrdinal          uint32                                              `json:"page_ordinal"`
	PreparedAttempt      runtimeports.PreparedProviderAttemptRefV2           `json:"prepared_attempt"`
	Attempt              runtimeports.OperationDispatchAttemptRefV3          `json:"attempt"`
	Observation          runtimeports.ProviderAttemptObservationRefV2        `json:"observation"`
	PrepareEnforcement   runtimeports.OperationDispatchEnforcementPhaseRefV4 `json:"prepare_enforcement"`
	ExecuteEnforcement   runtimeports.OperationDispatchEnforcementPhaseRefV4 `json:"execute_enforcement"`
	PrepareConsumption   runtimeports.OperationScopeEvidenceConsumptionRefV3 `json:"prepare_consumption"`
	ExecuteConsumption   runtimeports.OperationScopeEvidenceConsumptionRefV3 `json:"execute_consumption"`
	Schema               runtimeports.SchemaRefV2                            `json:"schema"`
	PayloadDigest        core.Digest                                         `json:"payload_digest"`
	PayloadRevision      core.Revision                                       `json:"payload_revision"`
	Owner                runtimeports.EffectOwnerRefV2                       `json:"owner"`
	Outcome              ToolOutcomeV2                                       `json:"outcome"`
	Disposition          ToolDispositionV2                                   `json:"disposition"`
	CreatedUnixNano      int64                                               `json:"created_unix_nano"`
}

func (f MCPDiscoveryPageDomainResultFactV1) Validate() error {
	if f.ContractVersion != MCPDiscoveryPageDomainResultContractVersionV1 || ValidateStableID(f.ID) != nil || f.Revision != 1 || f.Digest.Validate() != nil || f.TenantID == "" || f.Operation.Validate() != nil || f.OperationScopeDigest.Validate() != nil || f.Connection.Validate() != nil || f.Command.Validate() != nil || f.ProtocolReceipt.Validate() != nil || !runtimeports.IsMCPDiscoveryPageNamespaceV1(f.Namespace) || f.CursorDigest.Validate() != nil || f.PreparedAttempt.Validate() != nil || f.Attempt.Validate() != nil || f.Observation.Validate() != nil || f.PrepareEnforcement.Validate() != nil || f.ExecuteEnforcement.Validate() != nil || f.PrepareConsumption.Validate() != nil || f.ExecuteConsumption.Validate() != nil || f.Schema.Validate() != nil || f.PayloadDigest.Validate() != nil || f.PayloadRevision != 1 || validateEffectOwner(f.Owner) != nil || f.Outcome != ToolOutcomeSucceededV2 || f.Disposition != ToolDispositionConfirmedAppliedV2 || f.CreatedUnixNano <= 0 {
		return invalid("MCP Discovery Page DomainResult V1 is invalid")
	}
	opDigest, err := f.Operation.DigestV3()
	if err != nil || opDigest != f.Attempt.OperationDigest || f.Operation.ExecutionScopeDigest != f.OperationScopeDigest || f.Operation.ExecutionScope.Identity.TenantID != f.TenantID || f.Attempt.Delegation == nil || f.Observation.Delegation != *f.Attempt.Delegation || f.Observation.PreparedAttemptID != f.PreparedAttempt.ID || f.PreparedAttempt.AttemptID != f.Attempt.AttemptID || f.PreparedAttempt.OperationDigest != f.Attempt.OperationDigest || f.PrepareEnforcement.Phase != runtimeports.OperationDispatchEnforcementPrepareV4 || f.ExecuteEnforcement.Phase != runtimeports.OperationDispatchEnforcementExecuteV4 || f.PrepareConsumption == f.ExecuteConsumption {
		return conflict("MCP Discovery Page DomainResult causal chain drifted")
	}
	id, err := DeriveMCPDiscoveryPageDomainResultIDV1(f.Command, f.Attempt)
	if err != nil || id != f.ID {
		return conflict("MCP Discovery Page DomainResult identity drifted")
	}
	digest, err := f.ComputeDigest()
	if err != nil || digest != f.Digest {
		return conflict("MCP Discovery Page DomainResult digest drifted")
	}
	return nil
}

func (f MCPDiscoveryPageDomainResultFactV1) ComputeDigest() (core.Digest, error) {
	f.Digest = ""
	return Seal("praxis.tool-mcp.mcp-discovery-page-domain-result", MCPDiscoveryPageDomainResultContractVersionV1, "MCPDiscoveryPageDomainResultFactV1", f)
}

func SealMCPDiscoveryPageDomainResultFactV1(f MCPDiscoveryPageDomainResultFactV1) (MCPDiscoveryPageDomainResultFactV1, error) {
	f.ContractVersion = MCPDiscoveryPageDomainResultContractVersionV1
	id, err := DeriveMCPDiscoveryPageDomainResultIDV1(f.Command, f.Attempt)
	if err != nil {
		return MCPDiscoveryPageDomainResultFactV1{}, err
	}
	if f.ID != "" && f.ID != id {
		return MCPDiscoveryPageDomainResultFactV1{}, conflict("supplied MCP Discovery Page DomainResult ID drifted")
	}
	f.ID, f.Revision, f.Digest = id, 1, ""
	f.Digest, err = f.ComputeDigest()
	if err != nil {
		return MCPDiscoveryPageDomainResultFactV1{}, err
	}
	return f, f.Validate()
}

func DeriveMCPDiscoveryPageDomainResultIDV1(command ObjectRef, attempt runtimeports.OperationDispatchAttemptRefV3) (string, error) {
	if command.Validate() != nil || attempt.Validate() != nil {
		return "", invalid("MCP Discovery Page DomainResult identity input is invalid")
	}
	return StableID("mcp-discovery-page-domain-result", command.ID, attempt.AttemptID)
}

func (f MCPDiscoveryPageDomainResultFactV1) ObjectRef() ObjectRef {
	return ObjectRef{ID: f.ID, Revision: f.Revision, Digest: f.Digest}
}

type MCPDiscoveryPageDomainResultCurrentProjectionV1 struct {
	ContractVersion    string                                              `json:"contract_version"`
	Fact               ObjectRef                                           `json:"fact"`
	Command            ObjectRef                                           `json:"command"`
	ProtocolReceipt    ObjectRef                                           `json:"protocol_receipt"`
	Observation        runtimeports.ProviderAttemptObservationRefV2        `json:"observation"`
	PrepareEnforcement runtimeports.OperationDispatchEnforcementPhaseRefV4 `json:"prepare_enforcement"`
	ExecuteEnforcement runtimeports.OperationDispatchEnforcementPhaseRefV4 `json:"execute_enforcement"`
	PrepareConsumption runtimeports.OperationScopeEvidenceConsumptionRefV3 `json:"prepare_consumption"`
	ExecuteConsumption runtimeports.OperationScopeEvidenceConsumptionRefV3 `json:"execute_consumption"`
	Owner              runtimeports.EffectOwnerRefV2                       `json:"owner"`
	CheckedUnixNano    int64                                               `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                                               `json:"expires_unix_nano"`
	Digest             core.Digest                                         `json:"digest"`
}

func (p MCPDiscoveryPageDomainResultCurrentProjectionV1) Validate(now time.Time) error {
	if p.ContractVersion != MCPDiscoveryPageDomainResultContractVersionV1 || p.Fact.Validate() != nil || p.Command.Validate() != nil || p.ProtocolReceipt.Validate() != nil || p.Observation.Validate() != nil || p.PrepareEnforcement.Validate() != nil || p.ExecuteEnforcement.Validate() != nil || p.PrepareConsumption.Validate() != nil || p.ExecuteConsumption.Validate() != nil || validateEffectOwner(p.Owner) != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || time.Duration(p.ExpiresUnixNano-p.CheckedUnixNano) > MaxMCPDiscoveryPageDomainResultCurrentTTLV1 || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "MCP Discovery Page DomainResult current is invalid or expired")
	}
	digest, err := p.ComputeDigest()
	if err != nil || digest != p.Digest {
		return conflict("MCP Discovery Page DomainResult current digest drifted")
	}
	return nil
}

func (p MCPDiscoveryPageDomainResultCurrentProjectionV1) ComputeDigest() (core.Digest, error) {
	p.Digest = ""
	return Seal("praxis.tool-mcp.mcp-discovery-page-domain-result", MCPDiscoveryPageDomainResultContractVersionV1, "MCPDiscoveryPageDomainResultCurrentProjectionV1", p)
}
func SealMCPDiscoveryPageDomainResultCurrentProjectionV1(p MCPDiscoveryPageDomainResultCurrentProjectionV1, now time.Time) (MCPDiscoveryPageDomainResultCurrentProjectionV1, error) {
	p.ContractVersion = MCPDiscoveryPageDomainResultContractVersionV1
	p.Digest = ""
	digest, err := p.ComputeDigest()
	if err != nil {
		return p, err
	}
	p.Digest = digest
	return p, p.Validate(now)
}

type MCPDiscoveryPageDomainResultExactReaderV1 interface {
	InspectMCPDiscoveryPageDomainResultV1(context.Context, ObjectRef) (MCPDiscoveryPageDomainResultFactV1, error)
	InspectCurrentMCPDiscoveryPageDomainResultV1(context.Context, ObjectRef, time.Duration) (MCPDiscoveryPageDomainResultCurrentProjectionV1, error)
}
