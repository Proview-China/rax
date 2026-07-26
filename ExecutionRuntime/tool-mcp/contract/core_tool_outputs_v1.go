package contract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const CoreToolMaxPreviewBytesV1 = 8 << 10

type WorkspaceFileRefV1 struct {
	Path     string        `json:"path"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r WorkspaceFileRefV1) Validate() error {
	if validateWorkspaceRelativePathV1(r.Path, false) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return invalid("workspace file reference is invalid")
	}
	return nil
}

type WorkspaceObjectMetadataV1 struct {
	Path             string        `json:"path"`
	Kind             string        `json:"kind"`
	Revision         core.Revision `json:"revision"`
	Digest           core.Digest   `json:"digest"`
	SizeBytes        uint64        `json:"size_bytes"`
	Mode             uint32        `json:"mode"`
	ModifiedUnixNano int64         `json:"modified_unix_nano"`
	LinkTarget       string        `json:"link_target,omitempty"`
}

func (m WorkspaceObjectMetadataV1) Validate() error {
	if validateWorkspaceRelativePathV1(m.Path, true) != nil || m.Revision == 0 || m.Digest.Validate() != nil || m.ModifiedUnixNano <= 0 {
		return invalid("workspace object metadata is incomplete")
	}
	switch m.Kind {
	case "file", "directory", "symlink":
	default:
		return invalid("workspace object kind is invalid")
	}
	if len(m.LinkTarget) > 4096 || m.Kind != "symlink" && m.LinkTarget != "" {
		return invalid("workspace object link metadata is invalid")
	}
	return nil
}

type WorkspaceReadOutputV1 struct {
	File            WorkspaceFileRefV1 `json:"file"`
	StartByte       uint64             `json:"start_byte"`
	BytesReturned   uint64             `json:"bytes_returned"`
	TotalBytes      uint64             `json:"total_bytes"`
	Complete        bool               `json:"complete"`
	Content         *string            `json:"content"`
	ArtifactRef     *ObjectRef         `json:"artifact_ref"`
	CheckedUnixNano int64              `json:"checked_unix_nano"`
	ExpiresUnixNano int64              `json:"expires_unix_nano"`
}

func (v WorkspaceReadOutputV1) ValidateCurrent(now time.Time) error {
	if v.File.Validate() != nil || v.BytesReturned > CoreToolMaxReadBytesV1 || v.StartByte > v.TotalBytes || v.BytesReturned > v.TotalBytes-v.StartByte {
		return invalid("workspace.read output range or file is invalid")
	}
	if (v.Content == nil) == (v.ArtifactRef == nil) {
		return invalid("workspace.read output requires exactly one inline content or Artifact Ref")
	}
	if v.Content != nil && uint64(len(*v.Content)) != v.BytesReturned || v.ArtifactRef != nil && v.ArtifactRef.Validate() != nil {
		return invalid("workspace.read output payload is invalid")
	}
	if v.Complete && v.StartByte+v.BytesReturned != v.TotalBytes {
		return conflict("workspace.read complete output does not reach total bytes")
	}
	return validateOutputWindowV1(v.CheckedUnixNano, v.ExpiresUnixNano, now)
}

func (v WorkspaceReadOutputV1) Clone() WorkspaceReadOutputV1 {
	out := v
	if v.Content != nil {
		value := *v.Content
		out.Content = &value
	}
	if v.ArtifactRef != nil {
		value := *v.ArtifactRef
		out.ArtifactRef = &value
	}
	return out
}

func SealWorkspaceReadOutputV1(v WorkspaceReadOutputV1) (WorkspaceReadOutputV1, error) {
	v = v.Clone()
	return v, v.ValidateCurrent(time.Unix(0, v.CheckedUnixNano))
}

func DecodeWorkspaceReadOutputV1(raw []byte) (WorkspaceReadOutputV1, error) {
	var value WorkspaceReadOutputV1
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return WorkspaceReadOutputV1{}, err
	}
	return SealWorkspaceReadOutputV1(value)
}

type WorkspaceSearchMatchV1 struct {
	Path         string        `json:"path"`
	FileRevision core.Revision `json:"file_revision"`
	FileDigest   core.Digest   `json:"file_digest"`
	StartByte    uint64        `json:"start_byte"`
	EndByte      uint64        `json:"end_byte"`
	Preview      string        `json:"preview"`
}

func (m WorkspaceSearchMatchV1) Validate() error {
	if validateWorkspaceRelativePathV1(m.Path, false) != nil || m.FileRevision == 0 || m.FileDigest.Validate() != nil || m.EndByte <= m.StartByte || len(m.Preview) > CoreToolMaxPreviewBytesV1 {
		return invalid("workspace.search match is invalid or unbounded")
	}
	return nil
}

type WorkspaceSearchOutputV1 struct {
	WorkspaceRevision core.Revision            `json:"workspace_revision"`
	WorkspaceDigest   core.Digest              `json:"workspace_digest"`
	Matches           []WorkspaceSearchMatchV1 `json:"matches"`
	Complete          bool                     `json:"complete"`
	ArtifactRef       *ObjectRef               `json:"artifact_ref"`
	CheckedUnixNano   int64                    `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                    `json:"expires_unix_nano"`
}

