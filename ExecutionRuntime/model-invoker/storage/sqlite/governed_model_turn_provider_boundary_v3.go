package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type governedModelTurnProviderBoundaryRowV3 struct {
	boundaryID                 string
	revision                   uint64
	factDigest                 string
	refDigest                  string
	turnAttemptDigest          string
	runtimeBoundaryDigest      string
	runtimeRequestDigest       string
	turnID                     string
	turnRevision               uint64
	turnDigest                 string
	ackID                      string
	ackRevision                uint64
	ackDigest                  string
	dispatchReceiptID          string
	dispatchReceiptRevision    uint64
	dispatchReceiptDigest      string
	operationDigest            string
	effectID                   string
	runtimeAttemptID           string
	dispatchSequence           uint64
	providerAttemptOrdinal     uint64
	attemptRequestDigest       string
	providerBindingSetID       string
	providerBindingSetRevision uint64
	providerComponentID        string
	providerManifestDigest     string
	providerArtifactDigest     string
	providerCapability         string
	turnExpiresUnixNano        int64
	checkedUnixNano            int64
	expiresUnixNano            int64
	projectionDigest           string
	canonicalJSON              []byte
}

func (s *Store) EnsureGovernedModelTurnProviderBoundaryFactV3(
	ctx context.Context,
	fact modelinvoker.GovernedModelTurnProviderBoundaryFactV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryMutationV3, error) {
	if err := contextErrorV1(ctx, "ensure_provider_boundary_v3"); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{}, err
	}
	if err := fact.Validate(); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"ensure_provider_boundary_v3",
			"provider boundary V3 Fact is invalid",
			err,
		)
	}
	wire, err := modelinvoker.EncodeGovernedModelTurnProviderBoundaryFactV3(fact)
	if err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"ensure_provider_boundary_v3",
			"provider boundary V3 Fact cannot be encoded",
			err,
		)
	}
	requestDigest, err := fact.RuntimeRequest.DigestV1()
	if err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"ensure_provider_boundary_v3",
			"Runtime request digest cannot be derived",
			err,
		)
	}
	ref := fact.RefV3()
	tx, err := s.beginV1(ctx, "ensure_provider_boundary_v3")
	if err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{}, err
	}
	defer func() { _ = tx.Rollback() }()
	stored, err := loadGovernedModelTurnProviderBoundaryRowV3(
		ctx,
		tx,
		`WHERE boundary_id=? AND revision=?`,
		fact.ID,
		fact.Revision,
	)
	if err == nil {
		if err := stored.validateAgainstV3(fact, wire); err != nil {
			return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{}, err
		}
		return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{
			Fact:    fact.CloneV3(),
			Applied: false,
		}, nil
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorNotFound {
		return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{}, err
	}
	provider := fact.Provider
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO governed_model_turn_v3_provider_boundary_history(
		   boundary_id,revision,fact_digest,ref_digest,
		   turn_attempt_digest,runtime_boundary_digest,runtime_request_digest,
		   turn_id,turn_revision,turn_digest,
		   ack_id,ack_revision,ack_digest,
		   dispatch_receipt_id,dispatch_receipt_revision,dispatch_receipt_digest,
		   operation_digest,effect_id,runtime_attempt_id,
		   dispatch_sequence,provider_attempt_ordinal,attempt_request_digest,
		   provider_binding_set_id,provider_binding_set_revision,
		   provider_component_id,provider_manifest_digest,
		   provider_artifact_digest,provider_capability,
		   turn_expires_unix_nano,checked_unix_nano,expires_unix_nano,
		   projection_digest,canonical_json
		 ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		fact.ID,
		fact.Revision,
		string(fact.Digest),
		string(fact.RefDigest),
		string(ref.TurnAttempt.Digest),
		string(ref.RuntimeBoundary.Digest),
		string(requestDigest),
		fact.TurnRef.ID,
		fact.TurnRef.Revision,
		string(fact.TurnRef.Digest),
		fact.DispatchReceipt.AckRef.ID,
		fact.DispatchReceipt.AckRef.Revision,
		string(fact.DispatchReceipt.AckRef.Digest),
		fact.DispatchReceipt.ID,
		fact.DispatchReceipt.Revision,
		string(fact.DispatchReceipt.Digest),
		string(ref.RuntimeBoundary.OperationDigest),
		string(ref.RuntimeBoundary.EffectID),
		ref.RuntimeBoundary.RuntimeAttempt.AttemptID,
		ref.RuntimeBoundary.DispatchSequence,
		ref.RuntimeBoundary.ProviderAttemptOrdinal,
		string(ref.RuntimeBoundary.AttemptRequestDigest),
		provider.BindingSetID,
		provider.BindingSetRevision,
		string(provider.ComponentID),
		string(provider.ManifestDigest),
		string(provider.ArtifactDigest),
		string(provider.Capability),
		fact.TurnExpiresUnixNano,
		fact.CreatedUnixNano,
		fact.ExpiresUnixNano,
		string(fact.ProjectionDigest),
		wire,
	); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{}, mapDBErrorV1(
			ctx,
			"ensure_provider_boundary_v3",
			err,
			true,
		)
	}
	if err := tx.Commit(); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorIndeterminate,
			"ensure_provider_boundary_v3",
			"provider boundary V3 commit outcome is unknown",
			err,
		)
	}
	return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{
		Fact:    fact.CloneV3(),
		Applied: true,
	}, nil
}

