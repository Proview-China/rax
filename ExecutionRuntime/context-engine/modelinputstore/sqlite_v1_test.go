package modelinputstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

const modelInputStoreNowV1 = int64(1_900_000_000_000_000_000)

func storeDigestV1(value string) contract.Digest {
	return contract.DigestBytes([]byte(value))
}

func materialFixtureV1(t *testing.T, revision uint64, callID, name, output string) contract.ContextModelInputMaterialV1 {
	t.Helper()
	content := []byte(output)
	fragmentRef := contract.FactRef{ID: "tool-result-fragment", Revision: revision, Digest: storeDigestV1("fragment-" + callID)}
	binding, err := contract.SealContextModelInputSegmentBindingV1(contract.ContextModelInputSegmentBindingV1{
		FragmentRef: fragmentRef, Region: contract.RegionDynamicTail, Position: 1, Kind: contract.FragmentToolResult,
		Trust: contract.TrustObservation, Channel: contract.ContextModelInputFunctionResultV1, Role: contract.ContextModelInputRoleToolV1,
		Encoding: contract.ContextModelInputUTF8V1, CallID: callID, Name: name,
	})
	if err != nil {
		t.Fatal(err)
	}
	material, err := contract.SealContextModelInputMaterialV1(contract.ContextModelInputMaterialV1{
		Ref:                          contract.ContextModelInputMaterialRefV1{ID: "model-input-material-history", Revision: revision},
		DescriptorRef:                contract.ContextFrameConsumptionDescriptorRefV1{ID: "descriptor", Revision: 1, Digest: storeDigestV1("descriptor")},
		FrameRef:                     contract.FactRef{ID: "frame", Revision: 1, Digest: storeDigestV1("frame")},
		ManifestRef:                  contract.FactRef{ID: "manifest", Revision: 1, Digest: storeDigestV1("manifest")},
		GenerationRef:                contract.FactRef{ID: "generation", Revision: 1, Digest: storeDigestV1("generation")},
		MaterializedDescriptorDigest: storeDigestV1("materialized-descriptor"),
		OrderedSegments: []contract.ContextModelInputSegmentV1{{
			FragmentRef: fragmentRef, Region: contract.RegionDynamicTail, Position: 1, Kind: contract.FragmentToolResult,
			Trust: contract.TrustObservation, Channel: contract.ContextModelInputFunctionResultV1, Role: contract.ContextModelInputRoleToolV1,
			Encoding: contract.ContextModelInputUTF8V1, CallID: callID, Name: name,
			ContentRef: contract.ContentRef{Ref: "content-" + callID, Digest: contract.DigestBytes(content), Length: uint64(len(content))},
			Content:    content, SemanticBindingDigest: binding.Digest,
		}},
		CheckedUnixNano: modelInputStoreNowV1, ExpiresUnixNano: modelInputStoreNowV1 + 1_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	return material
}

func openStoreV1(t *testing.T, path string) *SQLiteV1 {
	t.Helper()
	store, err := OpenSQLiteV1(context.Background(), SQLiteConfigV1{Path: path, BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSQLiteRestartExactFunctionResultHistoryAndCurrentV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-input.db")
	first := materialFixtureV1(t, 1, "call-workspace-read-1", "workspace.read", "first output")
	second := materialFixtureV1(t, 2, "call-workspace-write-2", "workspace.write", "second output")
	store := openStoreV1(t, path)
	if receipt, err := store.CommitV1(context.Background(), first, nil, modelInputStoreNowV1+1); err != nil || !receipt.Created {
		t.Fatalf("first receipt=%+v err=%v", receipt, err)
	}
	if receipt, err := store.CommitV1(context.Background(), second, &first.Ref, modelInputStoreNowV1+1); err != nil || !receipt.Created {
		t.Fatalf("second receipt=%+v err=%v", receipt, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openStoreV1(t, path)
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
	historical, err := reopened.ReadContextModelInputMaterialExactV1(context.Background(), first.Ref, modelInputStoreNowV1+2)
	if err != nil || historical.OrderedSegments[0].CallID != "call-workspace-read-1" || historical.OrderedSegments[0].Name != "workspace.read" {
		t.Fatalf("historical=%+v err=%v", historical, err)
	}
	current, err := reopened.ReadContextModelInputMaterialCurrentV1(context.Background(), second.Ref.ID, modelInputStoreNowV1+2)
	if err != nil || current.Ref != second.Ref || current.OrderedSegments[0].CallID != "call-workspace-write-2" || current.OrderedSegments[0].Name != "workspace.write" {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	historical.OrderedSegments[0].CallID = "alias"
	again, err := reopened.ReadContextModelInputMaterialExactV1(context.Background(), first.Ref, modelInputStoreNowV1+2)
	if err != nil || again.OrderedSegments[0].CallID != "call-workspace-read-1" {
		t.Fatalf("exact reader leaked mutable state: %+v err=%v", again, err)
	}
}

func TestSQLiteMissingOrTamperedFunctionResultFailsClosedV1(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*contract.ContextModelInputMaterialV1)
	}{
		{"missing_call_id", func(m *contract.ContextModelInputMaterialV1) { m.OrderedSegments[0].CallID = "" }},
		{"missing_name", func(m *contract.ContextModelInputMaterialV1) { m.OrderedSegments[0].Name = "" }},
		{"tampered_name", func(m *contract.ContextModelInputMaterialV1) { m.OrderedSegments[0].Name = "workspace.delete" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "model-input.db")
			material := materialFixtureV1(t, 1, "call-workspace-read-1", "workspace.read", "output")
			store := openStoreV1(t, path)
			t.Cleanup(func() { _ = store.Close() })
			if _, err := store.CommitV1(context.Background(), material, nil, modelInputStoreNowV1+1); err != nil {
				t.Fatal(err)
			}
			corrupt := material.Clone()
			tc.mutate(&corrupt)
			payload, rowDigest, err := encodeRowV1(corrupt)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.db.Exec(`UPDATE context_model_input_material_history SET row_digest=?,payload=? WHERE material_id=? AND revision=?`, string(rowDigest), payload, material.Ref.ID, material.Ref.Revision); err != nil {
				t.Fatal(err)
			}
			if got, err := store.ReadContextModelInputMaterialExactV1(context.Background(), material.Ref, modelInputStoreNowV1+2); !errors.Is(err, contract.ErrConflict) || got.Ref.ID != "" {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}

func TestSQLiteExactRevisionDigestTTLAndCurrentDriftFailClosedV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-input.db")
	material := materialFixtureV1(t, 1, "call-workspace-read-1", "workspace.read", "output")
	store := openStoreV1(t, path)
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.CommitV1(context.Background(), material, nil, modelInputStoreNowV1+1); err != nil {
		t.Fatal(err)
	}
	wrongDigest := material.Ref
	wrongDigest.Digest = storeDigestV1("wrong")
	if _, err := store.ReadContextModelInputMaterialExactV1(context.Background(), wrongDigest, modelInputStoreNowV1+2); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("wrong digest error=%v", err)
	}
	missingRevision := material.Ref
	missingRevision.Revision++
	if _, err := store.ReadContextModelInputMaterialExactV1(context.Background(), missingRevision, modelInputStoreNowV1+2); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("missing revision error=%v", err)
	}
	if _, err := store.ReadContextModelInputMaterialExactV1(context.Background(), material.Ref, material.ExpiresUnixNano); !errors.Is(err, contract.ErrExpired) {
		t.Fatalf("now==expires error=%v", err)
	}
	if _, err := store.db.Exec(`UPDATE context_model_input_material_current SET highest_revision=2 WHERE material_id=?`, material.Ref.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadContextModelInputMaterialCurrentV1(context.Background(), material.Ref.ID, modelInputStoreNowV1+2); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("current drift error=%v", err)
	}
}
