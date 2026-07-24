# Memory + Knowledge backend-neutral组件框架闭环

时间：2026-07-17 15:28（Asia/Shanghai）

## 粗粒度事件

以`tmp.document/Memory&Knowledge.md`为最高业务输入，Memory与Knowledge两个Owner的backend-neutral组件框架完成软件闭环：

- Candidate/Admission/Review gate/Commit/Correction/Supersede/Forget/Withdraw及DomainResultAssociation/opaque Settlement Apply；
- Memory Pin/Archive/Merge/Decay/Expiry、Knowledge Source/Package/Conflict/Deprecate/Snapshot/Sync；
- Scope/View/Disclosure、Skill/Lexical/Vector/Graph Projection、Hybrid Retrieval、Citation/Coverage/Cursor；
- Consolidation、Export、Watch、Reindex、metadata-only Purge Intent、Owner Job Journal；
- V1/V2 Owner-local Current Reader、SDK/API/CLI、扩展Conformance、Source Connector Observation合同与diagnostic metrics schema；
- API Purge TTL冻结为有界`ttl_seconds`，Knowledge Source/Record exact inspect入口补齐，CLI响应正文增加1 MiB上限。

没有预选生产DB、向量库、图库、RPC、进程拓扑或SLA；没有引入Rust。内存/reference实现不宣称生产持久性、容量或可用性。

## 实际验证

```text
go test ./...
go test -count=100 ./contracttest ./memory ./knowledge ./projection ./retrieval ./consolidation ./conformance ./observability ./sdk ./api ./cli
go test -race -count=20 ./memory ./knowledge ./projection ./retrieval ./consolidation ./conformance ./observability ./sdk ./api ./cli
go test -race ./...
go vet ./...
gofmt -d .
git diff --check -- <Memory/Knowledge独占实现与资产路径>
import-boundary / domain-zero-network scan
draw.io XML validation
Markdown relative-link validation
```

以上全部通过。Owner-local P0/P1/P2=0/0/0。

## 外部边界

External P0仍为5，不因组件本地闭环归零：

1. 具名Turn exact Ref/Reader及Application无损传递；
2. Context-owned TransitionProof public nominal/canonical/current/TTL；
3. Application namespaced三阶段Refresh Port；
4. Memory/Knowledge Adapter、nonzero cardinality与G6B/production root；
5. Context接受`knowledge_reference`及完整Knowledge source exact chain。

当前Context Owner-local Refresh只接受Tool=1、Memory=0、Knowledge=0；Harness只消费Context Owner发布并复读current的exact Frame。真实远程Retrieval/Connector/物理Purge执行继续unsupported，Provider/Resolver=0。
