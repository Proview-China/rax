package fakes

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// SingleCallToolActionCoordinationStoreV2 is a deterministic process-local
// Fact store and fault-injection fixture. It makes no production durability,
// availability or service-level claim.
type SingleCallToolActionCoordinationStoreV2 struct {
	mu sync.Mutex

	facts    map[string]contract.SingleCallToolActionCoordinationFactV2
	initials map[string]contract.SingleCallToolActionCoordinationFactV2
	claims   map[string]contract.SingleCallToolActionVersionClaimV1

	loseNextCreateReply  bool
	loseNextCASReply     bool
	loseNextInspectReply bool

	createCommits uint64
	casCommits    uint64
}

func NewSingleCallToolActionCoordinationStoreV2() *SingleCallToolActionCoordinationStoreV2 {
	return &SingleCallToolActionCoordinationStoreV2{
		facts:    make(map[string]contract.SingleCallToolActionCoordinationFactV2),
		initials: make(map[string]contract.SingleCallToolActionCoordinationFactV2),
		claims:   make(map[string]contract.SingleCallToolActionVersionClaimV1),
	}
}

var _ applicationports.SingleCallToolActionCoordinationFactPortV2 = (*SingleCallToolActionCoordinationStoreV2)(nil)

func (store *SingleCallToolActionCoordinationStoreV2) CreateSingleCallToolActionCoordinationV2(ctx context.Context, fact contract.SingleCallToolActionCoordinationFactV2) (contract.SingleCallToolActionCoordinationFactV2, error) {
	if store == nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 store is nil")
	}
	if isNilSingleCallContextV2(ctx) {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call V2 create context is nil")
	}
	if err := fact.Validate(); err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	if fact.Revision != 1 || fact.State != contract.SingleCallToolActionPreparedV2 {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "new single-call V2 coordination must be prepared revision 1")
	}
	claim, err := contract.NewSingleCallToolActionVersionClaimV1(fact)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	factKey, err := singleCallToolActionFactKeyV2(fact.Request.Action.ExecutionScope, fact.ID)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	claimKey := string(claim.ConflictKey.Digest)

	store.mu.Lock()
	defer store.mu.Unlock()
	store.initializeLockedV2()

	if current, exists := store.facts[factKey]; exists {
		initial, ok := store.initials[factKey]
		if !ok {
			return contract.SingleCallToolActionCoordinationFactV2{}, indeterminateSingleCallStoreV2("single-call V2 initial fact is missing")
		}
		if err := store.validateClaimLockedV2(factKey, current); err != nil {
			return contract.SingleCallToolActionCoordinationFactV2{}, err
		}
		if initial.Digest != fact.Digest || initial.Request.Digest != fact.Request.Digest {
			return contract.SingleCallToolActionCoordinationFactV2{}, conflictSingleCallStoreV2("single-call V2 coordination ID already binds different initial content")
		}
		return cloneSingleCallCoordinationV2(current)
	}

	if existingClaim, occupied := store.claims[claimKey]; occupied {
		if !sameSingleCallVersionClaimV1(existingClaim, claim) {
			return contract.SingleCallToolActionCoordinationFactV2{}, conflictSingleCallStoreV2("single-call semantic conflict key is already claimed by another action version or coordination")
		}
		return contract.SingleCallToolActionCoordinationFactV2{}, indeterminateSingleCallStoreV2("single-call version claim exists without its coordination fact")
	}

	// One lock is the fixture's single linearization point: neither the Claim
	// nor the prepared Fact can become visible without the other.
	initialClone, err := cloneSingleCallCoordinationV2(fact)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	factClone, err := cloneSingleCallCoordinationV2(fact)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	store.claims[claimKey] = claim
	store.initials[factKey] = initialClone
	store.facts[factKey] = factClone
	store.createCommits++
	if store.loseNextCreateReply {
		store.loseNextCreateReply = false
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected single-call V2 create reply loss")
	}
	return cloneSingleCallCoordinationV2(fact)
}

func (store *SingleCallToolActionCoordinationStoreV2) InspectSingleCallToolActionCoordinationV2(ctx context.Context, scope core.ExecutionScope, id string) (contract.SingleCallToolActionCoordinationFactV2, error) {
	if store == nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 store is nil")
	}
	if isNilSingleCallContextV2(ctx) {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call V2 Inspect context is nil")
	}
	key, err := singleCallToolActionFactKeyV2(scope, id)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.loseNextInspectReply {
		store.loseNextInspectReply = false
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected single-call V2 Inspect reply loss")
	}
	current, ok := store.facts[key]
	if !ok {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "single-call V2 coordination fact not found")
	}
	if err := store.validateClaimLockedV2(key, current); err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	return cloneSingleCallCoordinationV2(current)
}

