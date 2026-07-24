package modelinvoker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ToolCallCandidateObservationProjectionPublisherV1 is the narrow create-once
// write port for sealed model observations. It grants no PendingAction, Action,
// dispatch, evidence, or settlement authority.
type ToolCallCandidateObservationProjectionPublisherV1 interface {
	PublishSealedProjectionV1(context.Context, ToolCallCandidateObservationProjectionV1) (ToolCallCandidateObservationRefV1, error)
}

// ToolCallCandidateObservationProjectionReaderV1 is the narrow exact read port
// exposed to downstream consumers. A full Ref is mandatory; weak lookup by ID,
// call ID, or source fragments is intentionally absent.
type ToolCallCandidateObservationProjectionReaderV1 interface {
	InspectExactProjectionV1(context.Context, ToolCallCandidateObservationRefV1) (ToolCallCandidateObservationProjectionV1, error)
}

// ToolCallCandidateObservationProjectionRepositoryV1 is the atomic producer
// capability used by Model Invoker. Ensure must create-once or return the exact
// existing projection at one linearization point. Direct never composes the
// separate Publisher and Reader ports to infer recovery state. Downstream
// consumers should still receive only the narrower ReaderV1 capability.
type ToolCallCandidateObservationProjectionRepositoryV1 interface {
	EnsureSealedProjectionV1(context.Context, ToolCallCandidateObservationProjectionV1) (ToolCallCandidateObservationProjectionV1, error)
}

// IsToolCallCandidateObservationProjectionRepositoryUnavailableV1 rejects nil
// and typed-nil framework Repository capabilities before the canonical
// producer invokes the dependency.
func IsToolCallCandidateObservationProjectionRepositoryUnavailableV1(repository ToolCallCandidateObservationProjectionRepositoryV1) bool {
	if repository == nil {
		return true
	}
	value := reflect.ValueOf(repository)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// EnsureToolCallCandidateObservationProjectionV1 accepts one already-sealed
// canonical Projection and crosses the atomic Repository barrier. It does not
// seal an Observation. An Indeterminate reply is retried at most once with the
// same sealed Projection. Reader absence classifications never participate in
// this recovery path.
func EnsureToolCallCandidateObservationProjectionV1(
	ctx context.Context,
	repository ToolCallCandidateObservationProjectionRepositoryV1,
	sealed ToolCallCandidateObservationProjectionV1,
) (ToolCallCandidateObservationProjectionV1, error) {
	if IsToolCallCandidateObservationProjectionRepositoryUnavailableV1(repository) {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorUnavailable, "ensure_producer", "repository is nil or typed-nil", nil)
	}
	if err := sealed.Validate(); err != nil {
		return ToolCallCandidateObservationProjectionV1{}, err
	}
	ensured, ensureErr := repository.EnsureSealedProjectionV1(ctx, sealed.Clone())
	if ensureErr == nil {
		return requireExactEnsuredProjectionV1(sealed, ensured)
	}
	if ToolCallCandidateObservationProjectionErrorKindOfV1(ensureErr) != ToolCallCandidateObservationProjectionErrorIndeterminate {
		return ToolCallCandidateObservationProjectionV1{}, ensureErr
	}
	if err := sealed.Validate(); err != nil {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorConflict, "ensure_producer", "local sealed projection drifted before recovery", err)
	}
	ensured, retryErr := repository.EnsureSealedProjectionV1(ctx, sealed.Clone())
	if retryErr != nil {
		return ToolCallCandidateObservationProjectionV1{}, errors.Join(ensureErr, retryErr)
	}
	return requireExactEnsuredProjectionV1(sealed, ensured)
}

func requireExactEnsuredProjectionV1(sealed, ensured ToolCallCandidateObservationProjectionV1) (ToolCallCandidateObservationProjectionV1, error) {
	if err := sealed.Validate(); err != nil {
		return ToolCallCandidateObservationProjectionV1{}, err
	}
	if err := ensured.Validate(); err != nil {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorConflict, "ensure_producer", "repository returned an invalid projection", err)
	}
	if sealed.Ref != ensured.Ref {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorConflict, "ensure_producer", "repository returned a different exact ref", nil)
	}
	sealedWire, sealedErr := json.Marshal(sealed)
	ensuredWire, ensuredErr := json.Marshal(ensured)
	if sealedErr != nil || ensuredErr != nil || !bytes.Equal(sealedWire, ensuredWire) {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorConflict, "ensure_producer", "repository returned different canonical projection content", errors.Join(sealedErr, ensuredErr))
	}
	return ensured.Clone(), nil
}

