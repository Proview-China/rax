package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const OfficialSDKDiscoveryContractVersionV1 = "praxis.tool-mcp.official-sdk-discovery/v1"

type OfficialSDKDiscoveryLimitsV1 struct {
	MaxPages     int `json:"max_pages"`
	MaxTools     int `json:"max_tools"`
	MaxResources int `json:"max_resources"`
	MaxPrompts   int `json:"max_prompts"`
}

func DefaultOfficialSDKDiscoveryLimitsV1() OfficialSDKDiscoveryLimitsV1 {
	return OfficialSDKDiscoveryLimitsV1{MaxPages: 32, MaxTools: toolcontract.MaxMCPDiscoveryToolsV2, MaxResources: toolcontract.MaxMCPDiscoveryResourcesV2, MaxPrompts: toolcontract.MaxMCPDiscoveryPromptsV2}
}

func (l OfficialSDKDiscoveryLimitsV1) Validate() error {
	if l.MaxPages <= 0 || l.MaxPages > 64 || l.MaxTools <= 0 || l.MaxTools > toolcontract.MaxMCPDiscoveryToolsV2 || l.MaxResources <= 0 || l.MaxResources > toolcontract.MaxMCPDiscoveryResourcesV2 || l.MaxPrompts <= 0 || l.MaxPrompts > toolcontract.MaxMCPDiscoveryPromptsV2 {
		return invalid("official MCP SDK discovery limits are invalid")
	}
	return nil
}

type OfficialSDKDiscoveryRequestV1 struct {
	Server                   toolcontract.ObjectRef        `json:"server"`
	Connection               toolcontract.MCPConnectionRef `json:"connection"`
	SnapshotRevision         core.Revision                 `json:"snapshot_revision"`
	Conformance              runtimeports.NamespacedNameV2 `json:"conformance"`
	RequestedExpiresUnixNano int64                         `json:"requested_expires_unix_nano"`
}

func (r OfficialSDKDiscoveryRequestV1) Validate(now time.Time) error {
	if r.Server.Validate() != nil || r.Connection.Validate() != nil || r.Connection.Server != r.Server || r.SnapshotRevision == 0 || runtimeports.ValidateNamespacedNameV2(r.Conformance) != nil || r.RequestedExpiresUnixNano <= 0 || r.RequestedExpiresUnixNano > r.Connection.ExpiresUnixNano {
		return invalid("official MCP SDK discovery request is invalid")
	}
	if now.IsZero() || now.UnixNano() < r.Connection.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "official MCP SDK discovery clock regressed")
	}
	if !now.Before(time.Unix(0, r.RequestedExpiresUnixNano)) || !now.Before(time.Unix(0, r.Connection.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "official MCP SDK discovery request expired")
	}
	return nil
}

type officialSDKSessionV1 interface {
	InitializeResult() *officialmcp.InitializeResult
	ID() string
	ListTools(context.Context, *officialmcp.ListToolsParams) (*officialmcp.ListToolsResult, error)
	ListResources(context.Context, *officialmcp.ListResourcesParams) (*officialmcp.ListResourcesResult, error)
	ListPrompts(context.Context, *officialmcp.ListPromptsParams) (*officialmcp.ListPromptsResult, error)
}

type OfficialSDKDiscoveryV1 struct {
	session officialSDKSessionV1
	clock   func() time.Time
	limits  OfficialSDKDiscoveryLimitsV1
}

func NewOfficialSDKDiscoveryV1(session *officialmcp.ClientSession, clock func() time.Time, limits OfficialSDKDiscoveryLimitsV1) (*OfficialSDKDiscoveryV1, error) {
	return newOfficialSDKDiscoveryV1(session, clock, limits)
}

func newOfficialSDKDiscoveryV1(session officialSDKSessionV1, clock func() time.Time, limits OfficialSDKDiscoveryLimitsV1) (*OfficialSDKDiscoveryV1, error) {
	if nilLikeOfficialSDKV1(session) || clock == nil || limits.Validate() != nil {
		return nil, invalid("official MCP SDK discovery dependencies are invalid")
	}
	return &OfficialSDKDiscoveryV1{session: session, clock: clock, limits: limits}, nil
}

