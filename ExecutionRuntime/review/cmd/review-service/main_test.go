package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testTokenV1  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testCursorV1 = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
)

func setCheckEnvironmentV1(t *testing.T, database string) {
	t.Helper()
	t.Setenv("PRAXIS_REVIEW_MODE", "check")
	t.Setenv("PRAXIS_REVIEW_DB", database)
	t.Setenv("PRAXIS_REVIEW_ADDR", "127.0.0.1:8087")
	t.Setenv("PRAXIS_REVIEW_CURSOR_KEY_HEX", testCursorV1)
	t.Setenv("PRAXIS_REVIEW_AUTH_JSON", `{"entries":[{"token":"`+testTokenV1+`","tenant_id":"test","subject_id":"operator","capabilities":["review.read"]}],"ttl_seconds":300}`)
	t.Setenv("PRAXIS_REVIEW_TLS_CERT", "")
	t.Setenv("PRAXIS_REVIEW_TLS_KEY", "")
}

func TestCheckModeUsesFullStartupPathWithoutListenerV1(t *testing.T) {
	database := filepath.Join(t.TempDir(), "review.db")
	setCheckEnvironmentV1(t, database)
	var output bytes.Buffer
	if err := runV1(&output); err != nil {
		t.Fatalf("check mode: %v", err)
	}
	if got, want := strings.TrimSpace(output.String()), checkResultV1; got != want {
		t.Fatalf("check result mismatch:\n got: %s\nwant: %s", got, want)
	}
	if info, err := os.Stat(database); err != nil || info.Size() == 0 {
		t.Fatalf("check mode did not initialize and inspect the SQLite database: info=%v err=%v", info, err)
	}
}

func TestCheckModeRejectsCorruptDatabaseV1(t *testing.T) {
	database := filepath.Join(t.TempDir(), "review.db")
	if err := os.WriteFile(database, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	setCheckEnvironmentV1(t, database)
	var output bytes.Buffer
	if err := runV1(&output); err == nil {
		t.Fatal("check mode accepted a corrupt SQLite database")
	}
	if output.Len() != 0 {
		t.Fatalf("failed check emitted success output: %q", output.String())
	}
}

func TestCheckModeRejectsUnknownModeBeforeDatabaseCreationV1(t *testing.T) {
	database := filepath.Join(t.TempDir(), "review.db")
	setCheckEnvironmentV1(t, database)
	t.Setenv("PRAXIS_REVIEW_MODE", "unknown")
	if err := runV1(&bytes.Buffer{}); err == nil {
		t.Fatal("unknown review service mode was accepted")
	}
	if _, err := os.Stat(database); !os.IsNotExist(err) {
		t.Fatalf("invalid mode touched the database: %v", err)
	}
}

func TestCheckModeRejectsInvalidTransportWithoutSuccessV1(t *testing.T) {
	database := filepath.Join(t.TempDir(), "review.db")
	setCheckEnvironmentV1(t, database)
	t.Setenv("PRAXIS_REVIEW_ADDR", "0.0.0.0:8087")
	var output bytes.Buffer
	if err := runV1(&output); err == nil {
		t.Fatal("non-loopback check without TLS was accepted")
	}
	if output.Len() != 0 {
		t.Fatalf("failed transport check emitted success output: %q", output.String())
	}
}

func TestCheckModeRejectsInvalidTLSBeforeDatabaseCreationV1(t *testing.T) {
	root := t.TempDir()
	database := filepath.Join(root, "review.db")
	certificate := filepath.Join(root, "tls.crt")
	key := filepath.Join(root, "tls.key")
	if err := os.WriteFile(certificate, []byte("invalid certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("invalid key"), 0o600); err != nil {
		t.Fatal(err)
	}
	setCheckEnvironmentV1(t, database)
	t.Setenv("PRAXIS_REVIEW_TLS_CERT", certificate)
	t.Setenv("PRAXIS_REVIEW_TLS_KEY", key)
	var output bytes.Buffer
	if err := runV1(&output); err == nil {
		t.Fatal("invalid TLS pair was accepted")
	}
	if output.Len() != 0 {
		t.Fatalf("failed TLS check emitted success output: %q", output.String())
	}
	if _, err := os.Stat(database); !os.IsNotExist(err) {
		t.Fatalf("invalid TLS touched the database: %v", err)
	}
}
