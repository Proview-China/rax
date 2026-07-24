package contract

import (
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type PackageDescriptorRef struct {
	ToolID   runtimeports.NamespacedNameV2 `json:"tool_id"`
	Revision core.Revision                 `json:"revision"`
	Digest   core.Digest                   `json:"digest"`
}

type ToolPackageManifest struct {
	ContractVersion    string                          `json:"contract_version"`
	ID                 runtimeports.NamespacedNameV2   `json:"id"`
	SemanticVersion    string                          `json:"semantic_version"`
	Revision           core.Revision                   `json:"revision"`
	Digest             core.Digest                     `json:"digest"`
	Publisher          core.OwnerRef                   `json:"publisher"`
	ArtifactDigest     core.Digest                     `json:"artifact_digest"`
	Signatures         []core.Digest                   `json:"signatures"`
	Descriptors        []PackageDescriptorRef          `json:"descriptors"`
	EffectKinds        []runtimeports.NamespacedNameV2 `json:"effect_kinds"`
	ReviewRequirement  runtimeports.NamespacedNameV2   `json:"review_requirement"`
	SandboxRequirement runtimeports.NamespacedNameV2   `json:"sandbox_requirement"`
	ProvenanceDigest   core.Digest                     `json:"provenance_digest"`
	CreatedUnixNano    int64                           `json:"created_unix_nano"`
}

func (m ToolPackageManifest) validateShape() error {
	if m.ContractVersion != PackageContractVersion || m.Revision == 0 || m.CreatedUnixNano <= 0 || runtimeports.ValidateNamespacedNameV2(m.ID) != nil || m.Publisher.Validate() != nil {
		return invalid("package identity, version, owner or time is invalid")
	}
	version, err := core.ParseSemanticVersion(m.SemanticVersion)
	if err != nil || version.String() != m.SemanticVersion || len(version.Build) != 0 {
		return invalid("package version must be canonical SemVer")
	}
	if m.ArtifactDigest.Validate() != nil || m.ProvenanceDigest.Validate() != nil || len(m.Signatures) == 0 || len(m.Signatures) > 16 || len(m.Descriptors) == 0 || len(m.Descriptors) > MaxPackageTools {
		return invalid("package artifacts, signatures or descriptors are incomplete")
	}
	for i, signature := range m.Signatures {
		if signature.Validate() != nil {
			return invalid("package signature digest is invalid")
		}
		if i > 0 && m.Signatures[i-1] >= signature {
			return invalid("package signature digests must be sorted and unique")
		}
	}
	for i, descriptor := range m.Descriptors {
		if runtimeports.ValidateNamespacedNameV2(descriptor.ToolID) != nil || descriptor.Revision == 0 || descriptor.Digest.Validate() != nil {
			return invalid("package descriptor reference is invalid")
		}
		if i > 0 && m.Descriptors[i-1].ToolID >= descriptor.ToolID {
			return invalid("package descriptors must be sorted and unique")
		}
	}
	if ValidateSortedUniqueNames(m.EffectKinds, MaxDescriptorEffects) != nil || runtimeports.ValidateNamespacedNameV2(m.ReviewRequirement) != nil || runtimeports.ValidateNamespacedNameV2(m.SandboxRequirement) != nil {
		return invalid("package governance requirements are invalid")
	}
	return nil
}

func (m ToolPackageManifest) Validate() error {
	if err := m.validateShape(); err != nil {
		return err
	}
	if err := m.Digest.Validate(); err != nil {
		return err
	}
	expected, err := m.ComputeDigest()
	if err != nil || expected != m.Digest {
		return conflict("package digest does not bind exact content")
	}
	return nil
}

func (m ToolPackageManifest) ComputeDigest() (core.Digest, error) {
	if err := m.validateShape(); err != nil {
		return "", err
	}
	m.Digest = ""
	return Seal("praxis.tool-mcp.package", PackageContractVersion, "ToolPackageManifest", m)
}

func SealPackage(m ToolPackageManifest) (ToolPackageManifest, error) {
	m.ContractVersion = PackageContractVersion
	m.EffectKinds = SortedUniqueNames(m.EffectKinds)
	sort.Slice(m.Descriptors, func(i, j int) bool { return m.Descriptors[i].ToolID < m.Descriptors[j].ToolID })
	sort.Slice(m.Signatures, func(i, j int) bool { return m.Signatures[i] < m.Signatures[j] })
	m.Digest = ""
	digest, err := m.ComputeDigest()
	if err != nil {
		return ToolPackageManifest{}, err
	}
	m.Digest = digest
	return m, nil
}
