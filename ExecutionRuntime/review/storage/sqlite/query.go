package sqlite

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
)

var _ reviewport.CaseQueryStoreV1 = (*Store)(nil)

func (s *Store) ListCasesV1(ctx context.Context, request reviewport.ListCasesRequestV1) (result reviewport.ListCasesResultV1, err error) {
	err = s.read(ctx, request.TenantID, func(state *memory.Store) error {
		result, err = state.ListCasesV1(ctx, request)
		return err
	})
	return
}
