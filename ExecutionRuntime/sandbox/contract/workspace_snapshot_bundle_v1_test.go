package contract

import (
	"bytes"
	"strings"
	"testing"
)

func TestWorkspaceSnapshotBundleV1CanonicalRoundTripAndClone(t *testing.T) {
	original := WorkspaceSnapshotBundleV1{
		SnapshotID:        "snapshot-1",
		TenantID:          "tenant-1",
		SourceScopeDigest: strings.Repeat("a", DigestSizeHex),
		Entries: []WorkspaceSnapshotEntryV1{
			{Path: "bin/run", Kind: WorkspaceSnapshotRegularFile, Executable: true, Content: []byte("#!/bin/sh\n")},
			{Path: "empty", Kind: WorkspaceSnapshotRegularFile, Content: []byte{}},
			{Path: "bin", Kind: WorkspaceSnapshotDirectory},
		},
		Excluded: []WorkspaceSnapshotExcludedV1{
			{Path: "pipe", Kind: WorkspaceSnapshotExcludedFIFO, Reason: WorkspaceSnapshotResidualUnsupportedKind},
		},
	}

	sealed, err := SealWorkspaceSnapshotBundleV1(original)
	if err != nil {
		t.Fatalf("seal bundle: %v", err)
	}
	if sealed.Entries[0].Path != "bin" || sealed.Entries[1].Path != "bin/run" || sealed.TotalBytes != uint64(len("#!/bin/sh\n")) {
		t.Fatalf("bundle was not normalized: %#v", sealed)
	}
	encoded, err := EncodeWorkspaceSnapshotBundleV1(sealed)
	if err != nil {
		t.Fatalf("encode bundle: %v", err)
	}
	decoded, err := DecodeWorkspaceSnapshotBundleV1(encoded)
	if err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if decoded.BundleDigest != sealed.BundleDigest || decoded.EntrySetDigest != sealed.EntrySetDigest {
		t.Fatal("canonical round trip changed exact digests")
	}

	original.Entries[0].Content[0] = 'X'
	if bytes.Equal(original.Entries[0].Content, sealed.Entries[1].Content) {
		t.Fatal("sealed bundle aliases caller content")
	}
	cloned := sealed.Clone()
	cloned.Entries[1].Content[0] = 'Y'
	if bytes.Equal(cloned.Entries[1].Content, sealed.Entries[1].Content) {
		t.Fatal("clone aliases source content")
	}
}

func TestWorkspaceSnapshotBundleV1RejectsPathAndParentSplices(t *testing.T) {
	base := workspaceSnapshotBundleFixtureV1(t)
	tests := map[string]func(*WorkspaceSnapshotBundleV1){
		"traversal": func(v *WorkspaceSnapshotBundleV1) { v.Entries[0].Path = "../escape" },
		"absolute":  func(v *WorkspaceSnapshotBundleV1) { v.Entries[0].Path = "/escape" },
		"backslash": func(v *WorkspaceSnapshotBundleV1) { v.Entries[0].Path = `dir\file` },
		"missing parent": func(v *WorkspaceSnapshotBundleV1) {
			v.Entries = []WorkspaceSnapshotEntryV1{{Path: "missing/file", Kind: WorkspaceSnapshotRegularFile, Content: []byte("x")}}
		},
		"duplicate": func(v *WorkspaceSnapshotBundleV1) { v.Entries = append(v.Entries, v.Entries[0]) },
		"excluded overlap": func(v *WorkspaceSnapshotBundleV1) {
			v.Excluded = append(v.Excluded, WorkspaceSnapshotExcludedV1{Path: v.Entries[0].Path, Kind: WorkspaceSnapshotExcludedSymlink, Reason: WorkspaceSnapshotResidualUnsupportedKind})
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := base.Clone()
			mutate(&candidate)
			if _, err := SealWorkspaceSnapshotBundleV1(candidate); err == nil {
				t.Fatal("expected invalid path/closure to fail")
			}
		})
	}
}

func TestWorkspaceSnapshotBundleV1RejectsTamperAndNonCanonicalJSON(t *testing.T) {
	sealed := workspaceSnapshotBundleFixtureV1(t)
	tampered := sealed.Clone()
	tampered.Entries[1].Content[0] ^= 0xff
	if err := tampered.ValidateShape(); err == nil {
		t.Fatal("content tamper passed exact validation")
	}
	tampered = sealed.Clone()
	tampered.EntrySetDigest = strings.Repeat("b", DigestSizeHex)
	if err := tampered.ValidateShape(); err == nil {
		t.Fatal("entry set digest tamper passed exact validation")
	}

	canonical, err := EncodeWorkspaceSnapshotBundleV1(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeWorkspaceSnapshotBundleV1(append([]byte(" "), canonical...)); err == nil {
		t.Fatal("non-canonical whitespace was accepted")
	}
	withUnknown := bytes.Replace(canonical, []byte(`{"contract_version":`), []byte(`{"unknown":true,"contract_version":`), 1)
	if _, err := DecodeWorkspaceSnapshotBundleV1(withUnknown); err == nil {
		t.Fatal("unknown field was accepted")
	}
	withDuplicate := bytes.Replace(canonical, []byte(`{"contract_version":`), []byte(`{"snapshot_id":"other","contract_version":`), 1)
	if _, err := DecodeWorkspaceSnapshotBundleV1(withDuplicate); err == nil {
		t.Fatal("duplicate field was accepted")
	}
}

func TestWorkspaceSnapshotBundleV1RejectsUnsupportedEntryKind(t *testing.T) {
	base := workspaceSnapshotBundleFixtureV1(t)
	base.Entries = append(base.Entries, WorkspaceSnapshotEntryV1{Path: "link", Kind: WorkspaceSnapshotEntryKindV1("symlink")})
	if _, err := SealWorkspaceSnapshotBundleV1(base); err == nil {
		t.Fatal("symlink entry must be a residual, not restorable content")
	}
}

func workspaceSnapshotBundleFixtureV1(t *testing.T) WorkspaceSnapshotBundleV1 {
	t.Helper()
	sealed, err := SealWorkspaceSnapshotBundleV1(WorkspaceSnapshotBundleV1{
		SnapshotID:        "snapshot-1",
		TenantID:          "tenant-1",
		SourceScopeDigest: strings.Repeat("a", DigestSizeHex),
		Entries: []WorkspaceSnapshotEntryV1{
			{Path: "dir", Kind: WorkspaceSnapshotDirectory},
			{Path: "dir/file", Kind: WorkspaceSnapshotRegularFile, Content: []byte("payload")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
