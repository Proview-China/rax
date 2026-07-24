package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

const (
	ContractVersionV1 = "praxis.tool-mcp.cli/v1"
	maxListPagesV1    = 1024
)

type CatalogPortV1 interface {
	ListRegistryV1(context.Context, api.ListRegistryRequestV1) (api.ListRegistryResultV1, error)
}

type InspectorPortV1 interface {
	InspectCapabilityV1(context.Context, contract.ObjectRef) (contract.CapabilityDescriptor, registry.Record, error)
	InspectToolV1(context.Context, contract.ObjectRef) (contract.ToolDescriptor, registry.Record, error)
	InspectPackageV1(context.Context, contract.ObjectRef) (contract.ToolPackageManifest, registry.Record, error)
	InspectToolAliasV1(context.Context, contract.ToolAliasRefV1) (contract.ToolAliasV1, registry.Record, error)
	InspectMCPToolMappingV1(context.Context, contract.MCPToolMappingManifestRefV1) (contract.MCPToolMappingManifestV1, registry.Record, error)
}

type MCPStatusPortV1 interface {
	InspectMCPConnectionStatusV1(context.Context, contract.ObjectRef) (mcp.ConnectionRecord, error)
}

type MCPConnectReadPortV1 interface {
	InspectMCPConnectIntentV1(context.Context, contract.ObjectRef) (contract.MCPConnectIntentV1, error)
	InspectMCPConnectProtocolReceiptV1(context.Context, contract.MCPConnectProtocolReceiptRefV1) (contract.MCPConnectProtocolReceiptV1, error)
	InspectMCPConnectionFactV2(context.Context, contract.MCPConnectionFactRefV2) (contract.MCPConnectionFactV2, error)
	InspectMCPConnectDomainResultV1(context.Context, contract.ObjectRef) (contract.MCPConnectDomainResultFactV1, error)
	InspectMCPConnectApplySettlementV1(context.Context, contract.ObjectRef) (contract.MCPConnectApplySettlementFactV1, error)
	InspectCurrentMCPConnectionAvailabilityV1(context.Context, contract.MCPConnectionFactRefV2, time.Duration) (contract.MCPConnectionAvailabilityCurrentProjectionV1, error)
}

type MCPDiscoveryReadPortV2 interface {
	InspectCurrentMCPCapabilitySnapshotV2(context.Context, contract.ObjectRef) (contract.MCPCapabilitySnapshotV2, error)
}

type MCPDiscoveryReadPortV3 interface {
	InspectCurrentMCPCapabilitySnapshotV3(context.Context, contract.ObjectRef) (contract.MCPCapabilitySnapshotV3, error)
}

type MCPCallReadPortV1 interface {
	InspectMCPExecutionCommandV1(context.Context, contract.MCPExecutionCommandRefV1) (contract.MCPExecutionCommandFactV1, error)
	InspectMCPProtocolReceiptV1(context.Context, contract.MCPProtocolReceiptRefV1) (contract.MCPProtocolReceiptV1, error)
}

type MCPProcessReadPortV1 interface {
	contract.MCPProcessObservationReadPortV1
}

type MCPToolDiscoveryMaterialReadPortV1 interface {
	contract.MCPToolDiscoveryMaterialExactReaderV1
}

type MCPResourceDiscoveryMaterialReadPortV1 interface {
	contract.MCPResourceDiscoveryMaterialExactReaderV1
}

type MCPPromptDiscoveryMaterialReadPortV1 interface {
	contract.MCPPromptDiscoveryMaterialExactReaderV1
}

type MCPDiscoveryPageToolMaterialSetReadPortV1 interface {
	contract.MCPDiscoveryPageToolMaterialSetExactReaderV1
}

type MCPDiscoveryPageResourceMaterialSetReadPortV1 interface {
	contract.MCPDiscoveryPageResourceMaterialSetExactReaderV1
}

type MCPDiscoveryPagePromptMaterialSetReadPortV1 interface {
	contract.MCPDiscoveryPagePromptMaterialSetExactReaderV1
}

type PackageVerificationPortV1 interface {
	VerifyPackageV1(context.Context, contract.ToolPackageVerifyRequestV1) (contract.ToolPackageVerificationFactV1, error)
}

type ListOutputV1 struct {
	ContractVersion string                    `json:"contract_version"`
	Snapshot        sdk.RegistrySnapshotRefV1 `json:"snapshot"`
	Records         []api.RegistryRecordV1    `json:"records"`
}

