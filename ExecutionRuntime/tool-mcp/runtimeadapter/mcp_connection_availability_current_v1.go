package runtimeadapter

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// MCPConnectionAvailabilityCurrentSourceV1 is the Tool-owned read boundary
// used by Runtime. It exposes neither Connect nor settlement write authority.
type MCPConnectionAvailabilityCurrentSourceV1 interface {
	toolcontract.MCPConnectionFactCurrentReaderV2
	toolcontract.MCPConnectionAvailabilityCurrentReaderV1
}

// MCPConnectionAvailabilityCurrentAdapterV1 losslessly projects the settled
// Tool-owned Connection availability into Runtime-neutral current evidence.
type MCPConnectionAvailabilityCurrentAdapterV1 struct {
	source MCPConnectionAvailabilityCurrentSourceV1
	clock  func() time.Time
	ttl    time.Duration
}

func NewMCPConnectionAvailabilityCurrentAdapterV1(source MCPConnectionAvailabilityCurrentSourceV1, clock func() time.Time, ttl time.Duration) (*MCPConnectionAvailabilityCurrentAdapterV1, error) {
	if nilLikeMCPConnectReceiptDependencyV1(source) || clock == nil || ttl <= 0 || ttl > toolcontract.MaxMCPConnectionAvailabilityTTLV1 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Connection availability current adapter dependencies are incomplete")
	}
	return &MCPConnectionAvailabilityCurrentAdapterV1{source: source, clock: clock, ttl: ttl}, nil
}

func MCPConnectionAvailabilityRuntimeRefV1(value toolcontract.MCPConnectionAvailabilityCurrentProjectionV1) runtimeports.MCPConnectionAvailabilityNeutralRefV1 {
	return runtimeports.MCPConnectionAvailabilityNeutralRefV1{
		Owner:                  value.Owner,
		ConnectionID:           value.Connection.ID,
		ConnectionRevision:     value.Connection.Revision,
		ConnectionDigest:       value.Connection.Digest,
		ApplyID:                value.ApplySettlement.ID,
		ApplyRevision:          value.ApplySettlement.Revision,
		ApplyDigest:            value.ApplySettlement.Digest,
		DomainResultID:         value.DomainResult.ID,
		DomainResultRevision:   value.DomainResult.Revision,
		DomainResultDigest:     value.DomainResult.Digest,
		SourceProjectionDigest: value.Digest,
	}
}

func (a *MCPConnectionAvailabilityCurrentAdapterV1) InspectCurrentMCPConnectionAvailabilityNeutralV1(ctx context.Context, exact runtimeports.MCPConnectionAvailabilityNeutralRefV1) (runtimeports.MCPConnectionAvailabilityNeutralProjectionV1, error) {
	if ctx == nil {
		return runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Connection availability current context is nil")
	}
	if err := ctx.Err(); err != nil {
		return runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{}, err
	}
	if a == nil || nilLikeMCPConnectReceiptDependencyV1(a.source) || a.clock == nil {
		return runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Connection availability current adapter is unavailable")
	}
	if err := exact.Validate(); err != nil {
		return runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{}, err
	}
	nowS1 := a.clock()
	if nowS1.IsZero() {
		return runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Connection availability current clock is zero")
	}
	connectionRef := toolcontract.MCPConnectionFactRefV2{ID: exact.ConnectionID, Revision: exact.ConnectionRevision, Digest: exact.ConnectionDigest}
	availability, err := a.source.InspectCurrentMCPConnectionAvailabilityV1(ctx, connectionRef, a.ttl)
	if err != nil {
		return runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{}, err
	}
	connection, err := a.source.InspectCurrentMCPConnectionFactV2(ctx, connectionRef)
	if err != nil {
		return runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{}, err
	}
	nowS2 := a.clock()
	if nowS2.IsZero() || nowS2.Before(nowS1) {
		return runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Connection availability current clock regressed")
	}
	if err := availability.Validate(nowS2); err != nil {
		return runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{}, err
	}
	if err := connection.Validate(); err != nil {
		return runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{}, err
	}
	if MCPConnectionAvailabilityRuntimeRefV1(availability) != exact || availability.Connection != connection.Ref || availability.Owner != connection.Owner || nowS2.Before(time.Unix(0, connection.CreatedUnixNano)) || !nowS2.Before(time.Unix(0, connection.ExpiresUnixNano)) {
		return runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connection availability source closure drifted")
	}
	expires := availability.ExpiresUnixNano
	if connection.ExpiresUnixNano < expires {
		expires = connection.ExpiresUnixNano
	}
	return runtimeports.SealMCPConnectionAvailabilityNeutralProjectionV1(runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{
		Ref:               exact,
		TenantID:          core.TenantID(connection.Coordinate.TenantID),
		RunID:             connection.Coordinate.RunID,
		SessionID:         connection.Coordinate.Session.ID,
		SessionRevision:   connection.Coordinate.Session.Revision,
		SessionDigest:     connection.Coordinate.Session.Digest,
		ConnectionEpoch:   connection.Coordinate.Epoch,
		ProviderTransport: connection.ProviderTransport,
		Provider:          connection.Provider,
		CheckedUnixNano:   nowS2.UnixNano(),
		ExpiresUnixNano:   expires,
	})
}

var _ runtimeports.MCPConnectionAvailabilityNeutralCurrentReaderV1 = (*MCPConnectionAvailabilityCurrentAdapterV1)(nil)
