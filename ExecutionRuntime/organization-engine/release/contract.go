// Package release publishes Organization component release candidates without
// upgrading an in-memory store, a SQLite file, or Review consumer state into a
// production claim.
package release

import (
	"strings"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	LocalReadinessContractVersionV1      = "praxis.organization/component-local-readiness/v1"
	ProductionReadinessContractVersionV1 = "praxis.organization/component-production-readiness/v1"
	ReleaseSemanticVersionV1             = "1.0.0"
	SQLiteBackendKindV1                  = "sqlite"

	ComponentIDV1   runtimeports.ComponentIDV2    = "components/organization"
	ComponentKindV1 runtimeports.ComponentKindV2  = "praxis/organization"
	CapabilityV1    runtimeports.CapabilityNameV2 = "praxis.organization/review-eligibility-current"
	ContractNameV1  runtimeports.NamespacedNameV2 = "praxis.organization/review-current"
	PortIDV1                                      = "praxis.organization/port/review-eligibility-current"
	ModuleIDV1                                    = "praxis.organization/module/review-current"
	FactoryIDV1                                   = "praxis.organization/factory/review-current"

	MaximumReadinessTTL = 24 * time.Hour
)

type LocalReadinessProjectionV1 struct {
	ContractVersion      string                       `json:"contract_version"`
	ComponentID          runtimeports.ComponentIDV2   `json:"component_id"`
	ReleaseID            string                       `json:"release_id"`
	Revision             core.Revision                `json:"revision"`
	ArtifactDigest       core.Digest                  `json:"artifact_digest"`
	BackendKind          string                       `json:"backend_kind"`
	SQLiteResourceRef    assemblycontract.ObjectRefV1 `json:"sqlite_resource_ref"`
	SchemaEvidenceRef    assemblycontract.ObjectRefV1 `json:"schema_evidence_ref"`
	IntegrityEvidenceRef assemblycontract.ObjectRefV1 `json:"integrity_evidence_ref"`
	RestartEvidenceRef   assemblycontract.ObjectRefV1 `json:"restart_evidence_ref"`
	HistoricalReaderRef  assemblycontract.ObjectRefV1 `json:"historical_reader_ref"`
	CurrentReaderRef     assemblycontract.ObjectRefV1 `json:"current_reader_ref"`
	CheckedUnixNano      int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                        `json:"expires_unix_nano"`
	Digest               core.Digest                  `json:"digest"`
}

func (p LocalReadinessProjectionV1) refs() []assemblycontract.ObjectRefV1 {
	return []assemblycontract.ObjectRefV1{p.SQLiteResourceRef, p.SchemaEvidenceRef, p.IntegrityEvidenceRef, p.RestartEvidenceRef, p.HistoricalReaderRef, p.CurrentReaderRef}
}

func LocalReadinessDigestV1(p LocalReadinessProjectionV1) (core.Digest, error) {
	p.ContractVersion = LocalReadinessContractVersionV1
	p.Digest = ""
	return core.CanonicalJSONDigest("praxis.organization.component-local-readiness", LocalReadinessContractVersionV1, "LocalReadinessProjectionV1", p)
}

func SealLocalReadinessV1(p LocalReadinessProjectionV1) (LocalReadinessProjectionV1, error) {
	p.ContractVersion = LocalReadinessContractVersionV1
	p.Digest = runtimeports.EvidenceGenesisDigestV2
	d, err := LocalReadinessDigestV1(p)
	if err != nil {
		return LocalReadinessProjectionV1{}, err
	}
	p.Digest = d
	return p, p.ValidateCurrent(time.Unix(0, p.CheckedUnixNano))
}

func (p LocalReadinessProjectionV1) ExactRefV1() assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: p.ReleaseID + "/sqlite-local-readiness", Revision: p.Revision, Digest: p.Digest}
}

func (p LocalReadinessProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != LocalReadinessContractVersionV1 || p.ComponentID != ComponentIDV1 || strings.TrimSpace(p.ReleaseID) == "" || p.Revision == 0 || p.BackendKind != SQLiteBackendKindV1 {
		return invalid("Organization local readiness identity or backend kind is invalid")
	}
	if err := p.ArtifactDigest.Validate(); err != nil {
		return err
	}
	if err := validateWindow(p.CheckedUnixNano, p.ExpiresUnixNano, now); err != nil {
		return err
	}
	if err := validateDistinctRefs(p.refs()); err != nil {
		return err
	}
	d, err := LocalReadinessDigestV1(p)
	if err != nil || d != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Organization local readiness digest drifted")
	}
	return p.ExactRefV1().Validate()
}

