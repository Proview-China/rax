package projection

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

// Catalog is a reference owner-neutral descriptor catalog. Owner stores keep
// separate instances, so it never merges Memory and Knowledge current state.
type Catalog struct {
	mu    sync.RWMutex
	items map[string][]contract.IndexDescriptorV1
}

func NewCatalog() *Catalog { return &Catalog{items: make(map[string][]contract.IndexDescriptorV1)} }
func (c *Catalog) Publish(now time.Time, in contract.IndexDescriptorV1, expected contract.ExpectedRevision) (contract.IndexDescriptorV1, error) {
	return c.PublishAtomic(now, in, expected, nil)
}

// PublishAtomic validates and reserves the descriptor while holding the
// catalog lock, then runs commit before making the descriptor visible. Owner
// stores use it to publish Projection+Descriptor as one observable unit.
func (c *Catalog) PublishAtomic(now time.Time, in contract.IndexDescriptorV1, expected contract.ExpectedRevision, commit func() error) (contract.IndexDescriptorV1, error) {
	if c == nil {
		return contract.IndexDescriptorV1{}, contract.ErrInvalidArgument
	}
	if err := expected.Validate(); err != nil {
		return contract.IndexDescriptorV1{}, err
	}
	if err := in.Validate(now); err != nil {
		return contract.IndexDescriptorV1{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	history := c.items[in.Ref.ID]
	exists := len(history) > 0
	revision := uint64(0)
	if exists {
		revision = history[len(history)-1].Ref.Revision
	}
	if !expected.Matches(exists, revision) || in.Ref.Revision != revision+1 {
		return contract.IndexDescriptorV1{}, contract.ErrRevisionConflict
	}
	for id, other := range c.items {
		if id != in.Ref.ID && len(other) > 0 && other[len(other)-1].Owner == in.Owner && other[len(other)-1].Kind == in.Kind && contract.SameRef(other[len(other)-1].ViewRef, in.ViewRef) {
			return contract.IndexDescriptorV1{}, fmt.Errorf("%w: duplicate semantic index", contract.ErrEvidenceConflict)
		}
	}
	if commit != nil {
		if err := commit(); err != nil {
			return contract.IndexDescriptorV1{}, err
		}
	}
	c.items[in.Ref.ID] = append(history, cloneDescriptor(in))
	return cloneDescriptor(in), nil
}
func (c *Catalog) Inspect(now time.Time, ref contract.Ref) (contract.IndexDescriptorV1, error) {
	if c == nil || ref.Validate() != nil {
		return contract.IndexDescriptorV1{}, contract.ErrInvalidArgument
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, item := range c.items[ref.ID] {
		if contract.SameRef(item.Ref, ref) {
			if err := item.Validate(now); err != nil {
				return contract.IndexDescriptorV1{}, err
			}
			return cloneDescriptor(item), nil
		}
	}
	if len(c.items[ref.ID]) > 0 {
		return contract.IndexDescriptorV1{}, contract.ErrEvidenceConflict
	}
	return contract.IndexDescriptorV1{}, contract.ErrNotFound
}
func (c *Catalog) List(now time.Time, owner contract.OwnerDomain) ([]contract.IndexDescriptorV1, error) {
	if c == nil {
		return nil, contract.ErrInvalidArgument
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := []contract.IndexDescriptorV1{}
	for _, history := range c.items {
		if len(history) == 0 {
			continue
		}
		item := history[len(history)-1]
		if item.Owner == owner && item.Validate(now) == nil {
			out = append(out, cloneDescriptor(item))
		}
	}
	slices.SortFunc(out, func(a, b contract.IndexDescriptorV1) int { return strings.Compare(a.Ref.ID, b.Ref.ID) })
	return out, nil
}
func cloneDescriptor(in contract.IndexDescriptorV1) contract.IndexDescriptorV1 {
	in.RecordRefs = append([]contract.Ref{}, in.RecordRefs...)
	in.Coverage.ProjectionRefs = append([]contract.Ref{}, in.Coverage.ProjectionRefs...)
	in.Coverage.DroppedReasons = append([]string{}, in.Coverage.DroppedReasons...)
	return in
}
