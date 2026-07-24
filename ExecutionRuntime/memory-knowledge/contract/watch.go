package contract

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	ChangeEventObjectKindV1 = "memory_knowledge_change_event"
	WatchCursorObjectKindV1 = "memory_knowledge_watch_cursor"
	ChangePageObjectKindV1  = "memory_knowledge_change_page"
)

type ChangeKind string

const (
	ChangeRecordCommitted  ChangeKind = "record_committed"
	ChangeRecordCorrected  ChangeKind = "record_corrected"
	ChangeRecordPinned     ChangeKind = "record_pinned"
	ChangeRecordArchived   ChangeKind = "record_archived"
	ChangeRecordForgotten  ChangeKind = "record_forgotten"
	ChangeRecordMerged     ChangeKind = "record_merged"
	ChangeRecordWithdrawn  ChangeKind = "record_withdrawn"
	ChangeSourceRegistered ChangeKind = "source_registered"
	ChangeSourceRefreshed  ChangeKind = "source_refreshed"
	ChangeSourceDeprecated ChangeKind = "source_deprecated"
	ChangeSourceWithdrawn  ChangeKind = "source_withdrawn"
)

// ChangeEventV1 is metadata-only. SubjectRef is an exact Owner fact reference;
// event payloads never carry record or source body bytes.
type ChangeEventV1 struct {
	ContractVersion string      `json:"contract_version"`
	ObjectKind      string      `json:"object_kind"`
	Ref             Ref         `json:"ref"`
	Owner           OwnerDomain `json:"owner"`
	TenantID        string      `json:"tenant_id"`
	Sequence        uint64      `json:"sequence"`
	Kind            ChangeKind  `json:"kind"`
	SubjectRef      Ref         `json:"subject_ref"`
	PreviousRef     Ref         `json:"previous_ref,omitempty"`
	AuthorityRef    Ref         `json:"authority_ref"`
	PolicyRef       Ref         `json:"policy_ref"`
	Scope           string      `json:"scope"`
	Sensitivity     string      `json:"sensitivity"`
	License         string      `json:"license,omitempty"`
	OccurredAt      time.Time   `json:"occurred_at"`
	Digest          string      `json:"digest"`
}

type WatchCursorV1 struct {
	ContractVersion string      `json:"contract_version"`
	ObjectKind      string      `json:"object_kind"`
	Ref             Ref         `json:"ref"`
	Owner           OwnerDomain `json:"owner"`
	TenantID        string      `json:"tenant_id"`
	AuthorityRef    Ref         `json:"authority_ref"`
	PolicyRef       Ref         `json:"policy_ref"`
	ViewRef         Ref         `json:"view_ref"`
	Sequence        uint64      `json:"sequence"`
	CreatedAt       time.Time   `json:"created_at"`
	ExpiresAt       time.Time   `json:"expires_at"`
	Digest          string      `json:"digest"`
}

type WatchRequestV1 struct {
	ViewRef   Ref            `json:"view_ref"`
	Cursor    *WatchCursorV1 `json:"cursor,omitempty"`
	Limit     int            `json:"limit"`
	ExpiresAt time.Time      `json:"expires_at"`
}

