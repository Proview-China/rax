package contract_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func outputWindowV1() (time.Time, int64, int64) {
	now := time.Unix(500, 0)
	return now, now.UnixNano(), now.Add(time.Minute).UnixNano()
}

func outputRefV1(id string) toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
}

func fileRefV1(name string, revision core.Revision) toolcontract.WorkspaceFileRefV1 {
	return toolcontract.WorkspaceFileRefV1{Path: name, Revision: revision, Digest: core.DigestBytes([]byte(name))}
}

func strptrV1(value string) *string { return &value }

func TestWorkspaceReadOutputInlineArtifactExactOne(t *testing.T) {
	now, checked, expires := outputWindowV1()
	inline, err := toolcontract.SealWorkspaceReadOutputV1(toolcontract.WorkspaceReadOutputV1{
		File: fileRefV1("a.go", 1), Complete: true, Content: strptrV1(""),
		CheckedUnixNano: checked, ExpiresUnixNano: expires,
	})
	if err != nil || inline.ValidateCurrent(now) != nil {
		t.Fatalf("inline read: %#v %v", inline, err)
	}
	artifact := outputRefV1("artifact-read")
	if _, err := toolcontract.SealWorkspaceReadOutputV1(toolcontract.WorkspaceReadOutputV1{
		File: fileRefV1("a.go", 1), BytesReturned: 4, TotalBytes: 8, ArtifactRef: &artifact,
		CheckedUnixNano: checked, ExpiresUnixNano: expires,
	}); err != nil {
		t.Fatal(err)
	}
	bad := inline
	bad.ArtifactRef = &artifact
	if bad.ValidateCurrent(now) == nil {
		t.Fatal("read accepted inline and Artifact Ref together")
	}
	bad = inline
	bad.BytesReturned = 1
	if bad.ValidateCurrent(now) == nil {
		t.Fatal("read accepted inline byte-count drift")
	}
}

