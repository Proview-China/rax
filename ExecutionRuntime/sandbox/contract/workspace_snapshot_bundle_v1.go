package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
)

const (
	WorkspaceSnapshotBundleContractVersionV1 = "praxis.sandbox/workspace-snapshot-bundle/v1"
	WorkspaceSnapshotEntrySetDigestDomainV1  = "praxis.sandbox/workspace-snapshot-entry-set/body/v1"
	WorkspaceSnapshotBundleDigestDomainV1    = "praxis.sandbox/workspace-snapshot-bundle/body/v1"

	MaxWorkspaceSnapshotEntriesV1      = 65_536
	MaxWorkspaceSnapshotContentBytesV1 = uint64(1 << 30)
	MaxWorkspaceSnapshotPathBytesV1    = 4_096
	WorkspaceRestoreMarkerPathV1       = ".praxis-workspace-root-v1.json"
)

type WorkspaceSnapshotEntryKindV1 string

const (
	WorkspaceSnapshotDirectory   WorkspaceSnapshotEntryKindV1 = "directory"
	WorkspaceSnapshotRegularFile WorkspaceSnapshotEntryKindV1 = "regular_file"
)

type WorkspaceSnapshotExcludedKindV1 string

const (
	WorkspaceSnapshotExcludedSymlink   WorkspaceSnapshotExcludedKindV1 = "symlink"
	WorkspaceSnapshotExcludedSubmodule WorkspaceSnapshotExcludedKindV1 = "submodule"
	WorkspaceSnapshotExcludedSocket    WorkspaceSnapshotExcludedKindV1 = "socket"
	WorkspaceSnapshotExcludedDevice    WorkspaceSnapshotExcludedKindV1 = "device"
	WorkspaceSnapshotExcludedFIFO      WorkspaceSnapshotExcludedKindV1 = "fifo"
	WorkspaceSnapshotExcludedSpecial   WorkspaceSnapshotExcludedKindV1 = "special"
)

type WorkspaceSnapshotResidualReasonV1 string

const (
	WorkspaceSnapshotResidualUnsupportedKind WorkspaceSnapshotResidualReasonV1 = "unsupported_kind"
	WorkspaceSnapshotResidualUnreadable      WorkspaceSnapshotResidualReasonV1 = "unreadable"
	WorkspaceSnapshotResidualPolicyExcluded  WorkspaceSnapshotResidualReasonV1 = "policy_excluded"
)

type WorkspaceSnapshotEntryV1 struct {
	Path          string                       `json:"path"`
	Kind          WorkspaceSnapshotEntryKindV1 `json:"kind"`
	Executable    bool                         `json:"executable"`
	Length        uint64                       `json:"length"`
	ContentDigest string                       `json:"content_digest,omitempty"`
	Content       []byte                       `json:"content,omitempty"`
}

type WorkspaceSnapshotExcludedV1 struct {
	Path   string                            `json:"path"`
	Kind   WorkspaceSnapshotExcludedKindV1   `json:"kind"`
	Reason WorkspaceSnapshotResidualReasonV1 `json:"reason"`
}

type WorkspaceSnapshotBundleV1 struct {
	ContractVersion   string                        `json:"contract_version"`
	SnapshotID        string                        `json:"snapshot_id"`
	TenantID          string                        `json:"tenant_id"`
	SourceScopeDigest string                        `json:"source_scope_digest"`
	Entries           []WorkspaceSnapshotEntryV1    `json:"entries"`
	Excluded          []WorkspaceSnapshotExcludedV1 `json:"excluded"`
	TotalBytes        uint64                        `json:"total_bytes"`
	EntrySetDigest    string                        `json:"entry_set_digest"`
	BundleDigest      string                        `json:"bundle_digest"`
}

func (v WorkspaceSnapshotBundleV1) Clone() WorkspaceSnapshotBundleV1 {
	clone := v
	clone.Entries = make([]WorkspaceSnapshotEntryV1, len(v.Entries))
	for i := range v.Entries {
		clone.Entries[i] = v.Entries[i]
		clone.Entries[i].Content = append([]byte(nil), v.Entries[i].Content...)
	}
	clone.Excluded = append([]WorkspaceSnapshotExcludedV1(nil), v.Excluded...)
	return clone
}