type ChangePageV1 struct {
	ContractVersion string          `json:"contract_version"`
	ObjectKind      string          `json:"object_kind"`
	Ref             Ref             `json:"ref"`
	Owner           OwnerDomain     `json:"owner"`
	TenantID        string          `json:"tenant_id"`
	ViewRef         Ref             `json:"view_ref"`
	BoundaryRef     Ref             `json:"boundary_ref"`
	Events          []ChangeEventV1 `json:"events"`
	NextCursor      WatchCursorV1   `json:"next_cursor"`
	CreatedAt       time.Time       `json:"created_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
	Digest          string          `json:"digest"`
}

func SealChangeEventV1(in ChangeEventV1) (ChangeEventV1, error) {
	in.ContractVersion, in.ObjectKind = FrameworkContractVersionV1, ChangeEventObjectKindV1
	in.OccurredAt = in.OccurredAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	digest, err := Digest(in)
	if err != nil {
		return ChangeEventV1{}, err
	}
	in.Ref.Digest, in.Digest = digest, digest
	if err := in.Validate(); err != nil {
		return ChangeEventV1{}, err
	}
	return in, nil
}

func (in ChangeEventV1) Validate() error {
	if in.ContractVersion != FrameworkContractVersionV1 || in.ObjectKind != ChangeEventObjectKindV1 || (in.Owner != OwnerMemory && in.Owner != OwnerKnowledge) || strings.TrimSpace(in.TenantID) == "" || in.Sequence == 0 || !validChangeKind(in.Owner, in.Kind) || in.Ref.Validate() != nil || in.SubjectRef.Validate() != nil || in.AuthorityRef.Validate() != nil || in.PolicyRef.Validate() != nil || strings.TrimSpace(in.Scope) == "" || strings.TrimSpace(in.Sensitivity) == "" || in.OccurredAt.IsZero() {
		return fmt.Errorf("%w: change event", ErrInvalidArgument)
	}
	if in.PreviousRef != (Ref{}) && in.PreviousRef.Validate() != nil {
		return fmt.Errorf("%w: previous change ref", ErrInvalidArgument)
	}
	if in.Owner == OwnerKnowledge && strings.TrimSpace(in.License) == "" {
		return fmt.Errorf("%w: knowledge change license", ErrInvalidArgument)
	}
	if in.Owner == OwnerMemory && in.License != "" {
		return fmt.Errorf("%w: memory change license", ErrInvalidArgument)
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return fmt.Errorf("%w: change event digest", ErrEvidenceConflict)
	}
	return nil
}

func SealWatchCursorV1(in WatchCursorV1) (WatchCursorV1, error) {
	in.ContractVersion, in.ObjectKind = FrameworkContractVersionV1, WatchCursorObjectKindV1
	in.CreatedAt, in.ExpiresAt = in.CreatedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	digest, err := Digest(in)
	if err != nil {
		return WatchCursorV1{}, err
	}
	in.Ref.Digest, in.Digest = digest, digest
	if err := in.Validate(in.CreatedAt); err != nil {
		return WatchCursorV1{}, err
	}
	return in, nil
}

func (in WatchCursorV1) Validate(now time.Time) error {
	if in.ContractVersion != FrameworkContractVersionV1 || in.ObjectKind != WatchCursorObjectKindV1 || (in.Owner != OwnerMemory && in.Owner != OwnerKnowledge) || strings.TrimSpace(in.TenantID) == "" || in.Ref.Validate() != nil || in.Ref.Revision != in.Sequence+1 || in.AuthorityRef.Validate() != nil || in.PolicyRef.Validate() != nil || in.ViewRef.Validate() != nil || in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: watch cursor", ErrInvalidArgument)
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return fmt.Errorf("%w: watch cursor digest", ErrEvidenceConflict)
	}
	return nil
}

func SealChangePageV1(in ChangePageV1) (ChangePageV1, error) {
	in.ContractVersion, in.ObjectKind = FrameworkContractVersionV1, ChangePageObjectKindV1
	in.CreatedAt, in.ExpiresAt = in.CreatedAt.UTC(), in.ExpiresAt.UTC()
	if in.Events == nil {
		in.Events = []ChangeEventV1{}
	}
	in.Ref.Digest, in.Digest = "", ""
	digest, err := Digest(in)
	if err != nil {
		return ChangePageV1{}, err
	}
	in.Ref.Digest, in.Digest = digest, digest
	if err := in.Validate(in.CreatedAt); err != nil {
		return ChangePageV1{}, err
	}
	return in, nil
}

func (in ChangePageV1) Validate(now time.Time) error {
	if in.ContractVersion != FrameworkContractVersionV1 || in.ObjectKind != ChangePageObjectKindV1 || (in.Owner != OwnerMemory && in.Owner != OwnerKnowledge) || strings.TrimSpace(in.TenantID) == "" || in.Ref.Validate() != nil || in.ViewRef.Validate() != nil || in.BoundaryRef.Validate() != nil || in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || !in.ExpiresAt.After(now) || in.NextCursor.Validate(now) != nil || in.NextCursor.Owner != in.Owner || in.NextCursor.TenantID != in.TenantID || !SameRef(in.NextCursor.ViewRef, in.ViewRef) {
		return fmt.Errorf("%w: change page", ErrInvalidArgument)
	}
	previous := uint64(0)
	for _, event := range in.Events {
		if event.Validate() != nil || event.Owner != in.Owner || event.TenantID != in.TenantID || event.Sequence <= previous || event.Sequence > in.NextCursor.Sequence {
			return fmt.Errorf("%w: change page event", ErrEvidenceConflict)
		}
		previous = event.Sequence
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return fmt.Errorf("%w: change page digest", ErrEvidenceConflict)
	}
	return nil
}

func CloneChangeEvents(in []ChangeEventV1) []ChangeEventV1 {
	return slices.Clone(in)
}

func validChangeKind(owner OwnerDomain, kind ChangeKind) bool {
	if owner == OwnerMemory {
		return kind == ChangeRecordCommitted || kind == ChangeRecordCorrected || kind == ChangeRecordPinned || kind == ChangeRecordArchived || kind == ChangeRecordForgotten || kind == ChangeRecordMerged
	}
	return kind == ChangeRecordCommitted || kind == ChangeRecordCorrected || kind == ChangeRecordWithdrawn || kind == ChangeSourceRegistered || kind == ChangeSourceRefreshed || kind == ChangeSourceDeprecated || kind == ChangeSourceWithdrawn
}
