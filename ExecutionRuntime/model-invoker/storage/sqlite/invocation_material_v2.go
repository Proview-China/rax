package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func (s *Store) EnsureAuthorizedInvocationMaterialV2(
	ctx context.Context,
	request modelinvoker.InvocationMaterialPersistRequestV2,
) (modelinvoker.InvocationMaterialV2, error) {
	if err := contextErrorV1(ctx, "ensure_invocation_material_v2"); err != nil {
		return modelinvoker.InvocationMaterialV2{}, err
	}
	if err := request.ValidateV2(); err != nil {
		return modelinvoker.InvocationMaterialV2{}, err
	}
	material := request.MaterialV2()
	wire, err := modelinvoker.EncodeInvocationMaterialV2(material)
	if err != nil {
		return modelinvoker.InvocationMaterialV2{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"ensure_invocation_material_v2",
			"material cannot be encoded",
			err,
		)
	}
	tx, err := s.beginV1(ctx, "ensure_invocation_material_v2")
	if err != nil {
		return modelinvoker.InvocationMaterialV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var materialDigest, preparedID, preparedDigest, routeCallDigest string
	var sourceLineageDigest, authorizationID, authorizationDigest string
	var currentID, currentDigest string
	var preparedRevision, currentRevision uint64
	var currentCheckedUnixNano, currentExpiresUnixNano, currentNotAfterUnixNano int64
	var expiresUnixNano int64
	var stored []byte
	err = tx.QueryRowContext(
		ctx,
		`SELECT material_digest,prepared_id,prepared_revision,prepared_digest,
		        current_id,current_revision,current_digest,current_checked_unix_nano,current_expires_unix_nano,current_not_after_unix_nano,
		        route_call_digest,authorization_id,source_lineage_digest,authorization_digest,expires_unix_nano,canonical_json
		 FROM invocation_material_v2_history WHERE material_id=? AND revision=?`,
		material.ID,
		material.Revision,
	).Scan(
		&materialDigest,
		&preparedID,
		&preparedRevision,
		&preparedDigest,
		&currentID,
		&currentRevision,
		&currentDigest,
		&currentCheckedUnixNano,
		&currentExpiresUnixNano,
		&currentNotAfterUnixNano,
		&routeCallDigest,
		&authorizationID,
		&sourceLineageDigest,
		&authorizationDigest,
		&expiresUnixNano,
		&stored,
	)
	if err == nil {
		if materialDigest != string(material.Digest) ||
			preparedID != material.PreparedRef.ID ||
			preparedRevision != uint64(material.PreparedRef.Revision) ||
			preparedDigest != string(material.PreparedRef.Digest) ||
			currentID != material.Authorization.CurrentRef.ID ||
			currentRevision != uint64(material.Authorization.CurrentRef.Revision) ||
			currentDigest != string(material.Authorization.CurrentRef.Digest) ||
			currentCheckedUnixNano != material.Authorization.CurrentRef.CheckedUnixNano ||
			currentExpiresUnixNano != material.Authorization.CurrentRef.ExpiresUnixNano ||
			currentNotAfterUnixNano != material.Authorization.CurrentRef.NotAfterUnixNano ||
			routeCallDigest != string(material.RouteCallDigest) ||
			authorizationID != material.Authorization.ID ||
			sourceLineageDigest != string(material.Authorization.SourceLineage.Digest) ||
			authorizationDigest != string(material.Authorization.Digest) ||
			expiresUnixNano != material.ExpiresUnixNano ||
			!bytes.Equal(stored, wire) {
			return modelinvoker.InvocationMaterialV2{}, errorV1(
				modelinvoker.GovernedModelInvocationErrorConflict,
				"ensure_invocation_material_v2",
				"exact material contains different canonical content",
				nil,
			)
		}
		return material.CloneV2(), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.InvocationMaterialV2{}, mapDBErrorV1(
			ctx,
			"ensure_invocation_material_v2",
			err,
			false,
		)
	}
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO invocation_material_v2_history(
		   material_id,revision,material_digest,
		   prepared_id,prepared_revision,prepared_digest,
		   current_id,current_revision,current_digest,
		   current_checked_unix_nano,current_expires_unix_nano,current_not_after_unix_nano,
		   route_call_digest,authorization_id,source_lineage_digest,authorization_digest,
		   expires_unix_nano,canonical_json
		 ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		material.ID,
		material.Revision,
		string(material.Digest),
		material.PreparedRef.ID,
		material.PreparedRef.Revision,
		string(material.PreparedRef.Digest),
		material.Authorization.CurrentRef.ID,
		material.Authorization.CurrentRef.Revision,
		string(material.Authorization.CurrentRef.Digest),
		material.Authorization.CurrentRef.CheckedUnixNano,
		material.Authorization.CurrentRef.ExpiresUnixNano,
		material.Authorization.CurrentRef.NotAfterUnixNano,
		string(material.RouteCallDigest),
		material.Authorization.ID,
		string(material.Authorization.SourceLineage.Digest),
		string(material.Authorization.Digest),
		material.ExpiresUnixNano,
		wire,
	); err != nil {
		return modelinvoker.InvocationMaterialV2{}, mapDBErrorV1(
			ctx,
			"ensure_invocation_material_v2",
			err,
			true,
		)
	}
	if err = tx.Commit(); err != nil {
		return modelinvoker.InvocationMaterialV2{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorIndeterminate,
			"ensure_invocation_material_v2",
			"material commit outcome is unknown",
			err,
		)
	}
	return material.CloneV2(), nil
}

func (s *Store) InspectExactInvocationMaterialV2(
	ctx context.Context,
	ref modelinvoker.InvocationMaterialRefV2,
) (modelinvoker.InvocationMaterialV2, error) {
	if err := contextErrorV1(ctx, "inspect_invocation_material_v2"); err != nil {
		return modelinvoker.InvocationMaterialV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.InvocationMaterialV2{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"inspect_invocation_material_v2",
			"exact material Ref is invalid",
			err,
		)
	}
	material, err := inspectExactInvocationMaterialV2Query(ctx, s.db, ref.ID, uint64(ref.Revision))
	if err != nil {
		return modelinvoker.InvocationMaterialV2{}, err
	}
	if material.RefV2() != ref {
		return modelinvoker.InvocationMaterialV2{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"inspect_invocation_material_v2",
			"stored material failed exact Ref revalidation",
			nil,
		)
	}
	return material.CloneV2(), nil
}

func inspectExactInvocationMaterialV2Query(
	ctx context.Context,
	query interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	id string,
	revision uint64,
) (modelinvoker.InvocationMaterialV2, error) {
	var materialDigest, preparedID, preparedDigest, routeCallDigest string
	var sourceLineageDigest, authorizationID, authorizationDigest string
	var currentID, currentDigest string
	var preparedRevision, currentRevision uint64
	var currentCheckedUnixNano, currentExpiresUnixNano, currentNotAfterUnixNano int64
	var expiresUnixNano int64
	var payload []byte
	err := query.QueryRowContext(
		ctx,
		`SELECT material_digest,prepared_id,prepared_revision,prepared_digest,
		        current_id,current_revision,current_digest,current_checked_unix_nano,current_expires_unix_nano,current_not_after_unix_nano,
		        route_call_digest,authorization_id,source_lineage_digest,authorization_digest,expires_unix_nano,canonical_json
		 FROM invocation_material_v2_history WHERE material_id=? AND revision=?`,
		id,
		revision,
	).Scan(
		&materialDigest,
		&preparedID,
		&preparedRevision,
		&preparedDigest,
		&currentID,
		&currentRevision,
		&currentDigest,
		&currentCheckedUnixNano,
		&currentExpiresUnixNano,
		&currentNotAfterUnixNano,
		&routeCallDigest,
		&authorizationID,
		&sourceLineageDigest,
		&authorizationDigest,
		&expiresUnixNano,
		&payload,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.InvocationMaterialV2{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorNotFound,
			"inspect_invocation_material_v2",
			"exact material is absent",
			nil,
		)
	}
	if err != nil {
		return modelinvoker.InvocationMaterialV2{}, mapDBErrorV1(
			ctx,
			"inspect_invocation_material_v2",
			err,
			false,
		)
	}
	material, decodeErr := modelinvoker.DecodeInvocationMaterialV2(payload)
	if decodeErr != nil {
		return modelinvoker.InvocationMaterialV2{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"inspect_invocation_material_v2",
			"stored material failed exact revalidation",
			decodeErr,
		)
	}
	if material.ID != id || uint64(material.Revision) != revision ||
		materialDigest != string(material.Digest) ||
		preparedID != material.PreparedRef.ID ||
		preparedRevision != uint64(material.PreparedRef.Revision) ||
		preparedDigest != string(material.PreparedRef.Digest) ||
		currentID != material.Authorization.CurrentRef.ID ||
		currentRevision != uint64(material.Authorization.CurrentRef.Revision) ||
		currentDigest != string(material.Authorization.CurrentRef.Digest) ||
		currentCheckedUnixNano != material.Authorization.CurrentRef.CheckedUnixNano ||
		currentExpiresUnixNano != material.Authorization.CurrentRef.ExpiresUnixNano ||
		currentNotAfterUnixNano != material.Authorization.CurrentRef.NotAfterUnixNano ||
		routeCallDigest != string(material.RouteCallDigest) ||
		authorizationID != material.Authorization.ID ||
		sourceLineageDigest != string(material.Authorization.SourceLineage.Digest) ||
		authorizationDigest != string(material.Authorization.Digest) ||
		expiresUnixNano != material.ExpiresUnixNano {
		return modelinvoker.InvocationMaterialV2{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"inspect_invocation_material_v2",
			"stored material metadata drifted",
			nil,
		)
	}
	return material.CloneV2(), nil
}

var _ modelinvoker.InvocationMaterialRepositoryV2 = (*Store)(nil)
