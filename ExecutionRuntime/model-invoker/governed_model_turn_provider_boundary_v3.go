package modelinvoker

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	GovernedModelTurnProviderBoundaryContractVersionV3 = "praxis.model-invoker.governed-model-turn-provider-boundary/v3"
	GovernedModelTurnProviderBoundaryStateCrossedV3    = "provider_boundary_crossed"
	governedModelTurnProviderBoundaryOwnerDomainV3     = "praxis.model-invoker"
	governedModelTurnProviderBoundaryOwnerIDV3         = "governed-model-turn-provider-boundary-v3"
)

// GovernedModelTurnProviderBoundaryRefV3 is the Model-owned exact coordinate
// for one immutable pre-provider boundary fact. Its stable identity is derived
// only from the original V3 Turn Attempt. Every other coordinate is body and
// therefore conflicts instead of creating a sibling fact.
type GovernedModelTurnProviderBoundaryRefV3 struct {
	ContractVersion         string                                         `json:"contract_version"`
	ID                      string                                         `json:"id"`
	Revision                core.Revision                                  `json:"revision"`
	Digest                  core.Digest                                    `json:"digest"`
	TurnAttempt             GovernedModelTurnAttemptRefV3                  `json:"turn_attempt"`
	TurnRef                 GovernedModelTurnRefV3                         `json:"turn_ref"`
	AckRef                  PreparedModelInvocationCommitAckRefV1          `json:"ack_ref"`
	DispatchReceiptID       string                                         `json:"dispatch_receipt_id"`
	DispatchReceiptRevision core.Revision                                  `json:"dispatch_receipt_revision"`
	DispatchReceiptDigest   core.Digest                                    `json:"dispatch_receipt_digest"`
	RuntimeBoundary         runtimeports.ModelProviderBoundaryCurrentRefV1 `json:"runtime_boundary"`
	Provider                runtimeports.ProviderBindingRefV2              `json:"provider"`
	TurnExpiresUnixNano     int64                                          `json:"turn_expires_unix_nano"`
	CheckedUnixNano         int64                                          `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                                          `json:"expires_unix_nano"`
}

// GovernedModelTurnProviderBoundaryFactV3 records that the Model-owned
// pre-provider boundary was crossed. It grants no Provider or Runtime
// authority and contains no physical invocation method.
type GovernedModelTurnProviderBoundaryFactV3 struct {
	ContractVersion     string                                                       `json:"contract_version"`
	ID                  string                                                       `json:"id"`
	Revision            core.Revision                                                `json:"revision"`
	Digest              core.Digest                                                  `json:"digest"`
	RefDigest           core.Digest                                                  `json:"ref_digest"`
	State               string                                                       `json:"state"`
	TurnRef             GovernedModelTurnRefV3                                       `json:"turn_ref"`
	DispatchReceipt     PreparedModelInvocationDispatchValidationReceiptV1           `json:"dispatch_receipt"`
	RuntimeRequest      runtimeports.InspectCurrentModelProviderActualPointRequestV1 `json:"runtime_request"`
	Provider            runtimeports.ProviderBindingRefV2                            `json:"provider"`
	TurnExpiresUnixNano int64                                                        `json:"turn_expires_unix_nano"`
	CreatedUnixNano     int64                                                        `json:"created_unix_nano"`
	ExpiresUnixNano     int64                                                        `json:"expires_unix_nano"`
	ProjectionDigest    core.Digest                                                  `json:"projection_digest"`
}

type GovernedModelTurnProviderBoundaryDraftV3 struct {
	TurnRef         GovernedModelTurnRefV3
	DispatchReceipt PreparedModelInvocationDispatchValidationReceiptV1
	RuntimeRequest  runtimeports.InspectCurrentModelProviderActualPointRequestV1
	Provider        runtimeports.ProviderBindingRefV2
	CheckedUnixNano int64
}

type PreparedModelInvocationCommitAckReaderV1 interface {
	InspectExactAck(context.Context, PreparedModelInvocationCommitAckRefV1) (PreparedModelInvocationCommitAckV1, error)
}

type GovernedModelTurnProviderBoundaryVerificationReadersV3 struct {
	TurnHistory     GovernedModelTurnRepositoryV3
	PreparedHistory PreparedModelInvocationReaderV1
	PreparedCurrent PreparedModelInvocationCurrentReaderV1
	AckHistory      PreparedModelInvocationCommitAckReaderV1
}

type GovernedModelTurnProviderBoundaryMutationV3 struct {
	Fact    GovernedModelTurnProviderBoundaryFactV3 `json:"fact"`
	Applied bool                                    `json:"applied"`
}

type GovernedModelTurnProviderBoundaryPersistenceDispositionV3 string

const (
	GovernedModelTurnProviderBoundaryPersistenceCreatedV3          GovernedModelTurnProviderBoundaryPersistenceDispositionV3 = "created"
	GovernedModelTurnProviderBoundaryPersistenceExistingV3         GovernedModelTurnProviderBoundaryPersistenceDispositionV3 = "existing"
	GovernedModelTurnProviderBoundaryPersistenceRecoveredUnknownV3 GovernedModelTurnProviderBoundaryPersistenceDispositionV3 = "recovered_unknown"
)

// GovernedModelTurnProviderBoundaryPersistenceResultV3 reports only the
// create-once outcome of the Model-owned local history write. It is not a
// Provider authorization, Permit, dispatch receipt, or capability to invoke.
// A future route gateway may continue only from a known-created result in the
// same call stack after completing every remaining S3 and Runtime guard.
type GovernedModelTurnProviderBoundaryPersistenceResultV3 struct {
	Fact        GovernedModelTurnProviderBoundaryFactV3                   `json:"fact"`
	Disposition GovernedModelTurnProviderBoundaryPersistenceDispositionV3 `json:"disposition"`
}

func (r GovernedModelTurnProviderBoundaryPersistenceResultV3) Validate() error {
	if r.Fact.Validate() != nil {
		return governedInvalidV1(
			"governed model turn provider boundary V3 persistence result Fact is invalid",
		)
	}
	switch r.Disposition {
	case GovernedModelTurnProviderBoundaryPersistenceCreatedV3,
		GovernedModelTurnProviderBoundaryPersistenceExistingV3,
		GovernedModelTurnProviderBoundaryPersistenceRecoveredUnknownV3:
		return nil
	default:
		return governedInvalidV1(
			"governed model turn provider boundary V3 persistence disposition is invalid",
		)
	}
}

type GovernedModelTurnProviderBoundaryRepositoryV3 interface {
	EnsureGovernedModelTurnProviderBoundaryFactV3(
		context.Context,
		GovernedModelTurnProviderBoundaryFactV3,
	) (GovernedModelTurnProviderBoundaryMutationV3, error)
	InspectGovernedModelTurnProviderBoundaryAttemptV3(
		context.Context,
		runtimeports.ModelProviderBoundaryCurrentRefV1,
	) (GovernedModelTurnProviderBoundaryFactV3, error)
	InspectExactGovernedModelTurnProviderBoundaryV3(
		context.Context,
		GovernedModelTurnProviderBoundaryRefV3,
	) (GovernedModelTurnProviderBoundaryFactV3, error)
}

// GovernedModelTurnProviderBoundaryTurnAttemptReaderV3 exposes the immutable
// winner for an original V3 Turn attempt. It lets a physical gateway reject a
// replay before resolving credentials or preparing a Provider adapter.
type GovernedModelTurnProviderBoundaryTurnAttemptReaderV3 interface {
	InspectGovernedModelTurnProviderBoundaryTurnAttemptV3(
		context.Context,
		GovernedModelTurnAttemptRefV3,
	) (GovernedModelTurnProviderBoundaryFactV3, error)
}

func GovernedModelTurnProviderBoundaryOwnerV3() core.OwnerRef {
	return core.OwnerRef{
		Domain: governedModelTurnProviderBoundaryOwnerDomainV3,
		ID:     core.OwnerID(governedModelTurnProviderBoundaryOwnerIDV3),
	}
}

func (r GovernedModelTurnProviderBoundaryRefV3) Validate() error {
	if r.ContractVersion != GovernedModelTurnProviderBoundaryContractVersionV3 ||
		strings.TrimSpace(r.ID) == "" || r.Revision != 1 ||
		r.Digest.Validate() != nil || r.TurnAttempt.Validate() != nil ||
		r.TurnRef.Validate() != nil || r.TurnRef.AttemptRefV3() != r.TurnAttempt ||
		r.AckRef.Validate() != nil ||
		strings.TrimSpace(r.DispatchReceiptID) == "" ||
		r.DispatchReceiptRevision != 1 ||
		r.DispatchReceiptDigest.Validate() != nil ||
		r.RuntimeBoundary.Validate() != nil || r.Provider.Validate() != nil ||
		r.TurnExpiresUnixNano <= 0 ||
		r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano ||
		r.ExpiresUnixNano != r.RuntimeBoundary.ExpiresUnixNano ||
		r.RuntimeBoundary.Owner != GovernedModelTurnProviderBoundaryOwnerV3() ||
		r.RuntimeBoundary.ID != r.ID ||
		r.RuntimeBoundary.Revision != r.Revision ||
		r.RuntimeBoundary.DispatchSequence != r.TurnRef.DispatchSequence ||
		r.RuntimeBoundary.ProviderAttemptOrdinal != r.TurnRef.ProviderAttemptOrdinal ||
		r.RuntimeBoundary.AttemptRequestDigest != r.TurnRef.AttemptRequestDigest ||
		r.RuntimeBoundary.AcknowledgementDigest != r.AckRef.Digest {
		return governedInvalidV1("governed model turn provider boundary V3 Ref is invalid")
	}
	id, err := governedModelTurnProviderBoundaryIdentityV3(r.TurnAttempt)
	if err != nil || id != r.ID {
		return governedConflictV1("governed model turn provider boundary V3 Ref ID drifted")
	}
	expected, err := governedModelTurnProviderBoundaryRefDigestV3(r)
	if err != nil || expected != r.Digest {
		return governedConflictV1("governed model turn provider boundary V3 Ref digest drifted")
	}
	return nil
}

func (f GovernedModelTurnProviderBoundaryFactV3) RefV3() GovernedModelTurnProviderBoundaryRefV3 {
	return GovernedModelTurnProviderBoundaryRefV3{
		ContractVersion:         f.ContractVersion,
		ID:                      f.ID,
		Revision:                f.Revision,
		Digest:                  f.RefDigest,
		TurnAttempt:             f.TurnRef.AttemptRefV3(),
		TurnRef:                 f.TurnRef,
		AckRef:                  f.DispatchReceipt.AckRef,
		DispatchReceiptID:       f.DispatchReceipt.ID,
		DispatchReceiptRevision: f.DispatchReceipt.Revision,
		DispatchReceiptDigest:   f.DispatchReceipt.Digest,
		RuntimeBoundary:         f.RuntimeRequest.ModelBoundary,
		Provider:                f.Provider,
		TurnExpiresUnixNano:     f.TurnExpiresUnixNano,
		CheckedUnixNano:         f.DispatchReceipt.CheckedUnixNano,
		ExpiresUnixNano:         f.ExpiresUnixNano,
	}
}

func (f GovernedModelTurnProviderBoundaryFactV3) RuntimeProjectionV1() (
	runtimeports.ModelProviderBoundaryCurrentProjectionV1,
	error,
) {
	return runtimeports.SealModelProviderBoundaryCurrentProjectionV1(
		runtimeports.ModelProviderBoundaryCurrentProjectionV1{
			Ref:             f.RuntimeRequest.ModelBoundary,
			State:           runtimeports.ModelProviderBoundaryCrossedV1,
			Provider:        f.Provider,
			CheckedUnixNano: f.CreatedUnixNano,
			ExpiresUnixNano: f.ExpiresUnixNano,
		},
	)
}

func (f GovernedModelTurnProviderBoundaryFactV3) Validate() error {
	if f.ContractVersion != GovernedModelTurnProviderBoundaryContractVersionV3 ||
		f.ID == "" || f.Revision != 1 ||
		f.Digest.Validate() != nil ||
		f.RefDigest.Validate() != nil ||
		f.State != GovernedModelTurnProviderBoundaryStateCrossedV3 ||
		f.TurnRef.Validate() != nil ||
		f.DispatchReceipt.Validate() != nil ||
		f.RuntimeRequest.Validate() != nil ||
		f.Provider.Validate() != nil ||
		f.Provider != f.RuntimeRequest.Verifier ||
		f.TurnExpiresUnixNano <= 0 ||
		f.CreatedUnixNano != f.DispatchReceipt.CheckedUnixNano ||
		f.ExpiresUnixNano <= f.CreatedUnixNano ||
		f.ExpiresUnixNano != f.RuntimeRequest.ModelBoundary.ExpiresUnixNano ||
		f.ProjectionDigest.Validate() != nil {
		return governedInvalidV1("governed model turn provider boundary V3 Fact is invalid")
	}
	if err := validateGovernedModelTurnProviderBoundaryLineageV3(
		f.TurnRef,
		f.DispatchReceipt,
		f.RuntimeRequest,
		f.Provider,
		f.TurnExpiresUnixNano,
		f.CreatedUnixNano,
		f.ExpiresUnixNano,
	); err != nil {
		return err
	}
	ref := f.RefV3()
	if err := ref.Validate(); err != nil ||
		ref.ID != f.ID || ref.Revision != f.Revision {
		return governedConflictV1("governed model turn provider boundary V3 Fact Ref drifted")
	}
	projection, err := f.RuntimeProjectionV1()
	if err != nil || projection.ProjectionDigest != f.ProjectionDigest {
		return governedConflictV1("governed model turn provider boundary V3 projection digest drifted")
	}
	expected, err := governedModelTurnProviderBoundaryFactDigestV3(f)
	if err != nil || expected != f.Digest {
		return governedConflictV1("governed model turn provider boundary V3 Fact digest drifted")
	}
	return nil
}

func (f GovernedModelTurnProviderBoundaryFactV3) CloneV3() GovernedModelTurnProviderBoundaryFactV3 {
	wire, _ := json.Marshal(f)
	var clone GovernedModelTurnProviderBoundaryFactV3
	_ = core.DecodeStrictJSON(wire, &clone)
	return clone
}

func EncodeGovernedModelTurnProviderBoundaryFactV3(
	fact GovernedModelTurnProviderBoundaryFactV3,
) ([]byte, error) {
	if err := fact.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(fact)
}

func DecodeGovernedModelTurnProviderBoundaryFactV3(
	wire []byte,
) (GovernedModelTurnProviderBoundaryFactV3, error) {
	var fact GovernedModelTurnProviderBoundaryFactV3
	if err := core.DecodeStrictJSON(wire, &fact); err != nil {
		return GovernedModelTurnProviderBoundaryFactV3{}, governedInvalidV1(
			"governed model turn provider boundary V3 payload is invalid",
		)
	}
	if err := fact.Validate(); err != nil {
		return GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	canonical, err := json.Marshal(fact)
	if err != nil || !reflect.DeepEqual(canonical, wire) {
		return GovernedModelTurnProviderBoundaryFactV3{}, governedConflictV1(
			"governed model turn provider boundary V3 payload is not canonical",
		)
	}
	return fact.CloneV3(), nil
}

// DeriveGovernedModelTurnProviderBoundaryRefV3 derives the Model authoritative
// boundary and verifies an optional pre-populated Runtime ModelBoundary.
func DeriveGovernedModelTurnProviderBoundaryRefV3(
	turn GovernedModelTurnOutcomeV3,
	receipt PreparedModelInvocationDispatchValidationReceiptV1,
	request runtimeports.InspectCurrentModelProviderActualPointRequestV1,
	provider runtimeports.ProviderBindingRefV2,
	checkedUnixNano int64,
) (GovernedModelTurnProviderBoundaryRefV3, error) {
	if turn.Validate() != nil || receipt.Validate() != nil ||
		provider.Validate() != nil || checkedUnixNano <= 0 {
		return GovernedModelTurnProviderBoundaryRefV3{}, governedInvalidV1(
			"governed model turn provider boundary V3 inputs are invalid",
		)
	}
	if receipt.CheckedUnixNano != checkedUnixNano ||
		receipt.PreparedRef != turn.PreparedRef ||
		receipt.CurrentRef != turn.CurrentRef ||
		receipt.DispatchSequence != turn.DispatchSequence ||
		receipt.ProviderAttemptOrdinal != turn.ProviderAttemptOrdinal ||
		receipt.AttemptRequestDigest != turn.AttemptRequestDigest ||
		receipt.AckRef.PreparedRef != turn.PreparedRef ||
		receipt.AckRef.CurrentRef != turn.CurrentRef ||
		request.Verifier != provider {
		return GovernedModelTurnProviderBoundaryRefV3{}, governedConflictV1(
			"governed model turn provider boundary V3 lineage drifted",
		)
	}
	if err := ValidateModelProviderActualPointRequestDraftV3(request); err != nil {
		return GovernedModelTurnProviderBoundaryRefV3{}, err
	}
	operationDigest, err := request.Operation.DigestV3()
	if err != nil {
		return GovernedModelTurnProviderBoundaryRefV3{}, err
	}
	id, err := governedModelTurnProviderBoundaryIdentityV3(turn.AttemptRefV3())
	if err != nil {
		return GovernedModelTurnProviderBoundaryRefV3{}, err
	}
	expires := minTimeUnixNanoMaterialV1(
		turn.ExpiresUnixNano,
		turn.CurrentRef.ExpiresUnixNano,
		turn.CurrentRef.NotAfterUnixNano,
		turn.MaterialRef.ExpiresUnixNano,
		receipt.AckRef.ExpiresUnixNano,
		receipt.AckRef.NotAfterUnixNano,
		request.RequestedNotAfterUnixNano,
	)
	runtimeBoundary, err := runtimeports.SealModelProviderBoundaryCurrentRefV1(
		runtimeports.ModelProviderBoundaryCurrentRefV1{
			Owner:                  GovernedModelTurnProviderBoundaryOwnerV3(),
			ID:                     id,
			Revision:               1,
			OperationDigest:        operationDigest,
			EffectID:               request.EffectID,
			RuntimeAttempt:         request.Attempt,
			DispatchSequence:       turn.DispatchSequence,
			ProviderAttemptOrdinal: turn.ProviderAttemptOrdinal,
			AttemptRequestDigest:   turn.AttemptRequestDigest,
			AcknowledgementDigest:  receipt.AckRef.Digest,
			ExpiresUnixNano:        expires,
		},
	)
	if err != nil {
		return GovernedModelTurnProviderBoundaryRefV3{}, err
	}
	if request.ModelBoundary != (runtimeports.ModelProviderBoundaryCurrentRefV1{}) &&
		!runtimeports.SameModelProviderBoundaryCurrentRefV1(
			request.ModelBoundary,
			runtimeBoundary,
		) {
		return GovernedModelTurnProviderBoundaryRefV3{}, governedConflictV1(
			"Runtime request carries another Model boundary",
		)
	}
	request.ModelBoundary = runtimeBoundary
	if err := request.Validate(); err != nil {
		return GovernedModelTurnProviderBoundaryRefV3{}, err
	}
	ref := GovernedModelTurnProviderBoundaryRefV3{
		ContractVersion:         GovernedModelTurnProviderBoundaryContractVersionV3,
		ID:                      id,
		Revision:                1,
		TurnAttempt:             turn.AttemptRefV3(),
		TurnRef:                 turn.RefV3(),
		AckRef:                  receipt.AckRef,
		DispatchReceiptID:       receipt.ID,
		DispatchReceiptRevision: receipt.Revision,
		DispatchReceiptDigest:   receipt.Digest,
		RuntimeBoundary:         runtimeBoundary,
		Provider:                provider,
		TurnExpiresUnixNano:     turn.ExpiresUnixNano,
		CheckedUnixNano:         checkedUnixNano,
		ExpiresUnixNano:         expires,
	}
	ref.Digest, err = governedModelTurnProviderBoundaryRefDigestV3(ref)
	if err != nil {
		return GovernedModelTurnProviderBoundaryRefV3{}, err
	}
	return ref, ref.Validate()
}

// BuildGovernedModelTurnProviderBoundaryFactV3 performs only Model-owned exact
// reads and sealing. It makes zero Provider, routegateway, Runtime guard,
// Harness, Context or Tool calls.
func BuildGovernedModelTurnProviderBoundaryFactV3(
	ctx context.Context,
	readers GovernedModelTurnProviderBoundaryVerificationReadersV3,
	draft GovernedModelTurnProviderBoundaryDraftV3,
	now time.Time,
) (GovernedModelTurnProviderBoundaryFactV3, error) {
	if ctx == nil || ctx.Err() != nil || now.IsZero() ||
		nilLikeBoundaryReaderV3(readers.TurnHistory) ||
		nilLikeBoundaryReaderV3(readers.PreparedHistory) ||
		nilLikeBoundaryReaderV3(readers.PreparedCurrent) ||
		nilLikeBoundaryReaderV3(readers.AckHistory) {
		return GovernedModelTurnProviderBoundaryFactV3{}, governedInvalidV1(
			"governed model turn provider boundary V3 readers are unavailable",
		)
	}
	if err := draft.TurnRef.Validate(); err != nil {
		return GovernedModelTurnProviderBoundaryFactV3{}, governedInvalidV1(
			"governed model turn provider boundary V3 Turn exact Ref is invalid",
		)
	}
	turn, err := readers.TurnHistory.InspectExactGovernedModelTurnV3(ctx, draft.TurnRef)
	if err != nil {
		return GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	if turn.Validate() != nil || turn.RefV3() != draft.TurnRef {
		return GovernedModelTurnProviderBoundaryFactV3{}, governedConflictV1(
			"governed model turn provider boundary V3 Turn exact Reader returned invalid or drifted content",
		)
	}
	if _, err := validateCurrentGovernedModelTurnWinnerV3(
		turn,
		turn.AttemptRefV3(),
		now,
	); err != nil {
		return GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	historical, err := readers.PreparedHistory.InspectExactPreparedModelInvocationV1(
		ctx,
		turn.PreparedRef,
	)
	if err != nil {
		return GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	current, err := readers.PreparedCurrent.InspectExactPreparedModelInvocationCurrentV1(
		ctx,
		turn.CurrentRef,
	)
	if err != nil {
		return GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	ack, err := readers.AckHistory.InspectExactAck(ctx, draft.DispatchReceipt.AckRef)
	if err != nil {
		return GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	verified, err := SealPreparedModelInvocationDispatchReceiptAgainstV1(
		historical,
		current,
		ack,
		draft.DispatchReceipt,
		time.Unix(0, draft.CheckedUnixNano),
	)
	if err != nil || verified != draft.DispatchReceipt {
		return GovernedModelTurnProviderBoundaryFactV3{}, governedConflictV1(
			"governed model turn provider boundary V3 receipt is not verified",
		)
	}
	ref, err := DeriveGovernedModelTurnProviderBoundaryRefV3(
		turn,
		verified,
		draft.RuntimeRequest,
		draft.Provider,
		draft.CheckedUnixNano,
	)
	if err != nil {
		return GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	if !now.Before(time.Unix(0, ref.ExpiresUnixNano)) ||
		now.UnixNano() < turn.CurrentRef.CheckedUnixNano ||
		draft.CheckedUnixNano != now.UnixNano() {
		return GovernedModelTurnProviderBoundaryFactV3{}, governedConflictV1(
			"governed model turn provider boundary V3 inputs are not current",
		)
	}
	request := draft.RuntimeRequest
	request.ModelBoundary = ref.RuntimeBoundary
	fact := GovernedModelTurnProviderBoundaryFactV3{
		ContractVersion:     GovernedModelTurnProviderBoundaryContractVersionV3,
		ID:                  ref.ID,
		Revision:            1,
		State:               GovernedModelTurnProviderBoundaryStateCrossedV3,
		TurnRef:             turn.RefV3(),
		DispatchReceipt:     verified,
		RuntimeRequest:      request,
		Provider:            draft.Provider,
		TurnExpiresUnixNano: turn.ExpiresUnixNano,
		CreatedUnixNano:     now.UnixNano(),
		ExpiresUnixNano:     ref.ExpiresUnixNano,
	}
	fact.RefDigest = ref.Digest
	projection, err := fact.RuntimeProjectionV1()
	if err != nil {
		return GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	fact.ProjectionDigest = projection.ProjectionDigest
	fact.Digest, err = governedModelTurnProviderBoundaryFactDigestV3(fact)
	if err != nil {
		return GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	return fact.CloneV3(), fact.Validate()
}

// EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3 preserves create-once
// semantics and reports whether this call created the immutable history row,
// observed an existing row, or recovered a commit with unknown outcome. The
// result grants no Provider invocation authority. An indeterminate or
// conflicting commit is recovered only by inspecting the pre-derived exact Ref.
func EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
	ctx context.Context,
	repository GovernedModelTurnProviderBoundaryRepositoryV3,
	fact GovernedModelTurnProviderBoundaryFactV3,
	clock func() time.Time,
) (GovernedModelTurnProviderBoundaryPersistenceResultV3, error) {
	if ctx == nil || ctx.Err() != nil || nilLikeBoundaryReaderV3(repository) ||
		fact.Validate() != nil || clock == nil {
		return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, governedInvalidV1(
			"governed model turn provider boundary V3 persistence inputs are invalid",
		)
	}
	baseline := clock()
	if err := validateCurrentGovernedModelTurnProviderBoundaryFactV3(
		fact,
		baseline,
		baseline,
	); err != nil {
		return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, err
	}
	ref := fact.RefV3()
	existing, inspectErr := repository.InspectExactGovernedModelTurnProviderBoundaryV3(
		ctx,
		ref,
	)
	if inspectErr == nil {
		if !reflect.DeepEqual(existing, fact) {
			return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, governedConflictV1(
				"governed model turn provider boundary V3 historical winner differs",
			)
		}
		if err := validateCurrentGovernedModelTurnProviderBoundaryFactV3(
			existing,
			baseline,
			clock(),
		); err != nil {
			return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, err
		}
		result := GovernedModelTurnProviderBoundaryPersistenceResultV3{
			Fact:        existing.CloneV3(),
			Disposition: GovernedModelTurnProviderBoundaryPersistenceExistingV3,
		}
		return result, result.Validate()
	}
	if GovernedModelInvocationErrorKindOfV1(inspectErr) !=
		GovernedModelInvocationErrorNotFound {
		return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, inspectErr
	}
	mutation, err := repository.EnsureGovernedModelTurnProviderBoundaryFactV3(ctx, fact)
	if err != nil {
		kind := GovernedModelInvocationErrorKindOfV1(err)
		if kind != GovernedModelInvocationErrorIndeterminate &&
			kind != GovernedModelInvocationErrorConflict {
			return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, err
		}
		recovery, cancel := context.WithDeadline(
			context.WithoutCancel(ctx),
			time.Unix(0, fact.ExpiresUnixNano),
		)
		defer cancel()
		winner, inspectExactErr :=
			repository.InspectExactGovernedModelTurnProviderBoundaryV3(recovery, ref)
		if inspectExactErr != nil {
			return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, errors.Join(
				err,
				inspectExactErr,
			)
		}
		if !reflect.DeepEqual(winner, fact) {
			return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, governedConflictV1(
				"governed model turn provider boundary V3 recovered another body",
			)
		}
		if err := validateCurrentGovernedModelTurnProviderBoundaryFactV3(
			winner,
			baseline,
			clock(),
		); err != nil {
			return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, err
		}
		result := GovernedModelTurnProviderBoundaryPersistenceResultV3{
			Fact:        winner.CloneV3(),
			Disposition: GovernedModelTurnProviderBoundaryPersistenceRecoveredUnknownV3,
		}
		return result, result.Validate()
	}
	if !reflect.DeepEqual(mutation.Fact, fact) {
		return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, governedConflictV1(
			"governed model turn provider boundary V3 repository returned another body",
		)
	}
	winner, inspectExactErr :=
		repository.InspectExactGovernedModelTurnProviderBoundaryV3(ctx, ref)
	if inspectExactErr != nil {
		return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, inspectExactErr
	}
	if !reflect.DeepEqual(winner, fact) {
		return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, governedConflictV1(
			"governed model turn provider boundary V3 persisted winner differs",
		)
	}
	if err := validateCurrentGovernedModelTurnProviderBoundaryFactV3(
		winner,
		baseline,
		clock(),
	); err != nil {
		return GovernedModelTurnProviderBoundaryPersistenceResultV3{}, err
	}
	disposition := GovernedModelTurnProviderBoundaryPersistenceExistingV3
	if mutation.Applied {
		disposition = GovernedModelTurnProviderBoundaryPersistenceCreatedV3
	}
	result := GovernedModelTurnProviderBoundaryPersistenceResultV3{
		Fact:        winner.CloneV3(),
		Disposition: disposition,
	}
	return result, result.Validate()
}

func validateCurrentGovernedModelTurnProviderBoundaryFactV3(
	fact GovernedModelTurnProviderBoundaryFactV3,
	baseline time.Time,
	current time.Time,
) error {
	if fact.Validate() != nil || baseline.IsZero() || current.IsZero() ||
		baseline.UnixNano() < fact.CreatedUnixNano ||
		current.Before(baseline) ||
		!baseline.Before(time.Unix(0, fact.ExpiresUnixNano)) ||
		!current.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return governedConflictV1(
			"governed model turn provider boundary V3 Fact is not current",
		)
	}
	return nil
}

func validateGovernedModelTurnProviderBoundaryLineageV3(
	turn GovernedModelTurnRefV3,
	receipt PreparedModelInvocationDispatchValidationReceiptV1,
	request runtimeports.InspectCurrentModelProviderActualPointRequestV1,
	provider runtimeports.ProviderBindingRefV2,
	turnExpiresUnixNano int64,
	checkedUnixNano int64,
	expiresUnixNano int64,
) error {
	if receipt.PreparedRef != turn.PreparedRef ||
		receipt.CurrentRef != turn.CurrentRef ||
		receipt.AckRef.PreparedRef != turn.PreparedRef ||
		receipt.AckRef.CurrentRef != turn.CurrentRef ||
		receipt.DispatchSequence != turn.DispatchSequence ||
		receipt.ProviderAttemptOrdinal != turn.ProviderAttemptOrdinal ||
		receipt.AttemptRequestDigest != turn.AttemptRequestDigest ||
		receipt.CheckedUnixNano != checkedUnixNano ||
		request.Verifier != provider ||
		request.ModelBoundary.ProviderAttemptOrdinal != turn.ProviderAttemptOrdinal ||
		request.ModelBoundary.DispatchSequence != turn.DispatchSequence ||
		request.ModelBoundary.AttemptRequestDigest != turn.AttemptRequestDigest ||
		request.ModelBoundary.AcknowledgementDigest != receipt.AckRef.Digest ||
		request.ModelBoundary.RuntimeAttempt != request.Attempt ||
		expiresUnixNano != minTimeUnixNanoMaterialV1(
			turnExpiresUnixNano,
			turn.CurrentRef.ExpiresUnixNano,
			turn.CurrentRef.NotAfterUnixNano,
			turn.MaterialRef.ExpiresUnixNano,
			receipt.AckRef.ExpiresUnixNano,
			receipt.AckRef.NotAfterUnixNano,
			request.RequestedNotAfterUnixNano,
			request.ModelBoundary.ExpiresUnixNano,
		) {
		return governedConflictV1(
			"governed model turn provider boundary V3 cross-owner lineage drifted",
		)
	}
	return nil
}

// ValidateModelProviderActualPointRequestDraftV3 validates every Runtime
// actual-point coordinate that exists before Model derives ModelBoundary. It
// performs no reads and does not grant dispatch authority.
func ValidateModelProviderActualPointRequestDraftV3(
	request runtimeports.InspectCurrentModelProviderActualPointRequestV1,
) error {
	boundary := request.ModelBoundary
	request.ModelBoundary = runtimeports.ModelProviderBoundaryCurrentRefV1{}
	if request.Operation.Validate() != nil || request.Operation.Kind != runtimeports.OperationScopeRunV3 ||
		request.EffectID == "" || request.ExpectedEffectRevision == 0 ||
		strings.TrimSpace(request.PermitID) == "" ||
		request.ExpectedPermitFactRevision == 0 ||
		request.PermitDigest.Validate() != nil ||
		request.AdmissionDigest.Validate() != nil ||
		request.ReviewAuthorization.Validate() != nil ||
		request.Attempt.Validate() != nil ||
		request.Verifier.Validate() != nil ||
		request.FenceDigest.Validate() != nil ||
		request.RequestedNotAfterUnixNano <= 0 {
		return governedInvalidV1("Runtime actual-point request draft is invalid")
	}
	operationDigest, err := request.Operation.DigestV3()
	if err != nil ||
		request.Attempt.OperationDigest != operationDigest ||
		request.Attempt.EffectID != request.EffectID ||
		request.Attempt.PermitID != request.PermitID {
		return governedConflictV1("Runtime actual-point request draft lineage drifted")
	}
	if boundary != (runtimeports.ModelProviderBoundaryCurrentRefV1{}) &&
		boundary.Validate() != nil {
		return governedInvalidV1("Runtime actual-point request boundary is invalid")
	}
	return nil
}

func governedModelTurnProviderBoundaryIdentityV3(
	attempt GovernedModelTurnAttemptRefV3,
) (string, error) {
	if err := attempt.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(
		"praxis.model-invoker.governed-model-turn-provider-boundary",
		"v3",
		"GovernedModelTurnProviderBoundaryIdentityV3",
		struct {
			TurnAttempt GovernedModelTurnAttemptRefV3 `json:"turn_attempt"`
		}{TurnAttempt: attempt},
	)
	if err != nil {
		return "", err
	}
	return "governed-model-turn-provider-boundary-v3/" +
		strings.TrimPrefix(string(digest), "sha256:"), nil
}

func governedModelTurnProviderBoundaryRefDigestV3(
	ref GovernedModelTurnProviderBoundaryRefV3,
) (core.Digest, error) {
	ref.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.governed-model-turn-provider-boundary",
		"v3",
		"GovernedModelTurnProviderBoundaryRefV3",
		ref,
	)
}

func governedModelTurnProviderBoundaryFactDigestV3(
	fact GovernedModelTurnProviderBoundaryFactV3,
) (core.Digest, error) {
	fact.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.governed-model-turn-provider-boundary",
		"v3",
		"GovernedModelTurnProviderBoundaryFactV3",
		fact,
	)
}

func nilLikeBoundaryReaderV3(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
