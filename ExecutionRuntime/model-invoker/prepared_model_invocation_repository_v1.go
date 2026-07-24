package modelinvoker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// PreparedModelInvocationRepositoryV1 is the atomic create-once Historical
// producer capability. It owns neither Registry nor provider dispatch.
type PreparedModelInvocationRepositoryV1 interface {
	EnsurePreparedModelInvocationV1(context.Context, PreparedModelInvocationFactV1) (PreparedModelInvocationFactV1, error)
}

// PreparedModelInvocationReaderV1 exposes exact immutable Historical reads.
type PreparedModelInvocationReaderV1 interface {
	InspectExactPreparedModelInvocationV1(context.Context, PreparedModelInvocationRefV1) (PreparedModelInvocationFactV1, error)
}

// PreparedModelInvocationCurrentReaderV1 exposes exact immutable Current
// reads. Historical retention and Current validity remain distinct.
type PreparedModelInvocationCurrentReaderV1 interface {
	InspectExactPreparedModelInvocationCurrentV1(context.Context, PreparedModelInvocationCurrentRefV1) (PreparedModelInvocationCurrentProjectionV1, error)
}

// PreparedModelInvocationCurrentRepositoryV1 atomically creates or returns the
// unique Current projection for one complete Historical Ref.
type PreparedModelInvocationCurrentRepositoryV1 interface {
	PreparedModelInvocationCurrentReaderV1
	EnsurePreparedModelInvocationCurrentV1(context.Context, PreparedModelInvocationCurrentProjectionV1) (PreparedModelInvocationCurrentProjectionV1, error)
}

type PreparedModelInvocationRepositoryErrorKindV1 string

const (
	PreparedModelInvocationRepositoryErrorInvalid             PreparedModelInvocationRepositoryErrorKindV1 = "invalid"
	PreparedModelInvocationRepositoryErrorConflict            PreparedModelInvocationRepositoryErrorKindV1 = "conflict"
	PreparedModelInvocationRepositoryErrorAuthoritativeAbsent PreparedModelInvocationRepositoryErrorKindV1 = "authoritative_not_found"
	PreparedModelInvocationRepositoryErrorUnknownAbsent       PreparedModelInvocationRepositoryErrorKindV1 = "unknown_not_found"
	PreparedModelInvocationRepositoryErrorRetentionUnreadable PreparedModelInvocationRepositoryErrorKindV1 = "retention_unreadable"
	PreparedModelInvocationRepositoryErrorUnavailable         PreparedModelInvocationRepositoryErrorKindV1 = "unavailable"
	PreparedModelInvocationRepositoryErrorIndeterminate       PreparedModelInvocationRepositoryErrorKindV1 = "indeterminate"
)

type PreparedModelInvocationRepositoryErrorV1 struct {
	Kind      PreparedModelInvocationRepositoryErrorKindV1
	Operation string
	Message   string
	Err       error
}

func (e *PreparedModelInvocationRepositoryErrorV1) Error() string {
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
	return fmt.Sprintf("prepared model invocation %s: %s", e.Operation, message)
}

func (e *PreparedModelInvocationRepositoryErrorV1) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *PreparedModelInvocationRepositoryErrorV1) Is(target error) bool {
	other, ok := target.(*PreparedModelInvocationRepositoryErrorV1)
	return ok && e != nil && other != nil && (other.Kind == "" || e.Kind == other.Kind)
}

