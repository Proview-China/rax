// Package release publishes the Application shared engine as an assembly
// candidate without upgrading local coordinators or fakes into production.
package release

import (
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"time"
)

const (
	LocalVersionV1                                   = "praxis.application/component-local-readiness/v1"
	ProductionVersionV1                              = "praxis.application/component-production-readiness/v1"
	ComponentIDV1       runtimeports.ComponentIDV2   = "praxis/application"
	ComponentKindV1     runtimeports.ComponentKindV2 = "praxis/application"
)

var capabilities = []runtimeports.CapabilityNameV2{"praxis.application/command-workflow-v2", "praxis.application/run-coordination-v3", "praxis.application/governed-operation-v3", "praxis.application/single-call-tool-action-v2", "praxis.application/context-refresh-v1", "praxis.application/checkpoint-coordination-v1"}

type LocalReadinessProjectionV1 struct {
	ContractVersion      string                       `json:"contract_version"`
	ReleaseID            string                       `json:"release_id"`
	Revision             core.Revision                `json:"revision"`
	ArtifactDigest       core.Digest                  `json:"artifact_digest"`
	CommandWorkflowRef   assemblycontract.ObjectRefV1 `json:"command_workflow_ref"`
	RunCoordinationRef   assemblycontract.ObjectRefV1 `json:"run_coordination_ref"`
	GovernedOperationRef assemblycontract.ObjectRefV1 `json:"governed_operation_ref"`
	G6ARef               assemblycontract.ObjectRefV1 `json:"g6a_ref"`
	ContextRefreshRef    assemblycontract.ObjectRefV1 `json:"context_refresh_ref"`
	CheckpointRef        assemblycontract.ObjectRefV1 `json:"checkpoint_ref"`
	CheckedUnixNano      int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                        `json:"expires_unix_nano"`
	Digest               core.Digest                  `json:"digest"`
}

func (p LocalReadinessProjectionV1) refs() []assemblycontract.ObjectRefV1 {
	return []assemblycontract.ObjectRefV1{p.CommandWorkflowRef, p.RunCoordinationRef, p.GovernedOperationRef, p.G6ARef, p.ContextRefreshRef, p.CheckpointRef}
}
func LocalDigestV1(p LocalReadinessProjectionV1) (core.Digest, error) {
	p.ContractVersion = LocalVersionV1
	p.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.component-local-readiness", LocalVersionV1, "LocalReadinessProjectionV1", p)
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
		return invalid("local readiness incomplete")
	}
	if e := window(p.CheckedUnixNano, p.ExpiresUnixNano, now); e != nil {
		return e
	}
	if e := distinct(p.refs()); e != nil {
		return e
	}
	d, e := LocalDigestV1(p)
	if e != nil || d != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "local readiness digest drift")
	}
	return nil
}

type ProductionReadinessProjectionV1 struct {
	ContractVersion                 string                       `json:"contract_version"`
	ReleaseID                       string                       `json:"release_id"`
	Revision                        core.Revision                `json:"revision"`
	ArtifactDigest                  core.Digest                  `json:"artifact_digest"`
	ManifestDigest                  core.Digest                  `json:"manifest_digest"`
	DurableCommandOutboxStoreRef    assemblycontract.ObjectRefV1 `json:"durable_command_outbox_store_ref"`
	DurableWorkflowJournalStoreRef  assemblycontract.ObjectRefV1 `json:"durable_workflow_journal_store_ref"`
	DurableOperationAttemptStoreRef assemblycontract.ObjectRefV1 `json:"durable_operation_attempt_store_ref"`
	DurableRunStoreRef              assemblycontract.ObjectRefV1 `json:"durable_run_store_ref"`
	DurableG6AStoreRef              assemblycontract.ObjectRefV1 `json:"durable_g6a_store_ref"`
	DurableContextRefreshStoreRef   assemblycontract.ObjectRefV1 `json:"durable_context_refresh_store_ref"`
	DurableCheckpointStoreRef       assemblycontract.ObjectRefV1 `json:"durable_checkpoint_store_ref"`
	OutboxWorkerRef                 assemblycontract.ObjectRefV1 `json:"outbox_worker_ref"`
	RecoveryWorkerRef               assemblycontract.ObjectRefV1 `json:"recovery_worker_ref"`
	RuntimeGovernanceGatewayRef     assemblycontract.ObjectRefV1 `json:"runtime_governance_gateway_ref"`
	RunSettlementGatewayRef         assemblycontract.ObjectRefV1 `json:"run_settlement_gateway_ref"`
	ExecutionGatewayRef             assemblycontract.ObjectRefV1 `json:"execution_gateway_ref"`
	CleanupOwnerRef                 assemblycontract.ObjectRefV1 `json:"cleanup_owner_ref"`
	ProductionRootRef               assemblycontract.ObjectRefV1 `json:"production_root_ref"`
	DeploymentAttestationRef        assemblycontract.ObjectRefV1 `json:"deployment_attestation_ref"`
	CertificationFactRef            assemblycontract.ObjectRefV1 `json:"certification_fact_ref"`
	CheckedUnixNano                 int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano                 int64                        `json:"expires_unix_nano"`
	Digest                          core.Digest                  `json:"digest"`
}