type InspectOutputV1 struct {
	ContractVersion string          `json:"contract_version"`
	Kind            string          `json:"kind"`
	Record          registry.Record `json:"record"`
	Object          any             `json:"object"`
}

type MCPStatusOutputV1 struct {
	ContractVersion string               `json:"contract_version"`
	Record          mcp.ConnectionRecord `json:"record"`
}

type MCPConnectInspectOutputV1 struct {
	ContractVersion string `json:"contract_version"`
	Kind            string `json:"kind"`
	Object          any    `json:"object"`
}

type MCPConnectionAvailabilityOutputV1 struct {
	ContractVersion string                                                `json:"contract_version"`
	Availability    contract.MCPConnectionAvailabilityCurrentProjectionV1 `json:"availability"`
}

type MCPCapabilitySnapshotOutputV2 struct {
	ContractVersion string                           `json:"contract_version"`
	Snapshot        contract.MCPCapabilitySnapshotV2 `json:"snapshot"`
}

type MCPCapabilitySnapshotOutputV3 struct {
	ContractVersion string                           `json:"contract_version"`
	Snapshot        contract.MCPCapabilitySnapshotV3 `json:"snapshot"`
}

type MCPExecutionCommandSummaryV1 struct {
	Ref              contract.MCPExecutionCommandRefV1          `json:"ref"`
	Connection       contract.ObjectRef                         `json:"connection"`
	Snapshot         contract.ObjectRef                         `json:"snapshot"`
	Method           string                                     `json:"method"`
	JSONRPCRequestID string                                     `json:"jsonrpc_request_id"`
	ParamsSchema     runtimeports.SchemaRefV2                   `json:"params_schema"`
	ParamsDigest     core.Digest                                `json:"params_digest"`
	ParamsRevision   core.Revision                              `json:"params_revision"`
	ParamsBytes      uint64                                     `json:"params_bytes"`
	Attempt          runtimeports.OperationDispatchAttemptRefV3 `json:"attempt"`
	CreatedUnixNano  int64                                      `json:"created_unix_nano"`
	NotAfterUnixNano int64                                      `json:"not_after_unix_nano"`
}

type MCPProtocolReceiptSummaryV1 struct {
	Ref              contract.MCPProtocolReceiptRefV1                              `json:"ref"`
	Command          contract.MCPExecutionCommandRefV1                             `json:"command"`
	AdmissionReceipt runtimeports.ControlledOperationProviderAdmissionReceiptRefV2 `json:"admission_receipt"`
	JSONRPCRequestID string                                                        `json:"jsonrpc_request_id"`
	ToolError        bool                                                          `json:"tool_error"`
	ResponseDigest   core.Digest                                                   `json:"response_digest"`
	ResponseBytes    uint64                                                        `json:"response_bytes"`
	ObservedUnixNano int64                                                         `json:"observed_unix_nano"`
}

type MCPProcessPageOutputV1 struct {
	ContractVersion string                               `json:"contract_version"`
	Page            contract.MCPProcessObservationPageV1 `json:"page"`
}

type PackageVerificationOutputV1 struct {
	ContractVersion string                                 `json:"contract_version"`
	Fact            contract.ToolPackageVerificationFactV1 `json:"fact"`
}

type RunnerV1 struct {
	catalog                CatalogPortV1
	inspector              InspectorPortV1
	mcpStatus              MCPStatusPortV1
	mcpConnect             MCPConnectReadPortV1
	mcpDiscovery           MCPDiscoveryReadPortV2
	mcpDiscoveryV3         MCPDiscoveryReadPortV3
	mcpCall                MCPCallReadPortV1
	mcpProcess             MCPProcessReadPortV1
	mcpMaterial            MCPToolDiscoveryMaterialReadPortV1
	mcpMaterialSet         MCPDiscoveryPageToolMaterialSetReadPortV1
	mcpResourceMaterial    MCPResourceDiscoveryMaterialReadPortV1
	mcpPromptMaterial      MCPPromptDiscoveryMaterialReadPortV1
	mcpResourceMaterialSet MCPDiscoveryPageResourceMaterialSetReadPortV1
	mcpPromptMaterialSet   MCPDiscoveryPagePromptMaterialSetReadPortV1
	packageVerification    PackageVerificationPortV1
}

func NewRunnerWithPackageVerificationV1(catalog CatalogPortV1, inspector InspectorPortV1, verification PackageVerificationPortV1) (*RunnerV1, error) {
	if nilLikeCLIV1(catalog) || nilLikeCLIV1(inspector) || nilLikeCLIV1(verification) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool CLI Package Verification dependencies are required")
	}
	return &RunnerV1{catalog: catalog, inspector: inspector, packageVerification: verification}, nil
}

