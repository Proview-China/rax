package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	WorkspaceReadCommandPublicationContractVersionV2 = "praxis.sandbox/workspace-read-command-publication/v2"
	WorkspaceReadSourceCurrentTypeURLV2              = "type.googleapis.com/praxis.sandbox.WorkspaceReadSourceCurrentProjectionV2"
	WorkspaceReadCommandPublicationTypeURLV2         = "type.googleapis.com/praxis.sandbox.WorkspaceReadCommandPublicationV2"
	WorkspaceReadCommandOwnerCurrentTypeURLV2        = "type.googleapis.com/praxis.sandbox.WorkspaceReadCommandOwnerCurrentV2"
	WorkspaceReadSourceCommandKindV2                 = runtimeports.NamespacedNameV2("praxis.tool/workspace-read-execution-command")
	WorkspaceReadEffectDispatchIntentV2              = "dispatch_intent"
	WorkspaceReadSourceCurrentMaxTTLV2               = 15 * time.Second
	WorkspaceReadCommandOwnerCurrentMaxTTLV2         = 15 * time.Second
)

const workspaceReadCommandPublicationCanonicalDomainV2 = "praxis.sandbox.workspace-read-command-publication"

// WorkspaceReadSourceCommandRefV2 is a coordinate-only exact reference. Its
// digest deliberately uses the Sandbox raw lowercase representation because
// WorkspaceReadCommandV1.SourceToolCommand uses contract.Ref.
type WorkspaceReadSourceCommandRefV2 struct {
	Owner    runtimeports.EffectOwnerRefV2 `json:"owner"`
	Kind     runtimeports.NamespacedNameV2 `json:"kind"`
	ID       string                        `json:"id"`
	Revision runtimecore.Revision          `json:"revision"`
	Digest   string                        `json:"digest"`
}

func (r WorkspaceReadSourceCommandRefV2) Validate() error {
	if validateWorkspaceReadSourceOwnerV2(r.Owner) != nil ||
		r.Kind != WorkspaceReadSourceCommandKindV2 ||
		strings.TrimSpace(r.ID) == "" ||
		r.Revision != 1 ||
		!validRawLowerDigestV2(r.Digest) {
		return errors.New("workspace read source command exact Ref is incomplete or noncanonical")
	}
	return nil
}

func (r WorkspaceReadSourceCommandRefV2) ContractRefV2() (Ref, error) {
	if err := r.Validate(); err != nil {
		return Ref{}, err
	}
	result := Ref{ID: r.ID, Revision: uint64(r.Revision), Digest: r.Digest}
	return result, result.ValidateShape("source command")
}

// WorkspaceReadSourceWorkspaceRefV2 mirrors the canonical inline payload.
// Unlike Sandbox contract.Ref, its digest is the Runtime/Tool core digest.
type WorkspaceReadSourceWorkspaceRefV2 struct {
	ID       string               `json:"id"`
	Revision runtimecore.Revision `json:"revision"`
	Digest   runtimecore.Digest   `json:"digest"`
}

func (r WorkspaceReadSourceWorkspaceRefV2) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 || !validCoreLowerDigestV2(r.Digest) {
		return errors.New("workspace read source workspace exact Ref is incomplete or noncanonical")
	}
	return nil
}

func (r WorkspaceReadSourceWorkspaceRefV2) ContractRefV2() (Ref, error) {
	if err := r.Validate(); err != nil {
		return Ref{}, err
	}
	raw := strings.TrimPrefix(string(r.Digest), "sha256:")
	result := Ref{ID: r.ID, Revision: uint64(r.Revision), Digest: raw}
	if err := result.ValidateShape("workspace view"); err != nil {
		return Ref{}, err
	}
	if "sha256:"+result.Digest != string(r.Digest) {
		return Ref{}, errors.New("workspace read workspace digest round-trip drifted")
	}
	return result, nil
}

// WorkspaceReadCanonicalPayloadV2 is the bounded neutral workspace.read body.
// Field order is part of its canonical JSON representation.
type WorkspaceReadCanonicalPayloadV2 struct {
	WorkspaceRoot     WorkspaceReadSourceWorkspaceRefV2 `json:"workspace_root"`
	RelativePath      string                            `json:"relative_path"`
	StartByte         uint64                            `json:"start_byte"`
	MaxBytes          uint64                            `json:"max_bytes"`
	RequestedNotAfter int64                             `json:"requested_not_after_unix_nano"`
}

func (p WorkspaceReadCanonicalPayloadV2) Validate() error {
	if p.WorkspaceRoot.Validate() != nil ||
		ValidateLogicalPath(p.RelativePath) != nil ||
		p.MaxBytes == 0 ||
		p.MaxBytes > WorkspaceReadMaxBytesV1 ||
		p.RequestedNotAfter <= 0 {
		return errors.New("workspace read canonical inline payload is invalid or unbounded")
	}
	return nil
}

// WorkspaceReadSourceCurrentProjectionV2 is a Sandbox-neutral consumer
// projection. A future external adapter must derive it from its authoritative
// exact/current fact; this DTO grants no authority by itself.
type WorkspaceReadSourceCurrentProjectionV2 struct {
	ContractVersion           string                                     `json:"contract_version"`
	TypeURL                   string                                     `json:"type_url"`
	SourceCommand             WorkspaceReadSourceCommandRefV2            `json:"source_command"`
	Operation                 runtimeports.OperationSubjectV3            `json:"operation"`
	OperationDigest           runtimecore.Digest                         `json:"operation_digest"`
	Prepared                  runtimeports.PreparedProviderAttemptRefV2  `json:"prepared"`
	PreparedSemanticDigest    runtimecore.Digest                         `json:"prepared_semantic_digest"`
	RuntimeAttempt            runtimeports.OperationDispatchAttemptRefV3 `json:"runtime_attempt"`
	RuntimeAttemptDigest      runtimecore.Digest                         `json:"runtime_attempt_digest"`
	RuntimeEffectIntentDigest runtimecore.Digest                         `json:"runtime_effect_intent_digest"`
	RuntimeEffectFactRevision runtimecore.Revision                       `json:"runtime_effect_fact_revision"`
	RuntimeEffectState        string                                     `json:"runtime_effect_state"`
	PayloadSchema             runtimeports.SchemaRefV2                   `json:"payload_schema"`
	PayloadDigest             runtimecore.Digest                         `json:"payload_digest"`
	PayloadRevision           runtimecore.Revision                       `json:"payload_revision"`
	CanonicalInline           []byte                                     `json:"canonical_inline"`
	CanonicalInlineLength     uint64                                     `json:"canonical_inline_length"`
	WorkspaceView             Ref                                        `json:"workspace_view"`
	RelativePath              string                                     `json:"relative_path"`
	StartByte                 uint64                                     `json:"start_byte"`
	MaxBytes                  uint64                                     `json:"max_bytes"`
	RequestedNotAfterUnixNano int64                                      `json:"requested_not_after_unix_nano"`
	SourceCreatedUnixNano     int64                                      `json:"source_created_unix_nano"`
	SourceNotAfterUnixNano    int64                                      `json:"source_not_after_unix_nano"`
	CheckedUnixNano           int64                                      `json:"checked_unix_nano"`
	ExpiresUnixNano           int64                                      `json:"expires_unix_nano"`
	ProjectionDigest          runtimecore.Digest                         `json:"projection_digest"`
}