func (s *Store) InspectGovernedModelTurnProviderBoundaryAttemptV3(
	ctx context.Context,
	ref runtimeports.ModelProviderBoundaryCurrentRefV1,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	if err := contextErrorV1(ctx, "inspect_provider_boundary_attempt_v3"); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"inspect_provider_boundary_attempt_v3",
			"Runtime provider boundary Ref is invalid",
			err,
		)
	}
	row, err := loadGovernedModelTurnProviderBoundaryRowV3(
		ctx,
		s.db,
		`WHERE runtime_boundary_digest=?`,
		string(ref.Digest),
	)
	if err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	fact, err := row.decodeV3()
	if err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	if !runtimeports.SameModelProviderBoundaryCurrentRefV1(
		fact.RuntimeRequest.ModelBoundary,
		ref,
	) {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"inspect_provider_boundary_attempt_v3",
			"stored provider boundary differs from Runtime Ref",
			nil,
		)
	}
	return fact.CloneV3(), nil
}

func (s *Store) InspectGovernedModelTurnProviderBoundaryTurnAttemptV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnAttemptRefV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	if err := contextErrorV1(ctx, "inspect_provider_boundary_turn_attempt_v3"); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"inspect_provider_boundary_turn_attempt_v3",
			"V3 Turn Attempt Ref is invalid",
			err,
		)
	}
	row, err := loadGovernedModelTurnProviderBoundaryRowV3(
		ctx,
		s.db,
		`WHERE turn_attempt_digest=?`,
		string(ref.Digest),
	)
	if err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	fact, err := row.decodeV3()
	if err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	if fact.TurnRef.AttemptRefV3() != ref {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"inspect_provider_boundary_turn_attempt_v3",
			"stored provider boundary belongs to another V3 Turn Attempt",
			nil,
		)
	}
	return fact.CloneV3(), nil
}

func (s *Store) InspectExactGovernedModelTurnProviderBoundaryV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnProviderBoundaryRefV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	if err := contextErrorV1(ctx, "inspect_provider_boundary_v3"); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"inspect_provider_boundary_v3",
			"exact provider boundary V3 Ref is invalid",
			err,
		)
	}
	row, err := loadGovernedModelTurnProviderBoundaryRowV3(
		ctx,
		s.db,
		`WHERE boundary_id=? AND revision=?`,
		ref.ID,
		ref.Revision,
	)
	if err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	fact, err := row.decodeV3()
	if err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	if fact.RefV3() != ref {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"inspect_provider_boundary_v3",
			"stored provider boundary failed exact Ref revalidation",
			nil,
		)
	}
	return fact.CloneV3(), nil
}

func loadGovernedModelTurnProviderBoundaryRowV3(
	ctx context.Context,
	query interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	clause string,
	args ...any,
) (governedModelTurnProviderBoundaryRowV3, error) {
	const columns = `
SELECT boundary_id,revision,fact_digest,ref_digest,
       turn_attempt_digest,runtime_boundary_digest,runtime_request_digest,
       turn_id,turn_revision,turn_digest,
       ack_id,ack_revision,ack_digest,
       dispatch_receipt_id,dispatch_receipt_revision,dispatch_receipt_digest,
       operation_digest,effect_id,runtime_attempt_id,
       dispatch_sequence,provider_attempt_ordinal,attempt_request_digest,
       provider_binding_set_id,provider_binding_set_revision,
       provider_component_id,provider_manifest_digest,
       provider_artifact_digest,provider_capability,
       turn_expires_unix_nano,checked_unix_nano,expires_unix_nano,
       projection_digest,canonical_json
FROM governed_model_turn_v3_provider_boundary_history `
	var row governedModelTurnProviderBoundaryRowV3
	err := query.QueryRowContext(ctx, columns+clause, args...).Scan(
		&row.boundaryID,
		&row.revision,
		&row.factDigest,
		&row.refDigest,
		&row.turnAttemptDigest,
		&row.runtimeBoundaryDigest,
		&row.runtimeRequestDigest,
		&row.turnID,
		&row.turnRevision,
		&row.turnDigest,
		&row.ackID,
		&row.ackRevision,
		&row.ackDigest,
		&row.dispatchReceiptID,
		&row.dispatchReceiptRevision,
		&row.dispatchReceiptDigest,
		&row.operationDigest,
		&row.effectID,
		&row.runtimeAttemptID,
		&row.dispatchSequence,
		&row.providerAttemptOrdinal,
		&row.attemptRequestDigest,
		&row.providerBindingSetID,
		&row.providerBindingSetRevision,
		&row.providerComponentID,
		&row.providerManifestDigest,
		&row.providerArtifactDigest,
		&row.providerCapability,
		&row.turnExpiresUnixNano,
		&row.checkedUnixNano,
		&row.expiresUnixNano,
		&row.projectionDigest,
		&row.canonicalJSON,
	)
	if err == nil {
		return row, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return governedModelTurnProviderBoundaryRowV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorNotFound,
			"load_provider_boundary_v3",
			"provider boundary V3 Fact was not found",
			err,
		)
	}
	return governedModelTurnProviderBoundaryRowV3{}, mapDBErrorV1(
		ctx,
		"load_provider_boundary_v3",
		err,
		false,
	)
}

