package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"time"
)

const (
	ContractVersion   = "praxis.continuity/v1alpha1"
	ProjectionSchema  = "praxis.continuity.timeline/v1alpha1"
	DefaultCursorTTL  = 15 * time.Minute
	MaxIDLength       = 256
	MaxDigestLength   = 128
	MaxReferenceCount = 4096
)

var boundedASCII = regexp.MustCompile(`^[\x21-\x7e]+$`)

type Scope struct {
	TenantID             string `json:"tenant_id"`
	IdentityID           string `json:"identity_id"`
	IdentityEpoch        uint64 `json:"identity_epoch"`
	LineageID            string `json:"lineage_id"`
	PlanDigest           string `json:"plan_digest"`
	InstanceID           string `json:"instance_id"`
	InstanceEpoch        uint64 `json:"instance_epoch"`
	SandboxLeaseID       string `json:"sandbox_lease_id,omitempty"`
	SandboxLeaseEpoch    uint64 `json:"sandbox_lease_epoch,omitempty"`
	RunID                string `json:"run_id,omitempty"`
	RunIdentityDigest    string `json:"run_identity_digest,omitempty"`
	AuthorityEpoch       uint64 `json:"authority_epoch"`
	ExecutionScopeDigest string `json:"execution_scope_digest"`
}

func (s Scope) Validate() error {
	required := map[string]string{
		"tenant_id": s.TenantID, "identity_id": s.IdentityID, "lineage_id": s.LineageID,
		"plan_digest": s.PlanDigest, "instance_id": s.InstanceID,
		"execution_scope_digest": s.ExecutionScopeDigest,
	}
	for field, value := range required {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if s.IdentityEpoch == 0 || s.InstanceEpoch == 0 || s.AuthorityEpoch == 0 {
		return NewError(ErrInvalidArgument, "scope", "identity, instance, and authority epochs must be non-zero")
	}
	if (s.SandboxLeaseID == "") != (s.SandboxLeaseEpoch == 0) {
		return NewError(ErrInvalidArgument, "sandbox_lease", "id and epoch must be provided together")
	}
	if (s.RunID == "") != (s.RunIdentityDigest == "") {
		return NewError(ErrInvalidArgument, "run", "id and identity digest must be provided together")
	}
	return nil
}

type OwnerBinding struct {
	BindingSetID    string `json:"binding_set_id"`
	BindingRevision uint64 `json:"binding_revision"`
	ComponentID     string `json:"component_id"`
	ManifestDigest  string `json:"manifest_digest"`
	ArtifactDigest  string `json:"artifact_digest"`
	Capability      string `json:"capability"`
	FactKind        string `json:"fact_kind"`
}

func (o OwnerBinding) Validate() error {
	for field, value := range map[string]string{
		"binding_set_id": o.BindingSetID, "component_id": o.ComponentID,
		"manifest_digest": o.ManifestDigest, "artifact_digest": o.ArtifactDigest,
		"capability": o.Capability, "fact_kind": o.FactKind,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if o.BindingRevision == 0 {
		return NewError(ErrInvalidArgument, "binding_revision", "must be non-zero")
	}
	return nil
}

type FactRef struct {
	ID          string       `json:"id"`
	Revision    uint64       `json:"revision"`
	Digest      string       `json:"digest"`
	SchemaRef   string       `json:"schema_ref"`
	Owner       OwnerBinding `json:"owner"`
	ScopeDigest string       `json:"scope_digest"`
	CreatedAt   int64        `json:"created_unix_nano"`
	UpdatedAt   int64        `json:"updated_unix_nano"`
}

func (r FactRef) Validate() error {
	for field, value := range map[string]string{
		"id": r.ID, "digest": r.Digest, "schema_ref": r.SchemaRef, "scope_digest": r.ScopeDigest,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if r.Revision == 0 || r.CreatedAt <= 0 || r.UpdatedAt < r.CreatedAt {
		return NewError(ErrInvalidArgument, "fact_ref", "invalid revision or timestamps")
	}
	return r.Owner.Validate()
}

type ResidualRef struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	OwnerID        string `json:"owner_id"`
	ScopeDigest    string `json:"scope_digest"`
	SubjectDigest  string `json:"subject_digest"`
	State          string `json:"state"`
	InspectionRef  string `json:"inspection_ref,omitempty"`
	ConflictDomain string `json:"conflict_domain"`
}

func (r ResidualRef) Validate() error {
	for field, value := range map[string]string{
		"id": r.ID, "kind": r.Kind, "owner_id": r.OwnerID, "scope_digest": r.ScopeDigest,
		"subject_digest": r.SubjectDigest, "state": r.State, "conflict_domain": r.ConflictDomain,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	return nil
}

func ValidateToken(field, value string) error {
	if value == "" {
		return NewError(ErrInvalidArgument, field, "must not be empty")
	}
	if len(value) > MaxIDLength || !boundedASCII.MatchString(value) {
		return NewError(ErrInvalidArgument, field, "must be bounded printable ASCII")
	}
	return nil
}

func ValidateDigest(field, value string) error {
	if err := ValidateToken(field, value); err != nil {
		return err
	}
	if len(value) > MaxDigestLength {
		return NewError(ErrInvalidArgument, field, "digest is too long")
	}
	return nil
}

func CanonicalDigest(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func DigestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func NormalizeSet(values []string) ([]string, error) {
	if len(values) > MaxReferenceCount {
		return nil, NewError(ErrInvalidArgument, "references", "too many references")
	}
	result := append([]string{}, values...)
	sort.Strings(result)
	for i, value := range result {
		if err := ValidateToken("reference", value); err != nil {
			return nil, err
		}
		if i > 0 && result[i-1] == value {
			return nil, NewError(ErrInvalidArgument, "references", "duplicate reference")
		}
	}
	return result, nil
}

func NormalizeOrdered(values []string) ([]string, error) {
	if len(values) > MaxReferenceCount {
		return nil, NewError(ErrInvalidArgument, "references", "too many references")
	}
	result := append([]string{}, values...)
	seen := make(map[string]struct{}, len(result))
	for _, value := range result {
		if err := ValidateToken("reference", value); err != nil {
			return nil, err
		}
		if _, ok := seen[value]; ok {
			return nil, NewError(ErrInvalidArgument, "references", "duplicate reference")
		}
		seen[value] = struct{}{}
	}
	return result, nil
}

func NormalizeResiduals(values []ResidualRef) ([]ResidualRef, error) {
	if len(values) > MaxReferenceCount {
		return nil, NewError(ErrInvalidArgument, "residual_refs", "too many references")
	}
	result := append([]ResidualRef{}, values...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	for i, residual := range result {
		if err := residual.Validate(); err != nil {
			return nil, err
		}
		if i > 0 && result[i-1].ID == residual.ID {
			return nil, NewError(ErrInvalidArgument, "residual_refs", "duplicate residual id")
		}
	}
	return result, nil
}

func Contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
