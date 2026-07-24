package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MCPListChangedContractVersionV1 = "praxis.tool-mcp.mcp-list-changed/v1"

type MCPListChangedNamespaceV1 string

const (
	MCPListChangedToolsV1     MCPListChangedNamespaceV1 = "tools"
	MCPListChangedResourcesV1 MCPListChangedNamespaceV1 = "resources"
	MCPListChangedPromptsV1   MCPListChangedNamespaceV1 = "prompts"
)

func (n MCPListChangedNamespaceV1) Validate() error {
	switch n {
	case MCPListChangedToolsV1, MCPListChangedResourcesV1, MCPListChangedPromptsV1:
		return nil
	default:
		return invalid("MCP list-changed namespace is invalid")
	}
}

type MCPListChangedObservationRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r MCPListChangedObservationRefV1) Validate() error {
	if ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return invalid("MCP list-changed Observation Ref is invalid")
	}
	return nil
}

// MCPListChangedObservationV1 is a Provider notification observation. It is
// not a Discovery Intent, Runtime Permit, Snapshot mutation, or Tool Surface.
type MCPListChangedObservationV1 struct {
	ContractVersion  string                         `json:"contract_version"`
	Ref              MCPListChangedObservationRefV1 `json:"ref"`
	Connection       MCPConnectionRef               `json:"connection"`
	Snapshot         ObjectRef                      `json:"snapshot"`
	Namespace        MCPListChangedNamespaceV1      `json:"namespace"`
	SourceSequence   uint64                         `json:"source_sequence"`
	ObservedUnixNano int64                          `json:"observed_unix_nano"`
}

func (o MCPListChangedObservationV1) Validate() error {
	if o.ContractVersion != MCPListChangedContractVersionV1 || o.Ref.Validate() != nil || o.Connection.Validate() != nil || o.Snapshot.Validate() != nil || o.Namespace.Validate() != nil || o.SourceSequence == 0 || o.ObservedUnixNano <= 0 {
		return invalid("MCP list-changed Observation is incomplete")
	}
	if o.ObservedUnixNano < o.Connection.CreatedUnixNano || o.ObservedUnixNano >= o.Connection.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP list-changed Observation is outside the Connection window")
	}
	id, err := DeriveMCPListChangedObservationIDV1(o.Connection, o.Snapshot, o.Namespace, o.SourceSequence)
	if err != nil || id != o.Ref.ID {
		return conflict("MCP list-changed Observation ID drifted")
	}
	digest, err := o.ComputeDigest()
	if err != nil || digest != o.Ref.Digest {
		return conflict("MCP list-changed Observation digest drifted")
	}
	return nil
}

func (o MCPListChangedObservationV1) ValidateCurrent(now time.Time) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < o.ObservedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP list-changed Observation clock regressed")
	}
	if !now.Before(time.Unix(0, o.Connection.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP list-changed Connection expired")
	}
	return nil
}

func (o MCPListChangedObservationV1) ComputeDigest() (core.Digest, error) {
	o.Ref.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp-list-changed", MCPListChangedContractVersionV1, "MCPListChangedObservationV1", o)
}

func SealMCPListChangedObservationV1(o MCPListChangedObservationV1) (MCPListChangedObservationV1, error) {
	o.ContractVersion = MCPListChangedContractVersionV1
	id, err := DeriveMCPListChangedObservationIDV1(o.Connection, o.Snapshot, o.Namespace, o.SourceSequence)
	if err != nil {
		return MCPListChangedObservationV1{}, err
	}
	if o.Ref.ID != "" && o.Ref.ID != id {
		return MCPListChangedObservationV1{}, conflict("supplied MCP list-changed Observation ID drifted")
	}
	o.Ref.ID, o.Ref.Revision = id, 1
	provided := o.Ref.Digest
	o.Ref.Digest = ""
	digest, err := o.ComputeDigest()
	if err != nil {
		return MCPListChangedObservationV1{}, err
	}
	if provided != "" && provided != digest {
		return MCPListChangedObservationV1{}, conflict("supplied MCP list-changed Observation digest drifted")
	}
	o.Ref.Digest = digest
	return o, o.Validate()
}

func DeriveMCPListChangedObservationIDV1(connection MCPConnectionRef, snapshot ObjectRef, namespace MCPListChangedNamespaceV1, sequence uint64) (string, error) {
	if connection.Validate() != nil || snapshot.Validate() != nil || namespace.Validate() != nil || sequence == 0 {
		return "", invalid("MCP list-changed Observation identity inputs are invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp-list-changed", MCPListChangedContractVersionV1, "MCPListChangedObservationIdentityV1", struct {
		Connection ObjectRef                 `json:"connection"`
		Epoch      core.Epoch                `json:"connection_epoch"`
		Snapshot   ObjectRef                 `json:"snapshot"`
		Namespace  MCPListChangedNamespaceV1 `json:"namespace"`
		Sequence   uint64                    `json:"source_sequence"`
	}{
		Connection: ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest},
		Epoch:      connection.Epoch,
		Snapshot:   snapshot,
		Namespace:  namespace,
		Sequence:   sequence,
	})
	if err != nil {
		return "", err
	}
	return "mcp-list-changed-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}
