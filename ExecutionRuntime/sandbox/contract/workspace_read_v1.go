package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	WorkspaceReadContractVersionV1 = "praxis.sandbox/workspace-read/v1"
	WorkspaceReadMaxBytesV1        = uint64(1 << 20)
	WorkspaceReadCommandKindV1     = "praxis.sandbox/workspace-read-command"
)

type WorkspaceReadStateV1 string

const (
	WorkspaceReadStartedV1  WorkspaceReadStateV1 = "started"
	WorkspaceReadObservedV1 WorkspaceReadStateV1 = "observed"
	WorkspaceReadUnknownV1  WorkspaceReadStateV1 = "indeterminate"
	WorkspaceReadFailedV1   WorkspaceReadStateV1 = "failed"
)

type WorkspaceReadCommandV1 struct {
	Meta                      Meta   `json:"meta"`
	TenantID                  string `json:"tenant_id"`
	SourceToolCommand         Ref    `json:"source_tool_command"`
	SourceToolPayloadSchema   string `json:"source_tool_payload_schema"`
	SourceToolPayloadDigest   string `json:"source_tool_payload_digest"`
	SourceToolPayloadRevision uint64 `json:"source_tool_payload_revision"`
	WorkspaceView             Ref    `json:"workspace_view"`
	FileScopeDigest           string `json:"file_scope_digest"`
	RelativePath              string `json:"relative_path"`
	StartByte                 uint64 `json:"start_byte"`
	MaxBytes                  uint64 `json:"max_bytes"`
	ExpectedFileRef           *Ref   `json:"expected_file_ref,omitempty"`
	RequestedNotAfterUnixNano int64  `json:"requested_not_after_unix_nano"`
	OperationDigest           string `json:"operation_digest"`
	EffectID                  string `json:"effect_id"`
	IntentRevision            uint64 `json:"intent_revision"`
	IntentDigest              string `json:"intent_digest"`
	AttemptID                 string `json:"attempt_id"`
	PreparedDigest            string `json:"prepared_digest"`
	DispatchDigest            string `json:"dispatch_digest"`
	ProviderComponent         string `json:"provider_component"`
	ProviderManifest          string `json:"provider_manifest"`
}

func (c WorkspaceReadCommandV1) ValidateShape() error {
	if err := c.Meta.ValidateShape(); err != nil {
		return err
	}
	if strings.TrimSpace(c.TenantID) == "" || c.SourceToolCommand.ValidateShape("source Tool command") != nil || strings.TrimSpace(c.SourceToolPayloadSchema) == "" || !ValidDigest(c.SourceToolPayloadDigest) || c.SourceToolPayloadRevision == 0 || c.WorkspaceView.ValidateShape("workspace view") != nil || !ValidDigest(c.FileScopeDigest) || ValidateLogicalPath(c.RelativePath) != nil || c.MaxBytes == 0 || c.MaxBytes > WorkspaceReadMaxBytesV1 || c.RequestedNotAfterUnixNano <= 0 || c.RequestedNotAfterUnixNano > c.Meta.ExpiresUnixNano {
		return errors.New("workspace read command coordinates are incomplete")
	}
	if c.ExpectedFileRef != nil && c.ExpectedFileRef.ValidateShape("expected file") != nil {
		return errors.New("workspace read expected file ref is invalid")
	}
	if !ValidDigest(c.OperationDigest) || strings.TrimSpace(c.EffectID) == "" || c.IntentRevision == 0 || !ValidDigest(c.IntentDigest) || strings.TrimSpace(c.AttemptID) == "" || !ValidDigest(c.PreparedDigest) || !ValidDigest(c.DispatchDigest) || strings.TrimSpace(c.ProviderComponent) == "" || !ValidDigest(c.ProviderManifest) {
		return errors.New("workspace read Runtime coordinates are incomplete")
	}
	expected, err := Digest("workspace-read-command", c.digestPayload())
	if err != nil || expected != c.Meta.Digest {
		return errors.New("workspace read command digest drifted")
	}
	return nil
}
func (c WorkspaceReadCommandV1) ValidateCurrent(now time.Time) error {
	if err := c.ValidateShape(); err != nil {
		return err
	}
	if err := c.Meta.ValidateCurrent(now); err != nil {
		return err
	}
	if now.UnixNano() >= c.RequestedNotAfterUnixNano {
		return errors.New("workspace read requested lifetime expired")
	}
	return nil
}
func (c WorkspaceReadCommandV1) digestPayload() any {
	copy := c
	copy.Meta = Meta{ExpiresUnixNano: c.Meta.ExpiresUnixNano}
	return copy
}
func SealWorkspaceReadCommandV1(c WorkspaceReadCommandV1, id string, now, expires time.Time) (WorkspaceReadCommandV1, error) {
	c.Meta.ExpiresUnixNano = expires.UnixNano()
	if c.RequestedNotAfterUnixNano == 0 {
		c.RequestedNotAfterUnixNano = expires.UnixNano()
	}
	m, e := NewMeta(id, 1, now, expires, "workspace-read-command", c.digestPayload())
	if e != nil {
		return WorkspaceReadCommandV1{}, e
	}
	c.Meta = m
	return c, c.ValidateCurrent(now)
}

