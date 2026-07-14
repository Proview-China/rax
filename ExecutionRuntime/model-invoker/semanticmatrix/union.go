package semanticmatrix

import (
	"fmt"
	"sort"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	operationspecs "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/specs"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/localcompat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/relaycompat"
	realtimespecs "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime/specs"
)

const UnionVersion = "praxis.model-invoker.union-surface-matrix/v1candidate"

type PrimitivePlane string

const (
	PlaneExecution PrimitivePlane = "execution"
	PlaneLLMLocal  PrimitivePlane = "llm.local"
	PlaneLLMRelay  PrimitivePlane = "llm.relay"
	PlaneOperation PrimitivePlane = "operation"
	PlaneRealtime  PrimitivePlane = "realtime"
)

type SurfaceRow struct {
	SurfaceID          string
	Plane              PrimitivePlane
	Provider           modelinvoker.ProviderID
	Primitive          string
	Protocol           modelinvoker.Protocol
	Lifecycle          operation.Lifecycle
	Support            string
	MechanismID        string
	Origin             string
	Fidelity           string
	InvocationBoundary string
	Implementation     string
}

// UnionMatrix proves two different things without flattening their request
// types: LLM routes remain in the audited Route matrix, while Harness,
// peripheral, realtime, relay, and local surfaces are projected into the
// public primitive boundary that owns their lifecycle.
type UnionMatrix struct {
	Version  string
	LLM      Matrix
	Surfaces []SurfaceRow
}

func BuildUnion(routeCatalog *catalog.Catalog, now time.Time) (UnionMatrix, error) {
	llm, err := Build(routeCatalog)
	if err != nil {
		return UnionMatrix{}, err
	}
	if now.IsZero() {
		return UnionMatrix{}, fmt.Errorf("union surface matrix: validation time is required")
	}

	rows, err := canonicalSurfaceRows(now)
	if err != nil {
		return UnionMatrix{}, err
	}
	matrix := UnionMatrix{Version: UnionVersion, LLM: llm, Surfaces: rows}
	if err := matrix.Validate(now); err != nil {
		return UnionMatrix{}, err
	}
	return matrix, nil
}

func canonicalSurfaceRows(now time.Time) ([]SurfaceRow, error) {
	rows := make([]SurfaceRow, 0, 256)
	profiles, err := profile.RepresentativeProfiles(now)
	if err != nil {
		return nil, err
	}
	for _, candidate := range profiles {
		for _, mechanism := range candidate.HarnessCapability.AvailableMechanisms {
			for _, intent := range mechanism.IntentKinds {
				rows = append(rows, SurfaceRow{
					SurfaceID: string(candidate.ID), Plane: PlaneExecution,
					Provider: modelinvoker.ProviderID(candidate.Selection.Provider), Primitive: string(intent),
					Support: "available", MechanismID: mechanism.ID, Origin: string(mechanism.Origin),
					Fidelity: string(mechanism.Fidelity), InvocationBoundary: "execution.Runtime",
					Implementation: "profile+union+execution",
				})
			}
		}
	}
	models := make(map[operation.Kind][]string, len(operation.AllKinds()))
	for _, kind := range operation.AllKinds() {
		models[kind] = []string{"matrix-model"}
	}
	for _, definition := range operationspecs.Definitions() {
		for _, spec := range definition.Specs(models) {
			rows = append(rows, SurfaceRow{
				SurfaceID: definition.ID, Plane: PlaneOperation, Provider: definition.Provider,
				Primitive: string(spec.Kind), Lifecycle: spec.Lifecycle, Support: string(spec.Support),
				Origin: string(definition.Kind), InvocationBoundary: "operation.Invoker",
				Implementation: "operation/nativehttp",
			})
		}
	}
	rows = append(rows, SurfaceRow{
		SurfaceID: "google.gemini-resumable-upload", Plane: PlaneOperation, Provider: "gemini",
		Primitive: string(operation.FileCreate), Lifecycle: operation.LifecycleResource,
		Support: string(operation.SupportNative), Origin: "official",
		InvocationBoundary: "operation.Invoker", Implementation: "operation/geminiupload",
	})
	for _, definition := range realtimespecs.Definitions() {
		rows = append(rows, SurfaceRow{
			SurfaceID: definition.ID, Plane: PlaneRealtime, Provider: definition.Provider,
			Primitive: "session.open", Lifecycle: operation.LifecycleRealtime,
			Support: "native", Origin: "official", InvocationBoundary: "realtime.Invoker",
			Implementation: "realtime/nativews",
		})
	}
	rows = append(rows, SurfaceRow{
		SurfaceID: "self-hosted.realtime", Plane: PlaneRealtime, Provider: "local-realtime",
		Primitive: "session.open", Lifecycle: operation.LifecycleRealtime,
		Support: "compatible", Origin: "local", InvocationBoundary: "realtime.Invoker",
		Implementation: "realtime/nativews",
	})
	for _, definition := range localcompat.Definitions() {
		for _, protocol := range []modelinvoker.Protocol{modelinvoker.ProtocolChatCompletions, modelinvoker.ProtocolResponses} {
			rows = append(rows, SurfaceRow{
				SurfaceID: "self-hosted." + string(definition.Product), Plane: PlaneLLMLocal,
				Provider: definition.Provider, Primitive: "invoke+stream", Protocol: protocol,
				Support: "explicit-attestation", Origin: "local", Fidelity: "compatible",
				InvocationBoundary: "modelinvoker.Invoker", Implementation: "provider/localcompat",
			})
		}
	}
	for _, protocol := range []modelinvoker.Protocol{
		modelinvoker.ProtocolChatCompletions, modelinvoker.ProtocolResponses,
		modelinvoker.ProtocolMessages, modelinvoker.ProtocolGenerateContent,
	} {
		rows = append(rows, SurfaceRow{
			SurfaceID: "third-party-relay", Plane: PlaneLLMRelay, Provider: relaycompat.ProviderID,
			Primitive: "invoke+stream", Protocol: protocol, Support: "compatible", Origin: "relay",
			Fidelity: "compatible", InvocationBoundary: "routegateway.Gateway",
			Implementation: "provider/relaycompat+routegateway.NewRelayCompatFactory",
		})
	}
	sort.Slice(rows, func(i, j int) bool { return surfaceRowKey(rows[i]) < surfaceRowKey(rows[j]) })
	return rows, nil
}

