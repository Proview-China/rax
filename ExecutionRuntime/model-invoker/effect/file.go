package effect

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

const (
	defaultMaxFileBytes    int64 = 16 << 20
	defaultMaxCaptureBytes int64 = 1 << 20
)

type FilePolicy struct {
	AllowedRoots    []string
	FollowSymlinks  bool
	MaxFileBytes    int64
	MaxCaptureBytes int64
	Redactor        ContentRedactor
}

type FileObserver struct {
	roots           []string
	followSymlinks  bool
	maxFileBytes    int64
	maxCaptureBytes int64
	redactor        ContentRedactor
}

type FileSnapshot struct {
	State           union.FileStateSnapshot
	ContentCaptured bool
	content         []byte
}

type FileExpectation struct {
	BeforeExists *bool
	BeforeHash   string
	AfterExists  *bool
	AfterHash    string
	AfterType    union.FileStateType
}

type FileValidation struct {
	Effect       union.EffectRecord
	Verification union.VerificationRecord
}

func NewFileObserver(policy FilePolicy) (*FileObserver, error) {
	if len(policy.AllowedRoots) == 0 {
		return nil, fmt.Errorf("%w: at least one allowed root is required", ErrInvalidPolicy)
	}
	roots := make([]string, 0, len(policy.AllowedRoots))
	seen := make(map[string]struct{}, len(policy.AllowedRoots))
	for _, root := range policy.AllowedRoots {
		if !filepath.IsAbs(root) {
			return nil, fmt.Errorf("%w: root %q is not absolute", ErrInvalidPolicy, root)
		}
		resolved, err := filepath.EvalSymlinks(filepath.Clean(root))
		if err != nil {
			return nil, fmt.Errorf("%w: resolve root %q: %v", ErrInvalidPolicy, root, err)
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("%w: root %q is not a directory", ErrInvalidPolicy, root)
		}
		if _, duplicate := seen[resolved]; duplicate {
			continue
		}
		seen[resolved] = struct{}{}
		roots = append(roots, resolved)
	}
	maxFileBytes := policy.MaxFileBytes
	if maxFileBytes == 0 {
		maxFileBytes = defaultMaxFileBytes
	}
	maxCaptureBytes := policy.MaxCaptureBytes
	if maxCaptureBytes == 0 {
		maxCaptureBytes = defaultMaxCaptureBytes
	}
	if maxFileBytes < 1 || maxCaptureBytes < 1 || maxCaptureBytes > maxFileBytes {
		return nil, fmt.Errorf("%w: invalid file or capture limit", ErrInvalidPolicy)
	}
	redactor := policy.Redactor
	if redactor == nil {
		redactor = passthroughRedactor{}
	}
	return &FileObserver{roots: roots, followSymlinks: policy.FollowSymlinks, maxFileBytes: maxFileBytes, maxCaptureBytes: maxCaptureBytes, redactor: redactor}, nil
}