type ProductionReadinessProjectionV1 struct {
	ContractVersion             string                       `json:"contract_version"`
	ComponentID                 runtimeports.ComponentIDV2   `json:"component_id"`
	ReleaseID                   string                       `json:"release_id"`
	Revision                    core.Revision                `json:"revision"`
	ArtifactDigest              core.Digest                  `json:"artifact_digest"`
	ManifestDigest              core.Digest                  `json:"manifest_digest"`
	LocalReadinessRef           assemblycontract.ObjectRefV1 `json:"local_readiness_ref"`
	ResourceBindingSetRef       assemblycontract.ObjectRefV1 `json:"resource_binding_set_ref"`
	CleanupCurrentRef           assemblycontract.ObjectRefV1 `json:"cleanup_current_ref"`
	DeploymentAttestationRef    assemblycontract.ObjectRefV1 `json:"deployment_attestation_ref"`
	ExecutableFactoryBindingRef assemblycontract.ObjectRefV1 `json:"executable_factory_binding_ref"`
	CertificationFactRef        assemblycontract.ObjectRefV1 `json:"certification_fact_ref"`
	CheckedUnixNano             int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano             int64                        `json:"expires_unix_nano"`
	Digest                      core.Digest                  `json:"digest"`
}

func (p ProductionReadinessProjectionV1) refs() []assemblycontract.ObjectRefV1 {
	return []assemblycontract.ObjectRefV1{p.LocalReadinessRef, p.ResourceBindingSetRef, p.CleanupCurrentRef, p.DeploymentAttestationRef, p.ExecutableFactoryBindingRef, p.CertificationFactRef}
}

func (p ProductionReadinessProjectionV1) evidenceRefs() []assemblycontract.ObjectRefV1 {
	refs := p.refs()
	// LocalReadinessRef is already emitted by the local closure and the final
	// CertificationFactRef is carried by ComponentRelease.CertificationRef.
	return refs[1 : len(refs)-1]
}

func ProductionReadinessDigestV1(p ProductionReadinessProjectionV1) (core.Digest, error) {
	p.ContractVersion = ProductionReadinessContractVersionV1
	p.Digest = ""
	return core.CanonicalJSONDigest("praxis.organization.component-production-readiness", ProductionReadinessContractVersionV1, "ProductionReadinessProjectionV1", p)
}

func SealProductionReadinessV1(p ProductionReadinessProjectionV1) (ProductionReadinessProjectionV1, error) {
	p.ContractVersion = ProductionReadinessContractVersionV1
	p.Digest = runtimeports.EvidenceGenesisDigestV2
	d, err := ProductionReadinessDigestV1(p)
	if err != nil {
		return ProductionReadinessProjectionV1{}, err
	}
	p.Digest = d
	return p, p.ValidateCurrent(time.Unix(0, p.CheckedUnixNano))
}

func (p ProductionReadinessProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != ProductionReadinessContractVersionV1 || p.ComponentID != ComponentIDV1 || strings.TrimSpace(p.ReleaseID) == "" || p.Revision == 0 {
		return invalid("Organization production readiness identity is invalid")
	}
	if err := p.ArtifactDigest.Validate(); err != nil {
		return err
	}
	if err := p.ManifestDigest.Validate(); err != nil {
		return err
	}
	if err := validateWindow(p.CheckedUnixNano, p.ExpiresUnixNano, now); err != nil {
		return err
	}
	if err := validateDistinctRefs(p.refs()); err != nil {
		return err
	}
	d, err := ProductionReadinessDigestV1(p)
	if err != nil || d != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Organization production readiness digest drifted")
	}
	return nil
}

type PublicationRequestV1 struct {
	ReleaseID       string                       `json:"release_id"`
	Revision        core.Revision                `json:"revision"`
	SourceRef       assemblycontract.ObjectRefV1 `json:"source_ref"`
	PublisherRef    assemblycontract.ObjectRefV1 `json:"publisher_ref"`
	TrustRef        assemblycontract.ObjectRefV1 `json:"trust_ref"`
	CertificationID string                       `json:"certification_id"`
	ArtifactDigest  core.Digest                  `json:"artifact_digest"`
	CreatedUnixNano int64                        `json:"created_unix_nano"`
	ExpiresUnixNano int64                        `json:"expires_unix_nano"`
}

func (r PublicationRequestV1) Validate(now time.Time) error {
	if strings.TrimSpace(r.ReleaseID) == "" || r.Revision == 0 || strings.TrimSpace(r.CertificationID) == "" {
		return invalid("Organization publication request identity is incomplete")
	}
	if err := r.ArtifactDigest.Validate(); err != nil {
		return err
	}
	for _, ref := range []assemblycontract.ObjectRefV1{r.SourceRef, r.PublisherRef, r.TrustRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return validateWindow(r.CreatedUnixNano, r.ExpiresUnixNano, now)
}

type PublicationResultV1 struct {
	Release             assemblercontract.ComponentReleaseV1
	LocalReady          bool
	ProductionReady     bool
	LocalReadiness      *LocalReadinessProjectionV1
	ProductionReadiness *ProductionReadinessProjectionV1
}

func validateWindow(checked, expires int64, now time.Time) error {
	if now.IsZero() || checked <= 0 || expires <= checked || now.UnixNano() < checked {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Organization readiness clock is invalid")
	}
	if expires-checked > int64(MaximumReadinessTTL) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "Organization readiness TTL exceeds its bound")
	}
	if !now.Before(time.Unix(0, expires)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Organization readiness expired")
	}
	return nil
}

func validateDistinctRefs(refs []assemblycontract.ObjectRefV1) error {
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if _, ok := seen[ref.ID]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Organization readiness proof refs alias")
		}
		seen[ref.ID] = struct{}{}
	}
	return nil
}

func invalid(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}
