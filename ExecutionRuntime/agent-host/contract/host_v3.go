package contract

import (
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const HostLifecycleContractVersionV3 = "praxis.agent-host/lifecycle/v3"

type StartRequestV3 struct {
	ContractVersion           string                     `json:"contract_version"`
	StartID                   string                     `json:"start_id"`
	DeploymentCurrentRef      HostDeploymentCurrentRefV1 `json:"deployment_current_ref"`
	Config                    HostConfigV1               `json:"config"`
	DefinitionSourceCurrent   ExactRefV1                 `json:"definition_source_current"`
	RequestedAtUnixNano       int64                      `json:"requested_at_unix_nano"`
	RequestedNotAfterUnixNano int64                      `json:"requested_not_after_unix_nano"`
	RequestDigest             DigestV1                   `json:"request_digest"`
}

func (r StartRequestV3) canonicalV3() StartRequestV3 { r.Config = r.Config.CanonicalV1(); return r }
func (r StartRequestV3) digestV3() (DigestV1, error) {
	r = r.canonicalV3()
	r.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string         `json:"domain"`
		Type   string         `json:"type"`
		Body   StartRequestV3 `json:"body"`
	}{Domain: "praxis.agent-host.lifecycle-v3", Type: "StartRequestV3", Body: r})
}
func SealStartRequestV3(r StartRequestV3) (StartRequestV3, error) {
	if r.ContractVersion != "" && r.ContractVersion != HostLifecycleContractVersionV3 {
		return StartRequestV3{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostV3 Start request version drifted")
	}
	r.ContractVersion = HostLifecycleContractVersionV3
	r = r.canonicalV3()
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := r.digestV3()
	if err != nil {
		return StartRequestV3{}, err
	}
	if provided != "" && provided != digest {
		return StartRequestV3{}, NewError(ErrorConflict, "host_v3_start_request_drift", "HostV3 Start request supplied a wrong non-zero digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}
func (r StartRequestV3) Validate() error {
	if r.ContractVersion != HostLifecycleContractVersionV3 || r.RequestedAtUnixNano <= 0 || r.RequestedNotAfterUnixNano <= r.RequestedAtUnixNano {
		return NewError(ErrorInvalidArgument, "host_v3_start_request_incomplete", "HostV3 Start request is incomplete")
	}
	if err := ValidateIdentifierV1("start id", r.StartID); err != nil {
		return err
	}
	if err := r.DeploymentCurrentRef.Validate(); err != nil {
		return err
	}
	if err := r.Config.Validate(); err != nil {
		return err
	}
	if r.Config.HostID != r.DeploymentCurrentRef.HostID || r.RequestedNotAfterUnixNano > r.DeploymentCurrentRef.ExpiresUnixNano {
		return NewError(ErrorConflict, "host_v3_start_deployment_drift", "HostV3 Start request exceeds or changes its deployment current")
	}
	if err := r.DefinitionSourceCurrent.Validate(); err != nil {
		return err
	}
	digest, err := r.digestV3()
	if err != nil || digest != r.RequestDigest {
		return NewError(ErrorConflict, "host_v3_start_request_drift", "HostV3 Start request digest drifted")
	}
	return nil
}
func (r StartRequestV3) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.RequestedAtUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "HostV3 Start request clock regressed")
	}
	if !now.Before(time.Unix(0, r.RequestedNotAfterUnixNano)) {
		return NewError(ErrorPrecondition, "host_v3_start_request_expired", "HostV3 Start request expired")
	}
	return nil
}
func (r StartRequestV3) ClaimInputV3() (HostStartClaimInputV3, error) {
	if err := r.Validate(); err != nil {
		return HostStartClaimInputV3{}, err
	}
	configDigest, err := r.Config.DigestV1()
	if err != nil {
		return HostStartClaimInputV3{}, err
	}
	return SealHostStartClaimInputV3(HostStartClaimInputV3{
		HostID: r.Config.HostID, StartID: r.StartID, DeploymentCurrentRef: r.DeploymentCurrentRef,
		HostConfigDigest: configDigest, DefinitionSourceRef: r.DefinitionSourceCurrent,
		RequestedOperation: HostStartOperationStartV1, CreatedUnixNano: r.RequestedAtUnixNano, ExpiresUnixNano: r.RequestedNotAfterUnixNano,
	})
}

type StartResultV3 struct {
	ContractVersion         string                                       `json:"contract_version"`
	HostID                  string                                       `json:"host_id"`
	StartID                 string                                       `json:"start_id"`
	RequestDigest           DigestV1                                     `json:"request_digest"`
	RequestNotAfterUnixNano int64                                        `json:"request_not_after_unix_nano"`
	StartClaim              HostStartClaimRefV1                          `json:"start_claim"`
	Journal                 ExactRefV1                                   `json:"journal"`
	CleanupClosure          ExactRefV1                                   `json:"cleanup_closure"`
	Ready                   SystemReadyCurrentRefV2                      `json:"ready"`
	Availability            runtimeports.AgentExecutionAvailabilityRefV1 `json:"availability"`
	CheckedUnixNano         int64                                        `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                                        `json:"expires_unix_nano"`
	ResultDigest            DigestV1                                     `json:"result_digest"`
}

func (r StartResultV3) digestV3() (DigestV1, error) {
	r.ResultDigest = ""
	return DigestJSONV1(struct {
		Domain string        `json:"domain"`
		Type   string        `json:"type"`
		Body   StartResultV3 `json:"body"`
	}{"praxis.agent-host.lifecycle-v3", "StartResultV3", r})
}
func SealStartResultV3(r StartResultV3) (StartResultV3, error) {
	if r.ContractVersion != "" && r.ContractVersion != HostLifecycleContractVersionV3 {
		return StartResultV3{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostV3 Start result version drifted")
	}
	r.ContractVersion = HostLifecycleContractVersionV3
	provided := r.ResultDigest
	r.ResultDigest = ""
	d, err := r.digestV3()
	if err != nil {
		return StartResultV3{}, err
	}
	if provided != "" && provided != d {
		return StartResultV3{}, NewError(ErrorConflict, "host_v3_start_result_drift", "HostV3 Start result supplied a wrong non-zero digest")
	}
	r.ResultDigest = d
	return r, r.Validate()
}
func (r StartResultV3) Validate() error {
	if r.ContractVersion != HostLifecycleContractVersionV3 || r.RequestNotAfterUnixNano <= 0 || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "host_v3_start_result_incomplete", "HostV3 Start result is incomplete")
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
	if err := r.StartClaim.Validate(); err != nil {
		return err
	}
	if err := r.Journal.Validate(); err != nil {
		return err
	}
	if err := r.CleanupClosure.Validate(); err != nil {
		return err
	}
	if err := r.Ready.Validate(); err != nil {
		return err
	}
	if err := r.Availability.Validate(); err != nil {
		return err
	}
	minimum := r.RequestNotAfterUnixNano
	for _, expires := range []int64{r.StartClaim.ExpiresUnixNano, r.Ready.ExpiresUnixNano, r.Availability.ExpiresUnixNano} {
		if expires < minimum {
			minimum = expires
		}
	}
	if r.StartClaim.HostID != r.HostID || r.StartClaim.StartID != r.StartID || r.ExpiresUnixNano != minimum {
		return NewError(ErrorConflict, "host_v3_start_result_coordinate_drift", "HostV3 Start result coordinates or expiry drifted")
	}
	d, err := r.digestV3()
	if err != nil || d != r.ResultDigest {
		return NewError(ErrorConflict, "host_v3_start_result_drift", "HostV3 Start result digest drifted")
	}
	return nil
}
func (r StartResultV3) ValidateFor(request StartRequestV3, now time.Time) error {
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	input, err := request.ClaimInputV3()
	if err != nil {
		return err
	}
	claim, err := input.ClaimV1()
	if err != nil {
		return err
	}
	expectedClaim, err := claim.CurrentRefV1()
	if err != nil {
		return err
	}
	if r.HostID != request.Config.HostID || r.StartID != request.StartID || r.RequestDigest != request.RequestDigest || r.RequestNotAfterUnixNano != request.RequestedNotAfterUnixNano || r.StartClaim != expectedClaim || now.UnixNano() < r.CheckedUnixNano || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return NewError(ErrorConflict, "host_v3_start_result_request_drift", "HostV3 Start result is not current for the exact request")
	}
	return nil
}

type InspectRequestV3 struct {
	ContractVersion           string              `json:"contract_version"`
	HostID                    string              `json:"host_id"`
	StartID                   string              `json:"start_id"`
	StartClaim                HostStartClaimRefV1 `json:"start_claim"`
	RequestedAtUnixNano       int64               `json:"requested_at_unix_nano"`
	RequestedNotAfterUnixNano int64               `json:"requested_not_after_unix_nano"`
	RequestDigest             DigestV1            `json:"request_digest"`
}

func (r InspectRequestV3) digestV3() (DigestV1, error) {
	r.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string           `json:"domain"`
		Type   string           `json:"type"`
		Body   InspectRequestV3 `json:"body"`
	}{"praxis.agent-host.lifecycle-v3", "InspectRequestV3", r})
}
func SealInspectRequestV3(r InspectRequestV3) (InspectRequestV3, error) {
	if r.ContractVersion != "" && r.ContractVersion != HostLifecycleContractVersionV3 {
		return InspectRequestV3{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostV3 Inspect version drifted")
	}
	r.ContractVersion = HostLifecycleContractVersionV3
	p := r.RequestDigest
	r.RequestDigest = ""
	d, e := r.digestV3()
	if e != nil {
		return InspectRequestV3{}, e
	}
	if p != "" && p != d {
		return InspectRequestV3{}, NewError(ErrorConflict, "host_v3_inspect_request_drift", "HostV3 Inspect supplied a wrong digest")
	}
	r.RequestDigest = d
	return r, r.Validate()
}
func (r InspectRequestV3) Validate() error {
	if r.ContractVersion != HostLifecycleContractVersionV3 || r.RequestedAtUnixNano <= 0 || r.RequestedNotAfterUnixNano <= r.RequestedAtUnixNano {
		return NewError(ErrorInvalidArgument, "host_v3_inspect_request_incomplete", "HostV3 Inspect request is incomplete")
	}
	if e := ValidateIdentifierV1("host id", r.HostID); e != nil {
		return e
	}
	if e := ValidateIdentifierV1("start id", r.StartID); e != nil {
		return e
	}
	if e := r.StartClaim.Validate(); e != nil {
		return e
	}
	if r.StartClaim.HostID != r.HostID || r.StartClaim.StartID != r.StartID {
		return NewError(ErrorConflict, "host_v3_inspect_claim_drift", "HostV3 Inspect claim coordinates drifted")
	}
	d, e := r.digestV3()
	if e != nil || d != r.RequestDigest {
		return NewError(ErrorConflict, "host_v3_inspect_request_drift", "HostV3 Inspect request digest drifted")
	}
	return nil
}

func (r InspectRequestV3) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.RequestedAtUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "HostV3 Inspect request clock regressed")
	}
	if !now.Before(time.Unix(0, r.RequestedNotAfterUnixNano)) {
		return NewError(ErrorPrecondition, "host_v3_inspect_request_expired", "HostV3 Inspect request expired")
	}
	return nil
}

type HostInspectPhaseV3 string

const (
	HostInspectClaimedV3       HostInspectPhaseV3 = "claimed"
	HostInspectStartingV3      HostInspectPhaseV3 = "starting"
	HostInspectReadyV3         HostInspectPhaseV3 = "ready"
	HostInspectStoppingV3      HostInspectPhaseV3 = "stopping"
	HostInspectClosedV3        HostInspectPhaseV3 = "closed"
	HostInspectIndeterminateV3 HostInspectPhaseV3 = "indeterminate"
)

func validHostInspectPhaseV3(p HostInspectPhaseV3) bool {
	switch p {
	case HostInspectClaimedV3, HostInspectStartingV3, HostInspectReadyV3, HostInspectStoppingV3, HostInspectClosedV3, HostInspectIndeterminateV3:
		return true
	}
	return false
}

type InspectResultV3 struct {
	ContractVersion         string                                       `json:"contract_version"`
	RequestDigest           DigestV1                                     `json:"request_digest"`
	RequestNotAfterUnixNano int64                                        `json:"request_not_after_unix_nano"`
	StartClaim              HostStartClaimV1                             `json:"start_claim"`
	Journal                 ExactRefV1                                   `json:"journal"`
	HasReady                bool                                         `json:"has_ready"`
	Ready                   SystemReadyCurrentRefV2                      `json:"ready"`
	HasAvailability         bool                                         `json:"has_availability"`
	Availability            runtimeports.AgentExecutionAvailabilityRefV1 `json:"availability"`
	HasCleanupClosure       bool                                         `json:"has_cleanup_closure"`
	CleanupClosure          ExactRefV1                                   `json:"cleanup_closure"`
	Phase                   HostInspectPhaseV3                           `json:"phase"`
	CheckedUnixNano         int64                                        `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                                        `json:"expires_unix_nano"`
	ResultDigest            DigestV1                                     `json:"result_digest"`
}

func (r InspectResultV3) digestV3() (DigestV1, error) {
	r.ResultDigest = ""
	return DigestJSONV1(struct {
		Domain string          `json:"domain"`
		Type   string          `json:"type"`
		Body   InspectResultV3 `json:"body"`
	}{"praxis.agent-host.lifecycle-v3", "InspectResultV3", r})
}
func SealInspectResultV3(r InspectResultV3) (InspectResultV3, error) {
	if r.ContractVersion != "" && r.ContractVersion != HostLifecycleContractVersionV3 {
		return InspectResultV3{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostV3 Inspect result version drifted")
	}
	r.ContractVersion = HostLifecycleContractVersionV3
	p := r.ResultDigest
	r.ResultDigest = ""
	d, e := r.digestV3()
	if e != nil {
		return InspectResultV3{}, e
	}
	if p != "" && p != d {
		return InspectResultV3{}, NewError(ErrorConflict, "host_v3_inspect_result_drift", "HostV3 Inspect result supplied a wrong digest")
	}
	r.ResultDigest = d
	return r, r.Validate()
}
func (r InspectResultV3) Validate() error {
	if r.ContractVersion != HostLifecycleContractVersionV3 || !validHostInspectPhaseV3(r.Phase) || r.RequestNotAfterUnixNano <= 0 || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "host_v3_inspect_result_incomplete", "HostV3 Inspect result is incomplete")
	}
	if e := r.RequestDigest.Validate(); e != nil {
		return e
	}
	if e := r.StartClaim.ValidateHistoricalV1(); e != nil {
		return e
	}
	if e := r.Journal.Validate(); e != nil {
		return e
	}
	// Claim expiry limits Start continuation, not historical Inspect. Inspect
	// remains available after Claim expiry and is bounded by its own request
	// plus any returned current projections.
	minimum := r.RequestNotAfterUnixNano
	if r.HasReady {
		if e := r.Ready.Validate(); e != nil {
			return e
		}
		if r.Ready.ExpiresUnixNano < minimum {
			minimum = r.Ready.ExpiresUnixNano
		}
	} else if r.Ready != (SystemReadyCurrentRefV2{}) {
		return NewError(ErrorConflict, "host_v3_inspect_ready_presence_drift", "HostV3 Inspect Ready presence drifted")
	}
	if r.HasAvailability {
		if e := r.Availability.Validate(); e != nil {
			return e
		}
		if r.Availability.ExpiresUnixNano < minimum {
			minimum = r.Availability.ExpiresUnixNano
		}
	} else if r.Availability != (runtimeports.AgentExecutionAvailabilityRefV1{}) {
		return NewError(ErrorConflict, "host_v3_inspect_availability_presence_drift", "HostV3 Inspect availability presence drifted")
	}
	if r.HasCleanupClosure {
		if e := r.CleanupClosure.Validate(); e != nil {
			return e
		}
	} else if r.CleanupClosure != (ExactRefV1{}) {
		return NewError(ErrorConflict, "host_v3_inspect_closure_presence_drift", "HostV3 Inspect Closure presence drifted")
	}
	if r.ExpiresUnixNano != minimum {
		return NewError(ErrorConflict, "host_v3_inspect_expiry_drift", "HostV3 Inspect result exceeds an exact Owner window")
	}
	d, e := r.digestV3()
	if e != nil || d != r.ResultDigest {
		return NewError(ErrorConflict, "host_v3_inspect_result_drift", "HostV3 Inspect result digest drifted")
	}
	return nil
}

