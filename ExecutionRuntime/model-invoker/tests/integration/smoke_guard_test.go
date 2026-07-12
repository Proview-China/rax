//go:build integration

package integration_test

import (
	"strings"
	"testing"
)

func hasExactProviderSmokeMarker(text, marker string) bool {
	return marker != "" && strings.TrimSpace(text) == marker
}

func TestProviderSmokeMarkerIsExact(t *testing.T) {
	const marker = "praxis-provider-ok"
	for _, value := range []string{"", "not-empty", "prefix " + marker, marker + " suffix", strings.ToUpper(marker)} {
		if hasExactProviderSmokeMarker(value, marker) {
			t.Fatalf("non-exact marker %q was accepted", value)
		}
	}
	if !hasExactProviderSmokeMarker(" \n"+marker+"\t", marker) {
		t.Fatal("exact marker with transport whitespace was rejected")
	}
}
