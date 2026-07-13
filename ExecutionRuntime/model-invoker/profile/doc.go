// Package profile resolves exact semantic Route profiles, composes scoped
// runtime policy, compares expected and actual Harness manifests, and compiles
// deterministic execution plans without contacting an upstream.
//
// The package is intentionally separate from modelinvoker's direct model
// Request/Response contract. It never resolves credentials and never starts a
// Provider, CLI, SDK, App Server, or ACP process.
package profile
