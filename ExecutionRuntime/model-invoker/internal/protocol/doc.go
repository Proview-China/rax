// Package protocol defines the SDK-neutral boundary shared by internal wire
// protocol drivers. Runtime provider identity is injected through Binding;
// credentials and provider SDK objects never cross this package boundary.
package protocol
