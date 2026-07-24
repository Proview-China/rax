package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const OfficialSDKConnectContractVersionV1 = "praxis.tool-mcp.official-sdk-connect/v1"

// MCPConnectCredentialMaterialV1 is an adapter-only current projection. Secret
// values are deliberately excluded from JSON, canonical digests, receipts and
// domain facts.
type MCPConnectCredentialMaterialV1 struct {
	CredentialFactsDigest core.Digest       `json:"credential_facts_digest"`
	CheckedUnixNano       int64             `json:"checked_unix_nano"`
	ExpiresUnixNano       int64             `json:"expires_unix_nano"`
	Environment           map[string]string `json:"-"`
	Headers               map[string]string `json:"-"`
}

func (m MCPConnectCredentialMaterialV1) ValidateCurrent(intent toolcontract.MCPConnectIntentV1, config toolcontract.MCPTransportConfigV1, authorization runtimeports.ControlledMCPConnectPhysicalAuthorizationV1, now time.Time) error {
	if m.CredentialFactsDigest.Validate() != nil || m.CredentialFactsDigest != authorization.CredentialFactsDigest || m.CheckedUnixNano <= 0 || m.ExpiresUnixNano <= m.CheckedUnixNano {
		return invalid("MCP Connect credential material is invalid")
	}
	if now.IsZero() || now.UnixNano() < m.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Connect credential material clock regressed")
	}
	if !now.Before(time.Unix(0, m.ExpiresUnixNano)) || m.ExpiresUnixNano > authorization.UnifiedNotAfterUnixNano || m.ExpiresUnixNano > intent.NotAfterUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Connect credential material expired or exceeds its authority")
	}
	switch config.Kind {
	case toolcontract.MCPTransportStdioV1:
		if len(m.Headers) != 0 || config.Stdio == nil || !sameStringSetV1(config.Stdio.CredentialPlaceholders, mapKeysV1(m.Environment)) {
			return core.NewError(core.ErrorConflict, core.ReasonCredentialLeaseMissing, "MCP stdio credential material does not match declared placeholders")
		}
		for name, value := range m.Environment {
			if !validCredentialEnvironmentNameV1(name) || strings.ContainsRune(value, '\x00') {
				return invalid("MCP stdio credential material contains an invalid environment entry")
			}
		}
	case toolcontract.MCPTransportStreamableHTTPV1:
		if len(m.Environment) != 0 || config.StreamableHTTP == nil {
			return core.NewError(core.ErrorConflict, core.ReasonCredentialLeaseMissing, "MCP HTTP credential material is transport-incompatible")
		}
		for name, value := range m.Headers {
			if !validMCPHTTPHeaderV1(name, value) {
				return invalid("MCP HTTP credential material contains a forbidden header")
			}
		}
	default:
		return invalid("MCP Connect credential material uses an unsupported transport")
	}
	return nil
}

type MCPConnectCredentialMaterialRequestV1 struct {
	Intent                  toolcontract.ObjectRef               `json:"intent"`
	TransportConfig         toolcontract.MCPTransportConfigRefV1 `json:"transport_config"`
	TransportKind           runtimeports.NamespacedNameV2        `json:"transport_kind"`
	CredentialLeases        []runtimeports.CredentialLeaseRefV2  `json:"credential_leases,omitempty"`
	CredentialFactsDigest   core.Digest                          `json:"credential_facts_digest"`
	UnifiedNotAfterUnixNano int64                                `json:"unified_not_after_unix_nano"`
}

type MCPConnectCredentialMaterializerV1 interface {
	MaterializeCurrentMCPConnectCredentialsV1(context.Context, MCPConnectCredentialMaterialRequestV1) (MCPConnectCredentialMaterialV1, error)
}

// NoCredentialMCPConnectMaterializerV1 is production-neutral and only supports
// exact no-credential fixtures. It is not a credential backend.
type NoCredentialMCPConnectMaterializerV1 struct {
	Clock func() time.Time
}

