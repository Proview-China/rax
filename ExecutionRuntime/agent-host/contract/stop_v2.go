package contract

import (
	"sort"
	"time"
)

const HostStopContractVersionV2 = "praxis.agent-host/stop/v2"

type StopRequestV2 struct {
	ContractVersion           string     `json:"contract_version"`
	HostID                    string     `json:"host_id"`
	StartID                   string     `json:"start_id"`
	StartClaimRef             ExactRefV1 `json:"start_claim_ref"`
	CleanupPlanRef            ExactRefV1 `json:"cleanup_plan_ref"`
	RequestedAtUnixNano       int64      `json:"requested_at_unix_nano"`
	RequestedNotAfterUnixNano int64      `json:"requested_not_after_unix_nano"`
	RequestDigest             DigestV1   `json:"request_digest"`
}

func (r StopRequestV2) digestV2() (DigestV1, error) {
	clone := r
	clone.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string        `json:"domain"`
		Type   string        `json:"type"`
		Body   StopRequestV2 `json:"body"`
	}{"praxis.agent-host.stop-v2", "StopRequestV2", clone})
}

func SealStopRequestV2(r StopRequestV2) (StopRequestV2, error) {
	if r.ContractVersion != "" && r.ContractVersion != HostStopContractVersionV2 {
		return StopRequestV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostV2 Stop request version drifted")
	}
	r.ContractVersion = HostStopContractVersionV2
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := r.digestV2()
	if err != nil {
		return StopRequestV2{}, err
	}
	if provided != "" && provided != digest {
		return StopRequestV2{}, NewError(ErrorConflict, "host_v2_stop_request_drift", "HostV2 Stop request supplied a wrong digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

func (r StopRequestV2) Validate() error {
	if r.ContractVersion != HostStopContractVersionV2 || r.RequestedAtUnixNano <= 0 || r.RequestedNotAfterUnixNano <= r.RequestedAtUnixNano {
		return NewError(ErrorInvalidArgument, "host_v2_stop_request_incomplete", "HostV2 Stop request is incomplete")
	}
	for name, value := range map[string]string{"host id": r.HostID, "start id": r.StartID} {
		if err := ValidateIdentifierV1(name, value); err != nil {
			return err
		}
	}
	if err := r.StartClaimRef.Validate(); err != nil {
		return err
	}
	if err := r.CleanupPlanRef.Validate(); err != nil {
		return err
	}
	digest, err := r.digestV2()
	if err != nil || digest != r.RequestDigest {
		return NewError(ErrorConflict, "host_v2_stop_request_drift", "HostV2 Stop request digest drifted")
	}
	return nil
}

func (r StopRequestV2) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.RequestedAtUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "HostV2 Stop request was checked before creation")
	}
	if !now.Before(time.Unix(0, r.RequestedNotAfterUnixNano)) {
		return NewError(ErrorPrecondition, "host_v2_stop_request_expired", "HostV2 Stop request expired")
	}
	return nil
}

type CleanupNodeRequestV2 struct {
	ContractVersion           string        `json:"contract_version"`
	HostID                    string        `json:"host_id"`
	StartID                   string        `json:"start_id"`
	AttemptID                 string        `json:"attempt_id"`
	PlanRef                   ExactRefV1    `json:"plan_ref"`
	Node                      CleanupNodeV2 `json:"node"`
	PredecessorRevision       uint64        `json:"predecessor_revision"`
	BarrierCurrentRefs        []ExactRefV1  `json:"barrier_current_refs"`
	RequestedNotAfterUnixNano int64         `json:"requested_not_after_unix_nano"`
	RequestDigest             DigestV1      `json:"request_digest"`
}