func (d *OfficialSDKDiscoveryV1) DiscoverV1(ctx context.Context, request OfficialSDKDiscoveryRequestV1) (toolcontract.MCPCapabilitySnapshotV2, error) {
	if d == nil || nilLikeOfficialSDKV1(d.session) || d.clock == nil || d.limits.Validate() != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "official MCP SDK discovery is unavailable")
	}
	if err := officialSDKContextV1(ctx); err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	now, err := d.freshTimeV1(time.Time{}, request.RequestedExpiresUnixNano)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	if err = request.Validate(now); err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	initialize := d.session.InitializeResult()
	if initialize == nil || initialize.ServerInfo == nil || initialize.Capabilities == nil || initialize.ProtocolVersion != request.Connection.NegotiatedProtocol || initialize.ProtocolVersion != toolcontract.MCPStableProtocolVersion {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "official MCP SDK initialize result drifted from the exact Connection")
	}
	if sessionID := d.session.ID(); sessionID != "" && sessionID != request.Connection.SessionID {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "official MCP SDK Session ID drifted from the exact Connection")
	}

	serverInfoDigest, err := digestOfficialSDKValueV1("MCPServerInfoV1", initialize.ServerInfo)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	serverCapabilitiesDigest, err := digestOfficialSDKValueV1("MCPServerCapabilitiesV1", initialize.Capabilities)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	instructionsDigest := core.DigestBytes([]byte(initialize.Instructions))

	var tools []toolcontract.MCPToolObservationV2
	if initialize.Capabilities.Tools != nil {
		tools, now, err = d.discoverToolsV1(ctx, now, request.RequestedExpiresUnixNano)
		if err != nil {
			return toolcontract.MCPCapabilitySnapshotV2{}, err
		}
	}
	var resources []toolcontract.MCPResourceObservationV2
	if initialize.Capabilities.Resources != nil {
		resources, now, err = d.discoverResourcesV1(ctx, now, request.RequestedExpiresUnixNano)
		if err != nil {
			return toolcontract.MCPCapabilitySnapshotV2{}, err
		}
	}
	var prompts []toolcontract.MCPPromptObservationV2
	if initialize.Capabilities.Prompts != nil {
		prompts, now, err = d.discoverPromptsV1(ctx, now, request.RequestedExpiresUnixNano)
		if err != nil {
			return toolcontract.MCPCapabilitySnapshotV2{}, err
		}
	}
	final, err := d.freshTimeV1(now, request.RequestedExpiresUnixNano)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	// MCP list ordering is not semantic. Normalize all namespaces before both
	// SourceDigest and the authoritative snapshot are sealed.
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	sort.Slice(resources, func(i, j int) bool { return resources[i].URI < resources[j].URI })
	sort.Slice(prompts, func(i, j int) bool { return prompts[i].Name < prompts[j].Name })
	sourceDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp.official-sdk", OfficialSDKDiscoveryContractVersionV1, "OfficialSDKDiscoverySourceV1", struct {
		ServerInfo         core.Digest                             `json:"server_info"`
		ServerCapabilities core.Digest                             `json:"server_capabilities"`
		Instructions       core.Digest                             `json:"instructions"`
		Tools              []toolcontract.MCPToolObservationV2     `json:"tools"`
		Resources          []toolcontract.MCPResourceObservationV2 `json:"resources"`
		Prompts            []toolcontract.MCPPromptObservationV2   `json:"prompts"`
	}{serverInfoDigest, serverCapabilitiesDigest, instructionsDigest, tools, resources, prompts})
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	connectionRef := toolcontract.ObjectRef{ID: request.Connection.ID, Revision: request.Connection.Revision, Digest: request.Connection.Digest}
	return toolcontract.SealMCPCapabilitySnapshotV2(toolcontract.MCPCapabilitySnapshotV2{
		Revision: request.SnapshotRevision, Server: request.Server, Connection: connectionRef, ConnectionEpoch: request.Connection.Epoch,
		ProtocolVersion: request.Connection.NegotiatedProtocol, ServerInfoDigest: serverInfoDigest, ServerCapabilitiesDigest: serverCapabilitiesDigest,
		InstructionsDigest: instructionsDigest, Tools: tools, Resources: resources, Prompts: prompts, SourceDigest: sourceDigest,
		Conformance: request.Conformance, CreatedUnixNano: final.UnixNano(), ExpiresUnixNano: request.RequestedExpiresUnixNano,
	})
}

