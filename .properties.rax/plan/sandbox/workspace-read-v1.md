# Bounded Workspace Read V1 Plan

## Completed

- exact command, sealed upstream TTL closure, reservation, attempt, observation, projection, and receipt contracts;
- Runtime V3 physical execution and exact Inspect ports;
- owner SQLite schema v14, immutable origin/current split, CAS, explicit incarnation-bound restart recovery, read-only Inspect, and concurrent reservation;
- Go kernel S1/S2 current closure and indeterminate handling;
- concrete Go dataplaneadapter to Rust Data Plane Unix IPC;
- Rust secure bounded read Provider and root registration;
- real public Executor-to-Go-adapter-to-Rust-Host black box, exact journal Inspect, 64-way concurrency, path/race/size/UTF-8 negative tests;
- ordinary, race, vet, Rust all-target, fmt, and clippy verification.

## Explicit non-goals

- no Praxis Console page, ViewModel, Command, Query, or Event contract;
- no generic filesystem browser;
- no binary read, directory listing, glob, shell, or arbitrary host path;
- no ToolResult-to-next-turn continuation claim;
- no AgentPackage or production Host composition claim outside this Sandbox slice.

## Next cross-owner handoff

Tool/Harness may consume the bounded observation only after they freeze an exact ToolResult/Context continuation contract. Until then this remains a complete Sandbox owner-local physical capability, not an end-user Console feature.
