# Harness Component Release V1实施计划

状态：P4 owner-local候选及单节点durable backend候选已实现；production仍为NO-GO。

## 已完成

- [x] 复读Harness Assembly、Route、CommitGate与current-reader live资产；
- [x] 新增owner-owned `releasecandidate` Builder、Readiness、Conformance和Publisher；
- [x] 发布完整Manifest、Module、Capability、Port、Factory descriptor、Owner、Artifact、Certification、Evidence、TTL及九项Plan Artifact；
- [x] Factory固定为descriptor-only，不导出可执行构造器；
- [x] SupportMode固定`reference_only`，production promotion Fail Closed；
- [x] lost reply只exact Inspect，不重试mutation；
- [x] 覆盖64并发、typed-nil、TTL、clock rollback、drift、证据和Plan role反例；
- [x] 代码仅导入Agent Assembler/Harness/Runtime公共合同，不导入Host或其他Owner实现。

## Production残余

- [x] 单节点SQLite Session/Event、Assembly Publication、M2/A2 Assembly current、Route current/Owner Artifact与Model ACK Store已实现并完成ordinary/race/vet；它们是Harness Owner的durable backend候选，不等于deployment attestation、production SLA或跨进程后端；
- [ ] M2独立Handoff current Owner Reader/backend：live exact Handoff Ref缺少Assembly Publication按scope/history定位所需坐标，禁止用sidecar或第二真值补齐；
- [ ] Route真实双层wiring及全路径no-bypass Conformance；
- [ ] Model actual-point guard/inventory/receipt全路径审计；
- [ ] Tool Consumer、Application生产协调与Context Refresh/Continuation；
- [ ] 可执行Factory、Cleanup Conformance和deployment attestation；
- [ ] production composition root。

本计划不授权Harness修改Runtime、Application、Model、Tool、Context或Host实现。
