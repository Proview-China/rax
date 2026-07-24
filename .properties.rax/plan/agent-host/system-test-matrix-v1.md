# Agent Host 全链系统测试矩阵 V1

## 1. 测试层级

| 层 | 允许 fixture | 证明什么 | 不证明什么 |
|---|---|---|---|
| L0 unit | 包内 fake | 单合同/状态机/canonical | 跨 Owner 或生产可用 |
| L1 owner integration | Owner test store/provider | Owner ports/current/recovery | production root |
| L2 public-port system | 只经公共 Port 的可控 fixture | Definition 到 Runtime/Harness/6+1 的组合语义 | 真实后端/SLA |
| L3 production smoke | production adapters/backends | root 可启动、最小任务、重启恢复 | 长期负载/SLA |
| L4 endurance/security | production-like | 长时间、并发、权限、资源和故障边界 | 未声明环境 |

## 2. 正向最小场景

```text
YAML Definition
-> sealed Definition
-> Resolved Plan + BindingPlan + AssemblyInput
-> Harness Generation/Handoff
-> Runtime Binding + Activation + Sandbox Ready
-> Context Frame
-> Model Prepared + predispatch gate
-> Review/Authority/Budget/Credential
-> Tool/MCP actual-point + settlement
-> Context refresh
-> Memory/Knowledge retrieve + governed candidate/commit
-> Checkpoint
-> new Instance/higher epoch Restore
-> Run settlement + Stop + Cleanup report
```

## 3. 硬反例

| 编号 | 反例 | 期望 |
|---|---|---|
| SYS-01 | YAML duplicate/alias/tag/unknown | Definition 前拒绝，零写 |
| SYS-02 | required 6+1 release 缺失/standalone | Resolve 拒绝，零 Assembly |
| SYS-03 | Catalog/Policy/Capability TTL crossing | S2/Ready fail closed |
| SYS-04 | 同 capability alias/multi provider | Conflict |
| SYS-05 | Sandbox unavailable/allocate unknown | quarantine + Inspect，不 Open Harness |
| SYS-06 | Review/Authority/Budget drift | Permit/dispatch 前拒绝 |
| SYS-07 | Provider pre-gate/raw bypass | provider calls=0，Conformance fail |
| SYS-08 | Tool/MCP lost reply | unknown + Inspect，不重复 effect |
| SYS-09 | Context refresh failure | 不推进 Continuation/Turn |
| SYS-10 | Memory/Knowledge commit lost reply | InspectCommit，不能二次正式写 |
| SYS-11 | checkpoint partial | 仅诊断，不声称可 Restore |
| SYS-12 | restore 原 Instance/epoch | 拒绝；必须新 Instance/更高 epoch |
| SYS-13 | Host crash after each lifecycle step | 按 journal+facts Inspect 恢复 |
| SYS-14 | 64 concurrent Start | 一次 Command/Activation/Instance |
| SYS-15 | stop cleanup unknown/residual | 分维度报告，冲突域不释放 |
| SYS-16 | custom component unknown required extension | fail closed |

## 4. 机械门

每个 Go 模块：

```text
targeted ordinary -count=100
targeted race -count=20
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
gofmt -l
import-boundary scan
git diff --check -- <owned scope>
```

Rust data plane：unit、property/fuzz、clippy、fmt、sanitizer/并发；TypeScript glue：typecheck、unit、integration、lockfile 与 bundle provenance。

## 5. Ready 裁决

| 最高通过层 | 可声明状态 |
|---|---|
| L0 | contract candidate |
| L1 | owner-local ready |
| L2 | system composition candidate |
| L3 | production smoke ready |
| L4 + 明确政策 | 对应环境下 SYSTEM_READY |

不得跳级，也不得把一次 test-only PASS 写成 production GO。
