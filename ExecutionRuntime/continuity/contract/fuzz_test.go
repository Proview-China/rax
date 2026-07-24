package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

func FuzzDecodeTimelineCursorCanonicalRoundTrip(f *testing.F) {
	now := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	query := contract.TimelineQuery{
		LedgerScopeDigest: "ledger-scope-1", AuthorityWatermark: "authority-1",
		PolicyWatermark: "policy-1", PageLimit: 10,
	}
	queryDigest, err := query.Digest()
	if err != nil {
		f.Fatal(err)
	}
	seed, err := (contract.TimelineCursor{
		LedgerScopeDigest: query.LedgerScopeDigest, QueryDigest: queryDigest,
		AuthorityWatermark: query.AuthorityWatermark, PolicyWatermark: query.PolicyWatermark,
		ProjectionSchema: contract.ProjectionSchema, PageLimit: query.PageLimit,
		IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(), State: "active",
	}).Encode()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add("")
	f.Add("not-base64")

	f.Fuzz(func(t *testing.T, token string) {
		cursor, err := contract.DecodeTimelineCursor(token)
		if err != nil {
			return
		}
		roundTrip, err := cursor.Encode()
		if err != nil {
			t.Fatalf("decoded cursor could not be encoded: %v", err)
		}
		if roundTrip != token {
			t.Fatalf("decoder accepted noncanonical token: %q != %q", token, roundTrip)
		}
	})
}
