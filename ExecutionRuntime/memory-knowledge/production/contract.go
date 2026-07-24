// Package production verifies deployment-owned exact proofs without selecting
// a database, RPC transport, process topology, or SLA.
package production

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ContractVersionV2 = "praxis.memory-knowledge/production-proof-bundle/v2"
	ObjectKindV2      = "memory_knowledge_production_proof_bundle"
	digestDomainV2    = "praxis.memory-knowledge.production-proof-bundle"

	MemoryFactStoreKindV2       runtimeports.ResourceHandleKindV1 = "praxis.memory/fact-store"
	MemoryContentStoreKindV2    runtimeports.ResourceHandleKindV1 = "praxis.memory/content-store"
	KnowledgeFactStoreKindV2    runtimeports.ResourceHandleKindV1 = "praxis.knowledge/fact-store"
	KnowledgeContentStoreKindV2 runtimeports.ResourceHandleKindV1 = "praxis.knowledge/content-store"
)

type AvailabilityProfileV2 string

const (
	ProfileNonHAV2 AvailabilityProfileV2 = "non_ha"
	ProfileHAV2    AvailabilityProfileV2 = "ha"
)

// ProductionProofBundleV2 associates external Owner facts. None of the
// referenced Authority, Context, Runtime, deployment, or certification facts
// are created or interpreted by Memory/Knowledge.
type ProductionProofBundleV2 struct {
	ContractVersion string                `json:"contract_version"`
	ObjectKind      string                `json:"object_kind"`
	ReleaseID       string                `json:"release_id"`
	Revision        core.Revision         `json:"revision"`
	Profile         AvailabilityProfileV2 `json:"profile"`
	ArtifactDigest  core.Digest           `json:"artifact_digest"`
	ManifestDigest  core.Digest           `json:"manifest_digest"`

	ResourceBindingSetRef       runtimeports.ResourceBindingSetRefV1 `json:"resource_binding_set_ref"`
	MemoryFactStoreRef          runtimeports.ResourceHandleRefV1     `json:"memory_fact_store_ref"`
	MemoryContentStoreRef       runtimeports.ResourceHandleRefV1     `json:"memory_content_store_ref"`
	KnowledgeFactStoreRef       runtimeports.ResourceHandleRefV1     `json:"knowledge_fact_store_ref"`
	KnowledgeContentStoreRef    runtimeports.ResourceHandleRefV1     `json:"knowledge_content_store_ref"`
	AuthorityPolicyCurrentRef   runtimeports.OwnerCurrentRefV1       `json:"authority_policy_current_ref"`
	CredentialCurrentRef        runtimeports.OwnerCurrentRefV1       `json:"credential_current_ref"`
	RetrievalIndexCurrentRef    runtimeports.OwnerCurrentRefV1       `json:"retrieval_index_current_ref"`
	ContextSourceCurrentRef     runtimeports.OwnerCurrentRefV1       `json:"context_source_current_ref"`
	SettlementCurrentRef        runtimeports.OwnerCurrentRefV1       `json:"settlement_current_ref"`
	PurgeEffectCurrentRef       runtimeports.OwnerCurrentRefV1       `json:"purge_effect_current_ref"`
	CleanupOwnerCurrentRef      runtimeports.OwnerCurrentRefV1       `json:"cleanup_owner_current_ref"`
	DeploymentAttestationRef    runtimeports.OwnerCurrentRefV1       `json:"deployment_attestation_ref"`
	CertificationFactRef        runtimeports.OwnerCurrentRefV1       `json:"certification_fact_ref"`
	SingleWriterFenceCurrentRef runtimeports.OwnerCurrentRefV1       `json:"single_writer_fence_current_ref"`
	RecoveryProofRef            runtimeports.OwnerCurrentRefV1       `json:"recovery_proof_ref"`
	BackupRestoreProofRef       runtimeports.OwnerCurrentRefV1       `json:"backup_restore_proof_ref"`

	ReplicaCount             uint32                          `json:"replica_count"`
	WriteQuorum              uint32                          `json:"write_quorum"`
	ReadQuorum               uint32                          `json:"read_quorum"`
	ReplicationCurrentRef    *runtimeports.OwnerCurrentRefV1 `json:"replication_current_ref,omitempty"`
	QuorumCurrentRef         *runtimeports.OwnerCurrentRefV1 `json:"quorum_current_ref,omitempty"`
	FailoverProofRef         *runtimeports.OwnerCurrentRefV1 `json:"failover_proof_ref,omitempty"`
	MonotonicCurrentProofRef *runtimeports.OwnerCurrentRefV1 `json:"monotonic_current_proof_ref,omitempty"`

	CheckedUnixNano int64       `json:"checked_unix_nano"`
	ExpiresUnixNano int64       `json:"expires_unix_nano"`
	Digest          core.Digest `json:"digest"`
}

