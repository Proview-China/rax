package contract

import (
	"context"
	"strconv"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	MCPDiscoveryPageContractVersionV1        = "praxis.tool-mcp.mcp-discovery-page/v1"
	MCPDiscoveryPageReceiptContractVersionV1 = "praxis.tool-mcp.mcp-discovery-page-receipt/v1"
	MaxMCPDiscoveryCursorBytesV1             = 4096
)

type MCPDiscoveryPageCommandV1 struct {
	ContractVersion  string                                             `json:"contract_version"`
	Ref              ObjectRef                                          `json:"ref"`
	Owner            runtimeports.EffectOwnerRefV2                      `json:"owner"`
	Connection       MCPConnectionFactRefV2                             `json:"connection"`
	Availability     runtimeports.MCPConnectionAvailabilityNeutralRefV1 `json:"connection_availability"`
	Namespace        runtimeports.NamespacedNameV2                      `json:"namespace"`
	Cursor           []byte                                             `json:"cursor"`
	CursorDigest     core.Digest                                        `json:"cursor_digest"`
	PageOrdinal      uint32                                             `json:"page_ordinal"`
	Operation        runtimeports.OperationSubjectV3                    `json:"operation"`
	OperationDigest  core.Digest                                        `json:"operation_digest"`
	EffectID         core.EffectIntentID                                `json:"effect_id"`
	EffectRevision   core.Revision                                      `json:"effect_revision"`
	EffectKind       runtimeports.EffectKindV2                          `json:"effect_kind"`
	PolicyProfile    runtimeports.NamespacedNameV2                      `json:"policy_profile"`
	IntentDigest     core.Digest                                        `json:"intent_digest"`
	Prepared         runtimeports.PreparedProviderAttemptRefV2          `json:"prepared"`
	Attempt          runtimeports.OperationDispatchAttemptRefV3         `json:"attempt"`
	Provider         runtimeports.ProviderBindingRefV2                  `json:"provider_binding"`
	CreatedUnixNano  int64                                              `json:"created_unix_nano"`
	NotAfterUnixNano int64                                              `json:"not_after_unix_nano"`
}

func (c MCPDiscoveryPageCommandV1) Validate() error {
	if c.ContractVersion != MCPDiscoveryPageContractVersionV1 || c.Ref.Validate() != nil || validateEffectOwner(c.Owner) != nil || c.Connection.Validate() != nil || c.Availability.Validate() != nil || !runtimeports.IsMCPDiscoveryPageNamespaceV1(c.Namespace) || len(c.Cursor) > MaxMCPDiscoveryCursorBytesV1 || c.CursorDigest != core.DigestBytes(c.Cursor) || c.Operation.Validate() != nil || c.OperationDigest.Validate() != nil || c.EffectID == "" || c.EffectRevision == 0 || c.IntentDigest.Validate() != nil || c.Prepared.Validate() != nil || c.Attempt.Validate() != nil || c.Provider.Validate() != nil || c.CreatedUnixNano <= 0 || c.NotAfterUnixNano <= c.CreatedUnixNano {
		return invalid("MCP Discovery Page Command V1 is incomplete")
	}
	opDigest, err := c.Operation.DigestV3()
	if err != nil || opDigest != c.OperationDigest || c.Operation.Kind != runtimeports.OperationScopeRunV3 || c.EffectKind != runtimeports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1 || c.PolicyProfile != runtimeports.OperationScopeEvidenceMCPDiscoveryPagePolicyProfileV1 || c.Attempt.OperationDigest != c.OperationDigest || c.Attempt.EffectID != c.EffectID || c.Attempt.IntentRevision != c.EffectRevision || c.Attempt.IntentDigest != c.IntentDigest || c.Prepared.AttemptID != c.Attempt.AttemptID || c.Prepared.OperationDigest != c.OperationDigest || c.Prepared.Provider != c.Provider {
		return conflict("MCP Discovery Page Command operation bindings drifted")
	}
	if c.Availability.ConnectionID != c.Connection.ID || c.Availability.ConnectionRevision != c.Connection.Revision || c.Availability.ConnectionDigest != c.Connection.Digest || c.Availability.Owner != c.Owner || c.Owner.ComponentID != c.Provider.ComponentID || c.Owner.ManifestDigest != c.Provider.ManifestDigest || c.Provider.Capability != runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1) {
		return conflict("MCP Discovery Page Command Connection or Provider drifted")
	}
	id, err := DeriveMCPDiscoveryPageCommandIDV1(c.Connection, c.Namespace, c.PageOrdinal, c.CursorDigest, c.Attempt)
	if err != nil || c.Ref.ID != id || c.Ref.Revision != 1 {
		return conflict("MCP Discovery Page Command identity drifted")
	}
	digest, err := c.ComputeDigest()
	if err != nil || digest != c.Ref.Digest {
		return conflict("MCP Discovery Page Command digest drifted")
	}
	return nil
}

func (c MCPDiscoveryPageCommandV1) ComputeDigest() (core.Digest, error) {
	c.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-discovery-page", MCPDiscoveryPageContractVersionV1, "MCPDiscoveryPageCommandV1", c)
}

func SealMCPDiscoveryPageCommandV1(c MCPDiscoveryPageCommandV1) (MCPDiscoveryPageCommandV1, error) {
	c.ContractVersion = MCPDiscoveryPageContractVersionV1
	c.Cursor = append([]byte(nil), c.Cursor...)
	c.CursorDigest = core.DigestBytes(c.Cursor)
	id, err := DeriveMCPDiscoveryPageCommandIDV1(c.Connection, c.Namespace, c.PageOrdinal, c.CursorDigest, c.Attempt)
	if err != nil {
		return MCPDiscoveryPageCommandV1{}, err
	}
	if c.Ref.ID != "" && c.Ref.ID != id {
		return MCPDiscoveryPageCommandV1{}, conflict("supplied MCP Discovery Page Command ID drifted")
	}
	c.Ref.ID, c.Ref.Revision, c.Ref.Digest = id, 1, ""
	c.Ref.Digest, err = c.ComputeDigest()
	if err != nil {
		return MCPDiscoveryPageCommandV1{}, err
	}
	return c, c.Validate()
}

