// Package sqlite provides the default SQLite metadata driver and its
// backend-neutral conformance surface.
package sqlite

import "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"

type MetadataSPI interface {
	ports.MetadataStore
	ports.RetentionStore
	ports.TimelineProjectionStore
}

type GovernanceSPI interface {
	ports.TimelineGovernanceRepositoryV1
	ports.TimelineProjectionPolicyRepositoryV1
	ports.CheckpointManifestRepositoryV2
}

type ProductionSPI interface {
	MetadataSPI
	GovernanceSPI
	ports.RestorePlanRepositoryV2
	ports.ArtifactRelationReaderV1
	ports.ContentIntegrityAuditReaderV1
	ports.ContentDeltaReaderV1
	ports.HistoryDerivationCandidateReaderV1
}
