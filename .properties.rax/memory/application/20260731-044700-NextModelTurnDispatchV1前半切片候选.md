# NextModelTurnDispatchV1 前半切片候选

时间：2026-07-31 04:47:00 +08:00

## 粗粒度进展

基于 `origin/main@efffaf455d0d83b3879568e7af225f4ae2b08692`，Application Eligibility-only合同与Harness durable derived-binding/Inspect前半切片已在独立worktree形成未提交候选。

早审P0发现初稿越界镜像Model/Context/Tool DTO后，已立即原地删除。当前Application只绑定existing Eligibility request/projection、RequestedNotAfter、RequestDigest与stable DerivedDispatch；Harness SQLite只保存该中立ref/digest/TTL。

保留的S1/S2、clock/TTL、create-once/Inspect、lost reply、restart、cancel、corruption与64并发测试继续通过focused验证。Model、Runtime guard、Provider调用面均为0。

独立敌手审计后又完成三组P1返修：

- SQLite打开改为同锁先分类ledger与完整物理对象；仅全空fresh库原子migration，完整库verify-only，partial/弱schema/额外trigger一律Conflict且不repair；
- history/current逐字段一致并独立强制`attempt_bound`，IntegrityCheck与public Inspect拒绝重算digest后的伪`outcome_bound`、未知state和结构伪造；
- DerivedDispatch补齐Continuation Attempt、stable ID、ref digest与各坐标结构验真；写入、读取、Inspect合同与restart共用；
- mutation context绑定最小TTL，并在锁后、mutation前、commit前fresh-read clock；锁等待/事务延迟跨TTL与rollback均零写。

发布前P2证据补齐：Application/Harness测试从真实production source自动扫描imports，禁止Model、Runtime guard、Provider、Harness dispatch与Console依赖；原先无证据的默认零调用字段已删除。另补全新空SQLite库上64 mixed-payload同stable identity同时首创race，闭合单一durable winner/current。

## 当前决定

- Application不持有Model Prepared/current/InvocationMaterial或Context/Tool lineage；
- Application DerivedDispatch与未来Harness/Model身份不作等式推导；
- Harness sidecar仅为 `attempt_bound`；
- 未来Harness V2自行显式关联exact Model envelope；
- 不建立第二dispatcher/store权威。

## 阻塞

未合并 Harness ExactModelTurnPortV2、真实Model outcome binding、Runtime guard紧贴物理Provider调用、全路径no-bypass、production composition root、HA/SLA与独立审计均未完成；Application production dispatch继续 HARD-BLOCKED。
