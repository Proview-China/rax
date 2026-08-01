package workspaceread

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
)

func authorizedCommandPublicationFixtureV2(
	t *testing.T,
) (
	testkit.WorkspaceReadCommandPublicationFixtureV2,
	contract.WorkspaceReadCommandV1,
	contract.WorkspaceReadCommandPublicationV2,
	contract.WorkspaceReadCommandOwnerCurrentV2,
	contract.WorkspaceReadCommandOwnerCurrentV2,
	time.Time,
) {
	t.Helper()
	now := time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC)
	fixture := testkit.WorkspaceReadCommandPublicationV2(now, "owner-capability")
	semantic, err := contract.SealWorkspaceReadCommandPublicationSemanticV2(
		fixture.Source, fixture.Effect, fixture.Prepared, fixture.Workspace, now,
	)
	if err != nil {
		t.Fatalf("seal semantic: %v", err)
	}
	commitNow := now.Add(time.Second)
	command, err := contract.SealWorkspaceReadPublishedCommandV2(semantic, commitNow)
	if err != nil {
		t.Fatalf("seal command: %v", err)
	}
	publication, err := contract.SealWorkspaceReadCommandPublicationV2(
		contract.WorkspaceReadCommandPublicationV2{Semantic: semantic},
		command,
		commitNow,
	)
	if err != nil {
		t.Fatalf("seal publication: %v", err)
	}
	body := contract.WorkspaceReadCommandOwnerCurrentV2{
		Command:                         command.Meta.Ref(),
		Publication:                     publication.Meta.Ref(),
		PublicationSemanticDigest:       semantic.Digest,
		SourceCommand:                   semantic.SourceCommand,
		SourceSemanticDigest:            semantic.SourceSemanticDigest,
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
		WorkspaceSemanticDigest:         semantic.WorkspaceSemanticDigest,
		WorkspaceCheckedUnixNano:        now.UnixNano(),
		WorkspaceExpiresUnixNano:        fixture.Workspace.Meta.ExpiresUnixNano,
		WorkspaceLeaseExpiresUnixNano:   fixture.Workspace.Lease.ExpiresUnixNano,
		SemanticNotAfterUnixNano:        semantic.SemanticNotAfterUnixNano,
	}
	current, err := contract.SealInitialWorkspaceReadCommandOwnerCurrentV2(body, commitNow)
	if err != nil {
		t.Fatalf("seal initial current: %v", err)
	}
	return fixture, command, publication, body, current, commitNow
}

func TestAuthorizedCommandPublicationV2InitialRefreshSealAndClock(t *testing.T) {
	fixture, command, publication, body, current, commitNow := authorizedCommandPublicationFixtureV2(t)
	initial, err := NewInitialCommandPublicationV2(
		command, publication, current,
		fixture.Source, fixture.Effect, fixture.Prepared, fixture.Workspace,
		commitNow,
	)
	if err != nil {
		t.Fatalf("new initial capability: %v", err)
	}
	mutation, gotCommand, gotPublication, expected, gotCurrent, err := initial.Open(commitNow)
	if err != nil {
		t.Fatalf("open initial capability: %v", err)
	}
	if mutation != CommandPublicationInitialV2 ||
		gotCommand.Meta.Ref() != command.Meta.Ref() ||
		gotPublication.Meta.Ref() != publication.Meta.Ref() ||
		expected.Meta.ID != "" ||
		gotCurrent.Meta.Ref() != current.Meta.Ref() {
		t.Fatal("initial capability returned another exact closure")
	}

	nextChecked := commitNow.Add(time.Second)
	body.WorkspaceCheckedUnixNano = nextChecked.UnixNano()
	next, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(body, current, nextChecked)
	if err != nil {
		t.Fatalf("seal next current: %v", err)
	}
	refresh, err := NewRefreshCommandPublicationV2(
		command, publication, current, next,
		fixture.Source, fixture.Effect, fixture.Prepared, fixture.Workspace,
		nextChecked,
	)
	if err != nil {
		t.Fatalf("new refresh capability: %v", err)
	}
	mutation, _, _, expected, gotCurrent, err = refresh.Open(nextChecked)
	if err != nil {
		t.Fatalf("open refresh capability: %v", err)
	}
	if mutation != CommandPublicationRefreshV2 ||
		expected.Meta.Ref() != current.Meta.Ref() ||
		gotCurrent.Meta.Revision != current.Meta.Revision+1 {
		t.Fatal("refresh capability did not preserve the exact successor")
	}

	tampered := initial
	tampered.seal = testkit.RuntimeDigest("tampered-owner-capability")
	if _, _, _, _, _, err := tampered.Open(commitNow); err == nil {
		t.Fatal("tampered capability seal must fail closed")
	}
	if _, _, _, _, _, err := initial.Open(commitNow.Add(-time.Nanosecond)); err == nil {
		t.Fatal("capability clock regression must fail closed")
	}
	if _, _, _, _, _, err := initial.Open(time.Unix(0, current.ExpiresUnixNano)); err == nil {
		t.Fatal("capability TTL crossing must fail closed")
	}
}
