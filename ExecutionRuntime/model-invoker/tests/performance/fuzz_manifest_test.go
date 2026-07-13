package performance_test

import (
	"encoding/hex"
	"fmt"
	"sort"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
)

func FuzzManifestDiff(f *testing.F) {
	f.Add([]byte("stable"), uint8(0), uint8(0))
	f.Add([]byte("opaque"), uint8(1), uint8(1))
	f.Add([]byte("duplicate"), uint8(2), uint8(2))
	f.Add([]byte{}, uint8(6), uint8(0))
	f.Fuzz(func(t *testing.T, data []byte, shape, modeByte uint8) {
		if len(data) > 1024 {
			data = data[:1024]
		}
		expected, actual := fuzzManifests(data, shape)
		mode := []profile.ContextMode{
			profile.ContextSemanticStable,
			profile.ContextVendorDefault,
			profile.ContextCustomExplicit,
		}[int(modeByte)%3]
		allowedP2 := []string(nil)
		if modeByte&0x4 != 0 {
			allowedP2 = []string{"event.fidelity"}
		}

		first, firstErr := profile.CompareManifests(expected, actual, mode, allowedP2)
		second, secondErr := profile.CompareManifests(expected, actual, mode, allowedP2)
		if errorText(firstErr) != errorText(secondErr) {
			t.Fatalf("manifest comparison was nondeterministic: %v != %v", firstErr, secondErr)
		}
		if firstErr != nil {
			return
		}
		firstDigest, err := first.Digest()
		if err != nil {
			t.Fatalf("first evaluation digest: %v", err)
		}
		secondDigest, err := second.Digest()
		if err != nil {
			t.Fatalf("second evaluation digest: %v", err)
		}
		if firstDigest != secondDigest {
			t.Fatalf("manifest evaluation digest changed: %q != %q", firstDigest, secondDigest)
		}
		paths := make([]string, len(first.Differences))
		for index, difference := range first.Differences {
			paths[index] = difference.Path
		}
		if !sort.StringsAreSorted(paths) {
			t.Fatalf("manifest differences are not canonical: %v", paths)
		}
	})
}

func fuzzManifests(data []byte, shape uint8) (profile.InjectionManifest, profile.InjectionManifest) {
	expected := profile.InjectionManifest{
		SchemaVersion: "v1", ProbeStatus: profile.ManifestProbeNotRun,
		Fields: []profile.ManifestField{
			{Path: "identity.model", State: profile.ManifestFieldPresent, Value: "model-a"},
			{Path: "instructions.semantic_profile", State: profile.ManifestFieldPresent, Value: "minimal"},
			{Path: "event.fidelity", State: profile.ManifestFieldPresent, Value: "reported"},
		},
	}
	value := hex.EncodeToString(data)
	if value == "" {
		value = "empty"
	}
	evidence := func(path string) profile.ManifestEvidence {
		return profile.ManifestEvidence{
			Source: profile.ManifestEvidenceObserved, Confidence: 100,
			Reference: "fuzz://manifest/" + path,
		}
	}
	actual := profile.InjectionManifest{
		SchemaVersion: "v1", ProbeStatus: profile.ManifestProbeObserved,
		Fields: []profile.ManifestField{
			{Path: "identity.model", State: profile.ManifestFieldPresent, Value: "model-a", Evidence: evidence("identity.model")},
			{Path: "instructions.semantic_profile", State: profile.ManifestFieldPresent, Value: value, Evidence: evidence("instructions.semantic_profile")},
			{Path: "event.fidelity", State: profile.ManifestFieldPresent, Value: "reported", Evidence: evidence("event.fidelity")},
		},
	}
	switch shape % 8 {
	case 1:
		actual.Fields[1].State = profile.ManifestFieldOpaque
		actual.Fields[1].Value = ""
		actual.Fields[1].Evidence.Source = profile.ManifestEvidenceOpaque
	case 2:
		actual.Fields = append(actual.Fields, actual.Fields[0])
	case 3:
		actual.Fields[0].State = profile.ManifestFieldState("invalid-" + fmt.Sprint(len(data)))
	case 4:
		actual.ProbeStatus = profile.ManifestProbeNotRun
	case 5:
		actual.Fields[1].Evidence.Confidence = 101
	case 6:
		actual.Fields[1].Value = ""
	case 7:
		actual.Fields = append(actual.Fields, profile.ManifestField{
			Path: "metadata.fuzz", State: profile.ManifestFieldPresent, Value: value,
			Evidence: evidence("metadata.fuzz"),
		})
	}
	return expected, actual
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
