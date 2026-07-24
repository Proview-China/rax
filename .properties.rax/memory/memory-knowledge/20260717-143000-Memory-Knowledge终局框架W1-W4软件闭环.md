# Memory + Knowledge终局框架扩展切片验证

时间：2026-07-17 14:30（Asia/Shanghai）

## 粗粒度事件

以`tmp.document/Memory&Knowledge.md`为最高业务输入，Memory/Knowledge backend-neutral框架完成一组W1-W4扩展切片：

- Memory与Knowledge保持两个独立Owner/current；
- Memory补齐Pin、Archive、Forget、Retention/Legal Hold门禁；
- Skill/Lexical/Vector/Graph Projection与Hybrid RRF reference backend完成；
- Consolidation只消费已Settlement exact输入并只产Proposal；
- Knowledge Sync阶段Journal及`Prepare DomainResult -> Runtime Settlement -> Owner Apply -> Projection/Snapshot/Publish`两阶段Controller完成；
- Go SDK、严格reference HTTP handler及CLI七命令面完成；
- 未选择生产DB、Vector/Graph backend、RPC、拓扑或SLA；无Rust。

本事件不宣称终局文档全部完成。仍需补Merge/Decay、Export/Watch、Knowledge Deprecate/Reindex、安全Admission、自定义backend conformance及External P0跨Owner接线。

## 验证真值

以下命令实际通过：

```text
go test ./...
go test -count=100 ./contracttest ./memory ./knowledge ./projection ./retrieval ./consolidation ./sdk ./api ./cli
go test -race -count=20 ./memory ./knowledge ./projection ./retrieval ./consolidation ./sdk ./api ./cli
go test -race ./...
go vet ./...
gofmt -d .
```

## 外部门

Owner-local P0/P1/P2保持0/0/0。External P0仍为5：Turn exact Reader/传递、Context TransitionProof、Application三阶段Port、两Owner非零Adapter/root、Context `knowledge_reference`。Harness live已出现public Turn applicability coordinate，但尚不能替代获批Application/Context合同。远程Retrieval Gateway、真实Connector与production root继续NO-GO。