type ToolCallCandidateObservationProjectionErrorKindV1 string

const (
	ToolCallCandidateObservationProjectionErrorInvalid             ToolCallCandidateObservationProjectionErrorKindV1 = "invalid"
	ToolCallCandidateObservationProjectionErrorConflict            ToolCallCandidateObservationProjectionErrorKindV1 = "conflict"
	ToolCallCandidateObservationProjectionErrorAuthoritativeAbsent ToolCallCandidateObservationProjectionErrorKindV1 = "authoritative_not_found"
	ToolCallCandidateObservationProjectionErrorUnknownAbsent       ToolCallCandidateObservationProjectionErrorKindV1 = "unknown_not_found"
	ToolCallCandidateObservationProjectionErrorRetentionUnreadable ToolCallCandidateObservationProjectionErrorKindV1 = "retention_unreadable"
	ToolCallCandidateObservationProjectionErrorUnavailable         ToolCallCandidateObservationProjectionErrorKindV1 = "unavailable"
	ToolCallCandidateObservationProjectionErrorIndeterminate       ToolCallCandidateObservationProjectionErrorKindV1 = "indeterminate"
)

type ToolCallCandidateObservationProjectionConsistencyV1 string

const (
	// ToolCallCandidateObservationProjectionConsistencyLinearizableNeverCreated
	// classifies a Reader-side exact absence proof for compatibility consumers.
	// Direct recovery never treats this classification as retry authority.
	ToolCallCandidateObservationProjectionConsistencyLinearizableNeverCreated ToolCallCandidateObservationProjectionConsistencyV1 = "linearizable_never_created"
)

// ToolCallCandidateObservationProjectionErrorV1 keeps repository failure
// semantics nominal. In particular, ordinary absence cannot be confused with
// a linearizable proof that the exact Ref/source tuple was never created.
type ToolCallCandidateObservationProjectionErrorV1 struct {
	Kind        ToolCallCandidateObservationProjectionErrorKindV1
	Operation   string
	Consistency ToolCallCandidateObservationProjectionConsistencyV1
	Message     string
	Err         error
}

func (e *ToolCallCandidateObservationProjectionErrorV1) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := e.Message
	if message == "" {
		message = string(e.Kind)
	}
	if e.Operation == "" {
		return message
	}
	return fmt.Sprintf("tool call observation projection %s: %s", e.Operation, message)
}

func (e *ToolCallCandidateObservationProjectionErrorV1) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *ToolCallCandidateObservationProjectionErrorV1) Is(target error) bool {
	other, ok := target.(*ToolCallCandidateObservationProjectionErrorV1)
	if !ok || e == nil || other == nil {
		return false
	}
	return (other.Kind == "" || e.Kind == other.Kind) &&
		(other.Consistency == "" || e.Consistency == other.Consistency)
}

func ToolCallCandidateObservationProjectionErrorKindOfV1(err error) ToolCallCandidateObservationProjectionErrorKindV1 {
	var repositoryError *ToolCallCandidateObservationProjectionErrorV1
	if errors.As(err, &repositoryError) && repositoryError != nil {
		return repositoryError.Kind
	}
	return ""
}

// IsToolCallCandidateObservationProjectionAuthoritativeNotFoundV1 recognizes
// the Reader-side compatibility classification. It grants no Direct publish or
// Ensure retry authority.
func IsToolCallCandidateObservationProjectionAuthoritativeNotFoundV1(err error) bool {
	var repositoryError *ToolCallCandidateObservationProjectionErrorV1
	return errors.As(err, &repositoryError) && repositoryError != nil &&
		repositoryError.Kind == ToolCallCandidateObservationProjectionErrorAuthoritativeAbsent &&
		repositoryError.Consistency == ToolCallCandidateObservationProjectionConsistencyLinearizableNeverCreated
}

type toolCallCandidateObservationProjectionSourceKeyV1 struct {
	InvocationID     string
	InvocationDigest core.Digest
	ResponseID       string
	SourceSequence   uint64
}

type storedToolCallCandidateObservationProjectionV1 struct {
	ref  ToolCallCandidateObservationRefV1
	wire json.RawMessage
}

// InMemoryToolCallCandidateObservationProjectionStoreV1 is a thread-safe
// reference store and test fixture. It is not a production persistence driver,
// composition root, retention service, or SLA-bearing repository.
type InMemoryToolCallCandidateObservationProjectionStoreV1 struct {
	mu           sync.RWMutex
	byID         map[string]storedToolCallCandidateObservationProjectionV1
	bySource     map[toolCallCandidateObservationProjectionSourceKeyV1]string
	ensureCalls  atomic.Uint64
	publishCalls atomic.Uint64
}

