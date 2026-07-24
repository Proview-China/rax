// Package release publishes Memory and Knowledge assembly candidates without
// treating reference stores or owner-local fixtures as production backends.
package release

import (
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"time"
)

const (
	LocalVersionV1                                      = "praxis.memory-knowledge/component-local-readiness/v1"
	ProductionVersionV1                                 = "praxis.memory-knowledge/component-production-readiness/v1"
	ComponentIDV1         runtimeports.ComponentIDV2    = "praxis/memory-knowledge"
	ComponentKindV1       runtimeports.ComponentKindV2  = "praxis/memory-knowledge"
	MemoryCapabilityV1    runtimeports.CapabilityNameV2 = "praxis.memory/owner-v1"
	KnowledgeCapabilityV1 runtimeports.CapabilityNameV2 = "praxis.knowledge/owner-v1"
)

type LocalReadinessProjectionV1 struct {
	ContractVersion           string                       `json:"contract_version"`
	ReleaseID                 string                       `json:"release_id"`
	Revision                  core.Revision                `json:"revision"`
	ArtifactDigest            core.Digest                  `json:"artifact_digest"`
	MemoryOwnerRef            assemblycontract.ObjectRefV1 `json:"memory_owner_ref"`
	KnowledgeOwnerRef         assemblycontract.ObjectRefV1 `json:"knowledge_owner_ref"`
	RetrievalRef              assemblycontract.ObjectRefV1 `json:"retrieval_ref"`
	MemoryContextSourceRef    assemblycontract.ObjectRefV1 `json:"memory_context_source_ref"`
	KnowledgeContextSourceRef assemblycontract.ObjectRefV1 `json:"knowledge_context_source_ref"`
	PurgeInspectRef           assemblycontract.ObjectRefV1 `json:"purge_inspect_ref"`
	CheckedUnixNano           int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano           int64                        `json:"expires_unix_nano"`
	Digest                    core.Digest                  `json:"digest"`
}

func (p LocalReadinessProjectionV1) refs() []assemblycontract.ObjectRefV1 {
	return []assemblycontract.ObjectRefV1{p.MemoryOwnerRef, p.KnowledgeOwnerRef, p.RetrievalRef, p.MemoryContextSourceRef, p.KnowledgeContextSourceRef, p.PurgeInspectRef}
}
func LocalDigestV1(p LocalReadinessProjectionV1) (core.Digest, error) {
	p.ContractVersion = LocalVersionV1
	p.Digest = ""
	return core.CanonicalJSONDigest("praxis.memory-knowledge.component-local-readiness", LocalVersionV1, "LocalReadinessProjectionV1", p)
}
func SealLocalV1(p LocalReadinessProjectionV1) (LocalReadinessProjectionV1, error) {
	p.ContractVersion = LocalVersionV1
	p.Digest = runtimeports.EvidenceGenesisDigestV2
	d, e := LocalDigestV1(p)
	if e != nil {
		return LocalReadinessProjectionV1{}, e
	}
	p.Digest = d
	return p, p.ValidateCurrent(time.Unix(0, p.CheckedUnixNano))
}
func (p LocalReadinessProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != LocalVersionV1 || p.ReleaseID == "" || p.Revision == 0 || p.ArtifactDigest.Validate() != nil {
		return invalid("local readiness identity is incomplete")
	}
	if e := window(p.CheckedUnixNano, p.ExpiresUnixNano, now); e != nil {
		return e
	}
	if e := distinct(p.refs()); e != nil {
		return e
	}
	d, e := LocalDigestV1(p)
	if e != nil || d != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "local readiness digest drifted")
	}
	return nil
}

type ProductionReadinessProjectionV1 struct {
	ContractVersion                 string                       `json:"contract_version"`
	ReleaseID                       string                       `json:"release_id"`
	Revision                        core.Revision                `json:"revision"`
	ArtifactDigest                  core.Digest                  `json:"artifact_digest"`
	ManifestDigest                  core.Digest                  `json:"manifest_digest"`
	DurableMemoryFactStoreRef       assemblycontract.ObjectRefV1 `json:"durable_memory_fact_store_ref"`
	DurableMemoryContentStoreRef    assemblycontract.ObjectRefV1 `json:"durable_memory_content_store_ref"`
	DurableKnowledgeFactStoreRef    assemblycontract.ObjectRefV1 `json:"durable_knowledge_fact_store_ref"`
	DurableKnowledgeContentStoreRef assemblycontract.ObjectRefV1 `json:"durable_knowledge_content_store_ref"`
	AuthorityPolicyCurrentRef       assemblycontract.ObjectRefV1 `json:"authority_policy_current_ref"`
	CredentialCurrentRef            assemblycontract.ObjectRefV1 `json:"credential_current_ref"`
	RetrievalIndexCurrentRef        assemblycontract.ObjectRefV1 `json:"retrieval_index_current_ref"`
	ContextSourceCurrentRef         assemblycontract.ObjectRefV1 `json:"context_source_current_ref"`
	SettlementCurrentRef            assemblycontract.ObjectRefV1 `json:"settlement_current_ref"`
	PurgeEffectRef                  assemblycontract.ObjectRefV1 `json:"purge_effect_ref"`
	CleanupOwnerRef                 assemblycontract.ObjectRefV1 `json:"cleanup_owner_ref"`
	DeploymentAttestationRef        assemblycontract.ObjectRefV1 `json:"deployment_attestation_ref"`
	CertificationFactRef            assemblycontract.ObjectRefV1 `json:"certification_fact_ref"`
	CheckedUnixNano                 int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano                 int64                        `json:"expires_unix_nano"`
	Digest                          core.Digest                  `json:"digest"`
}

