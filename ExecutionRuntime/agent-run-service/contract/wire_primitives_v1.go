package contract

import (
	"strconv"
	"strings"
	"time"
)

const (
	CrossLanguageWireContractVersionV1 = "praxis.cross-language-wire/v1"
	WireCanonicalDecimalStringV1       = "canonical_decimal_string"
	WireCanonicalSignedDecimalStringV1 = "canonical_signed_decimal_string"
	WireRFC3339NanoEncodingV1          = "rfc3339_nano_string"
	WireDigestAlgorithmV1              = "sha256"
)

// WireUint64V1 is always a JSON string. This prevents loss above 2^53 in
// JavaScript and preserves the complete Go uint64 range.
type WireUint64V1 string

func NewWireUint64V1(value uint64) WireUint64V1 {
	return WireUint64V1(strconv.FormatUint(value, 10))
}

func (v WireUint64V1) Uint64V1() (uint64, error) {
	return parseCanonicalUint64V1("wire uint64", string(v), true)
}

func (v WireUint64V1) ValidatePositiveV1(field string) error {
	_, err := parseCanonicalUint64V1(field, string(v), false)
	return err
}

// WireInt64V1 is the generic signed int64 primitive. It permits the complete
// int64 range; business fields impose positivity separately.
type WireInt64V1 string

func NewWireInt64V1(value int64) WireInt64V1 {
	return WireInt64V1(strconv.FormatInt(value, 10))
}

func (v WireInt64V1) Int64V1() (int64, error) {
	return parseCanonicalInt64V1("wire int64", string(v))
}

// WireUnixNanoV1 is deliberately distinct from WireRFC3339NanoV1. It supports
// the full signed int64 owner coordinate; timestamp fields call ValidatePositiveV1.
type WireUnixNanoV1 string

func NewWireUnixNanoV1(value int64) WireUnixNanoV1 {
	return WireUnixNanoV1(strconv.FormatInt(value, 10))
}

func (v WireUnixNanoV1) Int64V1() (int64, error) {
	return parseCanonicalInt64V1("wire UnixNano", string(v))
}

func (v WireUnixNanoV1) ValidatePositiveV1(field string) error {
	value, err := parseCanonicalInt64V1(field, string(v))
	if err != nil {
		return err
	}
	if value <= 0 {
		return NewError(FaultInvalidArgumentV1, "wire_unix_nano_not_positive", field+" must be positive")
	}
	return nil
}

type WireRFC3339NanoV1 string

func NewWireRFC3339NanoV1(value time.Time) (WireRFC3339NanoV1, error) {
	if value.IsZero() {
		return "", NewError(FaultInvalidArgumentV1, "wire_rfc3339_nano_zero", "RFC3339Nano time must be non-zero")
	}
	encoded := WireRFC3339NanoV1(value.UTC().Format(time.RFC3339Nano))
	if _, err := encoded.TimeV1(); err != nil {
		return "", err
	}
	return encoded, nil
}

func (v WireRFC3339NanoV1) TimeV1() (time.Time, error) {
	raw := string(v)
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || parsed.IsZero() || !strings.HasSuffix(raw, "Z") || parsed.Location() != time.UTC || parsed.UTC().Format(time.RFC3339Nano) != raw {
		return time.Time{}, NewError(FaultInvalidArgumentV1, "wire_rfc3339_nano_not_canonical", "RFC3339Nano must use its canonical UTC Z string encoding")
	}
	return parsed, nil
}

func parseCanonicalUint64V1(field, raw string, allowZero bool) (uint64, error) {
	if raw == "" || raw != strings.TrimSpace(raw) || (len(raw) > 1 && raw[0] == '0') {
		return 0, NewError(FaultInvalidArgumentV1, "wire_decimal_not_canonical", field+" must be a canonical unsigned decimal string")
	}
	for _, char := range raw {
		if char < '0' || char > '9' {
			return 0, NewError(FaultInvalidArgumentV1, "wire_decimal_not_canonical", field+" must be a canonical unsigned decimal string")
		}
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, NewError(FaultInvalidArgumentV1, "wire_decimal_overflow", field+" exceeds uint64")
	}
	if value == 0 && !allowZero {
		return 0, NewError(FaultInvalidArgumentV1, "wire_decimal_zero", field+" must be positive")
	}
	return value, nil
}

