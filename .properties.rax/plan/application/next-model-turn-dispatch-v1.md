# Application Next Model Turn Dispatch V1 实施计划候选

状态：早审 P0 原地重构完成的未提交候选，等待回审。

## 本轮产物

- [x] Eligibility-only `application/contract/next_model_turn_dispatch_v1.go`；
- [x] `StartOrInspectNextModelTurnV1` / `InspectNextModelTurnV1`公共Port；
- [x] Application import与行为conformance；
- [x] Harness Continuation S1/S2 binding；
- [x] Harness SQLite append-only history/current；
- [x] contract/store/restart/lost reply/64并发/cancel/corruption测试；
- [x] schema ledger与物理对象先分类、fresh-only原子migration、完整库verify-only；
- [x] history/current逐字段一致、attempt_bound-only与DerivedDispatch结构验真；
- [x] bounded mutation context及锁后/mutation前/commit前fresh clock；
- [x] missing table/index、partial no-ledger、weak schema、伪state、伪DerivedRef、TTL crossing敌手测试；
- [x] 自动production-import scanner覆盖Model/Runtime guard/Provider/Harness dispatch/Console禁止面；
- [x] 全新空库64 mixed-payload同stable identity首创race；
- [x] 同名design/plan/module/memory资产；
- [ ] 独立回审；
- [ ] Harness ExactModelTurnPortV2合并后的显式关联后半切片；
- [ ] production composition与no-bypass验收。

## P0 已删除

- [x] Application Prepared/current/InvocationMaterial/Authorization/SourceLineage DTO；
- [x] Application Context/Tool Owner与Kind literals；
- [x] RouteCall/Tool digest/Provider ordinal；
- [x] ExactInput与SQLite `exact_input_digest`；
- [x] 公开或持久化Harness Envelope、Model Attempt或Harness Dispatch身份；
- [x] 对未合并Harness V2私有实现的测试镜像。

## 保留验证

- canonical/strict JSON与固定public field set；
- S1/S2 Continuation exact current；
- fresh clock、最小TTL、`now == expiry`与rollback；
- pending/splice/unavailable/typed nil/cancel零写；
- lost reply原 exact ref、restart、corruption；
- schema no-silent-repair与live/restart伪事实拒绝；
- 锁等待/事务延迟跨TTL与commit前clock rollback零写；
- 64 same/different payload单winner；
- production source自动扫描证明Model/Runtime guard/Provider/Harness dispatch/Console禁止面不可达，负例不使用默认零计数；
- focused ordinary/race、full ordinary/vet、diff/import。

## 后半切片解锁条件

Harness V2公开合同合并后，由Harness Owner显式关联其 exact Model dispatch envelope与Application DerivedDispatchRef。真实Model outcome binding、Runtime guard紧贴物理Provider调用、全路径no-bypass和production root未完成前，Application production dispatch保持 HARD-BLOCKED。