func NewRunnerWithMCPDiscoveryV3(catalog CatalogPortV1, inspector InspectorPortV1, discovery MCPDiscoveryReadPortV3) (*RunnerV1, error) {
	if nilLikeCLIV1(catalog) || nilLikeCLIV1(inspector) || nilLikeCLIV1(discovery) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool CLI MCP Discovery V3 dependencies are required")
	}
	return &RunnerV1{catalog: catalog, inspector: inspector, mcpDiscoveryV3: discovery}, nil
}

func NewRunnerWithMCPDiscoveryMaterialReadV1(catalog CatalogPortV1, inspector InspectorPortV1, status MCPStatusPortV1, connect MCPConnectReadPortV1, discovery MCPDiscoveryReadPortV2, toolMaterial MCPToolDiscoveryMaterialReadPortV1, toolMaterialSet MCPDiscoveryPageToolMaterialSetReadPortV1, resourceMaterial MCPResourceDiscoveryMaterialReadPortV1, resourceMaterialSet MCPDiscoveryPageResourceMaterialSetReadPortV1, promptMaterial MCPPromptDiscoveryMaterialReadPortV1, promptMaterialSet MCPDiscoveryPagePromptMaterialSetReadPortV1) (*RunnerV1, error) {
	if nilLikeCLIV1(catalog) || nilLikeCLIV1(inspector) || nilLikeCLIV1(status) || nilLikeCLIV1(connect) || nilLikeCLIV1(discovery) || nilLikeCLIV1(toolMaterial) || nilLikeCLIV1(toolMaterialSet) || nilLikeCLIV1(resourceMaterial) || nilLikeCLIV1(resourceMaterialSet) || nilLikeCLIV1(promptMaterial) || nilLikeCLIV1(promptMaterialSet) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool CLI MCP Discovery Material read dependencies are required")
	}
	return &RunnerV1{catalog: catalog, inspector: inspector, mcpStatus: status, mcpConnect: connect, mcpDiscovery: discovery, mcpMaterial: toolMaterial, mcpMaterialSet: toolMaterialSet, mcpResourceMaterial: resourceMaterial, mcpResourceMaterialSet: resourceMaterialSet, mcpPromptMaterial: promptMaterial, mcpPromptMaterialSet: promptMaterialSet}, nil
}

func NewRunnerWithMCPToolDiscoveryReadV1(catalog CatalogPortV1, inspector InspectorPortV1, status MCPStatusPortV1, connect MCPConnectReadPortV1, discovery MCPDiscoveryReadPortV2, material MCPToolDiscoveryMaterialReadPortV1, materialSet MCPDiscoveryPageToolMaterialSetReadPortV1) (*RunnerV1, error) {
	if nilLikeCLIV1(catalog) || nilLikeCLIV1(inspector) || nilLikeCLIV1(status) || nilLikeCLIV1(connect) || nilLikeCLIV1(discovery) || nilLikeCLIV1(material) || nilLikeCLIV1(materialSet) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool CLI MCP Tool Discovery read dependencies are required")
	}
	return &RunnerV1{catalog: catalog, inspector: inspector, mcpStatus: status, mcpConnect: connect, mcpDiscovery: discovery, mcpMaterial: material, mcpMaterialSet: materialSet}, nil
}

func NewRunnerWithMCPToolDiscoveryMaterialV1(catalog CatalogPortV1, inspector InspectorPortV1, status MCPStatusPortV1, connect MCPConnectReadPortV1, discovery MCPDiscoveryReadPortV2, material MCPToolDiscoveryMaterialReadPortV1) (*RunnerV1, error) {
	if nilLikeCLIV1(catalog) || nilLikeCLIV1(inspector) || nilLikeCLIV1(status) || nilLikeCLIV1(connect) || nilLikeCLIV1(discovery) || nilLikeCLIV1(material) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool CLI MCP Tool Discovery Material dependencies are required")
	}
	return &RunnerV1{catalog: catalog, inspector: inspector, mcpStatus: status, mcpConnect: connect, mcpDiscovery: discovery, mcpMaterial: material}, nil
}

func NewRunnerWithMCPProcessV1(catalog CatalogPortV1, inspector InspectorPortV1, process MCPProcessReadPortV1) (*RunnerV1, error) {
	if nilLikeCLIV1(catalog) || nilLikeCLIV1(inspector) || nilLikeCLIV1(process) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool CLI MCP Process dependencies are required")
	}
	return &RunnerV1{catalog: catalog, inspector: inspector, mcpProcess: process}, nil
}

