package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MaxCasePageV1 = 256

type ListCasesRequestV1 struct {
	TenantID core.TenantID
	States   []contract.CaseStateV1
	AfterID  string
	Limit    int
}

type ListCasesResultV1 struct {
	Cases       []contract.ReviewCaseV1
	NextAfterID string
}

// CaseQueryStoreV1 is a read-only service projection over Review-owned facts.
// It grants no authority and must return deep clones from one committed
// snapshot.
type CaseQueryStoreV1 interface {
	ListCasesV1(context.Context, ListCasesRequestV1) (ListCasesResultV1, error)
}
