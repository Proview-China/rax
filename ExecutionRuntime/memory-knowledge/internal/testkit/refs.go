package testkit

import "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"

func Ref(id string, revision uint64) contract.Ref {
	return contract.Ref{ID: id, Revision: revision, Digest: contract.MustDigest(struct {
		ID       string
		Revision uint64
	}{id, revision})}
}