func TestWorkspaceSearchOutputBoundsOrderingAndArtifact(t *testing.T) {
	now, checked, expires := outputWindowV1()
	matches := []toolcontract.WorkspaceSearchMatchV1{
		{Path: "a.go", FileRevision: 1, FileDigest: core.DigestBytes([]byte("a")), StartByte: 1, EndByte: 2, Preview: "a"},
		{Path: "b.go", FileRevision: 2, FileDigest: core.DigestBytes([]byte("b")), StartByte: 3, EndByte: 4, Preview: "b"},
	}
	sealed, err := toolcontract.SealWorkspaceSearchOutputV1(toolcontract.WorkspaceSearchOutputV1{
		WorkspaceRevision: 7, WorkspaceDigest: core.DigestBytes([]byte("workspace")), Matches: matches,
		Complete: true, CheckedUnixNano: checked, ExpiresUnixNano: expires,
	})
	if err != nil || sealed.ValidateCurrent(now) != nil {
		t.Fatalf("search: %#v %v", sealed, err)
	}
	matches[0].Preview = "mutated"
	if sealed.Matches[0].Preview != "a" {
		t.Fatal("search seal aliases caller matches")
	}
	unsorted := sealed.Clone()
	unsorted.Matches[0], unsorted.Matches[1] = unsorted.Matches[1], unsorted.Matches[0]
	if unsorted.ValidateCurrent(now) == nil {
		t.Fatal("search accepted unsorted matches")
	}
	tooLarge := sealed.Clone()
	tooLarge.Matches[0].Preview = strings.Repeat("x", toolcontract.CoreToolMaxPreviewBytesV1+1)
	if tooLarge.ValidateCurrent(now) == nil {
		t.Fatal("search accepted unbounded preview")
	}
	artifact := outputRefV1("artifact-search")
	if _, err := toolcontract.SealWorkspaceSearchOutputV1(toolcontract.WorkspaceSearchOutputV1{
		WorkspaceRevision: 7, WorkspaceDigest: core.DigestBytes([]byte("workspace")), ArtifactRef: &artifact,
		CheckedUnixNano: checked, ExpiresUnixNano: expires,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceInspectOutputMetadataRangeAndCurrentness(t *testing.T) {
	now, checked, expires := outputWindowV1()
	entries := []toolcontract.WorkspaceObjectMetadataV1{{Path: "a.go", Kind: "file", Revision: 1, Digest: core.DigestBytes([]byte("a")), SizeBytes: 10, ModifiedUnixNano: checked}}
	sealed, err := toolcontract.SealWorkspaceInspectOutputV1(toolcontract.WorkspaceInspectOutputV1{
		Object:     toolcontract.WorkspaceObjectMetadataV1{Path: "", Kind: "directory", Revision: 7, Digest: core.DigestBytes([]byte("root")), ModifiedUnixNano: checked},
		RangeValid: true, Entries: entries, Complete: true, CheckedUnixNano: checked, ExpiresUnixNano: expires,
	})
	if err != nil || sealed.ValidateCurrent(now) != nil {
		t.Fatalf("inspect: %#v %v", sealed, err)
	}
	entries[0].Path = "changed"
	if sealed.Entries[0].Path != "a.go" {
		t.Fatal("inspect seal aliases caller entries")
	}
	if sealed.ValidateCurrent(time.Unix(0, expires)) == nil {
		t.Fatal("inspect accepted expired currentness")
	}
	bad := sealed.Clone()
	bad.Object.Kind = "device"
	if bad.ValidateCurrent(now) == nil {
		t.Fatal("inspect accepted unsafe metadata kind")
	}
}

func TestWorkspacePatchOutputBindsChangeSetBaseResultAndFiles(t *testing.T) {
	now, checked, expires := outputWindowV1()
	base := exactWorkspaceV1()
	result := base
	result.Revision++
	result.Digest = core.DigestBytes([]byte("result"))
	files := []toolcontract.WorkspaceFileRefV1{fileRefV1("b.go", result.Revision), fileRefV1("a.go", result.Revision)}
	sealed, err := toolcontract.SealWorkspacePatchOutputV1(toolcontract.WorkspacePatchOutputV1{
		ChangeSetRef: outputRefV1("change-set"), BaseWorkspace: base, ResultWorkspace: result, Files: files,
		CheckedUnixNano: checked, ExpiresUnixNano: expires,
	})
	if err != nil || sealed.ValidateCurrent(now) != nil {
		t.Fatalf("patch: %#v %v", sealed, err)
	}
	if sealed.Files[0].Path != "a.go" {
		t.Fatal("patch seal did not canonicalize file refs")
	}
	bad := sealed.Clone()
	bad.ResultWorkspace = bad.BaseWorkspace
	if bad.ValidateCurrent(now) == nil {
		t.Fatal("patch accepted unchanged result workspace")
	}
	bad = sealed.Clone()
	bad.Files[0].Revision = base.Revision
	if bad.ValidateCurrent(now) == nil {
		t.Fatal("patch accepted file outside result revision")
	}
}

func TestProcessExecOutputStreamsExactOneAndCaps(t *testing.T) {
	now, checked, expires := outputWindowV1()
	stdout, stderr := "ok", ""
	sealed, err := toolcontract.SealProcessExecOutputV1(toolcontract.ProcessExecOutputV1{
		AttemptRef: outputRefV1("attempt"), Stdout: &stdout, Stderr: &stderr,
		CheckedUnixNano: checked, ExpiresUnixNano: expires,
	})
	if err != nil || sealed.ValidateCurrent(now) != nil {
		t.Fatalf("exec: %#v %v", sealed, err)
	}
	stdout = "mutated"
	if *sealed.Stdout != "ok" {
		t.Fatal("exec seal aliases caller stdout")
	}
	artifact := outputRefV1("stdout-artifact")
	bad := sealed.Clone()
	bad.StdoutArtifactRef = &artifact
	if bad.ValidateCurrent(now) == nil {
		t.Fatal("exec accepted two stdout representations")
	}
	bad = sealed.Clone()
	tooLarge := strings.Repeat("x", int(toolcontract.CoreToolMaxOutputBytesV1)+1)
	bad.Stdout = &tooLarge
	if bad.ValidateCurrent(now) == nil {
		t.Fatal("exec accepted oversized stdout")
	}
}

func TestCoreToolOutputsStrictDecode(t *testing.T) {
	_, checked, expires := outputWindowV1()
	value, err := toolcontract.SealWorkspaceReadOutputV1(toolcontract.WorkspaceReadOutputV1{
		File: fileRefV1("a.go", 1), Content: strptrV1(""), Complete: true,
		CheckedUnixNano: checked, ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := toolcontract.DecodeWorkspaceReadOutputV1(raw); err != nil {
		t.Fatal(err)
	}
	unknown := append(raw[:len(raw)-1], []byte(",\"unknown\":true}")...)
	if _, err := toolcontract.DecodeWorkspaceReadOutputV1(unknown); err == nil {
		t.Fatal("strict output decoder accepted unknown field")
	}
}

func TestAllCoreToolOutputDecodersRejectUnknownFields(t *testing.T) {
	_, checked, expires := outputWindowV1()
	base := exactWorkspaceV1()
	result := base
	result.Revision++
	result.Digest = core.DigestBytes([]byte("result"))
	stdout, stderr := "ok", ""
	cases := []struct {
		name   string
		value  any
		decode func([]byte) error
	}{
		{"read", toolcontract.WorkspaceReadOutputV1{File: fileRefV1("a.go", 1), Content: strptrV1(""), Complete: true, CheckedUnixNano: checked, ExpiresUnixNano: expires}, func(raw []byte) error { _, err := toolcontract.DecodeWorkspaceReadOutputV1(raw); return err }},
		{"search", toolcontract.WorkspaceSearchOutputV1{WorkspaceRevision: 1, WorkspaceDigest: core.DigestBytes([]byte("workspace")), Matches: []toolcontract.WorkspaceSearchMatchV1{}, Complete: true, CheckedUnixNano: checked, ExpiresUnixNano: expires}, func(raw []byte) error { _, err := toolcontract.DecodeWorkspaceSearchOutputV1(raw); return err }},
		{"inspect", toolcontract.WorkspaceInspectOutputV1{Object: toolcontract.WorkspaceObjectMetadataV1{Kind: "directory", Revision: 1, Digest: core.DigestBytes([]byte("root")), ModifiedUnixNano: checked}, Entries: []toolcontract.WorkspaceObjectMetadataV1{}, Complete: true, CheckedUnixNano: checked, ExpiresUnixNano: expires}, func(raw []byte) error { _, err := toolcontract.DecodeWorkspaceInspectOutputV1(raw); return err }},
		{"patch", toolcontract.WorkspacePatchOutputV1{ChangeSetRef: outputRefV1("change-set"), BaseWorkspace: base, ResultWorkspace: result, Files: []toolcontract.WorkspaceFileRefV1{fileRefV1("a.go", result.Revision)}, CheckedUnixNano: checked, ExpiresUnixNano: expires}, func(raw []byte) error { _, err := toolcontract.DecodeWorkspacePatchOutputV1(raw); return err }},
		{"exec", toolcontract.ProcessExecOutputV1{AttemptRef: outputRefV1("attempt"), Stdout: &stdout, Stderr: &stderr, CheckedUnixNano: checked, ExpiresUnixNano: expires}, func(raw []byte) error { _, err := toolcontract.DecodeProcessExecOutputV1(raw); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.decode(raw); err != nil {
				t.Fatalf("valid output rejected: %v", err)
			}
			unknown := append(raw[:len(raw)-1], []byte(",\"unknown\":true}")...)
			if err := tc.decode(unknown); err == nil {
				t.Fatal("strict decoder accepted unknown field")
			}
		})
	}
}
