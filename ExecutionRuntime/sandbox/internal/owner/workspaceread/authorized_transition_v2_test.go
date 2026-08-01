package workspaceread_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	sqlitestore "github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestWorkspaceReadAuthorizedTransitionV2RejectsMissingAuthorityAndObservationV2(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_951_100_000, 0).UTC()
	expires := now.Add(time.Minute)
	store, err := sqlitestore.OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fixture := newAuthorizedFixtureV2(t, store, now, expires, "transition", "transition", delegationRefV2("delegation-transition"))
	attempt := contract.WorkspaceReadAttemptRefV1{
		ID: fixture.attempt.Meta.ID, Revision: fixture.attempt.Meta.Revision, Digest: fixture.attempt.Meta.Digest,
	}
	if _, err = ownerworkspaceread.NewAuthorizedExecutionV2(
		attempt,
		runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{},
		now,
	); err == nil {
		t.Fatal("missing Runtime authorization issued a Sandbox transition capability")
	}
	authority, err := ownerworkspaceread.NewAuthorizedExecutionV2(attempt, fixture.authorization, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = authority.Observed(contract.WorkspaceReadObservationV1{}, now); err == nil {
		t.Fatal("malformed provider observation issued an observed transition")
	}
}