func (store *SingleCallToolActionCoordinationStoreV2) CompareAndSwapSingleCallToolActionCoordinationV2(ctx context.Context, request applicationports.SingleCallToolActionCoordinationCASRequestV2) (contract.SingleCallToolActionCoordinationFactV2, error) {
	if store == nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 store is nil")
	}
	if isNilSingleCallContextV2(ctx) {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call V2 CAS context is nil")
	}
	if err := request.Validate(); err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	key, err := singleCallToolActionFactKeyV2(request.Scope, request.ID)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	current, ok := store.facts[key]
	if !ok {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "single-call V2 coordination fact not found")
	}
	if err := store.validateClaimLockedV2(key, current); err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	if current.Revision != request.ExpectedRevision || current.Digest != request.ExpectedDigest {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call V2 coordination revision or digest changed")
	}
	if err := contract.ValidateSingleCallToolActionCoordinationTransitionV2(current, request.Next); err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	nextClone, err := cloneSingleCallCoordinationV2(request.Next)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	store.facts[key] = nextClone
	store.casCommits++
	if store.loseNextCASReply {
		store.loseNextCASReply = false
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected single-call V2 CAS reply loss")
	}
	return cloneSingleCallCoordinationV2(request.Next)
}

// InspectSingleCallToolActionVersionClaimForTestV1 exposes immutable fixture
// evidence for white-box tests. It is intentionally not part of the public
// Application FactPort.
func (store *SingleCallToolActionCoordinationStoreV2) InspectSingleCallToolActionVersionClaimForTestV1(scope core.ExecutionScope, id string) (contract.SingleCallToolActionVersionClaimV1, error) {
	if store == nil {
		return contract.SingleCallToolActionVersionClaimV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 store is nil")
	}
	key, err := singleCallToolActionFactKeyV2(scope, id)
	if err != nil {
		return contract.SingleCallToolActionVersionClaimV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	current, ok := store.facts[key]
	if !ok {
		return contract.SingleCallToolActionVersionClaimV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "single-call V2 coordination fact not found")
	}
	initial, ok := store.initials[key]
	if !ok {
		return contract.SingleCallToolActionVersionClaimV1{}, indeterminateSingleCallStoreV2("single-call V2 initial fact is missing")
	}
	conflictKey, err := contract.DeriveSingleCallToolActionCrossVersionConflictKeyV1(current.Request)
	if err != nil {
		return contract.SingleCallToolActionVersionClaimV1{}, err
	}
	claim, ok := store.claims[string(conflictKey.Digest)]
	if !ok {
		return contract.SingleCallToolActionVersionClaimV1{}, indeterminateSingleCallStoreV2("single-call V2 version claim is missing")
	}
	if err := claim.ValidateFor(initial); err != nil {
		return contract.SingleCallToolActionVersionClaimV1{}, err
	}
	return claim, nil
}

// InspectSingleCallToolActionAtomicInitialClaimForTestV2 returns the immutable
// prepared Fact and its VersionClaim while holding the Store's one lock. This
// is a white-box fixture capability, not a production read port.
func (store *SingleCallToolActionCoordinationStoreV2) InspectSingleCallToolActionAtomicInitialClaimForTestV2(scope core.ExecutionScope, id string) (contract.SingleCallToolActionCoordinationFactV2, contract.SingleCallToolActionVersionClaimV1, error) {
	if store == nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, contract.SingleCallToolActionVersionClaimV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 store is nil")
	}
	key, err := singleCallToolActionFactKeyV2(scope, id)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, contract.SingleCallToolActionVersionClaimV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	initial, ok := store.initials[key]
	if !ok {
		return contract.SingleCallToolActionCoordinationFactV2{}, contract.SingleCallToolActionVersionClaimV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "single-call V2 initial fact not found")
	}
	conflictKey, err := contract.DeriveSingleCallToolActionCrossVersionConflictKeyV1(initial.Request)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, contract.SingleCallToolActionVersionClaimV1{}, err
	}
	claim, ok := store.claims[string(conflictKey.Digest)]
	if !ok {
		return contract.SingleCallToolActionCoordinationFactV2{}, contract.SingleCallToolActionVersionClaimV1{}, indeterminateSingleCallStoreV2("single-call V2 version claim is missing")
	}
	if err := claim.ValidateFor(initial); err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, contract.SingleCallToolActionVersionClaimV1{}, err
	}
	initialClone, err := cloneSingleCallCoordinationV2(initial)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, contract.SingleCallToolActionVersionClaimV1{}, err
	}
	return initialClone, claim, nil
}

