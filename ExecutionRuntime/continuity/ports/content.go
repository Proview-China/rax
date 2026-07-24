package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

// MetadataStore is the backend-neutral SPI for relational Fact/CAS metadata.
type MetadataStore interface {
	CreateJournal(context.Context, contract.WriteJournal) error
	CASJournal(context.Context, uint64, contract.WriteJournal) error
	InspectJournal(context.Context, string) (contract.WriteJournal, error)
	StageManifest(context.Context, contract.ObjectManifest) error
	CommitObjectReference(context.Context, string, string) error
	SetObjectVisible(context.Context, string, bool) error
	InspectObject(context.Context, string) (contract.ObjectManifest, bool, error)
}

// ContentStore is the backend-neutral content-addressed SPI for a KV backend.
type ContentStore interface {
	PutChunk(context.Context, contract.ChunkRef, []byte) error
	GetChunk(context.Context, contract.ChunkRef) ([]byte, error)
	HasChunk(context.Context, contract.ChunkRef) (bool, error)
}

type RetentionStore interface {
	CreateRetention(context.Context, contract.RetentionFact) error
	CASRetention(context.Context, uint64, contract.RetentionFact) error
	InspectRetention(context.Context, string) (contract.RetentionFact, error)
}