func NewRunnerWithMCPCallV1(catalog CatalogPortV1, inspector InspectorPortV1, status MCPStatusPortV1, connect MCPConnectReadPortV1, discovery MCPDiscoveryReadPortV2, call MCPCallReadPortV1) (*RunnerV1, error) {
	if nilLikeCLIV1(catalog) || nilLikeCLIV1(inspector) || nilLikeCLIV1(status) || nilLikeCLIV1(connect) || nilLikeCLIV1(discovery) || nilLikeCLIV1(call) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool CLI MCP Call dependencies are required")
	}
	return &RunnerV1{catalog: catalog, inspector: inspector, mcpStatus: status, mcpConnect: connect, mcpDiscovery: discovery, mcpCall: call}, nil
}

func NewRunnerWithMCPDiscoveryV2(catalog CatalogPortV1, inspector InspectorPortV1, status MCPStatusPortV1, connect MCPConnectReadPortV1, discovery MCPDiscoveryReadPortV2) (*RunnerV1, error) {
	if nilLikeCLIV1(catalog) || nilLikeCLIV1(inspector) || nilLikeCLIV1(status) || nilLikeCLIV1(connect) || nilLikeCLIV1(discovery) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool CLI Discovery dependencies are required")
	}
	return &RunnerV1{catalog: catalog, inspector: inspector, mcpStatus: status, mcpConnect: connect, mcpDiscovery: discovery}, nil
}

func NewRunnerWithMCPConnectV1(catalog CatalogPortV1, inspector InspectorPortV1, status MCPStatusPortV1, connect MCPConnectReadPortV1) (*RunnerV1, error) {
	if nilLikeCLIV1(catalog) || nilLikeCLIV1(inspector) || nilLikeCLIV1(status) || nilLikeCLIV1(connect) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool CLI dependencies are required")
	}
	return &RunnerV1{catalog: catalog, inspector: inspector, mcpStatus: status, mcpConnect: connect}, nil
}

func NewRunnerWithMCPV1(catalog CatalogPortV1, inspector InspectorPortV1, status MCPStatusPortV1) (*RunnerV1, error) {
	if nilLikeCLIV1(catalog) || nilLikeCLIV1(inspector) || nilLikeCLIV1(status) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool CLI dependencies are required")
	}
	return &RunnerV1{catalog: catalog, inspector: inspector, mcpStatus: status}, nil
}

func NewRunnerV1(catalog CatalogPortV1, inspector InspectorPortV1) (*RunnerV1, error) {
	if nilLikeCLIV1(catalog) || nilLikeCLIV1(inspector) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool CLI dependencies are required")
	}
	return &RunnerV1{catalog: catalog, inspector: inspector}, nil
}

func (r *RunnerV1) RunV1(ctx context.Context, args []string, output io.Writer) error {
	if r == nil || nilLikeCLIV1(r.catalog) || nilLikeCLIV1(r.inspector) || nilLikeCLIV1(output) {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Tool CLI is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool CLI context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(args) < 2 {
		return unsupportedCommandV1()
	}
	if args[0] == "mcp" {
		switch args[1] {
		case "status":
			if nilLikeCLIV1(r.mcpStatus) {
				return unsupportedCommandV1()
			}
			return r.runMCPStatusV1(ctx, args[2:], output)
		case "inspect":
			if nilLikeCLIV1(r.mcpConnect) {
				return unsupportedCommandV1()
			}
			return r.runMCPConnectInspectV1(ctx, args[2:], output)
		case "availability":
			if nilLikeCLIV1(r.mcpConnect) {
				return unsupportedCommandV1()
			}
			return r.runMCPConnectionAvailabilityV1(ctx, args[2:], output)
		case "snapshot":
			if nilLikeCLIV1(r.mcpDiscovery) {
				return unsupportedCommandV1()
			}
			return r.runMCPCapabilitySnapshotV2(ctx, args[2:], output)
		case "snapshot-v3":
			if nilLikeCLIV1(r.mcpDiscoveryV3) {
				return unsupportedCommandV1()
			}
			return r.runMCPCapabilitySnapshotV3(ctx, args[2:], output)
		case "process":
			if nilLikeCLIV1(r.mcpProcess) {
				return unsupportedCommandV1()
			}
			return r.runMCPProcessPageV1(ctx, args[2:], output)
		default:
			return unsupportedCommandV1()
		}
	}
	if args[0] == "package" {
		if args[1] != "verify" || nilLikeCLIV1(r.packageVerification) {
			return unsupportedCommandV1()
		}
		return r.runPackageVerifyV1(ctx, args[2:], output)
	}
	if args[0] != "tool" {
		return unsupportedCommandV1()
	}
	switch args[1] {
	case "list":
		return r.runToolListV1(ctx, args[2:], output)
	case "inspect":
		return r.runToolInspectV1(ctx, args[2:], output)
	default:
		return unsupportedCommandV1()
	}
}

func (r *RunnerV1) runPackageVerifyV1(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("package verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	requestJSON := flags.String("request-json", "", "sealed Package Verify request JSON containing exact refs only")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*requestJSON) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "package verify arguments are invalid")
	}
	var request contract.ToolPackageVerifyRequestV1
	decoder := json.NewDecoder(strings.NewReader(*requestJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "package verify request JSON is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "package verify request JSON has trailing content")
	}
	fact, err := r.packageVerification.VerifyPackageV1(ctx, request)
	if err != nil {
		return err
	}
	if fact.Validate() != nil || fact.Package != request.Subject.ArtifactBinding.Package || fact.PackageRegistry != request.Subject.PackageRegistry || fact.TrustPolicy != request.Subject.TrustPolicy || fact.ArtifactBindingDigest != request.Subject.ArtifactBinding.BindingDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "package verify returned another authoritative Fact")
	}
	return writeJSONV1(output, PackageVerificationOutputV1{ContractVersion: ContractVersionV1, Fact: fact})
}

