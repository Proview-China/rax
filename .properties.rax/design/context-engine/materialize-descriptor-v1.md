# Context Runtime API Materialize Descriptor V1

## Status

- Decision: accepted design candidate.
- Scope: Context Owner-local typed, neutral, exact and bounded read-only materialization.
- Implementation gate: this Design PR must merge before Go implementation starts.
- Production status: no currentness authority, Host composition, Model projection, Provider call, listener or SLA is granted.

## Purpose

`ContextRuntimeAPIV1` currently returns immutable Frames and `ContextFrameConsumptionDescriptorV1` values whose content is represented by exact `ContentRef` objects. A caller otherwise has to retain and call the Kernel Store directly to obtain bytes.

`MaterializeDescriptor` closes that Owner-local usability gap without changing ownership:

```text
immutable ContextFrameConsumptionDescriptorV1
+ explicit bounded materialization limits
+ Context-owned exact content Store
-> provider-neutral exact content items
```

The operation proves only that returned bytes exactly match the immutable Descriptor at the requested check time. It does not prove that the Descriptor remains Owner-current, create a Model projection, or authorize a Provider request.

## Public typed surface

`ContextRuntimeAPIV1` gains one method:

```go
MaterializeDescriptor(
    context.Context,
    MaterializeDescriptorRequestV1,
) (MaterializeDescriptorResultV1, error)
```

V1 remains typed Go only. No JSON operation, CLI command, HTTP/RPC listener, generated client or provider-specific DTO is added.

### Limits

```go
type MaterializeDescriptorLimitsV1 struct {
    MaxItems      uint32
    MaxItemBytes  uint64
    MaxTotalBytes uint64
}
```

All values are mandatory and positive. A caller may request smaller bounds but cannot exceed existing Context Owner-local safety maxima:

- `MaxItems <= 4`;
- `MaxItemBytes <= 68 MiB`;
- `MaxTotalBytes <= 100 MiB`.

The byte maxima reuse the existing Context Offline SDK generated-item and aggregate-output hard guards. They do not expand any Recipe, Frame, ContentRef or provider budget.

### Request

```go
type MaterializeDescriptorRequestV1 struct {
    Descriptor     contract.ContextFrameConsumptionDescriptorV1
    CheckedUnixNano int64
    Limits          MaterializeDescriptorLimitsV1
}
```

The request contains no caller-supplied currentness claim, cache hit, provider route or authority. `CheckedUnixNano` must satisfy:

```text
Descriptor.CheckedUnixNano <= CheckedUnixNano < Descriptor.ExpiresUnixNano
```

Time rollback and equality at expiry fail closed. Materialization never changes `ExpiresUnixNano` and never refreshes or extends any TTL.

### Result

```go
type MaterializedDescriptorRegionV1 string

const (
    MaterializedStablePrefixV1 MaterializedDescriptorRegionV1 = "stable_prefix"
    MaterializedSemiStableV1   MaterializedDescriptorRegionV1 = "semi_stable"
    MaterializedDynamicTailV1  MaterializedDescriptorRegionV1 = "dynamic_tail"
    MaterializedRenderedV1     MaterializedDescriptorRegionV1 = "rendered"
)

type MaterializedDescriptorItemV1 struct {
    Region MaterializedDescriptorRegionV1
    Ref    contract.ContentRef
    Bytes  []byte
}

type MaterializeDescriptorResultV1 struct {
    DescriptorRef  contract.ContextFrameConsumptionDescriptorRefV1
    Items          []MaterializedDescriptorItemV1
    CheckedUnixNano int64
    ExpiresUnixNano int64
    Digest          contract.Digest
}
```

Items have one canonical order:

1. `stable_prefix`;
2. `semi_stable` only when `Descriptor.SemiStable != nil`;
3. `dynamic_tail`;
4. `rendered`.

A Descriptor without SemiStable produces exactly three non-nil items. A Descriptor with SemiStable produces exactly four. Nil and empty item slices are not interchangeable in a successful result.

The result digest uses domain `praxis.context/materialized-descriptor-v1` and binds Descriptor Ref, canonical ordered region/ref pairs, Checked/Expires and version discriminator. `Validate` must additionally recompute every item ContentRef from `Bytes` before accepting the result; mutating returned bytes therefore invalidates the value.

## Exact and bounded algorithm

The algorithm order is mandatory:

1. check `ctx` and reject nil/canceled/deadline context;
2. validate Request, Descriptor and Limits;
3. derive the canonical 3-or-4 region/ref list from Descriptor;
4. before any output allocation or Store read, validate each Ref and use checked arithmetic over `Ref.Length`;
5. reject when item count, any item length, aggregate length or arithmetic exceeds the requested limits or hard maxima with `ErrLimitExceeded`;
6. read each exact Context `ContentRef` through the Service's existing `ContextAwareReferenceStoreV1`;
7. after every read, recheck context and require `len(bytes) == Ref.Length` and `DigestBytes(bytes) == Ref.Digest`;
8. deep-copy each byte slice into the result; no Store or caller alias may escape;
9. seal and validate the result digest;
10. return only the complete result.

