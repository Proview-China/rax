package contract

import (
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	MCPConnectContractVersionV1 = "praxis.tool-mcp.mcp-connect/v1"

	MCPConnectEffectKindV1    runtimeports.EffectKindV2     = "praxis.mcp/connect"
	MCPConnectPolicyProfileV1 runtimeports.NamespacedNameV2 = "praxis.mcp/run-connection-v1"

	MCPTransportStdioV1          runtimeports.NamespacedNameV2 = "praxis.mcp.transport/stdio"
	MCPTransportStreamableHTTPV1 runtimeports.NamespacedNameV2 = "praxis.mcp.transport/streamable-http"
)

const (
	MaxMCPTransportArgumentsV1            = 64
	MaxMCPTransportCredentialPlaceholders = 32
)

// MCPConnectionCoordinateV1 is the stable pre-provider identity of one
// Run/Session/Server connection epoch. Provider session identifiers never
// participate in this identity.
type MCPConnectionCoordinateV1 struct {
	ID            string      `json:"id"`
	TenantID      string      `json:"tenant_id"`
	IdentityID    string      `json:"identity_id"`
	IdentityEpoch core.Epoch  `json:"identity_epoch"`
	PlanDigest    core.Digest `json:"plan_digest"`
	InstanceID    string      `json:"instance_id"`
	InstanceEpoch core.Epoch  `json:"instance_epoch"`
	RunID         string      `json:"run_id"`
	Session       ObjectRef   `json:"session"`
	Server        ObjectRef   `json:"server"`
	Epoch         core.Epoch  `json:"connection_epoch"`
}

func (c MCPConnectionCoordinateV1) Validate() error {
	if ValidateStableID(c.ID) != nil || ValidateStableID(c.TenantID) != nil || ValidateStableID(c.IdentityID) != nil || c.IdentityEpoch == 0 || c.PlanDigest.Validate() != nil || ValidateStableID(c.InstanceID) != nil || c.InstanceEpoch == 0 || ValidateStableID(c.RunID) != nil || c.Session.Validate() != nil || c.Server.Validate() != nil || c.Epoch == 0 {
		return invalid("MCP Connection coordinate is incomplete")
	}
	id, err := DeriveMCPConnectionCoordinateIDV1(c)
	if err != nil || id != c.ID {
		return conflict("MCP Connection coordinate ID drifted")
	}
	return nil
}

func DeriveMCPConnectionCoordinateIDV1(c MCPConnectionCoordinateV1) (string, error) {
	c.ID = ""
	digest, err := Seal("praxis.tool-mcp.mcp-connect", MCPConnectContractVersionV1, "MCPConnectionCoordinateIdentityV1", c)
	if err != nil {
		return "", err
	}
	return StableID("mcp-connection", string(digest))
}

func SealMCPConnectionCoordinateV1(c MCPConnectionCoordinateV1) (MCPConnectionCoordinateV1, error) {
	provided := c.ID
	c.ID = ""
	id, err := DeriveMCPConnectionCoordinateIDV1(c)
	if err != nil {
		return MCPConnectionCoordinateV1{}, err
	}
	if provided != "" && provided != id {
		return MCPConnectionCoordinateV1{}, conflict("MCP Connection coordinate supplied a wrong ID")
	}
	c.ID = id
	return c, c.Validate()
}

type MCPTransportConfigRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r MCPTransportConfigRefV1) Validate() error {
	return (ObjectRef{ID: r.ID, Revision: r.Revision, Digest: r.Digest}).Validate()
}

func (r MCPTransportConfigRefV1) ObjectRef() ObjectRef {
	return ObjectRef{ID: r.ID, Revision: r.Revision, Digest: r.Digest}
}

type MCPStdioTransportConfigV1 struct {
	Executable             string   `json:"executable"`
	Arguments              []string `json:"arguments,omitempty"`
	WorkingDirectory       string   `json:"working_directory,omitempty"`
	CredentialPlaceholders []string `json:"credential_placeholders,omitempty"`
}