func (r *RunnerV1) runMCPCapabilitySnapshotV3(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("mcp snapshot-v3", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	id := flags.String("id", "", "exact MCP Capability Snapshot V3 ID")
	revision := flags.String("revision", "", "exact MCP Capability Snapshot V3 revision")
	digest := flags.String("digest", "", "exact MCP Capability Snapshot V3 digest")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "mcp snapshot-v3 arguments are invalid")
	}
	exact, err := exactMCPObjectRefV1(*id, *revision, *digest)
	if err != nil {
		return err
	}
	snapshot, err := r.mcpDiscoveryV3.InspectCurrentMCPCapabilitySnapshotV3(ctx, exact)
	if err != nil {
		return err
	}
	if snapshot.ObjectRef() != exact || snapshot.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "mcp snapshot-v3 returned another Capability Snapshot")
	}
	return writeJSONV1(output, MCPCapabilitySnapshotOutputV3{ContractVersion: ContractVersionV1, Snapshot: snapshot})
}

func (r *RunnerV1) runMCPProcessPageV1(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("mcp process", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	connectionID := flags.String("connection-id", "", "exact MCP Connection ID")
	connectionRevision := flags.String("connection-revision", "", "exact MCP Connection revision")
	connectionDigest := flags.String("connection-digest", "", "exact MCP Connection digest")
	connectionEpoch := flags.String("connection-epoch", "", "exact MCP Connection epoch")
	snapshotID := flags.String("snapshot-id", "", "exact MCP Capability Snapshot ID")
	snapshotRevision := flags.String("snapshot-revision", "", "exact MCP Capability Snapshot revision")
	snapshotDigest := flags.String("snapshot-digest", "", "exact MCP Capability Snapshot digest")
	after := flags.String("after", "0", "last consumed source sequence")
	limit := flags.String("limit", "100", "bounded page size")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "mcp process arguments are invalid")
	}
	connection, err := exactMCPObjectRefV1(*connectionID, *connectionRevision, *connectionDigest)
	if err != nil {
		return err
	}
	snapshot, err := exactMCPObjectRefV1(*snapshotID, *snapshotRevision, *snapshotDigest)
	if err != nil {
		return err
	}
	epoch, err := strconv.ParseUint(*connectionEpoch, 10, 64)
	if err != nil || epoch == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "mcp process Connection epoch is invalid")
	}
	afterSequence, err := strconv.ParseUint(*after, 10, 64)
	if err != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "mcp process after sequence is invalid")
	}
	pageLimit, err := strconv.ParseUint(*limit, 10, 32)
	if err != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "mcp process page size is invalid")
	}
	request := contract.MCPProcessObservationPageRequestV1{
		Connection: connection, ConnectionEpoch: core.Epoch(epoch), Snapshot: snapshot,
		AfterSourceSequence: afterSequence, Limit: uint32(pageLimit),
	}
	if err := request.Validate(); err != nil {
		return err
	}
	page, err := r.mcpProcess.ReadMCPProcessObservationPageV1(ctx, request)
	if err != nil {
		return err
	}
	if page.Validate() != nil || page.Request != request {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "mcp process returned another bounded page")
	}
	return writeJSONV1(output, MCPProcessPageOutputV1{ContractVersion: ContractVersionV1, Page: page})
}