type WorkspaceReadReservationV1 struct {
	Meta                Meta                      `json:"meta"`
	StableKeyDigest     string                    `json:"stable_key_digest"`
	AuthorizationDigest string                    `json:"authorization_digest"`
	RequestDigest       string                    `json:"request_digest"`
	PayloadDigest       string                    `json:"payload_digest"`
	Command             Ref                       `json:"command"`
	WorkspaceView       Ref                       `json:"workspace_view"`
	AttemptID           string                    `json:"attempt_id"`
	TTLClosure          WorkspaceReadTTLClosureV1 `json:"ttl_closure"`
}

// WorkspaceReadTTLClosureV1 seals every upstream lifetime that was current
// before the Sandbox owner reserved the physical attempt. The durable store can
// therefore reject an overlong reservation without importing another owner's
// mutable current.
type WorkspaceReadTTLClosureV1 struct {
	UnifiedNotAfterUnixNano       int64  `json:"unified_not_after_unix_nano"`
	RuntimeEnforcementExpiresNano int64  `json:"runtime_enforcement_expires_unix_nano"`
	AssociationExpiresUnixNano    int64  `json:"association_expires_unix_nano"`
	CommandRequestedNotAfterNano  int64  `json:"command_requested_not_after_unix_nano"`
	CommandExpiresUnixNano        int64  `json:"command_expires_unix_nano"`
	WorkspaceViewExpiresUnixNano  int64  `json:"workspace_view_expires_unix_nano"`
	WorkspaceLeaseExpiresUnixNano int64  `json:"workspace_lease_expires_unix_nano"`
	EffectiveExpiresUnixNano      int64  `json:"effective_expires_unix_nano"`
	Digest                        string `json:"digest"`
}

func SealWorkspaceReadTTLClosureV1(value WorkspaceReadTTLClosureV1) (WorkspaceReadTTLClosureV1, error) {
	value.EffectiveExpiresUnixNano = minInt64V1(
		value.UnifiedNotAfterUnixNano,
		value.RuntimeEnforcementExpiresNano,
		value.AssociationExpiresUnixNano,
		value.CommandRequestedNotAfterNano,
		value.CommandExpiresUnixNano,
		value.WorkspaceViewExpiresUnixNano,
		value.WorkspaceLeaseExpiresUnixNano,
	)
	value.Digest = ""
	digest, err := Digest("workspace-read-ttl-closure", value)
	if err != nil {
		return WorkspaceReadTTLClosureV1{}, err
	}
	value.Digest = digest
	if err = value.ValidateShape(); err != nil {
		return WorkspaceReadTTLClosureV1{}, err
	}
	return value, nil
}