func parseCanonicalInt64V1(field, raw string) (int64, error) {
	if raw == "" || raw != strings.TrimSpace(raw) || raw[0] == '+' || raw == "-0" || (len(raw) > 1 && raw[0] == '0') || (len(raw) > 2 && raw[0] == '-' && raw[1] == '0') {
		return 0, NewError(FaultInvalidArgumentV1, "wire_signed_decimal_not_canonical", field+" must be a canonical signed int64 decimal string")
	}
	start := 0
	if raw[0] == '-' {
		if len(raw) == 1 {
			return 0, NewError(FaultInvalidArgumentV1, "wire_signed_decimal_not_canonical", field+" must contain digits")
		}
		start = 1
	}
	for _, char := range raw[start:] {
		if char < '0' || char > '9' {
			return 0, NewError(FaultInvalidArgumentV1, "wire_signed_decimal_not_canonical", field+" must be a canonical signed int64 decimal string")
		}
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, NewError(FaultInvalidArgumentV1, "wire_signed_decimal_overflow", field+" exceeds int64")
	}
	return value, nil
}

type ExactRefWireV1 struct {
	Kind     string       `json:"kind"`
	ID       string       `json:"id"`
	Revision WireUint64V1 `json:"revision"`
	Digest   DigestV1     `json:"digest"`
}

func (r ExactRefWireV1) Validate() error {
	if err := ValidateIdentifierV1("exact ref kind", r.Kind); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("exact ref id", r.ID); err != nil {
		return err
	}
	if err := r.Revision.ValidatePositiveV1("exact ref revision"); err != nil {
		return err
	}
	return r.Digest.Validate()
}

type WireValidityWindowV1 struct {
	CheckedUnixNano WireUnixNanoV1 `json:"checked_unix_nano"`
	ExpiresUnixNano WireUnixNanoV1 `json:"expires_unix_nano"`
}

func (w WireValidityWindowV1) Validate() error {
	if err := w.CheckedUnixNano.ValidatePositiveV1("checked UnixNano"); err != nil {
		return err
	}
	if err := w.ExpiresUnixNano.ValidatePositiveV1("expires UnixNano"); err != nil {
		return err
	}
	checked, _ := w.CheckedUnixNano.Int64V1()
	expires, _ := w.ExpiresUnixNano.Int64V1()
	if expires <= checked {
		return NewError(FaultInvalidArgumentV1, "wire_validity_window_invalid", "expiry must be after the checked watermark")
	}
	return nil
}

func (w WireValidityWindowV1) ValidateAtV1(now WireUnixNanoV1) error {
	if err := w.Validate(); err != nil {
		return err
	}
	if err := now.ValidatePositiveV1("current UnixNano"); err != nil {
		return err
	}
	checked, _ := w.CheckedUnixNano.Int64V1()
	expires, _ := w.ExpiresUnixNano.Int64V1()
	current, _ := now.Int64V1()
	if current < checked {
		return NewError(FaultPreconditionFailedV1, "clock_regression", "current clock is before the checked watermark")
	}
	if current >= expires {
		return NewError(FaultPreconditionFailedV1, "wire_current_expired", "current clock reached or exceeded expiry")
	}
	return nil
}

