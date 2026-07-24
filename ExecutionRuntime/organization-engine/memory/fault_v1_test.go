package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestLostPublishReplyRecoversOnlyByExactInspectV1(t *testing.T) {
	store := memory.NewStore()
	now := time.Unix(1800000000, 0)
	v, err := contract.SealIdentityV1(contract.IdentityFactV1{FactMetaV1: contract.FactMetaV1{TenantID: "tenant-a", Revision: 1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(), State: contract.FactActiveV1}, SubjectKind: contract.SubjectHumanV1, SubjectID: "reviewer", DisplayHandle: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.PublishIdentityV1(context.Background(), nil, v); err != nil {
		t.Fatal(err)
	}
	lost := ports.IndeterminateV1("commit reply lost")
	if !core.HasCategory(lost, core.ErrorIndeterminate) {
		t.Fatal(lost)
	}
	got, err := store.InspectIdentityV1(context.WithoutCancel(context.Background()), v.ExactRef())
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest != v.Digest {
		t.Fatal("exact recovery drifted")
	}
}

func TestStagedRevisionFailureLeaksNoHistoryV1(t *testing.T) {
	store := memory.NewStore()
	now := time.Unix(1800000000, 0)
	first, err := contract.SealIdentityV1(contract.IdentityFactV1{FactMetaV1: contract.FactMetaV1{TenantID: "tenant-a", Revision: 1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(), State: contract.FactActiveV1}, SubjectKind: contract.SubjectHumanV1, SubjectID: "reviewer", DisplayHandle: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.PublishIdentityV1(context.Background(), nil, first); err != nil {
		t.Fatal(err)
	}
	gap := first
	gap.Revision = 3
	gap.UpdatedUnixNano = now.Add(time.Second).UnixNano()
	gap.DisplayHandle = "gap"
	gap, err = contract.SealIdentityV1(gap)
	if err != nil {
		t.Fatal(err)
	}
	expected := first.ExactRef()
	if err = store.PublishIdentityV1(context.Background(), &expected, gap); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("gap=%v", err)
	}
	if _, err = store.InspectIdentityV1(context.Background(), gap.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged history leaked: %v", err)
	}
	if _, err = store.InspectIdentityV1(context.Background(), first.ExactRef()); err != nil {
		t.Fatal(err)
	}
}
