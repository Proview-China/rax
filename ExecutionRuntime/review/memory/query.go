package memory

import (
	"context"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var _ reviewport.CaseQueryStoreV1 = (*Store)(nil)

func (s *Store) ListCasesV1(ctx context.Context, request reviewport.ListCasesRequestV1) (reviewport.ListCasesResultV1, error) {
	if err := checkContext(ctx); err != nil {
		return reviewport.ListCasesResultV1{}, err
	}
	if strings.TrimSpace(string(request.TenantID)) == "" || request.Limit <= 0 || request.Limit > reviewport.MaxCasePageV1 || len(request.States) > contract.MaxListItemsV1 {
		return reviewport.ListCasesResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review case page request is invalid")
	}
	allowed := make(map[contract.CaseStateV1]bool, len(request.States))
	for _, state := range request.States {
		if err := contract.ValidateCaseStateV1(state); err != nil {
			return reviewport.ListCasesResultV1{}, err
		}
		allowed[state] = true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefix := string(request.TenantID) + "\x00"
	ids := make([]string, 0)
	for itemKey, value := range s.cases {
		if !strings.HasPrefix(itemKey, prefix) || (len(allowed) != 0 && !allowed[value.State]) || value.ID <= request.AfterID {
			continue
		}
		ids = append(ids, value.ID)
	}
	sort.Strings(ids)
	result := reviewport.ListCasesResultV1{}
	if len(ids) > request.Limit {
		result.NextAfterID = ids[request.Limit-1]
		ids = ids[:request.Limit]
	}
	result.Cases = make([]contract.ReviewCaseV1, 0, len(ids))
	for _, id := range ids {
		value, err := clone(s.cases[key(request.TenantID, id)])
		if err != nil {
			return reviewport.ListCasesResultV1{}, err
		}
		result.Cases = append(result.Cases, value)
	}
	return result, nil
}
