package contract

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"
)

const (
	WorkspaceRewindCompositionContractV1 = "praxis.sandbox/workspace-rewind-composition/v1"
	workspaceRewindCompositionDigestV1   = "workspace-rewind-composition-v1"
)

type WorkspaceRewindCompositionStateV1 string

const WorkspaceRewindStructurallyClosedV1 WorkspaceRewindCompositionStateV1 = "structurally_closed"

type ComposeWorkspaceRewindRequestV1 struct {
	ContractVersion         string    `json:"contract_version"`
	RequestID               string    `json:"request_id"`
	IdempotencyKey          string    `json:"idempotency_key"`
	PlannedChangeSetID      string    `json:"planned_change_set_id"`
	SourceWorkspaceViewRef  Ref       `json:"source_workspace_view_ref"`
	ExpectedBaseRevision    string    `json:"expected_base_revision"`
	ExpectedFileScopeDigest string    `json:"expected_file_scope_digest"`
	KeepChangeSetRefs       []Ref     `json:"keep_change_set_refs"`
	DropChangeSetRefs       []Ref     `json:"drop_change_set_refs"`
	RequestedNotAfter       time.Time `json:"requested_not_after"`
	Digest                  string    `json:"digest"`
}

func SealComposeWorkspaceRewindRequestV1(value ComposeWorkspaceRewindRequestV1) (ComposeWorkspaceRewindRequestV1, error) {
	value.ContractVersion = WorkspaceRewindCompositionContractV1
	value.KeepChangeSetRefs = normalizeWorkspaceRewindRefsV1(value.KeepChangeSetRefs)
	value.DropChangeSetRefs = normalizeWorkspaceRewindRefsV1(value.DropChangeSetRefs)
	value.Digest = ""
	digest, err := value.CanonicalDigestV1()
	if err != nil {
		return ComposeWorkspaceRewindRequestV1{}, err
	}
	value.Digest = digest
	return value, value.ValidateShape()
}

func (r ComposeWorkspaceRewindRequestV1) ValidateShape() error {
	if r.ContractVersion != WorkspaceRewindCompositionContractV1 || strings.TrimSpace(r.RequestID) == "" || strings.TrimSpace(r.IdempotencyKey) == "" || strings.TrimSpace(r.PlannedChangeSetID) == "" {
		return errors.New("workspace rewind request identity is incomplete")
	}
	if r.SourceWorkspaceViewRef.ValidateShape("source workspace view") != nil || !ValidDigest(r.ExpectedBaseRevision) || !ValidDigest(r.ExpectedFileScopeDigest) || r.RequestedNotAfter.IsZero() {
		return errors.New("workspace rewind request source coordinates are invalid")
	}
	if len(r.KeepChangeSetRefs)+len(r.DropChangeSetRefs) == 0 {
		return errors.New("workspace rewind request requires keep or drop ChangeSets")
	}
	if err := validateWorkspaceRewindRefSetsV1(r.KeepChangeSetRefs, r.DropChangeSetRefs, r.PlannedChangeSetID); err != nil {
		return err
	}
	expected, err := r.CanonicalDigestV1()
	if err != nil || r.Digest != expected {
		return errors.New("workspace rewind request digest drifted")
	}
	return nil
}

func (r ComposeWorkspaceRewindRequestV1) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || !now.Before(r.RequestedNotAfter) {
		return errors.New("workspace rewind request expired")
	}
	return nil
}

func (r ComposeWorkspaceRewindRequestV1) CanonicalDigestV1() (string, error) {
	copy := r
	copy.KeepChangeSetRefs = normalizeWorkspaceRewindRefsV1(copy.KeepChangeSetRefs)
	copy.DropChangeSetRefs = normalizeWorkspaceRewindRefsV1(copy.DropChangeSetRefs)
	copy.Digest = ""
	return Digest("workspace-rewind-request-v1", copy)
}

func (r ComposeWorkspaceRewindRequestV1) Clone() ComposeWorkspaceRewindRequestV1 {
	copy := r
	copy.KeepChangeSetRefs = append([]Ref(nil), r.KeepChangeSetRefs...)
	copy.DropChangeSetRefs = append([]Ref(nil), r.DropChangeSetRefs...)
	return copy
}

type WorkspaceRewindCompositionFactV1 struct {
	Meta                    Meta                              `json:"meta"`
	RequestID               string                            `json:"request_id"`
	RequestDigest           string                            `json:"request_digest"`
	IdempotencyKey          string                            `json:"idempotency_key"`
	TenantID                string                            `json:"tenant_id"`
	ScopeDigest             string                            `json:"scope_digest"`
	SourceWorkspaceViewRef  Ref                               `json:"source_workspace_view_ref"`
	ExpectedBaseRevision    string                            `json:"expected_base_revision"`
	ExpectedFileScopeDigest string                            `json:"expected_file_scope_digest"`
	KeepChangeSetRefs       []Ref                             `json:"keep_change_set_refs"`
	DropChangeSetRefs       []Ref                             `json:"drop_change_set_refs"`
	PlannedChangeSetRef     Ref                               `json:"planned_change_set_ref"`
	SelectionDigest         string                            `json:"selection_digest"`
	State                   WorkspaceRewindCompositionStateV1 `json:"state"`
}

