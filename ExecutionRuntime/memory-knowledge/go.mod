module github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge

go 1.25.0

require (
	github.com/Proview-China/rax/ExecutionRuntime/agent-assembler v0.0.0
	github.com/Proview-China/rax/ExecutionRuntime/application v0.0.0
	github.com/Proview-China/rax/ExecutionRuntime/harness v0.0.0
	github.com/Proview-China/rax/ExecutionRuntime/runtime v0.0.0
)

require (
	github.com/Proview-China/rax/ExecutionRuntime/agent-definition v0.0.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.44.0 // indirect
	modernc.org/libc v1.73.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.53.0 // indirect
)

replace github.com/Proview-China/rax/ExecutionRuntime/agent-assembler => ../agent-assembler

replace github.com/Proview-China/rax/ExecutionRuntime/agent-definition => ../agent-definition

replace github.com/Proview-China/rax/ExecutionRuntime/application => ../application

replace github.com/Proview-China/rax/ExecutionRuntime/harness => ../harness

replace github.com/Proview-China/rax/ExecutionRuntime/model-invoker => ../model-invoker

replace github.com/Proview-China/rax/ExecutionRuntime/tool-mcp => ../tool-mcp

replace github.com/Proview-China/rax/ExecutionRuntime/continuity => ../continuity

replace github.com/Proview-China/rax/ExecutionRuntime/runtime => ../runtime
