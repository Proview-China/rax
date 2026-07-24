package contract

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MCPProcessObservationContractVersionV1 = "praxis.tool-mcp.mcp-process-observation/v1"

const MaxMCPProcessObservationPageSizeV1 uint32 = 256

type MCPProcessObservationExactReaderV1 interface {
	InspectMCPProcessObservationV1(context.Context, MCPProcessObservationRefV1) (MCPProcessObservationV1, error)
}

type MCPProcessObservationPageReaderV1 interface {
	ReadMCPProcessObservationPageV1(context.Context, MCPProcessObservationPageRequestV1) (MCPProcessObservationPageV1, error)
}

type MCPProcessObservationReadPortV1 interface {
	MCPProcessObservationExactReaderV1
	MCPProcessObservationPageReaderV1
}

type MCPProcessObservationKindV1 string

const (
	MCPProcessProgressV1 MCPProcessObservationKindV1 = "progress"
	MCPProcessLoggingV1  MCPProcessObservationKindV1 = "logging"
)

type MCPProcessObservationRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r MCPProcessObservationRefV1) Validate() error {
	if ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return invalid("MCP process Observation Ref is invalid")
	}
	return nil
}

type MCPProcessObservationInputV1 struct {
	Kind              MCPProcessObservationKindV1 `json:"kind"`
	CorrelationDigest core.Digest                 `json:"correlation_digest"`
	PayloadDigest     core.Digest                 `json:"payload_digest"`
	LoggingLevel      string                      `json:"logging_level,omitempty"`
	Logger            string                      `json:"logger,omitempty"`
	Progress          float64                     `json:"progress,omitempty"`
	Total             float64                     `json:"total,omitempty"`
}

func (i MCPProcessObservationInputV1) Validate() error {
	if i.CorrelationDigest.Validate() != nil || i.PayloadDigest.Validate() != nil || math.IsNaN(i.Progress) || math.IsInf(i.Progress, 0) || math.IsNaN(i.Total) || math.IsInf(i.Total, 0) || i.Progress < 0 || i.Total < 0 {
		return invalid("MCP process Observation input is invalid")
	}
	switch i.Kind {
	case MCPProcessProgressV1:
		if i.LoggingLevel != "" || i.Logger != "" || i.Total > 0 && i.Progress > i.Total {
			return invalid("MCP progress Observation fields are invalid")
		}
	case MCPProcessLoggingV1:
		if !validMCPLoggingLevelV1(i.LoggingLevel) || len(i.Logger) > 256 || !strings.EqualFold(strings.TrimSpace(i.Logger), i.Logger) || i.Progress != 0 || i.Total != 0 {
			return invalid("MCP logging Observation fields are invalid")
		}
	default:
		return invalid("MCP process Observation kind is invalid")
	}
	return nil
}

// MCPProcessObservationV1 is bounded process telemetry. It is never a Tool
// result, Runtime Evidence, Timeline fact, Review verdict, or execution right.
type MCPProcessObservationV1 struct {
	ContractVersion   string                      `json:"contract_version"`
	Ref               MCPProcessObservationRefV1  `json:"ref"`
	Connection        MCPConnectionRef            `json:"connection"`
	Snapshot          ObjectRef                   `json:"snapshot"`
	Kind              MCPProcessObservationKindV1 `json:"kind"`
	SourceSequence    uint64                      `json:"source_sequence"`
	CorrelationDigest core.Digest                 `json:"correlation_digest"`
	PayloadDigest     core.Digest                 `json:"payload_digest"`
	LoggingLevel      string                      `json:"logging_level,omitempty"`
	Logger            string                      `json:"logger,omitempty"`
	Progress          float64                     `json:"progress,omitempty"`
	Total             float64                     `json:"total,omitempty"`
	ObservedUnixNano  int64                       `json:"observed_unix_nano"`
}

