package ports

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

const (
	WorkspaceReadInspectionContractVersionV2 = "praxis.sandbox/workspace-read-inspection/v2"
	WorkspaceReadInspectionTypeURLV2         = "type.googleapis.com/praxis.sandbox.WorkspaceReadInspectionEnvelopeV2"
	WorkspaceReadInspectionMaxTTLV2          = 30 * time.Second
)

// WorkspaceReadInspectionEnvelopeV2 is a Sandbox-owned, read-only proof that
// CurrentProjection was reached from RequestedOriginAttemptRef. Its lifetime
// describes inspection freshness only; it does not renew execution eligibility
// or any expired upstream fact.
type WorkspaceReadInspectionEnvelopeV2 struct {
	ContractVersion           string                                      `json:"contract_version"`
	TypeURL                   string                                      `json:"type_url"`
	RequestedOriginAttemptRef contract.WorkspaceReadAttemptRefV1          `json:"requested_origin_attempt_ref"`
	CurrentProjection         contract.WorkspaceReadExecutionProjectionV1 `json:"current_projection"`
	CheckedUnixNano           int64                                       `json:"checked_unix_nano"`
	ExpiresUnixNano           int64                                       `json:"expires_unix_nano"`
	ProjectionDigest          string                                      `json:"projection_digest"`
}

func (e WorkspaceReadInspectionEnvelopeV2) ValidateShape() error {
	if e.ContractVersion != WorkspaceReadInspectionContractVersionV2 ||
		e.TypeURL != WorkspaceReadInspectionTypeURLV2 ||
		e.RequestedOriginAttemptRef.Validate() != nil ||
		e.CurrentProjection.ValidateShape() != nil ||
		e.CheckedUnixNano <= 0 ||
		e.ExpiresUnixNano <= e.CheckedUnixNano ||
		e.ExpiresUnixNano-e.CheckedUnixNano > WorkspaceReadInspectionMaxTTLV2.Nanoseconds() ||
		!contract.ValidDigest(e.ProjectionDigest) {
		return errors.New("workspace read inspection envelope is incomplete")
	}
	current := e.CurrentProjection.Attempt
	if current.Meta.ID != e.RequestedOriginAttemptRef.ID ||
		current.Meta.Revision < e.RequestedOriginAttemptRef.Revision {
		return errors.New("workspace read inspection current is not descended from the requested origin")
	}
	switch current.State {
	case contract.WorkspaceReadStartedV1:
		if current.Meta.Ref() != e.RequestedOriginAttemptRef.OwnerRef() {
			return errors.New("workspace read started inspection does not equal the requested origin")
		}
	case contract.WorkspaceReadObservedV1, contract.WorkspaceReadFailedV1, contract.WorkspaceReadUnknownV1:
		if current.Meta.Revision != e.RequestedOriginAttemptRef.Revision+1 {
			return errors.New("workspace read terminal inspection skipped the origin successor")
		}
	default:
		return errors.New("workspace read inspection state is invalid")
	}
	copy := e
	copy.ProjectionDigest = ""
	digest, err := contract.Digest("workspace-read-inspection-envelope-v2", copy)
	if err != nil || digest != e.ProjectionDigest {
		return errors.New("workspace read inspection envelope digest drifted")
	}
	return nil
}

func (e WorkspaceReadInspectionEnvelopeV2) ValidateCurrent(now time.Time) error {
	if err := e.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < e.CheckedUnixNano {
		return errors.New("workspace read inspection clock regressed")
	}
	if !now.Before(time.Unix(0, e.ExpiresUnixNano)) {
		return errors.New("workspace read inspection envelope expired")
	}
	return nil
}

func SealWorkspaceReadInspectionEnvelopeV2(e WorkspaceReadInspectionEnvelopeV2) (WorkspaceReadInspectionEnvelopeV2, error) {
	e.ContractVersion = WorkspaceReadInspectionContractVersionV2
	e.TypeURL = WorkspaceReadInspectionTypeURLV2
	e.ProjectionDigest = ""
	digest, err := contract.Digest("workspace-read-inspection-envelope-v2", e)
	if err != nil {
		return WorkspaceReadInspectionEnvelopeV2{}, err
	}
	e.ProjectionDigest = digest
	return e, e.ValidateShape()
}

func EncodeWorkspaceReadInspectionEnvelopeV2(e WorkspaceReadInspectionEnvelopeV2) (json.RawMessage, error) {
	if err := e.ValidateShape(); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

func DecodeWorkspaceReadInspectionEnvelopeV2(payload json.RawMessage) (WorkspaceReadInspectionEnvelopeV2, error) {
	envelope, err := contract.DecodeStrict[WorkspaceReadInspectionEnvelopeV2](payload)
	if err != nil {
		return WorkspaceReadInspectionEnvelopeV2{}, err
	}
	return envelope, envelope.ValidateShape()
}

type WorkspaceReadInspectionReaderV2 interface {
	InspectBoundedWorkspaceReadV2(context.Context, contract.WorkspaceReadAttemptRefV1) (WorkspaceReadInspectionEnvelopeV2, error)
}

// WorkspaceReadExecutionPortV2 is additive. The V1 physical execution and
// Admission handoff remain unchanged; consumers that require exact
// origin-to-current proof use InspectBoundedWorkspaceReadV2.
type WorkspaceReadExecutionPortV2 interface {
	WorkspaceReadExecutionPortV1
	WorkspaceReadInspectionReaderV2
}
