package contract

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ModelToolInjectionMaterialContractVersionV1 = "praxis.tool-mcp.model-tool-injection-material/v1"

const modelToolInjectionMaterialCanonicalDomainV1 = "praxis.tool-mcp.model-tool-injection-material"

type ModelToolInjectionMaterialRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r ModelToolInjectionMaterialRefV1) Validate() error {
	if r.ContractVersion != ModelToolInjectionMaterialContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 {
		return invalid("Model Tool Injection Material Ref is invalid")
	}
	return r.Digest.Validate()
}

type ModelToolInjectionEntryV1 struct {
	Order                 uint32                          `json:"order"`
	ModelName             string                          `json:"model_name"`
	CapabilityRef         ObjectRef                       `json:"capability_ref"`
	ToolRef               ObjectRef                       `json:"tool_ref"`
	DefinitionMaterialRef ToolDefinitionMaterialRefV1     `json:"definition_material_ref"`
	InputSchemaRef        runtimeports.SchemaRefV2        `json:"input_schema_ref"`
	DescriptionDigest     core.Digest                     `json:"description_digest"`
	Strict                bool                            `json:"strict"`
	Admission             AdmissionClass                  `json:"admission"`
	EffectKinds           []runtimeports.NamespacedNameV2 `json:"effect_kinds"`
	ReviewProfile         runtimeports.NamespacedNameV2   `json:"review_profile"`
	AuthorityRequirement  runtimeports.NamespacedNameV2   `json:"authority_requirement"`
	BudgetRequirement     runtimeports.NamespacedNameV2   `json:"budget_requirement"`
	SandboxRequirement    runtimeports.NamespacedNameV2   `json:"sandbox_requirement"`
	EvidenceRequirement   runtimeports.NamespacedNameV2   `json:"evidence_requirement"`
}

func (e ModelToolInjectionEntryV1) Validate() error {
	if ValidatePortableFunctionToolNameV1(e.ModelName) != nil || e.CapabilityRef.Validate() != nil || e.ToolRef.Validate() != nil || e.DefinitionMaterialRef.Validate() != nil || e.InputSchemaRef.Validate() != nil || e.DescriptionDigest.Validate() != nil {
		return invalid("Model Tool Injection entry identity or references are invalid")
	}
	if e.DefinitionMaterialRef.Tool != e.ToolRef || e.DefinitionMaterialRef.InputSchema != e.InputSchemaRef || e.DefinitionMaterialRef.DescriptionDigest != e.DescriptionDigest {
		return conflict("Model Tool Injection entry Definition Material closure drifted")
	}
	if !e.Strict {
		return invalid("Model Tool Injection entry must remain strict")
	}
	if e.Admission != AdmissionRequired && e.Admission != AdmissionPreApproved {
		return invalid("Model Tool Injection entry Admission is invalid")
	}
	if err := ValidateSortedUniqueNames(e.EffectKinds, MaxDescriptorEffects); err != nil {
		return err
	}
	for _, value := range []runtimeports.NamespacedNameV2{
		e.ReviewProfile,
		e.AuthorityRequirement,
		e.BudgetRequirement,
		e.SandboxRequirement,
		e.EvidenceRequirement,
	} {
		if runtimeports.ValidateNamespacedNameV2(value) != nil {
			return invalid("Model Tool Injection entry governance requirement is invalid")
		}
	}
	return nil
}

type ModelToolInjectionMaterialV1 struct {
	ContractVersion         string                          `json:"contract_version"`
	Ref                     ModelToolInjectionMaterialRefV1 `json:"ref"`
	Surface                 ToolSurfaceManifestCurrentRefV1 `json:"surface"`
	Entries                 []ModelToolInjectionEntryV1     `json:"ordered_entries"`
	ExpectedInjectionDigest core.Digest                     `json:"expected_injection_digest"`
	CompiledToolsDigest     core.Digest                     `json:"compiled_tools_digest"`
	CreatedUnixNano         int64                           `json:"created_unix_nano"`
	ExpiresUnixNano         int64                           `json:"expires_unix_nano"`
	Digest                  core.Digest                     `json:"digest"`
}

