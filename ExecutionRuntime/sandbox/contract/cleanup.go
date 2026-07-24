package contract

import (
	"errors"
	"fmt"
	"strings"
)

type CleanupState string

const (
	CleanupNotRequired    CleanupState = "not_required"
	CleanupConfirmedClean CleanupState = "confirmed_clean"
	CleanupResidual       CleanupState = "residual"
	CleanupIndeterminate  CleanupState = "indeterminate"
)

func (s CleanupState) Validate() error {
	switch s {
	case CleanupNotRequired, CleanupConfirmedClean, CleanupResidual, CleanupIndeterminate:
		return nil
	default:
		return fmt.Errorf("unsupported cleanup state %q", s)
	}
}

type CleanupReport struct {
	Processes          CleanupState `json:"processes"`
	FileMounts         CleanupState `json:"file_mounts"`
	Network            CleanupState `json:"network"`
	Secrets            CleanupState `json:"secrets"`
	BackgroundTasks    CleanupState `json:"background_tasks"`
	RemoteContinuation CleanupState `json:"remote_continuation"`
	ProviderRetention  CleanupState `json:"provider_retention"`
	EvidenceRefs       []Ref        `json:"evidence_refs,omitempty"`
}

func (r CleanupReport) ValidateShape() error {
	states := []CleanupState{r.Processes, r.FileMounts, r.Network, r.Secrets, r.BackgroundTasks, r.RemoteContinuation, r.ProviderRetention}
	for _, state := range states {
		if err := state.Validate(); err != nil {
			return err
		}
	}
	if len(r.EvidenceRefs) == 0 {
		return errors.New("cleanup report needs evidence refs")
	}
	for _, ref := range r.EvidenceRefs {
		if err := ref.ValidateShape("cleanup evidence ref"); err != nil {
			return err
		}
	}
	return nil
}

func (r CleanupReport) Complete() bool {
	states := []CleanupState{r.Processes, r.FileMounts, r.Network, r.Secrets, r.BackgroundTasks, r.RemoteContinuation, r.ProviderRetention}
	for _, state := range states {
		if state != CleanupConfirmedClean && state != CleanupNotRequired {
			return false
		}
	}
	return len(r.EvidenceRefs) > 0
}

type Residual struct {
	Kind            string `json:"kind"`
	ScopeDigest     string `json:"scope_digest"`
	ConflictDomain  string `json:"conflict_domain"`
	Owner           string `json:"owner"`
	Inspectable     bool   `json:"inspectable"`
	Compensatable   bool   `json:"compensatable"`
	EvidenceRefs    []Ref  `json:"evidence_refs"`
	ResolutionState string `json:"resolution_state"`
}

func (r Residual) ValidateShape() error {
	if strings.TrimSpace(r.Kind) == "" || strings.TrimSpace(r.ConflictDomain) == "" || strings.TrimSpace(r.Owner) == "" || strings.TrimSpace(r.ResolutionState) == "" {
		return errors.New("residual kind, conflict domain, owner, and resolution state are required")
	}
	if !ValidDigest(r.ScopeDigest) {
		return errors.New("residual scope digest is invalid")
	}
	if len(r.EvidenceRefs) == 0 {
		return errors.New("residual evidence refs are required")
	}
	for _, ref := range r.EvidenceRefs {
		if err := ref.ValidateShape("residual evidence ref"); err != nil {
			return err
		}
	}
	return nil
}