func (r governedModelTurnProviderBoundaryRowV3) decodeV3() (
	modelinvoker.GovernedModelTurnProviderBoundaryFactV3,
	error,
) {
	fact, err := modelinvoker.DecodeGovernedModelTurnProviderBoundaryFactV3(
		append([]byte(nil), r.canonicalJSON...),
	)
	if err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"decode_provider_boundary_v3",
			"provider boundary V3 canonical payload drifted",
			err,
		)
	}
	wire, err := modelinvoker.EncodeGovernedModelTurnProviderBoundaryFactV3(fact)
	if err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	if err := r.validateAgainstV3(fact, wire); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}, err
	}
	return fact.CloneV3(), nil
}

func (r governedModelTurnProviderBoundaryRowV3) validateAgainstV3(
	fact modelinvoker.GovernedModelTurnProviderBoundaryFactV3,
	wire []byte,
) error {
	ref := fact.RefV3()
	requestDigest, err := fact.RuntimeRequest.DigestV1()
	if err != nil ||
		r.boundaryID != fact.ID ||
		r.revision != uint64(fact.Revision) ||
		r.factDigest != string(fact.Digest) ||
		r.refDigest != string(fact.RefDigest) ||
		r.turnAttemptDigest != string(ref.TurnAttempt.Digest) ||
		r.runtimeBoundaryDigest != string(ref.RuntimeBoundary.Digest) ||
		r.runtimeRequestDigest != string(requestDigest) ||
		r.turnID != fact.TurnRef.ID ||
		r.turnRevision != uint64(fact.TurnRef.Revision) ||
		r.turnDigest != string(fact.TurnRef.Digest) ||
		r.ackID != fact.DispatchReceipt.AckRef.ID ||
		r.ackRevision != uint64(fact.DispatchReceipt.AckRef.Revision) ||
		r.ackDigest != string(fact.DispatchReceipt.AckRef.Digest) ||
		r.dispatchReceiptID != fact.DispatchReceipt.ID ||
		r.dispatchReceiptRevision != uint64(fact.DispatchReceipt.Revision) ||
		r.dispatchReceiptDigest != string(fact.DispatchReceipt.Digest) ||
		r.operationDigest != string(ref.RuntimeBoundary.OperationDigest) ||
		r.effectID != string(ref.RuntimeBoundary.EffectID) ||
		r.runtimeAttemptID != ref.RuntimeBoundary.RuntimeAttempt.AttemptID ||
		r.dispatchSequence != ref.RuntimeBoundary.DispatchSequence ||
		r.providerAttemptOrdinal != uint64(ref.RuntimeBoundary.ProviderAttemptOrdinal) ||
		r.attemptRequestDigest != string(ref.RuntimeBoundary.AttemptRequestDigest) ||
		r.providerBindingSetID != fact.Provider.BindingSetID ||
		r.providerBindingSetRevision != uint64(fact.Provider.BindingSetRevision) ||
		r.providerComponentID != string(fact.Provider.ComponentID) ||
		r.providerManifestDigest != string(fact.Provider.ManifestDigest) ||
		r.providerArtifactDigest != string(fact.Provider.ArtifactDigest) ||
		r.providerCapability != string(fact.Provider.Capability) ||
		r.turnExpiresUnixNano != fact.TurnExpiresUnixNano ||
		r.checkedUnixNano != fact.CreatedUnixNano ||
		r.expiresUnixNano != fact.ExpiresUnixNano ||
		r.projectionDigest != string(fact.ProjectionDigest) ||
		!bytes.Equal(r.canonicalJSON, wire) {
		return errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"validate_provider_boundary_v3",
			"provider boundary V3 indexed coordinates drifted",
			err,
		)
	}
	return nil
}

var _ modelinvoker.GovernedModelTurnProviderBoundaryRepositoryV3 = (*Store)(nil)
