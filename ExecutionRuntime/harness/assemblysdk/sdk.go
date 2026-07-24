package assemblysdk

import (
	"encoding/json"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type SDK struct{ compiler assemblycompiler.Compiler }

func New() SDK { return SDK{compiler: assemblycompiler.New()} }

func (s SDK) Compile(input assemblycontract.AssemblyInputV1) (assemblycontract.CompileResultV1, error) {
	result, err := s.compiler.Compile(clone(input))
	return clone(result), err
}

func Inspect(result assemblycontract.CompileResultV1) assemblycontract.CompileResultV1 {
	return clone(result)
}

type ExplanationV1 struct {
	Reference     string                                  `json:"reference"`
	Found         bool                                    `json:"found"`
	GenerationRef *assemblycontract.ObjectRefV1           `json:"generation_ref,omitempty"`
	Slot          *assemblycontract.ResolvedSlotV1        `json:"slot,omitempty"`
	Phase         *assemblycontract.ResolvedPhaseV1       `json:"phase,omitempty"`
	Diagnostics   []assemblycontract.AssemblyDiagnosticV1 `json:"diagnostics"`
}

func Explain(result assemblycontract.CompileResultV1, reference string) ExplanationV1 {
	explanation := ExplanationV1{Reference: reference, Diagnostics: []assemblycontract.AssemblyDiagnosticV1{}}
	if result.Generation != nil {
		ref := assemblycontract.ObjectRefV1{ID: result.Generation.GenerationID, Revision: result.Generation.Revision, Digest: result.Generation.Digest}
		explanation.GenerationRef = &ref
		if ref.ID == reference {
			explanation.Found = true
		}
	}
	if result.Graph != nil {
		for _, slot := range result.Graph.Slots {
			if slot.SlotID == reference || contains(slot.Contributions, reference) {
				copyValue := clone(slot)
				explanation.Slot = &copyValue
				explanation.Found = true
				break
			}
		}
		for _, phase := range result.Graph.Phases {
			if phase.HookFaceID == reference || phase.PhaseID == reference || contains(phase.Contributions, reference) {
				copyValue := clone(phase)
				explanation.Phase = &copyValue
				explanation.Found = true
				break
			}
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.ObjectPath == reference || diagnostic.FieldPath == reference || diagnostic.Code == reference {
			explanation.Diagnostics = append(explanation.Diagnostics, diagnostic)
			explanation.Found = true
		}
	}
	return clone(explanation)
}

type ChangeV1 struct {
	Kind      string      `json:"kind"`
	Reference string      `json:"reference"`
	Before    core.Digest `json:"before,omitempty"`
	After     core.Digest `json:"after,omitempty"`
}
type AssemblyDiffV1 struct {
	FromGeneration string     `json:"from_generation,omitempty"`
	ToGeneration   string     `json:"to_generation,omitempty"`
	Changed        bool       `json:"changed"`
	Changes        []ChangeV1 `json:"changes"`
}

func Diff(left, right assemblycontract.CompileResultV1) AssemblyDiffV1 {
	diff := AssemblyDiffV1{Changes: []ChangeV1{}}
	if left.Generation != nil {
		diff.FromGeneration = left.Generation.GenerationID
	}
	if right.Generation != nil {
		diff.ToGeneration = right.Generation.GenerationID
	}
	compareDigest := func(kind, ref string, before, after core.Digest) {
		if before != after {
			diff.Changed = true
			diff.Changes = append(diff.Changes, ChangeV1{Kind: kind, Reference: ref, Before: before, After: after})
		}
	}
	if left.Manifest != nil || right.Manifest != nil {
		var before, after core.Digest
		if left.Manifest != nil {
			before = left.Manifest.Digest
		}
		if right.Manifest != nil {
			after = right.Manifest.Digest
		}
		compareDigest("manifest", "assembly_manifest", before, after)
	}
	if left.Graph != nil || right.Graph != nil {
		var before, after core.Digest
		if left.Graph != nil {
			before = left.Graph.Digest
		}
		if right.Graph != nil {
			after = right.Graph.Digest
		}
		compareDigest("graph", "compiled_harness_graph", before, after)
	}
	if left.Handoff != nil || right.Handoff != nil {
		var before, after core.Digest
		if left.Handoff != nil {
			before = left.Handoff.Digest
		}
		if right.Handoff != nil {
			after = right.Handoff.Digest
		}
		compareDigest("handoff", "assembly_handoff", before, after)
	}
	return clone(diff)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func clone[T any](value T) T {
	payload, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var result T
	if err := json.Unmarshal(payload, &result); err != nil {
		return value
	}
	return result
}

func errNilBuilder() error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "assembly SDK builder is nil")
}