func (m *NoCredentialMCPConnectMaterializerV1) MaterializeCurrentMCPConnectCredentialsV1(ctx context.Context, request MCPConnectCredentialMaterialRequestV1) (MCPConnectCredentialMaterialV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPConnectCredentialMaterialV1{}, err
	}
	if m == nil || m.Clock == nil {
		return MCPConnectCredentialMaterialV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "no-credential MCP materializer is unavailable")
	}
	if len(request.CredentialLeases) != 0 || request.CredentialFactsDigest.Validate() != nil || request.UnifiedNotAfterUnixNano <= 0 {
		return MCPConnectCredentialMaterialV1{}, core.NewError(core.ErrorForbidden, core.ReasonCredentialLeaseMissing, "credential-bearing MCP Connect requires an injected credential materializer")
	}
	now := m.Clock()
	if now.IsZero() || !now.Before(time.Unix(0, request.UnifiedNotAfterUnixNano)) {
		return MCPConnectCredentialMaterialV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Connect credential projection cannot be issued")
	}
	return MCPConnectCredentialMaterialV1{
		CredentialFactsDigest: request.CredentialFactsDigest,
		CheckedUnixNano:       now.UnixNano(),
		ExpiresUnixNano:       request.UnifiedNotAfterUnixNano,
		Environment:           map[string]string{},
		Headers:               map[string]string{},
	}, nil
}

type OfficialSDKConnectSessionV1 interface {
	InitializeResult() *officialmcp.InitializeResult
	ID() string
	Close() error
}

// OfficialSDKDiscoveryPageSessionV1 is the exact connected official SDK
// session surface required for one governed Discovery page.
type OfficialSDKDiscoveryPageSessionV1 interface {
	OfficialSDKConnectSessionV1
	ListTools(context.Context, *officialmcp.ListToolsParams) (*officialmcp.ListToolsResult, error)
	ListResources(context.Context, *officialmcp.ListResourcesParams) (*officialmcp.ListResourcesResult, error)
	ListPrompts(context.Context, *officialmcp.ListPromptsParams) (*officialmcp.ListPromptsResult, error)
}

type MCPDiscoveryPageSessionReaderV1 interface {
	InspectMCPDiscoveryPageSessionV1(context.Context, toolcontract.MCPConnectProtocolReceiptRefV1) (OfficialSDKDiscoveryPageSessionV1, error)
}

type mcpOfficialConnectDriverV1 interface {
	Connect(context.Context, toolcontract.MCPTransportConfigV1, MCPConnectCredentialMaterialV1) (OfficialSDKConnectSessionV1, []byte, error)
}

type officialMCPConnectDriverV1 struct{}

func (officialMCPConnectDriverV1) Connect(ctx context.Context, config toolcontract.MCPTransportConfigV1, credentials MCPConnectCredentialMaterialV1) (OfficialSDKConnectSessionV1, []byte, error) {
	var transport officialmcp.Transport
	switch config.Kind {
	case toolcontract.MCPTransportStdioV1:
		cmd := exec.CommandContext(ctx, config.Stdio.Executable, config.Stdio.Arguments...)
		cmd.Dir = config.Stdio.WorkingDirectory
		cmd.Env = canonicalEnvironmentV1(credentials.Environment)
		cmd.Stderr = io.Discard
		transport = &officialmcp.CommandTransport{Command: cmd}
	case toolcontract.MCPTransportStreamableHTTPV1:
		base := http.DefaultTransport.(*http.Transport).Clone()
		client := &http.Client{
			Transport:     mcpCredentialRoundTripperV1{base: base, headers: cloneStringMapV1(credentials.Headers)},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
		transport = &officialmcp.StreamableClientTransport{
			Endpoint:             config.StreamableHTTP.Endpoint,
			HTTPClient:           client,
			MaxRetries:           -1,
			DisableStandaloneSSE: config.StreamableHTTP.DisableStandaloneSSE,
		}
	default:
		return nil, nil, invalid("official MCP SDK Connect transport is unsupported")
	}
	client := officialmcp.NewClient(&officialmcp.Implementation{Name: "praxis-tool-mcp", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, nil, err
	}
	initialize := session.InitializeResult()
	if initialize == nil {
		return session, nil, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "official MCP SDK returned no initialize result")
	}
	response, err := json.Marshal(initialize)
	if err != nil || len(response) == 0 || len(response) > toolcontract.MaxMCPConnectInitializeReceiptBytesV1 {
		return session, nil, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "official MCP SDK initialize result is not recordable")
	}
	return session, response, nil
}

type mcpCredentialRoundTripperV1 struct {
	base    http.RoundTripper
	headers map[string]string
}

