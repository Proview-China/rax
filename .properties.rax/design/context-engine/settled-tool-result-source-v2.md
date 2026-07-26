# Settled Tool Result Source V2

## Status

- Decision: accepted.
- Scope: Context-owned integration contract and acceptance criteria only.
- Implementation gate: Tool PR #7 must be merged into main before Go implementation starts.
- Compatibility: SettledActionContextSourceRequestV1 remains supported and unchanged.

## Purpose

SettledToolResultSourceV2 appends one current settled Tool result to the next immutable ContextFrame without copying or re-adjudicating Runtime or Application governance.

```text
current parent ContextFrame
+ current SettledToolResultProjectionV1
+ bounded local invalidation
-> next immutable ContextManifest
-> next immutable ContextFrame
-> ContextFrameConsumptionDescriptorV1
```

## Source boundary

V2 may bind only:

- exact Tool projection ID, revision, digest and expiry;
- exact Tool result and Tool references exposed by the projection;
- Runtime V4 inspection reference already sealed into the projection;
- payload schema, payload revision and payload digest;
- exactly one payload carrier: bounded inline bytes or exact ArtifactRef;
- classification, completeness, checked time and projection digest.

Before Context accepts the source, the Tool public current validation must succeed against the exact Tool result at the same check time.

V2 must not contain or reconstruct Runtime Permit, Authorization, Enforcement, Application association or dispatch governance. It must not make a second Tool settlement decision or carry provider-specific prompt or KV-cache state.

Run, turn, tenant scope and authority come from the parent frame, ExpectedCurrent and Host input. Context validates compatibility but does not source or decide them again.

## Currentness and lifetime

- Effective expiry is the minimum of Tool projection expiry, Runtime inspection expiry, parent/current window and refresh deadline.
- Expired or indeterminate input is rejected before a Manifest or Frame is sealed.
- Unknown outcome is recovered only through the owning exact reader or Inspect path. Context never retries or infers Tool execution.
- Same parent, exact source and idempotency key return the same winner.
- Stale revision, digest drift and ABA fail closed.

## Incremental frame behavior

- Stable Prefix is structurally shared and remains unchanged unless an explicitly stable Tool Surface fragment changes.
- A settled Tool result changes only the dynamic tail and directly dependent fragments.
- Local invalidation names the changed fragment set; unrelated caches stay reusable.
- Next Manifest, Frame and neutral consumption descriptor receive deterministic identities derived from parent and exact source.
- Provider lowering and provider Prefix or KV cache remain Model Invoker responsibilities.

## Payload policy

- Inline payload is non-empty, bounded and exactly matches PayloadDigest.
- Large payload uses the exact Tool-admitted ArtifactRef; Context stores only its reference and metadata.
- Inline and ArtifactRef are mutually exclusive.
- Context never materializes a large artifact merely to compile the frame.

## Acceptance tests

1. Current inline result creates a new immutable Manifest, Frame and neutral consumption descriptor.
2. Current artifact result records only the exact ArtifactRef.
3. Stable-prefix fingerprint remains unchanged while dynamic-tail and frame fingerprints change deterministically.
4. Unrelated Fragment, Frame and Projection cache entries remain reusable.
5. Expired projection, inspection or refresh deadline fails before sealing.
6. Result or projection revision and digest drift fails closed.
7. Inline digest drift, inline plus artifact, missing payload and oversized inline fail closed.
8. Artifact exact-ref or digest drift fails closed.
9. Duplicate request and lost reply return the same exact winner.
10. Concurrent identical refresh has one logical winner.
11. Stale revision and ABA cannot overwrite a newer generation.
12. Cancellation leaves no partially committed Manifest, Frame or descriptor.
13. Tool current validation is called before Context mutation.
14. No Runtime or Application governance type is copied into V2.
15. ordinary repeated tests, race tests, full module tests, vet and diff-check pass.

## Non-goals

- Changing Tool PR #7 or its public projection.
- Changing Runtime, Application, Harness, Memory, Knowledge or Sandbox contracts.
- Removing V1 compatibility.
- Provider-specific rendering or cache implementation.
- Fetching or materializing large artifacts.
- Claiming production composition before Host integration exists.

