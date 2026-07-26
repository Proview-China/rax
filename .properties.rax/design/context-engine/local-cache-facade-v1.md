# Context Local Cache Facade V1

## Status

- Decision: accepted implementation candidate after Design PR review.
- Scope: Context Owner-local, provider-neutral and explicitly disposable local cache facade.
- Implementation gate: this Design PR must merge before Go implementation starts.
- Authority status: cache entries, invalidation generations and telemetry are not Context Current Facts and grant no Runtime, Harness, Model or Provider authority.

## Purpose

Context already has three independent in-memory reference implementations:

- `fragmentcache.MemoryV1`;
- `framecache.MemoryV1`;
- `projectioncache.MemoryV1`.

They prove the individual key, TTL, invalidation-generation and attempt-recovery semantics, but a component factory or official Agent sample still has to know three concrete packages and three unrelated method sets. None of the caches has a shared bounded-capacity contract.

`ContextLocalCacheFacadeV1` closes that Owner-local product gap:

```text
typed Fragment / Frame / Projection cache ports
+ one immutable local capacity policy
+ deterministic synchronous eviction
+ process-local observation telemetry
-> one Context-owned cache facade and conformance surface
```

The facade is an acceleration layer. Deleting the whole facade or losing every entry must affect latency only; it must not change Context truth, Frame identity, Generation currentness, Tool settlement or Model behavior.

## Existing contracts remain authoritative for cache identity

V1 reuses without changing:

- `ContextFragmentCacheKeyV1`;
- `ContextFrameCacheKeyV1`;
- `ContextProjectionCacheKeyV1`;
- each sealed cache `EntryV1`;
- existing invalidation-generation fields and CAS rules;
- `ErrInspectOnly` recovery for a repeated mutation attempt;
- exact TTL and digest validation already owned by each cache package.

The facade does not create a generic cache key, replace a key digest, infer currentness or translate one cache category into another.

## Public typed ports

The public ports are category-specific. No `any` payload, untyped map or shared mutable entry is permitted.

```go
type FragmentCachePortV1 interface {
    PutFragmentV1(context.Context, string, fragmentcache.EntryV1, int64) (fragmentcache.EntryV1, error)
    GetFragmentV1(context.Context, contract.ContextFragmentCacheKeyV1, int64) (fragmentcache.EntryV1, error)
    InspectFragmentAttemptV1(context.Context, string, int64) (fragmentcache.EntryV1, error)
    InvalidateFragmentV1(context.Context, contract.ContextFragmentCacheKeyV1, uint64) error
}

type FrameCachePortV1 interface {
    PutFrameV1(context.Context, string, framecache.EntryV1, int64) (framecache.EntryV1, error)
    GetFrameV1(context.Context, contract.ContextFrameCacheKeyV1, int64) (framecache.EntryV1, error)
    InspectFrameAttemptV1(context.Context, string, int64) (framecache.EntryV1, error)
    InvalidateFrameV1(context.Context, contract.ContextFrameCacheKeyV1, uint64) error
}

type ProjectionCachePortV1 interface {
    PutProjectionV1(context.Context, string, projectioncache.EntryV1, int64) (projectioncache.EntryV1, error)
    GetProjectionV1(context.Context, contract.ContextProjectionCacheKeyV1, int64) (projectioncache.EntryV1, error)
    InspectProjectionAttemptV1(context.Context, string, int64) (projectioncache.EntryV1, error)
    InvalidateProjectionV1(
        context.Context,
        contract.ContextProjectionCacheKeyV1,
        uint64,
        contract.ContextCacheInvalidationReasonV1,
    ) error
}

type ContextLocalCacheFacadeV1 interface {
    FragmentCachePortV1
    FrameCachePortV1
    ProjectionCachePortV1
    ObserveLocalCacheV1(context.Context, int64) (LocalCacheTelemetryObservationV1, error)
}
```

The exact Go package placement may avoid import cycles, but the nominal split and method semantics are fixed. The facade must wrap the three typed backends; it must not expose their concrete maps, locks or entry pointers.

## Capacity policy

Each cache category has an immutable policy supplied at facade construction:

```go
type LocalCacheCapacityV1 struct {
    MaxEntries          uint32
    MaxChargeBytes      uint64
    AttemptRecoveryNanos int64
}

type LocalCacheConfigV1 struct {
    Fragment   LocalCacheCapacityV1
    Frame      LocalCacheCapacityV1
    Projection LocalCacheCapacityV1
}
```

All nine values are mandatory and positive. V1 hard ceilings per category are:

- `MaxEntries <= 100_000`;
- `MaxChargeBytes <= 512 MiB`.
- `AttemptRecoveryNanos <= 5 minutes`.