func (v WorkspaceSearchOutputV1) ValidateCurrent(now time.Time) error {
	if v.WorkspaceRevision == 0 || v.WorkspaceDigest.Validate() != nil || len(v.Matches) > int(CoreToolMaxSearchHitsV1) {
		return invalid("workspace.search output identity or match count is invalid")
	}
	if (v.Matches == nil) == (v.ArtifactRef == nil) {
		return invalid("workspace.search output requires exactly one bounded matches set or Artifact Ref")
	}
	if v.ArtifactRef != nil && v.ArtifactRef.Validate() != nil {
		return invalid("workspace.search Artifact Ref is invalid")
	}
	bytes := 0
	for i, match := range v.Matches {
		if match.Validate() != nil {
			return invalid("workspace.search output contains invalid match")
		}
		bytes += len(match.Path) + len(match.Preview)
		if bytes > int(CoreToolMaxSearchBytesV1) {
			return invalid("workspace.search inline matches exceed byte cap")
		}
		if i > 0 && !searchMatchLessV1(v.Matches[i-1], match) {
			return invalid("workspace.search matches must be sorted and unique")
		}
	}
	return validateOutputWindowV1(v.CheckedUnixNano, v.ExpiresUnixNano, now)
}

func (v WorkspaceSearchOutputV1) Clone() WorkspaceSearchOutputV1 {
	out := v
	if v.Matches != nil {
		out.Matches = make([]WorkspaceSearchMatchV1, len(v.Matches))
		copy(out.Matches, v.Matches)
	}
	if v.ArtifactRef != nil {
		value := *v.ArtifactRef
		out.ArtifactRef = &value
	}
	return out
}

func SealWorkspaceSearchOutputV1(v WorkspaceSearchOutputV1) (WorkspaceSearchOutputV1, error) {
	v = v.Clone()
	return v, v.ValidateCurrent(time.Unix(0, v.CheckedUnixNano))
}

func DecodeWorkspaceSearchOutputV1(raw []byte) (WorkspaceSearchOutputV1, error) {
	var value WorkspaceSearchOutputV1
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return WorkspaceSearchOutputV1{}, err
	}
	return SealWorkspaceSearchOutputV1(value)
}

func searchMatchLessV1(a, b WorkspaceSearchMatchV1) bool {
	return a.Path < b.Path || a.Path == b.Path && (a.StartByte < b.StartByte || a.StartByte == b.StartByte && a.EndByte < b.EndByte)
}

type WorkspaceInspectOutputV1 struct {
	Object          WorkspaceObjectMetadataV1   `json:"object"`
	RangeValid      bool                        `json:"range_valid"`
	Entries         []WorkspaceObjectMetadataV1 `json:"entries"`
	Complete        bool                        `json:"complete"`
	CheckedUnixNano int64                       `json:"checked_unix_nano"`
	ExpiresUnixNano int64                       `json:"expires_unix_nano"`
}

func (v WorkspaceInspectOutputV1) ValidateCurrent(now time.Time) error {
	if v.Object.Validate() != nil || v.Entries == nil || len(v.Entries) > int(CoreToolMaxInspectV1) {
		return invalid("workspace.inspect output object or entries are invalid")
	}
	for i, entry := range v.Entries {
		if entry.Validate() != nil {
			return invalid("workspace.inspect output contains invalid entry")
		}
		if i > 0 && v.Entries[i-1].Path >= entry.Path {
			return invalid("workspace.inspect entries must be sorted and unique")
		}
	}
	return validateOutputWindowV1(v.CheckedUnixNano, v.ExpiresUnixNano, now)
}

func (v WorkspaceInspectOutputV1) Clone() WorkspaceInspectOutputV1 {
	out := v
	if v.Entries != nil {
		out.Entries = make([]WorkspaceObjectMetadataV1, len(v.Entries))
		copy(out.Entries, v.Entries)
	}
	return out
}

func SealWorkspaceInspectOutputV1(v WorkspaceInspectOutputV1) (WorkspaceInspectOutputV1, error) {
	v = v.Clone()
	return v, v.ValidateCurrent(time.Unix(0, v.CheckedUnixNano))
}

func DecodeWorkspaceInspectOutputV1(raw []byte) (WorkspaceInspectOutputV1, error) {
	var value WorkspaceInspectOutputV1
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return WorkspaceInspectOutputV1{}, err
	}
	return SealWorkspaceInspectOutputV1(value)
}

type WorkspacePatchOutputV1 struct {
	ChangeSetRef    ObjectRef            `json:"change_set_ref"`
	BaseWorkspace   WorkspaceExactRefV1  `json:"base_workspace"`
	ResultWorkspace WorkspaceExactRefV1  `json:"result_workspace"`
	Files           []WorkspaceFileRefV1 `json:"files"`
	CheckedUnixNano int64                `json:"checked_unix_nano"`
	ExpiresUnixNano int64                `json:"expires_unix_nano"`
}