func (value WorkspaceReadTTLClosureV1) ValidateShape() error {
	expectedExpiry := minInt64V1(
		value.UnifiedNotAfterUnixNano,
		value.RuntimeEnforcementExpiresNano,
		value.AssociationExpiresUnixNano,
		value.CommandRequestedNotAfterNano,
		value.CommandExpiresUnixNano,
		value.WorkspaceViewExpiresUnixNano,
		value.WorkspaceLeaseExpiresUnixNano,
	)
	copy := value
	copy.Digest = ""
	digest, err := Digest("workspace-read-ttl-closure", copy)
	if err != nil ||
		value.UnifiedNotAfterUnixNano <= 0 ||
		value.RuntimeEnforcementExpiresNano <= 0 ||
		value.AssociationExpiresUnixNano <= 0 ||
		value.CommandRequestedNotAfterNano <= 0 ||
		value.CommandExpiresUnixNano <= 0 ||
		value.WorkspaceViewExpiresUnixNano <= 0 ||
		value.WorkspaceLeaseExpiresUnixNano <= 0 ||
		value.EffectiveExpiresUnixNano != expectedExpiry ||
		value.Digest != digest {
		return errors.New("workspace read TTL closure is incomplete")
	}
	return nil
}

func (value WorkspaceReadTTLClosureV1) ValidateCurrent(now time.Time) error {
	if err := value.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() >= value.EffectiveExpiresUnixNano {
		return errors.New("workspace read TTL closure expired")
	}
	return nil
}

func (r WorkspaceReadReservationV1) ValidateShape() error {
	if err := r.Meta.ValidateShape(); err != nil {
		return err
	}
	if !ValidDigest(r.StableKeyDigest) || !ValidDigest(r.AuthorizationDigest) || !ValidDigest(r.RequestDigest) || !ValidDigest(r.PayloadDigest) || r.Command.ValidateShape("command") != nil || r.WorkspaceView.ValidateShape("workspace view") != nil || strings.TrimSpace(r.AttemptID) == "" || r.TTLClosure.ValidateShape() != nil || r.Meta.ExpiresUnixNano != r.TTLClosure.EffectiveExpiresUnixNano {
		return errors.New("workspace read reservation is incomplete")
	}
	d, e := Digest("workspace-read-reservation", r.digestPayload())
	if e != nil || d != r.Meta.Digest {
		return errors.New("workspace read reservation digest drifted")
	}
	return nil
}
func (r WorkspaceReadReservationV1) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	if err := r.Meta.ValidateCurrent(now); err != nil {
		return err
	}
	return r.TTLClosure.ValidateCurrent(now)
}
func (r WorkspaceReadReservationV1) digestPayload() any {
	copy := r
	copy.Meta = Meta{ExpiresUnixNano: r.Meta.ExpiresUnixNano}
	return copy
}
func SealWorkspaceReadReservationV1(r WorkspaceReadReservationV1, id string, now, expires time.Time) (WorkspaceReadReservationV1, error) {
	if r.TTLClosure.Digest == "" {
		return WorkspaceReadReservationV1{}, errors.New("workspace read reservation requires a sealed TTL closure")
	}
	r.Meta.ExpiresUnixNano = expires.UnixNano()
	m, e := NewMeta(id, 1, now, expires, "workspace-read-reservation", r.digestPayload())
	if e != nil {
		return WorkspaceReadReservationV1{}, e
	}
	r.Meta = m
	return r, r.ValidateShape()
}

type WorkspaceReadReceiptBindingV1 struct {
	ID                string `json:"id"`
	Revision          uint64 `json:"revision"`
	Digest            string `json:"digest"`
	ObservationDigest string `json:"observation_digest,omitempty"`
	StableKeyDigest   string `json:"stable_key_digest"`
	CheckedUnixNano   int64  `json:"checked_unix_nano"`
	ExpiresUnixNano   int64  `json:"expires_unix_nano"`
}

func (r WorkspaceReadReceiptBindingV1) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 || !ValidDigest(r.Digest) || r.ObservationDigest != "" && !ValidDigest(r.ObservationDigest) || !ValidDigest(r.StableKeyDigest) || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano {
		return errors.New("workspace read receipt binding is incomplete")
	}
	return nil
}
func (r WorkspaceReadReceiptBindingV1) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.CheckedUnixNano || now.UnixNano() >= r.ExpiresUnixNano {
		return errors.New("workspace read receipt binding is expired or from the future")
	}
	return nil
}