func NewWorkspaceRewindCompositionFactV1(now, expires time.Time, request ComposeWorkspaceRewindRequestV1, view WorkspaceView, planned WorkspaceChangeSet) (WorkspaceRewindCompositionFactV1, error) {
	if err := request.ValidateCurrent(now); err != nil {
		return WorkspaceRewindCompositionFactV1{}, err
	}
	selection, err := WorkspaceRewindSelectionDigestV1(request.SourceWorkspaceViewRef, request.ExpectedBaseRevision, request.ExpectedFileScopeDigest, request.KeepChangeSetRefs, request.DropChangeSetRefs, planned.Meta.Ref())
	if err != nil {
		return WorkspaceRewindCompositionFactV1{}, err
	}
	fact := WorkspaceRewindCompositionFactV1{
		RequestID: request.RequestID, RequestDigest: request.Digest, IdempotencyKey: request.IdempotencyKey,
		TenantID: view.Lease.TenantID, ScopeDigest: view.Lease.ScopeDigest,
		SourceWorkspaceViewRef: request.SourceWorkspaceViewRef, ExpectedBaseRevision: request.ExpectedBaseRevision,
		ExpectedFileScopeDigest: request.ExpectedFileScopeDigest, KeepChangeSetRefs: append([]Ref(nil), request.KeepChangeSetRefs...),
		DropChangeSetRefs: append([]Ref(nil), request.DropChangeSetRefs...), PlannedChangeSetRef: planned.Meta.Ref(),
		SelectionDigest: selection, State: WorkspaceRewindStructurallyClosedV1,
	}
	// ExpiresUnixNano is part of the canonical payload. Set it before sealing
	// Meta so construction and subsequent ValidateShape use identical bytes.
	fact.Meta.ExpiresUnixNano = expires.UnixNano()
	fact.Meta, err = NewMeta("workspace-rewind-composition:"+request.RequestID, 1, now, expires, workspaceRewindCompositionDigestV1, fact.digestPayloadV1())
	if err != nil {
		return WorkspaceRewindCompositionFactV1{}, err
	}
	return fact, fact.ValidateShape()
}

func (f WorkspaceRewindCompositionFactV1) digestPayloadV1() any {
	return struct {
		RequestID, RequestDigest, IdempotencyKey, TenantID, ScopeDigest string
		SourceWorkspaceViewRef                                          Ref
		ExpectedBaseRevision, ExpectedFileScopeDigest                   string
		KeepChangeSetRefs, DropChangeSetRefs                            []Ref
		PlannedChangeSetRef                                             Ref
		SelectionDigest                                                 string
		State                                                           WorkspaceRewindCompositionStateV1
		ExpiresUnixNano                                                 int64
	}{f.RequestID, f.RequestDigest, f.IdempotencyKey, f.TenantID, f.ScopeDigest, f.SourceWorkspaceViewRef, f.ExpectedBaseRevision, f.ExpectedFileScopeDigest, normalizeWorkspaceRewindRefsV1(f.KeepChangeSetRefs), normalizeWorkspaceRewindRefsV1(f.DropChangeSetRefs), f.PlannedChangeSetRef, f.SelectionDigest, f.State, f.Meta.ExpiresUnixNano}
}

func (f WorkspaceRewindCompositionFactV1) ValidateShape() error {
	if f.Meta.ValidateShape() != nil || f.Meta.Revision != 1 || f.Meta.ID != "workspace-rewind-composition:"+f.RequestID || strings.TrimSpace(f.RequestID) == "" || strings.TrimSpace(f.IdempotencyKey) == "" || strings.TrimSpace(f.TenantID) == "" || !ValidDigest(f.ScopeDigest) || !ValidDigest(f.RequestDigest) || !ValidDigest(f.ExpectedBaseRevision) || !ValidDigest(f.ExpectedFileScopeDigest) || !ValidDigest(f.SelectionDigest) || f.State != WorkspaceRewindStructurallyClosedV1 {
		return errors.New("workspace rewind composition fact is incomplete")
	}
	if f.SourceWorkspaceViewRef.ValidateShape("source workspace view") != nil || f.PlannedChangeSetRef.ValidateShape("planned workspace change set") != nil {
		return errors.New("workspace rewind composition refs are invalid")
	}
	if err := validateWorkspaceRewindRefSetsV1(f.KeepChangeSetRefs, f.DropChangeSetRefs, f.PlannedChangeSetRef.ID); err != nil {
		return err
	}
	selection, err := WorkspaceRewindSelectionDigestV1(f.SourceWorkspaceViewRef, f.ExpectedBaseRevision, f.ExpectedFileScopeDigest, f.KeepChangeSetRefs, f.DropChangeSetRefs, f.PlannedChangeSetRef)
	if err != nil || selection != f.SelectionDigest {
		return errors.New("workspace rewind selection digest drifted")
	}
	digest, err := Digest(workspaceRewindCompositionDigestV1, f.digestPayloadV1())
	if err != nil || digest != f.Meta.Digest {
		return errors.New("workspace rewind composition digest drifted")
	}
	return nil
}

