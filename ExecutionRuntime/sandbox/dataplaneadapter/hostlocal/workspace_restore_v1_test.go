package hostlocal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"golang.org/x/sys/unix"
)

func TestWorkspaceCaptureAndStageV1IsolatedExactReplay(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	rootParent := t.TempDir()
	mustMkdirV1(t, filepath.Join(source, "bin"), 0o755)
	mustWriteV1(t, filepath.Join(source, "bin", "run"), []byte("#!/bin/sh\necho ok\n"), 0o755)
	mustWriteV1(t, filepath.Join(source, "empty"), nil, 0o644)
	if err := os.Symlink("bin/run", filepath.Join(source, "link")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(source, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustMkdirV1(t, filepath.Join(source, "nested-module"), 0o755)
	mustWriteV1(t, filepath.Join(source, "nested-module", ".git"), []byte("gitdir: elsewhere"), 0o644)
	mustWriteV1(t, filepath.Join(source, "nested-module", "ignored"), []byte("not captured"), 0o644)

	capture, err := NewWorkspaceCaptureV1(WorkspaceCaptureConfigV1{SourceRoot: source})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := capture.Capture(ctx, &WorkspaceCaptureRequestV1{
		SnapshotID: "snapshot-1", TenantID: "tenant-1", SourceScopeDigest: strings.Repeat("a", contract.DigestSizeHex),
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if len(bundle.Entries) != 3 || len(bundle.Excluded) != 3 {
		t.Fatalf("capture closure entries=%#v excluded=%#v", bundle.Entries, bundle.Excluded)
	}
	if bundle.Excluded[0].Kind != contract.WorkspaceSnapshotExcludedSymlink || bundle.Excluded[1].Kind != contract.WorkspaceSnapshotExcludedSubmodule || bundle.Excluded[2].Kind != contract.WorkspaceSnapshotExcludedFIFO {
		t.Fatalf("unexpected residual classification: %#v", bundle.Excluded)
	}

	now := time.Unix(1_750_000_000, 0)
	stage, err := NewWorkspaceStageV1(WorkspaceStageConfigV1{RootParent: rootParent, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	request := workspaceStageRequestFixtureV1(bundle, now)
	result, err := stage.Stage(ctx, &request)
	if err != nil || !result.Created {
		t.Fatalf("stage: %#v %v", result, err)
	}
	if strings.ContainsAny(result.RootRef.ID, `/\\`) {
		t.Fatalf("public root ref leaked a path: %#v", result.RootRef)
	}
	rootPath, err := stage.rootPath(result.RootRef)
	if err != nil {
		t.Fatal(err)
	}
	assertFileV1(t, filepath.Join(rootPath, "bin", "run"), []byte("#!/bin/sh\necho ok\n"), 0o700)
	assertFileV1(t, filepath.Join(rootPath, "empty"), nil, 0o600)
	if _, err := os.Lstat(filepath.Join(rootPath, "link")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("residual symlink was materialized: %v", err)
	}

	replay, err := stage.Stage(ctx, &request)
	if err != nil || replay.Created || !contract.SameWorkspaceRootRefV1(replay.RootRef, result.RootRef) {
		t.Fatalf("exact replay: %#v %v", replay, err)
	}
	inspected, err := stage.Inspect(ctx, &WorkspaceInspectRequestV1{ExpectedRootRef: result.RootRef, ExpectedBundle: bundle})
	if err != nil || !contract.SameWorkspaceRootRefV1(inspected.RootRef, result.RootRef) {
		t.Fatalf("inspect exact root: %#v %v", inspected, err)
	}
	assertFileV1(t, filepath.Join(source, "bin", "run"), []byte("#!/bin/sh\necho ok\n"), 0o755)
}

func TestWorkspaceStageV1ChangedContentConflictsAndNeverOverwrites(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	stage, err := NewWorkspaceStageV1(WorkspaceStageConfigV1{RootParent: t.TempDir(), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	bundle := workspaceBundleForStageV1(t, "one")
	request := workspaceStageRequestFixtureV1(bundle, now)
	winner, err := stage.Stage(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	drift := workspaceBundleForStageV1(t, "two")
	request.Bundle = drift
	if _, err := stage.Stage(context.Background(), &request); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("changed content should conflict, got %v", err)
	}
	rootPath, _ := stage.rootPath(winner.RootRef)
	assertFileV1(t, filepath.Join(rootPath, "file"), []byte("one"), 0o600)
}

func TestWorkspaceStageV1InspectRejectsTamperAndExtraEntry(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	stage, _ := NewWorkspaceStageV1(WorkspaceStageConfigV1{RootParent: t.TempDir(), Clock: func() time.Time { return now }})
	bundle := workspaceBundleForStageV1(t, "payload")
	request := workspaceStageRequestFixtureV1(bundle, now)
	result, err := stage.Stage(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	rootPath, _ := stage.rootPath(result.RootRef)
	if err := os.WriteFile(filepath.Join(rootPath, "file"), []byte("tamper"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.Inspect(context.Background(), &WorkspaceInspectRequestV1{ExpectedRootRef: result.RootRef, ExpectedBundle: bundle}); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("tamper should conflict, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "file"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "extra"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.Inspect(context.Background(), &WorkspaceInspectRequestV1{ExpectedRootRef: result.RootRef, ExpectedBundle: bundle}); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("extra entry should conflict, got %v", err)
	}
}

func TestWorkspaceStageV1RejectsTypedNilAndUntrustedRootCoordinates(t *testing.T) {
	if _, err := NewWorkspaceStageV1(WorkspaceStageConfigV1{}); err == nil {
		t.Fatal("empty trusted root config was accepted")
	}
	stage, _ := NewWorkspaceStageV1(WorkspaceStageConfigV1{RootParent: t.TempDir(), Clock: time.Now})
	if _, err := stage.Stage(context.Background(), nil); err == nil {
		t.Fatal("typed nil request was accepted")
	}
	if _, err := stage.Inspect(context.Background(), nil); err == nil {
		t.Fatal("typed nil inspect was accepted")
	}
}

func workspaceStageRequestFixtureV1(bundle contract.WorkspaceSnapshotBundleV1, now time.Time) WorkspaceStageRequestV1 {
	return WorkspaceStageRequestV1{
		StageAttemptRef:       workspaceRestoreExactRefFixtureV1(contract.WorkspaceRestoreAttemptTypeURLV1, contract.WorkspaceRestoreAttemptDigestDomainV1, "sandbox-stage-attempt-1", now),
		RuntimeRestoreAttempt: workspaceRestoreExactRefFixtureV1("praxis.runtime/restore-attempt/v2", "praxis.runtime/restore-attempt/body/v2", "restore-attempt-1", now),
		Target: contract.RuntimeLeaseBinding{
			TenantID: "tenant-1", InstanceID: "instance-new", InstanceEpoch: 2, LeaseID: "lease-new", LeaseEpoch: 2,
			FenceEpoch: 2, ScopeDigest: strings.Repeat("b", contract.DigestSizeHex), ObservedRevision: 1, ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
		},
		Bundle: bundle,
	}
}

func workspaceRestoreExactRefFixtureV1(typeURL, domain, id string, now time.Time) contract.SnapshotArtifactExactRefV2 {
	digest, _ := contract.Digest("workspace-restore-hostlocal-test-ref-v1", struct{ TypeURL, ID string }{typeURL, id})
	return contract.SnapshotArtifactExactRefV2{TypeURL: typeURL, Version: 1, ID: id, Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: digest, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
}

func workspaceBundleForStageV1(t *testing.T, content string) contract.WorkspaceSnapshotBundleV1 {
	t.Helper()
	bundle, err := contract.SealWorkspaceSnapshotBundleV1(contract.WorkspaceSnapshotBundleV1{
		SnapshotID: "snapshot-1", TenantID: "tenant-1", SourceScopeDigest: strings.Repeat("a", contract.DigestSizeHex),
		Entries: []contract.WorkspaceSnapshotEntryV1{{Path: "file", Kind: contract.WorkspaceSnapshotRegularFile, Content: []byte(content)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func mustMkdirV1(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Mkdir(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mustWriteV1(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertFileV1(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("%s content=%q want=%q", path, got, content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("%s mode=%o want=%o", path, info.Mode().Perm(), mode)
	}
}
