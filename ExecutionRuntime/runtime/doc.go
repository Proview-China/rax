// Package runtime provides the global Praxis execution-governance contracts.
//
// It owns execution lifecycle and coordination semantics. Domain components
// such as model invocation, context, tools, review, memory and sandbox remain
// behind versioned ports and keep ownership of their own facts.
package runtime

const ContractVersion = "praxis.runtime/v1alpha1"