No partial item, partial result or result digest becomes visible on failure. Store errors preserve existing sentinel/context identity; exact bytes that disagree with Ref return `ErrConflict`.

The operation is read-only. It does not write Fragment Cache, Frame Cache, Projection Cache or the underlying Store.

## Artifact boundary

An artifact-backed settled Tool result already places only Context-owned ArtifactRef metadata in the DynamicTail/Rendered content. `MaterializeDescriptor` may read those Context content refs, so it returns the metadata bytes already frozen in the Frame.

It must not:

- accept an Artifact Store or Artifact Reader;
- inspect, fetch, stream or cache the artifact body;
- follow an ArtifactRef transitively;
- replace metadata with a summary or body;
- claim the artifact itself is current.

The absence of any Artifact dependency is a compile-time boundary.

## Currentness and authority boundary

`MaterializeDescriptor` does not call `FrameConsumptionCurrentReaderV1` and does not claim currentness. The Descriptor was current when created, but may drift before its expiry; this operation proves only exact descriptor-to-bytes correspondence at `CheckedUnixNano`.

A caller that requires fresh Owner currentness must call `ConsumeFrame` again and materialize the returned Descriptor. This design does not combine those calls, invent a current snapshot, or publish a child Frame.

The result is not:

- a ModelContextProjection;
- a Provider request or cache handle;
- a Runtime/Review authority or Evidence;
- a Host continuation permit;
- a persisted/current Context Fact.

## Deep-copy and concurrency

- input slices are not retained;
- Store-returned slices are copied before return;
- every successful call returns independent item and byte slices;
- caller mutation cannot affect Store, Service or another result;
- identical Descriptor, Store bytes, check time and limits produce the same ordered refs and result digest;
- concurrent reads never create a logical write winner because the operation performs zero writes.

## Failure model

| Condition | Error identity | Result |
|---|---|---|
| nil service/context, invalid Descriptor/Limits/presence | `ErrInvalid` | zero |
| checked time before Descriptor check or at/after expiry | `ErrExpired` or `ErrConflict` for rollback | zero |
| item/byte limit or checked arithmetic overflow | `ErrLimitExceeded` | zero |
| exact content missing | `ErrNotFound` | zero |
| Store unavailable/unknown | existing Store sentinel | zero |
| returned bytes mismatch Length or Digest | `ErrConflict` | zero |
| canceled/deadline | original context error | zero |

There is no write or UnknownOutcome recovery path. Retrying a read is a caller policy; the facade adds no automatic retry.

## Explicit non-goals

- changing `contract`, `kernel`, `offlineapi` or `engineeringapi`;
- adding Frame/current publication, CAS or durable State Plane;
- adding function adapters as a separate product feature;
- adding JSON, CLI, HTTP, RPC, daemon or listener support;
- adding Host, Harness, Application, Model Invoker, Memory/Knowledge, Sandbox, Review, Continuity or Artifact dependencies;
- rendering provider messages, tokenizing, lowering Tool schemas or managing Prefix/KV cache;
- extending TTL or claiming currentness.

## Implementation scope after merge

- additive types and method in `ExecutionRuntime/context-engine/runtimeapi`;
- runtimeapi unit tests and Context black-box tests;
- minimal Context README status update if required.

No other module or Owner may be modified.

## Acceptance tests

1. Consume a current Frame, then materialize stable/semi/dynamic/rendered only through `ContextRuntimeAPIV1`.
2. Append inline Tool result and materialize the child Descriptor; stable/semi refs and bytes remain unchanged while dynamic/rendered change.
3. Artifact-backed Tool result materializes only frozen ArtifactRef metadata and performs zero Artifact-body reads by construction.
4. No-Semi Descriptor yields canonical three items; Semi Descriptor yields canonical four; ordering and digest are deterministic.
5. Missing, wrong-length and wrong-digest Store values fail closed with zero result.
6. Invalid Descriptor, rollback, expiry and equality-at-expiry fail closed without TTL extension.
7. Item count, per-item bytes, total bytes and uint64 overflow are rejected before Store read or output allocation.
8. Input/Store/output no-alias tests prove deep-copy; caller mutation invalidates only its returned value.
9. 64 concurrent materializations return equal digests and independent byte slices; cancellation/deadline preserve error identity.
10. Direct import scan proves no Host/Harness/Application/Model/Memory/Sandbox/Review/Continuity/Artifact dependency.
11. Implementation gates:

```bash
go test -count=100 ./runtimeapi ./tests/blackbox
go test -race -count=20 ./runtimeapi ./tests/blackbox
go test ./...
go test -race ./...
go vet ./...
gofmt -l <changed-go-files>
git diff --check origin/main...HEAD
```
