// Package rocksdb defines the content SPI and, under the continuity_rocksdb
// build tag, its optional production RocksDB implementation.
package rocksdb

import "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"

type ContentSPI interface {
	ports.ContentStore
}