// MCPProcessObservationPageRequestV1 addresses one exact process stream. The
// compact Connection coordinates remain exact because ObjectRef.Digest binds
// the full MCPConnectionRef; callers do not have to replay tenant/session
// fields merely to read bounded telemetry.
type MCPProcessObservationPageRequestV1 struct {
	Connection          ObjectRef  `json:"connection"`
	ConnectionEpoch     core.Epoch `json:"connection_epoch"`
	Snapshot            ObjectRef  `json:"snapshot"`
	AfterSourceSequence uint64     `json:"after_source_sequence"`
	Limit               uint32     `json:"limit"`
}

func (r MCPProcessObservationPageRequestV1) Validate() error {
	if r.Connection.Validate() != nil || r.ConnectionEpoch == 0 || r.Snapshot.Validate() != nil || r.Limit == 0 || r.Limit > MaxMCPProcessObservationPageSizeV1 {
		return invalid("MCP process Observation page request is invalid")
	}
	return nil
}

// MCPProcessObservationPageV1 is a bounded read projection, not a durable
// Event, Evidence bundle, Timeline, ToolResult, or execution authority. An
// empty page never proves that a Provider was not called.
type MCPProcessObservationPageV1 struct {
	ContractVersion          string                             `json:"contract_version"`
	Request                  MCPProcessObservationPageRequestV1 `json:"request"`
	Observations             []MCPProcessObservationV1          `json:"observations"`
	NextAfterSourceSequence  uint64                             `json:"next_after_source_sequence"`
	UpperBoundSourceSequence uint64                             `json:"upper_bound_source_sequence"`
	HasMore                  bool                               `json:"has_more"`
	PageDigest               core.Digest                        `json:"page_digest"`
}

func (p MCPProcessObservationPageV1) Validate() error {
	if p.ContractVersion != MCPProcessObservationContractVersionV1 || p.Request.Validate() != nil || len(p.Observations) > int(p.Request.Limit) || p.UpperBoundSourceSequence < p.Request.AfterSourceSequence {
		return invalid("MCP process Observation page is invalid")
	}
	last := p.Request.AfterSourceSequence
	for _, observation := range p.Observations {
		connection := ObjectRef{ID: observation.Connection.ID, Revision: observation.Connection.Revision, Digest: observation.Connection.Digest}
		if observation.Validate() != nil || connection != p.Request.Connection || observation.Connection.Epoch != p.Request.ConnectionEpoch || observation.Snapshot != p.Request.Snapshot || observation.SourceSequence <= last || observation.SourceSequence > p.UpperBoundSourceSequence {
			return conflict("MCP process Observation page content drifted")
		}
		last = observation.SourceSequence
	}
	if p.NextAfterSourceSequence != last {
		return conflict("MCP process Observation page cursor drifted")
	}
	if p.HasMore {
		if len(p.Observations) == 0 || p.NextAfterSourceSequence >= p.UpperBoundSourceSequence {
			return conflict("MCP process Observation page continuation drifted")
		}
	} else if p.NextAfterSourceSequence != p.UpperBoundSourceSequence {
		return conflict("MCP process Observation page upper bound drifted")
	}
	digest, err := p.ComputeDigestV1()
	if err != nil || p.PageDigest.Validate() != nil || p.PageDigest != digest {
		return conflict("MCP process Observation page digest drifted")
	}
	return nil
}

func (p MCPProcessObservationPageV1) ComputeDigestV1() (core.Digest, error) {
	p.PageDigest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp-process-observation", MCPProcessObservationContractVersionV1, "MCPProcessObservationPageV1", p)
}

func SealMCPProcessObservationPageV1(p MCPProcessObservationPageV1) (MCPProcessObservationPageV1, error) {
	p.ContractVersion = MCPProcessObservationContractVersionV1
	p.Observations = append([]MCPProcessObservationV1(nil), p.Observations...)
	provided := p.PageDigest
	p.PageDigest = ""
	digest, err := p.ComputeDigestV1()
	if err != nil {
		return MCPProcessObservationPageV1{}, err
	}
	if provided != "" && provided != digest {
		return MCPProcessObservationPageV1{}, conflict("supplied MCP process Observation page digest drifted")
	}
	p.PageDigest = digest
	return p, p.Validate()
}

