package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
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

func TestRunRejectsGovernedWritesAndNeverWritesPartialOutput(t *testing.T) {
	database := filepath.Join(t.TempDir(), "continuity.db")
	var output, errors bytes.Buffer
	code := run(context.Background(), []string{"-db", database, "restore"}, strings.NewReader(`{}`), &output, &errors)
	if code != 1 || output.Len() != 0 || !strings.Contains(errors.String(), "governed workflow capability is not configured") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, output.String(), errors.String())
	}
}

func TestRunRequiresDatabaseAndReadCommand(t *testing.T) {
	for _, args := range [][]string{nil, {"timeline", "watch"}, {"-db", "continuity.db"}} {
		var output, errors bytes.Buffer
		if code := run(context.Background(), args, strings.NewReader(`{}`), &output, &errors); code != 2 || output.Len() != 0 || !strings.Contains(errors.String(), "usage:") {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, output.String(), errors.String())
		}
	}
}