func (c MCPStdioTransportConfigV1) Validate() error {
	if invalidBoundedTransportStringV1(c.Executable, false) || len(c.Arguments) > MaxMCPTransportArgumentsV1 || invalidBoundedTransportStringV1(c.WorkingDirectory, true) {
		return invalid("MCP stdio config executable, arguments or working directory is invalid")
	}
	for _, argument := range c.Arguments {
		if invalidBoundedTransportStringV1(argument, true) {
			return invalid("MCP stdio argument is invalid")
		}
	}
	if len(c.CredentialPlaceholders) > MaxMCPTransportCredentialPlaceholders {
		return invalid("MCP stdio credential placeholders exceed the limit")
	}
	for i, placeholder := range c.CredentialPlaceholders {
		if !validCredentialPlaceholderV1(placeholder) || i > 0 && c.CredentialPlaceholders[i-1] >= placeholder {
			return invalid("MCP stdio credential placeholders must be sorted and unique")
		}
	}
	return nil
}

type MCPStreamableHTTPTransportConfigV1 struct {
	Endpoint             string `json:"endpoint"`
	DisableStandaloneSSE bool   `json:"disable_standalone_sse"`
}

func (c MCPStreamableHTTPTransportConfigV1) Validate() error {
	if invalidBoundedTransportStringV1(c.Endpoint, false) {
		return invalid("MCP Streamable HTTP endpoint is invalid")
	}
	parsed, err := url.Parse(c.Endpoint)
	if err != nil || parsed.User != nil || parsed.Fragment != "" || parsed.Host == "" || parsed.Path == "" {
		return invalid("MCP Streamable HTTP endpoint is not canonical")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	host := parsed.Hostname()
	if parsed.Scheme != "http" || host != "localhost" && net.ParseIP(host) == nil || host != "localhost" && !net.ParseIP(host).IsLoopback() {
		return invalid("plain HTTP MCP endpoints are restricted to loopback fixtures")
	}
	return nil
}

type MCPTransportConfigV1 struct {
	ContractVersion          string                              `json:"contract_version"`
	Ref                      MCPTransportConfigRefV1             `json:"ref"`
	Owner                    core.OwnerRef                       `json:"owner"`
	Server                   ObjectRef                           `json:"server"`
	Kind                     runtimeports.NamespacedNameV2       `json:"kind"`
	ProviderTransport        runtimeports.ProviderBindingRefV2   `json:"provider_transport_binding"`
	ArtifactDigest           core.Digest                         `json:"artifact_digest"`
	ConfigDigest             core.Digest                         `json:"config_digest"`
	NetworkScopeDigest       core.Digest                         `json:"network_scope_digest"`
	SandboxRequirementDigest core.Digest                         `json:"sandbox_requirement_digest"`
	Stdio                    *MCPStdioTransportConfigV1          `json:"stdio,omitempty"`
	StreamableHTTP           *MCPStreamableHTTPTransportConfigV1 `json:"streamable_http,omitempty"`
	CreatedUnixNano          int64                               `json:"created_unix_nano"`
}

func (c MCPTransportConfigV1) Validate() error {
	if c.ContractVersion != MCPConnectContractVersionV1 || c.Ref.Validate() != nil || c.Owner.Validate() != nil || c.Server.Validate() != nil || c.ProviderTransport.Validate() != nil || c.ArtifactDigest.Validate() != nil || c.ConfigDigest.Validate() != nil || c.NetworkScopeDigest.Validate() != nil || c.SandboxRequirementDigest.Validate() != nil || c.CreatedUnixNano <= 0 {
		return invalid("MCP Transport Config identity or governance binding is incomplete")
	}
	if c.Kind == MCPTransportStdioV1 {
		if c.Stdio == nil || c.StreamableHTTP != nil || c.Stdio.Validate() != nil {
			return invalid("MCP stdio Transport Config one-of is invalid")
		}
	} else if c.Kind == MCPTransportStreamableHTTPV1 {
		if c.StreamableHTTP == nil || c.Stdio != nil || c.StreamableHTTP.Validate() != nil {
			return invalid("MCP Streamable HTTP Config one-of is invalid")
		}
	} else {
		return invalid("MCP Transport kind is unsupported")
	}
	id, err := DeriveMCPTransportConfigIDV1(c.Server, c.Kind)
	if err != nil || c.Ref.ID != id {
		return conflict("MCP Transport Config ID drifted")
	}
	digest, err := c.DigestV1()
	if err != nil || digest != c.Ref.Digest {
		return conflict("MCP Transport Config digest drifted")
	}
	return nil
}

func (c MCPTransportConfigV1) DigestV1() (core.Digest, error) {
	c.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-connect", MCPConnectContractVersionV1, "MCPTransportConfigV1", c)
}

func DeriveMCPTransportConfigIDV1(server ObjectRef, kind runtimeports.NamespacedNameV2) (string, error) {
	if server.Validate() != nil || kind != MCPTransportStdioV1 && kind != MCPTransportStreamableHTTPV1 {
		return "", invalid("MCP Transport Config identity inputs are invalid")
	}
	return StableID("mcp-transport", server.ID, string(kind))
}

func SealMCPTransportConfigV1(c MCPTransportConfigV1) (MCPTransportConfigV1, error) {
	c.ContractVersion = MCPConnectContractVersionV1
	id, err := DeriveMCPTransportConfigIDV1(c.Server, c.Kind)
	if err != nil {
		return MCPTransportConfigV1{}, err
	}
	if c.Ref.ID != "" && c.Ref.ID != id {
		return MCPTransportConfigV1{}, conflict("MCP Transport Config supplied a wrong ID")
	}
	c.Ref.ID = id
	c.Ref.Digest = ""
	digest, err := c.DigestV1()
	if err != nil {
		return MCPTransportConfigV1{}, err
	}
	c.Ref.Digest = digest
	return c, c.Validate()
}

type MCPConnectIntentV1 struct {
	ContractVersion          string                                     `json:"contract_version"`
	Ref                      ObjectRef                                  `json:"ref"`
	Owner                    runtimeports.EffectOwnerRefV2              `json:"owner"`
	Coordinate               MCPConnectionCoordinateV1                  `json:"connection_coordinate"`
	Server                   ObjectRef                                  `json:"server"`
	TransportConfig          MCPTransportConfigRefV1                    `json:"transport_config"`
	Operation                runtimeports.OperationSubjectV3            `json:"operation"`
	OperationDigest          core.Digest                                `json:"operation_digest"`
	EffectID                 core.EffectIntentID                        `json:"effect_id"`
	EffectRevision           core.Revision                              `json:"effect_revision"`
	EffectKind               runtimeports.EffectKindV2                  `json:"effect_kind"`
	PolicyProfile            runtimeports.NamespacedNameV2              `json:"policy_profile"`
	IntentDigest             core.Digest                                `json:"intent_digest"`
	Attempt                  runtimeports.OperationDispatchAttemptRefV3 `json:"attempt"`
	CredentialLeases         []runtimeports.CredentialLeaseRefV2        `json:"credential_leases,omitempty"`
	Provider                 runtimeports.ProviderBindingRefV2          `json:"provider_binding"`
	ProviderTransport        runtimeports.ProviderBindingRefV2          `json:"provider_transport_binding"`
	NetworkScopeDigest       core.Digest                                `json:"network_scope_digest"`
	SandboxRequirementDigest core.Digest                                `json:"sandbox_requirement_digest"`
	CreatedUnixNano          int64                                      `json:"created_unix_nano"`
	NotAfterUnixNano         int64                                      `json:"not_after_unix_nano"`
}

func (f MCPConnectIntentV1) Validate() error {
	if f.ContractVersion != MCPConnectContractVersionV1 || f.Ref.Validate() != nil || f.Owner.Role != runtimeports.OwnerSettlement || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(f.Owner.ComponentID)) != nil || f.Owner.ManifestDigest.Validate() != nil || f.Coordinate.Validate() != nil || f.Server.Validate() != nil || f.TransportConfig.Validate() != nil || f.Operation.Validate() != nil || f.OperationDigest.Validate() != nil || f.IntentDigest.Validate() != nil || f.Attempt.Validate() != nil || f.Provider.Validate() != nil || f.ProviderTransport.Validate() != nil || f.NetworkScopeDigest.Validate() != nil || f.SandboxRequirementDigest.Validate() != nil || f.CreatedUnixNano <= 0 || f.NotAfterUnixNano <= f.CreatedUnixNano {
		return invalid("MCP Connect Intent is incomplete")
	}
	operationDigest, err := f.Operation.DigestV3()
	if err != nil || operationDigest != f.OperationDigest || f.Operation.Kind != runtimeports.OperationScopeRunV3 || string(f.Operation.RunID) != f.Coordinate.RunID {
		return conflict("MCP Connect Intent Operation drifted")
	}
	if f.Server != f.Coordinate.Server || f.EffectKind != MCPConnectEffectKindV1 || f.PolicyProfile != MCPConnectPolicyProfileV1 || f.Attempt.OperationDigest != f.OperationDigest || f.Attempt.EffectID != f.EffectID || f.Attempt.IntentDigest != f.IntentDigest || f.Attempt.IntentRevision == 0 || f.EffectRevision < f.Attempt.IntentRevision {
		return conflict("MCP Connect Intent Effect or coordinate drifted")
	}
	if f.Provider == f.ProviderTransport || f.Provider.BindingSetID != f.ProviderTransport.BindingSetID || f.Provider.BindingSetRevision != f.ProviderTransport.BindingSetRevision || string(f.Owner.ComponentID) != string(f.Provider.ComponentID) || f.Owner.ManifestDigest != f.Provider.ManifestDigest {
		return conflict("MCP Connect Intent Provider bindings drifted")
	}
	if err := validateCredentialLeasesV1(f.CredentialLeases); err != nil {
		return err
	}
	id, err := DeriveMCPConnectIntentIDV1(f.Coordinate, f.Attempt)
	if err != nil || f.Ref.ID != id {
		return conflict("MCP Connect Intent ID drifted")
	}
	digest, err := f.DigestV1()
	if err != nil || digest != f.Ref.Digest {
		return conflict("MCP Connect Intent digest drifted")
	}
	return nil
}