func (observer *FileObserver) Capture(path string) (FileSnapshot, error) {
	if observer == nil || len(observer.roots) == 0 {
		return FileSnapshot{}, fmt.Errorf("%w: observer is not initialized", ErrInvalidPolicy)
	}
	canonical, err := observer.authorizePath(path)
	if err != nil {
		return FileSnapshot{}, err
	}
	state := union.FileStateSnapshot{Path: canonical}
	info, err := os.Lstat(canonical)
	if errors.Is(err, os.ErrNotExist) {
		state.Type = "absent"
		return FileSnapshot{State: state}, nil
	}
	if err != nil {
		return FileSnapshot{}, fmt.Errorf("snapshot %q: %w", canonical, err)
	}
	state.Exists = true
	state.Size = info.Size()
	state.Mode = uint32(info.Mode())
	state.ModifiedAt = info.ModTime().UTC()
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		state.Type = "symlink"
		target, readErr := os.Readlink(canonical)
		if readErr != nil {
			return FileSnapshot{}, fmt.Errorf("read symlink %q: %w", canonical, readErr)
		}
		state.Symlink = target
		state.Hash = digestBytes([]byte(target))
		if !observer.followSymlinks {
			return FileSnapshot{State: state}, nil
		}
		followed, statErr := os.Stat(canonical)
		if statErr != nil {
			return FileSnapshot{}, fmt.Errorf("follow symlink %q: %w", canonical, statErr)
		}
		info = followed
		state.Size, state.Mode, state.ModifiedAt = info.Size(), uint32(info.Mode()), info.ModTime().UTC()
	case info.IsDir():
		state.Type = "directory"
		return FileSnapshot{State: state}, nil
	case info.Mode().IsRegular():
		state.Type = "regular"
	default:
		state.Type = "other"
		return FileSnapshot{}, fmt.Errorf("%w: %q", ErrUnsupportedFileType, canonical)
	}
	if info.IsDir() {
		state.Type = "directory"
		return FileSnapshot{State: state}, nil
	}
	if !info.Mode().IsRegular() {
		return FileSnapshot{}, fmt.Errorf("%w: %q", ErrUnsupportedFileType, canonical)
	}
	state.Type = "regular"
	if info.Size() > observer.maxFileBytes {
		return FileSnapshot{}, fmt.Errorf("%w: %q is %d bytes", ErrFileTooLarge, canonical, info.Size())
	}
	payload, err := os.ReadFile(canonical)
	if err != nil {
		return FileSnapshot{}, fmt.Errorf("read snapshot %q: %w", canonical, err)
	}
	state.Hash = digestBytes(payload)
	snapshot := FileSnapshot{State: state}
	if int64(len(payload)) <= observer.maxCaptureBytes {
		snapshot.ContentCaptured = true
		snapshot.content = append([]byte(nil), payload...)
	}
	return snapshot, nil
}

