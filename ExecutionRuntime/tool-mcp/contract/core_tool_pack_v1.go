package contract

import (
	"encoding/json"
	"path"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	CoreToolPackContractVersionV1 = "praxis.tool-mcp.core-tool-pack/v1"

	CoreToolWorkspaceReadV1    = "workspace.read"
	CoreToolWorkspaceSearchV1  = "workspace.search"
	CoreToolWorkspaceInspectV1 = "workspace.inspect"
	CoreToolWorkspacePatchV1   = "workspace.patch"
	CoreToolProcessExecV1      = "process.exec"

	CoreToolDefaultReadBytesV1   uint64 = 64 << 10
	CoreToolMaxReadBytesV1       uint64 = 1 << 20
	CoreToolDefaultSearchHitsV1  uint32 = 50
	CoreToolMaxSearchHitsV1      uint32 = 200
	CoreToolDefaultSearchBytesV1 uint64 = 256 << 10
	CoreToolMaxSearchBytesV1     uint64 = 1 << 20
	CoreToolDefaultInspectV1     uint32 = 200
	CoreToolMaxInspectV1         uint32 = 1000
	CoreToolMaxPatchChangesV1    int    = 64
	CoreToolMaxPatchHunksV1      int    = 1024
	CoreToolMaxPatchBytesV1      int    = 1 << 20
	CoreToolDefaultTimeoutMSV1   uint64 = 30_000
	CoreToolMaxTimeoutMSV1       uint64 = 300_000
	CoreToolDefaultOutputBytesV1 uint64 = 256 << 10
	CoreToolMaxOutputBytesV1     uint64 = 1 << 20
)

type WorkspaceExactRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r WorkspaceExactRefV1) Validate() error {
	return ObjectRef{ID: r.ID, Revision: r.Revision, Digest: r.Digest}.Validate()
}

type WorkspaceReadInputV1 struct {
	WorkspaceRoot     WorkspaceExactRefV1 `json:"workspace_root"`
	RelativePath      string              `json:"relative_path"`
	StartByte         uint64              `json:"start_byte"`
	MaxBytes          uint64              `json:"max_bytes"`
	RequestedNotAfter int64               `json:"requested_not_after_unix_nano"`
}

func SealWorkspaceReadInputV1(v WorkspaceReadInputV1) (WorkspaceReadInputV1, error) {
	if v.MaxBytes == 0 {
		v.MaxBytes = CoreToolDefaultReadBytesV1
	}
	return v, v.ValidateCurrent(time.Unix(0, v.RequestedNotAfter-1))
}

func DecodeWorkspaceReadInputV1(raw []byte) (WorkspaceReadInputV1, error) {
	var value WorkspaceReadInputV1
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return WorkspaceReadInputV1{}, err
	}
	return SealWorkspaceReadInputV1(value)
}

func (v WorkspaceReadInputV1) ValidateCurrent(now time.Time) error {
	if v.WorkspaceRoot.Validate() != nil || validateWorkspaceRelativePathV1(v.RelativePath, false) != nil ||
		v.MaxBytes == 0 || v.MaxBytes > CoreToolMaxReadBytesV1 {
		return invalid("workspace.read input is invalid or unbounded")
	}
	return validateRequestedNotAfterV1(v.RequestedNotAfter, now)
}

type WorkspaceSearchInputV1 struct {
	WorkspaceRoot     WorkspaceExactRefV1 `json:"workspace_root"`
	Query             string              `json:"query"`
	PathPrefix        string              `json:"path_prefix"`
	Mode              string              `json:"mode"`
	MaxResults        uint32              `json:"max_results"`
	MaxResultBytes    uint64              `json:"max_result_bytes"`
	RequestedNotAfter int64               `json:"requested_not_after_unix_nano"`
}

func SealWorkspaceSearchInputV1(v WorkspaceSearchInputV1) (WorkspaceSearchInputV1, error) {
	if v.Mode == "" {
		v.Mode = "literal"
	}
	if v.MaxResults == 0 {
		v.MaxResults = CoreToolDefaultSearchHitsV1
	}
	if v.MaxResultBytes == 0 {
		v.MaxResultBytes = CoreToolDefaultSearchBytesV1
	}
	return v, v.ValidateCurrent(time.Unix(0, v.RequestedNotAfter-1))
}

func DecodeWorkspaceSearchInputV1(raw []byte) (WorkspaceSearchInputV1, error) {
	var value WorkspaceSearchInputV1
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return WorkspaceSearchInputV1{}, err
	}
	return SealWorkspaceSearchInputV1(value)
}

