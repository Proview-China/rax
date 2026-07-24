package contract

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	MCPExecutionCommandContractVersionV1 = "praxis.tool-mcp.mcp-execution-command/v1"
	MCPExecutionCommandKindV1            = runtimeports.NamespacedNameV2("praxis.mcp/execution-command")
	MCPToolsCallMethodV1                 = "tools/call"
)

type MCPExecutionCommandRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r MCPExecutionCommandRefV1) Validate() error {
	if ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return invalid("MCP execution command Ref is invalid")
	}
	return nil
}

type MCPExecutionCommandFactV1 struct {
	ContractVersion      string                                     `json:"contract_version"`
	Ref                  MCPExecutionCommandRefV1                   `json:"ref"`
	Owner                runtimeports.EffectOwnerRefV2              `json:"owner"`
	BindingCurrent       SingleCallToolActionBindingCurrentRefV2    `json:"binding_current"`
	Candidate            ActionCandidateV3                          `json:"candidate"`
	InputContractCurrent ToolInputContractCurrentRefV1              `json:"input_contract_current"`
	Capability           CapabilityDescriptor                       `json:"capability"`
	Tool                 ToolDescriptor                             `json:"tool"`
	Server               MCPServerDescriptor                        `json:"server"`
	Connection           MCPConnectionRef                           `json:"connection"`
	Snapshot             MCPCapabilitySnapshotV2                    `json:"snapshot"`
	SnapshotTool         MCPToolObservationV2                       `json:"snapshot_tool"`
	Method               string                                     `json:"method"`
	JSONRPCRequestID     string                                     `json:"jsonrpc_request_id"`
	Params               runtimeports.OpaquePayloadV2               `json:"params"`
	ParamsRevision       core.Revision                              `json:"params_revision"`
	Operation            runtimeports.OperationSubjectV3            `json:"operation"`
	OperationDigest      core.Digest                                `json:"operation_digest"`
	EffectID             core.EffectIntentID                        `json:"effect_id"`
	EffectRevision       core.Revision                              `json:"effect_revision"`
	IntentDigest         core.Digest                                `json:"intent_digest"`
	Prepared             runtimeports.PreparedProviderAttemptRefV2  `json:"prepared"`
	Attempt              runtimeports.OperationDispatchAttemptRefV3 `json:"attempt"`
	Provider             runtimeports.ProviderBindingRefV2          `json:"provider"`
	CreatedUnixNano      int64                                      `json:"created_unix_nano"`
	NotAfterUnixNano     int64                                      `json:"not_after_unix_nano"`
}