func (p WorkspaceReadSourceCurrentProjectionV2) ValidateShape() error {
	if p.ContractVersion != WorkspaceReadCommandPublicationContractVersionV2 ||
		p.TypeURL != WorkspaceReadSourceCurrentTypeURLV2 ||
		p.SourceCommand.Validate() != nil ||
		p.Operation.Validate() != nil ||
		!validCoreLowerDigestV2(p.OperationDigest) ||
		p.Prepared.Validate() != nil ||
		!validCoreLowerDigestV2(p.PreparedSemanticDigest) ||
		p.RuntimeAttempt.Validate() != nil ||
		!validCoreLowerDigestV2(p.RuntimeAttemptDigest) ||
		!validCoreLowerDigestV2(p.RuntimeEffectIntentDigest) ||
		p.RuntimeEffectFactRevision == 0 ||
		p.RuntimeEffectState != WorkspaceReadEffectDispatchIntentV2 ||
		p.PayloadSchema.Validate() != nil ||
		!validCoreLowerDigestV2(p.PayloadDigest) ||
		p.PayloadRevision == 0 ||
		len(p.CanonicalInline) == 0 ||
		len(p.CanonicalInline) > runtimeports.MaxOpaqueInlineBytes ||
		p.CanonicalInlineLength != uint64(len(p.CanonicalInline)) ||
		p.WorkspaceView.ValidateShape("workspace view") != nil ||
		!validRawLowerDigestV2(p.WorkspaceView.Digest) ||
		ValidateLogicalPath(p.RelativePath) != nil ||
		p.MaxBytes == 0 ||
		p.MaxBytes > WorkspaceReadMaxBytesV1 ||
		p.RequestedNotAfterUnixNano <= 0 ||
		p.SourceCreatedUnixNano <= 0 ||
		p.SourceNotAfterUnixNano <= p.SourceCreatedUnixNano ||
		p.CheckedUnixNano < p.SourceCreatedUnixNano ||
		p.ExpiresUnixNano <= p.CheckedUnixNano ||
		p.ExpiresUnixNano > p.SourceNotAfterUnixNano ||
		time.Duration(p.ExpiresUnixNano-p.CheckedUnixNano) > WorkspaceReadSourceCurrentMaxTTLV2 ||
		!validCoreLowerDigestV2(p.ProjectionDigest) {
		return errors.New("workspace read source current projection is incomplete")
	}
	operationDigest, err := p.Operation.DigestV3()
	if err != nil || operationDigest != p.OperationDigest ||
		p.Prepared.OperationDigest != p.OperationDigest ||
		p.RuntimeAttempt.OperationDigest != p.OperationDigest ||
		p.Prepared.IntentID != p.RuntimeAttempt.EffectID ||
		p.Prepared.IntentRevision != p.RuntimeAttempt.IntentRevision ||
		p.Prepared.IntentDigest != p.RuntimeAttempt.IntentDigest ||
		p.RuntimeEffectIntentDigest != p.RuntimeAttempt.IntentDigest ||
		p.Prepared.PermitID != p.RuntimeAttempt.PermitID ||
		p.Prepared.PermitRevision != p.RuntimeAttempt.PermitRevision ||
		p.Prepared.PermitDigest != p.RuntimeAttempt.PermitDigest ||
		p.Prepared.AttemptID != p.RuntimeAttempt.AttemptID ||
		p.RuntimeAttempt.Delegation == nil ||
		p.RuntimeAttempt.Delegation.ID != p.Prepared.DeclaredDelegation.ID ||
		p.RuntimeAttempt.Delegation.Revision <= p.Prepared.DeclaredDelegation.Revision ||
		p.Prepared.PayloadSchema != p.PayloadSchema ||
		p.Prepared.PayloadDigest != p.PayloadDigest ||
		p.Prepared.PayloadRevision != p.PayloadRevision ||
		p.SourceCommand.Owner.ComponentID != p.Prepared.Provider.ComponentID ||
		p.SourceCommand.Owner.ManifestDigest != p.Prepared.Provider.ManifestDigest ||
		p.SourceCreatedUnixNano < p.Prepared.PreparedUnixNano ||
		p.SourceNotAfterUnixNano > p.RequestedNotAfterUnixNano ||
		p.SourceNotAfterUnixNano > p.Prepared.ExpiresUnixNano {
		return errors.New("workspace read source Runtime or Owner closure drifted")
	}
	attemptDigest, err := WorkspaceReadSourceRuntimeAttemptDigestV2(p.RuntimeAttempt)
	if err != nil || attemptDigest != p.RuntimeAttemptDigest {
		return errors.New("workspace read source Runtime Attempt digest drifted")
	}
	payload, err := decodeWorkspaceReadCanonicalPayloadV2(p.CanonicalInline)
	if err != nil ||
		runtimecore.DigestBytes(p.CanonicalInline) != p.PayloadDigest ||
		payload.RelativePath != p.RelativePath ||
		payload.StartByte != p.StartByte ||
		payload.MaxBytes != p.MaxBytes ||
		payload.RequestedNotAfter != p.RequestedNotAfterUnixNano {
		return errors.New("workspace read source canonical payload drifted")
	}
	workspaceRef, err := payload.WorkspaceRoot.ContractRefV2()
	if err != nil || !SameRef(workspaceRef, p.WorkspaceView) {
		return errors.New("workspace read source workspace coordinate drifted")
	}
	digest, err := p.ComputeDigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return errors.New("workspace read source current projection digest drifted")
	}
	return nil
}

