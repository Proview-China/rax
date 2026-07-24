# Memory/Knowledge Delta 10/11中立合同候选

时间：2026-07-16 21:08（Asia/Shanghai）

## 阶段变化

Memory/Knowledge Owner已完成本模块侧Delta 10/11联合设计候选。该事件只记录设计/计划状态，不代表Context、Application或Harness已接受公共合同，也不授权跨Owner Go、production root或远程Retrieval。

## 已冻结候选

- Memory与Knowledge保持两个独立Owner，各自发布nominal neutral Coordinate/Current/Item/ExactContent refs要求，继续复用Owner-local `InspectAttempt/InspectForTurn/ReadContentExact`三方法；
- neutral coordinate必须绑定完整Identity exact ref/epoch、ExecutionScope、Run、neutral Session与目标Turn；Adapter不得从字符串或digest补造字段；
- 固定可见性链：`Owner Reader S1 -> Context pending DomainResult/候选Frame不可见 -> Owner Reader S2 -> Context本地原子ApplySettlement+Generation current CAS -> exact Frame`；
- Memory/Knowledge只提供Owner current projection与exact content observation，不创建Context Candidate、Fragment Fact、DomainResult、Frame、Generation或Continuation；
- Application只协调attempt和exact refs；Harness只消费Context current exact Frame；
- Knowledge向Context Owner提交加法kind候选`knowledge_reference`，不得冒充`memory_recall`、Artifact或Instruction；
- TTL使用Owner锁域内fresh clock，`now >= expires`拒绝；caller时间只作上界；S2必须绑定S1 ClosureDigest和同一Run/Session/Turn/Item集合；
- closed reason使用固定枚举，失败返回零body；远程Gateway、Provider、Resolver与production root仍unsupported/NO-GO。

## 资产

- `design/memory-engine/context-refresh-neutral-delta10-11-v1.md`
- `design/knowledge-engine/context-refresh-neutral-delta10-11-v1.md`
- `plan/memory-knowledge/port-delta.md`
- `plan/memory-knowledge/acceptance-test-matrix.md`
- `module/memory-knowledge/README.md`

## 联合Owner门禁

### P0

1. Context Owner接受并发布Knowledge专属fragment kind及Recipe/strict Conformance；
2. 两个Owner完整Identity exact ref/epoch、Session与CurrentTurn/NextTurn nominal映射闭合；
3. Application公共Context Refresh Port、Context原子Apply+Generation CAS、exact Frame Inspect与跨模块fixture闭合。

### P1

1. neutral DTO公开包、ClosedReason映射、N/bytes/tokens/Estimator上限；
2. Context Recipe中的Owner顺序、Knowledge Region/Role/Required/Trust、Residual策略。

### P2

1. diagnostics、Citation/Residual展示及观测字段；不得泄露受限内容或改变Owner事实。

当前裁决：本模块候选**可联合评审**；P0未清零前跨Owner实现与非零G6B来源**NO-GO**。