func (r InspectResultV3) ValidateFor(request InspectRequestV3, now time.Time) error {
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	actualClaim, err := r.StartClaim.CurrentRefV1()
	if err != nil {
		return err
	}
	if r.RequestDigest != request.RequestDigest || r.RequestNotAfterUnixNano != request.RequestedNotAfterUnixNano || actualClaim != request.StartClaim || now.UnixNano() < r.CheckedUnixNano || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return NewError(ErrorConflict, "host_v3_inspect_result_request_drift", "HostV3 Inspect result is not current for the exact request")
	}
	return nil
}

type StopRequestV3 struct {
	ContractVersion           string              `json:"contract_version"`
	HostID                    string              `json:"host_id"`
	StartID                   string              `json:"start_id"`
	StartClaim                HostStartClaimRefV1 `json:"start_claim"`
	CleanupClosure            ExactRefV1          `json:"cleanup_closure"`
	RequestedAtUnixNano       int64               `json:"requested_at_unix_nano"`
	RequestedNotAfterUnixNano int64               `json:"requested_not_after_unix_nano"`
	RequestDigest             DigestV1            `json:"request_digest"`
}

func (r StopRequestV3) digestV3() (DigestV1, error) {
	r.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string        `json:"domain"`
		Type   string        `json:"type"`
		Body   StopRequestV3 `json:"body"`
	}{"praxis.agent-host.lifecycle-v3", "StopRequestV3", r})
}
func SealStopRequestV3(r StopRequestV3) (StopRequestV3, error) {
	if r.ContractVersion != "" && r.ContractVersion != HostLifecycleContractVersionV3 {
		return StopRequestV3{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostV3 Stop request version drifted")
	}
	r.ContractVersion = HostLifecycleContractVersionV3
	p := r.RequestDigest
	r.RequestDigest = ""
	d, e := r.digestV3()
	if e != nil {
		return StopRequestV3{}, e
	}
	if p != "" && p != d {
		return StopRequestV3{}, NewError(ErrorConflict, "host_v3_stop_request_drift", "HostV3 Stop request supplied a wrong digest")
	}
	r.RequestDigest = d
	return r, r.Validate()
}
func (r StopRequestV3) Validate() error {
	if r.ContractVersion != HostLifecycleContractVersionV3 || r.RequestedAtUnixNano <= 0 || r.RequestedNotAfterUnixNano <= r.RequestedAtUnixNano {
		return NewError(ErrorInvalidArgument, "host_v3_stop_request_incomplete", "HostV3 Stop request is incomplete")
	}
	if e := ValidateIdentifierV1("host id", r.HostID); e != nil {
		return e
	}
	if e := ValidateIdentifierV1("start id", r.StartID); e != nil {
		return e
	}
	if e := r.StartClaim.Validate(); e != nil {
		return e
	}
	if e := r.CleanupClosure.Validate(); e != nil {
		return e
	}
	if r.StartClaim.HostID != r.HostID || r.StartClaim.StartID != r.StartID {
		return NewError(ErrorConflict, "host_v3_stop_claim_drift", "HostV3 Stop claim coordinates drifted")
	}
	d, e := r.digestV3()
	if e != nil || d != r.RequestDigest {
		return NewError(ErrorConflict, "host_v3_stop_request_drift", "HostV3 Stop request digest drifted")
	}
	return nil
}

func (r StopRequestV3) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.RequestedAtUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "HostV3 Stop request clock regressed")
	}
	if !now.Before(time.Unix(0, r.RequestedNotAfterUnixNano)) {
		return NewError(ErrorPrecondition, "host_v3_stop_request_expired", "HostV3 Stop request expired")
	}
	return nil
}

