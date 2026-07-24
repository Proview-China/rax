package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestAgentActivationV2SQLiteCreateRestartAndVersionConflict(t *testing.T) {
	now := time.Unix(1_900_100_000, 0)
	path := filepath.Join(t.TempDir(), "application.db")
	store, err := OpenV1(context.Background(), ConfigV1{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	fact := sqliteActivationInitialFactV2(t, now)
	if _, err = store.CreateAgentActivationCoordinationV2(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenV1(context.Background(), ConfigV1{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.InspectAgentActivationCoordinationV2(context.Background(), fact.ActivationID)
	if err != nil || got.Digest != fact.Digest {
		t.Fatalf("restart inspect failed: %v", err)
	}
	legacy := sqliteActivationInitialFactV1(t, now, fact.ActivationID)
	if _, err = store.EnsureAgentActivationCoordinationV1(context.Background(), legacy); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("V2 claim did not reject V1: %v", err)
	}
}

func TestAgentActivationV2SQLiteV1ClaimRejectsV2(t *testing.T) {
	now := time.Unix(1_900_100_000, 0)
	store, err := OpenV1(context.Background(), ConfigV1{Path: filepath.Join(t.TempDir(), "application.db"), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	v2 := sqliteActivationInitialFactV2(t, now)
	v1 := sqliteActivationInitialFactV1(t, now, v2.ActivationID)
	if _, err = store.EnsureAgentActivationCoordinationV1(context.Background(), v1); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateAgentActivationCoordinationV2(context.Background(), v2); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("V1 claim did not reject V2: %v", err)
	}
}

func TestAgentActivationV2SQLiteLostCreateReplyRecoversExactFact(t *testing.T) {
	now := time.Unix(1_900_100_000, 0)
	store, err := OpenV1(context.Background(), ConfigV1{Path: filepath.Join(t.TempDir(), "application.db"), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fact := sqliteActivationInitialFactV2(t, now)
	store.LoseNextAgentActivationCoordinationCreateReplyV2()
	if _, err = store.CreateAgentActivationCoordinationV2(context.Background(), fact); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("expected lost reply, got %v", err)
	}
	got, err := store.InspectAgentActivationCoordinationV2(context.Background(), fact.ActivationID)
	if err != nil || got.Digest != fact.Digest {
		t.Fatalf("lost create recovery failed: %v", err)
	}
}

func sqliteActivationInitialFactV2(t *testing.T, now time.Time) contract.AgentActivationCoordinationFactV2 {
	t.Helper()
	expires := now.Add(time.Hour).UnixNano()
	ref := func(id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "praxis.sqlite.test", ID: core.OwnerID("owner-" + id)}, ContractVersion: "praxis.test/current/v1", ID: fmt.Sprintf("%s-current", id), Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires}
	}
	request, err := contract.SealAgentActivationStartRequestV2(contract.AgentActivationStartRequestV2{ActivationID: "sqlite-activation-v2", IdempotencyKey: "sqlite-activation-key", ProposedScope: contract.ProposedActivationScopeV2{Identity: core.AgentIdentityRef{TenantID: "tenant", ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: core.DigestBytes([]byte("plan"))}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, AuthorityEpoch: 1}, DefinitionCurrent: ref("definition"), PlanCurrent: ref("plan"), AssemblyCurrent: ref("assembly"), BindingSetCurrent: ref("binding"), AuthorityCurrent: ref("authority"), PolicyCurrent: ref("policy"), RequirementDigest: core.DigestBytes([]byte("requirements")), ProbeBudget: 8, RequestedNotAfterUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	base, err := contract.AgentActivationStepIntentBaseDigestV2(request, contract.AgentActivationPreflightV2, nil)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := contract.DeriveAgentActivationStepAttemptIDV2(request.ActivationID, request.RequestDigest, contract.AgentActivationPreflightV2)
	if err != nil {
		t.Fatal(err)
	}
	event, err := contract.SealAgentActivationStepEventV2(contract.AgentActivationStepEventV2{Sequence: 1, Step: contract.AgentActivationPreflightV2, State: contract.AgentActivationStepIntentRecordedV2, AttemptID: attempt, RequestDigest: base, RecordedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := contract.NewAgentActivationCoordinationFactV2(request, event, now)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func sqliteActivationInitialFactV1(t *testing.T, now time.Time, activationID string) contract.AgentActivationCoordinationFactV1 {
	t.Helper()
	expires := now.Add(time.Hour).UnixNano()
	ref := func(id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "praxis.sqlite.legacy", ID: core.OwnerID("owner-" + id)}, ContractVersion: "praxis.test/current/v1", ID: id + "-current", Revision: 1, Digest: core.DigestBytes([]byte("legacy-" + id)), ExpiresUnixNano: expires}
	}
	start, err := contract.SealAgentActivationStartRequestV1(contract.AgentActivationStartRequestV1{ActivationID: activationID, AttemptID: "legacy-attempt", IdempotencyKey: "legacy-key", DefinitionCurrent: ref("definition"), PlanCurrent: ref("plan"), AssemblyCurrent: ref("assembly"), BindingSetCurrent: ref("binding"), AuthorityCurrent: ref("authority"), PolicyCurrent: ref("policy"), BudgetCurrent: ref("budget"), CredentialCurrent: ref("credential"), SandboxAdapterBinding: ref("sandbox"), ExecutionAdapterBinding: ref("execution"), RequestedNotAfterUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	stepRequest, err := contract.SealAgentActivationStepRequestV1(contract.AgentActivationStepRequestV1{ActivationID: activationID, StartRequestDigest: start.RequestDigest, Step: contract.AgentActivationPreflightV1, RequestedNotAfterUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	event, err := contract.SealAgentActivationStepEventV1(contract.AgentActivationStepEventV1{Sequence: 1, Step: contract.AgentActivationPreflightV1, State: contract.AgentActivationStepIntentRecordedV1, AttemptID: stepRequest.AttemptID, RequestDigest: stepRequest.RequestDigest, RecordedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := contract.SealAgentActivationCoordinationFactV1(contract.AgentActivationCoordinationFactV1{ActivationID: activationID, Revision: 1, Request: start, Events: []contract.AgentActivationStepEventV1{event}})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}
