# Workspace Read V1 Module Map

| Layer | Files | Responsibility |
|---|---|---|
| Contract | ExecutionRuntime/sandbox/contract/workspace_read_v1.go | Exact command, TTL closure, attempt, observation, projection, digest, states |
| Public ports | ExecutionRuntime/sandbox/ports/workspace_read_v1.go | Runtime V3 entry and exact Inspect |
| Kernel | ExecutionRuntime/sandbox/kernel/workspace_read_v1.go | S1/S2, Runtime current re-read, reservation, CAS, sealing |
| SQLite | ExecutionRuntime/sandbox/storage/sqlite/workspace_read_v1.go | Durable origin/current, immutable Command and historical WorkspaceView exact re-read, path/range/file splice rejection, pure Inspect, explicit incarnation recovery, TTL write gates, concurrency |
| Go Data Plane adapter | ExecutionRuntime/sandbox/dataplaneadapter/workspace_read_v1.go | Typed seam to sealed IPC request/result |
| Rust Provider | ExecutionRuntime/sandbox/dataplane/src/workspace_read.rs | openat2, pread, exact file/range digest, S2 metadata |
| Root registration | Rust contract/provider/enforcer/bin files | Wire decode, result validation, Provider routing |
| Black boxes | Go and Rust workspace_read tests | IPC, replay/Inspect, concurrency, drift, fail-closed bounds |

The completion gate re-reads the immutable `WorkspaceReadCommandV1` and the exact
historical `WorkspaceView` before accepting an Observation. The Reservation,
Observation, Provider Receipt, deterministic view-scoped File identity,
relative path, byte range, expected file, scope digest, and WorkspaceView must
all remain on one exact coordinate.

Both S1 and S2 compare every Workspace lease binding field with Runtime current.
The real public Executor black box crosses the Go adapter and Rust Host IPC and
proves pre-actual zero-read rejection, post-read Unknown, EOF/empty success, and
S2 lease drift handling.

The Canvas, Sidebar, localStorage, and static Module Library labels are deliberately absent because they are not frozen public contracts.
