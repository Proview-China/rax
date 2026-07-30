package jsonv1_test

import (
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/transport/jsonv1"
)

type strictSampleV1 struct {
	Name   string  `json:"name"`
	Note   *string `json:"note,omitempty"`
	Nested struct {
		Value string `json:"value"`
	} `json:"nested"`
}

func TestDecoderV1DistinguishesOptionalAbsenceFromExplicitNull(t *testing.T) {
	decoder := jsonv1.NewDecoderV1(1024)
	var omitted strictSampleV1
	if err := decoder.DecodeStrictV1([]byte(`{"name":"one","nested":{"value":"two"}}`), &omitted); err != nil {
		t.Fatal(err)
	}
	var explicitNull strictSampleV1
	err := decoder.DecodeStrictV1([]byte(`{"name":"one","note":null,"nested":{"value":"two"}}`), &explicitNull)
	if err == nil || !strings.Contains(err.Error(), "json_optional_null_forbidden") {
		t.Fatalf("explicit null err=%v", err)
	}
}

func TestDecoderV1AcceptsOneStrictDocument(t *testing.T) {
	decoder := jsonv1.NewDecoderV1(1024)
	var target strictSampleV1
	if err := decoder.DecodeStrictV1([]byte(`{"name":"one","nested":{"value":"two"}}`), &target); err != nil {
		t.Fatal(err)
	}
	if target.Name != "one" || target.Nested.Value != "two" {
		t.Fatalf("target=%+v", target)
	}
}

func TestDecoderV1RejectsDuplicateUnknownTrailingAndOversize(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		max     int
		reason  string
	}{
		{"duplicate_root", `{"name":"one","name":"two","nested":{"value":"v"}}`, 1024, "json_duplicate_key"},
		{"duplicate_nested", `{"name":"one","nested":{"value":"v","value":"x"}}`, 1024, "json_duplicate_key"},
		{"unknown_field", `{"name":"one","nested":{"value":"v"},"future":true}`, 1024, "json_decode_failed"},
		{"unknown_nested_field", `{"name":"one","nested":{"value":"v","future":true}}`, 1024, "json_decode_failed"},
		{"trailing_document", `{"name":"one","nested":{"value":"v"}} {"name":"two"}`, 1024, "json_trailing_document"},
		{"oversize", `{"name":"one","nested":{"value":"v"}}`, 8, "json_payload_size_invalid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var target strictSampleV1
			err := jsonv1.NewDecoderV1(tc.max).DecodeStrictV1([]byte(tc.payload), &target)
			if err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("err=%v want reason=%s", err, tc.reason)
			}
		})
	}
}

func TestDecoderV1RejectsJSONNumberForWireRevision(t *testing.T) {
	var ref contract.ExactRefWireV1
	payload := `{"kind":"praxis.test/current","id":"current-1","revision":9007199254740993,"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
	if err := jsonv1.NewDecoderV1(1024).DecodeStrictV1([]byte(payload), &ref); err == nil {
		t.Fatal("accepted JSON number for wire uint64 revision")
	}
}

func TestDecoderV1RequiresNonNilPointer(t *testing.T) {
	decoder := jsonv1.NewDecoderV1(1024)
	if err := decoder.DecodeStrictV1([]byte(`{"name":"one","nested":{"value":"two"}}`), strictSampleV1{}); err == nil {
		t.Fatal("accepted non-pointer decode target")
	}
}