// OccupySingleCallToolActionVersionClaimForTestV1 lets conformance tests place
// an exact V1 claim in the same semantic conflict domain before attempting a
// V2 create. It writes no Coordination Fact and is not a production port.
func (store *SingleCallToolActionCoordinationStoreV2) OccupySingleCallToolActionVersionClaimForTestV1(claim contract.SingleCallToolActionVersionClaimV1) error {
	if store == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 store is nil")
	}
	if err := claim.Validate(); err != nil {
		return err
	}
	if claim.ClaimedActionVersion != contract.SingleCallToolActionContractVersionV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "test claim must occupy the V1 action version")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.initializeLockedV2()
	key := string(claim.ConflictKey.Digest)
	if current, ok := store.claims[key]; ok {
		if sameSingleCallVersionClaimV1(current, claim) {
			return nil
		}
		return conflictSingleCallStoreV2("single-call semantic conflict key is already claimed")
	}
	store.claims[key] = claim
	return nil
}

func (store *SingleCallToolActionCoordinationStoreV2) Counts() (uint64, uint64) {
	if store == nil {
		return 0, 0
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.createCommits, store.casCommits
}

func (store *SingleCallToolActionCoordinationStoreV2) LoseNextCreateReplyForTestV2() error {
	return store.setFaultForTestV2("create")
}

func (store *SingleCallToolActionCoordinationStoreV2) LoseNextCASReplyForTestV2() error {
	return store.setFaultForTestV2("cas")
}

func (store *SingleCallToolActionCoordinationStoreV2) LoseNextInspectReplyForTestV2() error {
	return store.setFaultForTestV2("inspect")
}

func (store *SingleCallToolActionCoordinationStoreV2) setFaultForTestV2(kind string) error {
	if store == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 store is nil")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	switch kind {
	case "create":
		store.loseNextCreateReply = true
	case "cas":
		store.loseNextCASReply = true
	case "inspect":
		store.loseNextInspectReply = true
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "unknown single-call V2 fault injection")
	}
	return nil
}

func (store *SingleCallToolActionCoordinationStoreV2) initializeLockedV2() {
	if store.facts == nil {
		store.facts = make(map[string]contract.SingleCallToolActionCoordinationFactV2)
	}
	if store.initials == nil {
		store.initials = make(map[string]contract.SingleCallToolActionCoordinationFactV2)
	}
	if store.claims == nil {
		store.claims = make(map[string]contract.SingleCallToolActionVersionClaimV1)
	}
}

func (store *SingleCallToolActionCoordinationStoreV2) validateClaimLockedV2(factKey string, current contract.SingleCallToolActionCoordinationFactV2) error {
	initial, ok := store.initials[factKey]
	if !ok {
		return indeterminateSingleCallStoreV2("single-call V2 initial fact is missing")
	}
	if initial.Revision != 1 || initial.State != contract.SingleCallToolActionPreparedV2 || initial.ID != current.ID || initial.Request.Digest != current.Request.Digest || initial.CreatedUnixNano != current.CreatedUnixNano {
		return conflictSingleCallStoreV2("single-call V2 current fact no longer binds its immutable initial fact")
	}
	conflictKey, err := contract.DeriveSingleCallToolActionCrossVersionConflictKeyV1(current.Request)
	if err != nil {
		return err
	}
	claim, ok := store.claims[string(conflictKey.Digest)]
	if !ok {
		return indeterminateSingleCallStoreV2("single-call V2 version claim is missing")
	}
	if err := claim.ValidateFor(initial); err != nil {
		return err
	}
	return nil
}

func singleCallToolActionFactKeyV2(scope core.ExecutionScope, id string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) || len(id) > contract.MaxSingleCallCoordinateIDBytesV2 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call V2 coordination ID is invalid")
	}
	digest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return "", err
	}
	return string(digest) + "\x00" + id, nil
}

func cloneSingleCallCoordinationV2(value contract.SingleCallToolActionCoordinationFactV2) (contract.SingleCallToolActionCoordinationFactV2, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "single-call V2 fact clone encode failed")
	}
	var clone contract.SingleCallToolActionCoordinationFactV2
	if err := json.Unmarshal(payload, &clone); err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "single-call V2 fact clone decode failed")
	}
	return clone, nil
}

func isNilSingleCallContextV2(ctx context.Context) bool {
	if ctx == nil {
		return true
	}
	value := reflect.ValueOf(ctx)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func sameSingleCallVersionClaimV1(left, right contract.SingleCallToolActionVersionClaimV1) bool {
	return left.ContractVersion == right.ContractVersion && left.ConflictKey == right.ConflictKey && left.ClaimedActionVersion == right.ClaimedActionVersion && left.CoordinationID == right.CoordinationID && left.CoordinationDigest == right.CoordinationDigest && left.Revision == right.Revision && left.CreatedUnixNano == right.CreatedUnixNano && left.Digest == right.Digest
}

func conflictSingleCallStoreV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, message)
}

func indeterminateSingleCallStoreV2(message string) error {
	return core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, message)
}
