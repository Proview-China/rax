package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type RiskClass string

const (
	RiskLow      RiskClass = "low"
	RiskModerate RiskClass = "moderate"
	RiskHigh     RiskClass = "high"
)

type CapabilityDescriptor struct {
	ContractVersion      string                          `json:"contract_version"`
	ID                   runtimeports.NamespacedNameV2   `json:"id"`
	SemanticVersion      string                          `json:"semantic_version"`
	Revision             core.Revision                   `json:"revision"`
	Digest               core.Digest                     `json:"digest"`
	Owner                core.OwnerRef                   `json:"owner"`
	InputSchema          runtimeports.SchemaRefV2        `json:"input_schema"`
	OutputSchema         runtimeports.SchemaRefV2        `json:"output_schema"`
	ActionScopeSchema    runtimeports.SchemaRefV2        `json:"action_scope_schema"`
	EffectKinds          []runtimeports.NamespacedNameV2 `json:"effect_kinds"`
	Risk                 RiskClass                       `json:"risk"`
	ReviewProfile        runtimeports.NamespacedNameV2   `json:"review_profile"`
	AuthorityRequirement runtimeports.NamespacedNameV2   `json:"authority_requirement"`
	BudgetRequirement    runtimeports.NamespacedNameV2   `json:"budget_requirement"`
	SandboxRequirement   runtimeports.NamespacedNameV2   `json:"sandbox_requirement"`
	EvidenceRequirement  runtimeports.NamespacedNameV2   `json:"evidence_requirement"`
	Compatibility        runtimeports.VersionRangeV2     `json:"compatibility"`
	CreatedUnixNano      int64                           `json:"created_unix_nano"`
}

func (d CapabilityDescriptor) Validate() error {
	if err := d.validateShape(); err != nil {
		return err
	}
	if err := d.Digest.Validate(); err != nil {
		return err
	}
	expected, err := d.ComputeDigest()
	if err != nil || expected != d.Digest {
		return conflict("capability digest does not bind exact content")
	}
	return nil
}

func (d CapabilityDescriptor) validateShape() error {
	if d.ContractVersion != CapabilityContractVersion || d.Revision == 0 || d.CreatedUnixNano <= 0 {
		return invalid("capability requires contract version, revision and creation time")
	}
	if runtimeports.ValidateNamespacedNameV2(d.ID) != nil || d.Owner.Validate() != nil {
		return invalid("capability requires namespaced id and owner")
	}
	version, err := core.ParseSemanticVersion(d.SemanticVersion)
	if err != nil || version.String() != d.SemanticVersion || len(version.Build) != 0 {
		return invalid("capability semantic version must be canonical and omit build metadata")
	}
	if d.InputSchema.Validate() != nil || d.OutputSchema.Validate() != nil || d.ActionScopeSchema.Validate() != nil {
		return invalid("capability schema reference is invalid")
	}
	if err := ValidateSortedUniqueNames(d.EffectKinds, MaxDescriptorEffects); err != nil {
		return err
	}
	if d.Risk != RiskLow && d.Risk != RiskModerate && d.Risk != RiskHigh {
		return invalid("capability risk class is invalid")
	}
	for _, value := range []runtimeports.NamespacedNameV2{d.ReviewProfile, d.AuthorityRequirement, d.BudgetRequirement, d.SandboxRequirement, d.EvidenceRequirement} {
		if runtimeports.ValidateNamespacedNameV2(value) != nil {
			return invalid("capability governance requirement must be namespaced")
		}
	}
	return d.Compatibility.Validate()
}

func (d CapabilityDescriptor) ComputeDigest() (core.Digest, error) {
	if err := d.validateShape(); err != nil {
		return "", err
	}
	d.Digest = ""
	return Seal("praxis.tool-mcp.capability", CapabilityContractVersion, "CapabilityDescriptor", d)
}

func SealCapability(d CapabilityDescriptor) (CapabilityDescriptor, error) {
	d.ContractVersion = CapabilityContractVersion
	d.EffectKinds = SortedUniqueNames(d.EffectKinds)
	d.Digest = ""
	digest, err := d.ComputeDigest()
	if err != nil {
		return CapabilityDescriptor{}, err
	}
	d.Digest = digest
	return d, nil
}

type ToolMechanism string

const (
	MechanismLocal  ToolMechanism = "local"
	MechanismMCP    ToolMechanism = "mcp"
	MechanismHosted ToolMechanism = "hosted"
	MechanismRemote ToolMechanism = "remote"
	MechanismWASM   ToolMechanism = "wasm"
)

