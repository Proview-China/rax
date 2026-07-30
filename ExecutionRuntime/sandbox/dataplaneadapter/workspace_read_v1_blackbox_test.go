package dataplaneadapter_test

import "testing"

// Keep the original public Go-to-Rust black-box gate, but run it through the
// exact V2 owner path instead of maintaining a second hand-written IPC request.
func TestWorkspaceReadGoAdapterCallsRustDataPlane(t *testing.T) {
	runWorkspaceReadPublicExecutorV1(t, workspaceReadExecutorCaseV1{
		startByte:        6,
		expectedState:    "observed",
		expectedContent:  "Praxis",
		expectedAdapter:  1,
		expectedPhysical: 1,
	})
}
