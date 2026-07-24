package hostlocal

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type ConfigV2 struct {
	Root            string
	Key             []byte
	Namespace       contract.SnapshotArtifactExactRefV2
	Clock           func() time.Time
	MaxContentBytes uint64
	MaxArtifactTTL  time.Duration
}

type StoreV2 struct {
	root      string
	aead      cipher.AEAD
	namespace contract.SnapshotArtifactExactRefV2
	clock     func() time.Time
	maxBytes  uint64
	maxTTL    time.Duration
}

type envelopeV2 struct {
	ContractVersion string                                `json:"contract_version"`
	StorageRef      contract.SnapshotStorageArtifactRefV2 `json:"storage_ref"`
	Nonce           []byte                                `json:"nonce"`
	Ciphertext      []byte                                `json:"ciphertext"`
}

func NewStoreV2(config ConfigV2) (*StoreV2, error) {
	if strings.TrimSpace(config.Root) == "" || config.Clock == nil || config.MaxContentBytes == 0 || config.MaxArtifactTTL <= 0 || len(config.Key) != 32 {
		return nil, errors.New("host-local snapshot content store config is incomplete")
	}
	now := config.Clock()
	if err := config.Namespace.ValidateCurrent("host-local snapshot namespace", now); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(append([]byte(nil), config.Key...))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "objects"), 0o700); err != nil {
		return nil, err
	}
	return &StoreV2{root: root, aead: aead, namespace: config.Namespace, clock: config.Clock, maxBytes: config.MaxContentBytes, maxTTL: config.MaxArtifactTTL}, nil
}

