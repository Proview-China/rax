package corepack

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const (
	PackageIDV1  = runtimeports.NamespacedNameV2("praxis.core-tool/local-coding-pack-v1")
	EffectKindV1 = runtimeports.NamespacedNameV2("praxis.tool/execute")
)

type DefinitionV1 struct {
	ModelName    string
	Capability   runtimeports.NamespacedNameV2
	Tool         runtimeports.NamespacedNameV2
	Risk         contract.RiskClass
	Review       runtimeports.NamespacedNameV2
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
	Description  string
	TimeoutMS    uint64
	ResultBytes  uint64
}

type CatalogConfigV1 struct {
	Owner            core.OwnerRef
	ArtifactDigest   core.Digest
	SignatureDigest  core.Digest
	ProvenanceDigest core.Digest
	CreatedAt        time.Time
}

type CatalogV1 struct {
	Capabilities []contract.CapabilityDescriptor
	Tools        []contract.ToolDescriptor
	Package      contract.ToolPackageManifest
	Materials    []contract.ToolDefinitionMaterialV1
	Definitions  []DefinitionV1
}

func BuildCatalogV1(config CatalogConfigV1) (CatalogV1, error) {
	if config.Owner.Validate() != nil || config.ArtifactDigest.Validate() != nil ||
		config.SignatureDigest.Validate() != nil || config.ProvenanceDigest.Validate() != nil ||
		config.CreatedAt.IsZero() {
		return CatalogV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Core Tool Pack catalog config is incomplete")
	}
	definitions := coreDefinitionsV1()
	catalog := CatalogV1{Definitions: cloneDefinitionsV1(definitions)}
	scopeSchema := schemaRefV1("action-scope", json.RawMessage(actionScopeSchemaV1))
	for _, definition := range definitions {
		input := schemaRefV1(schemaNameV1(definition.ModelName, "input"), definition.InputSchema)
		output := schemaRefV1(schemaNameV1(definition.ModelName, "output"), definition.OutputSchema)
		capability, err := contract.SealCapability(contract.CapabilityDescriptor{
			ID: definition.Capability, SemanticVersion: "1.0.0", Revision: 1, Owner: config.Owner,
			InputSchema: input, OutputSchema: output, ActionScopeSchema: scopeSchema,
			EffectKinds: []runtimeports.NamespacedNameV2{EffectKindV1}, Risk: definition.Risk,
			ReviewProfile:        definition.Review,
			AuthorityRequirement: "praxis.core-tool/authority-workspace-v1",
			BudgetRequirement:    "praxis.core-tool/budget-bounded-v1",
			SandboxRequirement:   "praxis.core-tool/sandbox-required-v1",
			EvidenceRequirement:  "praxis.core-tool/evidence-settled-v1",
			Compatibility:        runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
			CreatedUnixNano:      config.CreatedAt.UTC().UnixNano(),
		})
		if err != nil {
			return CatalogV1{}, err
		}
		tool, err := contract.SealTool(contract.ToolDescriptor{
			ID: definition.Tool, SemanticVersion: "1.0.0", Revision: 1, Owner: config.Owner,
			Capability:     objectRefV1(capability.ID, capability.Revision, capability.Digest),
			ArtifactDigest: config.ArtifactDigest, Mechanism: contract.MechanismLocal,
			InputSchema: input, OutputSchema: output,
			EffectKinds:   []runtimeports.NamespacedNameV2{EffectKindV1},
			TimeoutMillis: definition.TimeoutMS, ConcurrencyLimit: 64, CancellationSupported: true,
			Idempotency:    "praxis.core-tool/canonical-command-v1",
			ConflictDomain: "tenant/workspace/action", ResultLimitBytes: definition.ResultBytes,
			Conformance:     "praxis.core-tool/local-coding-v1",
			CreatedUnixNano: config.CreatedAt.UTC().UnixNano(),
		})
		if err != nil {
			return CatalogV1{}, err
		}
		description := core.DigestBytes([]byte(definition.Description))
		materialRef, err := contract.DeriveToolDefinitionMaterialRefV1(objectRefV1(tool.ID, tool.Revision, tool.Digest), input, description)
		if err != nil {
			return CatalogV1{}, err
		}
		material := contract.ToolDefinitionMaterialV1{Ref: materialRef, Description: definition.Description, InputSchema: append(json.RawMessage(nil), definition.InputSchema...)}
		if err := material.Validate(); err != nil {
			return CatalogV1{}, err
		}
		catalog.Capabilities = append(catalog.Capabilities, capability)
		catalog.Tools = append(catalog.Tools, tool)
		catalog.Materials = append(catalog.Materials, material)
	}
	descriptors := make([]contract.PackageDescriptorRef, 0, len(catalog.Tools))
	for _, tool := range catalog.Tools {
		descriptors = append(descriptors, contract.PackageDescriptorRef{ToolID: tool.ID, Revision: tool.Revision, Digest: tool.Digest})
	}
	pkg, err := contract.SealPackage(contract.ToolPackageManifest{
		ID: PackageIDV1, SemanticVersion: "1.0.0", Revision: 1, Publisher: config.Owner,
		ArtifactDigest: config.ArtifactDigest, Signatures: []core.Digest{config.SignatureDigest},
		Descriptors: descriptors, EffectKinds: []runtimeports.NamespacedNameV2{EffectKindV1},
		ReviewRequirement:  "praxis.core-tool/review-mixed-v1",
		SandboxRequirement: "praxis.core-tool/sandbox-required-v1",
		ProvenanceDigest:   config.ProvenanceDigest, CreatedUnixNano: config.CreatedAt.UTC().UnixNano(),
	})
	if err != nil {
		return CatalogV1{}, err
	}
	catalog.Package = pkg
	return catalog, nil
}

