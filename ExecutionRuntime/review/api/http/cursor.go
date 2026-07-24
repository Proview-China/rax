package reviewhttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const cursorContractV1 = "praxis.review/http-list-cursor-v1"
const traceCursorContractV2 = "praxis.review/http-trace-cursor-v2"

type listCursorV1 struct {
	ContractVersion string                 `json:"contract_version"`
	TenantID        core.TenantID          `json:"tenant_id"`
	States          []contract.CaseStateV1 `json:"states"`
	AfterID         string                 `json:"after_id"`
	ExpiresUnixNano int64                  `json:"expires_unix_nano"`
}

func encodeCursorV1(value listCursorV1, key []byte) (string, error) {
	value.ContractVersion = cursorContractV1
	sort.Slice(value.States, func(i, j int) bool { return value.States[i] < value.States[j] })
	payload, err := json.Marshal(value)
	if err != nil {
		return "", core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review list cursor could not be encoded")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	framed := append(payload, mac.Sum(nil)...)
	return base64.RawURLEncoding.EncodeToString(framed), nil
}

func decodeCursorV1(encoded string, key []byte, now time.Time) (listCursorV1, error) {
	var zero listCursorV1
	framed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(framed) <= sha256.Size || len(framed) > 8192 {
		return zero, core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceCursorInvalid, "review list cursor is malformed")
	}
	payload, signature := framed[:len(framed)-sha256.Size], framed[len(framed)-sha256.Size:]
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return zero, core.NewError(core.ErrorConflict, core.ReasonEvidenceCursorInvalid, "review list cursor signature drifted")
	}
	if err := core.DecodeStrictJSON(payload, &zero); err != nil {
		return listCursorV1{}, err
	}
	if zero.ContractVersion != cursorContractV1 || zero.TenantID == "" || zero.AfterID == "" || zero.ExpiresUnixNano <= 0 {
		return listCursorV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceCursorInvalid, "review list cursor is incomplete")
	}
	for _, state := range zero.States {
		if err := contract.ValidateCaseStateV1(state); err != nil {
			return listCursorV1{}, err
		}
	}
	if now.IsZero() || now.UnixNano() >= zero.ExpiresUnixNano {
		return listCursorV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceCursorInvalid, "review list cursor expired")
	}
	return zero, nil
}

type traceCursorV2 struct {
	ContractVersion string                      `json:"contract_version"`
	TenantID        core.TenantID               `json:"tenant_id"`
	CaseID          string                      `json:"case_id"`
	After           reviewport.TracePageAfterV2 `json:"after"`
	ExpiresUnixNano int64                       `json:"expires_unix_nano"`
}

func encodeTraceCursorV2(value traceCursorV2, key []byte) (string, error) {
	value.ContractVersion = traceCursorContractV2
	payload, err := json.Marshal(value)
	if err != nil {
		return "", core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review Trace cursor could not be encoded")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(append(payload, mac.Sum(nil)...)), nil
}

func decodeTraceCursorV2(encoded string, key []byte, now time.Time) (traceCursorV2, error) {
	var value traceCursorV2
	framed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(framed) <= sha256.Size || len(framed) > 8192 {
		return value, core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceCursorInvalid, "review Trace cursor is malformed")
	}
	payload, signature := framed[:len(framed)-sha256.Size], framed[len(framed)-sha256.Size:]
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return traceCursorV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceCursorInvalid, "review Trace cursor signature drifted")
	}
	if err := core.DecodeStrictJSON(payload, &value); err != nil {
		return traceCursorV2{}, err
	}
	if value.ContractVersion != traceCursorContractV2 || value.TenantID == "" || value.CaseID == "" || value.ExpiresUnixNano <= 0 {
		return traceCursorV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceCursorInvalid, "review Trace cursor is incomplete")
	}
	if err := value.After.Validate(); err != nil {
		return traceCursorV2{}, err
	}
	if now.IsZero() || now.UnixNano() >= value.ExpiresUnixNano {
		return traceCursorV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceCursorInvalid, "review Trace cursor expired")
	}
	return value, nil
}