func (p ProductionReadinessProjectionV1) refs() []assemblycontract.ObjectRefV1 {
	return []assemblycontract.ObjectRefV1{p.DurableMemoryFactStoreRef, p.DurableMemoryContentStoreRef, p.DurableKnowledgeFactStoreRef, p.DurableKnowledgeContentStoreRef, p.AuthorityPolicyCurrentRef, p.CredentialCurrentRef, p.RetrievalIndexCurrentRef, p.ContextSourceCurrentRef, p.SettlementCurrentRef, p.PurgeEffectRef, p.CleanupOwnerRef, p.DeploymentAttestationRef, p.CertificationFactRef}
}
func (p ProductionReadinessProjectionV1) evidence() []assemblycontract.ObjectRefV1 {
	return p.refs()[:len(p.refs())-1]
}
func ProductionDigestV1(p ProductionReadinessProjectionV1) (core.Digest, error) {
	p.ContractVersion = ProductionVersionV1
	p.Digest = ""
	return core.CanonicalJSONDigest("praxis.memory-knowledge.component-production-readiness", ProductionVersionV1, "ProductionReadinessProjectionV1", p)
}
func SealProductionV1(p ProductionReadinessProjectionV1) (ProductionReadinessProjectionV1, error) {
	p.ContractVersion = ProductionVersionV1
	p.Digest = runtimeports.EvidenceGenesisDigestV2
	d, e := ProductionDigestV1(p)
	if e != nil {
		return ProductionReadinessProjectionV1{}, e
	}
	p.Digest = d
	return p, p.ValidateCurrent(time.Unix(0, p.CheckedUnixNano))
}
func (p ProductionReadinessProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != ProductionVersionV1 || p.ReleaseID == "" || p.Revision == 0 || p.ArtifactDigest.Validate() != nil || p.ManifestDigest.Validate() != nil {
		return invalid("production readiness identity is incomplete")
	}
	if e := window(p.CheckedUnixNano, p.ExpiresUnixNano, now); e != nil {
		return e
	}
	if e := distinct(p.refs()); e != nil {
		return e
	}
	d, e := ProductionDigestV1(p)
	if e != nil || d != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "production readiness digest drifted")
	}
	return nil
}

type PublicationRequestV1 struct {
	ReleaseID       string
	Revision        core.Revision
	SourceRef       assemblycontract.ObjectRefV1
	PublisherRef    assemblycontract.ObjectRefV1
	TrustRef        assemblycontract.ObjectRefV1
	CertificationID string
	ArtifactDigest  core.Digest
	CreatedUnixNano int64
	ExpiresUnixNano int64
}

func (r PublicationRequestV1) Validate(now time.Time) error {
	if r.ReleaseID == "" || r.Revision == 0 || r.CertificationID == "" || r.SourceRef.Validate() != nil || r.PublisherRef.Validate() != nil || r.TrustRef.Validate() != nil || r.ArtifactDigest.Validate() != nil {
		return invalid("publication request is incomplete")
	}
	return window(r.CreatedUnixNano, r.ExpiresUnixNano, now)
}

type PublicationResultV1 struct {
	Release         assemblercontract.ComponentReleaseV1
	LocalReady      bool
	ProductionReady bool
	Local           *LocalReadinessProjectionV1
	Production      *ProductionReadinessProjectionV1
}

func window(c, e int64, n time.Time) error {
	if n.IsZero() || c <= 0 || e <= c || n.UnixNano() < c {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "readiness clock is invalid")
	}
	if !n.Before(time.Unix(0, e)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "readiness expired")
	}
	return nil
}
func distinct(rs []assemblycontract.ObjectRefV1) error {
	seen := map[string]struct{}{}
	for _, r := range rs {
		if e := r.Validate(); e != nil {
			return e
		}
		if _, ok := seen[r.ID]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "readiness roles alias")
		}
		seen[r.ID] = struct{}{}
	}
	return nil
}
func invalid(m string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, m)
}
