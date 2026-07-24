package modelinvoker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	PreparedModelInvocationContractVersionV1          = "praxis.model-invoker.prepared-model-invocation/v1"
	PreparedModelInvocationCurrentContractVersionV1   = "praxis.model-invoker.prepared-model-invocation-current/v1"
	PreparedModelInvocationCommitAckContractVersionV1 = "praxis.model-invoker.prepared-model-invocation-commit-ack/v1"
	PreparedModelInvocationDispatchReceiptVersionV1   = "praxis.model-invoker.prepared-model-invocation-dispatch-validation/v1"
	preparedModelInvocationCanonicalDomainV1          = "praxis.model-invoker.prepared-model-invocation"
)

// PreparedModelInvocationCapabilitySnapshotRefV1 is a Model-owned exact
// coordinate for the sealed capability snapshot consumed by PurePrepare.
type PreparedModelInvocationCapabilitySnapshotRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

// PreparedModelInvocationGateImplementationRefV1 identifies the external
// Harness/host implementation that formed an ACK. It grants no authority.
type PreparedModelInvocationGateImplementationRefV1 struct {
	Owner           core.OwnerRef `json:"owner"`
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

// PreparedModelInvocationSurfaceBindingRefV1 is the neutral five-field
// coordinate carried by Model. It contains no Tool ACK payload or permit.
type PreparedModelInvocationSurfaceBindingRefV1 struct {
	Owner           core.OwnerRef `json:"owner"`
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

type PreparedModelInvocationRefV1 struct {
	ContractVersion      string        `json:"contract_version"`
	ID                   string        `json:"id"`
	Revision             core.Revision `json:"revision"`
	Digest               core.Digest   `json:"digest"`
	InvocationID         string        `json:"invocation_id"`
	InvocationDigest     core.Digest   `json:"invocation_digest"`
	UnifiedRequestDigest core.Digest   `json:"unified_request_digest"`
}

type PreparedModelInvocationFactV1 struct {
	ContractVersion               string                                         `json:"contract_version"`
	ID                            string                                         `json:"id"`
	Revision                      core.Revision                                  `json:"revision"`
	InvocationID                  string                                         `json:"invocation_id"`
	InvocationDigest              core.Digest                                    `json:"invocation_digest"`
	UnifiedRequestDigest          core.Digest                                    `json:"unified_request_digest"`
	RequestToolsDigest            core.Digest                                    `json:"request_tools_digest"`
	PreparedPlanDigest            core.Digest                                    `json:"prepared_plan_digest"`
	RouteDigest                   core.Digest                                    `json:"route_digest"`
	ProfileDigest                 core.Digest                                    `json:"profile_digest"`
	ActualToolSurfaceDigest       core.Digest                                    `json:"actual_tool_surface_digest"`
	ActualProviderInjectionDigest core.Digest                                    `json:"actual_provider_injection_digest"`
	CapabilitySnapshotRef         PreparedModelInvocationCapabilitySnapshotRefV1 `json:"capability_snapshot_ref"`
	RegistrySnapshotRef           runtimeports.RegistrySnapshotRefV1             `json:"registry_snapshot_ref"`
	CreatedUnixNano               int64                                          `json:"created_unix_nano"`
	NotAfterUnixNano              int64                                          `json:"not_after_unix_nano"`
	Digest                        core.Digest                                    `json:"digest"`
}

type PreparedModelInvocationCurrentRefV1 struct {
	ContractVersion  string                       `json:"contract_version"`
	ID               string                       `json:"id"`
	Revision         core.Revision                `json:"revision"`
	Digest           core.Digest                  `json:"digest"`
	Prepared         PreparedModelInvocationRefV1 `json:"prepared"`
	CheckedUnixNano  int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                        `json:"expires_unix_nano"`
	NotAfterUnixNano int64                        `json:"not_after_unix_nano"`
}

type PreparedModelInvocationCurrentProjectionV1 struct {
	ContractVersion               string                                         `json:"contract_version"`
	ID                            string                                         `json:"id"`
	Revision                      core.Revision                                  `json:"revision"`
	Digest                        core.Digest                                    `json:"digest"`
	Prepared                      PreparedModelInvocationRefV1                   `json:"prepared"`
	CapabilitySnapshotRef         PreparedModelInvocationCapabilitySnapshotRefV1 `json:"capability_snapshot_ref"`
	RegistrySnapshotRef           runtimeports.RegistrySnapshotRefV1             `json:"registry_snapshot_ref"`
	ActualToolSurfaceDigest       core.Digest                                    `json:"actual_tool_surface_digest"`
	ActualProviderInjectionDigest core.Digest                                    `json:"actual_provider_injection_digest"`
	CheckedUnixNano               int64                                          `json:"checked_unix_nano"`
	ExpiresUnixNano               int64                                          `json:"expires_unix_nano"`
	NotAfterUnixNano              int64                                          `json:"not_after_unix_nano"`
}

type PreparedModelInvocationCommitAckRefV1 struct {
	ContractVersion   string                                     `json:"contract_version"`
	ID                string                                     `json:"id"`
	Revision          core.Revision                              `json:"revision"`
	Digest            core.Digest                                `json:"digest"`
	PreparedRef       PreparedModelInvocationRefV1               `json:"prepared_ref"`
	CurrentRef        PreparedModelInvocationCurrentRefV1        `json:"current_ref"`
	SurfaceBindingRef PreparedModelInvocationSurfaceBindingRefV1 `json:"surface_binding_ref"`
	CheckedUnixNano   int64                                      `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                                      `json:"expires_unix_nano"`
	NotAfterUnixNano  int64                                      `json:"not_after_unix_nano"`
}

type PreparedModelInvocationCommitAckV1 struct {
	ContractVersion       string                                         `json:"contract_version"`
	ID                    string                                         `json:"id"`
	Revision              core.Revision                                  `json:"revision"`
	Digest                core.Digest                                    `json:"digest"`
	PreparedRef           PreparedModelInvocationRefV1                   `json:"prepared_ref"`
	CurrentRef            PreparedModelInvocationCurrentRefV1            `json:"current_ref"`
	GateImplementationRef PreparedModelInvocationGateImplementationRefV1 `json:"gate_implementation_ref"`
	SurfaceBindingRef     PreparedModelInvocationSurfaceBindingRefV1     `json:"surface_binding_ref"`
	CheckedUnixNano       int64                                          `json:"checked_unix_nano"`
	ExpiresUnixNano       int64                                          `json:"expires_unix_nano"`
	NotAfterUnixNano      int64                                          `json:"not_after_unix_nano"`
}

// PreparedModelInvocationDispatchValidationReceiptV1 is a non-authoritative
// record of one exact pre-dispatch validation. It is not reusable as a permit.
type PreparedModelInvocationDispatchValidationReceiptV1 struct {
	ContractVersion               string                                `json:"contract_version"`
	ID                            string                                `json:"id"`
	Revision                      core.Revision                         `json:"revision"`
	Digest                        core.Digest                           `json:"digest"`
	PreparedRef                   PreparedModelInvocationRefV1          `json:"prepared_ref"`
	CurrentRef                    PreparedModelInvocationCurrentRefV1   `json:"current_ref"`
	AckRef                        PreparedModelInvocationCommitAckRefV1 `json:"ack_ref"`
	DispatchSequence              uint64                                `json:"dispatch_sequence"`
	BoundaryKind                  string                                `json:"boundary_kind"`
	ProviderAttemptOrdinal        uint32                                `json:"provider_attempt_ordinal"`
	AttemptRequestDigest          core.Digest                           `json:"attempt_request_digest"`
	ActualToolSurfaceDigest       core.Digest                           `json:"actual_tool_surface_digest"`
	ActualProviderInjectionDigest core.Digest                           `json:"actual_provider_injection_digest"`
	CheckedUnixNano               int64                                 `json:"checked_unix_nano"`
}

// PreparedModelInvocationCommitGateV1 is the only public Model Gate method
// set. The implementation is supplied by Harness/host; Model owns only this
// nominal, canonical validation and the dispatch guard.
type PreparedModelInvocationCommitGateV1 interface {
	Commit(context.Context, PreparedModelInvocationRefV1, PreparedModelInvocationCurrentRefV1) (PreparedModelInvocationCommitAckV1, error)
	InspectExactAck(context.Context, PreparedModelInvocationCommitAckRefV1) (PreparedModelInvocationCommitAckV1, error)
}

func (r PreparedModelInvocationCapabilitySnapshotRefV1) Validate() error {
	return validatePreparedExactRefV1(r.ContractVersion, r.ID, r.Revision, r.Digest, nil)
}

func (r PreparedModelInvocationGateImplementationRefV1) Validate() error {
	return validatePreparedExactRefV1(r.ContractVersion, r.ID, r.Revision, r.Digest, &r.Owner)
}

func (r PreparedModelInvocationSurfaceBindingRefV1) Validate() error {
	return validatePreparedExactRefV1(r.ContractVersion, r.ID, r.Revision, r.Digest, &r.Owner)
}

func (r PreparedModelInvocationRefV1) Validate() error {
	if r.ContractVersion != PreparedModelInvocationContractVersionV1 || blankPreparedV1(r.ID) || r.Revision != 1 || blankPreparedV1(r.InvocationID) {
		return preparedInvalidV1("prepared Ref identity is invalid")
	}
	for _, digest := range []core.Digest{r.Digest, r.InvocationDigest, r.UnifiedRequestDigest} {
		if err := digest.Validate(); err != nil {
			return preparedInvalidV1("prepared Ref digest is invalid")
		}
	}
	if r.InvocationDigest != r.UnifiedRequestDigest {
		return preparedConflictV1("prepared Ref invocation digest differs from request digest")
	}
	expectedID, err := preparedIdentityV1(r.InvocationID, r.InvocationDigest)
	if err != nil || expectedID != r.ID {
		return preparedConflictV1("prepared Ref ID drifted")
	}
	return nil
}

func (f PreparedModelInvocationFactV1) Ref() PreparedModelInvocationRefV1 {
	return PreparedModelInvocationRefV1{
		ContractVersion:      f.ContractVersion,
		ID:                   f.ID,
		Revision:             f.Revision,
		Digest:               f.Digest,
		InvocationID:         f.InvocationID,
		InvocationDigest:     f.InvocationDigest,
		UnifiedRequestDigest: f.UnifiedRequestDigest,
	}
}

func (f PreparedModelInvocationFactV1) Clone() PreparedModelInvocationFactV1 { return f }

func (f PreparedModelInvocationFactV1) Validate() error {
	if err := validatePreparedFactInputsV1(f); err != nil {
		return err
	}
	if err := f.Ref().Validate(); err != nil {
		return err
	}
	expected, err := digestPreparedFactV1(f)
	if err != nil || expected != f.Digest {
		return preparedConflictV1("prepared Fact digest drifted")
	}
	return nil
}

func SealPreparedModelInvocationFactV1(f PreparedModelInvocationFactV1) (PreparedModelInvocationFactV1, error) {
	if f.ContractVersion != "" && f.ContractVersion != PreparedModelInvocationContractVersionV1 {
		return PreparedModelInvocationFactV1{}, preparedInvalidV1("prepared Fact version is invalid")
	}
	f.ContractVersion = PreparedModelInvocationContractVersionV1
	if f.Revision != 0 && f.Revision != 1 {
		return PreparedModelInvocationFactV1{}, preparedInvalidV1("prepared Fact revision is invalid")
	}
	f.Revision = 1
	if f.InvocationDigest != f.UnifiedRequestDigest {
		return PreparedModelInvocationFactV1{}, preparedConflictV1("invocation and unified request digests differ")
	}
	expectedID, err := preparedIdentityV1(f.InvocationID, f.InvocationDigest)
	if err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	if f.ID != "" && f.ID != expectedID {
		return PreparedModelInvocationFactV1{}, preparedConflictV1("supplied prepared Fact ID drifted")
	}
	f.ID = expectedID
	providedDigest := f.Digest
	f.Digest = ""
	if err := validatePreparedFactInputsV1(f); err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	f.Digest, err = digestPreparedFactV1(f)
	if err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	if providedDigest != "" && providedDigest != f.Digest {
		return PreparedModelInvocationFactV1{}, preparedConflictV1("supplied prepared Fact digest drifted")
	}
	return f, f.Validate()
}

func (p PreparedModelInvocationCurrentProjectionV1) Ref() PreparedModelInvocationCurrentRefV1 {
	return PreparedModelInvocationCurrentRefV1{
		ContractVersion:  p.ContractVersion,
		ID:               p.ID,
		Revision:         p.Revision,
		Digest:           p.Digest,
		Prepared:         p.Prepared,
		CheckedUnixNano:  p.CheckedUnixNano,
		ExpiresUnixNano:  p.ExpiresUnixNano,
		NotAfterUnixNano: p.NotAfterUnixNano,
	}
}

func (p PreparedModelInvocationCurrentProjectionV1) Clone() PreparedModelInvocationCurrentProjectionV1 {
	return p
}

func (r PreparedModelInvocationCurrentRefV1) Validate() error {
	if r.ContractVersion != PreparedModelInvocationCurrentContractVersionV1 || blankPreparedV1(r.ID) || r.Revision != 1 {
		return preparedInvalidV1("prepared Current Ref identity is invalid")
	}
	if r.CheckedUnixNano <= 0 || r.CheckedUnixNano >= r.ExpiresUnixNano || r.ExpiresUnixNano > r.NotAfterUnixNano {
		return preparedInvalidV1("prepared Current Ref time bounds are invalid")
	}
	if err := r.Prepared.Validate(); err != nil {
		return err
	}
	if err := r.Digest.Validate(); err != nil {
		return preparedInvalidV1("prepared Current Ref digest is invalid")
	}
	expectedID, err := preparedCurrentIdentityV1(r.Prepared)
	if err != nil || expectedID != r.ID {
		return preparedConflictV1("prepared Current Ref ID drifted")
	}
	return nil
}

func (p PreparedModelInvocationCurrentProjectionV1) Validate() error {
	if err := validatePreparedCurrentInputsV1(p); err != nil {
		return err
	}
	if err := p.Ref().Validate(); err != nil {
		return err
	}
	expected, err := digestPreparedCurrentV1(p)
	if err != nil || expected != p.Digest {
		return preparedConflictV1("prepared Current digest drifted")
	}
	return nil
}

func (p PreparedModelInvocationCurrentProjectionV1) ValidateAgainstFact(f PreparedModelInvocationFactV1) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Prepared != f.Ref() || p.CapabilitySnapshotRef != f.CapabilitySnapshotRef || p.RegistrySnapshotRef != f.RegistrySnapshotRef ||
		p.ActualToolSurfaceDigest != f.ActualToolSurfaceDigest || p.ActualProviderInjectionDigest != f.ActualProviderInjectionDigest ||
		p.CheckedUnixNano < f.CreatedUnixNano || p.NotAfterUnixNano != f.NotAfterUnixNano {
		return preparedConflictV1("prepared Current lineage differs from Historical Fact")
	}
	return nil
}

func (p PreparedModelInvocationCurrentProjectionV1) ValidateCurrent(expected PreparedModelInvocationCurrentRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if p.Ref() != expected {
		return preparedConflictV1("prepared Current exact Ref drifted")
	}
	return validatePreparedCurrentTimeV1(p.CheckedUnixNano, p.ExpiresUnixNano, now)
}

func SealPreparedModelInvocationCurrentV1(p PreparedModelInvocationCurrentProjectionV1) (PreparedModelInvocationCurrentProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != PreparedModelInvocationCurrentContractVersionV1 {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedInvalidV1("prepared Current version is invalid")
	}
	p.ContractVersion = PreparedModelInvocationCurrentContractVersionV1
	if p.Revision != 0 && p.Revision != 1 {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedInvalidV1("prepared Current revision is invalid")
	}
	p.Revision = 1
	expectedID, err := preparedCurrentIdentityV1(p.Prepared)
	if err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	if p.ID != "" && p.ID != expectedID {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedConflictV1("supplied prepared Current ID drifted")
	}
	p.ID = expectedID
	providedDigest := p.Digest
	p.Digest = ""
	if err := validatePreparedCurrentInputsV1(p); err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	p.Digest, err = digestPreparedCurrentV1(p)
	if err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	if providedDigest != "" && providedDigest != p.Digest {
		return PreparedModelInvocationCurrentProjectionV1{}, preparedConflictV1("supplied prepared Current digest drifted")
	}
	return p, p.Validate()
}

func (a PreparedModelInvocationCommitAckV1) Ref() PreparedModelInvocationCommitAckRefV1 {
	return PreparedModelInvocationCommitAckRefV1{
		ContractVersion:   a.ContractVersion,
		ID:                a.ID,
		Revision:          a.Revision,
		Digest:            a.Digest,
		PreparedRef:       a.PreparedRef,
		CurrentRef:        a.CurrentRef,
		SurfaceBindingRef: a.SurfaceBindingRef,
		CheckedUnixNano:   a.CheckedUnixNano,
		ExpiresUnixNano:   a.ExpiresUnixNano,
		NotAfterUnixNano:  a.NotAfterUnixNano,
	}
}

func (a PreparedModelInvocationCommitAckV1) Clone() PreparedModelInvocationCommitAckV1 { return a }

func (r PreparedModelInvocationCommitAckRefV1) Validate() error {
	if err := validatePreparedAckRefInputsV1(r, true); err != nil {
		return err
	}
	expectedID, err := preparedAckIdentityV1(r.PreparedRef, r.CurrentRef)
	if err != nil || expectedID != r.ID {
		return preparedConflictV1("commit ACK Ref ID drifted")
	}
	return nil
}

func (a PreparedModelInvocationCommitAckV1) Validate() error {
	if err := a.Ref().Validate(); err != nil {
		return err
	}
	if err := a.GateImplementationRef.Validate(); err != nil {
		return err
	}
	expected, err := digestPreparedAckV1(a)
	if err != nil || expected != a.Digest {
		return preparedConflictV1("commit ACK digest drifted")
	}
	return nil
}

func (a PreparedModelInvocationCommitAckV1) ValidateCurrent(current PreparedModelInvocationCurrentProjectionV1, now time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if err := current.ValidateCurrent(a.CurrentRef, now); err != nil {
		return err
	}
	if a.PreparedRef != current.Prepared || a.CheckedUnixNano < current.CheckedUnixNano ||
		a.ExpiresUnixNano > current.ExpiresUnixNano || a.NotAfterUnixNano != current.NotAfterUnixNano {
		return preparedConflictV1("commit ACK current lineage drifted")
	}
	return validatePreparedCurrentTimeV1(a.CheckedUnixNano, a.ExpiresUnixNano, now)
}

func SealPreparedModelInvocationCommitAckV1(a PreparedModelInvocationCommitAckV1) (PreparedModelInvocationCommitAckV1, error) {
	if a.ContractVersion != "" && a.ContractVersion != PreparedModelInvocationCommitAckContractVersionV1 {
		return PreparedModelInvocationCommitAckV1{}, preparedInvalidV1("commit ACK version is invalid")
	}
	a.ContractVersion = PreparedModelInvocationCommitAckContractVersionV1
	if a.Revision != 0 && a.Revision != 1 {
		return PreparedModelInvocationCommitAckV1{}, preparedInvalidV1("commit ACK revision is invalid")
	}
	a.Revision = 1
	expectedID, err := preparedAckIdentityV1(a.PreparedRef, a.CurrentRef)
	if err != nil {
		return PreparedModelInvocationCommitAckV1{}, err
	}
	if a.ID != "" && a.ID != expectedID {
		return PreparedModelInvocationCommitAckV1{}, preparedConflictV1("supplied commit ACK ID drifted")
	}
	a.ID = expectedID
	providedDigest := a.Digest
	a.Digest = ""
	if err := validatePreparedAckRefInputsV1(a.Ref(), false); err != nil {
		return PreparedModelInvocationCommitAckV1{}, err
	}
	if err := a.GateImplementationRef.Validate(); err != nil {
		return PreparedModelInvocationCommitAckV1{}, err
	}
	a.Digest, err = digestPreparedAckV1(a)
	if err != nil {
		return PreparedModelInvocationCommitAckV1{}, err
	}
	if providedDigest != "" && providedDigest != a.Digest {
		return PreparedModelInvocationCommitAckV1{}, preparedConflictV1("supplied commit ACK digest drifted")
	}
	return a, a.Validate()
}

func (r PreparedModelInvocationDispatchValidationReceiptV1) Clone() PreparedModelInvocationDispatchValidationReceiptV1 {
	return r
}

func (r PreparedModelInvocationDispatchValidationReceiptV1) Validate() error {
	if err := validatePreparedReceiptInputsV1(r, true); err != nil {
		return err
	}
	expectedID, err := preparedReceiptIdentityV1(r.PreparedRef, r.DispatchSequence)
	if err != nil || expectedID != r.ID {
		return preparedConflictV1("dispatch validation receipt ID drifted")
	}
	expected, err := digestPreparedReceiptV1(r)
	if err != nil || expected != r.Digest {
		return preparedConflictV1("dispatch validation receipt digest drifted")
	}
	return nil
}

func SealPreparedModelInvocationDispatchReceiptV1(r PreparedModelInvocationDispatchValidationReceiptV1) (PreparedModelInvocationDispatchValidationReceiptV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != PreparedModelInvocationDispatchReceiptVersionV1 {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, preparedInvalidV1("dispatch receipt version is invalid")
	}
	r.ContractVersion = PreparedModelInvocationDispatchReceiptVersionV1
	if r.Revision != 0 && r.Revision != 1 {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, preparedInvalidV1("dispatch receipt revision is invalid")
	}
	r.Revision = 1
	expectedID, err := preparedReceiptIdentityV1(r.PreparedRef, r.DispatchSequence)
	if err != nil {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, err
	}
	if r.ID != "" && r.ID != expectedID {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, preparedConflictV1("supplied dispatch receipt ID drifted")
	}
	r.ID = expectedID
	providedDigest := r.Digest
	r.Digest = ""
	if err := validatePreparedReceiptInputsV1(r, false); err != nil {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, err
	}
	r.Digest, err = digestPreparedReceiptV1(r)
	if err != nil {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, err
	}
	if providedDigest != "" && providedDigest != r.Digest {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, preparedConflictV1("supplied dispatch receipt digest drifted")
	}
	return r, r.Validate()
}

// SealPreparedModelInvocationDispatchReceiptAgainstV1 performs the complete
// M0 dispatch guard over Historical, Current and ACK before sealing a receipt.
// It performs no provider, backend, Tool, Harness or Runtime implementation call.
func SealPreparedModelInvocationDispatchReceiptAgainstV1(
	fact PreparedModelInvocationFactV1,
	current PreparedModelInvocationCurrentProjectionV1,
	ack PreparedModelInvocationCommitAckV1,
	draft PreparedModelInvocationDispatchValidationReceiptV1,
	now time.Time,
) (PreparedModelInvocationDispatchValidationReceiptV1, error) {
	if err := current.ValidateAgainstFact(fact); err != nil {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, err
	}
	if err := ack.ValidateCurrent(current, now); err != nil {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, err
	}
	if now.IsZero() || draft.CheckedUnixNano != now.UnixNano() {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, preparedInvalidV1("dispatch receipt checked time must equal guard time")
	}
	if draft.PreparedRef != fact.Ref() || draft.CurrentRef != current.Ref() || draft.AckRef != ack.Ref() ||
		draft.ActualToolSurfaceDigest != fact.ActualToolSurfaceDigest ||
		draft.ActualProviderInjectionDigest != fact.ActualProviderInjectionDigest {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, preparedConflictV1("dispatch receipt inputs drifted from prepared invocation")
	}
	return SealPreparedModelInvocationDispatchReceiptV1(draft)
}

func EncodePreparedModelInvocationFactV1(f PreparedModelInvocationFactV1) (json.RawMessage, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return marshalCanonicalPreparedV1(f)
}

func DecodePreparedModelInvocationFactV1(raw json.RawMessage) (PreparedModelInvocationFactV1, error) {
	var fact PreparedModelInvocationFactV1
	if err := decodeCanonicalPreparedV1(raw, &fact); err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	return fact.Clone(), fact.Validate()
}

func EncodePreparedModelInvocationCurrentV1(p PreparedModelInvocationCurrentProjectionV1) (json.RawMessage, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return marshalCanonicalPreparedV1(p)
}

func DecodePreparedModelInvocationCurrentV1(raw json.RawMessage) (PreparedModelInvocationCurrentProjectionV1, error) {
	var current PreparedModelInvocationCurrentProjectionV1
	if err := decodeCanonicalPreparedV1(raw, &current); err != nil {
		return PreparedModelInvocationCurrentProjectionV1{}, err
	}
	return current.Clone(), current.Validate()
}

func EncodePreparedModelInvocationCommitAckV1(a PreparedModelInvocationCommitAckV1) (json.RawMessage, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return marshalCanonicalPreparedV1(a)
}

func DecodePreparedModelInvocationCommitAckV1(raw json.RawMessage) (PreparedModelInvocationCommitAckV1, error) {
	var ack PreparedModelInvocationCommitAckV1
	if err := decodeCanonicalPreparedV1(raw, &ack); err != nil {
		return PreparedModelInvocationCommitAckV1{}, err
	}
	return ack.Clone(), ack.Validate()
}

func EncodePreparedModelInvocationDispatchReceiptV1(r PreparedModelInvocationDispatchValidationReceiptV1) (json.RawMessage, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return marshalCanonicalPreparedV1(r)
}

func DecodePreparedModelInvocationDispatchReceiptV1(raw json.RawMessage) (PreparedModelInvocationDispatchValidationReceiptV1, error) {
	var receipt PreparedModelInvocationDispatchValidationReceiptV1
	if err := decodeCanonicalPreparedV1(raw, &receipt); err != nil {
		return PreparedModelInvocationDispatchValidationReceiptV1{}, err
	}
	return receipt.Clone(), receipt.Validate()
}

// PrepareModelInvocationFactV1 exact-reads and pins the Runtime-neutral
// Registry coordinate before sealing. The Reader is a framework dependency,
// never a provider-controlled method.
func PrepareModelInvocationFactV1(
	ctx context.Context,
	reader runtimeports.RegistrySnapshotExactReaderV1,
	draft PreparedModelInvocationFactV1,
) (PreparedModelInvocationFactV1, error) {
	if ctx == nil || nilLikePreparedV1(reader) {
		return PreparedModelInvocationFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Registry snapshot exact Reader is required")
	}
	if err := draft.RegistrySnapshotRef.Validate(); err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	observed, err := reader.InspectExactRegistrySnapshotV1(ctx, draft.RegistrySnapshotRef)
	if err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	if err := observed.Validate(); err != nil {
		return PreparedModelInvocationFactV1{}, err
	}
	if observed != draft.RegistrySnapshotRef {
		return PreparedModelInvocationFactV1{}, preparedConflictV1("Registry snapshot exact Reader returned another Ref")
	}
	draft.RegistrySnapshotRef = observed
	return SealPreparedModelInvocationFactV1(draft)
}

// CrossPreparedModelInvocationCommitGateV1 calls Commit exactly once. If the
// outcome is Indeterminate, recovery is allowed only when Commit also returned
// a complete canonical ACK; its stable exact Ref is then inspected once. The
// helper never blindly repeats Commit.
func CrossPreparedModelInvocationCommitGateV1(
	ctx context.Context,
	gate PreparedModelInvocationCommitGateV1,
	prepared PreparedModelInvocationRefV1,
	current PreparedModelInvocationCurrentRefV1,
) (PreparedModelInvocationCommitAckV1, error) {
	if ctx == nil || nilLikePreparedV1(gate) {
		return PreparedModelInvocationCommitAckV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "prepared invocation CommitGate is required")
	}
	if err := validatePreparedGateInputsV1(prepared, current); err != nil {
		return PreparedModelInvocationCommitAckV1{}, err
	}
	ack, commitErr := gate.Commit(ctx, prepared, current)
	if commitErr == nil {
		return requireExactPreparedAckV1(prepared, current, ack)
	}
	if !core.HasCategory(commitErr, core.ErrorIndeterminate) {
		return PreparedModelInvocationCommitAckV1{}, commitErr
	}
	if err := ack.Validate(); err != nil {
		return PreparedModelInvocationCommitAckV1{}, errors.Join(commitErr, preparedConflictV1("indeterminate Commit did not return a stable exact ACK Ref"))
	}
	if ack.PreparedRef != prepared || ack.CurrentRef != current {
		return PreparedModelInvocationCommitAckV1{}, errors.Join(commitErr, preparedConflictV1("indeterminate Commit ACK coordinate drifted"))
	}
	recovered, inspectErr := gate.InspectExactAck(ctx, ack.Ref())
	if inspectErr != nil {
		return PreparedModelInvocationCommitAckV1{}, errors.Join(commitErr, inspectErr)
	}
	if recovered.Ref() != ack.Ref() {
		return PreparedModelInvocationCommitAckV1{}, errors.Join(commitErr, preparedConflictV1("lost-reply ACK exact read drifted"))
	}
	return requireExactPreparedAckV1(prepared, current, recovered)
}

func InspectPreparedModelInvocationCommitAckV1(
	ctx context.Context,
	gate PreparedModelInvocationCommitGateV1,
	ref PreparedModelInvocationCommitAckRefV1,
) (PreparedModelInvocationCommitAckV1, error) {
	if ctx == nil || nilLikePreparedV1(gate) {
		return PreparedModelInvocationCommitAckV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "prepared invocation CommitGate is required")
	}
	if err := ref.Validate(); err != nil {
		return PreparedModelInvocationCommitAckV1{}, err
	}
	ack, err := gate.InspectExactAck(ctx, ref)
	if err != nil {
		return PreparedModelInvocationCommitAckV1{}, err
	}
	if err := ack.Validate(); err != nil {
		return PreparedModelInvocationCommitAckV1{}, err
	}
	if ack.Ref() != ref {
		return PreparedModelInvocationCommitAckV1{}, preparedConflictV1("CommitGate exact read drifted")
	}
	return ack.Clone(), nil
}

func validatePreparedFactInputsV1(f PreparedModelInvocationFactV1) error {
	if f.ContractVersion != PreparedModelInvocationContractVersionV1 || blankPreparedV1(f.ID) || f.Revision != 1 || blankPreparedV1(f.InvocationID) {
		return preparedInvalidV1("prepared Fact identity is invalid")
	}
	if f.CreatedUnixNano <= 0 || f.CreatedUnixNano >= f.NotAfterUnixNano {
		return preparedInvalidV1("prepared Fact time bounds are invalid")
	}
	for _, digest := range []core.Digest{
		f.InvocationDigest,
		f.UnifiedRequestDigest,
		f.RequestToolsDigest,
		f.PreparedPlanDigest,
		f.RouteDigest,
		f.ProfileDigest,
		f.ActualToolSurfaceDigest,
		f.ActualProviderInjectionDigest,
	} {
		if err := digest.Validate(); err != nil {
			return preparedInvalidV1("prepared Fact contains an invalid digest")
		}
	}
	if f.InvocationDigest != f.UnifiedRequestDigest {
		return preparedConflictV1("invocation digest differs from unified request digest")
	}
	if err := f.CapabilitySnapshotRef.Validate(); err != nil {
		return err
	}
	if err := f.RegistrySnapshotRef.Validate(); err != nil {
		return err
	}
	return validatePreparedRegistryRefTextV1(f.RegistrySnapshotRef)
}

func validatePreparedCurrentInputsV1(p PreparedModelInvocationCurrentProjectionV1) error {
	if p.ContractVersion != PreparedModelInvocationCurrentContractVersionV1 || blankPreparedV1(p.ID) || p.Revision != 1 {
		return preparedInvalidV1("prepared Current identity is invalid")
	}
	if p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || p.ExpiresUnixNano > p.NotAfterUnixNano {
		return preparedInvalidV1("prepared Current time bounds are invalid")
	}
	if err := p.Prepared.Validate(); err != nil {
		return err
	}
	if err := p.CapabilitySnapshotRef.Validate(); err != nil {
		return err
	}
	if err := p.RegistrySnapshotRef.Validate(); err != nil {
		return err
	}
	if err := validatePreparedRegistryRefTextV1(p.RegistrySnapshotRef); err != nil {
		return err
	}
	for _, digest := range []core.Digest{p.ActualToolSurfaceDigest, p.ActualProviderInjectionDigest} {
		if err := digest.Validate(); err != nil {
			return preparedInvalidV1("prepared Current digest is invalid")
		}
	}
	return nil
}

func validatePreparedAckRefInputsV1(r PreparedModelInvocationCommitAckRefV1, requireDigest bool) error {
	if r.ContractVersion != PreparedModelInvocationCommitAckContractVersionV1 || blankPreparedV1(r.ID) || r.Revision != 1 {
		return preparedInvalidV1("commit ACK Ref identity is invalid")
	}
	if r.CheckedUnixNano <= 0 || r.CheckedUnixNano >= r.ExpiresUnixNano || r.ExpiresUnixNano > r.NotAfterUnixNano {
		return preparedInvalidV1("commit ACK Ref time bounds are invalid")
	}
	if err := r.PreparedRef.Validate(); err != nil {
		return err
	}
	if err := r.CurrentRef.Validate(); err != nil {
		return err
	}
	if err := r.SurfaceBindingRef.Validate(); err != nil {
		return err
	}
	if r.CurrentRef.Prepared != r.PreparedRef || r.CheckedUnixNano < r.CurrentRef.CheckedUnixNano ||
		r.NotAfterUnixNano != r.CurrentRef.NotAfterUnixNano || r.ExpiresUnixNano > r.CurrentRef.ExpiresUnixNano {
		return preparedConflictV1("commit ACK Ref lineage drifted")
	}
	if requireDigest {
		if err := r.Digest.Validate(); err != nil {
			return preparedInvalidV1("commit ACK Ref digest is invalid")
		}
	} else if r.Digest != "" {
		return preparedInvalidV1("unsealed commit ACK must not carry a digest")
	}
	return nil
}

func validatePreparedReceiptInputsV1(r PreparedModelInvocationDispatchValidationReceiptV1, requireDigest bool) error {
	if r.ContractVersion != PreparedModelInvocationDispatchReceiptVersionV1 || blankPreparedV1(r.ID) || r.Revision != 1 ||
		r.DispatchSequence == 0 || blankPreparedV1(r.BoundaryKind) || r.ProviderAttemptOrdinal == 0 || r.CheckedUnixNano <= 0 {
		return preparedInvalidV1("dispatch receipt fields are invalid")
	}
	if err := r.PreparedRef.Validate(); err != nil {
		return err
	}
	if err := r.CurrentRef.Validate(); err != nil {
		return err
	}
	if err := r.AckRef.Validate(); err != nil {
		return err
	}
	if r.CurrentRef.Prepared != r.PreparedRef || r.AckRef.PreparedRef != r.PreparedRef || r.AckRef.CurrentRef != r.CurrentRef {
		return preparedConflictV1("dispatch validation receipt lineage drifted")
	}
	if r.CheckedUnixNano < r.AckRef.CheckedUnixNano || r.CheckedUnixNano >= r.AckRef.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "dispatch receipt lies outside ACK current window")
	}
	for _, digest := range []core.Digest{r.AttemptRequestDigest, r.ActualToolSurfaceDigest, r.ActualProviderInjectionDigest} {
		if err := digest.Validate(); err != nil {
			return preparedInvalidV1("dispatch receipt digest is invalid")
		}
	}
	if requireDigest {
		if err := r.Digest.Validate(); err != nil {
			return preparedInvalidV1("dispatch receipt content digest is invalid")
		}
	} else if r.Digest != "" {
		return preparedInvalidV1("unsealed dispatch receipt must not carry a digest")
	}
	return nil
}

func validatePreparedExactRefV1(version, id string, revision core.Revision, digest core.Digest, owner *core.OwnerRef) error {
	if blankPreparedV1(version) || blankPreparedV1(id) || revision == 0 {
		return preparedInvalidV1("exact Ref is invalid")
	}
	if err := digest.Validate(); err != nil {
		return preparedInvalidV1("exact Ref digest is invalid")
	}
	if owner != nil {
		if err := owner.Validate(); err != nil {
			return err
		}
		if blankPreparedV1(owner.Domain) || blankPreparedV1(string(owner.ID)) {
			return preparedInvalidV1("exact Ref owner text is invalid")
		}
	}
	return nil
}

func validatePreparedRegistryRefTextV1(ref runtimeports.RegistrySnapshotRefV1) error {
	if blankPreparedV1(ref.ContractVersion) || blankPreparedV1(ref.ID) || blankPreparedV1(ref.Owner.Domain) || blankPreparedV1(string(ref.Owner.ID)) {
		return preparedInvalidV1("Registry snapshot Ref contains invalid text")
	}
	return nil
}

func validatePreparedCurrentTimeV1(checkedUnixNano, expiresUnixNano int64, now time.Time) error {
	if now.IsZero() {
		return preparedInvalidV1("current validation time is required")
	}
	if now.UnixNano() < checkedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "prepared invocation clock regressed")
	}
	if !now.Before(time.Unix(0, expiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "prepared invocation current proof expired")
	}
	return nil
}

func validatePreparedGateInputsV1(prepared PreparedModelInvocationRefV1, current PreparedModelInvocationCurrentRefV1) error {
	if err := prepared.Validate(); err != nil {
		return err
	}
	if err := current.Validate(); err != nil {
		return err
	}
	if current.Prepared != prepared {
		return preparedConflictV1("CommitGate inputs differ")
	}
	return nil
}

func requireExactPreparedAckV1(
	prepared PreparedModelInvocationRefV1,
	current PreparedModelInvocationCurrentRefV1,
	ack PreparedModelInvocationCommitAckV1,
) (PreparedModelInvocationCommitAckV1, error) {
	if err := ack.Validate(); err != nil {
		return PreparedModelInvocationCommitAckV1{}, err
	}
	if ack.PreparedRef != prepared || ack.CurrentRef != current {
		return PreparedModelInvocationCommitAckV1{}, preparedConflictV1("CommitGate returned another coordinate")
	}
	return ack.Clone(), nil
}

func preparedIdentityV1(invocationID string, digest core.Digest) (string, error) {
	if blankPreparedV1(invocationID) || digest.Validate() != nil {
		return "", preparedInvalidV1("invocation coordinate is invalid")
	}
	identity, err := core.CanonicalJSONDigest(
		preparedModelInvocationCanonicalDomainV1,
		"v1",
		"PreparedModelInvocationIdentityV1",
		struct {
			ContractVersion  string      `json:"contract_version"`
			InvocationID     string      `json:"invocation_id"`
			InvocationDigest core.Digest `json:"invocation_digest"`
		}{
			ContractVersion:  PreparedModelInvocationContractVersionV1,
			InvocationID:     invocationID,
			InvocationDigest: digest,
		},
	)
	if err != nil {
		return "", err
	}
	return "prepared-model-invocation/" + strings.TrimPrefix(string(identity), "sha256:"), nil
}

func preparedCurrentIdentityV1(ref PreparedModelInvocationRefV1) (string, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(preparedModelInvocationCanonicalDomainV1, "v1", "PreparedModelInvocationCurrentIdentityV1", ref)
	if err != nil {
		return "", err
	}
	return "prepared-model-invocation-current/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func preparedAckIdentityV1(prepared PreparedModelInvocationRefV1, current PreparedModelInvocationCurrentRefV1) (string, error) {
	if err := validatePreparedGateInputsV1(prepared, current); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(
		preparedModelInvocationCanonicalDomainV1,
		"v1",
		"PreparedModelInvocationCommitAckIdentityV1",
		struct {
			Prepared PreparedModelInvocationRefV1        `json:"prepared"`
			Current  PreparedModelInvocationCurrentRefV1 `json:"current"`
		}{Prepared: prepared, Current: current},
	)
	if err != nil {
		return "", err
	}
	return "prepared-model-invocation-ack/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func preparedReceiptIdentityV1(prepared PreparedModelInvocationRefV1, sequence uint64) (string, error) {
	if err := prepared.Validate(); err != nil {
		return "", err
	}
	if sequence == 0 {
		return "", preparedInvalidV1("dispatch sequence is required")
	}
	digest, err := core.CanonicalJSONDigest(
		preparedModelInvocationCanonicalDomainV1,
		"v1",
		"PreparedModelInvocationDispatchReceiptIdentityV1",
		struct {
			Prepared PreparedModelInvocationRefV1 `json:"prepared"`
			Sequence uint64                       `json:"sequence"`
		}{Prepared: prepared, Sequence: sequence},
	)
	if err != nil {
		return "", err
	}
	return "prepared-model-invocation-dispatch/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func digestPreparedFactV1(f PreparedModelInvocationFactV1) (core.Digest, error) {
	f.Digest = ""
	return core.CanonicalJSONDigest(preparedModelInvocationCanonicalDomainV1, "v1", "PreparedModelInvocationFactV1", f)
}

func digestPreparedCurrentV1(p PreparedModelInvocationCurrentProjectionV1) (core.Digest, error) {
	p.Digest = ""
	return core.CanonicalJSONDigest(preparedModelInvocationCanonicalDomainV1, "v1", "PreparedModelInvocationCurrentProjectionV1", p)
}

func digestPreparedAckV1(a PreparedModelInvocationCommitAckV1) (core.Digest, error) {
	a.Digest = ""
	return core.CanonicalJSONDigest(preparedModelInvocationCanonicalDomainV1, "v1", "PreparedModelInvocationCommitAckV1", a)
}

func digestPreparedReceiptV1(r PreparedModelInvocationDispatchValidationReceiptV1) (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest(preparedModelInvocationCanonicalDomainV1, "v1", "PreparedModelInvocationDispatchValidationReceiptV1", r)
}

func blankPreparedV1(value string) bool {
	return !utf8.ValidString(value) || strings.TrimSpace(value) == ""
}

func preparedInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func preparedConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}

func nilLikePreparedV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func marshalCanonicalPreparedV1(value any) (json.RawMessage, error) {
	wire, err := json.Marshal(value)
	if err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "prepared invocation value is not JSON serializable")
	}
	if len(wire) > core.MaxCanonicalDocumentBytes {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "prepared invocation document exceeds the canonical bound")
	}
	return append(json.RawMessage(nil), wire...), nil
}

func decodeCanonicalPreparedV1(raw json.RawMessage, target any) error {
	if err := core.DecodeStrictJSON(raw, target); err != nil {
		return err
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(raw, canonical) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "prepared invocation wire is not canonical JSON")
	}
	return nil
}