func PreparedModelInvocationRepositoryErrorKindOfV1(err error) PreparedModelInvocationRepositoryErrorKindV1 {
	var repositoryError *PreparedModelInvocationRepositoryErrorV1
	if errors.As(err, &repositoryError) && repositoryError != nil {
		return repositoryError.Kind
	}
	switch {
	case core.HasCategory(err, core.ErrorInvalidArgument):
		return PreparedModelInvocationRepositoryErrorInvalid
	case core.HasCategory(err, core.ErrorConflict):
		return PreparedModelInvocationRepositoryErrorConflict
	case core.HasCategory(err, core.ErrorNotFound):
		return PreparedModelInvocationRepositoryErrorUnknownAbsent
	case core.HasCategory(err, core.ErrorUnavailable), core.HasCategory(err, core.ErrorCapabilityUnavailable):
		return PreparedModelInvocationRepositoryErrorUnavailable
	case core.HasCategory(err, core.ErrorIndeterminate):
		return PreparedModelInvocationRepositoryErrorIndeterminate
	}
	return ""
}

func IsPreparedModelInvocationRepositoryUnavailableV1(repository PreparedModelInvocationRepositoryV1) bool {
	return nilLikePreparedV1(repository)
}

func IsPreparedModelInvocationCurrentRepositoryUnavailableV1(repository PreparedModelInvocationCurrentRepositoryV1) bool {
	return nilLikePreparedV1(repository)
}

// EnsurePreparedModelInvocationFactV1 crosses the atomic Historical barrier.
// An Indeterminate outcome may repeat the same atomic Ensure once; it never
// changes identity, canonical content or repository capability.
func EnsurePreparedModelInvocationFactV1(
	ctx context.Context,
	repository PreparedModelInvocationRepositoryV1,
	sealed PreparedModelInvocationFactV1,
) (PreparedModelInvocationFactV1, error) {
	if err := preparedRepositoryContextErrorV1(ctx, "ensure_historical"); err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	if IsPreparedModelInvocationRepositoryUnavailableV1(repository) {
		return PreparedModelInvocationFactV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorInvalid, "ensure_historical", "repository is nil or typed-nil", nil)
	}
	if err := sealed.Validate(); err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	ensured, err := repository.EnsurePreparedModelInvocationV1(ctx, sealed.Clone())
	if err == nil {
		return requireExactPreparedFactV1(sealed, ensured)
	}
	if !isPreparedRepositoryIndeterminateV1(err) {
		return PreparedModelInvocationFactV1{}, normalizePreparedRepositoryDependencyErrorV1(err, "ensure_historical")
	}
	ensured, retryErr := repository.EnsurePreparedModelInvocationV1(ctx, sealed.Clone())
	if retryErr != nil {
		return PreparedModelInvocationFactV1{}, normalizePreparedRepositoryDependencyErrorV1(retryErr, "ensure_historical_recovery")
	}
	return requireExactPreparedFactV1(sealed, ensured)
}

// EnsurePreparedModelInvocationCurrentProjectionV1 crosses the atomic Current
// barrier with the same one-retry, same-canonical-input recovery rule.
func EnsurePreparedModelInvocationCurrentProjectionV1(
	ctx context.Context,
	repository PreparedModelInvocationCurrentRepositoryV1,
	sealed PreparedModelInvocationCurrentProjectionV1,
) (PreparedModelInvocationCurrentProjectionV1, error) {
	if err := preparedRepositoryContextErrorV1(ctx, "ensure_current"); err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	if IsPreparedModelInvocationCurrentRepositoryUnavailableV1(repository) {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorInvalid, "ensure_current", "repository is nil or typed-nil", nil)
	}
	if err := sealed.Validate(); err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	ensured, err := repository.EnsurePreparedModelInvocationCurrentV1(ctx, sealed.Clone())
	if err == nil {
		return requireExactPreparedCurrentV1(sealed, ensured)
	}
	if !isPreparedRepositoryIndeterminateV1(err) {
		return PreparedModelInvocationCurrentProjectionV1{}, normalizePreparedRepositoryDependencyErrorV1(err, "ensure_current")
	}
	ensured, retryErr := repository.EnsurePreparedModelInvocationCurrentV1(ctx, sealed.Clone())
	if retryErr != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, normalizePreparedRepositoryDependencyErrorV1(retryErr, "ensure_current_recovery")
	}
	return requireExactPreparedCurrentV1(sealed, ensured)
}