func (r CleanupNodeRequestV2) canonicalV2() CleanupNodeRequestV2 {
	r.BarrierCurrentRefs = append([]ExactRefV1{}, r.BarrierCurrentRefs...)
	sort.Slice(r.BarrierCurrentRefs, func(i, j int) bool {
		return r.BarrierCurrentRefs[i].Kind+"\x00"+r.BarrierCurrentRefs[i].ID < r.BarrierCurrentRefs[j].Kind+"\x00"+r.BarrierCurrentRefs[j].ID
	})
	return r
}
func (r CleanupNodeRequestV2) digestV2() (DigestV1, error) {
	clone := r.canonicalV2()
	clone.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string               `json:"domain"`
		Type   string               `json:"type"`
		Body   CleanupNodeRequestV2 `json:"body"`
	}{"praxis.agent-host.cleanup-node-request-v2", "CleanupNodeRequestV2", clone})
}
func SealCleanupNodeRequestV2(r CleanupNodeRequestV2) (CleanupNodeRequestV2, error) {
	r.ContractVersion = CleanupContractVersionV2
	r = r.canonicalV2()
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := r.digestV2()
	if err != nil {
		return CleanupNodeRequestV2{}, err
	}
	if provided != "" && provided != digest {
		return CleanupNodeRequestV2{}, NewError(ErrorConflict, "cleanup_node_request_drift", "cleanup node request supplied a wrong digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}
func (r CleanupNodeRequestV2) Validate() error {
	if r.ContractVersion != CleanupContractVersionV2 || r.PredecessorRevision == 0 || r.RequestedNotAfterUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "cleanup_node_request_incomplete", "cleanup node request is incomplete")
	}
	for name, value := range map[string]string{"host id": r.HostID, "start id": r.StartID, "attempt id": r.AttemptID} {
		if err := ValidateIdentifierV1(name, value); err != nil {
			return err
		}
	}
	if err := r.PlanRef.Validate(); err != nil {
		return err
	}
	if err := r.Node.Validate(); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, ref := range r.BarrierCurrentRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := ref.Kind + "\x00" + ref.ID
		if _, ok := seen[key]; ok {
			return NewError(ErrorConflict, "cleanup_barrier_ref_duplicate", "cleanup node request duplicates a barrier Ref")
		}
		seen[key] = struct{}{}
	}
	digest, err := r.digestV2()
	if err != nil || digest != r.RequestDigest {
		return NewError(ErrorConflict, "cleanup_node_request_drift", "cleanup node request digest drifted")
	}
	return nil
}

type CleanupNodeResultV2 struct {
	ContractVersion string               `json:"contract_version"`
	AttemptID       string               `json:"attempt_id"`
	RequestDigest   DigestV1             `json:"request_digest"`
	ResultRef       ExactRefV1           `json:"result_ref"`
	Disposition     CleanupDispositionV2 `json:"disposition"`
	CheckedUnixNano int64                `json:"checked_unix_nano"`
	ExpiresUnixNano int64                `json:"expires_unix_nano"`
	ResultDigest    DigestV1             `json:"result_digest"`
}

func (r CleanupNodeResultV2) digestV2() (DigestV1, error) {
	clone := r
	clone.ResultDigest = ""
	return DigestJSONV1(struct {
		Domain string              `json:"domain"`
		Type   string              `json:"type"`
		Body   CleanupNodeResultV2 `json:"body"`
	}{"praxis.agent-host.cleanup-node-result-v2", "CleanupNodeResultV2", clone})
}
func SealCleanupNodeResultV2(r CleanupNodeResultV2) (CleanupNodeResultV2, error) {
	r.ContractVersion = CleanupContractVersionV2
	provided := r.ResultDigest
	r.ResultDigest = ""
	digest, err := r.digestV2()
	if err != nil {
		return CleanupNodeResultV2{}, err
	}
	if provided != "" && provided != digest {
		return CleanupNodeResultV2{}, NewError(ErrorConflict, "cleanup_node_result_drift", "cleanup node result supplied a wrong digest")
	}
	r.ResultDigest = digest
	return r, r.Validate()
}
func (r CleanupNodeResultV2) Validate() error {
	if r.ContractVersion != CleanupContractVersionV2 || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "cleanup_node_result_incomplete", "cleanup node result is incomplete")
	}
	if err := ValidateIdentifierV1("attempt id", r.AttemptID); err != nil {
		return err
	}
	if err := r.RequestDigest.Validate(); err != nil {
		return err
	}
	if err := r.ResultRef.Validate(); err != nil {
		return err
	}
	if r.Disposition != CleanupDispositionSettledV2 && r.Disposition != CleanupDispositionResidualV2 {
		return NewError(ErrorInvalidArgument, "cleanup_node_result_disposition_invalid", "cleanup node disposition is unsupported")
	}
	digest, err := r.digestV2()
	if err != nil || digest != r.ResultDigest {
		return NewError(ErrorConflict, "cleanup_node_result_drift", "cleanup node result digest drifted")
	}
	return nil
}
func (r CleanupNodeResultV2) ValidateCurrent(request CleanupNodeRequestV2, now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if r.AttemptID != request.AttemptID || r.RequestDigest != request.RequestDigest || r.ExpiresUnixNano > request.RequestedNotAfterUnixNano || now.UnixNano() < r.CheckedUnixNano || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return NewError(ErrorConflict, "cleanup_node_result_request_drift", "cleanup node result is not current for exact request")
	}
	return nil
}