func CloneMCPProcessObservationPageV1(p MCPProcessObservationPageV1) MCPProcessObservationPageV1 {
	p.Observations = append([]MCPProcessObservationV1(nil), p.Observations...)
	return p
}

func (o MCPProcessObservationV1) Validate() error {
	input := MCPProcessObservationInputV1{Kind: o.Kind, CorrelationDigest: o.CorrelationDigest, PayloadDigest: o.PayloadDigest, LoggingLevel: o.LoggingLevel, Logger: o.Logger, Progress: o.Progress, Total: o.Total}
	if o.ContractVersion != MCPProcessObservationContractVersionV1 || o.Ref.Validate() != nil || o.Connection.Validate() != nil || o.Snapshot.Validate() != nil || input.Validate() != nil || o.SourceSequence == 0 || o.ObservedUnixNano < o.Connection.CreatedUnixNano || o.ObservedUnixNano >= o.Connection.ExpiresUnixNano {
		return invalid("MCP process Observation is incomplete")
	}
	id, err := DeriveMCPProcessObservationIDV1(o.Connection, o.Snapshot, o.Kind, o.SourceSequence)
	if err != nil || id != o.Ref.ID {
		return conflict("MCP process Observation ID drifted")
	}
	digest, err := o.ComputeDigestV1()
	if err != nil || digest != o.Ref.Digest {
		return conflict("MCP process Observation digest drifted")
	}
	return nil
}

func (o MCPProcessObservationV1) ValidateCurrent(now time.Time) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < o.ObservedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP process Observation clock regressed")
	}
	if !now.Before(time.Unix(0, o.Connection.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP process Observation Connection expired")
	}
	return nil
}

func (o MCPProcessObservationV1) ComputeDigestV1() (core.Digest, error) {
	o.Ref.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp-process-observation", MCPProcessObservationContractVersionV1, "MCPProcessObservationV1", o)
}

func SealMCPProcessObservationV1(o MCPProcessObservationV1) (MCPProcessObservationV1, error) {
	o.ContractVersion = MCPProcessObservationContractVersionV1
	id, err := DeriveMCPProcessObservationIDV1(o.Connection, o.Snapshot, o.Kind, o.SourceSequence)
	if err != nil {
		return MCPProcessObservationV1{}, err
	}
	if o.Ref.ID != "" && o.Ref.ID != id {
		return MCPProcessObservationV1{}, conflict("supplied MCP process Observation ID drifted")
	}
	o.Ref.ID, o.Ref.Revision = id, 1
	provided := o.Ref.Digest
	o.Ref.Digest = ""
	digest, err := o.ComputeDigestV1()
	if err != nil {
		return MCPProcessObservationV1{}, err
	}
	if provided != "" && provided != digest {
		return MCPProcessObservationV1{}, conflict("supplied MCP process Observation digest drifted")
	}
	o.Ref.Digest = digest
	return o, o.Validate()
}

func DeriveMCPProcessObservationIDV1(connection MCPConnectionRef, snapshot ObjectRef, kind MCPProcessObservationKindV1, sequence uint64) (string, error) {
	if connection.Validate() != nil || snapshot.Validate() != nil || sequence == 0 || kind != MCPProcessProgressV1 && kind != MCPProcessLoggingV1 {
		return "", invalid("MCP process Observation identity inputs are invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp-process-observation", MCPProcessObservationContractVersionV1, "MCPProcessObservationIdentityV1", struct {
		Connection ObjectRef                   `json:"connection"`
		Epoch      core.Epoch                  `json:"connection_epoch"`
		Snapshot   ObjectRef                   `json:"snapshot"`
		Kind       MCPProcessObservationKindV1 `json:"kind"`
		Sequence   uint64                      `json:"source_sequence"`
	}{ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest}, connection.Epoch, snapshot, kind, sequence})
	if err != nil {
		return "", err
	}
	return StableID("mcp-process-observation", string(digest))
}

func validMCPLoggingLevelV1(level string) bool {
	switch level {
	case "debug", "info", "notice", "warning", "error", "critical", "alert", "emergency":
		return true
	default:
		return false
	}
}