func (d *OfficialSDKDiscoveryV1) discoverToolsV1(ctx context.Context, previous time.Time, expires int64) ([]toolcontract.MCPToolObservationV2, time.Time, error) {
	var result []toolcontract.MCPToolObservationV2
	cursor := ""
	seen := map[string]struct{}{"": {}}
	for page := 0; page < d.limits.MaxPages; page++ {
		response, err := d.session.ListTools(ctx, &officialmcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, time.Time{}, err
		}
		previous, err = d.freshTimeV1(previous, expires)
		if err != nil || response == nil {
			if err == nil {
				err = invalid("official MCP SDK returned a nil Tools page")
			}
			return nil, time.Time{}, err
		}
		for _, tool := range response.Tools {
			if tool == nil || len(result) >= d.limits.MaxTools {
				return nil, time.Time{}, invalid("official MCP SDK Tools page is nil or exceeds limits")
			}
			observation, err := toolObservationFromOfficialSDKV1(tool)
			if err != nil {
				return nil, time.Time{}, err
			}
			result = append(result, observation)
		}
		if response.NextCursor == "" {
			return result, previous, nil
		}
		if _, exists := seen[response.NextCursor]; exists {
			return nil, time.Time{}, officialSDKConflictV1("official MCP SDK Tools pagination cursor cycled")
		}
		seen[response.NextCursor], cursor = struct{}{}, response.NextCursor
	}
	return nil, time.Time{}, invalid("official MCP SDK Tools pagination exceeds page limit")
}

func (d *OfficialSDKDiscoveryV1) discoverResourcesV1(ctx context.Context, previous time.Time, expires int64) ([]toolcontract.MCPResourceObservationV2, time.Time, error) {
	var result []toolcontract.MCPResourceObservationV2
	cursor := ""
	seen := map[string]struct{}{"": {}}
	for page := 0; page < d.limits.MaxPages; page++ {
		response, err := d.session.ListResources(ctx, &officialmcp.ListResourcesParams{Cursor: cursor})
		if err != nil {
			return nil, time.Time{}, err
		}
		previous, err = d.freshTimeV1(previous, expires)
		if err != nil || response == nil {
			if err == nil {
				err = invalid("official MCP SDK returned a nil Resources page")
			}
			return nil, time.Time{}, err
		}
		for _, resource := range response.Resources {
			if resource == nil || len(result) >= d.limits.MaxResources {
				return nil, time.Time{}, invalid("official MCP SDK Resources page is nil or exceeds limits")
			}
			observation, err := resourceObservationFromOfficialSDKV1(resource)
			if err != nil {
				return nil, time.Time{}, err
			}
			result = append(result, observation)
		}
		if response.NextCursor == "" {
			return result, previous, nil
		}
		if _, exists := seen[response.NextCursor]; exists {
			return nil, time.Time{}, officialSDKConflictV1("official MCP SDK Resources pagination cursor cycled")
		}
		seen[response.NextCursor], cursor = struct{}{}, response.NextCursor
	}
	return nil, time.Time{}, invalid("official MCP SDK Resources pagination exceeds page limit")
}

func (d *OfficialSDKDiscoveryV1) discoverPromptsV1(ctx context.Context, previous time.Time, expires int64) ([]toolcontract.MCPPromptObservationV2, time.Time, error) {
	var result []toolcontract.MCPPromptObservationV2
	cursor := ""
	seen := map[string]struct{}{"": {}}
	for page := 0; page < d.limits.MaxPages; page++ {
		response, err := d.session.ListPrompts(ctx, &officialmcp.ListPromptsParams{Cursor: cursor})
		if err != nil {
			return nil, time.Time{}, err
		}
		previous, err = d.freshTimeV1(previous, expires)
		if err != nil || response == nil {
			if err == nil {
				err = invalid("official MCP SDK returned a nil Prompts page")
			}
			return nil, time.Time{}, err
		}
		for _, prompt := range response.Prompts {
			if prompt == nil || len(result) >= d.limits.MaxPrompts {
				return nil, time.Time{}, invalid("official MCP SDK Prompts page is nil or exceeds limits")
			}
			observation, err := promptObservationFromOfficialSDKV1(prompt)
			if err != nil {
				return nil, time.Time{}, err
			}
			result = append(result, observation)
		}
		if response.NextCursor == "" {
			return result, previous, nil
		}
		if _, exists := seen[response.NextCursor]; exists {
			return nil, time.Time{}, officialSDKConflictV1("official MCP SDK Prompts pagination cursor cycled")
		}
		seen[response.NextCursor], cursor = struct{}{}, response.NextCursor
	}
	return nil, time.Time{}, invalid("official MCP SDK Prompts pagination exceeds page limit")
}

