package api

import (
	"context"
	"reflect"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

const (
	CatalogContractVersionV1 = "praxis.tool-mcp.api.catalog/v1"
	MaxRegistryPageSizeV1    = 100
)

type CatalogSDKPortV1 interface {
	InspectRegistrySnapshotV1(context.Context) (registry.Snapshot, error)
	InspectCapabilityV1(context.Context, contract.ObjectRef) (contract.CapabilityDescriptor, registry.Record, error)
	InspectToolV1(context.Context, contract.ObjectRef) (contract.ToolDescriptor, registry.Record, error)
	InspectPackageV1(context.Context, contract.ObjectRef) (contract.ToolPackageManifest, registry.Record, error)
	InspectToolAliasV1(context.Context, contract.ToolAliasRefV1) (contract.ToolAliasV1, registry.Record, error)
}

type RegistryRecordV1 struct {
	Kind             string         `json:"kind"`
	ID               string         `json:"id"`
	ObjectRevision   core.Revision  `json:"object_revision"`
	ObjectDigest     core.Digest    `json:"object_digest"`
	State            registry.State `json:"state"`
	RegistryRevision core.Revision  `json:"registry_revision"`
	UpdatedUnixNano  int64          `json:"updated_unix_nano"`
}

func (r RegistryRecordV1) Validate() error {
	return registry.Record{
		Kind: r.Kind, ID: r.ID, ObjectRevision: r.ObjectRevision, ObjectDigest: r.ObjectDigest,
		State: r.State, RegistryRevision: r.RegistryRevision, UpdatedUnixNano: r.UpdatedUnixNano,
	}.Validate()
}

type RegistryPageCursorV1 struct {
	ContractVersion string                    `json:"contract_version"`
	Snapshot        sdk.RegistrySnapshotRefV1 `json:"snapshot"`
	KindFilter      string                    `json:"kind_filter,omitempty"`
	AfterKind       string                    `json:"after_kind"`
	AfterID         string                    `json:"after_id"`
	Digest          core.Digest               `json:"digest"`
}

func (c RegistryPageCursorV1) Validate() error {
	if c.ContractVersion != CatalogContractVersionV1 || c.Snapshot.Validate() != nil || !validRegistryKindV1(c.KindFilter, true) || !validRegistryKindV1(c.AfterKind, false) || strings.TrimSpace(c.AfterID) == "" || len(c.AfterID) > 256 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Registry page cursor is invalid")
	}
	digest, err := c.ComputeDigest()
	if err != nil || digest != c.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry page cursor digest drifted")
	}
	return nil
}

func (c RegistryPageCursorV1) ComputeDigest() (core.Digest, error) {
	c.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.api", CatalogContractVersionV1, "RegistryPageCursorV1", c)
}

func sealRegistryPageCursorV1(c RegistryPageCursorV1) (RegistryPageCursorV1, error) {
	c.ContractVersion = CatalogContractVersionV1
	c.Digest = ""
	digest, err := c.ComputeDigest()
	if err != nil {
		return RegistryPageCursorV1{}, err
	}
	c.Digest = digest
	return c, c.Validate()
}

type ListRegistryRequestV1 struct {
	PageSize   int                   `json:"page_size"`
	KindFilter string                `json:"kind_filter,omitempty"`
	Cursor     *RegistryPageCursorV1 `json:"cursor,omitempty"`
}

func (r ListRegistryRequestV1) Validate() error {
	if r.PageSize <= 0 || r.PageSize > MaxRegistryPageSizeV1 || !validRegistryKindV1(r.KindFilter, true) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Registry page request is invalid")
	}
	if r.Cursor != nil {
		if err := r.Cursor.Validate(); err != nil {
			return err
		}
		if r.Cursor.KindFilter != r.KindFilter {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry page filter differs from cursor")
		}
	}
	return nil
}

type ListRegistryResultV1 struct {
	ContractVersion string                    `json:"contract_version"`
	Snapshot        sdk.RegistrySnapshotRefV1 `json:"snapshot"`
	Records         []RegistryRecordV1        `json:"records"`
	Next            *RegistryPageCursorV1     `json:"next,omitempty"`
}

type CatalogV1 struct {
	sdk CatalogSDKPortV1
}

func NewCatalogV1(port CatalogSDKPortV1) (*CatalogV1, error) {
	if nilLikeCatalogV1(port) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Catalog SDK Port is required")
	}
	return &CatalogV1{sdk: port}, nil
}

func (c *CatalogV1) ListRegistryV1(ctx context.Context, request ListRegistryRequestV1) (ListRegistryResultV1, error) {
	if c == nil || nilLikeCatalogV1(c.sdk) {
		return ListRegistryResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Catalog API is unavailable")
	}
	if ctx == nil {
		return ListRegistryResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Catalog API context is required")
	}
	if err := ctx.Err(); err != nil {
		return ListRegistryResultV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ListRegistryResultV1{}, err
	}
	snapshot, err := c.sdk.InspectRegistrySnapshotV1(ctx)
	if err != nil {
		return ListRegistryResultV1{}, err
	}
	snapshotRef := sdk.RegistrySnapshotRefV1{Revision: snapshot.Revision, Digest: snapshot.Digest}
	if request.Cursor != nil && request.Cursor.Snapshot != snapshotRef {
		return ListRegistryResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry changed between pages")
	}
	records := append([]registry.Record(nil), snapshot.Records...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].Kind != records[j].Kind {
			return records[i].Kind < records[j].Kind
		}
		return records[i].ID < records[j].ID
	})
	page := make([]RegistryRecordV1, 0, request.PageSize)
	more := false
	for _, record := range records {
		if request.KindFilter != "" && record.Kind != request.KindFilter || request.Cursor != nil && !afterRegistryCursorV1(record, *request.Cursor) {
			continue
		}
		if len(page) == request.PageSize {
			more = true
			break
		}
		projected := RegistryRecordV1{
			Kind: record.Kind, ID: record.ID, ObjectRevision: record.ObjectRevision, ObjectDigest: record.ObjectDigest,
			State: record.State, RegistryRevision: record.RegistryRevision, UpdatedUnixNano: record.UpdatedUnixNano,
		}
		if err := projected.Validate(); err != nil {
			return ListRegistryResultV1{}, err
		}
		page = append(page, projected)
	}
	final, err := c.sdk.InspectRegistrySnapshotV1(ctx)
	if err != nil {
		return ListRegistryResultV1{}, err
	}
	if final.Revision != snapshotRef.Revision || final.Digest != snapshotRef.Digest {
		return ListRegistryResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry changed while a page was projected")
	}
	result := ListRegistryResultV1{ContractVersion: CatalogContractVersionV1, Snapshot: snapshotRef, Records: page}
	if more && len(page) != 0 {
		last := page[len(page)-1]
		next, err := sealRegistryPageCursorV1(RegistryPageCursorV1{Snapshot: snapshotRef, KindFilter: request.KindFilter, AfterKind: last.Kind, AfterID: last.ID})
		if err != nil {
			return ListRegistryResultV1{}, err
		}
		result.Next = &next
	}
	return result, nil
}

func afterRegistryCursorV1(record registry.Record, cursor RegistryPageCursorV1) bool {
	return record.Kind > cursor.AfterKind || record.Kind == cursor.AfterKind && record.ID > cursor.AfterID
}

func validRegistryKindV1(value string, optional bool) bool {
	if value == "" {
		return optional
	}
	return value == "capability" || value == "tool" || value == "package" || value == "tool-alias"
}

func nilLikeCatalogV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