func (w WireValidityWindowV1) ValidateWithinRequestV1(requested, notAfter WireUnixNanoV1) error {
	if err := w.Validate(); err != nil {
		return err
	}
	if err := requested.ValidatePositiveV1("request UnixNano"); err != nil {
		return err
	}
	if err := notAfter.ValidatePositiveV1("request not-after UnixNano"); err != nil {
		return err
	}
	requestTime, _ := requested.Int64V1()
	requestExpiry, _ := notAfter.Int64V1()
	checked, _ := w.CheckedUnixNano.Int64V1()
	expires, _ := w.ExpiresUnixNano.Int64V1()
	if requestExpiry <= requestTime {
		return NewError(FaultInvalidArgumentV1, "request_window_invalid", "request not-after must be after request time")
	}
	if checked < requestTime || checked >= requestExpiry || expires > requestExpiry {
		return NewError(FaultPreconditionFailedV1, "result_window_outside_request", "result window is outside the exact request interval")
	}
	return nil
}

type CrossLanguageWirePrimitivesV1 struct {
	ContractVersion       string   `json:"contract_version"`
	Uint64Encoding        string   `json:"uint64_encoding"`
	SignedInt64Encoding   string   `json:"signed_int64_encoding"`
	UnixNanoEncoding      string   `json:"unix_nano_encoding"`
	RFC3339NanoEncoding   string   `json:"rfc3339_nano_encoding"`
	ExactRefRequiredParts []string `json:"exact_ref_required_parts"`
	DigestAlgorithm       string   `json:"digest_algorithm"`
	DescriptorDigest      DigestV1 `json:"descriptor_digest"`
}

func CanonicalCrossLanguageWirePrimitivesV1() CrossLanguageWirePrimitivesV1 {
	value := CrossLanguageWirePrimitivesV1{
		ContractVersion:       CrossLanguageWireContractVersionV1,
		Uint64Encoding:        WireCanonicalDecimalStringV1,
		SignedInt64Encoding:   WireCanonicalSignedDecimalStringV1,
		UnixNanoEncoding:      WireCanonicalSignedDecimalStringV1,
		RFC3339NanoEncoding:   WireRFC3339NanoEncodingV1,
		ExactRefRequiredParts: []string{"kind", "id", "revision", "digest"},
		DigestAlgorithm:       WireDigestAlgorithmV1,
	}
	digest, _ := value.digestV1()
	value.DescriptorDigest = digest
	return value
}

func (v CrossLanguageWirePrimitivesV1) digestV1() (DigestV1, error) {
	clone := v
	clone.DescriptorDigest = ""
	return DigestJSONV1(struct {
		Domain string                        `json:"domain"`
		Type   string                        `json:"type"`
		Body   CrossLanguageWirePrimitivesV1 `json:"body"`
	}{"praxis.cross-language-wire", "CrossLanguageWirePrimitivesV1", clone})
}

func (v CrossLanguageWirePrimitivesV1) Validate() error {
	canonical := CanonicalCrossLanguageWirePrimitivesV1()
	if v.ContractVersion != canonical.ContractVersion || v.Uint64Encoding != canonical.Uint64Encoding || v.SignedInt64Encoding != canonical.SignedInt64Encoding || v.UnixNanoEncoding != canonical.UnixNanoEncoding || v.RFC3339NanoEncoding != canonical.RFC3339NanoEncoding || v.DigestAlgorithm != canonical.DigestAlgorithm || len(v.ExactRefRequiredParts) != len(canonical.ExactRefRequiredParts) {
		return NewError(FaultRevisionConflictV1, "wire_primitives_drift", "cross-language wire primitives drifted")
	}
	for index := range canonical.ExactRefRequiredParts {
		if v.ExactRefRequiredParts[index] != canonical.ExactRefRequiredParts[index] {
			return NewError(FaultRevisionConflictV1, "wire_exact_ref_schema_drift", "cross-language exact ref schema drifted")
		}
	}
	digest, err := v.digestV1()
	if err != nil || digest != v.DescriptorDigest {
		return NewError(FaultRevisionConflictV1, "wire_primitives_digest_drift", "cross-language wire primitive digest drifted")
	}
	return nil
}