type InMemoryToolCallCandidateObservationProjectionStoreStatsV1 struct {
	EnsureCalls  uint64
	PublishCalls uint64
	Records      uint64
}

func NewInMemoryToolCallCandidateObservationProjectionStoreV1() *InMemoryToolCallCandidateObservationProjectionStoreV1 {
	return &InMemoryToolCallCandidateObservationProjectionStoreV1{
		byID:     make(map[string]storedToolCallCandidateObservationProjectionV1),
		bySource: make(map[toolCallCandidateObservationProjectionSourceKeyV1]string),
	}
}

func (store *InMemoryToolCallCandidateObservationProjectionStoreV1) PublishSealedProjectionV1(ctx context.Context, projection ToolCallCandidateObservationProjectionV1) (ToolCallCandidateObservationRefV1, error) {
	if store == nil {
		return ToolCallCandidateObservationRefV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorUnavailable, "publish", "reference store is unavailable", nil)
	}
	store.publishCalls.Add(1)
	ensured, err := store.ensureSealedProjectionV1(ctx, projection, "publish")
	if err != nil {
		return ToolCallCandidateObservationRefV1{}, err
	}
	return ensured.Ref, nil
}

// EnsureSealedProjectionV1 atomically creates the sealed Projection or returns
// the exact existing canonical Projection. It never reports absence as a
// recovery instruction and always returns a deep clone.
func (store *InMemoryToolCallCandidateObservationProjectionStoreV1) EnsureSealedProjectionV1(ctx context.Context, projection ToolCallCandidateObservationProjectionV1) (ToolCallCandidateObservationProjectionV1, error) {
	if store == nil {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorUnavailable, "ensure", "reference store is unavailable", nil)
	}
	store.ensureCalls.Add(1)
	return store.ensureSealedProjectionV1(ctx, projection, "ensure")
}

func (store *InMemoryToolCallCandidateObservationProjectionStoreV1) ensureSealedProjectionV1(ctx context.Context, projection ToolCallCandidateObservationProjectionV1, operation string) (ToolCallCandidateObservationProjectionV1, error) {
	if err := projectionContextErrorV1(ctx, operation); err != nil {
		return ToolCallCandidateObservationProjectionV1{}, err
	}
	wire, err := canonicalToolCallCandidateObservationProjectionWireV1(projection)
	if err != nil {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorInvalid, operation, "sealed projection is invalid", err)
	}
	source := projectionSourceKeyV1(projection.Ref)

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := projectionContextErrorV1(ctx, operation); err != nil {
		return ToolCallCandidateObservationProjectionV1{}, err
	}
	if store.byID == nil {
		store.byID = make(map[string]storedToolCallCandidateObservationProjectionV1)
	}
	if store.bySource == nil {
		store.bySource = make(map[toolCallCandidateObservationProjectionSourceKeyV1]string)
	}
	if existing, ok := store.byID[projection.Ref.ID]; ok {
		existingID, sourceExists := store.bySource[source]
		if sourceExists && existingID == projection.Ref.ID && existing.ref == projection.Ref && bytes.Equal(existing.wire, wire) {
			decoded, decodeErr := DecodeToolCallCandidateObservationProjectionV1(existing.wire)
			if decodeErr != nil || decoded.Ref != projection.Ref {
				return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorConflict, operation, "stored projection failed strict revalidation", decodeErr)
			}
			return decoded.Clone(), nil
		}
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorConflict, operation, "projection ID already exists with different canonical content", nil)
	}
	if existingID, ok := store.bySource[source]; ok {
		existing := store.byID[existingID]
		if existing.ref == projection.Ref && bytes.Equal(existing.wire, wire) {
			decoded, decodeErr := DecodeToolCallCandidateObservationProjectionV1(existing.wire)
			if decodeErr != nil || decoded.Ref != projection.Ref {
				return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorConflict, operation, "stored projection failed strict revalidation", decodeErr)
			}
			return decoded.Clone(), nil
		}
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorConflict, operation, "projection source coordinate already exists with different identity or content", nil)
	}
	store.byID[projection.Ref.ID] = storedToolCallCandidateObservationProjectionV1{ref: projection.Ref, wire: append(json.RawMessage(nil), wire...)}
	store.bySource[source] = projection.Ref.ID
	return projection.Clone(), nil
}