func (r *RunnerV1) runMCPCapabilitySnapshotV2(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("mcp snapshot", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	id := flags.String("id", "", "exact MCP Capability Snapshot ID")
	revision := flags.String("revision", "", "exact MCP Capability Snapshot revision")
	digest := flags.String("digest", "", "exact MCP Capability Snapshot digest")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "mcp snapshot arguments are invalid")
	}
	exact, err := exactMCPObjectRefV1(*id, *revision, *digest)
	if err != nil {
		return err
	}
	snapshot, err := r.mcpDiscovery.InspectCurrentMCPCapabilitySnapshotV2(ctx, exact)
	if err != nil {
		return err
	}
	if snapshot.ObjectRef() != exact || snapshot.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "mcp snapshot returned another Capability Snapshot")
	}
	return writeJSONV1(output, MCPCapabilitySnapshotOutputV2{ContractVersion: ContractVersionV1, Snapshot: snapshot})
}

func (r *RunnerV1) runMCPConnectInspectV1(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("mcp inspect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "intent, receipt, connection, domain-result, apply, call-command, call-receipt, tool-material, tool-material-set, resource-material, resource-material-set, prompt-material, or prompt-material-set")
	id := flags.String("id", "", "exact MCP object ID")
	revision := flags.String("revision", "", "exact MCP object revision")
	digest := flags.String("digest", "", "exact MCP object digest")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "mcp inspect arguments are invalid")
	}
	exact, err := exactMCPObjectRefV1(*id, *revision, *digest)
	if err != nil {
		return err
	}
	result := MCPConnectInspectOutputV1{ContractVersion: ContractVersionV1, Kind: *kind}
	switch *kind {
	case "intent":
		result.Object, err = r.mcpConnect.InspectMCPConnectIntentV1(ctx, exact)
	case "receipt":
		result.Object, err = r.mcpConnect.InspectMCPConnectProtocolReceiptV1(ctx, contract.MCPConnectProtocolReceiptRefV1{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest})
	case "call-command":
		if nilLikeCLIV1(r.mcpCall) {
			return unsupportedCommandV1()
		}
		var command contract.MCPExecutionCommandFactV1
		command, err = r.mcpCall.InspectMCPExecutionCommandV1(ctx, contract.MCPExecutionCommandRefV1{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest})
		if err == nil {
			result.Object = summarizeMCPExecutionCommandV1(command)
		}
	case "call-receipt":
		if nilLikeCLIV1(r.mcpCall) {
			return unsupportedCommandV1()
		}
		var receipt contract.MCPProtocolReceiptV1
		receipt, err = r.mcpCall.InspectMCPProtocolReceiptV1(ctx, contract.MCPProtocolReceiptRefV1{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest})
		if err == nil {
			result.Object = summarizeMCPProtocolReceiptV1(receipt)
		}
	case "tool-material":
		if nilLikeCLIV1(r.mcpMaterial) {
			return unsupportedCommandV1()
		}
		result.Object, err = r.mcpMaterial.InspectExactMCPToolDiscoveryMaterialV1(ctx, contract.MCPToolDiscoveryMaterialRefV1{ContractVersion: contract.MCPToolDiscoveryMaterialContractVersionV1, ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest})
	case "tool-material-set":
		if nilLikeCLIV1(r.mcpMaterialSet) {
			return unsupportedCommandV1()
		}
		result.Object, err = r.mcpMaterialSet.InspectMCPDiscoveryPageToolMaterialSetV1(ctx, exact)
	case "resource-material":
		if nilLikeCLIV1(r.mcpResourceMaterial) {
			return unsupportedCommandV1()
		}
		result.Object, err = r.mcpResourceMaterial.InspectExactMCPResourceDiscoveryMaterialV1(ctx, contract.MCPResourceDiscoveryMaterialRefV1{ContractVersion: contract.MCPResourceDiscoveryMaterialContractVersionV1, ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest})
	case "resource-material-set":
		if nilLikeCLIV1(r.mcpResourceMaterialSet) {
			return unsupportedCommandV1()
		}
		result.Object, err = r.mcpResourceMaterialSet.InspectMCPDiscoveryPageResourceMaterialSetV1(ctx, exact)
	case "prompt-material":
		if nilLikeCLIV1(r.mcpPromptMaterial) {
			return unsupportedCommandV1()
		}
		result.Object, err = r.mcpPromptMaterial.InspectExactMCPPromptDiscoveryMaterialV1(ctx, contract.MCPPromptDiscoveryMaterialRefV1{ContractVersion: contract.MCPPromptDiscoveryMaterialContractVersionV1, ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest})
	case "prompt-material-set":
		if nilLikeCLIV1(r.mcpPromptMaterialSet) {
			return unsupportedCommandV1()
		}
		result.Object, err = r.mcpPromptMaterialSet.InspectMCPDiscoveryPagePromptMaterialSetV1(ctx, exact)
	case "connection":
		result.Object, err = r.mcpConnect.InspectMCPConnectionFactV2(ctx, contract.MCPConnectionFactRefV2{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest})
	case "domain-result":
		result.Object, err = r.mcpConnect.InspectMCPConnectDomainResultV1(ctx, exact)
	case "apply":
		result.Object, err = r.mcpConnect.InspectMCPConnectApplySettlementV1(ctx, exact)
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "mcp inspect kind is invalid")
	}
	if err != nil {
		return err
	}
	return writeJSONV1(output, result)
}

