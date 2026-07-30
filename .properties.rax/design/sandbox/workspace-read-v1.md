# Bounded Workspace Read V1

## Status

Sandbox owner-local and Go-to-Rust transport slice implemented. This document does not define a Praxis Console page contract and does not claim the cross-owner ToolResult continuation is complete.

## Public boundary

The only Runtime-facing execution entry remains ControlledOperationPhysicalExecutionPortV3. Recovery uses the exact original WorkspaceReadAttemptRefV1. The exported kernel.WorkspaceReadActualPointV1 interface is only an internal Sandbox composition seam so dataplaneadapter can implement the Rust IPC bridge; it is not a product or Owner port.

## Exact command and result

WorkspaceReadCommandV1 binds tenant, exact source Tool command and payload, exact WorkspaceView, FileScopeDigest, logical path, StartByte, MaxBytes, optional ExpectedFileRef, TTL, and Runtime/Provider coordinates.

The result binds the exact whole-file ref and an independent bounded-range ContentDigest. It records start, returned, total, complete, S1/S2 times, the Runtime admission receipt, and the Provider receipt.

## Actual point and current checks

1. Go S1 re-reads association, command, and WorkspaceView.
2. Go independently re-reads Sandbox current and Runtime V4 current before reservation.
3. Go seals the natural minimum of Runtime authorization/enforcement, association, command, WorkspaceView, and lease TTLs. SQLite rejects any reservation or admission that exceeds this closure.
4. SQLite reserves an immutable original attempt and mutable current projection.
5. Rust resolves the path with Linux openat2 using BENEATH, NO_MAGICLINKS, and NO_SYMLINKS.
6. Rust validates regular-file type, the 1 MiB whole-file bound, and StartByte <= EOF.
7. The physical actual point is immediately before the first pread. Only then is PhysicalReadCount incremented.
8. Rust checks the same fd and a second exact-path open for dev, ino, size, mtime, and ctime drift. Drift returns ProviderUnknown with no content.
9. Go S2 re-reads association, command, and WorkspaceView after the Rust actual point.
10. Go seals the observation with the natural minimum of attempt, reservation, admission, and Provider-receipt TTLs, then advances SQLite by CAS.

Symlink, escape, special-file, oversized-file, and StartByte-after-EOF rejection occur before the actual point and count zero physical reads. Non-UTF-8 and post-read drift occur after the actual point and count one.

## Recovery

Public states are started, observed, and indeterminate. The immutable origin row prevents replacing an attempt ID with another original digest. Inspect validates the original ref, then returns the latest current projection without mutating it. A completed lost reply recovers the exact observation. Only an explicit owner recovery action with durable evidence of a different process incarnation may advance a restarted `started` attempt to `indeterminate`; concurrent Inspect can never poison live execution. Blind replay is forbidden.

## Security bounds

- Linux only for V1 secure path resolution.
- UTF-8 text only.
- Whole file <= 1 MiB; returned range <= command MaxBytes.
- No direct Go filesystem read and no shell execution.
- Unix IPC peer UID validation remains mandatory.
- Provider payload and response are canonical-digest checked on both sides.
