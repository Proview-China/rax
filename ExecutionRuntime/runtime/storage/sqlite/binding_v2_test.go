package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestBindingSetTransactionStagedFailureLeaksNothing(t *testing.T) {
	base := time.Unix(2_310_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return base.Add(2 * time.Second) })
	certified, set := certifiedBinding(t, store, base, "set-stage", "binding-stage", "review/stage", "review/attest")
	store.failNextStageForTest()
	if _, err := store.CommitBindingSet(context.Background(), control.CommitBindingSetRequestV2{Set: set, Expected: []control.ExpectedBindingRevisionV2{{BindingID: certified.ID, ExpectedRevision: certified.Revision}}}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("injected staged failure was not Unavailable: %v", err)
	}
	current, err := store.InspectBinding(context.Background(), certified.ID)
	if err != nil || current.State != control.BindingCertified || current.Revision != certified.Revision {
		t.Fatalf("staged failure leaked member write: %+v %v", current, err)
	}
	if _, err := store.InspectBindingSet(context.Background(), set.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked BindingSet: %v", err)
	}
}

func TestBindingSetLostReplyRecoversByExactInspect(t *testing.T) {
	base := time.Unix(2_320_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return base.Add(2 * time.Second) })
	certified, set := certifiedBinding(t, store, base, "set-lost", "binding-lost", "review/lost", "review/attest")
	store.loseNextReplyForTest()
	if _, err := store.CommitBindingSet(context.Background(), control.CommitBindingSetRequestV2{Set: set, Expected: []control.ExpectedBindingRevisionV2{{BindingID: certified.ID, ExpectedRevision: certified.Revision}}}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost commit reply was not Indeterminate: %v", err)
	}
	committed, err := store.InspectBindingSet(context.Background(), set.ID)
	if err != nil || committed.ID != set.ID || committed.Members[0].BindingRevision != certified.Revision+1 {
		t.Fatalf("exact Inspect did not recover committed BindingSet: %+v %v", committed, err)
	}
}

func TestBindingFactCASStagedFailureAndLostReplyExactInspect(t *testing.T) {
	for _, lostReply := range []bool{false, true} {
		name := "stage"
		if lostReply {
			name = "lost-reply"
		}
		t.Run(name, func(t *testing.T) {
			base := time.Unix(2_330_000_000, 0)
			store := openTestStore(t, testDBPath(t), func() time.Time { return base.Add(2 * time.Second) })
			certified, _ := certifiedBinding(t, store, base, "set-fact", "binding-fact", "review/fact", "review/attest")
			revoked := certified
			revoked.Revision++
			revoked.State = control.BindingRevoked
			revoked.InvalidationReason = core.ReasonBindingDrift
			if lostReply {
				store.loseNextReplyForTest()
			} else {
				store.failNextStageForTest()
			}
			_, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: certified.Revision, Next: revoked})
			if lostReply {
				if !core.HasCategory(err, core.ErrorIndeterminate) {
					t.Fatalf("lost Binding Fact CAS reply was not Indeterminate: %v", err)
				}
				current, inspectErr := store.InspectBinding(context.Background(), certified.ID)
				if inspectErr != nil || current.Revision != revoked.Revision || current.State != control.BindingRevoked {
					t.Fatalf("exact Inspect did not recover Binding Fact CAS: %+v %v", current, inspectErr)
				}
				return
			}
			if !core.HasCategory(err, core.ErrorUnavailable) {
				t.Fatalf("staged Binding Fact CAS failure was not Unavailable: %v", err)
			}
			current, inspectErr := store.InspectBinding(context.Background(), certified.ID)
			if inspectErr != nil || current.Revision != certified.Revision || current.State != control.BindingCertified {
				t.Fatalf("staged Binding Fact failure leaked revision: %+v %v", current, inspectErr)
			}
		})
	}
}