type ToolDescriptor struct {
	ContractVersion       string                          `json:"contract_version"`
	ID                    runtimeports.NamespacedNameV2   `json:"id"`
	SemanticVersion       string                          `json:"semantic_version"`
	Revision              core.Revision                   `json:"revision"`
	Digest                core.Digest                     `json:"digest"`
	Owner                 core.OwnerRef                   `json:"owner"`
	Capability            ObjectRef                       `json:"capability"`
	ArtifactDigest        core.Digest                     `json:"artifact_digest"`
	Mechanism             ToolMechanism                   `json:"mechanism"`
	InputSchema           runtimeports.SchemaRefV2        `json:"input_schema"`
	OutputSchema          runtimeports.SchemaRefV2        `json:"output_schema"`
	EffectKinds           []runtimeports.NamespacedNameV2 `json:"effect_kinds"`
	TimeoutMillis         uint64                          `json:"timeout_millis"`
	ConcurrencyLimit      uint32                          `json:"concurrency_limit"`
	CancellationSupported bool                            `json:"cancellation_supported"`
	Idempotency           runtimeports.NamespacedNameV2   `json:"idempotency"`
	ConflictDomain        string                          `json:"conflict_domain_template"`
	ResultLimitBytes      uint64                          `json:"result_limit_bytes"`
	Conformance           runtimeports.NamespacedNameV2   `json:"conformance"`
	Residuals             []Residual                      `json:"residuals,omitempty"`
	CreatedUnixNano       int64                           `json:"created_unix_nano"`
}

func (d ToolDescriptor) Validate() error {
	if err := d.validateShape(); err != nil {
		return err
	}
	if err := d.Digest.Validate(); err != nil {
		return err
	}
	expected, err := d.ComputeDigest()
	if err != nil || expected != d.Digest {
		return conflict("tool digest does not bind exact content")
	}
	return nil
}

func (d ToolDescriptor) validateShape() error {
	if d.ContractVersion != ToolContractVersion || d.Revision == 0 || d.CreatedUnixNano <= 0 || d.TimeoutMillis == 0 || d.ConcurrencyLimit == 0 || d.ResultLimitBytes == 0 {
		return invalid("tool descriptor has incomplete version, limits or time")
	}
	if runtimeports.ValidateNamespacedNameV2(d.ID) != nil || d.Owner.Validate() != nil || d.Capability.Validate() != nil || d.ArtifactDigest.Validate() != nil {
		return invalid("tool identity, owner, capability or artifact is invalid")
	}
	version, err := core.ParseSemanticVersion(d.SemanticVersion)
	if err != nil || version.String() != d.SemanticVersion || len(version.Build) != 0 {
		return invalid("tool semantic version must be canonical")
	}
	switch d.Mechanism {
	case MechanismLocal, MechanismMCP, MechanismHosted, MechanismRemote, MechanismWASM:
	default:
		return invalid("tool mechanism is invalid")
	}
	if d.InputSchema.Validate() != nil || d.OutputSchema.Validate() != nil || ValidateSortedUniqueNames(d.EffectKinds, MaxDescriptorEffects) != nil {
		return invalid("tool schema or effect list is invalid")
	}
	if runtimeports.ValidateNamespacedNameV2(d.Idempotency) != nil || runtimeports.ValidateNamespacedNameV2(d.Conformance) != nil || strings.TrimSpace(d.ConflictDomain) == "" || len(d.ConflictDomain) > MaxStringBytes {
		return invalid("tool execution declarations are incomplete")
	}
	if len(d.Residuals) > MaxResiduals {
		return invalid("tool residual list exceeds limit")
	}
	for _, residual := range d.Residuals {
		if err := residual.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (d ToolDescriptor) ValidateAgainst(capability CapabilityDescriptor) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if err := capability.Validate(); err != nil {
		return err
	}
	if d.Capability.ID != string(capability.ID) || d.Capability.Revision != capability.Revision || d.Capability.Digest != capability.Digest || d.InputSchema != capability.InputSchema || d.OutputSchema != capability.OutputSchema {
		return conflict("tool binds a different capability or schema")
	}
	for _, effect := range capability.EffectKinds {
		if !ContainsName(d.EffectKinds, effect) {
			return conflict("tool weakens capability effect requirements")
		}
	}
	return nil
}

func (d ToolDescriptor) ComputeDigest() (core.Digest, error) {
	if err := d.validateShape(); err != nil {
		return "", err
	}
	d.Digest = ""
	return Seal("praxis.tool-mcp.tool", ToolContractVersion, "ToolDescriptor", d)
}

func SealTool(d ToolDescriptor) (ToolDescriptor, error) {
	d.ContractVersion = ToolContractVersion
	d.EffectKinds = SortedUniqueNames(d.EffectKinds)
	d.Digest = ""
	digest, err := d.ComputeDigest()
	if err != nil {
		return ToolDescriptor{}, err
	}
	d.Digest = digest
	return d, nil
}

func NewTimestamp() int64 { return time.Now().UTC().UnixNano() }
