package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/current"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/ports"
	orgsqlite "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/storage/sqlite"
)

func TestSQLiteStoreConformanceV1(t *testing.T) {
	conformance.RunStoreAndReaderV1(t, func(t *testing.T) (ports.StoreV1, func()) {
		s, err := orgsqlite.Open(context.Background(), orgsqlite.Config{Path: filepath.Join(t.TempDir(), "organization.db"), Clock: func() time.Time { return time.Unix(1800000000, 0) }})
		if err != nil {
			t.Fatal(err)
		}
		return s, func() { _ = s.Close() }
	})
}

func TestSQLiteRestartIntegrityV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "organization.db")
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	s, err := orgsqlite.Open(ctx, orgsqlite.Config{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	source := conformance.SeedDirectV1(t, ctx, s, now)
	_ = source
	if err = s.IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = orgsqlite.Open(ctx, orgsqlite.Config{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
	reader, err := current.NewReaderV1(s, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reader.ResolveCurrentReviewEligibilityV1(ctx, source); err != nil {
		t.Fatalf("restart current read: %v", err)
	}
}
