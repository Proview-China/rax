# Review P4 assembly-candidate 收口

时间：2026-07-18 00:47 +08:00

## 结果

- 新增 `ExecutionRuntime/review/releasecandidate`，产出公共 `ComponentReleaseV1`。
- 候选固定 `support_mode=reference_only`；Manifest、Module、Capability、Port、Factory descriptor、effect/settlement/cleanup owners、artifact、candidate certification、evidence 与 TTL 已闭合。
- Conformance 保持 `restricted_controlled`，Residual 保持 `inspectable`；即使调用者改写 support mode 并重算 certification，Assembler production 校验仍失败关闭。
- Host 只有 descriptor，没有 Review->Host 反向 import、真实 factory 注册或 composition root。
- production 仍为 NO-GO：REV-D11 Decision/Verdict/Policy/Evidence/Authority/Scope current、durable/remote Effect/Human/cleanup production conformance 与 composition root 未形成同一 current cut 的外部证明。
- 后续 live 复核：Runtime SQLite 已落 Binding、Policy/Authority/Scope journal 和 Evidence applicability fact port；Review 已有 exact Target/Assignment proof reader 与 `ExternalSourceV1`。但 Evidence public current reader 还需完整 EvidenceSubject Records/SourcePolicy/ProviderBinding/Presence/ConsumerAssociation current closure 和宿主 association，且仓内没有真实 production composition。故缺口已缩小但尚不是“只缺一层 Review adapter”。
- 唯一后续 Delta：external-current production qualification + composition root；Review 不提供私有 promotion seam。

## 真实验证

在 `ExecutionRuntime/review` 运行并通过：

```text
go test -race ./releasecandidate
go test ./releasecandidate -count=100
go test -race ./releasecandidate -count=20
go list -deps ./releasecandidate | rg 'ExecutionRuntime/(host|application|model-invoker)' && exit 1 || true
go test ./...
```

覆盖：unit、blackbox、fault、64 并发、TTL crossing、clock regression、artifact drift、typed nil、production fail-closed 与 import hygiene。

未执行 production root/真实 remote Provider/Human 平台端到端测试，因为对应 root 与外部 Owner 证明不存在；不得记录为通过。
