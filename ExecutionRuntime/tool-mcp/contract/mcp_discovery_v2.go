package contract

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const MCPDiscoveryContractVersionV2 = "praxis.tool-mcp.mcp-discovery/v2"

const (
	MaxMCPDiscoveryToolsV2     = 256
	MaxMCPDiscoveryResourcesV2 = 256
	MaxMCPDiscoveryPromptsV2   = 256
)

type MCPToolObservationV2 struct {
	Name               string      `json:"name"`
	Title              string      `json:"title,omitempty"`
	ObjectDigest       core.Digest `json:"object_digest"`
	DescriptionDigest  core.Digest `json:"description_digest"`
	InputSchemaDigest  core.Digest `json:"input_schema_digest"`
	OutputSchemaDigest core.Digest `json:"output_schema_digest"`
	AnnotationsDigest  core.Digest `json:"annotations_digest"`
	MetaDigest         core.Digest `json:"meta_digest"`
}

func (o MCPToolObservationV2) Validate() error {
	if boundedMCPTextV2(o.Name, 128, true) != nil || boundedMCPTextV2(o.Title, MaxStringBytes, false) != nil || o.ObjectDigest.Validate() != nil || o.DescriptionDigest.Validate() != nil || o.InputSchemaDigest.Validate() != nil || o.OutputSchemaDigest.Validate() != nil || o.AnnotationsDigest.Validate() != nil || o.MetaDigest.Validate() != nil {
		return invalid("MCP Tool discovery observation is invalid")
	}
	return nil
}

type MCPResourceObservationV2 struct {
	URI               string      `json:"uri"`
	Name              string      `json:"name"`
	Title             string      `json:"title,omitempty"`
	MIMEType          string      `json:"mime_type,omitempty"`
	Size              int64       `json:"size,omitempty"`
	ObjectDigest      core.Digest `json:"object_digest"`
	DescriptionDigest core.Digest `json:"description_digest"`
	AnnotationsDigest core.Digest `json:"annotations_digest"`
	MetaDigest        core.Digest `json:"meta_digest"`
}

func (o MCPResourceObservationV2) Validate() error {
	if boundedMCPTextV2(o.URI, 4096, true) != nil || boundedMCPTextV2(o.Name, 256, true) != nil || boundedMCPTextV2(o.Title, MaxStringBytes, false) != nil || boundedMCPTextV2(o.MIMEType, 256, false) != nil || o.Size < 0 || o.ObjectDigest.Validate() != nil || o.DescriptionDigest.Validate() != nil || o.AnnotationsDigest.Validate() != nil || o.MetaDigest.Validate() != nil {
		return invalid("MCP Resource discovery observation is invalid")
	}
	return nil
}

type MCPPromptObservationV2 struct {
	Name              string      `json:"name"`
	Title             string      `json:"title,omitempty"`
	ObjectDigest      core.Digest `json:"object_digest"`
	DescriptionDigest core.Digest `json:"description_digest"`
	ArgumentsDigest   core.Digest `json:"arguments_digest"`
	MetaDigest        core.Digest `json:"meta_digest"`
}

func (o MCPPromptObservationV2) Validate() error {
	if boundedMCPTextV2(o.Name, 128, true) != nil || boundedMCPTextV2(o.Title, MaxStringBytes, false) != nil || o.ObjectDigest.Validate() != nil || o.DescriptionDigest.Validate() != nil || o.ArgumentsDigest.Validate() != nil || o.MetaDigest.Validate() != nil {
		return invalid("MCP Prompt discovery observation is invalid")
	}
	return nil
}