func (p WorkspaceReadSourceCurrentProjectionV2) ValidateCurrent(expected WorkspaceReadSourceCommandRefV2, now time.Time) error {
	if err := p.ValidateShape(); err != nil {
		return err
	}
	if p.SourceCommand != expected {
		return errors.New("workspace read source current belongs to another exact command")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return errors.New("workspace read source current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return errors.New("workspace read source current expired")
	}
	return nil
}

func (p WorkspaceReadSourceCurrentProjectionV2) ComputeDigestV2() (runtimecore.Digest, error) {
	p = CloneWorkspaceReadSourceCurrentProjectionV2(p)
	p.ProjectionDigest = ""
	return runtimecore.CanonicalJSONDigest(
		workspaceReadCommandPublicationCanonicalDomainV2,
		WorkspaceReadCommandPublicationContractVersionV2,
		"WorkspaceReadSourceCurrentProjectionV2",
		p,
	)
}

func SealWorkspaceReadSourceCurrentProjectionV2(p WorkspaceReadSourceCurrentProjectionV2) (WorkspaceReadSourceCurrentProjectionV2, error) {
	p = CloneWorkspaceReadSourceCurrentProjectionV2(p)
	p.ContractVersion = WorkspaceReadCommandPublicationContractVersionV2
	p.TypeURL = WorkspaceReadSourceCurrentTypeURLV2
	p.ProjectionDigest = ""
	digest, err := p.ComputeDigestV2()
	if err != nil {
		return WorkspaceReadSourceCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, p.ValidateShape()
}

func WorkspaceReadSourceRuntimeAttemptDigestV2(attempt runtimeports.OperationDispatchAttemptRefV3) (runtimecore.Digest, error) {
	if err := attempt.Validate(); err != nil {
		return "", err
	}
	return runtimecore.CanonicalJSONDigest(
		workspaceReadCommandPublicationCanonicalDomainV2,
		WorkspaceReadCommandPublicationContractVersionV2,
		"OperationDispatchAttemptRefV3",
		attempt,
	)
}

func (p WorkspaceReadSourceCurrentProjectionV2) CanonicalPayloadV2() (WorkspaceReadCanonicalPayloadV2, error) {
	if err := p.ValidateShape(); err != nil {
		return WorkspaceReadCanonicalPayloadV2{}, err
	}
	return decodeWorkspaceReadCanonicalPayloadV2(p.CanonicalInline)
}

func (p WorkspaceReadSourceCurrentProjectionV2) StableSemanticDigestV2() (runtimecore.Digest, error) {
	p = CloneWorkspaceReadSourceCurrentProjectionV2(p)
	p.CheckedUnixNano = 0
	p.ExpiresUnixNano = 0
	p.ProjectionDigest = ""
	return runtimecore.CanonicalJSONDigest(
		workspaceReadCommandPublicationCanonicalDomainV2,
		WorkspaceReadCommandPublicationContractVersionV2,
		"WorkspaceReadSourceStableSemanticV2",
		p,
	)
}

func SameWorkspaceReadSourceSemanticV2(left, right WorkspaceReadSourceCurrentProjectionV2) bool {
	leftDigest, leftErr := left.StableSemanticDigestV2()
	rightDigest, rightErr := right.StableSemanticDigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func CloneWorkspaceReadSourceCurrentProjectionV2(p WorkspaceReadSourceCurrentProjectionV2) WorkspaceReadSourceCurrentProjectionV2 {
	p.CanonicalInline = append([]byte(nil), p.CanonicalInline...)
	if p.Operation.ExecutionScope.SandboxLease != nil {
		lease := *p.Operation.ExecutionScope.SandboxLease
		p.Operation.ExecutionScope.SandboxLease = &lease
	}
	if p.RuntimeAttempt.Delegation != nil {
		delegation := *p.RuntimeAttempt.Delegation
		p.RuntimeAttempt.Delegation = &delegation
	}
	return p
}

// WorkspaceReadCommandPublicationSemanticV2 contains only stable semantic
// facts and absolute lifetimes. No outer current Checked/Expires/Digest is
// allowed in this object.
type WorkspaceReadCommandPublicationSemanticV2 struct {
	SourceCommand                 WorkspaceReadSourceCommandRefV2                            `json:"source_command"`
	SourceSemanticDigest          runtimecore.Digest                                         `json:"source_semantic_digest"`
	Operation                     runtimeports.OperationSubjectV3                            `json:"operation"`
	OperationDigest               runtimecore.Digest                                         `json:"operation_digest"`
	Prepared                      runtimeports.PreparedProviderAttemptRefV2                  `json:"prepared"`
	PreparedSemanticDigest        runtimecore.Digest                                         `json:"prepared_semantic_digest"`
	RuntimeAttempt                runtimeports.OperationDispatchAttemptRefV3                 `json:"runtime_attempt"`
	RuntimeAttemptDigest          runtimecore.Digest                                         `json:"runtime_attempt_digest"`
	RuntimeEffectIntent           runtimeports.OperationEffectIntentV3                       `json:"runtime_effect_intent"`
	RuntimeEffectIntentDigest     runtimecore.Digest                                         `json:"runtime_effect_intent_digest"`
	RuntimeEffectFactRevision     runtimecore.Revision                                       `json:"runtime_effect_fact_revision"`
	RuntimePreparedSnapshot       runtimeports.ControlledOperationPreparedSemanticSnapshotV2 `json:"runtime_prepared_snapshot"`
	PayloadSchema                 runtimeports.SchemaRefV2                                   `json:"payload_schema"`
	PayloadDigest                 runtimecore.Digest                                         `json:"payload_digest"`
	PayloadRevision               runtimecore.Revision                                       `json:"payload_revision"`
	CanonicalInline               []byte                                                     `json:"canonical_inline"`
	Workspace                     WorkspaceView                                              `json:"workspace"`
	WorkspaceSemanticDigest       string                                                     `json:"workspace_semantic_digest"`
	RelativePath                  string                                                     `json:"relative_path"`
	StartByte                     uint64                                                     `json:"start_byte"`
	MaxBytes                      uint64                                                     `json:"max_bytes"`
	RequestedNotAfterUnixNano     int64                                                      `json:"requested_not_after_unix_nano"`
	SourceCreatedUnixNano         int64                                                      `json:"source_created_unix_nano"`
	SourceNotAfterUnixNano        int64                                                      `json:"source_not_after_unix_nano"`
	PreparedNotAfterUnixNano      int64                                                      `json:"prepared_not_after_unix_nano"`
	EffectiveCreatedLowerUnixNano int64                                                      `json:"effective_created_lower_unix_nano"`
	SemanticNotAfterUnixNano      int64                                                      `json:"semantic_not_after_unix_nano"`
	Digest                        runtimecore.Digest                                         `json:"digest"`
}

func (s WorkspaceReadCommandPublicationSemanticV2) Validate() error {
	if s.SourceCommand.Validate() != nil ||
		!validCoreLowerDigestV2(s.SourceSemanticDigest) ||
		s.Operation.Validate() != nil ||
		!validCoreLowerDigestV2(s.OperationDigest) ||
		s.Prepared.Validate() != nil ||
		!validCoreLowerDigestV2(s.PreparedSemanticDigest) ||
		s.RuntimeAttempt.Validate() != nil ||
		!validCoreLowerDigestV2(s.RuntimeAttemptDigest) ||
		s.RuntimeEffectIntent.Validate() != nil ||
		!validCoreLowerDigestV2(s.RuntimeEffectIntentDigest) ||
		s.RuntimeEffectFactRevision == 0 ||
		s.RuntimePreparedSnapshot.Validate() != nil ||
		s.PayloadSchema.Validate() != nil ||
		!validCoreLowerDigestV2(s.PayloadDigest) ||
		s.PayloadRevision == 0 ||
		len(s.CanonicalInline) == 0 ||
		len(s.CanonicalInline) > runtimeports.MaxOpaqueInlineBytes ||
		s.Workspace.ValidateShape() != nil ||
		!validRawLowerDigestV2(s.WorkspaceSemanticDigest) ||
		ValidateLogicalPath(s.RelativePath) != nil ||
		s.MaxBytes == 0 ||
		s.MaxBytes > WorkspaceReadMaxBytesV1 ||
		s.RequestedNotAfterUnixNano <= 0 ||
		s.SourceCreatedUnixNano <= 0 ||
		s.SourceNotAfterUnixNano <= s.SourceCreatedUnixNano ||
		s.PreparedNotAfterUnixNano <= 0 ||
		s.EffectiveCreatedLowerUnixNano <= 0 ||
		s.SemanticNotAfterUnixNano <= s.EffectiveCreatedLowerUnixNano ||
		!validCoreLowerDigestV2(s.Digest) {
		return errors.New("workspace read publication semantic closure is incomplete")
	}
	expectedCreated := maxUnixNanoV2(
		s.SourceCreatedUnixNano,
		s.Prepared.PreparedUnixNano,
		s.Workspace.Meta.UpdatedUnixNano,
	)
	expectedExpiry := minUnixNanoV2(
		s.RequestedNotAfterUnixNano,
		s.SourceNotAfterUnixNano,
		s.RuntimeEffectIntent.ExpiresUnixNano,
		s.PreparedNotAfterUnixNano,
		s.Workspace.Meta.ExpiresUnixNano,
		s.Workspace.Lease.ExpiresUnixNano,
	)
	if expectedCreated != s.EffectiveCreatedLowerUnixNano ||
		expectedExpiry != s.SemanticNotAfterUnixNano ||
		s.SourceCommand.Owner.ComponentID != s.Prepared.Provider.ComponentID ||
		s.SourceCommand.Owner.ManifestDigest != s.Prepared.Provider.ManifestDigest ||
		!workspaceReadEffectOwnersContainV2(s.RuntimeEffectIntent.Owners, s.SourceCommand.Owner) ||
		s.OperationDigest != s.Prepared.OperationDigest ||
		s.OperationDigest != s.RuntimeAttempt.OperationDigest ||
		s.PreparedSemanticDigest == "" ||
		s.RuntimeEffectIntentDigest != s.RuntimeAttempt.IntentDigest ||
		s.RuntimeEffectIntent.Provider != s.Prepared.Provider ||
		s.RuntimeEffectIntent.ID != s.RuntimeAttempt.EffectID ||
		s.RuntimeEffectIntent.Revision != s.RuntimeAttempt.IntentRevision ||
		s.RuntimeEffectIntent.Payload.Schema != s.PayloadSchema ||
		s.RuntimeEffectIntent.Payload.ContentDigest != s.PayloadDigest ||
		s.RuntimeEffectIntent.PayloadRevision != s.PayloadRevision ||
		s.RuntimePreparedSnapshot.Prepared != s.Prepared ||
		s.RuntimePreparedSnapshot.SemanticDigest != s.PreparedSemanticDigest ||
		s.Prepared.IntentID != s.RuntimeAttempt.EffectID ||
		s.Prepared.IntentRevision != s.RuntimeAttempt.IntentRevision ||
		s.Prepared.IntentDigest != s.RuntimeAttempt.IntentDigest ||
		s.Prepared.PermitID != s.RuntimeAttempt.PermitID ||
		s.Prepared.PermitRevision != s.RuntimeAttempt.PermitRevision ||
		s.Prepared.PermitDigest != s.RuntimeAttempt.PermitDigest ||
		s.Prepared.AttemptID != s.RuntimeAttempt.AttemptID ||
		s.RuntimeAttempt.Delegation == nil ||
		s.RuntimeAttempt.Delegation.ID != s.Prepared.DeclaredDelegation.ID ||
		s.RuntimeAttempt.Delegation.Revision <= s.Prepared.DeclaredDelegation.Revision ||
		s.RuntimePreparedSnapshot.Delegation != *s.RuntimeAttempt.Delegation ||
		s.Prepared.PayloadSchema != s.PayloadSchema ||
		s.Prepared.PayloadDigest != s.PayloadDigest ||
		s.Prepared.PayloadRevision != s.PayloadRevision ||
		s.SourceCreatedUnixNano < s.Prepared.PreparedUnixNano ||
		s.SourceNotAfterUnixNano > s.RequestedNotAfterUnixNano ||
		s.SourceNotAfterUnixNano > s.RuntimeEffectIntent.ExpiresUnixNano ||
		s.PreparedNotAfterUnixNano != s.Prepared.ExpiresUnixNano {
		return errors.New("workspace read publication stable semantic axes drifted")
	}
	operationDigest, err := s.Operation.DigestV3()
	if err != nil || operationDigest != s.OperationDigest {
		return errors.New("workspace read publication Operation digest drifted")
	}
	intentDigest, err := s.RuntimeEffectIntent.DigestV3()
	if err != nil ||
		intentDigest != s.RuntimeEffectIntentDigest ||
		!runtimeports.SameOperationSubjectV3(s.RuntimeEffectIntent.Operation, s.Operation) ||
		!sameWorkspaceReadRuntimeAttemptV2(s.RuntimePreparedSnapshot.Attempt, s.RuntimeAttempt) {
		return errors.New("workspace read publication Runtime Effect intent digest drifted")
	}
	attemptDigest, err := WorkspaceReadSourceRuntimeAttemptDigestV2(s.RuntimeAttempt)
	if err != nil || attemptDigest != s.RuntimeAttemptDigest {
		return errors.New("workspace read publication Runtime Attempt digest drifted")
	}
	sourceDigest, err := s.SourceSemanticDigestFromPublicationV2()
	if err != nil || sourceDigest != s.SourceSemanticDigest {
		return errors.New("workspace read publication Source semantic digest drifted")
	}
	workspaceDigest, err := Digest("workspace-read-workspace-semantic-v2", cloneWorkspaceViewV2(s.Workspace))
	if err != nil || workspaceDigest != s.WorkspaceSemanticDigest {
		return errors.New("workspace read publication Workspace semantic digest drifted")
	}
	payload, err := decodeWorkspaceReadCanonicalPayloadV2(s.CanonicalInline)
	if err != nil ||
		runtimecore.DigestBytes(s.CanonicalInline) != s.PayloadDigest ||
		payload.RelativePath != s.RelativePath ||
		payload.StartByte != s.StartByte ||
		payload.MaxBytes != s.MaxBytes ||
		payload.RequestedNotAfter != s.RequestedNotAfterUnixNano {
		return errors.New("workspace read publication canonical payload drifted")
	}
	workspaceRef, err := payload.WorkspaceRoot.ContractRefV2()
	if err != nil || !SameRef(workspaceRef, s.Workspace.Meta.Ref()) {
		return errors.New("workspace read publication Workspace coordinate drifted")
	}
	if err := validateWorkspaceReadOperationWorkspaceClosureV2(s.Operation, s.Workspace, s.RelativePath); err != nil {
		return err
	}
	digest, err := s.ComputeDigestV2()
	if err != nil || digest != s.Digest {
		return errors.New("workspace read publication semantic digest drifted")
	}
	return nil
}

func (s WorkspaceReadCommandPublicationSemanticV2) SourceSemanticDigestFromPublicationV2() (runtimecore.Digest, error) {
	source := WorkspaceReadSourceCurrentProjectionV2{
		ContractVersion:           WorkspaceReadCommandPublicationContractVersionV2,
		TypeURL:                   WorkspaceReadSourceCurrentTypeURLV2,
		SourceCommand:             s.SourceCommand,
		Operation:                 s.Operation,
		OperationDigest:           s.OperationDigest,
		Prepared:                  s.Prepared,
		PreparedSemanticDigest:    s.PreparedSemanticDigest,
		RuntimeAttempt:            s.RuntimeAttempt,
		RuntimeAttemptDigest:      s.RuntimeAttemptDigest,
		RuntimeEffectIntentDigest: s.RuntimeEffectIntentDigest,
		RuntimeEffectFactRevision: s.RuntimeEffectFactRevision,
		RuntimeEffectState:        WorkspaceReadEffectDispatchIntentV2,
		PayloadSchema:             s.PayloadSchema,
		PayloadDigest:             s.PayloadDigest,
		PayloadRevision:           s.PayloadRevision,
		CanonicalInline:           append([]byte(nil), s.CanonicalInline...),
		CanonicalInlineLength:     uint64(len(s.CanonicalInline)),
		WorkspaceView:             s.Workspace.Meta.Ref(),
		RelativePath:              s.RelativePath,
		StartByte:                 s.StartByte,
		MaxBytes:                  s.MaxBytes,
		RequestedNotAfterUnixNano: s.RequestedNotAfterUnixNano,
		SourceCreatedUnixNano:     s.SourceCreatedUnixNano,
		SourceNotAfterUnixNano:    s.SourceNotAfterUnixNano,
	}
	return source.StableSemanticDigestV2()
}

func (s WorkspaceReadCommandPublicationSemanticV2) ComputeDigestV2() (runtimecore.Digest, error) {
	s = CloneWorkspaceReadCommandPublicationSemanticV2(s)
	s.Digest = ""
	return runtimecore.CanonicalJSONDigest(
		workspaceReadCommandPublicationCanonicalDomainV2,
		WorkspaceReadCommandPublicationContractVersionV2,
		"WorkspaceReadCommandPublicationSemanticV2",
		s,
	)
}

func SealWorkspaceReadCommandPublicationSemanticV2(
	source WorkspaceReadSourceCurrentProjectionV2,
	effect runtimeports.ControlledOperationEffectCurrentProjectionV2,
	prepared runtimeports.ControlledOperationPreparedCurrentProjectionV2,
	workspace WorkspaceView,
	s2 time.Time,
) (WorkspaceReadCommandPublicationSemanticV2, error) {
	if s2.IsZero() ||
		source.ValidateCurrent(source.SourceCommand, s2) != nil ||
		effect.Validate(s2) != nil ||
		prepared.Validate() != nil ||
		prepared.CheckedUnixNano > s2.UnixNano() ||
		s2.UnixNano() >= prepared.ExpiresUnixNano ||
		workspace.ValidateCurrent(s2) != nil {
		return WorkspaceReadCommandPublicationSemanticV2{}, errors.New("workspace read publication inputs are not current at S2")
	}
	if err := validateWorkspaceReadCreateCurrentClosureV2(source, effect, prepared, workspace); err != nil {
		return WorkspaceReadCommandPublicationSemanticV2{}, err
	}
	sourceSemanticDigest, err := source.StableSemanticDigestV2()
	if err != nil {
		return WorkspaceReadCommandPublicationSemanticV2{}, err
	}
	workspaceDigest, err := Digest("workspace-read-workspace-semantic-v2", workspace)
	if err != nil {
		return WorkspaceReadCommandPublicationSemanticV2{}, err
	}
	semantic := WorkspaceReadCommandPublicationSemanticV2{
		SourceCommand:             source.SourceCommand,
		SourceSemanticDigest:      sourceSemanticDigest,
		Operation:                 source.Operation,
		OperationDigest:           source.OperationDigest,
		Prepared:                  source.Prepared,
		PreparedSemanticDigest:    source.PreparedSemanticDigest,
		RuntimeAttempt:            source.RuntimeAttempt,
		RuntimeAttemptDigest:      source.RuntimeAttemptDigest,
		RuntimeEffectIntent:       cloneWorkspaceReadEffectIntentV2(effect.Intent),
		RuntimeEffectIntentDigest: source.RuntimeEffectIntentDigest,
		RuntimeEffectFactRevision: source.RuntimeEffectFactRevision,
		RuntimePreparedSnapshot:   cloneWorkspaceReadPreparedSnapshotV2(prepared.Snapshot),
		PayloadSchema:             source.PayloadSchema,
		PayloadDigest:             source.PayloadDigest,
		PayloadRevision:           source.PayloadRevision,
		CanonicalInline:           append([]byte(nil), source.CanonicalInline...),
		Workspace:                 cloneWorkspaceViewV2(workspace),
		WorkspaceSemanticDigest:   workspaceDigest,
		RelativePath:              source.RelativePath,
		StartByte:                 source.StartByte,
		MaxBytes:                  source.MaxBytes,
		RequestedNotAfterUnixNano: source.RequestedNotAfterUnixNano,
		SourceCreatedUnixNano:     source.SourceCreatedUnixNano,
		SourceNotAfterUnixNano:    source.SourceNotAfterUnixNano,
		PreparedNotAfterUnixNano:  source.Prepared.ExpiresUnixNano,
		EffectiveCreatedLowerUnixNano: maxUnixNanoV2(
			source.SourceCreatedUnixNano,
			source.Prepared.PreparedUnixNano,
			workspace.Meta.UpdatedUnixNano,
		),
	}
	semantic.SemanticNotAfterUnixNano = minUnixNanoV2(
		semantic.RequestedNotAfterUnixNano,
		semantic.SourceNotAfterUnixNano,
		semantic.RuntimeEffectIntent.ExpiresUnixNano,
		semantic.PreparedNotAfterUnixNano,
		semantic.Workspace.Meta.ExpiresUnixNano,
		semantic.Workspace.Lease.ExpiresUnixNano,
	)
	digest, err := semantic.ComputeDigestV2()
	if err != nil {
		return WorkspaceReadCommandPublicationSemanticV2{}, err
	}
	semantic.Digest = digest
	return semantic, semantic.Validate()
}

func validateWorkspaceReadCreateCurrentClosureV2(
	source WorkspaceReadSourceCurrentProjectionV2,
	effect runtimeports.ControlledOperationEffectCurrentProjectionV2,
	prepared runtimeports.ControlledOperationPreparedCurrentProjectionV2,
	workspace WorkspaceView,
) error {
	if !runtimeports.SameOperationSubjectV3(source.Operation, effect.Intent.Operation) ||
		effect.IntentDigest != source.RuntimeEffectIntentDigest ||
		effect.FactRevision != source.RuntimeEffectFactRevision ||
		effect.State != WorkspaceReadEffectDispatchIntentV2 ||
		effect.Intent.ID != source.RuntimeAttempt.EffectID ||
		effect.Intent.Revision != source.RuntimeAttempt.IntentRevision ||
		effect.Intent.Provider != source.Prepared.Provider ||
		source.SourceCommand.Owner.ComponentID != effect.Intent.Provider.ComponentID ||
		source.SourceCommand.Owner.ManifestDigest != effect.Intent.Provider.ManifestDigest ||
		!workspaceReadEffectOwnersContainV2(effect.Intent.Owners, source.SourceCommand.Owner) ||
		effect.Intent.Payload.Schema != source.PayloadSchema ||
		effect.Intent.Payload.ContentDigest != source.PayloadDigest ||
		effect.Intent.PayloadRevision != source.PayloadRevision ||
		source.SourceNotAfterUnixNano > effect.Intent.ExpiresUnixNano ||
		prepared.Snapshot.Prepared != source.Prepared ||
		prepared.Snapshot.SemanticDigest != source.PreparedSemanticDigest ||
		!sameWorkspaceReadRuntimeAttemptV2(prepared.Snapshot.Attempt, source.RuntimeAttempt) ||
		source.RuntimeAttempt.Delegation == nil ||
		prepared.Snapshot.Delegation != *source.RuntimeAttempt.Delegation ||
		prepared.Snapshot.OperationDigest != source.OperationDigest ||
		prepared.Snapshot.EffectID != source.RuntimeAttempt.EffectID ||
		prepared.Snapshot.IntentRevision != source.RuntimeAttempt.IntentRevision ||
		prepared.Snapshot.IntentDigest != source.RuntimeAttempt.IntentDigest ||
		prepared.Snapshot.ProviderBinding != source.Prepared.Provider ||
		prepared.Snapshot.PayloadSchema != source.PayloadSchema ||
		prepared.Snapshot.PayloadDigest != source.PayloadDigest ||
		prepared.Snapshot.PayloadRevision != source.PayloadRevision ||
		!SameRef(workspace.Meta.Ref(), source.WorkspaceView) {
		return errors.New("workspace read publication S2 exact-current closure drifted")
	}
	return validateWorkspaceReadOperationWorkspaceClosureV2(source.Operation, workspace, source.RelativePath)
}

func validateWorkspaceReadOperationWorkspaceClosureV2(
	operation runtimeports.OperationSubjectV3,
	workspace WorkspaceView,
	relativePath string,
) error {
	scope := operation.ExecutionScope
	lease := workspace.Lease
	if scope.SandboxLease == nil ||
		string(scope.Identity.TenantID) != lease.TenantID ||
		string(scope.Instance.ID) != lease.InstanceID ||
		uint64(scope.Instance.Epoch) != lease.InstanceEpoch ||
		string(scope.SandboxLease.ID) != lease.LeaseID ||
		uint64(scope.SandboxLease.Epoch) != lease.LeaseEpoch ||
		uint64(scope.AuthorityEpoch) != lease.FenceEpoch ||
		rawCoreDigestV2(operation.ExecutionScopeDigest) != lease.ScopeDigest ||
		!workspaceReadPathWithinScopesV2(relativePath, workspace.ReadScopes) ||
		workspaceReadPathWithinScopesV2(relativePath, workspace.HiddenScopes) {
		return errors.New("workspace read Operation, Lease, Fence, scope or path authorization drifted")
	}
	return nil
}

func workspaceReadPathWithinScopesV2(relativePath string, scopes []string) bool {
	for _, scope := range scopes {
		if relativePath == scope || strings.HasPrefix(relativePath, scope+"/") {
			return true
		}
	}
	return false
}

// WorkspaceReadCommandPublicationV2 is immutable and contains only the stable
// semantic closure. Fresh outer-current proof is stored in OwnerCurrentV2.
type WorkspaceReadCommandPublicationV2 struct {
	ContractVersion string                                    `json:"contract_version"`
	TypeURL         string                                    `json:"type_url"`
	Meta            Meta                                      `json:"meta"`
	Command         Ref                                       `json:"command"`
	Semantic        WorkspaceReadCommandPublicationSemanticV2 `json:"semantic"`
}

// SealWorkspaceReadPublishedCommandV2 is the only V2 canonical Command
// factory. It derives the ID and rejects fields not present in the canonical
// workspace.read input, including ExpectedFileRef.
func SealWorkspaceReadPublishedCommandV2(
	semantic WorkspaceReadCommandPublicationSemanticV2,
	commitNow time.Time,
) (WorkspaceReadCommandV1, error) {
	if err := semantic.Validate(); err != nil {
		return WorkspaceReadCommandV1{}, err
	}
	if commitNow.IsZero() ||
		commitNow.UnixNano() < semantic.EffectiveCreatedLowerUnixNano ||
		!commitNow.Before(time.Unix(0, semantic.SemanticNotAfterUnixNano)) {
		return WorkspaceReadCommandV1{}, errors.New("workspace read Command owner commit clock is invalid")
	}
	sourceRef, err := semantic.SourceCommand.ContractRefV2()
	if err != nil {
		return WorkspaceReadCommandV1{}, err
	}
	dispatchDigest, err := runtimecore.CanonicalJSONDigest(
		"praxis.sandbox.workspace-read",
		"1.0.0",
		"OperationDispatchAttemptRefV3",
		semantic.RuntimeAttempt,
	)
	if err != nil {
		return WorkspaceReadCommandV1{}, err
	}
	id, err := DeriveWorkspaceReadCommandIDV2(semantic.SourceCommand)
	if err != nil {
		return WorkspaceReadCommandV1{}, err
	}
	command := WorkspaceReadCommandV1{
		TenantID:                  string(semantic.Operation.ExecutionScope.Identity.TenantID),
		SourceToolCommand:         sourceRef,
		SourceToolPayloadSchema:   semantic.PayloadSchema.Key(),
		SourceToolPayloadDigest:   string(semantic.PayloadDigest),
		SourceToolPayloadRevision: uint64(semantic.PayloadRevision),
		WorkspaceView:             semantic.Workspace.Meta.Ref(),
		FileScopeDigest:           semantic.Workspace.FileScopeDigest,
		RelativePath:              semantic.RelativePath,
		StartByte:                 semantic.StartByte,
		MaxBytes:                  semantic.MaxBytes,
		ExpectedFileRef:           nil,
		RequestedNotAfterUnixNano: semantic.RequestedNotAfterUnixNano,
		OperationDigest:           string(semantic.OperationDigest),
		EffectID:                  string(semantic.RuntimeAttempt.EffectID),
		IntentRevision:            uint64(semantic.RuntimeAttempt.IntentRevision),
		IntentDigest:              string(semantic.RuntimeAttempt.IntentDigest),
		AttemptID:                 semantic.RuntimeAttempt.AttemptID,
		PreparedDigest:            string(semantic.Prepared.Digest),
		DispatchDigest:            string(dispatchDigest),
		ProviderComponent:         string(semantic.Prepared.Provider.ComponentID),
		ProviderManifest:          string(semantic.Prepared.Provider.ManifestDigest),
	}
	sealed, err := SealWorkspaceReadCommandV1(
		command,
		id,
		commitNow,
		time.Unix(0, semantic.SemanticNotAfterUnixNano),
	)
	if err != nil {
		return WorkspaceReadCommandV1{}, err
	}
	if sealed.ExpectedFileRef != nil {
		return WorkspaceReadCommandV1{}, errors.New("workspace read published Command cannot carry an expected File Ref")
	}
	return sealed, validateWorkspaceReadPublicationCommandClosureV2(sealed, semantic)
}

func (p WorkspaceReadCommandPublicationV2) ValidateShape() error {
	if p.ContractVersion != WorkspaceReadCommandPublicationContractVersionV2 ||
		p.TypeURL != WorkspaceReadCommandPublicationTypeURLV2 ||
		p.Meta.ValidateShape() != nil ||
		p.Command.ValidateShape("workspace read command") != nil ||
		p.Semantic.Validate() != nil ||
		p.Meta.CreatedUnixNano < p.Semantic.EffectiveCreatedLowerUnixNano ||
		p.Meta.UpdatedUnixNano != p.Meta.CreatedUnixNano ||
		p.Meta.ExpiresUnixNano != p.Semantic.SemanticNotAfterUnixNano ||
		p.Meta.CreatedUnixNano >= p.Meta.ExpiresUnixNano {
		return errors.New("workspace read command publication is incomplete")
	}
	expectedCommandID, err := DeriveWorkspaceReadCommandIDV2(p.Semantic.SourceCommand)
	if err != nil ||
		p.Command.ID != expectedCommandID ||
		p.Command.Revision != 1 {
		return errors.New("workspace read publication Command identity drifted")
	}
	expectedPublicationID, err := DeriveWorkspaceReadCommandPublicationIDV2(p.Semantic.SourceCommand)
	if err != nil ||
		p.Meta.ID != expectedPublicationID ||
		p.Meta.Revision != 1 {
		return errors.New("workspace read publication identity drifted")
	}
	copy := CloneWorkspaceReadCommandPublicationV2(p)
	copy.Meta = Meta{ExpiresUnixNano: p.Meta.ExpiresUnixNano}
	digest, err := Digest("workspace-read-command-publication-v2", copy)
	if err != nil || digest != p.Meta.Digest {
		return errors.New("workspace read command publication digest drifted")
	}
	return nil
}

func SealWorkspaceReadCommandPublicationV2(
	p WorkspaceReadCommandPublicationV2,
	command WorkspaceReadCommandV1,
	commitNow time.Time,
) (WorkspaceReadCommandPublicationV2, error) {
	p = CloneWorkspaceReadCommandPublicationV2(p)
	if err := p.Semantic.Validate(); err != nil {
		return WorkspaceReadCommandPublicationV2{}, err
	}
	if err := validateWorkspaceReadPublicationCommandClosureV2(command, p.Semantic); err != nil {
		return WorkspaceReadCommandPublicationV2{}, err
	}
	if commitNow.IsZero() ||
		commitNow.UnixNano() < p.Semantic.EffectiveCreatedLowerUnixNano ||
		!commitNow.Before(time.Unix(0, p.Semantic.SemanticNotAfterUnixNano)) {
		return WorkspaceReadCommandPublicationV2{}, errors.New("workspace read publication owner commit clock is invalid")
	}
	if command.Meta.CreatedUnixNano != commitNow.UnixNano() ||
		command.Meta.UpdatedUnixNano != command.Meta.CreatedUnixNano {
		return WorkspaceReadCommandPublicationV2{}, errors.New("workspace read Command and publication must share one initial owner commit clock")
	}
	p.ContractVersion = WorkspaceReadCommandPublicationContractVersionV2
	p.TypeURL = WorkspaceReadCommandPublicationTypeURLV2
	p.Command = command.Meta.Ref()
	id, err := DeriveWorkspaceReadCommandPublicationIDV2(p.Semantic.SourceCommand)
	if err != nil {
		return WorkspaceReadCommandPublicationV2{}, err
	}
	expires := time.Unix(0, p.Semantic.SemanticNotAfterUnixNano)
	p.Meta = Meta{ExpiresUnixNano: expires.UnixNano()}
	copy := CloneWorkspaceReadCommandPublicationV2(p)
	copy.Meta = Meta{ExpiresUnixNano: expires.UnixNano()}
	meta, err := NewMeta(id, 1, commitNow, expires, "workspace-read-command-publication-v2", copy)
	if err != nil {
		return WorkspaceReadCommandPublicationV2{}, err
	}
	p.Meta = meta
	return p, p.ValidateShape()
}

func ValidateWorkspaceReadPublicationCommandV2(command WorkspaceReadCommandV1, publication WorkspaceReadCommandPublicationV2) error {
	if err := publication.ValidateShape(); err != nil {
		return err
	}
	if !SameRef(command.Meta.Ref(), publication.Command) {
		return errors.New("workspace read publication references another Command")
	}
	if command.Meta.CreatedUnixNano != publication.Meta.CreatedUnixNano ||
		command.Meta.UpdatedUnixNano != publication.Meta.UpdatedUnixNano ||
		command.Meta.UpdatedUnixNano != command.Meta.CreatedUnixNano {
		return errors.New("workspace read Command and publication owner commit clocks drifted")
	}
	return validateWorkspaceReadPublicationCommandClosureV2(command, publication.Semantic)
}

func validateWorkspaceReadPublicationCommandClosureV2(command WorkspaceReadCommandV1, semantic WorkspaceReadCommandPublicationSemanticV2) error {
	if command.ValidateShape() != nil ||
		command.ExpectedFileRef != nil ||
		command.Meta.CreatedUnixNano < semantic.EffectiveCreatedLowerUnixNano ||
		command.Meta.ExpiresUnixNano != semantic.SemanticNotAfterUnixNano {
		return errors.New("workspace read Command owner time closure drifted")
	}
	sourceRef, err := semantic.SourceCommand.ContractRefV2()
	if err != nil ||
		!SameRef(sourceRef, command.SourceToolCommand) ||
		!SameRef(semantic.Workspace.Meta.Ref(), command.WorkspaceView) ||
		command.TenantID != string(semantic.Operation.ExecutionScope.Identity.TenantID) ||
		command.FileScopeDigest != semantic.Workspace.FileScopeDigest ||
		command.RelativePath != semantic.RelativePath ||
		command.StartByte != semantic.StartByte ||
		command.MaxBytes != semantic.MaxBytes ||
		command.RequestedNotAfterUnixNano != semantic.RequestedNotAfterUnixNano ||
		command.OperationDigest != string(semantic.OperationDigest) ||
		command.EffectID != string(semantic.RuntimeAttempt.EffectID) ||
		command.IntentRevision != uint64(semantic.RuntimeAttempt.IntentRevision) ||
		command.IntentDigest != string(semantic.RuntimeAttempt.IntentDigest) ||
		command.AttemptID != semantic.RuntimeAttempt.AttemptID ||
		command.PreparedDigest != string(semantic.Prepared.Digest) ||
		command.ProviderComponent != string(semantic.Prepared.Provider.ComponentID) ||
		command.ProviderManifest != string(semantic.Prepared.Provider.ManifestDigest) ||
		command.SourceToolPayloadSchema != semantic.PayloadSchema.Key() ||
		command.SourceToolPayloadDigest != string(semantic.PayloadDigest) ||
		command.SourceToolPayloadRevision != uint64(semantic.PayloadRevision) {
		return errors.New("workspace read Command differs from publication semantic closure")
	}
	dispatchDigest, err := runtimecore.CanonicalJSONDigest(
		"praxis.sandbox.workspace-read",
		"1.0.0",
		"OperationDispatchAttemptRefV3",
		semantic.RuntimeAttempt,
	)
	if err != nil || command.DispatchDigest != string(dispatchDigest) {
		return errors.New("workspace read Command dispatch digest drifted")
	}
	return nil
}

func CloneWorkspaceReadCommandPublicationSemanticV2(s WorkspaceReadCommandPublicationSemanticV2) WorkspaceReadCommandPublicationSemanticV2 {
	s.CanonicalInline = append([]byte(nil), s.CanonicalInline...)
	if s.Operation.ExecutionScope.SandboxLease != nil {
		lease := *s.Operation.ExecutionScope.SandboxLease
		s.Operation.ExecutionScope.SandboxLease = &lease
	}
	if s.RuntimeAttempt.Delegation != nil {
		delegation := *s.RuntimeAttempt.Delegation
		s.RuntimeAttempt.Delegation = &delegation
	}
	s.RuntimeEffectIntent = cloneWorkspaceReadEffectIntentV2(s.RuntimeEffectIntent)
	s.RuntimePreparedSnapshot = cloneWorkspaceReadPreparedSnapshotV2(s.RuntimePreparedSnapshot)
	s.Workspace = cloneWorkspaceViewV2(s.Workspace)
	return s
}

func CloneWorkspaceReadCommandPublicationV2(p WorkspaceReadCommandPublicationV2) WorkspaceReadCommandPublicationV2 {
	p.Semantic = CloneWorkspaceReadCommandPublicationSemanticV2(p.Semantic)
	return p
}

// WorkspaceReadCommandOwnerCurrentV2 is append-only current history. Every
// transient current proof is carried here and bounded by the immutable
// Command/Publication semantic lifetime.
type WorkspaceReadCommandOwnerCurrentV2 struct {
	ContractVersion                 string                          `json:"contract_version"`
	TypeURL                         string                          `json:"type_url"`
	Meta                            Meta                            `json:"meta"`
	Command                         Ref                             `json:"command"`
	Publication                     Ref                             `json:"publication"`
	PublicationSemanticDigest       runtimecore.Digest              `json:"publication_semantic_digest"`
	SourceCommand                   WorkspaceReadSourceCommandRefV2 `json:"source_command"`
	SourceSemanticDigest            runtimecore.Digest              `json:"source_semantic_digest"`
	SourceProjectionDigest          runtimecore.Digest              `json:"source_projection_digest"`
	SourceCheckedUnixNano           int64                           `json:"source_checked_unix_nano"`
	SourceExpiresUnixNano           int64                           `json:"source_expires_unix_nano"`
	RuntimeEffectProjectionDigest   runtimecore.Digest              `json:"runtime_effect_projection_digest"`
	RuntimeEffectCheckedUnixNano    int64                           `json:"runtime_effect_checked_unix_nano"`
	RuntimeEffectExpiresUnixNano    int64                           `json:"runtime_effect_expires_unix_nano"`
	RuntimePreparedProjectionDigest runtimecore.Digest              `json:"runtime_prepared_projection_digest"`
	RuntimePreparedCheckedUnixNano  int64                           `json:"runtime_prepared_checked_unix_nano"`
	RuntimePreparedExpiresUnixNano  int64                           `json:"runtime_prepared_expires_unix_nano"`
	WorkspaceView                   Ref                             `json:"workspace_view"`
	WorkspaceSemanticDigest         string                          `json:"workspace_semantic_digest"`
	WorkspaceCheckedUnixNano        int64                           `json:"workspace_checked_unix_nano"`
	WorkspaceExpiresUnixNano        int64                           `json:"workspace_expires_unix_nano"`
	WorkspaceLeaseExpiresUnixNano   int64                           `json:"workspace_lease_expires_unix_nano"`
	SemanticNotAfterUnixNano        int64                           `json:"semantic_not_after_unix_nano"`
	CheckedUnixNano                 int64                           `json:"checked_unix_nano"`
	ExpiresUnixNano                 int64                           `json:"expires_unix_nano"`
	ProjectionDigest                string                          `json:"projection_digest"`
}

func (p WorkspaceReadCommandOwnerCurrentV2) ValidateShape() error {
	if p.ContractVersion != WorkspaceReadCommandPublicationContractVersionV2 ||
		p.TypeURL != WorkspaceReadCommandOwnerCurrentTypeURLV2 ||
		p.Meta.ValidateShape() != nil ||
		p.Command.ValidateShape("workspace read command") != nil ||
		p.Publication.ValidateShape("workspace read publication") != nil ||
		!validCoreLowerDigestV2(p.PublicationSemanticDigest) ||
		p.SourceCommand.Validate() != nil ||
		!validCoreLowerDigestV2(p.SourceSemanticDigest) ||
		!validCoreLowerDigestV2(p.SourceProjectionDigest) ||
		p.SourceCheckedUnixNano <= 0 ||
		p.SourceExpiresUnixNano <= p.SourceCheckedUnixNano ||
		!validCoreLowerDigestV2(p.RuntimeEffectProjectionDigest) ||
		p.RuntimeEffectCheckedUnixNano <= 0 ||
		p.RuntimeEffectExpiresUnixNano <= p.RuntimeEffectCheckedUnixNano ||
		!validCoreLowerDigestV2(p.RuntimePreparedProjectionDigest) ||
		p.RuntimePreparedCheckedUnixNano <= 0 ||
		p.RuntimePreparedExpiresUnixNano <= p.RuntimePreparedCheckedUnixNano ||
		p.WorkspaceView.ValidateShape("workspace view") != nil ||
		!validRawLowerDigestV2(p.WorkspaceSemanticDigest) ||
		p.WorkspaceCheckedUnixNano <= 0 ||
		p.WorkspaceExpiresUnixNano <= p.WorkspaceCheckedUnixNano ||
		p.WorkspaceLeaseExpiresUnixNano <= p.WorkspaceCheckedUnixNano ||
		p.SemanticNotAfterUnixNano <= 0 ||
		p.CheckedUnixNano != p.Meta.UpdatedUnixNano ||
		p.ExpiresUnixNano != p.Meta.ExpiresUnixNano ||
		p.ExpiresUnixNano <= p.CheckedUnixNano ||
		p.CheckedUnixNano < p.SourceCheckedUnixNano ||
		p.CheckedUnixNano < p.RuntimeEffectCheckedUnixNano ||
		p.CheckedUnixNano < p.RuntimePreparedCheckedUnixNano ||
		p.CheckedUnixNano < p.WorkspaceCheckedUnixNano ||
		time.Duration(p.ExpiresUnixNano-p.CheckedUnixNano) > WorkspaceReadCommandOwnerCurrentMaxTTLV2 ||
		p.ExpiresUnixNano > p.SemanticNotAfterUnixNano ||
		p.ExpiresUnixNano > p.SourceExpiresUnixNano ||
		p.ExpiresUnixNano > p.RuntimeEffectExpiresUnixNano ||
		p.ExpiresUnixNano > p.RuntimePreparedExpiresUnixNano ||
		p.ExpiresUnixNano > p.WorkspaceExpiresUnixNano ||
		p.ExpiresUnixNano > p.WorkspaceLeaseExpiresUnixNano ||
		!validRawLowerDigestV2(p.ProjectionDigest) {
		return errors.New("workspace read command Owner current is incomplete")
	}
	expectedExpires := workspaceReadCommandOwnerCurrentExpiryV2(p, time.Unix(0, p.CheckedUnixNano))
	if p.ExpiresUnixNano != expectedExpires ||
		(p.Meta.Revision == 1 && p.Meta.CreatedUnixNano != p.CheckedUnixNano) {
		return errors.New("workspace read command Owner current revision or derived expiry drifted")
	}
	expectedCommandID, err := DeriveWorkspaceReadCommandIDV2(p.SourceCommand)
	if err != nil ||
		p.Command.ID != expectedCommandID ||
		p.Command.Revision != 1 {
		return errors.New("workspace read Owner current Command identity drifted")
	}
	expectedPublicationID, err := DeriveWorkspaceReadCommandPublicationIDV2(p.SourceCommand)
	if err != nil ||
		p.Publication.ID != expectedPublicationID ||
		p.Publication.Revision != 1 {
		return errors.New("workspace read Owner current Publication identity drifted")
	}
	expectedCurrentID, err := DeriveWorkspaceReadCommandOwnerCurrentIDV2(p.Command)
	if err != nil || p.Meta.ID != expectedCurrentID {
		return errors.New("workspace read Owner current identity drifted")
	}
	projectionCopy := p
	projectionCopy.ProjectionDigest = ""
	projectionDigest, err := Digest("workspace-read-command-owner-current-projection-v2", projectionCopy)
	if err != nil || projectionDigest != p.ProjectionDigest {
		return errors.New("workspace read command Owner current projection digest drifted")
	}
	metaCopy := p
	metaCopy.Meta = Meta{ExpiresUnixNano: p.Meta.ExpiresUnixNano}
	metaCopy.ProjectionDigest = ""
	metaDigest, err := Digest("workspace-read-command-owner-current-v2", metaCopy)
	if err != nil || metaDigest != p.Meta.Digest {
		return errors.New("workspace read command Owner current fact digest drifted")
	}
	return nil
}

func (p WorkspaceReadCommandOwnerCurrentV2) ValidateCurrent(now time.Time) error {
	if err := p.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return errors.New("workspace read command Owner current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return errors.New("workspace read command Owner current expired")
	}
	return nil
}

// ValidateWorkspaceReadCommandOwnerClosureV2 is the mandatory three-fact join.
// OwnerCurrent.ValidateShape only proves a self-consistent envelope; it cannot
// prove that the referenced immutable Command and Publication are the same
// owner facts.
func ValidateWorkspaceReadCommandOwnerClosureV2(
	command WorkspaceReadCommandV1,
	publication WorkspaceReadCommandPublicationV2,
	current WorkspaceReadCommandOwnerCurrentV2,
) error {
	if err := ValidateWorkspaceReadPublicationCommandV2(command, publication); err != nil {
		return err
	}
	if err := current.ValidateShape(); err != nil {
		return err
	}
	expectedCommandID, err := DeriveWorkspaceReadCommandIDV2(publication.Semantic.SourceCommand)
	if err != nil {
		return err
	}
	expectedPublicationID, err := DeriveWorkspaceReadCommandPublicationIDV2(publication.Semantic.SourceCommand)
	if err != nil {
		return err
	}
	expectedCurrentID, err := DeriveWorkspaceReadCommandOwnerCurrentIDV2(command.Meta.Ref())
	if err != nil {
		return err
	}
	if !SameRef(publication.Command, command.Meta.Ref()) ||
		!SameRef(current.Command, command.Meta.Ref()) ||
		!SameRef(current.Publication, publication.Meta.Ref()) ||
		current.PublicationSemanticDigest != publication.Semantic.Digest ||
		current.SourceCommand != publication.Semantic.SourceCommand ||
		current.SourceSemanticDigest != publication.Semantic.SourceSemanticDigest ||
		!SameRef(current.WorkspaceView, publication.Semantic.Workspace.Meta.Ref()) ||
		current.WorkspaceSemanticDigest != publication.Semantic.WorkspaceSemanticDigest ||
		current.SemanticNotAfterUnixNano != publication.Semantic.SemanticNotAfterUnixNano ||
		command.Meta.ID != expectedCommandID ||
		command.Meta.Revision != 1 ||
		publication.Meta.ID != expectedPublicationID ||
		publication.Meta.Revision != 1 ||
		current.Meta.ID != expectedCurrentID ||
		current.Meta.Revision == 0 ||
		current.Meta.CreatedUnixNano != command.Meta.CreatedUnixNano ||
		current.Meta.CreatedUnixNano != publication.Meta.CreatedUnixNano {
		return errors.New("workspace read Command, Publication, and Owner current closure drifted")
	}
	return nil
}

// SealInitialWorkspaceReadCommandOwnerCurrentV2 creates revision 1. The
// initial current fact shares the single owner commit clock with the immutable
// Command and Publication and derives its expiry from every current upper
// bound; callers cannot choose an expiry.
func SealInitialWorkspaceReadCommandOwnerCurrentV2(
	p WorkspaceReadCommandOwnerCurrentV2,
	checked time.Time,
) (WorkspaceReadCommandOwnerCurrentV2, error) {
	if checked.IsZero() {
		return WorkspaceReadCommandOwnerCurrentV2{}, errors.New("workspace read command initial Owner current clock is invalid")
	}
	return sealWorkspaceReadCommandOwnerCurrentV2(p, 1, checked, checked)
}

// SealNextWorkspaceReadCommandOwnerCurrentV2 creates exactly one successor.
// The predecessor supplies the exact append-only shape/CAS coordinate; it need
// not still be current. Fresh upstream owner reads, not the old current fact,
// authorize the successor. The stable owner closure and original Created
// timestamp are preserved.
func SealNextWorkspaceReadCommandOwnerCurrentV2(
	p WorkspaceReadCommandOwnerCurrentV2,
	expected WorkspaceReadCommandOwnerCurrentV2,
	checked time.Time,
) (WorkspaceReadCommandOwnerCurrentV2, error) {
	if err := expected.ValidateShape(); err != nil {
		return WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	if checked.IsZero() || checked.UnixNano() < expected.CheckedUnixNano {
		return WorkspaceReadCommandOwnerCurrentV2{}, errors.New("workspace read command Owner current successor clock regressed")
	}
	if p.Command != expected.Command ||
		p.Publication != expected.Publication ||
		p.PublicationSemanticDigest != expected.PublicationSemanticDigest ||
		p.SourceCommand != expected.SourceCommand ||
		p.SourceSemanticDigest != expected.SourceSemanticDigest ||
		p.WorkspaceView != expected.WorkspaceView ||
		p.WorkspaceSemanticDigest != expected.WorkspaceSemanticDigest ||
		p.SemanticNotAfterUnixNano != expected.SemanticNotAfterUnixNano {
		return WorkspaceReadCommandOwnerCurrentV2{}, errors.New("workspace read command Owner current stable closure drifted")
	}
	if p.SourceCheckedUnixNano < expected.SourceCheckedUnixNano ||
		p.RuntimeEffectCheckedUnixNano < expected.RuntimeEffectCheckedUnixNano ||
		p.RuntimePreparedCheckedUnixNano < expected.RuntimePreparedCheckedUnixNano ||
		p.WorkspaceCheckedUnixNano < expected.WorkspaceCheckedUnixNano {
		return WorkspaceReadCommandOwnerCurrentV2{}, errors.New("workspace read command Owner current input watermark regressed")
	}
	return sealWorkspaceReadCommandOwnerCurrentV2(
		p,
		expected.Meta.Revision+1,
		time.Unix(0, expected.Meta.CreatedUnixNano),
		checked,
	)
}

// ValidateWorkspaceReadCommandOwnerFreshClosureV2 is the qualification join
// performed after the stored Command/Publication/OwnerCurrent three-fact join.
// It re-reads every upstream owner at use time; the stored current envelope
// cannot authorize execution by itself.
func ValidateWorkspaceReadCommandOwnerFreshClosureV2(
	current WorkspaceReadCommandOwnerCurrentV2,
	source WorkspaceReadSourceCurrentProjectionV2,
	effect runtimeports.ControlledOperationEffectCurrentProjectionV2,
	prepared runtimeports.ControlledOperationPreparedCurrentProjectionV2,
	workspace WorkspaceView,
	now time.Time,
) error {
	if err := current.ValidateCurrent(now); err != nil {
		return err
	}
	if err := source.ValidateCurrent(current.SourceCommand, now); err != nil {
		return err
	}
	if err := effect.Validate(now); err != nil {
		return err
	}
	if err := prepared.Validate(); err != nil ||
		prepared.CheckedUnixNano > now.UnixNano() ||
		now.UnixNano() >= prepared.ExpiresUnixNano {
		return errors.New("workspace read command fresh Prepared current is invalid")
	}
	if err := workspace.ValidateCurrent(now); err != nil {
		return err
	}
	workspaceDigest, err := Digest("workspace-read-workspace-semantic-v2", cloneWorkspaceViewV2(workspace))
	if err != nil {
		return err
	}
	sourceSemanticDigest, err := source.StableSemanticDigestV2()
	if err != nil {
		return err
	}
	if current.SourceProjectionDigest != source.ProjectionDigest ||
		current.SourceSemanticDigest != sourceSemanticDigest ||
		current.SourceCheckedUnixNano != source.CheckedUnixNano ||
		current.SourceExpiresUnixNano != source.ExpiresUnixNano ||
		current.RuntimeEffectProjectionDigest != effect.Digest ||
		current.RuntimeEffectCheckedUnixNano != effect.CheckedUnixNano ||
		current.RuntimeEffectExpiresUnixNano != effect.ExpiresUnixNano ||
		current.RuntimePreparedProjectionDigest != prepared.ProjectionDigest ||
		current.RuntimePreparedCheckedUnixNano != prepared.CheckedUnixNano ||
		current.RuntimePreparedExpiresUnixNano != prepared.ExpiresUnixNano ||
		!SameRef(current.WorkspaceView, workspace.Meta.Ref()) ||
		current.WorkspaceSemanticDigest != workspaceDigest ||
		current.WorkspaceExpiresUnixNano != workspace.Meta.ExpiresUnixNano ||
		current.WorkspaceLeaseExpiresUnixNano != workspace.Lease.ExpiresUnixNano ||
		current.WorkspaceCheckedUnixNano > current.CheckedUnixNano ||
		current.WorkspaceCheckedUnixNano > now.UnixNano() {
		return errors.New("workspace read command fresh current proof drifted")
	}
	if err := validateWorkspaceReadCreateCurrentClosureV2(source, effect, prepared, workspace); err != nil {
		return err
	}
	expectedExpiry := workspaceReadCommandOwnerCurrentExpiryV2(current, time.Unix(0, current.CheckedUnixNano))
	if current.ExpiresUnixNano != expectedExpiry {
		return errors.New("workspace read command fresh current natural expiry drifted")
	}
	return nil
}

func sealWorkspaceReadCommandOwnerCurrentV2(
	p WorkspaceReadCommandOwnerCurrentV2,
	revision uint64,
	created, checked time.Time,
) (WorkspaceReadCommandOwnerCurrentV2, error) {
	if revision == 0 || created.IsZero() || checked.IsZero() || checked.Before(created) {
		return WorkspaceReadCommandOwnerCurrentV2{}, errors.New("workspace read command Owner current seal times are invalid")
	}
	p.ContractVersion = WorkspaceReadCommandPublicationContractVersionV2
	p.TypeURL = WorkspaceReadCommandOwnerCurrentTypeURLV2
	p.CheckedUnixNano = checked.UnixNano()
	p.ExpiresUnixNano = workspaceReadCommandOwnerCurrentExpiryV2(p, checked)
	if p.ExpiresUnixNano <= p.CheckedUnixNano {
		return WorkspaceReadCommandOwnerCurrentV2{}, errors.New("workspace read command Owner current has no fresh lifetime")
	}
	p.ProjectionDigest = ""
	id, err := DeriveWorkspaceReadCommandOwnerCurrentIDV2(p.Command)
	if err != nil {
		return WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	p.Meta = Meta{ExpiresUnixNano: p.ExpiresUnixNano}
	metaCopy := p
	metaCopy.Meta = Meta{ExpiresUnixNano: p.ExpiresUnixNano}
	metaDigest, err := Digest("workspace-read-command-owner-current-v2", metaCopy)
	if err != nil {
		return WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	p.Meta = Meta{
		ContractVersion: ContractFamily,
		ID:              id,
		Revision:        revision,
		Digest:          metaDigest,
		CreatedUnixNano: created.UnixNano(),
		UpdatedUnixNano: checked.UnixNano(),
		ExpiresUnixNano: p.ExpiresUnixNano,
	}
	projectionCopy := p
	projectionCopy.ProjectionDigest = ""
	projectionDigest, err := Digest("workspace-read-command-owner-current-projection-v2", projectionCopy)
	if err != nil {
		return WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	p.ProjectionDigest = projectionDigest
	return p, p.ValidateShape()
}

func workspaceReadCommandOwnerCurrentExpiryV2(
	p WorkspaceReadCommandOwnerCurrentV2,
	checked time.Time,
) int64 {
	if checked.IsZero() {
		return 0
	}
	return minUnixNanoV2(
		p.SemanticNotAfterUnixNano,
		p.SourceExpiresUnixNano,
		p.RuntimeEffectExpiresUnixNano,
		p.RuntimePreparedExpiresUnixNano,
		p.WorkspaceExpiresUnixNano,
		p.WorkspaceLeaseExpiresUnixNano,
		checked.Add(WorkspaceReadCommandOwnerCurrentMaxTTLV2).UnixNano(),
	)
}

func DeriveWorkspaceReadCommandIDV2(source WorkspaceReadSourceCommandRefV2) (string, error) {
	return deriveWorkspaceReadPublicationIDV2("workspace-read-command", source)
}

func DeriveWorkspaceReadCommandPublicationIDV2(source WorkspaceReadSourceCommandRefV2) (string, error) {
	return deriveWorkspaceReadPublicationIDV2("workspace-read-command-publication", source)
}

func DeriveWorkspaceReadCommandOwnerCurrentIDV2(command Ref) (string, error) {
	if err := command.ValidateShape("workspace read command"); err != nil {
		return "", err
	}
	return deriveWorkspaceReadPublicationIDV2("workspace-read-command-owner-current", command)
}

func deriveWorkspaceReadPublicationIDV2(kind string, value any) (string, error) {
	digest, err := runtimecore.CanonicalJSONDigest(
		workspaceReadCommandPublicationCanonicalDomainV2,
		WorkspaceReadCommandPublicationContractVersionV2,
		kind,
		value,
	)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", kind, strings.TrimPrefix(string(digest), "sha256:")), nil
}

func decodeWorkspaceReadCanonicalPayloadV2(raw []byte) (WorkspaceReadCanonicalPayloadV2, error) {
	var payload WorkspaceReadCanonicalPayloadV2
	if len(raw) == 0 || len(raw) > runtimeports.MaxOpaqueInlineBytes {
		return payload, errors.New("workspace read canonical inline payload is empty or oversized")
	}
	if err := runtimecore.DecodeStrictJSON(raw, &payload); err != nil {
		return WorkspaceReadCanonicalPayloadV2{}, err
	}
	if err := payload.Validate(); err != nil {
		return WorkspaceReadCanonicalPayloadV2{}, err
	}
	canonical, err := json.Marshal(payload)
	if err != nil || !bytes.Equal(canonical, raw) {
		return WorkspaceReadCanonicalPayloadV2{}, errors.New("workspace read inline payload is not canonical JSON")
	}
	return payload, nil
}

func validateWorkspaceReadSourceOwnerV2(owner runtimeports.EffectOwnerRefV2) error {
	if owner.Role != runtimeports.OwnerSettlement ||
		runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(owner.ComponentID)) != nil ||
		!validCoreLowerDigestV2(owner.ManifestDigest) {
		return errors.New("workspace read source Owner is incomplete or noncanonical")
	}
	return nil
}

func workspaceReadEffectOwnersContainV2(owners []runtimeports.EffectOwnerRefV2, expected runtimeports.EffectOwnerRefV2) bool {
	for _, owner := range owners {
		if owner == expected {
			return true
		}
	}
	return false
}

func sameWorkspaceReadRuntimeAttemptV2(
	left, right runtimeports.OperationDispatchAttemptRefV3,
) bool {
	leftDigest, leftErr := WorkspaceReadSourceRuntimeAttemptDigestV2(left)
	rightDigest, rightErr := WorkspaceReadSourceRuntimeAttemptDigestV2(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func cloneWorkspaceReadEffectIntentV2(value runtimeports.OperationEffectIntentV3) runtimeports.OperationEffectIntentV3 {
	if value.Operation.ExecutionScope.SandboxLease != nil {
		lease := *value.Operation.ExecutionScope.SandboxLease
		value.Operation.ExecutionScope.SandboxLease = &lease
	}
	value.Payload.Inline = append([]byte(nil), value.Payload.Inline...)
	value.Owners = append([]runtimeports.EffectOwnerRefV2(nil), value.Owners...)
	value.CredentialLeases = append([]runtimeports.CredentialLeaseRefV2(nil), value.CredentialLeases...)
	return value
}

func cloneWorkspaceReadPreparedSnapshotV2(
	value runtimeports.ControlledOperationPreparedSemanticSnapshotV2,
) runtimeports.ControlledOperationPreparedSemanticSnapshotV2 {
	if value.Attempt.Delegation != nil {
		delegation := *value.Attempt.Delegation
		value.Attempt.Delegation = &delegation
	}
	return value
}

func cloneWorkspaceViewV2(value WorkspaceView) WorkspaceView {
	value.ReadScopes = append([]string(nil), value.ReadScopes...)
	value.WriteScopes = append([]string(nil), value.WriteScopes...)
	value.HiddenScopes = append([]string(nil), value.HiddenScopes...)
	return value
}

func rawCoreDigestV2(value runtimecore.Digest) string {
	if !validCoreLowerDigestV2(value) {
		return ""
	}
	return strings.TrimPrefix(string(value), "sha256:")
}

func validCoreLowerDigestV2(value runtimecore.Digest) bool {
	text := string(value)
	return value.Validate() == nil &&
		strings.HasPrefix(text, "sha256:") &&
		len(text) == len("sha256:")+DigestSizeHex &&
		text == strings.ToLower(text)
}

func validRawLowerDigestV2(value string) bool {
	return len(value) == DigestSizeHex && value == strings.ToLower(value) && ValidDigest(value)
}

func minUnixNanoV2(values ...int64) int64 {
	result := int64(0)
	for _, value := range values {
		if result == 0 || value < result {
			result = value
		}
	}
	return result
}

func maxUnixNanoV2(values ...int64) int64 {
	result := int64(0)
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
}