func (f MCPExecutionCommandFactV1) Validate() error {
	if f.ContractVersion != MCPExecutionCommandContractVersionV1 || f.Ref.Validate() != nil || validateEffectOwner(f.Owner) != nil || f.BindingCurrent.Validate() != nil || f.Candidate.Validate() != nil || f.InputContractCurrent.Validate() != nil || f.Capability.Validate() != nil || f.Tool.Validate() != nil || f.Server.Validate() != nil || f.Connection.Validate() != nil || f.Snapshot.Validate() != nil || f.SnapshotTool.Validate() != nil || f.Method != MCPToolsCallMethodV1 || ValidateStableID(f.JSONRPCRequestID) != nil || f.Params.Validate() != nil || f.ParamsRevision != 1 || f.Operation.Validate() != nil || f.OperationDigest.Validate() != nil || f.EffectID == "" || f.EffectRevision == 0 || f.IntentDigest.Validate() != nil || f.Prepared.Validate() != nil || f.Attempt.Validate() != nil || f.Provider.Validate() != nil || f.CreatedUnixNano <= 0 || f.NotAfterUnixNano <= f.CreatedUnixNano {
		return invalid("MCP execution command is incomplete")
	}
	operationDigest, err := f.Operation.DigestV3()
	if err != nil || operationDigest != f.OperationDigest || f.Candidate.InputContractCurrentRef != f.InputContractCurrent || f.Candidate.Capability.ID != string(f.Capability.ID) || f.Candidate.Capability.Revision != f.Capability.Revision || f.Candidate.Capability.Digest != f.Capability.Digest || f.Candidate.Tool.ID != string(f.Tool.ID) || f.Candidate.Tool.Revision != f.Tool.Revision || f.Candidate.Tool.Digest != f.Tool.Digest || f.Tool.Mechanism != MechanismMCP || f.Tool.ValidateAgainst(f.Capability) != nil || f.Owner != f.Candidate.ExpectedOwner || f.Provider.ComponentID != f.Owner.ComponentID || f.Provider.ManifestDigest != f.Owner.ManifestDigest || f.Provider.Capability != runtimeports.CapabilityNameV2(f.Candidate.EffectKind) {
		return conflict("MCP execution command Tool, Capability, Owner or Operation binding drifted")
	}
	connectionRef := ObjectRef{ID: f.Connection.ID, Revision: f.Connection.Revision, Digest: f.Connection.Digest}
	serverRef := ObjectRef{ID: f.Server.ID, Revision: f.Server.Revision, Digest: f.Server.Digest}
	if f.Connection.Server != serverRef || f.Snapshot.Server != f.Connection.Server || f.Snapshot.Connection != connectionRef || f.Snapshot.ConnectionEpoch != f.Connection.Epoch || f.Snapshot.ProtocolVersion != f.Connection.NegotiatedProtocol || f.Connection.TenantID != string(f.Candidate.TenantID) || f.Connection.RunID != f.Candidate.RunID {
		return conflict("MCP execution command Server, Connection or Snapshot binding drifted")
	}
	if !containsExactSnapshotToolV1(f.Snapshot.Tools, f.SnapshotTool) || f.SnapshotTool.Name != f.Candidate.SourceCandidate.CallName || f.SnapshotTool.InputSchemaDigest != f.Candidate.InputSchema.ContentDigest {
		return conflict("MCP execution command Snapshot Tool mapping drifted")
	}
	if f.Params.Schema != f.Candidate.Payload.Schema || f.Params.ContentDigest != f.Candidate.Payload.ContentDigest || f.Params.LimitPolicy != f.Candidate.Payload.LimitPolicy || !bytes.Equal(f.Params.Inline, f.Candidate.Payload.Inline) || f.Params.Ref != "" || f.Prepared.PayloadSchema != f.Params.Schema || f.Prepared.PayloadDigest != f.Params.ContentDigest || f.Prepared.PayloadRevision != f.ParamsRevision {
		return conflict("MCP execution command canonical Params drifted")
	}
	if f.Prepared.OperationDigest != f.OperationDigest || f.Attempt.OperationDigest != f.OperationDigest || f.Prepared.IntentID != f.EffectID || f.Attempt.EffectID != f.EffectID || f.Prepared.IntentRevision != f.EffectRevision || f.Attempt.IntentRevision != f.EffectRevision || f.Prepared.IntentDigest != f.IntentDigest || f.Attempt.IntentDigest != f.IntentDigest || f.Prepared.AttemptID != f.Attempt.AttemptID || f.Prepared.Provider != f.Provider {
		return conflict("MCP execution command Runtime Attempt binding drifted")
	}
	if f.NotAfterUnixNano > f.BindingUpperBoundUnixNanoV1() {
		return conflict("MCP execution command exceeds an exact current upper bound")
	}
	id, requestID, err := DeriveMCPExecutionCommandIdentityV1(f)
	if err != nil || id != f.Ref.ID || requestID != f.JSONRPCRequestID {
		return conflict("MCP execution command stable identity drifted")
	}
	digest, err := f.ComputeDigest()
	if err != nil || digest != f.Ref.Digest {
		return conflict("MCP execution command digest drifted")
	}
	return nil
}

func (f MCPExecutionCommandFactV1) ValidateCurrent(now time.Time) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < f.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP execution command clock regressed")
	}
	if !now.Before(time.Unix(0, f.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP execution command expired")
	}
	return nil
}

