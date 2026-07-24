package contract

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const ExportManifestObjectKindV1 = "memory_knowledge_export_manifest"

type ExportEntryV1 struct {
	RecordRef    Ref         `json:"record_ref"`
	ContentRef   *ContentRef `json:"content_ref,omitempty"`
	SourceRefs   []Ref       `json:"source_refs"`
	EvidenceRefs []Ref       `json:"evidence_refs"`
	Scope        string      `json:"scope"`
	Sensitivity  string      `json:"sensitivity"`
	License      string      `json:"license,omitempty"`
}
type ExportManifestV1 struct {
	ContractVersion string          `json:"contract_version"`
	ObjectKind      string          `json:"object_kind"`
	Ref             Ref             `json:"ref"`
	Owner           OwnerDomain     `json:"owner"`
	TenantID        string          `json:"tenant_id"`
	ViewRef         Ref             `json:"view_ref"`
	Entries         []ExportEntryV1 `json:"entries"`
	CreatedAt       time.Time       `json:"created_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
	Digest          string          `json:"digest"`
}

func SealExportManifestV1(in ExportManifestV1) (ExportManifestV1, error) {
	in.ContractVersion, in.ObjectKind = FrameworkContractVersionV1, ExportManifestObjectKindV1
	for i := range in.Entries {
		in.Entries[i].SourceRefs = NormalizeRefs(in.Entries[i].SourceRefs)
		in.Entries[i].EvidenceRefs = NormalizeRefs(in.Entries[i].EvidenceRefs)
	}
	slices.SortFunc(in.Entries, func(a, b ExportEntryV1) int {
		if c := strings.Compare(a.RecordRef.ID, b.RecordRef.ID); c != 0 {
			return c
		}
		if a.RecordRef.Revision < b.RecordRef.Revision {
			return -1
		}
		if a.RecordRef.Revision > b.RecordRef.Revision {
			return 1
		}
		return strings.Compare(a.RecordRef.Digest, b.RecordRef.Digest)
	})
	in.CreatedAt, in.ExpiresAt = in.CreatedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	d, e := Digest(in)
	if e != nil {
		return ExportManifestV1{}, e
	}
	in.Ref.Digest, in.Digest = d, d
	if e = in.Validate(in.CreatedAt); e != nil {
		return ExportManifestV1{}, e
	}
	return in, nil
}
func (in ExportManifestV1) Validate(now time.Time) error {
	if in.ContractVersion != FrameworkContractVersionV1 || in.ObjectKind != ExportManifestObjectKindV1 || in.Ref.Validate() != nil || in.Owner != OwnerMemory && in.Owner != OwnerKnowledge || strings.TrimSpace(in.TenantID) == "" || in.ViewRef.Validate() != nil || in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: export manifest", ErrInvalidArgument)
	}
	seen := map[string]struct{}{}
	for _, entry := range in.Entries {
		if entry.RecordRef.Validate() != nil || strings.TrimSpace(entry.Scope) == "" || strings.TrimSpace(entry.Sensitivity) == "" {
			return ErrInvalidArgument
		}
		if _, ok := seen[entry.RecordRef.ID]; ok {
			return fmt.Errorf("%w: duplicate export record", ErrEvidenceConflict)
		}
		seen[entry.RecordRef.ID] = struct{}{}
		if entry.ContentRef != nil && entry.ContentRef.Validate() != nil {
			return ErrInvalidArgument
		}
		for _, r := range append(slices.Clone(entry.SourceRefs), entry.EvidenceRefs...) {
			if r.Validate() != nil {
				return ErrInvalidArgument
			}
		}
	}
	c := in
	c.Ref.Digest, c.Digest = "", ""
	d, e := Digest(c)
	if e != nil {
		return e
	}
	if d != in.Digest || in.Ref.Digest != d {
		return fmt.Errorf("%w: export digest", ErrEvidenceConflict)
	}
	return nil
}
