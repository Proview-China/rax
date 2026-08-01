package kernel_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	sqlitestore "github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

type historySourceReader struct {
	value contract.WorkspaceReadSourceCurrentProjectionV2
}

func (r historySourceReader) InspectWorkspaceReadSourceCurrentV2(context.Context, contract.WorkspaceReadSourceCommandRefV2) (contract.WorkspaceReadSourceCurrentProjectionV2, error) {
	return r.value, nil
}

type historyEffectReader struct {
	value runtimeports.ControlledOperationEffectCurrentProjectionV2
}

func (r historyEffectReader) InspectCurrentControlledOperationEffectV2(context.Context, runtimeports.OperationSubjectV3, runtimecore.EffectIntentID) (runtimeports.ControlledOperationEffectCurrentProjectionV2, error) {
	return r.value, nil
}

type historyPreparedReader struct {
	value runtimeports.ControlledOperationPreparedCurrentProjectionV2
}

func (r historyPreparedReader) InspectCurrentControlledOperationPreparedV2(context.Context, runtimeports.PreparedProviderAttemptRefV2) (runtimeports.ControlledOperationPreparedCurrentProjectionV2, error) {
	return r.value, nil
}

type historyWorkspaceReader struct{ value contract.WorkspaceView }

func (r historyWorkspaceReader) InspectWorkspaceViewCurrentV1(context.Context, contract.Ref) (contract.WorkspaceView, error) {
	return r.value, nil
}

func TestWorkspaceReadCommandOwnerPublicCurrentRejectsFutureOrphanHistoryV2(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	fixture := testkit.WorkspaceReadCommandPublicationV2(now, "public-history")
	semantic, err := contract.SealWorkspaceReadCommandPublicationSemanticV2(fixture.Source, fixture.Effect, fixture.Prepared, fixture.Workspace, now)
	if err != nil {
		t.Fatal(err)
	}
	command, err := contract.SealWorkspaceReadPublishedCommandV2(semantic, now)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := contract.SealWorkspaceReadCommandPublicationV2(contract.WorkspaceReadCommandPublicationV2{Semantic: semantic}, command, now)
	if err != nil {
		t.Fatal(err)
	}
	body := publicHistoryCurrentBody(command, publication, fixture, now)
	first, err := contract.SealInitialWorkspaceReadCommandOwnerCurrentV2(body, now)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := ownerworkspaceread.NewInitialCommandPublicationV2(command, publication, first, fixture.Source, fixture.Effect, fixture.Prepared, fixture.Workspace, now)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := sqlitestore.OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, created, err := store.ApplyWorkspaceReadCommandPublicationV2(ctx, capability); err != nil || !created {
		t.Fatalf("apply publication created=%v err=%v", created, err)
	}
	owner, err := kernel.NewWorkspaceReadCommandOwnerV2(
		historySourceReader{fixture.Source}, historyEffectReader{fixture.Effect},
		historyPreparedReader{fixture.Prepared}, historyWorkspaceReader{fixture.Workspace},
		store, func() time.Time { return now.Add(2 * time.Second) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.InspectWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, command.Meta.Ref()); err != nil {
		t.Fatalf("valid public current rejected: %v", err)
	}

	secondChecked := now.Add(time.Second)
	body.CheckedUnixNano = secondChecked.UnixNano()
	second, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(body, first, secondChecked)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.ExecContext(ctx, `INSERT INTO workspace_read_command_owner_current_history_v2(
		current_id,revision,digest,command_id,command_revision,command_digest,
		publication_id,publication_revision,publication_digest,checked_unix_nano,expires_unix_nano,body
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		second.Meta.ID, second.Meta.Revision, second.Meta.Digest,
		second.Command.ID, second.Command.Revision, second.Command.Digest,
		second.Publication.ID, second.Publication.Revision, second.Publication.Digest,
		second.CheckedUnixNano, second.ExpiresUnixNano, encoded,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.InspectWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, command.Meta.Ref()); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("future orphan public current error=%v, want Conflict", err)
	}
}

func publicHistoryCurrentBody(command contract.WorkspaceReadCommandV1, publication contract.WorkspaceReadCommandPublicationV2, fixture testkit.WorkspaceReadCommandPublicationFixtureV2, checked time.Time) contract.WorkspaceReadCommandOwnerCurrentV2 {
	return contract.WorkspaceReadCommandOwnerCurrentV2{
		Command: command.Meta.Ref(), Publication: publication.Meta.Ref(),
		PublicationSemanticDigest: publication.Semantic.Digest,
		SourceCommand:             publication.Semantic.SourceCommand, SourceSemanticDigest: publication.Semantic.SourceSemanticDigest,
		SourceProjectionDigest: fixture.Source.ProjectionDigest, SourceCheckedUnixNano: fixture.Source.CheckedUnixNano, SourceExpiresUnixNano: fixture.Source.ExpiresUnixNano,
		RuntimeEffectProjectionDigest: fixture.Effect.Digest, RuntimeEffectCheckedUnixNano: fixture.Effect.CheckedUnixNano, RuntimeEffectExpiresUnixNano: fixture.Effect.ExpiresUnixNano,
		RuntimePreparedProjectionDigest: fixture.Prepared.ProjectionDigest, RuntimePreparedCheckedUnixNano: fixture.Prepared.CheckedUnixNano, RuntimePreparedExpiresUnixNano: fixture.Prepared.ExpiresUnixNano,
		WorkspaceView: fixture.Workspace.Meta.Ref(), WorkspaceSemanticDigest: publication.Semantic.WorkspaceSemanticDigest,
		WorkspaceCheckedUnixNano: checked.UnixNano(), WorkspaceExpiresUnixNano: fixture.Workspace.Meta.ExpiresUnixNano, WorkspaceLeaseExpiresUnixNano: fixture.Workspace.Lease.ExpiresUnixNano,
		SemanticNotAfterUnixNano: publication.Semantic.SemanticNotAfterUnixNano,
	}
}