func (r mcpCredentialRoundTripperV1) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.Header = request.Header.Clone()
	for name, value := range r.headers {
		copy.Header.Set(name, value)
	}
	return r.base.RoundTrip(copy)
}

type MCPConnectPhysicalStateV1 string

const (
	MCPConnectPhysicalAdmittedV1 MCPConnectPhysicalStateV1 = "admitted"
	MCPConnectPhysicalObservedV1 MCPConnectPhysicalStateV1 = "observed"
	MCPConnectPhysicalUnknownV1  MCPConnectPhysicalStateV1 = "unknown"
)

type MCPConnectPhysicalEntryV1 struct {
	ID                  string                                                        `json:"id"`
	Revision            core.Revision                                                 `json:"revision"`
	StableKeyDigest     core.Digest                                                   `json:"stable_key_digest"`
	AuthorizationDigest core.Digest                                                   `json:"authorization_digest"`
	Authorization       runtimeports.ControlledMCPConnectPhysicalAuthorizationV1      `json:"authorization"`
	Intent              toolcontract.ObjectRef                                        `json:"intent"`
	TransportConfig     toolcontract.MCPTransportConfigRefV1                          `json:"transport_config"`
	AdmissionReceipt    runtimeports.ControlledOperationProviderAdmissionReceiptRefV2 `json:"admission_receipt"`
	NotAfterUnixNano    int64                                                         `json:"not_after_unix_nano"`
	State               MCPConnectPhysicalStateV1                                     `json:"state"`
	ProtocolReceipt     *toolcontract.MCPConnectProtocolReceiptV1                     `json:"protocol_receipt,omitempty"`
	UnknownReasonDigest core.Digest                                                   `json:"unknown_reason_digest,omitempty"`
	UpdatedUnixNano     int64                                                         `json:"updated_unix_nano"`
	session             OfficialSDKConnectSessionV1
}

type mcpConnectPhysicalStoreV1 interface {
	beginMCPConnectV1(context.Context, runtimeports.ControlledMCPConnectPhysicalAuthorizationV1, toolcontract.MCPConnectIntentV1, toolcontract.MCPTransportConfigV1, time.Time) (MCPConnectPhysicalEntryV1, bool, error)
	completeMCPConnectV1(context.Context, core.Digest, toolcontract.MCPConnectProtocolReceiptV1, OfficialSDKConnectSessionV1, time.Time) (MCPConnectPhysicalEntryV1, error)
	markMCPConnectUnknownV1(core.Digest, core.Digest, OfficialSDKConnectSessionV1, time.Time)
	InspectMCPConnectPhysicalV1(context.Context, core.Digest) (MCPConnectPhysicalEntryV1, error)
}

type InMemoryMCPConnectPhysicalRepositoryV1 struct {
	mu      sync.RWMutex
	entries map[string]MCPConnectPhysicalEntryV1
}

func NewInMemoryMCPConnectPhysicalRepositoryV1() *InMemoryMCPConnectPhysicalRepositoryV1 {
	return &InMemoryMCPConnectPhysicalRepositoryV1{entries: make(map[string]MCPConnectPhysicalEntryV1)}
}

func (r *InMemoryMCPConnectPhysicalRepositoryV1) beginMCPConnectV1(ctx context.Context, authorization runtimeports.ControlledMCPConnectPhysicalAuthorizationV1, intent toolcontract.MCPConnectIntentV1, config toolcontract.MCPTransportConfigV1, now time.Time) (MCPConnectPhysicalEntryV1, bool, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPConnectPhysicalEntryV1{}, false, err
	}
	if r == nil {
		return MCPConnectPhysicalEntryV1{}, false, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Connect physical repository is unavailable")
	}
	if err := authorization.ValidateCurrent(now); err != nil || intent.Validate() != nil || config.Validate() != nil {
		if err != nil {
			return MCPConnectPhysicalEntryV1{}, false, err
		}
		return MCPConnectPhysicalEntryV1{}, false, invalid("MCP Connect admission facts are invalid")
	}
	id := "mcp-connect-entry-" + strings.TrimPrefix(string(authorization.StableKeyDigest), "sha256:")
	admission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
		ID: id + "-admission", Revision: 1, StableKeyDigest: authorization.StableKeyDigest, Admitted: true,
	})
	if err != nil {
		return MCPConnectPhysicalEntryV1{}, false, err
	}
	entry := MCPConnectPhysicalEntryV1{
		ID: id, Revision: 1, StableKeyDigest: authorization.StableKeyDigest, AuthorizationDigest: authorization.Digest, Authorization: authorization,
		Intent: intent.Ref, TransportConfig: config.Ref, AdmissionReceipt: admission,
		NotAfterUnixNano: authorization.UnifiedNotAfterUnixNano,
		State:            MCPConnectPhysicalAdmittedV1, UpdatedUnixNano: now.UnixNano(),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return MCPConnectPhysicalEntryV1{}, false, err
	}
	if winner, ok := r.entries[id]; ok {
		if winner.StableKeyDigest != entry.StableKeyDigest || winner.AuthorizationDigest != entry.AuthorizationDigest || !reflect.DeepEqual(winner.Authorization, entry.Authorization) || winner.Intent != entry.Intent || winner.TransportConfig != entry.TransportConfig || winner.NotAfterUnixNano != entry.NotAfterUnixNano {
			return MCPConnectPhysicalEntryV1{}, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Connect physical stable key binds another command")
		}
		return cloneMCPConnectPhysicalEntryV1(winner), false, nil
	}
	r.entries[id] = entry
	return cloneMCPConnectPhysicalEntryV1(entry), true, nil
}