func (s *StoreV2) PutSnapshotContentV2(ctx context.Context, input *contract.PutSnapshotContentRequestV2) (contract.PutSnapshotContentResultV2, error) {
	if input == nil {
		return contract.PutSnapshotContentResultV2{}, errors.New("snapshot content put request is required")
	}
	request := input.Clone()
	now := s.clock()
	if err := request.ValidateCurrent(now, s.maxBytes); err != nil {
		return contract.PutSnapshotContentResultV2{}, err
	}
	if err := s.namespace.ValidateCurrent("host-local snapshot namespace", now); err != nil {
		return contract.PutSnapshotContentResultV2{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.PutSnapshotContentResultV2{}, err
	}
	contentSum := sha256.Sum256(request.Content)
	contentDigest := hex.EncodeToString(contentSum[:])
	identityDigest, err := contract.Digest("praxis.sandbox/host-local-snapshot-content-identity/v2", struct {
		TenantID      string
		DataDomain    string
		Namespace     contract.SnapshotArtifactExactRefV2
		ContentDigest string
		Schema        contract.Ref
	}{request.TenantID, request.DataDomain, s.namespace, contentDigest, request.SchemaRef})
	if err != nil {
		return contract.PutSnapshotContentResultV2{}, err
	}
	expires := minUnixNanoV2(request.RequestedNotAfter, s.namespace.ExpiresUnixNano, now.Add(s.maxTTL).UnixNano())
	storageRef, err := contract.SealSnapshotStorageArtifactRefV2(contract.SnapshotStorageArtifactRefV2{StorageArtifactID: "host-local-snapshot-" + identityDigest[:32], Revision: 1, TenantID: request.TenantID, DataDomain: request.DataDomain, StorageNamespaceExactRef: s.namespace, ContentDigest: contentDigest, SchemaRef: request.SchemaRef, Length: uint64(len(request.Content)), EncryptionFactRef: request.EncryptionFactRef, ResidencyFactRef: request.ResidencyFactRef, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		return contract.PutSnapshotContentResultV2{}, err
	}
	aad, err := json.Marshal(storageRef)
	if err != nil {
		return contract.PutSnapshotContentResultV2{}, err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return contract.PutSnapshotContentResultV2{}, err
	}
	sealed := s.aead.Seal(nil, nonce, request.Content, aad)
	payload, err := json.Marshal(envelopeV2{ContractVersion: "praxis.sandbox/host-local-snapshot-content/v2", StorageRef: storageRef, Nonce: nonce, Ciphertext: sealed})
	if err != nil {
		return contract.PutSnapshotContentResultV2{}, err
	}
	path, err := s.objectPath(storageRef.StorageArtifactID)
	if err != nil {
		return contract.PutSnapshotContentResultV2{}, err
	}
	created, err := writeCreateOnceV2(path, payload)
	if err != nil {
		return contract.PutSnapshotContentResultV2{}, err
	}
	if !created {
		existing, inspectErr := s.InspectSnapshotContentV2(ctx, &contract.InspectSnapshotContentRequestV2{ExpectedRef: storageRef})
		if inspectErr != nil {
			return contract.PutSnapshotContentResultV2{}, inspectErr
		}
		if !bytes.Equal(existing.Content, request.Content) {
			return contract.PutSnapshotContentResultV2{}, fmt.Errorf("%w: host-local snapshot identity has different content", ports.ErrConflict)
		}
	}
	return contract.PutSnapshotContentResultV2{StorageRef: storageRef, Created: created}, nil
}

func (s *StoreV2) InspectSnapshotContentV2(ctx context.Context, input *contract.InspectSnapshotContentRequestV2) (contract.InspectSnapshotContentResultV2, error) {
	if input == nil {
		return contract.InspectSnapshotContentResultV2{}, errors.New("snapshot content inspect request is required")
	}
	now := s.clock()
	if err := input.ValidateCurrent(now); err != nil {
		return contract.InspectSnapshotContentResultV2{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.InspectSnapshotContentResultV2{}, err
	}
	path, err := s.objectPath(input.ExpectedRef.StorageArtifactID)
	if err != nil {
		return contract.InspectSnapshotContentResultV2{}, err
	}
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return contract.InspectSnapshotContentResultV2{}, ports.ErrNotFound
	}
	if err != nil {
		return contract.InspectSnapshotContentResultV2{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope envelopeV2
	if err := decoder.Decode(&envelope); err != nil {
		return contract.InspectSnapshotContentResultV2{}, fmt.Errorf("%w: decode host-local snapshot envelope: %v", ports.ErrConflict, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF || envelope.ContractVersion != "praxis.sandbox/host-local-snapshot-content/v2" || envelope.StorageRef != input.ExpectedRef || envelope.StorageRef.ValidateCurrent(now) != nil || len(envelope.Nonce) != s.aead.NonceSize() {
		return contract.InspectSnapshotContentResultV2{}, fmt.Errorf("%w: host-local snapshot envelope exact ref drifted", ports.ErrConflict)
	}
	aad, err := json.Marshal(envelope.StorageRef)
	if err != nil {
		return contract.InspectSnapshotContentResultV2{}, err
	}
	plain, err := s.aead.Open(nil, envelope.Nonce, envelope.Ciphertext, aad)
	if err != nil {
		return contract.InspectSnapshotContentResultV2{}, fmt.Errorf("%w: host-local snapshot authentication failed", ports.ErrConflict)
	}
	digest := sha256.Sum256(plain)
	if uint64(len(plain)) != envelope.StorageRef.Length || hex.EncodeToString(digest[:]) != envelope.StorageRef.ContentDigest {
		return contract.InspectSnapshotContentResultV2{}, fmt.Errorf("%w: host-local snapshot content digest drifted", ports.ErrConflict)
	}
	return contract.InspectSnapshotContentResultV2{StorageRef: envelope.StorageRef, Content: append([]byte(nil), plain...)}, nil
}

func (s *StoreV2) objectPath(id string) (string, error) {
	const prefix = "host-local-snapshot-"
	if !strings.HasPrefix(id, prefix) || len(id) != len(prefix)+32 {
		return "", fmt.Errorf("%w: host-local snapshot storage ID is non-canonical", ports.ErrConflict)
	}
	digestPart := strings.TrimPrefix(id, prefix)
	if _, err := hex.DecodeString(digestPart); err != nil {
		return "", fmt.Errorf("%w: host-local snapshot storage ID is non-canonical", ports.ErrConflict)
	}
	return filepath.Join(s.root, "objects", digestPart[:2], id+".snapshot"), nil
}

func writeCreateOnceV2(path string, payload []byte) (bool, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	temp, err := os.CreateTemp(dir, ".snapshot-*.tmp")
	if err != nil {
		return false, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return false, err
	}
	if _, err := temp.Write(payload); err != nil {
		temp.Close()
		return false, err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return false, err
	}
	if err := temp.Close(); err != nil {
		return false, err
	}
	if err := os.Link(tempPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil {
		return false, err
	}
	return true, closeErr
}

func minUnixNanoV2(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

var _ ports.SnapshotContentStoreV2 = (*StoreV2)(nil)
