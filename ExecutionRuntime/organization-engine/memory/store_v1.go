package memory

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type storedRef struct {
	Revision core.Revision
	Digest   core.Digest
}

type Store struct {
	mu          sync.RWMutex
	history     map[string]map[core.Revision][]byte
	current     map[string]storedRef
	projections map[string][]byte
}

func NewStore() *Store {
	return &Store{history: map[string]map[core.Revision][]byte{}, current: map[string]storedRef{}, projections: map[string][]byte{}}
}

func (s *Store) CreateOrInspectReviewEligibilityProjectionV1(ctx context.Context, value contract.ReviewEligibilityCurrentProjectionV1) (contract.ReviewEligibilityCurrentProjectionV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.IndeterminateV1("organization projection create context ended")
	}
	if err := value.Validate(); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "organization projection encode failed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.projectionClosureCurrentLocked(value) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.ConflictV1("projection closure is not current at publish")
	}
	k := key("projection", value.Ref.TenantID, value.Ref.ID)
	if old, ok := s.projections[k]; ok {
		if string(old) == string(payload) {
			return value.Clone(), nil
		}
		var existing contract.ReviewEligibilityCurrentProjectionV1
		if err := json.Unmarshal(old, &existing); err != nil {
			return contract.ReviewEligibilityCurrentProjectionV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "organization projection decode failed")
		}
		if sameProjectionClosure(existing, value) {
			return existing.Clone(), nil
		}
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.ConflictV1("projection id already carries different closure")
	}
	s.projections[k] = append([]byte(nil), payload...)
	return value.Clone(), nil
}

func (s *Store) projectionClosureCurrentLocked(v contract.ReviewEligibilityCurrentProjectionV1) bool {
	check := func(kind string, r storedRef, tenant core.TenantID, id string) bool {
		got, ok := s.current[key(kind, tenant, id)]
		return ok && got == r
	}
	if !check("identity", storedRef{v.Identity.Revision, v.Identity.Digest}, v.Identity.TenantID, v.Identity.ID) || !check("identity", storedRef{v.ResponsibilityIdentity.Revision, v.ResponsibilityIdentity.Digest}, v.ResponsibilityIdentity.TenantID, v.ResponsibilityIdentity.ID) || !check("responsibility", storedRef{v.Responsibility.Revision, v.Responsibility.Digest}, v.Responsibility.TenantID, v.Responsibility.ID) {
		return false
	}
	for _, x := range v.Roles {
		if !check("role", storedRef{x.Revision, x.Digest}, x.TenantID, x.ID) {
			return false
		}
	}
	if v.Delegation != nil {
		if v.DelegatorIdentity == nil || !check("delegation", storedRef{v.Delegation.Revision, v.Delegation.Digest}, v.Delegation.TenantID, v.Delegation.ID) || !check("identity", storedRef{v.DelegatorIdentity.Revision, v.DelegatorIdentity.Digest}, v.DelegatorIdentity.TenantID, v.DelegatorIdentity.ID) {
			return false
		}
	}
	return true
}

func sameProjectionClosure(a, b contract.ReviewEligibilityCurrentProjectionV1) bool {
	if a.Ref.ID != b.Ref.ID || a.Ref.Identity != b.Ref.Identity || a.Ref.Responsibility != b.Ref.Responsibility || len(a.Ref.Roles) != len(b.Ref.Roles) {
		return false
	}
	for i := range a.Ref.Roles {
		if a.Ref.Roles[i] != b.Ref.Roles[i] {
			return false
		}
	}
	if (a.Ref.Delegation == nil) != (b.Ref.Delegation == nil) {
		return false
	}
	return a.Ref.Delegation == nil || *a.Ref.Delegation == *b.Ref.Delegation
}