type preparedModelInvocationCoordinateV1 struct {
	InvocationID     string
	InvocationDigest core.Digest
}

type storedPreparedModelInvocationV1 struct {
	ref  PreparedModelInvocationRefV1
	wire json.RawMessage
}

type storedPreparedModelInvocationCurrentV1 struct {
	ref  PreparedModelInvocationCurrentRefV1
	wire json.RawMessage
}

// InMemoryPreparedModelInvocationStoreV1 is a thread-safe reference Store and
// conformance fixture. It is not a production driver, composition root,
// retention service or SLA-bearing repository.
type InMemoryPreparedModelInvocationStoreV1 struct {
	mu sync.RWMutex

	historicalByID         map[string]storedPreparedModelInvocationV1
	historicalByInvocation map[preparedModelInvocationCoordinateV1]string
	currentByID            map[string]storedPreparedModelInvocationCurrentV1
	currentByPrepared      map[PreparedModelInvocationRefV1]string

	historicalEnsureCalls atomic.Uint64
	historicalReadCalls   atomic.Uint64
	currentEnsureCalls    atomic.Uint64
	currentReadCalls      atomic.Uint64
}

type InMemoryPreparedModelInvocationStoreStatsV1 struct {
	HistoricalEnsureCalls uint64
	HistoricalReadCalls   uint64
	HistoricalRecords     uint64
	CurrentEnsureCalls    uint64
	CurrentReadCalls      uint64
	CurrentRecords        uint64
}

func NewInMemoryPreparedModelInvocationStoreV1() *InMemoryPreparedModelInvocationStoreV1 {
	return &InMemoryPreparedModelInvocationStoreV1{
		historicalByID:         make(map[string]storedPreparedModelInvocationV1),
		historicalByInvocation: make(map[preparedModelInvocationCoordinateV1]string),
		currentByID:            make(map[string]storedPreparedModelInvocationCurrentV1),
		currentByPrepared:      make(map[PreparedModelInvocationRefV1]string),
	}
}

func (store *InMemoryPreparedModelInvocationStoreV1) EnsurePreparedModelInvocationV1(
	ctx context.Context,
	fact PreparedModelInvocationFactV1,
) (PreparedModelInvocationFactV1, error) {
	if store == nil {
		return PreparedModelInvocationFactV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorUnavailable, "ensure_historical", "reference store is unavailable", nil)
	}
	store.historicalEnsureCalls.Add(1)
	if err := preparedRepositoryContextErrorV1(ctx, "ensure_historical"); err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	wire, err := EncodePreparedModelInvocationFactV1(fact)
	if err != nil {
		return PreparedModelInvocationFactV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorInvalid, "ensure_historical", "sealed Historical Fact is invalid", err)
	}
	coordinate := preparedHistoricalCoordinateV1(fact.Ref())

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := preparedRepositoryContextErrorV1(ctx, "ensure_historical"); err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	store.initLockedV1()
	if existing, ok := store.historicalByID[fact.ID]; ok {
		existingID, coordinateExists := store.historicalByInvocation[coordinate]
		if coordinateExists && existingID == fact.ID && existing.ref == fact.Ref() && bytes.Equal(existing.wire, wire) {
			return decodeStoredPreparedFactV1(existing, fact.Ref(), "ensure_historical")
		}
		return PreparedModelInvocationFactV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, "ensure_historical", "Historical ID already exists with different canonical content", nil)
	}
	if existingID, ok := store.historicalByInvocation[coordinate]; ok {
		existing := store.historicalByID[existingID]
		if existing.ref == fact.Ref() && bytes.Equal(existing.wire, wire) {
			return decodeStoredPreparedFactV1(existing, fact.Ref(), "ensure_historical")
		}
		return PreparedModelInvocationFactV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, "ensure_historical", "Invocation coordinate already exists with different Historical content", nil)
	}
	store.historicalByID[fact.ID] = storedPreparedModelInvocationV1{ref: fact.Ref(), wire: append(json.RawMessage(nil), wire...)}
	store.historicalByInvocation[coordinate] = fact.ID
	return fact.Clone(), nil
}

