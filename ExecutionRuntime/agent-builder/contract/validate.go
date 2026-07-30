package contract

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func invalid(reason core.ReasonCode, message string) error {
	return core.NewError(core.ErrorInvalidArgument, reason, message)
}

func (r AgentPackageRefV1) Validate() error {
	if strings.TrimSpace(r.PackageID) == "" || r.Revision == 0 || r.ContractVersion != ContractVersionV1 || r.SchemaVersion != SchemaVersionV1 {
		return invalid(core.ReasonInvalidReference, "agent package ref identity and schema are required")
	}
	return r.Digest.Validate()
}

func (m AgentPackageLockManifestV1) Validate() error {
	if m.ContractVersion != ContractVersionV1 || m.SchemaVersion != SchemaVersionV1 || m.ObjectKind != LockObjectKindV1 {
		return invalid(core.ReasonInvalidState, "agent package lock discriminator is invalid")
	}
	if m.DefinitionRef.Validate() != nil || m.ResolvedPlanRef.Validate() != nil || m.ResolutionFactsRef.Validate() != nil || m.CatalogRef.Validate() != nil {
		return invalid(core.ReasonInvalidReference, "agent package lock requires exact upstream refs")
	}
	if len(m.ComponentReleaseRefs) == 0 || len(m.ComponentReleaseRefs) > MaxLockedComponentReleasesV1 {
		return invalid(core.ReasonComponentMissing, "agent package lock component release set is empty or too large")
	}
	seen := map[string]struct{}{}
	for _, ref := range m.ComponentReleaseRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if _, ok := seen[ref.ReleaseID]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "agent package lock repeats a component release")
		}
		seen[ref.ReleaseID] = struct{}{}
	}
	for _, digest := range []core.Digest{m.BindingPlanDigest, m.AssemblyInputDigest, m.Digest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if m.FrozenUnixNano <= 0 {
		return invalid(core.ReasonInvalidState, "agent package lock frozen time is required")
	}
	if m.HarnessCompilerVersion != assemblycontract.CompilerVersionV1 {
		return invalid(core.ReasonInvalidState, "agent package lock compiler version is unsupported")
	}
	if err := m.PublicationRef.Validate(); err != nil {
		return err
	}
	if err := m.GenerationRef.Validate(); err != nil {
		return err
	}
	for _, ref := range []assemblycontract.ObjectRefV1{m.ManifestRef, m.GraphRef, m.HandoffRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	publicationID, err := assemblycontract.DeriveAssemblyPublicationIDV2(m.AssemblyInputDigest, m.GenerationRef.ID)
	if err != nil {
		return err
	}
	if m.PublicationRef.PublicationID != publicationID || m.ManifestRef.ID != publicationID+"/manifest" || m.GraphRef.ID != publicationID+"/graph" || m.HandoffRef.ID != publicationID+"/handoff" || m.ManifestRef.Revision != 1 || m.GraphRef.Revision != 1 || m.HandoffRef.Revision != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "agent package artifact refs do not use the exact Harness publication coordinates")
	}
	want, err := LockDigestV1(m)
	if err != nil {
		return err
	}
	if want != m.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "agent package lock digest does not match canonical content")
	}
	return nil
}

func (p AgentPackageV1) Validate() error {
	if p.ContractVersion != ContractVersionV1 || p.SchemaVersion != SchemaVersionV1 || p.ObjectKind != PackageObjectKindV1 || !strings.HasPrefix(p.PackageID, "agent-package-") || p.Revision != 1 || p.CreatedUnixNano <= 0 {
		return invalid(core.ReasonInvalidState, "agent package identity, revision or frozen time is invalid")
	}
	if err := p.Lock.Validate(); err != nil {
		return err
	}
	if p.PackageID != packageIDV1(p.Lock.Digest) || p.CreatedUnixNano != p.Lock.FrozenUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "agent package identity or frozen time differs from its lock")
	}
	want, err := PackageDigestV1(p)
	if err != nil {
		return err
	}
	if want != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "agent package digest does not match canonical content")
	}
	return p.RefV1().Validate()
}