func SealWorkspaceSnapshotBundleV1(value WorkspaceSnapshotBundleV1) (WorkspaceSnapshotBundleV1, error) {
	value = value.Clone()
	value.ContractVersion = WorkspaceSnapshotBundleContractVersionV1
	value.TotalBytes = 0
	value.EntrySetDigest = ""
	value.BundleDigest = ""

	for i := range value.Entries {
		entry := &value.Entries[i]
		switch entry.Kind {
		case WorkspaceSnapshotDirectory:
			if entry.Executable || entry.Length != 0 || entry.ContentDigest != "" || len(entry.Content) != 0 {
				return WorkspaceSnapshotBundleV1{}, fmt.Errorf("directory %q carries file fields", entry.Path)
			}
			entry.Content = nil
		case WorkspaceSnapshotRegularFile:
			entry.Length = uint64(len(entry.Content))
			sum := sha256.Sum256(entry.Content)
			entry.ContentDigest = hex.EncodeToString(sum[:])
			if value.TotalBytes > MaxWorkspaceSnapshotContentBytesV1-entry.Length {
				return WorkspaceSnapshotBundleV1{}, errors.New("workspace snapshot content exceeds byte bound")
			}
			value.TotalBytes += entry.Length
		default:
			return WorkspaceSnapshotBundleV1{}, fmt.Errorf("unsupported workspace snapshot entry kind %q", entry.Kind)
		}
	}
	sort.Slice(value.Entries, func(i, j int) bool { return value.Entries[i].Path < value.Entries[j].Path })
	sort.Slice(value.Excluded, func(i, j int) bool { return value.Excluded[i].Path < value.Excluded[j].Path })

	if err := value.validateIdentityAndClosure(); err != nil {
		return WorkspaceSnapshotBundleV1{}, err
	}
	entryDigest, err := workspaceSnapshotEntrySetDigestV1(value)
	if err != nil {
		return WorkspaceSnapshotBundleV1{}, err
	}
	value.EntrySetDigest = entryDigest
	bundleDigest, err := workspaceSnapshotBundleDigestV1(value)
	if err != nil {
		return WorkspaceSnapshotBundleV1{}, err
	}
	value.BundleDigest = bundleDigest
	return value, value.ValidateShape()
}

func (v WorkspaceSnapshotBundleV1) ValidateShape() error {
	if v.ContractVersion != WorkspaceSnapshotBundleContractVersionV1 {
		return errors.New("workspace snapshot bundle contract version is invalid")
	}
	if err := v.validateIdentityAndClosure(); err != nil {
		return err
	}
	var total uint64
	for _, entry := range v.Entries {
		switch entry.Kind {
		case WorkspaceSnapshotDirectory:
			if entry.Executable || entry.Length != 0 || entry.ContentDigest != "" || len(entry.Content) != 0 {
				return fmt.Errorf("directory %q carries file fields", entry.Path)
			}
		case WorkspaceSnapshotRegularFile:
			if entry.Length != uint64(len(entry.Content)) || !ValidDigest(entry.ContentDigest) {
				return fmt.Errorf("regular file %q length or digest shape is invalid", entry.Path)
			}
			sum := sha256.Sum256(entry.Content)
			if hex.EncodeToString(sum[:]) != entry.ContentDigest {
				return fmt.Errorf("regular file %q content digest mismatch", entry.Path)
			}
			if total > MaxWorkspaceSnapshotContentBytesV1-entry.Length {
				return errors.New("workspace snapshot content exceeds byte bound")
			}
			total += entry.Length
		default:
			return fmt.Errorf("unsupported workspace snapshot entry kind %q", entry.Kind)
		}
	}
	if total != v.TotalBytes {
		return errors.New("workspace snapshot total bytes mismatch")
	}
	entryDigest, err := workspaceSnapshotEntrySetDigestV1(v)
	if err != nil {
		return err
	}
	if entryDigest != v.EntrySetDigest {
		return errors.New("workspace snapshot entry set digest mismatch")
	}
	bundleDigest, err := workspaceSnapshotBundleDigestV1(v)
	if err != nil {
		return err
	}
	if bundleDigest != v.BundleDigest {
		return errors.New("workspace snapshot bundle digest mismatch")
	}
	return nil
}

