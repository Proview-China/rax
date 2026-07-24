package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	applicationsqlite "github.com/Proview-China/rax/ExecutionRuntime/application/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestRestoreStageAndExecutionResultsSQLiteV1LostReplyRestartReplay(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "application.db")
	stageFixture := newRestoreStageActionGatewayFixtureV1(t)
	store, err := applicationsqlite.OpenV1(ctx, applicationsqlite.ConfigV1{Path: path, Clock: func() time.Time { return stageFixture.now }})
	if err != nil {
		t.Fatal(err)
	}
	stageFixture.gateway, err = NewRestoreStageActionGatewayV1(RestoreStageActionGatewayConfigV1{Results: store, Authorization: stageFixture.authorization, Participant: stageFixture.participant, Enforcement: stageFixture.enforcement, Governance: stageFixture.governance, Evidence: stageFixture.evidence, Settlements: stageFixture.settlements, Clock: func() time.Time { return stageFixture.now }})
	if err != nil {
		t.Fatal(err)
	}
	store.LoseNextRestoreStageResultReplyV1()
	stageResult, err := stageFixture.gateway.ExecuteRestoreStageActionV1(ctx, stageFixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := applicationsqlite.OpenV1(ctx, applicationsqlite.ConfigV1{Path: path, Clock: func() time.Time { return stageFixture.now }})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replayGateway, err := NewRestoreStageActionGatewayV1(RestoreStageActionGatewayConfigV1{Results: reopened, Authorization: stageFixture.authorization, Participant: stageFixture.participant, Enforcement: stageFixture.enforcement, Governance: stageFixture.governance, Evidence: stageFixture.evidence, Settlements: stageFixture.settlements, Clock: func() time.Time { return stageFixture.now }})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := replayGateway.ExecuteRestoreStageActionV1(ctx, stageFixture.request)
	if err != nil || replay.Digest != stageResult.Digest || stageFixture.participant.executeCalls != 1 {
		t.Fatalf("Stage replay=%+v err=%v execute=%d", replay, err, stageFixture.participant.executeCalls)
	}

	executionFixture := newRestoreExecutionFixtureV1(t)
	executionPath := filepath.Join(t.TempDir(), "application-execution.db")
	executionStore, err := applicationsqlite.OpenV1(ctx, applicationsqlite.ConfigV1{Path: executionPath, Clock: func() time.Time { return executionFixture.now }})
	if err != nil {
		t.Fatal(err)
	}
	executionFixture.coordinator, err = NewRestoreExecutionCoordinatorV1(RestoreExecutionCoordinatorConfigV1{Intents: executionStore, Results: executionStore, Restore: executionFixture.restore, Materialization: executionFixture.materialization, Stage: executionFixture.stage, Context: executionFixture.context, Activation: executionFixture.activation, Clock: func() time.Time { return executionFixture.now }})
	if err != nil {
		t.Fatal(err)
	}
	executionStore.LoseNextRestoreExecutionResultReplyV1()
	executionResult, err := executionFixture.coordinator.ExecuteRestoreV1(ctx, executionFixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if err := executionStore.Close(); err != nil {
		t.Fatal(err)
	}
	executionReopened, err := applicationsqlite.OpenV1(ctx, applicationsqlite.ConfigV1{Path: executionPath, Clock: func() time.Time { return executionFixture.now }})
	if err != nil {
		t.Fatal(err)
	}
	defer executionReopened.Close()
	executionReplay, err := NewRestoreExecutionCoordinatorV1(RestoreExecutionCoordinatorConfigV1{Intents: executionReopened, Results: executionReopened, Restore: executionFixture.restore, Materialization: executionFixture.materialization, Stage: executionFixture.stage, Context: executionFixture.context, Activation: executionFixture.activation, Clock: func() time.Time { return executionFixture.now }})
	if err != nil {
		t.Fatal(err)
	}
	replayedExecution, err := executionReplay.ExecuteRestoreV1(ctx, executionFixture.request)
	if err != nil || replayedExecution.Digest != executionResult.Digest || executionFixture.stage.providerCalls != 1 || executionFixture.activation.calls != 1 {
		t.Fatalf("execution replay=%+v err=%v stage=%d activation=%d", replayedExecution, err, executionFixture.stage.providerCalls, executionFixture.activation.calls)
	}
	intent, err := executionReopened.InspectRestoreExecutionIntentV1(ctx, core.TenantID(executionFixture.request.RestorePlan.TenantID), executionFixture.request.ID)
	if err != nil || intent.RequestDigest != executionFixture.request.Digest || intent.ValidateCurrent(executionFixture.now) != nil {
		t.Fatalf("durable Restore Intent after reopen=%+v err=%v", intent, err)
	}
}