func (r *InMemoryMCPConnectPhysicalRepositoryV1) completeMCPConnectV1(ctx context.Context, stable core.Digest, receipt toolcontract.MCPConnectProtocolReceiptV1, session OfficialSDKConnectSessionV1, now time.Time) (MCPConnectPhysicalEntryV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPConnectPhysicalEntryV1{}, err
	}
	if r == nil || stable.Validate() != nil || receipt.Validate() != nil || nilLikeOfficialSDKConnectV1(session) || now.IsZero() {
		return MCPConnectPhysicalEntryV1{}, invalid("MCP Connect physical completion is invalid")
	}
	id := "mcp-connect-entry-" + strings.TrimPrefix(string(stable), "sha256:")
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[id]
	if !ok {
		return MCPConnectPhysicalEntryV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect physical Entry not found")
	}
	if receipt.StableKeyDigest != stable || receipt.Intent != entry.Intent || receipt.TransportConfig != entry.TransportConfig || receipt.AdmissionReceipt != entry.AdmissionReceipt {
		return MCPConnectPhysicalEntryV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Connect Protocol Receipt belongs to another Entry")
	}
	if entry.State == MCPConnectPhysicalObservedV1 {
		if entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref != receipt.Ref || !sameOfficialSDKConnectSessionV1(entry.session, session) {
			return MCPConnectPhysicalEntryV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Connect Entry already observed another response")
		}
		return cloneMCPConnectPhysicalEntryV1(entry), nil
	}
	if entry.State != MCPConnectPhysicalAdmittedV1 {
		return MCPConnectPhysicalEntryV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "unknown MCP Connect Entry is inspect-only")
	}
	receipt = toolcontract.CloneMCPConnectProtocolReceiptV1(receipt)
	entry.Revision++
	entry.State, entry.ProtocolReceipt, entry.session, entry.UpdatedUnixNano = MCPConnectPhysicalObservedV1, &receipt, session, now.UnixNano()
	r.entries[id] = entry
	return cloneMCPConnectPhysicalEntryV1(entry), nil
}

func (r *InMemoryMCPConnectPhysicalRepositoryV1) markMCPConnectUnknownV1(stable core.Digest, reason core.Digest, session OfficialSDKConnectSessionV1, now time.Time) {
	if r == nil || stable.Validate() != nil || reason.Validate() != nil || now.IsZero() {
		return
	}
	id := "mcp-connect-entry-" + strings.TrimPrefix(string(stable), "sha256:")
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[id]
	if !ok || entry.State != MCPConnectPhysicalAdmittedV1 {
		return
	}
	entry.Revision++
	entry.State, entry.UnknownReasonDigest, entry.session, entry.UpdatedUnixNano = MCPConnectPhysicalUnknownV1, reason, session, now.UnixNano()
	r.entries[id] = entry
}

