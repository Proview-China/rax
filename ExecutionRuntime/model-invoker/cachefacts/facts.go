// Package cachefacts exports the machine-checkable Provider cache transport
// facts owned by model-invoker. It deliberately describes transport only; it
// does not select keys, TTLs, routes, storage, or cache policy.
package cachefacts

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const Version = "praxis.model-invoker.provider-cache-facts/v1candidate"

type RequestControl string

const (
	RequestControlNotExposed           RequestControl = "not_exposed"
	RequestControlStrictProviderOption RequestControl = "strict_provider_option"
)

type KeyOwnership string

const (
	KeyOwnershipNotExposed              KeyOwnership = "not_exposed"
	KeyOwnershipCallerProviderNamespace KeyOwnership = "caller_provider_namespace"
)

type UsageTransport string

const (
	UsageNotNormalized        UsageTransport = "not_normalized"
	UsageNormalizedIfReported UsageTransport = "normalized_if_reported"
)

type Row struct {
	RouteID            upstream.RouteID
	Provider           upstream.ProviderID
	Offering           upstream.OfferingID
	AdapterID          modelinvoker.ProviderID
	Protocol           upstream.ProtocolID
	DeclaredSupport    catalog.CapabilitySupport
	RequestControl     RequestControl
	KeyOwnership       KeyOwnership
	TTLControl         string
	StateRelation      string
	CacheReadUsage     UsageTransport
	CacheWriteUsage    UsageTransport
	UsageTotalRule     string
	ErrorContract      string
	EvidenceStatus     catalog.EvidenceStatus
	EvidenceTTL        catalog.EvidenceTTLClass
	EvidenceValidUntil time.Time
	EvidenceDigest     string
	TransportCodePath  string
	Limitations        []string
}

type Matrix struct {
	Version string
	Rows    []Row
}

func Build(routeCatalog *catalog.Catalog) (Matrix, error) {
	if routeCatalog == nil {
		return Matrix{}, fmt.Errorf("cache facts: catalog is required")
	}
	rows := make([]Row, 0, 39)
	for _, entry := range routeCatalog.Entries() {
		if !entry.Implementation.Callable {
			continue
		}
		read, write, codePath := protocolUsage(entry.Route.Protocol.ID)
		requestControl := RequestControlNotExposed
		keyOwnership := KeyOwnershipNotExposed
		limitations := []string{
			"reported counts do not prove a cache hit, price, or remaining entitlement",
			"no key generation, TTL, storage, routing, eviction, or cross-Route reuse",
		}
		if entry.Implementation.AdapterID == "xai" && entry.Route.Protocol.ID == upstream.ProtocolResponses {
			requestControl = RequestControlStrictProviderOption
			keyOwnership = KeyOwnershipCallerProviderNamespace
			limitations = append(limitations, "prompt_cache_key is best-effort Provider transport and has no Praxis TTL contract")
		}
		rows = append(rows, Row{
			RouteID: entry.ID, Provider: entry.Route.Provider, Offering: entry.Route.Offering.ID,
			AdapterID: modelinvoker.ProviderID(entry.Implementation.AdapterID), Protocol: entry.Route.Protocol.ID,
			DeclaredSupport: promptCachingSupport(entry), RequestControl: requestControl, KeyOwnership: keyOwnership,
			TTLControl: "not_exposed", StateRelation: "separate_binding_scoped_continuation",
			CacheReadUsage: read, CacheWriteUsage: write, UsageTotalRule: "provider_total_preserved_no_readdition",
			ErrorContract:  "route_provider_error_no_cache_specific_class",
			EvidenceStatus: entry.Evidence.Status, EvidenceTTL: entry.Evidence.TTLClass,
			EvidenceValidUntil: entry.Evidence.ValidUntil, EvidenceDigest: entry.Evidence.Digest,
			TransportCodePath: codePath, Limitations: limitations,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].RouteID < rows[j].RouteID })
	matrix := Matrix{Version: Version, Rows: rows}
	if err := matrix.Validate(); err != nil {
		return Matrix{}, err
	}
	return matrix, nil
}

