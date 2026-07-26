# Context Runtime API V1

## Status

- Decision: accepted design candidate.
- Scope: Context Owner-local typed facade only.
- Implementation gate: this Design PR must merge before Go implementation starts.
- Production status: no Host composition root, Capability, listener, remote service or SLA is granted.

## Purpose

`runtimeapi` gives an embedded Agent Host one stable Context-owned entry point for consuming an immutable current Frame and appending one already-settled Tool result.

```text
current Context frame closure
  -> ConsumeFrame
  -> ContextFrameConsumptionDescriptorV1

current Context frame closure
+ Tool-owned current SettledToolResultProjectionV1
  -> AppendSettledToolResult
  -> child Manifest / Frame / Generation
  -> ContextFrameConsumptionDescriptorV1
```

The facade exposes existing verified behavior. It creates no new state model, authority, settlement decision, wire protocol or cross-owner contract.

## Package and public surface

The implementation package is `ExecutionRuntime/context-engine/runtimeapi`.

```go
type AppendSettledToolResultRequestV2 = kernel.AppendSettledToolResultRequestV2
type AppendSettledToolResultResultV2 = kernel.AppendSettledToolResultResultV2

type ContextRuntimeAPIV1 interface {
    ConsumeFrame(context.Context, contract.ContextFrameConsumptionRequestV1) (contract.ContextFrameConsumptionDescriptorV1, error)
    AppendSettledToolResult(context.Context, AppendSettledToolResultRequestV2) (AppendSettledToolResultResultV2, error)
}
```

The aliases reuse the existing Kernel request and result exactly. They do not define a second nominal DTO or add fields, validation rules or lifecycle semantics.

V1 is typed Go only. It adds no JSON operation, CLI command, HTTP/RPC listener, generated client or provider-specific projection.

## Construction and dependencies

`ServiceV1` is constructed with:

- `kernel.FrameConsumptionCurrentReaderV1`;
- `kernel.SettledToolResultCurrentReaderV2`;
- `kernel.ContextAwareReferenceStoreV1`.

Construction rejects nil and typed-nil dependencies. The service owns no Store and creates no goroutine. It only retains injected interfaces and delegates to:

- `kernel.BuildFrameConsumptionDescriptorV1` for `ConsumeFrame`;
- `kernel.AppendSettledToolResultV2` for `AppendSettledToolResult`.

The facade must not weaken, duplicate or bypass Kernel validation. Currentness, TTL, exact refs, cancellation, deterministic identity, invalidation and content-addressed residual behavior remain in existing contracts and Kernel.

## Owner boundary

| Object or decision | Owner | Runtime API behavior |
|---|---|---|
| Fragment, Manifest, Frame, Generation and descriptor | Context | returns existing immutable values |
| settled Tool result and current projection | Tool/MCP | consumes existing public projection through existing exact Reader |
| current validation | Context Kernel and injected Readers | delegates without reinterpretation |
| provider-specific projection | Model Invoker | not exposed or implemented |
| Prefix/KV cache | Provider Adapter or local inference backend | not exposed or claimed |
| Runtime settlement, Review verdict and Effect authority | existing owners | never created or copied |
| Host lifecycle and continuation | future Host composition | not implemented |

`runtimeapi` must not import Application, Harness, Model Invoker, Memory/Knowledge, Sandbox, Review or Continuity. Tool/MCP public types remain reachable only through the already-merged Kernel request/result and Reader.

## Method behavior

### ConsumeFrame

- accepts existing sealed `ContextFrameConsumptionRequestV1`;
- uses existing S1 validation, descriptor construction and S2 reread;
- returns exact immutable `ContextFrameConsumptionDescriptorV1`;
- returns zero descriptor on invalid, expired, drifted, unknown or canceled input;
- performs no cache write, Model projection or provider call.

### AppendSettledToolResult

- accepts the V2 request alias;
- uses the exact Tool result Reader and official current validation already enforced by Kernel;
- appends only dynamic `tool_result` or `artifact_reference` fragments;
- preserves stable and semi-stable content refs unless existing invariants fail;
- returns existing source, child Manifest, Frame, Generation and neutral descriptor;
- returns zero result on unknown, expired, drifted, invalid XOR or cancellation;
- never retries Tool execution or infers UnknownOutcome.

Large bodies remain outside Context and enter only through exact ArtifactRef metadata. A canceled content-addressed Put may leave an unreferenced reusable blob, but no Manifest, Frame, Generation or descriptor is published.

## Failure model

The facade preserves existing sentinel identity and context errors; it defines no new error taxonomy.

- invalid or typed-nil construction fails before a usable service exists;
- invalid, conflict, expired, unknown, unavailable and unsupported retain `errors.Is` behavior;
- `context.Canceled` and `context.DeadlineExceeded` remain detectable;
- UnknownOutcome recovery stays exact Inspect through the owner Reader; no write retry helper is added.

## Explicit non-goals

- changing `contract`, `kernel`, `offlineapi`, `engineeringapi` or existing SDK operations;
- defining new request, result, Frame, ToolResult or projection nominals;
- adding JSON, CLI, HTTP, RPC, daemon or listener support;
- adding Host/Application/Harness/Model adapters or cross-owner Ports;
- adding Memory/Knowledge, Sandbox, Review or Continuity dependencies;
- adding persistence, provider cache, Provider calls or production composition;
- claiming a production-ready Component Release.

## Implementation scope after merge

- `ExecutionRuntime/context-engine/runtimeapi/service_v1.go`;
- `ExecutionRuntime/context-engine/runtimeapi/service_v1_test.go`;
- `ExecutionRuntime/context-engine/tests/blackbox/runtime_api_v1_test.go`;
- minimal Context module status update if required.

No other module or Owner may be modified.

## Acceptance tests

1. External black-box code calls both methods through `ContextRuntimeAPIV1` without invoking Kernel functions.
2. `ConsumeFrame` returns the exact descriptor for a current Frame closure.
3. Inline Tool result returns child Manifest, Frame, Generation and descriptor.
4. Artifact-backed result exposes only ArtifactRef metadata.
5. Stable and semi-stable refs/fingerprints remain unchanged while dynamic and Frame fingerprints change.
6. Unknown, expiry, drift, invalid XOR and cancellation return zero product and preserve error identity.
7. Identical replay is deterministic; 64 concurrent calls observe one logical result.
8. Constructors reject nil and typed-nil dependencies.
9. Import checks prove no dependency on Application, Harness, Model Invoker, Memory/Knowledge, Sandbox, Review or Continuity.
10. Implementation gates:

```bash
go test -count=100 ./runtimeapi ./tests/blackbox
go test -race -count=20 ./runtimeapi ./tests/blackbox
go test ./...
go test -race ./...
go vet ./...
gofmt -l <changed-go-files>
git diff --check origin/main...HEAD
```

