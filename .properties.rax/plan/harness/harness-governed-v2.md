# Harness Governed V2计划

状态：公共开发基座已完成；生产Provider与部署级后端继续演进。

- [x] Endpoint/Session/Candidate/Pending Action/Input公共合同；
- [x] Session集中状态迁移与value-semantic Scope校验；
- [x] Session/Candidate Fact Port及线程安全CAS fake；
- [x] Create/CAS lost reply exact Inspect；
- [x] 100路并发单Session transition；
- [x] 自定义同Component多Capability边界；
- [x] 初始Candidate与Action continuation可恢复准备；
- [x] conformance candidate不自授生产/dispatch/Outcome；
- [x] Session阶段绑定Runtime GovernedExecutionAttemptRefs，裸CAS无治理链不能进入in-flight/action/input/terminal；
- [x] Runtime GovernedExecutionPort host relay；Prepare/Execute回包丢失只做本地Inspect，绝不重派；
- [x] 自定义Data Provider Conformance；并发Prepare create-once、精确本地Inspect、Execute单次调用且不自授任何治理资格；
- [x] Relay恶意换Permit/payload/attempt/provider/delegation反例及context丢回包恢复；
- [x] TurnState同ref换sidecar/claim/时间冲突、64路并发单次线性化与裸CAS越权反例；
- [x] Observation→waiting_settlement→ApplySettledTurn门禁；Action/Input/Claim只能由exact Settlement绑定的规范化DomainResult派生；
- [x] Application V3真实Domain Adapter；严格ModelTurn Effect解码、持久Binding Fact/Port、exact endpoint/session/relay/provider/candidate交叉校验；
- [x] Application前置Domain Reservation；Session+Candidate单赢家、Session CAS与Reservation Fact同Owner原子提交、lost reply Inspect、首次current与历史过期恢复分离；
- [x] Harness核心Contract去除Application反向依赖；桥类型迁移至`bridgecontract`，合法namespaced自定义Step通过同一公共Port且不增加kind switch；
- [x] pre-prepared unknown failed-only与post-prepared unknown独立Inspect provenance；回包丢失只Inspect Session/Binding；
- [x] Binding按状态重算Application Basis，拒绝内部Observation/Authorization/Settlement/DomainResult换包；
- [x] 真实Application Coordinator+Domain Router+Harness Adapter跨模块Observation/Settlement黑盒闭环；
- [x] Harness Data Provider Prepare→Enforcement→ExecutePrepared→Inspect公共合同、参考接线与故障验收；
- [ ] Model Invoker独立Provider bridge；
- [ ] Cancel与Close独立Operation Effect；
- [x] Application公共Run Coordinator的Claim Evidence→Runtime Settlement→terminal cleanup/Reconcile协调合同；
- [x] 本切面故障矩阵、并发、race/vet与跨模块验收；
- [x] Runtime/Application/Harness认证执行组合闭环：正常完成、Claim lost-reply恢复、unknown只Inspect不重派；
- [ ] 真实Provider/生产持久后端接入后的部署级故障与长期验收。

完成后产物仍是公共Harness基座和接线协议，不是具体Codex/Claude/ACP生产实现，也不选择数据库/RPC/Scheduler/SLA。