func (f MCPConnectIntentV1) DigestV1() (core.Digest, error) {
	f.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-connect", MCPConnectContractVersionV1, "MCPConnectIntentV1", f)
}

func DeriveMCPConnectIntentIDV1(coordinate MCPConnectionCoordinateV1, attempt runtimeports.OperationDispatchAttemptRefV3) (string, error) {
	if coordinate.Validate() != nil || attempt.Validate() != nil {
		return "", invalid("MCP Connect Intent identity inputs are invalid")
	}
	return StableID("mcp-connect", coordinate.ID, attempt.AttemptID)
}

func SealMCPConnectIntentV1(f MCPConnectIntentV1) (MCPConnectIntentV1, error) {
	f.ContractVersion = MCPConnectContractVersionV1
	sortCredentialLeasesV1(f.CredentialLeases)
	id, err := DeriveMCPConnectIntentIDV1(f.Coordinate, f.Attempt)
	if err != nil {
		return MCPConnectIntentV1{}, err
	}
	if f.Ref.ID != "" && f.Ref.ID != id {
		return MCPConnectIntentV1{}, conflict("MCP Connect Intent supplied a wrong ID")
	}
	f.Ref.ID = id
	f.Ref.Digest = ""
	digest, err := f.DigestV1()
	if err != nil {
		return MCPConnectIntentV1{}, err
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

func (f MCPConnectIntentV1) RuntimeDomainCommandRefV1() runtimeports.OperationDomainCommandRefV1 {
	return runtimeports.OperationDomainCommandRefV1{Owner: f.Owner, Kind: "praxis.mcp/connect-intent-v1", ID: f.Ref.ID, Revision: f.Ref.Revision, Digest: f.Ref.Digest}
}

func invalidBoundedTransportStringV1(value string, allowEmpty bool) bool {
	if value == "" {
		return !allowEmpty
	}
	return strings.TrimSpace(value) != value || len(value) > MaxStringBytes || strings.ContainsRune(value, '\x00')
}

func validCredentialPlaceholderV1(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, ch := range []byte(value) {
		if !((ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}
	return true
}

func validateCredentialLeasesV1(values []runtimeports.CredentialLeaseRefV2) error {
	for i, value := range values {
		if ValidateStableID(value.Ref) != nil || strings.TrimSpace(value.Class) == "" || len(value.Class) > 128 || value.ScopeDigest.Validate() != nil || value.Epoch == 0 {
			return invalid("MCP Connect credential lease is invalid")
		}
		if i > 0 && credentialLeaseKeyV1(values[i-1]) >= credentialLeaseKeyV1(value) {
			return invalid("MCP Connect credential leases must be sorted and unique")
		}
	}
	return nil
}

func sortCredentialLeasesV1(values []runtimeports.CredentialLeaseRefV2) {
	sort.Slice(values, func(i, j int) bool { return credentialLeaseKeyV1(values[i]) < credentialLeaseKeyV1(values[j]) })
}

func credentialLeaseKeyV1(value runtimeports.CredentialLeaseRefV2) string {
	return value.Ref + "\x00" + value.Class + "\x00" + string(value.ScopeDigest)
}
