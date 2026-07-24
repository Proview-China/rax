package contract

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	MCPConnectDomainResultContractVersionV1 = "praxis.tool-mcp.mcp-connect-domain-result/v1"
	MCPConnectDomainResultRuntimeKindV1     = runtimeports.NamespacedNameV2("praxis.mcp/connect-domain-result")
	MaxMCPConnectDomainResultCurrentTTLV1   = 30 * time.Second
)

type MCPConnectDomainResultFactV1 struct {
	ContractVersion      string                                              `json:"contract_version"`
	ID                   string                                              `json:"id"`
	Revision             core.Revision                                       `json:"revision"`
	Digest               core.Digest                                         `json:"digest"`
	TenantID             core.TenantID                                       `json:"tenant_id"`
	Operation            runtimeports.OperationSubjectV3                     `json:"operation"`
	OperationScopeDigest core.Digest                                         `json:"operation_scope_digest"`
	Connection           MCPConnectionFactRefV2                              `json:"connection"`
	Intent               ObjectRef                                           `json:"intent"`
	ProtocolReceipt      MCPConnectProtocolReceiptRefV1                      `json:"protocol_receipt"`
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

func (f MCPConnectDomainResultFactV1) Validate() error {
	if f.ContractVersion != MCPConnectDomainResultContractVersionV1 || ValidateStableID(f.ID) != nil || f.Revision != 1 || f.Digest.Validate() != nil || strings.TrimSpace(string(f.TenantID)) == "" || f.Operation.Validate() != nil || f.OperationScopeDigest.Validate() != nil || f.Connection.Validate() != nil || f.Intent.Validate() != nil || f.ProtocolReceipt.Validate() != nil || f.PreparedAttempt.Validate() != nil || f.Attempt.Validate() != nil || f.Observation.Validate() != nil || f.PrepareEnforcement.Validate() != nil || f.ExecuteEnforcement.Validate() != nil || f.PrepareConsumption.Validate() != nil || f.ExecuteConsumption.Validate() != nil || f.Schema.Validate() != nil || f.PayloadDigest.Validate() != nil || f.PayloadRevision != 1 || validateEffectOwner(f.Owner) != nil || f.Outcome != ToolOutcomeSucceededV2 || f.Disposition != ToolDispositionConfirmedAppliedV2 || f.CreatedUnixNano <= 0 {
		return invalid("MCP Connect DomainResult V1 is invalid")
	}
	if f.PrepareEnforcement.Phase != runtimeports.OperationDispatchEnforcementPrepareV4 || f.ExecuteEnforcement.Phase != runtimeports.OperationDispatchEnforcementExecuteV4 || f.PrepareConsumption == f.ExecuteConsumption {
		return conflict("MCP Connect DomainResult V1 requires distinct prepare and execute closure")
	}
	operationDigest, err := f.Operation.DigestV3()
	if err != nil || operationDigest != f.Attempt.OperationDigest || f.Operation.ExecutionScopeDigest != f.OperationScopeDigest || core.TenantID(f.Operation.ExecutionScope.Identity.TenantID) != f.TenantID || f.Attempt.Delegation == nil || f.Observation.Delegation != *f.Attempt.Delegation || f.Observation.PreparedAttemptID != f.PreparedAttempt.ID || f.PreparedAttempt.AttemptID != f.Attempt.AttemptID || f.PreparedAttempt.OperationDigest != f.Attempt.OperationDigest || f.PreparedAttempt.IntentID != f.Attempt.EffectID || f.PreparedAttempt.IntentRevision != f.Attempt.IntentRevision || f.PreparedAttempt.IntentDigest != f.Attempt.IntentDigest || f.PrepareEnforcement.AttemptID != f.Attempt.AttemptID || f.ExecuteEnforcement.AttemptID != f.Attempt.AttemptID || f.PrepareEnforcement.OperationDigest != f.Attempt.OperationDigest || f.ExecuteEnforcement.OperationDigest != f.Attempt.OperationDigest || f.PrepareEnforcement.EffectID != f.Attempt.EffectID || f.ExecuteEnforcement.EffectID != f.Attempt.EffectID {
		return conflict("MCP Connect DomainResult V1 causal chain drifted")
	}
	id, err := DeriveMCPConnectDomainResultIDV1(f.Connection, f.Attempt)
	if err != nil || id != f.ID {
		return conflict("MCP Connect DomainResult V1 ID drifted")
	}
	digest, err := f.ComputeDigest()
	if err != nil || digest != f.Digest {
		return conflict("MCP Connect DomainResult V1 digest drifted")
	}
	return nil
}

func (f MCPConnectDomainResultFactV1) ComputeDigest() (core.Digest, error) {
	f.Digest = ""
	return Seal("praxis.tool-mcp.mcp-connect-domain-result", MCPConnectDomainResultContractVersionV1, "MCPConnectDomainResultFactV1", f)
}

func SealMCPConnectDomainResultFactV1(f MCPConnectDomainResultFactV1) (MCPConnectDomainResultFactV1, error) {
	f.ContractVersion = MCPConnectDomainResultContractVersionV1
	id, err := DeriveMCPConnectDomainResultIDV1(f.Connection, f.Attempt)
	if err != nil {
		return MCPConnectDomainResultFactV1{}, err
	}
	if f.ID != "" && f.ID != id {
		return MCPConnectDomainResultFactV1{}, conflict("supplied MCP Connect DomainResult V1 ID drifted")
	}
	f.ID, f.Revision = id, 1
	provided := f.Digest
	f.Digest = ""
	digest, err := f.ComputeDigest()
	if err != nil {
		return MCPConnectDomainResultFactV1{}, err
	}
	if provided != "" && provided != digest {
		return MCPConnectDomainResultFactV1{}, conflict("supplied MCP Connect DomainResult V1 digest drifted")
	}
	f.Digest = digest
	return f, f.Validate()
}

func DeriveMCPConnectDomainResultIDV1(connection MCPConnectionFactRefV2, attempt runtimeports.OperationDispatchAttemptRefV3) (string, error) {
	if connection.Validate() != nil || attempt.Validate() != nil {
		return "", invalid("MCP Connect DomainResult V1 identity inputs are invalid")
	}
	return StableID("mcp-connect-domain-result", connection.ID, attempt.AttemptID)
}

func (f MCPConnectDomainResultFactV1) ObjectRef() ObjectRef {
	return ObjectRef{ID: f.ID, Revision: f.Revision, Digest: f.Digest}
}

type MCPConnectDomainResultCurrentProjectionV1 struct {
	ContractVersion    string                                              `json:"contract_version"`
	Fact               ObjectRef                                           `json:"fact"`
	Connection         MCPConnectionFactRefV2                              `json:"connection"`
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

func (p MCPConnectDomainResultCurrentProjectionV1) Validate(now time.Time) error {
	if p.ContractVersion != MCPConnectDomainResultContractVersionV1 || p.Fact.Validate() != nil || p.Connection.Validate() != nil || p.Observation.Validate() != nil || p.PrepareEnforcement.Validate() != nil || p.ExecuteEnforcement.Validate() != nil || p.PrepareConsumption.Validate() != nil || p.ExecuteConsumption.Validate() != nil || validateEffectOwner(p.Owner) != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || time.Duration(p.ExpiresUnixNano-p.CheckedUnixNano) > MaxMCPConnectDomainResultCurrentTTLV1 || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "MCP Connect DomainResult current projection is invalid or expired")
	}
	digest, err := p.ComputeDigest()
	if err != nil || digest != p.Digest {
		return conflict("MCP Connect DomainResult current projection digest drifted")
	}
	return nil
}

func (p MCPConnectDomainResultCurrentProjectionV1) ComputeDigest() (core.Digest, error) {
	p.Digest = ""
	return Seal("praxis.tool-mcp.mcp-connect-domain-result", MCPConnectDomainResultContractVersionV1, "MCPConnectDomainResultCurrentProjectionV1", p)
}

func SealMCPConnectDomainResultCurrentProjectionV1(p MCPConnectDomainResultCurrentProjectionV1, now time.Time) (MCPConnectDomainResultCurrentProjectionV1, error) {
	p.ContractVersion = MCPConnectDomainResultContractVersionV1
	p.Digest = ""
	digest, err := p.ComputeDigest()
	if err != nil {
		return MCPConnectDomainResultCurrentProjectionV1{}, err
	}
	p.Digest = digest
	return p, p.Validate(now)
}

type MCPConnectDomainResultExactReaderV1 interface {
	InspectMCPConnectDomainResultV1(context.Context, ObjectRef) (MCPConnectDomainResultFactV1, error)
	InspectCurrentMCPConnectDomainResultV1(context.Context, ObjectRef, time.Duration) (MCPConnectDomainResultCurrentProjectionV1, error)
}
