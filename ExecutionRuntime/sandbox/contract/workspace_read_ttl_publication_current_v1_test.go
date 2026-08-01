package contract

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestWorkspaceReadTTLClosureV1IncludesPublishedCommandCurrentAndKeepsLegacyWire(t *testing.T) {
	base := time.Unix(0, 10_000)
	legacy, err := SealWorkspaceReadTTLClosureV1(WorkspaceReadTTLClosureV1{
		UnifiedNotAfterUnixNano:       base.Add(10 * time.Second).UnixNano(),
		RuntimeEnforcementExpiresNano: base.Add(9 * time.Second).UnixNano(),
		AssociationExpiresUnixNano:    base.Add(8 * time.Second).UnixNano(),
		CommandRequestedNotAfterNano:  base.Add(7 * time.Second).UnixNano(),
		CommandExpiresUnixNano:        base.Add(6 * time.Second).UnixNano(),
		WorkspaceViewExpiresUnixNano:  base.Add(5 * time.Second).UnixNano(),
		WorkspaceLeaseExpiresUnixNano: base.Add(4 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.PublishedCommandCurrentExpiresUnixNano != 0 || legacy.EffectiveExpiresUnixNano != base.Add(4*time.Second).UnixNano() {
		t.Fatalf("legacy zero/omitted publication current changed: %#v", legacy)
	}
	wire, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(wire, []byte("published_command_current")) {
		t.Fatalf("legacy V1 wire unexpectedly gained the additive field: %s", wire)
	}
	published := legacy
	published.PublishedCommandCurrentExpiresUnixNano = base.Add(3 * time.Second).UnixNano()
	published.Digest = ""
	published, err = SealWorkspaceReadTTLClosureV1(published)
	if err != nil {
		t.Fatal(err)
	}
	if published.EffectiveExpiresUnixNano != published.PublishedCommandCurrentExpiresUnixNano {
		t.Fatalf("published current was not the natural TTL minimum: %#v", published)
	}
	if err = published.ValidateCurrent(time.Unix(0, published.PublishedCommandCurrentExpiresUnixNano)); err == nil {
		t.Fatal("published current remained eligible at exact expiry")
	}
}
