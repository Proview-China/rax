package contract

import (
	"testing"
	"time"
)

func TestWorkspaceReadCommandRejectsNonCanonicalPathsBeforeExecution(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	digest, err := Digest("workspace-read-path-test", "coordinate")
	if err != nil {
		t.Fatal(err)
	}
	base := WorkspaceReadCommandV1{
		TenantID: "tenant", SourceToolCommand: Ref{ID: "tool", Revision: 1, Digest: digest},
		SourceToolPayloadSchema: "praxis.tool/workspace-read@1", SourceToolPayloadDigest: digest, SourceToolPayloadRevision: 1,
		WorkspaceView: Ref{ID: "workspace", Revision: 1, Digest: digest}, FileScopeDigest: digest, MaxBytes: 1,
		RequestedNotAfterUnixNano: expires.UnixNano(), OperationDigest: digest, EffectID: "effect",
		IntentRevision: 1, IntentDigest: digest, AttemptID: "attempt", PreparedDigest: digest, DispatchDigest: digest,
		ProviderComponent: "provider", ProviderManifest: digest,
	}
	for _, value := range []string{"../secret", "/absolute", "src\x00secret", `src\secret`} {
		value := value
		t.Run(value, func(t *testing.T) {
			command := base
			command.RelativePath = value
			if _, err := SealWorkspaceReadCommandV1(command, "command", now, expires); err == nil {
				t.Fatal("non-canonical path reached an executable command")
			}
		})
	}
}