func (m ModelToolInjectionMaterialV1) Validate() error {
	if m.ContractVersion != ModelToolInjectionMaterialContractVersionV1 || m.Ref.ContractVersion != m.ContractVersion || m.Surface.Validate() != nil || len(m.Entries) == 0 || len(m.Entries) > MaxSurfaceEntries || m.CreatedUnixNano <= 0 || m.CreatedUnixNano >= m.ExpiresUnixNano {
		return invalid("Model Tool Injection Material identity, entries, or lifetime are invalid")
	}
	if err := m.Ref.Validate(); err != nil {
		return err
	}
	if m.Ref.Revision != 1 || m.Digest != m.Ref.Digest || m.ExpectedInjectionDigest.Validate() != nil || m.CompiledToolsDigest.Validate() != nil {
		return conflict("Model Tool Injection Material repeated digest or revision fields drifted")
	}
	seenModelNames := make(map[string]struct{}, len(m.Entries))
	for index, entry := range m.Entries {
		if err := entry.Validate(); err != nil {
			return err
		}
		if _, exists := seenModelNames[entry.ModelName]; exists {
			return conflict("Model Tool Injection entries contain a duplicate Model name")
		}
		seenModelNames[entry.ModelName] = struct{}{}
		if entry.Order != uint32(index) {
			return invalid("Model Tool Injection entries are not consecutively ordered")
		}
		if index > 0 && modelToolInjectionEntryLessV1(entry, m.Entries[index-1]) {
			return invalid("Model Tool Injection entries are not in canonical order")
		}
	}
	expected, err := ComputeExpectedInjectionDigest(modelToolInjectionSurfaceEntriesV1(m.Entries))
	if err != nil || expected != m.ExpectedInjectionDigest {
		return conflict("Model Tool Injection expected digest drifted")
	}
	expectedID, err := DeriveModelToolInjectionMaterialIDV1(m)
	if err != nil || expectedID != m.Ref.ID {
		return conflict("Model Tool Injection Material ID drifted")
	}
	digest, err := m.ComputeDigest()
	if err != nil || digest != m.Digest {
		return conflict("Model Tool Injection Material canonical digest drifted")
	}
	return nil
}

func (m ModelToolInjectionMaterialV1) ValidateCurrent(expected ModelToolInjectionMaterialRefV1, now time.Time) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if expected.Validate() != nil || m.Ref != expected {
		return conflict("Model Tool Injection Material exact Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < m.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Tool Injection Material current clock regressed")
	}
	if !now.Before(time.Unix(0, m.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Model Tool Injection Material expired")
	}
	return nil
}

func (m ModelToolInjectionMaterialV1) ComputeDigest() (core.Digest, error) {
	m = m.Clone()
	m.Ref.Digest = ""
	m.Digest = ""
	return core.CanonicalJSONDigest(modelToolInjectionMaterialCanonicalDomainV1, ModelToolInjectionMaterialContractVersionV1, "ModelToolInjectionMaterialV1", m)
}

func SealModelToolInjectionMaterialV1(m ModelToolInjectionMaterialV1) (ModelToolInjectionMaterialV1, error) {
	m = m.Clone()
	if m.ContractVersion != "" && m.ContractVersion != ModelToolInjectionMaterialContractVersionV1 {
		return ModelToolInjectionMaterialV1{}, invalid("Model Tool Injection Material contract version drifted")
	}
	m.ContractVersion = ModelToolInjectionMaterialContractVersionV1
	m.Ref.ContractVersion = m.ContractVersion
	if m.Ref.Revision != 0 && m.Ref.Revision != 1 {
		return ModelToolInjectionMaterialV1{}, conflict("supplied Model Tool Injection Material revision drifted")
	}
	m.Ref.Revision = 1
	for index := range m.Entries {
		m.Entries[index].EffectKinds = SortedUniqueNames(m.Entries[index].EffectKinds)
	}
	expectedID, err := DeriveModelToolInjectionMaterialIDV1(m)
	if err != nil {
		return ModelToolInjectionMaterialV1{}, err
	}
	if m.Ref.ID != "" && m.Ref.ID != expectedID {
		return ModelToolInjectionMaterialV1{}, conflict("supplied Model Tool Injection Material ID drifted")
	}
	m.Ref.ID = expectedID
	providedRefDigest, providedDigest := m.Ref.Digest, m.Digest
	m.Ref.Digest, m.Digest = "", ""
	digest, err := m.ComputeDigest()
	if err != nil {
		return ModelToolInjectionMaterialV1{}, err
	}
	for _, provided := range []core.Digest{providedRefDigest, providedDigest} {
		if provided != "" && provided != digest {
			return ModelToolInjectionMaterialV1{}, conflict("supplied Model Tool Injection Material digest drifted")
		}
	}
	m.Ref.Digest, m.Digest = digest, digest
	if err := m.Validate(); err != nil {
		return ModelToolInjectionMaterialV1{}, err
	}
	return m, nil
}

