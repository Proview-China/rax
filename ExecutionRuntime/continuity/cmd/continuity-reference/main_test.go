package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/cli"
)

func TestRunOpensSQLiteAndExecutesReadOnlyTimelineWatch(t *testing.T) {
	database := filepath.Join(t.TempDir(), "continuity.db")
	input := strings.NewReader(`{"query":{"ledger_scope_digest":"scope-1","authority_watermark":"authority-1","policy_watermark":"policy-1","page_limit":10}}`)
	var output, errors bytes.Buffer
	if code := run(context.Background(), []string{"-db", database, "timeline", "watch"}, input, &output, &errors); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errors.String())
	}
	var result cli.OutputV1
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ContractVersion != cli.ContractVersionV1 || result.Page == nil || len(result.Page.Records) != 0 || result.Page.NextCursor == "" {
		t.Fatalf("unexpected output: %#v", result)
	}
}

func TestRunCreatesFreshSQLiteWithPrivatePermissionsDespiteUmask(t *testing.T) {
	database := filepath.Join(privateTempDir(t), "continuity.db")
	previous := syscall.Umask(0o022)
	defer syscall.Umask(previous)

	var output, errors bytes.Buffer
	if code := run(context.Background(), []string{"-db", database, "timeline", "watch"}, validWatchInput(), &output, &errors); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errors.String())
	}
	info, err := os.Stat(database)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("fresh database mode=%#o, want 0600", got)
	}
}

func TestRunRejectsUnsafeExistingSQLitePermissions(t *testing.T) {
	database := filepath.Join(privateTempDir(t), "continuity.db")
	if err := os.WriteFile(database, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(database, 0o644); err != nil {
		t.Fatal(err)
	}

	var output, errors bytes.Buffer
	code := run(context.Background(), []string{"-db", database, "timeline", "watch"}, validWatchInput(), &output, &errors)
	if code != 1 || output.Len() != 0 || !strings.Contains(errors.String(), "private mode 0600") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, output.String(), errors.String())
	}
	info, err := os.Stat(database)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("unsafe existing mode was silently changed to %#o", got)
	}
}

func TestRunRejectsDatabaseSymlinkAndSymlinkedParent(t *testing.T) {
	root := privateTempDir(t)
	target := filepath.Join(root, "target.db")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	databaseLink := filepath.Join(root, "continuity.db")
	if err := os.Symlink(target, databaseLink); err != nil {
		t.Fatal(err)
	}
	parentTarget := filepath.Join(root, "real")
	if err := os.Mkdir(parentTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	parentLink := filepath.Join(root, "linked")
	if err := os.Symlink(parentTarget, parentLink); err != nil {
		t.Fatal(err)
	}

	for _, database := range []string{databaseLink, filepath.Join(parentLink, "continuity.db")} {
		var output, errors bytes.Buffer
		code := run(context.Background(), []string{"-db", database, "timeline", "watch"}, validWatchInput(), &output, &errors)
		if code != 1 || output.Len() != 0 || !strings.Contains(errors.String(), "symbolic link") {
			t.Fatalf("path=%q code=%d stdout=%q stderr=%q", database, code, output.String(), errors.String())
		}
	}
}

func TestRunRejectsInvalidDatabasePaths(t *testing.T) {
	root := privateTempDir(t)
	directory := filepath.Join(root, "directory.db")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	missingParent := filepath.Join(root, "missing", "continuity.db")

	for _, database := range []string{directory, missingParent} {
		var output, errors bytes.Buffer
		code := run(context.Background(), []string{"-db", database, "timeline", "watch"}, validWatchInput(), &output, &errors)
		if code != 1 || output.Len() != 0 {
			t.Fatalf("path=%q code=%d stdout=%q stderr=%q", database, code, output.String(), errors.String())
		}
	}
}

func TestRunRejectsGovernedWritesAndNeverWritesPartialOutput(t *testing.T) {
	database := filepath.Join(t.TempDir(), "continuity.db")
	var output, errors bytes.Buffer
	code := run(context.Background(), []string{"-db", database, "restore"}, strings.NewReader(`{}`), &output, &errors)
	if code != 1 || output.Len() != 0 || !strings.Contains(errors.String(), "governed workflow capability is not configured") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, output.String(), errors.String())
	}
}

func validWatchInput() *strings.Reader {
	return strings.NewReader(`{"query":{"ledger_scope_digest":"scope-1","authority_watermark":"authority-1","policy_watermark":"policy-1","page_limit":10}}`)
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRunRequiresDatabaseAndReadCommand(t *testing.T) {
	for _, args := range [][]string{nil, {"timeline", "watch"}, {"-db", "continuity.db"}} {
		var output, errors bytes.Buffer
		if code := run(context.Background(), args, strings.NewReader(`{}`), &output, &errors); code != 2 || output.Len() != 0 || !strings.Contains(errors.String(), "usage:") {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, output.String(), errors.String())
		}
	}
}
