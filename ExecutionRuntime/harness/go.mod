module github.com/Proview-China/rax/ExecutionRuntime/harness

go 1.25.0

require (
	github.com/Proview-China/rax/ExecutionRuntime/application v0.0.0
	github.com/Proview-China/rax/ExecutionRuntime/runtime v0.0.0
)

replace github.com/Proview-China/rax/ExecutionRuntime/application => ../application
replace github.com/Proview-China/rax/ExecutionRuntime/runtime => ../runtime
