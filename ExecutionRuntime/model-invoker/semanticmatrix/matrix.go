// Package semanticmatrix builds the machine-checkable v1 candidate capability
// matrix from the active upstream Catalog. Runtime contract checks live in
// tests so generation never resolves credentials or constructs adapters.
package semanticmatrix

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const Version = "praxis.model-invoker.semantic-matrix/v1candidate"

type Row struct {
	RouteID                     upstream.RouteID
	Provider                    upstream.ProviderID
	Offering                    upstream.OfferingID
	AdapterID                   modelinvoker.ProviderID
	Protocol                    upstream.ProtocolID
	Capability                  modelinvoker.Capability
	Support                     catalog.CapabilitySupport
	Action                      modelinvoker.MappingAction
	RequiresExplicitDegradation bool
	Limitations                 []string
	EvidenceDigest              string
	CodePaths                   []string
	TestEvidence                []string
}

type Matrix struct {
	Version   string
	Protocols []upstream.ProtocolID
	Rows      []Row
}

func Build(routeCatalog *catalog.Catalog) (Matrix, error) {
	if routeCatalog == nil {
		return Matrix{}, fmt.Errorf("semantic matrix: catalog is required")
	}
	var rows []Row
	protocolSet := map[upstream.ProtocolID]struct{}{}
	for _, entry := range routeCatalog.Entries() {
		if !entry.Implementation.Callable {
			continue
		}
		protocolSet[entry.Route.Protocol.ID] = struct{}{}
		capabilities := make(map[string]catalog.CapabilityMetadata, len(entry.Capabilities))
		for _, capability := range entry.Capabilities {
			capabilities[capability.ID] = capability
		}
		for _, capabilityID := range modelinvoker.AllCapabilities() {
			metadata, ok := capabilities[string(capabilityID)]
			if !ok {
				return Matrix{}, fmt.Errorf("semantic matrix: route %q is missing capability %q", entry.ID, capabilityID)
			}
			action, explicit := actionFor(metadata.Support)
			rows = append(rows, Row{
				RouteID: entry.ID, Provider: entry.Route.Provider, Offering: entry.Route.Offering.ID,
				AdapterID: modelinvoker.ProviderID(entry.Implementation.AdapterID), Protocol: entry.Route.Protocol.ID,
				Capability: capabilityID, Support: metadata.Support, Action: action,
				RequiresExplicitDegradation: explicit, Limitations: append([]string(nil), metadata.Limitations...),
				EvidenceDigest: entry.Evidence.Digest, CodePaths: append([]string(nil), entry.Implementation.CodePaths...),
				TestEvidence: append([]string(nil), entry.Implementation.TestEvidence...),
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].RouteID != rows[j].RouteID {
			return rows[i].RouteID < rows[j].RouteID
		}
		return rows[i].Capability < rows[j].Capability
	})
	protocols := make([]upstream.ProtocolID, 0, len(protocolSet))
	for protocol := range protocolSet {
		protocols = append(protocols, protocol)
	}
	sort.Slice(protocols, func(i, j int) bool { return protocols[i] < protocols[j] })
	matrix := Matrix{Version: Version, Protocols: protocols, Rows: rows}
	if err := matrix.Validate(); err != nil {
		return Matrix{}, err
	}
	return matrix, nil
}

func (matrix Matrix) Validate() error {
	if matrix.Version != Version {
		return fmt.Errorf("semantic matrix: unexpected version %q", matrix.Version)
	}
	if len(matrix.Protocols) != 6 {
		return fmt.Errorf("semantic matrix: protocols = %d, want 6", len(matrix.Protocols))
	}
	routes, adapters := map[upstream.RouteID]int{}, map[modelinvoker.ProviderID]struct{}{}
	seen := map[string]struct{}{}
	for _, row := range matrix.Rows {
		if row.RouteID == "" || row.AdapterID == "" || row.Protocol == "" || row.Capability == "" || row.EvidenceDigest == "" || row.Action == "" {
			return fmt.Errorf("semantic matrix: incomplete row for %q/%q", row.RouteID, row.Capability)
		}
		key := string(row.RouteID) + "\x00" + string(row.Capability)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("semantic matrix: duplicate row %q/%q", row.RouteID, row.Capability)
		}
		seen[key] = struct{}{}
		routes[row.RouteID]++
		adapters[row.AdapterID] = struct{}{}
		expectedAction, explicit := actionFor(row.Support)
		if row.Action != expectedAction || row.RequiresExplicitDegradation != explicit {
			return fmt.Errorf("semantic matrix: invalid action for %q/%q", row.RouteID, row.Capability)
		}
	}
	if len(routes) != 39 {
		return fmt.Errorf("semantic matrix: callable routes = %d, want 39", len(routes))
	}
	if len(adapters) != 14 {
		return fmt.Errorf("semantic matrix: adapters = %d, want 14", len(adapters))
	}
	for routeID, count := range routes {
		if count != len(modelinvoker.AllCapabilities()) {
			return fmt.Errorf("semantic matrix: route %q rows = %d, want %d", routeID, count, len(modelinvoker.AllCapabilities()))
		}
	}
	return nil
}

func (matrix Matrix) CSV() ([]byte, error) {
	if err := matrix.Validate(); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"matrix_version", "route_id", "provider", "offering", "adapter_id", "protocol", "capability", "support", "action", "requires_explicit_degradation", "limitations", "evidence_digest", "code_paths", "test_evidence"}); err != nil {
		return nil, err
	}
	for _, row := range matrix.Rows {
		if err := writer.Write([]string{
			matrix.Version, string(row.RouteID), string(row.Provider), string(row.Offering), string(row.AdapterID), string(row.Protocol), string(row.Capability), string(row.Support), string(row.Action),
			fmt.Sprintf("%t", row.RequiresExplicitDegradation), strings.Join(row.Limitations, " | "), row.EvidenceDigest, strings.Join(row.CodePaths, " | "), strings.Join(row.TestEvidence, " | "),
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

func actionFor(support catalog.CapabilitySupport) (modelinvoker.MappingAction, bool) {
	switch support {
	case catalog.CapabilityNative:
		return modelinvoker.MappingExact, false
	case catalog.CapabilityCompatible:
		return modelinvoker.MappingTransformed, false
	case catalog.CapabilityPartial:
		return modelinvoker.MappingDegraded, true
	case catalog.CapabilityUnsupported, catalog.CapabilityUnknown, "":
		return modelinvoker.MappingRejected, false
	default:
		return modelinvoker.MappingRejected, false
	}
}

func RuntimeSupport(contract modelinvoker.CapabilityContract, capability modelinvoker.Capability) catalog.CapabilitySupport {
	support, ok := contract[capability]
	if !ok {
		return catalog.CapabilityUnsupported
	}
	switch support.Level {
	case modelinvoker.SupportNative:
		return catalog.CapabilityNative
	case modelinvoker.SupportCompatible:
		return catalog.CapabilityCompatible
	case modelinvoker.SupportPartial:
		return catalog.CapabilityPartial
	case modelinvoker.SupportUnsupported, "":
		return catalog.CapabilityUnsupported
	default:
		return catalog.CapabilityUnknown
	}
}