func DeriveMCPDiscoveryPageCommandIDV1(connection MCPConnectionFactRefV2, namespace runtimeports.NamespacedNameV2, ordinal uint32, cursor core.Digest, attempt runtimeports.OperationDispatchAttemptRefV3) (string, error) {
	if connection.Validate() != nil || !runtimeports.IsMCPDiscoveryPageNamespaceV1(namespace) || cursor.Validate() != nil || attempt.Validate() != nil {
		return "", invalid("MCP Discovery Page Command identity input is invalid")
	}
	return StableID("mcp-discovery-page", connection.ID, string(namespace), strconv.FormatUint(uint64(ordinal), 10), string(cursor), attempt.AttemptID)
}

func (c MCPDiscoveryPageCommandV1) RuntimeDomainCommandRefV1() runtimeports.OperationDomainCommandRefV1 {
	return runtimeports.OperationDomainCommandRefV1{Owner: c.Owner, Kind: "praxis.mcp/discovery-page-command", ID: c.Ref.ID, Revision: c.Ref.Revision, Digest: c.Ref.Digest}
}

type MCPDiscoveryPageCommandExactReaderV1 interface {
	InspectMCPDiscoveryPageCommandV1(context.Context, ObjectRef) (MCPDiscoveryPageCommandV1, error)
}

type MCPDiscoveryPageProtocolReceiptV1 struct {
	ContractVersion      string                                                        `json:"contract_version"`
	Ref                  ObjectRef                                                     `json:"ref"`
	Command              ObjectRef                                                     `json:"command"`
	StableKeyDigest      core.Digest                                                   `json:"stable_key_digest"`
	AdmissionReceipt     runtimeports.ControlledOperationProviderAdmissionReceiptRefV2 `json:"admission_receipt"`
	Namespace            runtimeports.NamespacedNameV2                                 `json:"namespace"`
	CursorDigest         core.Digest                                                   `json:"cursor_digest"`
	PageOrdinal          uint32                                                        `json:"page_ordinal"`
	ResponsePageDigest   core.Digest                                                   `json:"response_page_digest"`
	NextCursor           []byte                                                        `json:"next_cursor"`
	NextCursorDigest     core.Digest                                                   `json:"next_cursor_digest"`
	ItemCount            uint32                                                        `json:"item_count"`
	ProviderOperationRef string                                                        `json:"provider_operation_ref,omitempty"`
	ObservedUnixNano     int64                                                         `json:"observed_unix_nano"`
}

func (r MCPDiscoveryPageProtocolReceiptV1) Validate() error {
	if r.ContractVersion != MCPDiscoveryPageReceiptContractVersionV1 || r.Ref.Validate() != nil || r.Command.Validate() != nil || r.StableKeyDigest.Validate() != nil || r.AdmissionReceipt.Validate() != nil || !r.AdmissionReceipt.Admitted || !runtimeports.IsMCPDiscoveryPageNamespaceV1(r.Namespace) || r.CursorDigest.Validate() != nil || r.ResponsePageDigest.Validate() != nil || len(r.NextCursor) > MaxMCPDiscoveryCursorBytesV1 || r.NextCursorDigest != core.DigestBytes(r.NextCursor) || len(r.ProviderOperationRef) > 512 || r.ObservedUnixNano <= 0 {
		return invalid("MCP Discovery Page Protocol Receipt V1 is invalid")
	}
	id, err := StableID("mcp-discovery-page-receipt", r.Command.ID, string(r.StableKeyDigest))
	if err != nil || r.Ref.ID != id || r.Ref.Revision != 1 {
		return conflict("MCP Discovery Page Protocol Receipt identity drifted")
	}
	digest, err := r.ComputeDigest()
	if err != nil || digest != r.Ref.Digest {
		return conflict("MCP Discovery Page Protocol Receipt digest drifted")
	}
	return nil
}

func (r MCPDiscoveryPageProtocolReceiptV1) ComputeDigest() (core.Digest, error) {
	r.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-discovery-page-receipt", MCPDiscoveryPageReceiptContractVersionV1, "MCPDiscoveryPageProtocolReceiptV1", r)
}

func SealMCPDiscoveryPageProtocolReceiptV1(r MCPDiscoveryPageProtocolReceiptV1) (MCPDiscoveryPageProtocolReceiptV1, error) {
	r.ContractVersion = MCPDiscoveryPageReceiptContractVersionV1
	r.NextCursor = append([]byte(nil), r.NextCursor...)
	r.NextCursorDigest = core.DigestBytes(r.NextCursor)
	id, err := StableID("mcp-discovery-page-receipt", r.Command.ID, string(r.StableKeyDigest))
	if err != nil {
		return MCPDiscoveryPageProtocolReceiptV1{}, err
	}
	r.Ref.ID, r.Ref.Revision, r.Ref.Digest = id, 1, ""
	r.Ref.Digest, err = r.ComputeDigest()
	if err != nil {
		return MCPDiscoveryPageProtocolReceiptV1{}, err
	}
	return r, r.Validate()
}

type MCPDiscoveryPageProtocolReceiptExactReaderV1 interface {
	InspectMCPDiscoveryPageProtocolReceiptV1(context.Context, ObjectRef) (MCPDiscoveryPageProtocolReceiptV1, error)
}
