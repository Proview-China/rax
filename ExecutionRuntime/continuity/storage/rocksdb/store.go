//go:build cgo && continuity_rocksdb

package rocksdb

/*
#cgo LDFLAGS: -lrocksdb -lstdc++ -lm -lzstd -llz4 -lz -lsnappy -lbz2
#include <stdlib.h>
#include <rocksdb/c.h>
*/
import "C"

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

// Store is the optional production content-addressed backend. It is isolated
// behind the continuity_rocksdb build tag because it requires CGO and a
// compatible system RocksDB C API. The bridge intentionally exposes only the
// ContentStore surface; RocksDB types never cross this package boundary.
type Store struct {
	mu sync.RWMutex

	db      *C.rocksdb_t
	options *C.rocksdb_options_t
	read    *C.rocksdb_readoptions_t
	write   *C.rocksdb_writeoptions_t
	closed  bool
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, contract.NewError(contract.ErrInvalidArgument, "rocksdb_path", "path is required")
	}
	options := C.rocksdb_options_create()
	C.rocksdb_options_set_create_if_missing(options, 1)
	C.rocksdb_options_set_paranoid_checks(options, 1)
	C.rocksdb_options_set_compression(options, C.rocksdb_snappy_compression)
	C.rocksdb_options_set_bytes_per_sync(options, 1<<20)
	C.rocksdb_options_set_wal_bytes_per_sync(options, 1<<20)
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var cError *C.char
	db := C.rocksdb_open(options, cpath, &cError)
	if err := takeError(cError); err != nil {
		C.rocksdb_options_destroy(options)
		return nil, unavailable("open", err)
	}
	read := C.rocksdb_readoptions_create()
	C.rocksdb_readoptions_set_verify_checksums(read, 1)
	write := C.rocksdb_writeoptions_create()
	C.rocksdb_writeoptions_set_sync(write, 1)
	C.rocksdb_writeoptions_disable_WAL(write, 0)
	return &Store{db: db, options: options, read: read, write: write}, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	C.rocksdb_close(s.db)
	C.rocksdb_readoptions_destroy(s.read)
	C.rocksdb_writeoptions_destroy(s.write)
	C.rocksdb_options_destroy(s.options)
	return nil
}

func (s *Store) PutChunk(ctx context.Context, ref contract.ChunkRef, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateChunk(ref, data); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return contract.NewError(contract.ErrUnavailable, "rocksdb", "store is closed")
	}
	existing, err := s.getLocked(ref)
	if err != nil {
		return err
	}
	if len(existing) != 0 {
		return validateChunk(ref, existing)
	}
	key := chunkKey(ref)
	cKey := C.CBytes(key)
	cValue := C.CBytes(data)
	defer C.free(cKey)
	defer C.free(cValue)
	var cError *C.char
	C.rocksdb_put(s.db, s.write, (*C.char)(cKey), C.size_t(len(key)), (*C.char)(cValue), C.size_t(len(data)), &cError)
	if err := takeError(cError); err != nil {
		return unavailable("put chunk", err)
	}
	return nil
}

func (s *Store) GetChunk(ctx context.Context, ref contract.ChunkRef) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, contract.NewError(contract.ErrUnavailable, "rocksdb", "store is closed")
	}
	data, err := s.getLocked(ref)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, contract.NewError(contract.ErrNotFound, "chunk", "chunk not found")
	}
	if err := validateChunk(ref, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Store) HasChunk(ctx context.Context, ref contract.ChunkRef) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := ref.Validate(); err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return false, contract.NewError(contract.ErrUnavailable, "rocksdb", "store is closed")
	}
	data, err := s.getLocked(ref)
	if err != nil {
		return false, err
	}
	if len(data) == 0 {
		return false, nil
	}
	if err := validateChunk(ref, data); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) getLocked(ref contract.ChunkRef) ([]byte, error) {
	key := chunkKey(ref)
	cKey := C.CBytes(key)
	defer C.free(cKey)
	var length C.size_t
	var cError *C.char
	value := C.rocksdb_get(s.db, s.read, (*C.char)(cKey), C.size_t(len(key)), &length, &cError)
	if err := takeError(cError); err != nil {
		return nil, unavailable("get chunk", err)
	}
	if value == nil {
		return nil, nil
	}
	defer C.rocksdb_free(unsafe.Pointer(value))
	return C.GoBytes(unsafe.Pointer(value), C.int(length)), nil
}

type Metrics struct {
	LatestSequenceNumber uint64
	EstimatedKeys        uint64
	LiveDataSizeBytes    uint64
	PendingCompaction    uint64
}

func (s *Store) Metrics() Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return Metrics{}
	}
	return Metrics{
		LatestSequenceNumber: uint64(C.rocksdb_get_latest_sequence_number(s.db)),
		EstimatedKeys:        s.property("rocksdb.estimate-num-keys"),
		LiveDataSizeBytes:    s.property("rocksdb.estimate-live-data-size"),
		PendingCompaction:    s.property("rocksdb.estimate-pending-compaction-bytes"),
	}
}

func (s *Store) property(name string) uint64 {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	value := C.rocksdb_property_value(s.db, cName)
	if value == nil {
		return 0
	}
	defer C.rocksdb_free(unsafe.Pointer(value))
	parsed, _ := strconv.ParseUint(C.GoString(value), 10, 64)
	return parsed
}

func validateChunk(ref contract.ChunkRef, data []byte) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if int64(len(data)) != ref.Length || contract.DigestBytes(data) != ref.Digest {
		return contract.NewError(contract.ErrContentDigestMismatch, "chunk", "length or digest mismatch")
	}
	return nil
}

func chunkKey(ref contract.ChunkRef) []byte { return []byte("chunk/v1/" + ref.Digest) }

func takeError(value *C.char) error {
	if value == nil {
		return nil
	}
	defer C.rocksdb_free(unsafe.Pointer(value))
	return &rocksError{message: C.GoString(value)}
}

type rocksError struct{ message string }

func (e *rocksError) Error() string { return e.message }

func unavailable(operation string, err error) error {
	return contract.NewError(contract.ErrUnavailable, "rocksdb", operation+": "+err.Error())
}

var _ ContentSPI = (*Store)(nil)
