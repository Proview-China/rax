package contract

import (
	"errors"
	"testing"
)

func TestContextOwnerSourceContributionAggregateContentLimitV1(t *testing.T) {
	item := func(rank uint32, id string) ContextOwnerSourceItemV1 {
		value, err := SealContextOwnerSourceItemV1(ContextOwnerSourceItemV1{
			Rank: rank, OwnerItemDigest: DigestBytes([]byte("owner-" + id)),
			RecordRef:        ContextTypedFactRefV1{Kind: "memory/record", Ref: ownerSourceTestRefV1("record-" + id)},
			StableOwnerChain: []ContextTypedFactRefV1{},
			Content:          ContentRef{Ref: "content-" + id, Digest: DigestBytes([]byte("content-" + id)), Length: MaxContextTurnRefreshSourceBytesV1/2 + 1},
			TokenEstimate:    1, Sensitivity: SensitivityInternal, CitationDigest: DigestBytes([]byte("citation-" + id)),
			ExpiresUnixNano: 2,
		})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	_, err := SealContextOwnerSourceContributionV1(ContextOwnerSourceContributionV1{
		Owner:               ContextOwnerSourceMemoryV1,
		EnvelopeRef:         ContextTypedFactRefV1{Kind: "memory/envelope", Ref: ownerSourceTestRefV1("envelope")},
		OwnerProjectionRef:  ContextTypedFactRefV1{Kind: "memory/projection", Ref: ownerSourceTestRefV1("projection")},
		StableClosureDigest: DigestBytes([]byte("closure")), StableAssociationDigest: DigestBytes([]byte("association")),
		SourceSessionRef:  ContextTypedFactRefV1{Kind: "runtime/session", Ref: ownerSourceTestRefV1("session")},
		SourceTurnRef:     ContextTypedFactRefV1{Kind: "runtime/turn", Ref: ownerSourceTestRefV1("turn")},
		SourceTurnOrdinal: 1, Items: []ContextOwnerSourceItemV1{item(0, "a"), item(1, "b")},
		CheckedUnixNano: 1, ExpiresUnixNano: 2,
	})
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("aggregate owner content limit was bypassed: %v", err)
	}
}

func ownerSourceTestRefV1(id string) FactRef {
	return FactRef{ID: id, Revision: 1, Digest: DigestBytes([]byte(id))}
}