func (f MCPExecutionCommandFactV1) BindingUpperBoundUnixNanoV1() int64 {
	upper := f.Candidate.RequestedExpiresUnixNano
	for _, value := range []int64{f.Connection.ExpiresUnixNano, f.Snapshot.ExpiresUnixNano, f.Prepared.ExpiresUnixNano} {
		if upper == 0 || value < upper {
			upper = value
		}
	}
	return upper
}

func (f MCPExecutionCommandFactV1) ComputeDigest() (core.Digest, error) {
	f = CloneMCPExecutionCommandFactV1(f)
	f.Ref.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp-execution-command", MCPExecutionCommandContractVersionV1, "MCPExecutionCommandFactV1", f)
}

func SealMCPExecutionCommandFactV1(f MCPExecutionCommandFactV1) (MCPExecutionCommandFactV1, error) {
	f = CloneMCPExecutionCommandFactV1(f)
	f.ContractVersion = MCPExecutionCommandContractVersionV1
	id, requestID, err := DeriveMCPExecutionCommandIdentityV1(f)
	if err != nil {
		return MCPExecutionCommandFactV1{}, err
	}
	if f.Ref.ID != "" && f.Ref.ID != id || f.JSONRPCRequestID != "" && f.JSONRPCRequestID != requestID {
		return MCPExecutionCommandFactV1{}, conflict("supplied MCP execution command identity drifted")
	}
	f.Ref.ID, f.Ref.Revision, f.JSONRPCRequestID = id, 1, requestID
	provided := f.Ref.Digest
	f.Ref.Digest = ""
	digest, err := f.ComputeDigest()
	if err != nil {
		return MCPExecutionCommandFactV1{}, err
	}
	if provided != "" && provided != digest {
		return MCPExecutionCommandFactV1{}, conflict("supplied MCP execution command digest drifted")
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

func DeriveMCPExecutionCommandIdentityV1(f MCPExecutionCommandFactV1) (string, string, error) {
	connection := ObjectRef{ID: f.Connection.ID, Revision: f.Connection.Revision, Digest: f.Connection.Digest}
	snapshot := ObjectRef{ID: f.Snapshot.ID, Revision: f.Snapshot.Revision, Digest: f.Snapshot.Digest}
	if f.BindingCurrent.Validate() != nil || f.Candidate.Validate() != nil || f.Prepared.Validate() != nil || f.Attempt.Validate() != nil || f.Connection.Validate() != nil || f.Snapshot.Validate() != nil || f.SnapshotTool.ObjectDigest.Validate() != nil {
		return "", "", invalid("MCP execution command identity inputs are invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp-execution-command", MCPExecutionCommandContractVersionV1, "MCPExecutionCommandIdentityV1", struct {
		Binding         SingleCallToolActionBindingCurrentRefV2    `json:"binding"`
		Candidate       ObjectRef                                  `json:"candidate"`
		Prepared        runtimeports.PreparedProviderAttemptRefV2  `json:"prepared"`
		Attempt         runtimeports.OperationDispatchAttemptRefV3 `json:"attempt"`
		Connection      ObjectRef                                  `json:"connection"`
		ConnectionEpoch core.Epoch                                 `json:"connection_epoch"`
		Snapshot        ObjectRef                                  `json:"snapshot"`
		SnapshotTool    core.Digest                                `json:"snapshot_tool"`
	}{f.BindingCurrent, f.Candidate.ObjectRef(), f.Prepared, f.Attempt, connection, f.Connection.Epoch, snapshot, f.SnapshotTool.ObjectDigest})
	if err != nil {
		return "", "", err
	}
	suffix := strings.TrimPrefix(string(digest), "sha256:")
	return "mcp-execution-command-" + suffix, "mcp-request-" + suffix, nil
}

func (f MCPExecutionCommandFactV1) RuntimeDomainCommandRefV1() runtimeports.OperationDomainCommandRefV1 {
	return runtimeports.OperationDomainCommandRefV1{Owner: f.Owner, Kind: MCPExecutionCommandKindV1, ID: f.Ref.ID, Revision: f.Ref.Revision, Digest: f.Ref.Digest}
}

type MCPExecutionCommandCurrentProjectionV1 struct {
	ContractVersion  string                    `json:"contract_version"`
	Fact             MCPExecutionCommandFactV1 `json:"fact"`
	CheckedUnixNano  int64                     `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                     `json:"expires_unix_nano"`
	ProjectionDigest core.Digest               `json:"projection_digest"`
}

func (p MCPExecutionCommandCurrentProjectionV1) ValidateCurrent(expected MCPExecutionCommandRefV1, now time.Time) error {
	if p.ContractVersion != MCPExecutionCommandContractVersionV1 || p.Fact.Validate() != nil || p.Fact.Ref != expected || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ExpiresUnixNano > p.Fact.NotAfterUnixNano || p.ProjectionDigest.Validate() != nil {
		return invalid("MCP execution command current Projection is invalid")
	}
	digest, err := p.ComputeDigest()
	if err != nil || digest != p.ProjectionDigest {
		return conflict("MCP execution command current Projection digest drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP execution command current Projection clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP execution command current Projection expired")
	}
	return nil
}

func (p MCPExecutionCommandCurrentProjectionV1) ComputeDigest() (core.Digest, error) {
	p.Fact = CloneMCPExecutionCommandFactV1(p.Fact)
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp-execution-command", MCPExecutionCommandContractVersionV1, "MCPExecutionCommandCurrentProjectionV1", p)
}

func SealMCPExecutionCommandCurrentProjectionV1(p MCPExecutionCommandCurrentProjectionV1) (MCPExecutionCommandCurrentProjectionV1, error) {
	p.Fact = CloneMCPExecutionCommandFactV1(p.Fact)
	p.ContractVersion = MCPExecutionCommandContractVersionV1
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	digest, err := p.ComputeDigest()
	if err != nil {
		return MCPExecutionCommandCurrentProjectionV1{}, err
	}
	if provided != "" && provided != digest {
		return MCPExecutionCommandCurrentProjectionV1{}, conflict("supplied MCP execution command current Projection digest drifted")
	}
	p.ProjectionDigest = digest
	return p, p.ValidateCurrent(p.Fact.Ref, time.Unix(0, p.CheckedUnixNano))
}

type MCPExecutionCommandStoreV1 interface {
	CreateMCPExecutionCommandV1(context.Context, MCPExecutionCommandFactV1) (MCPExecutionCommandFactV1, error)
	InspectMCPExecutionCommandV1(context.Context, MCPExecutionCommandRefV1) (MCPExecutionCommandFactV1, error)
}

type MCPExecutionCommandExactReaderV1 interface {
	InspectMCPExecutionCommandV1(context.Context, MCPExecutionCommandRefV1) (MCPExecutionCommandFactV1, error)
}

// MCPExecutionCommandAttemptReaderV1 is the N=1 reverse index used only after
// Runtime has produced an exact Attempt. It must return the immutable command
// bound to that Attempt or fail closed; it is not a latest/name resolver.
type MCPExecutionCommandAttemptReaderV1 interface {
	InspectMCPExecutionCommandByAttemptV1(context.Context, runtimeports.OperationDispatchAttemptRefV3) (MCPExecutionCommandFactV1, error)
}

type MCPExecutionCommandCurrentReaderV1 interface {
	InspectCurrentMCPExecutionCommandV1(context.Context, MCPExecutionCommandRefV1) (MCPExecutionCommandCurrentProjectionV1, error)
}

func CloneMCPExecutionCommandFactV1(f MCPExecutionCommandFactV1) MCPExecutionCommandFactV1 {
	f.Candidate = CloneActionCandidateV3(f.Candidate)
	f.Snapshot = CloneMCPCapabilitySnapshotV2(f.Snapshot)
	f.Params.Inline = append([]byte(nil), f.Params.Inline...)
	f.Capability.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), f.Capability.EffectKinds...)
	f.Tool.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), f.Tool.EffectKinds...)
	f.Tool.Residuals = append([]Residual(nil), f.Tool.Residuals...)
	return f
}

func containsExactSnapshotToolV1(values []MCPToolObservationV2, expected MCPToolObservationV2) bool {
	for _, value := range values {
		if value.Name == expected.Name {
			return value == expected
		}
	}
	return false
}