func DeriveModelToolInjectionMaterialIDV1(material ModelToolInjectionMaterialV1) (string, error) {
	material = material.Clone()
	if material.Surface.Validate() != nil || material.ExpectedInjectionDigest.Validate() != nil || material.CompiledToolsDigest.Validate() != nil || len(material.Entries) == 0 || material.CreatedUnixNano <= 0 || material.CreatedUnixNano >= material.ExpiresUnixNano {
		return "", invalid("Model Tool Injection Material identity inputs are invalid")
	}
	for index, entry := range material.Entries {
		if entry.Validate() != nil || entry.Order != uint32(index) {
			return "", invalid("Model Tool Injection Material identity entries are invalid")
		}
	}
	digest, err := core.CanonicalJSONDigest(modelToolInjectionMaterialCanonicalDomainV1, ModelToolInjectionMaterialContractVersionV1, "ModelToolInjectionMaterialIdentityV1", struct {
		Surface                 ToolSurfaceManifestCurrentRefV1 `json:"surface"`
		Entries                 []ModelToolInjectionEntryV1     `json:"ordered_entries"`
		ExpectedInjectionDigest core.Digest                     `json:"expected_injection_digest"`
		CompiledToolsDigest     core.Digest                     `json:"compiled_tools_digest"`
		CreatedUnixNano         int64                           `json:"created_unix_nano"`
		ExpiresUnixNano         int64                           `json:"expires_unix_nano"`
	}{
		Surface: material.Surface, Entries: material.Entries,
		ExpectedInjectionDigest: material.ExpectedInjectionDigest, CompiledToolsDigest: material.CompiledToolsDigest,
		CreatedUnixNano: material.CreatedUnixNano, ExpiresUnixNano: material.ExpiresUnixNano,
	})
	if err != nil {
		return "", err
	}
	return "model-tool-injection-v1-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func (m ModelToolInjectionMaterialV1) Clone() ModelToolInjectionMaterialV1 {
	m.Entries = append([]ModelToolInjectionEntryV1(nil), m.Entries...)
	for index := range m.Entries {
		m.Entries[index].EffectKinds = append([]runtimeports.NamespacedNameV2(nil), m.Entries[index].EffectKinds...)
	}
	return m
}

type ModelToolInjectionMaterialReaderV1 interface {
	InspectExactModelToolInjectionMaterialV1(context.Context, ModelToolInjectionMaterialRefV1) (ModelToolInjectionMaterialV1, error)
}

func modelToolInjectionSurfaceEntriesV1(entries []ModelToolInjectionEntryV1) []ToolSurfaceEntry {
	result := make([]ToolSurfaceEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, ToolSurfaceEntry{
			Capability:        entry.CapabilityRef,
			Tool:              entry.ToolRef,
			ModelName:         entry.ModelName,
			InputSchema:       entry.InputSchemaRef,
			DescriptionDigest: entry.DescriptionDigest,
			Order:             entry.Order,
			Visibility:        SurfaceVisible,
			Allowed:           true,
			Admission:         entry.Admission,
			MechanismDigest:   entry.ToolRef.Digest,
			EffectKinds:       append([]runtimeports.NamespacedNameV2(nil), entry.EffectKinds...),
		})
	}
	return result
}

func modelToolInjectionEntryLessV1(left, right ModelToolInjectionEntryV1) bool {
	if left.ModelName != right.ModelName {
		return left.ModelName < right.ModelName
	}
	if left.CapabilityRef.ID != right.CapabilityRef.ID {
		return left.CapabilityRef.ID < right.CapabilityRef.ID
	}
	return left.ToolRef.ID < right.ToolRef.ID
}
