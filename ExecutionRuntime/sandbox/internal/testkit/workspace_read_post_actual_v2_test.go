package testkit

import (
	"testing"
	"time"
)

func TestWorkspaceReadPostActualV2BuildsExactRuntimeAndPublicationInputs(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_980_000_000, 0)
	fixture := WorkspaceReadPostActualV2(now, "shape")
	if err := fixture.RuntimeCurrent.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Publication.Source.ValidateCurrent(fixture.Publication.Source.SourceCommand, now); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Publication.Effect.Validate(now); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Publication.Prepared.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Publication.Workspace.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}
}