type StopResultV3 struct {
	ContractVersion string         `json:"contract_version"`
	RequestDigest   DigestV1       `json:"request_digest"`
	Journal         ExactRefV1     `json:"journal"`
	CleanupClosure  ExactRefV1     `json:"cleanup_closure"`
	CleanupResult   ExactRefV1     `json:"cleanup_result"`
	State           CleanupStateV1 `json:"state"`
	CheckedUnixNano int64          `json:"checked_unix_nano"`
	ResultDigest    DigestV1       `json:"result_digest"`
}

func (r StopResultV3) digestV3() (DigestV1, error) {
	r.ResultDigest = ""
	return DigestJSONV1(struct {
		Domain string       `json:"domain"`
		Type   string       `json:"type"`
		Body   StopResultV3 `json:"body"`
	}{"praxis.agent-host.lifecycle-v3", "StopResultV3", r})
}
func SealStopResultV3(r StopResultV3) (StopResultV3, error) {
	if r.ContractVersion != "" && r.ContractVersion != HostLifecycleContractVersionV3 {
		return StopResultV3{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostV3 Stop result version drifted")
	}
	r.ContractVersion = HostLifecycleContractVersionV3
	p := r.ResultDigest
	r.ResultDigest = ""
	d, e := r.digestV3()
	if e != nil {
		return StopResultV3{}, e
	}
	if p != "" && p != d {
		return StopResultV3{}, NewError(ErrorConflict, "host_v3_stop_result_drift", "HostV3 Stop result supplied a wrong digest")
	}
	r.ResultDigest = d
	return r, r.Validate()
}
func (r StopResultV3) Validate() error {
	if r.ContractVersion != HostLifecycleContractVersionV3 || r.CheckedUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "host_v3_stop_result_incomplete", "HostV3 Stop result is incomplete")
	}
	if e := r.RequestDigest.Validate(); e != nil {
		return e
	}
	for _, ref := range []ExactRefV1{r.Journal, r.CleanupClosure, r.CleanupResult} {
		if e := ref.Validate(); e != nil {
			return e
		}
	}
	if r.State != CleanupClosedV1 && r.State != CleanupResidualV1 && r.State != CleanupIndeterminateV1 {
		return NewError(ErrorInvalidArgument, "host_v3_stop_state_invalid", "HostV3 Stop result state is invalid")
	}
	d, e := r.digestV3()
	if e != nil || d != r.ResultDigest {
		return NewError(ErrorConflict, "host_v3_stop_result_drift", "HostV3 Stop result digest drifted")
	}
	return nil
}

func (r StopResultV3) ValidateFor(request StopRequestV3) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if r.RequestDigest != request.RequestDigest || r.CleanupClosure != request.CleanupClosure {
		return NewError(ErrorConflict, "host_v3_stop_result_request_drift", "HostV3 Stop result does not bind the exact request and Cleanup Closure")
	}
	return nil
}
