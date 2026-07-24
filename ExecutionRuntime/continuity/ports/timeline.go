package ports

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// TimelineOwnerCurrentProjectionV1 is the neutral current projection returned
// by a domain Owner. It proves only the exact Owner fact supplied by the
// request; it neither creates Timeline/Evidence facts nor grants authority.
type TimelineOwnerCurrentProjectionV1 struct {
	Fact            contract.TimelineOwnerFactRefV1 `json:"fact"`
	CheckedUnixNano int64                           `json:"checked_unix_nano"`
	ExpiresUnixNano int64                           `json:"expires_unix_nano"`
	Digest          string                          `json:"digest"`
}

// TimelineTypedOwnerCurrentReaderV1 is retained for source compatibility.
// Its request lacks TenantID and therefore must not be used by production
// owner adapters that require tenant-scoped exact Inspect.
type TimelineTypedOwnerCurrentReaderV1 interface {
	InspectTimelineOwnerCurrentV1(context.Context, contract.TimelineOwnerFactRefV1) (TimelineOwnerCurrentProjectionV1, error)
	ValidateTimelineOwnerCurrentV1(context.Context, TimelineOwnerCurrentProjectionV1) error
}

type TimelineTypedOwnerRouterV1 interface {
	ReaderForTimelineOwnerV1(contract.TimelineOwnerFactRefV1) (TimelineTypedOwnerCurrentReaderV1, error)
}

// TimelineOwnerCurrentInspectRequestV2 is the complete nominal coordinate for
// a tenant-scoped exact Owner read. ScopeDigest remains an opaque execution
// scope coordinate and must never be parsed to recover TenantID.
type TimelineOwnerCurrentInspectRequestV2 struct {
	TenantID runtimecore.TenantID            `json:"tenant_id"`
	Fact     contract.TimelineOwnerFactRefV1 `json:"fact"`
}

func (r TimelineOwnerCurrentInspectRequestV2) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" {
		return contract.NewError(contract.ErrInvalidArgument, "tenant_id", "tenant is required")
	}
	if err := r.Fact.Validate(); err != nil {
		return err
	}
	if r.Fact.Owner.FactKind != r.Fact.FactKind {
		return contract.NewError(contract.ErrProjectionConflict, "owner_fact_kind", "Owner binding and exact fact kind differ")
	}
	return nil
}

// TimelineTypedOwnerCurrentReaderV2 must Inspect the exact tenant-scoped fact
// and return a deep-cloned projection. Validate performs a fresh exact re-read;
// it must not publish, mutate, allocate a sequence, or create Evidence.
type TimelineTypedOwnerCurrentReaderV2 interface {
	InspectTimelineOwnerCurrentV2(context.Context, TimelineOwnerCurrentInspectRequestV2) (TimelineOwnerCurrentProjectionV1, error)
	ValidateTimelineOwnerCurrentV2(context.Context, TimelineOwnerCurrentInspectRequestV2, TimelineOwnerCurrentProjectionV1) error
}

type TimelineTypedOwnerRouterV2 interface {
	ReaderForTimelineOwnerV2(TimelineOwnerCurrentInspectRequestV2) (TimelineTypedOwnerCurrentReaderV2, error)
}

// TimelineProjectionStore stores Continuity projections only. It never appends
// to or allocates sequence numbers for the Runtime Evidence Ledger.
type TimelineProjectionStore interface {
	PutProjection(context.Context, contract.TimelineEventRecord) (contract.TimelineEventRecord, bool, error)
	InspectByEvidence(context.Context, string) (contract.TimelineEventRecord, error)
	ListLedgerScope(context.Context, string) ([]contract.TimelineEventRecord, error)
	CreateTombstoneOverlay(context.Context, contract.TimelineProjectionTombstoneFactV1) (contract.TimelineProjectionTombstoneFactV1, bool, error)
	InspectTombstone(context.Context, string) (contract.TimelineProjectionTombstoneFactV1, error)
}