func (store *InMemoryPreparedModelInvocationStoreV1) InspectExactPreparedModelInvocationV1(
	ctx context.Context,
	ref PreparedModelInvocationRefV1,
) (PreparedModelInvocationFactV1, error) {
	if store == nil {
		return PreparedModelInvocationFactV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorUnavailable, "inspect_historical", "reference store is unavailable", nil)
	}
	store.historicalReadCalls.Add(1)
	if err := preparedRepositoryContextErrorV1(ctx, "inspect_historical"); err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return PreparedModelInvocationFactV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorInvalid, "inspect_historical", "full exact Historical Ref is invalid", err)
	}
	coordinate := preparedHistoricalCoordinateV1(ref)
	store.mu.RLock()
	if err := preparedRepositoryContextErrorV1(ctx, "inspect_historical"); err != nil {
		store.mu.RUnlock()
		return PreparedModelInvocationFactV1{}, err
	}
	existing, idExists := store.historicalByID[ref.ID]
	existingID, coordinateExists := store.historicalByInvocation[coordinate]
	if idExists {
		existing.wire = append(json.RawMessage(nil), existing.wire...)
	}
	store.mu.RUnlock()
	if err := preparedRepositoryContextErrorV1(ctx, "inspect_historical"); err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	if !idExists && !coordinateExists {
		return PreparedModelInvocationFactV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorAuthoritativeAbsent, "inspect_historical", "linearizable lookup proved the exact Historical coordinate was never created", nil)
	}
	if !idExists || !coordinateExists || existingID != ref.ID || existing.ref != ref {
		return PreparedModelInvocationFactV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, "inspect_historical", "stored Historical identity differs from exact Ref", nil)
	}
	return decodeStoredPreparedFactV1(existing, ref, "inspect_historical")
}

func (store *InMemoryPreparedModelInvocationStoreV1) EnsurePreparedModelInvocationCurrentV1(
	ctx context.Context,
	current PreparedModelInvocationCurrentProjectionV1,
) (PreparedModelInvocationCurrentProjectionV1, error) {
	if store == nil {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorUnavailable, "ensure_current", "reference store is unavailable", nil)
	}
	store.currentEnsureCalls.Add(1)
	if err := preparedRepositoryContextErrorV1(ctx, "ensure_current"); err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	wire, err := EncodePreparedModelInvocationCurrentV1(current)
	if err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorInvalid, "ensure_current", "sealed Current projection is invalid", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := preparedRepositoryContextErrorV1(ctx, "ensure_current"); err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	store.initLockedV1()
	historical, ok := store.historicalByID[current.Prepared.ID]
	if !ok || historical.ref != current.Prepared {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, "ensure_current", "Current has no exact Historical parent in this repository", nil)
	}
	fact, err := decodeStoredPreparedFactV1(historical, current.Prepared, "ensure_current")
	if err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	if err := current.ValidateAgainstFact(fact); err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, "ensure_current", "Current differs from Historical parent", err)
	}
	if existing, ok := store.currentByID[current.ID]; ok {
		existingID, preparedExists := store.currentByPrepared[current.Prepared]
		if preparedExists && existingID == current.ID && existing.ref == current.Ref() && bytes.Equal(existing.wire, wire) {
			return decodeStoredPreparedCurrentV1(existing, current.Ref(), "ensure_current")
		}
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, "ensure_current", "Current ID already exists with different canonical content", nil)
	}
	if existingID, ok := store.currentByPrepared[current.Prepared]; ok {
		existing := store.currentByID[existingID]
		if existing.ref == current.Ref() && bytes.Equal(existing.wire, wire) {
			return decodeStoredPreparedCurrentV1(existing, current.Ref(), "ensure_current")
		}
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, "ensure_current", "Historical Ref already has another Current projection", nil)
	}
	store.currentByID[current.ID] = storedPreparedModelInvocationCurrentV1{ref: current.Ref(), wire: append(json.RawMessage(nil), wire...)}
	store.currentByPrepared[current.Prepared] = current.ID
	return current.Clone(), nil
}

