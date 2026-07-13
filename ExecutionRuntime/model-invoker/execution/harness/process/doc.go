// Package process runs an explicitly selected local Harness executable without
// a shell and exposes bounded JSON Lines or JSON-RPC-over-NDJSON framing.
//
// The package deliberately does not discover executables through PATH, inherit
// the parent environment, inspect user login state, or own a Runtime sandbox.
// Callers must provide an absolute executable, an allowed working directory,
// and an explicit environment allowlist.
package process