func (matrix Matrix) Validate() error {
	if matrix.Version != Version {
		return fmt.Errorf("cache facts: unexpected version %q", matrix.Version)
	}
	seen := map[upstream.RouteID]struct{}{}
	adapters := map[modelinvoker.ProviderID]struct{}{}
	protocols := map[upstream.ProtocolID]struct{}{}
	strictControls := 0
	for _, row := range matrix.Rows {
		if row.RouteID == "" || row.AdapterID == "" || row.Protocol == "" || row.DeclaredSupport == "" ||
			row.RequestControl == "" || row.KeyOwnership == "" || row.TTLControl == "" || row.StateRelation == "" ||
			row.CacheReadUsage == "" || row.CacheWriteUsage == "" || row.UsageTotalRule == "" || row.ErrorContract == "" ||
			row.EvidenceStatus == "" || row.EvidenceTTL == "" || row.EvidenceValidUntil.IsZero() || row.EvidenceDigest == "" ||
			row.TransportCodePath == "" || len(row.Limitations) == 0 {
			return fmt.Errorf("cache facts: incomplete row for %q", row.RouteID)
		}
		if _, exists := seen[row.RouteID]; exists {
			return fmt.Errorf("cache facts: duplicate route %q", row.RouteID)
		}
		seen[row.RouteID] = struct{}{}
		adapters[row.AdapterID] = struct{}{}
		protocols[row.Protocol] = struct{}{}
		if row.RequestControl == RequestControlStrictProviderOption {
			strictControls++
			if row.AdapterID != "xai" || row.Protocol != upstream.ProtocolResponses || row.KeyOwnership != KeyOwnershipCallerProviderNamespace {
				return fmt.Errorf("cache facts: unauthorized request control for %q", row.RouteID)
			}
		} else if row.KeyOwnership != KeyOwnershipNotExposed {
			return fmt.Errorf("cache facts: key ownership exposed without request control for %q", row.RouteID)
		}
		if row.TTLControl != "not_exposed" || row.StateRelation != "separate_binding_scoped_continuation" {
			return fmt.Errorf("cache facts: policy or State conflation for %q", row.RouteID)
		}
	}
	if len(matrix.Rows) != 39 || len(adapters) != 14 || len(protocols) != 6 {
		return fmt.Errorf("cache facts: routes/adapters/protocols = %d/%d/%d, want 39/14/6", len(matrix.Rows), len(adapters), len(protocols))
	}
	if strictControls != 1 {
		return fmt.Errorf("cache facts: strict request controls = %d, want 1", strictControls)
	}
	return nil
}

func (matrix Matrix) CSV() ([]byte, error) {
	if err := matrix.Validate(); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{
		"facts_version", "route_id", "provider", "offering", "adapter_id", "protocol", "declared_prompt_caching_support",
		"request_control", "key_ownership", "ttl_control", "state_relation", "cache_read_usage", "cache_write_usage",
		"usage_total_rule", "error_contract", "evidence_status", "evidence_ttl", "evidence_valid_until", "evidence_digest",
		"transport_code_path", "limitations",
	}); err != nil {
		return nil, err
	}
	for _, row := range matrix.Rows {
		if err := writer.Write([]string{
			matrix.Version, string(row.RouteID), string(row.Provider), string(row.Offering), string(row.AdapterID), string(row.Protocol), string(row.DeclaredSupport),
			string(row.RequestControl), string(row.KeyOwnership), row.TTLControl, row.StateRelation, string(row.CacheReadUsage), string(row.CacheWriteUsage),
			row.UsageTotalRule, row.ErrorContract, string(row.EvidenceStatus), string(row.EvidenceTTL), row.EvidenceValidUntil.UTC().Format(time.RFC3339), row.EvidenceDigest,
			row.TransportCodePath, strings.Join(row.Limitations, " | "),
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func protocolUsage(protocolID upstream.ProtocolID) (UsageTransport, UsageTransport, string) {
	switch protocolID {
	case upstream.ProtocolResponses:
		return UsageNormalizedIfReported, UsageNotNormalized, "internal/protocol/openairesponses/normalize.go"
	case upstream.ProtocolChatCompletions:
		return UsageNormalizedIfReported, UsageNotNormalized, "internal/protocol/openaichat/normalize.go"
	case upstream.ProtocolMessages:
		return UsageNormalizedIfReported, UsageNormalizedIfReported, "internal/protocol/anthropicmessages/normalize.go"
	case upstream.ProtocolGenerateContent:
		return UsageNormalizedIfReported, UsageNotNormalized, "internal/protocol/geminigenerate/normalize.go"
	case upstream.ProtocolBedrockConverse:
		return UsageNormalizedIfReported, UsageNormalizedIfReported, "internal/protocol/bedrock/mapping.go"
	case upstream.ProtocolBedrockInvoke:
		return UsageNotNormalized, UsageNotNormalized, "internal/protocol/bedrock/mapping.go"
	default:
		return UsageNotNormalized, UsageNotNormalized, "unsupported_protocol"
	}
}

func promptCachingSupport(entry catalog.Entry) catalog.CapabilitySupport {
	for _, capability := range entry.Capabilities {
		if capability.ID == string(modelinvoker.CapabilityPromptCaching) {
			return capability.Support
		}
	}
	return catalog.CapabilityUnknown
}