func (store *InMemoryPreparedModelInvocationStoreV1) InspectExactPreparedModelInvocationCurrentV1(
	ctx context.Context,
	ref PreparedModelInvocationCurrentRefV1,
) (PreparedModelInvocationCurrentProjectionV1, error) {
	if store == nil {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorUnavailable, "inspect_current", "reference store is unavailable", nil)
	}
	store.currentReadCalls.Add(1)
	if err := preparedRepositoryContextErrorV1(ctx, "inspect_current"); err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorInvalid, "inspect_current", "full exact Current Ref is invalid", err)
	}
	store.mu.RLock()
	if err := preparedRepositoryContextErrorV1(ctx, "inspect_current"); err != nil {
		store.mu.RUnlock()
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	existing, idExists := store.currentByID[ref.ID]
	existingID, preparedExists := store.currentByPrepared[ref.Prepared]
	if idExists {
		existing.wire = append(json.RawMessage(nil), existing.wire...)
	}
	store.mu.RUnlock()
	if err := preparedRepositoryContextErrorV1(ctx, "inspect_current"); err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	if !idExists && !preparedExists {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorAuthoritativeAbsent, "inspect_current", "linearizable lookup proved the exact Current coordinate was never created", nil)
	}
	if !idExists || !preparedExists || existingID != ref.ID || existing.ref != ref {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, "inspect_current", "stored Current identity differs from exact Ref", nil)
	}
	return decodeStoredPreparedCurrentV1(existing, ref, "inspect_current")
}

func (store *InMemoryPreparedModelInvocationStoreV1) StatsV1() InMemoryPreparedModelInvocationStoreStatsV1 {
	if store == nil {
		return InMemoryPreparedModelInvocationStoreStatsV1{}
	}
	store.mu.RLock()
	historicalRecords := len(store.historicalByID)
	currentRecords := len(store.currentByID)
	store.mu.RUnlock()
	return InMemoryPreparedModelInvocationStoreStatsV1{
		HistoricalEnsureCalls: store.historicalEnsureCalls.Load(),
		HistoricalReadCalls:   store.historicalReadCalls.Load(),
		HistoricalRecords:     uint64(historicalRecords),
		CurrentEnsureCalls:    store.currentEnsureCalls.Load(),
		CurrentReadCalls:      store.currentReadCalls.Load(),
		CurrentRecords:        uint64(currentRecords),
	}
}

func (store *InMemoryPreparedModelInvocationStoreV1) initLockedV1() {
	if store.historicalByID == nil {
		store.historicalByID = make(map[string]storedPreparedModelInvocationV1)
	}
	if store.historicalByInvocation == nil {
		store.historicalByInvocation = make(map[preparedModelInvocationCoordinateV1]string)
	}
	if store.currentByID == nil {
		store.currentByID = make(map[string]storedPreparedModelInvocationCurrentV1)
	}
	if store.currentByPrepared == nil {
		store.currentByPrepared = make(map[PreparedModelInvocationRefV1]string)
	}
}

func requireExactPreparedFactV1(sealed, ensured PreparedModelInvocationFactV1) (PreparedModelInvocationFactV1, error) {
	sealedWire, sealedErr := EncodePreparedModelInvocationFactV1(sealed)
	ensuredWire, ensuredErr := EncodePreparedModelInvocationFactV1(ensured)
	if sealedErr != nil || ensuredErr != nil || sealed.Ref() != ensured.Ref() || !bytes.Equal(sealedWire, ensuredWire) {
		return PreparedModelInvocationFactV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, "ensure_historical", "repository returned different Historical content", errors.Join(sealedErr, ensuredErr))
	}
	return ensured.Clone(), nil
}

