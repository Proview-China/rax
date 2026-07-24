package workspacefs

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestCaptureRealOverlayDiffAndContentAddressedBlobs(t *testing.T) {
	if !ports.Supported(ports.FeatureWorkspaceCapture) {
		t.Fatal("live workspace capture feature is not advertised")
	}
	driver, view, base, overlay, blobs := workspaceFixture(t)
	writeFile(t, filepath.Join(base, "src/generated/existing.txt"), "before")
	writeFile(t, filepath.Join(base, "src/readme.txt"), "read-only")
	writeFile(t, filepath.Join(overlay, "src/generated/existing.txt"), "after")
	writeFile(t, filepath.Join(overlay, "src/generated/new.txt"), "new")
	writeFile(t, filepath.Join(overlay, "src/readme.txt"), "read-only")
	view.BaseRevision = inspectBase(t, driver, view)

	set, err := driver.CaptureWorkspaceChangeSetV1(context.Background(), captureRequest(view, "changeset-real"))
	if err != nil {
		t.Fatal(err)
	}
	if set.State != contract.ChangeSetStaged || len(set.Changes) != 2 || set.Changes[0].Kind != contract.WorkspaceModify || set.Changes[1].Kind != contract.WorkspaceAdd {
		t.Fatalf("unexpected real change set: %#v", set)
	}
	for _, change := range set.Changes {
		if change.BlobRef == nil {
			t.Fatal("materialized add/modify lacks a blob ref")
		}
		matches, err := filepath.Glob(filepath.Join(blobs, "*.blob"))
		if err != nil || len(matches) != 2 {
			t.Fatalf("content-addressed blobs = %v err=%v", matches, err)
		}
	}
	if set.RuntimeSettlement != nil || set.CommittedRevision != "" {
		t.Fatal("capture forged governed commit state")
	}
}

func TestReadOnlyHiddenSymlinkAndBaseDriftFailClosed(t *testing.T) {
	t.Run("read-only drift", func(t *testing.T) {
		driver, view, base, overlay, _ := workspaceFixture(t)
		writeFile(t, filepath.Join(base, "src/readme.txt"), "before")
		writeFile(t, filepath.Join(overlay, "src/readme.txt"), "after")
		view.BaseRevision = inspectBase(t, driver, view)
		if _, err := driver.CaptureWorkspaceChangeSetV1(context.Background(), captureRequest(view, "read-only")); err == nil {
			t.Fatal("read-only path drift was captured")
		}
	})
	t.Run("hidden path", func(t *testing.T) {
		driver, view, _, overlay, _ := workspaceFixture(t)
		writeFile(t, filepath.Join(overlay, "src/generated/private/secret"), "secret")
		view.BaseRevision = inspectBase(t, driver, view)
		if _, err := driver.CaptureWorkspaceChangeSetV1(context.Background(), captureRequest(view, "hidden")); err == nil {
			t.Fatal("hidden path was captured")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		driver, view, base, overlay, _ := workspaceFixture(t)
		writeFile(t, filepath.Join(base, "src/generated/file"), "base")
		view.BaseRevision = inspectBase(t, driver, view)
		if err := os.MkdirAll(filepath.Join(overlay, "src/generated"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/etc/passwd", filepath.Join(overlay, "src/generated/file")); err != nil {
			t.Fatal(err)
		}
		if _, err := driver.CaptureWorkspaceChangeSetV1(context.Background(), captureRequest(view, "symlink")); err == nil {
			t.Fatal("symlink path was captured")
		}
	})
	t.Run("base revision", func(t *testing.T) {
		driver, view, base, overlay, _ := workspaceFixture(t)
		writeFile(t, filepath.Join(base, "src/generated/file"), "base")
		writeFile(t, filepath.Join(overlay, "src/generated/file"), "changed")
		view.BaseRevision = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		if _, err := driver.CaptureWorkspaceChangeSetV1(context.Background(), captureRequest(view, "base-drift")); err == nil {
			t.Fatal("base revision drift was captured")
		}
	})
}

func TestS1S2HostDriftReturnsZeroChangeSet(t *testing.T) {
	driver, view, base, overlay, _ := workspaceFixture(t)
	writeFile(t, filepath.Join(base, "src/generated/file"), "base")
	writeFile(t, filepath.Join(overlay, "src/generated/file"), "after-s1")
	view.BaseRevision = inspectBase(t, driver, view)
	driver.afterS1 = func() { writeFile(t, filepath.Join(overlay, "src/generated/file"), "drifted") }
	set, err := driver.CaptureWorkspaceChangeSetV1(context.Background(), captureRequest(view, "host-drift"))
	if err == nil {
		t.Fatal("host drift produced a change set")
	}
	if set.Meta.ID != "" || len(set.Changes) != 0 {
		t.Fatalf("host drift returned partial authority: %#v", set)
	}
}

func TestSixtyFourConcurrentCapturesAreExactAndBlobCreateOnce(t *testing.T) {
	driver, view, base, overlay, blobs := workspaceFixture(t)
	writeFile(t, filepath.Join(base, "src/generated/file"), "base")
	writeFile(t, filepath.Join(overlay, "src/generated/file"), "overlay")
	view.BaseRevision = inspectBase(t, driver, view)
	var failures atomic.Int64
	var digests sync.Map
	var wait sync.WaitGroup
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			set, err := driver.CaptureWorkspaceChangeSetV1(context.Background(), captureRequest(view, "concurrent"))
			if err != nil {
				failures.Add(1)
				return
			}
			digests.Store(set.Meta.Digest, struct{}{})
		}()
	}
	wait.Wait()
	if failures.Load() != 0 {
		t.Fatalf("concurrent capture failures=%d", failures.Load())
	}
	count := 0
	digests.Range(func(_, _ any) bool { count++; return true })
	if count != 1 {
		t.Fatalf("concurrent capture digests=%d, want 1", count)
	}
	matches, err := filepath.Glob(filepath.Join(blobs, "*.blob"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("blob create-once files=%v err=%v", matches, err)
	}
}

func workspaceFixture(t *testing.T) (*DriverV1, contract.WorkspaceView, string, string, string) {
	t.Helper()
	root := t.TempDir()
	base := filepath.Join(root, "base")
	overlay := filepath.Join(root, "overlay")
	blobs := filepath.Join(root, "blobs")
	for _, path := range []string{base, overlay} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	view := testkit.WorkspaceView()
	driver, err := NewDriverV1([]BindingV1{{ViewRef: view.Meta.Ref(), BaseRoot: base, OverlayRoot: overlay, BlobRoot: blobs}}, LimitsV1{MaxFiles: 100, MaxTotalByte: 1 << 20, MaxFileByte: 1 << 18}, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	return driver, view, base, overlay, blobs
}

func inspectBase(t *testing.T, driver *DriverV1, view contract.WorkspaceView) string {
	t.Helper()
	revision, err := driver.InspectWorkspaceBaseRevisionV1(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func captureRequest(view contract.WorkspaceView, id string) ports.CaptureWorkspaceChangeSetRequest {
	return ports.CaptureWorkspaceChangeSetRequest{ChangeSetID: id, View: view, RequestedNotAfter: testkit.FixedNow.Add(time.Hour)}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