// WorkspaceReadAttemptRefV1 is the only public recovery coordinate. Stable
// digests remain Owner-internal indices and are never caller-selected current.
type WorkspaceReadAttemptRefV1 struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   string `json:"digest"`
}

func (r WorkspaceReadAttemptRefV1) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 || !ValidDigest(r.Digest) {
		return errors.New("workspace read attempt ref is incomplete")
	}
	return nil
}
func (r WorkspaceReadAttemptRefV1) OwnerRef() Ref {
	return Ref{ID: r.ID, Revision: r.Revision, Digest: r.Digest}
}

type WorkspaceReadObservationV1 struct {
	Meta              Meta                          `json:"meta"`
	Reservation       Ref                           `json:"reservation"`
	Command           Ref                           `json:"command"`
	WorkspaceView     Ref                           `json:"workspace_view"`
	File              Ref                           `json:"file"`
	RelativePath      string                        `json:"relative_path"`
	StartByte         uint64                        `json:"start_byte"`
	ReturnedBytes     uint64                        `json:"returned_bytes"`
	TotalBytes        uint64                        `json:"total_bytes"`
	Complete          bool                          `json:"complete"`
	Content           string                        `json:"content"`
	ContentDigest     string                        `json:"content_digest"`
	S1CheckedUnixNano int64                         `json:"s1_checked_unix_nano"`
	S2CheckedUnixNano int64                         `json:"s2_checked_unix_nano"`
	AdmissionReceipt  WorkspaceReadReceiptBindingV1 `json:"admission_receipt"`
	ProviderReceipt   WorkspaceReadReceiptBindingV1 `json:"provider_receipt"`
}

func (o WorkspaceReadObservationV1) ValidateShape() error {
	if err := o.Meta.ValidateShape(); err != nil {
		return err
	}
	if o.Reservation.ValidateShape("reservation") != nil || o.Command.ValidateShape("command") != nil || o.WorkspaceView.ValidateShape("workspace view") != nil || o.File.ValidateShape("file") != nil || ValidateLogicalPath(o.RelativePath) != nil || o.ReturnedBytes != uint64(len([]byte(o.Content))) || o.TotalBytes > WorkspaceReadMaxBytesV1 || o.StartByte > o.TotalBytes || o.ReturnedBytes > o.TotalBytes-o.StartByte || o.Complete != (o.StartByte+o.ReturnedBytes == o.TotalBytes) || !ValidDigest(o.ContentDigest) || o.S1CheckedUnixNano <= 0 || o.S2CheckedUnixNano < o.S1CheckedUnixNano || o.S2CheckedUnixNano > o.Meta.CreatedUnixNano || o.AdmissionReceipt.Validate() != nil || o.ProviderReceipt.Validate() != nil || !ValidDigest(o.ProviderReceipt.ObservationDigest) || o.AdmissionReceipt.ObservationDigest != "" || o.AdmissionReceipt.CheckedUnixNano > o.S1CheckedUnixNano || o.ProviderReceipt.CheckedUnixNano > o.S2CheckedUnixNano || o.AdmissionReceipt.StableKeyDigest != o.ProviderReceipt.StableKeyDigest || o.Meta.ExpiresUnixNano > o.AdmissionReceipt.ExpiresUnixNano || o.Meta.ExpiresUnixNano > o.ProviderReceipt.ExpiresUnixNano {
		return errors.New("workspace read observation is incomplete")
	}
	if o.ContentDigest != WorkspaceReadContentDigestV1([]byte(o.Content), o.StartByte, o.TotalBytes, o.Complete) {
		return errors.New("workspace read range content digest drifted")
	}
	d, e := Digest("workspace-read-observation", o.digestPayload())
	if e != nil || d != o.Meta.Digest {
		return errors.New("workspace read observation digest drifted")
	}
	return nil
}
func (o WorkspaceReadObservationV1) ValidateCurrent(now time.Time) error {
	if err := o.ValidateShape(); err != nil {
		return err
	}
	if err := o.Meta.ValidateCurrent(now); err != nil {
		return err
	}
	if now.UnixNano() < o.S1CheckedUnixNano || now.UnixNano() < o.S2CheckedUnixNano || o.AdmissionReceipt.ValidateCurrent(now) != nil || o.ProviderReceipt.ValidateCurrent(now) != nil {
		return errors.New("workspace read observation currentness expired or is from the future")
	}
	return nil
}
func (o WorkspaceReadObservationV1) digestPayload() any {
	copy := o
	copy.Meta = Meta{ExpiresUnixNano: o.Meta.ExpiresUnixNano}
	return copy
}
func SealWorkspaceReadObservationV1(o WorkspaceReadObservationV1, id string, now, expires time.Time) (WorkspaceReadObservationV1, error) {
	o.Meta.ExpiresUnixNano = expires.UnixNano()
	m, e := NewMeta(id, 1, now, expires, "workspace-read-observation", o.digestPayload())
	if e != nil {
		return WorkspaceReadObservationV1{}, e
	}
	o.Meta = m
	return o, o.ValidateShape()
}
func WorkspaceReadContentDigestV1(content []byte, start, total uint64, complete bool) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "praxis.sandbox/workspace-read-range/v1%c%d%c%d%c%t%c", 0, start, 0, total, 0, complete, 0)
	_, _ = h.Write(content)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// WorkspaceReadFileIDV1 binds the public file identity to the exact
