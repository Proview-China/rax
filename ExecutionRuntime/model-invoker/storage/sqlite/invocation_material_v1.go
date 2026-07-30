package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func (s *Store) EnsureAuthorizedInvocationMaterialV1(ctx context.Context, request modelinvoker.InvocationMaterialPersistRequestV1) (modelinvoker.InvocationMaterialV1, error) {
	if err := contextErrorV1(ctx, "ensure_invocation_material"); err != nil {
		return modelinvoker.InvocationMaterialV1{}, err
	}
	if err := request.ValidateV1(); err != nil {
		return modelinvoker.InvocationMaterialV1{}, err
	}
	material := request.MaterialV1()
	wire, err := modelinvoker.EncodeInvocationMaterialV1(material)
	if err != nil {
		return modelinvoker.InvocationMaterialV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "ensure_invocation_material", "material cannot be encoded", err)
	}
	tx, err := s.beginV1(ctx, "ensure_invocation_material")
	if err != nil {
		return modelinvoker.InvocationMaterialV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var digest, route string
	var stored []byte
	err = tx.QueryRowContext(ctx, `SELECT material_digest,route_call_digest,canonical_json FROM invocation_material_history WHERE material_id=? AND revision=?`, material.ID, material.Revision).Scan(&digest, &route, &stored)
	if err == nil {
		if digest != string(material.Digest) || route != string(material.RouteCallDigest) || !bytes.Equal(stored, wire) {
			return modelinvoker.InvocationMaterialV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "ensure_invocation_material", "exact material contains different canonical content", nil)
		}
		return material.CloneV1(), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.InvocationMaterialV1{}, mapDBErrorV1(ctx, "ensure_invocation_material", err, false)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO invocation_material_history(material_id,revision,material_digest,route_call_digest,canonical_json) VALUES(?,?,?,?,?)`, material.ID, material.Revision, string(material.Digest), string(material.RouteCallDigest), wire); err != nil {
		return modelinvoker.InvocationMaterialV1{}, mapDBErrorV1(ctx, "ensure_invocation_material", err, true)
	}
	if err = tx.Commit(); err != nil {
		return modelinvoker.InvocationMaterialV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "ensure_invocation_material", "material commit outcome is unknown", err)
	}
	return material.CloneV1(), nil
}
func (s *Store) InspectExactInvocationMaterialV1(ctx context.Context, ref modelinvoker.InvocationMaterialRefV1) (modelinvoker.InvocationMaterialV1, error) {
	if err := contextErrorV1(ctx, "inspect_invocation_material"); err != nil {
		return modelinvoker.InvocationMaterialV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.InvocationMaterialV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "inspect_invocation_material", "exact material Ref is invalid", err)
	}
	var digest, route string
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT material_digest,route_call_digest,canonical_json FROM invocation_material_history WHERE material_id=? AND revision=?`, ref.ID, ref.Revision).Scan(&digest, &route, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.InvocationMaterialV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorNotFound, "inspect_invocation_material", "exact material is absent", nil)
	}
	if err != nil {
		return modelinvoker.InvocationMaterialV1{}, mapDBErrorV1(ctx, "inspect_invocation_material", err, false)
	}
	material, decodeErr := modelinvoker.DecodeInvocationMaterialV1(payload)
	if decodeErr != nil {
		return modelinvoker.InvocationMaterialV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_invocation_material", "stored material failed exact revalidation", decodeErr)
	}
	if material.RefV1() != ref || digest != string(ref.Digest) || route != string(ref.RouteCallDigest) {
		return modelinvoker.InvocationMaterialV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_invocation_material", "stored material failed exact Ref revalidation", nil)
	}
	return material.CloneV1(), nil
}

var _ modelinvoker.InvocationMaterialRepositoryV1 = (*Store)(nil)
