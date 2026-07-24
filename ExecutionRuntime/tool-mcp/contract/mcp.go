package contract

import (
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type MCPServerDescriptor struct {
	ContractVersion    string                          `json:"contract_version"`
	ID                 string                          `json:"id"`
	Revision           core.Revision                   `json:"revision"`
	Digest             core.Digest                     `json:"digest"`
	Owner              core.OwnerRef                   `json:"owner"`
	Source             runtimeports.NamespacedNameV2   `json:"source"`
	MinimumProtocol    string                          `json:"minimum_protocol"`
	MaximumProtocol    string                          `json:"maximum_protocol"`
	Transports         []runtimeports.NamespacedNameV2 `json:"transports"`
	AuthRequirement    runtimeports.NamespacedNameV2   `json:"auth_requirement"`
	TrustClass         runtimeports.NamespacedNameV2   `json:"trust_class"`
	NetworkScopeDigest core.Digest                     `json:"network_scope_digest"`
	ArtifactDigest     core.Digest                     `json:"artifact_digest"`
	ConfigDigest       core.Digest                     `json:"config_digest"`
	Conformance        runtimeports.NamespacedNameV2   `json:"conformance"`
	CreatedUnixNano    int64                           `json:"created_unix_nano"`
}

func (d MCPServerDescriptor) validateShape() error {
	if d.ContractVersion != MCPContractVersion || ValidateStableID(d.ID) != nil || d.Revision == 0 || d.Owner.Validate() != nil || d.CreatedUnixNano <= 0 {
		return invalid("MCP server identity, owner or revision is invalid")
	}
	if runtimeports.ValidateNamespacedNameV2(d.Source) != nil || runtimeports.ValidateNamespacedNameV2(d.AuthRequirement) != nil || runtimeports.ValidateNamespacedNameV2(d.TrustClass) != nil || runtimeports.ValidateNamespacedNameV2(d.Conformance) != nil {
		return invalid("MCP server governance names are invalid")
	}
	if ValidateSortedUniqueNames(d.Transports, 8) != nil || d.NetworkScopeDigest.Validate() != nil || d.ArtifactDigest.Validate() != nil || d.ConfigDigest.Validate() != nil {
		return invalid("MCP server transports or digests are invalid")
	}
	if !validProtocolVersion(d.MinimumProtocol) || !validProtocolVersion(d.MaximumProtocol) || d.MinimumProtocol > d.MaximumProtocol {
		return invalid("MCP protocol range is invalid")
	}
	return nil
}

func (d MCPServerDescriptor) Validate() error {
	if err := d.validateShape(); err != nil {
		return err
	}
	expected, err := d.ComputeDigest()
	if err != nil || d.Digest.Validate() != nil || expected != d.Digest {
		return conflict("MCP server digest does not bind exact content")
	}
	return nil
}

func (d MCPServerDescriptor) ComputeDigest() (core.Digest, error) {
	if err := d.validateShape(); err != nil {
		return "", err
	}
	d.Digest = ""
	return Seal("praxis.tool-mcp.mcp", MCPContractVersion, "MCPServerDescriptor", d)
}

func SealMCPServer(d MCPServerDescriptor) (MCPServerDescriptor, error) {
	d.ContractVersion = MCPContractVersion
	d.Transports = SortedUniqueNames(d.Transports)
	d.Digest = ""
	digest, err := d.ComputeDigest()
	if err != nil {
		return MCPServerDescriptor{}, err
	}
	d.Digest = digest
	return d, nil
}

type MCPConnectionRef struct {
	ContractVersion    string        `json:"contract_version"`
	ID                 string        `json:"id"`
	Revision           core.Revision `json:"revision"`
	Digest             core.Digest   `json:"digest"`
	Epoch              core.Epoch    `json:"epoch"`
	Server             ObjectRef     `json:"server"`
	TenantID           string        `json:"tenant_id"`
	IdentityID         string        `json:"identity_id"`
	PlanDigest         core.Digest   `json:"plan_digest"`
	InstanceID         string        `json:"instance_id"`
	RunID              string        `json:"run_id,omitempty"`
	NegotiatedProtocol string        `json:"negotiated_protocol"`
	SessionID          string        `json:"session_id"`
	CreatedUnixNano    int64         `json:"created_unix_nano"`
	ExpiresUnixNano    int64         `json:"expires_unix_nano"`
}

func (r MCPConnectionRef) validateShape() error {
	if r.ContractVersion != MCPContractVersion || ValidateStableID(r.ID) != nil || r.Revision == 0 || r.Epoch == 0 || r.Server.Validate() != nil || r.PlanDigest.Validate() != nil || r.CreatedUnixNano <= 0 || r.ExpiresUnixNano <= r.CreatedUnixNano {
		return invalid("MCP connection identity, epoch, server or lifetime is invalid")
	}
	for _, value := range []string{r.TenantID, r.IdentityID, r.InstanceID, r.SessionID} {
		if strings.TrimSpace(value) == "" || len(value) > 256 {
			return invalid("MCP connection scope is blank or unbounded")
		}
	}
	if !validProtocolVersion(r.NegotiatedProtocol) || r.NegotiatedProtocol > MCPStableProtocolVersion {
		return invalid("MCP negotiated protocol is invalid")
	}
	return nil
}

func (r MCPConnectionRef) Validate() error {
	if err := r.validateShape(); err != nil {
		return err
	}
	expected, err := r.ComputeDigest()
	if err != nil || r.Digest.Validate() != nil || expected != r.Digest {
		return conflict("MCP connection digest does not bind exact content")
	}
	return nil
}

func (r MCPConnectionRef) ComputeDigest() (core.Digest, error) {
	if err := r.validateShape(); err != nil {
		return "", err
	}
	r.Digest = ""
	return Seal("praxis.tool-mcp.mcp", MCPContractVersion, "MCPConnectionRef", r)
}

func SealMCPConnection(r MCPConnectionRef) (MCPConnectionRef, error) {
	r.ContractVersion = MCPContractVersion
	r.Digest = ""
	digest, err := r.ComputeDigest()
	if err != nil {
		return MCPConnectionRef{}, err
	}
	r.Digest = digest
	return r, nil
}

type MCPToolObservation struct {
	Name               string      `json:"name"`
	DescriptionDigest  core.Digest `json:"description_digest"`
	InputSchemaDigest  core.Digest `json:"input_schema_digest"`
	OutputSchemaDigest core.Digest `json:"output_schema_digest,omitempty"`
}

func (o MCPToolObservation) Validate() error {
	if strings.TrimSpace(o.Name) == "" || len(o.Name) > 128 || o.DescriptionDigest.Validate() != nil || o.InputSchemaDigest.Validate() != nil {
		return invalid("MCP tool observation is invalid")
	}
	if o.OutputSchemaDigest != "" && o.OutputSchemaDigest.Validate() != nil {
		return invalid("MCP tool output schema digest is invalid")
	}
	return nil
}

type MCPCapabilitySnapshot struct {
	ContractVersion  string                        `json:"contract_version"`
	ID               string                        `json:"id"`
	Revision         core.Revision                 `json:"revision"`
	Digest           core.Digest                   `json:"digest"`
	Server           ObjectRef                     `json:"server"`
	Connection       ObjectRef                     `json:"connection"`
	ConnectionEpoch  core.Epoch                    `json:"connection_epoch"`
	ProtocolVersion  string                        `json:"protocol_version"`
	Tools            []MCPToolObservation          `json:"tools"`
	SourceDigest     core.Digest                   `json:"source_digest"`
	ValidationDigest core.Digest                   `json:"validation_digest"`
	Conformance      runtimeports.NamespacedNameV2 `json:"conformance"`
	Residuals        []Residual                    `json:"residuals,omitempty"`
	CreatedUnixNano  int64                         `json:"created_unix_nano"`
	ExpiresUnixNano  int64                         `json:"expires_unix_nano"`
}

func (s MCPCapabilitySnapshot) validateShape() error {
	if s.ContractVersion != MCPContractVersion || ValidateStableID(s.ID) != nil || s.Revision == 0 || s.Server.Validate() != nil || s.Connection.Validate() != nil || s.ConnectionEpoch == 0 || !validProtocolVersion(s.ProtocolVersion) || s.ProtocolVersion > MCPStableProtocolVersion || s.SourceDigest.Validate() != nil || s.ValidationDigest.Validate() != nil || s.CreatedUnixNano <= 0 || s.ExpiresUnixNano <= s.CreatedUnixNano {
		return invalid("MCP capability snapshot is incomplete")
	}
	if runtimeports.ValidateNamespacedNameV2(s.Conformance) != nil || len(s.Tools) > MaxSurfaceEntries || len(s.Residuals) > MaxResiduals {
		return invalid("MCP capability snapshot limits are invalid")
	}
	for i, tool := range s.Tools {
		if err := tool.Validate(); err != nil {
			return err
		}
		if i > 0 && s.Tools[i-1].Name >= tool.Name {
			return invalid("MCP tools must be sorted and unique")
		}
	}
	for _, residual := range s.Residuals {
		if err := residual.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (s MCPCapabilitySnapshot) Validate() error {
	if err := s.validateShape(); err != nil {
		return err
	}
	expected, err := s.ComputeDigest()
	if err != nil || s.Digest.Validate() != nil || expected != s.Digest {
		return conflict("MCP snapshot digest does not bind exact content")
	}
	return nil
}

func (s MCPCapabilitySnapshot) ComputeDigest() (core.Digest, error) {
	if err := s.validateShape(); err != nil {
		return "", err
	}
	s.Digest = ""
	return Seal("praxis.tool-mcp.mcp", MCPContractVersion, "MCPCapabilitySnapshot", s)
}

func SealMCPSnapshot(s MCPCapabilitySnapshot) (MCPCapabilitySnapshot, error) {
	s.ContractVersion = MCPContractVersion
	sort.Slice(s.Tools, func(i, j int) bool { return s.Tools[i].Name < s.Tools[j].Name })
	s.Digest = ""
	digest, err := s.ComputeDigest()
	if err != nil {
		return MCPCapabilitySnapshot{}, err
	}
	s.Digest = digest
	return s, nil
}

func validProtocolVersion(value string) bool {
	if len(value) != 10 || value[4] != '-' || value[7] != '-' {
		return false
	}
	for i, c := range []byte(value) {
		if i == 4 || i == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}
