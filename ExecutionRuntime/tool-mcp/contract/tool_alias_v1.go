package contract

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ToolAliasContractVersionV1 = "praxis.tool-mcp.tool-alias/v1"

type ToolAliasRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ToolAliasRefV1) Validate() error {
	if ValidateStableID(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return invalid("Tool Alias Ref is invalid")
	}
	return nil
}

func (r ToolAliasRefV1) ObjectRef() ObjectRef {
	return ObjectRef{ID: r.ID, Revision: r.Revision, Digest: r.Digest}
}

// ToolAliasV1 is an assembly-time Registry fact. It never grants authority or
// permits a Run to follow a moving alias.
type ToolAliasV1 struct {
	ContractVersion string                        `json:"contract_version"`
	Ref             ToolAliasRefV1                `json:"ref"`
	Alias           runtimeports.NamespacedNameV2 `json:"alias"`
	Owner           core.OwnerRef                 `json:"owner"`
	Tool            ObjectRef                     `json:"tool"`
	CreatedUnixNano int64                         `json:"created_unix_nano"`
}

func (a ToolAliasV1) Validate() error {
	if a.ContractVersion != ToolAliasContractVersionV1 || a.Ref.Validate() != nil || runtimeports.ValidateNamespacedNameV2(a.Alias) != nil || a.Owner.Validate() != nil || a.Tool.Validate() != nil || a.CreatedUnixNano <= 0 {
		return invalid("Tool Alias is incomplete")
	}
	id, err := DeriveToolAliasIDV1(a.Owner, a.Alias)
	if err != nil || id != a.Ref.ID {
		return conflict("Tool Alias stable ID drifted")
	}
	digest, err := a.ComputeDigestV1()
	if err != nil || digest != a.Ref.Digest {
		return conflict("Tool Alias digest drifted")
	}
	return nil
}

func (a ToolAliasV1) ValidateAt(now time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < a.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Alias clock regressed")
	}
	return nil
}

func (a ToolAliasV1) ComputeDigestV1() (core.Digest, error) {
	a.Ref.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.tool-alias", ToolAliasContractVersionV1, "ToolAliasV1", a)
}

func SealToolAliasV1(a ToolAliasV1) (ToolAliasV1, error) {
	a.ContractVersion = ToolAliasContractVersionV1
	if a.Ref.Revision == 0 {
		return ToolAliasV1{}, invalid("Tool Alias revision is required")
	}
	id, err := DeriveToolAliasIDV1(a.Owner, a.Alias)
	if err != nil {
		return ToolAliasV1{}, err
	}
	if a.Ref.ID != "" && a.Ref.ID != id {
		return ToolAliasV1{}, conflict("supplied Tool Alias ID drifted")
	}
	a.Ref.ID = id
	provided := a.Ref.Digest
	a.Ref.Digest = ""
	digest, err := a.ComputeDigestV1()
	if err != nil {
		return ToolAliasV1{}, err
	}
	if provided != "" && provided != digest {
		return ToolAliasV1{}, conflict("supplied Tool Alias digest drifted")
	}
	a.Ref.Digest = digest
	return a, a.Validate()
}

func DeriveToolAliasIDV1(owner core.OwnerRef, alias runtimeports.NamespacedNameV2) (string, error) {
	if owner.Validate() != nil || runtimeports.ValidateNamespacedNameV2(alias) != nil {
		return "", invalid("Tool Alias identity inputs are invalid")
	}
	return StableID("tool-alias", owner.Domain, string(owner.ID), string(alias))
}
