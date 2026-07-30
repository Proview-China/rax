package contract

import (
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	VerifiedAgentPackageClosureContractVersionV1 = "praxis.agent.package-closure/v1"
	VerifiedAgentPackageClosureObjectKindV1      = "VerifiedAgentPackageClosureV1"
)

// VerifiedAgentPackageClosureV1 is a value proof that one exact AgentPackage and
// one committed Harness Publication form the same immutable compilation
// closure. It is not an activation, factory, runtime or production grant.
type VerifiedAgentPackageClosureV1 struct {
	ContractVersion string                                       `json:"contract_version"`
	SchemaVersion   string                                       `json:"schema_version"`
	ObjectKind      string                                       `json:"object_kind"`
	Package         AgentPackageV1                               `json:"package"`
	Publication     assemblycontract.AssemblyPublicationBundleV2 `json:"publication"`
	ClosureDigest   core.Digest                                  `json:"closure_digest"`
}

func VerifiedAgentPackageClosureDigestV1(value VerifiedAgentPackageClosureV1) (core.Digest, error) {
	value = clone(value)
	value.ClosureDigest = ""
	return core.CanonicalJSONDigest(
		"praxis.agent.package-closure",
		VerifiedAgentPackageClosureContractVersionV1,
		VerifiedAgentPackageClosureObjectKindV1,
		value,
	)
}

func SealVerifiedAgentPackageClosureV1(pkg AgentPackageV1, publication assemblycontract.AssemblyPublicationBundleV2) (VerifiedAgentPackageClosureV1, error) {
	value := VerifiedAgentPackageClosureV1{
		ContractVersion: VerifiedAgentPackageClosureContractVersionV1,
		SchemaVersion:   SchemaVersionV1,
		ObjectKind:      VerifiedAgentPackageClosureObjectKindV1,
		Package:         clone(pkg),
		Publication:     clone(publication),
	}
	digest, err := VerifiedAgentPackageClosureDigestV1(value)
	if err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	value.ClosureDigest = digest
	if err = value.Validate(); err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	return value, nil
}

func (value VerifiedAgentPackageClosureV1) Validate() error {
	if value.ContractVersion != VerifiedAgentPackageClosureContractVersionV1 ||
		value.SchemaVersion != SchemaVersionV1 ||
		value.ObjectKind != VerifiedAgentPackageClosureObjectKindV1 {
		return invalid(core.ReasonInvalidState, "verified package closure discriminator is invalid")
	}
	if err := value.Package.Validate(); err != nil {
		return err
	}
	if err := value.Publication.Validate(); err != nil {
		return err
	}

	lock := value.Package.Lock
	publication := value.Publication
	publicationRef := assemblycontract.AssemblyPublicationRefV2{
		PublicationID: publication.Publication.PublicationID,
		Revision:      publication.Publication.Revision,
		Digest:        publication.Publication.Digest,
	}
	if publicationRef != lock.PublicationRef {
		return closureDriftV1("verified package closure Publication ref drifted")
	}
	if publication.Publication.Artifacts.Generation != lock.GenerationRef ||
		publication.Publication.Artifacts.Manifest != lock.ManifestRef ||
		publication.Publication.Artifacts.Graph != lock.GraphRef ||
		publication.Publication.Artifacts.Handoff != lock.HandoffRef {
		return closureDriftV1("verified package closure artifact refs drifted")
	}
	if publication.Publication.InputDigest != lock.AssemblyInputDigest ||
		publication.Generation.InputDigest != lock.AssemblyInputDigest ||
		publication.Manifest.InputDigest != lock.AssemblyInputDigest ||
		publication.Graph.InputDigest != lock.AssemblyInputDigest {
		return closureDriftV1("verified package closure Assembly Input digest drifted")
	}
	if publication.Generation.CompilerVersion != lock.HarnessCompilerVersion ||
		publication.Generation.CreatedUnixNano != lock.FrozenUnixNano {
		return closureDriftV1("verified package closure compiler or frozen time drifted")
	}
	if publication.Handoff.GenerationRef != lock.GenerationRef ||
		publication.Handoff.ManifestDigest != lock.ManifestRef.Digest ||
		publication.Handoff.GraphDigest != lock.GraphRef.Digest {
		return closureDriftV1("verified package closure Handoff refs drifted")
	}
	want, err := VerifiedAgentPackageClosureDigestV1(value)
	if err != nil {
		return err
	}
	if want != value.ClosureDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "verified package closure digest drifted")
	}
	return nil
}

func (value VerifiedAgentPackageClosureV1) PublicationRefV2() assemblycontract.AssemblyPublicationRefV2 {
	return assemblycontract.AssemblyPublicationRefV2{
		PublicationID: value.Publication.Publication.PublicationID,
		Revision:      value.Publication.Publication.Revision,
		Digest:        value.Publication.Publication.Digest,
	}
}

func CloneVerifiedAgentPackageClosureV1(value VerifiedAgentPackageClosureV1) VerifiedAgentPackageClosureV1 {
	return clone(value)
}

func closureDriftV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, message)
}