func toolObservationFromOfficialSDKV1(value *officialmcp.Tool) (toolcontract.MCPToolObservationV2, error) {
	object, err := digestOfficialSDKValueV1("MCPToolObjectV1", value)
	if err != nil {
		return toolcontract.MCPToolObservationV2{}, err
	}
	input, err := digestOfficialSDKValueV1("MCPToolInputSchemaV1", value.InputSchema)
	if err != nil {
		return toolcontract.MCPToolObservationV2{}, err
	}
	output, err := digestOfficialSDKValueV1("MCPToolOutputSchemaV1", value.OutputSchema)
	if err != nil {
		return toolcontract.MCPToolObservationV2{}, err
	}
	annotations, err := digestOfficialSDKValueV1("MCPToolAnnotationsV1", value.Annotations)
	if err != nil {
		return toolcontract.MCPToolObservationV2{}, err
	}
	meta, err := digestOfficialSDKValueV1("MCPToolMetaV1", value.Meta)
	if err != nil {
		return toolcontract.MCPToolObservationV2{}, err
	}
	result := toolcontract.MCPToolObservationV2{Name: value.Name, Title: value.Title, ObjectDigest: object, DescriptionDigest: core.DigestBytes([]byte(value.Description)), InputSchemaDigest: input, OutputSchemaDigest: output, AnnotationsDigest: annotations, MetaDigest: meta}
	return result, result.Validate()
}

func resourceObservationFromOfficialSDKV1(value *officialmcp.Resource) (toolcontract.MCPResourceObservationV2, error) {
	object, err := digestOfficialSDKValueV1("MCPResourceObjectV1", value)
	if err != nil {
		return toolcontract.MCPResourceObservationV2{}, err
	}
	annotations, err := digestOfficialSDKValueV1("MCPResourceAnnotationsV1", value.Annotations)
	if err != nil {
		return toolcontract.MCPResourceObservationV2{}, err
	}
	meta, err := digestOfficialSDKValueV1("MCPResourceMetaV1", value.Meta)
	if err != nil {
		return toolcontract.MCPResourceObservationV2{}, err
	}
	result := toolcontract.MCPResourceObservationV2{URI: value.URI, Name: value.Name, Title: value.Title, MIMEType: value.MIMEType, Size: value.Size, ObjectDigest: object, DescriptionDigest: core.DigestBytes([]byte(value.Description)), AnnotationsDigest: annotations, MetaDigest: meta}
	return result, result.Validate()
}

func promptObservationFromOfficialSDKV1(value *officialmcp.Prompt) (toolcontract.MCPPromptObservationV2, error) {
	object, err := digestOfficialSDKValueV1("MCPPromptObjectV1", value)
	if err != nil {
		return toolcontract.MCPPromptObservationV2{}, err
	}
	arguments, err := digestOfficialSDKValueV1("MCPPromptArgumentsV1", value.Arguments)
	if err != nil {
		return toolcontract.MCPPromptObservationV2{}, err
	}
	meta, err := digestOfficialSDKValueV1("MCPPromptMetaV1", value.Meta)
	if err != nil {
		return toolcontract.MCPPromptObservationV2{}, err
	}
	result := toolcontract.MCPPromptObservationV2{Name: value.Name, Title: value.Title, ObjectDigest: object, DescriptionDigest: core.DigestBytes([]byte(value.Description)), ArgumentsDigest: arguments, MetaDigest: meta}
	return result, result.Validate()
}

func digestOfficialSDKValueV1(discriminator string, value any) (core.Digest, error) {
	payload, err := canonicalOfficialSDKValueV1(value)
	if err != nil {
		return "", err
	}
	var decoded any
	if err = core.DecodeStrictJSON(payload, &decoded); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp.official-sdk", OfficialSDKDiscoveryContractVersionV1, discriminator, decoded)
}

func canonicalOfficialSDKValueV1(value any) (json.RawMessage, error) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) == 0 || len(payload) > MaxMessageBytes {
		return nil, invalid("official MCP SDK value cannot be canonically encoded")
	}
	var decoded any
	if err = core.DecodeStrictJSON(payload, &decoded); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(decoded)
	if err != nil || len(canonical) > MaxMessageBytes {
		return nil, invalid("official MCP SDK value cannot be canonically encoded")
	}
	return canonical, nil
}

func (d *OfficialSDKDiscoveryV1) freshTimeV1(previous time.Time, expires int64) (time.Time, error) {
	now := d.clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "official MCP SDK discovery clock regressed")
	}
	if expires > 0 && !now.Before(time.Unix(0, expires)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "official MCP SDK discovery crossed its current window")
	}
	return now, nil
}

func officialSDKContextV1(ctx context.Context) error {
	if ctx == nil {
		return invalid("official MCP SDK discovery context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func nilLikeOfficialSDKV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func officialSDKConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, message)
}
