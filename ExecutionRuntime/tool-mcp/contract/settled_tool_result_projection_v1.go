package contract

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const SettledToolResultProjectionContractVersionV1 = "praxis.tool-mcp.settled-tool-result-projection/v1"

type SettledToolResultProjectionV1 struct {
	ContractVersion  string                                          `json:"contract_version"`
	Result           ObjectRef                                       `json:"result"`
	Tool             ObjectRef                                       `json:"tool"`
	Inspection       runtimeports.OperationInspectionSettlementRefV4 `json:"inspection"`
	Schema           runtimeports.SchemaRefV2                        `json:"schema"`
	PayloadDigest    core.Digest                                     `json:"payload_digest"`
	PayloadRevision  core.Revision                                   `json:"payload_revision"`
	Inline           []byte                                          `json:"inline,omitempty"`
	Artifact         *ObjectRef                                      `json:"artifact,omitempty"`
	Classification   core.Digest                                     `json:"classification_digest"`
	Complete         bool                                            `json:"complete"`
	CheckedUnixNano  int64                                           `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                           `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                     `json:"projection_digest"`
}

func SealSettledToolResultProjectionV1(p SettledToolResultProjectionV1) (SettledToolResultProjectionV1, error) {
	p.ContractVersion = SettledToolResultProjectionContractVersionV1
	p.Inline = append([]byte(nil), p.Inline...)
	if p.Artifact != nil {
		value := *p.Artifact
		p.Artifact = &value
	}
	p.ProjectionDigest = ""
	digest, err := p.computeDigest()
	if err != nil {
		return SettledToolResultProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, nil
}

func (p SettledToolResultProjectionV1) ValidateCurrent(result ToolResultV2, now time.Time) error {
	if p.ContractVersion != SettledToolResultProjectionContractVersionV1 || p.Result.Validate() != nil ||
		p.Tool.Validate() != nil || result.Validate() != nil ||
		p.Result != (ObjectRef{ID: result.ID, Revision: result.Revision, Digest: result.Digest}) ||
		p.Inspection != result.Inspection || p.Schema != result.Schema ||
		p.PayloadDigest != result.PayloadDigest || p.PayloadRevision != result.PayloadRevision ||
		p.Classification.Validate() != nil || p.CheckedUnixNano <= 0 ||
		p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() ||
		!now.Before(time.Unix(0, p.ExpiresUnixNano)) ||
		p.ExpiresUnixNano > p.Inspection.ExpiresUnixNano {
		return conflict("settled Tool result projection is incomplete, stale or drifts from exact result")
	}
	if p.Inspection.Validate(now) != nil || (p.Inline == nil) == (p.Artifact == nil) {
		return invalid("settled Tool result projection requires one current inline or artifact payload")
	}
	if p.Inline != nil {
		if len(p.Inline) == 0 || len(p.Inline) > runtimeports.MaxOpaqueInlineBytes ||
			core.DigestBytes(p.Inline) != p.PayloadDigest {
			return conflict("settled Tool result inline payload differs from exact digest")
		}
	} else {
		if p.Artifact.Validate() != nil || p.Artifact.Digest != p.PayloadDigest || !containsExactObjectRefV1(result.Artifacts, *p.Artifact) {
			return conflict("settled Tool result artifact is not an exact result artifact")
		}
	}
	if !p.Complete && p.Artifact == nil {
		return invalid("incomplete result must use an inspectable artifact")
	}
	expected, err := p.computeDigest()
	if err != nil || expected != p.ProjectionDigest {
		return conflict("settled Tool result projection digest drifted")
	}
	return nil
}

func (p SettledToolResultProjectionV1) Clone() SettledToolResultProjectionV1 {
	p.Inline = append([]byte(nil), p.Inline...)
	if p.Artifact != nil {
		value := *p.Artifact
		p.Artifact = &value
	}
	return p
}

func (p SettledToolResultProjectionV1) computeDigest() (core.Digest, error) {
	p.ProjectionDigest = ""
	return Seal("praxis.tool-mcp.settled-result", SettledToolResultProjectionContractVersionV1, "SettledToolResultProjectionV1", p)
}

func containsExactObjectRefV1(values []ObjectRef, target ObjectRef) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
