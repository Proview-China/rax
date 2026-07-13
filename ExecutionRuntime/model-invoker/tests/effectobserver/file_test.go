package effectobserver_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestFileObserverCreatesObservedAndVerifiedEffectFromRealState(t *testing.T) {
	root := t.TempDir()
	observer, err := effect.NewFileObserver(effect.FilePolicy{AllowedRoots: []string{root}, MaxFileBytes: 1024, MaxCaptureBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.go")
	before, err := observer.Capture(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.State.Exists || before.State.Type != "absent" {
		t.Fatalf("before = %#v", before.State)
	}
	if err := os.WriteFile(path, []byte("package config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := observer.Capture(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_720_000_000, 0).UTC()
	observed, err := observer.Observe("effect-1", union.IntentNode{ID: "intent-1", Kind: union.IntentCreateFile, Target: path, Required: true}, "attempt-1", before, after, now)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Kind != "file_created" || observed.VerificationStatus != union.VerificationUnverified || observed.Payload.WorkspaceChange == nil {
		t.Fatalf("observed = %#v", observed)
	}
	if diff := observed.Payload.WorkspaceChange.UnifiedDiff; !strings.Contains(diff, "+package config") {
		t.Fatalf("diff = %q", diff)
	}
	wantFalse, wantTrue := false, true
	validated, err := effect.VerifyFileEffect(observed, "verify-1", effect.FileExpectation{BeforeExists: &wantFalse, AfterExists: &wantTrue, AfterHash: after.State.Hash, AfterType: "regular"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if validated.Effect.VerificationStatus != union.VerificationVerified || validated.Verification.Status != union.VerificationVerified {
		t.Fatalf("validation = %#v", validated)
	}
}

func TestFileObserverRejectsOutsideRootsAndParentSymlinkTraversal(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	observer, err := effect.NewFileObserver(effect.FilePolicy{AllowedRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := observer.Capture(filepath.Join(outside, "escape")); !errors.Is(err, effect.ErrPathOutsideRoots) {
		t.Fatalf("outside error = %v", err)
	}
	link := filepath.Join(root, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := observer.Capture(filepath.Join(link, "escape")); !errors.Is(err, effect.ErrSymlinkNotAllowed) {
		t.Fatalf("symlink traversal error = %v", err)
	}
	linkSnapshot, err := observer.Capture(link)
	if err != nil {
		t.Fatal(err)
	}
	if linkSnapshot.State.Type != "symlink" || linkSnapshot.State.Symlink != outside {
		t.Fatalf("link snapshot = %#v", linkSnapshot.State)
	}
}

func TestFileObserverNoChangeDoesNotInventEffect(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "same.txt")
	if err := os.WriteFile(path, []byte("same\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	observer, err := effect.NewFileObserver(effect.FilePolicy{AllowedRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := observer.Capture(path)
	after, _ := observer.Capture(path)
	_, err = observer.Observe("effect-1", union.IntentNode{ID: "intent-1", Kind: union.IntentModifyFile, Target: path}, "attempt-1", before, after, time.Now())
	if !errors.Is(err, effect.ErrNoObservableChange) {
		t.Fatalf("Observe() error = %v", err)
	}
}

func TestFileObserverMoveRequiresMatchingSourceAndDestinationHash(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "destination.txt")
	if err := os.WriteFile(source, []byte("move me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	observer, err := effect.NewFileObserver(effect.FilePolicy{AllowedRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	sourceBefore, _ := observer.Capture(source)
	destinationBefore, _ := observer.Capture(destination)
	if err := os.Rename(source, destination); err != nil {
		t.Fatal(err)
	}
	sourceAfter, _ := observer.Capture(source)
	destinationAfter, _ := observer.Capture(destination)
	moveIntent := union.IntentNode{
		ID: "intent-move", Kind: union.IntentMoveFile, Target: source,
		Specification: json.RawMessage(`{"destination":` + strconv.Quote(destination) + `}`),
	}
	observed, err := observer.ObserveMove("effect-move", moveIntent, "attempt-move", sourceBefore, sourceAfter, destinationBefore, destinationAfter, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	change := observed.Payload.WorkspaceChange
	if observed.Kind != "file_moved" || change == nil || change.Destination != destination || change.DestinationAfter == nil || change.DestinationAfter.Hash != sourceBefore.State.Hash {
		t.Fatalf("move effect = %#v", observed)
	}
}

func TestFileObserverRedactsKnownSecretsBeforeDiffPublication(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "secret.txt")
	secret := []byte("test-secret-value")
	redactor, err := effect.NewLiteralRedactor(secret)
	if err != nil {
		t.Fatal(err)
	}
	observer, err := effect.NewFileObserver(effect.FilePolicy{AllowedRoots: []string{root}, Redactor: redactor})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := observer.Capture(path)
	if err := os.WriteFile(path, append([]byte("credential="), secret...), 0o600); err != nil {
		t.Fatal(err)
	}
	after, _ := observer.Capture(path)
	observed, err := observer.Observe("effect-secret", union.IntentNode{ID: "intent-secret", Kind: union.IntentCreateFile, Target: path}, "attempt-secret", before, after, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	diff := observed.Payload.WorkspaceChange.UnifiedDiff
	if strings.Contains(diff, string(secret)) || !strings.Contains(diff, "[REDACTED]") {
		t.Fatalf("diff was not safely redacted: %q", diff)
	}
}
