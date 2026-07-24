package fault_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestEveryPersistedJournalCutRecoversByInspectingOriginalIdentity(t *testing.T) {
	for _, cut := range []contract.JournalState{
		contract.JournalMetadataPending,
		contract.JournalContentStaged,
		contract.JournalReferenceCommitted,
		contract.JournalVisible,
	} {
		t.Run(string(cut), func(t *testing.T) {
			ctx := context.Background()
			backend := memory.New()
			clock := &testkit.Clock{Time: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
			fired := false
			manager, _ := domain.NewContentManager(backend, backend, clock, 3, func(state contract.JournalState, _ contract.WriteJournal) error {
				if state == cut && !fired {
					fired = true
					return errors.New("simulated lost response")
				}
				return nil
			})
			data := []byte("fault-injection-payload")
			_, journal, err := manager.Put(ctx, domain.PutObjectRequest{
				JournalID: "journal-1", ObjectID: "object-1", SchemaVersion: "content/v1",
				Classification: "internal", OwnerID: "continuity", ScopeDigest: "scope-1",
				RetentionPolicyRef: "retention-1", Compression: "identity", Data: data,
			})
			if !contract.HasCode(err, contract.ErrCrossStoreIndeterminate) || journal.State != cut {
				t.Fatalf("cut=%s journal=%#v err=%v", cut, journal, err)
			}
			recovered, err := manager.Recover(ctx, journal.JournalID, data)
			if err != nil || recovered.State != contract.JournalClosed || recovered.JournalID != "journal-1" || recovered.ObjectID != "object-1" || recovered.LastInspectionRef == "" {
				t.Fatalf("recovered=%#v err=%v", recovered, err)
			}
		})
	}
}