func (f WorkspaceRewindCompositionFactV1) ValidateCurrent(now time.Time) error {
	if err := f.ValidateShape(); err != nil {
		return err
	}
	return f.Meta.ValidateCurrent(now)
}

func (f WorkspaceRewindCompositionFactV1) Clone() WorkspaceRewindCompositionFactV1 {
	copy := f
	copy.KeepChangeSetRefs = append([]Ref(nil), f.KeepChangeSetRefs...)
	copy.DropChangeSetRefs = append([]Ref(nil), f.DropChangeSetRefs...)
	return copy
}

func WorkspaceRewindSelectionDigestV1(view Ref, baseRevision, fileScope string, keep, drop []Ref, planned Ref) (string, error) {
	return Digest("workspace-rewind-selection-v1", struct {
		View                    Ref
		BaseRevision, FileScope string
		Keep, Drop              []Ref
		Planned                 Ref
	}{view, baseRevision, fileScope, normalizeWorkspaceRewindRefsV1(keep), normalizeWorkspaceRewindRefsV1(drop), planned})
}

// ComposeWorkspaceRewindChangesV1 implements the first-version structural
// policy only. Any shared path, base/view drift, indeterminate source, or
// semantic dependency not represented by exact ChangeSets fails closed.
func ComposeWorkspaceRewindChangesV1(view WorkspaceView, keep, drop []WorkspaceChangeSet) ([]WorkspaceChange, error) {
	if err := view.ValidateShape(); err != nil {
		return nil, err
	}
	if len(keep) == 0 {
		return nil, errors.New("workspace rewind must keep at least one ChangeSet")
	}
	paths := make(map[string]string)
	result := make([]WorkspaceChange, 0)
	for _, group := range []struct {
		name string
		sets []WorkspaceChangeSet
	}{{"keep", keep}, {"drop", drop}} {
		for _, set := range group.sets {
			if err := set.ValidateShape(); err != nil || !SameRef(set.ViewRef, view.Meta.Ref()) || set.BaseArtifactRef != view.BaseArtifactRef || set.BaseRevision != view.BaseRevision {
				return nil, errors.New("workspace rewind source differs from the exact View or base revision")
			}
			switch set.State {
			case ChangeSetStaged, ChangeSetCandidate, ChangeSetGoverned, ChangeSetCommitted:
			default:
				return nil, errors.New("workspace rewind source is rejected or indeterminate")
			}
			for _, path := range set.CanonicalPathSet {
				if previous, exists := paths[path]; exists {
					return nil, errors.New("workspace rewind path conflict between " + previous + " and " + group.name)
				}
				paths[path] = group.name
			}
			if group.name == "keep" {
				result = append(result, set.Changes...)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.TargetPath != right.TargetPath {
			return left.TargetPath < right.TargetPath
		}
		return left.Kind < right.Kind
	})
	return append([]WorkspaceChange(nil), result...), nil
}

func SameWorkspaceRewindCompositionV1(left, right WorkspaceRewindCompositionFactV1) bool {
	return reflect.DeepEqual(left, right)
}

func normalizeWorkspaceRewindRefsV1(values []Ref) []Ref {
	result := append([]Ref(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		if result[i].Revision != result[j].Revision {
			return result[i].Revision < result[j].Revision
		}
		return result[i].Digest < result[j].Digest
	})
	return result
}

func validateWorkspaceRewindRefSetsV1(keep, drop []Ref, plannedID string) error {
	coordinates := make(map[struct {
		ID       string
		Revision uint64
	}]string, len(keep)+len(drop))
	for _, group := range []struct {
		name string
		refs []Ref
	}{{"keep", keep}, {"drop", drop}} {
		if !reflect.DeepEqual(group.refs, normalizeWorkspaceRewindRefsV1(group.refs)) {
			return errors.New("workspace rewind refs must be sorted")
		}
		for _, ref := range group.refs {
			if err := ref.ValidateShape(group.name + " ChangeSet"); err != nil {
				return err
			}
			if ref.ID == plannedID {
				return errors.New("planned ChangeSet cannot reuse a source identity")
			}
			coordinate := struct {
				ID       string
				Revision uint64
			}{ref.ID, ref.Revision}
			if previous, exists := coordinates[coordinate]; exists {
				return errors.New("same ChangeSet coordinate appears in " + previous + " and " + group.name)
			}
			coordinates[coordinate] = group.name
		}
	}
	return nil
}