func (observer *FileObserver) authorizePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: path %q is not absolute", ErrPathOutsideRoots, path)
	}
	clean := filepath.Clean(path)
	parent := clean
	if info, err := os.Lstat(clean); err == nil && info.Mode()&os.ModeSymlink != 0 && !observer.followSymlinks {
		// Observing the link itself is safe; only resolving through a parent
		// link would escape the lexical workspace boundary.
		parent = filepath.Dir(clean)
	}
	for {
		info, err := os.Lstat(parent)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 && !observer.followSymlinks && parent != clean {
				return "", fmt.Errorf("%w: %q", ErrSymlinkNotAllowed, parent)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("authorize path %q: %w", clean, err)
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", fmt.Errorf("%w: no existing ancestor for %q", ErrPathOutsideRoots, clean)
		}
		parent = next
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("authorize path %q: %w", clean, err)
	}
	for _, root := range observer.roots {
		if pathWithin(root, resolvedParent) {
			return clean, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrPathOutsideRoots, clean)
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func (observer *FileObserver) Observe(
	effectID union.EffectID,
	intent union.IntentNode,
	attemptID union.MechanismAttemptID,
	before, after FileSnapshot,
	occurredAt time.Time,
) (union.EffectRecord, error) {
	if observer == nil || effectID == "" || intent.ID == "" || attemptID == "" || occurredAt.IsZero() {
		return union.EffectRecord{}, fmt.Errorf("%w: effect identity, intent, attempt and time are required", ErrInvalidPolicy)
	}
	if before.State.Path == "" || before.State.Path != after.State.Path || filepath.Clean(intent.Target) != before.State.Path {
		return union.EffectRecord{}, ErrSnapshotMismatch
	}
	if snapshotsEqual(before.State, after.State) {
		return union.EffectRecord{}, ErrNoObservableChange
	}
	changeKind, effectKind, err := classifyFileEffect(intent.Kind, before.State, after.State)
	if err != nil {
		return union.EffectRecord{}, err
	}
	change := &union.WorkspaceChange{Kind: changeKind, Path: before.State.Path, Before: cloneState(&before.State), After: cloneState(&after.State)}
	beforeComparable := before.ContentCaptured || !before.State.Exists
	afterComparable := after.ContentCaptured || !after.State.Exists
	if beforeComparable && afterComparable && !bytes.Equal(before.content, after.content) {
		change.UnifiedDiff = fullUnifiedDiff(before.State.Path, observer.redactor.Redact(before.content), observer.redactor.Redact(after.content))
	}
	beforeDigest, err := digestValue(before.State)
	if err != nil {
		return union.EffectRecord{}, err
	}
	afterDigest, err := digestValue(after.State)
	if err != nil {
		return union.EffectRecord{}, err
	}
	observed := union.EffectRecord{
		ID: effectID, IntentIDs: []union.IntentID{intent.ID}, MechanismAttemptID: attemptID,
		Kind: effectKind, Target: before.State.Path,
		Payload: union.EffectPayload{WorkspaceChange: change},
		EvidenceRefs: []union.EvidenceRef{
			{Kind: "filesystem_snapshot_before", Source: "praxis_filesystem_observer", Digest: beforeDigest, CapturedAt: occurredAt, Sensitivity: "internal"},
			{Kind: "filesystem_snapshot_after", Source: "praxis_filesystem_observer", Digest: afterDigest, CapturedAt: occurredAt, Sensitivity: "internal"},
		},
		ObservationSource: "praxis_filesystem_observer", VerificationStatus: union.VerificationUnverified,
		Confidence: "observed", OccurredAt: occurredAt.UTC(),
	}
	if err := observed.Validate(); err != nil {
		return union.EffectRecord{}, fmt.Errorf("%w: invalid file Effect: %v", ErrInvalidPolicy, err)
	}
	return observed, nil
}

func (observer *FileObserver) ObserveMove(
	effectID union.EffectID,
	intent union.IntentNode,
	attemptID union.MechanismAttemptID,
	sourceBefore, sourceAfter, destinationBefore, destinationAfter FileSnapshot,
	occurredAt time.Time,
) (union.EffectRecord, error) {
	if observer == nil || effectID == "" || intent.ID == "" || attemptID == "" || occurredAt.IsZero() || intent.Kind != union.IntentMoveFile {
		return union.EffectRecord{}, fmt.Errorf("%w: move identity or intent is invalid", ErrInvalidPolicy)
	}
	if sourceBefore.State.Path == "" || sourceBefore.State.Path != sourceAfter.State.Path ||
		destinationBefore.State.Path == "" || destinationBefore.State.Path != destinationAfter.State.Path ||
		filepath.Clean(intent.Target) != sourceBefore.State.Path {
		return union.EffectRecord{}, ErrSnapshotMismatch
	}
	expectedDestination, err := expectedMoveDestination(intent)
	if err != nil {
		return union.EffectRecord{}, err
	}
	if destinationBefore.State.Path != expectedDestination {
		return union.EffectRecord{}, ErrIntentMismatch
	}
	if !sourceBefore.State.Exists || sourceAfter.State.Exists || destinationBefore.State.Exists || !destinationAfter.State.Exists ||
		sourceBefore.State.Type != "regular" || destinationAfter.State.Type != "regular" ||
		sourceBefore.State.Hash == "" || sourceBefore.State.Hash != destinationAfter.State.Hash {
		return union.EffectRecord{}, ErrIntentMismatch
	}
	change := &union.WorkspaceChange{
		Kind: "moved", Path: sourceBefore.State.Path, Destination: destinationBefore.State.Path,
		Before: cloneState(&sourceBefore.State), After: cloneState(&sourceAfter.State),
		DestinationBefore: cloneState(&destinationBefore.State), DestinationAfter: cloneState(&destinationAfter.State),
	}
	digests := make([]union.EvidenceRef, 0, 4)
	for _, item := range []struct {
		kind  string
		state union.FileStateSnapshot
	}{
		{"filesystem_source_before", sourceBefore.State},
		{"filesystem_source_after", sourceAfter.State},
		{"filesystem_destination_before", destinationBefore.State},
		{"filesystem_destination_after", destinationAfter.State},
	} {
		digest, err := digestValue(item.state)
		if err != nil {
			return union.EffectRecord{}, err
		}
		digests = append(digests, union.EvidenceRef{Kind: item.kind, Source: "praxis_filesystem_observer", Digest: digest, CapturedAt: occurredAt.UTC(), Sensitivity: "internal"})
	}
	observed := union.EffectRecord{
		ID: effectID, IntentIDs: []union.IntentID{intent.ID}, MechanismAttemptID: attemptID,
		Kind: "file_moved", Target: sourceBefore.State.Path,
		Payload: union.EffectPayload{WorkspaceChange: change}, EvidenceRefs: digests,
		ObservationSource: "praxis_filesystem_observer", VerificationStatus: union.VerificationUnverified,
		Confidence: "observed", OccurredAt: occurredAt.UTC(),
	}
	if err := observed.Validate(); err != nil {
		return union.EffectRecord{}, fmt.Errorf("%w: invalid move Effect: %v", ErrInvalidPolicy, err)
	}
	return observed, nil
}

func expectedMoveDestination(intent union.IntentNode) (string, error) {
	var specification struct {
		Destination string `json:"destination"`
	}
	if len(intent.Specification) == 0 || json.Unmarshal(intent.Specification, &specification) != nil ||
		!filepath.IsAbs(specification.Destination) || filepath.Clean(specification.Destination) != specification.Destination {
		return "", fmt.Errorf("%w: MoveFile requires an absolute clean destination specification", ErrInvalidPolicy)
	}
	return specification.Destination, nil
}

func VerifyFileEffect(effect union.EffectRecord, verificationID union.VerificationID, expectation FileExpectation, completedAt time.Time) (FileValidation, error) {
	if verificationID == "" || completedAt.IsZero() || effect.ID == "" || effect.Payload.WorkspaceChange == nil || len(effect.IntentIDs) == 0 {
		return FileValidation{}, fmt.Errorf("%w: file verification input is incomplete", ErrInvalidPolicy)
	}
	change := effect.Payload.WorkspaceChange
	status := union.VerificationVerified
	failureCode := ""
	if expectation.BeforeExists != nil && (change.Before == nil || change.Before.Exists != *expectation.BeforeExists) {
		status, failureCode = union.VerificationContradicted, "before_existence_mismatch"
	}
	if expectation.BeforeHash != "" && (change.Before == nil || change.Before.Hash != expectation.BeforeHash) {
		status, failureCode = union.VerificationContradicted, "before_hash_mismatch"
	}
	if expectation.AfterExists != nil && (change.After == nil || change.After.Exists != *expectation.AfterExists) {
		status, failureCode = union.VerificationContradicted, "after_existence_mismatch"
	}
	if expectation.AfterHash != "" && (change.After == nil || change.After.Hash != expectation.AfterHash) {
		status, failureCode = union.VerificationContradicted, "after_hash_mismatch"
	}
	if expectation.AfterType != "" && (change.After == nil || change.After.Type != expectation.AfterType) {
		status, failureCode = union.VerificationContradicted, "after_type_mismatch"
	}
	verifiedEffect := effect
	verifiedEffect.VerificationStatus = status
	verifiedEffect.VerificationRefs = appendUniqueVerification(effect.VerificationRefs, verificationID)
	verifiedEffect.Confidence = string(status)
	verification := union.VerificationRecord{
		ID: verificationID, EffectIDs: []union.EffectID{effect.ID}, IntentIDs: append([]union.IntentID(nil), effect.IntentIDs...),
		Kind: "filesystem_postcondition", Status: status,
		Verifier:     union.VersionedIdentity{ID: "praxis.filesystem-observer", Version: "v1"},
		EvidenceRefs: append([]union.EvidenceRef(nil), effect.EvidenceRefs...), FailureCode: failureCode, CompletedAt: completedAt.UTC(),
	}
	if err := verifiedEffect.Validate(); err != nil {
		return FileValidation{}, fmt.Errorf("%w: invalid verified file Effect: %v", ErrInvalidPolicy, err)
	}
	if err := verification.Validate(); err != nil {
		return FileValidation{}, fmt.Errorf("%w: invalid file verification: %v", ErrInvalidPolicy, err)
	}
	return FileValidation{Effect: verifiedEffect, Verification: verification}, nil
}

func appendUniqueVerification(values []union.VerificationID, value union.VerificationID) []union.VerificationID {
	result := append([]union.VerificationID(nil), values...)
	for _, existing := range result {
		if existing == value {
			return result
		}
	}
	return append(result, value)
}

func classifyFileEffect(kind union.IntentKind, before, after union.FileStateSnapshot) (string, string, error) {
	switch kind {
	case union.IntentCreateFile:
		if before.Exists || !after.Exists || after.Type != "regular" {
			return "", "", ErrIntentMismatch
		}
		return "created", "file_created", nil
	case union.IntentModifyFile:
		if !before.Exists || !after.Exists || before.Type != "regular" || after.Type != "regular" || before.Hash == after.Hash {
			return "", "", ErrIntentMismatch
		}
		return "changed", "file_changed", nil
	case union.IntentRewriteFile:
		if !before.Exists || !after.Exists || before.Type != "regular" || after.Type != "regular" || before.Hash == after.Hash {
			return "", "", ErrIntentMismatch
		}
		return "rewritten", "file_rewritten", nil
	case union.IntentDeleteFile:
		if !before.Exists || after.Exists || before.Type != "regular" {
			return "", "", ErrIntentMismatch
		}
		return "deleted", "file_deleted", nil
	case union.IntentCreateDirectory:
		if before.Exists || !after.Exists || after.Type != "directory" {
			return "", "", ErrIntentMismatch
		}
		return "directory_created", "directory_created", nil
	case union.IntentDeleteDirectory:
		if !before.Exists || after.Exists || before.Type != "directory" {
			return "", "", ErrIntentMismatch
		}
		return "directory_deleted", "directory_deleted", nil
	default:
		return "", "", ErrIntentMismatch
	}
}

func snapshotsEqual(left, right union.FileStateSnapshot) bool {
	return left.Path == right.Path && left.Type == right.Type && left.Exists == right.Exists && left.Hash == right.Hash &&
		left.Size == right.Size && left.Mode == right.Mode && left.Symlink == right.Symlink
}

func cloneState(state *union.FileStateSnapshot) *union.FileStateSnapshot {
	if state == nil {
		return nil
	}
	clone := *state
	return &clone
}

func fullUnifiedDiff(path string, before, after []byte) string {
	var builder strings.Builder
	builder.WriteString("--- a/")
	builder.WriteString(filepath.ToSlash(strings.TrimPrefix(path, string(filepath.Separator))))
	builder.WriteString("\n+++ b/")
	builder.WriteString(filepath.ToSlash(strings.TrimPrefix(path, string(filepath.Separator))))
	builder.WriteByte('\n')
	beforeLines := splitLines(before)
	afterLines := splitLines(after)
	fmt.Fprintf(&builder, "@@ -1,%d +1,%d @@\n", len(beforeLines), len(afterLines))
	for _, line := range beforeLines {
		builder.WriteByte('-')
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	for _, line := range afterLines {
		builder.WriteByte('+')
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func splitLines(payload []byte) []string {
	if len(payload) == 0 {
		return nil
	}
	text := strings.TrimSuffix(string(payload), "\n")
	if text == "" {
		return []string{""}
	}
	return strings.Split(text, "\n")
}
