package modelinvokeradapter_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/framestore"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/modelinputstore"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/modelinvokeradapter"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestInvocationMaterialContextPairAdapterV2UsesDurableMaterialAndFrameCurrentReaders(t *testing.T) {
	ctx := context.Background()
	frameFixture, err := testfixture.NewFrameStoreFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	frameStore, err := framestore.OpenSQLiteV1(ctx, framestore.SQLiteConfigV1{
		Path: t.TempDir() + "/frame.db", Owner: frameFixture.Owner,
		Clock: func() time.Time { return frameFixture.Now }, MaxTTL: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = frameStore.Close() })
	frameReceipt, err := frameStore.CommitCurrentV1(
		ctx, "commit-context-model-input-pair-frame",
		frameFixture.State, nil, frameFixture.Now.UnixNano(),
	)
	if err != nil {
		t.Fatal(err)
	}

	sourceFixture, err := testfixture.NewModelInputSourceFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	material := sourceFixture.Material.Clone()
	material.Ref.ID = "durable-context-model-input"
	material.Ref.Revision = 1
	material.Ref.Digest = ""
	material.FrameRef = frameReceipt.FrameRef
	material.CheckedUnixNano = frameFixture.Now.Add(-time.Second).UnixNano()
	material.ExpiresUnixNano = frameFixture.Now.Add(15 * time.Second).UnixNano()
	material.Digest = ""
	material, err = contract.SealContextModelInputMaterialV1(material)
	if err != nil {
		t.Fatal(err)
	}
	materialStore, err := modelinputstore.OpenSQLiteV1(ctx, modelinputstore.SQLiteConfigV1{
		Path: t.TempDir() + "/material.db", BusyTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = materialStore.Close() })
	if _, err := materialStore.CommitV1(ctx, material, nil, frameFixture.Now.UnixNano()); err != nil {
		t.Fatal(err)
	}

	source, err := kernel.NewContextModelInputSourceCurrentReaderV1(
		frameFixture.Owner, materialStore, materialStore, frameStore,
		func() time.Time { return frameFixture.Now }, 10*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := modelinvokeradapter.NewInvocationMaterialContextPairAdapterV2(
		frameFixture.Owner, source, func() time.Time { return frameFixture.Now }, 10*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	contextOwner := modelinvoker.ContextOwnerRef{
		ComponentID:   frameFixture.Owner.ComponentID,
		BindingDigest: core.Digest(frameFixture.Owner.BindingDigest),
	}
	_, neutral, err := modelinvoker.MapContextOwnerRefToNeutralOwnerV1(contextOwner)
	if err != nil {
		t.Fatal(err)
	}
	frame := modelinvoker.InvocationMaterialExactSourceRefV1{
		Owner: neutral, Kind: modelinvoker.InvocationMaterialContextFrameKindV2,
		ID: frameReceipt.FrameRef.ID, Revision: core.Revision(frameReceipt.FrameRef.Revision),
		Digest: core.Digest(frameReceipt.FrameRef.Digest),
	}
	materialSource := modelinvoker.InvocationMaterialExactSourceRefV1{
		Owner: neutral, Kind: modelinvoker.InvocationMaterialContextMaterialKindV2,
		ID: material.Ref.ID, Revision: core.Revision(material.Ref.Revision),
		Digest: core.Digest(material.Ref.Digest),
	}
	expected, err := modelinvoker.DigestGovernedModelTurnContextBodyV2(
		[]modelinvoker.Instruction{
			{Role: modelinvoker.RoleSystem, Text: "follow the exact workspace policy"},
			{Role: modelinvoker.RoleDeveloper, Text: "return bounded evidence"},
		},
		[]modelinvoker.InputItem{
			modelinvoker.MessageInput(modelinvoker.RoleUser, "inspect README.md"),
			modelinvoker.FunctionCallInput(
				"call-workspace-read-1",
				"workspace.read",
				json.RawMessage(`{"path":"README.md"}`),
			),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := adapter.InspectExactInvocationContextPairV2(
		ctx, frame, materialSource, expected,
	)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ContextFrame != frame ||
		projection.ContextMaterial != materialSource ||
		projection.ContextMappedInputDigest != expected ||
		projection.ExpiresUnixNano != frameFixture.Now.Add(10*time.Second).UnixNano() {
		t.Fatalf("durable exact-current projection drifted: %+v", projection)
	}
}