func (v WorkspaceSearchInputV1) ValidateCurrent(now time.Time) error {
	if v.WorkspaceRoot.Validate() != nil || strings.TrimSpace(v.Query) == "" || len(v.Query) > 4096 ||
		validateWorkspaceRelativePathV1(v.PathPrefix, true) != nil ||
		(v.Mode != "literal" && v.Mode != "regexp-re2") ||
		v.MaxResults == 0 || v.MaxResults > CoreToolMaxSearchHitsV1 ||
		v.MaxResultBytes == 0 || v.MaxResultBytes > CoreToolMaxSearchBytesV1 {
		return invalid("workspace.search input is invalid or unbounded")
	}
	return validateRequestedNotAfterV1(v.RequestedNotAfter, now)
}

type WorkspaceInspectRangeV1 struct {
	StartByte uint64 `json:"start_byte"`
	MaxBytes  uint64 `json:"max_bytes"`
}

type WorkspaceInspectInputV1 struct {
	WorkspaceRoot     WorkspaceExactRefV1     `json:"workspace_root"`
	RelativePath      string                  `json:"relative_path"`
	Range             WorkspaceInspectRangeV1 `json:"range"`
	MaxEntries        uint32                  `json:"max_entries"`
	RequestedNotAfter int64                   `json:"requested_not_after_unix_nano"`
}

func SealWorkspaceInspectInputV1(v WorkspaceInspectInputV1) (WorkspaceInspectInputV1, error) {
	if v.MaxEntries == 0 {
		v.MaxEntries = CoreToolDefaultInspectV1
	}
	return v, v.ValidateCurrent(time.Unix(0, v.RequestedNotAfter-1))
}

func DecodeWorkspaceInspectInputV1(raw []byte) (WorkspaceInspectInputV1, error) {
	var value WorkspaceInspectInputV1
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return WorkspaceInspectInputV1{}, err
	}
	return SealWorkspaceInspectInputV1(value)
}

func (v WorkspaceInspectInputV1) ValidateCurrent(now time.Time) error {
	if v.WorkspaceRoot.Validate() != nil || validateWorkspaceRelativePathV1(v.RelativePath, true) != nil ||
		v.Range.MaxBytes > CoreToolMaxReadBytesV1 || v.MaxEntries == 0 || v.MaxEntries > CoreToolMaxInspectV1 {
		return invalid("workspace.inspect input is invalid or unbounded")
	}
	return validateRequestedNotAfterV1(v.RequestedNotAfter, now)
}

type WorkspacePatchHunkV1 struct {
	OldStart uint32                 `json:"old_start"`
	OldLines uint32                 `json:"old_lines"`
	NewStart uint32                 `json:"new_start"`
	NewLines uint32                 `json:"new_lines"`
	Lines    []WorkspacePatchLineV1 `json:"lines"`
}

type WorkspacePatchLineV1 struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

type WorkspacePatchChangeV1 struct {
	RelativePath string                 `json:"relative_path"`
	BaseRevision core.Revision          `json:"base_revision"`
	BaseDigest   core.Digest            `json:"base_digest"`
	Hunks        []WorkspacePatchHunkV1 `json:"hunks"`
}

type WorkspacePatchInputV1 struct {
	WorkspaceRoot     WorkspaceExactRefV1      `json:"workspace_root"`
	Changes           []WorkspacePatchChangeV1 `json:"changes"`
	RequestedNotAfter int64                    `json:"requested_not_after_unix_nano"`
}

func (v WorkspacePatchInputV1) ValidateCurrent(now time.Time) error {
	if v.WorkspaceRoot.Validate() != nil || len(v.Changes) == 0 || len(v.Changes) > CoreToolMaxPatchChangesV1 {
		return invalid("workspace.patch requires bounded changes")
	}
	total := 0
	var previousPath string
	for _, change := range v.Changes {
		if validateWorkspaceRelativePathV1(change.RelativePath, false) != nil || change.BaseRevision == 0 ||
			change.BaseDigest.Validate() != nil || len(change.Hunks) == 0 || len(change.Hunks) > CoreToolMaxPatchHunksV1 {
			return invalid("workspace.patch change lacks exact base or bounded hunks")
		}
		if previousPath != "" && previousPath >= change.RelativePath {
			return invalid("workspace.patch paths must be sorted and unique")
		}
		previousPath = change.RelativePath
		for _, hunk := range change.Hunks {
			if hunk.OldStart == 0 || hunk.NewStart == 0 || len(hunk.Lines) == 0 {
				return invalid("workspace.patch hunk is invalid")
			}
			var oldCount, newCount uint32
			for _, line := range hunk.Lines {
				switch line.Op {
				case "context":
					oldCount++
					newCount++
				case "delete":
					oldCount++
				case "insert":
					newCount++
				default:
					return invalid("workspace.patch line operation is invalid")
				}
				total += len(line.Text)
				if total > CoreToolMaxPatchBytesV1 {
					return invalid("workspace.patch canonical body exceeds limit")
				}
			}
			if oldCount != hunk.OldLines || newCount != hunk.NewLines {
				return conflict("workspace.patch hunk counts drift from structured lines")
			}
		}
	}
	canonical, err := json.Marshal(v)
	if err != nil || len(canonical) > CoreToolMaxPatchBytesV1 {
		return invalid("workspace.patch canonical body exceeds limit")
	}
	return validateRequestedNotAfterV1(v.RequestedNotAfter, now)
}

