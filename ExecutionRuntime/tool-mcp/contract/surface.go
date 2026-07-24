package contract

import (
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type SurfaceVisibility string

const (
	SurfaceVisible SurfaceVisibility = "visible"
	SurfaceHidden  SurfaceVisibility = "hidden"
)

type AdmissionClass string

const (
	AdmissionRequired    AdmissionClass = "required"
	AdmissionPreApproved AdmissionClass = "pre_approved"
)

type ToolSurfaceEntry struct {
	Capability        ObjectRef                       `json:"capability"`
	Tool              ObjectRef                       `json:"tool"`
	ModelName         string                          `json:"model_name"`
	InputSchema       runtimeports.SchemaRefV2        `json:"input_schema"`
	DescriptionDigest core.Digest                     `json:"description_digest"`
	Order             uint32                          `json:"order"`
	Visibility        SurfaceVisibility               `json:"visibility"`
	Allowed           bool                            `json:"allowed"`
	Admission         AdmissionClass                  `json:"admission"`
	MechanismDigest   core.Digest                     `json:"mechanism_digest"`
	EffectKinds       []runtimeports.NamespacedNameV2 `json:"effect_kinds"`
}

func (e ToolSurfaceEntry) Validate() error {
	if e.Capability.Validate() != nil || e.Tool.Validate() != nil || e.InputSchema.Validate() != nil || e.DescriptionDigest.Validate() != nil || e.MechanismDigest.Validate() != nil {
		return invalid("surface entry references or digests are invalid")
	}
	if strings.TrimSpace(e.ModelName) == "" || len(e.ModelName) > 128 {
		return invalid("surface model name is blank or unbounded")
	}
	if e.Visibility != SurfaceVisible && e.Visibility != SurfaceHidden {
		return invalid("surface visibility is invalid")
	}
	if e.Admission != AdmissionRequired && e.Admission != AdmissionPreApproved {
		return invalid("surface admission class is invalid")
	}
	if e.Admission == AdmissionPreApproved && (!e.Allowed || e.Visibility != SurfaceVisible) {
		return invalid("pre-approved surface entry must also be visible and allowed")
	}
	if e.Allowed && e.Visibility != SurfaceVisible {
		return invalid("allowed surface entry must be visible")
	}
	return ValidateSortedUniqueNames(e.EffectKinds, MaxDescriptorEffects)
}

type ToolSurfaceManifest struct {
	ContractVersion         string                        `json:"contract_version"`
	ID                      string                        `json:"id"`
	Revision                core.Revision                 `json:"revision"`
	Digest                  core.Digest                   `json:"digest"`
	Owner                   core.OwnerRef                 `json:"owner"`
	ResolvedPlanDigest      core.Digest                   `json:"resolved_plan_digest"`
	ProfileDigest           core.Digest                   `json:"profile_digest"`
	CapabilityGrantDigest   core.Digest                   `json:"capability_grant_digest"`
	RegistrySnapshotDigest  core.Digest                   `json:"registry_snapshot_digest"`
	Entries                 []ToolSurfaceEntry            `json:"ordered_entries"`
	Dialect                 runtimeports.NamespacedNameV2 `json:"dialect"`
	ExpectedInjectionDigest core.Digest                   `json:"expected_injection_digest"`
	Residuals               []Residual                    `json:"residuals,omitempty"`
	CreatedUnixNano         int64                         `json:"created_unix_nano"`
	ExpiresUnixNano         int64                         `json:"expires_unix_nano"`
}

func (m ToolSurfaceManifest) validateShape() error {
	if m.ContractVersion != SurfaceContractVersion || ValidateStableID(m.ID) != nil || m.Revision == 0 || m.Owner.Validate() != nil || m.CreatedUnixNano <= 0 || m.ExpiresUnixNano <= m.CreatedUnixNano {
		return invalid("surface identity, owner, revision or lifetime is invalid")
	}
	for _, digest := range []core.Digest{m.ResolvedPlanDigest, m.ProfileDigest, m.CapabilityGrantDigest, m.RegistrySnapshotDigest, m.ExpectedInjectionDigest} {
		if digest.Validate() != nil {
			return invalid("surface input or injection digest is invalid")
		}
	}
	if runtimeports.ValidateNamespacedNameV2(m.Dialect) != nil || len(m.Entries) == 0 || len(m.Entries) > MaxSurfaceEntries || len(m.Residuals) > MaxResiduals {
		return invalid("surface dialect or entry count is invalid")
	}
	seenModel := make(map[string]struct{}, len(m.Entries))
	for i, entry := range m.Entries {
		if err := entry.Validate(); err != nil || entry.Order != uint32(i) {
			return invalid("surface entries must be valid and consecutively ordered")
		}
		if _, exists := seenModel[entry.ModelName]; exists {
			return conflict("surface model names must be unique")
		}
		seenModel[entry.ModelName] = struct{}{}
		if i > 0 && surfaceEntryLess(entry, m.Entries[i-1]) {
			return invalid("surface entries are not in canonical order")
		}
	}
	for _, residual := range m.Residuals {
		if err := residual.Validate(); err != nil {
			return err
		}
	}
	expected, err := ComputeExpectedInjectionDigest(m.Entries)
	if err != nil || expected != m.ExpectedInjectionDigest {
		return conflict("surface expected injection digest drifted")
	}
	return nil
}

func (m ToolSurfaceManifest) Validate() error {
	if err := m.validateShape(); err != nil {
		return err
	}
	if err := m.Digest.Validate(); err != nil {
		return err
	}
	expected, err := m.ComputeDigest()
	if err != nil || expected != m.Digest {
		return conflict("surface digest does not bind exact content")
	}
	return nil
}

func (m ToolSurfaceManifest) ComputeDigest() (core.Digest, error) {
	if err := m.validateShape(); err != nil {
		return "", err
	}
	m.Digest = ""
	return Seal("praxis.tool-mcp.surface", SurfaceContractVersion, "ToolSurfaceManifest", m)
}

func SealSurface(m ToolSurfaceManifest) (ToolSurfaceManifest, error) {
	m.ContractVersion = SurfaceContractVersion
	sort.Slice(m.Entries, func(i, j int) bool { return surfaceEntryLess(m.Entries[i], m.Entries[j]) })
	for i := range m.Entries {
		m.Entries[i].Order = uint32(i)
		m.Entries[i].EffectKinds = SortedUniqueNames(m.Entries[i].EffectKinds)
	}
	expected, err := ComputeExpectedInjectionDigest(m.Entries)
	if err != nil {
		return ToolSurfaceManifest{}, err
	}
	m.ExpectedInjectionDigest = expected
	m.Digest = ""
	digest, err := m.ComputeDigest()
	if err != nil {
		return ToolSurfaceManifest{}, err
	}
	m.Digest = digest
	return m, nil
}

func ComputeExpectedInjectionDigest(entries []ToolSurfaceEntry) (core.Digest, error) {
	type injectionEntry struct {
		ModelName   string                          `json:"model_name"`
		InputSchema runtimeports.SchemaRefV2        `json:"input_schema"`
		Description core.Digest                     `json:"description_digest"`
		Effects     []runtimeports.NamespacedNameV2 `json:"effect_kinds"`
	}
	values := make([]injectionEntry, 0, len(entries))
	for _, entry := range entries {
		values = append(values, injectionEntry{entry.ModelName, entry.InputSchema, entry.DescriptionDigest, entry.EffectKinds})
	}
	return Seal("praxis.tool-mcp.surface", SurfaceContractVersion, "ExpectedInjection", values)
}

func surfaceEntryLess(left, right ToolSurfaceEntry) bool {
	if left.ModelName != right.ModelName {
		return left.ModelName < right.ModelName
	}
	if left.Capability.ID != right.Capability.ID {
		return left.Capability.ID < right.Capability.ID
	}
	return left.Tool.ID < right.Tool.ID
}