func requireExactPreparedCurrentV1(sealed, ensured PreparedModelInvocationCurrentProjectionV1) (PreparedModelInvocationCurrentProjectionV1, error) {
	sealedWire, sealedErr := EncodePreparedModelInvocationCurrentV1(sealed)
	ensuredWire, ensuredErr := EncodePreparedModelInvocationCurrentV1(ensured)
	if sealedErr != nil || ensuredErr != nil || sealed.Ref() != ensured.Ref() || !bytes.Equal(sealedWire, ensuredWire) {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, "ensure_current", "repository returned different Current content", errors.Join(sealedErr, ensuredErr))
	}
	return ensured.Clone(), nil
}

func decodeStoredPreparedFactV1(stored storedPreparedModelInvocationV1, expected PreparedModelInvocationRefV1, operation string) (PreparedModelInvocationFactV1, error) {
	fact, err := DecodePreparedModelInvocationFactV1(stored.wire)
	if err != nil || fact.Ref() != expected || stored.ref != expected {
		return PreparedModelInvocationFactV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, operation, "stored Historical Fact failed strict exact revalidation", err)
	}
	return fact.Clone(), nil
}

func decodeStoredPreparedCurrentV1(stored storedPreparedModelInvocationCurrentV1, expected PreparedModelInvocationCurrentRefV1, operation string) (PreparedModelInvocationCurrentProjectionV1, error) {
	current, err := DecodePreparedModelInvocationCurrentV1(stored.wire)
	if err != nil || current.Ref() != expected || stored.ref != expected {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorConflict, operation, "stored Current projection failed strict exact revalidation", err)
	}
	return current.Clone(), nil
}

func preparedHistoricalCoordinateV1(ref PreparedModelInvocationRefV1) preparedModelInvocationCoordinateV1 {
	return preparedModelInvocationCoordinateV1{InvocationID: ref.InvocationID, InvocationDigest: ref.InvocationDigest}
}

func preparedRepositoryErrorV1(kind PreparedModelInvocationRepositoryErrorKindV1, operation, message string, err error) error {
	return &PreparedModelInvocationRepositoryErrorV1{Kind: kind, Operation: operation, Message: message, Err: err}
}

func preparedRepositoryContextErrorV1(ctx context.Context, operation string) error {
	if ctx == nil {
		return preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorInvalid, operation, "context is nil", nil)
	}
	if err := ctx.Err(); err != nil {
		return preparedRepositoryErrorV1(PreparedModelInvocationRepositoryErrorIndeterminate, operation, "context ended before the repository operation linearized", err)
	}
	return nil
}

func isPreparedRepositoryIndeterminateV1(err error) bool {
	return PreparedModelInvocationRepositoryErrorKindOfV1(err) == PreparedModelInvocationRepositoryErrorIndeterminate
}

func normalizePreparedRepositoryDependencyErrorV1(err error, operation string) error {
	if err == nil {
		return nil
	}
	var repositoryError *PreparedModelInvocationRepositoryErrorV1
	if errors.As(err, &repositoryError) && repositoryError != nil {
		return err
	}
	kind := PreparedModelInvocationRepositoryErrorKindOfV1(err)
	if kind == "" {
		kind = PreparedModelInvocationRepositoryErrorIndeterminate
	}
	return preparedRepositoryErrorV1(kind, operation, "repository dependency failed", err)
}

var (
	_ PreparedModelInvocationRepositoryV1        = (*InMemoryPreparedModelInvocationStoreV1)(nil)
	_ PreparedModelInvocationReaderV1            = (*InMemoryPreparedModelInvocationStoreV1)(nil)
	_ PreparedModelInvocationCurrentRepositoryV1 = (*InMemoryPreparedModelInvocationStoreV1)(nil)
)