type StopResultV2 struct {
	ContractVersion string             `json:"contract_version"`
	HostID          string             `json:"host_id"`
	StartID         string             `json:"start_id"`
	RequestDigest   DigestV1           `json:"request_digest"`
	Journal         ExactRefV1         `json:"journal"`
	Phase           HostPhaseV2        `json:"phase"`
	Attempts        []CleanupAttemptV2 `json:"attempts"`
	Residuals       []ExactRefV1       `json:"residuals"`
	CheckedUnixNano int64              `json:"checked_unix_nano"`
	ResultDigest    DigestV1           `json:"result_digest"`
}

func (r StopResultV2) canonicalV2() StopResultV2 {
	r.Attempts = append([]CleanupAttemptV2{}, r.Attempts...)
	sort.Slice(r.Attempts, func(i, j int) bool { return r.Attempts[i].AttemptID < r.Attempts[j].AttemptID })
	r.Residuals = append([]ExactRefV1{}, r.Residuals...)
	sort.Slice(r.Residuals, func(i, j int) bool {
		return r.Residuals[i].Kind+"\x00"+r.Residuals[i].ID < r.Residuals[j].Kind+"\x00"+r.Residuals[j].ID
	})
	return r
}
func (r StopResultV2) digestV2() (DigestV1, error) {
	clone := r.canonicalV2()
	clone.ResultDigest = ""
	return DigestJSONV1(struct {
		Domain string       `json:"domain"`
		Type   string       `json:"type"`
		Body   StopResultV2 `json:"body"`
	}{"praxis.agent-host.stop-v2", "StopResultV2", clone})
}
func SealStopResultV2(r StopResultV2) (StopResultV2, error) {
	r.ContractVersion = HostStopContractVersionV2
	r = r.canonicalV2()
	provided := r.ResultDigest
	r.ResultDigest = ""
	digest, err := r.digestV2()
	if err != nil {
		return StopResultV2{}, err
	}
	if provided != "" && provided != digest {
		return StopResultV2{}, NewError(ErrorConflict, "host_v2_stop_result_drift", "HostV2 Stop result supplied wrong digest")
	}
	r.ResultDigest = digest
	return r, r.Validate()
}
func (r StopResultV2) Validate() error {
	if r.ContractVersion != HostStopContractVersionV2 || r.CheckedUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "host_v2_stop_result_incomplete", "HostV2 Stop result is incomplete")
	}
	if err := ValidateIdentifierV1("host id", r.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", r.StartID); err != nil {
		return err
	}
	if err := r.RequestDigest.Validate(); err != nil {
		return err
	}
	if err := r.Journal.Validate(); err != nil {
		return err
	}
	if r.Phase != HostClosedV2 && r.Phase != HostReconcilingV2 && r.Phase != HostIndeterminateV2 {
		return NewError(ErrorConflict, "host_v2_stop_phase_disposition_drift", "Stop result must expose a closed or reconciliation phase")
	}
	attemptIDs := make(map[string]struct{}, len(r.Attempts))
	residualResults := make(map[ExactRefV1]struct{})
	for _, attempt := range r.Attempts {
		if err := attempt.Validate(); err != nil {
			return err
		}
		if _, exists := attemptIDs[attempt.AttemptID]; exists {
			return NewError(ErrorConflict, "host_v2_stop_attempt_duplicate", "Stop result duplicates a cleanup attempt")
		}
		attemptIDs[attempt.AttemptID] = struct{}{}
		if attempt.State == CleanupResultRecordedV2 && attempt.ResultDisposition == CleanupDispositionResidualV2 {
			residualResults[*attempt.ResultRef] = struct{}{}
		}
		if r.Phase == HostClosedV2 && (attempt.State != CleanupResultRecordedV2 || attempt.ResultDisposition != CleanupDispositionSettledV2) {
			return NewError(ErrorPrecondition, "host_v2_stop_attempt_unsettled", "closed Stop requires every cleanup attempt to be exactly settled")
		}
	}
	seenResiduals := make(map[ExactRefV1]struct{}, len(r.Residuals))
	for _, residual := range r.Residuals {
		if err := residual.Validate(); err != nil {
			return err
		}
		if _, exists := seenResiduals[residual]; exists {
			return NewError(ErrorConflict, "host_v2_stop_residual_duplicate", "Stop result duplicates a residual Ref")
		}
		if _, exists := residualResults[residual]; !exists {
			return NewError(ErrorConflict, "host_v2_stop_residual_splice", "Stop residual is not backed by an exact recorded residual attempt")
		}
		seenResiduals[residual] = struct{}{}
	}
	if r.Phase == HostClosedV2 && len(r.Residuals) != 0 {
		return NewError(ErrorPrecondition, "host_v2_stop_residual_open", "closed Stop cannot carry residual cleanup")
	}
	digest, err := r.digestV2()
	if err != nil || digest != r.ResultDigest {
		return NewError(ErrorConflict, "host_v2_stop_result_drift", "HostV2 Stop result digest drifted")
	}
	return nil
}
