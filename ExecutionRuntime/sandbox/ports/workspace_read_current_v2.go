package ports

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

const WorkspaceReadCurrentContractVersionV2 = "praxis.sandbox/workspace-read-current/v2"

type WorkspaceReadCurrentQueryV2 struct {
	ContractVersion  string                                 `json:"contract_version"`
	Base             WorkspaceReadCurrentQueryV1            `json:"base"`
	Reservation      contract.Ref                           `json:"reservation"`
	Attempt          contract.WorkspaceReadAttemptRefV1     `json:"attempt"`
	AdmissionReceipt contract.WorkspaceReadReceiptBindingV1 `json:"admission_receipt"`
	Digest           string                                 `json:"digest"`
}

func (q WorkspaceReadCurrentQueryV2) Validate() error {
	if q.ContractVersion != WorkspaceReadCurrentContractVersionV2 ||
		q.Base.Validate() != nil ||
		q.Reservation.ValidateShape("workspace read reservation") != nil ||
		q.Attempt.Validate() != nil ||
		q.AdmissionReceipt.Validate() != nil ||
		q.AdmissionReceipt.ObservationDigest != "" ||
		q.AdmissionReceipt.StableKeyDigest != string(q.Base.StableKeyDigest) ||
		!contract.ValidDigest(q.Digest) {
		return errors.New("workspace read current v2 query is incomplete")
	}
	copy := q
	copy.Digest = ""
	digest, err := contract.Digest("workspace-read-current-query-v2", copy)
	if err != nil || digest != q.Digest {
		return errors.New("workspace read current v2 query digest drifted")
	}
	return nil
}

func (q WorkspaceReadCurrentQueryV2) ValidateCurrent(now time.Time) error {
	if err := q.Validate(); err != nil {
		return err
	}
	if err := q.Base.ValidateCurrent(now); err != nil {
		return err
	}
	return q.AdmissionReceipt.ValidateCurrent(now)
}

func SealWorkspaceReadCurrentQueryV2(q WorkspaceReadCurrentQueryV2) (WorkspaceReadCurrentQueryV2, error) {
	q.ContractVersion = WorkspaceReadCurrentContractVersionV2
	q.Digest = ""
	digest, err := contract.Digest("workspace-read-current-query-v2", q)
	if err != nil {
		return WorkspaceReadCurrentQueryV2{}, err
	}
	q.Digest = digest
	return q, q.Validate()
}

type WorkspaceReadCurrentProjectionV2 struct {
	ContractVersion           string                                 `json:"contract_version"`
	Base                      WorkspaceReadCurrentProjectionV1       `json:"base"`
	Reservation               contract.WorkspaceReadReservationV1    `json:"reservation"`
	Attempt                   contract.WorkspaceReadAttemptV1        `json:"attempt"`
	AttemptState              contract.WorkspaceReadStateV1          `json:"attempt_state"`
	AdmissionReceipt          contract.WorkspaceReadReceiptBindingV1 `json:"admission_receipt"`
	SandboxOwnerClosureDigest string                                 `json:"sandbox_owner_closure_digest"`
	S1CheckedUnixNano         int64                                  `json:"s1_checked_unix_nano"`
	S2CheckedUnixNano         int64                                  `json:"s2_checked_unix_nano"`
	ExpiresUnixNano           int64                                  `json:"expires_unix_nano"`
	SemanticDigest            string                                 `json:"semantic_digest"`
	ProjectionDigest          string                                 `json:"projection_digest"`
}

