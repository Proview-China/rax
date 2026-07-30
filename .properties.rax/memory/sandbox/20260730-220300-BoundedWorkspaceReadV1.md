# 2026-07-30 Bounded Workspace Read V1

The Sandbox owner implemented one bounded, exact workspace.read physical slice on branch agent/sandbox-workspace-read-v1 from base c421b245.

Frozen decisions:

- Runtime-facing entry is V3 controlled physical execution; exact attempt ref is the only recovery coordinate.
- public states are started, observed, and indeterminate; Inspect is read-only.
- immutable origin and mutable current are separate SQLite rows.
- explicit restart recovery requires a different durable owner incarnation and writes recovery evidence.
- reservation and completion use sealed natural-minimum TTL closures; `now == expires` fails closed.
- Linux openat2/pread is authoritative; direct Go file reads are forbidden.
- physical count increments immediately before pread, after all pre-actual-point validation.
- S2 checks both the same fd and a second exact-path open; any drift is ProviderUnknown with no content.
- Go independently re-reads Runtime/Sandbox current before IPC and command/workspace current after the Rust actual point.
- the concrete Go adapter calls the real Rust Host over UID-bound Unix IPC.

Verification evidence:

- go test ./... PASS.
- go test -race ./... PASS for the Sandbox module.
- go vet ./... PASS.
- cargo test --all-targets PASS; privileged containerd/KVM cases remain intentionally ignored by their existing environment gates.
- cargo fmt --check PASS.
- cargo clippy --all-targets -- -D warnings PASS.
- explicit public Executor-to-Go-adapter-to-Rust-Host black box PASS and recovered the exact durable result with Inspect.

Boundary: this evidence is Sandbox owner-local plus real transport. It does not mean any Praxis Console page, AgentRunService public wire, ToolResult continuation, or production multi-owner Host composition is complete.
