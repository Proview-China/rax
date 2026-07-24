// Package workspacefs implements the Sandbox-owned local Base/Overlay capture
// driver. Capture writes immutable blobs and staged ChangeSets; the separately
// governed commit actual point is implemented by the Rust Data Plane.
package workspacefs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type BindingV1 struct {
	ViewRef     contract.Ref
	BaseRoot    string
	OverlayRoot string
	BlobRoot    string
}

type LimitsV1 struct {
	MaxFiles     int
	MaxTotalByte int64
	MaxFileByte  int64
}

type DriverV1 struct {
	bindings map[string]BindingV1
	limits   LimitsV1
	clock    func() time.Time
	afterS1  func()
}

func NewDriverV1(bindings []BindingV1, limits LimitsV1, clock func() time.Time) (*DriverV1, error) {
	if len(bindings) == 0 || limits.MaxFiles <= 0 || limits.MaxTotalByte <= 0 || limits.MaxFileByte <= 0 || limits.MaxFileByte > limits.MaxTotalByte || clock == nil {
		return nil, errors.New("workspace file driver bindings, limits, and clock are required")
	}
	result := &DriverV1{bindings: make(map[string]BindingV1, len(bindings)), limits: limits, clock: clock}
	for _, binding := range bindings {
		if err := binding.ViewRef.ValidateShape("workspace binding view ref"); err != nil {
			return nil, err
		}
		if _, exists := result.bindings[binding.ViewRef.ID]; exists {
			return nil, errors.New("workspace view binding ID is duplicated")
		}
		if err := validateRoot(binding.BaseRoot, false); err != nil {
			return nil, fmt.Errorf("base root: %w", err)
		}
		if err := validateRoot(binding.OverlayRoot, false); err != nil {
			return nil, fmt.Errorf("overlay root: %w", err)
		}
		if err := validateRoot(binding.BlobRoot, true); err != nil {
			return nil, fmt.Errorf("blob root: %w", err)
		}
		base, _ := filepath.EvalSymlinks(binding.BaseRoot)
		overlay, _ := filepath.EvalSymlinks(binding.OverlayRoot)
		blobs, _ := filepath.EvalSymlinks(binding.BlobRoot)
		if base == overlay || base == blobs || overlay == blobs || pathContains(base, overlay) || pathContains(overlay, base) || pathContains(base, blobs) || pathContains(blobs, base) || pathContains(overlay, blobs) || pathContains(blobs, overlay) {
			return nil, errors.New("workspace roots must be separate and non-nested")
		}
		binding.BaseRoot, binding.OverlayRoot, binding.BlobRoot = base, overlay, blobs
		result.bindings[binding.ViewRef.ID] = binding
	}
	return result, nil
}

var _ ports.WorkspaceChangeSetCapturePortV1 = (*DriverV1)(nil)

func (d *DriverV1) InspectWorkspaceBaseRevisionV1(ctx context.Context, view contract.WorkspaceView) (string, error) {
	binding, ok := d.bindings[view.Meta.ID]
	if !ok || !contract.SameRef(binding.ViewRef, view.Meta.Ref()) {
		return "", errors.New("workspace view has no exact filesystem binding")
	}
	tree, err := d.readTree(ctx, binding.BaseRoot, view, false)
	if err != nil {
		return "", err
	}
	return tree.revision, nil
}