The charge of an entry is the byte length of its canonical sealed JSON representation, computed with checked arithmetic before admission. Charge is an internal resource-accounting value, not part of the entry digest or cache identity.

An entry whose own charge exceeds the category policy fails with `ErrLimitExceeded` and leaves the cache unchanged. One stored value may be referenced by both the ordinary key index and the completed-attempt index, but its charge is counted exactly once. Capacity includes values retained inside the bounded attempt recovery window. Capacity configuration is immutable for the lifetime of one facade; resizing, live reconfiguration and remote quota control are not V1 operations.

Each category admits at most `MaxEntries` completed-attempt records. Attempt metadata contains only attempt ID, exact stored-value reference and recovery deadline; it does not copy the entry payload and does not add a second entry charge. Put and Inspect synchronously remove attempt records whose recovery deadline is at or before their explicit `now`. If cleanup still leaves `MaxEntries` attempt records, a new attempt admission fails with `ErrLimitExceeded` and publishes nothing. An already recorded exact attempt remains inspectable until its recovery deadline even while new attempts are rejected.

## Deterministic synchronous eviction

There is no background goroutine, wall-clock sampler, LRU clock or probabilistic policy.

Every successful Put performs one atomic category-local mutation:

1. validate context, attempt, sealed entry and supplied `now`;
2. compute the candidate charge with checked arithmetic;
3. reject an oversized candidate before mutation;
4. under the category lock, remove entries with `ExpiresUnixNano <= now` and completed-attempt records whose bounded recovery deadline is at or before `now`;
5. preserve the existing attempt and invalidation-generation checks;
6. if capacity is still insufficient, select existing values that are no longer protected by an attempt recovery window, by ascending:
   1. `ExpiresUnixNano`;
   2. cache Key Digest lexical bytes;
7. evict the minimum prefix of that ordering needed for the candidate;
8. admit the candidate and record its attempt mapping atomically.

The incoming candidate is never selected as its own victim. Eviction never changes or extends another entry's TTL and never increments an invalidation generation. A completed Put remains exactly inspectable until the earlier of entry expiry and `admitted time + AttemptRecoveryNanos`. While protected, its single stored value is charged against capacity and cannot be evicted. If protected values leave insufficient room, the new cache admission fails with `ErrLimitExceeded`; callers may continue without caching. After the recovery deadline, the attempt mapping may be purged and the value may become evictable. Inspect then returns `ErrNotFound` and must never recreate or replay the Put.

## TTL and invalidation semantics

- Put requires `now < ExpiresUnixNano`.
- Get at `now >= ExpiresUnixNano` returns `ErrExpired` and never refreshes the entry.
- Inspect Attempt requires explicit positive `now`, synchronously clears expired recovery records, returns the exact completed mutation result only inside its bounded recovery window and never refreshes TTL or performs another Put.
- Invalidate requires the existing `next == current + 1` CAS rule.
- Eviction does not masquerade as invalidation and does not modify the generation.
- A stale key generation returns `ErrConflict` even if an entry with the same digest remains physically present.
- Put, Get, Inspect Attempt and Observe all compare their explicit `now` with the facade's last accepted operation time. Clock rollback fails closed with `ErrConflict`; V1 does not rewrite timestamps.

The cache's invalidation generation is local concurrency control. It is not a Context Generation Current pointer, publication revision or Owner-current Fact.

## Deep-copy and concurrency boundary

- inputs are validated and not retained by reference;
- successful Put, Get and Inspect results are deep copies;
- caller mutation cannot alter stored entries, capacity charge or telemetry;
- category operations are linearizable within one facade instance;
- the three categories may execute independently;
- one attempt ID is namespaced by cache category, so equal text in different categories does not collide;
- concurrent same-attempt replay returns `ErrInspectOnly` after one winner;
- concurrent same-key/different-value Put returns one winner and `ErrConflict` for incompatible values;
- cancellation before the linearization point publishes nothing;
- cancellation after an indeterminate mutation result recovers only through `Inspect*AttemptV1`.

## Telemetry is an Observation

`ObserveLocalCacheV1` returns a deep-copied process-local snapshot:

```go
type LocalCacheCountersV1 struct {
    Hits          uint64
    Misses        uint64
    Conflicts     uint64
    Admissions    uint64
    Evictions     uint64
    ExpiredPurges uint64
    Invalidations map[contract.ContextCacheInvalidationReasonV1]uint64
    Entries       uint32
    ChargeBytes   uint64
}

type LocalCacheTelemetryObservationV1 struct {
    ConfigDigest    contract.Digest
    Fragment        LocalCacheCountersV1
    Frame           LocalCacheCountersV1
    Projection      LocalCacheCountersV1
    ObservedUnixNano int64
}
```