func (c CatalogV1) Validate() error {
	if len(c.Capabilities) != 5 || len(c.Tools) != 5 || len(c.Materials) != 5 || len(c.Definitions) != 5 || c.Package.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Core Tool Pack catalog is incomplete")
	}
	for i := range c.Tools {
		if c.Capabilities[i].Validate() != nil || c.Tools[i].ValidateAgainst(c.Capabilities[i]) != nil ||
			c.Materials[i].Validate() != nil || c.Materials[i].Ref.Tool != objectRefV1(c.Tools[i].ID, c.Tools[i].Revision, c.Tools[i].Digest) {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Core Tool Pack catalog exact bindings drifted")
		}
	}
	return nil
}

func coreDefinitionsV1() []DefinitionV1 {
	return []DefinitionV1{
		definitionV1(contract.CoreToolWorkspaceReadV1, "workspace-read", contract.RiskLow, "praxis.core-tool/review-bypass-v1", workspaceReadSchemaV1, workspaceReadOutputSchemaV1, "Read a bounded byte range from one exact workspace file.", contract.CoreToolDefaultTimeoutMSV1, contract.CoreToolMaxReadBytesV1),
		definitionV1(contract.CoreToolWorkspaceSearchV1, "workspace-search", contract.RiskLow, "praxis.core-tool/review-bypass-v1", workspaceSearchSchemaV1, workspaceSearchOutputSchemaV1, "Search bounded text matches inside one exact workspace.", contract.CoreToolDefaultTimeoutMSV1, contract.CoreToolMaxSearchBytesV1),
		definitionV1(contract.CoreToolWorkspaceInspectV1, "workspace-inspect", contract.RiskLow, "praxis.core-tool/review-bypass-v1", workspaceInspectSchemaV1, workspaceInspectOutputSchemaV1, "Inspect bounded file or directory metadata and currentness.", contract.CoreToolDefaultTimeoutMSV1, contract.CoreToolMaxReadBytesV1),
		definitionV1(contract.CoreToolWorkspacePatchV1, "workspace-patch", contract.RiskModerate, "praxis.core-tool/review-auto-v1", workspacePatchSchemaV1, workspacePatchOutputSchemaV1, "Atomically apply structured text hunks against exact base digests.", contract.CoreToolDefaultTimeoutMSV1, uint64(contract.CoreToolMaxPatchBytesV1)),
		definitionV1(contract.CoreToolProcessExecV1, "process-exec", contract.RiskModerate, "praxis.core-tool/review-auto-v1", processExecSchemaV1, processExecOutputSchemaV1, "Execute bounded argv without an implicit shell in a governed sandbox.", contract.CoreToolMaxTimeoutMSV1, contract.CoreToolMaxOutputBytesV1),
	}
}

