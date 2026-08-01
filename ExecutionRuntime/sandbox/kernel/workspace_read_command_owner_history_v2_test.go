package kernel

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestValidateWorkspaceReadCommandOwnerHistoryV2RejectsNonTailPointerAndGaps(t *testing.T) {
	now := time.Now().UTC()
	fixture := testkit.WorkspaceReadCommandPublicationV2(now, "kernel-history")
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
	body := workspaceReadCommandOwnerHistoryBodyV2(command, publication, fixture, now)
	first, err := contract.SealInitialWorkspaceReadCommandOwnerCurrentV2(body, now)
	if err != nil {
		t.Fatal(err)
	}
	secondChecked := now.Add(time.Second)
	body.CheckedUnixNano = secondChecked.UnixNano()
	second, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(body, first, secondChecked)
	if err != nil {
		t.Fatal(err)
	}

	if err = validateWorkspaceReadCommandOwnerHistoryV2(
		command,
		publication,
		[]contract.WorkspaceReadCommandOwnerCurrentV2{first, second},
		second,
	); err != nil {
		t.Fatalf("valid history rejected: %v", err)
	}
	tests := []struct {
		name    string
		history []contract.WorkspaceReadCommandOwnerCurrentV2
		pointer contract.WorkspaceReadCommandOwnerCurrentV2
	}{
		{name: "future orphan", history: []contract.WorkspaceReadCommandOwnerCurrentV2{first, second}, pointer: first},
		{name: "missing predecessor", history: []contract.WorkspaceReadCommandOwnerCurrentV2{second}, pointer: second},
		{name: "pointer rollback", history: []contract.WorkspaceReadCommandOwnerCurrentV2{first, second}, pointer: first},
		{name: "history missing tail", history: []contract.WorkspaceReadCommandOwnerCurrentV2{first}, pointer: second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateWorkspaceReadCommandOwnerHistoryV2(
				command,
				publication,
				test.history,
				test.pointer,
			); !errors.Is(err, ports.ErrConflict) {
				t.Fatalf("history error=%v, want Conflict", err)
			}
		})
	}
}

func workspaceReadCommandOwnerHistoryBodyV2(
	command contract.WorkspaceReadCommandV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	fixture testkit.WorkspaceReadCommandPublicationFixtureV2,
	checked time.Time,
) contract.WorkspaceReadCommandOwnerCurrentV2 {
	return contract.WorkspaceReadCommandOwnerCurrentV2{
		Command:                         command.Meta.Ref(),
		Publication:                     publication.Meta.Ref(),
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
		WorkspaceCheckedUnixNano:        checked.UnixNano(),
		WorkspaceExpiresUnixNano:        fixture.Workspace.Meta.ExpiresUnixNano,
		WorkspaceLeaseExpiresUnixNano:   fixture.Workspace.Lease.ExpiresUnixNano,
		SemanticNotAfterUnixNano:        publication.Semantic.SemanticNotAfterUnixNano,
	}
}