func (r *InMemoryMCPConnectPhysicalRepositoryV1) InspectMCPConnectPhysicalV1(ctx context.Context, stable core.Digest) (MCPConnectPhysicalEntryV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPConnectPhysicalEntryV1{}, err
	}
	if r == nil || stable.Validate() != nil {
		return MCPConnectPhysicalEntryV1{}, invalid("MCP Connect physical Inspect is invalid")
	}
	id := "mcp-connect-entry-" + strings.TrimPrefix(string(stable), "sha256:")
	r.mu.RLock()
	entry, ok := r.entries[id]
	r.mu.RUnlock()
	if !ok {
		return MCPConnectPhysicalEntryV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect physical Entry not found")
	}
	return cloneMCPConnectPhysicalEntryV1(entry), nil
}

func (r *InMemoryMCPConnectPhysicalRepositoryV1) InspectMCPConnectProtocolReceiptV1(ctx context.Context, exact toolcontract.MCPConnectProtocolReceiptRefV1) (toolcontract.MCPConnectProtocolReceiptV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPConnectProtocolReceiptV1{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPConnectProtocolReceiptV1{}, invalid("MCP Connect Protocol Receipt exact Inspect is invalid")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		if entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref.ID != exact.ID {
			continue
		}
		if entry.ProtocolReceipt.Ref != exact {
			return toolcontract.MCPConnectProtocolReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect Protocol Receipt exact Ref drifted")
		}
		return toolcontract.CloneMCPConnectProtocolReceiptV1(*entry.ProtocolReceipt), nil
	}
	return toolcontract.MCPConnectProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect Protocol Receipt not found")
}

func (r *InMemoryMCPConnectPhysicalRepositoryV1) InspectMCPConnectProtocolReceiptByIDV1(ctx context.Context, id string) (toolcontract.MCPConnectProtocolReceiptV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPConnectProtocolReceiptV1{}, err
	}
	if r == nil || toolcontract.ValidateStableID(id) != nil {
		return toolcontract.MCPConnectProtocolReceiptV1{}, invalid("MCP Connect Protocol Receipt ID Inspect is invalid")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var found *toolcontract.MCPConnectProtocolReceiptV1
	for _, entry := range r.entries {
		if entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref.ID != id {
			continue
		}
		if found != nil && found.Ref != entry.ProtocolReceipt.Ref {
			return toolcontract.MCPConnectProtocolReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Connect Protocol Receipt ID is not unique")
		}
		copy := toolcontract.CloneMCPConnectProtocolReceiptV1(*entry.ProtocolReceipt)
		found = &copy
	}
	if found == nil {
		return toolcontract.MCPConnectProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect Protocol Receipt not found")
	}
	return toolcontract.CloneMCPConnectProtocolReceiptV1(*found), nil
}

func (r *InMemoryMCPConnectPhysicalRepositoryV1) inspectMCPConnectSessionV1(ctx context.Context, exact toolcontract.MCPConnectProtocolReceiptRefV1) (MCPConnectPhysicalEntryV1, OfficialSDKConnectSessionV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPConnectPhysicalEntryV1{}, nil, err
	}
	if r == nil || exact.Validate() != nil {
		return MCPConnectPhysicalEntryV1{}, nil, invalid("MCP Connect Session exact Inspect is invalid")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		if entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref.ID != exact.ID {
			continue
		}
		if entry.ProtocolReceipt.Ref != exact || entry.State != MCPConnectPhysicalObservedV1 || nilLikeOfficialSDKConnectV1(entry.session) {
			return MCPConnectPhysicalEntryV1{}, nil, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect Session exact closure drifted")
		}
		return cloneMCPConnectPhysicalEntryV1(entry), entry.session, nil
	}
	return MCPConnectPhysicalEntryV1{}, nil, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect Session not found")
}

func (r *InMemoryMCPConnectPhysicalRepositoryV1) InspectMCPDiscoveryPageSessionV1(ctx context.Context, exact toolcontract.MCPConnectProtocolReceiptRefV1) (OfficialSDKDiscoveryPageSessionV1, error) {
	_, session, err := r.inspectMCPConnectSessionV1(ctx, exact)
	if err != nil {
		return nil, err
	}
	discovery, ok := session.(OfficialSDKDiscoveryPageSessionV1)
	if !ok || nilLikeOfficialSDKConnectV1(discovery) {
		return nil, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "exact MCP Connect Session has no official Discovery page surface")
	}
	return discovery, nil
}