// WorkspaceView and canonical logical path. The whole-file digest remains the
// content coordinate while this ID prevents a valid file ref from being
// spliced across another path in the same view.
func WorkspaceReadFileIDV1(workspaceViewID, relativePath string) (string, error) {
	if strings.TrimSpace(workspaceViewID) == "" || ValidateLogicalPath(relativePath) != nil {
		return "", errors.New("workspace read file identity coordinates are invalid")
	}
	sum := sha256.Sum256([]byte(workspaceViewID + "\x00" + relativePath))
	return "workspace-file-" + hex.EncodeToString(sum[:]), nil
}

type WorkspaceReadAttemptV1 struct {
	Meta             Meta                          `json:"meta"`
	StableKeyDigest  string                        `json:"stable_key_digest"`
	RequestDigest    string                        `json:"request_digest"`
	PayloadDigest    string                        `json:"payload_digest"`
	Reservation      Ref                           `json:"reservation"`
	AdmissionReceipt WorkspaceReadReceiptBindingV1 `json:"admission_receipt"`
	State            WorkspaceReadStateV1          `json:"state"`
	Observation      *Ref                          `json:"observation,omitempty"`
	UnknownDigest    string                        `json:"unknown_digest,omitempty"`
	FailureDigest    string                        `json:"failure_digest,omitempty"`
}