func (d *DriverV1) CaptureWorkspaceChangeSetV1(ctx context.Context, request ports.CaptureWorkspaceChangeSetRequest) (contract.WorkspaceChangeSet, error) {
	now := d.clock()
	if request.ChangeSetID == "" || request.RequestedNotAfter.IsZero() || now.IsZero() {
		return contract.WorkspaceChangeSet{}, errors.New("workspace capture coordinates are required")
	}
	if err := request.View.ValidateCurrent(now); err != nil {
		return contract.WorkspaceChangeSet{}, fmt.Errorf("workspace view: %w", err)
	}
	binding, ok := d.bindings[request.View.Meta.ID]
	if !ok || !contract.SameRef(binding.ViewRef, request.View.Meta.Ref()) {
		return contract.WorkspaceChangeSet{}, errors.New("workspace view has no exact filesystem binding")
	}
	baseS1, err := d.readTree(ctx, binding.BaseRoot, request.View, false)
	if err != nil {
		return contract.WorkspaceChangeSet{}, fmt.Errorf("base S1: %w", err)
	}
	if baseS1.revision != request.View.BaseRevision {
		return contract.WorkspaceChangeSet{}, errors.New("workspace base revision drifted")
	}
	overlayS1, err := d.readTree(ctx, binding.OverlayRoot, request.View, true)
	if err != nil {
		return contract.WorkspaceChangeSet{}, fmt.Errorf("overlay S1: %w", err)
	}
	if d.afterS1 != nil {
		d.afterS1()
	}
	changes, blobs, err := buildChanges(baseS1, overlayS1, request.View)
	if err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	if len(changes) == 0 {
		return contract.WorkspaceChangeSet{}, errors.New("workspace overlay has no changes")
	}
	for _, blob := range blobs {
		if err := writeBlobCreateOnce(binding.BlobRoot, blob); err != nil {
			return contract.WorkspaceChangeSet{}, err
		}
	}
	baseS2, err := d.readTree(ctx, binding.BaseRoot, request.View, false)
	if err != nil || baseS2.revision != baseS1.revision {
		return contract.WorkspaceChangeSet{}, errors.New("workspace base changed during capture")
	}
	overlayS2, err := d.readTree(ctx, binding.OverlayRoot, request.View, true)
	if err != nil || overlayS2.revision != overlayS1.revision {
		return contract.WorkspaceChangeSet{}, errors.New("workspace overlay changed during capture")
	}
	return kernel.StageWorkspaceChangeSet(now, request.RequestedNotAfter, request.ChangeSetID, request.View, changes)
}

type fileRecord struct {
	path    string
	mode    fs.FileMode
	content []byte
	digest  string
}

type treeSnapshot struct {
	files    map[string]fileRecord
	revision string
}

func (d *DriverV1) readTree(ctx context.Context, root string, view contract.WorkspaceView, rejectHidden bool) (treeSnapshot, error) {
	records := make(map[string]fileRecord)
	var total int64
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if current == root {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		logical := filepath.ToSlash(relative)
		if err := contract.ValidateLogicalPath(logical); err != nil {
			return err
		}
		if within(logical, view.HiddenScopes) {
			if rejectHidden {
				return errors.New("hidden workspace path is materialized in the overlay")
			}
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("workspace path %q is a symlink or special file", logical)
		}
		if info.IsDir() {
			return nil
		}
		if !within(logical, view.ReadScopes) && !within(logical, view.WriteScopes) {
			return fmt.Errorf("workspace path %q is outside the view", logical)
		}
		if len(records) >= d.limits.MaxFiles || info.Size() > d.limits.MaxFileByte || total+info.Size() > d.limits.MaxTotalByte {
			return errors.New("workspace capture exceeds configured file limits")
		}
		content, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		if int64(len(content)) != info.Size() {
			return errors.New("workspace file changed while being read")
		}
		total += int64(len(content))
		digest := sha256.Sum256(content)
		records[logical] = fileRecord{path: logical, mode: info.Mode().Perm(), content: content, digest: hex.EncodeToString(digest[:])}
		return nil
	})
	if err != nil {
		return treeSnapshot{}, err
	}
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	type treeEntry struct {
		Path   string `json:"path"`
		Mode   uint32 `json:"mode"`
		Digest string `json:"digest"`
		Length int    `json:"length"`
	}
	entries := make([]treeEntry, 0, len(keys))
	for _, key := range keys {
		record := records[key]
		entries = append(entries, treeEntry{Path: key, Mode: uint32(record.mode), Digest: record.digest, Length: len(record.content)})
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return treeSnapshot{}, err
	}
	digest := sha256.Sum256(encoded)
	return treeSnapshot{files: records, revision: "sha256:" + hex.EncodeToString(digest[:])}, nil
}