func summarizeMCPExecutionCommandV1(command contract.MCPExecutionCommandFactV1) MCPExecutionCommandSummaryV1 {
	return MCPExecutionCommandSummaryV1{
		Ref:        command.Ref,
		Connection: contract.ObjectRef{ID: command.Connection.ID, Revision: command.Connection.Revision, Digest: command.Connection.Digest},
		Snapshot:   contract.ObjectRef{ID: command.Snapshot.ID, Revision: command.Snapshot.Revision, Digest: command.Snapshot.Digest},
		Method:     command.Method, JSONRPCRequestID: command.JSONRPCRequestID,
		ParamsSchema: command.Params.Schema, ParamsDigest: command.Params.ContentDigest, ParamsRevision: command.ParamsRevision, ParamsBytes: command.Params.Length,
		Attempt: command.Attempt, CreatedUnixNano: command.CreatedUnixNano, NotAfterUnixNano: command.NotAfterUnixNano,
	}
}

func summarizeMCPProtocolReceiptV1(receipt contract.MCPProtocolReceiptV1) MCPProtocolReceiptSummaryV1 {
	return MCPProtocolReceiptSummaryV1{
		Ref: receipt.Ref, Command: receipt.Command, AdmissionReceipt: receipt.AdmissionReceipt,
		JSONRPCRequestID: receipt.JSONRPCRequestID, ToolError: receipt.ToolError,
		ResponseDigest: receipt.ResponseDigest, ResponseBytes: uint64(len(receipt.CanonicalResponse)), ObservedUnixNano: receipt.ObservedUnixNano,
	}
}

func (r *RunnerV1) runMCPConnectionAvailabilityV1(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("mcp availability", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	id := flags.String("id", "", "exact MCP Connection ID")
	revision := flags.String("revision", "", "exact MCP Connection revision")
	digest := flags.String("digest", "", "exact MCP Connection digest")
	ttlText := flags.String("ttl", "5s", "requested current lease")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "mcp availability arguments are invalid")
	}
	exact, err := exactMCPObjectRefV1(*id, *revision, *digest)
	if err != nil {
		return err
	}
	ttl, err := time.ParseDuration(*ttlText)
	if err != nil || ttl <= 0 || ttl > contract.MaxMCPConnectionAvailabilityTTLV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "mcp availability TTL is invalid")
	}
	availability, err := r.mcpConnect.InspectCurrentMCPConnectionAvailabilityV1(ctx, contract.MCPConnectionFactRefV2{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest}, ttl)
	if err != nil {
		return err
	}
	return writeJSONV1(output, MCPConnectionAvailabilityOutputV1{ContractVersion: ContractVersionV1, Availability: availability})
}

