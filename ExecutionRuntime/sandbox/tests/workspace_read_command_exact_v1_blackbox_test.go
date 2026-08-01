package sandbox_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
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
	command := publishWorkspaceReadCommandExactBlackboxV2(t, ctx, store, created)
	if command.Meta.ExpiresUnixNano >= expires.UnixNano() {
		t.Fatalf("publication fixture no longer has a bounded lifetime: %#v", command.Meta)
	}
	if err != nil {
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
	if _, ok := any(reopened).(ports.WorkspaceReadCommandCurrentReaderV1); ok {
		t.Fatal("Store structurally regained the public raw current port")
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

func publishWorkspaceReadCommandExactBlackboxV2(
	t *testing.T,
	ctx context.Context,
	store *sqlite.Store,
	now time.Time,
) contract.WorkspaceReadCommandV1 {
	t.Helper()
	fixture := testkit.WorkspaceReadCommandPublicationV2(now, "exact-blackbox")
	semantic, err := contract.SealWorkspaceReadCommandPublicationSemanticV2(
		fixture.Source,
		fixture.Effect,
		fixture.Prepared,
		fixture.Workspace,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	command, err := contract.SealWorkspaceReadPublishedCommandV2(semantic, now)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := contract.SealWorkspaceReadCommandPublicationV2(
		contract.WorkspaceReadCommandPublicationV2{Semantic: semantic},
		command,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err := contract.SealInitialWorkspaceReadCommandOwnerCurrentV2(
		contract.WorkspaceReadCommandOwnerCurrentV2{
			Command: command.Meta.Ref(), Publication: publication.Meta.Ref(),
			PublicationSemanticDigest:       publication.Semantic.Digest,
			SourceCommand:                   publication.Semantic.SourceCommand,
			SourceSemanticDigest:            publication.Semantic.SourceSemanticDigest,
			SourceProjectionDigest:          fixture.Source.ProjectionDigest,
			SourceCheckedUnixNano:           fixture.Source.CheckedUnixNano,
			SourceExpiresUnixNano:           fixture.Source.ExpiresUnixNano,
			RuntimeEffectProjectionDigest:   fixture.Effect.Digest,
			RuntimeEffectCheckedUnixNano:    fixture.Effect.CheckedUnixNano,
			RuntimeEffectExpiresUnixNano:    fixture.Effect.ExpiresUnixNano,
			RuntimePreparedProjectionDigest: fixture.Prepared.ProjectionDigest,
			RuntimePreparedCheckedUnixNano:  fixture.Prepared.CheckedUnixNano,
			RuntimePreparedExpiresUnixNano:  fixture.Prepared.ExpiresUnixNano,
			WorkspaceView:                   fixture.Workspace.Meta.Ref(),
			WorkspaceSemanticDigest:         publication.Semantic.WorkspaceSemanticDigest,
			WorkspaceCheckedUnixNano:        now.UnixNano(),
			WorkspaceExpiresUnixNano:        fixture.Workspace.Meta.ExpiresUnixNano,
			WorkspaceLeaseExpiresUnixNano:   fixture.Workspace.Lease.ExpiresUnixNano,
			SemanticNotAfterUnixNano:        publication.Semantic.SemanticNotAfterUnixNano,
		},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := ownerworkspaceread.NewInitialCommandPublicationV2(
		command,
		publication,
		current,
		fixture.Source,
		fixture.Effect,
		fixture.Prepared,
		fixture.Workspace,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.ApplyWorkspaceReadCommandPublicationV2(ctx, capability); err != nil {
		t.Fatal(err)
	}
	return command
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