func (p WorkspaceReadCurrentProjectionV2) ValidateCurrent(now time.Time) error {
	expectedExpires := minimumWorkspaceReadCurrentExpiryV2(
		p.Base.ExpiresUnixNano,
		p.Reservation.Meta.ExpiresUnixNano,
		p.Attempt.Meta.ExpiresUnixNano,
		p.AdmissionReceipt.ExpiresUnixNano,
	)
	if err := p.Base.ValidateCurrent(now); err != nil {
		return fmt.Errorf("workspace read current v2 base: %w", err)
	}
	if err := p.Reservation.ValidateCurrent(now); err != nil {
		return fmt.Errorf("workspace read current v2 reservation: %w", err)
	}
	if err := p.Attempt.ValidateCurrent(now); err != nil {
		return fmt.Errorf("workspace read current v2 attempt: %w", err)
	}
	if err := p.AdmissionReceipt.ValidateCurrent(now); err != nil {
		return fmt.Errorf("workspace read current v2 admission: %w", err)
	}
	if p.ContractVersion != WorkspaceReadCurrentContractVersionV2 ||
		p.AttemptState != contract.WorkspaceReadStartedV1 ||
		p.Attempt.State != contract.WorkspaceReadStartedV1 ||
		p.AttemptState != p.Attempt.State ||
		p.AdmissionReceipt.Validate() != nil ||
		p.AdmissionReceipt.ObservationDigest != "" ||
		p.Attempt.AdmissionReceipt != p.AdmissionReceipt ||
		p.Attempt.Reservation != p.Reservation.Meta.Ref() ||
		p.S1CheckedUnixNano <= 0 ||
		p.S2CheckedUnixNano < p.S1CheckedUnixNano ||
		p.ExpiresUnixNano <= p.S2CheckedUnixNano ||
		!contract.ValidDigest(p.SandboxOwnerClosureDigest) ||
		!contract.ValidDigest(p.SemanticDigest) ||
		!contract.ValidDigest(p.ProjectionDigest) {
		return errors.New("workspace read current v2 projection is incomplete")
	}
	if p.ExpiresUnixNano != expectedExpires {
		return fmt.Errorf("workspace read current v2 projection TTL widened or narrowed: got %d want %d", p.ExpiresUnixNano, expectedExpires)
	}
	semantic, err := workspaceReadCurrentSemanticDigestV2(p)
	if err != nil || semantic != p.SemanticDigest {
		return errors.New("workspace read current v2 semantic digest drifted")
	}
	copy := p
	copy.ProjectionDigest = ""
	digest, err := contract.Digest("workspace-read-current-projection-v2", copy)
	if err != nil || digest != p.ProjectionDigest {
		return errors.New("workspace read current v2 projection digest drifted")
	}
	if now.IsZero() || now.UnixNano() < p.S2CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return errors.New("workspace read current v2 projection is expired or from the future")
	}
	return nil
}

func minimumWorkspaceReadCurrentExpiryV2(values ...int64) int64 {
	minimum := int64(0)
	for _, value := range values {
		if minimum == 0 || value < minimum {
			minimum = value
		}
	}
	return minimum
}

func SealWorkspaceReadCurrentProjectionV2(p WorkspaceReadCurrentProjectionV2) (WorkspaceReadCurrentProjectionV2, error) {
	expectedExpires := minimumWorkspaceReadCurrentExpiryV2(
		p.Base.ExpiresUnixNano,
		p.Reservation.Meta.ExpiresUnixNano,
		p.Attempt.Meta.ExpiresUnixNano,
		p.AdmissionReceipt.ExpiresUnixNano,
	)
	if p.S1CheckedUnixNano <= 0 ||
		p.S2CheckedUnixNano < p.S1CheckedUnixNano ||
		p.ExpiresUnixNano <= p.S2CheckedUnixNano ||
		p.ExpiresUnixNano != expectedExpires {
		return WorkspaceReadCurrentProjectionV2{}, errors.New("workspace read current v2 projection TTL closure is invalid")
	}
	p.ContractVersion = WorkspaceReadCurrentContractVersionV2
	p.SemanticDigest = ""
	p.ProjectionDigest = ""
	semantic, err := workspaceReadCurrentSemanticDigestV2(p)
	if err != nil {
		return WorkspaceReadCurrentProjectionV2{}, err
	}
	p.SemanticDigest = semantic
	digest, err := contract.Digest("workspace-read-current-projection-v2", p)
	if err != nil {
		return WorkspaceReadCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, nil
}

func workspaceReadCurrentSemanticDigestV2(p WorkspaceReadCurrentProjectionV2) (string, error) {
	p.ProjectionDigest = ""
	p.SemanticDigest = ""
	p.S1CheckedUnixNano = 0
	p.S2CheckedUnixNano = 0
	p.Base.S1CheckedUnixNano = 0
	p.Base.S2CheckedUnixNano = 0
	p.Base.RuntimeEnforcementDigest = ""
	p.Base.ProjectionDigest = ""
	p.Base.SemanticDigest = ""
	return contract.Digest("workspace-read-current-projection-v2", p)
}

type WorkspaceReadCurrentProjectionReaderV2 interface {
	InspectWorkspaceReadCurrentV2(context.Context, WorkspaceReadCurrentQueryV2) (WorkspaceReadCurrentProjectionV2, error)
}