type OfficialSDKConnectExecutorV1 struct {
	intents     MCPConnectIntentReaderV1
	configs     MCPTransportConfigReaderV1
	servers     MCPServerDescriptorReaderV1
	credentials MCPConnectCredentialMaterializerV1
	entries     mcpConnectPhysicalStoreV1
	driver      mcpOfficialConnectDriverV1
	clock       func() time.Time
}

func NewOfficialSDKConnectExecutorV1(intents MCPConnectIntentReaderV1, configs MCPTransportConfigReaderV1, servers MCPServerDescriptorReaderV1, credentials MCPConnectCredentialMaterializerV1, entries *InMemoryMCPConnectPhysicalRepositoryV1, clock func() time.Time) (*OfficialSDKConnectExecutorV1, error) {
	return newOfficialSDKConnectExecutorV1(intents, configs, servers, credentials, entries, officialMCPConnectDriverV1{}, clock)
}

func newOfficialSDKConnectExecutorV1(intents MCPConnectIntentReaderV1, configs MCPTransportConfigReaderV1, servers MCPServerDescriptorReaderV1, credentials MCPConnectCredentialMaterializerV1, entries mcpConnectPhysicalStoreV1, driver mcpOfficialConnectDriverV1, clock func() time.Time) (*OfficialSDKConnectExecutorV1, error) {
	if nilLikeOfficialSDKConnectV1(intents) || nilLikeOfficialSDKConnectV1(configs) || nilLikeOfficialSDKConnectV1(servers) || nilLikeOfficialSDKConnectV1(credentials) || nilLikeOfficialSDKConnectV1(entries) || nilLikeOfficialSDKConnectV1(driver) || clock == nil {
		return nil, invalid("official MCP SDK Connect executor dependencies are incomplete")
	}
	return &OfficialSDKConnectExecutorV1{intents: intents, configs: configs, servers: servers, credentials: credentials, entries: entries, driver: driver, clock: clock}, nil
}

func (e *OfficialSDKConnectExecutorV1) ConnectControlledMCPV1(ctx context.Context, authorization runtimeports.ControlledMCPConnectPhysicalAuthorizationV1) (runtimeports.ControlledOperationProviderAdmissionReceiptRefV2, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if e == nil || nilLikeOfficialSDKConnectV1(e.intents) || nilLikeOfficialSDKConnectV1(e.configs) || nilLikeOfficialSDKConnectV1(e.servers) || nilLikeOfficialSDKConnectV1(e.credentials) || nilLikeOfficialSDKConnectV1(e.entries) || nilLikeOfficialSDKConnectV1(e.driver) || e.clock == nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "official MCP SDK Connect executor is unavailable")
	}
	if err := authorization.Validate(); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	previous, err := e.freshV1(time.Time{})
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	intentRef := toolcontract.ObjectRef{ID: authorization.DomainCommand.ID, Revision: authorization.DomainCommand.Revision, Digest: authorization.DomainCommand.Digest}
	intent, config, server, err := e.inspectExactCurrentV1(ctx, intentRef)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	previous, err = e.freshV1(previous)
	if err != nil || validateMCPConnectFactsV1(intent, config, server, authorization, previous) != nil {
		if err != nil {
			return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
		}
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, validateMCPConnectFactsV1(intent, config, server, authorization, previous)
	}
	material, err := e.credentials.MaterializeCurrentMCPConnectCredentialsV1(ctx, MCPConnectCredentialMaterialRequestV1{
		Intent: intent.Ref, TransportConfig: config.Ref, TransportKind: config.Kind,
		CredentialLeases:        append([]runtimeports.CredentialLeaseRefV2(nil), intent.CredentialLeases...),
		CredentialFactsDigest:   authorization.CredentialFactsDigest,
		UnifiedNotAfterUnixNano: authorization.UnifiedNotAfterUnixNano,
	})
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	intent2, config2, server2, err := e.inspectExactCurrentV1(ctx, intentRef)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	actual, err := e.freshV1(previous)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if !reflect.DeepEqual(intent, intent2) || !reflect.DeepEqual(config, config2) || !reflect.DeepEqual(server, server2) {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect facts drifted between S1 and actual entry")
	}
	if err = validateMCPConnectFactsV1(intent2, config2, server2, authorization, actual); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if err = material.ValidateCurrent(intent2, config2, authorization, actual); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	entry, created, err := e.entries.beginMCPConnectV1(ctx, authorization, intent2, config2, actual)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if !created {
		if entry.State == MCPConnectPhysicalObservedV1 {
			return entry.AdmissionReceipt, nil
		}
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Connect was already admitted and requires exact Inspect")
	}
	session, response, connectErr := e.driver.Connect(ctx, config2, material)
	observed, clockErr := e.freshV1(actual)
	if connectErr != nil || clockErr != nil || nilLikeOfficialSDKConnectV1(session) || len(response) == 0 {
		reason := "MCP Connect returned an unknown outcome"
		if connectErr != nil {
			reason = connectErr.Error()
		} else if clockErr != nil {
			reason = clockErr.Error()
		}
		e.entries.markMCPConnectUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte(reason)), session, nonZeroExecutionTimeV1(observed, actual))
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Connect outcome requires exact Inspect")
	}
	initialize := session.InitializeResult()
	if initialize == nil {
		e.entries.markMCPConnectUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte("initialize-result-missing")), session, observed)
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Connect initialize response is missing")
	}
	if err = validateMCPNegotiatedProtocolV1(server2, initialize.ProtocolVersion); err != nil {
		e.entries.markMCPConnectUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte("negotiated-protocol-outside-server-range")), session, observed)
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Connect negotiated protocol is outside the exact Server Descriptor range")
	}
	canonicalInitialize, marshalErr := json.Marshal(initialize)
	if marshalErr != nil || !bytes.Equal(response, canonicalInitialize) {
		e.entries.markMCPConnectUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte("initialize-response-session-drift")), session, observed)
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Connect initialize response differs from the exact Session observation")
	}
	receipt, err := toolcontract.SealMCPConnectProtocolReceiptV1(toolcontract.MCPConnectProtocolReceiptV1{
		Intent: intent2.Ref, TransportConfig: config2.Ref, StableKeyDigest: authorization.StableKeyDigest,
		AdmissionReceipt: entry.AdmissionReceipt, TransportKind: config2.Kind,
		NegotiatedProtocol: initialize.ProtocolVersion, ProviderSessionID: session.ID(),
		InitializeResponse: response, ObservedUnixNano: observed.UnixNano(),
	})
	if err != nil {
		e.entries.markMCPConnectUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte("receipt-seal-failed")), session, observed)
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Connect Protocol Receipt could not be sealed")
	}
	if _, err = e.entries.completeMCPConnectV1(ctx, authorization.StableKeyDigest, receipt, session, observed); err != nil {
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Connect Protocol Receipt persistence requires exact Inspect")
	}
	return entry.AdmissionReceipt, nil
}