func (matrix UnionMatrix) Validate(now time.Time) error {
	if matrix.Version != UnionVersion {
		return fmt.Errorf("union surface matrix: unexpected version %q", matrix.Version)
	}
	if err := matrix.LLM.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		return fmt.Errorf("union surface matrix: validation time is required")
	}
	seen := make(map[string]struct{}, len(matrix.Surfaces))
	planes := make(map[PrimitivePlane]int)
	operationKinds := make(map[operation.Kind]struct{})
	surfaces := make(map[string]struct{})
	for _, row := range matrix.Surfaces {
		if strings.TrimSpace(row.SurfaceID) == "" || row.Plane == "" || strings.TrimSpace(string(row.Provider)) == "" ||
			strings.TrimSpace(row.Primitive) == "" || strings.TrimSpace(row.Support) == "" ||
			strings.TrimSpace(row.InvocationBoundary) == "" || strings.TrimSpace(row.Implementation) == "" {
			return fmt.Errorf("union surface matrix: incomplete row %+v", row)
		}
		key := surfaceRowKey(row)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("union surface matrix: duplicate row %q", key)
		}
		seen[key] = struct{}{}
		planes[row.Plane]++
		surfaces[row.SurfaceID] = struct{}{}
		if row.Plane == PlaneOperation {
			kind := operation.Kind(row.Primitive)
			operationKinds[kind] = struct{}{}
			if row.Lifecycle == "" {
				return fmt.Errorf("union surface matrix: operation %q has no lifecycle", key)
			}
		}
		if row.Plane == PlaneRealtime && row.Lifecycle != operation.LifecycleRealtime {
			return fmt.Errorf("union surface matrix: realtime %q has invalid lifecycle", key)
		}
	}
	expectedRows, err := canonicalSurfaceRows(now)
	if err != nil {
		return err
	}
	if len(matrix.Surfaces) != len(expectedRows) {
		return fmt.Errorf("union surface matrix: surface rows = %d, want %d", len(matrix.Surfaces), len(expectedRows))
	}
	expected := make(map[SurfaceRow]struct{}, len(expectedRows))
	for _, row := range expectedRows {
		expected[row] = struct{}{}
	}
	for _, row := range matrix.Surfaces {
		if _, ok := expected[row]; !ok {
			return fmt.Errorf("union surface matrix: non-canonical row %+v", row)
		}
	}
	for _, plane := range []PrimitivePlane{PlaneExecution, PlaneLLMLocal, PlaneLLMRelay, PlaneOperation, PlaneRealtime} {
		if planes[plane] == 0 {
			return fmt.Errorf("union surface matrix: plane %q is empty", plane)
		}
	}
	for _, kind := range operation.AllKinds() {
		if _, ok := operationKinds[kind]; !ok {
			return fmt.Errorf("union surface matrix: operation kind %q has no upstream implementation", kind)
		}
	}
	for _, definition := range operationspecs.Definitions() {
		if _, ok := surfaces[definition.ID]; !ok {
			return fmt.Errorf("union surface matrix: operation surface %q is missing", definition.ID)
		}
	}
	for _, definition := range realtimespecs.Definitions() {
		if _, ok := surfaces[definition.ID]; !ok {
			return fmt.Errorf("union surface matrix: realtime surface %q is missing", definition.ID)
		}
	}
	profiles, err := profile.RepresentativeProfiles(now)
	if err != nil {
		return err
	}
	for _, candidate := range profiles {
		if _, ok := surfaces[string(candidate.ID)]; !ok {
			return fmt.Errorf("union surface matrix: profile surface %q is missing", candidate.ID)
		}
	}
	if _, inference := surfaces["xai.inference"]; !inference {
		return fmt.Errorf("union surface matrix: xAI inference surface is missing")
	}
	if _, management := surfaces["xai.management"]; !management {
		return fmt.Errorf("union surface matrix: xAI management surface is missing")
	}
	return nil
}

func surfaceRowKey(row SurfaceRow) string {
	return strings.Join([]string{
		string(row.Plane), row.SurfaceID, string(row.Provider), row.Primitive,
		string(row.Protocol), string(row.Lifecycle), row.MechanismID,
	}, "\x00")
}