func exactMCPObjectRefV1(id, revision, digest string) (contract.ObjectRef, error) {
	if strings.TrimSpace(id) == "" {
		return contract.ObjectRef{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact MCP object ID is required")
	}
	revisionValue, err := strconv.ParseUint(revision, 10, 64)
	if err != nil || revisionValue == 0 {
		return contract.ObjectRef{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact MCP object revision is invalid")
	}
	exact := contract.ObjectRef{ID: id, Revision: core.Revision(revisionValue), Digest: core.Digest(digest)}
	if err := exact.Validate(); err != nil {
		return contract.ObjectRef{}, err
	}
	return exact, nil
}

func (r *RunnerV1) runMCPStatusV1(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("mcp status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	id := flags.String("id", "", "exact MCP Connection ID")
	revision := flags.String("revision", "", "exact MCP Connection revision")
	digest := flags.String("digest", "", "exact MCP Connection digest")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*id) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "mcp status arguments are invalid")
	}
	revisionValue, err := strconv.ParseUint(*revision, 10, 64)
	if err != nil || revisionValue == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "mcp status revision is invalid")
	}
	exact := contract.ObjectRef{ID: *id, Revision: core.Revision(revisionValue), Digest: core.Digest(*digest)}
	if err := exact.Validate(); err != nil {
		return err
	}
	record, err := r.mcpStatus.InspectMCPConnectionStatusV1(ctx, exact)
	if err != nil {
		return err
	}
	if record.Connection.ID != exact.ID || record.Connection.Revision != exact.Revision || record.Connection.Digest != exact.Digest || record.Revision == 0 {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "mcp status returned another Connection")
	}
	return writeJSONV1(output, MCPStatusOutputV1{ContractVersion: ContractVersionV1, Record: record})
}

func (r *RunnerV1) runToolListV1(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("tool list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "capability, tool, package, or tool-alias")
	pageSize := flags.Int("page-size", 100, "page size")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "tool list arguments are invalid")
	}
	request := api.ListRegistryRequestV1{PageSize: *pageSize, KindFilter: *kind}
	var result ListOutputV1
	for pageIndex := 0; pageIndex < maxListPagesV1; pageIndex++ {
		page, err := r.catalog.ListRegistryV1(ctx, request)
		if err != nil {
			return err
		}
		if err := validateCatalogPageV1(page); err != nil {
			return err
		}
		if result.ContractVersion == "" {
			result.ContractVersion, result.Snapshot = ContractVersionV1, page.Snapshot
		} else if result.Snapshot != page.Snapshot {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "tool list Registry Snapshot drifted")
		}
		result.Records = append(result.Records, page.Records...)
		if page.Next == nil {
			return writeJSONV1(output, result)
		}
		request.Cursor = page.Next
	}
	return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "tool list exceeded page limit")
}

func validateCatalogPageV1(page api.ListRegistryResultV1) error {
	if page.ContractVersion != api.CatalogContractVersionV1 || page.Snapshot.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "tool list Catalog page is invalid")
	}
	for _, record := range page.Records {
		if err := record.Validate(); err != nil {
			return err
		}
	}
	if page.Next != nil {
		if err := page.Next.Validate(); err != nil {
			return err
		}
		if page.Next.Snapshot != page.Snapshot {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "tool list cursor binds another Registry Snapshot")
		}
	}
	return nil
}

func (r *RunnerV1) runToolInspectV1(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("tool inspect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "capability, tool, package, alias, or mcp-mapping")
	id := flags.String("id", "", "exact object ID")
	revision := flags.String("revision", "", "exact object revision")
	digest := flags.String("digest", "", "exact object digest")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*id) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "tool inspect arguments are invalid")
	}
	revisionValue, err := strconv.ParseUint(*revision, 10, 64)
	if err != nil || revisionValue == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "tool inspect revision is invalid")
	}
	exact := contract.ObjectRef{ID: *id, Revision: core.Revision(revisionValue), Digest: core.Digest(*digest)}
	if err := exact.Validate(); err != nil {
		return err
	}
	result := InspectOutputV1{ContractVersion: ContractVersionV1, Kind: *kind}
	switch *kind {
	case "capability":
		result.Object, result.Record, err = r.inspector.InspectCapabilityV1(ctx, exact)
	case "tool":
		result.Object, result.Record, err = r.inspector.InspectToolV1(ctx, exact)
	case "package":
		result.Object, result.Record, err = r.inspector.InspectPackageV1(ctx, exact)
	case "alias":
		result.Object, result.Record, err = r.inspector.InspectToolAliasV1(ctx, contract.ToolAliasRefV1{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest})
	case "mcp-mapping":
		result.Object, result.Record, err = r.inspector.InspectMCPToolMappingV1(ctx, contract.MCPToolMappingManifestRefV1{ContractVersion: contract.MCPToolMappingContractVersionV1, ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest})
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "tool inspect kind is invalid")
	}
	if err != nil {
		return err
	}
	return writeJSONV1(output, result)
}

func writeJSONV1(output io.Writer, value any) error {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Tool CLI output cannot be encoded")
	}
	if _, err := output.Write(encoded.Bytes()); err != nil {
		return err
	}
	return nil
}

func unsupportedCommandV1() error {
	return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Tool CLI command is unsupported until its governed Port is available")
}

func nilLikeCLIV1(value any) bool {
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
