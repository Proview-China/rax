package contract_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

func TestCrossLanguageWirePrimitivesV1GoldenBoundaries(t *testing.T) {
	if err := contract.CanonicalCrossLanguageWirePrimitivesV1().Validate(); err != nil {
		t.Fatal(err)
	}
	values := []uint64{0, 1, 1<<53 - 1, 1 << 53, 1<<53 + 1, math.MaxUint64}
	for _, value := range values {
		wire := contract.NewWireUint64V1(value)
		got, err := wire.Uint64V1()
		if err != nil || got != value {
			t.Fatalf("uint64 round trip value=%d got=%d err=%v", value, got, err)
		}
		encoded, err := json.Marshal(struct {
			Value contract.WireUint64V1 `json:"value"`
		}{wire})
		if err != nil || !strings.Contains(string(encoded), `"value":"`+string(wire)+`"`) {
			t.Fatalf("uint64 must marshal as string: %s err=%v", encoded, err)
		}
	}

	for _, value := range []int64{math.MinInt64, -1, 0, 1, math.MaxInt64} {
		wire := contract.NewWireInt64V1(value)
		got, err := wire.Int64V1()
		if err != nil || got != value {
			t.Fatalf("int64 round trip value=%d got=%d err=%v", value, got, err)
		}
		unix := contract.NewWireUnixNanoV1(value)
		gotUnix, err := unix.Int64V1()
		if err != nil || gotUnix != value {
			t.Fatalf("UnixNano round trip value=%d got=%d err=%v", value, gotUnix, err)
		}
	}
}

func TestCrossLanguageWirePrimitivesV1RejectNonCanonicalNumbers(t *testing.T) {
	for _, raw := range []contract.WireUint64V1{"", "00", "01", "+1", "-1", "1e3", "18446744073709551616"} {
		if _, err := raw.Uint64V1(); err == nil {
			t.Fatalf("accepted non-canonical uint64 %q", raw)
		}
	}
	for _, raw := range []contract.WireInt64V1{"", "00", "01", "-0", "-01", "+1", "1e3", "9223372036854775808", "-9223372036854775809"} {
		if _, err := raw.Int64V1(); err == nil {
			t.Fatalf("accepted non-canonical int64 %q", raw)
		}
	}
	for _, raw := range []contract.WireUnixNanoV1{"-1", "0"} {
		if err := raw.ValidatePositiveV1("business timestamp"); err == nil {
			t.Fatalf("accepted non-positive business UnixNano %q", raw)
		}
	}
}

func TestWireRFC3339NanoV1NormalizesUTCAndRejectsOffset(t *testing.T) {
	instant := time.Date(2026, 7, 30, 18, 0, 0, 123456789, time.FixedZone("CST", 8*60*60))
	wire, err := contract.NewWireRFC3339NanoV1(instant)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(wire), "2026-07-30T10:00:00.123456789Z"; got != want {
		t.Fatalf("UTC normalization got=%s want=%s", got, want)
	}
	parsed, err := wire.TimeV1()
	if err != nil || !parsed.Equal(instant) || parsed.Location() != time.UTC {
		t.Fatalf("UTC round trip parsed=%v err=%v", parsed, err)
	}
	if _, err := (contract.WireRFC3339NanoV1("2026-07-30T18:00:00.123456789+08:00")).TimeV1(); err == nil {
		t.Fatal("accepted non-UTC RFC3339Nano offset")
	}
	if _, err := contract.NewWireRFC3339NanoV1(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("constructed an RFC3339Nano value with an out-of-range year")
	}
}

func TestWireValidityWindowV1TTLAndClockRegression(t *testing.T) {
	window := contract.WireValidityWindowV1{
		CheckedUnixNano: contract.NewWireUnixNanoV1(100),
		ExpiresUnixNano: contract.NewWireUnixNanoV1(200),
	}
	cases := []struct {
		name    string
		now     int64
		wantErr bool
		reason  string
	}{
		{"before_checked", 99, true, "clock_regression"},
		{"at_checked", 100, false, ""},
		{"before_expiry", 199, false, ""},
		{"at_expiry", 200, true, "wire_current_expired"},
		{"after_expiry", 201, true, "wire_current_expired"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := window.ValidateAtV1(contract.NewWireUnixNanoV1(tc.now))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if tc.reason != "" && !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("err=%v missing reason=%s", err, tc.reason)
			}
		})
	}
}

func TestWireValidityWindowV1MustStayWithinExactRequest(t *testing.T) {
	requested, notAfter := contract.NewWireUnixNanoV1(100), contract.NewWireUnixNanoV1(200)
	cases := []struct {
		name    string
		checked int64
		expires int64
		wantErr bool
	}{
		{"exact_bounds", 100, 200, false},
		{"old_snapshot", 99, 150, true},
		{"checked_at_not_after", 200, 201, true},
		{"expiry_after_request", 150, 201, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			window := contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(tc.checked), ExpiresUnixNano: contract.NewWireUnixNanoV1(tc.expires)}
			err := window.ValidateWithinRequestV1(requested, notAfter)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestExactRefWireV1KeepsRevisionAsString(t *testing.T) {
	ref := exactRefV1(t, "praxis.test/current", "current-1", 1<<53+1)
	if err := ref.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"revision":"9007199254740993"`) || strings.Contains(string(encoded), `"revision":9007199254740993`) {
		t.Fatalf("revision was not string encoded: %s", encoded)
	}
}
