package hostlocal

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
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

const workspaceRestoreIndexVersionV1 = "praxis.sandbox/host-local-workspace-restore-index/v1"

type WorkspaceCaptureConfigV1 struct {
	SourceRoot string
}

type WorkspaceCaptureRequestV1 struct {
	SnapshotID        string
	TenantID          string
	SourceScopeDigest string
}

type WorkspaceCaptureV1 struct {
	sourceRoot string
}

func NewWorkspaceCaptureV1(config WorkspaceCaptureConfigV1) (*WorkspaceCaptureV1, error) {
	root, err := validateTrustedDirectoryV1(config.SourceRoot, false)
	if err != nil {
		return nil, fmt.Errorf("workspace capture source root: %w", err)
	}
	return &WorkspaceCaptureV1{sourceRoot: root}, nil
}

func (c *WorkspaceCaptureV1) Capture(ctx context.Context, input *WorkspaceCaptureRequestV1) (contract.WorkspaceSnapshotBundleV1, error) {
	if input == nil {
		return contract.WorkspaceSnapshotBundleV1{}, errors.New("workspace capture request is required")
	}
	if err := ctx.Err(); err != nil {
		return contract.WorkspaceSnapshotBundleV1{}, err
	}
	rootFile, err := os.OpenRoot(c.sourceRoot)
	if err != nil {
		return contract.WorkspaceSnapshotBundleV1{}, err
	}
	defer rootFile.Close()

	bundle := contract.WorkspaceSnapshotBundleV1{
		SnapshotID: input.SnapshotID, TenantID: input.TenantID, SourceScopeDigest: input.SourceScopeDigest,
	}
	err = filepath.WalkDir(c.sourceRoot, func(absolute string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if absolute == c.sourceRoot {
			return walkErr
		}
		relative, relErr := filepath.Rel(c.sourceRoot, absolute)
		if relErr != nil {
			return relErr
		}
		relative = filepath.ToSlash(relative)
		if walkErr != nil {
			bundle.Excluded = append(bundle.Excluded, contract.WorkspaceSnapshotExcludedV1{Path: relative, Kind: contract.WorkspaceSnapshotExcludedSpecial, Reason: contract.WorkspaceSnapshotResidualUnreadable})
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			bundle.Excluded = append(bundle.Excluded, contract.WorkspaceSnapshotExcludedV1{Path: relative, Kind: contract.WorkspaceSnapshotExcludedSpecial, Reason: contract.WorkspaceSnapshotResidualUnreadable})
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		mode := info.Mode()
		switch {
		case mode.IsDir():
			if isNestedSubmoduleV1(absolute) {
				bundle.Excluded = append(bundle.Excluded, contract.WorkspaceSnapshotExcludedV1{Path: relative, Kind: contract.WorkspaceSnapshotExcludedSubmodule, Reason: contract.WorkspaceSnapshotResidualUnsupportedKind})
				return fs.SkipDir
			}
			bundle.Entries = append(bundle.Entries, contract.WorkspaceSnapshotEntryV1{Path: relative, Kind: contract.WorkspaceSnapshotDirectory})
			return nil
		case mode.IsRegular():
			content, err := readWorkspaceRegularNoFollowV1(rootFile, relative, info)
			if err != nil {
				bundle.Excluded = append(bundle.Excluded, contract.WorkspaceSnapshotExcludedV1{Path: relative, Kind: contract.WorkspaceSnapshotExcludedSpecial, Reason: contract.WorkspaceSnapshotResidualUnreadable})
				return nil
			}
			bundle.Entries = append(bundle.Entries, contract.WorkspaceSnapshotEntryV1{Path: relative, Kind: contract.WorkspaceSnapshotRegularFile, Executable: mode.Perm()&0o111 != 0, Content: content})
			return nil
		case mode&os.ModeSymlink != 0:
			bundle.Excluded = append(bundle.Excluded, contract.WorkspaceSnapshotExcludedV1{Path: relative, Kind: contract.WorkspaceSnapshotExcludedSymlink, Reason: contract.WorkspaceSnapshotResidualUnsupportedKind})
		case mode&os.ModeSocket != 0:
			bundle.Excluded = append(bundle.Excluded, contract.WorkspaceSnapshotExcludedV1{Path: relative, Kind: contract.WorkspaceSnapshotExcludedSocket, Reason: contract.WorkspaceSnapshotResidualUnsupportedKind})
		case mode&os.ModeNamedPipe != 0:
			bundle.Excluded = append(bundle.Excluded, contract.WorkspaceSnapshotExcludedV1{Path: relative, Kind: contract.WorkspaceSnapshotExcludedFIFO, Reason: contract.WorkspaceSnapshotResidualUnsupportedKind})
		case mode&os.ModeDevice != 0:
			bundle.Excluded = append(bundle.Excluded, contract.WorkspaceSnapshotExcludedV1{Path: relative, Kind: contract.WorkspaceSnapshotExcludedDevice, Reason: contract.WorkspaceSnapshotResidualUnsupportedKind})
		default:
			bundle.Excluded = append(bundle.Excluded, contract.WorkspaceSnapshotExcludedV1{Path: relative, Kind: contract.WorkspaceSnapshotExcludedSpecial, Reason: contract.WorkspaceSnapshotResidualUnsupportedKind})
		}
		return nil
	})
	if err != nil {
		return contract.WorkspaceSnapshotBundleV1{}, err
	}
	return contract.SealWorkspaceSnapshotBundleV1(bundle)
}

type WorkspaceStageConfigV1 struct {
	RootParent string
	Clock      func() time.Time
}

type WorkspaceStageRequestV1 = contract.WorkspaceRestoreProviderRequestV1

type WorkspaceStageResultV1 = contract.WorkspaceRestoreProviderResultV1

type WorkspaceInspectRequestV1 struct {
	ExpectedRootRef contract.WorkspaceRootRefV1
	ExpectedBundle  contract.WorkspaceSnapshotBundleV1
}

type WorkspaceInspectResultV1 struct {
	RootRef contract.WorkspaceRootRefV1
}

type WorkspaceStageV1 struct {
	rootParent string
	clock      func() time.Time
}

type workspaceRestoreIndexV1 struct {
	ContractVersion string                      `json:"contract_version"`
	IdentityDigest  string                      `json:"identity_digest"`
	RootRef         contract.WorkspaceRootRefV1 `json:"root_ref"`
}

type workspaceRestoreMarkerV1 struct {
	ContractVersion string                      `json:"contract_version"`
	RootRef         contract.WorkspaceRootRefV1 `json:"root_ref"`
	EntrySetDigest  string                      `json:"entry_set_digest"`
}

func NewWorkspaceStageV1(config WorkspaceStageConfigV1) (*WorkspaceStageV1, error) {
	if config.Clock == nil {
		return nil, errors.New("workspace stage clock is required")
	}
	root, err := validateTrustedDirectoryV1(config.RootParent, true)
	if err != nil {
		return nil, fmt.Errorf("workspace stage root parent: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".restore-index"), 0o700); err != nil {
		return nil, err
	}
	return &WorkspaceStageV1{rootParent: root, clock: config.Clock}, nil
}

func (s *WorkspaceStageV1) Stage(ctx context.Context, input *WorkspaceStageRequestV1) (WorkspaceStageResultV1, error) {
	if input == nil {
		return WorkspaceStageResultV1{}, errors.New("workspace stage request is required")
	}
	request := input.Clone()
	now := s.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return WorkspaceStageResultV1{}, err
	}
	if request.Bundle.TenantID != request.Target.TenantID {
		return WorkspaceStageResultV1{}, errors.New("workspace bundle tenant does not match target lease")
	}
	if err := ctx.Err(); err != nil {
		return WorkspaceStageResultV1{}, err
	}

	identityDigest, rootRef, err := workspaceRestoreIdentityV1(request)
	if err != nil {
		return WorkspaceStageResultV1{}, err
	}
	index := workspaceRestoreIndexV1{ContractVersion: workspaceRestoreIndexVersionV1, IdentityDigest: identityDigest, RootRef: rootRef}
	indexBytes, err := json.Marshal(index)
	if err != nil {
		return WorkspaceStageResultV1{}, err
	}
	indexPath := filepath.Join(s.rootParent, ".restore-index", identityDigest+".json")
	createdIndex, err := writeCreateOnceV2(indexPath, indexBytes)
	if err != nil {
		return WorkspaceStageResultV1{}, err
	}
	if !createdIndex {
		existing, err := readWorkspaceRestoreIndexV1(indexPath)
		if err != nil || existing != index {
			return WorkspaceStageResultV1{}, fmt.Errorf("%w: workspace restore identity is bound to different exact content", ports.ErrConflict)
		}
	}

	if inspected, err := s.Inspect(ctx, &WorkspaceInspectRequestV1{ExpectedRootRef: rootRef, ExpectedBundle: request.Bundle}); err == nil {
		return WorkspaceStageResultV1{RootRef: inspected.RootRef, Created: false}, nil
	} else if !errors.Is(err, ports.ErrNotFound) {
		return WorkspaceStageResultV1{}, err
	}

	temporary, err := os.MkdirTemp(s.rootParent, ".restore-*.tmp")
	if err != nil {
		return WorkspaceStageResultV1{}, err
	}
	defer os.RemoveAll(temporary)
	if err := os.Chmod(temporary, 0o700); err != nil {
		return WorkspaceStageResultV1{}, err
	}
	if err := materializeWorkspaceBundleV1(temporary, rootRef, request.Bundle); err != nil {
		return WorkspaceStageResultV1{}, err
	}
	finalPath, err := s.rootPath(rootRef)
	if err != nil {
		return WorkspaceStageResultV1{}, err
	}
	if err := os.Rename(temporary, finalPath); err != nil {
		if _, inspectErr := s.Inspect(ctx, &WorkspaceInspectRequestV1{ExpectedRootRef: rootRef, ExpectedBundle: request.Bundle}); inspectErr == nil {
			return WorkspaceStageResultV1{RootRef: rootRef, Created: false}, nil
		}
		return WorkspaceStageResultV1{}, err
	}
	if err := syncDirectoryV1(s.rootParent); err != nil {
		return WorkspaceStageResultV1{}, err
	}
	inspected, err := s.Inspect(ctx, &WorkspaceInspectRequestV1{ExpectedRootRef: rootRef, ExpectedBundle: request.Bundle})
	if err != nil {
		return WorkspaceStageResultV1{}, err
	}
	return WorkspaceStageResultV1{RootRef: inspected.RootRef, Created: true}, nil
}

func (s *WorkspaceStageV1) Inspect(ctx context.Context, input *WorkspaceInspectRequestV1) (WorkspaceInspectResultV1, error) {
	if input == nil {
		return WorkspaceInspectResultV1{}, errors.New("workspace inspect request is required")
	}
	request := *input
	request.ExpectedBundle = input.ExpectedBundle.Clone()
	if err := request.ExpectedRootRef.ValidateShape(); err != nil {
		return WorkspaceInspectResultV1{}, err
	}
	if err := request.ExpectedBundle.ValidateShape(); err != nil {
		return WorkspaceInspectResultV1{}, err
	}
	if request.ExpectedRootRef.BundleDigest != request.ExpectedBundle.BundleDigest || request.ExpectedRootRef.TenantID != request.ExpectedBundle.TenantID {
		return WorkspaceInspectResultV1{}, fmt.Errorf("%w: expected workspace root and bundle do not bind", ports.ErrConflict)
	}
	if err := ctx.Err(); err != nil {
		return WorkspaceInspectResultV1{}, err
	}
	identityDigest, err := contract.Digest("praxis.sandbox/host-local-workspace-restore-identity/v1", struct {
		StageAttemptRef       contract.SnapshotArtifactExactRefV2
		RuntimeRestoreAttempt contract.SnapshotArtifactExactRefV2
		Target                contract.RuntimeLeaseBinding
	}{request.ExpectedRootRef.StageAttemptRef, request.ExpectedRootRef.RuntimeRestoreAttempt, request.ExpectedRootRef.Target})
	if err != nil {
		return WorkspaceInspectResultV1{}, err
	}
	indexPath := filepath.Join(s.rootParent, ".restore-index", identityDigest+".json")
	index, err := readWorkspaceRestoreIndexV1(indexPath)
	if errors.Is(err, os.ErrNotExist) {
		return WorkspaceInspectResultV1{}, ports.ErrNotFound
	}
	if err != nil || index.RootRef != request.ExpectedRootRef || index.IdentityDigest != identityDigest {
		return WorkspaceInspectResultV1{}, fmt.Errorf("%w: workspace restore index drifted", ports.ErrConflict)
	}
	rootPath, err := s.rootPath(request.ExpectedRootRef)
	if err != nil {
		return WorkspaceInspectResultV1{}, err
	}
	if _, err := os.Lstat(rootPath); errors.Is(err, os.ErrNotExist) {
		return WorkspaceInspectResultV1{}, ports.ErrNotFound
	} else if err != nil {
		return WorkspaceInspectResultV1{}, err
	}
	if err := inspectWorkspaceRootV1(rootPath, request.ExpectedRootRef, request.ExpectedBundle); err != nil {
		return WorkspaceInspectResultV1{}, err
	}
	return WorkspaceInspectResultV1{RootRef: request.ExpectedRootRef}, nil
}

func (s *WorkspaceStageV1) StageWorkspaceRestoreV1(ctx context.Context, input *contract.WorkspaceRestoreProviderRequestV1) (contract.WorkspaceRestoreProviderResultV1, error) {
	return s.Stage(ctx, input)
}

func (s *WorkspaceStageV1) InspectWorkspaceRestoreV1(ctx context.Context, input *contract.WorkspaceRestoreProviderRequestV1) (contract.WorkspaceRestoreProviderResultV1, error) {
	if input == nil {
		return contract.WorkspaceRestoreProviderResultV1{}, errors.New("workspace provider inspect request is required")
	}
	request := input.Clone()
	if err := request.ValidateShape(); err != nil {
		return contract.WorkspaceRestoreProviderResultV1{}, err
	}
	_, expected, err := workspaceRestoreIdentityV1(request)
	if err != nil {
		return contract.WorkspaceRestoreProviderResultV1{}, err
	}
	result, err := s.Inspect(ctx, &WorkspaceInspectRequestV1{ExpectedRootRef: expected, ExpectedBundle: request.Bundle})
	if err != nil {
		return contract.WorkspaceRestoreProviderResultV1{}, err
	}
	return contract.WorkspaceRestoreProviderResultV1{RootRef: result.RootRef, Created: false}, nil
}

func workspaceRestoreIdentityV1(request WorkspaceStageRequestV1) (string, contract.WorkspaceRootRefV1, error) {
	identityDigest, err := contract.Digest("praxis.sandbox/host-local-workspace-restore-identity/v1", struct {
		StageAttemptRef       contract.SnapshotArtifactExactRefV2
		RuntimeRestoreAttempt contract.SnapshotArtifactExactRefV2
		Target                contract.RuntimeLeaseBinding
	}{request.StageAttemptRef, request.RuntimeRestoreAttempt, request.Target})
	if err != nil {
		return "", contract.WorkspaceRootRefV1{}, err
	}
	id := "workspace-root-" + identityDigest[:24] + "-" + request.Bundle.BundleDigest[:12]
	rootRef, err := contract.SealWorkspaceRootRefV1(contract.WorkspaceRootRefV1{ID: id, TenantID: request.Target.TenantID, RestoreAttemptID: request.RuntimeRestoreAttempt.ID, RuntimeRestoreAttempt: request.RuntimeRestoreAttempt, StageAttemptRef: request.StageAttemptRef, Target: request.Target, BundleDigest: request.Bundle.BundleDigest})
	return identityDigest, rootRef, err
}

func materializeWorkspaceBundleV1(rootPath string, rootRef contract.WorkspaceRootRefV1, bundle contract.WorkspaceSnapshotBundleV1) error {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, entry := range bundle.Entries {
		if entry.Kind != contract.WorkspaceSnapshotDirectory {
			continue
		}
		if err := root.Mkdir(entry.Path, 0o700); err != nil {
			return err
		}
		if err := root.Chmod(entry.Path, 0o700); err != nil {
			return err
		}
	}
	for _, entry := range bundle.Entries {
		if entry.Kind != contract.WorkspaceSnapshotRegularFile {
			continue
		}
		mode := fs.FileMode(0o600)
		if entry.Executable {
			mode = 0o700
		}
		file, err := root.OpenFile(entry.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return err
		}
		if err := file.Chmod(mode); err != nil {
			file.Close()
			return err
		}
		if _, err := file.Write(entry.Content); err != nil {
			file.Close()
			return err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	marker := workspaceRestoreMarkerV1{ContractVersion: workspaceRestoreIndexVersionV1, RootRef: rootRef, EntrySetDigest: bundle.EntrySetDigest}
	markerBytes, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	markerFile, err := root.OpenFile(contract.WorkspaceRestoreMarkerPathV1, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := markerFile.Write(markerBytes); err != nil {
		markerFile.Close()
		return err
	}
	if err := markerFile.Sync(); err != nil {
		markerFile.Close()
		return err
	}
	if err := markerFile.Close(); err != nil {
		return err
	}
	directories := []string{"."}
	for _, entry := range bundle.Entries {
		if entry.Kind == contract.WorkspaceSnapshotDirectory {
			directories = append(directories, entry.Path)
		}
	}
	sort.Slice(directories, func(i, j int) bool { return strings.Count(directories[i], "/") > strings.Count(directories[j], "/") })
	for _, directory := range directories {
		file, err := root.Open(directory)
		if err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func inspectWorkspaceRootV1(rootPath string, rootRef contract.WorkspaceRootRefV1, bundle contract.WorkspaceSnapshotBundleV1) error {
	markerBytes, err := os.ReadFile(filepath.Join(rootPath, contract.WorkspaceRestoreMarkerPathV1))
	if err != nil {
		return fmt.Errorf("%w: workspace restore marker missing: %v", ports.ErrConflict, err)
	}
	marker, err := contract.DecodeStrict[workspaceRestoreMarkerV1](markerBytes)
	if err != nil || marker.ContractVersion != workspaceRestoreIndexVersionV1 || marker.RootRef != rootRef || marker.EntrySetDigest != bundle.EntrySetDigest {
		return fmt.Errorf("%w: workspace restore marker drifted", ports.ErrConflict)
	}
	canonicalMarker, _ := json.Marshal(marker)
	if !bytes.Equal(markerBytes, canonicalMarker) {
		return fmt.Errorf("%w: workspace restore marker is not canonical", ports.ErrConflict)
	}

	expected := make(map[string]contract.WorkspaceSnapshotEntryV1, len(bundle.Entries))
	for _, entry := range bundle.Entries {
		expected[entry.Path] = entry
	}
	seen := make(map[string]struct{}, len(expected))
	err = filepath.WalkDir(rootPath, func(absolute string, dirEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if absolute == rootPath {
			info, err := dirEntry.Info()
			if err != nil || info.Mode().Perm() != 0o700 {
				return fmt.Errorf("%w: workspace root mode drifted", ports.ErrConflict)
			}
			return nil
		}
		relative, err := filepath.Rel(rootPath, absolute)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == contract.WorkspaceRestoreMarkerPathV1 {
			return nil
		}
		entry, ok := expected[relative]
		if !ok {
			return fmt.Errorf("%w: unexpected workspace root entry %q", ports.ErrConflict, relative)
		}
		info, err := dirEntry.Info()
		if err != nil {
			return err
		}
		switch entry.Kind {
		case contract.WorkspaceSnapshotDirectory:
			if !info.IsDir() || info.Mode().Perm() != 0o700 {
				return fmt.Errorf("%w: workspace directory %q drifted", ports.ErrConflict, relative)
			}
		case contract.WorkspaceSnapshotRegularFile:
			mode := fs.FileMode(0o600)
			if entry.Executable {
				mode = 0o700
			}
			if !info.Mode().IsRegular() || info.Mode().Perm() != mode {
				return fmt.Errorf("%w: workspace file %q type or mode drifted", ports.ErrConflict, relative)
			}
			content, err := os.ReadFile(absolute)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(content)
			if uint64(len(content)) != entry.Length || hex.EncodeToString(digest[:]) != entry.ContentDigest {
				return fmt.Errorf("%w: workspace file %q content drifted", ports.ErrConflict, relative)
			}
		}
		seen[relative] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("%w: workspace root is missing expected entries", ports.ErrConflict)
	}
	return nil
}

func (s *WorkspaceStageV1) rootPath(ref contract.WorkspaceRootRefV1) (string, error) {
	if err := ref.ValidateShape(); err != nil {
		return "", err
	}
	if !strings.HasPrefix(ref.ID, "workspace-root-") || strings.ContainsAny(ref.ID, `/\\`) {
		return "", fmt.Errorf("%w: workspace root ref ID is non-canonical", ports.ErrConflict)
	}
	return filepath.Join(s.rootParent, ref.ID), nil
}

func readWorkspaceRestoreIndexV1(path string) (workspaceRestoreIndexV1, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return workspaceRestoreIndexV1{}, err
	}
	index, err := contract.DecodeStrict[workspaceRestoreIndexV1](payload)
	if err != nil {
		return workspaceRestoreIndexV1{}, err
	}
	canonical, err := json.Marshal(index)
	if err != nil || !bytes.Equal(payload, canonical) || index.ContractVersion != workspaceRestoreIndexVersionV1 || !contract.ValidDigest(index.IdentityDigest) || index.RootRef.ValidateShape() != nil {
		return workspaceRestoreIndexV1{}, fmt.Errorf("%w: workspace restore index is invalid", ports.ErrConflict)
	}
	return index, nil
}

func validateTrustedDirectoryV1(value string, create bool) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("trusted directory is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	if create {
		if err := os.MkdirAll(absolute, 0o700); err != nil {
			return "", err
		}
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("trusted path must be a real directory")
	}
	return absolute, nil
}

func isNestedSubmoduleV1(directory string) bool {
	info, err := os.Lstat(filepath.Join(directory, ".git"))
	return err == nil && info.Mode().IsRegular()
}

func syncDirectoryV1(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil {
		return err
	}
	return closeErr
}

var _ ports.WorkspaceRestoreProviderV1 = (*WorkspaceStageV1)(nil)
