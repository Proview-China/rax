package workspaceread

import (
	"errors"
	"testing"

	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestCloneJSONFailureIsTypedConflictAndDoesNotPanic(t *testing.T) {
	value, err := cloneJSON(func() {})
	if !errors.Is(err, sandboxports.ErrConflict) {
		t.Fatalf("clone error=%v want conflict", err)
	}
	if value != nil {
		t.Fatal("clone returned a value after marshal failure")
	}
}
