package contract

import (
	"fmt"
	"strings"
)

type ContextOwnerSourceKindV1 string

const (
	ContextOwnerSourceMemoryV1    ContextOwnerSourceKindV1 = "memory"
	ContextOwnerSourceKnowledgeV1 ContextOwnerSourceKindV1 = "knowledge"
)

type ContextTypedFactRefV1 struct {
	Kind string  `json:"kind"`
	Ref  FactRef `json:"ref"`
}

func (r ContextTypedFactRefV1) Validate() error {
	if strings.TrimSpace(r.Kind) == "" || strings.Count(r.Kind, "/") != 1 || r.Ref.Validate() != nil {
		return fmt.Errorf("%w: typed context fact ref", ErrInvalid)
	}
	return nil
}

// ContextOwnerSourceItemV1 is a Context-owned materialization binding. Owner
// refs remain exact evidence coordinates; Content is the Context-local copy
// that may be admitted into a pending Frame.
type ContextOwnerSourceItemV1 struct {
	Rank             uint32                  `json:"rank"`
	OwnerItemDigest  Digest                  `json:"owner_item_digest"`
	RecordRef        ContextTypedFactRefV1   `json:"record_ref"`
	StableOwnerChain []ContextTypedFactRefV1 `json:"stable_owner_chain"`
	Content          ContentRef              `json:"content"`
	TokenEstimate    uint64                  `json:"token_estimate"`
	Sensitivity      Sensitivity             `json:"sensitivity"`
	CitationDigest   Digest                  `json:"citation_digest"`
	License          string                  `json:"license,omitempty"`
	ExpiresUnixNano  int64                   `json:"expires_unix_nano"`
	Digest           Digest                  `json:"digest"`
}

func (i ContextOwnerSourceItemV1) digestValue() (Digest, error) {
	copy := i
	copy.Digest = ""
	return DigestJSON(copy)
}

func (i ContextOwnerSourceItemV1) Validate() error {
	if i.OwnerItemDigest.Validate() != nil || i.RecordRef.Validate() != nil || i.Content.Validate() != nil || i.TokenEstimate == 0 || !validSensitivity(i.Sensitivity) || i.CitationDigest.Validate() != nil || i.ExpiresUnixNano <= 0 || i.Digest.Validate() != nil {
		return fmt.Errorf("%w: context owner source item", ErrInvalid)
	}
	seen := make(map[ContextTypedFactRefV1]struct{}, len(i.StableOwnerChain))
	for _, ref := range i.StableOwnerChain {
		if ref.Validate() != nil {
			return fmt.Errorf("%w: context owner source chain", ErrInvalid)
		}
		if _, ok := seen[ref]; ok {
			return fmt.Errorf("%w: duplicate context owner source ref", ErrConflict)
		}
		seen[ref] = struct{}{}
	}
	d, err := i.digestValue()
	if err != nil || d != i.Digest {
		return fmt.Errorf("%w: context owner source item digest", ErrConflict)
	}
	return nil
}

func SealContextOwnerSourceItemV1(i ContextOwnerSourceItemV1) (ContextOwnerSourceItemV1, error) {
	i.StableOwnerChain = append([]ContextTypedFactRefV1(nil), i.StableOwnerChain...)
	i.Digest = ""
	d, err := i.digestValue()
	if err != nil {
		return ContextOwnerSourceItemV1{}, err
	}
	i.Digest = d
	return i, i.Validate()
}

type ContextOwnerSourceContributionV1 struct {
	Owner                   ContextOwnerSourceKindV1   `json:"owner"`
	EnvelopeRef             ContextTypedFactRefV1      `json:"envelope_ref"`
	OwnerProjectionRef      ContextTypedFactRefV1      `json:"owner_projection_ref"`
	StableClosureDigest     Digest                     `json:"stable_closure_digest"`
	StableAssociationDigest Digest                     `json:"stable_association_digest"`
	SourceSessionRef        ContextTypedFactRefV1      `json:"source_session_ref"`
	SourceTurnRef           ContextTypedFactRefV1      `json:"source_turn_ref"`
	SourceTurnOrdinal       uint32                     `json:"source_turn_ordinal"`
	Items                   []ContextOwnerSourceItemV1 `json:"items"`
	CheckedUnixNano         int64                      `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                      `json:"expires_unix_nano"`
	Digest                  Digest                     `json:"digest"`
}

func (c ContextOwnerSourceContributionV1) digestValue() (Digest, error) {
	copy := c
	copy.Digest = ""
	return DigestJSON(copy)
}

func (c ContextOwnerSourceContributionV1) Validate() error {
	if (c.Owner != ContextOwnerSourceMemoryV1 && c.Owner != ContextOwnerSourceKnowledgeV1) || c.EnvelopeRef.Validate() != nil || c.OwnerProjectionRef.Validate() != nil || c.StableClosureDigest.Validate() != nil || c.StableAssociationDigest.Validate() != nil || c.SourceSessionRef.Validate() != nil || c.SourceTurnRef.Validate() != nil || c.SourceTurnOrdinal == 0 || len(c.Items) == 0 || len(c.Items) > 256 || c.CheckedUnixNano <= 0 || c.ExpiresUnixNano <= c.CheckedUnixNano || c.Digest.Validate() != nil {
		return fmt.Errorf("%w: context owner source contribution", ErrInvalid)
	}
	seen := make(map[string]struct{}, len(c.Items))
	var contentBytes uint64
	for index, item := range c.Items {
		if item.Rank != uint32(index) || item.Validate() != nil || item.ExpiresUnixNano > c.ExpiresUnixNano {
			return fmt.Errorf("%w: context owner source contribution item", ErrConflict)
		}
		if item.Content.Length > MaxContextTurnRefreshSourceBytesV1-contentBytes {
			return fmt.Errorf("%w: context owner source contribution content", ErrLimitExceeded)
		}
		contentBytes += item.Content.Length
		key := item.RecordRef.Kind + "\x00" + item.RecordRef.Ref.ID + "\x00" + string(item.RecordRef.Ref.Digest)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: duplicate context owner semantic record", ErrConflict)
		}
		seen[key] = struct{}{}
	}
	d, err := c.digestValue()
	if err != nil || d != c.Digest {
		return fmt.Errorf("%w: context owner contribution digest", ErrConflict)
	}
	return nil
}

func SealContextOwnerSourceContributionV1(c ContextOwnerSourceContributionV1) (ContextOwnerSourceContributionV1, error) {
	c.Items = append([]ContextOwnerSourceItemV1(nil), c.Items...)
	c.Digest = ""
	d, err := c.digestValue()
	if err != nil {
		return ContextOwnerSourceContributionV1{}, err
	}
	c.Digest = d
	return c, c.Validate()
}
