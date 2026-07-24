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
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

const checkpointRuntimeQueryVersionV2 = "praxis.sandbox/checkpoint-current-query/v1"

type CheckpointWorkspaceArtifactReaderConfigV2 struct {
	CheckpointStoreRoot string
	Clock               func() time.Time
}

type CheckpointWorkspaceArtifactReaderV2 struct {
	root  string
	clock func() time.Time
}

type checkpointArtifactRecordV2 struct {
	ContractVersion                   string `json:"contract_version"`
	SubjectDigest                     string `json:"subject_digest"`
	CheckpointAttemptID               string `json:"checkpoint_attempt_id"`
	ParticipantID                     string `json:"participant_id"`
	PrepareReservationID              string `json:"prepare_reservation_id"`
	PrepareReservationRevision        uint64 `json:"prepare_reservation_revision"`
	PrepareReservationDigest          string `json:"prepare_reservation_digest"`
	PrepareReservationExpiresUnixNano int64  `json:"prepare_reservation_expires_unix_nano"`
	SourceDigest                      string `json:"source_digest"`
	ContentDigest                     string `json:"content_digest"`
	ContentLength                     uint64 `json:"content_length"`
	State                             string `json:"state"`
	OperationDigest                   string `json:"operation_digest"`
	DispatchAttemptID                 string `json:"dispatch_attempt_id"`
	RecordedUnixNano                  int64  `json:"recorded_unix_nano"`
	ExpiresUnixNano                   int64  `json:"expires_unix_nano"`
}

func NewCheckpointWorkspaceArtifactReaderV2(config CheckpointWorkspaceArtifactReaderConfigV2) (*CheckpointWorkspaceArtifactReaderV2, error) {
	if config.Clock == nil {
		return nil, errors.New("checkpoint workspace artifact reader clock is required")
	}
	root, err := validateTrustedDirectoryV1(config.CheckpointStoreRoot, false)
	if err != nil {
		return nil, fmt.Errorf("checkpoint store root: %w", err)
	}
	return &CheckpointWorkspaceArtifactReaderV2{root: root, clock: config.Clock}, nil
}

func (r *CheckpointWorkspaceArtifactReaderV2) InspectCheckpointWorkspaceArtifactV2(ctx context.Context, input *contract.InspectCheckpointWorkspaceArtifactRequestV2) (contract.CheckpointWorkspaceArtifactInspectionV2, error) {
	if input == nil {
		return contract.CheckpointWorkspaceArtifactInspectionV2{}, errors.New("checkpoint workspace artifact request is required")
	}
	request := *input
	now := r.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.CheckpointWorkspaceArtifactInspectionV2{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.CheckpointWorkspaceArtifactInspectionV2{}, err
	}
	subject := strings.TrimPrefix(request.Observation.SubjectDigest, "sha256:")
	if len(subject) != contract.DigestSizeHex || strings.ContainsAny(subject, `/\\`) {
		return contract.CheckpointWorkspaceArtifactInspectionV2{}, fmt.Errorf("%w: checkpoint subject is non-canonical", ports.ErrConflict)
	}
	artifactRoot := filepath.Join(r.root, "artifacts", subject)
	recordPath := filepath.Join(artifactRoot, "current.json")
	recordS1, err := inspectCheckpointArtifactRecordV2(recordPath, request.Observation, now)
	if err != nil {
		return contract.CheckpointWorkspaceArtifactInspectionV2{}, err
	}
	contentRoot := filepath.Join(artifactRoot, "staging")
	manifestS1, lengthS1, err := inspectCheckpointContentManifestV2(ctx, contentRoot)
	if err != nil {
		return contract.CheckpointWorkspaceArtifactInspectionV2{}, err
	}
	if manifestS1 != request.Observation.ContentDigest || lengthS1 != request.Observation.ContentLength {
		return contract.CheckpointWorkspaceArtifactInspectionV2{}, fmt.Errorf("%w: checkpoint content differs from Provider observation", ports.ErrConflict)
	}
	bundle, err := captureCheckpointWorkspaceBundleV2(ctx, contentRoot, request)
	if err != nil {
		return contract.CheckpointWorkspaceArtifactInspectionV2{}, err
	}
	recordS2, err := inspectCheckpointArtifactRecordV2(recordPath, request.Observation, r.clock())
	if err != nil || recordS2 != recordS1 {
		return contract.CheckpointWorkspaceArtifactInspectionV2{}, fmt.Errorf("%w: checkpoint artifact record drifted during inspection", ports.ErrConflict)
	}
	manifestS2, lengthS2, err := inspectCheckpointContentManifestV2(ctx, contentRoot)
	if err != nil || manifestS2 != manifestS1 || lengthS2 != lengthS1 {
		return contract.CheckpointWorkspaceArtifactInspectionV2{}, fmt.Errorf("%w: checkpoint content drifted during inspection", ports.ErrConflict)
	}
	fresh := r.clock()
	if !fresh.Before(time.Unix(0, request.Observation.ExpiresUnixNano)) {
		return contract.CheckpointWorkspaceArtifactInspectionV2{}, ports.ErrStale
	}
	return contract.SealCheckpointWorkspaceArtifactInspectionV2(contract.CheckpointWorkspaceArtifactInspectionV2{
		Observation: request.Observation, Bundle: bundle, CheckedUnixNano: fresh.UnixNano(), ExpiresUnixNano: request.Observation.ExpiresUnixNano,
	}, fresh)
}

