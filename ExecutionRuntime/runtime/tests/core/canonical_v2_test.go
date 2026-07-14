package core_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestCanonicalDigestUsesDomainSeparation(t *testing.T) {
	t.Parallel()
	value := struct {
		Value string `json:"value"`
	}{Value: "same"}
	left, err := core.CanonicalJSONDigest("praxis.runtime.binding", "v2", "Manifest", value)
	if err != nil {
		t.Fatal(err)
	}
	right, err := core.CanonicalJSONDigest("praxis.runtime.effect", "v2", "Manifest", value)
	if err != nil {
		t.Fatal(err)
	}
	if left == right {
		t.Fatal("different canonical domains must never share an identity digest")
	}
}

func TestStrictJSONRejectsDuplicateKeysAndTrailingDocuments(t *testing.T) {
	t.Parallel()
	target := struct {
		Value string `json:"value"`
	}{}
	if err := core.DecodeStrictJSON([]byte(`{"value":"a","value":"b"}`), &target); !core.HasReason(err, core.ReasonDuplicateCanonicalKey) {
		t.Fatalf("duplicate key must fail with a machine-readable reason: %v", err)
	}
	if err := core.DecodeStrictJSON([]byte(`{"value":"a"} {"value":"b"}`), &target); !core.HasReason(err, core.ReasonInvalidCanonicalForm) {
		t.Fatalf("trailing document must fail: %v", err)
	}
}

func TestStrictSemanticVersionAndBuildIdentity(t *testing.T) {
	t.Parallel()
	version, err := core.ParseSemanticVersion("1.2.3-rc.1+linux.amd64")
	if err != nil || version.String() != "1.2.3-rc.1+linux.amd64" {
		t.Fatalf("canonical semantic version must round trip: %v %+v", err, version)
	}
	for _, invalid := range []string{"01.2.3", "1.2", "1.2.3-01", "1.2.3+", "1.2.3+meta+again", "1.2.3 ", "1.2.18446744073709551616"} {
		if _, err := core.ParseSemanticVersion(invalid); !core.HasReason(err, core.ReasonInvalidSemanticVersion) {
			t.Fatalf("invalid semantic version %q must fail deterministically: %v", invalid, err)
		}
	}
	plain, _ := core.ParseSemanticVersion("1.2.3")
	build, _ := core.ParseSemanticVersion("1.2.3+build.7")
	if core.CompareSemanticVersion(plain, build) != 0 {
		t.Fatal("SemVer range precedence must ignore build metadata")
	}
}

func FuzzCanonicalDigestDeterministic(f *testing.F) {
	f.Add("value")
	f.Add("")
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 64<<10 {
			t.Skip()
		}
		input := struct {
			Value string `json:"value"`
		}{Value: value}
		left, leftErr := core.CanonicalJSONDigest("praxis.runtime.test", "v1", "FuzzValue", input)
		right, rightErr := core.CanonicalJSONDigest("praxis.runtime.test", "v1", "FuzzValue", input)
		if (leftErr == nil) != (rightErr == nil) || left != right {
			t.Fatalf("same normalized input must produce the same result: %v %v %s %s", leftErr, rightErr, left, right)
		}
	})
}
