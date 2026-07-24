package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	reviewsqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
)

func BenchmarkSQLiteInspectV1(b *testing.B) {
	ctx := context.Background()
	now := time.Unix(1_900_800_000, 0)
	store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: filepath.Join(b.TempDir(), "review.sqlite"), Clock: func() time.Time { return now }})
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	testkit.PublishRubric(ctx, store, now, "tenant-a")
	owner, err := service.New(store, func() time.Time { return now })
	if err != nil {
		b.Fatal(err)
	}
	target := testkit.Target(now)
	request := testkit.Request(now, target, "case-benchmark")
	trace := testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)
	if _, err := owner.SubmitV1(ctx, service.SubmitCommandV1{Request: request, Target: target, Trace: trace}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := owner.InspectV1(ctx, target.TenantID, request.CaseID); err != nil {
			b.Fatal(err)
		}
	}
}
