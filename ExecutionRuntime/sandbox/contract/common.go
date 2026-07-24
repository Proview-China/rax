package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

const (
	ContractFamily = "praxis.sandbox/v2"
	DigestSizeHex  = sha256.Size * 2
)

type Ref struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   string `json:"digest"`
}

func (r Ref) ValidateShape(name string) error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("%s id is required", name)
	}
	if r.Revision == 0 {
		return fmt.Errorf("%s revision must be positive", name)
	}
	if !ValidDigest(r.Digest) {
		return fmt.Errorf("%s digest must be a sha256 hex digest", name)
	}
	return nil
}

func SameRef(a, b Ref) bool {
	return a.ID == b.ID && a.Revision == b.Revision && a.Digest == b.Digest
}

type Meta struct {
	ContractVersion string `json:"contract_version"`
	ID              string `json:"id"`
	Revision        uint64 `json:"revision"`
	Digest          string `json:"digest"`
	CreatedUnixNano int64  `json:"created_unix_nano"`
	UpdatedUnixNano int64  `json:"updated_unix_nano"`
	ExpiresUnixNano int64  `json:"expires_unix_nano"`
}

func (m Meta) Ref() Ref {
	return Ref{ID: m.ID, Revision: m.Revision, Digest: m.Digest}
}

func (m Meta) ValidateShape() error {
	if m.ContractVersion != ContractFamily {
		return fmt.Errorf("contract version must be %q", ContractFamily)
	}
	if err := m.Ref().ValidateShape("meta"); err != nil {
		return err
	}
	if m.CreatedUnixNano <= 0 || m.UpdatedUnixNano < m.CreatedUnixNano {
		return errors.New("invalid object timestamps")
	}
	if m.ExpiresUnixNano <= m.CreatedUnixNano {
		return errors.New("object expiry must be after creation time")
	}
	return nil
}

func (m Meta) ValidateCurrent(now time.Time) error {
	if err := m.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() {
		return errors.New("current validation requires a clock")
	}
	if now.UnixNano() < m.UpdatedUnixNano {
		return errors.New("object update time is in the future")
	}
	if now.UnixNano() >= m.ExpiresUnixNano {
		return errors.New("object is expired")
	}
	return nil
}

func NewMeta(id string, revision uint64, now, expires time.Time, discriminator string, payload any) (Meta, error) {
	if strings.TrimSpace(id) == "" || revision == 0 {
		return Meta{}, errors.New("id and positive revision are required")
	}
	if now.IsZero() || !expires.After(now) {
		return Meta{}, errors.New("valid creation and expiry times are required")
	}
	digest, err := Digest(discriminator, payload)
	if err != nil {
		return Meta{}, err
	}
	return Meta{
		ContractVersion: ContractFamily,
		ID:              id,
		Revision:        revision,
		Digest:          digest,
		CreatedUnixNano: now.UnixNano(),
		UpdatedUnixNano: now.UnixNano(),
		ExpiresUnixNano: expires.UnixNano(),
	}, nil
}

func NextMeta(current Meta, now time.Time, discriminator string, payload any) (Meta, error) {
	if err := current.ValidateShape(); err != nil {
		return Meta{}, fmt.Errorf("current meta is invalid: %w", err)
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano {
		return Meta{}, errors.New("cannot advance meta with invalid clock")
	}
	digest, err := Digest(discriminator, payload)
	if err != nil {
		return Meta{}, err
	}
	return Meta{
		ContractVersion: ContractFamily,
		ID:              current.ID,
		Revision:        current.Revision + 1,
		Digest:          digest,
		CreatedUnixNano: current.CreatedUnixNano,
		UpdatedUnixNano: now.UnixNano(),
		ExpiresUnixNano: current.ExpiresUnixNano,
	}, nil
}

func Digest(discriminator string, value any) (string, error) {
	if strings.TrimSpace(discriminator) == "" {
		return "", errors.New("digest discriminator is required")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal digest payload: %w", err)
	}
	h := sha256.New()
	_, _ = io.WriteString(h, ContractFamily)
	_, _ = h.Write([]byte{0})
	_, _ = io.WriteString(h, discriminator)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(payload)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ValidDigest(value string) bool {
	value = strings.TrimPrefix(value, "sha256:")
	if len(value) != DigestSizeHex {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func DecodeStrict[T any](data []byte) (T, error) {
	var zero T
	if len(bytes.TrimSpace(data)) == 0 {
		return zero, errors.New("empty json document")
	}
	duplicateDecoder := json.NewDecoder(bytes.NewReader(data))
	duplicateDecoder.UseNumber()
	if err := scanJSONValue(duplicateDecoder); err != nil {
		return zero, err
	}
	if _, err := duplicateDecoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return zero, errors.New("trailing json document")
		}
		return zero, fmt.Errorf("trailing json document: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result T
	if err := decoder.Decode(&result); err != nil {
		return zero, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return zero, errors.New("trailing json document")
		}
		return zero, fmt.Errorf("trailing json document: %w", err)
	}
	return result, nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("json object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate json key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("unterminated json object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("unterminated json array")
		}
	default:
		return fmt.Errorf("unexpected json delimiter %q", delim)
	}
	return nil
}

func ValidateSortedUnique(values []string, name string) error {
	if !slices.IsSorted(values) {
		return fmt.Errorf("%s must be sorted", name)
	}
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty value", name)
		}
		if i > 0 && values[i-1] == value {
			return fmt.Errorf("%s contains duplicate %q", name, value)
		}
	}
	return nil
}
