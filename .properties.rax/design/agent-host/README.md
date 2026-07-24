# Agent Host V1

## 1. 当前裁决

- 状态：设计已确认；H1-H3与HostV2 Start/Inspect参考纵切已实现并通过门禁。真实Activation、Stop Closure、全6+1 production root与H5仍为NO-GO。
- 目标：把 Definition、Assembler、Harness、Runtime/Application 和 6+1 的生产 adapters 组装成一个可启动、可检查、可停止的 Agent。
- 技术：首版核心与 Composition Root 使用 Go；CLI 只是同一 Host API 的薄入口。
- 当前事实：仓库还没有 production root，本设计不能被写成“已经可以一条命令启动”。

`agent-host` 只拥有依赖注入、进程级生命周期和宿主入口。它不拥有任何领域事实，不成为第二个 Runtime、Tool、Review、Memory 或 Sandbox Owner。

## 2. 唯一导入方向

```text
praxis-agent CLI / Host API
          |
          v
       agent-host
       /    |    \
Definition Assembler Harness public assembly
                    |
                    v
       Runtime public ports + Application public ports
                    |
                    v
     Owner public adapters/factories for all 6+1
                    |
                    v
      State Plane / Sandbox / Remote Providers
```

禁止依赖：Runtime `foundation`、任意 `fakes`、任意 `internal`、组件 testkit、Harness kernel 私有实现、裸 Provider SDK。

组件之间不得互相 import 实现包；只通过公共 Port 连接。CLI 不直连 Store、Kernel、Gateway 或 Provider。

## 3. 启动时序

```text
0  Validate config/schema/signature/secret refs       zero effect
1  Open production stores/services                    host resources
2  Decode + seal Definition; Assembler Resolve        zero provider calls
3  Harness Compile -> Generation/Manifest/Graph/Handoff
4  Runtime Binding/Association/Conformance             linear facts
5  Construct owner factories in graph order            no business dispatch
6  Wire Runtime gateways, Application coordinators,
   Harness bridges and all 6+1 owner adapters
7  Run exact-current/no-bypass/readiness checks
8  Publish Host Ready
9  Accept commands only through Host -> Application -> Runtime
```

停止按反向 DAG：停止接纳 -> fence -> reconcile unknown -> settle -> cleanup -> release Sandbox/Identity -> 关闭 stores/services。进程退出不等于 cleanup 成功。

## 4. CLI 与 Host API

```text
praxis-agent validate  --definition agent.yaml
praxis-agent assemble  --definition agent.yaml --output plan.json
praxis-agent run       --definition agent.yaml
praxis-agent inspect   --agent <identity-or-instance-ref>
praxis-agent stop      --agent <instance-ref>
```

CLI 和未来 API 共用同一 `HostV1`；不存在 CLI 特权旁路。`validate` 与 `assemble` 不启动 Sandbox/Provider。`run` 只有在所有 6+1 production readiness 通过后才可进入 Runtime Activation。

## 5. 6+1 放置与接线

| 域 | Host 注入对象 | 真实 Owner/运行位置 |
|---|---|---|
| continuity | checkpoint/restore/timeline public adapters | Runtime 协调；各组件 participant；State Plane 持久化 |
| tool + MCP | Tool/MCP gateway、surface、provider transport adapters | Tool/MCP Owner；actual runner 在受控执行点 |
| memory + knowledge | retrieve/candidate/commit/current adapters | 领域 Owner + State Plane |
| sandbox | Environment/lease/current/enforcement adapters | Sandbox Controller + Data Plane Enforcer |
| review | current reader、review request/verdict adapter | Review Owner，Host 不签 Verdict |
| context + cache | prepare/refresh/frame/cache adapters | Context Owner + State Plane |
| Harness | Assembly compiler/adapter、ExecutionPort bridge | Sandbox/Data Plane 或受控远程 surface |

Host 只选择已在 Resolved Plan/Graph 中绑定的 factory，不以配置中的任意包名、URL 或反射字符串加载代码。

## 6. Ready 的严格含义

`SystemReadyV1` 不是“进程能启动”，而是：

- Definition、Plan、Generation、Handoff、Binding/Association exact 串联；
- 6+1 全部 required release 为 production，current 且未过期；
- Sandbox production Environment Provider 已就绪；
- 所有治理 Gateway、Owner Current Reader、Inspect/Cleanup/Settlement 路径可访问；
- Provider 不存在 raw bypass，实际执行点会再次验权；
- production stores/services 已打开，未注入 fake/internal/testkit；
- 自定义组件仅使用已注册 public contracts；
- readiness 记录不授予 Effect、Permit、Outcome 或领域 Commit。

详细生命周期见 [composition-root-v1.md](composition-root-v1.md)，H3第一纵切Delta见 [h3-owner-adapter-delta-v1.md](h3-owner-adapter-delta-v1.md)，新增Port见 [port-deltas/h3-owner-current-readers-v1.md](port-deltas/h3-owner-current-readers-v1.md)，H4显式Activation/Association时序见 [h4-production-lifecycle-v2.md](h4-production-lifecycle-v2.md)。[Cleanup Closure V2](cleanup-closure-v2.md) 与 [Declarative Composition Root V1](declarative-composition-root-v1.md) 已完成独立设计复审（P0/P1=0），当前仍待用户设计审核，尚未授权Go实现。验收见 [acceptance.md](acceptance.md)，图见 [architecture.drawio](architecture.drawio)。

## 7. 进入 Plan 的门

- [x] 用户确认 Host API/CLI 和启动/停止时序；
- [x] 用户确认 `SystemReadyV1` 的全 6+1 硬门；
- [x] Definition/Assembler 设计同时确认；
- [ ] 各组件 production Release/Adapter 缺口进入对应 Owner Plan；
- [ ] 明确首个生产 State Plane、Provider 配置与 secret broker 的选择由后续计划决定，本设计不预选。