func SealProductionProofBundleV2(value ProductionProofBundleV2) (ProductionProofBundleV2, error) {
	if value.ContractVersion != "" && value.ContractVersion != ContractVersionV2 {
		return ProductionProofBundleV2{}, invalid("production proof contract version drifted")
	}
	if value.ObjectKind != "" && value.ObjectKind != ObjectKindV2 {
		return ProductionProofBundleV2{}, invalid("production proof object kind drifted")
	}
	value.ContractVersion = ContractVersionV2
	value.ObjectKind = ObjectKindV2
	provided := value.Digest
	value.Digest = ""
	digest, err := digestProductionProofBundleV2(value)
	if err != nil {
		return ProductionProofBundleV2{}, err
	}
	if provided != "" && provided != digest {
		return ProductionProofBundleV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "production proof supplied a wrong digest")
	}
	value.Digest = digest
	return value, value.Validate()
}

func (value ProductionProofBundleV2) Validate() error {
	if value.ContractVersion != ContractVersionV2 || value.ObjectKind != ObjectKindV2 || strings.TrimSpace(value.ReleaseID) == "" || value.Revision == 0 || value.CheckedUnixNano <= 0 || value.ExpiresUnixNano <= value.CheckedUnixNano {
		return invalid("production proof identity or window is incomplete")
	}
	if value.ArtifactDigest.Validate() != nil || value.ManifestDigest.Validate() != nil || value.ResourceBindingSetRef.Validate() != nil {
		return invalid("production proof exact base is invalid")
	}
	resources := []runtimeports.ResourceHandleRefV1{value.MemoryFactStoreRef, value.MemoryContentStoreRef, value.KnowledgeFactStoreRef, value.KnowledgeContentStoreRef}
	kinds := []runtimeports.ResourceHandleKindV1{MemoryFactStoreKindV2, MemoryContentStoreKindV2, KnowledgeFactStoreKindV2, KnowledgeContentStoreKindV2}
	seen := map[string]struct{}{}
	for i, ref := range resources {
		if ref.Validate() != nil || ref.Kind != kinds[i] || ref.ExpiresUnixNano < value.ExpiresUnixNano {
			return invalid("production durable resource role is invalid")
		}
		if _, exists := seen[ref.ID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production durable resource roles alias")
		}
		seen[ref.ID] = struct{}{}
	}
	currents := value.requiredCurrents()
	for _, ref := range currents {
		if ref.Validate() != nil || ref.ExpiresUnixNano < value.ExpiresUnixNano {
			return invalid("production owner current is invalid or shorter than bundle TTL")
		}
		if _, exists := seen[ref.ID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production proof roles alias")
		}
		seen[ref.ID] = struct{}{}
	}
	switch value.Profile {
	case ProfileNonHAV2:
		if value.ReplicaCount != 1 || value.WriteQuorum != 1 || value.ReadQuorum != 1 || value.ReplicationCurrentRef != nil || value.QuorumCurrentRef != nil || value.FailoverProofRef != nil || value.MonotonicCurrentProofRef != nil {
			return invalid("non-HA proof may only claim one writer and no HA evidence")
		}
	case ProfileHAV2:
		if value.ReplicaCount < 3 || value.WriteQuorum <= value.ReplicaCount/2 || value.WriteQuorum > value.ReplicaCount || value.ReadQuorum == 0 || value.ReadQuorum > value.ReplicaCount {
			return invalid("HA quorum shape is invalid")
		}
		for _, ref := range []*runtimeports.OwnerCurrentRefV1{value.ReplicationCurrentRef, value.QuorumCurrentRef, value.FailoverProofRef, value.MonotonicCurrentProofRef} {
			if ref == nil || ref.Validate() != nil || ref.ExpiresUnixNano < value.ExpiresUnixNano {
				return invalid("HA proof is incomplete or stale")
			}
			if _, exists := seen[ref.ID]; exists {
				return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "HA proof roles alias")
			}
			seen[ref.ID] = struct{}{}
		}
	default:
		return invalid("production availability profile is invalid")
	}
	copy := value
	copy.Digest = ""
	digest, err := digestProductionProofBundleV2(copy)
	if err != nil || digest != value.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "production proof digest drifted")
	}
	return nil
}

func (value ProductionProofBundleV2) ValidateCurrent(now time.Time) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < value.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "production proof clock regressed")
	}
	if !now.Before(time.Unix(0, value.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "production proof expired")
	}
	return nil
}

func (value ProductionProofBundleV2) requiredCurrents() []runtimeports.OwnerCurrentRefV1 {
	return []runtimeports.OwnerCurrentRefV1{
		value.AuthorityPolicyCurrentRef, value.CredentialCurrentRef, value.RetrievalIndexCurrentRef,
		value.ContextSourceCurrentRef, value.SettlementCurrentRef, value.PurgeEffectCurrentRef,
		value.CleanupOwnerCurrentRef, value.DeploymentAttestationRef, value.CertificationFactRef,
		value.SingleWriterFenceCurrentRef, value.RecoveryProofRef, value.BackupRestoreProofRef,
	}
}

func digestProductionProofBundleV2(value ProductionProofBundleV2) (core.Digest, error) {
	return core.CanonicalJSONDigest(digestDomainV2, ContractVersionV2, ObjectKindV2, value)
}

func invalid(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}
