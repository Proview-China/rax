// Package contextsourcev2 contains owner-neutral mechanics only. It does not
// define Memory or Knowledge DTOs, current state, or domain semantics.
package contextsourcev2

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

const lockPollInterval = time.Millisecond

// RLock acquires the caller's owner consistency lock without hiding context
// cancellation behind an unbounded sync.RWMutex wait.
func RLock(ctx context.Context, mu *sync.RWMutex) error {
	if ctx == nil {
		return contract.ErrInvalidArgument
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if mu.TryRLock() {
			if err := ctx.Err(); err != nil {
				mu.RUnlock()
				return err
			}
			return nil
		}
		timer := time.NewTimer(lockPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func NormalizeStrings(values []string, allowEmpty bool) ([]string, error) {
	out := slices.Clone(values)
	for _, value := range out {
		if value != strings.TrimSpace(value) || (!allowEmpty && value == "") {
			return nil, contract.ErrInvalidArgument
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func NormalizeRefs(values []contract.Ref) ([]contract.Ref, error) {
	seen := make(map[string]contract.Ref, len(values))
	for _, ref := range values {
		if err := ref.Validate(); err != nil {
			return nil, err
		}
		if prior, ok := seen[ref.ID]; ok && !contract.SameRef(prior, ref) {
			return nil, contract.ErrEvidenceConflict
		}
		seen[ref.ID] = ref
	}
	return contract.NormalizeRefs(values), nil
}

func SameRefs(a, b []contract.Ref) bool {
	na, err := NormalizeRefs(a)
	if err != nil {
		return false
	}
	nb, err := NormalizeRefs(b)
	return err == nil && slices.Equal(na, nb)
}