func (store *InMemoryToolCallCandidateObservationProjectionStoreV1) InspectExactProjectionV1(ctx context.Context, ref ToolCallCandidateObservationRefV1) (ToolCallCandidateObservationProjectionV1, error) {
	if store == nil {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorUnavailable, "inspect", "reference store is unavailable", nil)
	}
	if err := projectionContextErrorV1(ctx, "inspect"); err != nil {
		return ToolCallCandidateObservationProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorInvalid, "inspect", "full exact projection ref is invalid", err)
	}
	source := projectionSourceKeyV1(ref)

	store.mu.RLock()
	existing, idExists := store.byID[ref.ID]
	existingID, sourceExists := store.bySource[source]
	if idExists {
		existing.wire = append(json.RawMessage(nil), existing.wire...)
	}
	store.mu.RUnlock()

	if !idExists && !sourceExists {
		return ToolCallCandidateObservationProjectionV1{}, &ToolCallCandidateObservationProjectionErrorV1{
			Kind: ToolCallCandidateObservationProjectionErrorAuthoritativeAbsent, Operation: "inspect",
			Consistency: ToolCallCandidateObservationProjectionConsistencyLinearizableNeverCreated,
			Message:     "linearizable lookup proved that the exact ref and source were never created",
		}
	}
	if !idExists || !sourceExists || existingID != ref.ID || existing.ref != ref {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorConflict, "inspect", "stored projection identity or source differs from the exact ref", nil)
	}
	projection, err := DecodeToolCallCandidateObservationProjectionV1(existing.wire)
	if err != nil {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorConflict, "inspect", "stored projection failed strict revalidation", err)
	}
	if projection.Ref != ref {
		return ToolCallCandidateObservationProjectionV1{}, projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorConflict, "inspect", "decoded projection ref differs from the requested exact ref", nil)
	}
	return projection.Clone(), nil
}

func (store *InMemoryToolCallCandidateObservationProjectionStoreV1) StatsV1() InMemoryToolCallCandidateObservationProjectionStoreStatsV1 {
	if store == nil {
		return InMemoryToolCallCandidateObservationProjectionStoreStatsV1{}
	}
	store.mu.RLock()
	records := len(store.byID)
	store.mu.RUnlock()
	return InMemoryToolCallCandidateObservationProjectionStoreStatsV1{
		EnsureCalls: store.ensureCalls.Load(), PublishCalls: store.publishCalls.Load(), Records: uint64(records),
	}
}

func canonicalToolCallCandidateObservationProjectionWireV1(projection ToolCallCandidateObservationProjectionV1) (json.RawMessage, error) {
	if err := projection.Validate(); err != nil {
		return nil, err
	}
	wire, err := json.Marshal(projection)
	if err != nil {
		return nil, err
	}
	decoded, err := DecodeToolCallCandidateObservationProjectionV1(wire)
	if err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(decoded)
	if err != nil || !bytes.Equal(wire, canonical) {
		return nil, toolCallObservationError(ErrorMapping, "tool_call_observation_projection_not_canonical", "tool call observation projection is not canonical")
	}
	return append(json.RawMessage(nil), wire...), nil
}

func projectionSourceKeyV1(ref ToolCallCandidateObservationRefV1) toolCallCandidateObservationProjectionSourceKeyV1 {
	return toolCallCandidateObservationProjectionSourceKeyV1{
		InvocationID: ref.InvocationID, InvocationDigest: ref.InvocationDigest,
		ResponseID: ref.Source.ResponseID, SourceSequence: ref.Source.SourceSequence,
	}
}

func projectionRepositoryErrorV1(kind ToolCallCandidateObservationProjectionErrorKindV1, operation, message string, err error) error {
	return &ToolCallCandidateObservationProjectionErrorV1{Kind: kind, Operation: operation, Message: message, Err: err}
}

func projectionContextErrorV1(ctx context.Context, operation string) error {
	if ctx == nil {
		return projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorIndeterminate, operation, "context is nil", context.Canceled)
	}
	if err := ctx.Err(); err != nil {
		return projectionRepositoryErrorV1(ToolCallCandidateObservationProjectionErrorIndeterminate, operation, "context ended before the repository operation linearized", err)
	}
	return nil
}

var (
	_ ToolCallCandidateObservationProjectionRepositoryV1 = (*InMemoryToolCallCandidateObservationProjectionStoreV1)(nil)
	_ ToolCallCandidateObservationProjectionPublisherV1  = (*InMemoryToolCallCandidateObservationProjectionStoreV1)(nil)
	_ ToolCallCandidateObservationProjectionReaderV1     = (*InMemoryToolCallCandidateObservationProjectionStoreV1)(nil)
)
