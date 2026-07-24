package contract

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

type WorkspaceView struct {
	Meta            Meta                `json:"meta"`
	BaseArtifactRef Ref                 `json:"base_artifact_ref"`
	BaseRevision    string              `json:"base_revision"`
	OverlayRef      Ref                 `json:"overlay_ref"`
	PolicyRef       Ref                 `json:"policy_ref"`
	Lease           RuntimeLeaseBinding `json:"lease"`
	ReadScopes      []string            `json:"read_scopes"`
	WriteScopes     []string            `json:"write_scopes"`
	HiddenScopes    []string            `json:"hidden_scopes,omitempty"`
	FileScopeDigest string              `json:"file_scope_digest"`
}

func (v WorkspaceView) ValidateShape() error {
	if err := v.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := v.BaseArtifactRef.ValidateShape("base artifact ref"); err != nil {
		return err
	}
	if strings.TrimSpace(v.BaseRevision) == "" {
		return errors.New("base revision is required")
	}
	if err := v.OverlayRef.ValidateShape("overlay ref"); err != nil {
		return err
	}
	if err := v.PolicyRef.ValidateShape("policy ref"); err != nil {
		return err
	}
	if err := v.Lease.ValidateShape(); err != nil {
		return err
	}
	for name, values := range map[string][]string{"read scopes": v.ReadScopes, "write scopes": v.WriteScopes, "hidden scopes": v.HiddenScopes} {
		if err := ValidateSortedUnique(values, name); err != nil {
			return err
		}
		for _, value := range values {
			if err := ValidateLogicalPath(value); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	if !ValidDigest(v.FileScopeDigest) {
		return errors.New("file scope digest is invalid")
	}
	return nil
}

func (v WorkspaceView) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	if err := v.Meta.ValidateCurrent(now); err != nil {
		return err
	}
	return v.Lease.ValidateCurrent(now)
}

func ValidateLogicalPath(value string) error {
	if value == "" || path.IsAbs(value) || strings.Contains(value, "\\") {
		return fmt.Errorf("path %q must be a non-empty logical slash path", value)
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return fmt.Errorf("path %q is not canonical or escapes scope", value)
	}
	return nil
}

type WorkspaceChangeKind string

const (
	WorkspaceAdd    WorkspaceChangeKind = "add"
	WorkspaceModify WorkspaceChangeKind = "modify"
	WorkspaceDelete WorkspaceChangeKind = "delete"
	WorkspaceRename WorkspaceChangeKind = "rename"
)

type WorkspaceChange struct {
	Kind       WorkspaceChangeKind `json:"kind"`
	Path       string              `json:"path"`
	TargetPath string              `json:"target_path,omitempty"`
	BlobRef    *Ref                `json:"blob_ref,omitempty"`
}

func (c WorkspaceChange) Validate() error {
	switch c.Kind {
	case WorkspaceAdd, WorkspaceModify:
		if c.BlobRef == nil {
			return errors.New("add/modify change needs blob ref")
		}
		if err := c.BlobRef.ValidateShape("change blob ref"); err != nil {
			return err
		}
	case WorkspaceDelete:
		if c.BlobRef != nil || c.TargetPath != "" {
			return errors.New("delete change cannot carry blob or target path")
		}
	case WorkspaceRename:
		if c.TargetPath == "" || c.BlobRef != nil {
			return errors.New("rename change needs target path and no blob")
		}
	default:
		return fmt.Errorf("unsupported workspace change kind %q", c.Kind)
	}
	if err := ValidateLogicalPath(c.Path); err != nil {
		return err
	}
	if c.TargetPath != "" {
		return ValidateLogicalPath(c.TargetPath)
	}
	return nil
}

type WorkspaceChangeSetState string

const (
	ChangeSetStaged    WorkspaceChangeSetState = "staged"
	ChangeSetCandidate WorkspaceChangeSetState = "candidate"
	ChangeSetGoverned  WorkspaceChangeSetState = "governed"
	ChangeSetCommitted WorkspaceChangeSetState = "committed"
	ChangeSetRejected  WorkspaceChangeSetState = "rejected"
	ChangeSetUnknown   WorkspaceChangeSetState = "indeterminate"
)

type WorkspaceChangeSet struct {
	Meta              Meta                           `json:"meta"`
	ViewRef           Ref                            `json:"view_ref"`
	BaseArtifactRef   Ref                            `json:"base_artifact_ref"`
	BaseRevision      string                         `json:"base_revision"`
	Changes           []WorkspaceChange              `json:"changes"`
	CanonicalPathSet  []string                       `json:"canonical_path_set"`
	State             WorkspaceChangeSetState        `json:"state"`
	RuntimeSettlement *RuntimeOperationSettlementRef `json:"runtime_settlement,omitempty"`
	CommittedRevision string                         `json:"committed_revision,omitempty"`
}

const workspaceChangeSetDigestDiscriminator = "workspace-change-set"

type workspaceChangeSetDigestPayload struct {
	ViewRef           Ref
	BaseArtifactRef   Ref
	BaseRevision      string
	Changes           []WorkspaceChange
	Paths             []string
	State             WorkspaceChangeSetState
	RuntimeSettlement *RuntimeOperationSettlementRef
	CommittedRevision string
	ExpiresUnixNano   int64
}

// SealWorkspaceChangeSetMeta binds the final expiry into the canonical
// change-set digest. Callers must use the returned Meta without changing its
// expiry; ValidateShape verifies the same canonical seal.
func SealWorkspaceChangeSetMeta(id string, revision uint64, now, expires time.Time, set WorkspaceChangeSet) (Meta, error) {
	set.Meta.ExpiresUnixNano = expires.UnixNano()
	return NewMeta(id, revision, now, expires, workspaceChangeSetDigestDiscriminator, set.digestPayload())
}

func (s WorkspaceChangeSet) digestPayload() workspaceChangeSetDigestPayload {
	return workspaceChangeSetDigestPayload{
		ViewRef:           s.ViewRef,
		BaseArtifactRef:   s.BaseArtifactRef,
		BaseRevision:      s.BaseRevision,
		Changes:           s.Changes,
		Paths:             s.CanonicalPathSet,
		State:             s.State,
		RuntimeSettlement: s.RuntimeSettlement,
		CommittedRevision: s.CommittedRevision,
		ExpiresUnixNano:   s.Meta.ExpiresUnixNano,
	}
}

func (s WorkspaceChangeSet) ValidateShape() error {
	if err := s.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := s.ViewRef.ValidateShape("workspace view ref"); err != nil {
		return err
	}
	if err := s.BaseArtifactRef.ValidateShape("base artifact ref"); err != nil {
		return err
	}
	if strings.TrimSpace(s.BaseRevision) == "" || len(s.Changes) == 0 {
		return errors.New("base revision and changes are required")
	}
	paths := make([]string, 0, len(s.Changes)*2)
	for _, change := range s.Changes {
		if err := change.Validate(); err != nil {
			return err
		}
		paths = append(paths, change.Path)
		if change.TargetPath != "" {
			paths = append(paths, change.TargetPath)
		}
	}
	sort.Strings(paths)
	if len(paths) != len(s.CanonicalPathSet) {
		return errors.New("canonical path set does not match changes")
	}
	for i := range paths {
		if paths[i] != s.CanonicalPathSet[i] || (i > 0 && paths[i] == paths[i-1]) {
			return errors.New("canonical path set must be sorted, unique, and exact")
		}
	}
	switch s.State {
	case ChangeSetStaged, ChangeSetCandidate, ChangeSetGoverned, ChangeSetRejected, ChangeSetUnknown:
		if s.RuntimeSettlement != nil || s.CommittedRevision != "" {
			return errors.New("non-committed change set cannot claim runtime settlement or committed revision")
		}
	case ChangeSetCommitted:
		if s.RuntimeSettlement == nil || s.RuntimeSettlement.ValidateShape() != nil || !ValidDigest(s.CommittedRevision) {
			return errors.New("committed change set requires exact Runtime settlement and committed revision")
		}
	default:
		return fmt.Errorf("unsupported change set state %q", s.State)
	}
	expectedDigest, err := Digest(workspaceChangeSetDigestDiscriminator, s.digestPayload())
	if err != nil {
		return fmt.Errorf("workspace change set digest: %w", err)
	}
	if s.Meta.Digest != expectedDigest {
		return errors.New("workspace change set canonical digest does not match content or expiry")
	}
	return nil
}

// ApplyWorkspaceCommitSettlement advances only after the Sandbox DomainResult
// has been independently settled by Runtime. It never mutates Runtime facts.
func ApplyWorkspaceCommitSettlement(now time.Time, current WorkspaceChangeSet, result SandboxDomainResultFact, settlement RuntimeOperationSettlementRef, committedRevision string) (WorkspaceChangeSet, error) {
	if err := current.ValidateCurrent(now); err != nil {
		return WorkspaceChangeSet{}, err
	}
	if current.State == ChangeSetCommitted {
		if current.RuntimeSettlement != nil && SameRef(current.RuntimeSettlement.OpaqueRef, settlement.OpaqueRef) && SameRef(current.RuntimeSettlement.DomainResultRef, result.Meta.Ref()) && current.CommittedRevision == committedRevision {
			return current, nil
		}
		return WorkspaceChangeSet{}, errors.New("committed workspace change set conflicts with requested closure")
	}
	if current.State == ChangeSetRejected || current.State == ChangeSetUnknown {
		return WorkspaceChangeSet{}, errors.New("workspace change set terminal state cannot commit")
	}
	if err := result.ValidateCurrent(now); err != nil || result.Kind != EffectWorkspaceCommit || result.Disposition != DispositionConfirmedApplied || result.Payload.WorkspaceChangeSetRef == nil || !SameRef(*result.Payload.WorkspaceChangeSetRef, current.Meta.Ref()) {
		return WorkspaceChangeSet{}, errors.New("workspace DomainResult does not confirm this exact change set")
	}
	if err := settlement.ValidateShape(); err != nil || settlement.OperationID != result.OperationID || settlement.AttemptID != result.AttemptID || !SameRef(settlement.DomainResultRef, result.Meta.Ref()) {
		return WorkspaceChangeSet{}, errors.New("Runtime settlement does not bind the exact workspace DomainResult")
	}
	if !ValidDigest(committedRevision) {
		return WorkspaceChangeSet{}, errors.New("committed workspace revision is invalid")
	}
	next := current
	next.State = ChangeSetCommitted
	next.RuntimeSettlement = &settlement
	next.CommittedRevision = committedRevision
	meta, err := NextMeta(current.Meta, now, workspaceChangeSetDigestDiscriminator, next.digestPayload())
	if err != nil {
		return WorkspaceChangeSet{}, err
	}
	next.Meta = meta
	if err := next.ValidateCurrent(now); err != nil {
		return WorkspaceChangeSet{}, err
	}
	return next, nil
}

func (s WorkspaceChangeSet) ValidateCurrent(now time.Time) error {
	if err := s.ValidateShape(); err != nil {
		return err
	}
	return s.Meta.ValidateCurrent(now)
}