type MCPCapabilitySnapshotV2 struct {
	ContractVersion          string                        `json:"contract_version"`
	ID                       string                        `json:"id"`
	Revision                 core.Revision                 `json:"revision"`
	Digest                   core.Digest                   `json:"digest"`
	Server                   ObjectRef                     `json:"server"`
	Connection               ObjectRef                     `json:"connection"`
	ConnectionEpoch          core.Epoch                    `json:"connection_epoch"`
	ProtocolVersion          string                        `json:"protocol_version"`
	ServerInfoDigest         core.Digest                   `json:"server_info_digest"`
	ServerCapabilitiesDigest core.Digest                   `json:"server_capabilities_digest"`
	InstructionsDigest       core.Digest                   `json:"instructions_digest"`
	Tools                    []MCPToolObservationV2        `json:"tools"`
	Resources                []MCPResourceObservationV2    `json:"resources"`
	Prompts                  []MCPPromptObservationV2      `json:"prompts"`
	SourceDigest             core.Digest                   `json:"source_digest"`
	ValidationDigest         core.Digest                   `json:"validation_digest"`
	Conformance              runtimeports.NamespacedNameV2 `json:"conformance"`
	Residuals                []Residual                    `json:"residuals,omitempty"`
	CreatedUnixNano          int64                         `json:"created_unix_nano"`
	ExpiresUnixNano          int64                         `json:"expires_unix_nano"`
}

func (s MCPCapabilitySnapshotV2) validateShape() error {
	if s.ContractVersion != MCPDiscoveryContractVersionV2 || ValidateStableID(s.ID) != nil || s.Revision == 0 || s.Server.Validate() != nil || s.Connection.Validate() != nil || s.ConnectionEpoch == 0 || !validProtocolVersion(s.ProtocolVersion) || s.ProtocolVersion > MCPStableProtocolVersion || s.ServerInfoDigest.Validate() != nil || s.ServerCapabilitiesDigest.Validate() != nil || s.InstructionsDigest.Validate() != nil || s.SourceDigest.Validate() != nil || s.ValidationDigest.Validate() != nil || runtimeports.ValidateNamespacedNameV2(s.Conformance) != nil || s.CreatedUnixNano <= 0 || s.ExpiresUnixNano <= s.CreatedUnixNano {
		return invalid("MCP Capability Snapshot V2 is incomplete")
	}
	if len(s.Tools) > MaxMCPDiscoveryToolsV2 || len(s.Resources) > MaxMCPDiscoveryResourcesV2 || len(s.Prompts) > MaxMCPDiscoveryPromptsV2 || len(s.Residuals) > MaxResiduals {
		return invalid("MCP Capability Snapshot V2 exceeds limits")
	}
	for i, tool := range s.Tools {
		if tool.Validate() != nil || i > 0 && s.Tools[i-1].Name >= tool.Name {
			return invalid("MCP Capability Snapshot V2 Tools are invalid or not unique")
		}
	}
	for i, resource := range s.Resources {
		if resource.Validate() != nil || i > 0 && s.Resources[i-1].URI >= resource.URI {
			return invalid("MCP Capability Snapshot V2 Resources are invalid or not unique")
		}
	}
	for i, prompt := range s.Prompts {
		if prompt.Validate() != nil || i > 0 && s.Prompts[i-1].Name >= prompt.Name {
			return invalid("MCP Capability Snapshot V2 Prompts are invalid or not unique")
		}
	}
	for _, residual := range s.Residuals {
		if residual.Validate() != nil {
			return invalid("MCP Capability Snapshot V2 Residual is invalid")
		}
	}
	return nil
}

func (s MCPCapabilitySnapshotV2) Validate() error {
	if err := s.validateShape(); err != nil {
		return err
	}
	id, err := DeriveMCPCapabilitySnapshotIDV2(s.Server, s.Connection, s.ConnectionEpoch)
	if err != nil || id != s.ID {
		return conflict("MCP Capability Snapshot V2 ID drifted")
	}
	validation, err := s.ComputeValidationDigest()
	if err != nil || validation != s.ValidationDigest {
		return conflict("MCP Capability Snapshot V2 validation digest drifted")
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Digest {
		return conflict("MCP Capability Snapshot V2 digest drifted")
	}
	return nil
}

func (s MCPCapabilitySnapshotV2) ValidateCurrent(now time.Time) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < s.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Capability Snapshot V2 clock regressed")
	}
	if !now.Before(time.Unix(0, s.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Capability Snapshot V2 expired")
	}
	return nil
}

func (s MCPCapabilitySnapshotV2) ComputeValidationDigest() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp", MCPDiscoveryContractVersionV2, "MCPCapabilitySnapshotValidationV2", struct {
		Tools     []MCPToolObservationV2     `json:"tools"`
		Resources []MCPResourceObservationV2 `json:"resources"`
		Prompts   []MCPPromptObservationV2   `json:"prompts"`
	}{s.Tools, s.Resources, s.Prompts})
}