Counters are monotonically increasing for one process-local facade and saturate at `math.MaxUint64` instead of wrapping. `Entries` and `ChargeBytes` are gauges. The observation:

- is not a Fact, Current Projection, Evidence, cache-hit authority or billing record;
- has no publication, CAS, reset or durability operation;
- cannot authorize Model or Provider reuse;
- may be lost on restart;
- records only operations observed by this facade instance.

A Provider prefix/KV cache hit requires a later Model/Provider observation contract and is explicitly unrelated to a local Projection Cache hit.

## Facade behavior

`NewContextLocalCacheFacadeV1` requires exactly one typed backend per category, one validated immutable config and no external clock, network, Store or Provider dependency. Time is explicit on every operation and observation.

The first reference implementation may use bounded in-memory backends. The facade may adapt the existing `MemoryV1` stores or replace their internal containers, provided all old package tests and this conformance suite remain green.

The facade does not automatically populate caches from `ConsumeFrame`, `AppendSettledToolResult` or `MaterializeDescriptor`. Automatic read-through/write-through policy belongs to a later Agent sample or composition design; V1 only supplies safe typed cache operations.

## Failure model

| Condition | Error identity | Mutation |
|---|---|---|
| nil context/facade/backend, invalid config or entry | `ErrInvalid` | none |
| candidate, configured charge limit or recovery-protected capacity exceeded | `ErrLimitExceeded` | none |
| entry absent or evicted on ordinary Get | `ErrNotFound` | none |
| entry expired | `ErrExpired` | optional synchronous expired purge only |
| stale invalidation generation or same key/different value | `ErrConflict` | none |
| repeated completed mutation attempt | `ErrInspectOnly` | none; caller may Inspect |
| backend unavailable/unknown | existing sentinel | no retry by facade |
| canceled/deadline before linearization | original context error | none |
| mutation outcome unknown | existing unknown sentinel | Inspect exact attempt only |

No partial admission, capacity accounting or telemetry update becomes visible on a definitely failed mutation. Observation failure never changes cache contents.

## Conformance kit

One reusable `localcacheconformance` suite must accept a facade factory and run the same contract against the reference implementation and future backends.

Mandatory cases:

1. typed Put/Get/Inspect/Invalidate for Fragment, Frame and Projection;
2. deterministic entry and telemetry output for the same operation schedule;
3. exact boundary at entry and byte capacities;
4. candidate-too-large rejection before mutation;
5. expired purge before deterministic `Expires + Digest` eviction;
6. eviction does not extend TTL or increment invalidation generation;
7. Put/Inspect cleanup keeps at most `MaxEntries` attempt records, completed-attempt Inspect remains exact during the bounded recovery window, and after the deadline Inspect is NotFound without replay;
8. stale generation, equality-at-expiry and clock rollback fail closed;
9. 64 concurrent same-attempt and same-key/different-value writers;
10. ABA sequence `generation N -> N+1` rejects an old N key after later activity;
11. cancellation and UnknownOutcome recover only through exact Inspect;
12. deep-copy/no-alias for inputs, outputs, maps and nested slices;
13. counters saturate, snapshots do not alias and telemetry never changes decisions;
14. direct import scan proves no Runtime, Harness, Model Invoker, Application, Tool/MCP, Memory/Knowledge, Sandbox, Review, Continuity or Provider dependency.

## Explicit non-goals

- Context Current publication, Frame Current, Recipe/Prompt publish, rollback or revoke;
- changing Context Generation current CAS or introducing another state Owner;
- SQLite, RocksDB, Redis, remote cache or durable State Plane backend;
- automatic read-through/write-through composition;
- Provider cache observation, Prompt/KV cache management or cache billing;
- StructuralValueEvaluator, CompressionEvidence or small-model compression;
- HTTP/RPC listener, CLI command, JSON operation or generated client;
- Host/Harness continuation, ModelContextProjection or Provider lowering;
- changing existing Context Cache Key or Entry contracts.

## Implementation scope after merge

- additive typed cache ports and facade package under `ExecutionRuntime/context-engine`;
- bounded in-memory reference implementation, adapting the existing three cache packages;
- reusable conformance suite and owner-local black-box/failure tests;
- minimal Context README status update.

No other module or Owner may be modified.

## Implementation gates

```bash
go test -count=100 ./fragmentcache ./framecache ./projectioncache ./localcache ./localcacheconformance
go test -race -count=20 ./fragmentcache ./framecache ./projectioncache ./localcache ./localcacheconformance
go test ./...
go test -race ./...
go vet ./...
gofmt -l <changed-go-files>
git diff --check origin/main...HEAD
```