type blobRecord struct {
	ref     contract.Ref
	content []byte
	mode    fs.FileMode
}

func buildChanges(base, overlay treeSnapshot, view contract.WorkspaceView) ([]contract.WorkspaceChange, []blobRecord, error) {
	pathSet := make(map[string]struct{}, len(base.files)+len(overlay.files))
	for value := range base.files {
		pathSet[value] = struct{}{}
	}
	for value := range overlay.files {
		pathSet[value] = struct{}{}
	}
	paths := make([]string, 0, len(pathSet))
	for value := range pathSet {
		paths = append(paths, value)
	}
	sort.Strings(paths)
	changes := make([]contract.WorkspaceChange, 0)
	blobs := make([]blobRecord, 0)
	for _, value := range paths {
		before, hadBefore := base.files[value]
		after, hasAfter := overlay.files[value]
		if hadBefore && hasAfter && before.digest == after.digest && before.mode == after.mode {
			continue
		}
		if !within(value, view.WriteScopes) || within(value, view.HiddenScopes) {
			return nil, nil, fmt.Errorf("read-only or hidden path %q changed", value)
		}
		switch {
		case hadBefore && !hasAfter:
			changes = append(changes, contract.WorkspaceChange{Kind: contract.WorkspaceDelete, Path: value})
		case !hadBefore && hasAfter:
			blob, err := makeBlob(after)
			if err != nil {
				return nil, nil, err
			}
			changes = append(changes, contract.WorkspaceChange{Kind: contract.WorkspaceAdd, Path: value, BlobRef: &blob.ref})
			blobs = append(blobs, blob)
		case hadBefore && hasAfter:
			blob, err := makeBlob(after)
			if err != nil {
				return nil, nil, err
			}
			changes = append(changes, contract.WorkspaceChange{Kind: contract.WorkspaceModify, Path: value, BlobRef: &blob.ref})
			blobs = append(blobs, blob)
		}
	}
	return changes, blobs, nil
}

func makeBlob(record fileRecord) (blobRecord, error) {
	descriptor := struct {
		ContentDigest string `json:"content_digest"`
		Length        int    `json:"length"`
		Mode          uint32 `json:"mode"`
	}{ContentDigest: record.digest, Length: len(record.content), Mode: uint32(record.mode)}
	digest, err := contract.Digest("workspace-file-blob-v1", descriptor)
	if err != nil {
		return blobRecord{}, err
	}
	return blobRecord{ref: contract.Ref{ID: "workspace-blob-" + record.digest, Revision: 1, Digest: digest}, content: slices.Clone(record.content), mode: record.mode}, nil
}

func writeBlobCreateOnce(root string, blob blobRecord) error {
	name := strings.TrimPrefix(blob.ref.ID, "workspace-blob-")
	if len(name) != sha256.Size*2 {
		return errors.New("workspace blob identity is invalid")
	}
	path := filepath.Join(root, name+".blob")
	temporary, err := os.CreateTemp(root, ".workspace-blob-*.next")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err = temporary.Write(blob.content); err == nil {
		err = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Link(temporaryPath, path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
		return errors.New("workspace blob create-once path is not a regular owned file")
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(existing, blob.content) {
		return errors.New("workspace blob create-once identity conflicts")
	}
	return nil
}

func validateRoot(value string, create bool) error {
	if !filepath.IsAbs(value) {
		return errors.New("workspace driver root must be absolute")
	}
	if create {
		if err := os.MkdirAll(value, 0o700); err != nil {
			return err
		}
	}
	info, err := os.Lstat(value)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return errors.New("workspace driver root must be a real directory")
	}
	return nil
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func within(value string, scopes []string) bool {
	for _, scope := range scopes {
		if value == scope || strings.HasPrefix(value, scope+"/") {
			return true
		}
	}
	return false
}