func (a WorkspaceReadAttemptV1) digestPayload() any {
	copy := a
	copy.Meta = Meta{ExpiresUnixNano: a.Meta.ExpiresUnixNano}
	return copy
}
func SealWorkspaceReadAttemptV1(a WorkspaceReadAttemptV1, id string, revision uint64, now, expires time.Time) (WorkspaceReadAttemptV1, error) {
	a.Meta.ExpiresUnixNano = expires.UnixNano()
	m, e := NewMeta(id, revision, now, expires, "workspace-read-attempt", a.digestPayload())
	if e != nil {
		return WorkspaceReadAttemptV1{}, e
	}
	a.Meta = m
	return a, a.ValidateShape()
}
func (a WorkspaceReadAttemptV1) ValidateShape() error {
	if err := a.Meta.ValidateShape(); err != nil {
		return err
	}
	if !ValidDigest(a.StableKeyDigest) || !ValidDigest(a.RequestDigest) || !ValidDigest(a.PayloadDigest) || a.Reservation.ValidateShape("reservation") != nil || a.AdmissionReceipt.Validate() != nil || a.AdmissionReceipt.CheckedUnixNano > a.Meta.CreatedUnixNano || a.AdmissionReceipt.StableKeyDigest != a.StableKeyDigest || a.Meta.ExpiresUnixNano > a.AdmissionReceipt.ExpiresUnixNano {
		return errors.New("workspace read attempt is incomplete")
	}
	switch a.State {
	case WorkspaceReadStartedV1:
		if a.Observation != nil || a.UnknownDigest != "" || a.FailureDigest != "" {
			return errors.New("reserved read carries terminal data")
		}
	case WorkspaceReadObservedV1:
		if a.Observation == nil || a.Observation.ValidateShape("observation") != nil || a.UnknownDigest != "" || a.FailureDigest != "" {
			return errors.New("observed read lacks observation")
		}
	case WorkspaceReadUnknownV1:
		if a.Observation != nil || !ValidDigest(a.UnknownDigest) || a.FailureDigest != "" {
			return errors.New("indeterminate read lacks digest")
		}
	case WorkspaceReadFailedV1:
		if a.Observation != nil || a.UnknownDigest != "" || !ValidDigest(a.FailureDigest) {
			return errors.New("failed read lacks deterministic failure digest")
		}
	default:
		return errors.New("workspace read state is invalid")
	}
	d, e := Digest("workspace-read-attempt", a.digestPayload())
	if e != nil || d != a.Meta.Digest {
		return errors.New("workspace read attempt digest drifted")
	}
	return nil
}
func (a WorkspaceReadAttemptV1) ValidateCurrent(now time.Time) error {
	if err := a.ValidateShape(); err != nil {
		return err
	}
	if err := a.Meta.ValidateCurrent(now); err != nil {
		return err
	}
	if err := a.AdmissionReceipt.ValidateCurrent(now); err != nil {
		return errors.New("workspace read admission receipt expired")
	}
	return nil
}

type WorkspaceReadExecutionProjectionV1 struct {
	Attempt          WorkspaceReadAttemptV1         `json:"attempt"`
	Reservation      WorkspaceReadReservationV1     `json:"reservation"`
	AdmissionReceipt WorkspaceReadReceiptBindingV1  `json:"admission_receipt"`
	Observation      *WorkspaceReadObservationV1    `json:"observation,omitempty"`
	ProviderReceipt  *WorkspaceReadReceiptBindingV1 `json:"provider_receipt,omitempty"`
}

func (p WorkspaceReadExecutionProjectionV1) ValidateShape() error {
	if err := p.Attempt.ValidateShape(); err != nil {
		return err
	}
	if err := p.Reservation.ValidateShape(); err != nil {
		return err
	}
	expectedAttemptExpiry := minInt64V1(p.Reservation.Meta.ExpiresUnixNano, p.AdmissionReceipt.ExpiresUnixNano)
	if !SameRef(p.Attempt.Reservation, p.Reservation.Meta.Ref()) || p.Attempt.RequestDigest != p.Reservation.RequestDigest || p.Attempt.PayloadDigest != p.Reservation.PayloadDigest || p.AdmissionReceipt != p.Attempt.AdmissionReceipt || p.AdmissionReceipt.Validate() != nil || p.Attempt.Meta.ExpiresUnixNano != expectedAttemptExpiry {
		return errors.New("workspace read projection reservation drifted")
	}
	if p.Attempt.State == WorkspaceReadObservedV1 {
		if p.Observation == nil || p.ProviderReceipt == nil || p.Observation.ValidateShape() != nil || !SameRef(*p.Attempt.Observation, p.Observation.Meta.Ref()) || p.Observation.AdmissionReceipt != p.AdmissionReceipt || p.Observation.ProviderReceipt != *p.ProviderReceipt || p.Observation.Meta.ExpiresUnixNano != minInt64V1(p.Attempt.Meta.ExpiresUnixNano, p.Reservation.Meta.ExpiresUnixNano, p.AdmissionReceipt.ExpiresUnixNano, p.ProviderReceipt.ExpiresUnixNano) {
			return errors.New("workspace read projection observation drifted")
		}
	} else if p.Observation != nil || p.ProviderReceipt != nil {
		return errors.New("non-observed projection carries observation")
	}
	return nil
}

func minInt64V1(values ...int64) int64 {
	if len(values) == 0 {
		return 0
	}
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
