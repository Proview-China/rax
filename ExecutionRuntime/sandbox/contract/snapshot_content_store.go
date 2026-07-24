package contract

import (
	"errors"
	"strings"
	"time"
)

type PutSnapshotContentRequestV2 struct {
	TenantID          string `json:"tenant_id"`
	DataDomain        string `json:"data_domain"`
	Content           []byte `json:"content"`
	SchemaRef         Ref    `json:"schema_ref"`
	EncryptionFactRef Ref    `json:"encryption_fact_ref"`
	ResidencyFactRef  Ref    `json:"residency_fact_ref"`
	RequestedNotAfter int64  `json:"requested_not_after"`
}

func (r PutSnapshotContentRequestV2) Clone() PutSnapshotContentRequestV2 {
	r.Content = append([]byte(nil), r.Content...)
	return r
}

func (r PutSnapshotContentRequestV2) ValidateCurrent(now time.Time, maxBytes uint64) error {
	if strings.TrimSpace(r.TenantID) == "" || strings.TrimSpace(r.DataDomain) == "" || len(r.Content) == 0 || uint64(len(r.Content)) > maxBytes || r.RequestedNotAfter <= 0 || now.IsZero() || now.UnixNano() >= r.RequestedNotAfter {
		return errors.New("snapshot content put request is incomplete, oversized, or stale")
	}
	for name, ref := range map[string]Ref{"schema": r.SchemaRef, "encryption fact": r.EncryptionFactRef, "residency fact": r.ResidencyFactRef} {
		if err := ref.ValidateShape("snapshot content " + name); err != nil {
			return err
		}
	}
	return nil
}

type PutSnapshotContentResultV2 struct {
	StorageRef SnapshotStorageArtifactRefV2 `json:"storage_ref"`
	Created    bool                         `json:"created"`
}

type InspectSnapshotContentRequestV2 struct {
	ExpectedRef SnapshotStorageArtifactRefV2 `json:"expected_ref"`
}

func (r InspectSnapshotContentRequestV2) ValidateCurrent(now time.Time) error {
	return r.ExpectedRef.ValidateCurrent(now)
}

type InspectSnapshotContentResultV2 struct {
	StorageRef SnapshotStorageArtifactRefV2 `json:"storage_ref"`
	Content    []byte                       `json:"content"`
}

func (r InspectSnapshotContentResultV2) Clone() InspectSnapshotContentResultV2 {
	r.Content = append([]byte(nil), r.Content...)
	return r
}