func definitionV1(model, name string, risk contract.RiskClass, review runtimeports.NamespacedNameV2, input, output, description string, timeout, result uint64) DefinitionV1 {
	return DefinitionV1{
		ModelName:  model,
		Capability: runtimeports.NamespacedNameV2("praxis.core-tool/" + name),
		Tool:       runtimeports.NamespacedNameV2("praxis.core-tool/" + name + "-local-v1"),
		Risk:       risk, Review: review, InputSchema: json.RawMessage(input),
		OutputSchema: json.RawMessage(output), Description: description,
		TimeoutMS: timeout, ResultBytes: result,
	}
}

func schemaRefV1(name string, body json.RawMessage) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{
		Namespace: "praxis.core-tool.schema", Name: name, Version: "1.0.0",
		MediaType: "application/schema+json", ContentDigest: core.DigestBytes(body),
	}
}

func schemaNameV1(model, direction string) string {
	result := make([]byte, 0, len(model)+len(direction)+1)
	for i := range len(model) {
		if model[i] == '.' {
			result = append(result, '-')
		} else {
			result = append(result, model[i])
		}
	}
	return string(result) + "-" + direction
}

func objectRefV1(id runtimeports.NamespacedNameV2, revision core.Revision, digest core.Digest) contract.ObjectRef {
	return contract.ObjectRef{ID: string(id), Revision: revision, Digest: digest}
}

func cloneDefinitionsV1(values []DefinitionV1) []DefinitionV1 {
	out := append([]DefinitionV1(nil), values...)
	for i := range out {
		out[i].InputSchema = append(json.RawMessage(nil), out[i].InputSchema...)
		out[i].OutputSchema = append(json.RawMessage(nil), out[i].OutputSchema...)
	}
	return out
}

func (c CatalogV1) Clone() CatalogV1 {
	out := c
	out.Capabilities = append([]contract.CapabilityDescriptor(nil), c.Capabilities...)
	out.Tools = append([]contract.ToolDescriptor(nil), c.Tools...)
	out.Materials = append([]contract.ToolDefinitionMaterialV1(nil), c.Materials...)
	for i := range out.Materials {
		out.Materials[i] = out.Materials[i].Clone()
	}
	out.Definitions = cloneDefinitionsV1(c.Definitions)
	out.Package.Descriptors = append([]contract.PackageDescriptorRef(nil), c.Package.Descriptors...)
	out.Package.Signatures = append([]core.Digest(nil), c.Package.Signatures...)
	out.Package.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), c.Package.EffectKinds...)
	return out
}

func SortedModelNamesV1(c CatalogV1) []string {
	names := make([]string, 0, len(c.Definitions))
	for _, definition := range c.Definitions {
		names = append(names, definition.ModelName)
	}
	sort.Strings(names)
	return names
}