func (v WorkspacePatchOutputV1) ValidateCurrent(now time.Time) error {
	if v.ChangeSetRef.Validate() != nil || v.BaseWorkspace.Validate() != nil || v.ResultWorkspace.Validate() != nil ||
		v.BaseWorkspace.ID != v.ResultWorkspace.ID || v.ResultWorkspace.Revision <= v.BaseWorkspace.Revision || len(v.Files) == 0 || len(v.Files) > CoreToolMaxPatchChangesV1 {
		return invalid("workspace.patch output closure is invalid")
	}
	for i, file := range v.Files {
		if file.Validate() != nil || file.Revision != v.ResultWorkspace.Revision {
			return conflict("workspace.patch file does not bind result workspace")
		}
		if i > 0 && v.Files[i-1].Path >= file.Path {
			return invalid("workspace.patch file refs must be sorted and unique")
		}
	}
	return validateOutputWindowV1(v.CheckedUnixNano, v.ExpiresUnixNano, now)
}

func (v WorkspacePatchOutputV1) Clone() WorkspacePatchOutputV1 {
	out := v
	out.Files = append([]WorkspaceFileRefV1(nil), v.Files...)
	return out
}

func SealWorkspacePatchOutputV1(v WorkspacePatchOutputV1) (WorkspacePatchOutputV1, error) {
	v = v.Clone()
	sort.Slice(v.Files, func(i, j int) bool { return v.Files[i].Path < v.Files[j].Path })
	return v, v.ValidateCurrent(time.Unix(0, v.CheckedUnixNano))
}

func DecodeWorkspacePatchOutputV1(raw []byte) (WorkspacePatchOutputV1, error) {
	var value WorkspacePatchOutputV1
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return WorkspacePatchOutputV1{}, err
	}
	return SealWorkspacePatchOutputV1(value)
}

type ProcessExecOutputV1 struct {
	AttemptRef        ObjectRef  `json:"attempt_ref"`
	ExitCode          int32      `json:"exit_code"`
	Stdout            *string    `json:"stdout"`
	Stderr            *string    `json:"stderr"`
	StdoutArtifactRef *ObjectRef `json:"stdout_artifact_ref"`
	StderrArtifactRef *ObjectRef `json:"stderr_artifact_ref"`
	TimedOut          bool       `json:"timed_out"`
	CheckedUnixNano   int64      `json:"checked_unix_nano"`
	ExpiresUnixNano   int64      `json:"expires_unix_nano"`
}

func (v ProcessExecOutputV1) ValidateCurrent(now time.Time) error {
	if v.AttemptRef.Validate() != nil || (v.Stdout == nil) == (v.StdoutArtifactRef == nil) || (v.Stderr == nil) == (v.StderrArtifactRef == nil) {
		return invalid("process.exec output requires exact Attempt and one representation per stream")
	}
	if v.Stdout != nil && uint64(len(*v.Stdout)) > CoreToolMaxOutputBytesV1 || v.Stderr != nil && uint64(len(*v.Stderr)) > CoreToolMaxOutputBytesV1 ||
		v.StdoutArtifactRef != nil && v.StdoutArtifactRef.Validate() != nil || v.StderrArtifactRef != nil && v.StderrArtifactRef.Validate() != nil {
		return invalid("process.exec output stream is invalid or exceeds cap")
	}
	return validateOutputWindowV1(v.CheckedUnixNano, v.ExpiresUnixNano, now)
}

func (v ProcessExecOutputV1) Clone() ProcessExecOutputV1 {
	out := v
	if v.Stdout != nil {
		value := *v.Stdout
		out.Stdout = &value
	}
	if v.Stderr != nil {
		value := *v.Stderr
		out.Stderr = &value
	}
	if v.StdoutArtifactRef != nil {
		value := *v.StdoutArtifactRef
		out.StdoutArtifactRef = &value
	}
	if v.StderrArtifactRef != nil {
		value := *v.StderrArtifactRef
		out.StderrArtifactRef = &value
	}
	return out
}

func SealProcessExecOutputV1(v ProcessExecOutputV1) (ProcessExecOutputV1, error) {
	v = v.Clone()
	return v, v.ValidateCurrent(time.Unix(0, v.CheckedUnixNano))
}

func DecodeProcessExecOutputV1(raw []byte) (ProcessExecOutputV1, error) {
	var value ProcessExecOutputV1
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return ProcessExecOutputV1{}, err
	}
	return SealProcessExecOutputV1(value)
}

func validateOutputWindowV1(checked, expires int64, now time.Time) error {
	if now.IsZero() || checked <= 0 || expires <= checked || now.UnixNano() < checked || !now.Before(time.Unix(0, expires)) {
		return invalid("core tool output currentness window is invalid or expired")
	}
	return nil
}
