# Runtime Evidence Ledger V2模块说明

## 作用

该模块提供单主、可恢复、可审计的执行证据账本，并把Harness/组件产生的Observation、Receipt、Attestation、Claim与领域权威Fact严格分层。它不解释组件业务正文，也不把Claim直接提升为Runtime Outcome。

## 代码组成

- 公共Evidence/Source/Policy/Ledger/Tombstone合同：`ports/evidence_v2.go`
- V2 Claim精确侧车合同：`ports/run_claim_v2.go`
- Source状态机、Record摘要链与Append校验：`control/evidence_ledger_v2.go`
- Application侧唯一治理入口：`control/evidence_governance_v2.go`
- 测试Ledger Fact Owner：`fakes/evidence_ledger_v2.go`
- 测试Claim Association Fact Owner：`fakes/run_claim_association_v2.go`
- 自定义Source Conformance：`conformance/evidence_v2.go`
- V2 Claim摄取与恢复：`kernel/run_claim_gateway_v2.go`

## 可预期行为

- Source cursor和Record在一个Fact Owner事务语义中同时提交；
- 同source key同内容幂等，换内容Conflict；strict gap不消耗ledger sequence；
- Ledger Owner独占全局sequence，Timeline只读投影；
- Source续租可推进精确current治理水位，不能偷换Scope、Owner、能力或配置；
- claim和authoritative资格只来自独立current Policy，不来自Source自报；
- authoritative payload必须与独立Owner Fact Inspector返回的schema/content digest/revision完全一致；
- late evidence只引用更旧的精确V2 Record，不能成为Run/Effect权威输入；
- Run Claim先落V2 Record，再落create-once Association；任一步丢回包都Inspect，不盲重派；
- Association只绑定Claim与Run，不完成Run、不产生Outcome。

## 公共接入面

组件runtime-adapter只可依赖`runtime/core`与`runtime/ports`中的V2类型和Port。Application使用`EvidenceGovernancePortV2`及`RunClaimAssociationPortV2`；不得直接把raw `EvidenceLedgerFactPortV2`当治理入口，也不得依赖`control`、`kernel`、`foundation`或`fakes`实现。

## Live capability caveat

1. 当前所有Evidence分区都要求CurrentScope含active running/stopping Run，尚不支持pre-run tenant/identity证据；
2. Gateway对Binding、Authority、Policy、Run/Effect的复读与Ledger append不是生产跨Store原子事务；Record保存精确水位，漂移依靠重读与reconcile处理；
3. `fakes`只证明状态机、线性化与故障恢复语义，不宣称生产持久性、分布式一致性或SLA；
4. V1 Evidence/Timeline保持legacy restricted，无V1字段自动升级、无V1/V2双写；
5. 当前未选择生产Backend、RPC、Scheduler、签名、retention或Watch SLA。