func validateMCPNegotiatedProtocolV1(server toolcontract.MCPServerDescriptor, negotiated string) error {
	if server.Validate() != nil || negotiated < server.MinimumProtocol || negotiated > server.MaximumProtocol || negotiated > toolcontract.MCPStableProtocolVersion {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP negotiated protocol is outside the exact Server Descriptor range")
	}
	return nil
}

func (e *OfficialSDKConnectExecutorV1) InspectMCPConnectPhysicalV1(ctx context.Context, stable core.Digest) (MCPConnectPhysicalEntryV1, error) {
	if e == nil || nilLikeOfficialSDKConnectV1(e.entries) {
		return MCPConnectPhysicalEntryV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "official MCP SDK Connect Inspect is unavailable")
	}
	return e.entries.InspectMCPConnectPhysicalV1(ctx, stable)
}

func (e *OfficialSDKConnectExecutorV1) freshV1(previous time.Time) (time.Time, error) {
	now := e.clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "official MCP SDK Connect clock regressed")
	}
	return now, nil
}

func (e *OfficialSDKConnectExecutorV1) inspectExactCurrentV1(ctx context.Context, intentRef toolcontract.ObjectRef) (toolcontract.MCPConnectIntentV1, toolcontract.MCPTransportConfigV1, toolcontract.MCPServerDescriptor, error) {
	intent, err := e.intents.InspectMCPConnectIntentV1(ctx, intentRef)
	if err != nil {
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
	}
	currentIntent, err := e.intents.InspectCurrentMCPConnectIntentV1(ctx, intent.Ref.ID)
	if err != nil || !reflect.DeepEqual(intent, currentIntent) {
		if err != nil {
			return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
		}
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect Intent is no longer current")
	}
	config, err := e.configs.InspectMCPTransportConfigV1(ctx, intent.TransportConfig)
	if err != nil {
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
	}
	currentConfig, err := e.configs.InspectCurrentMCPTransportConfigV1(ctx, config.Ref.ID)
	if err != nil || !reflect.DeepEqual(config, currentConfig) {
		if err != nil {
			return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
		}
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Transport Config is no longer current")
	}
	server, err := e.servers.InspectMCPServerDescriptorV1(ctx, intent.Server)
	if err != nil {
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
	}
	currentServer, err := e.servers.InspectCurrentMCPServerDescriptorV1(ctx, server.ID)
	if err != nil || !reflect.DeepEqual(server, currentServer) {
		if err != nil {
			return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
		}
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Server Descriptor is no longer current")
	}
	return intent, config, server, nil
}