func (s *Store) InspectReviewEligibilityProjectionV1(ctx context.Context, ref contract.ReviewEligibilityProjectionRefV1) (contract.ReviewEligibilityCurrentProjectionV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.IndeterminateV1("organization projection inspect context ended")
	}
	if err := ref.Validate(); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	payload, ok := s.projections[key("projection", ref.TenantID, ref.ID)]
	payload = append([]byte(nil), payload...)
	s.mu.RUnlock()
	if !ok {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.NotFoundV1("organization projection not found")
	}
	var out contract.ReviewEligibilityCurrentProjectionV1
	if err := json.Unmarshal(payload, &out); err != nil {
		return out, core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "organization projection decode failed")
	}
	if err := out.Validate(); err != nil {
		return out, err
	}
	if out.ProjectionDigest != ref.Digest {
		return out, ports.ConflictV1("organization projection exact digest drifted")
	}
	return out.Clone(), nil
}

var _ ports.StoreV1 = (*Store)(nil)

func key(kind string, tenant core.TenantID, id string) string {
	return kind + "\x00" + string(tenant) + "\x00" + id
}

func (s *Store) publish(ctx context.Context, kind string, tenant core.TenantID, id string, revision core.Revision, digest core.Digest, expected *storedRef, value any) error {
	if err := ctx.Err(); err != nil {
		return ports.IndeterminateV1("organization memory mutation context ended")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "organization memory encode failed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(kind, tenant, id)
	current, exists := s.current[k]
	revisions := s.history[k]
	if revisions != nil {
		if old, ok := revisions[revision]; ok {
			if string(old) == string(payload) && current == (storedRef{revision, digest}) {
				return nil
			}
			return ports.ConflictV1("same revision carries different content or is no longer current")
		}
	}
	if expected == nil {
		if exists || len(revisions) != 0 || revision != 1 {
			return ports.ConflictV1("first publish requires empty history and revision one")
		}
	} else {
		if !exists || current != *expected {
			return ports.ConflictV1("current full ref CAS failed")
		}
		if revision != current.Revision+1 {
			return ports.ConflictV1("revision must increase by exactly one")
		}
	}
	if revisions == nil {
		revisions = map[core.Revision][]byte{}
		s.history[k] = revisions
	}
	revisions[revision] = append([]byte(nil), payload...)
	s.current[k] = storedRef{revision, digest}
	return nil
}

func (s *Store) inspect(ctx context.Context, kind string, tenant core.TenantID, id string, revision core.Revision, digest core.Digest, out any) error {
	if err := ctx.Err(); err != nil {
		return ports.IndeterminateV1("organization memory inspect context ended")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	payload, ok := s.history[key(kind, tenant, id)][revision]
	if !ok {
		return ports.NotFoundV1("organization exact fact not found")
	}
	if srefDigest(payload, out, digest) != nil {
		return ports.ConflictV1("organization exact fact digest drifted")
	}
	return nil
}

func srefDigest(payload []byte, out any, digest core.Digest) error {
	if err := json.Unmarshal(append([]byte(nil), payload...), out); err != nil {
		return err
	}
	var actual core.Digest
	switch v := out.(type) {
	case *contract.IdentityFactV1:
		actual = v.Digest
	case *contract.RoleGrantFactV1:
		actual = v.Digest
	case *contract.DelegationFactV1:
		actual = v.Digest
	case *contract.ResponsibilityFactV1:
		actual = v.Digest
	}
	if actual != digest {
		return ports.ConflictV1("exact digest mismatch")
	}
	return nil
}

func ref(v any) *storedRef {
	switch x := v.(type) {
	case *contract.IdentityRefV1:
		if x == nil {
			return nil
		}
		return &storedRef{x.Revision, x.Digest}
	case *contract.RoleGrantRefV1:
		if x == nil {
			return nil
		}
		return &storedRef{x.Revision, x.Digest}
	case *contract.DelegationRefV1:
		if x == nil {
			return nil
		}
		return &storedRef{x.Revision, x.Digest}
	case *contract.ResponsibilityRefV1:
		if x == nil {
			return nil
		}
		return &storedRef{x.Revision, x.Digest}
	default:
		return nil
	}
}

func (s *Store) PublishIdentityV1(c context.Context, e *contract.IdentityRefV1, v contract.IdentityFactV1) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return s.publish(c, "identity", v.TenantID, v.ID, v.Revision, v.Digest, ref(e), v)
}
func (s *Store) PublishRoleGrantV1(c context.Context, e *contract.RoleGrantRefV1, v contract.RoleGrantFactV1) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return s.publish(c, "role", v.TenantID, v.ID, v.Revision, v.Digest, ref(e), v)
}
func (s *Store) PublishDelegationV1(c context.Context, e *contract.DelegationRefV1, v contract.DelegationFactV1) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return s.publish(c, "delegation", v.TenantID, v.ID, v.Revision, v.Digest, ref(e), v)
}
func (s *Store) PublishResponsibilityV1(c context.Context, e *contract.ResponsibilityRefV1, v contract.ResponsibilityFactV1) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return s.publish(c, "responsibility", v.TenantID, v.ID, v.Revision, v.Digest, ref(e), v)
}
func (s *Store) InspectIdentityV1(c context.Context, r contract.IdentityRefV1) (v contract.IdentityFactV1, err error) {
	if err = r.Validate(); err != nil {
		return
	}
	err = s.inspect(c, "identity", r.TenantID, r.ID, r.Revision, r.Digest, &v)
	if err == nil {
		err = v.Validate()
	}
	return
}
func (s *Store) InspectRoleGrantV1(c context.Context, r contract.RoleGrantRefV1) (v contract.RoleGrantFactV1, err error) {
	if err = r.Validate(); err != nil {
		return
	}
	err = s.inspect(c, "role", r.TenantID, r.ID, r.Revision, r.Digest, &v)
	if err == nil {
		err = v.Validate()
	}
	return
}
func (s *Store) InspectDelegationV1(c context.Context, r contract.DelegationRefV1) (v contract.DelegationFactV1, err error) {
	if err = r.Validate(); err != nil {
		return
	}
	err = s.inspect(c, "delegation", r.TenantID, r.ID, r.Revision, r.Digest, &v)
	if err == nil {
		err = v.Validate()
	}
	return
}
func (s *Store) InspectResponsibilityV1(c context.Context, r contract.ResponsibilityRefV1) (v contract.ResponsibilityFactV1, err error) {
	if err = r.Validate(); err != nil {
		return
	}
	err = s.inspect(c, "responsibility", r.TenantID, r.ID, r.Revision, r.Digest, &v)
	if err == nil {
		err = v.Validate()
	}
	return
}