func (s MCPCapabilitySnapshotV2) ComputeDigest() (core.Digest, error) {
	if err := s.validateShape(); err != nil {
		return "", err
	}
	s = CloneMCPCapabilitySnapshotV2(s)
	s.Digest = ""
	return Seal("praxis.tool-mcp.mcp", MCPDiscoveryContractVersionV2, "MCPCapabilitySnapshotV2", s)
}

func SealMCPCapabilitySnapshotV2(s MCPCapabilitySnapshotV2) (MCPCapabilitySnapshotV2, error) {
	s = CloneMCPCapabilitySnapshotV2(s)
	s.ContractVersion = MCPDiscoveryContractVersionV2
	sort.Slice(s.Tools, func(i, j int) bool { return s.Tools[i].Name < s.Tools[j].Name })
	sort.Slice(s.Resources, func(i, j int) bool { return s.Resources[i].URI < s.Resources[j].URI })
	sort.Slice(s.Prompts, func(i, j int) bool { return s.Prompts[i].Name < s.Prompts[j].Name })
	id, err := DeriveMCPCapabilitySnapshotIDV2(s.Server, s.Connection, s.ConnectionEpoch)
	if err != nil {
		return MCPCapabilitySnapshotV2{}, err
	}
	if s.ID != "" && s.ID != id {
		return MCPCapabilitySnapshotV2{}, conflict("supplied MCP Capability Snapshot V2 ID drifted")
	}
	s.ID = id
	validation, err := s.ComputeValidationDigest()
	if err != nil {
		return MCPCapabilitySnapshotV2{}, err
	}
	if s.ValidationDigest != "" && s.ValidationDigest != validation {
		return MCPCapabilitySnapshotV2{}, conflict("supplied MCP Capability Snapshot V2 validation digest drifted")
	}
	s.ValidationDigest = validation
	provided := s.Digest
	s.Digest = ""
	digest, err := s.ComputeDigest()
	if err != nil {
		return MCPCapabilitySnapshotV2{}, err
	}
	if provided != "" && provided != digest {
		return MCPCapabilitySnapshotV2{}, conflict("supplied MCP Capability Snapshot V2 digest drifted")
	}
	s.Digest = digest
	return s, s.Validate()
}

func DeriveMCPCapabilitySnapshotIDV2(server, connection ObjectRef, epoch core.Epoch) (string, error) {
	if server.Validate() != nil || connection.Validate() != nil || epoch == 0 {
		return "", invalid("MCP Capability Snapshot V2 identity inputs are invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp", MCPDiscoveryContractVersionV2, "MCPCapabilitySnapshotIdentityV2", struct {
		Server     ObjectRef  `json:"server"`
		Connection ObjectRef  `json:"connection"`
		Epoch      core.Epoch `json:"epoch"`
	}{server, connection, epoch})
	if err != nil {
		return "", err
	}
	return "mcp-snapshot-v2-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func CloneMCPCapabilitySnapshotV2(s MCPCapabilitySnapshotV2) MCPCapabilitySnapshotV2 {
	s.Tools = append([]MCPToolObservationV2(nil), s.Tools...)
	s.Resources = append([]MCPResourceObservationV2(nil), s.Resources...)
	s.Prompts = append([]MCPPromptObservationV2(nil), s.Prompts...)
	s.Residuals = append([]Residual(nil), s.Residuals...)
	return s
}

func (s MCPCapabilitySnapshotV2) ObjectRef() ObjectRef {
	return ObjectRef{ID: s.ID, Revision: s.Revision, Digest: s.Digest}
}

type MCPCapabilitySnapshotExactReaderV2 interface {
	InspectMCPCapabilitySnapshotV2(context.Context, ObjectRef) (MCPCapabilitySnapshotV2, error)
}

type MCPCapabilitySnapshotCurrentReaderV2 interface {
	InspectCurrentMCPCapabilitySnapshotV2(context.Context, string) (MCPCapabilitySnapshotV2, error)
}

func boundedMCPTextV2(value string, maximum int, required bool) error {
	if len(value) > maximum || strings.TrimSpace(value) != value || required && value == "" {
		return invalid("MCP discovery text is blank, unbounded or not canonical")
	}
	return nil
}