func validateMCPConnectFactsV1(intent toolcontract.MCPConnectIntentV1, config toolcontract.MCPTransportConfigV1, server toolcontract.MCPServerDescriptor, authorization runtimeports.ControlledMCPConnectPhysicalAuthorizationV1, now time.Time) error {
	if err := authorization.ValidateCurrent(now); err != nil {
		return err
	}
	if intent.Validate() != nil || config.Validate() != nil || server.Validate() != nil {
		return invalid("MCP Connect exact facts are invalid")
	}
	if !now.Before(time.Unix(0, intent.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Connect Intent expired")
	}
	if intent.RuntimeDomainCommandRefV1() != authorization.DomainCommand || intent.Operation != authorization.Operation || intent.OperationDigest != authorization.OperationDigest || intent.EffectID != authorization.EffectID || intent.EffectRevision != authorization.EffectFactRevision || intent.IntentDigest != authorization.IntentDigest || intent.Attempt != authorization.Attempt || intent.Provider != authorization.Provider || intent.ProviderTransport != authorization.ProviderTransport || intent.TransportConfig != config.Ref || intent.Server != config.Server || intent.Server != (toolcontract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest}) || config.ProviderTransport != authorization.ProviderTransport || config.ArtifactDigest != server.ArtifactDigest || config.ConfigDigest != server.ConfigDigest || config.NetworkScopeDigest != intent.NetworkScopeDigest || config.NetworkScopeDigest != server.NetworkScopeDigest || config.SandboxRequirementDigest != intent.SandboxRequirementDigest || !containsTransportV1(server.Transports, config.Kind) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect Tool facts differ from Runtime authorization")
	}
	return nil
}

func containsTransportV1(values []runtimeports.NamespacedNameV2, value runtimeports.NamespacedNameV2) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func cloneMCPConnectPhysicalEntryV1(entry MCPConnectPhysicalEntryV1) MCPConnectPhysicalEntryV1 {
	if entry.ProtocolReceipt != nil {
		copy := toolcontract.CloneMCPConnectProtocolReceiptV1(*entry.ProtocolReceipt)
		entry.ProtocolReceipt = &copy
	}
	return entry
}

func sameOfficialSDKConnectSessionV1(left, right OfficialSDKConnectSessionV1) bool {
	if nilLikeOfficialSDKConnectV1(left) || nilLikeOfficialSDKConnectV1(right) {
		return false
	}
	l, r := reflect.ValueOf(left), reflect.ValueOf(right)
	return l.Type() == r.Type() && l.Kind() == reflect.Pointer && l.Pointer() == r.Pointer()
}

func nilLikeOfficialSDKConnectV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func canonicalEnvironmentV1(values map[string]string) []string {
	keys := mapKeysV1(values)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func mapKeysV1(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sameStringSetV1(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func cloneStringMapV1(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func validCredentialEnvironmentNameV1(value string) bool {
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

func validMCPHTTPHeaderV1(name, value string) bool {
	canonical := http.CanonicalHeaderKey(name)
	if canonical == "" || canonical != name || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n") {
		return false
	}
	switch canonical {
	case "Connection", "Content-Length", "Host", "Mcp-Protocol-Version", "Mcp-Session-Id", "Proxy-Authorization", "Proxy-Authenticate", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return false
	}
	return true
}

var _ toolcontract.MCPConnectProtocolReceiptExactReaderV1 = (*InMemoryMCPConnectPhysicalRepositoryV1)(nil)
var _ toolcontract.MCPConnectProtocolReceiptIDReaderV1 = (*InMemoryMCPConnectPhysicalRepositoryV1)(nil)
