package kernel

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceRestoreSettlementOwnerV1OpaqueLostReplyAndConcurrent(t *testing.T) {
	stageFixture := newWorkspaceRestoreOwnerFixtureV1(t, false)
	mustPrepareWorkspaceRestoreV1(t, stageFixture.owner, stageFixture.request)
	stage, err := stageFixture.owner.StageWorkspaceV1(context.Background(), &stageFixture.request)
	if err != nil {
		t.Fatal(err)
	}
	store := testkit.NewWorkspaceRestoreSettlementMemoryStoreV1()
	store.LoseNextCreateReplyV1()
	owner, err := NewWorkspaceRestoreSettlementOwnerV1(stageFixture.store, store, func() time.Time { return stageFixture.now })
	if err != nil {
		t.Fatal(err)
	}
	settlement := workspaceRestoreOpaqueSettlementFixtureV1(stage)
	applied, err := owner.ApplyWorkspaceRestoreSettlementV1(context.Background(), settlement)
	if err != nil || applied.RuntimeSettlement != settlement || applied.StageFactRef != stage.ExactRef() {
		t.Fatalf("lost reply ApplySettlement=%+v err=%v", applied, err)
	}

	var wg sync.WaitGroup
	errorsC := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, applyErr := owner.ApplyWorkspaceRestoreSettlementV1(context.Background(), settlement)
			if applyErr != nil || got != applied {
				errorsC <- applyErr
			}
		}()
	}
	wg.Wait()
	close(errorsC)
	for applyErr := range errorsC {
		t.Fatalf("concurrent exact ApplySettlement drifted: %v", applyErr)
	}
	current, err := owner.InspectWorkspaceRestoreApplySettlementByStageV1(context.Background(), stage.TenantID, stage.ExactRef())
	if err != nil || current != applied {
		t.Fatalf("current ApplySettlement=%+v err=%v", current, err)
	}
}

func TestWorkspaceRestoreSettlementOwnerV1RejectsCrossFactAndSecondSettlement(t *testing.T) {
	stageFixture := newWorkspaceRestoreOwnerFixtureV1(t, true)
	mustPrepareWorkspaceRestoreV1(t, stageFixture.owner, stageFixture.request)
	stage, err := stageFixture.owner.StageWorkspaceV1(context.Background(), &stageFixture.request)
	if err != nil {
		t.Fatal(err)
	}
	store := testkit.NewWorkspaceRestoreSettlementMemoryStoreV1()
	owner, err := NewWorkspaceRestoreSettlementOwnerV1(stageFixture.store, store, func() time.Time { return stageFixture.now })
	if err != nil {
		t.Fatal(err)
	}
	settlement := workspaceRestoreOpaqueSettlementFixtureV1(stage)
	if _, err := owner.ApplyWorkspaceRestoreSettlementV1(context.Background(), settlement); err != nil {
		t.Fatal(err)
	}
	other := settlement
	other.ID = "different-runtime-settlement"
	other.Digest = strings.Repeat("e", contract.DigestSizeHex)
	if _, err := owner.ApplyWorkspaceRestoreSettlementV1(context.Background(), other); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("second Settlement for same Stage Fact was accepted: %v", err)
	}
	splice := settlement
	splice.DomainResult.ID = "other-stage-fact"
	if _, err := owner.ApplyWorkspaceRestoreSettlementV1(context.Background(), splice); err == nil {
		t.Fatal("cross-Fact opaque Settlement was accepted")
	}
}

func TestRuntimeRestoreStageSettlementRefV1HasNoRuntimeSemantics(t *testing.T) {
	typeOf := reflect.TypeOf(contract.RuntimeRestoreStageSettlementRefV1{})
	if typeOf.NumField() != 6 {
		t.Fatalf("opaque Runtime Settlement ref fields=%d want=6", typeOf.NumField())
	}
	for _, forbidden := range []string{"Outcome", "Disposition", "Status", "Success", "Unknown", "Activate", "Rollback", "Root"} {
		if _, ok := typeOf.FieldByName(forbidden); ok {
			t.Fatalf("opaque Runtime Settlement ref exposes %s", forbidden)
		}
	}
}

func workspaceRestoreOpaqueSettlementFixtureV1(stage contract.WorkspaceRestoreStageFactV1) contract.RuntimeRestoreStageSettlementRefV1 {
	return contract.RuntimeRestoreStageSettlementRefV1{ID: "runtime-restore-stage-settlement", Revision: 1, Digest: strings.Repeat("a", contract.DigestSizeHex), OperationDigest: strings.Repeat("b", contract.DigestSizeHex), EffectID: "restore-effect", DomainResult: stage.ExactRef()}
}