func inspectCheckpointArtifactRecordV2(path string, expected contract.CheckpointWorkspaceArtifactObservationV2, now time.Time) (checkpointArtifactRecordV2, error) {
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return checkpointArtifactRecordV2{}, ports.ErrNotFound
	}
	if err != nil {
		return checkpointArtifactRecordV2{}, err
	}
	record, err := contract.DecodeStrict[checkpointArtifactRecordV2](payload)
	if err != nil {
		return checkpointArtifactRecordV2{}, fmt.Errorf("%w: checkpoint artifact record is invalid: %v", ports.ErrConflict, err)
	}
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(payload, canonical) {
		return checkpointArtifactRecordV2{}, fmt.Errorf("%w: checkpoint artifact record is not canonical", ports.ErrConflict)
	}
	if record.ContractVersion != checkpointRuntimeQueryVersionV2 || record.SubjectDigest != strings.TrimPrefix(expected.SubjectDigest, "sha256:") || record.ContentDigest != expected.ContentDigest || record.ContentLength != expected.ContentLength || record.State != expected.State || record.RecordedUnixNano != expected.RecordedUnixNano || record.ExpiresUnixNano != expected.ExpiresUnixNano || record.ExpiresUnixNano <= now.UnixNano() || !contract.ValidDigest(record.PrepareReservationDigest) || !contract.ValidDigest(record.SourceDigest) || !contract.ValidDigest(record.OperationDigest) || record.PrepareReservationID == "" || record.PrepareReservationRevision == 0 || record.PrepareReservationExpiresUnixNano <= now.UnixNano() || record.CheckpointAttemptID == "" || record.ParticipantID == "" || record.DispatchAttemptID == "" {
		return checkpointArtifactRecordV2{}, fmt.Errorf("%w: checkpoint artifact record differs from exact observation", ports.ErrConflict)
	}
	return record, nil
}

func inspectCheckpointContentManifestV2(ctx context.Context, root string) (string, uint64, error) {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		if errors.Is(err, os.ErrNotExist) {
			return "", 0, ports.ErrNotFound
		}
		return "", 0, fmt.Errorf("%w: checkpoint staging root is invalid", ports.ErrConflict)
	}
	entries := map[string]string{}
	var length uint64
	rootFile, err := os.OpenRoot(root)
	if err != nil {
		return "", 0, err
	}
	defer rootFile.Close()
	err = filepath.WalkDir(root, func(absolute string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if absolute == root {
			return nil
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("checkpoint staging contains a symlink")
		}
		switch {
		case info.IsDir():
			entries[relative+"/"] = "directory"
		case info.Mode().IsRegular():
			content, err := readWorkspaceRegularNoFollowV1(rootFile, relative, info)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(content)
			executable := 0
			if info.Mode().Perm()&0o111 != 0 {
				executable = 1
			}
			entries[relative] = fmt.Sprintf("file:sha256:%s:executable=%d", hex.EncodeToString(sum[:]), executable)
			if length > ^uint64(0)-uint64(len(content)) {
				return errors.New("checkpoint content length overflow")
			}
			length += uint64(len(content))
		default:
			return errors.New("checkpoint staging contains a special file")
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	payload, err := json.Marshal(entries)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	hash.Write([]byte("praxis.sandbox/data-plane-ipc/v1"))
	hash.Write([]byte{0})
	hash.Write([]byte("CheckpointArtifactContentV1"))
	hash.Write([]byte{0})
	hash.Write(payload)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), length, nil
}

func captureCheckpointWorkspaceBundleV2(ctx context.Context, root string, request contract.InspectCheckpointWorkspaceArtifactRequestV2) (contract.WorkspaceSnapshotBundleV1, error) {
	rootFile, err := os.OpenRoot(root)
	if err != nil {
		return contract.WorkspaceSnapshotBundleV1{}, err
	}
	defer rootFile.Close()
	bundle := contract.WorkspaceSnapshotBundleV1{SnapshotID: request.SnapshotID, TenantID: request.TenantID, SourceScopeDigest: strings.TrimPrefix(request.SourceScopeDigest, "sha256:")}
	err = filepath.WalkDir(root, func(absolute string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if absolute == root {
			return nil
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			bundle.Entries = append(bundle.Entries, contract.WorkspaceSnapshotEntryV1{Path: relative, Kind: contract.WorkspaceSnapshotDirectory})
		case info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0:
			content, err := readWorkspaceRegularNoFollowV1(rootFile, relative, info)
			if err != nil {
				return err
			}
			bundle.Entries = append(bundle.Entries, contract.WorkspaceSnapshotEntryV1{Path: relative, Kind: contract.WorkspaceSnapshotRegularFile, Executable: info.Mode().Perm()&0o111 != 0, Content: content})
		default:
			return errors.New("checkpoint workspace artifact contains unsupported content")
		}
		return nil
	})
	if err != nil {
		return contract.WorkspaceSnapshotBundleV1{}, err
	}
	return contract.SealWorkspaceSnapshotBundleV1(bundle)
}

var _ ports.CheckpointWorkspaceArtifactReaderV2 = (*CheckpointWorkspaceArtifactReaderV2)(nil)
