package decoder_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/decoder"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"gopkg.in/yaml.v3"
)

func TestDecodeYAMLAndStrictJSONRoundTripV1(t *testing.T) {
	source := conformance.SourceV1(time.Unix(1_800_010_000, 0))
	payload, err := yaml.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decoder.DecodeYAMLV1(payload, conformance.CatalogV1())
	if err != nil {
		t.Fatalf("decode yaml: %v\n%s", err, payload)
	}
	strict, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	again, err := decoder.DecodeJSONV1(strict, conformance.CatalogV1())
	if err != nil || again.DefinitionID != source.DefinitionID {
		t.Fatalf("round trip: %#v %v", again, err)
	}
}

func TestYAMLSafetySubsetRejectsBeforeSemanticValidationV1(t *testing.T) {
	cases := map[string]string{
		"duplicate":      "a: 1\na: 2\n",
		"merge":          "base: &base {a: 1}\nvalue: {<<: *base}\n",
		"anchor":         "a: &x value\nb: value\n",
		"alias":          "a: &x value\nb: *x\n",
		"tag":            "a: !custom value\n",
		"non-string-key": "1: value\n",
		"float":          "a: 1.2\n",
		"nan":            "a: .nan\n",
		"infinity":       "a: .inf\n",
		"timestamp":      "a: 2026-07-17\n",
		"hex":            "a: 0x10\n",
		"multi-document": "a: 1\n---\nb: 2\n",
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := decoder.DecodeYAMLV1([]byte(payload), conformance.CatalogV1())
			if err == nil || !core.HasCategory(err, core.ErrorInvalidArgument) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestYAMLUnknownFieldAndOversizeRejectedV1(t *testing.T) {
	_, err := decoder.DecodeYAMLV1([]byte("contract_version: praxis.agent.definition/v1\nunknown: true\n"), conformance.CatalogV1())
	if err == nil {
		t.Fatal("unknown field accepted")
	}
	_, err = decoder.DecodeYAMLV1([]byte(strings.Repeat("a", decoder.MaxYAMLBytesV1+1)), conformance.CatalogV1())
	if !core.HasReason(err, core.ReasonCanonicalLimitExceeded) {
		t.Fatalf("oversize = %v", err)
	}
}

func TestStrictJSONDuplicateAndOwnerFieldsRejectedV1(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"contract_version":"praxis.agent.definition/v1","contract_version":"praxis.agent.definition/v1"}`),
		[]byte(`{"contract_version":"praxis.agent.definition/v1","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`),
		[]byte(`{"contract_version":"praxis.agent.definition/v1","created_unix_nano":1}`),
	}
	for _, payload := range cases {
		if _, err := decoder.DecodeJSONV1(payload, conformance.CatalogV1()); err == nil {
			t.Fatalf("strict JSON accepted %s", payload)
		}
	}
}

func FuzzStrictYAMLDecoderV1(f *testing.F) {
	source := conformance.SourceV1(time.Unix(1_800_010_100, 0))
	valid, _ := yaml.Marshal(source)
	f.Add(valid)
	f.Add([]byte("a: &x [*x]\n"))
	f.Add([]byte("a: 1.0\n"))
	f.Fuzz(func(t *testing.T, payload []byte) {
		decoded, err := decoder.DecodeYAMLV1(payload, conformance.CatalogV1())
		if err == nil && decoded.ContractVersion == "" {
			t.Fatal("successful decoder returned empty contract version")
		}
	})
}