func (p ProductionReadinessProjectionV1) refs() []assemblycontract.ObjectRefV1 {
	return []assemblycontract.ObjectRefV1{p.DurableCommandOutboxStoreRef, p.DurableWorkflowJournalStoreRef, p.DurableOperationAttemptStoreRef, p.DurableRunStoreRef, p.DurableG6AStoreRef, p.DurableContextRefreshStoreRef, p.DurableCheckpointStoreRef, p.OutboxWorkerRef, p.RecoveryWorkerRef, p.RuntimeGovernanceGatewayRef, p.RunSettlementGatewayRef, p.ExecutionGatewayRef, p.CleanupOwnerRef, p.ProductionRootRef, p.DeploymentAttestationRef, p.CertificationFactRef}
}
func (p ProductionReadinessProjectionV1) evidence() []assemblycontract.ObjectRefV1 {
	return p.refs()[:len(p.refs())-1]
}
func ProductionDigestV1(p ProductionReadinessProjectionV1) (core.Digest, error) {
	p.ContractVersion = ProductionVersionV1
	p.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.component-production-readiness", ProductionVersionV1, "ProductionReadinessProjectionV1", p)
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
		return invalid("production readiness incomplete")
	}
	if e := window(p.CheckedUnixNano, p.ExpiresUnixNano, now); e != nil {
		return e
	}
	if e := distinct(p.refs()); e != nil {
		return e
	}
	d, e := ProductionDigestV1(p)
	if e != nil || d != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "production readiness digest drift")
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

func (r PublicationRequestV1) Validate(n time.Time) error {
	if r.ReleaseID == "" || r.Revision == 0 || r.CertificationID == "" || r.SourceRef.Validate() != nil || r.PublisherRef.Validate() != nil || r.TrustRef.Validate() != nil || r.ArtifactDigest.Validate() != nil {
		return invalid("publication request incomplete")
	}
	return window(r.CreatedUnixNano, r.ExpiresUnixNano, n)
}

type PublicationResultV1 struct {
	Release                     assemblercontract.ComponentReleaseV1
	LocalReady, ProductionReady bool
	Local                       *LocalReadinessProjectionV1
	Production                  *ProductionReadinessProjectionV1
}

func window(c, e int64, n time.Time) error {
	if n.IsZero() || c <= 0 || e <= c || n.UnixNano() < c {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "readiness clock invalid")
	}
	if !n.Before(time.Unix(0, e)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "readiness expired")
	}
	return nil
}
func distinct(rs []assemblycontract.ObjectRefV1) error {
	s := map[string]struct{}{}
	for _, r := range rs {
		if e := r.Validate(); e != nil {
			return e
		}
		if _, ok := s[r.ID]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "readiness alias")
		}
		s[r.ID] = struct{}{}
	}
	return nil
}
func invalid(m string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, m)
}
