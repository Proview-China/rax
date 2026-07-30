# Agent Activation V2 Conformance基线纠偏

## 事件

在最新`origin/main`复现
`TestAgentActivationV2ConformanceCandidateNeverClaimsProduction`失败。根因是同一旧提交
同时写入了互相矛盾的实现与测试：

- aggregate-only Conformance实现明确不把单个Coordination Fact当成Store原子Create或
  append-only CAS证明，因此`VersionClaimAtomicPayload=false`、
  `AppendOnlyHistory=false`；
- 测试却要求上述两个Store级证明为true；
- Committed Scope检查使用Go结构体直接比较，错误地把
  `SandboxLease`指针地址当成事实身份，持久化深拷贝后产生假阴性。

## 裁决

- 单聚合Conformance只证明八步闭包、invocation write-ahead、
  unknown inspect-only与Committed Scope治理值exact；
- Committed Scope统一复用Runtime公开`SameExecutionScopeV2`值语义；
- VersionClaim原子载荷与append-only history继续只能由Store/Owner测试证明，
  aggregate-only candidate保持false；
- `ProductionEligible`固定false，本次纠偏不解锁production Activation，不改变
  P0-P3、Owner current后端、Host Service V3或production root完成门。

## 验证

- Agent Activation V2 Conformance单轮通过；
- target ordinary `count=100`通过；
- target race `count=20`通过；
- Application full ordinary、full race与`go vet ./...`通过；
- Runtime full ordinary与`go vet ./...`通过。
