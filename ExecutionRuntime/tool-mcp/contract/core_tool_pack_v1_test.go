package contract_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func exactWorkspaceV1() toolcontract.WorkspaceExactRefV1 {
	return toolcontract.WorkspaceExactRefV1{ID: "workspace-v1", Revision: 7, Digest: core.DigestBytes([]byte("workspace"))}
}

func TestCoreToolInputsSealDefaultsAndValidate(t *testing.T) {
	now := time.Unix(100, 0)
	read, err := toolcontract.SealWorkspaceReadInputV1(toolcontract.WorkspaceReadInputV1{
		WorkspaceRoot: exactWorkspaceV1(), RelativePath: "src/main.go", RequestedNotAfter: now.Add(time.Second).UnixNano(),
	})
	if err != nil || read.MaxBytes != toolcontract.CoreToolDefaultReadBytesV1 || read.ValidateCurrent(now) != nil {
		t.Fatalf("read seal: %#v %v", read, err)
	}
	search, err := toolcontract.SealWorkspaceSearchInputV1(toolcontract.WorkspaceSearchInputV1{
		WorkspaceRoot: exactWorkspaceV1(), Query: "needle", RequestedNotAfter: now.Add(time.Second).UnixNano(),
	})
	if err != nil || search.Mode != "literal" || search.MaxResults != toolcontract.CoreToolDefaultSearchHitsV1 {
		t.Fatalf("search seal: %#v %v", search, err)
	}
	inspect, err := toolcontract.SealWorkspaceInspectInputV1(toolcontract.WorkspaceInspectInputV1{
		WorkspaceRoot: exactWorkspaceV1(), RelativePath: "", RequestedNotAfter: now.Add(time.Second).UnixNano(),
	})
	if err != nil || inspect.MaxEntries != toolcontract.CoreToolDefaultInspectV1 {
		t.Fatalf("inspect seal: %#v %v", inspect, err)
	}
	exec, err := toolcontract.SealProcessExecInputV1(toolcontract.ProcessExecInputV1{
		WorkspaceRoot: exactWorkspaceV1(), Argv: []string{"go", "test", "./..."}, CWD: ".", RequestedNotAfter: now.Add(time.Second).UnixNano(),
	})
	if err != nil || exec.TimeoutMillis != toolcontract.CoreToolDefaultTimeoutMSV1 || exec.MaxStdoutBytes != toolcontract.CoreToolDefaultOutputBytesV1 || exec.MaxStderrBytes != toolcontract.CoreToolDefaultOutputBytesV1 {
		t.Fatalf("exec seal: %#v %v", exec, err)
	}
}

func TestCoreToolInputsFailClosed(t *testing.T) {
	now := time.Unix(100, 0)
	tests := map[string]func() error{
		"path escape": func() error {
			return (toolcontract.WorkspaceReadInputV1{WorkspaceRoot: exactWorkspaceV1(), RelativePath: "../secret", MaxBytes: 1, RequestedNotAfter: now.Add(time.Second).UnixNano()}).ValidateCurrent(now)
		},
		"absolute path": func() error {
			return (toolcontract.WorkspaceInspectInputV1{WorkspaceRoot: exactWorkspaceV1(), RelativePath: "/etc", MaxEntries: 1, RequestedNotAfter: now.Add(time.Second).UnixNano()}).ValidateCurrent(now)
		},
		"expired": func() error {
			return (toolcontract.WorkspaceSearchInputV1{WorkspaceRoot: exactWorkspaceV1(), Query: "x", Mode: "literal", MaxResults: 1, MaxResultBytes: 1, RequestedNotAfter: now.UnixNano()}).ValidateCurrent(now)
		},
		"shell": func() error {
			return (toolcontract.ProcessExecInputV1{WorkspaceRoot: exactWorkspaceV1(), Argv: []string{"bash", "-c", "echo pwned"}, CWD: ".", TimeoutMillis: 1, MaxStdoutBytes: 1, MaxStderrBytes: 1, RequestedNotAfter: now.Add(time.Second).UnixNano()}).ValidateCurrent(now)
		},
		"patch without base": func() error {
			return (toolcontract.WorkspacePatchInputV1{WorkspaceRoot: exactWorkspaceV1(), RequestedNotAfter: now.Add(time.Second).UnixNano(), Changes: []toolcontract.WorkspacePatchChangeV1{{RelativePath: "a", Hunks: []toolcontract.WorkspacePatchHunkV1{{OldStart: 1, NewStart: 1, NewLines: 1, Lines: []toolcontract.WorkspacePatchLineV1{{Op: "insert", Text: "x"}}}}}}}).ValidateCurrent(now)
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := test(); err == nil {
				t.Fatal("expected fail-closed error")
			}
		})
	}
}

func TestProcessExecSealDeepCopies(t *testing.T) {
	now := time.Unix(100, 0)
	argv := []string{"go", "version"}
	env := map[string]string{"LANG": "C"}
	sealed, err := toolcontract.SealProcessExecInputV1(toolcontract.ProcessExecInputV1{
		WorkspaceRoot: exactWorkspaceV1(), Argv: argv, CWD: ".", Env: env, RequestedNotAfter: now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	argv[0] = "bash"
	env["LANG"] = "changed"
	if sealed.Argv[0] != "go" || sealed.Env["LANG"] != "C" {
		t.Fatal("sealed input aliases caller memory")
	}
}

func TestCoreToolStrictDecodeRejectsUnknownDuplicateAndTrailing(t *testing.T) {
	now := time.Unix(100, 0)
	input, err := toolcontract.SealWorkspaceReadInputV1(toolcontract.WorkspaceReadInputV1{
		WorkspaceRoot: exactWorkspaceV1(), RelativePath: "a", RequestedNotAfter: now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := toolcontract.DecodeWorkspaceReadInputV1(raw); err != nil {
		t.Fatal(err)
	}
	for name, malformed := range map[string][]byte{
		"unknown":   append(raw[:len(raw)-1], []byte(`,"unknown":true}`)...),
		"trailing":  append(append([]byte(nil), raw...), []byte(` {}`)...),
		"duplicate": []byte(`{"workspace_root":{"id":"workspace-v1","revision":7,"digest":"` + string(exactWorkspaceV1().Digest) + `"},"relative_path":"a","relative_path":"b","start_byte":0,"max_bytes":1,"requested_not_after_unix_nano":101000000000}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := toolcontract.DecodeWorkspaceReadInputV1(malformed); err == nil {
				t.Fatal("strict decoder accepted malformed JSON")
			}
		})
	}
}