func (v WorkspaceSnapshotBundleV1) validateIdentityAndClosure() error {
	if strings.TrimSpace(v.SnapshotID) == "" || strings.TrimSpace(v.TenantID) == "" || !ValidDigest(v.SourceScopeDigest) {
		return errors.New("workspace snapshot identity or source scope is invalid")
	}
	if len(v.Entries)+len(v.Excluded) == 0 {
		return errors.New("workspace snapshot must contain an entry or declared residual")
	}
	if len(v.Entries)+len(v.Excluded) > MaxWorkspaceSnapshotEntriesV1 {
		return errors.New("workspace snapshot exceeds entry bound")
	}

	directories := make(map[string]struct{})
	allPaths := make(map[string]string, len(v.Entries)+len(v.Excluded))
	last := ""
	for i, entry := range v.Entries {
		if err := validateWorkspaceSnapshotPathV1(entry.Path); err != nil {
			return err
		}
		if i > 0 && entry.Path <= last {
			return errors.New("workspace snapshot entries must be sorted and unique")
		}
		last = entry.Path
		allPaths[entry.Path] = "entry"
		if entry.Kind == WorkspaceSnapshotDirectory {
			directories[entry.Path] = struct{}{}
		}
	}
	last = ""
	for i, excluded := range v.Excluded {
		if err := validateWorkspaceSnapshotPathV1(excluded.Path); err != nil {
			return err
		}
		if i > 0 && excluded.Path <= last {
			return errors.New("workspace snapshot residuals must be sorted and unique")
		}
		last = excluded.Path
		if _, exists := allPaths[excluded.Path]; exists {
			return fmt.Errorf("workspace snapshot path %q is both content and residual", excluded.Path)
		}
		allPaths[excluded.Path] = "residual"
		if err := excluded.ValidateShape(); err != nil {
			return err
		}
	}
	for snapshotPath := range allPaths {
		parent := path.Dir(snapshotPath)
		if parent != "." {
			if _, ok := directories[parent]; !ok {
				return fmt.Errorf("workspace snapshot path %q lacks exact parent directory %q", snapshotPath, parent)
			}
		}
		for ancestor := parent; ancestor != "."; ancestor = path.Dir(ancestor) {
			if kind, ok := allPaths[ancestor]; ok && kind != "entry" {
				return fmt.Errorf("workspace snapshot path %q is below residual %q", snapshotPath, ancestor)
			}
		}
	}
	return nil
}

func (v WorkspaceSnapshotExcludedV1) ValidateShape() error {
	switch v.Kind {
	case WorkspaceSnapshotExcludedSymlink, WorkspaceSnapshotExcludedSubmodule, WorkspaceSnapshotExcludedSocket,
		WorkspaceSnapshotExcludedDevice, WorkspaceSnapshotExcludedFIFO, WorkspaceSnapshotExcludedSpecial:
	default:
		return fmt.Errorf("unsupported workspace snapshot residual kind %q", v.Kind)
	}
	switch v.Reason {
	case WorkspaceSnapshotResidualUnsupportedKind, WorkspaceSnapshotResidualUnreadable, WorkspaceSnapshotResidualPolicyExcluded:
		return nil
	default:
		return fmt.Errorf("unsupported workspace snapshot residual reason %q", v.Reason)
	}
}

func EncodeWorkspaceSnapshotBundleV1(value WorkspaceSnapshotBundleV1) ([]byte, error) {
	if err := value.ValidateShape(); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func DecodeWorkspaceSnapshotBundleV1(data []byte) (WorkspaceSnapshotBundleV1, error) {
	value, err := DecodeStrict[WorkspaceSnapshotBundleV1](data)
	if err != nil {
		return WorkspaceSnapshotBundleV1{}, fmt.Errorf("decode workspace snapshot bundle: %w", err)
	}
	if err := value.ValidateShape(); err != nil {
		return WorkspaceSnapshotBundleV1{}, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return WorkspaceSnapshotBundleV1{}, err
	}
	if !bytes.Equal(data, canonical) {
		return WorkspaceSnapshotBundleV1{}, errors.New("workspace snapshot bundle bytes are not strict canonical json")
	}
	return value.Clone(), nil
}

func validateWorkspaceSnapshotPathV1(value string) error {
	if value == WorkspaceRestoreMarkerPathV1 || strings.HasPrefix(value, WorkspaceRestoreMarkerPathV1+"/") {
		return errors.New("workspace snapshot path uses the reserved restore marker namespace")
	}
	if len(value) > MaxWorkspaceSnapshotPathBytesV1 {
		return fmt.Errorf("workspace snapshot path exceeds %d bytes", MaxWorkspaceSnapshotPathBytesV1)
	}
	return ValidateLogicalPath(value)
}

func workspaceSnapshotEntrySetDigestV1(v WorkspaceSnapshotBundleV1) (string, error) {
	return Digest(WorkspaceSnapshotEntrySetDigestDomainV1, struct {
		Entries  []WorkspaceSnapshotEntryV1
		Excluded []WorkspaceSnapshotExcludedV1
	}{v.Entries, v.Excluded})
}

func workspaceSnapshotBundleDigestV1(v WorkspaceSnapshotBundleV1) (string, error) {
	v.BundleDigest = ""
	return Digest(WorkspaceSnapshotBundleDigestDomainV1, v)
}