const actionScopeSchemaV1 = `{"additionalProperties":false,"properties":{"action_id":{"type":"string"},"scope_digest":{"type":"string"}},"required":["action_id","scope_digest"],"type":"object"}`
const workspaceRefSchemaPropertiesV1 = `"workspace_root":{"additionalProperties":false,"properties":{"digest":{"type":"string"},"id":{"type":"string"},"revision":{"minimum":1,"type":"integer"}},"required":["id","revision","digest"],"type":"object"}`
const workspaceReadSchemaV1 = `{"additionalProperties":false,"properties":{"max_bytes":{"maximum":1048576,"minimum":1,"type":"integer"},"relative_path":{"maxLength":4096,"type":"string"},"requested_not_after_unix_nano":{"minimum":1,"type":"integer"},"start_byte":{"minimum":0,"type":"integer"},` + workspaceRefSchemaPropertiesV1 + `},"required":["workspace_root","relative_path","max_bytes","requested_not_after_unix_nano"],"type":"object"}`
const workspaceSearchSchemaV1 = `{"additionalProperties":false,"properties":{"max_result_bytes":{"maximum":1048576,"minimum":1,"type":"integer"},"max_results":{"maximum":200,"minimum":1,"type":"integer"},"mode":{"enum":["literal","regexp-re2"]},"path_prefix":{"maxLength":4096,"type":"string"},"query":{"maxLength":4096,"minLength":1,"type":"string"},"requested_not_after_unix_nano":{"minimum":1,"type":"integer"},` + workspaceRefSchemaPropertiesV1 + `},"required":["workspace_root","query","mode","max_results","max_result_bytes","requested_not_after_unix_nano"],"type":"object"}`
const workspaceInspectSchemaV1 = `{"additionalProperties":false,"properties":{"max_entries":{"maximum":1000,"minimum":1,"type":"integer"},"range":{"additionalProperties":false,"properties":{"max_bytes":{"maximum":1048576,"minimum":0,"type":"integer"},"start_byte":{"minimum":0,"type":"integer"}},"required":["start_byte","max_bytes"],"type":"object"},"relative_path":{"maxLength":4096,"type":"string"},"requested_not_after_unix_nano":{"minimum":1,"type":"integer"},` + workspaceRefSchemaPropertiesV1 + `},"required":["workspace_root","relative_path","range","max_entries","requested_not_after_unix_nano"],"type":"object"}`
const workspacePatchSchemaV1 = `{"additionalProperties":false,"properties":{"changes":{"items":{"additionalProperties":false,"properties":{"base_digest":{"type":"string"},"base_revision":{"minimum":1,"type":"integer"},"hunks":{"items":{"type":"object"},"maxItems":1024,"minItems":1,"type":"array"},"relative_path":{"type":"string"}},"required":["relative_path","base_revision","base_digest","hunks"],"type":"object"},"maxItems":64,"minItems":1,"type":"array"},"requested_not_after_unix_nano":{"minimum":1,"type":"integer"},` + workspaceRefSchemaPropertiesV1 + `},"required":["workspace_root","changes","requested_not_after_unix_nano"],"type":"object"}`
const processExecSchemaV1 = `{"additionalProperties":false,"properties":{"argv":{"items":{"maxLength":8192,"minLength":1,"type":"string"},"maxItems":128,"minItems":1,"type":"array"},"cwd":{"maxLength":4096,"type":"string"},"env":{"maxProperties":64,"type":"object"},"max_stderr_bytes":{"maximum":1048576,"minimum":1,"type":"integer"},"max_stdout_bytes":{"maximum":1048576,"minimum":1,"type":"integer"},"requested_not_after_unix_nano":{"minimum":1,"type":"integer"},"timeout_millis":{"maximum":300000,"minimum":1,"type":"integer"},` + workspaceRefSchemaPropertiesV1 + `},"required":["workspace_root","argv","cwd","timeout_millis","max_stdout_bytes","max_stderr_bytes","requested_not_after_unix_nano"],"type":"object"}`
const workspaceReadOutputSchemaV1 = `{"additionalProperties":false,"properties":{"artifact_ref":{"type":["object","null"]},"bytes_returned":{"minimum":0,"type":"integer"},"complete":{"type":"boolean"},"content":{"type":"string"},"file":{"type":"object"},"start_byte":{"minimum":0,"type":"integer"},"total_bytes":{"minimum":0,"type":"integer"}},"required":["file","start_byte","bytes_returned","total_bytes","complete","content","artifact_ref"],"type":"object"}`
const workspaceSearchOutputSchemaV1 = `{"additionalProperties":false,"properties":{"artifact_ref":{"type":["object","null"]},"complete":{"type":"boolean"},"matches":{"type":"array"},"workspace_digest":{"type":"string"},"workspace_revision":{"minimum":1,"type":"integer"}},"required":["workspace_revision","workspace_digest","matches","complete","artifact_ref"],"type":"object"}`
const workspaceInspectOutputSchemaV1 = `{"additionalProperties":false,"properties":{"complete":{"type":"boolean"},"entries":{"type":"array"},"object":{"type":"object"},"range_valid":{"type":"boolean"}},"required":["object","range_valid","entries","complete"],"type":"object"}`
const workspacePatchOutputSchemaV1 = `{"additionalProperties":false,"properties":{"base_workspace":{"type":"object"},"change_set_ref":{"type":"object"},"files":{"type":"array"},"result_workspace":{"type":"object"}},"required":["change_set_ref","base_workspace","result_workspace","files"],"type":"object"}`
const processExecOutputSchemaV1 = `{"additionalProperties":false,"properties":{"attempt_ref":{"type":"object"},"exit_code":{"type":"integer"},"stderr":{"type":"string"},"stderr_artifact_ref":{"type":["object","null"]},"stdout":{"type":"string"},"stdout_artifact_ref":{"type":["object","null"]},"timed_out":{"type":"boolean"}},"required":["attempt_ref","exit_code","stdout","stderr","stdout_artifact_ref","stderr_artifact_ref","timed_out"],"type":"object"}`
