package contract

import (
	"testing"
	"time"
)

func TestWorkspaceReadCommandTTLUsesMetaExpiryAsNaturalUpperBound(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	requested := now.Add(time.Hour)
	base := workspaceReadCommandTTLFixtureV1(t, requested)

	t.Run("equal", func(t *testing.T) {
		command, err := SealWorkspaceReadCommandV1(base, "command-equal", now, requested)
		if err != nil {
			t.Fatal(err)
		}
		if command.Meta.ExpiresUnixNano != command.RequestedNotAfterUnixNano {
			t.Fatalf("equal TTL drifted: meta=%d requested=%d", command.Meta.ExpiresUnixNano, command.RequestedNotAfterUnixNano)
		}
	})

	t.Run("default requested bound", func(t *testing.T) {
		command := base
		command.RequestedNotAfterUnixNano = 0
		sealed, err := SealWorkspaceReadCommandV1(command, "command-default", now, requested)
		if err != nil {
			t.Fatal(err)
		}
		if sealed.RequestedNotAfterUnixNano != sealed.Meta.ExpiresUnixNano {
			t.Fatalf("default requested bound=%d want=%d", sealed.RequestedNotAfterUnixNano, sealed.Meta.ExpiresUnixNano)
		}
	})

	t.Run("Meta expires earlier", func(t *testing.T) {
		metaExpires := requested.Add(-time.Minute)
		command, err := SealWorkspaceReadCommandV1(base, "command-meta-earlier", now, metaExpires)
		if err != nil {
			t.Fatal(err)
		}
		if command.Meta.ExpiresUnixNano >= command.RequestedNotAfterUnixNano {
			t.Fatalf("test precondition failed: meta=%d requested=%d", command.Meta.ExpiresUnixNano, command.RequestedNotAfterUnixNano)
		}
		if err = command.ValidateCurrent(metaExpires.Add(-time.Nanosecond)); err != nil {
			t.Fatalf("Command expired before Meta boundary: %v", err)
		}
		if err = command.ValidateCurrent(metaExpires); err == nil {
			t.Fatal("Command remained current at exact Meta expiry")
		}
	})

	t.Run("Meta expires later", func(t *testing.T) {
		if _, err := SealWorkspaceReadCommandV1(base, "command-meta-later", now, requested.Add(time.Nanosecond)); err == nil {
			t.Fatal("Command extended Meta expiry beyond requested upper bound")
		}
	})
}

func TestWorkspaceReadCommandCurrentTTLBoundariesFailClosed(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	command, err := SealWorkspaceReadCommandV1(
		workspaceReadCommandTTLFixtureV1(t, expires),
		"command-boundaries",
		now,
		expires,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err = command.ValidateCurrent(now.Add(-time.Nanosecond)); err == nil {
		t.Fatal("Command accepted a clock earlier than its owner timestamp")
	}
	if err = command.ValidateCurrent(expires.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("Command expired before the half-open TTL boundary: %v", err)
	}
	if err = command.ValidateCurrent(time.Unix(0, command.Meta.ExpiresUnixNano)); err == nil {
		t.Fatal("Command remained current at exact Meta expiry")
	}
	if err = command.ValidateCurrent(time.Unix(0, command.RequestedNotAfterUnixNano)); err == nil {
		t.Fatal("Command remained current at exact requested expiry")
	}
}

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

func workspaceReadCommandTTLFixtureV1(t *testing.T, requested time.Time) WorkspaceReadCommandV1 {
	t.Helper()
	digest, err := Digest("workspace-read-command-ttl-test", "coordinate")
	if err != nil {
		t.Fatal(err)
	}
	return WorkspaceReadCommandV1{
		TenantID: "tenant", SourceToolCommand: Ref{ID: "tool", Revision: 1, Digest: digest},
		SourceToolPayloadSchema: "praxis.tool/workspace-read@1", SourceToolPayloadDigest: digest, SourceToolPayloadRevision: 1,
		WorkspaceView: Ref{ID: "workspace", Revision: 1, Digest: digest}, FileScopeDigest: digest,
		RelativePath: "src/main.go", MaxBytes: 1, RequestedNotAfterUnixNano: requested.UnixNano(),
		OperationDigest: digest, EffectID: "effect", IntentRevision: 1, IntentDigest: digest,
		AttemptID: "attempt", PreparedDigest: digest, DispatchDigest: digest,
		ProviderComponent: "provider", ProviderManifest: digest,
	}
}
