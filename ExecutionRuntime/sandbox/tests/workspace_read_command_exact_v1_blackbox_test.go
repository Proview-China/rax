package sandbox_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestWorkspaceReadCommandExactReaderV1BlackboxExpiredRestartAndConcurrency(t *testing.T) {
	ctx := context.Background()
	created := time.Unix(1_960_000_000, 0)
	expires := created.Add(time.Minute)
	current := created
	database := t.TempDir() + "/sandbox.db"
	store, err := sqlite.OpenWithClock(ctx, database, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	command := workspaceReadCommandExactBlackboxFixtureV1(t, created, expires)
	if _, err = store.CreateWorkspaceReadCommandV1(ctx, command); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	current = expires.Add(time.Hour)
	reopened, err := sqlite.OpenWithClock(ctx, database, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if _, err = reopened.InspectWorkspaceReadCommandCurrentV1(ctx, command.Meta.Ref()); err == nil {
		t.Fatal("expired Command remained readable through the current port")
	}
	var reader ports.WorkspaceReadCommandExactReaderV1 = reopened

	const readers = 64
	var group sync.WaitGroup
	failures := make(chan error, readers)
	for range readers {
		group.Add(1)
		go func() {
			defer group.Done()
			got, inspectErr := reader.InspectWorkspaceReadCommandExactV1(ctx, command.Meta.Ref())
			if inspectErr == nil && !reflect.DeepEqual(got, command) {
				inspectErr = ports.ErrConflict
			}
			failures <- inspectErr
		}()
	}
	group.Wait()
	close(failures)
	for inspectErr := range failures {
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
	}
}

func TestWorkspaceReadCommandExactReaderV1PublicSurfaceIsReadOnly(t *testing.T) {
	surface := reflect.TypeOf((*ports.WorkspaceReadCommandExactReaderV1)(nil)).Elem()
	if surface.NumMethod() != 1 ||
		surface.Method(0).Name != "InspectWorkspaceReadCommandExactV1" {
		t.Fatalf("historical Command reader surface gained mutation or execution power: %v", surface)
	}
}

func workspaceReadCommandExactBlackboxFixtureV1(t *testing.T, now, expires time.Time) contract.WorkspaceReadCommandV1 {
	t.Helper()
	digest := func(value string) string {
		result, err := contract.Digest("workspace-read-command-exact-blackbox", value)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	command, err := contract.SealWorkspaceReadCommandV1(
		contract.WorkspaceReadCommandV1{
			TenantID:                  "tenant",
			SourceToolCommand:         contract.Ref{ID: "tool-command", Revision: 1, Digest: digest("tool-command")},
			SourceToolPayloadSchema:   "praxis.tool/workspace-read@1",
			SourceToolPayloadDigest:   digest("payload"),
			SourceToolPayloadRevision: 1,
			WorkspaceView:             contract.Ref{ID: "workspace", Revision: 1, Digest: digest("workspace")},
			FileScopeDigest:           digest("scope"),
			RelativePath:              "src/main.txt",
			MaxBytes:                  32,
			RequestedNotAfterUnixNano: expires.UnixNano(),
			OperationDigest:           digest("operation"),
			EffectID:                  "effect",
			IntentRevision:            1,
			IntentDigest:              digest("intent"),
			AttemptID:                 "attempt",
			PreparedDigest:            digest("prepared"),
			DispatchDigest:            digest("dispatch"),
			ProviderComponent:         "provider",
			ProviderManifest:          digest("provider-manifest"),
		},
		"workspace-read-command-exact-blackbox",
		now,
		expires,
	)
	if err != nil {
		t.Fatal(err)
	}
	return command
}
