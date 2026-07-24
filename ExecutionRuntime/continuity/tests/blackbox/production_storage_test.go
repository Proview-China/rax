//go:build cgo && continuity_rocksdb

package blackbox_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	continuityrocks "github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/rocksdb"
	continuitysqlite "github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/sqlite"
)

func TestProductionStoresRecoverEveryDurableJournalCutAfterReopen(t *testing.T) {
	for _, cut := range []contract.JournalState{
		contract.JournalMetadataPending,
		contract.JournalContentStaged,
		contract.JournalReferenceCommitted,
		contract.JournalVisible,
	} {
		cut := cut
		t.Run(string(cut), func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			sqlitePath := filepath.Join(root, "continuity.db")
			rocksPath := filepath.Join(root, "content")
			metadata, err := continuitysqlite.Open(ctx, sqlitePath)
			if err != nil {
				t.Fatal(err)
			}
			content, err := continuityrocks.Open(rocksPath)
			if err != nil {
				t.Fatal(err)
			}
			clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)}
			fired := false
			manager, _ := domain.NewContentManager(metadata, content, clock, 4, func(state contract.JournalState, _ contract.WriteJournal) error {
				if state == cut && !fired {
					fired = true
					return errors.New("simulated process loss after durable cut")
				}
				return nil
			})
			data := []byte("cross-store-production-recovery")
			manifest, journal, err := manager.Put(ctx, domain.PutObjectRequest{
				JournalID: "journal-1", ObjectID: "object-1", SchemaVersion: "content/v1",
				Classification: "sensitive", OwnerID: "continuity", ScopeDigest: "scope-1",
				RetentionPolicyRef: "retention-1", Compression: "identity",
				EncryptionRef: "envelope-1", Data: data,
			})
			if !contract.HasCode(err, contract.ErrCrossStoreIndeterminate) || journal.State != cut {
				t.Fatalf("cut=%s journal=%#v err=%v", cut, journal, err)
			}
			metadata.Close()
			content.Close()

			metadata, err = continuitysqlite.Open(ctx, sqlitePath)
			if err != nil {
				t.Fatal(err)
			}
			defer metadata.Close()
			content, err = continuityrocks.Open(rocksPath)
			if err != nil {
				t.Fatal(err)
			}
			defer content.Close()
			manager, _ = domain.NewContentManager(metadata, content, clock, 4, nil)
			recovered, err := manager.Recover(ctx, journal.JournalID, data)
			if err != nil || recovered.State != contract.JournalClosed || recovered.JournalID != journal.JournalID {
				t.Fatalf("cut=%s recovered=%#v err=%v", cut, recovered, err)
			}
			got, storedManifest, err := manager.Read(ctx, manifest.ObjectID)
			if err != nil || string(got) != string(data) || storedManifest.Digest != manifest.Digest {
				t.Fatalf("cut=%s got=%q manifest=%#v err=%v", cut, got, storedManifest, err)
			}
		})
	}
}
