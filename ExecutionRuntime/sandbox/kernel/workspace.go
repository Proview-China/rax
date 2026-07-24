package kernel

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

// StageWorkspaceChangeSet builds an in-memory overlay model only. It performs
// no filesystem access and cannot commit a real workspace.
func StageWorkspaceChangeSet(now, expires time.Time, id string, view contract.WorkspaceView, changes []contract.WorkspaceChange) (contract.WorkspaceChangeSet, error) {
	if err := view.ValidateCurrent(now); err != nil {
		return contract.WorkspaceChangeSet{}, fmt.Errorf("workspace view: %w", err)
	}
	expiryLimit := earliestUnixNano(view.Meta.ExpiresUnixNano, view.Lease.ExpiresUnixNano)
	if expires.IsZero() || expires.UnixNano() > expiryLimit {
		return contract.WorkspaceChangeSet{}, fmt.Errorf("workspace change set expiry exceeds upstream expiry %d", expiryLimit)
	}
	if len(changes) == 0 {
		return contract.WorkspaceChangeSet{}, errors.New("workspace changes are required")
	}
	paths := make([]string, 0, len(changes)*2)
	for _, change := range changes {
		if err := change.Validate(); err != nil {
			return contract.WorkspaceChangeSet{}, err
		}
		if !withinAnyScope(change.Path, view.WriteScopes) || withinAnyScope(change.Path, view.HiddenScopes) {
			return contract.WorkspaceChangeSet{}, fmt.Errorf("path %q is outside writable view", change.Path)
		}
		paths = append(paths, change.Path)
		if change.TargetPath != "" {
			if !withinAnyScope(change.TargetPath, view.WriteScopes) || withinAnyScope(change.TargetPath, view.HiddenScopes) {
				return contract.WorkspaceChangeSet{}, fmt.Errorf("target path %q is outside writable view", change.TargetPath)
			}
			paths = append(paths, change.TargetPath)
		}
	}
	sort.Strings(paths)
	for i := 1; i < len(paths); i++ {
		if paths[i] == paths[i-1] {
			return contract.WorkspaceChangeSet{}, fmt.Errorf("duplicate workspace path %q", paths[i])
		}
	}
	result := contract.WorkspaceChangeSet{
		ViewRef:          view.Meta.Ref(),
		BaseArtifactRef:  view.BaseArtifactRef,
		BaseRevision:     view.BaseRevision,
		Changes:          append([]contract.WorkspaceChange(nil), changes...),
		CanonicalPathSet: append([]string(nil), paths...),
		State:            contract.ChangeSetStaged,
	}
	meta, err := contract.SealWorkspaceChangeSetMeta(id, 1, now, expires, result)
	if err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	result.Meta = meta
	if err := result.ValidateCurrent(now); err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	return result, nil
}

// earliestUnixNano keeps derived facts within every TTL-bearing upstream fact
// available to this Wave 1 contract. Ref-only upstream objects have no local
// expiry to infer or extend here.
func earliestUnixNano(values ...int64) int64 {
	earliest := values[0]
	for _, value := range values[1:] {
		if value < earliest {
			earliest = value
		}
	}
	return earliest
}

func withinAnyScope(value string, scopes []string) bool {
	for _, scope := range scopes {
		if value == scope || strings.HasPrefix(value, scope+"/") {
			return true
		}
	}
	return false
}