func (s *Store) ReadReviewEligibilityClosureV1(ctx context.Context, source contract.ReviewEligibilitySourceV1) (ports.ReviewEligibilityClosureV1, error) {
	if err := ctx.Err(); err != nil {
		return ports.ReviewEligibilityClosureV1{}, ports.IndeterminateV1("organization memory closure context ended")
	}
	ids, roleIDs, delegationID, responsibilityID, err := ports.StableIDsForSourceV1(source)
	if err != nil {
		return ports.ReviewEligibilityClosureV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	load := func(kind, id string, out any) error {
		k := key(kind, source.TenantID, id)
		r, ok := s.current[k]
		if !ok {
			return ports.NotFoundV1("organization current fact not found")
		}
		p := s.history[k][r.Revision]
		return srefDigest(p, out, r.Digest)
	}
	var out ports.ReviewEligibilityClosureV1
	if err = load("identity", ids, &out.Identity); err != nil {
		return out, err
	}
	out.Roles = make([]contract.RoleGrantFactV1, len(roleIDs))
	for i, id := range roleIDs {
		if err = load("role", id, &out.Roles[i]); err != nil {
			return ports.ReviewEligibilityClosureV1{}, err
		}
	}
	if source.RequireDelegation {
		var d contract.DelegationFactV1
		if err = load("delegation", delegationID, &d); err != nil {
			return ports.ReviewEligibilityClosureV1{}, err
		}
		out.Delegation = &d
		var delegator contract.IdentityFactV1
		if err = load("identity", d.Delegator.ID, &delegator); err != nil {
			return ports.ReviewEligibilityClosureV1{}, err
		}
		out.DelegatorIdentity = &delegator
	}
	if err = load("responsibility", responsibilityID, &out.Responsibility); err != nil {
		return ports.ReviewEligibilityClosureV1{}, err
	}
	if err = load("identity", out.Responsibility.Identity.ID, &out.ResponsibilityIdentity); err != nil {
		return ports.ReviewEligibilityClosureV1{}, err
	}
	return out.Clone(), nil
}