func DecodeWorkspacePatchInputV1(raw []byte) (WorkspacePatchInputV1, error) {
	var value WorkspacePatchInputV1
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return WorkspacePatchInputV1{}, err
	}
	if err := value.ValidateCurrent(time.Unix(0, value.RequestedNotAfter-1)); err != nil {
		return WorkspacePatchInputV1{}, err
	}
	value.Changes = append([]WorkspacePatchChangeV1(nil), value.Changes...)
	return value, nil
}

type ProcessExecInputV1 struct {
	WorkspaceRoot     WorkspaceExactRefV1 `json:"workspace_root"`
	Argv              []string            `json:"argv"`
	CWD               string              `json:"cwd"`
	Env               map[string]string   `json:"env"`
	TimeoutMillis     uint64              `json:"timeout_millis"`
	MaxStdoutBytes    uint64              `json:"max_stdout_bytes"`
	MaxStderrBytes    uint64              `json:"max_stderr_bytes"`
	RequestedNotAfter int64               `json:"requested_not_after_unix_nano"`
}

func SealProcessExecInputV1(v ProcessExecInputV1) (ProcessExecInputV1, error) {
	if v.TimeoutMillis == 0 {
		v.TimeoutMillis = CoreToolDefaultTimeoutMSV1
	}
	if v.MaxStdoutBytes == 0 {
		v.MaxStdoutBytes = CoreToolDefaultOutputBytesV1
	}
	if v.MaxStderrBytes == 0 {
		v.MaxStderrBytes = CoreToolDefaultOutputBytesV1
	}
	v.Argv = append([]string(nil), v.Argv...)
	if v.Env != nil {
		env := make(map[string]string, len(v.Env))
		for key, value := range v.Env {
			env[key] = value
		}
		v.Env = env
	}
	return v, v.ValidateCurrent(time.Unix(0, v.RequestedNotAfter-1))
}

func DecodeProcessExecInputV1(raw []byte) (ProcessExecInputV1, error) {
	var value ProcessExecInputV1
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return ProcessExecInputV1{}, err
	}
	return SealProcessExecInputV1(value)
}

func (v ProcessExecInputV1) ValidateCurrent(now time.Time) error {
	if v.WorkspaceRoot.Validate() != nil || len(v.Argv) == 0 || len(v.Argv) > 128 || validateWorkspaceRelativePathV1(v.CWD, true) != nil ||
		len(v.Env) > 64 || v.TimeoutMillis == 0 || v.TimeoutMillis > CoreToolMaxTimeoutMSV1 ||
		v.MaxStdoutBytes == 0 || v.MaxStdoutBytes > CoreToolMaxOutputBytesV1 ||
		v.MaxStderrBytes == 0 || v.MaxStderrBytes > CoreToolMaxOutputBytesV1 {
		return invalid("process.exec input is invalid or unbounded")
	}
	total := 0
	for _, arg := range v.Argv {
		if arg == "" || strings.IndexByte(arg, 0) >= 0 || len(arg) > 8192 {
			return invalid("process.exec argv contains invalid argument")
		}
		total += len(arg)
	}
	if total > 65536 || isShellExecutableV1(v.Argv[0]) {
		return invalid("process.exec forbids implicit or compatibility shell execution")
	}
	for key, value := range v.Env {
		if strings.TrimSpace(key) != key || key == "" || strings.IndexByte(key, 0) >= 0 ||
			strings.IndexByte(value, 0) >= 0 || len(key)+len(value) > 8192 {
			return invalid("process.exec environment is invalid")
		}
	}
	return validateRequestedNotAfterV1(v.RequestedNotAfter, now)
}

func validateWorkspaceRelativePathV1(value string, allowRoot bool) error {
	if value == "" && allowRoot {
		return nil
	}
	if value == "" || strings.IndexByte(value, 0) >= 0 || strings.HasPrefix(value, "/") ||
		strings.Contains(value, "\\") || len(value) > 4096 {
		return invalid("workspace path is not a bounded portable relative path")
	}
	clean := path.Clean(value)
	if clean == "." {
		if allowRoot {
			return nil
		}
		return invalid("workspace path must identify an object")
	}
	if clean != value || clean == ".." || strings.HasPrefix(clean, "../") {
		return invalid("workspace path escapes or is not canonical")
	}
	return nil
}

func validateRequestedNotAfterV1(value int64, now time.Time) error {
	if now.IsZero() || value <= 0 || !now.Before(time.Unix(0, value)) {
		return invalid("requested currentness window is absent or expired")
	}
	return nil
}

func isShellExecutableV1(value string) bool {
	switch strings.ToLower(path.Base(value)) {
	case "sh", "bash", "zsh", "dash", "fish", "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return true
	default:
		return false
	}
}
