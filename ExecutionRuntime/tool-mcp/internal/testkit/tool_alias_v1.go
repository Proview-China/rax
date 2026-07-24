package testkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func ToolAliasV1(revision uint64, tool contract.ObjectRef, created time.Time) contract.ToolAliasV1 {
	value, err := contract.SealToolAliasV1(contract.ToolAliasV1{
		Ref:   contract.ToolAliasRefV1{Revision: core.Revision(revision)},
		Alias: "tool/default", Owner: Owner(), Tool: tool, CreatedUnixNano: created.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return value
}

func ToolAliasNameV1() runtimeports.NamespacedNameV2 { return "tool/default" }
